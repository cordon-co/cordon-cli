// Package store manages the two SQLite databases used by cordon.
//
// Policy database (.cordon/policy.db in the repo): stores file rule definitions.
// Data database (~/.cordon/repos/<perimeter_id>/data.db): stores audit logs,
// pass state, and demarcation history. Never committed to the repo.
package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: set pragmas on policy.db: %w", err)
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

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000;"); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: set pragmas on data.db: %w", err)
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

	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		return "", fmt.Errorf("set busy_timeout on policy.db: %w", err)
	}

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
// generates and stores a new one if none exists. The ID is derived
// deterministically from the git repository's root commit hash so that
// the same repo (even cloned separately) maps to the same data directory.
// Falls back to a random UUID for non-git directories.
func EnsurePerimeterID(db *sql.DB, absRepoRoot string) (string, error) {
	id, err := GetPerimeterID(db)
	if err == nil {
		return id, nil
	}

	id, err = deriveRepoID(absRepoRoot)
	if err != nil {
		// Non-git directory — fall back to random UUID.
		id, err = newUUID()
		if err != nil {
			return "", fmt.Errorf("store: generate perimeter id: %w", err)
		}
	}

	_, err = db.Exec(`INSERT OR IGNORE INTO perimeter_meta (key, value) VALUES ('perimeter_id', ?)`, id)
	if err != nil {
		return "", fmt.Errorf("store: write perimeter id: %w", err)
	}
	return id, nil
}

// deriveRepoID produces a deterministic perimeter ID from the git
// repository's root commit (the first commit). The root commit hash is
// identical across all clones of a repository, making it a stable
// identifier. The result is a SHA-256-based hex string (first 32 chars).
func deriveRepoID(absRepoRoot string) (string, error) {
	cmd := exec.Command("git", "rev-list", "--max-parents=0", "HEAD")
	cmd.Dir = absRepoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git root commit: %w", err)
	}

	// rev-list may return multiple root commits (e.g. after a graft).
	// Use the first one, which is the earliest.
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "", fmt.Errorf("no root commit found")
	}
	rootCommit := lines[0]

	// Hash with a fixed prefix to namespace these IDs.
	h := sha256.Sum256([]byte("cordon:repo:" + rootCommit))
	return fmt.Sprintf("%x", h[:16]), nil
}

// SetInstalledAgents stores the list of installed agent IDs in perimeter_meta.
func SetInstalledAgents(db *sql.DB, agentIDs []string) error {
	value := strings.Join(agentIDs, ",")
	_, err := db.Exec(
		`INSERT OR REPLACE INTO perimeter_meta (key, value) VALUES ('installed_agents', ?)`,
		value,
	)
	if err != nil {
		return fmt.Errorf("store: write installed_agents: %w", err)
	}
	return nil
}

// GetInstalledAgents reads the list of installed agent IDs from perimeter_meta.
// Returns nil if not set (pre-registry installations).
func GetInstalledAgents(db *sql.DB) ([]string, error) {
	var value string
	err := db.QueryRow(`SELECT value FROM perimeter_meta WHERE key = 'installed_agents'`).Scan(&value)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: read installed_agents: %w", err)
	}
	if value == "" {
		return nil, nil
	}
	return strings.Split(value, ","), nil
}

// HasPerimeterID reports whether the policy database already contains a
// perimeter_id entry. Returns false on any error (including missing table).
func HasPerimeterID(dbPath string) bool {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return false
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.Exec("PRAGMA busy_timeout=5000;") //nolint: read-only, best-effort

	var id string
	err = db.QueryRow(`SELECT value FROM perimeter_meta WHERE key = 'perimeter_id'`).Scan(&id)
	return err == nil && id != ""
}
