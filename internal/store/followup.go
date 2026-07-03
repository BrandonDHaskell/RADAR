package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// openApplicationStatuses are the statuses ListFollowUps considers; a closed
// or withdrawn application never needs a follow-up.
const openApplicationStatusFilter = "status NOT IN ('closed_offer', 'closed_rejected', 'withdrawn')"

// FollowUp is one reason an application needs attention: an explicit
// follow-up date come due (on the application or on a correspondence entry)
// or, with staleDays set, no recorded activity in that many days. A single
// application can appear more than once, once per reason.
type FollowUp struct {
	ApplicationID int64
	PostingTitle  string
	CompanyName   string
	Status        string
	Reason        string
	DueDate       *time.Time
}

// ListFollowUps returns every open application with a follow-up due today
// or overdue, from either the application's own next_follow_up_date or a
// correspondence entry flagged follow_up_needed (an unscheduled needed
// follow-up, i.e. no follow_up_date, is always considered due: there is no
// future date to defer it by). If staleDays > 0, it also surfaces open
// applications with no recorded activity (an update or a correspondence
// entry) in at least that many days, even without an explicit follow-up
// date, so a forgotten application doesn't fall through silently.
func ListFollowUps(ctx context.Context, pool *pgxpool.Pool, staleDays int) ([]FollowUp, error) {
	rows, err := pool.Query(ctx, `
		WITH last_activity AS (
			SELECT application_id, max(occurred_at) AS last_occurred_at
			FROM correspondence
			GROUP BY application_id
		)
		SELECT a.id, p.title, c.name, a.status,
		       'application follow-up due ' || a.next_follow_up_date, a.next_follow_up_date
		FROM applications a
		JOIN postings p ON p.id = a.posting_id
		JOIN companies c ON c.id = p.company_id
		WHERE a.`+openApplicationStatusFilter+`
		  AND a.next_follow_up_date IS NOT NULL AND a.next_follow_up_date <= CURRENT_DATE

		UNION ALL

		SELECT a.id, p.title, c.name, a.status,
		       CASE WHEN co.follow_up_date IS NULL THEN 'correspondence follow-up needed (no date set)'
		            ELSE 'correspondence follow-up due ' || co.follow_up_date END,
		       co.follow_up_date
		FROM correspondence co
		JOIN applications a ON a.id = co.application_id
		JOIN postings p ON p.id = a.posting_id
		JOIN companies c ON c.id = p.company_id
		WHERE a.`+openApplicationStatusFilter+`
		  AND co.follow_up_needed
		  AND (co.follow_up_date IS NULL OR co.follow_up_date <= CURRENT_DATE)

		UNION ALL

		SELECT a.id, p.title, c.name, a.status,
		       'stale: no activity in ' || $1::int || '+ days', NULL::date
		FROM applications a
		JOIN postings p ON p.id = a.posting_id
		JOIN companies c ON c.id = p.company_id
		LEFT JOIN last_activity la ON la.application_id = a.id
		WHERE $1::int > 0 AND a.`+openApplicationStatusFilter+`
		  AND greatest(a.updated_at, coalesce(la.last_occurred_at, a.updated_at)) < now() - make_interval(days => $1::int)

		ORDER BY 1
	`, staleDays)
	if err != nil {
		return nil, fmt.Errorf("listing follow-ups: %w", err)
	}
	defer rows.Close()

	var followUps []FollowUp
	for rows.Next() {
		var f FollowUp
		if err := rows.Scan(&f.ApplicationID, &f.PostingTitle, &f.CompanyName, &f.Status, &f.Reason, &f.DueDate); err != nil {
			return nil, fmt.Errorf("scanning follow-up: %w", err)
		}
		followUps = append(followUps, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing follow-ups: %w", err)
	}
	return followUps, nil
}
