package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestScreenCandidatesAndSetPostingScreen(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-screen-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name:     "Screen Test Co",
		ATSType:  "greenhouse",
		ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID)
	})

	posting, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID:    company.ID,
		Source:       "greenhouse",
		ExternalID:   "ext-1",
		Title:        "Executive Assistant",
		Location:     "New York, NY",
		CanonicalKey: "screen test co|executive assistant|",
		ContentHash:  "hash-v1",
	})
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	const hash1 = "profile-hash-1"
	const hash2 = "profile-hash-2"

	// A never-screened posting is pending under any profile hash.
	candidates, err := store.ScreenCandidates(ctx, pool, hash1)
	if err != nil {
		t.Fatalf("ScreenCandidates: %v", err)
	}
	if !containsScreenCandidate(candidates, posting.ID) {
		t.Fatal("ScreenCandidates: newly created posting missing, want it pending")
	}

	reason := "title_exclusion:executive assistant"
	if err := store.SetPostingScreen(ctx, pool, posting.ID, "excluded", &reason, hash1); err != nil {
		t.Fatalf("SetPostingScreen: %v", err)
	}

	var status, gotReason, gotHash string
	if err := pool.QueryRow(ctx,
		"SELECT screen_status, screen_reason, screen_profile_hash FROM postings WHERE id = $1", posting.ID,
	).Scan(&status, &gotReason, &gotHash); err != nil {
		t.Fatalf("querying screen columns: %v", err)
	}
	if status != "excluded" || gotReason != reason || gotHash != hash1 {
		t.Errorf("screen columns = (%q, %q, %q), want (%q, %q, %q)", status, gotReason, gotHash, "excluded", reason, hash1)
	}

	// Screened under hash1: no longer a candidate under hash1.
	candidates, err = store.ScreenCandidates(ctx, pool, hash1)
	if err != nil {
		t.Fatalf("ScreenCandidates: %v", err)
	}
	if containsScreenCandidate(candidates, posting.ID) {
		t.Error("ScreenCandidates(hash1): posting still a candidate after being screened under hash1")
	}

	// A profile edit (different hash) re-admits it as a candidate.
	candidates, err = store.ScreenCandidates(ctx, pool, hash2)
	if err != nil {
		t.Fatalf("ScreenCandidates: %v", err)
	}
	if !containsScreenCandidate(candidates, posting.ID) {
		t.Error("ScreenCandidates(hash2): posting should be a re-screen candidate after a profile hash change")
	}

	// Re-admitting under hash2 (exclusion phrase removed) flips it to passed.
	if err := store.SetPostingScreen(ctx, pool, posting.ID, "passed", nil, hash2); err != nil {
		t.Fatalf("SetPostingScreen (re-admit): %v", err)
	}
	var gotReasonAfterReadmit *string
	if err := pool.QueryRow(ctx,
		"SELECT screen_status, screen_reason, screen_profile_hash FROM postings WHERE id = $1", posting.ID,
	).Scan(&status, &gotReasonAfterReadmit, &gotHash); err != nil {
		t.Fatalf("querying screen columns after re-admit: %v", err)
	}
	if status != "passed" || gotReasonAfterReadmit != nil || gotHash != hash2 {
		t.Errorf("screen columns after re-admit = (%q, %v, %q), want (%q, nil, %q)", status, gotReasonAfterReadmit, gotHash, "passed", hash2)
	}
}

func containsScreenCandidate(candidates []store.ScreenCandidate, id int64) bool {
	for _, c := range candidates {
		if c.ID == id {
			return true
		}
	}
	return false
}
