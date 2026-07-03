package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestContactCRUD(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-contact-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name: "Contact Test Co", ATSType: "greenhouse", ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	created, err := store.CreateContact(ctx, pool, store.NewContact{
		CompanyID: company.ID, Name: "Jamie Recruiter", Type: "recruiter", Email: "jamie@example.com",
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}
	if created.CompanyID != company.ID {
		t.Errorf("CompanyID = %d, want %d", created.CompanyID, company.ID)
	}
	if created.Type != "recruiter" {
		t.Errorf("Type = %q, want %q", created.Type, "recruiter")
	}

	contacts, err := store.ListContacts(ctx, pool)
	if err != nil {
		t.Fatalf("ListContacts: %v", err)
	}
	if !containsContact(contacts, created.ID) {
		t.Errorf("ListContacts did not include contact %d", created.ID)
	}
	for _, c := range contacts {
		if c.ID == created.ID && c.CompanyName != "Contact Test Co" {
			t.Errorf("CompanyName = %q, want %q", c.CompanyName, "Contact Test Co")
		}
	}

	if _, err := store.CreateContact(ctx, pool, store.NewContact{
		CompanyID: 999999999, Name: "Ghost",
	}); !errors.Is(err, store.ErrCompanyNotFound) {
		t.Errorf("CreateContact(nonexistent company): got err %v, want ErrCompanyNotFound", err)
	}
}

func containsContact(contacts []store.Contact, id int64) bool {
	for _, c := range contacts {
		if c.ID == id {
			return true
		}
	}
	return false
}
