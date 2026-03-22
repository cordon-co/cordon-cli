package command

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/cordon-co/cordon-cli/cli/internal/reporoot"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
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

	allRules, err := store.ListRules(policyDB)
	if err != nil {
		return fmt.Errorf("command list: %w", err)
	}

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

	fmt.Printf("%-30s  %-6s  %-16s  %s\n", "PATTERN", "TYPE", "CREATED BY", "CREATED AT")
	fmt.Println(strings.Repeat("-", 80))
	for _, r := range allRules {
		createdAt := r.CreatedAt
		if len(createdAt) > 10 {
			createdAt = createdAt[:10] // show date portion only
		}
		createdBy := r.CreatedBy
		if createdBy == "" {
			createdBy = "local"
		}
		fmt.Printf("%-30s  %-6s  %-16s  %s\n", r.Pattern, r.RuleType, createdBy, createdAt)
	}
	return nil
}
