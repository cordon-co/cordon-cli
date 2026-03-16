package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// hookCmd is invoked as a PreToolUse hook by Claude Code and VS Code agents.
// It reads a JSON payload from stdin, checks the file path against policy,
// and exits 0 (allow) or 2 with a JSON deny response.
//
// The --json flag is not used here: output format is always JSON because
// this command is consumed by the agent platform, not a human.
var hookCmd = &cobra.Command{
	Use:    "hook",
	Short:  "Evaluate a PreToolUse hook payload (reads JSON from stdin)",
	Hidden: true, // not shown in help; invoked only by agent hook config
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Stub: allow all writes. Real implementation will:
		//   1. Read JSON payload from os.Stdin
		//   2. Extract tool name and file path
		//   3. Check path against policy database
		//   4. Exit 0 (allow) or write deny JSON and exit 2
		fmt.Fprintln(os.Stderr, "cordon hook: not implemented (fail-open)")
		return nil
	},
}
