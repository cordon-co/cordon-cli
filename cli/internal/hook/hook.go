// Package hook implements PreToolUse hook evaluation for cordon.
// It parses the JSON payload sent by Claude Code and VS Code agents and
// writes an allow or deny decision.
package hook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ErrDenied is returned by Evaluate when the hook decision is "block".
// cmd/hook.go checks for this sentinel and calls os.Exit(2).
var ErrDenied = errors.New("cordon: write denied")

// writingTools is the set of tool names that constitute write operations.
// Non-writing tools (Read, Bash, etc.) are passed through silently.
// VS Code fires hooks on all tools regardless of matcher; this guards against that.
var writingTools = map[string]bool{
	"Write":     true,
	"Edit":      true,
	"MultiEdit": true,
}

type hookPayload struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type toolInputPath struct {
	Path string `json:"path"`
}

// Evaluate reads a PreToolUse JSON payload from r, determines whether the
// operation should be allowed or denied, and writes the deny response to w if
// blocked.
//
// Return values:
//   - nil           → allow (exit 0)
//   - ErrDenied     → deny written to w; caller must exit 2
//   - other error   → malformed payload or IO error; caller should exit 1
//
// Evaluate never calls os.Exit itself, making it fully testable.
func Evaluate(r io.Reader, w io.Writer) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("hook: read stdin: %w", err)
	}

	var payload hookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("hook: parse payload: %w", err)
	}

	// Pass through non-writing tools silently (exit 0).
	if !writingTools[payload.ToolName] {
		return nil
	}

	var inp toolInputPath
	if err := json.Unmarshal([]byte(payload.ToolInput), &inp); err != nil {
		return fmt.Errorf("hook: parse tool_input: %w", err)
	}
	if inp.Path == "" {
		return fmt.Errorf("hook: tool_input.path is empty")
	}

	if err := writeDeny(w, inp.Path); err != nil {
		return err
	}
	return ErrDenied
}

func writeDeny(w io.Writer, path string) error {
	type denyResponse struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}

	resp := denyResponse{
		Decision: "block",
		Reason: fmt.Sprintf(
			"cordon: write to %s is not permitted. Request a pass with: cordon pass issue --file %s",
			path, path,
		),
	}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(resp)
}
