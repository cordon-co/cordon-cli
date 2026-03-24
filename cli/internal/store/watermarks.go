package store

import (
	"database/sql"
	"fmt"
)

// GetWatermark returns the last synced row ID for a given table.
// Returns 0 if no watermark has been set.
func GetWatermark(db *sql.DB, tableName string) (int64, error) {
	var lastID int64
	err := db.QueryRow(`SELECT last_id FROM sync_watermarks WHERE table_name = ?`, tableName).Scan(&lastID)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: get watermark for %s: %w", tableName, err)
	}
	return lastID, nil
}

// SetWatermark updates the sync watermark for a given table.
func SetWatermark(db *sql.DB, tableName string, lastID int64) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO sync_watermarks (table_name, last_id) VALUES (?, ?)`,
		tableName, lastID,
	)
	if err != nil {
		return fmt.Errorf("store: set watermark for %s: %w", tableName, err)
	}
	return nil
}

// MaxServerSeq returns the highest server_seq in the local policy_events table,
// or 0 if no events have been synced from the server yet.
func MaxServerSeq(db *sql.DB) (int64, error) {
	var seq sql.NullInt64
	err := db.QueryRow(`SELECT MAX(server_seq) FROM policy_events`).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("store: max server_seq: %w", err)
	}
	if !seq.Valid {
		return 0, nil
	}
	return seq.Int64, nil
}

// HookLogEntriesSince returns hook_log rows with id > afterID, ordered by id ASC.
func HookLogEntriesSince(db *sql.DB, afterID int64) ([]HookLogEntry, int64, error) {
	rows, err := db.Query(
		`SELECT id, ts, tool_name, file_path, tool_input, decision, os_user, agent, pass_id, notify, parent_hash, hash
		 FROM hook_log WHERE id > ? ORDER BY id ASC`, afterID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("store: hook_log since %d: %w", afterID, err)
	}
	defer rows.Close()

	var entries []HookLogEntry
	var maxID int64
	for rows.Next() {
		var e HookLogEntry
		var notify int
		if err := rows.Scan(&e.ID, &e.Ts, &e.ToolName, &e.FilePath, &e.ToolInput,
			&e.Decision, &e.OSUser, &e.Agent, &e.PassID, &notify, &e.ParentHash, &e.Hash); err != nil {
			return nil, 0, fmt.Errorf("store: scan hook_log entry: %w", err)
		}
		e.Notify = notify != 0
		entries = append(entries, e)
		if e.ID > maxID {
			maxID = e.ID
		}
	}
	return entries, maxID, rows.Err()
}

// AuditEntriesSince returns audit_log rows with id > afterID, ordered by id ASC.
func AuditEntriesSince(db *sql.DB, afterID int64) ([]AuditEntry, int64, error) {
	rows, err := db.Query(
		`SELECT id, event_type, tool_name, file_path, file_rule_id, pass_id, user, agent, detail, timestamp, parent_hash, hash
		 FROM audit_log WHERE id > ? ORDER BY id ASC`, afterID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("store: audit_log since %d: %w", afterID, err)
	}
	defer rows.Close()

	var entries []AuditEntry
	var maxID int64
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.EventType, &e.ToolName, &e.FilePath, &e.FileRuleID,
			&e.PassID, &e.User, &e.Agent, &e.Detail, &e.Timestamp, &e.ParentHash, &e.Hash); err != nil {
			return nil, 0, fmt.Errorf("store: scan audit_log entry: %w", err)
		}
		entries = append(entries, e)
		if e.ID > maxID {
			maxID = e.ID
		}
	}
	return entries, maxID, rows.Err()
}

// PassesSince returns passes rows with rowid > afterID, ordered by rowid ASC.
func PassesSince(db *sql.DB, afterID int64) ([]Pass, int64, error) {
	rows, err := db.Query(
		`SELECT rowid, id, file_rule_id, pattern, file_path, issued_to, issued_by, status,
		        duration_minutes, issued_at, expires_at, revoked_at, revoked_by
		 FROM passes WHERE rowid > ? ORDER BY rowid ASC`, afterID,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("store: passes since %d: %w", afterID, err)
	}
	defer rows.Close()

	var passes []Pass
	var maxID int64
	for rows.Next() {
		var p Pass
		var rowid int64
		if err := rows.Scan(&rowid, &p.ID, &p.FileRuleID, &p.Pattern, &p.FilePath,
			&p.IssuedTo, &p.IssuedBy, &p.Status, &p.DurationMinutes,
			&p.IssuedAt, &p.ExpiresAt, &p.RevokedAt, &p.RevokedBy); err != nil {
			return nil, 0, fmt.Errorf("store: scan pass: %w", err)
		}
		passes = append(passes, p)
		if rowid > maxID {
			maxID = rowid
		}
	}
	return passes, maxID, rows.Err()
}
