package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
		embedder := embed.NewVoyageProvider(cfg.Embedding.APIKey, cfg.Embedding.Model, cfg.Embedding.Dimension)
		llmProvider := llm.NewAnthropicProvider(cfg.LLM.APIKey, cfg.LLM.Model)

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

			embedded, err := embedPostings(ctx, pool, embedder, cfg.Embedding.Model, result.ToEmbed)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sync %s: embedding: %v\n", c.Name, err)
			}

			// Catch postings still missing an embedding: either an embed
			// call above just failed, or a previous run's did. dedup only
			// re-queues content that changed, so this backfill is what
			// makes an embedding failure self-healing on the next sync.
			missingEmbeddings, err := store.PostingsMissingEmbedding(ctx, pool, c.ID, fetcher.Source())
			if err != nil {
				fmt.Fprintf(os.Stderr, "sync %s: checking for missing embeddings: %v\n", c.Name, err)
			} else if len(missingEmbeddings) > 0 {
				backfilled, err := embedPostings(ctx, pool, embedder, cfg.Embedding.Model, missingToCandidates(missingEmbeddings))
				if err != nil {
					fmt.Fprintf(os.Stderr, "sync %s: backfilling embeddings: %v\n", c.Name, err)
				}
				embedded += backfilled
			}

			scoreIDs := changedPostingIDs(result.ToEmbed)
			missingScores, err := store.PostingsMissingFitScore(ctx, pool, c.ID, fetcher.Source())
			if err != nil {
				fmt.Fprintf(os.Stderr, "sync %s: checking for missing fit scores: %v\n", c.Name, err)
			} else {
				scoreIDs = dedupeIDs(scoreIDs, missingScores)
			}

			var scoreResult match.ScoreResult
			if len(scoreIDs) > 0 {
				scoreResult, err = match.ScorePostings(ctx, pool, embedder, llmProvider, cfg.LLM.Model, profile, scoreIDs)
				if err != nil {
					fmt.Fprintf(os.Stderr, "sync %s: scoring: %v\n", c.Name, err)
				}
			}

			fmt.Printf("%s: %d new, %d updated, %d unchanged, %d closed, %d embedded, %d scored, %d semantic-only\n",
				c.Name, result.Inserted, result.Updated, result.Unchanged, result.Closed, embedded,
				scoreResult.Scored, scoreResult.SemanticOnly)
		}

		if synced == 0 {
			fmt.Println("no confirmed companies matched (check ATS type, --source, or --company)")
		}
		return nil
	},
}

// embedPostings embeds each candidate's text and stores the resulting
// vector, returning how many were successfully embedded.
func embedPostings(ctx context.Context, pool *pgxpool.Pool, provider embed.Provider, model string, candidates []dedup.EmbedCandidate) (int, error) {
	if len(candidates) == 0 {
		return 0, nil
	}

	texts := make([]string, len(candidates))
	for i, c := range candidates {
		texts[i] = c.Text
	}

	vectors, err := provider.Embed(ctx, texts, embed.InputTypeDocument)
	if err != nil {
		return 0, err
	}

	var embedded int
	for i, c := range candidates {
		if err := store.UpsertPostingEmbedding(ctx, pool, c.PostingID, vectors[i], model); err != nil {
			return embedded, err
		}
		embedded++
	}
	return embedded, nil
}

// missingToCandidates builds embed candidates for postings found by
// store.PostingsMissingEmbedding, using the same text format as dedup.Sync's
// own change-driven candidates.
func missingToCandidates(missing []store.PostingToEmbed) []dedup.EmbedCandidate {
	candidates := make([]dedup.EmbedCandidate, len(missing))
	for i, p := range missing {
		candidates[i] = dedup.EmbedCandidate{
			PostingID: p.PostingID,
			Text:      dedup.FormatEmbeddingText(p.CompanyName, p.Title, p.Department, p.Location, p.Description),
		}
	}
	return candidates
}

// changedPostingIDs extracts posting IDs from dedup's change-driven embed
// candidates: a posting whose content changed needs re-scoring too, since a
// new description or title can change its fit.
func changedPostingIDs(candidates []dedup.EmbedCandidate) []int64 {
	ids := make([]int64, len(candidates))
	for i, c := range candidates {
		ids[i] = c.PostingID
	}
	return ids
}

// dedupeIDs merges a and b into a single slice with no duplicate posting IDs.
func dedupeIDs(a, b []int64) []int64 {
	seen := make(map[int64]bool, len(a)+len(b))
	result := make([]int64, 0, len(a)+len(b))
	for _, ids := range [][]int64{a, b} {
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				result = append(result, id)
			}
		}
	}
	return result
}

func init() {
	syncCmd.Flags().StringVar(&syncSource, "source", "", "limit sync to one ATS (greenhouse|lever|ashby|workable)")
	syncCmd.Flags().Int64Var(&syncCompany, "company", 0, "limit sync to one company id")
	syncCmd.Flags().StringVar(&syncSince, "since", "", "only re-check companies last synced before this duration ago (not yet implemented)")
	syncCmd.Flags().BoolVar(&syncAllowEmpty, "allow-empty", false, "allow a 0-posting fetch to close all open postings for that company/source")
	rootCmd.AddCommand(syncCmd)
}
