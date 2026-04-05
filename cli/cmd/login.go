package cmd

import (
	"github.com/cordon-co/cordon-cli/cli/cmd/auth"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/spf13/cobra"
)

// loginCmd is a top-level alias for "cordon auth login".
var loginCmd = &cobra.Command{
	Use:    "login",
	Short:  "Authenticate via GitHub OAuth (alias for \"cordon auth login\")",
	Hidden: true,
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return auth.RunLogin(cmd, args)
	},
}

func init() {
	loginCmd.Flags().StringVar(&flags.Token, "token", "", "Machine token for non-interactive authentication")
}
