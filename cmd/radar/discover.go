package main

import "github.com/spf13/cobra"

var discoverCmd = &cobra.Command{
	Use:   "discover",
	Short: "Run the Built In SF discovery scraper (best-effort), inserting candidate companies",
	RunE:  notImplemented,
}

func init() {
	rootCmd.AddCommand(discoverCmd)
}
