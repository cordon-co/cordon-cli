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

	// Bash tool: check whether the command targets any files via shell write patterns.
	if payload.ToolName == "Bash" {
		return evaluateBash(payload, w)
	}

	// Extract the file path; tolerate missing/non-path tools gracefully.
	var inp toolInputPath
	if len(payload.ToolInput) > 0 {
		// Ignore parse errors — not all tools have a path field.
		_ = json.Unmarshal([]byte(payload.ToolInput), &inp)
	}
	filePath := inp.effectivePath()

	// Non-writing tools: allow and log; no deny response.
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

// evaluateBash handles the Bash tool. It parses the command string for shell
// write patterns (redirections, tee, sed -i, cp, mv). If any write target is
// detected the command is denied; otherwise it is allowed and logged.
func evaluateBash(payload hookPayload, w io.Writer) (*Event, error) {
	command := parseBashToolInput(payload.ToolInput)
	targets := bashWriteTargets(command)

	// No write pattern detected — allow.
	if len(targets) == 0 {
		return &Event{
			ToolName:  payload.ToolName,
			FilePath:  "",
			ToolInput: payload.ToolInput,
			Decision:  DecisionAllow,
			Cwd:       payload.Cwd,
		}, nil
	}

	// TODO: check each target against the policy database (zones + passes).
	// For now, deny any bash command that writes to any file.
	primaryTarget := targets[0]
	event := &Event{
		ToolName:  payload.ToolName,
		FilePath:  primaryTarget,
		ToolInput: payload.ToolInput,
		Decision:  DecisionDeny,
		Cwd:       payload.Cwd,
	}

	if err := writeBashDeny(w, primaryTarget, targets); err != nil {
		return nil, err
	}
	return event, ErrDenied
}

// policyDenyReason returns the denial reason string for a direct file write.
func policyDenyReason(path string) string {
	passCmd := "cordon pass issue --file " + path
	if path == "" {
		path = "this file"
		passCmd = "cordon pass issue --file <filepath>"
	}
	return fmt.Sprintf(
		"CORDON POLICY: %s is protected by a Cordon zone policy. "+
			"Do not attempt to write to this file through any alternative method, "+
			"including shell commands such as echo, sed, tee, cp, mv, or any other approach. "+
			"This is an enforced policy restriction, not a technical error. "+
			"If you need to modify this file, request access using the cordon_request_access MCP tool "+
			"or ask the user to run: %s",
		path, passCmd,
	)
}

// policyBashDenyReason returns the denial reason for a bash command that
// targets one or more protected files.
func policyBashDenyReason(primary string, all []string) string {
	target := primary
	if target == "" {
		target = "a protected file"
	}
	passCmd := "cordon pass issue --file " + primary
	if primary == "" {
		passCmd = "cordon pass issue --file <filepath>"
	}
	return fmt.Sprintf(
		"CORDON POLICY: This command would write to %s which is protected by a Cordon zone policy. "+
			"Do not attempt to write to this file through any alternative method, "+
			"including shell commands such as echo, sed, tee, cp, mv, or any other approach. "+
			"This is an enforced policy restriction, not a technical error. "+
			"If you need to modify this file, request access using the cordon_request_access MCP tool "+
			"or ask the user to run: %s",
		target, passCmd,
	)
}

func writeDeny(w io.Writer, path string) error {
	return encodedeny(w, policyDenyReason(path))
}

func writeBashDeny(w io.Writer, primary string, all []string) error {
	return encodedeny(w, policyBashDenyReason(primary, all))
}

func encodedeny(w io.Writer, reason string) error {
	type denyResponse struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(denyResponse{Decision: "block", Reason: reason})
}
