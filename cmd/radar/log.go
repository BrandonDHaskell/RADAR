package main

import "github.com/spf13/cobra"

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
	RunE:  notImplemented,
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
