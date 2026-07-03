package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CorrespondenceDirections are the direction values accepted by the
// correspondence table.
var CorrespondenceDirections = []string{"inbound", "outbound"}

// CorrespondenceChannels are the channel values accepted by the
// correspondence table.
var CorrespondenceChannels = []string{"email", "linkedin", "phone", "other"}

// ErrContactNotFound is returned when a contact id does not exist.
var ErrContactNotFound = errors.New("contact not found")

// Correspondence is a row in the correspondence table.
type Correspondence struct {
	ID             int64
	ApplicationID  int64
	ContactID      *int64
	Direction      string
	Channel        string
	Summary        string
	OccurredAt     time.Time
	FollowUpNeeded bool
	FollowUpDate   *time.Time
}

// NewCorrespondence holds the fields accepted when logging a correspondence
// entry via the CLI.
type NewCorrespondence struct {
	ApplicationID  int64
	ContactID      *int64
	Direction      string
	Channel        string
	Summary        string
	FollowUpNeeded bool
	FollowUpDate   *time.Time
}

// LogCorrespondence inserts a correspondence entry against an application.
func LogCorrespondence(ctx context.Context, pool *pgxpool.Pool, in NewCorrespondence) (*Correspondence, error) {
	c := &Correspondence{}
	err := pool.QueryRow(ctx, `
		INSERT INTO correspondence (application_id, contact_id, direction, channel, summary, follow_up_needed, follow_up_date)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7)
		RETURNING id, application_id, contact_id, direction, coalesce(channel, ''), coalesce(summary, ''),
		          occurred_at, follow_up_needed, follow_up_date
	`, in.ApplicationID, in.ContactID, in.Direction, in.Channel, in.Summary, in.FollowUpNeeded, in.FollowUpDate).Scan(
		&c.ID, &c.ApplicationID, &c.ContactID, &c.Direction, &c.Channel, &c.Summary,
		&c.OccurredAt, &c.FollowUpNeeded, &c.FollowUpDate,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			if strings.Contains(pgErr.ConstraintName, "contact_id") {
				return nil, ErrContactNotFound
			}
			return nil, ErrApplicationNotFound
		}
		return nil, fmt.Errorf("logging correspondence: %w", err)
	}
	return c, nil
}
