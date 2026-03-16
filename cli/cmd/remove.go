package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cordon-co/cordon/internal/claudecfg"
	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Uninstall Cordon from the current repository",
	Long: `Removes all Cordon configuration from the current repository:

  - Removes the Cordon hook entry from .claude/settings.local.json
    (leaves all other hook entries intact)
  - Removes the Cordon MCP server entry from .claude/settings.local.json
    (leaves all other MCP server entries intact)
  - Removes the .cordon/ directory

User-level data (~/.cordon/repos/<hash>/) is not removed.`,
	Args: cobra.NoArgs,
	RunE: runRemove,
}

type removeResult struct {
	RepoRoot string   `json:"repo_root"`
	Removed  []string `json:"removed"`
}

func runRemove(cmd *cobra.Command, args []string) error {
	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("remove: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("remove: resolve repo root: %w", err)
	}

	var removed []string

	if err := claudecfg.RemoveCordonEntries(absRoot); err != nil {
		return fmt.Errorf("remove: update settings.local.json: %w", err)
	}
	removed = append(removed, ".claude/settings.local.json (Cordon entries removed)")

	cordonDir := filepath.Join(absRoot, ".cordon")
	if _, err := os.Stat(cordonDir); err == nil {
		if err := os.RemoveAll(cordonDir); err != nil {
			return fmt.Errorf("remove: delete .cordon/: %w", err)
		}
		removed = append(removed, ".cordon/")
	}

	result := removeResult{
		RepoRoot: absRoot,
		Removed:  removed,
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	homeDir, _ := os.UserHomeDir()
	fmt.Printf("cordon removed from %s\n", absRoot)
	for _, item := range result.Removed {
		fmt.Printf("  %s\n", item)
	}
	_ = homeDir
	return nil
}
