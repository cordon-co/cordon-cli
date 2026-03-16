package pass

import (
	"fmt"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/spf13/cobra"
)

var issueFile string
var issueDuration string

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Issue a temporary access pass",
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

func init() {
	issueCmd.Flags().StringVar(&issueFile, "file", "", "File path to grant access to")
	issueCmd.Flags().StringVar(&issueDuration, "duration", "", "Duration (e.g. 60m, 1w, indefinite)")
}
