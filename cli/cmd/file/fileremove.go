package file

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/cordon-co/cordon-cli/cli/internal/codexpolicy"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/cordon-co/cordon-cli/cli/internal/reporoot"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <pattern>",
	Short: "Remove a file rule",
	Args:  cobra.ExactArgs(1),
	RunE:  runFileRemove,
}

type fileRemoveResult struct {
	Pattern string `json:"pattern"`
	Removed bool   `json:"removed"`
}

func runFileRemove(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("file remove: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("file remove: resolve repo root: %w", err)
	}

	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("file remove: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("file remove: migrate policy database: %w", err)
	}

	removed, err := store.RemoveFileRule(policyDB, pattern)
	if err != nil {
		return fmt.Errorf("file remove: %w", err)
	}

	if removed {
		// Log to audit database.
		user := store.CurrentOSUser()
		dataDB, err := store.OpenDataDB(absRoot)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not open data database for audit: %v\n", err)
		} else {
			defer dataDB.Close()
			if err := store.MigrateDataDB(dataDB); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not migrate data database: %v\n", err)
			} else {
				_ = store.InsertAudit(dataDB, store.AuditEntry{
					EventType: "file_remove",
					FilePath:  pattern,
					User:      user,
				})
			}
		}

		// Regenerate the Codex policy file.
		rules, err := store.ListFileRules(policyDB)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not list file rules for Codex policy: %v\n", err)
		} else if err := codexpolicy.Generate(absRoot, rules); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not regenerate Codex policy: %v\n", err)
		}
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(fileRemoveResult{Pattern: pattern, Removed: removed}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if removed {
		fmt.Printf("removed file rule: %s\n", pattern)
	} else {
		fmt.Printf("no file rule found for pattern: %s\n", pattern)
	}
	return nil
}
