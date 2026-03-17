package pass

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var listAll bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List active passes",
	Args:  cobra.NoArgs,
	RunE:  runPassList,
}

func init() {
	listCmd.Flags().BoolVar(&listAll, "all", false, "Include expired and revoked passes")
}

type passListResult struct {
	Passes []store.Pass `json:"passes"`
}

func runPassList(cmd *cobra.Command, args []string) error {
	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("pass list: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("pass list: resolve repo root: %w", err)
	}

	dataDB, err := store.OpenDataDB(absRoot)
	if err != nil {
		return fmt.Errorf("pass list: open data database: %w", err)
	}
	defer dataDB.Close()

	if err := store.MigrateDataDB(dataDB); err != nil {
		return fmt.Errorf("pass list: migrate data database: %w", err)
	}

	// Auto-expire stale passes before listing.
	if _, err := store.ExpireStale(dataDB); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not expire stale passes: %v\n", err)
	}

	var passes []store.Pass
	if listAll {
		passes, err = store.ListAllPasses(dataDB)
	} else {
		passes, err = store.ListPasses(dataDB)
	}
	if err != nil {
		return fmt.Errorf("pass list: %w", err)
	}

	if flags.JSON {
		result := passListResult{Passes: passes}
		if result.Passes == nil {
			result.Passes = []store.Pass{}
		}
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if len(passes) == 0 {
		if listAll {
			fmt.Println("no passes")
		} else {
			fmt.Println("no active passes")
		}
		return nil
	}

	if listAll {
		var active, inactive []store.Pass
		for _, p := range passes {
			if p.Status == "active" {
				active = append(active, p)
			} else {
				inactive = append(inactive, p)
			}
		}
		printPassTable("ACTIVE", active)
		if len(inactive) > 0 {
			if len(active) > 0 {
				fmt.Println()
			}
			printPassTable("INACTIVE", inactive)
		}
	} else {
		printPassTable("ACTIVE", passes)
	}
	return nil
}

func printPassTable(header string, passes []store.Pass) {
	if len(passes) == 0 {
		return
	}
	fmt.Printf("=== %s ===\n", header)
	fmt.Printf("%-36s  %-40s  %-9s  %s\n", "ID", "FILE", "STATUS", "EXPIRES")
	fmt.Println(strings.Repeat("-", 100))
	for _, p := range passes {
		file := p.FilePath
		if file == "" {
			file = p.Pattern + " (zone-wide)"
		}
		if len(file) > 40 {
			file = "…" + file[len(file)-39:]
		}

		expires := "never"
		if p.ExpiresAt != "" {
			if t, err := time.Parse(time.RFC3339, p.ExpiresAt); err == nil {
				expires = t.Local().Format("2006-01-02 15:04")
			} else {
				expires = p.ExpiresAt
			}
		}

		fmt.Printf("%-36s  %-40s  %-9s  %s\n", p.ID, file, p.Status, expires)
	}
}
