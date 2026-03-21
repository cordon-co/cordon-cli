// Package store manages the two SQLite databases used by cordon.
//
// Policy database (.cordon/policy.db in the repo): stores file rule definitions.
// Data database (~/.cordon/repos/<perimeter_id>/data.db): stores audit logs,
// pass state, and demarcation history. Never committed to the repo.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers "sqlite" driver with database/sql
)

// OpenPolicyDB opens (or creates) the policy database at
// <absRepoRoot>/.cordon/policy.db with WAL journal mode enabled.
// The .cordon/ directory is created if it does not exist.
// absRepoRoot must be an absolute path.
func OpenPolicyDB(absRepoRoot string) (*sql.DB, error) {
	dir := filepath.Join(absRepoRoot, ".cordon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("store: create .cordon directory: %w", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(dir, "policy.db"))
	if err != nil {
		return nil, fmt.Errorf("store: open policy.db: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: set WAL mode on policy.db: %w", err)
	}

	return db, nil
}

// OpenDataDB opens (or creates) the data database at
// ~/.cordon/repos/<perimeter_id>/data.db with WAL journal mode enabled.
// The directory is created if it does not exist.
// absRepoRoot must be an absolute path — the perimeter_id is read from
// the policy database in <absRepoRoot>/.cordon/policy.db.
func OpenDataDB(absRepoRoot string) (*sql.DB, error) {
	dbPath, err := DataDBPath(absRepoRoot)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("store: create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("store: open data.db: %w", err)
	}

	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: set WAL mode on data.db: %w", err)
	}

	return db, nil
}

// DataDBPath returns the filesystem path of the data database for the given
// absolute repo root without opening it. It reads the perimeter_id from the
// policy database to determine the path.
func DataDBPath(absRepoRoot string) (string, error) {
	id, err := ReadPerimeterID(absRepoRoot)
	if err != nil {
		return "", fmt.Errorf("store: resolve perimeter id: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("store: resolve home directory: %w", err)
	}

	return filepath.Join(homeDir, ".cordon", "repos", id, "data.db"), nil
}

// DataDBPathFromID returns the filesystem path of the data database for a
// known perimeter ID without reading the policy database. Useful when the
// caller already has the perimeter ID (e.g. during init).
func DataDBPathFromID(perimeterID string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("store: resolve home directory: %w", err)
	}
	return filepath.Join(homeDir, ".cordon", "repos", perimeterID, "data.db"), nil
}

// ReadPerimeterID opens the policy database at absRepoRoot/.cordon/policy.db,
// reads the perimeter_id from the perimeter_meta table, and closes the
// database. Returns an error if no perimeter_id is set (run cordon init).
func ReadPerimeterID(absRepoRoot string) (string, error) {
	policyPath := filepath.Join(absRepoRoot, ".cordon", "policy.db")
	db, err := sql.Open("sqlite", policyPath)
	if err != nil {
		return "", fmt.Errorf("open policy.db: %w", err)
	}
	defer db.Close()

	return GetPerimeterID(db)
}

// GetPerimeterID reads the perimeter_id from an already-open policy database.
// Returns an error if no perimeter_id is set.
func GetPerimeterID(db *sql.DB) (string, error) {
	var id string
	err := db.QueryRow(`SELECT value FROM perimeter_meta WHERE key = 'perimeter_id'`).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("no perimeter_id found — run 'cordon init' to set up this project")
	}
	return id, nil
}

// EnsurePerimeterID reads the perimeter_id from the policy database, or
// generates and stores a new UUID v4 if none exists. Returns the ID.
func EnsurePerimeterID(db *sql.DB) (string, error) {
	id, err := GetPerimeterID(db)
	if err == nil {
		return id, nil
	}

	id, err = newUUID()
	if err != nil {
		return "", fmt.Errorf("store: generate perimeter id: %w", err)
	}

	_, err = db.Exec(`INSERT OR IGNORE INTO perimeter_meta (key, value) VALUES ('perimeter_id', ?)`, id)
	if err != nil {
		return "", fmt.Errorf("store: write perimeter id: %w", err)
	}
	return id, nil
}
