package store

import (
	"database/sql"
	"os/user"
)

// HookLogEntry is a single row written to the hook_log table.
type HookLogEntry struct {
	Ts        int64  // Unix microseconds
	ToolName  string
	FilePath  string
	ToolInput string // raw JSON of the tool_input field
	Decision  string // "allow" or "deny"
	OSUser    string
	Agent     string
}

// InsertHookLog appends a hook invocation to the audit log.
func InsertHookLog(db *sql.DB, e HookLogEntry) error {
	_, err := db.Exec(
		`INSERT INTO hook_log (ts, tool_name, file_path, tool_input, decision, os_user, agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.Ts, e.ToolName, e.FilePath, e.ToolInput, e.Decision, e.OSUser, e.Agent,
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
