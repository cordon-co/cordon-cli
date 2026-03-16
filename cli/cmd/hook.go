package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/cordon-co/cordon/internal/hook"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
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
//
//	0 — allow
//	1 — malformed payload or IO error (cobra handles this via returned error)
//	2 — deny (os.Exit called directly to bypass cobra's exit-1 handling)
var hookCmd = &cobra.Command{
	Use:    "hook",
	Short:  "Evaluate a PreToolUse hook payload (reads JSON from stdin)",
	Hidden: true, // not shown in help; invoked only by agent hook config
	Args:   cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		event, err := hook.Evaluate(os.Stdin, os.Stdout)

		// Log every invocation. Logging failures are non-fatal (fail-open).
		if event != nil {
			logHookEvent(event)
		}

		if errors.Is(err, hook.ErrDenied) {
			os.Exit(2)
		}
		return err // nil → exit 0; other errors → cobra prints and exits 1
	},
}

// logHookEvent writes a hook invocation to the audit log.
// Any failure is printed to stderr and does not affect the hook decision.
func logHookEvent(event *hook.Event) {
	absRoot, err := resolveRepoRoot(event.Cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cordon: audit log: %v\n", err)
		return
	}

	db, err := store.OpenDataDB(absRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cordon: audit log: open database: %v\n", err)
		return
	}
	defer db.Close()

	if err := store.MigrateDataDB(db); err != nil {
		fmt.Fprintf(os.Stderr, "cordon: audit log: migrate schema: %v\n", err)
		return
	}

	entry := store.HookLogEntry{
		Ts:        time.Now().UnixMicro(),
		ToolName:  event.ToolName,
		FilePath:  event.FilePath,
		ToolInput: string(event.ToolInput),
		Decision:  string(event.Decision),
		OSUser:    store.CurrentOSUser(),
		Agent:     "", // TODO: detect agent platform from environment
	}

	if err := store.InsertHookLog(db, entry); err != nil {
		fmt.Fprintf(os.Stderr, "cordon: audit log: insert: %v\n", err)
	}
}

// resolveRepoRoot returns the absolute repo root to use for locating the data
// database. It prefers the cwd from the hook payload (which is the agent's
// working directory and reliably points to the repo root), falling back to
// walking up from the process working directory.
func resolveRepoRoot(payloadCwd string) (string, error) {
	if payloadCwd != "" {
		abs, err := filepath.Abs(payloadCwd)
		if err == nil {
			return abs, nil
		}
	}

	root, _, err := reporoot.Find()
	if err != nil {
		return "", fmt.Errorf("find repo root: %w", err)
	}
	return filepath.Abs(root)
}
