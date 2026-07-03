package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestListFollowUps(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-followup-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name: "Followup Test Co", ATSType: "greenhouse", ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	// Application with its own follow-up date due yesterday.
	postingA, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ext-fu-a",
		Title: "Role A", CanonicalKey: "followup test co|role a|", ContentHash: "hash-a",
	})
	if err != nil {
		t.Fatalf("UpsertPosting A: %v", err)
	}
	yesterday := time.Now().AddDate(0, 0, -1)
	appA, err := store.ApplyToPosting(ctx, pool, postingA.ID, "", false, &yesterday)
	if err != nil {
		t.Fatalf("ApplyToPosting A: %v", err)
	}

	// Application with a correspondence follow-up due today, no app-level date.
	postingB, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ext-fu-b",
		Title: "Role B", CanonicalKey: "followup test co|role b|", ContentHash: "hash-b",
	})
	if err != nil {
		t.Fatalf("UpsertPosting B: %v", err)
	}
	appB, err := store.ApplyToPosting(ctx, pool, postingB.ID, "", false, nil)
	if err != nil {
		t.Fatalf("ApplyToPosting B: %v", err)
	}
	today := time.Now()
	if _, err := store.LogCorrespondence(ctx, pool, store.NewCorrespondence{
		ApplicationID: appB.ID, Direction: "outbound", FollowUpNeeded: true, FollowUpDate: &today,
	}); err != nil {
		t.Fatalf("LogCorrespondence B: %v", err)
	}

	// Application with a future follow-up date: should not surface.
	postingC, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ext-fu-c",
		Title: "Role C", CanonicalKey: "followup test co|role c|", ContentHash: "hash-c",
	})
	if err != nil {
		t.Fatalf("UpsertPosting C: %v", err)
	}
	future := time.Now().AddDate(0, 1, 0)
	if _, err := store.ApplyToPosting(ctx, pool, postingC.ID, "", false, &future); err != nil {
		t.Fatalf("ApplyToPosting C: %v", err)
	}

	// Closed application with an overdue follow-up date: should not surface.
	postingD, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ext-fu-d",
		Title: "Role D", CanonicalKey: "followup test co|role d|", ContentHash: "hash-d",
	})
	if err != nil {
		t.Fatalf("UpsertPosting D: %v", err)
	}
	appD, err := store.ApplyToPosting(ctx, pool, postingD.ID, "", false, &yesterday)
	if err != nil {
		t.Fatalf("ApplyToPosting D: %v", err)
	}
	if _, err := store.CloseApplication(ctx, pool, appD.ID, "withdrawn"); err != nil {
		t.Fatalf("CloseApplication D: %v", err)
	}

	followUps, err := store.ListFollowUps(ctx, pool, 0)
	if err != nil {
		t.Fatalf("ListFollowUps: %v", err)
	}

	ids := make(map[int64]int)
	for _, f := range followUps {
		ids[f.ApplicationID]++
	}
	if ids[appA.ID] != 1 {
		t.Errorf("appA appeared %d times, want 1 (overdue application follow-up)", ids[appA.ID])
	}
	if ids[appB.ID] != 1 {
		t.Errorf("appB appeared %d times, want 1 (correspondence follow-up due today)", ids[appB.ID])
	}
	if _, ok := ids[appD.ID]; ok {
		t.Error("closed application D appeared in follow-ups, want excluded")
	}

	// With staleDays set, an application with old activity and no explicit
	// follow-up date should also surface.
	postingE, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ext-fu-e",
		Title: "Role E", CanonicalKey: "followup test co|role e|", ContentHash: "hash-e",
	})
	if err != nil {
		t.Fatalf("UpsertPosting E: %v", err)
	}
	appE, err := store.ApplyToPosting(ctx, pool, postingE.ID, "", false, nil)
	if err != nil {
		t.Fatalf("ApplyToPosting E: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE applications SET updated_at = now() - interval '30 days' WHERE id = $1", appE.ID); err != nil {
		t.Fatalf("backdating application E: %v", err)
	}

	staleFollowUps, err := store.ListFollowUps(ctx, pool, 20)
	if err != nil {
		t.Fatalf("ListFollowUps(stale=20): %v", err)
	}
	staleIDs := make(map[int64]bool)
	for _, f := range staleFollowUps {
		staleIDs[f.ApplicationID] = true
	}
	if !staleIDs[appE.ID] {
		t.Error("stale application E did not appear with --stale 20")
	}
}
