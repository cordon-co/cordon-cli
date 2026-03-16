package store

import (
	"database/sql"
	"fmt"
	"time"
)

// AuditEntry is a single row written to the audit_log table.
type AuditEntry struct {
	EventType string // 'hook_allow', 'hook_deny', 'zone_add', 'zone_remove',
	//                   'pass_issue', 'pass_revoke', 'pass_expire', 'integrity_check'
	ToolName  string // agent tool name for hook events; empty otherwise
	FilePath  string // file involved, if applicable
	ZoneID    string // zone involved, if applicable
	PassID    string // pass involved, if applicable
	User      string // user performing the action
	Agent     string // agent platform identifier for hook events
	Detail    string // additional context (deny reason, etc.)
	Timestamp string // ISO 8601; auto-set to now if empty
}

// InsertAudit appends a structured event to the audit_log table.
// If e.Timestamp is empty, the current UTC time is used.
func InsertAudit(db *sql.DB, e AuditEntry) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := db.Exec(
		`INSERT INTO audit_log
		 (event_type, tool_name, file_path, zone_id, pass_id, user, agent, detail, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.EventType, e.ToolName, e.FilePath, e.ZoneID, e.PassID,
		e.User, e.Agent, e.Detail, e.Timestamp,
	)
	if err != nil {
		return fmt.Errorf("store: insert audit: %w", err)
	}
	return nil
}

// ListAudit returns all audit_log rows in descending timestamp order.
// It is used by 'cordon log'; more filtering options can be added later.
func ListAudit(db *sql.DB) ([]AuditEntry, error) {
	rows, err := db.Query(
		`SELECT event_type, tool_name, file_path, zone_id, pass_id, user, agent, detail, timestamp
		 FROM audit_log ORDER BY timestamp DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list audit: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(
			&e.EventType, &e.ToolName, &e.FilePath, &e.ZoneID, &e.PassID,
			&e.User, &e.Agent, &e.Detail, &e.Timestamp,
		); err != nil {
			return nil, fmt.Errorf("store: scan audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
