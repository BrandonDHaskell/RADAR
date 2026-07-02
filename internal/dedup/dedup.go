// Package dedup turns a batch of freshly fetched postings for one company
// into upsert and expiry calls against the store: new postings are
// inserted, existing ones are updated (or left alone if unchanged), and
// postings that no longer appear in the source are marked closed.
//
// A fetch that returns zero postings is treated with suspicion rather than
// taken at face value: if the company currently has open postings for that
// source, Sync skips expiry and reports it instead of closing everything,
// since a transient empty response should not silently hide every role
// until the next sync. Pass allowEmpty to override this guard for a board
// that is genuinely empty.
package dedup

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BrandonDHaskell/RADAR/internal/ingest"
	"github.com/BrandonDHaskell/RADAR/internal/normalize"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

// Result summarizes what a Sync call did, for CLI reporting.
type Result struct {
	Inserted      int
	Updated       int
	Unchanged     int
	Closed        int
	ExpirySkipped bool  // true if the empty-fetch guard skipped expiry
	OpenAtSkip    int64 // open posting count at the time ExpirySkipped was set
}

// Sync upserts postings fetched for companyID/source and closes any
// previously open posting for that company/source not present in postings.
// If postings is empty and allowEmpty is false, Sync first checks whether
// the company has open postings for that source; if it does, Sync leaves
// everything untouched and reports ExpirySkipped instead of closing every
// open posting on the strength of a single empty response.
func Sync(ctx context.Context, pool *pgxpool.Pool, companyID int64, source, companyName string, postings []ingest.NormalizedPosting, allowEmpty bool) (Result, error) {
	if len(postings) == 0 && !allowEmpty {
		openCount, err := store.CountOpenPostings(ctx, pool, companyID, source)
		if err != nil {
			return Result{}, err
		}
		if openCount > 0 {
			return Result{ExpirySkipped: true, OpenAtSkip: openCount}, nil
		}
	}

	var res Result
	seenExternalIDs := make([]string, 0, len(postings))

	for _, p := range postings {
		seenExternalIDs = append(seenExternalIDs, p.ExternalID)

		upsertResult, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
			CompanyID:       companyID,
			Source:          source,
			ExternalID:      p.ExternalID,
			Title:           p.Title,
			Location:        p.Location,
			IsRemote:        p.IsRemote,
			Department:      p.Department,
			EmploymentType:  p.EmploymentType,
			SalaryMin:       p.SalaryMin,
			SalaryMax:       p.SalaryMax,
			SalaryCurrency:  p.SalaryCurrency,
			Description:     p.Description,
			ApplyURL:        p.ApplyURL,
			SourceURL:       p.SourceURL,
			CanonicalKey:    normalize.CanonicalKey(companyName, p),
			ContentHash:     normalize.ContentHash(p),
			SourceUpdatedAt: p.SourceUpdatedAt,
		})
		if err != nil {
			return Result{}, fmt.Errorf("syncing posting %s/%s for company %d: %w", source, p.ExternalID, companyID, err)
		}

		switch {
		case upsertResult.Inserted:
			res.Inserted++
		case upsertResult.Changed:
			res.Updated++
		default:
			res.Unchanged++
		}
	}

	closed, err := store.ExpirePostings(ctx, pool, companyID, source, seenExternalIDs)
	if err != nil {
		return Result{}, err
	}
	res.Closed = int(closed)

	return res, nil
}
