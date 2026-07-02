package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// withTestEndpoint points voyageEndpoint at server for the duration of the
// test, restoring the real endpoint afterward.
func withTestEndpoint(t *testing.T, server *httptest.Server) {
	t.Helper()
	original := voyageEndpoint
	voyageEndpoint = server.URL
	t.Cleanup(func() { voyageEndpoint = original })
}

func TestVoyageProviderEmbedSendsExpectedRequest(t *testing.T) {
	var gotAuth, gotContentType string
	var gotReq voyageRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}

		resp := voyageResponse{}
		for i := range gotReq.Input {
			resp.Data = append(resp.Data, struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{Embedding: make([]float32, gotReq.OutputDimension), Index: i})
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()
	withTestEndpoint(t, server)

	provider := NewVoyageProvider("test-key", "voyage-4", 1024)
	vectors, err := provider.Embed(context.Background(), []string{"hello", "world"}, InputTypeDocument)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if gotAuth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer test-key")
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type header = %q, want %q", gotContentType, "application/json")
	}
	if gotReq.Model != "voyage-4" {
		t.Errorf("request Model = %q, want %q", gotReq.Model, "voyage-4")
	}
	if gotReq.InputType != "document" {
		t.Errorf("request InputType = %q, want %q", gotReq.InputType, "document")
	}
	if gotReq.OutputDimension != 1024 {
		t.Errorf("request OutputDimension = %d, want 1024", gotReq.OutputDimension)
	}
	if len(vectors) != 2 || len(vectors[0]) != 1024 || len(vectors[1]) != 1024 {
		t.Errorf("vectors = %d results, want 2 of length 1024", len(vectors))
	}
}

func TestVoyageProviderEmbedChunksLargeBatches(t *testing.T) {
	var requestCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requestCount, 1)
		var req voyageRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := voyageResponse{}
		for i := range req.Input {
			resp.Data = append(resp.Data, struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{Embedding: make([]float32, req.OutputDimension), Index: i})
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()
	withTestEndpoint(t, server)

	texts := make([]string, maxBatchSize+1)
	for i := range texts {
		texts[i] = "text"
	}

	provider := NewVoyageProvider("test-key", "voyage-4", 8)
	vectors, err := provider.Embed(context.Background(), texts, InputTypeDocument)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != len(texts) {
		t.Errorf("got %d vectors, want %d", len(vectors), len(texts))
	}
	if got := atomic.LoadInt32(&requestCount); got != 2 {
		t.Errorf("requestCount = %d, want 2 (one per batch)", got)
	}
}

func TestVoyageProviderEmbedRejectsWrongDimension(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := voyageResponse{}
		resp.Data = append(resp.Data, struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}{Embedding: make([]float32, 7), Index: 0}) // wrong dimension
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()
	withTestEndpoint(t, server)

	provider := NewVoyageProvider("test-key", "voyage-4", 1024)
	_, err := provider.Embed(context.Background(), []string{"hello"}, InputTypeDocument)
	if err == nil {
		t.Fatal("Embed: got nil error, want a dimension-mismatch error")
	}
	if !strings.Contains(err.Error(), "dimension") {
		t.Errorf("error = %q, want it to mention dimension", err.Error())
	}
}

func TestVoyageProviderEmbedRetriesOn500(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var req voyageRequest
		json.NewDecoder(r.Body).Decode(&req)
		resp := voyageResponse{Data: []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		}{{Embedding: make([]float32, req.OutputDimension), Index: 0}}}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()
	withTestEndpoint(t, server)

	provider := NewVoyageProvider("test-key", "voyage-4", 4)
	_, err := provider.Embed(context.Background(), []string{"hello"}, InputTypeQuery)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got != 2 {
		t.Errorf("attempts = %d, want 2", got)
	}
}
