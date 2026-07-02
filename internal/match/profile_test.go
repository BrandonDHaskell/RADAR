package match

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTestProfile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test profile: %v", err)
	}
	return path
}

func TestLoadProfileValid(t *testing.T) {
	path := writeTestProfile(t, `{
		"summary": "A test operator.",
		"roles": [{"tag": "automation-engineer", "label": "Automation Engineer", "summary": "Automates things."}],
		"verified_skills": ["Go", "SQL"],
		"notable_experience": ["Built a thing."],
		"known_gaps": ["No CS degree."],
		"preferences": {"locations": ["Remote"], "remote_ok": true}
	}`)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if p.Summary != "A test operator." {
		t.Errorf("Summary = %q", p.Summary)
	}
	if len(p.Roles) != 1 || p.Roles[0].Tag != "automation-engineer" {
		t.Errorf("Roles = %+v", p.Roles)
	}
	if !p.Preferences.RemoteOK {
		t.Error("Preferences.RemoteOK = false, want true")
	}
}

func TestLoadProfileNormalizesTypoRoleTag(t *testing.T) {
	path := writeTestProfile(t, `{
		"summary": "A test operator.",
		"roles": [{"tag": "automtion-engineer", "label": "Automation Engineer", "summary": "Automates things."}]
	}`)

	p, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if p.Roles[0].Tag != "automation-engineer" {
		t.Errorf("Roles[0].Tag = %q, want normalized %q", p.Roles[0].Tag, "automation-engineer")
	}
}

func TestLoadProfileRejectsUnknownRoleTag(t *testing.T) {
	path := writeTestProfile(t, `{
		"summary": "A test operator.",
		"roles": [{"tag": "chief-vibes-officer", "label": "Vibes", "summary": "Vibes."}]
	}`)

	if _, err := LoadProfile(path); err == nil {
		t.Fatal("LoadProfile: got nil error, want an error for an unknown role tag")
	}
}

func TestLoadProfileRejectsMissingSummary(t *testing.T) {
	path := writeTestProfile(t, `{"summary": ""}`)

	if _, err := LoadProfile(path); err == nil {
		t.Fatal("LoadProfile: got nil error, want an error for a missing summary")
	}
}

func TestLoadProfileMissingFile(t *testing.T) {
	if _, err := LoadProfile(filepath.Join(t.TempDir(), "does-not-exist.json")); err == nil {
		t.Fatal("LoadProfile: got nil error, want an error for a missing file")
	}
}

func TestProfileEmbeddingTextIncludesCoreFields(t *testing.T) {
	p := &Profile{
		Summary: "A test operator.",
		Roles: []RoleSummary{
			{Tag: "automation-engineer", Label: "Automation Engineer", Summary: "Automates things."},
		},
		VerifiedSkills:    []string{"Go", "SQL"},
		NotableExperience: []string{"Built a thing."},
	}

	text := p.EmbeddingText()
	for _, want := range []string{"A test operator.", "Automation Engineer", "Automates things.", "Go, SQL", "Built a thing."} {
		if !strings.Contains(text, want) {
			t.Errorf("EmbeddingText() = %q, want it to contain %q", text, want)
		}
	}
}
