package store_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestApplyToPostingCreatesAndUpdates(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-apply-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name: "Apply Test Co", ATSType: "greenhouse", ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	posting, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ext-apply",
		Title: "Automation Engineer", CanonicalKey: "apply test co|automation engineer|", ContentHash: "hash-1",
	})
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	a, err := store.ApplyToPosting(ctx, pool, posting.ID, "general", true, nil)
	if err != nil {
		t.Fatalf("ApplyToPosting: %v", err)
	}
	if a.Status != "applied" {
		t.Errorf("Status = %q, want %q", a.Status, "applied")
	}
	if a.AppliedAt == nil {
		t.Error("AppliedAt is nil, want set")
	}
	if a.ResumeVariant != "general" {
		t.Errorf("ResumeVariant = %q, want %q", a.ResumeVariant, "general")
	}
	firstAppliedAt := *a.AppliedAt

	followUp := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	updated, err := store.ApplyToPosting(ctx, pool, posting.ID, "swe-focused", false, &followUp)
	if err != nil {
		t.Fatalf("ApplyToPosting (re-run): %v", err)
	}
	if updated.ID != a.ID {
		t.Errorf("re-applying created a new row: %d != %d", updated.ID, a.ID)
	}
	if updated.ResumeVariant != "swe-focused" {
		t.Errorf("ResumeVariant after re-run = %q, want %q", updated.ResumeVariant, "swe-focused")
	}
	if updated.UsedCoverLetter {
		t.Error("UsedCoverLetter after re-run = true, want false")
	}
	if updated.NextFollowUpDate == nil || !updated.NextFollowUpDate.Equal(followUp) {
		t.Errorf("NextFollowUpDate = %v, want %v", updated.NextFollowUpDate, followUp)
	}
	if !updated.AppliedAt.Equal(firstAppliedAt) {
		t.Errorf("AppliedAt changed on re-run: %v != %v (should be preserved)", updated.AppliedAt, firstAppliedAt)
	}

	if _, err := store.ApplyToPosting(ctx, pool, 999999999, "", false, nil); !errors.Is(err, store.ErrPostingNotFound) {
		t.Errorf("ApplyToPosting(nonexistent posting): got err %v, want ErrPostingNotFound", err)
	}
}

func TestCloseApplication(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-close-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name: "Close Test Co", ATSType: "greenhouse", ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	posting, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ext-close",
		Title: "Technical Support Engineer", CanonicalKey: "close test co|technical support engineer|", ContentHash: "hash-1",
	})
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	a, err := store.ApplyToPosting(ctx, pool, posting.ID, "", false, nil)
	if err != nil {
		t.Fatalf("ApplyToPosting: %v", err)
	}

	closed, err := store.CloseApplication(ctx, pool, a.ID, "closed_rejected")
	if err != nil {
		t.Fatalf("CloseApplication: %v", err)
	}
	if closed.Status != "closed_rejected" {
		t.Errorf("Status = %q, want %q", closed.Status, "closed_rejected")
	}

	if _, err := store.CloseApplication(ctx, pool, 999999999, "withdrawn"); !errors.Is(err, store.ErrApplicationNotFound) {
		t.Errorf("CloseApplication(nonexistent): got err %v, want ErrApplicationNotFound", err)
	}
}
