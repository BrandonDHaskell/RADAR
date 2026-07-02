package main

import "github.com/spf13/cobra"

var (
	applyResumeVariant string
	applyCoverLetter   bool
	applyFollowUp      string
)

var applyCmd = &cobra.Command{
	Use:   "apply <posting_id>",
	Short: "Mark a posting applied",
	Args:  cobra.ExactArgs(1),
	RunE:  notImplemented,
}

func init() {
	applyCmd.Flags().StringVar(&applyResumeVariant, "resume-variant", "", "which role-specific resume/view was used")
	applyCmd.Flags().BoolVar(&applyCoverLetter, "cover-letter", false, "a cover letter was used")
	applyCmd.Flags().StringVar(&applyFollowUp, "follow-up", "", "next follow-up date (YYYY-MM-DD)")
	rootCmd.AddCommand(applyCmd)
}
