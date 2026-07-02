package store_test

import (
	"context"
	"testing"
)

func TestOpenAppliesMigrations(t *testing.T) {
	pool := openTestPool(t)

	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM companies").Scan(&count); err != nil {
		t.Fatalf("querying companies table: %v", err)
	}
}
