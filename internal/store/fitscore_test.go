package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestRefreshSemanticScoresDoesNotTouchVerdictColumns(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-semantic-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{Name: "Semantic Test Co", ATSType: "greenhouse", ATSToken: token})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	posting, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ext-1", Title: "Automation Engineer",
		CanonicalKey: "semantic test co|automation engineer|", ContentHash: "hash-v1",
	})
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}
	if err := store.SetPostingScreen(ctx, pool, posting.ID, "passed", nil, "any-hash"); err != nil {
		t.Fatalf("SetPostingScreen: %v", err)
	}
	vec := make([]float32, 1024)
	vec[0] = 1
	if err := store.UpsertPostingEmbedding(ctx, pool, posting.ID, vec, "test-model", "hash-v1"); err != nil {
		t.Fatalf("UpsertPostingEmbedding: %v", err)
	}

	// Seed a verdict first, to prove RefreshSemanticScores leaves it alone.
	v, ph, ch := "pursue", "profile-hash", "hash-v1"
	if err := store.UpsertVerdict(ctx, pool, store.Verdict{
		PostingID: posting.ID, LLMVerdict: &v, VerdictProfileHash: &ph, VerdictContentHash: &ch,
	}); err != nil {
		t.Fatalf("UpsertVerdict: %v", err)
	}

	scored, err := store.RefreshSemanticScores(ctx, pool, vec)
	if err != nil {
		t.Fatalf("RefreshSemanticScores: %v", err)
	}
	if scored < 1 {
		t.Errorf("RefreshSemanticScores rows affected = %d, want at least 1", scored)
	}

	var semanticScore float32
	var llmVerdict, verdictProfileHash string
	if err := pool.QueryRow(ctx,
		"SELECT semantic_score, llm_verdict, verdict_profile_hash FROM fit_scores WHERE posting_id = $1", posting.ID,
	).Scan(&semanticScore, &llmVerdict, &verdictProfileHash); err != nil {
		t.Fatalf("querying fit_scores: %v", err)
	}
	if semanticScore < 0.999 {
		t.Errorf("semantic_score = %v, want ~1.0 (identical vectors)", semanticScore)
	}
	if llmVerdict != "pursue" || verdictProfileHash != "profile-hash" {
		t.Errorf("RefreshSemanticScores touched verdict columns: llm_verdict=%q verdict_profile_hash=%q, want unchanged", llmVerdict, verdictProfileHash)
	}
}

func TestRefreshSemanticScoresExcludesUnscreenedClosedAndStaleEmbeddings(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-semantic-excl-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{Name: "Semantic Excl Co", ATSType: "greenhouse", ATSToken: token})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	vec := make([]float32, 1024)
	vec[0] = 1

	mk := func(externalID string) int64 {
		p, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
			CompanyID: company.ID, Source: "greenhouse", ExternalID: externalID, Title: "Role " + externalID,
			CanonicalKey: "semantic excl co|role|" + externalID, ContentHash: "hash-" + externalID,
		})
		if err != nil {
			t.Fatalf("UpsertPosting(%s): %v", externalID, err)
		}
		return p.ID
	}

	pendingID := mk("pending") // never screened: screen_status stays 'pending'
	if err := store.UpsertPostingEmbedding(ctx, pool, pendingID, vec, "test-model", "hash-pending"); err != nil {
		t.Fatalf("UpsertPostingEmbedding: %v", err)
	}

	closedID := mk("closed")
	if err := store.SetPostingScreen(ctx, pool, closedID, "passed", nil, "h"); err != nil {
		t.Fatalf("SetPostingScreen: %v", err)
	}
	if err := store.UpsertPostingEmbedding(ctx, pool, closedID, vec, "test-model", "hash-closed"); err != nil {
		t.Fatalf("UpsertPostingEmbedding: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE postings SET is_open = false WHERE id = $1", closedID); err != nil {
		t.Fatalf("closing posting: %v", err)
	}

	staleEmbeddingID := mk("stale-embed")
	if err := store.SetPostingScreen(ctx, pool, staleEmbeddingID, "passed", nil, "h"); err != nil {
		t.Fatalf("SetPostingScreen: %v", err)
	}
	// Embed with a content_hash that does NOT match the posting's current one.
	if err := store.UpsertPostingEmbedding(ctx, pool, staleEmbeddingID, vec, "test-model", "outdated-hash"); err != nil {
		t.Fatalf("UpsertPostingEmbedding: %v", err)
	}

	if _, err := store.RefreshSemanticScores(ctx, pool, vec); err != nil {
		t.Fatalf("RefreshSemanticScores: %v", err)
	}

	for _, id := range []int64{pendingID, closedID, staleEmbeddingID} {
		var count int
		if err := pool.QueryRow(ctx, "SELECT count(*) FROM fit_scores WHERE posting_id = $1", id).Scan(&count); err != nil {
			t.Fatalf("counting fit_scores for %d: %v", id, err)
		}
		if count != 0 {
			t.Errorf("posting %d got a semantic score, want excluded (pending/closed/stale-embedding)", id)
		}
	}
}

func TestVerdictCandidatePool(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-pool-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{Name: "Pool Test Co", ATSType: "greenhouse", ATSToken: token})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	const profileHash = "current-profile-hash"
	vec := make([]float32, 1024)
	vec[0] = 1

	mkScreened := func(externalID string, score float32) int64 {
		t.Helper()
		p, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
			CompanyID: company.ID, Source: "greenhouse", ExternalID: externalID, Title: "Role " + externalID,
			CanonicalKey: "pool test co|role|" + externalID, ContentHash: "hash-" + externalID,
		})
		if err != nil {
			t.Fatalf("UpsertPosting(%s): %v", externalID, err)
		}
		if err := store.SetPostingScreen(ctx, pool, p.ID, "passed", nil, profileHash); err != nil {
			t.Fatalf("SetPostingScreen(%s): %v", externalID, err)
		}
		if err := store.UpsertPostingEmbedding(ctx, pool, p.ID, vec, "test-model", "hash-"+externalID); err != nil {
			t.Fatalf("UpsertPostingEmbedding(%s): %v", externalID, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO fit_scores (posting_id, semantic_score, computed_at) VALUES ($1, $2, now())
			ON CONFLICT (posting_id) DO UPDATE SET semantic_score = EXCLUDED.semantic_score
		`, p.ID, score); err != nil {
			t.Fatalf("seeding semantic score(%s): %v", externalID, err)
		}
		return p.ID
	}

	noVerdictID := mkScreened("no-verdict", 0.9)

	failedID := mkScreened("failed", 0.8)
	failReason := "LLM verdict failed: simulated"
	if err := store.UpsertVerdict(ctx, pool, store.Verdict{PostingID: failedID, LLMReasoning: &failReason}); err != nil {
		t.Fatalf("UpsertVerdict(failed): %v", err)
	}

	freshID := mkScreened("fresh", 0.85)
	fv, fph, fch := "pursue", profileHash, "hash-fresh"
	if err := store.UpsertVerdict(ctx, pool, store.Verdict{PostingID: freshID, LLMVerdict: &fv, VerdictProfileHash: &fph, VerdictContentHash: &fch}); err != nil {
		t.Fatalf("UpsertVerdict(fresh): %v", err)
	}

	profileStaleID := mkScreened("profile-stale", 0.7)
	pv, pph, pch := "skip", "old-profile-hash", "hash-profile-stale"
	if err := store.UpsertVerdict(ctx, pool, store.Verdict{PostingID: profileStaleID, LLMVerdict: &pv, VerdictProfileHash: &pph, VerdictContentHash: &pch}); err != nil {
		t.Fatalf("UpsertVerdict(profile-stale): %v", err)
	}

	contentStaleID := mkScreened("content-stale", 0.6)
	cv, cph, cch := "skip", profileHash, "old-content-hash"
	if err := store.UpsertVerdict(ctx, pool, store.Verdict{PostingID: contentStaleID, LLMVerdict: &cv, VerdictProfileHash: &cph, VerdictContentHash: &cch}); err != nil {
		t.Fatalf("UpsertVerdict(content-stale): %v", err)
	}

	belowFloorID := mkScreened("below-floor", 0.1)

	t.Run("selects no-verdict, failed, profile-stale, and content-stale, excludes fresh", func(t *testing.T) {
		got, err := store.VerdictCandidatePool(ctx, pool, profileHash, 0, 100)
		if err != nil {
			t.Fatalf("VerdictCandidatePool: %v", err)
		}
		gotSet := map[int64]bool{}
		for _, id := range got {
			gotSet[id] = true
		}
		for _, id := range []int64{noVerdictID, failedID, profileStaleID, contentStaleID, belowFloorID} {
			if !gotSet[id] {
				t.Errorf("posting %d missing from pool, want it present", id)
			}
		}
		if gotSet[freshID] {
			t.Error("fresh-verdict posting present in pool, want it excluded")
		}
	})

	t.Run("min_semantic_score floors out low-scoring postings", func(t *testing.T) {
		got, err := store.VerdictCandidatePool(ctx, pool, profileHash, 0.5, 100)
		if err != nil {
			t.Fatalf("VerdictCandidatePool: %v", err)
		}
		for _, id := range got {
			if id == belowFloorID {
				t.Error("below-floor posting present despite min_semantic_score=0.5")
			}
		}
	})

	t.Run("limit caps the pool, ranked by semantic score descending", func(t *testing.T) {
		got, err := store.VerdictCandidatePool(ctx, pool, profileHash, 0, 1)
		if err != nil {
			t.Fatalf("VerdictCandidatePool: %v", err)
		}
		if len(got) != 1 || got[0] != noVerdictID {
			t.Errorf("VerdictCandidatePool(limit=1) = %v, want [%d] (highest semantic score)", got, noVerdictID)
		}
	})

	t.Run("a successful verdict removes the posting from the pool", func(t *testing.T) {
		v, ph, ch := "stretch", profileHash, "hash-no-verdict"
		if err := store.UpsertVerdict(ctx, pool, store.Verdict{PostingID: noVerdictID, LLMVerdict: &v, VerdictProfileHash: &ph, VerdictContentHash: &ch}); err != nil {
			t.Fatalf("UpsertVerdict: %v", err)
		}
		got, err := store.VerdictCandidatePool(ctx, pool, profileHash, 0, 100)
		if err != nil {
			t.Fatalf("VerdictCandidatePool: %v", err)
		}
		for _, id := range got {
			if id == noVerdictID {
				t.Error("posting still in pool after a successful verdict was written")
			}
		}
	})

	t.Run("a failed verdict stays in the pool", func(t *testing.T) {
		reason := "LLM verdict failed: still broken"
		if err := store.UpsertVerdict(ctx, pool, store.Verdict{PostingID: failedID, LLMReasoning: &reason}); err != nil {
			t.Fatalf("UpsertVerdict: %v", err)
		}
		got, err := store.VerdictCandidatePool(ctx, pool, profileHash, 0, 100)
		if err != nil {
			t.Fatalf("VerdictCandidatePool: %v", err)
		}
		found := false
		for _, id := range got {
			if id == failedID {
				found = true
			}
		}
		if !found {
			t.Error("failed-verdict posting missing from pool, want it retried automatically")
		}
	})
}
