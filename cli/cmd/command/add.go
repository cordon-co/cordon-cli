package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var addAllow bool

var addCmd = &cobra.Command{
	Use:   "add <pattern>",
	Short: "Add a command rule",
	Long:  "Add a command rule. Deny rules (default) block matching commands. Allow rules permit commands, overriding deny rules.",
	Args:  cobra.ExactArgs(1),
	RunE:  runCommandAdd,
}

func init() {
	addCmd.Flags().BoolVar(&addAllow, "allow", false, "Create an allow rule (permits command, overrides deny rules)")
}

type commandAddResult struct {
	Rule store.CommandRule `json:"rule"`
}

func runCommandAdd(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("command add: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("command add: resolve repo root: %w", err)
	}

	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("command add: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("command add: migrate policy database: %w", err)
	}

	ruleAccess := "deny"
	if addAllow {
		ruleAccess = "allow"
	}
	ruleAuthority := "standard"

	r, err := store.AddRule(policyDB, pattern, ruleAccess, ruleAuthority, store.CurrentOSUser())
	if err != nil {
		if errors.Is(err, store.ErrDuplicatePattern) {
			return fmt.Errorf("command rule already exists: %s", pattern)
		}
		return fmt.Errorf("command add: %w", err)
	}

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
				EventType: "command_add",
				Detail:    fmt.Sprintf("pattern=%s rule_type=%s rule_authority=%s", r.Pattern, r.RuleType, r.RuleAuthority),
				User:      store.CurrentOSUser(),
			})
		}
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(commandAddResult{Rule: *r}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	ruleLabel := "deny command rule"
	if r.RuleType == "allow" {
		ruleLabel = "allow command rule"
	}
	if r.RuleAuthority == "guardian" {
		ruleLabel += " (guardian)"
	}
	fmt.Printf("added %s: %s\n", ruleLabel, r.Pattern)
	return nil
}
