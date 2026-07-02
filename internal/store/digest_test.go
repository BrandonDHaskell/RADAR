package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

const testDigestProfileHash = "test-profile-hash-v1"

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

	mustUpsertPosting := func(externalID, title, contentHash string) int64 {
		t.Helper()
		p, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
			CompanyID:    company.ID,
			Source:       "greenhouse",
			ExternalID:   externalID,
			Title:        title,
			CanonicalKey: fmt.Sprintf("digest test co|%s|", title),
			ContentHash:  contentHash,
		})
		if err != nil {
			t.Fatalf("UpsertPosting(%s): %v", externalID, err)
		}
		return p.ID
	}
	mustSetSemanticScore := func(postingID int64, score float32) {
		t.Helper()
		if _, err := pool.Exec(ctx, `
			INSERT INTO fit_scores (posting_id, semantic_score, computed_at)
			VALUES ($1, $2, now())
			ON CONFLICT (posting_id) DO UPDATE SET semantic_score = EXCLUDED.semantic_score
		`, postingID, score); err != nil {
			t.Fatalf("seeding semantic score for %d: %v", postingID, err)
		}
	}
	// mustSetVerdict writes a fresh verdict: hashes matching
	// testDigestProfileHash and the posting's own content hash.
	mustSetVerdict := func(postingID int64, verdict, contentHash string) {
		t.Helper()
		v := verdict
		ph := testDigestProfileHash
		ch := contentHash
		if err := store.UpsertVerdict(ctx, pool, store.Verdict{
			PostingID:          postingID,
			LLMVerdict:         &v,
			VerdictProfileHash: &ph,
			VerdictContentHash: &ch,
		}); err != nil {
			t.Fatalf("UpsertVerdict(%d): %v", postingID, err)
		}
	}
	// mustSetStaleVerdict writes a verdict computed against a different
	// profile and content version, simulating a profile edit or posting
	// content change since the verdict was written.
	mustSetStaleVerdict := func(postingID int64, verdict string) {
		t.Helper()
		v := verdict
		staleProfileHash := "old-profile-hash"
		staleContentHash := "old-content-hash"
		if err := store.UpsertVerdict(ctx, pool, store.Verdict{
			PostingID:          postingID,
			LLMVerdict:         &v,
			VerdictProfileHash: &staleProfileHash,
			VerdictContentHash: &staleContentHash,
		}); err != nil {
			t.Fatalf("UpsertVerdict(%d): %v", postingID, err)
		}
	}

	pursueID := mustUpsertPosting("pursue", "Pursue Role", "hash-pursue")
	mustSetSemanticScore(pursueID, 0.9)
	mustSetVerdict(pursueID, "pursue", "hash-pursue")

	stretchID := mustUpsertPosting("stretch", "Stretch Role", "hash-stretch")
	mustSetSemanticScore(stretchID, 0.5)
	mustSetVerdict(stretchID, "stretch", "hash-stretch")

	skipID := mustUpsertPosting("skip", "Skip Role", "hash-skip")
	mustSetSemanticScore(skipID, 0.2)
	mustSetVerdict(skipID, "skip", "hash-skip")

	// A verdict exists but is stale (hash mismatch): it must rank as if
	// unscored (rank 0) despite a high raw semantic score, and be reported
	// via VerdictStale rather than shown.
	staleID := mustUpsertPosting("stale", "Stale Verdict Role", "hash-stale")
	mustSetSemanticScore(staleID, 0.6)
	mustSetStaleVerdict(staleID, "pursue")

	unscoredID := mustUpsertPosting("unscored", "Unscored Role", "hash-unscored")
	// deliberately no fit_scores row

	appliedID := mustUpsertPosting("applied", "Already Applied Role", "hash-applied")
	mustSetSemanticScore(appliedID, 0.99)
	mustSetVerdict(appliedID, "pursue", "hash-applied")
	if _, err := pool.Exec(ctx, "INSERT INTO applications (posting_id, status) VALUES ($1, 'identified')", appliedID); err != nil {
		t.Fatalf("seeding application: %v", err)
	}

	closedID := mustUpsertPosting("closed", "Closed Role", "hash-closed")
	mustSetSemanticScore(closedID, 0.99)
	mustSetVerdict(closedID, "pursue", "hash-closed")
	if _, err := pool.Exec(ctx, "UPDATE postings SET is_open = false WHERE id = $1", closedID); err != nil {
		t.Fatalf("closing posting: %v", err)
	}

	excludedID := mustUpsertPosting("excluded", "Excluded Role", "hash-excluded")
	mustSetSemanticScore(excludedID, 0.99)
	mustSetVerdict(excludedID, "pursue", "hash-excluded")
	if _, err := pool.Exec(ctx, "UPDATE postings SET screen_status = 'excluded', screen_reason = 'title_exclusion:test' WHERE id = $1", excludedID); err != nil {
		t.Fatalf("excluding posting: %v", err)
	}

	t.Run("no filter returns all open, screened-in, un-applied postings ranked by fresh verdict then score", func(t *testing.T) {
		// DigestPostings ranks across every company, by design (the digest
		// is a global top-N). A small limit here would make this assertion
		// depend on how much other data happens to be in the dev database;
		// this subtest isn't testing the limit, so use a limit high enough
		// that it can't interfere, and rely on postingIDsForTestCompany to
		// filter down to what this test created.
		got, err := store.DigestPostings(ctx, pool, testDigestProfileHash, "", 1000)
		if err != nil {
			t.Fatalf("DigestPostings: %v", err)
		}
		ids := postingIDsForTestCompany(got)
		// staleID (semantic 0.6, but stale) ranks below skipID (rank 1) and
		// above unscoredID (no semantic score at all).
		want := []int64{pursueID, stretchID, skipID, staleID, unscoredID}
		if !idsEqual(ids, want) {
			t.Errorf("DigestPostings ids = %v, want %v (in order)", ids, want)
		}
	})

	t.Run("stale verdict is hidden and reported, applied and closed and screened-out postings are absent", func(t *testing.T) {
		got, err := store.DigestPostings(ctx, pool, testDigestProfileHash, "", 1000)
		if err != nil {
			t.Fatalf("DigestPostings: %v", err)
		}
		byID := make(map[int64]store.DigestPosting)
		for _, p := range got {
			byID[p.ID] = p
		}

		stale, ok := byID[staleID]
		if !ok {
			t.Fatal("stale posting missing from digest, want it present with a hidden verdict")
		}
		if !stale.VerdictStale {
			t.Error("stale posting: VerdictStale = false, want true")
		}
		if stale.LLMVerdict != nil {
			t.Errorf("stale posting: LLMVerdict = %v, want nil (stale verdicts are hidden)", *stale.LLMVerdict)
		}

		for _, id := range []int64{appliedID, closedID, excludedID} {
			if _, present := byID[id]; present {
				t.Errorf("posting %d should be absent from the digest (applied, closed, or excluded)", id)
			}
		}
	})

	t.Run("min-verdict stretch excludes skip, stale, and unscored", func(t *testing.T) {
		got, err := store.DigestPostings(ctx, pool, testDigestProfileHash, "stretch", 1000)
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
		got, err := store.DigestPostings(ctx, pool, testDigestProfileHash, "", 1)
		if err != nil {
			t.Fatalf("DigestPostings: %v", err)
		}
		if len(got) != 1 || got[0].ID != pursueID {
			t.Errorf("DigestPostings(limit=1) = %+v, want just the pursue posting", got)
		}
	})

	t.Run("invalid min-verdict is rejected", func(t *testing.T) {
		if _, err := store.DigestPostings(ctx, pool, testDigestProfileHash, "maybe", 10); err == nil {
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
