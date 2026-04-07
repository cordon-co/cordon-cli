package updatecheck

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCheckedWithin24Hours(t *testing.T) {
	now := time.Date(2026, 4, 7, 12, 0, 0, 0, time.UTC)
	if !checkedWithin24Hours(now.Add(-2*time.Hour).Format(time.RFC3339), now) {
		t.Fatalf("expected recent check to be considered within 24h")
	}
	if checkedWithin24Hours(now.Add(-25*time.Hour).Format(time.RFC3339), now) {
		t.Fatalf("expected stale check to be outside 24h")
	}
	if checkedWithin24Hours("bad", now) {
		t.Fatalf("expected invalid timestamp to be treated as stale")
	}
}

func TestIsDifferentVersion(t *testing.T) {
	if !isDifferentVersion("v0.1.0", "v0.2.0") {
		t.Fatalf("expected differing versions to be detected")
	}
	if isDifferentVersion("v0.2.0", "0.2.0") {
		t.Fatalf("expected equivalent versions to match")
	}
	if isDifferentVersion("dev", "v0.2.0") {
		t.Fatalf("dev builds should skip update prompts")
	}
}

func TestFetchLatestReleaseTagFromRedirect(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/cordon-co/cordon-cli/releases/tag/v1.2.3", http.StatusFound)
	}))
	defer ts.Close()

	tag, err := fetchLatestReleaseTagFromURL(http.DefaultClient, ts.URL)
	if err != nil {
		t.Fatalf("fetchLatestReleaseTagFromURL() error = %v", err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("tag = %q, want v1.2.3", tag)
	}
}

func TestWriteConfigPreservesUnknownFields(t *testing.T) {
	tmp := t.TempDir()
	p := filepath.Join(tmp, "config.json")
	data := []byte(`{"api_url":"https://api.cordon.sh","skip_update_check":false}`)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	cfg, raw, err := readConfig(p)
	if err != nil {
		t.Fatalf("readConfig: %v", err)
	}
	cfg.LastUpdateCheck = "2026-04-07T00:00:00Z"
	if err := writeConfig(p, cfg, raw); err != nil {
		t.Fatalf("writeConfig: %v", err)
	}

	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, `"api_url": "https://api.cordon.sh"`) {
		t.Fatalf("expected api_url to be preserved, got: %s", text)
	}
	if !strings.Contains(text, `"last_update_check": "2026-04-07T00:00:00Z"`) {
		t.Fatalf("expected last_update_check to be written, got: %s", text)
	}
}

func TestReadYesNo(t *testing.T) {
	yes, err := readYesNo(strings.NewReader("\n"))
	if err != nil {
		t.Fatalf("readYesNo empty: %v", err)
	}
	if !yes {
		t.Fatalf("empty answer should default to yes")
	}
	no, err := readYesNo(strings.NewReader("n\n"))
	if err != nil {
		t.Fatalf("readYesNo n: %v", err)
	}
	if no {
		t.Fatalf("n should be treated as no")
	}
}

func TestMaybeRunDevVersionDoesNotWriteConfig(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	var out bytes.Buffer
	var errOut bytes.Buffer
	MaybeRun(strings.NewReader(""), &out, &errOut, "dev")

	p := filepath.Join(tmpHome, ".cordon", "config.json")
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("expected no config write for dev version")
	}
}
