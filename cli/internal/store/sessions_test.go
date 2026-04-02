package store

import (
	"testing"
	"time"
)

func TestPendingSessions_OnlyRecentAndChanged(t *testing.T) {
	db := newTestDataDB(t)

	now := time.Now().UnixMicro()
	window := time.Hour

	// Session with new hook activity after last extraction: should be pending.
	if err := InsertHookLog(db, HookLogEntry{
		Ts:             now - int64(10*time.Minute/time.Microsecond),
		ToolName:       "Write",
		FilePath:       "/repo/a.txt",
		Decision:       "allow",
		OSUser:         "test",
		Agent:          "codex",
		SessionID:      "sess-active",
		TranscriptPath: "/tmp/a.jsonl",
	}); err != nil {
		t.Fatalf("insert hook log (active): %v", err)
	}
	if err := UpsertSession(db, Session{
		SessionID:      "sess-active",
		Agent:          "codex",
		TranscriptPath: "/tmp/a.jsonl",
		FirstSeenAt:    now - int64(20*time.Minute/time.Microsecond),
		LastSeenAt:     now - int64(15*time.Minute/time.Microsecond),
		UpdatedAt:      now - int64(20*time.Minute/time.Microsecond),
	}); err != nil {
		t.Fatalf("upsert session (active): %v", err)
	}

	// Session with no new hook activity since extraction: should NOT be pending.
	if err := InsertHookLog(db, HookLogEntry{
		Ts:             now - int64(20*time.Minute/time.Microsecond),
		ToolName:       "Write",
		FilePath:       "/repo/b.txt",
		Decision:       "allow",
		OSUser:         "test",
		Agent:          "codex",
		SessionID:      "sess-unchanged",
		TranscriptPath: "/tmp/b.jsonl",
	}); err != nil {
		t.Fatalf("insert hook log (unchanged): %v", err)
	}
	if err := UpsertSession(db, Session{
		SessionID:      "sess-unchanged",
		Agent:          "codex",
		TranscriptPath: "/tmp/b.jsonl",
		FirstSeenAt:    now - int64(20*time.Minute/time.Microsecond),
		LastSeenAt:     now - int64(20*time.Minute/time.Microsecond),
		UpdatedAt:      now - int64(5*time.Minute/time.Microsecond), // newer than last hook
	}); err != nil {
		t.Fatalf("upsert session (unchanged): %v", err)
	}

	// Session with old hook activity outside the activity window: should NOT be pending.
	if err := InsertHookLog(db, HookLogEntry{
		Ts:             now - int64(2*time.Hour/time.Microsecond),
		ToolName:       "Write",
		FilePath:       "/repo/c.txt",
		Decision:       "allow",
		OSUser:         "test",
		Agent:          "codex",
		SessionID:      "sess-old",
		TranscriptPath: "/tmp/c.jsonl",
	}); err != nil {
		t.Fatalf("insert hook log (old): %v", err)
	}

	pending, err := PendingSessions(db, window)
	if err != nil {
		t.Fatalf("pending sessions: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("expected 1 pending session, got %d", len(pending))
	}
	if pending[0].SessionID != "sess-active" {
		t.Fatalf("expected sess-active pending, got %q", pending[0].SessionID)
	}
}

func TestPendingSessions_NewRecentSessionIsPending(t *testing.T) {
	db := newTestDataDB(t)
	now := time.Now().UnixMicro()

	if err := InsertHookLog(db, HookLogEntry{
		Ts:             now - int64(2*time.Minute/time.Microsecond),
		ToolName:       "Write",
		FilePath:       "/repo/new.txt",
		Decision:       "allow",
		OSUser:         "test",
		Agent:          "codex",
		SessionID:      "sess-new",
		TranscriptPath: "/tmp/new.jsonl",
	}); err != nil {
		t.Fatalf("insert hook log (new): %v", err)
	}

	pending, err := PendingSessions(db, time.Hour)
	if err != nil {
		t.Fatalf("pending sessions: %v", err)
	}
	if len(pending) != 1 || pending[0].SessionID != "sess-new" {
		t.Fatalf("expected sess-new pending, got %#v", pending)
	}
}
