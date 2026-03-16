package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon/internal/claudecfg"
	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise Cordon in the current repository",
	Long: `Creates .cordon/policy.db, writes the PreToolUse hook and MCP server
entries into .claude/settings.local.json, and creates the user-level data
database at ~/.cordon/repos/<hash>/data.db.

Running cordon init more than once is safe — all operations are idempotent.`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

type initResult struct {
	RepoRoot     string `json:"repo_root"`
	PolicyDB     string `json:"policy_db"`
	DataDB       string `json:"data_db"`
	SettingsFile string `json:"settings_file"`
}

func runInit(cmd *cobra.Command, args []string) error {
	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("init: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("init: resolve repo root: %w", err)
	}

	// Policy database (.cordon/policy.db)
	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("init: open policy database: %w", err)
	}
	defer policyDB.Close()

	// Data database (~/.cordon/repos/<hash>/data.db)
	dataDB, err := store.OpenDataDB(absRoot)
	if err != nil {
		return fmt.Errorf("init: open data database: %w", err)
	}
	defer dataDB.Close()

	dataDBPath, err := store.DataDBPath(absRoot)
	if err != nil {
		return fmt.Errorf("init: resolve data db path: %w", err)
	}

	// .claude/settings.local.json
	settingsPath := filepath.Join(absRoot, ".claude", "settings.local.json")
	if err := claudecfg.AddCordonEntries(absRoot); err != nil {
		return fmt.Errorf("init: update settings.local.json: %w", err)
	}

	result := initResult{
		RepoRoot:     absRoot,
		PolicyDB:     filepath.Join(absRoot, ".cordon", "policy.db"),
		DataDB:       dataDBPath,
		SettingsFile: settingsPath,
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	homeDir, _ := os.UserHomeDir()
	fmt.Printf("cordon initialised in %s\n", absRoot)
	fmt.Printf("  policy DB:    %s\n", shortenHome(result.PolicyDB, homeDir))
	fmt.Printf("  data DB:      %s\n", shortenHome(result.DataDB, homeDir))
	fmt.Printf("  Claude hooks: %s\n", shortenHome(result.SettingsFile, homeDir))
	return nil
}

// shortenHome replaces the user home directory prefix with ~ for display.
func shortenHome(path, homeDir string) string {
	if homeDir == "" {
		return path
	}
	rel, err := filepath.Rel(homeDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return filepath.Join("~", rel)
}
