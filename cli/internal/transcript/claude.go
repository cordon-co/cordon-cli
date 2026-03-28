package transcript

import (
	"bufio"
	"encoding/json"
	"os"
)

// extractClaude parses a Claude Code JSONL transcript.
//
// Each line is a JSON object. Lines with a nested message containing
// role:"assistant" have a usage object — we sum all of them (Claude does not
// provide a running total). Lines with type:"ai-title" provide the session
// description.
func extractClaude(path string) (*Result, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var r Result
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024) // handle long lines

	for scanner.Scan() {
		line := scanner.Bytes()

		var entry claudeEntry
		if json.Unmarshal(line, &entry) != nil {
			continue // skip malformed lines
		}

		// Extract aiTitle description.
		if entry.Type == "ai-title" && entry.AITitle != "" {
			r.Description = entry.AITitle
		}

		// Sum usage from assistant messages.
		// Claude reports input_tokens as only new/uncached tokens. Total input
		// context = input_tokens + cache_creation + cache_read.
		if entry.Message.Role == "assistant" && entry.Message.Usage != nil {
			u := entry.Message.Usage
			r.InputTokens += u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
			r.OutputTokens += u.OutputTokens
			r.CacheReadTokens += u.CacheReadInputTokens
		}
	}

	return &r, scanner.Err()
}

type claudeEntry struct {
	Type    string       `json:"type"`
	AITitle string       `json:"aiTitle"`
	Message claudeMessage `json:"message"`
}

type claudeMessage struct {
	Role  string       `json:"role"`
	Usage *claudeUsage `json:"usage"`
}

type claudeUsage struct {
	InputTokens              int64 `json:"input_tokens"`
	OutputTokens             int64 `json:"output_tokens"`
	CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}
