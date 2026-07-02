package main

import "github.com/spf13/cobra"

var followupsStale int

var followupsCmd = &cobra.Command{
	Use:   "followups",
	Short: "List applications and correspondence due for follow-up today",
	RunE:  notImplemented,
}

func init() {
	followupsCmd.Flags().IntVar(&followupsStale, "stale", 0, "also surface applications with no activity in this many days")
	rootCmd.AddCommand(followupsCmd)
}
