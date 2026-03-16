package cmd

import (
	"errors"
	"os"

	"github.com/cordon-co/cordon/internal/hook"
	"github.com/spf13/cobra"
)

// hookCmd is invoked as a PreToolUse hook by Claude Code and VS Code agents.
// It reads a JSON payload from stdin, checks the file path against policy,
// and exits 0 (allow) or 2 with a JSON deny response.
//
// The --json flag is not meaningful here: output format is always JSON because
// this command is consumed by the agent platform, not a human.
//
// Exit codes:
//   0 — allow (or non-writing tool, passed through silently)
//   1 — malformed payload or IO error (cobra handles this via returned error)
//   2 — deny (os.Exit called directly to bypass cobra's exit-1 handling)
var hookCmd = &cobra.Command{
	Use:    "hook",
	Short:  "Evaluate a PreToolUse hook payload (reads JSON from stdin)",
	Hidden: true, // not shown in help; invoked only by agent hook config
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := hook.Evaluate(os.Stdin, os.Stdout)
		if errors.Is(err, hook.ErrDenied) {
			os.Exit(2)
		}
		return err // nil → exit 0; other errors → cobra prints and exits 1
	},
}
