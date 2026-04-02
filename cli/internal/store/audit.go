package store

import (
	"database/sql"
	"fmt"
	"time"
)

// AuditEntry is a single row written to the audit_log table.
type AuditEntry struct {
	ID         int64  // auto-increment primary key; populated by queries, ignored on insert
	EventType  string // 'hook_allow', 'hook_deny', 'file_add', 'file_remove',
	//                   'pass_issue', 'pass_revoke', 'pass_expire', 'integrity_check'
	ToolName   string // agent tool name for hook events; empty otherwise
	FilePath   string // file involved, if applicable
	FileRuleID string // file rule involved, if applicable
	PassID     string // pass involved, if applicable
	User       string // user performing the action
	Agent      string // agent platform identifier for hook events
	Detail     string // additional context (deny reason, etc.)
	Timestamp  string // ISO 8601; auto-set to now if empty
	ParentHash string // hash of previous audit_log entry
	Hash       string // SHA-256 hash for tamper evidence
}

// InsertAudit appends a structured event to the audit_log table.
// If e.Timestamp is empty, the current UTC time is used.
// The hash chain is computed automatically from the previous entry.
func InsertAudit(db *sql.DB, e AuditEntry) error {
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}

	// Read the hash of the most recent entry for chain linkage.
	var parentHash string
	err := db.QueryRow("SELECT hash FROM audit_log ORDER BY id DESC LIMIT 1").Scan(&parentHash)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("store: read audit_log parent hash: %w", err)
	}

	e.ParentHash = parentHash
	e.Hash = computeDataHash(
		e.EventType, e.FilePath, e.User, e.Agent,
		e.Detail, e.Timestamp, parentHash,
	)

	_, err = db.Exec(
		`INSERT INTO audit_log
		 (event_type, tool_name, file_path, file_rule_id, pass_id, user, agent, detail, timestamp, parent_hash, hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.EventType, e.ToolName, e.FilePath, e.FileRuleID, e.PassID,
		e.User, e.Agent, e.Detail, e.Timestamp, e.ParentHash, e.Hash,
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
		`SELECT event_type, tool_name, file_path, file_rule_id, pass_id, user, agent, detail, timestamp
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
			&e.EventType, &e.ToolName, &e.FilePath, &e.FileRuleID, &e.PassID,
			&e.User, &e.Agent, &e.Detail, &e.Timestamp,
		); err != nil {
			return nil, fmt.Errorf("store: scan audit entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
