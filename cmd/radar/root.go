package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/config"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var (
	cfgPath string
	cfg     *config.Config
)

var rootCmd = &cobra.Command{
	Use:   "radar",
	Short: "RADAR aggregates, ranks, and tracks job postings against your profile.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		mustExist := cmd.Flags().Changed("config")
		loaded, err := config.Load(cfgPath, mustExist)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		cfg = loaded
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", config.DefaultPath(), "path to config.yaml")
}

// notImplemented is the RunE for commands whose build phase has not landed
// yet. It keeps the full CLI surface from Section 8 wired and discoverable
// via --help while each command is implemented phase by phase.
func notImplemented(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("%s: not yet implemented", cmd.CommandPath())
}

// openDB requires a configured DATABASE_URL, applies pending migrations, and
// returns a ready connection pool. Callers are responsible for closing it.
func openDB(ctx context.Context) (*pgxpool.Pool, error) {
	if err := cfg.RequireDatabase(); err != nil {
		return nil, err
	}
	return store.Open(ctx, cfg.Database.URL)
}
