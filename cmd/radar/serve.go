package main

import "github.com/spf13/cobra"

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Run RADAR as a long-lived background service with the internal scheduler",
	RunE:  notImplemented,
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
