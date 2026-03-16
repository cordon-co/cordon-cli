package pass

import (
	"fmt"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/spf13/cobra"
)

var revokeCmd = &cobra.Command{
	Use:   "revoke <pass-id>",
	Short: "Revoke an active pass",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flags.JSON {
			fmt.Println(`{"error":"not implemented"}`)
			return nil
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "not implemented")
		return nil
	},
}
