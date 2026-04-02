package cmd

import (
	"github.com/cordon-co/cordon-cli/cli/cmd/auth"
	"github.com/spf13/cobra"
)

// logoutCmd is a top-level alias for "cordon auth logout".
var logoutCmd = &cobra.Command{
	Use:    "logout",
	Short:  "Clear stored credentials (alias for \"cordon auth logout\")",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return auth.RunLogout(cmd, args)
	},
}
