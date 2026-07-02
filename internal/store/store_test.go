package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestOpenAppliesMigrations(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}

	ctx := context.Background()
	pool, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer pool.Close()

	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM companies").Scan(&count); err != nil {
		t.Fatalf("querying companies table: %v", err)
	}
}
