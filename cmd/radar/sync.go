package main

import "github.com/spf13/cobra"

var (
	syncSource  string
	syncCompany int64
	syncSince   string
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch, normalize, embed, and score postings for confirmed companies",
	RunE:  notImplemented,
}

func init() {
	syncCmd.Flags().StringVar(&syncSource, "source", "", "limit sync to one ATS (greenhouse|lever|ashby|workable)")
	syncCmd.Flags().Int64Var(&syncCompany, "company", 0, "limit sync to one company id")
	syncCmd.Flags().StringVar(&syncSince, "since", "", "only re-check companies last synced before this duration ago")
	rootCmd.AddCommand(syncCmd)
}
