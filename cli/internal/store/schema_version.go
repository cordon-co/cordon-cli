package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/buildinfo"
)

const (
	currentPolicySchemaVersion = 1
	currentDataSchemaVersion   = 1
)

func prepareDBMigration(db *sql.DB, dbKind string, targetSchema int) error {
	if err := ensureSchemaMetaTable(db); err != nil {
		return err
	}

	current, err := readSchemaVersion(db)
	if err != nil {
		return err
	}
	if current > targetSchema {
		return fmt.Errorf("%s database schema version %d is newer than this cordon build supports (max %d); install a newer cordon version", dbKind, current, targetSchema)
	}

	if current < targetSchema {
		if err := backupBeforeMigration(db, dbKind, targetSchema); err != nil {
			return err
		}
	}
	return nil
}

func finalizeDBMigration(db *sql.DB, targetSchema int) error {
	if err := writeSchemaMeta(db, "schema_version", strconv.Itoa(targetSchema)); err != nil {
		return err
	}
	if err := writeSchemaMeta(db, "migrated_by", buildinfo.Version); err != nil {
		return err
	}
	return writeSchemaMeta(db, "migrated_at", time.Now().UTC().Format(time.RFC3339))
}

func ensureSchemaMetaTable(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_meta (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("store: create schema_meta: %w", err)
	}
	return nil
}

func writeSchemaMeta(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO schema_meta (key, value) VALUES (?, ?)`, key, value)
	if err != nil {
		return fmt.Errorf("store: write schema_meta[%s]: %w", key, err)
	}
	return nil
}

func readSchemaVersion(db *sql.DB) (int, error) {
	exists, err := tableExists(db, "schema_meta")
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}

	var raw string
	err = db.QueryRow(`SELECT value FROM schema_meta WHERE key = 'schema_version'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: read schema version: %w", err)
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("store: parse schema version %q: %w", raw, err)
	}
	return v, nil
}

func tableExists(db *sql.DB, name string) (bool, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil {
		return false, fmt.Errorf("store: check table %s: %w", name, err)
	}
	return count > 0, nil
}

func hasMigratableContent(db *sql.DB) (bool, error) {
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master
		 WHERE type = 'table'
		   AND name NOT LIKE 'sqlite_%'
		   AND name <> 'schema_meta'`,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("store: check migratable content: %w", err)
	}
	return count > 0, nil
}

func backupBeforeMigration(db *sql.DB, dbKind string, targetSchema int) error {
	hasData, err := hasMigratableContent(db)
	if err != nil {
		return err
	}
	if !hasData {
		return nil
	}

	mainPath, err := mainDBPath(db)
	if err != nil {
		return err
	}
	if mainPath == "" || mainPath == ":memory:" {
		return nil
	}

	backupPath, err := migrationBackupPath(db, dbKind, mainPath)
	if err != nil {
		return err
	}

	// Preserve the first pre-upgrade snapshot for a given target schema.
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("store: stat backup path %s: %w", backupPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return fmt.Errorf("store: create backup directory: %w", err)
	}

	// VACUUM INTO creates a consistent snapshot even for WAL databases.
	if _, err := db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapeSQLiteString(backupPath))); err != nil {
		return fmt.Errorf("store: backup %s before schema migration to %d: %w", dbKind, targetSchema, err)
	}
	return nil
}

func migrationBackupPath(db *sql.DB, dbKind, dbPath string) (string, error) {
	base := filepath.Base(dbPath)
	if dbKind == "policy" {
		perimeterID, err := readPerimeterIDFromPolicyDB(db)
		if err == nil && perimeterID != "" {
			homeDir, homeErr := os.UserHomeDir()
			if homeErr != nil {
				return "", fmt.Errorf("store: resolve home directory for backup: %w", homeErr)
			}
			return filepath.Join(homeDir, ".cordon", "repos", perimeterID, fmt.Sprintf("%s.%s.install.backup", base, buildinfo.Version)), nil
		}
	}
	return filepath.Join(filepath.Dir(dbPath), fmt.Sprintf("%s.%s.install.backup", base, buildinfo.Version)), nil
}

func readPerimeterIDFromPolicyDB(db *sql.DB) (string, error) {
	exists, err := tableExists(db, "perimeter_meta")
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	var id string
	err = db.QueryRow(`SELECT value FROM perimeter_meta WHERE key = 'perimeter_id'`).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("store: read perimeter_id for backup: %w", err)
	}
	return id, nil
}

func mainDBPath(db *sql.DB) (string, error) {
	rows, err := db.Query(`PRAGMA database_list`)
	if err != nil {
		return "", fmt.Errorf("store: query database_list: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var seq int
		var name, file string
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return "", fmt.Errorf("store: scan database_list: %w", err)
		}
		if name == "main" {
			return file, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("store: iterate database_list: %w", err)
	}
	return "", nil
}

func escapeSQLiteString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
