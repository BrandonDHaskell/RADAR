package main

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var closeStatus string

var closeCmd = &cobra.Command{
	Use:   "close <application_id>",
	Short: "Close out an application",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !slices.Contains(store.ApplicationCloseStatuses, closeStatus) {
			return fmt.Errorf("--status must be one of %v", store.ApplicationCloseStatuses)
		}

		applicationID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid application id %q: %w", args[0], err)
		}

		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		a, err := store.CloseApplication(ctx, pool, applicationID, closeStatus)
		if err != nil {
			return err
		}

		fmt.Printf("application %d -> %s\n", a.ID, a.Status)
		return nil
	},
}

func init() {
	closeCmd.Flags().StringVar(&closeStatus, "status", "", "closed_offer|closed_rejected|withdrawn (required)")
	_ = closeCmd.MarkFlagRequired("status")
	rootCmd.AddCommand(closeCmd)
}
