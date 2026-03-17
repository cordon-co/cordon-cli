package command

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <pattern>",
	Short: "Remove a custom command rule",
	Long:  "Remove a custom command rule. Built-in rules cannot be removed.",
	Args:  cobra.ExactArgs(1),
	RunE:  runCommandRemove,
}

type commandRemoveResult struct {
	Pattern string `json:"pattern"`
	Removed bool   `json:"removed"`
}

func runCommandRemove(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("command remove: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("command remove: resolve repo root: %w", err)
	}

	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("command remove: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("command remove: migrate policy database: %w", err)
	}

	removed, err := store.RemoveRule(policyDB, pattern)
	if err != nil {
		return fmt.Errorf("command remove: %w", err)
	}

	if removed {
		// Audit log — non-fatal.
		dataDB, err := store.OpenDataDB(absRoot)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not open data database for audit: %v\n", err)
		} else {
			defer dataDB.Close()
			if err := store.MigrateDataDB(dataDB); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not migrate data database: %v\n", err)
			} else {
				_ = store.InsertAudit(dataDB, store.AuditEntry{
					EventType: "command_remove",
					Detail:    fmt.Sprintf("pattern=%s", pattern),
					User:      store.CurrentOSUser(),
				})
			}
		}
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(commandRemoveResult{Pattern: pattern, Removed: removed}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if removed {
		fmt.Printf("removed command rule: %s\n", pattern)
	} else {
		fmt.Printf("no custom rule found for pattern: %s\n", pattern)
	}
	return nil
}
