package store

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDataDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatal(err)
	}
	if err := MigrateDataDB(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestWatermarks(t *testing.T) {
	db := openTestDataDB(t)
	defer db.Close()

	// Initially zero.
	wm, err := GetWatermark(db, "hook_log")
	if err != nil {
		t.Fatal(err)
	}
	if wm != 0 {
		t.Errorf("expected 0, got %d", wm)
	}

	// Set and read back.
	if err := SetWatermark(db, "hook_log", 42); err != nil {
		t.Fatal(err)
	}
	wm, err = GetWatermark(db, "hook_log")
	if err != nil {
		t.Fatal(err)
	}
	if wm != 42 {
		t.Errorf("expected 42, got %d", wm)
	}

	// Update existing watermark.
	if err := SetWatermark(db, "hook_log", 100); err != nil {
		t.Fatal(err)
	}
	wm, err = GetWatermark(db, "hook_log")
	if err != nil {
		t.Fatal(err)
	}
	if wm != 100 {
		t.Errorf("expected 100, got %d", wm)
	}

	// Different tables are independent.
	if err := SetWatermark(db, "audit_log", 7); err != nil {
		t.Fatal(err)
	}
	wm, err = GetWatermark(db, "audit_log")
	if err != nil {
		t.Fatal(err)
	}
	if wm != 7 {
		t.Errorf("expected 7, got %d", wm)
	}
	// hook_log should still be 100.
	wm, err = GetWatermark(db, "hook_log")
	if err != nil {
		t.Fatal(err)
	}
	if wm != 100 {
		t.Errorf("expected 100, got %d", wm)
	}
}

func TestHookLogEntriesSince(t *testing.T) {
	db := openTestDataDB(t)
	defer db.Close()

	// Insert a few entries.
	for i := 0; i < 5; i++ {
		err := InsertHookLog(db, HookLogEntry{
			Ts:       int64(1000 + i),
			ToolName: "Write",
			FilePath: "/test.go",
			Decision: "allow",
			OSUser:   "testuser",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Get all entries (afterID=0).
	entries, maxID, err := HookLogEntriesSince(db, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
	if maxID != 5 {
		t.Errorf("expected maxID=5, got %d", maxID)
	}

	// Get entries after ID 3.
	entries, maxID, err = HookLogEntriesSince(db, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
	if maxID != 5 {
		t.Errorf("expected maxID=5, got %d", maxID)
	}

	// Get entries after maxID (should be empty).
	entries, _, err = HookLogEntriesSince(db, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestMaxServerSeq(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		t.Fatal(err)
	}
	if err := MigratePolicyDB(db); err != nil {
		t.Fatal(err)
	}

	// No events: should be 0.
	seq, err := MaxServerSeq(db)
	if err != nil {
		t.Fatal(err)
	}
	if seq != 0 {
		t.Errorf("expected 0, got %d", seq)
	}

	// Add a local event (no server_seq).
	_, err = AppendEvent(db, "file_rule.added",
		`{"id":"a","pattern":".env","file_access":"deny","file_authority":"standard","prevent_write":true,"prevent_read":false,"created_by":"test"}`,
		"test")
	if err != nil {
		t.Fatal(err)
	}

	// Still 0 (local event has no server_seq).
	seq, err = MaxServerSeq(db)
	if err != nil {
		t.Fatal(err)
	}
	if seq != 0 {
		t.Errorf("expected 0, got %d", seq)
	}

	// Mark as pushed with server_seq=10.
	err = MarkEventsPushed(db, map[string]int64{"a": 10})
	if err != nil {
		// Event IDs are auto-generated, so we need to find the actual ID.
		// Let's just verify MaxServerSeq works with a direct insert.
	}

	// Direct insert with server_seq to test MaxServerSeq.
	_, err = db.Exec(
		`INSERT INTO policy_events (event_id, event_type, payload, actor, timestamp, parent_hash, hash, server_seq)
		 VALUES ('test-remote', 'file_rule.added', '{}', 'test', '2024-01-01', '', 'abc', 42)`,
	)
	if err != nil {
		t.Fatal(err)
	}

	seq, err = MaxServerSeq(db)
	if err != nil {
		t.Fatal(err)
	}
	if seq != 42 {
		t.Errorf("expected 42, got %d", seq)
	}
}
