package file

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active file rules",
	Args:  cobra.NoArgs,
	RunE:  runFileList,
}

type fileListResult struct {
	FileRules []store.FileRule `json:"file_rules"`
}

func runFileList(cmd *cobra.Command, args []string) error {
	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("file list: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("file list: resolve repo root: %w", err)
	}

	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("file list: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("file list: migrate policy database: %w", err)
	}

	rules, err := store.ListFileRules(policyDB)
	if err != nil {
		return fmt.Errorf("file list: %w", err)
	}

	if flags.JSON {
		result := fileListResult{FileRules: rules}
		if result.FileRules == nil {
			result.FileRules = []store.FileRule{}
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(rules) == 0 {
		fmt.Println("no file rules configured")
		return nil
	}

	// Human-readable table.
	fmt.Printf("%-30s  %-9s  %-12s  %-16s  %s\n", "PATTERN", "TYPE", "OPS", "CREATED BY", "CREATED AT")
	fmt.Println(strings.Repeat("-", 92))
	for _, f := range rules {
		createdAt := f.CreatedAt
		if len(createdAt) > 10 {
			createdAt = createdAt[:10] // show date portion only
		}
		createdBy := f.CreatedBy
		if createdBy == "" {
			createdBy = "local"
		}
		blocks := "write"
		if f.PreventRead || f.FileType == "allow" {
			blocks = "read+write"
		}
		fmt.Printf("%-30s  %-9s  %-12s  %-16s  %s\n", f.Pattern, f.FileType, blocks, createdBy, createdAt)
	}
	return nil
}
