package transcript

import (
	"encoding/json"
	"os"
)

// extractGemini parses a Gemini CLI JSON transcript.
//
// The file is a single JSON object with a messages array. The last message
// with type:"gemini" contains a running tally in its tokens object.
func extractGemini(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var transcript geminiTranscript
	if err := json.Unmarshal(data, &transcript); err != nil {
		return nil, err
	}

	// Walk backwards to find the last gemini message (running tally).
	for i := len(transcript.Messages) - 1; i >= 0; i-- {
		msg := transcript.Messages[i]
		if msg.Type == "gemini" && msg.Tokens != nil {
			return &Result{
				InputTokens:     msg.Tokens.Input,
				OutputTokens:    msg.Tokens.Output + msg.Tokens.Thoughts,
				CacheReadTokens: msg.Tokens.Cached,
			}, nil
		}
	}

	return &Result{}, nil
}

type geminiTranscript struct {
	Messages []geminiMessage `json:"messages"`
}

type geminiMessage struct {
	Type   string       `json:"type"`
	Tokens *geminiTokens `json:"tokens"`
}

type geminiTokens struct {
	Input    int64 `json:"input"`
	Output   int64 `json:"output"`
	Cached   int64 `json:"cached"`
	Thoughts int64 `json:"thoughts"`
}
