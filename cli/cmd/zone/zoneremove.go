package zone

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/cordon-co/cordon/internal/codexpolicy"
	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove <pattern>",
	Short: "Remove a zone",
	Args:  cobra.ExactArgs(1),
	RunE:  runZoneRemove,
}

type zoneRemoveResult struct {
	Pattern string `json:"pattern"`
	Removed bool   `json:"removed"`
}

func runZoneRemove(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("zone remove: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("zone remove: resolve repo root: %w", err)
	}

	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("zone remove: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("zone remove: migrate policy database: %w", err)
	}

	removed, err := store.RemoveZone(policyDB, pattern)
	if err != nil {
		return fmt.Errorf("zone remove: %w", err)
	}

	if removed {
		// Log to audit database.
		user := store.CurrentOSUser()
		dataDB, err := store.OpenDataDB(absRoot)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not open data database for audit: %v\n", err)
		} else {
			defer dataDB.Close()
			if err := store.MigrateDataDB(dataDB); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not migrate data database: %v\n", err)
			} else {
				_ = store.InsertAudit(dataDB, store.AuditEntry{
					EventType: "zone_remove",
					FilePath:  pattern,
					User:      user,
				})
			}
		}

		// Regenerate the Codex policy file.
		zones, err := store.ListZones(policyDB)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not list zones for Codex policy: %v\n", err)
		} else if err := codexpolicy.Generate(absRoot, zones); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not regenerate Codex policy: %v\n", err)
		}
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(zoneRemoveResult{Pattern: pattern, Removed: removed}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if removed {
		fmt.Printf("removed zone: %s\n", pattern)
	} else {
		fmt.Printf("no zone found for pattern: %s\n", pattern)
	}
	return nil
}
