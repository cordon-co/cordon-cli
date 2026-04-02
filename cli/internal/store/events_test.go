package store

import (
	"database/sql"
	"encoding/json"
	"testing"
)

func TestAppendEvent(t *testing.T) {
	db := newTestPolicyDB(t)

	payload, _ := json.Marshal(map[string]interface{}{
		"id":             "rule-1",
		"pattern":        ".env",
		"file_access":    "deny",
		"file_authority": "standard",
		"prevent_write":  true,
		"prevent_read":   false,
		"created_by":     "test",
	})

	ev, err := AppendEvent(db, "file_rule.added", string(payload), "test")
	if err != nil {
		t.Fatal(err)
	}

	if ev.Seq == 0 {
		t.Error("expected seq > 0")
	}
	if ev.EventID == "" {
		t.Error("expected non-empty event_id")
	}

	// Verify the projection was updated.
	rules, err := ListFileRules(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 file rule, got %d", len(rules))
	}
	if rules[0].Pattern != ".env" {
		t.Errorf("pattern = %q, want .env", rules[0].Pattern)
	}
}

func TestAppendMultipleEvents(t *testing.T) {
	db := newTestPolicyDB(t)

	p1, _ := json.Marshal(map[string]interface{}{
		"id": "r1", "pattern": ".env", "file_access": "deny",
		"file_authority": "standard", "prevent_write": true,
		"prevent_read": false, "created_by": "test",
	})
	_, err := AppendEvent(db, "file_rule.added", string(p1), "test")
	if err != nil {
		t.Fatal(err)
	}

	p2, _ := json.Marshal(map[string]interface{}{
		"id": "r2", "pattern": "*.pem", "file_access": "deny",
		"file_authority": "standard", "prevent_write": true,
		"prevent_read": true, "created_by": "test",
	})
	_, err = AppendEvent(db, "file_rule.added", string(p2), "test")
	if err != nil {
		t.Fatal(err)
	}
}

func TestReplayEvents(t *testing.T) {
	db := newTestPolicyDB(t)

	// Add rules via events.
	p1, _ := json.Marshal(map[string]interface{}{
		"id": "r1", "pattern": ".env", "file_access": "deny",
		"file_authority": "standard", "prevent_write": true,
		"prevent_read": false, "created_by": "test",
	})
	AppendEvent(db, "file_rule.added", string(p1), "test")

	p2, _ := json.Marshal(map[string]interface{}{
		"id": "c1", "pattern": "rm -rf /*", "rule_access": "deny",
		"rule_authority": "standard", "created_by": "test",
	})
	AppendEvent(db, "command_rule.added", string(p2), "test")

	// Clear projections manually.
	db.Exec("DELETE FROM file_rules")
	db.Exec("DELETE FROM command_rules")

	// Replay should restore them.
	if err := ReplayEvents(db); err != nil {
		t.Fatal(err)
	}

	rules, _ := ListFileRules(db)
	if len(rules) != 1 || rules[0].Pattern != ".env" {
		t.Errorf("expected 1 file rule (.env), got %d", len(rules))
	}

	cmdRules, _ := ListRules(db)
	if len(cmdRules) != 1 || cmdRules[0].Pattern != "rm -rf /*" {
		t.Errorf("expected 1 command rule, got %d", len(cmdRules))
	}
}

func TestReplayEvents_Idempotent(t *testing.T) {
	db := newTestPolicyDB(t)

	p1, _ := json.Marshal(map[string]interface{}{
		"id": "r1", "pattern": ".env", "file_access": "deny",
		"file_authority": "standard", "prevent_write": true,
		"prevent_read": false, "created_by": "test",
	})
	AppendEvent(db, "file_rule.added", string(p1), "test")

	// Replay twice.
	if err := ReplayEvents(db); err != nil {
		t.Fatal(err)
	}
	if err := ReplayEvents(db); err != nil {
		t.Fatal(err)
	}

	rules, _ := ListFileRules(db)
	if len(rules) != 1 {
		t.Errorf("expected 1 file rule after double replay, got %d", len(rules))
	}
}

func TestListUnpushedEvents(t *testing.T) {
	db := newTestPolicyDB(t)

	p1, _ := json.Marshal(map[string]interface{}{
		"id": "r1", "pattern": ".env", "file_access": "deny",
		"file_authority": "standard", "prevent_write": true,
		"prevent_read": false, "created_by": "test",
	})
	AppendEvent(db, "file_rule.added", string(p1), "test")

	events, err := ListUnpushedEvents(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 unpushed event, got %d", len(events))
	}
	if events[0].ServerSeq != nil {
		t.Error("expected nil server_seq for unpushed event")
	}
}

func TestMarkEventsPushed(t *testing.T) {
	db := newTestPolicyDB(t)

	p1, _ := json.Marshal(map[string]interface{}{
		"id": "r1", "pattern": ".env", "file_access": "deny",
		"file_authority": "standard", "prevent_write": true,
		"prevent_read": false, "created_by": "test",
	})
	ev, _ := AppendEvent(db, "file_rule.added", string(p1), "test")

	err := MarkEventsPushed(db, map[string]int64{ev.EventID: 42})
	if err != nil {
		t.Fatal(err)
	}

	events, _ := ListUnpushedEvents(db)
	if len(events) != 0 {
		t.Errorf("expected 0 unpushed events after marking, got %d", len(events))
	}
}

func TestAppendRemoteEvents(t *testing.T) {
	db := newTestPolicyDB(t)

	serverSeq := int64(1)
	remoteEv := PolicyEvent{
		EventID:   "remote-id-1",
		EventType: "file_rule.added",
		Payload:   `{"id":"rr1","pattern":"secrets.json","file_access":"deny","file_authority":"standard","prevent_write":true,"prevent_read":true,"created_by":"admin"}`,
		Actor:     "admin",
		Timestamp: "2024-06-01T00:00:00Z",
		ServerSeq: &serverSeq,
	}

	if err := AppendRemoteEvents(db, []PolicyEvent{remoteEv}); err != nil {
		t.Fatal(err)
	}

	rules, _ := ListFileRules(db)
	if len(rules) != 1 || rules[0].Pattern != "secrets.json" {
		t.Errorf("expected 1 file rule (secrets.json), got %v", rules)
	}
}

func TestAddFileRuleCreatesEvent(t *testing.T) {
	db := newTestPolicyDB(t)

	_, err := AddFileRule(db, ".env", "deny", "standard", "alice", false)
	if err != nil {
		t.Fatal(err)
	}

	// Verify event was created.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM policy_events WHERE event_type = 'file_rule.added'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 file_rule.added event, got %d", count)
	}

	// Verify projection is correct.
	rules, _ := ListFileRules(db)
	if len(rules) != 1 || rules[0].Pattern != ".env" {
		t.Errorf("expected 1 file rule (.env), got %v", rules)
	}
}

func TestRemoveFileRuleCreatesEvent(t *testing.T) {
	db := newTestPolicyDB(t)

	AddFileRule(db, ".env", "deny", "standard", "alice", false)

	removed, err := RemoveFileRule(db, ".env")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("expected removed=true")
	}

	// Verify event was created.
	var count int
	db.QueryRow("SELECT COUNT(*) FROM policy_events WHERE event_type = 'file_rule.removed'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 file_rule.removed event, got %d", count)
	}

	// Verify projection is updated.
	rules, _ := ListFileRules(db)
	if len(rules) != 0 {
		t.Errorf("expected 0 file rules after removal, got %d", len(rules))
	}
}

func TestAddCommandRuleCreatesEvent(t *testing.T) {
	db := newTestPolicyDB(t)

	_, err := AddRule(db, "rm -rf /*", "deny", "standard", "alice")
	if err != nil {
		t.Fatal(err)
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM policy_events WHERE event_type = 'command_rule.added'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 command_rule.added event, got %d", count)
	}
}

func TestRemoveCommandRuleCreatesEvent(t *testing.T) {
	db := newTestPolicyDB(t)

	AddRule(db, "rm -rf /*", "deny", "standard", "alice")

	removed, err := RemoveRule(db, "rm -rf /*")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("expected removed=true")
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM policy_events WHERE event_type = 'command_rule.removed'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 command_rule.removed event, got %d", count)
	}
}

func TestMigrationFromExistingState(t *testing.T) {
	// Create a database with rules but no events (simulating pre-event-sourcing state).
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	// Create tables WITHOUT policy_events (old schema).
	stmts := []string{
		`CREATE TABLE file_rules (
			id TEXT PRIMARY KEY, pattern TEXT NOT NULL,
			file_access TEXT NOT NULL DEFAULT 'deny',
			file_authority TEXT NOT NULL DEFAULT 'standard',
			prevent_write INTEGER NOT NULL DEFAULT 1,
			prevent_read INTEGER NOT NULL DEFAULT 0,
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX idx_file_rules_pattern ON file_rules(pattern)`,
		`CREATE TABLE command_rules (
			id TEXT PRIMARY KEY, pattern TEXT NOT NULL,
			rule_access TEXT NOT NULL DEFAULT 'deny',
			rule_authority TEXT NOT NULL DEFAULT 'standard',
			created_by TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX idx_command_rules_pattern ON command_rules(pattern)`,
		`CREATE TABLE perimeter_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatal(err)
		}
	}

	// Insert rules directly (pre-event-sourcing).
	db.Exec(`INSERT INTO file_rules (id, pattern, file_access, file_authority, prevent_write, prevent_read, created_by, created_at, updated_at)
		VALUES ('fr1', '.env', 'deny', 'standard', 1, 0, 'seed', '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`)
	db.Exec(`INSERT INTO command_rules (id, pattern, rule_access, rule_authority, created_by, created_at, updated_at)
		VALUES ('cr1', 'rm -rf /*', 'deny', 'standard', 'seed', '2024-01-02T00:00:00Z', '2024-01-02T00:00:00Z')`)

	// Run migration — this should create the policy_events table and generate synthetic events.
	if err := MigratePolicyDB(db); err != nil {
		t.Fatal(err)
	}

	// Verify events were generated.
	var eventCount int
	db.QueryRow("SELECT COUNT(*) FROM policy_events").Scan(&eventCount)
	if eventCount != 2 {
		t.Errorf("expected 2 migration events, got %d", eventCount)
	}

	// Verify projections still have the original rules.
	rules, _ := ListFileRules(db)
	if len(rules) != 1 || rules[0].Pattern != ".env" {
		t.Errorf("expected file rule .env, got %v", rules)
	}

	cmdRules, _ := ListRules(db)
	if len(cmdRules) != 1 || cmdRules[0].Pattern != "rm -rf /*" {
		t.Errorf("expected command rule rm -rf /*, got %v", cmdRules)
	}
}

func TestMigrationSkipsWhenEventsExist(t *testing.T) {
	db := newTestPolicyDB(t)

	// Add a rule (creates an event).
	AddFileRule(db, ".env", "deny", "standard", "test", false)

	var countBefore int
	db.QueryRow("SELECT COUNT(*) FROM policy_events").Scan(&countBefore)

	// Run migration again — should be a no-op.
	if err := migrateExistingRulesToEvents(db); err != nil {
		t.Fatal(err)
	}

	var countAfter int
	db.QueryRow("SELECT COUNT(*) FROM policy_events").Scan(&countAfter)
	if countAfter != countBefore {
		t.Errorf("migration should be no-op when events exist: before=%d, after=%d", countBefore, countAfter)
	}
}
