// Package transcript extracts token usage and session metadata from agent
// transcript files. Each agent stores transcripts in a different format;
// this package provides a unified Extract interface that dispatches to
// agent-specific parsers.
package transcript

import (
	"errors"
	"os"
)

// Result holds the extracted data from a transcript file.
// Token fields are unified across agents:
//   - InputTokens:     total input context (includes cached tokens)
//   - OutputTokens:    total generated tokens (includes thoughts for Gemini)
//   - CacheReadTokens: portion of input that came from cache
type Result struct {
	InputTokens     int64
	OutputTokens    int64
	CacheReadTokens int64
	Description     string // e.g. Claude's aiTitle
}

// Extract reads the transcript file at path and extracts token usage and
// session description. The agent string determines which parser to use.
//
// Returns (nil, nil) if the agent is unsupported or the file doesn't exist.
func Extract(path, agent string) (*Result, error) {
	if path == "" {
		return nil, nil
	}

	// Check file exists before dispatching — missing transcripts are not errors.
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	switch agent {
	case "claude-code":
		return extractClaude(path)
	case "codex":
		return extractCodex(path)
	case "gemini-cli":
		return extractGemini(path)
	default:
		return nil, nil
	}
}
