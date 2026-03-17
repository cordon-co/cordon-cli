package store

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Zone is a protected file, folder, or glob pattern stored in policy.db.
type Zone struct {
	ID           string
	Pattern      string
	ZoneType     string // "standard" or "guardian"
	PreventWrite bool   // always true for now
	PreventRead  bool   // opt-in via --prevent-read
	CreatedBy    string
	CreatedAt    string // ISO 8601
	UpdatedAt    string // ISO 8601
}

// AddZone inserts a new zone into the policy database.
// preventRead enables read enforcement in addition to the always-on write enforcement.
// Returns an error if the pattern already exists (UNIQUE constraint violation).
func AddZone(db *sql.DB, pattern, zoneType, createdBy string, preventRead bool) (*Zone, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("store: generate zone id: %w", err)
	}

	z := Zone{
		ID:           id,
		Pattern:      pattern,
		ZoneType:     zoneType,
		PreventWrite: true,
		PreventRead:  preventRead,
		CreatedBy:    createdBy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	pw := 1
	pr := 0
	if preventRead {
		pr = 1
	}

	_, err = db.Exec(
		`INSERT INTO zones (id, pattern, zone_type, prevent_write, prevent_read, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		z.ID, z.Pattern, z.ZoneType, pw, pr, z.CreatedBy, z.CreatedAt, z.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: add zone: %w", err)
	}
	return &z, nil
}

// ListZones returns all zones ordered by creation time.
func ListZones(db *sql.DB) ([]Zone, error) {
	rows, err := db.Query(
		`SELECT id, pattern, zone_type, prevent_write, prevent_read, created_by, created_at, updated_at
		 FROM zones ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list zones: %w", err)
	}
	defer rows.Close()

	var zones []Zone
	for rows.Next() {
		var z Zone
		var pw, pr int
		if err := rows.Scan(&z.ID, &z.Pattern, &z.ZoneType, &pw, &pr, &z.CreatedBy, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan zone: %w", err)
		}
		z.PreventWrite = pw != 0
		z.PreventRead = pr != 0
		zones = append(zones, z)
	}
	return zones, rows.Err()
}

// RemoveZone deletes the zone with the given pattern.
// Returns (true, nil) if a zone was removed, (false, nil) if no matching zone exists.
func RemoveZone(db *sql.DB, pattern string) (bool, error) {
	res, err := db.Exec(`DELETE FROM zones WHERE pattern = ?`, pattern)
	if err != nil {
		return false, fmt.Errorf("store: remove zone: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: remove zone rows affected: %w", err)
	}
	return n > 0, nil
}

// ZoneForPath returns the first zone whose pattern covers filePath, or nil if
// no zone matches.
//
// repoRoot is the absolute repo root path. It is used to convert absolute
// filePaths to repo-relative paths before matching, so that relative patterns
// like "*.gitignore" or "src/" match absolute paths received from the hook.
// Pass an empty string to skip relative-path matching.
func ZoneForPath(db *sql.DB, filePath, repoRoot string) (*Zone, error) {
	zones, err := ListZones(db)
	if err != nil {
		return nil, err
	}
	for _, z := range zones {
		if pathMatchesZone(z.Pattern, filePath, repoRoot) {
			zCopy := z
			return &zCopy, nil
		}
	}
	return nil, nil
}

// NormalizePattern converts an absolute path pattern to a repo-relative path.
// Glob patterns and already-relative patterns are returned unchanged.
// If the absolute path is outside the repo root, it is returned as-is.
func NormalizePattern(pattern, repoRoot string) string {
	if !filepath.IsAbs(pattern) {
		return pattern
	}
	if repoRoot == "" {
		return pattern
	}
	rel, err := filepath.Rel(repoRoot, pattern)
	if err != nil || strings.HasPrefix(rel, "..") {
		return pattern // outside repo — keep absolute
	}
	return rel
}

// StandardGuardrailZones is the default set of guardrail zones offered during
// `cordon init`. All are seeded with prevent_read=true so agents cannot read
// credential files into their context. They are stored as normal zones in
// policy.db, so they appear in `cordon zone list` and can be removed if desired.
var StandardGuardrailZones = []struct {
	Pattern     string
	PreventRead bool
}{
	// Environment / secrets files
	{".env", true},
	{".env.*", true},
	{".envrc", true},
	// Cloud / service credentials
	{"credentials.json", true},
	{"secrets.json", true},
	{"service-account.json", true},
	// Certificates and private keys
	{"*.pem", true},
	{"*.key", true},
	{"*.p12", true},
	{"*.pfx", true},
}

// newUUID generates a random UUID v4 string without external dependencies.
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
