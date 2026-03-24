package store

import (
	"testing"
)

func TestComputeDataHash_Deterministic(t *testing.T) {
	h1 := computeDataHash("field1", "field2", "field3")
	h2 := computeDataHash("field1", "field2", "field3")
	if h1 != h2 {
		t.Errorf("same inputs produced different hashes: %s vs %s", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("hash length = %d, want 64", len(h1))
	}
}

func TestComputeDataHash_DifferentInputs(t *testing.T) {
	h1 := computeDataHash("a", "b", "c")
	h2 := computeDataHash("a", "b", "d")
	if h1 == h2 {
		t.Error("different inputs should produce different hashes")
	}
}

func TestInsertHookLog_HashChain(t *testing.T) {
	db := newTestDataDB(t)

	e1 := HookLogEntry{
		Ts:       1000000,
		ToolName: "Write",
		FilePath: "/test/file.go",
		Decision: "allow",
		OSUser:   "testuser",
		Agent:    "claude-code",
	}
	if err := InsertHookLog(db, e1); err != nil {
		t.Fatal(err)
	}

	// Read back the first entry.
	var hash1, parentHash1 string
	err := db.QueryRow("SELECT parent_hash, hash FROM hook_log WHERE id = 1").Scan(&parentHash1, &hash1)
	if err != nil {
		t.Fatal(err)
	}
	if parentHash1 != "" {
		t.Errorf("first entry parent_hash = %q, want empty", parentHash1)
	}
	if hash1 == "" {
		t.Error("first entry hash should not be empty")
	}

	// Insert second entry.
	e2 := HookLogEntry{
		Ts:       2000000,
		ToolName: "Edit",
		FilePath: "/test/other.go",
		Decision: "deny",
		OSUser:   "testuser",
		Agent:    "claude-code",
	}
	if err := InsertHookLog(db, e2); err != nil {
		t.Fatal(err)
	}

	var hash2, parentHash2 string
	err = db.QueryRow("SELECT parent_hash, hash FROM hook_log WHERE id = 2").Scan(&parentHash2, &hash2)
	if err != nil {
		t.Fatal(err)
	}
	if parentHash2 != hash1 {
		t.Errorf("second entry parent_hash = %q, want %q", parentHash2, hash1)
	}
	if hash2 == "" || hash2 == hash1 {
		t.Error("second entry hash should be non-empty and different from first")
	}
}

func TestInsertHookLog_NotifyFlag(t *testing.T) {
	db := newTestDataDB(t)

	e := HookLogEntry{
		Ts:       1000000,
		ToolName: "Write",
		FilePath: "/test/file.go",
		Decision: "deny",
		OSUser:   "testuser",
		Notify:   true,
	}
	if err := InsertHookLog(db, e); err != nil {
		t.Fatal(err)
	}

	var notify int
	err := db.QueryRow("SELECT notify FROM hook_log WHERE id = 1").Scan(&notify)
	if err != nil {
		t.Fatal(err)
	}
	if notify != 1 {
		t.Errorf("notify = %d, want 1", notify)
	}
}

func TestInsertAudit_HashChain(t *testing.T) {
	db := newTestDataDB(t)

	e1 := AuditEntry{
		EventType: "file_add",
		FilePath:  ".env",
		User:      "alice",
		Detail:    "added file rule",
	}
	if err := InsertAudit(db, e1); err != nil {
		t.Fatal(err)
	}

	var hash1, parentHash1 string
	err := db.QueryRow("SELECT parent_hash, hash FROM audit_log WHERE id = 1").Scan(&parentHash1, &hash1)
	if err != nil {
		t.Fatal(err)
	}
	if parentHash1 != "" {
		t.Errorf("first audit entry parent_hash = %q, want empty", parentHash1)
	}
	if hash1 == "" {
		t.Error("first audit entry hash should not be empty")
	}

	e2 := AuditEntry{
		EventType: "file_remove",
		FilePath:  ".env",
		User:      "alice",
		Detail:    "removed file rule",
	}
	if err := InsertAudit(db, e2); err != nil {
		t.Fatal(err)
	}

	var hash2, parentHash2 string
	err = db.QueryRow("SELECT parent_hash, hash FROM audit_log WHERE id = 2").Scan(&parentHash2, &hash2)
	if err != nil {
		t.Fatal(err)
	}
	if parentHash2 != hash1 {
		t.Errorf("second audit entry parent_hash = %q, want %q", parentHash2, hash1)
	}
}
