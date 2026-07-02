package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ExcludedPosting is one row of the operator's Stage 0 false-negative
// review (`radar excluded`).
type ExcludedPosting struct {
	CompanyName string
	Title       string
	Reason      string
}

// ExcludedPostings returns the most recently excluded open postings, for
// the operator to audit Stage 0 (and Stage 1, once implemented) for wrong
// exclusions.
func ExcludedPostings(ctx context.Context, pool *pgxpool.Pool, limit int) ([]ExcludedPosting, error) {
	rows, err := pool.Query(ctx, `
		SELECT c.name, p.title, coalesce(p.screen_reason, '')
		FROM postings p JOIN companies c ON c.id = p.company_id
		WHERE p.is_open = true AND p.screen_status = 'excluded'
		ORDER BY p.updated_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("querying excluded postings: %w", err)
	}
	defer rows.Close()

	var result []ExcludedPosting
	for rows.Next() {
		var e ExcludedPosting
		if err := rows.Scan(&e.CompanyName, &e.Title, &e.Reason); err != nil {
			return nil, fmt.Errorf("scanning excluded posting: %w", err)
		}
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("querying excluded postings: %w", err)
	}
	return result, nil
}
