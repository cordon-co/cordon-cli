package store

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"time"
)

// Zone is a protected file, folder, or glob pattern stored in policy.db.
type Zone struct {
	ID        string
	Pattern   string
	ZoneType  string // "standard" or "guardian"
	CreatedBy string
	CreatedAt string // ISO 8601
	UpdatedAt string // ISO 8601
}

// AddZone inserts a new zone into the policy database.
// Returns an error if the pattern already exists (UNIQUE constraint violation).
func AddZone(db *sql.DB, pattern, zoneType, createdBy string) (*Zone, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	id, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("store: generate zone id: %w", err)
	}

	z := Zone{
		ID:        id,
		Pattern:   pattern,
		ZoneType:  zoneType,
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = db.Exec(
		`INSERT INTO zones (id, pattern, zone_type, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		z.ID, z.Pattern, z.ZoneType, z.CreatedBy, z.CreatedAt, z.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("store: add zone: %w", err)
	}
	return &z, nil
}

// ListZones returns all zones ordered by creation time.
func ListZones(db *sql.DB) ([]Zone, error) {
	rows, err := db.Query(
		`SELECT id, pattern, zone_type, created_by, created_at, updated_at
		 FROM zones ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list zones: %w", err)
	}
	defer rows.Close()

	var zones []Zone
	for rows.Next() {
		var z Zone
		if err := rows.Scan(&z.ID, &z.Pattern, &z.ZoneType, &z.CreatedBy, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan zone: %w", err)
		}
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
// no zone matches. Matching uses exact, single-level glob, and directory-prefix
// strategies (see pathMatchesZone).
func ZoneForPath(db *sql.DB, filePath string) (*Zone, error) {
	zones, err := ListZones(db)
	if err != nil {
		return nil, err
	}
	for _, z := range zones {
		if pathMatchesZone(z.Pattern, filePath) {
			zCopy := z
			return &zCopy, nil
		}
	}
	return nil, nil
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
