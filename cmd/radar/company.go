package main

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var companyCmd = &cobra.Command{
	Use:   "company",
	Short: "Manage the seed list of companies",
}

var (
	companyAddName    string
	companyAddATS     string
	companyAddToken   string
	companyAddWebsite string
	companyAddNotes   string
)

var companyAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a company to the seed list",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !slices.Contains(store.ValidATSTypes, companyAddATS) {
			return fmt.Errorf("--ats must be one of %v", store.ValidATSTypes)
		}

		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		c, err := store.CreateCompany(ctx, pool, store.NewCompany{
			Name:       companyAddName,
			WebsiteURL: companyAddWebsite,
			ATSType:    companyAddATS,
			ATSToken:   companyAddToken,
			Notes:      companyAddNotes,
		})
		if err != nil {
			return err
		}

		fmt.Printf("added company %d: %s (status=%s)\n", c.ID, c.Name, c.Status)
		return nil
	},
}

var companyListStatus string

var companyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List companies",
	RunE: func(cmd *cobra.Command, args []string) error {
		if companyListStatus != "" && !slices.Contains(
			[]string{store.CompanyStatusCandidate, store.CompanyStatusConfirmed, store.CompanyStatusArchived},
			companyListStatus,
		) {
			return fmt.Errorf("--status must be one of candidate|confirmed|archived")
		}

		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		companies, err := store.ListCompanies(ctx, pool, companyListStatus)
		if err != nil {
			return err
		}
		if len(companies) == 0 {
			fmt.Println("no companies found")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tSTATUS\tATS\tTOKEN")
		for _, c := range companies {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", c.ID, c.Name, c.Status, c.ATSType, c.ATSToken)
		}
		return w.Flush()
	},
}

var companyConfirmCmd = &cobra.Command{
	Use:   "confirm <id>",
	Short: "Confirm a candidate company for active syncing",
	Args:  cobra.ExactArgs(1),
	RunE:  setCompanyStatusRunE(store.CompanyStatusConfirmed),
}

var companyArchiveCmd = &cobra.Command{
	Use:   "archive <id>",
	Short: "Archive a company",
	Args:  cobra.ExactArgs(1),
	RunE:  setCompanyStatusRunE(store.CompanyStatusArchived),
}

// setCompanyStatusRunE builds a RunE that parses the positional company id
// and transitions it to status, shared by confirm and archive.
func setCompanyStatusRunE(status string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid company id %q: %w", args[0], err)
		}

		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		c, err := store.SetCompanyStatus(ctx, pool, id, status)
		if err != nil {
			return err
		}

		fmt.Printf("company %d: %s -> %s\n", c.ID, c.Name, c.Status)
		return nil
	}
}

func init() {
	companyAddCmd.Flags().StringVar(&companyAddName, "name", "", "company name (required)")
	companyAddCmd.Flags().StringVar(&companyAddATS, "ats", "none", "ATS type: greenhouse|lever|ashby|workable|dayforce|none")
	companyAddCmd.Flags().StringVar(&companyAddToken, "token", "", "ATS board token / site / subdomain")
	companyAddCmd.Flags().StringVar(&companyAddWebsite, "website", "", "company website URL")
	companyAddCmd.Flags().StringVar(&companyAddNotes, "notes", "", "free-form notes")
	_ = companyAddCmd.MarkFlagRequired("name")

	companyListCmd.Flags().StringVar(&companyListStatus, "status", "", "filter by status: candidate|confirmed|archived")

	companyCmd.AddCommand(companyAddCmd, companyListCmd, companyConfirmCmd, companyArchiveCmd)
	rootCmd.AddCommand(companyCmd)
}
