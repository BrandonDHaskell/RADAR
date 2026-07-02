package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ScreenCandidate is an open posting whose Stage 0 screen is pending, or
// was last computed against a different profile version.
type ScreenCandidate struct {
	ID       int64
	Title    string
	Location string
	IsRemote bool
}

// ScreenCandidates returns open postings that still need a Stage 0
// screening decision under profileHash: never screened, or screened
// against a different profile.
func ScreenCandidates(ctx context.Context, pool *pgxpool.Pool, profileHash string) ([]ScreenCandidate, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, title, coalesce(location, ''), is_remote FROM postings
		WHERE is_open = true
		  AND (screen_status = 'pending' OR screen_profile_hash IS DISTINCT FROM $1)
	`, profileHash)
	if err != nil {
		return nil, fmt.Errorf("finding screen candidates: %w", err)
	}
	defer rows.Close()

	var result []ScreenCandidate
	for rows.Next() {
		var c ScreenCandidate
		if err := rows.Scan(&c.ID, &c.Title, &c.Location, &c.IsRemote); err != nil {
			return nil, fmt.Errorf("scanning screen candidate: %w", err)
		}
		result = append(result, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("finding screen candidates: %w", err)
	}
	return result, nil
}

// SetPostingScreen records a Stage 0 screening decision for a posting.
// reason is nil on pass; on exclusion it is a short machine-readable tag
// such as "title_exclusion:<phrase>" or "location", surfaced verbatim by
// `radar excluded`.
func SetPostingScreen(ctx context.Context, pool *pgxpool.Pool, postingID int64, status string, reason *string, profileHash string) error {
	_, err := pool.Exec(ctx, `
		UPDATE postings
		SET screen_status = $2, screen_reason = $3, screen_profile_hash = $4, updated_at = now()
		WHERE id = $1
	`, postingID, status, reason, profileHash)
	if err != nil {
		return fmt.Errorf("setting screen status for posting %d: %w", postingID, err)
	}
	return nil
}
