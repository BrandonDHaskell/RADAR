package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostingUpsert holds the fields needed to insert or update a posting row.
type PostingUpsert struct {
	CompanyID       int64
	Source          string
	ExternalID      string
	Title           string
	Location        string
	IsRemote        bool
	Department      string
	EmploymentType  string
	SalaryMin       *float64
	SalaryMax       *float64
	SalaryCurrency  string
	Description     string
	ApplyURL        string
	SourceURL       string
	CanonicalKey    string
	ContentHash     string
	SourceUpdatedAt *time.Time
}

// PostingUpsertResult reports what UpsertPosting did.
type PostingUpsertResult struct {
	ID       int64
	Inserted bool // true if this was a brand new posting
	Changed  bool // true if the row was newly inserted or its content_hash differed
}

// UpsertPosting inserts a posting or, if (source, external_id) already
// exists, updates it and reopens it (is_open = true, last_seen_at = now()).
// Changed reports whether the row is new or its content actually differs
// from what was stored, so callers know whether re-embedding is warranted.
func UpsertPosting(ctx context.Context, pool *pgxpool.Pool, p PostingUpsert) (PostingUpsertResult, error) {
	var res PostingUpsertResult
	var previousContentHash *string

	err := pool.QueryRow(ctx, `
		WITH existing AS (
			SELECT content_hash FROM postings WHERE source = $2 AND external_id = $3
		)
		INSERT INTO postings (
			company_id, source, external_id, title, location, is_remote, department,
			employment_type, salary_min, salary_max, salary_currency, description,
			apply_url, source_url, canonical_key, content_hash, source_updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		)
		ON CONFLICT (source, external_id) DO UPDATE SET
			company_id       = EXCLUDED.company_id,
			title            = EXCLUDED.title,
			location         = EXCLUDED.location,
			is_remote        = EXCLUDED.is_remote,
			department       = EXCLUDED.department,
			employment_type  = EXCLUDED.employment_type,
			salary_min       = EXCLUDED.salary_min,
			salary_max       = EXCLUDED.salary_max,
			salary_currency  = EXCLUDED.salary_currency,
			description      = EXCLUDED.description,
			apply_url        = EXCLUDED.apply_url,
			source_url       = EXCLUDED.source_url,
			canonical_key    = EXCLUDED.canonical_key,
			content_hash     = EXCLUDED.content_hash,
			source_updated_at = EXCLUDED.source_updated_at,
			is_open          = true,
			last_seen_at     = now(),
			updated_at       = now()
		RETURNING id, (xmax = 0) AS inserted, (SELECT content_hash FROM existing)
	`,
		p.CompanyID, p.Source, p.ExternalID, p.Title, p.Location, p.IsRemote, p.Department,
		p.EmploymentType, p.SalaryMin, p.SalaryMax, p.SalaryCurrency, p.Description,
		p.ApplyURL, p.SourceURL, p.CanonicalKey, p.ContentHash, p.SourceUpdatedAt,
	).Scan(&res.ID, &res.Inserted, &previousContentHash)
	if err != nil {
		return PostingUpsertResult{}, fmt.Errorf("upserting posting %s/%s: %w", p.Source, p.ExternalID, err)
	}

	res.Changed = res.Inserted || previousContentHash == nil || *previousContentHash != p.ContentHash
	return res, nil
}

// CountOpenPostings returns how many open postings exist for a company/source.
func CountOpenPostings(ctx context.Context, pool *pgxpool.Pool, companyID int64, source string) (int64, error) {
	var count int64
	err := pool.QueryRow(ctx, `
		SELECT count(*) FROM postings WHERE company_id = $1 AND source = $2 AND is_open = true
	`, companyID, source).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting open postings for company %d/%s: %w", companyID, source, err)
	}
	return count, nil
}

// ExpirePostings marks postings for (companyID, source) closed if their
// external_id is not present in openExternalIDs, i.e. they no longer appear
// in the source's current listing. It returns the number of rows closed.
func ExpirePostings(ctx context.Context, pool *pgxpool.Pool, companyID int64, source string, openExternalIDs []string) (int64, error) {
	tag, err := pool.Exec(ctx, `
		UPDATE postings
		SET is_open = false, updated_at = now()
		WHERE company_id = $1 AND source = $2 AND is_open = true
		  AND NOT (external_id = ANY($3::text[]))
	`, companyID, source, openExternalIDs)
	if err != nil {
		return 0, fmt.Errorf("expiring postings for company %d/%s: %w", companyID, source, err)
	}
	return tag.RowsAffected(), nil
}
