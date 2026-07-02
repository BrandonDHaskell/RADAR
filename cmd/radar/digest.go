package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/digest"
	"github.com/BrandonDHaskell/RADAR/internal/match"
	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var (
	digestFormat     string
	digestLimit      int
	digestMinVerdict string
	digestOut        string
)

var digestCmd = &cobra.Command{
	Use:   "digest",
	Short: "Generate the digest of top open, un-applied postings ranked by fit",
	Long: "Generate the digest of top open, un-applied postings ranked by fit.\n\n" +
		"Requires a valid profile.json: the digest computes the profile's current hash to decide whether a stored " +
		"verdict is still fresh (see profile_path in config.yaml). A missing or invalid profile is an error.",
	RunE: func(cmd *cobra.Command, args []string) error {
		format := digest.Format(cfg.Digest.Format)
		if cmd.Flags().Changed("format") {
			format = digest.Format(digestFormat)
		}
		if format != digest.FormatMarkdown && format != digest.FormatHTML {
			return fmt.Errorf("--format must be %q or %q", digest.FormatMarkdown, digest.FormatHTML)
		}

		limit := cfg.Digest.Limit
		if cmd.Flags().Changed("limit") {
			limit = digestLimit
		}

		minVerdict := digestMinVerdict
		switch minVerdict {
		case "", "pursue", "stretch", "skip":
		default:
			return fmt.Errorf("--min-verdict must be pursue, stretch, or skip")
		}

		outPath := cfg.Digest.OutPath
		if cmd.Flags().Changed("out") {
			outPath = digestOut
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

		postings, err := store.DigestPostings(ctx, pool, profile.Hash, minVerdict, limit)
		if err != nil {
			return err
		}

		if outPath == "" {
			return digest.Render(os.Stdout, format, postings)
		}

		f, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("creating digest output file: %w", err)
		}
		defer f.Close()

		if err := digest.Render(f, format, postings); err != nil {
			return err
		}
		fmt.Printf("wrote digest to %s (%d postings)\n", outPath, len(postings))
		return nil
	},
}

func init() {
	digestCmd.Flags().StringVar(&digestFormat, "format", "md", "output format: md|html")
	digestCmd.Flags().IntVar(&digestLimit, "limit", 10, "number of postings to include")
	digestCmd.Flags().StringVar(&digestMinVerdict, "min-verdict", "", "minimum verdict to include: pursue|stretch|skip")
	digestCmd.Flags().StringVar(&digestOut, "out", "", "output file path (default: configured digest.out_path, or stdout)")
	rootCmd.AddCommand(digestCmd)
}
