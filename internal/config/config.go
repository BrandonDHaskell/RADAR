// Package config loads RADAR's non-secret settings from a YAML file and
// overlays secrets from environment variables. Secrets are never read from
// the YAML file, so the file itself is safe to commit as a template.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds RADAR's full runtime configuration.
type Config struct {
	Database    DatabaseConfig  `yaml:"-"`
	Embedding   EmbeddingConfig `yaml:"embedding"`
	LLM         LLMConfig       `yaml:"llm"`
	Digest      DigestConfig    `yaml:"digest"`
	Schedule    ScheduleConfig  `yaml:"schedule"`
	ProfilePath string          `yaml:"profile_path"`
}

// DatabaseConfig holds the Postgres connection string. It is sourced only
// from the DATABASE_URL environment variable, never from the YAML file.
type DatabaseConfig struct {
	URL string `yaml:"-"`
}

// EmbeddingConfig configures the embedding provider used to vectorize
// postings and the operator's profile.
type EmbeddingConfig struct {
	Provider  string `yaml:"provider"`
	Model     string `yaml:"model"`
	Dimension int    `yaml:"dimension"`
	APIKey    string `yaml:"-"`
}

// LLMConfig configures the provider used to generate fit verdicts.
type LLMConfig struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	APIKey   string `yaml:"-"`
}

// DigestConfig holds defaults for the `radar digest` command.
type DigestConfig struct {
	Format  string `yaml:"format"`
	Limit   int    `yaml:"limit"`
	OutPath string `yaml:"out_path"`
}

// ScheduleConfig holds cron expressions used by `radar serve`.
type ScheduleConfig struct {
	SyncCron   string `yaml:"sync_cron"`
	DigestCron string `yaml:"digest_cron"`
}

// defaults returns a Config populated with RADAR's built-in defaults, before
// the YAML file or environment overlay are applied.
func defaults() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Embedding: EmbeddingConfig{
			Provider:  "voyage",
			Model:     "voyage-3",
			Dimension: 1024,
		},
		LLM: LLMConfig{
			Provider: "anthropic",
			Model:    "claude-haiku-4-5-20251001",
		},
		Digest: DigestConfig{
			Format: "md",
			Limit:  10,
		},
		Schedule: ScheduleConfig{
			SyncCron:   "0 8 * * 6", // Saturday 08:00
			DigestCron: "0 8 * * 1", // Monday 08:00
		},
		ProfilePath: filepath.Join(home, ".config", "radar", "profile.json"),
	}
}

// DefaultPath returns the default config file location,
// ~/.config/radar/config.yaml.
func DefaultPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "radar", "config.yaml")
}

// Load reads the YAML config at path (if it exists), applies defaults for
// anything unset, and overlays secrets and the database URL from the
// environment. A missing file at path is not an error; RADAR falls back to
// defaults, which is convenient for first-run and for tests.
func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config %s: %w", path, err)
		}
	} else if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}

	cfg.Database.URL = os.Getenv("DATABASE_URL")
	cfg.Embedding.APIKey = os.Getenv("VOYAGE_API_KEY")
	cfg.LLM.APIKey = os.Getenv("ANTHROPIC_API_KEY")

	return &cfg, nil
}

// RequireDatabase returns an error if no database URL is configured. Callers
// that need a database connection should call this before using cfg.Database.URL.
func (c *Config) RequireDatabase() error {
	if c.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL is not set")
	}
	return nil
}
