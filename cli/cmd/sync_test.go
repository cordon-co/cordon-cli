package cmd

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cordon-co/cordon-cli/cli/internal/api"
	"github.com/cordon-co/cordon-cli/cli/internal/store"
	_ "modernc.org/sqlite"
)

func openCmdTestDataDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateDataDB(db); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSyncDataPush_IncludesSecretFields(t *testing.T) {
	db := openCmdTestDataDB(t)
	if err := store.InsertHookLog(db, store.HookLogEntry{
		Ts:              1000,
		ToolName:        "Write",
		FilePath:        "secret.txt",
		ToolInput:       `{"content":"<SECRET:github-pat>"}`,
		Decision:        "allow",
		OSUser:          "tester",
		SecretsDetected: true,
		SecretRuleIDs:   `["github-pat"]`,
	}); err != nil {
		t.Fatal(err)
	}

	var got ingestRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode ingest request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"accepted":{"hook_log":1,"audit_log":0,"passes":0,"sessions":0},"chain_status":{"hook_log":"ok","audit_log":"ok"},"notifications_triggered":0}`))
	}))
	defer srv.Close()

	client := &api.Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	pushed, err := syncDataPush(db, client, "perim-1", "client-1")
	if err != nil {
		t.Fatalf("syncDataPush: %v", err)
	}
	if pushed != 1 {
		t.Fatalf("pushed = %d, want 1", pushed)
	}
	if got.HookLog == nil || len(*got.HookLog) != 1 {
		l := 0
		if got.HookLog != nil {
			l = len(*got.HookLog)
		}
		t.Fatalf("hook_log length = %d, want 1", l)
	}
	if (*got.HookLog)[0].SecretsDetected == nil || *(*got.HookLog)[0].SecretsDetected != 1 {
		t.Fatalf("secrets_detected = %v, want 1", (*got.HookLog)[0].SecretsDetected)
	}
	if (*got.HookLog)[0].SecretRuleIds == nil || *(*got.HookLog)[0].SecretRuleIds != `["github-pat"]` {
		t.Fatalf("secret_rule_ids = %v, want [\"github-pat\"]", (*got.HookLog)[0].SecretRuleIds)
	}
}
