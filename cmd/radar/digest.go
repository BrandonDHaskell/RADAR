package main

import "github.com/spf13/cobra"

var (
	digestFormat     string
	digestLimit      int
	digestMinVerdict string
	digestOut        string
)

var digestCmd = &cobra.Command{
	Use:   "digest",
	Short: "Generate the digest of top open, un-applied postings ranked by fit",
	RunE:  notImplemented,
}

func init() {
	digestCmd.Flags().StringVar(&digestFormat, "format", "md", "output format: md|html")
	digestCmd.Flags().IntVar(&digestLimit, "limit", 10, "number of postings to include")
	digestCmd.Flags().StringVar(&digestMinVerdict, "min-verdict", "", "minimum verdict to include: pursue|stretch|skip")
	digestCmd.Flags().StringVar(&digestOut, "out", "", "output file path (default: configured digest.out_path)")
	rootCmd.AddCommand(digestCmd)
}
