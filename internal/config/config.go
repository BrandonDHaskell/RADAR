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
			Model:     "voyage-4",
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

// Load reads the YAML config at path, applies defaults for anything unset,
// and overlays secrets and the database URL from the environment. If
// mustExist is false, a missing file at path is not an error and RADAR
// falls back to defaults, which is convenient for the default path and for
// tests. If mustExist is true (an operator explicitly passed --config), a
// missing file is an error naming the path, since a silent fallback there
// would hide a typo.
func Load(path string, mustExist bool) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config %s: %w", path, err)
		}
		if mustExist {
			return nil, fmt.Errorf("config file %s does not exist", path)
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

// RequireEmbedding returns an error if no Voyage API key is configured.
// Callers that need to call the embedding provider should call this before
// using cfg.Embedding.APIKey.
func (c *Config) RequireEmbedding() error {
	if c.Embedding.APIKey == "" {
		return fmt.Errorf("VOYAGE_API_KEY is not set")
	}
	return nil
}
