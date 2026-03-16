package zone

import (
	"fmt"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active zones",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flags.JSON {
			fmt.Println(`{"zones":[]}`)
			return nil
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "not implemented")
		return nil
	},
}
