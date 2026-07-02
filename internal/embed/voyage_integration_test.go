package embed

import (
	"context"
	"os"
	"testing"
)

func TestVoyageProviderEmbedRealAPI(t *testing.T) {
	apiKey := os.Getenv("VOYAGE_API_KEY")
	if apiKey == "" {
		t.Skip("VOYAGE_API_KEY not set; skipping integration test")
	}

	provider := NewVoyageProvider(apiKey, "voyage-4", 1024)
	vectors, err := provider.Embed(context.Background(), []string{
		"Senior Software Engineer, backend infrastructure",
		"Staff Product Designer, growth team",
	}, InputTypeDocument)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(vectors) != 2 {
		t.Fatalf("got %d vectors, want 2", len(vectors))
	}
	for i, v := range vectors {
		if len(v) != 1024 {
			t.Errorf("vectors[%d] has %d dimensions, want 1024", i, len(v))
		}
	}
	if vectors[0][0] == vectors[1][0] && vectors[0][1] == vectors[1][1] {
		t.Error("two different texts produced suspiciously identical embeddings")
	}
}
