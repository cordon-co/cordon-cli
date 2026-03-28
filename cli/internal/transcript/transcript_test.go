package transcript

import "testing"

func TestExtract_UnsupportedAgent(t *testing.T) {
	r, err := Extract("/some/path", "cursor")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if r != nil {
		t.Errorf("expected nil result for unsupported agent, got %+v", r)
	}
}

func TestExtract_EmptyPath(t *testing.T) {
	r, err := Extract("", "claude-code")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if r != nil {
		t.Errorf("expected nil result for empty path, got %+v", r)
	}
}

func TestExtract_MissingFile(t *testing.T) {
	r, err := Extract("/nonexistent/transcript.jsonl", "claude-code")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if r != nil {
		t.Errorf("expected nil result for missing file, got %+v", r)
	}
}
