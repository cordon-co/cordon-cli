package store

import "database/sql"

// MigrateDataDB creates all tables and indexes in the data database if they
// do not exist. Safe to call on every open — all statements are idempotent.
func MigrateDataDB(db *sql.DB) error {
	stmts := []string{
		// hook_log — one row per PreToolUse hook invocation.
		//
		// ts:         Unix microseconds; precise enough for ordering, compact as INTEGER.
		// tool_name:  the agent tool that was intercepted (Write, Edit, MultiEdit, …).
		// file_path:  the primary path extracted from tool_input.
		// tool_input: full raw JSON of the tool_input field — preserves all details
		//             (content, old_string/new_string for edits, etc.) for audit replay.
		// decision:   "allow" or "deny".
		// os_user:    OS-level username of the developer running the agent.
		// agent:      agent platform identifier (e.g. "claude-code", "vscode"); empty
		//             until detection is implemented.
		`CREATE TABLE IF NOT EXISTS hook_log (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			ts         INTEGER NOT NULL,
			tool_name  TEXT    NOT NULL,
			file_path  TEXT    NOT NULL,
			tool_input TEXT    NOT NULL,
			decision   TEXT    NOT NULL CHECK(decision IN ('allow','deny')),
			os_user    TEXT    NOT NULL DEFAULT '',
			agent      TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS hook_log_ts        ON hook_log(ts)`,
		`CREATE INDEX IF NOT EXISTS hook_log_file_path ON hook_log(file_path)`,
		`CREATE INDEX IF NOT EXISTS hook_log_decision  ON hook_log(decision)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
