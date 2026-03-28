package store

import (
	"database/sql"
	"strings"
)

// MigratePolicyDB creates all tables and indexes in the policy database if
// they do not exist. Safe to call on every open — all statements are idempotent.
func MigratePolicyDB(db *sql.DB) error {
	stmts := []string{
		// file_rules — one row per protected file, folder, or glob pattern.
		//
		// id:             UUID v4 (hex string).
		// pattern:        file path, directory path, or glob pattern.
		// file_access:    'deny' (blocks access) or 'allow' (permits access, overrides deny rules).
		// file_authority: 'standard' (any member) or 'elevated' (elevated/admin only).
		// prevent_write:  1 = block agent write tools (always true for now).
		// prevent_read:   1 = also block agent read tools (opt-in via --prevent-read).
		// created_by:     user identifier (github username or OS username for local users).
		// created_at:     ISO 8601 timestamp.
		// updated_at:     ISO 8601 timestamp.
		`CREATE TABLE IF NOT EXISTS file_rules (
			id             TEXT    PRIMARY KEY,
			pattern        TEXT    NOT NULL,
			file_access    TEXT    NOT NULL DEFAULT 'deny' CHECK(file_access IN ('allow','deny')),
			file_authority TEXT    NOT NULL DEFAULT 'standard' CHECK(file_authority IN ('standard','elevated')),
			prevent_write  INTEGER NOT NULL DEFAULT 1,
			prevent_read   INTEGER NOT NULL DEFAULT 0,
			created_by     TEXT    NOT NULL DEFAULT '',
			created_at     TEXT    NOT NULL,
			updated_at     TEXT    NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_file_rules_pattern ON file_rules(pattern)`,

		// command_rules — one row per command rule pattern.
		//
		// pattern:        glob-style pattern matched against the full bash command string.
		// rule_access:    'deny' (blocks command) or 'allow' (permits command, overrides deny rules).
		// rule_authority: 'standard' (any member) or 'elevated' (elevated/admin only).
		// created_by:     user identifier.
		// created_at / updated_at: ISO 8601 timestamps.
		`CREATE TABLE IF NOT EXISTS command_rules (
			id             TEXT PRIMARY KEY,
			pattern        TEXT NOT NULL,
			rule_access    TEXT NOT NULL DEFAULT 'deny' CHECK(rule_access IN ('allow','deny')),
			rule_authority TEXT NOT NULL DEFAULT 'standard' CHECK(rule_authority IN ('standard','elevated')),
			created_by     TEXT NOT NULL DEFAULT '',
			created_at     TEXT NOT NULL,
			updated_at     TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_command_rules_pattern ON command_rules(pattern)`,

		// perimeter_meta — singleton table storing project-level metadata.
		//
		// perimeter_id: UUID v4 identifying this cordon project. Used to locate the
		//               corresponding data.db under ~/.cordon/repos/<perimeter_id>/.
		//               Decouples data storage from the filesystem path and git.
		`CREATE TABLE IF NOT EXISTS perimeter_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,

		// policy_events — immutable, append-only log of every policy mutation.
		// The existing file_rules and command_rules tables are projections rebuilt
		// from this event log. The hash chain provides tamper detection and
		// deterministic replay for sync.
		`CREATE TABLE IF NOT EXISTS policy_events (
			seq            INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id       TEXT    NOT NULL UNIQUE,
			event_type     TEXT    NOT NULL,
			payload        TEXT    NOT NULL,
			actor          TEXT    NOT NULL,
			timestamp      TEXT    NOT NULL,
			parent_hash    TEXT    NOT NULL DEFAULT '',
			hash           TEXT    NOT NULL,
			server_seq     INTEGER
		)`,
		`CREATE INDEX IF NOT EXISTS idx_policy_events_server_seq ON policy_events(server_seq)`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	// Migrate existing rules to policy events if the event log is empty but
	// rules already exist (pre-event-sourcing databases).
	if err := migrateExistingRulesToEvents(db); err != nil {
		return err
	}

	// Additive column migrations for notification flag on policy tables.
	policyAlterStmts := []string{
		`ALTER TABLE file_rules ADD COLUMN notify INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE command_rules ADD COLUMN notify INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range policyAlterStmts {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumn(err) {
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
			agent      TEXT    NOT NULL DEFAULT '',
			pass_id    TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS hook_log_ts        ON hook_log(ts)`,
		`CREATE INDEX IF NOT EXISTS hook_log_file_path ON hook_log(file_path)`,
		`CREATE INDEX IF NOT EXISTS hook_log_decision  ON hook_log(decision)`,

		// passes — one row per issued access pass.
		//
		// id:               UUID v4.
		// file_rule_id:     references the file rule this pass grants access to.
		// pattern:          the file rule pattern at time of issuance (denormalized for audit).
		// file_path:        specific file if pass is file-scoped; empty string if rule-wide.
		// issued_to:        user identifier of pass recipient.
		// issued_by:        user identifier of pass issuer (self or elevated).
		// status:           'active', 'expired', or 'revoked'.
		// duration_minutes: NULL for indefinite passes.
		// issued_at:        ISO 8601 timestamp.
		// expires_at:       ISO 8601 timestamp; empty string for indefinite.
		// revoked_at:       ISO 8601 timestamp; empty string if not revoked.
		// revoked_by:       user identifier; empty string if not revoked.
		`CREATE TABLE IF NOT EXISTS passes (
			id               TEXT    PRIMARY KEY,
			file_rule_id     TEXT    NOT NULL,
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

		// audit_log — structured event log for file rule, pass, and integrity events.
		//
		// event_type: 'hook_allow', 'hook_deny', 'file_add', 'file_remove',
		//             'pass_issue', 'pass_revoke', 'pass_expire', 'integrity_check'.
		// tool_name:  agent tool name for hook events; empty otherwise.
		// file_path:  file involved, if applicable.
		// file_rule_id: file rule involved, if applicable.
		// pass_id:    pass involved, if applicable.
		// user:       user identifier performing the action.
		// agent:      agent platform identifier for hook events; empty otherwise.
		// detail:     additional context (deny reason, command string, etc.).
		// timestamp:  ISO 8601 timestamp.
		`CREATE TABLE IF NOT EXISTS audit_log (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type   TEXT NOT NULL,
			tool_name    TEXT NOT NULL DEFAULT '',
			file_path    TEXT NOT NULL DEFAULT '',
			file_rule_id TEXT NOT NULL DEFAULT '',
			pass_id      TEXT NOT NULL DEFAULT '',
			user         TEXT NOT NULL DEFAULT '',
			agent        TEXT NOT NULL DEFAULT '',
			detail       TEXT NOT NULL DEFAULT '',
			timestamp    TEXT NOT NULL
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

	// sync_watermarks — tracks the last-synced row ID for each data table.
	// Used by cordon sync to push only new rows since the last sync.
	syncStmts := []string{
		`CREATE TABLE IF NOT EXISTS sync_watermarks (
			table_name TEXT PRIMARY KEY,
			last_id    INTEGER NOT NULL DEFAULT 0
		)`,
	}
	for _, stmt := range syncStmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	// Additive column migrations for existing databases.
	// ALTER TABLE … ADD COLUMN is a no-op error when the column already exists;
	// we ignore that specific error ("duplicate column name").
	alterStmts := []string{
		`ALTER TABLE hook_log ADD COLUMN pass_id TEXT NOT NULL DEFAULT ''`,
		// Hash chain columns for tamper evidence.
		`ALTER TABLE hook_log ADD COLUMN notify INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE hook_log ADD COLUMN parent_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hook_log ADD COLUMN hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE audit_log ADD COLUMN parent_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE audit_log ADD COLUMN hash TEXT NOT NULL DEFAULT ''`,
		// Session tracking columns for transcript extraction.
		`ALTER TABLE hook_log ADD COLUMN session_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE hook_log ADD COLUMN transcript_path TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range alterStmts {
		if _, err := db.Exec(stmt); err != nil && !isDuplicateColumn(err) {
			return err
		}
	}

	// Additional indexes for migrated columns.
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS hook_log_session_id ON hook_log(session_id)`); err != nil {
		return err
	}

	return nil
}

// isDuplicateColumn returns true if the error is SQLite's "duplicate column name" error,
// which occurs when ALTER TABLE ADD COLUMN is run for a column that already exists.
func isDuplicateColumn(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}
