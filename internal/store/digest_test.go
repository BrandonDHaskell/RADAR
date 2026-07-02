package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestDigestPostings(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-digest-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name:     "Digest Test Co",
		ATSType:  "greenhouse",
		ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID)
	})

	mustUpsertPosting := func(externalID, title string) int64 {
		t.Helper()
		p, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
			CompanyID:    company.ID,
			Source:       "greenhouse",
			ExternalID:   externalID,
			Title:        title,
			CanonicalKey: fmt.Sprintf("digest test co|%s|", title),
			ContentHash:  "hash-" + externalID,
		})
		if err != nil {
			t.Fatalf("UpsertPosting(%s): %v", externalID, err)
		}
		return p.ID
	}
	mustSetFitScore := func(postingID int64, verdict string, semanticScore float32) {
		t.Helper()
		v := verdict
		if err := store.UpsertFitScore(ctx, pool, store.FitScore{
			PostingID:     postingID,
			SemanticScore: &semanticScore,
			LLMVerdict:    &v,
		}); err != nil {
			t.Fatalf("UpsertFitScore(%d): %v", postingID, err)
		}
	}

	pursueID := mustUpsertPosting("pursue", "Pursue Role")
	mustSetFitScore(pursueID, "pursue", 0.9)

	stretchID := mustUpsertPosting("stretch", "Stretch Role")
	mustSetFitScore(stretchID, "stretch", 0.5)

	skipID := mustUpsertPosting("skip", "Skip Role")
	mustSetFitScore(skipID, "skip", 0.2)

	unscoredID := mustUpsertPosting("unscored", "Unscored Role")
	// deliberately no fit_scores row

	appliedID := mustUpsertPosting("applied", "Already Applied Role")
	mustSetFitScore(appliedID, "pursue", 0.99)
	if _, err := pool.Exec(ctx, "INSERT INTO applications (posting_id, status) VALUES ($1, 'identified')", appliedID); err != nil {
		t.Fatalf("seeding application: %v", err)
	}

	closedID := mustUpsertPosting("closed", "Closed Role")
	mustSetFitScore(closedID, "pursue", 0.99)
	if _, err := pool.Exec(ctx, "UPDATE postings SET is_open = false WHERE id = $1", closedID); err != nil {
		t.Fatalf("closing posting: %v", err)
	}

	t.Run("no filter returns all open, un-applied postings ranked by verdict then score", func(t *testing.T) {
		got, err := store.DigestPostings(ctx, pool, "", 10)
		if err != nil {
			t.Fatalf("DigestPostings: %v", err)
		}
		ids := postingIDsForTestCompany(got)
		want := []int64{pursueID, stretchID, skipID, unscoredID}
		if !idsEqual(ids, want) {
			t.Errorf("DigestPostings ids = %v, want %v (in order)", ids, want)
		}
	})

	t.Run("min-verdict stretch excludes skip and unscored", func(t *testing.T) {
		got, err := store.DigestPostings(ctx, pool, "stretch", 10)
		if err != nil {
			t.Fatalf("DigestPostings: %v", err)
		}
		ids := postingIDsForTestCompany(got)
		want := []int64{pursueID, stretchID}
		if !idsEqual(ids, want) {
			t.Errorf("DigestPostings(min-verdict=stretch) ids = %v, want %v", ids, want)
		}
	})

	t.Run("limit truncates the result", func(t *testing.T) {
		got, err := store.DigestPostings(ctx, pool, "", 1)
		if err != nil {
			t.Fatalf("DigestPostings: %v", err)
		}
		if len(got) != 1 || got[0].ID != pursueID {
			t.Errorf("DigestPostings(limit=1) = %+v, want just the pursue posting", got)
		}
	})

	t.Run("invalid min-verdict is rejected", func(t *testing.T) {
		if _, err := store.DigestPostings(ctx, pool, "maybe", 10); err == nil {
			t.Error("DigestPostings(min-verdict=maybe): got nil error, want an error")
		}
	})
}

// postingIDsForTestCompany filters out postings belonging to companies
// created by other tests running concurrently in other packages against
// the same database (go test ./... runs packages in parallel by default).
func postingIDsForTestCompany(postings []store.DigestPosting) []int64 {
	var ids []int64
	for _, p := range postings {
		if p.CompanyName == "Digest Test Co" {
			ids = append(ids, p.ID)
		}
	}
	return ids
}

func idsEqual(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
