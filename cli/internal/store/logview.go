package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"
)

// LogFilter controls which entries ListUnifiedLog returns.
//
// The category flags (Allow, Deny, Granted, Pass) are additive: when none are
// set every category is shown; when one or more are set only those categories
// are included.
type LogFilter struct {
	File    string    // substring match on file_path; empty = no filter
	Since   time.Time // zero = no filter
	Until   time.Time // zero = no filter; exclusive upper bound
	Agent   string    // exact match on agent; empty = no filter
	Allow   bool      // include hook_allow events
	Deny    bool      // include hook_deny events
	Granted bool      // include hook events authorized by a pass (non-empty pass_id)
	Pass    bool      // include pass lifecycle events (pass_issue, pass_revoke, pass_expire)
}

// hasCategories reports whether any category flag is set.
func (f LogFilter) hasCategories() bool {
	return f.Allow || f.Deny || f.Granted || f.Pass
}

// wantHookLog reports whether the filter includes any hook_log categories.
func (f LogFilter) wantHookLog() bool {
	return !f.hasCategories() || f.Allow || f.Deny || f.Granted
}

// wantAuditLog reports whether the filter includes any audit_log categories.
func (f LogFilter) wantAuditLog() bool {
	return !f.hasCategories() || f.Pass
}

// UnifiedEntry is a normalised view of a row from either hook_log or audit_log.
type UnifiedEntry struct {
	Time       time.Time `json:"time"`
	EventType  string    `json:"event_type"` // "hook_allow", "hook_deny", "file_add", …
	ToolName   string    `json:"tool_name,omitempty"`
	FilePath   string    `json:"file_path,omitempty"`
	Command    string    `json:"command,omitempty"` // Bash command string (from tool_input)
	FileRuleID string    `json:"file_rule_id,omitempty"`
	PassID     string    `json:"pass_id,omitempty"`
	User       string    `json:"user,omitempty"`
	Agent      string    `json:"agent,omitempty"`
	Detail     string    `json:"detail,omitempty"`
	SessionID  string    `json:"session_id,omitempty"`
}

// ListUnifiedLog queries hook_log and audit_log from the data database, merges
// the results, applies category and time filters, and returns them sorted
// newest-first.
func ListUnifiedLog(db *sql.DB, f LogFilter) ([]UnifiedEntry, error) {
	var hookEntries []UnifiedEntry
	if f.wantHookLog() {
		var err error
		hookEntries, err = queryHookLog(db, f)
		if err != nil {
			return nil, err
		}
	}

	var auditEntries []UnifiedEntry
	if f.wantAuditLog() {
		var err error
		auditEntries, err = queryAuditLog(db, f)
		if err != nil {
			return nil, err
		}
	}

	entries := append(hookEntries, auditEntries...)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Time.After(entries[j].Time)
	})
	return entries, nil
}

func queryHookLog(db *sql.DB, f LogFilter) ([]UnifiedEntry, error) {
	q := `SELECT ts, tool_name, file_path, decision, os_user, agent, pass_id, session_id,
	             COALESCE(command_raw, json_extract(tool_input, '$.command'), '') FROM hook_log WHERE 1=1`
	var args []any

	if f.File != "" {
		q += ` AND file_path LIKE ?`
		args = append(args, "%"+f.File+"%")
	}
	if f.hasCategories() {
		// Build an OR of the requested hook_log categories.
		var conds []string
		if f.Allow {
			conds = append(conds, "decision = 'allow'")
		}
		if f.Deny {
			conds = append(conds, "decision = 'deny'")
		}
		if f.Granted {
			conds = append(conds, "pass_id != ''")
		}
		if len(conds) > 0 {
			q += ` AND (` + strings.Join(conds, " OR ") + `)`
		}
	}
	if !f.Since.IsZero() {
		q += ` AND ts >= ?`
		args = append(args, f.Since.UnixMicro())
	}
	if !f.Until.IsZero() {
		q += ` AND ts < ?`
		args = append(args, f.Until.UnixMicro())
	}
	if f.Agent != "" {
		q += ` AND agent = ?`
		args = append(args, f.Agent)
	}
	q += ` ORDER BY ts DESC`

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query hook_log: %w", err)
	}
	defer rows.Close()

	var result []UnifiedEntry
	for rows.Next() {
		var ts int64
		var toolName, filePath, decision, osUser, agent, passID, sessionID, command string
		if err := rows.Scan(&ts, &toolName, &filePath, &decision, &osUser, &agent, &passID, &sessionID, &command); err != nil {
			return nil, fmt.Errorf("store: scan hook_log: %w", err)
		}
		eventType := "hook_allow"
		if decision == "deny" {
			eventType = "hook_deny"
		}
		result = append(result, UnifiedEntry{
			Time:      time.UnixMicro(ts),
			EventType: eventType,
			ToolName:  toolName,
			FilePath:  filePath,
			Command:   command,
			User:      osUser,
			Agent:     agent,
			PassID:    passID,
			SessionID: sessionID,
		})
	}
	return result, rows.Err()
}

func queryAuditLog(db *sql.DB, f LogFilter) ([]UnifiedEntry, error) {
	q := `SELECT event_type, tool_name, file_path, file_rule_id, pass_id, user, agent, detail, timestamp
	      FROM audit_log WHERE 1=1`
	var args []any

	if f.File != "" {
		q += ` AND file_path LIKE ?`
		args = append(args, "%"+f.File+"%")
	}
	if !f.Since.IsZero() {
		q += ` AND timestamp >= ?`
		args = append(args, f.Since.UTC().Format(time.RFC3339))
	}
	if !f.Until.IsZero() {
		q += ` AND timestamp < ?`
		args = append(args, f.Until.UTC().Format(time.RFC3339))
	}
	if f.Agent != "" {
		q += ` AND agent = ?`
		args = append(args, f.Agent)
	}
	if f.Pass {
		q += ` AND event_type IN ('pass_issue','pass_revoke','pass_expire')`
	}
	q += ` ORDER BY timestamp DESC`

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: query audit_log: %w", err)
	}
	defer rows.Close()

	var result []UnifiedEntry
	for rows.Next() {
		var e UnifiedEntry
		var ts string
		if err := rows.Scan(&e.EventType, &e.ToolName, &e.FilePath, &e.FileRuleID, &e.PassID,
			&e.User, &e.Agent, &e.Detail, &ts); err != nil {
			return nil, fmt.Errorf("store: scan audit_log: %w", err)
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			t = time.Time{}
		}
		e.Time = t
		result = append(result, e)
	}
	return result, rows.Err()
}
