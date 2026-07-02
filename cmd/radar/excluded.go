package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var excludedLimit int

var excludedCmd = &cobra.Command{
	Use:   "excluded",
	Short: "List recently excluded postings, for the weekly Stage 0 false-negative review",
	Long: "List recently excluded postings, for the weekly Stage 0 false-negative review.\n\n" +
		"There is no un-exclude command. If a posting was wrongly excluded, correct it by editing profile.json " +
		"(usually preferences.title_exclusions or preferences.locations): that changes the profile hash, which " +
		"re-screens every posting automatically on the next sync.",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		excluded, err := store.ExcludedPostings(ctx, pool, excludedLimit)
		if err != nil {
			return err
		}
		if len(excluded) == 0 {
			fmt.Println("no excluded postings")
			return nil
		}

		for _, e := range excluded {
			fmt.Printf("%s, %s, %s\n", e.CompanyName, e.Title, e.Reason)
		}
		return nil
	},
}

func init() {
	excludedCmd.Flags().IntVar(&excludedLimit, "limit", 20, "number of excluded postings to show")
	rootCmd.AddCommand(excludedCmd)
}
