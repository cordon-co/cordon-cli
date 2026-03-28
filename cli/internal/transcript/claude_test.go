package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractClaude(t *testing.T) {
	// Two assistant messages with usage + an aiTitle line.
	transcript := `{"parentUuid":"a","isSidechain":false,"message":{"model":"claude-sonnet-4-6","id":"msg_01","type":"message","role":"assistant","content":[{"type":"text","text":"hello"}],"stop_reason":"end_turn","usage":{"input_tokens":100,"output_tokens":50,"cache_read_input_tokens":200,"cache_creation_input_tokens":30}}}
{"type":"ai-title","sessionId":"sess-1","aiTitle":"List repository files with ls command"}
{"parentUuid":"b","isSidechain":false,"message":{"model":"claude-sonnet-4-6","id":"msg_02","type":"message","role":"assistant","content":[{"type":"text","text":"done"}],"stop_reason":"end_turn","usage":{"input_tokens":150,"output_tokens":60,"cache_read_input_tokens":300,"cache_creation_input_tokens":10}}}
`
	path := writeTemp(t, "claude.jsonl", transcript)

	r, err := extractClaude(path)
	if err != nil {
		t.Fatalf("extractClaude: %v", err)
	}

	// Usage should be summed across both messages.
	// InputTokens = (100+30+200) + (150+10+300) = 330 + 460 = 790
	assertEq(t, "InputTokens", r.InputTokens, 790)
	assertEq(t, "OutputTokens", r.OutputTokens, 110)
	// CacheReadTokens = 200 + 300 = 500
	assertEq(t, "CacheReadTokens", r.CacheReadTokens, 500)
	if r.Description != "List repository files with ls command" {
		t.Errorf("Description = %q, want %q", r.Description, "List repository files with ls command")
	}
}

func TestExtractClaude_Empty(t *testing.T) {
	path := writeTemp(t, "empty.jsonl", "")
	r, err := extractClaude(path)
	if err != nil {
		t.Fatalf("extractClaude: %v", err)
	}
	assertEq(t, "InputTokens", r.InputTokens, 0)
}

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func assertEq(t *testing.T, field string, got, want int64) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %d, want %d", field, got, want)
	}
}
