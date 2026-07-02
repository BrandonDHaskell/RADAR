package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestCompanyCRUD(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-%d", time.Now().UnixNano())

	created, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name:     "Acme Test Co",
		ATSType:  "greenhouse",
		ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", created.ID)
	})

	if created.Status != store.CompanyStatusCandidate {
		t.Errorf("status = %q, want %q", created.Status, store.CompanyStatusCandidate)
	}

	if _, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name:     "Duplicate Co",
		ATSType:  "greenhouse",
		ATSToken: token,
	}); !errors.Is(err, store.ErrDuplicateCompany) {
		t.Errorf("duplicate insert: got err %v, want ErrDuplicateCompany", err)
	}

	all, err := store.ListCompanies(ctx, pool, "")
	if err != nil {
		t.Fatalf("ListCompanies: %v", err)
	}
	if !containsCompany(all, created.ID) {
		t.Errorf("ListCompanies(\"\") did not include company %d", created.ID)
	}

	candidates, err := store.ListCompanies(ctx, pool, store.CompanyStatusCandidate)
	if err != nil {
		t.Fatalf("ListCompanies(candidate): %v", err)
	}
	if !containsCompany(candidates, created.ID) {
		t.Errorf("ListCompanies(candidate) did not include company %d", created.ID)
	}

	confirmed, err := store.SetCompanyStatus(ctx, pool, created.ID, store.CompanyStatusConfirmed)
	if err != nil {
		t.Fatalf("SetCompanyStatus(confirmed): %v", err)
	}
	if confirmed.Status != store.CompanyStatusConfirmed {
		t.Errorf("status = %q, want %q", confirmed.Status, store.CompanyStatusConfirmed)
	}

	archived, err := store.SetCompanyStatus(ctx, pool, created.ID, store.CompanyStatusArchived)
	if err != nil {
		t.Fatalf("SetCompanyStatus(archived): %v", err)
	}
	if archived.Status != store.CompanyStatusArchived {
		t.Errorf("status = %q, want %q", archived.Status, store.CompanyStatusArchived)
	}

	if _, err := store.SetCompanyStatus(ctx, pool, 999999999, store.CompanyStatusConfirmed); !errors.Is(err, store.ErrCompanyNotFound) {
		t.Errorf("SetCompanyStatus(nonexistent): got err %v, want ErrCompanyNotFound", err)
	}
}

func TestSetCompanyStatusRejectsRemovedActiveStatus(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-%d", time.Now().UnixNano())

	created, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name:     "Status Check Co",
		ATSType:  "greenhouse",
		ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", created.ID)
	})

	if _, err := store.SetCompanyStatus(ctx, pool, created.ID, "active"); err == nil {
		t.Error("SetCompanyStatus(active) succeeded, want a CHECK constraint error since active was removed")
	}
}

func containsCompany(companies []store.Company, id int64) bool {
	for _, c := range companies {
		if c.ID == id {
			return true
		}
	}
	return false
}
