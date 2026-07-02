package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// EmbeddingCandidate identifies an open, screened-in posting whose
// embedding is missing or stale, along with the fields needed to build its
// embedding text.
type EmbeddingCandidate struct {
	PostingID   int64
	CompanyName string
	Title       string
	Department  string
	Location    string
	Description string
	ContentHash string
}

// EmbeddingCandidates returns open, screened-in postings (Stage 2) that
// have no posting_embeddings row, or whose stored embedding's content_hash
// no longer matches the posting's current content_hash. An embedding is
// current if and only if that hash matches, so this query is
// self-healing: a posting whose embedding call previously failed simply
// stays selected until it succeeds.
func EmbeddingCandidates(ctx context.Context, pool *pgxpool.Pool) ([]EmbeddingCandidate, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.id, c.name, p.title, coalesce(p.department, ''), coalesce(p.location, ''), coalesce(p.description, ''), p.content_hash
		FROM postings p
		JOIN companies c ON c.id = p.company_id
		LEFT JOIN posting_embeddings pe ON pe.posting_id = p.id
		WHERE p.is_open = true AND p.screen_status = 'passed'
		  AND (pe.posting_id IS NULL OR pe.content_hash IS DISTINCT FROM p.content_hash)
	`)
	if err != nil {
		return nil, fmt.Errorf("finding embedding candidates: %w", err)
	}
	defer rows.Close()

	var result []EmbeddingCandidate
	for rows.Next() {
		var c EmbeddingCandidate
		if err := rows.Scan(&c.PostingID, &c.CompanyName, &c.Title, &c.Department, &c.Location, &c.Description, &c.ContentHash); err != nil {
			return nil, fmt.Errorf("scanning embedding candidate: %w", err)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finding embedding candidates: %w", err)
	}
	return result, nil
}

// UpsertPostingEmbedding stores or replaces the embedding for postingID.
// model records which embedding model produced the vector, so a future
// model change is visible per-row rather than silently mixed. contentHash
// is the posting's content_hash at embedding time, so Stage 2 can later
// tell a current embedding from a stale one without recomputing anything.
func UpsertPostingEmbedding(ctx context.Context, pool *pgxpool.Pool, postingID int64, embedding []float32, model, contentHash string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO posting_embeddings (posting_id, embedding, model, content_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (posting_id) DO UPDATE SET
			embedding    = EXCLUDED.embedding,
			model        = EXCLUDED.model,
			content_hash = EXCLUDED.content_hash,
			created_at   = now()
	`, postingID, pgvector.NewVector(embedding), model, contentHash)
	if err != nil {
		return fmt.Errorf("upserting embedding for posting %d: %w", postingID, err)
	}
	return nil
}
