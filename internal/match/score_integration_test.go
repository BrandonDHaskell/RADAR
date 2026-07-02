package match_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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

// fixedVectorEmbedder always returns the same 1024-dimension vector,
// regardless of input text, so the profile embedding is trivially
// identical to a posting embedding seeded with the same vector: their
// cosine similarity is exactly 1.
type fixedVectorEmbedder struct{}

func (fixedVectorEmbedder) Dimension() int { return 1024 }

func (fixedVectorEmbedder) Embed(ctx context.Context, texts []string, inputType embed.InputType) ([][]float32, error) {
	vec := make([]float32, 1024)
	vec[0] = 1
	vectors := make([][]float32, len(texts))
	for i := range texts {
		vectors[i] = vec
	}
	return vectors, nil
}

type stubLLMProvider struct {
	verdict *llm.Verdict
	err     error
}

func (s stubLLMProvider) FitVerdict(ctx context.Context, systemPrompt, userPrompt string) (*llm.Verdict, error) {
	return s.verdict, s.err
}

func seedCompanyAndPosting(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (companyID, postingID int64) {
	t.Helper()
	token := fmt.Sprintf("test-score-%d", time.Now().UnixNano())

	company, err := store.CreateCompany(ctx, pool, store.NewCompany{
		Name:     "Score Test Co",
		ATSType:  "greenhouse",
		ATSToken: token,
	})
	if err != nil {
		t.Fatalf("CreateCompany: %v", err)
	}
	t.Cleanup(func() {
		pool.Exec(context.Background(), "DELETE FROM companies WHERE id = $1", company.ID)
	})

	posting, err := store.UpsertPosting(ctx, pool, store.PostingUpsert{
		CompanyID:    company.ID,
		Source:       "greenhouse",
		ExternalID:   "ext-1",
		Title:        "Automation Engineer",
		Description:  "Automate things.",
		CanonicalKey: "score test co|automation engineer|",
		ContentHash:  "hash-v1",
	})
	if err != nil {
		t.Fatalf("UpsertPosting: %v", err)
	}

	vec := make([]float32, 1024)
	vec[0] = 1
	if err := store.UpsertPostingEmbedding(ctx, pool, posting.ID, vec, "test-model"); err != nil {
		t.Fatalf("UpsertPostingEmbedding: %v", err)
	}

	return company.ID, posting.ID
}

func testProfile() *match.Profile {
	return &match.Profile{
		Summary: "Test operator.",
		Roles: []match.RoleSummary{
			{Tag: "automation-engineer", Label: "Automation Engineer", Summary: "Automates things."},
		},
	}
}

func TestScorePostingsSuccess(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	_, postingID := seedCompanyAndPosting(t, ctx, pool)

	llmProvider := stubLLMProvider{verdict: &llm.Verdict{
		Verdict:        "pursue",
		MatchedRoleTag: "automation-engineer",
		Reasoning:      "Strong match on automation experience.",
	}}

	res, err := match.ScorePostings(ctx, pool, fixedVectorEmbedder{}, llmProvider, "test-model", testProfile(), []int64{postingID})
	if err != nil {
		t.Fatalf("ScorePostings: %v", err)
	}
	if res.Scored != 1 || res.SemanticOnly != 0 {
		t.Errorf("ScorePostings result = %+v, want Scored=1 SemanticOnly=0", res)
	}

	var semanticScore float32
	var llmVerdict, matchedRoleTag, model string
	if err := pool.QueryRow(ctx,
		"SELECT semantic_score, llm_verdict, matched_role_tag, model FROM fit_scores WHERE posting_id = $1",
		postingID,
	).Scan(&semanticScore, &llmVerdict, &matchedRoleTag, &model); err != nil {
		t.Fatalf("querying fit_scores: %v", err)
	}
	if semanticScore < 0.999 {
		t.Errorf("semantic_score = %v, want ~1.0 (identical vectors)", semanticScore)
	}
	if llmVerdict != "pursue" {
		t.Errorf("llm_verdict = %q, want %q", llmVerdict, "pursue")
	}
	if matchedRoleTag != "automation-engineer" {
		t.Errorf("matched_role_tag = %q, want %q", matchedRoleTag, "automation-engineer")
	}
	if model != "test-model" {
		t.Errorf("model = %q, want %q", model, "test-model")
	}
}

func TestScorePostingsFallsBackToSemanticOnlyOnLLMFailure(t *testing.T) {
	pool := openTestPool(t)
	ctx := context.Background()
	_, postingID := seedCompanyAndPosting(t, ctx, pool)

	llmProvider := stubLLMProvider{err: errors.New("simulated provider failure")}

	res, err := match.ScorePostings(ctx, pool, fixedVectorEmbedder{}, llmProvider, "test-model", testProfile(), []int64{postingID})
	if err != nil {
		t.Fatalf("ScorePostings: %v", err)
	}
	if res.Scored != 0 || res.SemanticOnly != 1 {
		t.Errorf("ScorePostings result = %+v, want Scored=0 SemanticOnly=1", res)
	}

	var semanticScore float32
	var llmVerdict, matchedRoleTag, model *string
	var llmReasoning string
	if err := pool.QueryRow(ctx,
		"SELECT semantic_score, llm_verdict, matched_role_tag, model, llm_reasoning FROM fit_scores WHERE posting_id = $1",
		postingID,
	).Scan(&semanticScore, &llmVerdict, &matchedRoleTag, &model, &llmReasoning); err != nil {
		t.Fatalf("querying fit_scores: %v", err)
	}
	if semanticScore < 0.999 {
		t.Errorf("semantic_score = %v, want ~1.0 even though the LLM call failed", semanticScore)
	}
	if llmVerdict != nil {
		t.Errorf("llm_verdict = %v, want nil (LLM call failed)", *llmVerdict)
	}
	if !strings.Contains(llmReasoning, "semantic score only") {
		t.Errorf("llm_reasoning = %q, want it to flag semantic-only fallback", llmReasoning)
	}
}
