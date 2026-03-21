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
		checker := buildPolicyChecker()
		rdChecker := buildReadChecker()
		cmdChecker := buildCommandChecker()
		event, err := hook.Evaluate(os.Stdin, os.Stdout, os.Stderr, checker, rdChecker, cmdChecker)

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

// buildPolicyChecker returns a PolicyChecker that opens the policy and data
// databases from the agent cwd on each call.
//
// On any infrastructure error (DB unreadable, repo root not found) the checker
// fails open — it returns (true, "") so the hook allows the write and logs the
// failure. This matches Cordon's fail-open design principle.
func buildPolicyChecker() hook.PolicyChecker {
	return func(filePath, cwd string) (allowed bool, passID string) {
		absRoot, err := resolveRepoRoot(cwd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cordon: policy check: resolve repo root: %v\n", err)
			return true, "" // fail-open
		}

		policyDB, err := store.OpenPolicyDB(absRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cordon: policy check: open policy db: %v\n", err)
			return true, "" // fail-open
		}
		defer policyDB.Close()

		if err := store.MigratePolicyDB(policyDB); err != nil {
			fmt.Fprintf(os.Stderr, "cordon: policy check: migrate policy db: %v\n", err)
			return true, "" // fail-open
		}

		rule, err := store.FileRuleForPath(policyDB, filePath, absRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cordon: policy check: file rule lookup: %v\n", err)
			return true, "" // fail-open
		}
		if rule == nil {
			// File is not covered by any file rule — allow.
			return true, ""
		}

		// File is covered by a file rule. Check for an active pass in the data database.
		dataDB, err := store.OpenDataDB(absRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cordon: policy check: open data db: %v\n", err)
			return false, "" // has file rule, data DB unavailable — deny
		}
		defer dataDB.Close()

		if err := store.MigrateDataDB(dataDB); err != nil {
			fmt.Fprintf(os.Stderr, "cordon: policy check: migrate data db: %v\n", err)
			return false, "" // has file rule, data DB unavailable — deny
		}

		pass, err := store.ActivePassForPath(dataDB, filePath, absRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cordon: policy check: pass lookup: %v\n", err)
			return false, "" // has file rule, pass lookup failed — deny
		}
		if pass == nil {
			return false, "" // has file rule, no active pass — deny
		}
		return true, pass.ID // has file rule, active pass — allow
	}
}

// buildReadChecker returns a ReadChecker that denies reads of files covered
// by file rules where prevent_read=true, unless an active pass covers the file.
//
// Fails open on any infrastructure error.
func buildReadChecker() hook.ReadChecker {
	return func(filePath, cwd string) (allowed bool, passID string) {
		absRoot, err := resolveRepoRoot(cwd)
		if err != nil {
			return true, "" // fail-open
		}

		policyDB, err := store.OpenPolicyDB(absRoot)
		if err != nil {
			return true, "" // fail-open
		}
		defer policyDB.Close()

		if err := store.MigratePolicyDB(policyDB); err != nil {
			return true, "" // fail-open
		}

		rule, err := store.FileRuleForPath(policyDB, filePath, absRoot)
		if err != nil || rule == nil || !rule.PreventRead {
			return true, "" // fail-open or not in a prevent-read file rule
		}

		// File is in a prevent-read file rule. Check for an active pass.
		dataDB, err := store.OpenDataDB(absRoot)
		if err != nil {
			return false, "" // has file rule, data DB unavailable — deny
		}
		defer dataDB.Close()

		if err := store.MigrateDataDB(dataDB); err != nil {
			return false, "" // has file rule, data DB unavailable — deny
		}

		pass, err := store.ActivePassForPath(dataDB, filePath, absRoot)
		if err != nil || pass == nil {
			return false, "" // has file rule, no active pass — deny
		}
		return true, pass.ID
	}
}

// buildCommandChecker returns a CommandChecker that checks custom command rules
// from the policy database. Built-in rules are always checked first within
// hook.evaluateBash itself, so this checker only needs to handle custom rules.
//
// Fails open on any infrastructure error.
func buildCommandChecker() hook.CommandChecker {
	return func(command, cwd string) (allowed bool, matched *hook.MatchedRule) {
		absRoot, err := resolveRepoRoot(cwd)
		if err != nil {
			return true, nil // fail-open
		}

		policyDB, err := store.OpenPolicyDB(absRoot)
		if err != nil {
			return true, nil // fail-open
		}
		defer policyDB.Close()

		if err := store.MigratePolicyDB(policyDB); err != nil {
			return true, nil // fail-open
		}

		rule, err := store.MatchCommandRule(policyDB, command)
		if err != nil || rule == nil {
			return true, nil // fail-open or no match
		}

		return false, &hook.MatchedRule{
			Pattern:       rule.Pattern,
			RuleType:      rule.RuleType,
			RuleAuthority: rule.RuleAuthority,
		}
	}
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
