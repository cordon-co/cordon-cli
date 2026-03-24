// Package auth implements the "cordon auth" subcommand group.
package auth

import "github.com/spf13/cobra"

// Cmd is the parent "auth" command. Registered in cmd/root.go.
var Cmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage authentication",
	Long:  "Log in, log out, and check authentication status with Cordon Cloud.",
}

func init() {
	Cmd.AddCommand(loginCmd, logoutCmd, statusCmd)
}
