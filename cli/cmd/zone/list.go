package zone

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active zones",
	Args:  cobra.NoArgs,
	RunE:  runZoneList,
}

type zoneListResult struct {
	Zones []store.Zone `json:"zones"`
}

func runZoneList(cmd *cobra.Command, args []string) error {
	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("zone list: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("zone list: resolve repo root: %w", err)
	}

	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return fmt.Errorf("zone list: open policy database: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return fmt.Errorf("zone list: migrate policy database: %w", err)
	}

	zones, err := store.ListZones(policyDB)
	if err != nil {
		return fmt.Errorf("zone list: %w", err)
	}

	if flags.JSON {
		result := zoneListResult{Zones: zones}
		if result.Zones == nil {
			result.Zones = []store.Zone{}
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(zones) == 0 {
		fmt.Println("no zones configured")
		return nil
	}

	// Human-readable table.
	fmt.Printf("%-50s  %-9s  %-12s  %-16s  %s\n", "PATTERN", "TYPE", "OPS", "CREATED BY", "CREATED AT")
	fmt.Println(strings.Repeat("-", 112))
	for _, z := range zones {
		createdAt := z.CreatedAt
		if len(createdAt) > 10 {
			createdAt = createdAt[:10] // show date portion only
		}
		createdBy := z.CreatedBy
		if createdBy == "" {
			createdBy = "local"
		}
		blocks := "write"
		if z.PreventRead || z.ZoneType == "allow" {
			blocks = "read+write"
		}
		fmt.Printf("%-50s  %-9s  %-12s  %-16s  %s\n", z.Pattern, z.ZoneType, blocks, createdBy, createdAt)
	}
	return nil
}
