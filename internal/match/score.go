package match

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/BrandonDHaskell/RADAR/internal/embed"
	"github.com/BrandonDHaskell/RADAR/internal/llm"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

// ScoreResult summarizes what ScorePostings did, for CLI reporting.
type ScoreResult struct {
	Scored       int // semantic score and LLM verdict both computed
	SemanticOnly int // LLM verdict failed; semantic score alone was stored
}

// ScorePostings computes semantic and LLM fit scores for postingIDs against
// profile and writes the results to fit_scores.
//
// If the LLM verdict fails for a posting (provider error, malformed
// output, or a hallucinated role tag), that posting still gets its
// semantic score stored and is flagged in llm_reasoning rather than being
// silently dropped: the project spec requires parsing the LLM response
// defensively and falling back to semantic-only scoring on failure.
func ScorePostings(ctx context.Context, pool *pgxpool.Pool, embedder embed.Provider, llmProvider llm.Provider, llmModel string, profile *Profile, postingIDs []int64) (ScoreResult, error) {
	var res ScoreResult
	if len(postingIDs) == 0 {
		return res, nil
	}

	profileVectors, err := embedder.Embed(ctx, []string{profile.EmbeddingText()}, embed.InputTypeQuery)
	if err != nil {
		return res, fmt.Errorf("embedding profile: %w", err)
	}
	profileVector := profileVectors[0]

	semanticScores, err := store.SemanticScores(ctx, pool, profileVector, postingIDs)
	if err != nil {
		return res, err
	}
	semanticByID := make(map[int64]float32, len(semanticScores))
	for _, s := range semanticScores {
		semanticByID[s.PostingID] = s.Score
	}

	details, err := store.PostingDetails(ctx, pool, postingIDs)
	if err != nil {
		return res, err
	}

	systemPrompt := buildSystemPrompt(profile)

	for _, d := range details {
		var semanticPtr *float32
		if s, ok := semanticByID[d.ID]; ok {
			semanticPtr = &s
		}

		verdict, err := llmProvider.FitVerdict(ctx, systemPrompt, buildUserPrompt(d))
		if err != nil || !isValidVerdict(verdict) {
			reason := "semantic score only: LLM verdict unavailable"
			if err != nil {
				reason = fmt.Sprintf("semantic score only: LLM verdict failed (%v)", err)
			} else if verdict != nil {
				reason = fmt.Sprintf("semantic score only: LLM returned an invalid verdict %q / role tag %q", verdict.Verdict, verdict.MatchedRoleTag)
			}
			if upsertErr := store.UpsertFitScore(ctx, pool, store.FitScore{
				PostingID:     d.ID,
				SemanticScore: semanticPtr,
				LLMReasoning:  &reason,
			}); upsertErr != nil {
				return res, upsertErr
			}
			res.SemanticOnly++
			continue
		}

		model := llmModel
		if upsertErr := store.UpsertFitScore(ctx, pool, store.FitScore{
			PostingID:      d.ID,
			SemanticScore:  semanticPtr,
			LLMVerdict:     &verdict.Verdict,
			LLMScore:       verdict.Score,
			LLMReasoning:   &verdict.Reasoning,
			MatchedRoleTag: &verdict.MatchedRoleTag,
			Model:          &model,
		}); upsertErr != nil {
			return res, upsertErr
		}
		res.Scored++
	}

	return res, nil
}

// isValidVerdict rejects a response that structured outputs should have
// already prevented, but that the pipeline must not trust unconditionally:
// an unknown verdict value or a role tag the schema doesn't actually
// enforce at the database layer.
func isValidVerdict(v *llm.Verdict) bool {
	if v == nil {
		return false
	}
	if v.Verdict != "pursue" && v.Verdict != "stretch" && v.Verdict != "skip" {
		return false
	}
	for _, tag := range ValidRoleTags {
		if tag == v.MatchedRoleTag {
			return true
		}
	}
	return false
}

func buildSystemPrompt(profile *Profile) string {
	var b strings.Builder
	b.WriteString("You are assessing job posting fit for a real job seeker against their verified professional profile. ")
	b.WriteString("Honesty is a hard requirement: assess fit against the profile as written, and do not invent or inflate qualifications the profile does not support. ")
	b.WriteString("A confident skip is a valuable, correct output, not a failure. Your verdict and reasoning must be able to survive a technical follow-up question from a hiring manager. ")
	b.WriteString("If a posting's minimum qualifications gate on required domain expertise the profile does not show, lean toward stretch or skip rather than pursue.\n\n")

	b.WriteString("Operator profile:\n")
	b.WriteString(profile.Summary)
	b.WriteString("\n\n")
	for _, r := range profile.Roles {
		fmt.Fprintf(&b, "%s: %s\n", r.Label, r.Summary)
	}
	if len(profile.VerifiedSkills) > 0 {
		fmt.Fprintf(&b, "\nVerified skills: %s\n", strings.Join(profile.VerifiedSkills, ", "))
	}
	if len(profile.NotableExperience) > 0 {
		b.WriteString("\nNotable experience:\n")
		for _, e := range profile.NotableExperience {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}
	if len(profile.KnownGaps) > 0 {
		b.WriteString("\nKnown gaps (weigh these honestly):\n")
		for _, g := range profile.KnownGaps {
			fmt.Fprintf(&b, "- %s\n", g)
		}
	}
	if len(profile.Preferences.Locations) > 0 {
		fmt.Fprintf(&b, "\nLocation preferences: %s (remote ok: %v)\n",
			strings.Join(profile.Preferences.Locations, ", "), profile.Preferences.RemoteOK)
	}

	b.WriteString("\nRespond with a verdict (pursue, stretch, or skip), the single best-fitting role tag, a short reasoning grounded in the profile, and an optional numeric confidence score. ")
	b.WriteString("Do not use em dashes anywhere in your reasoning; use commas, colons, periods, or parentheses instead.")
	return b.String()
}

func buildUserPrompt(d store.PostingDetail) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Title: %s\nCompany: %s\n", d.Title, d.CompanyName)
	if d.Location != "" {
		fmt.Fprintf(&b, "Location: %s\n", d.Location)
	}
	if d.SalaryMin != nil || d.SalaryMax != nil {
		b.WriteString("Salary: ")
		if d.SalaryMin != nil {
			fmt.Fprintf(&b, "%.0f", *d.SalaryMin)
		}
		if d.SalaryMax != nil {
			fmt.Fprintf(&b, "-%.0f", *d.SalaryMax)
		}
		if d.SalaryCurrency != "" {
			fmt.Fprintf(&b, " %s", d.SalaryCurrency)
		}
		b.WriteString("\n")
	}
	b.WriteString("\nDescription:\n")
	b.WriteString(d.Description)
	return b.String()
}
