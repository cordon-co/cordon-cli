package transcript

import (
	"bufio"
	"encoding/json"
	"os"
)

// extractCodex parses a Codex JSONL transcript.
//
// It scans for event_msg lines with payload.type:"token_count". The last
// such line contains the running total — last match wins.
func extractCodex(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var r Result
	found := false

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry codexEntry
		if json.Unmarshal(line, &entry) != nil {
			continue
		}

		if entry.Type == "event_msg" && entry.Payload.Type == "token_count" {
			u := entry.Payload.Info.TotalTokenUsage
			r.InputTokens = u.InputTokens
			r.OutputTokens = u.OutputTokens
			r.CacheReadTokens = u.CachedInputTokens
			found = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if !found {
		return &Result{}, nil
	}
	return &r, nil
}

type codexEntry struct {
	Type    string       `json:"type"`
	Payload codexPayload `json:"payload"`
}

type codexPayload struct {
	Type string    `json:"type"`
	Info codexInfo `json:"info"`
}

type codexInfo struct {
	TotalTokenUsage codexTokenUsage `json:"total_token_usage"`
}

type codexTokenUsage struct {
	InputTokens       int64 `json:"input_tokens"`
	CachedInputTokens int64 `json:"cached_input_tokens"`
	OutputTokens      int64 `json:"output_tokens"`
}
