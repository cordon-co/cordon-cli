package cmd

import (
	"fmt"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Uninstall Cordon from the current repository",
	Long: `Removes all Cordon configuration from the current repository:

  - Removes the Cordon hook entry from .claude/settings.local.json
    (leaves all other hook entries intact)
  - Removes the model_instructions_file reference from .codex/config.toml
    (leaves all other Codex config intact)
  - Removes the MCP server entry from .claude/settings.local.json
  - Removes the .cordon/ directory

User-level data (~/.cordon/repos/<repo-hash>/) is not removed.`,
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
