package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var followupsStale int

var followupsCmd = &cobra.Command{
	Use:   "followups",
	Short: "List applications and correspondence due for follow-up today",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		followUps, err := store.ListFollowUps(ctx, pool, followupsStale)
		if err != nil {
			return err
		}
		if len(followUps) == 0 {
			fmt.Println("no follow-ups due")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "APPLICATION\tCOMPANY\tTITLE\tSTATUS\tREASON")
		for _, f := range followUps {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", f.ApplicationID, f.CompanyName, f.PostingTitle, f.Status, f.Reason)
		}
		return w.Flush()
	},
}

func init() {
	followupsCmd.Flags().IntVar(&followupsStale, "stale", 0, "also surface applications with no activity in this many days")
	rootCmd.AddCommand(followupsCmd)
}
