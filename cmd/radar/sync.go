package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/dedup"
	"github.com/BrandonDHaskell/RADAR/internal/ingest"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var (
	syncSource  string
	syncCompany int64
	syncSince   string
)

// syncFetchers maps ats_type to its Fetcher. Only greenhouse is implemented
// so far (Phase 6 adds lever, ashby, workable behind the same interface).
var syncFetchers = map[string]func(*ingest.Client) ingest.Fetcher{
	"greenhouse": func(c *ingest.Client) ingest.Fetcher { return ingest.NewGreenhouseFetcher(c) },
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Fetch, normalize, embed, and score postings for confirmed companies",
	RunE: func(cmd *cobra.Command, args []string) error {
		if syncSource != "" {
			if _, ok := syncFetchers[syncSource]; !ok {
				return fmt.Errorf("--source %q is not implemented yet", syncSource)
			}
		}

		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		companies, err := store.ListCompanies(ctx, pool, store.CompanyStatusConfirmed)
		if err != nil {
			return err
		}

		client := ingest.NewClient(2, 4, 20*time.Second)
		var synced int
		for _, c := range companies {
			if syncCompany != 0 && c.ID != syncCompany {
				continue
			}
			if syncSource != "" && c.ATSType != syncSource {
				continue
			}
			newFetcher, ok := syncFetchers[c.ATSType]
			if !ok {
				continue // no adapter for this company's ATS yet
			}
			fetcher := newFetcher(client)

			postings, err := fetcher.Fetch(ctx, c.ATSToken)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sync %s: %v\n", c.Name, err)
				continue
			}

			result, err := dedup.Sync(ctx, pool, c.ID, fetcher.Source(), c.Name, postings)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sync %s: %v\n", c.Name, err)
				continue
			}
			synced++
			fmt.Printf("%s: %d new, %d updated, %d unchanged, %d closed\n",
				c.Name, result.Inserted, result.Updated, result.Unchanged, result.Closed)
		}

		if synced == 0 {
			fmt.Println("no confirmed companies matched (check ATS type, --source, or --company)")
		}
		return nil
	},
}

func init() {
	syncCmd.Flags().StringVar(&syncSource, "source", "", "limit sync to one ATS (greenhouse|lever|ashby|workable)")
	syncCmd.Flags().Int64Var(&syncCompany, "company", 0, "limit sync to one company id")
	syncCmd.Flags().StringVar(&syncSince, "since", "", "only re-check companies last synced before this duration ago (not yet implemented)")
	rootCmd.AddCommand(syncCmd)
}
