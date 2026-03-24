package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cordon-co/cordon-cli/cli/internal/codexpolicy"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/cordon-co/cordon-cli/cli/internal/reporoot"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
	"github.com/spf13/cobra"
)

var (
	elevated    bool
	preventRead bool
	allow       bool
)

var addCmd = &cobra.Command{
	Use:   "add <pattern>",
	Short: "Add a file rule",
	Long:  "Protect a file, folder, or glob pattern from agent writes.",
	Args:  cobra.ExactArgs(1),
	RunE:  runFileAdd,
}

func init() {
	addCmd.Flags().BoolVar(&preventRead, "prevent-read", false, "Also block agent read access (e.g. for credential files)")
	addCmd.Flags().BoolVar(&allow, "allow", false, "Create an allow file rule (permits access, overrides deny rules)")
	addCmd.Flags().BoolVar(&elevated, "elevated", false, "Create an elevated-authority rule (requires elevated/admin permissions to remove)")
}

type fileAddResult struct {
	FileRule store.FileRule `json:"file_rule"`
}

func runFileAdd(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("file add: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("file add: resolve repo root: %w", err)
	}

	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("file add: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("file add: migrate policy database: %w", err)
	}

	if allow && preventRead {
		return fmt.Errorf("file add: --allow and --prevent-read cannot be used together (allow rules permit access)")
	}

	fileAccess := "deny"
	if allow {
		fileAccess = "allow"
	}
	fileAuthority := "standard"
	if elevated {
		fileAuthority = "elevated"
	}

	user := store.CurrentOSUser()

	// Normalize the pattern to a repo-relative path so that absolute paths
	// like /home/user/repo/src/main.go are stored as src/main.go.
	// Glob patterns and already-relative patterns are unchanged.
	pattern = store.NormalizePattern(pattern, absRoot)

	f, err := store.AddFileRule(policyDB, pattern, fileAccess, fileAuthority, user, preventRead)
	if err != nil {
		if errors.Is(err, store.ErrDuplicatePattern) {
			return fmt.Errorf("file rule already exists: %s", pattern)
		}
		return fmt.Errorf("file add: %w", err)
	}

	// Log to audit database.
	dataDB, err := store.OpenDataDB(absRoot)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not open data database for audit: %v\n", err)
	} else {
		defer dataDB.Close()
		if err := store.MigrateDataDB(dataDB); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not migrate data database: %v\n", err)
		} else {
			_ = store.InsertAudit(dataDB, store.AuditEntry{
				EventType:  "file_add",
				FileRuleID: f.ID,
				FilePath:   f.Pattern,
				User:       user,
				Detail:     fmt.Sprintf("file_access=%s file_authority=%s", f.FileType, f.FileAuthority),
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

	if flags.JSON {
		out, _ := json.MarshalIndent(fileAddResult{FileRule: *f}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	ruleLabel := "deny rule"
	if f.FileType == "allow" {
		ruleLabel = "allow rule"
	}
	if f.FileAuthority == "elevated" {
		ruleLabel += " (elevated)"
	}
	readLabel := ""
	if f.PreventRead {
		readLabel = " (read+write)"
	}
	fmt.Printf("added %s%s: %s\n", ruleLabel, readLabel, f.Pattern)
	return nil
}
