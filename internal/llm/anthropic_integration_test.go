package llm

import (
	"context"
	"os"
	"testing"
)

func TestAnthropicProviderFitVerdictRealAPI(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set; skipping integration test")
	}

	provider := NewAnthropicProvider(apiKey, "claude-haiku-4-5")
	verdict, err := provider.FitVerdict(context.Background(),
		"You are assessing job fit. The candidate is a Go backend engineer with 8 years of experience in distributed systems, PostgreSQL, and Kubernetes. They have no frontend or design experience.",
		"Title: Senior Backend Engineer\nCompany: Acme Corp\nLocation: Remote\n\nDescription:\nWe're looking for a backend engineer with strong Go experience, PostgreSQL, and distributed systems knowledge to join our platform team.",
	)
	if err != nil {
		t.Fatalf("FitVerdict: %v", err)
	}

	if verdict.Verdict != "pursue" && verdict.Verdict != "stretch" && verdict.Verdict != "skip" {
		t.Errorf("Verdict = %q, want one of pursue|stretch|skip", verdict.Verdict)
	}
	validTags := map[string]bool{
		"business-systems-analyst":   true,
		"implementation-specialist":  true,
		"technical-program-manager":  true,
		"technical-support-engineer": true,
		"automation-engineer":        true,
	}
	if !validTags[verdict.MatchedRoleTag] {
		t.Errorf("MatchedRoleTag = %q, want one of the five canonical tags", verdict.MatchedRoleTag)
	}
	if verdict.Reasoning == "" {
		t.Error("Reasoning is empty")
	}
}
