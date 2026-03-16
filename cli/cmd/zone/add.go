package zone

import (
	"fmt"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/spf13/cobra"
)

var guardian bool

var addCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add a zone",
	Long:  "Protect a file, folder, or glob pattern from agent writes.",
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

func init() {
	addCmd.Flags().BoolVar(&guardian, "guardian", false, "Create a guardian zone (requires guardian/admin role)")
}
