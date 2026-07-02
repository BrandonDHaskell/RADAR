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
