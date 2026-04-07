package store

import (
	"testing"
	"time"
)

func TestListUnifiedLog_UntilAndLimit(t *testing.T) {
	db := newTestDataDB(t)

	base := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)

	if err := InsertHookLog(db, HookLogEntry{
		Ts:       base.Add(-3 * time.Hour).UnixMicro(),
		ToolName: "Write",
		FilePath: "/repo/a.txt",
		Decision: "allow",
		OSUser:   "tester",
	}); err != nil {
		t.Fatalf("insert hook old: %v", err)
	}
	if err := InsertAudit(db, AuditEntry{
		EventType: "pass_issue",
		FilePath:  "/repo/b.txt",
		User:      "tester",
		Timestamp: base.Add(-2 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("insert audit middle: %v", err)
	}
	if err := InsertHookLog(db, HookLogEntry{
		Ts:       base.Add(-1 * time.Hour).UnixMicro(),
		ToolName: "Edit",
		FilePath: "/repo/c.txt",
		Decision: "deny",
		OSUser:   "tester",
	}); err != nil {
		t.Fatalf("insert hook newest: %v", err)
	}

	entries, err := ListUnifiedLog(db, LogFilter{Until: base.Add(-90 * time.Minute)})
	if err != nil {
		t.Fatalf("query with until: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries before until cutoff, got %d", len(entries))
	}
	if entries[0].FilePath != "/repo/b.txt" || entries[1].FilePath != "/repo/a.txt" {
		t.Fatalf("unexpected order/content for until query: %#v", entries)
	}

	entries, err = ListUnifiedLog(db, LogFilter{Limit: 2})
	if err != nil {
		t.Fatalf("query with limit: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries with limit, got %d", len(entries))
	}
	if entries[0].FilePath != "/repo/c.txt" || entries[1].FilePath != "/repo/b.txt" {
		t.Fatalf("unexpected order/content for limit query: %#v", entries)
	}
}
