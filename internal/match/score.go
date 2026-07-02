package match

import (
	"fmt"
	"strings"

	"github.com/BrandonDHaskell/RADAR/internal/llm"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

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

// buildSystemPrompt is frozen: the honesty language here must not change
// without a deliberate, separately reviewed decision. Restructure callers
// around it rather than editing it in passing.
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
