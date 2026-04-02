package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// PolicyEvent is an immutable record of a policy mutation.
type PolicyEvent struct {
	Seq       int64  `json:"-"`                    // local auto-increment; not sent to server
	EventID   string `json:"event_id"`             // UUID v4
	EventType string `json:"event_type"`           // "file_rule.added", "file_rule.removed", etc.
	Payload   string `json:"payload"`              // JSON blob
	Actor     string `json:"actor"`                // GitHub username or OS username
	Timestamp string `json:"timestamp"`            // ISO 8601
	ServerSeq *int64 `json:"server_seq,omitempty"` // nil until server acknowledges
}

// AppendEvent writes a policy event and applies it to the projection tables
// in a single transaction. Returns the written event with seq assigned.
func AppendEvent(db *sql.DB, eventType, payload, actor string) (*PolicyEvent, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("store: begin event tx: %w", err)
	}
	defer tx.Rollback()

	ev, err := appendEventTx(tx, eventType, payload, actor, true)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: commit event tx: %w", err)
	}
	return ev, nil
}

// appendEventTx is the internal version that works within an existing transaction.
// If applyProjection is true, it also applies the event to the projection tables.
func appendEventTx(tx *sql.Tx, eventType, payload, actor string, applyProjection bool) (*PolicyEvent, error) {
	eventID, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("store: generate event id: %w", err)
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)

	ev := &PolicyEvent{
		EventID:   eventID,
		EventType: eventType,
		Payload:   payload,
		Actor:     actor,
		Timestamp: timestamp,
	}

	res, err := tx.Exec(
		`INSERT INTO policy_events (event_id, event_type, payload, actor, timestamp)
		 VALUES (?, ?, ?, ?, ?)`,
		ev.EventID, ev.EventType, ev.Payload, ev.Actor, ev.Timestamp,
	)
	if err != nil {
		return nil, fmt.Errorf("store: insert event: %w", err)
	}

	seq, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("store: get event seq: %w", err)
	}
	ev.Seq = seq

	if applyProjection {
		if err := applyEventToProjection(tx, ev); err != nil {
			return nil, err
		}
	}

	return ev, nil
}

// applyEventToProjection applies a single event to the projection tables within a transaction.
func applyEventToProjection(tx *sql.Tx, ev *PolicyEvent) error {
	switch ev.EventType {
	case "file_rule.added":
		return applyFileRuleAdded(tx, ev.Payload)
	case "file_rule.removed":
		return applyFileRuleRemoved(tx, ev.Payload)
	case "file_rule.updated":
		return applyFileRuleUpdated(tx, ev.Payload)
	case "command_rule.added":
		return applyCommandRuleAdded(tx, ev.Payload)
	case "command_rule.removed":
		return applyCommandRuleRemoved(tx, ev.Payload)
	case "command_rule.updated":
		return applyCommandRuleUpdated(tx, ev.Payload)
	default:
		// Unknown event types are silently ignored for forward compatibility.
		return nil
	}
}

func applyFileRuleAdded(tx *sql.Tx, payload string) error {
	var p struct {
		ID            string `json:"id"`
		Pattern       string `json:"pattern"`
		FileAccess    string `json:"file_access"`
		FileAuthority string `json:"file_authority"`
		PreventWrite  bool   `json:"prevent_write"`
		PreventRead   bool   `json:"prevent_read"`
		CreatedBy     string `json:"created_by"`
		CreatedAt     string `json:"created_at"`
		UpdatedAt     string `json:"updated_at"`
		Notify        bool   `json:"notify"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return fmt.Errorf("store: unmarshal file_rule.added: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	if p.UpdatedAt == "" {
		p.UpdatedAt = now
	}
	_, err := tx.Exec(
		`INSERT INTO file_rules (id, pattern, file_access, file_authority, prevent_write, prevent_read, created_by, created_at, updated_at, notify)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Pattern, p.FileAccess, p.FileAuthority, p.PreventWrite, p.PreventRead, p.CreatedBy, p.CreatedAt, p.UpdatedAt, p.Notify,
	)
	return err
}

func applyFileRuleRemoved(tx *sql.Tx, payload string) error {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return fmt.Errorf("store: unmarshal file_rule.removed: %w", err)
	}
	_, err := tx.Exec(`DELETE FROM file_rules WHERE id = ?`, p.ID)
	return err
}

func applyFileRuleUpdated(tx *sql.Tx, payload string) error {
	var p map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return fmt.Errorf("store: unmarshal file_rule.updated: %w", err)
	}
	id, ok := p["id"].(string)
	if !ok {
		return fmt.Errorf("store: file_rule.updated missing id")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for k, v := range p {
		if k == "id" || k == "pattern" {
			continue
		}
		col := k
		_, err := tx.Exec(fmt.Sprintf(`UPDATE file_rules SET %s = ?, updated_at = ? WHERE id = ?`, col), v, now, id)
		if err != nil {
			return fmt.Errorf("store: update file_rules.%s: %w", col, err)
		}
	}
	return nil
}

func applyCommandRuleAdded(tx *sql.Tx, payload string) error {
	var p struct {
		ID            string `json:"id"`
		Pattern       string `json:"pattern"`
		RuleAccess    string `json:"rule_access"`
		RuleAuthority string `json:"rule_authority"`
		CreatedBy     string `json:"created_by"`
		CreatedAt     string `json:"created_at"`
		UpdatedAt     string `json:"updated_at"`
		Notify        bool   `json:"notify"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return fmt.Errorf("store: unmarshal command_rule.added: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	if p.UpdatedAt == "" {
		p.UpdatedAt = now
	}
	_, err := tx.Exec(
		`INSERT INTO command_rules (id, pattern, rule_access, rule_authority, created_by, created_at, updated_at, notify)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Pattern, p.RuleAccess, p.RuleAuthority, p.CreatedBy, p.CreatedAt, p.UpdatedAt, p.Notify,
	)
	return err
}

func applyCommandRuleRemoved(tx *sql.Tx, payload string) error {
	var p struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return fmt.Errorf("store: unmarshal command_rule.removed: %w", err)
	}
	_, err := tx.Exec(`DELETE FROM command_rules WHERE id = ?`, p.ID)
	return err
}

func applyCommandRuleUpdated(tx *sql.Tx, payload string) error {
	var p map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return fmt.Errorf("store: unmarshal command_rule.updated: %w", err)
	}
	id, ok := p["id"].(string)
	if !ok {
		return fmt.Errorf("store: command_rule.updated missing id")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for k, v := range p {
		if k == "id" || k == "pattern" {
			continue
		}
		col := k
		_, err := tx.Exec(fmt.Sprintf(`UPDATE command_rules SET %s = ?, updated_at = ? WHERE id = ?`, col), v, now, id)
		if err != nil {
			return fmt.Errorf("store: update command_rules.%s: %w", col, err)
		}
	}
	return nil
}

// ReplayEvents rebuilds file_rules and command_rules from the full event log.
// Called after sync pull or during migration. Runs in a single transaction.
func ReplayEvents(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin replay tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM file_rules`); err != nil {
		return fmt.Errorf("store: clear file_rules: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM command_rules`); err != nil {
		return fmt.Errorf("store: clear command_rules: %w", err)
	}

	rows, err := tx.Query(`SELECT seq, event_id, event_type, payload, actor, timestamp, server_seq
		FROM policy_events ORDER BY seq ASC`)
	if err != nil {
		return fmt.Errorf("store: query events for replay: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ev PolicyEvent
		if err := rows.Scan(&ev.Seq, &ev.EventID, &ev.EventType, &ev.Payload, &ev.Actor,
			&ev.Timestamp, &ev.ServerSeq); err != nil {
			return fmt.Errorf("store: scan event: %w", err)
		}
		if err := applyEventToProjectionReplay(tx, &ev); err != nil {
			return fmt.Errorf("store: apply event seq=%d: %w", ev.Seq, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: iterate events: %w", err)
	}

	return tx.Commit()
}

// applyEventToProjectionReplay applies an event during replay, using INSERT OR REPLACE
// to handle duplicate patterns that can arise from concurrent remote additions.
func applyEventToProjectionReplay(tx *sql.Tx, ev *PolicyEvent) error {
	switch ev.EventType {
	case "file_rule.added":
		return applyFileRuleAddedReplay(tx, ev.Payload)
	case "file_rule.removed":
		return applyFileRuleRemoved(tx, ev.Payload)
	case "file_rule.updated":
		return applyFileRuleUpdated(tx, ev.Payload)
	case "command_rule.added":
		return applyCommandRuleAddedReplay(tx, ev.Payload)
	case "command_rule.removed":
		return applyCommandRuleRemoved(tx, ev.Payload)
	case "command_rule.updated":
		return applyCommandRuleUpdated(tx, ev.Payload)
	default:
		return nil
	}
}

func applyFileRuleAddedReplay(tx *sql.Tx, payload string) error {
	var p struct {
		ID            string `json:"id"`
		Pattern       string `json:"pattern"`
		FileAccess    string `json:"file_access"`
		FileAuthority string `json:"file_authority"`
		PreventWrite  bool   `json:"prevent_write"`
		PreventRead   bool   `json:"prevent_read"`
		CreatedBy     string `json:"created_by"`
		CreatedAt     string `json:"created_at"`
		UpdatedAt     string `json:"updated_at"`
		Notify        bool   `json:"notify"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return fmt.Errorf("store: unmarshal file_rule.added replay: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	if p.UpdatedAt == "" {
		p.UpdatedAt = now
	}
	_, err := tx.Exec(
		`INSERT OR REPLACE INTO file_rules (id, pattern, file_access, file_authority, prevent_write, prevent_read, created_by, created_at, updated_at, notify)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Pattern, p.FileAccess, p.FileAuthority, p.PreventWrite, p.PreventRead, p.CreatedBy, p.CreatedAt, p.UpdatedAt, p.Notify,
	)
	return err
}

func applyCommandRuleAddedReplay(tx *sql.Tx, payload string) error {
	var p struct {
		ID            string `json:"id"`
		Pattern       string `json:"pattern"`
		RuleAccess    string `json:"rule_access"`
		RuleAuthority string `json:"rule_authority"`
		CreatedBy     string `json:"created_by"`
		CreatedAt     string `json:"created_at"`
		UpdatedAt     string `json:"updated_at"`
		Notify        bool   `json:"notify"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return fmt.Errorf("store: unmarshal command_rule.added replay: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	if p.UpdatedAt == "" {
		p.UpdatedAt = now
	}
	_, err := tx.Exec(
		`INSERT OR REPLACE INTO command_rules (id, pattern, rule_access, rule_authority, created_by, created_at, updated_at, notify)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Pattern, p.RuleAccess, p.RuleAuthority, p.CreatedBy, p.CreatedAt, p.UpdatedAt, p.Notify,
	)
	return err
}

// ListUnpushedEvents returns all events where server_seq IS NULL, ordered by seq ASC.
func ListUnpushedEvents(db *sql.DB) ([]PolicyEvent, error) {
	rows, err := db.Query(
		`SELECT seq, event_id, event_type, payload, actor, timestamp, server_seq
		 FROM policy_events WHERE server_seq IS NULL ORDER BY seq ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("store: list unpushed events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// MarkEventsPushed updates server_seq for events that have been acknowledged by the server.
// assignments maps event_id -> server_seq.
func MarkEventsPushed(db *sql.DB, assignments map[string]int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin mark pushed tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`UPDATE policy_events SET server_seq = ? WHERE event_id = ?`)
	if err != nil {
		return fmt.Errorf("store: prepare mark pushed: %w", err)
	}
	defer stmt.Close()

	for eventID, serverSeq := range assignments {
		if _, err := stmt.Exec(serverSeq, eventID); err != nil {
			return fmt.Errorf("store: mark event %s pushed: %w", eventID, err)
		}
	}

	return tx.Commit()
}

// AppendRemoteEvents inserts events received from the server and rebuilds projections.
// Runs in a single transaction.
func AppendRemoteEvents(db *sql.DB, events []PolicyEvent) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin remote events tx: %w", err)
	}
	defer tx.Rollback()

	for _, ev := range events {
		_, err := tx.Exec(
			`INSERT INTO policy_events (event_id, event_type, payload, actor, timestamp, server_seq)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			ev.EventID, ev.EventType, ev.Payload, ev.Actor, ev.Timestamp, ev.ServerSeq,
		)
		if err != nil {
			return fmt.Errorf("store: insert remote event %s: %w", ev.EventID, err)
		}
	}

	// Rebuild projections from the full event log.
	if _, err := tx.Exec(`DELETE FROM file_rules`); err != nil {
		return fmt.Errorf("store: clear file_rules for rebuild: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM command_rules`); err != nil {
		return fmt.Errorf("store: clear command_rules for rebuild: %w", err)
	}

	rows, err := tx.Query(`SELECT seq, event_id, event_type, payload, actor, timestamp, server_seq
		FROM policy_events ORDER BY seq ASC`)
	if err != nil {
		return fmt.Errorf("store: query events for rebuild: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ev PolicyEvent
		if err := rows.Scan(&ev.Seq, &ev.EventID, &ev.EventType, &ev.Payload, &ev.Actor,
			&ev.Timestamp, &ev.ServerSeq); err != nil {
			return fmt.Errorf("store: scan event for rebuild: %w", err)
		}
		if err := applyEventToProjectionReplay(tx, &ev); err != nil {
			return fmt.Errorf("store: apply remote event seq=%d: %w", ev.Seq, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("store: iterate events for rebuild: %w", err)
	}

	return tx.Commit()
}

// scanEvents reads all rows from a policy_events query into a slice.
func scanEvents(rows *sql.Rows) ([]PolicyEvent, error) {
	var events []PolicyEvent
	for rows.Next() {
		var ev PolicyEvent
		if err := rows.Scan(&ev.Seq, &ev.EventID, &ev.EventType, &ev.Payload, &ev.Actor,
			&ev.Timestamp, &ev.ServerSeq); err != nil {
			return nil, fmt.Errorf("store: scan event: %w", err)
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// migrateExistingRulesToEvents generates synthetic genesis events for any
// pre-existing rules that have no corresponding events. This is called during
// MigratePolicyDB to handle the transition from state-based to event-sourced policy.
func migrateExistingRulesToEvents(db *sql.DB) error {
	// Check if there are already events — if so, migration is not needed.
	var eventCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM policy_events").Scan(&eventCount); err != nil {
		return fmt.Errorf("store: count events for migration: %w", err)
	}
	if eventCount > 0 {
		return nil
	}

	// Check if there are any rules to migrate.
	var ruleCount int
	if err := db.QueryRow("SELECT (SELECT COUNT(*) FROM file_rules) + (SELECT COUNT(*) FROM command_rules)").Scan(&ruleCount); err != nil {
		return fmt.Errorf("store: count rules for migration: %w", err)
	}
	if ruleCount == 0 {
		return nil
	}

	// Collect all rules with timestamps for ordering.
	type migrationEntry struct {
		eventType string
		payload   string
		timestamp string
	}

	var entries []migrationEntry

	// Read file rules.
	fileRows, err := db.Query(
		`SELECT id, pattern, file_access, file_authority, prevent_write, prevent_read, created_by, created_at
		 FROM file_rules ORDER BY created_at ASC`,
	)
	if err != nil {
		return fmt.Errorf("store: read file rules for migration: %w", err)
	}
	defer fileRows.Close()

	for fileRows.Next() {
		var id, pattern, fileAccess, fileAuthority, createdBy, createdAt string
		var preventWrite, preventRead int
		if err := fileRows.Scan(&id, &pattern, &fileAccess, &fileAuthority, &preventWrite, &preventRead, &createdBy, &createdAt); err != nil {
			return fmt.Errorf("store: scan file rule for migration: %w", err)
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"id":             id,
			"pattern":        pattern,
			"file_access":    fileAccess,
			"file_authority": fileAuthority,
			"prevent_write":  preventWrite != 0,
			"prevent_read":   preventRead != 0,
			"created_by":     createdBy,
			"created_at":     createdAt,
			"updated_at":     createdAt,
		})
		entries = append(entries, migrationEntry{
			eventType: "file_rule.added",
			payload:   string(payload),
			timestamp: createdAt,
		})
	}
	if err := fileRows.Err(); err != nil {
		return fmt.Errorf("store: iterate file rules for migration: %w", err)
	}

	// Read command rules.
	cmdRows, err := db.Query(
		`SELECT id, pattern, rule_access, rule_authority, created_by, created_at
		 FROM command_rules ORDER BY created_at ASC`,
	)
	if err != nil {
		return fmt.Errorf("store: read command rules for migration: %w", err)
	}
	defer cmdRows.Close()

	for cmdRows.Next() {
		var id, pattern, ruleAccess, ruleAuthority, createdBy, createdAt string
		if err := cmdRows.Scan(&id, &pattern, &ruleAccess, &ruleAuthority, &createdBy, &createdAt); err != nil {
			return fmt.Errorf("store: scan command rule for migration: %w", err)
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"id":             id,
			"pattern":        pattern,
			"rule_access":    ruleAccess,
			"rule_authority": ruleAuthority,
			"created_by":     createdBy,
			"created_at":     createdAt,
			"updated_at":     createdAt,
		})
		entries = append(entries, migrationEntry{
			eventType: "command_rule.added",
			payload:   string(payload),
			timestamp: createdAt,
		})
	}
	if err := cmdRows.Err(); err != nil {
		return fmt.Errorf("store: iterate command rules for migration: %w", err)
	}

	// Sort by timestamp across both rule types.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].timestamp < entries[j].timestamp
	})

	// Write synthetic events (skip projection writes since projections already exist).
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("store: begin migration tx: %w", err)
	}
	defer tx.Rollback()

	for _, entry := range entries {
		if _, err := appendEventTx(tx, entry.eventType, entry.payload, "system", false); err != nil {
			return fmt.Errorf("store: append migration event: %w", err)
		}
	}

	return tx.Commit()
}
