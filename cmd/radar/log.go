package main

import (
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var (
	logDirection    string
	logChannel      string
	logSummary      string
	logContact      int64
	logFollowUp     bool
	logFollowUpDate string
)

var logCmd = &cobra.Command{
	Use:   "log <application_id>",
	Short: "Log a correspondence entry against an application",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !slices.Contains(store.CorrespondenceDirections, logDirection) {
			return fmt.Errorf("--direction must be one of %v", store.CorrespondenceDirections)
		}
		if logChannel != "" && !slices.Contains(store.CorrespondenceChannels, logChannel) {
			return fmt.Errorf("--channel must be one of %v", store.CorrespondenceChannels)
		}

		applicationID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid application id %q: %w", args[0], err)
		}

		var contactID *int64
		if logContact != 0 {
			contactID = &logContact
		}

		var followUpDate *time.Time
		if logFollowUpDate != "" {
			t, err := time.Parse("2006-01-02", logFollowUpDate)
			if err != nil {
				return fmt.Errorf("invalid --follow-up-date %q, want YYYY-MM-DD: %w", logFollowUpDate, err)
			}
			followUpDate = &t
		}

		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		c, err := store.LogCorrespondence(ctx, pool, store.NewCorrespondence{
			ApplicationID:  applicationID,
			ContactID:      contactID,
			Direction:      logDirection,
			Channel:        logChannel,
			Summary:        logSummary,
			FollowUpNeeded: logFollowUp || followUpDate != nil,
			FollowUpDate:   followUpDate,
		})
		if err != nil {
			return err
		}

		fmt.Printf("logged correspondence %d against application %d\n", c.ID, c.ApplicationID)
		return nil
	},
}

func init() {
	logCmd.Flags().StringVar(&logDirection, "direction", "", "inbound|outbound (required)")
	logCmd.Flags().StringVar(&logChannel, "channel", "", "email|linkedin|phone|other")
	logCmd.Flags().StringVar(&logSummary, "summary", "", "short summary of the correspondence")
	logCmd.Flags().Int64Var(&logContact, "contact", 0, "contact id")
	logCmd.Flags().BoolVar(&logFollowUp, "follow-up", false, "flag this correspondence as needing follow-up")
	logCmd.Flags().StringVar(&logFollowUpDate, "follow-up-date", "", "follow-up date (YYYY-MM-DD)")
	_ = logCmd.MarkFlagRequired("direction")
	rootCmd.AddCommand(logCmd)
}
