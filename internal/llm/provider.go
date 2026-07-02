// Package llm defines the LLM provider interface used to generate fit
// verdicts, plus a Claude (Anthropic) implementation. The pipeline is
// written against the interface so a different provider (hosted or local)
// can be swapped in without touching callers.
package llm

import "context"

// Verdict is the LLM's structured fit assessment for one posting against
// the operator's profile.
type Verdict struct {
	Verdict        string   `json:"verdict"` // pursue | stretch | skip
	MatchedRoleTag string   `json:"matched_role_tag"`
	Reasoning      string   `json:"reasoning"`
	Score          *float64 `json:"score,omitempty"`
}

// Provider generates a structured fit verdict from a system and user
// prompt. Implementations must return JSON matching Verdict's shape;
// callers should still treat the result defensively, since honesty and
// correctness here matter more than uptime (see project spec Section 6).
type Provider interface {
	FitVerdict(ctx context.Context, systemPrompt, userPrompt string) (*Verdict, error)
}
