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

// ErrDenied is returned by Evaluate when the hook decision is "deny".
// cmd/hook.go checks for this sentinel and calls os.Exit(2).
var ErrDenied = errors.New("cordon: write denied")

// Decision is the outcome of a hook evaluation.
type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

// Event is returned by Evaluate for every tool invocation (writing or not).
// It carries all fields needed for audit logging.
type Event struct {
	ToolName  string
	FilePath  string          // may be empty for tools with no file path (e.g. Bash)
	ToolInput json.RawMessage // full raw tool_input JSON from the hook payload
	Decision  Decision
	Cwd       string // cwd from the hook payload; used by the logger for DB path discovery
}

// writingTools is the set of tool names that constitute write operations and
// are subject to zone enforcement. Non-writing tools are always allowed but
// still logged.
// VS Code fires hooks on all tools regardless of matcher; this map prevents
// non-writing tools from being denied.
var writingTools = map[string]bool{
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
	"Delete":       true,
}

// hookPayload is the JSON structure sent by Claude Code via stdin.
// Claude Code also sends session_id, transcript_path, hook_event_name, etc.;
// those fields are ignored here (unknown fields are discarded by encoding/json).
type hookPayload struct {
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
	Cwd       string          `json:"cwd"` // agent working directory; equals repo root
}

// toolInputPath extracts the file path from a tool's input JSON.
// Claude Code uses "file_path" for Write/Edit/Read etc.
// Some tools or future formats may use "path" instead.
type toolInputPath struct {
	FilePath string `json:"file_path"` // Claude Code native field name
	Path     string `json:"path"`      // fallback / alternative field name
}

func (t toolInputPath) effectivePath() string {
	if t.FilePath != "" {
		return t.FilePath
	}
	return t.Path
}

// Evaluate reads a PreToolUse JSON payload from r, determines whether the
// operation should be allowed or denied, writes the deny response to w if
// blocked, and returns an Event for every invocation for audit logging.
//
// Return values:
//   - event, nil error      → allowed (exit 0); event carries the log data
//   - event, ErrDenied      → denied; JSON written to w; caller must exit 2
//   - nil, other error      → malformed payload or IO error; caller should exit 1
//
// Evaluate never calls os.Exit itself, making it fully testable.
func Evaluate(r io.Reader, w io.Writer) (*Event, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("hook: read stdin: %w", err)
	}

	var payload hookPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("hook: parse payload: %w", err)
	}

	// Extract the file path; tolerate missing/non-path tools gracefully.
	var inp toolInputPath
	if len(payload.ToolInput) > 0 {
		// Ignore parse errors — not all tools have a path field.
		_ = json.Unmarshal([]byte(payload.ToolInput), &inp)
	}
	filePath := inp.effectivePath()

	// Non-writing tools: allow, log the invocation, no deny response.
	if !writingTools[payload.ToolName] {
		return &Event{
			ToolName:  payload.ToolName,
			FilePath:  filePath,
			ToolInput: payload.ToolInput,
			Decision:  DecisionAllow,
			Cwd:       payload.Cwd,
		}, nil
	}

	// TODO: check filePath against policy database (zones + passes).
	// For now, deny all writes.
	event := &Event{
		ToolName:  payload.ToolName,
		FilePath:  filePath,
		ToolInput: payload.ToolInput,
		Decision:  DecisionDeny,
		Cwd:       payload.Cwd,
	}

	if err := writeDeny(w, filePath); err != nil {
		return nil, err
	}
	return event, ErrDenied
}

func writeDeny(w io.Writer, path string) error {
	type denyResponse struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}

	reason := fmt.Sprintf(
		"cordon: write to %s is not permitted. Request a pass with: cordon pass issue --file %s",
		path, path,
	)
	if path == "" {
		reason = "cordon: write is not permitted (could not determine file path)"
	}

	resp := denyResponse{Decision: "block", Reason: reason}

	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(resp)
}
