// Package pass implements the "cordon pass" subcommand group.
package pass

import "github.com/spf13/cobra"

// Cmd is the parent "pass" command. Registered in cmd/root.go.
var Cmd = &cobra.Command{
	Use:   "pass",
	Short: "Manage temporary access passes",
	Long:  "Issue, list, and revoke passes that grant agents temporary write access to protected files and commands.",
}

func init() {
	Cmd.AddCommand(issueCmd, listCmd, revokeCmd)
}
