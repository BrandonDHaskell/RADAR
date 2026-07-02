package main

import "github.com/spf13/cobra"

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
	RunE:  notImplemented,
}

var contactListCmd = &cobra.Command{
	Use:   "list",
	Short: "List contacts",
	RunE:  notImplemented,
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
