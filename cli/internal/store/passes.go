package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// Pass is a temporary access grant stored in data.db.
type Pass struct {
	ID              string
	FileRuleID      string
	Pattern         string
	FilePath        string // empty string if rule-wide
	IssuedTo        string
	IssuedBy        string
	Status          string // "active", "expired", "revoked"
	DurationMinutes *int   // nil for indefinite
	IssuedAt        string // ISO 8601
	ExpiresAt       string // ISO 8601; empty string for indefinite
	RevokedAt       string // ISO 8601; empty string if not revoked
	RevokedBy       string // empty string if not revoked
}

// IssuePass inserts a new pass into the data database.
func IssuePass(db *sql.DB, p Pass) error {
	id, err := newUUID()
	if err != nil {
		return fmt.Errorf("store: generate pass id: %w", err)
	}
	p.ID = id

	_, err = db.Exec(
		`INSERT INTO passes
		 (id, file_rule_id, pattern, file_path, issued_to, issued_by, status,
		  duration_minutes, issued_at, expires_at, revoked_at, revoked_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.FileRuleID, p.Pattern, p.FilePath, p.IssuedTo, p.IssuedBy, p.Status,
		durationMinutesToSQL(p.DurationMinutes), p.IssuedAt, p.ExpiresAt,
		p.RevokedAt, p.RevokedBy,
	)
	if err != nil {
		return fmt.Errorf("store: issue pass: %w", err)
	}
	return nil
}

// ListPasses returns active passes ordered by issued_at descending (most recent first).
func ListPasses(db *sql.DB) ([]Pass, error) {
	return listPassesWhere(db, `WHERE status = 'active'`)
}

// ListAllPasses returns all passes (active, expired, revoked) ordered by issued_at descending.
func ListAllPasses(db *sql.DB) ([]Pass, error) {
	return listPassesWhere(db, "")
}

func listPassesWhere(db *sql.DB, where string) ([]Pass, error) {
	q := `SELECT id, file_rule_id, pattern, file_path, issued_to, issued_by, status,
	             duration_minutes, issued_at, expires_at, revoked_at, revoked_by
	      FROM passes ` + where + ` ORDER BY issued_at DESC`
	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("store: list passes: %w", err)
	}
	defer rows.Close()

	var passes []Pass
	for rows.Next() {
		p, err := scanPass(rows)
		if err != nil {
			return nil, err
		}
		passes = append(passes, p)
	}
	return passes, rows.Err()
}

// RevokePass sets a pass status to 'revoked' and records who revoked it.
// Returns (true, nil) if a pass was updated, (false, nil) if the pass was not
// found or was already in a terminal state (expired/revoked).
func RevokePass(db *sql.DB, passID, revokedBy string) (bool, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`UPDATE passes SET status='revoked', revoked_at=?, revoked_by=?
		 WHERE id=? AND status='active'`,
		now, revokedBy, passID,
	)
	if err != nil {
		return false, fmt.Errorf("store: revoke pass: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: revoke pass rows affected: %w", err)
	}
	return n > 0, nil
}

// ActivePassForPath returns the first active, non-expired pass that covers
// filePath, or nil if none exists.
//
// repoRoot is the absolute repo root used to relativize filePath before
// matching — consistent with how file rules are stored (relative patterns).
//
// A pass covers filePath if:
//   - The pass is file-scoped (file_path != '') and matches filePath (exact or
//     relative), OR
//   - The pass is rule-wide (file_path == '') and its file rule pattern covers filePath.
func ActivePassForPath(db *sql.DB, filePath, repoRoot string) (*Pass, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	rows, err := db.Query(
		`SELECT id, file_rule_id, pattern, file_path, issued_to, issued_by, status,
		        duration_minutes, issued_at, expires_at, revoked_at, revoked_by
		 FROM passes
		 WHERE status = 'active'
		   AND (expires_at = '' OR expires_at > ?)`,
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("store: active pass query: %w", err)
	}
	defer rows.Close()

	// Compute the repo-relative form of filePath once for reuse.
	relFilePath := relPath(filePath, repoRoot)

	for rows.Next() {
		p, err := scanPass(rows)
		if err != nil {
			return nil, err
		}
		if p.FilePath != "" {
			// File-scoped pass: match either the stored path or the relativized path.
			if p.FilePath == filePath || (relFilePath != "" && p.FilePath == relFilePath) {
				return &p, nil
			}
		} else {
			// Rule-wide pass: check whether the file rule pattern covers this file.
			if pathMatchesFileRule(p.Pattern, filePath, repoRoot) {
				return &p, nil
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: active pass scan: %w", err)
	}
	return nil, nil
}

// relPath returns the repo-relative form of an absolute path.
// Returns an empty string if the path is outside the repo or on error.
func relPath(absPath, repoRoot string) string {
	if repoRoot == "" || !filepath.IsAbs(absPath) {
		return ""
	}
	rel, err := filepath.Rel(repoRoot, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ""
	}
	return rel
}

// ExpireStale updates all passes whose expires_at has passed to status 'expired'.
// Returns the number of passes expired.
func ExpireStale(db *sql.DB) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := db.Exec(
		`UPDATE passes SET status='expired'
		 WHERE status='active' AND expires_at != '' AND expires_at <= ?`,
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("store: expire stale passes: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("store: expire stale passes rows affected: %w", err)
	}
	return n, nil
}

// scanPass reads a single Pass from a *sql.Rows cursor.
func scanPass(rows *sql.Rows) (Pass, error) {
	var p Pass
	var dur sql.NullInt64
	err := rows.Scan(
		&p.ID, &p.FileRuleID, &p.Pattern, &p.FilePath,
		&p.IssuedTo, &p.IssuedBy, &p.Status,
		&dur, &p.IssuedAt, &p.ExpiresAt, &p.RevokedAt, &p.RevokedBy,
	)
	if err != nil {
		return Pass{}, fmt.Errorf("store: scan pass: %w", err)
	}
	if dur.Valid {
		n := int(dur.Int64)
		p.DurationMinutes = &n
	}
	return p, nil
}

// durationMinutesToSQL converts a *int duration to a SQL-compatible value
// (nil becomes NULL in SQLite).
func durationMinutesToSQL(d *int) interface{} {
	if d == nil {
		return nil
	}
	return *d
}
