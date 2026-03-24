package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
)

// DeriveRemotePerimeterID normalizes a git remote URL and hashes it to produce
// a perimeter ID that matches what cordon-web computes when a repo is added
// via the dashboard.
//
// Normalization: strip git@host: → host/, strip https://, strip .git suffix,
// lowercase everything → github.com/org/repo.
// Hash: SHA-256("cordon:remote:" + normalized)[:32] hex.
func DeriveRemotePerimeterID(remoteURL string) (string, error) {
	normalized, err := NormalizeRemoteURL(remoteURL)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte("cordon:remote:" + normalized))
	return fmt.Sprintf("%x", h[:16]), nil
}

// NormalizeRemoteURL converts a git remote URL to canonical form:
// github.com/org/repo (lowercase, no protocol, no .git suffix).
func NormalizeRemoteURL(remoteURL string) (string, error) {
	raw := strings.TrimSpace(remoteURL)
	if raw == "" {
		return "", fmt.Errorf("empty remote URL")
	}

	var normalized string

	switch {
	case strings.HasPrefix(raw, "git@"):
		// git@github.com:org/repo.git → github.com/org/repo
		raw = strings.TrimPrefix(raw, "git@")
		normalized = strings.Replace(raw, ":", "/", 1)
	case strings.Contains(raw, "://"):
		// https://github.com/org/repo.git → github.com/org/repo
		parts := strings.SplitN(raw, "://", 2)
		if len(parts) != 2 {
			return "", fmt.Errorf("malformed remote URL: %s", remoteURL)
		}
		normalized = parts[1]
		// Strip optional user@ prefix (e.g. git://user@host/path)
		if idx := strings.Index(normalized, "@"); idx >= 0 && idx < strings.Index(normalized, "/") {
			normalized = normalized[idx+1:]
		}
	default:
		// Assume it's already in host/path form or some other format.
		normalized = raw
	}

	normalized = strings.TrimSuffix(normalized, ".git")
	normalized = strings.TrimSuffix(normalized, "/")
	normalized = strings.ToLower(normalized)

	return normalized, nil
}

// GetOriginRemoteURL runs `git remote get-url origin` and returns the URL.
// Returns empty string (no error) if no remote named "origin" exists.
func GetOriginRemoteURL(absRepoRoot string) (string, error) {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = absRepoRoot
	out, err := cmd.Output()
	if err != nil {
		// No origin remote — not an error, just no URL.
		return "", nil
	}
	return strings.TrimSpace(string(out)), nil
}

// MigratePerimeterID updates the perimeter_id from root-commit-based to
// remote-URL-based for authenticated users. This is a one-time migration
// that ensures the local perimeter ID matches what cordon-web computed.
//
// Only runs if:
// - User is authenticated (api.IsLoggedIn())
// - Origin remote exists
// - Current perimeter_id differs from remote-derived ID
//
// On migration, it updates perimeter_meta and moves the data.db directory.
func MigratePerimeterID(db *sql.DB, absRepoRoot string) error {
	if !api.IsLoggedIn() {
		return nil
	}

	remoteURL, err := GetOriginRemoteURL(absRepoRoot)
	if err != nil {
		return fmt.Errorf("migrate perimeter id: get origin URL: %w", err)
	}
	if remoteURL == "" {
		return nil // no remote = can't compute team ID
	}

	newID, err := DeriveRemotePerimeterID(remoteURL)
	if err != nil {
		return fmt.Errorf("migrate perimeter id: derive remote ID: %w", err)
	}

	currentID, err := GetPerimeterID(db)
	if err != nil {
		return fmt.Errorf("migrate perimeter id: read current ID: %w", err)
	}

	if currentID == newID {
		return nil // already migrated
	}

	// Update the perimeter_meta table.
	_, err = db.Exec(`UPDATE perimeter_meta SET value = ? WHERE key = 'perimeter_id'`, newID)
	if err != nil {
		return fmt.Errorf("migrate perimeter id: update perimeter_meta: %w", err)
	}

	// Move the data.db directory from old path to new path.
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("migrate perimeter id: resolve home: %w", err)
	}

	oldDir := filepath.Join(homeDir, ".cordon", "repos", currentID)
	newDir := filepath.Join(homeDir, ".cordon", "repos", newID)

	// Only move if old directory exists and new doesn't.
	if info, err := os.Stat(oldDir); err == nil && info.IsDir() {
		if _, err := os.Stat(newDir); os.IsNotExist(err) {
			if err := os.MkdirAll(filepath.Dir(newDir), 0o755); err != nil {
				return fmt.Errorf("migrate perimeter id: create parent dir: %w", err)
			}
			if err := os.Rename(oldDir, newDir); err != nil {
				return fmt.Errorf("migrate perimeter id: move data directory: %w", err)
			}
		}
	}

	return nil
}
