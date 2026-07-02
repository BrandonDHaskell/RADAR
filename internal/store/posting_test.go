package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestUpsertAndExpirePosting(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-posting-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name:     "Posting Test Co",
		ATSType:  "greenhouse",
		ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID)
	})

	base := store.PostingUpsert{
		CompanyID:    company.ID,
		Source:       "greenhouse",
		ExternalID:   "ext-1",
		Title:        "Software Engineer",
		Location:     "Remote",
		CanonicalKey: "posting test co|software engineer|remote",
		ContentHash:  "hash-v1",
	}

	inserted, err := store.UpsertPosting(ctx, pool, base)
	if err != nil {
		t.Fatalf("UpsertPosting (insert): %v", err)
	}
	if !inserted.Inserted || !inserted.Changed {
		t.Errorf("first upsert: got Inserted=%v Changed=%v, want both true", inserted.Inserted, inserted.Changed)
	}

	unchanged, err := store.UpsertPosting(ctx, pool, base)
	if err != nil {
		t.Fatalf("UpsertPosting (unchanged): %v", err)
	}
	if unchanged.ID != inserted.ID {
		t.Errorf("unchanged upsert returned different id: %d != %d", unchanged.ID, inserted.ID)
	}
	if unchanged.Inserted || unchanged.Changed {
		t.Errorf("unchanged upsert: got Inserted=%v Changed=%v, want both false", unchanged.Inserted, unchanged.Changed)
	}

	changedInput := base
	changedInput.Title = "Senior Software Engineer"
	changedInput.ContentHash = "hash-v2"

	updated, err := store.UpsertPosting(ctx, pool, changedInput)
	if err != nil {
		t.Fatalf("UpsertPosting (changed): %v", err)
	}
	if updated.Inserted {
		t.Error("changed upsert reported Inserted = true, want false")
	}
	if !updated.Changed {
		t.Error("changed upsert reported Changed = false, want true")
	}

	// A posting missing from the "open" list passed to ExpirePostings should be closed.
	closedCount, err := store.ExpirePostings(ctx, pool, company.ID, "greenhouse", []string{"some-other-id"})
	if err != nil {
		t.Fatalf("ExpirePostings: %v", err)
	}
	if closedCount != 1 {
		t.Errorf("ExpirePostings closed %d rows, want 1", closedCount)
	}

	var isOpen bool
	if err := pool.QueryRow(ctx, "SELECT is_open FROM postings WHERE id = $1", updated.ID).Scan(&isOpen); err != nil {
		t.Fatalf("querying is_open: %v", err)
	}
	if isOpen {
		t.Error("posting should be closed after ExpirePostings, but is_open = true")
	}

	// Re-fetching a posting that reappears in the source should reopen it.
	reopened, err := store.UpsertPosting(ctx, pool, changedInput)
	if err != nil {
		t.Fatalf("UpsertPosting (reopen): %v", err)
	}
	if reopened.Inserted {
		t.Error("reopen upsert reported Inserted = true, want false")
	}
	if err := pool.QueryRow(ctx, "SELECT is_open FROM postings WHERE id = $1", reopened.ID).Scan(&isOpen); err != nil {
		t.Fatalf("querying is_open after reopen: %v", err)
	}
	if !isOpen {
		t.Error("posting should be open again after re-upsert, but is_open = false")
	}
}
