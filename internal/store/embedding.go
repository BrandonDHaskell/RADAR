package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
)

// PostingToEmbed identifies an open posting that currently has no row in
// posting_embeddings, along with the fields needed to build its embedding
// text.
type PostingToEmbed struct {
	PostingID   int64
	CompanyName string
	Title       string
	Department  string
	Location    string
	Description string
}

// PostingsMissingEmbedding returns open postings for companyID/source that
// have no posting_embeddings row yet. dedup.Sync only re-queues postings
// whose content changed, so a posting whose embedding call previously
// failed (network error, rate limit, provider outage) would otherwise
// never be retried; callers should embed whatever this returns after
// handling dedup's own change-driven candidates.
func PostingsMissingEmbedding(ctx context.Context, pool *pgxpool.Pool, companyID int64, source string) ([]PostingToEmbed, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.id, c.name, p.title, coalesce(p.department, ''), coalesce(p.location, ''), coalesce(p.description, '')
		FROM postings p
		JOIN companies c ON c.id = p.company_id
		LEFT JOIN posting_embeddings pe ON pe.posting_id = p.id
		WHERE p.company_id = $1 AND p.source = $2 AND p.is_open = true AND pe.posting_id IS NULL
	`, companyID, source)
	if err != nil {
		return nil, fmt.Errorf("finding postings missing embeddings for company %d/%s: %w", companyID, source, err)
	}
	defer rows.Close()

	var result []PostingToEmbed
	for rows.Next() {
		var p PostingToEmbed
		if err := rows.Scan(&p.PostingID, &p.CompanyName, &p.Title, &p.Department, &p.Location, &p.Description); err != nil {
			return nil, fmt.Errorf("scanning posting missing embedding: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finding postings missing embeddings for company %d/%s: %w", companyID, source, err)
	}
	return result, nil
}

// UpsertPostingEmbedding stores or replaces the embedding for postingID.
// model records which embedding model produced the vector, so a future
// model change is visible per-row rather than silently mixed.
func UpsertPostingEmbedding(ctx context.Context, pool *pgxpool.Pool, postingID int64, embedding []float32, model string) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO posting_embeddings (posting_id, embedding, model)
		VALUES ($1, $2, $3)
		ON CONFLICT (posting_id) DO UPDATE SET
			embedding  = EXCLUDED.embedding,
			model      = EXCLUDED.model,
			created_at = now()
	`, postingID, pgvector.NewVector(embedding), model)
	if err != nil {
		return fmt.Errorf("upserting embedding for posting %d: %w", postingID, err)
	}
	return nil
}
