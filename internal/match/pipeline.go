package match

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BrandonDHaskell/RADAR/internal/embed"
	"github.com/BrandonDHaskell/RADAR/internal/llm"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

// PipelineResult summarizes one RunPipeline call, for CLI reporting.
type PipelineResult struct {
	Screened          int
	Excluded          int
	Embedded          int
	SemanticScored    int64
	VerdictsRequested int
	VerdictsWritten   int
	VerdictsFailed    int
}

// RunPipeline runs the corpus-wide evaluation pipeline once: screen
// (Stage 0), embed new or changed postings (Stage 2), recompute semantic
// scores (Stage 3), then request LLM verdicts for the top llmTopK
// candidates by semantic score (Stage 4, optionally floored at
// minSemanticScore).
//
// Callers should run this once per sync regardless of whether the fetch
// loop that preceded it was filtered to one company: the invariants here
// (one profile embedding, one global ranking, one shortlist) are
// corpus-level, and every stage is cheap when there is nothing new to do.
//
// Each stage's error is collected rather than aborting the pipeline,
// since a later stage can still usefully run against data an earlier
// stage already produced in a prior run (for example, Stage 4 can still
// verdict previously embedded postings even if Stage 2 failed this time).
func RunPipeline(ctx context.Context, pool *pgxpool.Pool, embedder embed.Provider, embedModel string, llmProvider llm.Provider, llmModel string, profile *Profile, llmTopK int, minSemanticScore float64) (PipelineResult, error) {
	var res PipelineResult
	var errs []error

	screenRes, err := ScreenPostings(ctx, pool, profile)
	if err != nil {
		errs = append(errs, fmt.Errorf("screening: %w", err))
	}
	res.Screened, res.Excluded = screenRes.Screened, screenRes.Excluded

	embedded, err := embedPostings(ctx, pool, embedder, embedModel)
	if err != nil {
		errs = append(errs, fmt.Errorf("embedding: %w", err))
	}
	res.Embedded = embedded

	profileVectors, err := embedder.Embed(ctx, []string{profile.EmbeddingText()}, embed.InputTypeQuery)
	if err != nil {
		errs = append(errs, fmt.Errorf("embedding profile: %w", err))
	} else {
		scored, err := store.RefreshSemanticScores(ctx, pool, profileVectors[0])
		if err != nil {
			errs = append(errs, fmt.Errorf("refreshing semantic scores: %w", err))
		}
		res.SemanticScored = scored
	}

	requested, written, failed, err := runVerdictStage(ctx, pool, llmProvider, llmModel, profile, llmTopK, minSemanticScore)
	if err != nil {
		errs = append(errs, fmt.Errorf("verdicts: %w", err))
	}
	res.VerdictsRequested, res.VerdictsWritten, res.VerdictsFailed = requested, written, failed

	return res, errors.Join(errs...)
}

// embedPostings is Stage 2: embed every open, screened-in posting whose
// embedding is missing or stale, batched across companies in one call to
// the provider (which itself chunks further as needed).
func embedPostings(ctx context.Context, pool *pgxpool.Pool, embedder embed.Provider, model string) (int, error) {
	candidates, err := store.EmbeddingCandidates(ctx, pool)
	if err != nil {
		return 0, err
	}
	if len(candidates) == 0 {
		return 0, nil
	}

	texts := make([]string, len(candidates))
	for i, c := range candidates {
		texts[i] = formatEmbeddingText(c.CompanyName, c.Title, c.Department, c.Location, c.Description)
	}

	vectors, err := embedder.Embed(ctx, texts, embed.InputTypeDocument)
	if err != nil {
		return 0, err
	}

	var embedded int
	for i, c := range candidates {
		if err := store.UpsertPostingEmbedding(ctx, pool, c.PostingID, vectors[i], model, c.ContentHash); err != nil {
			return embedded, err
		}
		embedded++
	}
	return embedded, nil
}

// formatEmbeddingText composes the text sent to the embedding provider for
// a posting: the fields that carry its semantic content.
func formatEmbeddingText(companyName, title, department, location, description string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s at %s\n", title, companyName)
	if department != "" {
		fmt.Fprintf(&b, "Department: %s\n", department)
	}
	if location != "" {
		fmt.Fprintf(&b, "Location: %s\n", location)
	}
	if description != "" {
		b.WriteString("\n")
		b.WriteString(description)
	}
	return strings.TrimSpace(b.String())
}

// runVerdictStage is Stage 4: build the verdict candidate pool, take the
// top llmTopK by semantic score, and request a verdict for each. A failed
// or invalid verdict is written with only the failure note and no hashes,
// which leaves the posting in the pool for automatic retry next run; no
// separate retry bookkeeping is needed.
func runVerdictStage(ctx context.Context, pool *pgxpool.Pool, llmProvider llm.Provider, llmModel string, profile *Profile, llmTopK int, minSemanticScore float64) (requested, written, failed int, err error) {
	candidateIDs, err := store.VerdictCandidatePool(ctx, pool, profile.Hash, minSemanticScore, llmTopK)
	if err != nil {
		return 0, 0, 0, err
	}
	if len(candidateIDs) == 0 {
		return 0, 0, 0, nil
	}

	details, err := store.PostingDetails(ctx, pool, candidateIDs)
	if err != nil {
		return 0, 0, 0, err
	}
	requested = len(details)

	systemPrompt := buildSystemPrompt(profile)

	for _, d := range details {
		verdict, verdictErr := llmProvider.FitVerdict(ctx, systemPrompt, buildUserPrompt(d))
		if verdictErr != nil || !isValidVerdict(verdict) {
			reason := "LLM verdict unavailable"
			switch {
			case verdictErr != nil:
				reason = fmt.Sprintf("LLM verdict failed: %v", verdictErr)
			case verdict != nil:
				reason = fmt.Sprintf("LLM returned an invalid verdict %q / role tag %q", verdict.Verdict, verdict.MatchedRoleTag)
			}
			if upsertErr := store.UpsertVerdict(ctx, pool, store.Verdict{
				PostingID:    d.ID,
				LLMReasoning: &reason,
			}); upsertErr != nil {
				return requested, written, failed, upsertErr
			}
			failed++
			continue
		}

		model := llmModel
		profileHash := profile.Hash
		contentHash := d.ContentHash
		if upsertErr := store.UpsertVerdict(ctx, pool, store.Verdict{
			PostingID:          d.ID,
			LLMVerdict:         &verdict.Verdict,
			LLMScore:           verdict.Score,
			LLMReasoning:       &verdict.Reasoning,
			MatchedRoleTag:     &verdict.MatchedRoleTag,
			Model:              &model,
			VerdictProfileHash: &profileHash,
			VerdictContentHash: &contentHash,
		}); upsertErr != nil {
			return requested, written, failed, upsertErr
		}
		written++
	}

	return requested, written, failed, nil
}
