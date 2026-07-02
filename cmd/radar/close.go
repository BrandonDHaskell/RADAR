package main

import "github.com/spf13/cobra"

var closeStatus string

var closeCmd = &cobra.Command{
	Use:   "close <application_id>",
	Short: "Close out an application",
	Args:  cobra.ExactArgs(1),
	RunE:  notImplemented,
}

func init() {
	closeCmd.Flags().StringVar(&closeStatus, "status", "", "closed_offer|closed_rejected|withdrawn (required)")
	_ = closeCmd.MarkFlagRequired("status")
	rootCmd.AddCommand(closeCmd)
}
