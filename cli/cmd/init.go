package cmd

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon/internal/claudecfg"
	"github.com/cordon-co/cordon/internal/codexpolicy"
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
	CodexPolicy  string `json:"codex_policy"`
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

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("init: migrate policy database: %w", err)
	}

	// Data database (~/.cordon/repos/<hash>/data.db)
	dataDB, err := store.OpenDataDB(absRoot)
	if err != nil {
		return fmt.Errorf("init: open data database: %w", err)
	}
	defer dataDB.Close()

	if err := store.MigrateDataDB(dataDB); err != nil {
		return fmt.Errorf("init: migrate data database: %w", err)
	}

	dataDBPath, err := store.DataDBPath(absRoot)
	if err != nil {
		return fmt.Errorf("init: resolve data db path: %w", err)
	}

	// .claude/settings.local.json
	settingsPath := filepath.Join(absRoot, ".claude", "settings.local.json")
	if err := claudecfg.AddCordonEntries(absRoot); err != nil {
		return fmt.Errorf("init: update settings.local.json: %w", err)
	}

	// Standard guardrails — prompt user to opt in (skip in --json mode).
	if !flags.JSON {
		if err := promptAndAddGuardrails(cmd, policyDB); err != nil {
			return fmt.Errorf("init: guardrails: %w", err)
		}
	}

	// .cordon/codex-policy.md — generate from current file rule list (may be empty on first init).
	rules, err := store.ListFileRules(policyDB)
	if err != nil {
		return fmt.Errorf("init: list file rules for Codex policy: %w", err)
	}
	if err := codexpolicy.Generate(absRoot, rules); err != nil {
		return fmt.Errorf("init: generate Codex policy: %w", err)
	}
	codexPolicyPath := filepath.Join(absRoot, ".cordon", "codex-policy.md")

	result := initResult{
		RepoRoot:     absRoot,
		PolicyDB:     filepath.Join(absRoot, ".cordon", "policy.db"),
		DataDB:       dataDBPath,
		SettingsFile: settingsPath,
		CodexPolicy:  codexPolicyPath,
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	homeDir, _ := os.UserHomeDir()
	fmt.Printf("cordon initialised in %s\n", absRoot)
	fmt.Printf("  policy DB:     %s\n", shortenHome(result.PolicyDB, homeDir))
	fmt.Printf("  data DB:       %s\n", shortenHome(result.DataDB, homeDir))
	fmt.Printf("  Claude hooks:  %s\n", shortenHome(result.SettingsFile, homeDir))
	fmt.Printf("  Codex policy:  %s\n", shortenHome(result.CodexPolicy, homeDir))
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

// promptAndAddGuardrails offers the user the standard set of guardrails.
// Rules and file rules that already exist are skipped (idempotent). If the user
// declines, nothing is added. The prompt defaults to yes.
func promptAndAddGuardrails(cmd *cobra.Command, policyDB *sql.DB) error {
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Add standard guardrails? These include sensible defaults for common")
	fmt.Fprintln(cmd.OutOrStdout(), "footguns like 'git reset --hard', 'rm -rf', and credential file protection")
	fmt.Fprintln(cmd.OutOrStdout(), "(.env, credentials.json, *.pem, etc. — blocked for both read and write).")
	fmt.Fprint(cmd.OutOrStdout(), "Add guardrails? [Y/n]: ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	answer := strings.TrimSpace(scanner.Text())

	// Default to yes on empty input.
	if answer != "" && strings.ToLower(answer) != "y" && strings.ToLower(answer) != "yes" {
		fmt.Fprintln(cmd.OutOrStdout(), "  skipped.")
		return nil
	}

	user := store.CurrentOSUser()
	added := 0

	for _, pattern := range store.StandardGuardrails {
		_, err := store.AddRule(policyDB, pattern, "deny", "standard", user)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				continue
			}
			return fmt.Errorf("add guardrail rule %q: %w", pattern, err)
		}
		added++
	}

	for _, f := range store.StandardGuardrailFileRules {
		_, err := store.AddFileRule(policyDB, f.Pattern, "deny", "standard", user, f.PreventRead)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				continue
			}
			return fmt.Errorf("add guardrail file rule %q: %w", f.Pattern, err)
		}
		added++
	}

	if added > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  added %d guardrail(s).\n", added)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  already configured.")
	}
	return nil
}
