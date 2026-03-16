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

var guardian bool

var addCmd = &cobra.Command{
	Use:   "add <pattern>",
	Short: "Add a zone",
	Long:  "Protect a file, folder, or glob pattern from agent writes.",
	Args:  cobra.ExactArgs(1),
	RunE:  runZoneAdd,
}

func init() {
	addCmd.Flags().BoolVar(&guardian, "guardian", false, "Create a guardian zone (requires guardian/admin role)")
}

type zoneAddResult struct {
	Zone store.Zone `json:"zone"`
}

func runZoneAdd(cmd *cobra.Command, args []string) error {
	pattern := args[0]

	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("zone add: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("zone add: resolve repo root: %w", err)
	}

	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("zone add: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("zone add: migrate policy database: %w", err)
	}

	zoneType := "standard"
	if guardian {
		zoneType = "guardian"
	}

	user := store.CurrentOSUser()

	z, err := store.AddZone(policyDB, pattern, zoneType, user)
	if err != nil {
		return fmt.Errorf("zone add: %w", err)
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
				EventType: "zone_add",
				ZoneID:    z.ID,
				FilePath:  z.Pattern,
				User:      user,
				Detail:    fmt.Sprintf("zone_type=%s", z.ZoneType),
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

	if flags.JSON {
		out, _ := json.MarshalIndent(zoneAddResult{Zone: *z}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	zoneLabel := z.ZoneType
	if z.ZoneType == "guardian" {
		zoneLabel = "guardian zone"
	} else {
		zoneLabel = "standard zone"
	}
	fmt.Printf("added %s: %s\n", zoneLabel, z.Pattern)
	return nil
}
