package pass

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/cordon-co/cordon/internal/flags"
	"github.com/cordon-co/cordon/internal/reporoot"
	"github.com/cordon-co/cordon/internal/store"
	"github.com/spf13/cobra"
)

var revokeCmd = &cobra.Command{
	Use:   "revoke <pass-id>",
	Short: "Revoke an active pass",
	Args:  cobra.ExactArgs(1),
	RunE:  runPassRevoke,
}

type passRevokeResult struct {
	PassID  string `json:"pass_id"`
	Revoked bool   `json:"revoked"`
}

func runPassRevoke(cmd *cobra.Command, args []string) error {
	passID := args[0]

	root, warn, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("pass revoke: find repo root: %w", err)
	}
	if warn != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warn)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("pass revoke: resolve repo root: %w", err)
	}

	dataDB, err := store.OpenDataDB(absRoot)
	if err != nil {
		return fmt.Errorf("pass revoke: open data database: %w", err)
	}
	defer dataDB.Close()

	if err := store.MigrateDataDB(dataDB); err != nil {
		return fmt.Errorf("pass revoke: migrate data database: %w", err)
	}

	user := store.CurrentOSUser()

	revoked, err := store.RevokePass(dataDB, passID, user)
	if err != nil {
		return fmt.Errorf("pass revoke: %w", err)
	}

	if revoked {
		// Audit log.
		_ = store.InsertAudit(dataDB, store.AuditEntry{
			EventType: "pass_revoke",
			PassID:    passID,
			User:      user,
		})
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(passRevokeResult{PassID: passID, Revoked: revoked}, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	if revoked {
		fmt.Printf("revoked pass: %s\n", passID)
	} else {
		fmt.Printf("pass not found or already inactive: %s\n", passID)
	}
	return nil
}
