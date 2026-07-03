package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var (
	applyResumeVariant string
	applyCoverLetter   bool
	applyFollowUp      string
)

var applyCmd = &cobra.Command{
	Use:   "apply <posting_id>",
	Short: "Mark a posting applied",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		postingID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid posting id %q: %w", args[0], err)
		}

		var followUpDate *time.Time
		if applyFollowUp != "" {
			t, err := time.Parse("2006-01-02", applyFollowUp)
			if err != nil {
				return fmt.Errorf("invalid --follow-up date %q, want YYYY-MM-DD: %w", applyFollowUp, err)
			}
			followUpDate = &t
		}

		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		a, err := store.ApplyToPosting(ctx, pool, postingID, applyResumeVariant, applyCoverLetter, followUpDate)
		if err != nil {
			return err
		}

		fmt.Printf("application %d: posting %d -> %s\n", a.ID, a.PostingID, a.Status)
		return nil
	},
}

func init() {
	applyCmd.Flags().StringVar(&applyResumeVariant, "resume-variant", "", "which role-specific resume/view was used")
	applyCmd.Flags().BoolVar(&applyCoverLetter, "cover-letter", false, "a cover letter was used")
	applyCmd.Flags().StringVar(&applyFollowUp, "follow-up", "", "next follow-up date (YYYY-MM-DD)")
	rootCmd.AddCommand(applyCmd)
}
