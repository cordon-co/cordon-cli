// Package file implements the "cordon file" subcommand group.
package file

import "github.com/spf13/cobra"

// Cmd is the parent "file" command. Registered in cmd/root.go.
var Cmd = &cobra.Command{
	Use:   "file",
	Short: "Manage protected file rules",
	Long:  "Add, list, and remove file rules that restrict agent write access.",
}

func init() {
	Cmd.AddCommand(addCmd, listCmd, removeCmd)
}
