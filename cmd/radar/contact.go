package main

import (
	"fmt"
	"os"
	"slices"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/BrandonDHaskell/RADAR/internal/store"
)

var contactCmd = &cobra.Command{
	Use:   "contact",
	Short: "Manage contacts (recruiters, hiring managers, team leads)",
}

var (
	contactAddCompany int64
	contactAddName    string
	contactAddType    string
	contactAddEmail   string
)

var contactAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a contact",
	RunE: func(cmd *cobra.Command, args []string) error {
		if contactAddType != "" && !slices.Contains(store.ContactTypes, contactAddType) {
			return fmt.Errorf("--type must be one of %v", store.ContactTypes)
		}

		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		c, err := store.CreateContact(ctx, pool, store.NewContact{
			CompanyID: contactAddCompany,
			Name:      contactAddName,
			Type:      contactAddType,
			Email:     contactAddEmail,
		})
		if err != nil {
			return err
		}

		fmt.Printf("added contact %d: %s (company %d)\n", c.ID, c.Name, c.CompanyID)
		return nil
	},
}

var contactListCmd = &cobra.Command{
	Use:   "list",
	Short: "List contacts",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		pool, err := openDB(ctx)
		if err != nil {
			return err
		}
		defer pool.Close()

		contacts, err := store.ListContacts(ctx, pool)
		if err != nil {
			return err
		}
		if len(contacts) == 0 {
			fmt.Println("no contacts found")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tNAME\tCOMPANY\tTYPE\tEMAIL")
		for _, c := range contacts {
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", c.ID, c.Name, c.CompanyName, c.Type, c.Email)
		}
		return w.Flush()
	},
}

func init() {
	contactAddCmd.Flags().Int64Var(&contactAddCompany, "company", 0, "company id (required)")
	contactAddCmd.Flags().StringVar(&contactAddName, "name", "", "contact name (required)")
	contactAddCmd.Flags().StringVar(&contactAddType, "type", "", "recruiter|hiring_manager|team_lead|referral|other")
	contactAddCmd.Flags().StringVar(&contactAddEmail, "email", "", "contact email")
	_ = contactAddCmd.MarkFlagRequired("company")
	_ = contactAddCmd.MarkFlagRequired("name")

	contactCmd.AddCommand(contactAddCmd, contactListCmd)
	rootCmd.AddCommand(contactCmd)
}
