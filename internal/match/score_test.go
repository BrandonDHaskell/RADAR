package match

import (
	"strings"
	"testing"

	"github.com/BrandonDHaskell/RADAR/internal/llm"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

func TestBuildSystemPromptIncludesHonestyRequirementsAndProfile(t *testing.T) {
	profile := &Profile{
		Summary: "A test operator summary.",
		Roles: []RoleSummary{
			{Tag: "automation-engineer", Label: "Automation Engineer", Summary: "Automates things."},
		},
		VerifiedSkills:    []string{"Go", "SQL"},
		NotableExperience: []string{"Built a thing."},
		KnownGaps:         []string{"No CS degree."},
		Preferences:       Preferences{Locations: []string{"Remote"}, RemoteOK: true},
	}

	prompt := buildSystemPrompt(profile)

	for _, want := range []string{
		"do not invent or inflate",
		"confident skip",
		"Do not use em dashes",
		"A test operator summary.",
		"Automation Engineer: Automates things.",
		"Go, SQL",
		"Built a thing.",
		"No CS degree.",
		"Remote",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildSystemPrompt() missing %q in:\n%s", want, prompt)
		}
	}
}

func TestBuildUserPromptIncludesPostingFields(t *testing.T) {
	min, max := 100000.0, 150000.0
	posting := store.PostingDetail{
		Title:          "Software Engineer",
		CompanyName:    "Acme",
		Location:       "Remote",
		Description:    "Build things.",
		SalaryMin:      &min,
		SalaryMax:      &max,
		SalaryCurrency: "USD",
	}

	prompt := buildUserPrompt(posting)

	for _, want := range []string{"Software Engineer", "Acme", "Remote", "100000", "150000", "USD", "Build things."} {
		if !strings.Contains(prompt, want) {
			t.Errorf("buildUserPrompt() missing %q in:\n%s", want, prompt)
		}
	}
}

func TestIsValidVerdict(t *testing.T) {
	tests := []struct {
		name string
		v    *llm.Verdict
		want bool
	}{
		{"nil", nil, false},
		{"valid pursue", &llm.Verdict{Verdict: "pursue", MatchedRoleTag: "automation-engineer"}, true},
		{"valid skip", &llm.Verdict{Verdict: "skip", MatchedRoleTag: "technical-program-manager"}, true},
		{"bad verdict value", &llm.Verdict{Verdict: "maybe", MatchedRoleTag: "automation-engineer"}, false},
		{"unknown role tag", &llm.Verdict{Verdict: "pursue", MatchedRoleTag: "chief-vibes-officer"}, false},
		{"typo role tag not auto-normalized here", &llm.Verdict{Verdict: "pursue", MatchedRoleTag: "automtion-engineer"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidVerdict(tt.v); got != tt.want {
				t.Errorf("isValidVerdict(%+v) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}
