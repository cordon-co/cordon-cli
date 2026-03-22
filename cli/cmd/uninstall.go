package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon-cli/cli/internal/agents"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/spf13/cobra"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Uninstall Cordon from the current repository",
	Long: `Removes all Cordon configuration from the current repository:

  - Removes all agent platform hooks and MCP server entries
    (leaves all other entries intact)
  - Removes the .cordon/ directory (local policy will be discarded)

User-level data (~/.cordon/repos/<hash>/) is not removed.`,
	Args: cobra.NoArgs,
	RunE: runUninstall,
}

type uninstallResult struct {
	RepoRoot string   `json:"repo_root"`
	Removed  []string `json:"removed"`
}

func runUninstall(cmd *cobra.Command, args []string) error {
	// Uninstall only operates on the current working directory — it must not
	// walk up the tree, as that could accidentally uninstall a parent project.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("uninstall: get working directory: %w", err)
	}

	absRoot, err := filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("uninstall: resolve directory: %w", err)
	}

	// Check that Cordon is actually installed in this directory.
	cordonDir := filepath.Join(absRoot, ".cordon")
	if _, err := os.Stat(cordonDir); os.IsNotExist(err) {
		if flags.JSON {
			out, _ := json.MarshalIndent(map[string]interface{}{
				"error":     "not_installed",
				"repo_root": absRoot,
			}, "", "  ")
			fmt.Println(string(out))
			return nil
		}
		return fmt.Errorf("no .cordon/ install folder found. 'cordon uninstall' must be run from the project directory where Cordon was initialised")
	}

	// Prompt for confirmation unless in --json mode (non-interactive).
	if !flags.JSON {
		fmt.Fprintln(cmd.OutOrStdout(), "This will remove all Cordon configuration from this repository.")
		fmt.Fprintln(cmd.OutOrStdout(), "Any local policy (file rules, command rules) in .cordon/ will be discarded.")
		fmt.Fprint(cmd.OutOrStdout(), "Are you sure? [Y/n]: ")

		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())

		if answer != "" && !strings.EqualFold(answer, "y") && !strings.EqualFold(answer, "yes") {
			fmt.Fprintln(cmd.OutOrStdout(), "Aborted.")
			return nil
		}
	}

	var removed []string

	if err := agents.RemoveAll(absRoot); err != nil {
		return fmt.Errorf("uninstall: remove agent configurations: %w", err)
	}
	removed = append(removed, "agent configurations removed")

	if _, err := os.Stat(cordonDir); err == nil {
		if err := os.RemoveAll(cordonDir); err != nil {
			return fmt.Errorf("uninstall: delete .cordon/: %w", err)
		}
		removed = append(removed, ".cordon/")
	}

	result := uninstallResult{
		RepoRoot: absRoot,
		Removed:  removed,
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("Cordon removed from %s\n", absRoot)
	return nil
}
