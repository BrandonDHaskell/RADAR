package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestLogCorrespondence(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-corr-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name: "Correspondence Test Co", ATSType: "greenhouse", ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	posting, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ext-corr",
		Title: "Business Systems Analyst", CanonicalKey: "correspondence test co|business systems analyst|", ContentHash: "hash-1",
	})
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	application, err := store.ApplyToPosting(ctx, pool, posting.ID, "", false, nil)
	if err != nil {
		t.Fatalf("ApplyToPosting: %v", err)
	}

	contact, err := store.CreateContact(ctx, pool, store.NewContact{
		CompanyID: company.ID, Name: "Sam Hiring Manager", Type: "hiring_manager",
	})
	if err != nil {
		t.Fatalf("CreateContact: %v", err)
	}

	followUp := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	c, err := store.LogCorrespondence(ctx, pool, store.NewCorrespondence{
		ApplicationID: application.ID, ContactID: &contact.ID, Direction: "outbound",
		Channel: "email", Summary: "Sent follow-up", FollowUpNeeded: true, FollowUpDate: &followUp,
	})
	if err != nil {
		t.Fatalf("LogCorrespondence: %v", err)
	}
	if c.Direction != "outbound" {
		t.Errorf("Direction = %q, want %q", c.Direction, "outbound")
	}
	if c.ContactID == nil || *c.ContactID != contact.ID {
		t.Errorf("ContactID = %v, want %d", c.ContactID, contact.ID)
	}
	if !c.FollowUpNeeded {
		t.Error("FollowUpNeeded = false, want true")
	}
	if c.FollowUpDate == nil || !c.FollowUpDate.Equal(followUp) {
		t.Errorf("FollowUpDate = %v, want %v", c.FollowUpDate, followUp)
	}

	if _, err := store.LogCorrespondence(ctx, pool, store.NewCorrespondence{
		ApplicationID: 999999999, Direction: "inbound",
	}); !errors.Is(err, store.ErrApplicationNotFound) {
		t.Errorf("LogCorrespondence(nonexistent application): got err %v, want ErrApplicationNotFound", err)
	}

	ghostContact := int64(999999999)
	if _, err := store.LogCorrespondence(ctx, pool, store.NewCorrespondence{
		ApplicationID: application.ID, ContactID: &ghostContact, Direction: "inbound",
	}); !errors.Is(err, store.ErrContactNotFound) {
		t.Errorf("LogCorrespondence(nonexistent contact): got err %v, want ErrContactNotFound", err)
	}
}
