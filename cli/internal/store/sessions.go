package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Session represents a row in the sessions table.
// Token fields are unified across agents:
//   - InputTokens:     total input context (includes cached tokens)
//   - OutputTokens:    total generated tokens (includes thoughts for Gemini)
//   - CacheReadTokens: portion of input that came from cache
type Session struct {
	SessionID       string
	Agent           string
	Description     string
	TranscriptPath  string
	InputTokens     int64
	OutputTokens    int64
	CacheReadTokens int64
	FirstSeenAt     int64 // Unix microseconds (matches hook_log.ts)
	LastSeenAt      int64 // Unix microseconds
	UpdatedAt       int64 // Unix microseconds
}

// PendingSession is the minimal info needed to drive transcript extraction.
type PendingSession struct {
	SessionID      string
	Agent          string
	TranscriptPath string
	FirstSeenAt    int64 // Unix microseconds
	LastSeenAt     int64 // Unix microseconds
}

// UpsertSession inserts or updates a session row. On conflict (session already
// exists), it updates token counts, description, and timestamps.
func UpsertSession(db *sql.DB, s Session) error {
	_, err := db.Exec(`
		INSERT INTO sessions (session_id, agent, description, transcript_path,
			input_tokens, output_tokens, cache_read_tokens,
			first_seen_at, last_seen_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
			description       = excluded.description,
			transcript_path   = CASE WHEN excluded.transcript_path != '' THEN excluded.transcript_path ELSE sessions.transcript_path END,
			input_tokens      = excluded.input_tokens,
			output_tokens     = excluded.output_tokens,
			cache_read_tokens = excluded.cache_read_tokens,
			last_seen_at      = excluded.last_seen_at,
			updated_at        = excluded.updated_at`,
		s.SessionID, s.Agent, s.Description, s.TranscriptPath,
		s.InputTokens, s.OutputTokens, s.CacheReadTokens,
		s.FirstSeenAt, s.LastSeenAt, s.UpdatedAt,
	)
	return err
}

// PendingSessions returns sessions from hook_log that either don't exist in the
// sessions table or have a stale updated_at (older than staleThreshold).
func PendingSessions(db *sql.DB, staleThreshold time.Duration) ([]PendingSession, error) {
	cutoff := time.Now().Add(-staleThreshold).UnixMicro()

	// Include sessions with empty transcript_path (e.g. Cursor, which sends
	// conversation_id but no transcript on early hook calls). These sessions
	// still appear in the sessions table with basic metadata from hook_log.
	// MAX(transcript_path) picks the non-empty path when some hook_log entries
	// have it and others don't (Cursor may start sending it mid-session).
	rows, err := db.Query(`
		SELECT h.session_id, h.agent, MAX(h.transcript_path),
		       MIN(h.ts) AS first_seen, MAX(h.ts) AS last_seen
		FROM hook_log h
		LEFT JOIN sessions s ON h.session_id = s.session_id
		WHERE h.session_id != ''
		  AND (s.session_id IS NULL OR s.updated_at < ?)
		GROUP BY h.session_id, h.agent`, cutoff)
	if err != nil {
		return nil, fmt.Errorf("store: pending sessions: %w", err)
	}
	defer rows.Close()

	var result []PendingSession
	for rows.Next() {
		var p PendingSession
		if err := rows.Scan(&p.SessionID, &p.Agent, &p.TranscriptPath,
			&p.FirstSeenAt, &p.LastSeenAt); err != nil {
			return nil, fmt.Errorf("store: scan pending session: %w", err)
		}
		result = append(result, p)
	}
	return result, rows.Err()
}
