package command

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/hook"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List command rules",
	Long:  "List all command rules (built-in and custom).",
	Args:  cobra.NoArgs,
	RunE:  runCommandList,
}

type commandListResult struct {
	Rules []store.CommandRule `json:"rules"`
}

func runCommandList(cmd *cobra.Command, args []string) error {
	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("command list: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("command list: resolve repo root: %w", err)
	}

	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("command list: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("command list: migrate policy database: %w", err)
	}

	customRules, err := store.ListRules(policyDB)
	if err != nil {
		return fmt.Errorf("command list: %w", err)
	}

	// Merge built-in rules (as CommandRule structs) with custom rules.
	allRules := hook.BuiltinRulesAsStore()
	allRules = append(allRules, customRules...)

	if flags.JSON {
		result := commandListResult{Rules: allRules}
		if result.Rules == nil {
			result.Rules = []store.CommandRule{}
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(allRules) == 0 {
		fmt.Println("no command rules configured")
		return nil
	}

	fmt.Printf("%-30s  %-9s  %s\n", "PATTERN", "TYPE", "REASON")
	fmt.Println(strings.Repeat("-", 80))
	for _, r := range allRules {
		fmt.Printf("%-30s  %-9s  %s\n", r.Pattern, r.RuleType, r.Reason)
	}
	return nil
}
