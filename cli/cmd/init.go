package cmd

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon-cli/internal/agents"
	"github.com/cordon-co/cordon-cli/internal/codexpolicy"
	"github.com/cordon-co/cordon-cli/internal/flags"
	"github.com/cordon-co/cordon-cli/internal/store"
	"github.com/cordon-co/cordon-cli/internal/tui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialise Cordon in the current repository",
	Long: `Creates .cordon/policy.db, configures agent platform hooks and MCP
servers, and creates the user-level data database at
~/.cordon/repos/<hash>/data.db.

An interactive agent selector lets you choose which platforms to configure.
Running cordon init more than once is safe — it detects an existing
installation and returns an informative message.`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

type initResult struct {
	RepoRoot string   `json:"repo_root"`
	PolicyDB string   `json:"policy_db"`
	DataDB   string   `json:"data_db"`
	Agents   []string `json:"agents"`
}

func runInit(cmd *cobra.Command, args []string) error {
	// init always operates on the current working directory — it must not
	// walk up the tree (that would find ~/.cordon/ and initialise in $HOME).
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("init: get working directory: %w", err)
	}

	absRoot, err := filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("init: resolve repo root: %w", err)
	}

	// Check if already initialised.
	policyDBPath := filepath.Join(absRoot, ".cordon", "policy.db")
	if store.HasPerimeterID(policyDBPath) {
		if flags.JSON {
			out, _ := json.MarshalIndent(map[string]interface{}{
				"already_initialised": true,
				"repo_root":          absRoot,
			}, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		fmt.Printf("Cordon is already initialised in %s\n", absRoot)
		return nil
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

	// Ensure a stable perimeter ID exists for this project.
	perimeterID, err := store.EnsurePerimeterID(policyDB)
	if err != nil {
		return fmt.Errorf("init: ensure perimeter id: %w", err)
	}

	// Data database (~/.cordon/repos/<perimeter_id>/data.db)
	dataDBPath, err := store.DataDBPathFromID(perimeterID)
	if err != nil {
		return fmt.Errorf("init: resolve data db path: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(dataDBPath), 0o755); err != nil {
		return fmt.Errorf("init: create data directory: %w", err)
	}

	dataDB, err := sql.Open("sqlite", dataDBPath)
	if err != nil {
		return fmt.Errorf("init: open data database: %w", err)
	}
	defer dataDB.Close()

	if _, err := dataDB.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		dataDB.Close()
		return fmt.Errorf("init: set WAL mode on data.db: %w", err)
	}

	if err := store.MigrateDataDB(dataDB); err != nil {
		return fmt.Errorf("init: migrate data database: %w", err)
	}

	// Agent platform selection.
	selectedIDs, selectedNames, err := selectAgents(cmd)
	if err != nil {
		return fmt.Errorf("init: agent selection: %w", err)
	}

	// Install selected agents.
	if err := agents.InstallSelected(absRoot, selectedIDs); err != nil {
		return fmt.Errorf("init: install agents: %w", err)
	}

	// Store installed agent IDs in perimeter_meta.
	if err := store.SetInstalledAgents(policyDB, selectedIDs); err != nil {
		return fmt.Errorf("init: store installed agents: %w", err)
	}

	// Standard guardrails — prompt user to opt in (skip in --json mode).
	var addedCommands []string
	var addedFiles []string
	if !flags.JSON {
		var err error
		addedCommands, addedFiles, err = promptAndAddGuardrails(cmd, policyDB)
		if err != nil {
			return fmt.Errorf("init: guardrails: %w", err)
		}
	}

	// Regenerate codex-policy.md after guardrails are added (if Codex was selected).
	if hasAgent(selectedIDs, "codex") {
		rules, err := store.ListFileRules(policyDB)
		if err != nil {
			return fmt.Errorf("init: list file rules for Codex policy: %w", err)
		}
		if err := codexpolicy.Generate(absRoot, rules); err != nil {
			return fmt.Errorf("init: generate Codex policy: %w", err)
		}
	}

	result := initResult{
		RepoRoot: absRoot,
		PolicyDB: filepath.Join(absRoot, ".cordon", "policy.db"),
		DataDB:   dataDBPath,
		Agents:   selectedNames,
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("\nCordon initialised in %s\n", absRoot)
	if len(selectedNames) > 0 {
		fmt.Printf("  Managed Agents:  %s\n", strings.Join(selectedNames, ", "))
	}
	if len(addedCommands) > 0 {
		fmt.Println("  Command Rules:")
		for _, c := range addedCommands {
			fmt.Printf("    deny  %s\n", c)
		}
	}
	if len(addedFiles) > 0 {
		fmt.Println("  File Rules:")
		for _, f := range addedFiles {
			fmt.Printf("    deny  %s\n", f)
		}
	}
	if hasAgent(selectedIDs, "cursor") {
		fmt.Println("\n  Note: Cordon MCP will need to be enabled in Cursor Settings -> Tools and MCP")
	}
	return nil
}

// selectAgents presents the interactive TUI (or auto-selects in --json/non-TTY).
// Returns the selected agent IDs and display names.
func selectAgents(cmd *cobra.Command) (ids []string, names []string, err error) {
	allAgents := agents.All()

	if flags.JSON {
		// Auto-select all installable agents.
		for _, a := range allAgents {
			if a.Installable() {
				ids = append(ids, a.ID())
				names = append(names, a.DisplayName())
			}
		}
		return ids, names, nil
	}

	// Build TUI options.
	options := make([]tui.Option, len(allAgents))
	for i, a := range allAgents {
		options[i] = tui.Option{
			Label:      a.DisplayName(),
			Selectable: a.Installable(),
			Selected:   a.Installable(), // pre-select all installable
		}
		if !a.Installable() {
			options[i].Suffix = "(coming soon)"
		}
	}

	fmt.Fprintln(cmd.ErrOrStderr())
	fmt.Fprintln(cmd.ErrOrStderr(), "Select agent platforms to configure (space to toggle, enter to confirm):")
	selected, err := tui.MultiSelect("", options)
	if err != nil {
		if err == tui.ErrAborted {
			// User aborted — fall back to all installable.
			for _, a := range allAgents {
				if a.Installable() {
					ids = append(ids, a.ID())
					names = append(names, a.DisplayName())
				}
			}
			return ids, names, nil
		}
		return nil, nil, err
	}

	for _, idx := range selected {
		ids = append(ids, allAgents[idx].ID())
		names = append(names, allAgents[idx].DisplayName())
	}
	return ids, names, nil
}

func hasAgent(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
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
// Returns the lists of command patterns and file patterns that were added.
func promptAndAddGuardrails(cmd *cobra.Command, policyDB *sql.DB) (addedCommands []string, addedFiles []string, err error) {
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
		return nil, nil, nil
	}

	user := store.CurrentOSUser()

	for _, pattern := range store.StandardGuardrails {
		_, err := store.AddRule(policyDB, pattern, "deny", "standard", user)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				continue
			}
			return nil, nil, fmt.Errorf("add guardrail rule %q: %w", pattern, err)
		}
		addedCommands = append(addedCommands, pattern)
	}

	for _, f := range store.StandardGuardrailFileRules {
		_, err := store.AddFileRule(policyDB, f.Pattern, "deny", "standard", user, f.PreventRead)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				continue
			}
			return nil, nil, fmt.Errorf("add guardrail file rule %q: %w", f.Pattern, err)
		}
		addedFiles = append(addedFiles, f.Pattern)
	}

	total := len(addedCommands) + len(addedFiles)
	if total > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "  added %d guardrail(s).\n", total)
	} else {
		fmt.Fprintln(cmd.OutOrStdout(), "  already configured.")
	}
	return addedCommands, addedFiles, nil
}
