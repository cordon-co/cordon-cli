package zone

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/cordon-co/cordon/internal/codexpolicy"
	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var (
	guardian     bool
	preventRead bool
	allow        bool
)

var addCmd = &cobra.Command{
	Use:   "add <pattern>",
	Short: "Add a zone",
	Long:  "Protect a file, folder, or glob pattern from agent writes.",
	Args:  cobra.ExactArgs(1),
	RunE:  runZoneAdd,
}

func init() {
	addCmd.Flags().BoolVar(&guardian, "guardian", false, "Create a guardian zone (requires guardian/admin role)")
	addCmd.Flags().BoolVar(&preventRead, "prevent-read", false, "Also block agent read access (e.g. for credential files)")
	addCmd.Flags().BoolVar(&allow, "allow", false, "Create an allow zone (permits access, overrides deny zones)")
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

	if allow && preventRead {
		return fmt.Errorf("zone add: --allow and --prevent-read cannot be used together (allow zones permit access)")
	}

	zoneAccess := "deny"
	if allow {
		zoneAccess = "allow"
	}
	zoneAuthority := "standard"
	if guardian {
		zoneAuthority = "guardian"
	}

	user := store.CurrentOSUser()

	// Normalize the pattern to a repo-relative path so that absolute paths
	// like /home/user/repo/src/main.go are stored as src/main.go.
	// Glob patterns and already-relative patterns are unchanged.
	pattern = store.NormalizePattern(pattern, absRoot)

	z, err := store.AddZone(policyDB, pattern, zoneAccess, zoneAuthority, user, preventRead)
	if err != nil {
		if errors.Is(err, store.ErrDuplicatePattern) {
			return fmt.Errorf("zone already exists: %s", pattern)
		}
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
				Detail:    fmt.Sprintf("zone_access=%s zone_authority=%s", z.ZoneType, z.ZoneAuthority),
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

	zoneLabel := "deny zone"
	if z.ZoneType == "allow" {
		zoneLabel = "allow zone"
	}
	if z.ZoneAuthority == "guardian" {
		zoneLabel += " (guardian)"
	}
	readLabel := ""
	if z.PreventRead {
		readLabel = " (read+write)"
	}
	fmt.Printf("added %s%s: %s\n", zoneLabel, readLabel, z.Pattern)
	return nil
}
