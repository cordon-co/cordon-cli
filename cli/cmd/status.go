package cmd

import (
	"fmt"

	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show auth state, policy summary, and integrity check",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flags.JSON {
			fmt.Println(`{"authenticated":false,"policy":null,"integrity":null}`)
			return nil
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "not implemented")
		return nil
	},
}
