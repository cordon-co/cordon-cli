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
	"github.com/cordon-co/cordon-cli/cli/internal/apicontract"
	"github.com/cordon-co/cordon-cli/cli/internal/flags"
	"github.com/cordon-co/cordon-cli/cli/internal/policysync"
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
	defer func() {
		if err := lockFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "sync: close lock file: %v\n", err)
		}
	}()

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
	pid, ok, err := policysync.LookupPerimeter(client, perimeterID)
	if err != nil {
		return nil, mapPerimeterLookupError(err)
	}
	if !ok {
		return nil, fmt.Errorf("this repository is not registered in your Cordon dashboard")
	}

	// --- Policy Pull ---
	pulled, err := policysync.PullEvents(policyDB, client, pid)
	if err != nil {
		return nil, fmt.Errorf("policy pull: %w", err)
	}

	// --- Policy Push ---
	pushed, err := syncPolicyPush(policyDB, client, pid)
	if err != nil {
		return nil, fmt.Errorf("policy push: %w", err)
	}

	// --- Policy Pull (final reconciliation) ---
	finalPulled, err := policysync.PullEvents(policyDB, client, pid)
	if err != nil {
		return nil, fmt.Errorf("policy pull after push: %w", err)
	}
	pulled += finalPulled

	// --- Data Push ---
	clientID, err := api.EnsureClientID()
	if err != nil {
		return nil, fmt.Errorf("resolve client id: %w", err)
	}

	dataDB, err := store.OpenDataDB(absRoot)
	if err != nil {
		return nil, fmt.Errorf("open data db: %w", err)
	}
	defer dataDB.Close()

	if err := store.MigrateDataDB(dataDB); err != nil {
		return nil, fmt.Errorf("migrate data db: %w", err)
	}

	dataPushed, err := syncDataPush(dataDB, client, pid, clientID)
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

func mapPerimeterLookupError(err error) error {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == 402 {
		return fmt.Errorf("repository access requires an active paid Cordon plan for this account")
	}
	return fmt.Errorf("perimeter lookup: %w", err)
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

type policyPushRequest = apicontract.PolicyPushRequest
type policyPushResponse = apicontract.PolicyPushResponse

func pushEvents(policyDB *sql.DB, client *api.Client, perimeterID string, events []store.PolicyEvent) (int, error) {
	maxSeq, err := store.MaxServerSeq(policyDB)
	if err != nil {
		return 0, err
	}

	var resp policyPushResponse
	wireEvents := make([]apicontract.PolicyEvent, 0, len(events))
	for _, e := range events {
		wireEvents = append(wireEvents, apicontract.PolicyEvent{
			EventId:    e.EventID,
			EventType:  e.EventType,
			Payload:    e.Payload,
			Actor:      e.Actor,
			Timestamp:  e.Timestamp,
			ServerSeq:  e.ServerSeq,
		})
	}
	_, err = client.PostJSON(
		fmt.Sprintf("/api/v1/perimeters/%s/policy/events", perimeterID),
		policyPushRequest{Events: wireEvents, LastKnownServerSeq: maxSeq},
		&resp,
	)
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.Code == "events_behind" {
			// Pull first, then retry.
			if _, pullErr := policysync.PullEvents(policyDB, client, perimeterID); pullErr != nil {
				return 0, fmt.Errorf("pull before retry: %w", pullErr)
			}
			newUnpushed, err := store.ListUnpushedEvents(policyDB)
			if err != nil {
				return 0, err
			}
			if len(newUnpushed) == 0 {
				return 0, nil
			}
			newMaxSeq, err := store.MaxServerSeq(policyDB)
			if err != nil {
				return 0, err
			}
			_, err = client.PostJSON(
				fmt.Sprintf("/api/v1/perimeters/%s/policy/events", perimeterID),
				policyPushRequest{Events: wireEventsFromStore(newUnpushed), LastKnownServerSeq: newMaxSeq},
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
type ingestRequest = apicontract.DataIngestRequest
type ingestResponse = apicontract.DataIngestResponse

// ingestBatchSize is the maximum number of entries per table per ingest POST.
// If any table has more entries than this, multiple POSTs are made with
// watermarks advancing between each batch.
const ingestBatchSize = 1000

// syncDataPush pushes hook_log, audit_log, passes, and sessions since the last watermarks.
// Data is sent in batches of up to ingestBatchSize entries per table per request.
// The loop continues until all tables are fully drained.
func syncDataPush(dataDB *sql.DB, client *api.Client, perimeterID, clientID string) (int, error) {
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
		hookItems := make([]apicontract.HookLogEntry, len(hookEntries))
		for i, e := range hookEntries {
			secretsDetected := 0
			if e.SecretsDetected {
				secretsDetected = 1
			}
			hookItems[i] = apicontract.HookLogEntry{
				Id:                   e.ID,
				Ts:                   e.Ts,
				ToolName:             e.ToolName,
				FilePath:             e.FilePath,
				ToolInput:            ptr(e.ToolInput),
				CommandRaw:           ptr(e.CommandRaw),
				CommandParsedOk:      ptr(e.CommandParsed),
				CommandParseError:    ptr(e.CommandParseError),
				CommandParser:        ptr(e.CommandParser),
				CommandParserVersion: ptr(e.CommandParserVersion),
				CommandOpsJson:       ptr(e.CommandOpsJSON),
				DeniedOpIndex:        ptr(e.DeniedOpIndex),
				DeniedOpReason:       ptr(e.DeniedOpReason),
				MatchedRulePattern:   ptr(e.MatchedRulePattern),
				MatchedRuleType:      ptr(e.MatchedRuleType),
				Ambiguity:            ptr(e.Ambiguity),
				Decision:             e.Decision,
				OsUser:               nilIfEmpty(e.OSUser),
				Agent:                nilIfEmpty(e.Agent),
				PassId:               nilIfEmpty(e.PassID),
				Notify:               ptr(e.Notify),
				SessionId:            nilIfEmpty(e.SessionID),
				TranscriptPath:       nilIfEmpty(e.TranscriptPath),
				SecretsDetected:      ptr(secretsDetected),
				SecretRuleIds:        nilIfEmpty(e.SecretRuleIDs),
				ParentHash:           nilIfEmpty(e.ParentHash),
				Hash:                 nilIfEmpty(e.Hash),
			}
		}

		auditItems := make([]apicontract.AuditLogEntry, len(auditEntries))
		for i, e := range auditEntries {
			auditItems[i] = apicontract.AuditLogEntry{
				Id:         e.ID,
				EventType:  e.EventType,
				FilePath:   nilIfEmpty(e.FilePath),
				User:       nilIfEmpty(e.User),
				Detail:     nilIfEmpty(e.Detail),
				Timestamp:  mustParseRFC3339(e.Timestamp),
				ParentHash: nilIfEmpty(e.ParentHash),
				Hash:       nilIfEmpty(e.Hash),
			}
		}

		passItems := make([]apicontract.Pass, len(passes))
		for i, p := range passes {
			passItems[i] = apicontract.Pass{
				Id:         p.ID,
				FileRuleId: nilIfEmpty(p.FileRuleID),
				Pattern:    p.Pattern,
				Status:     p.Status,
				IssuedTo:   p.IssuedTo,
				IssuedBy:   p.IssuedBy,
				IssuedAt:   mustParseRFC3339(p.IssuedAt),
				ExpiresAt:  parseOptionalRFC3339(p.ExpiresAt),
			}
		}

		sessionItems := make([]apicontract.Session, len(sessions))
		for i, s := range sessions {
			sessionItems[i] = apicontract.Session{
				SessionId:       s.SessionID,
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
				ClientId: ptr(clientID),
				HookLog:  &hookItems,
				AuditLog: &auditItems,
				Passes:   &passItems,
				Sessions: &sessionItems,
				Watermarks: &apicontract.IngestWatermarks{
					HookLog:            ptr(newHookWM),
					AuditLog:           ptr(newAuditWM),
					PassesLastSyncedAt: ptr(time.Now().UTC()),
					Sessions:           ptr(newSessionsWM),
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

func wireEventsFromStore(events []store.PolicyEvent) []apicontract.PolicyEvent {
	out := make([]apicontract.PolicyEvent, 0, len(events))
	for _, e := range events {
		out = append(out, apicontract.PolicyEvent{
			EventId:    e.EventID,
			EventType:  e.EventType,
			Payload:    e.Payload,
			Actor:      e.Actor,
			Timestamp:  e.Timestamp,
			ServerSeq:  e.ServerSeq,
		})
	}
	return out
}

func mustParseRFC3339(v string) time.Time {
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t
	}
	return time.Now().UTC()
}

func parseOptionalRFC3339(v string) *time.Time {
	if v == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return &t
	}
	return nil
}

func nilIfEmpty(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func ptr[T any](v T) *T {
	return &v
}
