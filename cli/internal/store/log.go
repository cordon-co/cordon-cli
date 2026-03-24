package store

import (
	"database/sql"
	"fmt"
	"os/user"
)

// HookLogEntry is a single row written to the hook_log table.
type HookLogEntry struct {
	Ts         int64  // Unix microseconds
	ToolName   string
	FilePath   string
	ToolInput  string // raw JSON of the tool_input field
	Decision   string // "allow" or "deny"
	OSUser     string
	Agent      string
	PassID     string
	Notify     bool   // rule had notification flags
	ParentHash string // hash of previous hook_log entry
	Hash       string // SHA-256 hash for tamper evidence
}

// InsertHookLog appends a hook invocation to the audit log.
// It computes the hash chain automatically from the previous entry.
// Note: tool_input is excluded from the hash computation (see spec §14.4).
func InsertHookLog(db *sql.DB, e HookLogEntry) error {
	// Read the hash of the most recent entry for chain linkage.
	var parentHash string
	err := db.QueryRow("SELECT hash FROM hook_log ORDER BY id DESC LIMIT 1").Scan(&parentHash)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("store: read hook_log parent hash: %w", err)
	}

	e.ParentHash = parentHash
	e.Hash = computeDataHash(
		fmt.Sprintf("%d", e.Ts), e.ToolName, e.FilePath,
		e.Decision, e.OSUser, e.Agent, parentHash,
	)

	var notify int
	if e.Notify {
		notify = 1
	}

	_, err = db.Exec(
		`INSERT INTO hook_log (ts, tool_name, file_path, tool_input, decision, os_user, agent, pass_id, notify, parent_hash, hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.Ts, e.ToolName, e.FilePath, e.ToolInput, e.Decision, e.OSUser, e.Agent, e.PassID,
		notify, e.ParentHash, e.Hash,
	)
	return err
}

// CurrentOSUser returns the current OS username, or an empty string if it
// cannot be determined.
func CurrentOSUser() string {
	u, err := user.Current()
	if err != nil {
		return ""
	}
	return u.Username
}
