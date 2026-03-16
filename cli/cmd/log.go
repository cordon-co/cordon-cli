package cmd

import (
	"fmt"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/spf13/cobra"
)

var logFile string
var logDeniedOnly bool
var logSince string
var logExport string

// logCmd — named logCmd to avoid shadowing the standard library "log" package.
var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Display the audit log",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flags.JSON {
			fmt.Println(`{"entries":[]}`)
			return nil
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "not implemented")
		return nil
	},
}

func init() {
	logCmd.Flags().StringVar(&logFile, "file", "", "Filter by file path")
	logCmd.Flags().BoolVar(&logDeniedOnly, "denied-only", false, "Show only denied operations")
	logCmd.Flags().StringVar(&logSince, "since", "", "Filter by time (e.g. 24h, 7d)")
	logCmd.Flags().StringVar(&logExport, "export", "", "Export format (csv)")
}
