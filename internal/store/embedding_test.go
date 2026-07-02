package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestUpsertPostingEmbedding(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-embedding-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name:     "Embedding Test Co",
		ATSType:  "greenhouse",
		ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID)
	})

	posting, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID:    company.ID,
		Source:       "greenhouse",
		ExternalID:   "ext-1",
		Title:        "Software Engineer",
		CanonicalKey: "embedding test co|software engineer|",
		ContentHash:  "hash-v1",
	})
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	vecA := make([]float32, 1024)
	vecA[0] = 0.5
	if err := store.UpsertPostingEmbedding(ctx, pool, posting.ID, vecA, "voyage-4", "hash-v1"); err != nil {
		t.Fatalf("UpsertPostingEmbedding: %v", err)
	}

	var model, contentHash string
	var dims int
	if err := pool.QueryRow(ctx,
		"SELECT model, vector_dims(embedding), content_hash FROM posting_embeddings WHERE posting_id = $1",
		posting.ID,
	).Scan(&model, &dims, &contentHash); err != nil {
		t.Fatalf("querying posting_embeddings: %v", err)
	}
	if model != "voyage-4" {
		t.Errorf("model = %q, want %q", model, "voyage-4")
	}
	if dims != 1024 {
		t.Errorf("vector_dims = %d, want 1024", dims)
	}
	if contentHash != "hash-v1" {
		t.Errorf("content_hash = %q, want %q", contentHash, "hash-v1")
	}

	// Re-upserting should replace the row, not duplicate it.
	vecB := make([]float32, 1024)
	vecB[1] = 0.9
	if err := store.UpsertPostingEmbedding(ctx, pool, posting.ID, vecB, "voyage-4", "hash-v2"); err != nil {
		t.Fatalf("UpsertPostingEmbedding (replace): %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx,
		"SELECT count(*) FROM posting_embeddings WHERE posting_id = $1", posting.ID,
	).Scan(&count); err != nil {
		t.Fatalf("counting posting_embeddings: %v", err)
	}
	if count != 1 {
		t.Errorf("posting_embeddings row count = %d, want 1 (upsert should replace, not duplicate)", count)
	}

	var contentHashAfterReplace string
	if err := pool.QueryRow(ctx,
		"SELECT content_hash FROM posting_embeddings WHERE posting_id = $1", posting.ID,
	).Scan(&contentHashAfterReplace); err != nil {
		t.Fatalf("querying content_hash after replace: %v", err)
	}
	if contentHashAfterReplace != "hash-v2" {
		t.Errorf("content_hash after replace = %q, want %q", contentHashAfterReplace, "hash-v2")
	}

	// Cosine distance to itself should be ~0.
	var selfDistance float64
	if err := pool.QueryRow(ctx,
		"SELECT embedding <=> embedding FROM posting_embeddings WHERE posting_id = $1", posting.ID,
	).Scan(&selfDistance); err != nil {
		t.Fatalf("querying cosine distance: %v", err)
	}
	if selfDistance > 0.0001 {
		t.Errorf("self cosine distance = %v, want ~0", selfDistance)
	}
}
