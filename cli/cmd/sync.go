package cmd

import (
	"fmt"

	"github.com/cordon-co/cordon-cli/internal/flags"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync policy and audit data with Cordon Cloud",
	Long:  "Pulls policy from api.cordon.sh and pushes local audit data. Cloud wins on conflict.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flags.JSON {
			fmt.Println(`{"error":"not implemented"}`)
			return nil
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "not implemented")
		return nil
	},
}
