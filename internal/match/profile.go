// Package match owns the operator's profile: loading it from profile.json
// and, in a later phase, using it to compute semantic and LLM fit scores
// against postings.
package match

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
)

// ValidRoleTags are the operator's canonical role tags. Spelling must match
// exactly; these also appear as the fit_scores.matched_role_tag CHECK
// constraint values in the database schema.
var ValidRoleTags = []string{
	"business-systems-analyst",
	"implementation-specialist",
	"technical-program-manager",
	"technical-support-engineer",
	"automation-engineer",
}

// automationEngineerTypo is a known misspelling that has appeared in
// imported resume data; LoadProfile normalizes it on the way in.
const automationEngineerTypo = "automtion-engineer"

// RoleSummary is a role-tag-specific framing of the operator's background,
// used both for the profile embedding and, in a later phase, to help the
// LLM pick the best-fitting role tag for a posting.
type RoleSummary struct {
	Tag     string `json:"tag"`
	Label   string `json:"label"`
	Summary string `json:"summary"`
}

// Preferences steers location fit and honest stretch/skip verdicts.
type Preferences struct {
	Locations []string `json:"locations"`
	RemoteOK  bool     `json:"remote_ok"`
}

// Profile is the operator's verified, compact representation of themself,
// loaded from profile.json (Section 15 of the project spec). Only
// verified_skills, notable_experience, and known_gaps should be edited by
// hand; the matching pipeline treats this as ground truth and must not
// invent or inflate anything beyond it.
type Profile struct {
	Summary           string        `json:"summary"`
	Roles             []RoleSummary `json:"roles"`
	VerifiedSkills    []string      `json:"verified_skills"`
	NotableExperience []string      `json:"notable_experience"`
	KnownGaps         []string      `json:"known_gaps"`
	Preferences       Preferences   `json:"preferences"`
}

// LoadProfile reads and validates the profile at path.
func LoadProfile(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile %s: %w", path, err)
	}

	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile %s: %w", path, err)
	}

	for i, r := range p.Roles {
		if r.Tag == automationEngineerTypo {
			p.Roles[i].Tag = "automation-engineer"
		}
	}

	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("profile %s: %w", path, err)
	}
	return &p, nil
}

func (p *Profile) validate() error {
	if strings.TrimSpace(p.Summary) == "" {
		return fmt.Errorf("summary is required")
	}
	for _, r := range p.Roles {
		if !slices.Contains(ValidRoleTags, r.Tag) {
			return fmt.Errorf("role tag %q is not one of %v", r.Tag, ValidRoleTags)
		}
	}
	return nil
}

// EmbeddingText composes a single block of text representing the whole
// profile, suitable for one embedding call. The project spec calls for
// embedding the profile once, rather than one vector per role tag.
func (p *Profile) EmbeddingText() string {
	var b strings.Builder

	b.WriteString(p.Summary)

	if len(p.Roles) > 0 {
		b.WriteString("\n\n")
		for _, r := range p.Roles {
			fmt.Fprintf(&b, "%s: %s\n", r.Label, r.Summary)
		}
	}

	if len(p.VerifiedSkills) > 0 {
		fmt.Fprintf(&b, "\nSkills: %s\n", strings.Join(p.VerifiedSkills, ", "))
	}

	if len(p.NotableExperience) > 0 {
		b.WriteString("\nExperience:\n")
		for _, e := range p.NotableExperience {
			fmt.Fprintf(&b, "- %s\n", e)
		}
	}

	return strings.TrimSpace(b.String())
}
