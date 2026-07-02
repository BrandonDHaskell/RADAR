package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	// maxBatchSize keeps request bodies modest; well under Voyage's
	// documented 1,000-item list limit.
	maxBatchSize = 128
	maxAttempts  = 3
)

// voyageEndpoint is a var, not a const, so tests in this package can point
// it at an httptest server instead of the real Voyage API.
var voyageEndpoint = "https://api.voyageai.com/v1/embeddings"

// VoyageProvider implements Provider using the Voyage AI embeddings API
// (https://api.voyageai.com/v1/embeddings).
type VoyageProvider struct {
	apiKey    string
	model     string
	dimension int
	http      *http.Client
}

// NewVoyageProvider returns a VoyageProvider using apiKey and model,
// explicitly requesting dimension-sized output vectors on every call so the
// result always matches the configured pgvector column.
func NewVoyageProvider(apiKey, model string, dimension int) *VoyageProvider {
	return &VoyageProvider{
		apiKey:    apiKey,
		model:     model,
		dimension: dimension,
		http:      &http.Client{Timeout: 60 * time.Second},
	}
}

// Dimension returns the configured output dimension.
func (v *VoyageProvider) Dimension() int { return v.dimension }

// Embed embeds texts, chunking into batches of at most maxBatchSize to stay
// well under Voyage's per-request list and token limits.
func (v *VoyageProvider) Embed(ctx context.Context, texts []string, inputType InputType) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	result := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += maxBatchSize {
		end := min(start+maxBatchSize, len(texts))
		batch, err := v.embedBatch(ctx, texts[start:end], inputType)
		if err != nil {
			return nil, err
		}
		result = append(result, batch...)
	}
	return result, nil
}

type voyageRequest struct {
	Input           []string `json:"input"`
	Model           string   `json:"model"`
	InputType       string   `json:"input_type"`
	OutputDimension int      `json:"output_dimension"`
}

type voyageResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

// embedBatch embeds a single batch, retrying transient failures (network
// errors, 429, 5xx) with backoff.
func (v *VoyageProvider) embedBatch(ctx context.Context, texts []string, inputType InputType) ([][]float32, error) {
	reqBody, err := json.Marshal(voyageRequest{
		Input:           texts,
		Model:           v.model,
		InputType:       string(inputType),
		OutputDimension: v.dimension,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling voyage request: %w", err)
	}

	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		vectors, retryable, err := v.doEmbedBatch(ctx, reqBody, len(texts))
		if err == nil {
			return vectors, nil
		}
		lastErr = err
		if !retryable || attempt == maxAttempts {
			break
		}
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		backoff *= 2
	}
	return nil, fmt.Errorf("embedding %d texts via voyage: %w", len(texts), lastErr)
}

func (v *VoyageProvider) doEmbedBatch(ctx context.Context, reqBody []byte, wantCount int) (vectors [][]float32, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, voyageEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.apiKey)

	resp, err := v.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return nil, retryable, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var parsed voyageResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false, fmt.Errorf("parsing voyage response: %w", err)
	}
	if len(parsed.Data) != wantCount {
		return nil, false, fmt.Errorf("voyage returned %d embeddings for %d inputs", len(parsed.Data), wantCount)
	}

	vectors = make([][]float32, wantCount)
	for _, d := range parsed.Data {
		if d.Index < 0 || d.Index >= wantCount {
			return nil, false, fmt.Errorf("voyage returned out-of-range index %d for %d inputs", d.Index, wantCount)
		}
		if len(d.Embedding) != v.dimension {
			return nil, false, fmt.Errorf("voyage returned a %d-dimension embedding, want %d (check embedding.dimension in config)", len(d.Embedding), v.dimension)
		}
		vectors[d.Index] = d.Embedding
	}
	return vectors, false, nil
}
