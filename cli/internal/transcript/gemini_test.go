package transcript

import "testing"

func TestExtractGemini(t *testing.T) {
	transcript := `{
  "sessionId": "8e6cdfdc-a201-4f48-aea6-266fde3d5935",
  "messages": [
    {
      "id": "msg1",
      "type": "user",
      "content": [{"text": "list the files"}]
    },
    {
      "id": "msg2",
      "type": "gemini",
      "content": "Listing files.",
      "tokens": {"input": 4000, "output": 10, "cached": 0, "thoughts": 0, "tool": 0, "total": 4010}
    },
    {
      "id": "msg3",
      "type": "gemini",
      "content": "Done.",
      "tokens": {"input": 8106, "output": 24, "cached": 100, "thoughts": 50, "tool": 0, "total": 8180}
    }
  ]
}`
	path := writeTemp(t, "gemini.json", transcript)

	r, err := extractGemini(path)
	if err != nil {
		t.Fatalf("extractGemini: %v", err)
	}

	// Last gemini message wins (running tally).
	// OutputTokens includes thoughts: 24 + 50 = 74.
	assertEq(t, "InputTokens", r.InputTokens, 8106)
	assertEq(t, "OutputTokens", r.OutputTokens, 74)
	assertEq(t, "CacheReadTokens", r.CacheReadTokens, 100)
}

func TestExtractGemini_NoGeminiMessages(t *testing.T) {
	transcript := `{"messages": [{"type": "user", "content": [{"text": "hi"}]}]}`
	path := writeTemp(t, "gemini-empty.json", transcript)

	r, err := extractGemini(path)
	if err != nil {
		t.Fatalf("extractGemini: %v", err)
	}
	assertEq(t, "InputTokens", r.InputTokens, 0)
}
