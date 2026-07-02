package main

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/dedup"
	"github.com/BrandonDHaskell/RADAR/internal/embed"
	"github.com/BrandonDHaskell/RADAR/internal/ingest"
	"github.com/BrandonDHaskell/RADAR/internal/llm"
	"github.com/BrandonDHaskell/RADAR/internal/match"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var (
	syncSource     string
	syncCompany    int64
	syncSince      string
	syncAllowEmpty bool
)

// syncFetchers maps ats_type to its Fetcher.
var syncFetchers = map[string]func(*ingest.Client) ingest.Fetcher{
	"greenhouse": func(c *ingest.Client) ingest.Fetcher { return ingest.NewGreenhouseFetcher(c) },
	"lever":      func(c *ingest.Client) ingest.Fetcher { return ingest.NewLeverFetcher(c) },
	"ashby":      func(c *ingest.Client) ingest.Fetcher { return ingest.NewAshbyFetcher(c) },
	"workable":   func(c *ingest.Client) ingest.Fetcher { return ingest.NewWorkableFetcher(c) },
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
		if err := cfg.RequireEmbedding(); err != nil {
			return err
		}
		if err := cfg.RequireLLM(); err != nil {
			return err
		}

		profile, err := match.LoadProfile(cfg.ProfilePath)
		if err != nil {
			return fmt.Errorf("loading profile: %w", err)
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

		// Stage 1: fetch and reconcile each confirmed board. --source and
		// --company scope this loop only; the evaluation pipeline below
		// always runs corpus-wide (see RunPipeline's doc comment).
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
			if c.ATSToken == "" {
				fmt.Fprintf(os.Stderr, "warning: %s has ats_type %s but no token, skipping\n", c.Name, c.ATSType)
				continue
			}
			fetcher := newFetcher(client)

			postings, err := fetcher.Fetch(ctx, c.ATSToken)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sync %s: %v\n", c.Name, err)
				continue
			}

			result, err := dedup.Sync(ctx, pool, c.ID, fetcher.Source(), c.Name, postings, syncAllowEmpty)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sync %s: %v\n", c.Name, err)
				continue
			}
			synced++
			if result.ExpirySkipped {
				fmt.Fprintf(os.Stderr,
					"warning: %s returned 0 postings but %d are open; skipped closing them (re-run with --allow-empty if the board is truly empty)\n",
					c.Name, result.OpenAtSkip)
				continue
			}
			fmt.Printf("%s: %d new, %d updated, %d unchanged, %d closed\n",
				c.Name, result.Inserted, result.Updated, result.Unchanged, result.Closed)
		}

		if synced == 0 {
			fmt.Println("no confirmed companies matched (check ATS type, --source, or --company)")
		}

		// Stage 2: run the evaluation pipeline once, corpus-wide: screen,
		// embed, recompute semantic scores, then request verdicts for the
		// top-K shortlist. This runs even when the fetch loop above was
		// filtered to one company, since a profile edit or a newly-passed
		// posting from an earlier sync can still be due for evaluation.
		embedder := embed.NewVoyageProvider(cfg.Embedding.APIKey, cfg.Embedding.Model, cfg.Embedding.Dimension)
		llmProvider := llm.NewAnthropicProvider(cfg.LLM.APIKey, cfg.LLM.Model)

		pipelineResult, err := match.RunPipeline(ctx, pool, embedder, cfg.Embedding.Model, llmProvider, cfg.LLM.Model,
			profile, cfg.Match.LLMTopK, cfg.Match.MinSemanticScore)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sync: evaluation pipeline: %v\n", err)
		}

		fmt.Printf("screened %d (excluded %d), embedded %d, semantic %d, verdicts %d requested / %d written / %d failed\n",
			pipelineResult.Screened, pipelineResult.Excluded, pipelineResult.Embedded, pipelineResult.SemanticScored,
			pipelineResult.VerdictsRequested, pipelineResult.VerdictsWritten, pipelineResult.VerdictsFailed)

		return nil
	},
}

func init() {
	syncCmd.Flags().StringVar(&syncSource, "source", "", "limit sync to one ATS (greenhouse|lever|ashby|workable)")
	syncCmd.Flags().Int64Var(&syncCompany, "company", 0, "limit sync to one company id")
	syncCmd.Flags().StringVar(&syncSince, "since", "", "only re-check companies last synced before this duration ago (not yet implemented)")
	syncCmd.Flags().BoolVar(&syncAllowEmpty, "allow-empty", false, "allow a 0-posting fetch to close all open postings for that company/source")
	rootCmd.AddCommand(syncCmd)
}
