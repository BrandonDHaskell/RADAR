package main

import "github.com/spf13/cobra"

var companyCmd = &cobra.Command{
	Use:   "company",
	Short: "Manage the seed list of companies",
}

var (
	companyAddName  string
	companyAddATS   string
	companyAddToken string
)

var companyAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a company to the seed list",
	RunE:  notImplemented,
}

var companyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List companies",
	RunE:  notImplemented,
}

var companyConfirmCmd = &cobra.Command{
	Use:   "confirm <id>",
	Short: "Confirm a candidate company for active syncing",
	Args:  cobra.ExactArgs(1),
	RunE:  notImplemented,
}

var companyArchiveCmd = &cobra.Command{
	Use:   "archive <id>",
	Short: "Archive a company",
	Args:  cobra.ExactArgs(1),
	RunE:  notImplemented,
}

func init() {
	companyAddCmd.Flags().StringVar(&companyAddName, "name", "", "company name (required)")
	companyAddCmd.Flags().StringVar(&companyAddATS, "ats", "none", "ATS type: greenhouse|lever|ashby|workable|none")
	companyAddCmd.Flags().StringVar(&companyAddToken, "token", "", "ATS board token / site / subdomain")
	_ = companyAddCmd.MarkFlagRequired("name")

	companyCmd.AddCommand(companyAddCmd, companyListCmd, companyConfirmCmd, companyArchiveCmd)
	rootCmd.AddCommand(companyCmd)
}
