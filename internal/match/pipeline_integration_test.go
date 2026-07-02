package match_test

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BrandonDHaskell/RADAR/internal/embed"
	"github.com/BrandonDHaskell/RADAR/internal/llm"
	"github.com/BrandonDHaskell/RADAR/internal/match"
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

// countingEmbedder returns a fixed vector for every input and counts how
// many Embed calls it received, so tests can assert the pipeline embeds
// the profile exactly once regardless of how many postings it also embeds.
type countingEmbedder struct {
	calls int32
}

func (c *countingEmbedder) Dimension() int { return 1024 }

func (c *countingEmbedder) Embed(ctx context.Context, texts []string, inputType embed.InputType) ([][]float32, error) {
	atomic.AddInt32(&c.calls, 1)
	vec := make([]float32, 1024)
	vec[0] = 1
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = vec
	}
	return vectors, nil
}

// countingLLM always returns the same verdict and counts how many times it
// was called, so tests can assert the top-K gate actually limits calls.
type countingLLM struct {
	calls int32
}

func (l *countingLLM) FitVerdict(ctx context.Context, systemPrompt, userPrompt string) (*llm.Verdict, error) {
	atomic.AddInt32(&l.calls, 1)
	return &llm.Verdict{
		Verdict:        "pursue",
		MatchedRoleTag: "automation-engineer",
		Reasoning:      "Fake verdict for pipeline integration test.",
	}, nil
}

func testPipelineProfile(hash string) *match.Profile {
	p := &match.Profile{
		Summary: "Test operator for pipeline integration tests.",
		Roles: []match.RoleSummary{
			{Tag: "automation-engineer", Label: "Automation Engineer", Summary: "Automates things."},
		},
		Preferences: match.Preferences{
			RemoteOK:        true,
			TitleExclusions: []string{"executive assistant"},
		},
	}
	p.Hash = hash
	return p
}

func TestRunPipelineScreensEmbedsScoresAndVerdicts(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-pipeline-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name: "Pipeline Test Co", ATSType: "greenhouse", ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	// A posting that must die at Stage 0: never embedded, never LLM'd.
	excludedPosting, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ea", Title: "Executive Assistant to the CEO",
		CanonicalKey: "pipeline test co|executive assistant|", ContentHash: "hash-ea",
	})
	if err != nil {
		t.Fatalf("UpsertPosting(excluded): %v", err)
	}

	// A posting that should flow through every stage and get a verdict.
	passingPosting, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID: company.ID, Source: "greenhouse", ExternalID: "ok", Title: "Automation Engineer",
		Description:  "Build automation tooling.",
		CanonicalKey: "pipeline test co|automation engineer|", ContentHash: "hash-ok",
	})
	if err != nil {
		t.Fatalf("UpsertPosting(passing): %v", err)
	}

	profile := testPipelineProfile("pipeline-test-hash")
	embedder := &countingEmbedder{}
	llmProvider := &countingLLM{}

	res, err := match.RunPipeline(ctx, pool, embedder, "test-embed-model", llmProvider, "test-llm-model", profile, 40, 0)
	if err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}

	if res.Screened != 2 {
		t.Errorf("Screened = %d, want 2", res.Screened)
	}
	if res.Excluded != 1 {
		t.Errorf("Excluded = %d, want 1", res.Excluded)
	}
	if res.Embedded != 1 {
		t.Errorf("Embedded = %d, want 1 (only the passing posting)", res.Embedded)
	}
	if res.VerdictsRequested != 1 || res.VerdictsWritten != 1 {
		t.Errorf("VerdictsRequested/Written = %d/%d, want 1/1", res.VerdictsRequested, res.VerdictsWritten)
	}

	// Exactly one profile embedding call plus one posting-embedding call.
	if got := atomic.LoadInt32(&embedder.calls); got != 2 {
		t.Errorf("embedder.calls = %d, want 2 (one profile embed, one posting embed batch)", got)
	}
	if got := atomic.LoadInt32(&llmProvider.calls); got != 1 {
		t.Errorf("llmProvider.calls = %d, want 1 (only the passing posting)", got)
	}

	// The excluded posting: no embedding, no fit_scores row, screen_status excluded.
	var screenStatus, screenReason string
	if err := pool.QueryRow(ctx, "SELECT screen_status, coalesce(screen_reason, '') FROM postings WHERE id = $1", excludedPosting.ID).
		Scan(&screenStatus, &screenReason); err != nil {
		t.Fatalf("querying excluded posting screen state: %v", err)
	}
	if screenStatus != "excluded" || screenReason != "title_exclusion:executive assistant" {
		t.Errorf("excluded posting screen = (%q, %q), want (%q, %q)", screenStatus, screenReason, "excluded", "title_exclusion:executive assistant")
	}
	var embCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM posting_embeddings WHERE posting_id = $1", excludedPosting.ID).Scan(&embCount); err != nil {
		t.Fatalf("counting excluded posting embeddings: %v", err)
	}
	if embCount != 0 {
		t.Error("excluded posting has an embedding, want none (it must never reach Stage 2)")
	}

	// The passing posting: embedded, scored, and verdicted.
	var verdict, roleTag string
	var semanticScore float32
	if err := pool.QueryRow(ctx, "SELECT llm_verdict, matched_role_tag, semantic_score FROM fit_scores WHERE posting_id = $1", passingPosting.ID).
		Scan(&verdict, &roleTag, &semanticScore); err != nil {
		t.Fatalf("querying passing posting fit_scores: %v", err)
	}
	if verdict != "pursue" || roleTag != "automation-engineer" {
		t.Errorf("passing posting verdict = (%q, %q), want (%q, %q)", verdict, roleTag, "pursue", "automation-engineer")
	}
	if semanticScore < 0.999 {
		t.Errorf("passing posting semantic_score = %v, want ~1.0", semanticScore)
	}

	// Excluded postings must never appear in the digest.
	digestPostings, err := store.DigestPostings(ctx, pool, profile.Hash, "", 100)
	if err != nil {
		t.Fatalf("DigestPostings: %v", err)
	}
	for _, p := range digestPostings {
		if p.ID == excludedPosting.ID {
			t.Error("excluded posting appears in the digest, want it absent")
		}
	}

	excludedList, err := store.ExcludedPostings(ctx, pool, 20)
	if err != nil {
		t.Fatalf("ExcludedPostings: %v", err)
	}
	foundInExcludedList := false
	for _, e := range excludedList {
		if e.Title == "Executive Assistant to the CEO" {
			foundInExcludedList = true
		}
	}
	if !foundInExcludedList {
		t.Error("excluded posting missing from ExcludedPostings, want it listed for operator review")
	}
}

func TestRunPipelineRespectsLLMTopK(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	token := fmt.Sprintf("test-topk-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name: "TopK Test Co", ATSType: "greenhouse", ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() { pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID) })

	for i := 0; i < 5; i++ {
		externalID := fmt.Sprintf("topk-%d", i)
		if _, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
			CompanyID: company.ID, Source: "greenhouse", ExternalID: externalID, Title: "Automation Engineer " + externalID,
			CanonicalKey: "topk test co|automation engineer|" + externalID, ContentHash: "hash-" + externalID,
		}); err != nil {
			t.Fatalf("UpsertPosting(%s): %v", externalID, err)
		}
	}

	profile := testPipelineProfile("topk-test-hash")
	embedder := &countingEmbedder{}
	llmProvider := &countingLLM{}

	res, err := match.RunPipeline(ctx, pool, embedder, "test-embed-model", llmProvider, "test-llm-model", profile, 2, 0)
	if err != nil {
		t.Fatalf("RunPipeline: %v", err)
	}

	if res.VerdictsRequested != 2 {
		t.Errorf("VerdictsRequested = %d, want 2 (llm_top_k)", res.VerdictsRequested)
	}
	if got := atomic.LoadInt32(&llmProvider.calls); got != 2 {
		t.Errorf("llmProvider.calls = %d, want 2 (llm_top_k gate)", got)
	}
}
