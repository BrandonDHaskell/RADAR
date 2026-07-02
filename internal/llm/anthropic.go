package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	anthropicVersion = "2023-06-01"
	maxAttempts      = 3
)

// anthropicEndpoint is a var, not a const, so tests in this package can
// point it at an httptest server instead of the real Claude API.
var anthropicEndpoint = "https://api.anthropic.com/v1/messages"

// AnthropicProvider implements Provider using the Claude Messages API,
// constraining the response to Verdict's shape via structured outputs
// (output_config.format), which Haiku 4.5 and later models support.
type AnthropicProvider struct {
	apiKey    string
	model     string
	maxTokens int
	http      *http.Client
}

// NewAnthropicProvider returns an AnthropicProvider using apiKey and model.
func NewAnthropicProvider(apiKey, model string) *AnthropicProvider {
	return &AnthropicProvider{
		apiKey:    apiKey,
		model:     model,
		maxTokens: 1024,
		http:      &http.Client{Timeout: 60 * time.Second},
	}
}

var verdictSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"verdict": map[string]any{
			"type": "string",
			"enum": []string{"pursue", "stretch", "skip"},
		},
		"matched_role_tag": map[string]any{
			"type": "string",
			"enum": []string{
				"business-systems-analyst",
				"implementation-specialist",
				"technical-program-manager",
				"technical-support-engineer",
				"automation-engineer",
			},
		},
		"reasoning": map[string]any{"type": "string"},
		"score":     map[string]any{"type": "number"},
	},
	"required":             []string{"verdict", "matched_role_tag", "reasoning"},
	"additionalProperties": false,
}

type anthropicRequest struct {
	Model        string                `json:"model"`
	MaxTokens    int                   `json:"max_tokens"`
	System       string                `json:"system,omitempty"`
	Messages     []anthropicMessage    `json:"messages"`
	OutputConfig anthropicOutputConfig `json:"output_config"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicOutputConfig struct {
	Format anthropicFormat `json:"format"`
}

type anthropicFormat struct {
	Type   string         `json:"type"`
	Schema map[string]any `json:"schema"`
}

type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
}

// FitVerdict asks Claude to assess fit for the posting/profile combination
// described by systemPrompt and userPrompt, retrying transient failures
// (network errors, 429, 5xx) with backoff.
func (p *AnthropicProvider) FitVerdict(ctx context.Context, systemPrompt, userPrompt string) (*Verdict, error) {
	reqBody, err := json.Marshal(anthropicRequest{
		Model:     p.model,
		MaxTokens: p.maxTokens,
		System:    systemPrompt,
		Messages:  []anthropicMessage{{Role: "user", Content: userPrompt}},
		OutputConfig: anthropicOutputConfig{
			Format: anthropicFormat{Type: "json_schema", Schema: verdictSchema},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling anthropic request: %w", err)
	}

	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		text, retryable, err := p.doRequest(ctx, reqBody)
		if err == nil {
			return parseVerdict(text)
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
	return nil, fmt.Errorf("requesting fit verdict: %w", lastErr)
}

func (p *AnthropicProvider) doRequest(ctx context.Context, reqBody []byte) (text string, retryable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicEndpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := p.http.Do(req)
	if err != nil {
		return "", true, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, err
	}

	if resp.StatusCode != http.StatusOK {
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return "", retryable, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", false, fmt.Errorf("parsing anthropic response: %w", err)
	}
	if parsed.StopReason == "refusal" {
		return "", false, fmt.Errorf("model declined to respond (stop_reason: refusal)")
	}
	for _, block := range parsed.Content {
		if block.Type == "text" {
			return block.Text, false, nil
		}
	}
	return "", false, fmt.Errorf("no text content in response (stop_reason: %s)", parsed.StopReason)
}

// parseVerdict defensively parses the model's JSON output. Structured
// outputs should return clean JSON with no markdown fences, but this
// strips them anyway as a safety net, per the project spec's requirement
// to parse defensively rather than trust the provider unconditionally.
func parseVerdict(text string) (*Verdict, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var v Verdict
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return nil, fmt.Errorf("parsing verdict JSON: %w", err)
	}
	if v.Verdict == "" || v.MatchedRoleTag == "" {
		return nil, fmt.Errorf("verdict missing required fields")
	}
	return &v, nil
}
