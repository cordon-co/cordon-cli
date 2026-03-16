package store

import "database/sql"

// MigratePolicyDB creates all tables and indexes in the policy database if
// they do not exist. Safe to call on every open — all statements are idempotent.
func MigratePolicyDB(db *sql.DB) error {
	stmts := []string{
		// zones — one row per protected file, folder, or glob pattern.
		//
		// id:         UUID v4 (hex string).
		// pattern:    file path, directory path, or glob pattern.
		// zone_type:  'standard' (any member) or 'guardian' (guardian/admin only).
		// created_by: user identifier (github username or OS username for local users).
		// created_at: ISO 8601 timestamp.
		// updated_at: ISO 8601 timestamp.
		`CREATE TABLE IF NOT EXISTS zones (
			id         TEXT PRIMARY KEY,
			pattern    TEXT NOT NULL,
			zone_type  TEXT NOT NULL DEFAULT 'standard' CHECK(zone_type IN ('standard','guardian')),
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_zones_pattern ON zones(pattern)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

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

		// passes — one row per issued access pass.
		//
		// id:               UUID v4.
		// zone_id:          references the zone this pass grants access to.
		// pattern:          the zone pattern at time of issuance (denormalized for audit).
		// file_path:        specific file if pass is file-scoped; empty string if zone-wide.
		// issued_to:        user identifier of pass recipient.
		// issued_by:        user identifier of pass issuer (self or guardian).
		// status:           'active', 'expired', or 'revoked'.
		// duration_minutes: NULL for indefinite passes.
		// issued_at:        ISO 8601 timestamp.
		// expires_at:       ISO 8601 timestamp; empty string for indefinite.
		// revoked_at:       ISO 8601 timestamp; empty string if not revoked.
		// revoked_by:       user identifier; empty string if not revoked.
		`CREATE TABLE IF NOT EXISTS passes (
			id               TEXT    PRIMARY KEY,
			zone_id          TEXT    NOT NULL,
			pattern          TEXT    NOT NULL,
			file_path        TEXT    NOT NULL DEFAULT '',
			issued_to        TEXT    NOT NULL,
			issued_by        TEXT    NOT NULL,
			status           TEXT    NOT NULL DEFAULT 'active' CHECK(status IN ('active','expired','revoked')),
			duration_minutes INTEGER,
			issued_at        TEXT    NOT NULL,
			expires_at       TEXT    NOT NULL DEFAULT '',
			revoked_at       TEXT    NOT NULL DEFAULT '',
			revoked_by       TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_passes_status  ON passes(status)`,
		`CREATE INDEX IF NOT EXISTS idx_passes_pattern ON passes(pattern)`,

		// audit_log — structured event log for zone, pass, and integrity events.
		//
		// event_type: 'hook_allow', 'hook_deny', 'zone_add', 'zone_remove',
		//             'pass_issue', 'pass_revoke', 'pass_expire', 'integrity_check'.
		// tool_name:  agent tool name for hook events; empty otherwise.
		// file_path:  file involved, if applicable.
		// zone_id:    zone involved, if applicable.
		// pass_id:    pass involved, if applicable.
		// user:       user identifier performing the action.
		// agent:      agent platform identifier for hook events; empty otherwise.
		// detail:     additional context (deny reason, command string, etc.).
		// timestamp:  ISO 8601 timestamp.
		`CREATE TABLE IF NOT EXISTS audit_log (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			tool_name  TEXT NOT NULL DEFAULT '',
			file_path  TEXT NOT NULL DEFAULT '',
			zone_id    TEXT NOT NULL DEFAULT '',
			pass_id    TEXT NOT NULL DEFAULT '',
			user       TEXT NOT NULL DEFAULT '',
			agent      TEXT NOT NULL DEFAULT '',
			detail     TEXT NOT NULL DEFAULT '',
			timestamp  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_timestamp  ON audit_log(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_event_type ON audit_log(event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_file_path  ON audit_log(file_path)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}
