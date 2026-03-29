package cmd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/cordon-co/cordon-cli/cli/internal/reporoot"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
	cordsync "github.com/cordon-co/cordon-cli/cli/internal/sync"
	"github.com/spf13/cobra"
)

var syncBackground bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync policy and audit data with Cordon Cloud",
	Long:  "Pulls policy from api.cordon.sh and pushes local audit data. Cloud wins on conflict.",
	Args:  cobra.NoArgs,
	RunE:  runSync,
}

func init() {
	syncCmd.Flags().BoolVar(&syncBackground, "background", false, "Run sync as a detached background process with file locking")
}

// syncResult is the JSON output of a successful sync.
type syncResult struct {
	PolicyPulled int `json:"policy_pulled"`
	PolicyPushed int `json:"policy_pushed"`
	DataPushed   int `json:"data_pushed"`
}

func runSync(cmd *cobra.Command, args []string) error {
	if !api.IsLoggedIn() {
		if flags.JSON {
			fmt.Println(`{"error":"not authenticated — run 'cordon auth login' first"}`)
			return nil
		}
		return fmt.Errorf("not authenticated — run 'cordon auth login' first")
	}

	root, _, err := reporoot.Find()
	if err != nil {
		return fmt.Errorf("sync: find repo root: %w", err)
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("sync: resolve repo root: %w", err)
	}

	if syncBackground {
		return runSyncBackground(absRoot)
	}

	return runSyncForeground(cmd, absRoot)
}

// runSyncBackground acquires an exclusive lock, redirects output to a log file,
// runs sync logic, and writes .last_sync on success.
func runSyncBackground(absRoot string) error {
	perimeterID, err := store.ReadPerimeterID(absRoot)
	if err != nil {
		return err
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	repoDir := filepath.Join(homeDir, ".cordon", "repos", perimeterID)
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		return err
	}

	// Acquire exclusive lock.
	lockPath := filepath.Join(repoDir, ".sync.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lockFile.Close()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil // another sync is running — exit silently
	}
	defer syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)

	// Redirect output to log file.
	logPath := filepath.Join(repoDir, "sync.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return err
	}
	defer logFile.Close()

	result, err := doSync(absRoot, logFile)
	if err != nil {
		fmt.Fprintf(logFile, "sync error: %v\n", err)
		return err
	}

	fmt.Fprintf(logFile, "sync complete: pulled %d policy events, pushed %d policy events, pushed %d log entries\n",
		result.PolicyPulled, result.PolicyPushed, result.DataPushed)

	return cordsync.TouchLastSync(absRoot)
}

// runSyncForeground runs sync in the foreground, printing output to the user.
func runSyncForeground(cmd *cobra.Command, absRoot string) error {
	result, err := doSync(absRoot, cmd.ErrOrStderr())
	if err != nil {
		if flags.JSON {
			out, _ := json.Marshal(map[string]string{"error": err.Error()})
			fmt.Println(string(out))
			return nil
		}
		return fmt.Errorf("sync: %w", err)
	}

	if err := cordsync.TouchLastSync(absRoot); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not update .last_sync: %v\n", err)
	}

	if flags.JSON {
		out, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(out))
		return nil
	}

	fmt.Printf("Synced: pulled %d policy events, pushed %d policy events, pushed %d log entries\n",
		result.PolicyPulled, result.PolicyPushed, result.DataPushed)
	return nil
}

// doSync performs the actual sync logic: perimeter ID migration, policy pull/push, data push.
func doSync(absRoot string, logWriter io.Writer) (*syncResult, error) {
	// Open policy DB and run perimeter ID migration.
	policyDB, err := store.OpenPolicyDB(absRoot)
	if err != nil {
		return nil, fmt.Errorf("open policy db: %w", err)
	}
	defer policyDB.Close()

	if err := store.MigratePolicyDB(policyDB); err != nil {
		return nil, fmt.Errorf("migrate policy db: %w", err)
	}

	if err := store.MigratePerimeterID(policyDB, absRoot); err != nil {
		fmt.Fprintf(logWriter, "warning: perimeter ID migration: %v\n", err)
	}

	perimeterID, err := store.GetPerimeterID(policyDB)
	if err != nil {
		return nil, fmt.Errorf("read perimeter id: %w", err)
	}

	client, err := api.NewClient()
	if err != nil {
		return nil, fmt.Errorf("create api client: %w", err)
	}

	// Lookup perimeter on the server.
	// Spec §2.4: response is { perimeter_id, name, role }.
	var lookupResp struct {
		PerimeterID string `json:"perimeter_id"`
		Name        string `json:"name"`
		Role        string `json:"role"`
	}
	_, err = client.GetJSON(fmt.Sprintf("/api/v1/perimeters/lookup?perimeter_id=%s", perimeterID), &lookupResp)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return nil, fmt.Errorf("this repository is not registered in your Cordon dashboard")
		}
		return nil, fmt.Errorf("perimeter lookup: %w", err)
	}

	// The perimeter_id is used as the path parameter for all subsequent API calls.
	pid := lookupResp.PerimeterID

	// --- Policy Pull ---
	pulled, err := syncPolicyPull(policyDB, client, pid)
	if err != nil {
		return nil, fmt.Errorf("policy pull: %w", err)
	}

	// --- Policy Push ---
	pushed, err := syncPolicyPush(policyDB, client, pid)
	if err != nil {
		return nil, fmt.Errorf("policy push: %w", err)
	}

	// --- Data Push ---
	dataDB, err := store.OpenDataDB(absRoot)
	if err != nil {
		return nil, fmt.Errorf("open data db: %w", err)
	}
	defer dataDB.Close()

	if err := store.MigrateDataDB(dataDB); err != nil {
		return nil, fmt.Errorf("migrate data db: %w", err)
	}

	dataPushed, err := syncDataPush(dataDB, client, pid)
	if err != nil {
		fmt.Fprintf(logWriter, "warning: data push: %v\n", err)
		dataPushed = 0
	}

	return &syncResult{
		PolicyPulled: pulled,
		PolicyPushed: pushed,
		DataPushed:   dataPushed,
	}, nil
}

// syncPolicyPull fetches remote policy events after the local max server_seq.
// Handles pagination via has_more (spec §3.2).
func syncPolicyPull(policyDB *sql.DB, client *api.Client, perimeterID string) (int, error) {
	totalPulled := 0
	afterSeq, err := store.MaxServerSeq(policyDB)
	if err != nil {
		return 0, err
	}

	for {
		var pullResp struct {
			Events  []store.PolicyEvent `json:"events"`
			HasMore bool                `json:"has_more"`
		}
		_, err = client.GetJSON(
			fmt.Sprintf("/api/v1/perimeters/%s/policy/events?after_server_seq=%d&limit=1000", perimeterID, afterSeq),
			&pullResp,
		)
		if err != nil {
			return totalPulled, err
		}

		if len(pullResp.Events) == 0 {
			break
		}

		if err := store.AppendRemoteEvents(policyDB, pullResp.Events); err != nil {
			return totalPulled, err
		}
		totalPulled += len(pullResp.Events)

		if !pullResp.HasMore {
			break
		}

		// Advance cursor to the last received server_seq for the next page.
		lastEvent := pullResp.Events[len(pullResp.Events)-1]
		if lastEvent.ServerSeq != nil {
			afterSeq = *lastEvent.ServerSeq
		} else {
			break // shouldn't happen — remote events always have server_seq
		}
	}

	return totalPulled, nil
}

// syncPolicyPush sends unpushed local events to the server.
// Handles 409 (events_behind) by pulling again and retrying once.
func syncPolicyPush(policyDB *sql.DB, client *api.Client, perimeterID string) (int, error) {
	unpushed, err := store.ListUnpushedEvents(policyDB)
	if err != nil {
		return 0, err
	}
	if len(unpushed) == 0 {
		return 0, nil
	}

	pushed, err := pushEvents(policyDB, client, perimeterID, unpushed)
	if err != nil {
		return 0, err
	}
	return pushed, nil
}

// policyPushRequest matches spec §3.1.
type policyPushRequest struct {
	Events             []store.PolicyEvent `json:"events"`
	LastKnownServerSeq int64               `json:"last_known_server_seq"`
}

// policyPushResponse matches spec §3.1.
type policyPushResponse struct {
	Accepted             int              `json:"accepted"`
	ServerSeqAssignments map[string]int64 `json:"server_seq_assignments"`
}

func pushEvents(policyDB *sql.DB, client *api.Client, perimeterID string, events []store.PolicyEvent) (int, error) {
	maxSeq, err := store.MaxServerSeq(policyDB)
	if err != nil {
		return 0, err
	}

	var resp policyPushResponse
	_, err = client.PostJSON(
		fmt.Sprintf("/api/v1/perimeters/%s/policy/events", perimeterID),
		policyPushRequest{Events: events, LastKnownServerSeq: maxSeq},
		&resp,
	)
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.Code == "events_behind" {
			// Pull first, then retry.
			if _, pullErr := syncPolicyPull(policyDB, client, perimeterID); pullErr != nil {
				return 0, fmt.Errorf("pull before retry: %w", pullErr)
			}
			// Re-read unpushed (may have changed after pull).
			newUnpushed, err := store.ListUnpushedEvents(policyDB)
			if err != nil {
				return 0, err
			}
			if len(newUnpushed) == 0 {
				return 0, nil
			}
			// Recompute max server_seq after pull.
			newMaxSeq, err := store.MaxServerSeq(policyDB)
			if err != nil {
				return 0, err
			}
			// Retry push once.
			_, err = client.PostJSON(
				fmt.Sprintf("/api/v1/perimeters/%s/policy/events", perimeterID),
				policyPushRequest{Events: newUnpushed, LastKnownServerSeq: newMaxSeq},
				&resp,
			)
			if err != nil {
				return 0, err
			}
		} else {
			return 0, err
		}
	}

	if err := store.MarkEventsPushed(policyDB, resp.ServerSeqAssignments); err != nil {
		return 0, err
	}

	return len(resp.ServerSeqAssignments), nil
}

// --- Data Ingest ---

// ingestHookLogEntry matches the spec §4.1 hook_log item shape (includes id).
type ingestHookLogEntry struct {
	ID                   int64  `json:"id"`
	Ts                   int64  `json:"ts"`
	ToolName             string `json:"tool_name"`
	FilePath             string `json:"file_path"`
	ToolInput            string `json:"tool_input"`
	CommandRaw           string `json:"command_raw"`
	CommandParsed        bool   `json:"command_parsed_ok"`
	CommandParseError    string `json:"command_parse_error"`
	CommandParser        string `json:"command_parser"`
	CommandParserVersion string `json:"command_parser_version"`
	CommandOpsJSON       string `json:"command_ops_json"`
	DeniedOpIndex        int    `json:"denied_op_index"`
	DeniedOpReason       string `json:"denied_op_reason"`
	MatchedRulePattern   string `json:"matched_rule_pattern"`
	MatchedRuleType      string `json:"matched_rule_type"`
	Ambiguity            string `json:"ambiguity"`
	Decision             string `json:"decision"`
	OSUser               string `json:"os_user"`
	Agent                string `json:"agent"`
	PassID               string `json:"pass_id"`
	Notify               bool   `json:"notify"`
	SessionID            string `json:"session_id"`
	TranscriptPath       string `json:"transcript_path"`
	ParentHash           string `json:"parent_hash"`
	Hash                 string `json:"hash"`
}

// ingestAuditEntry matches the spec §4.1 audit_log item shape (includes id).
type ingestAuditEntry struct {
	ID         int64  `json:"id"`
	EventType  string `json:"event_type"`
	FilePath   string `json:"file_path"`
	User       string `json:"user"`
	Detail     string `json:"detail"`
	Timestamp  string `json:"timestamp"`
	ParentHash string `json:"parent_hash"`
	Hash       string `json:"hash"`
}

// ingestPass matches the spec §4.1 passes item shape.
type ingestPass struct {
	ID         string `json:"id"`
	FileRuleID string `json:"file_rule_id"`
	Pattern    string `json:"pattern"`
	Status     string `json:"status"`
	IssuedTo   string `json:"issued_to"`
	IssuedBy   string `json:"issued_by"`
	IssuedAt   string `json:"issued_at"`
	ExpiresAt  string `json:"expires_at"`
}

// ingestSession matches the spec §4.1 sessions item shape.
type ingestSession struct {
	SessionID       string `json:"session_id"`
	Agent           string `json:"agent"`
	Description     string `json:"description"`
	TranscriptPath  string `json:"transcript_path"`
	InputTokens     int64  `json:"input_tokens"`
	OutputTokens    int64  `json:"output_tokens"`
	CacheReadTokens int64  `json:"cache_read_tokens"`
	FirstSeenAt     int64  `json:"first_seen_at"`
	LastSeenAt      int64  `json:"last_seen_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

type ingestWatermarks struct {
	HookLog            int64  `json:"hook_log"`
	AuditLog           int64  `json:"audit_log"`
	PassesLastSyncedAt string `json:"passes_last_synced_at"`
	Sessions           int64  `json:"sessions"`
}

type ingestRequest struct {
	HookLog    []ingestHookLogEntry `json:"hook_log"`
	AuditLog   []ingestAuditEntry   `json:"audit_log"`
	Passes     []ingestPass         `json:"passes"`
	Sessions   []ingestSession      `json:"sessions"`
	Watermarks ingestWatermarks     `json:"watermarks"`
}

type ingestResponse struct {
	Accepted struct {
		HookLog  int `json:"hook_log"`
		AuditLog int `json:"audit_log"`
		Passes   int `json:"passes"`
		Sessions int `json:"sessions"`
	} `json:"accepted"`
	ChainStatus struct {
		HookLog  string `json:"hook_log"`
		AuditLog string `json:"audit_log"`
	} `json:"chain_status"`
	NotificationsTriggered int `json:"notifications_triggered"`
}

// ingestBatchSize is the maximum number of entries per table per ingest POST.
// If any table has more entries than this, multiple POSTs are made with
// watermarks advancing between each batch.
const ingestBatchSize = 1000

// syncDataPush pushes hook_log, audit_log, passes, and sessions since the last watermarks.
// Data is sent in batches of up to ingestBatchSize entries per table per request.
// The loop continues until all tables are fully drained.
func syncDataPush(dataDB *sql.DB, client *api.Client, perimeterID string) (int, error) {
	totalPushed := 0

	for {
		hookWM, err := store.GetWatermark(dataDB, "hook_log")
		if err != nil {
			return totalPushed, err
		}
		auditWM, err := store.GetWatermark(dataDB, "audit_log")
		if err != nil {
			return totalPushed, err
		}
		passesWM, err := store.GetWatermark(dataDB, "passes")
		if err != nil {
			return totalPushed, err
		}
		sessionsWM, err := store.GetWatermark(dataDB, "sessions")
		if err != nil {
			return totalPushed, err
		}

		hookEntries, hookMax, err := store.HookLogEntriesSince(dataDB, hookWM, ingestBatchSize)
		if err != nil {
			return totalPushed, err
		}
		auditEntries, auditMax, err := store.AuditEntriesSince(dataDB, auditWM, ingestBatchSize)
		if err != nil {
			return totalPushed, err
		}
		passes, passMax, err := store.PassesSince(dataDB, passesWM, ingestBatchSize)
		if err != nil {
			return totalPushed, err
		}
		sessions, sessionsMax, err := store.SessionsSince(dataDB, sessionsWM, ingestBatchSize)
		if err != nil {
			return totalPushed, err
		}

		batchTotal := len(hookEntries) + len(auditEntries) + len(passes) + len(sessions)
		if batchTotal == 0 {
			break
		}

		// Convert to spec-shaped structs.
		hookItems := make([]ingestHookLogEntry, len(hookEntries))
		for i, e := range hookEntries {
			hookItems[i] = ingestHookLogEntry{
				ID:                   e.ID,
				Ts:                   e.Ts,
				ToolName:             e.ToolName,
				FilePath:             e.FilePath,
				ToolInput:            e.ToolInput,
				CommandRaw:           e.CommandRaw,
				CommandParsed:        e.CommandParsed,
				CommandParseError:    e.CommandParseError,
				CommandParser:        e.CommandParser,
				CommandParserVersion: e.CommandParserVersion,
				CommandOpsJSON:       e.CommandOpsJSON,
				DeniedOpIndex:        e.DeniedOpIndex,
				DeniedOpReason:       e.DeniedOpReason,
				MatchedRulePattern:   e.MatchedRulePattern,
				MatchedRuleType:      e.MatchedRuleType,
				Ambiguity:            e.Ambiguity,
				Decision:             e.Decision,
				OSUser:               e.OSUser,
				Agent:                e.Agent,
				PassID:               e.PassID,
				Notify:               e.Notify,
				SessionID:            e.SessionID,
				TranscriptPath:       e.TranscriptPath,
				ParentHash:           e.ParentHash,
				Hash:                 e.Hash,
			}
		}

		auditItems := make([]ingestAuditEntry, len(auditEntries))
		for i, e := range auditEntries {
			auditItems[i] = ingestAuditEntry{
				ID:         e.ID,
				EventType:  e.EventType,
				FilePath:   e.FilePath,
				User:       e.User,
				Detail:     e.Detail,
				Timestamp:  e.Timestamp,
				ParentHash: e.ParentHash,
				Hash:       e.Hash,
			}
		}

		passItems := make([]ingestPass, len(passes))
		for i, p := range passes {
			passItems[i] = ingestPass{
				ID:         p.ID,
				FileRuleID: p.FileRuleID,
				Pattern:    p.Pattern,
				Status:     p.Status,
				IssuedTo:   p.IssuedTo,
				IssuedBy:   p.IssuedBy,
				IssuedAt:   p.IssuedAt,
				ExpiresAt:  p.ExpiresAt,
			}
		}

		sessionItems := make([]ingestSession, len(sessions))
		for i, s := range sessions {
			sessionItems[i] = ingestSession{
				SessionID:       s.SessionID,
				Agent:           s.Agent,
				Description:     s.Description,
				TranscriptPath:  s.TranscriptPath,
				InputTokens:     s.InputTokens,
				OutputTokens:    s.OutputTokens,
				CacheReadTokens: s.CacheReadTokens,
				FirstSeenAt:     s.FirstSeenAt,
				LastSeenAt:      s.LastSeenAt,
				UpdatedAt:       s.UpdatedAt,
			}
		}

		// Watermarks: the new high-water marks for this batch.
		newHookWM := hookWM
		if hookMax > 0 {
			newHookWM = hookMax
		}
		newAuditWM := auditWM
		if auditMax > 0 {
			newAuditWM = auditMax
		}
		newSessionsWM := sessionsWM
		if sessionsMax > 0 {
			newSessionsWM = sessionsMax
		}

		var resp ingestResponse
		_, err = client.PostJSON(
			fmt.Sprintf("/api/v1/perimeters/%s/data/ingest", perimeterID),
			ingestRequest{
				HookLog:  hookItems,
				AuditLog: auditItems,
				Passes:   passItems,
				Sessions: sessionItems,
				Watermarks: ingestWatermarks{
					HookLog:            newHookWM,
					AuditLog:           newAuditWM,
					PassesLastSyncedAt: time.Now().UTC().Format(time.RFC3339),
					Sessions:           newSessionsWM,
				},
			},
			&resp,
		)
		if err != nil {
			return totalPushed, err
		}

		// Update local watermarks on success.
		if len(hookEntries) > 0 {
			if err := store.SetWatermark(dataDB, "hook_log", hookMax); err != nil {
				return totalPushed, err
			}
		}
		if len(auditEntries) > 0 {
			if err := store.SetWatermark(dataDB, "audit_log", auditMax); err != nil {
				return totalPushed, err
			}
		}
		if len(passes) > 0 {
			if err := store.SetWatermark(dataDB, "passes", passMax); err != nil {
				return totalPushed, err
			}
		}
		if len(sessions) > 0 {
			if err := store.SetWatermark(dataDB, "sessions", sessionsMax); err != nil {
				return totalPushed, err
			}
		}

		totalPushed += batchTotal

		// If no table hit the batch limit, all data has been drained.
		if len(hookEntries) < ingestBatchSize && len(auditEntries) < ingestBatchSize &&
			len(passes) < ingestBatchSize && len(sessions) < ingestBatchSize {
			break
		}
	}

	return totalPushed, nil
}
