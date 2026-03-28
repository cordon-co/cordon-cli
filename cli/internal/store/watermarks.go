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
// Pass limit <= 0 to return all matching rows.
func HookLogEntriesSince(db *sql.DB, afterID int64, limit int) ([]HookLogEntry, int64, error) {
	q := `SELECT id, ts, tool_name, file_path, tool_input, decision, os_user, agent, pass_id, notify, session_id, transcript_path, parent_hash, hash
		 FROM hook_log WHERE id > ? ORDER BY id ASC`
	var args []any
	args = append(args, afterID)
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := db.Query(q, args...)
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
			&e.Decision, &e.OSUser, &e.Agent, &e.PassID, &notify, &e.SessionID, &e.TranscriptPath, &e.ParentHash, &e.Hash); err != nil {
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
// Pass limit <= 0 to return all matching rows.
func AuditEntriesSince(db *sql.DB, afterID int64, limit int) ([]AuditEntry, int64, error) {
	q := `SELECT id, event_type, tool_name, file_path, file_rule_id, pass_id, user, agent, detail, timestamp, parent_hash, hash
		 FROM audit_log WHERE id > ? ORDER BY id ASC`
	var args []any
	args = append(args, afterID)
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := db.Query(q, args...)
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
// Pass limit <= 0 to return all matching rows.
func PassesSince(db *sql.DB, afterID int64, limit int) ([]Pass, int64, error) {
	q := `SELECT rowid, id, file_rule_id, pattern, file_path, issued_to, issued_by, status,
		        duration_minutes, issued_at, expires_at, revoked_at, revoked_by
		 FROM passes WHERE rowid > ? ORDER BY rowid ASC`
	var args []any
	args = append(args, afterID)
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := db.Query(q, args...)
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

// SessionsSince returns sessions with updated_at > afterUpdatedAt, ordered by updated_at ASC.
// The watermark is updated_at (Unix microseconds) rather than an autoincrement ID,
// so both new sessions and re-extracted sessions (with bumped updated_at) are captured.
// Pass limit <= 0 to return all matching rows.
func SessionsSince(db *sql.DB, afterUpdatedAt int64, limit int) ([]Session, int64, error) {
	q := `SELECT session_id, agent, description, transcript_path,
		        input_tokens, output_tokens, cache_read_tokens,
		        first_seen_at, last_seen_at, updated_at
		 FROM sessions WHERE updated_at > ? ORDER BY updated_at ASC`
	var args []any
	args = append(args, afterUpdatedAt)
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: sessions since %d: %w", afterUpdatedAt, err)
	}
	defer rows.Close()

	var sessions []Session
	var maxUpdatedAt int64
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.SessionID, &s.Agent, &s.Description, &s.TranscriptPath,
			&s.InputTokens, &s.OutputTokens, &s.CacheReadTokens,
			&s.FirstSeenAt, &s.LastSeenAt, &s.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("store: scan session: %w", err)
		}
		sessions = append(sessions, s)
		if s.UpdatedAt > maxUpdatedAt {
			maxUpdatedAt = s.UpdatedAt
		}
	}
	return sessions, maxUpdatedAt, rows.Err()
}
