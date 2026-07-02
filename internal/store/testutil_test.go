package store_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

// openTestPool skips the test if DATABASE_URL is not set; otherwise it
// returns a connection pool with migrations applied, closed automatically
// via t.Cleanup.
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
