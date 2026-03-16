package cmd

import (
	"fmt"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise Cordon in the current repository",
	Long: `Creates .cordon/ with policy.db, writes PreToolUse hook entries to
.claude/settings.local.json, configures Codex model instructions, and adds the
MCP server entry.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if flags.JSON {
			fmt.Println(`{"error":"not implemented"}`)
			return nil
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "not implemented")
		return nil
	},
}
