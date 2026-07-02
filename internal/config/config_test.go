package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadExpandsTildeInYAMLPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "profile_path: ~/radar-test-profile.json\ndigest:\n  out_path: ~/radar-test-digest.md\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	cfg, err := Load(path, true)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	wantProfile := filepath.Join(home, "radar-test-profile.json")
	if cfg.ProfilePath != wantProfile {
		t.Errorf("ProfilePath = %q, want %q", cfg.ProfilePath, wantProfile)
	}
	wantOut := filepath.Join(home, "radar-test-digest.md")
	if cfg.Digest.OutPath != wantOut {
		t.Errorf("Digest.OutPath = %q, want %q", cfg.Digest.OutPath, wantOut)
	}
}

func TestLoadLeavesAbsolutePathsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "profile_path: /etc/radar/profile.json\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	cfg, err := Load(path, true)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.ProfilePath != "/etc/radar/profile.json" {
		t.Errorf("ProfilePath = %q, want unchanged absolute path", cfg.ProfilePath)
	}
}

func TestLoadDefaultsMatchConfig(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"), false)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Match.LLMTopK != 40 {
		t.Errorf("Match.LLMTopK = %d, want 40", cfg.Match.LLMTopK)
	}
	if cfg.Match.MinSemanticScore != 0 {
		t.Errorf("Match.MinSemanticScore = %v, want 0", cfg.Match.MinSemanticScore)
	}
	if cfg.Match.Triage.Enabled {
		t.Error("Match.Triage.Enabled = true, want false by default")
	}
}

func TestLoadRejectsInvalidMatchConfig(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{"zero top k", "match:\n  llm_top_k: 0\n"},
		{"negative top k", "match:\n  llm_top_k: -1\n"},
		{"negative min score", "match:\n  llm_top_k: 40\n  min_semantic_score: -0.1\n"},
		{"min score at 1", "match:\n  llm_top_k: 40\n  min_semantic_score: 1.0\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.yaml), 0o600); err != nil {
				t.Fatalf("writing test config: %v", err)
			}
			if _, err := Load(path, true); err == nil {
				t.Error("Load: got nil error, want a validation error")
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}

	tests := []struct {
		in   string
		want string
	}{
		{"~", home},
		{"~/foo/bar.json", filepath.Join(home, "foo", "bar.json")},
		{"/absolute/path.json", "/absolute/path.json"},
		{"relative/path.json", "relative/path.json"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := expandHome(tt.in); got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
