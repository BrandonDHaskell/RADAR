package dedup_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BrandonDHaskell/RADAR/internal/dedup"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	pool, err := store.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSyncSkipsExpiryOnEmptyFetch(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-dedup-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name:     "Dedup Test Co",
		ATSType:  "greenhouse",
		ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID)
	})

	seed, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID:    company.ID,
		Source:       "greenhouse",
		ExternalID:   "ext-1",
		Title:        "Software Engineer",
		Location:     "Remote",
		CanonicalKey: "dedup test co|software engineer|remote",
		ContentHash:  "hash-v1",
	})
	if err != nil {
		t.Fatalf("seed UpsertPosting: %v", err)
	}

	// Empty fetch, guard enabled: the open posting must survive untouched.
	guarded, err := dedup.Sync(ctx, pool, company.ID, "greenhouse", company.Name, nil, false)
	if err != nil {
		t.Fatalf("Sync (guarded): %v", err)
	}
	if !guarded.ExpirySkipped {
		t.Error("Sync (guarded): ExpirySkipped = false, want true")
	}
	if guarded.Closed != 0 {
		t.Errorf("Sync (guarded): Closed = %d, want 0", guarded.Closed)
	}
	if guarded.OpenAtSkip != 1 {
		t.Errorf("Sync (guarded): OpenAtSkip = %d, want 1", guarded.OpenAtSkip)
	}

	isOpen := queryIsOpen(t, ctx, pool, seed.ID)
	if !isOpen {
		t.Error("posting was closed despite the empty-fetch guard")
	}

	// Empty fetch, guard overridden: the posting should now close.
	overridden, err := dedup.Sync(ctx, pool, company.ID, "greenhouse", company.Name, nil, true)
	if err != nil {
		t.Fatalf("Sync (allowEmpty): %v", err)
	}
	if overridden.ExpirySkipped {
		t.Error("Sync (allowEmpty): ExpirySkipped = true, want false")
	}
	if overridden.Closed != 1 {
		t.Errorf("Sync (allowEmpty): Closed = %d, want 1", overridden.Closed)
	}

	if queryIsOpen(t, ctx, pool, seed.ID) {
		t.Error("posting should be closed after Sync with allowEmpty=true")
	}
}

func queryIsOpen(t *testing.T, ctx context.Context, pool *pgxpool.Pool, postingID int64) bool {
	t.Helper()
	var isOpen bool
	if err := pool.QueryRow(ctx, "SELECT is_open FROM postings WHERE id = $1", postingID).Scan(&isOpen); err != nil {
		t.Fatalf("querying is_open: %v", err)
	}
	return isOpen
}
