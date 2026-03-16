// Package zone implements the "cordon zone" subcommand group.
package zone

import "github.com/spf13/cobra"

// Cmd is the parent "zone" command. Registered in cmd/root.go.
var Cmd = &cobra.Command{
	Use:   "zone",
	Short: "Manage protected zones",
	Long:  "Add, list, and remove file zones that restrict agent write access.",
}

func init() {
	Cmd.AddCommand(addCmd, listCmd, removeCmd)
}
