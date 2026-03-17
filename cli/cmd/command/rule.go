package command

import "github.com/spf13/cobra"

// Cmd is the parent command for managing command rules.
var Cmd = &cobra.Command{
	Use:   "command",
	Short: "Manage command rules",
	Long:  "Add, list, and remove command rules that restrict which shell commands agents can run.",
}

func init() {
	Cmd.AddCommand(addCmd, listCmd, removeCmd)
}
