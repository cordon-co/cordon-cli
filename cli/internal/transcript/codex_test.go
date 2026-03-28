package transcript

import "testing"

func TestExtractCodex(t *testing.T) {
	transcript := `{"timestamp":"2026-03-27T23:38:00.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":50000,"cached_input_tokens":40000,"output_tokens":100,"reasoning_output_tokens":0,"total_tokens":50100}}}}
{"timestamp":"2026-03-27T23:39:03.834Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":98336,"cached_input_tokens":86656,"output_tokens":242,"reasoning_output_tokens":0,"total_tokens":98578}}}}
`
	path := writeTemp(t, "codex.jsonl", transcript)

	r, err := extractCodex(path)
	if err != nil {
		t.Fatalf("extractCodex: %v", err)
	}

	// Last event_msg wins (running total).
	assertEq(t, "InputTokens", r.InputTokens, 98336)
	assertEq(t, "OutputTokens", r.OutputTokens, 242)
	assertEq(t, "CacheReadTokens", r.CacheReadTokens, 86656)
}

func TestExtractCodex_NoTokenCount(t *testing.T) {
	transcript := `{"timestamp":"2026-03-27T23:38:00.000Z","type":"event_msg","payload":{"type":"something_else"}}
`
	path := writeTemp(t, "codex-empty.jsonl", transcript)

	r, err := extractCodex(path)
	if err != nil {
		t.Fatalf("extractCodex: %v", err)
	}
	assertEq(t, "InputTokens", r.InputTokens, 0)
}
