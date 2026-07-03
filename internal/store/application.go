package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ApplicationCloseStatuses are the terminal statuses CloseApplication accepts.
var ApplicationCloseStatuses = []string{"closed_offer", "closed_rejected", "withdrawn"}

// ErrApplicationNotFound is returned when an application id does not exist.
var ErrApplicationNotFound = errors.New("application not found")

// ErrPostingNotFound is returned when a posting id does not exist.
var ErrPostingNotFound = errors.New("posting not found")

// Application is a row in the applications table.
type Application struct {
	ID               int64
	PostingID        int64
	Status           string
	AppliedAt        *time.Time
	ResumeVariant    string
	UsedCoverLetter  bool
	NextFollowUpDate *time.Time
	Notes            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ApplyToPosting marks postingID applied: it creates the application if none
// exists yet, or updates it in place if the operator re-runs apply to
// correct the resume variant, cover letter flag, or follow-up date.
// applied_at is set once, on first insert, and preserved across re-runs.
func ApplyToPosting(ctx context.Context, pool *pgxpool.Pool, postingID int64, resumeVariant string, usedCoverLetter bool, nextFollowUpDate *time.Time) (*Application, error) {
	a := &Application{}
	err := pool.QueryRow(ctx, `
		INSERT INTO applications (posting_id, status, applied_at, resume_variant, used_cover_letter, next_follow_up_date)
		VALUES ($1, 'applied', now(), NULLIF($2, ''), $3, $4)
		ON CONFLICT (posting_id) DO UPDATE SET
			status               = 'applied',
			resume_variant       = NULLIF($2, ''),
			used_cover_letter    = $3,
			next_follow_up_date  = $4,
			updated_at           = now()
		RETURNING id, posting_id, status, applied_at, coalesce(resume_variant, ''), used_cover_letter,
		          next_follow_up_date, coalesce(notes, ''), created_at, updated_at
	`, postingID, resumeVariant, usedCoverLetter, nextFollowUpDate).Scan(
		&a.ID, &a.PostingID, &a.Status, &a.AppliedAt, &a.ResumeVariant, &a.UsedCoverLetter,
		&a.NextFollowUpDate, &a.Notes, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, ErrPostingNotFound
		}
		return nil, fmt.Errorf("applying to posting %d: %w", postingID, err)
	}
	return a, nil
}

// CloseApplication transitions an application to a terminal status
// (one of ApplicationCloseStatuses).
func CloseApplication(ctx context.Context, pool *pgxpool.Pool, applicationID int64, status string) (*Application, error) {
	a := &Application{}
	err := pool.QueryRow(ctx, `
		UPDATE applications
		SET status = $2, updated_at = now()
		WHERE id = $1
		RETURNING id, posting_id, status, applied_at, coalesce(resume_variant, ''), used_cover_letter,
		          next_follow_up_date, coalesce(notes, ''), created_at, updated_at
	`, applicationID, status).Scan(
		&a.ID, &a.PostingID, &a.Status, &a.AppliedAt, &a.ResumeVariant, &a.UsedCoverLetter,
		&a.NextFollowUpDate, &a.Notes, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrApplicationNotFound
		}
		return nil, fmt.Errorf("closing application %d: %w", applicationID, err)
	}
	return a, nil
}
