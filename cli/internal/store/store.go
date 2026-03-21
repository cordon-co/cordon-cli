// Package store manages the two SQLite databases used by cordon.
//
// Policy database (.cordon/policy.db in the repo): stores file rule definitions.
// Data database (~/.cordon/repos/<hash>/data.db): stores audit logs, pass
// state, and demarcation history. Never committed to the repo.
package store

import (
	"crypto/sha256"
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
// ~/.cordon/repos/<hash>/data.db with WAL journal mode enabled.
// The directory is created if it does not exist.
// absRepoRoot must be an absolute path — the hash is derived from it.
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
// absolute repo root without opening it. Useful for reporting paths to the
// user after init.
func DataDBPath(absRepoRoot string) (string, error) {
	sum := sha256.Sum256([]byte(absRepoRoot))
	hash := fmt.Sprintf("%x", sum)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("store: resolve home directory: %w", err)
	}

	return filepath.Join(homeDir, ".cordon", "repos", hash, "data.db"), nil
}
