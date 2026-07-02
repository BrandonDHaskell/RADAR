// Package dedup turns a batch of freshly fetched postings for one company
// into upsert and expiry calls against the store: new postings are
// inserted, existing ones are updated (or left alone if unchanged), and
// postings that no longer appear in the source are marked closed.
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
	Inserted  int
	Updated   int
	Unchanged int
	Closed    int
}

// Sync upserts postings fetched for companyID/source and closes any
// previously open posting for that company/source not present in postings.
// It should only be called with a successful fetch result: an empty
// postings slice is treated as "the board is genuinely empty" and will
// close every open posting for that company/source.
func Sync(ctx context.Context, pool *pgxpool.Pool, companyID int64, source, companyName string, postings []ingest.NormalizedPosting) (Result, error) {
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
