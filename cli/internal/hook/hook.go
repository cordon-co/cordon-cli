// Package hook implements PreToolUse hook evaluation for cordon.
// It parses the JSON payload sent by Claude Code and VS Code agents and
// writes an allow or deny decision.
package hook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
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

// PolicyChecker checks whether a write to filePath should be allowed.
//
// filePath is the file being written; cwd is the agent working directory
// (from the hook payload), which is used to locate the policy database.
//
// Return values:
//   - allowed=true,  passID=""    — file is not in any zone (allow)
//   - allowed=true,  passID="…"   — file is in a zone and covered by an active pass (allow)
//   - allowed=false, passID=""    — file is in a zone with no active pass (deny)
//
// On infrastructure errors (DB unreadable, etc.) the checker should return
// (true, "") to fail-open per Cordon's fail-open policy.
//
// A nil PolicyChecker causes all writes to be allowed (fail-open).
type PolicyChecker func(filePath, cwd string) (allowed bool, passID string)

// Event is returned by Evaluate for every tool invocation (writing or not).
// It carries all fields needed for audit logging.
type Event struct {
	ToolName  string
	FilePath  string          // may be empty for tools with no file path (e.g. Bash)
	ToolInput json.RawMessage // full raw tool_input JSON from the hook payload
	Decision  Decision
	PassID    string // non-empty if write was allowed via an active pass
	Cwd       string // cwd from the hook payload; used by the logger for DB path discovery
}

// writingTools is the set of tool names that constitute write operations and
// are subject to zone enforcement. Non-writing tools are always allowed but
// still logged.
// VS Code fires hooks on all tools regardless of matcher; this map prevents
// non-writing tools from being denied.
var writingTools = map[string]bool{
	// Claude Code
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
	"Delete":       true,
	// VS Code Copilot
	"apply_patch":     true,
	"create_file":     true,
	"edit":            true,
	"editFiles":       true,
	"editNotebook":    true,
	"createFile":      true,
	"createDirectory": true,
	"deleteFile":      true,
	"moveFile":        true,
	"renameFile":      true,
}

// copilotTools identifies tools that originate from VS Code Copilot.
// When denying these tools the response format differs from Claude Code.
var copilotTools = map[string]bool{
	"apply_patch":     true,
	"create_file":     true,
	"edit":            true,
	"editFiles":       true,
	"editNotebook":    true,
	"createFile":      true,
	"createDirectory": true,
	"deleteFile":      true,
	"moveFile":        true,
	"renameFile":      true,
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
// Different agents use different field names for the target file path.
type toolInputPath struct {
	FilePath    string `json:"file_path"`    // Claude Code (Write, Edit, etc.)
	Path        string `json:"path"`         // generic fallback
	Filename    string `json:"filename"`     // VS Code Copilot (create_file, etc.)
	Destination string `json:"destination"`  // VS Code Copilot (moveFile, renameFile)
	NewPath     string `json:"newPath"`      // VS Code Copilot (renameFile variant)
}

func (t toolInputPath) effectivePath() string {
	if t.FilePath != "" {
		return t.FilePath
	}
	if t.Path != "" {
		return t.Path
	}
	if t.Filename != "" {
		return t.Filename
	}
	if t.Destination != "" {
		return t.Destination
	}
	return t.NewPath
}

// Evaluate reads a PreToolUse JSON payload from r, determines whether the
// operation should be allowed or denied, writes the deny response to w
// (stdout) if blocked, and returns an Event for every invocation for audit
// logging. errW receives a plain-text denial message for agents (like VS Code
// Copilot) that read stderr rather than parsing the JSON on stdout.
//
// checker is consulted for every writing tool. Pass nil to allow all writes
// (fail-open behaviour, used when databases are unavailable).
//
// Return values:
//   - event, nil error      → allowed (exit 0); event carries the log data
//   - event, ErrDenied      → denied; JSON written to w; caller must exit 2
//   - nil, other error      → malformed payload or IO error; caller should exit 1
//
// Evaluate never calls os.Exit itself, making it fully testable.
func Evaluate(r io.Reader, w io.Writer, errW io.Writer, checker PolicyChecker) (*Event, error) {
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
		return evaluateBash(payload, w, errW, checker)
	}

	// apply_patch: file paths are embedded in the patch body, potentially multiple.
	if payload.ToolName == "apply_patch" {
		return evaluateApplyPatch(payload, w, errW, checker)
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

	// Check the file against the policy database (zones + passes).
	allowed, passID := checkPolicy(checker, filePath, payload.Cwd)

	if allowed {
		return &Event{
			ToolName:  payload.ToolName,
			FilePath:  filePath,
			ToolInput: payload.ToolInput,
			Decision:  DecisionAllow,
			PassID:    passID,
			Cwd:       payload.Cwd,
		}, nil
	}

	event := &Event{
		ToolName:  payload.ToolName,
		FilePath:  filePath,
		ToolInput: payload.ToolInput,
		Decision:  DecisionDeny,
		Cwd:       payload.Cwd,
	}
	if err := writeDeny(w, errW, payload.ToolName, filePath); err != nil {
		return nil, err
	}
	return event, ErrDenied
}

// evaluateBash handles the Bash tool. It parses the command string for shell
// write patterns (redirections, tee, sed -i, cp, mv). If any write target is
// detected the command is denied; otherwise it is allowed and logged.
func evaluateBash(payload hookPayload, w io.Writer, errW io.Writer, checker PolicyChecker) (*Event, error) {
	command := parseBashToolInput(payload.ToolInput)

	// Block agents from running the cordon CLI directly.
	if isCordonCommand(command) {
		event := &Event{
			ToolName:  payload.ToolName,
			FilePath:  "",
			ToolInput: payload.ToolInput,
			Decision:  DecisionDeny,
			Cwd:       payload.Cwd,
		}
		reason := "CORDON POLICY: Agents are not permitted to run the cordon command directly. " +
			"Only the Cordon MCP can be used to execute cordon commands by agents. " +
			"Do not attempt to bypass this restriction."
		if err := encodeClaudeDeny(w, reason); err != nil {
			return nil, err
		}
		fmt.Fprintf(errW, "%s\n", reason)
		return event, ErrDenied
	}

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

	// Check each target against the policy database. Deny if any target is in
	// a zone without an active pass. We deny on the first violation found.
	for _, target := range targets {
		allowed, _ := checkPolicy(checker, target, payload.Cwd)
		if !allowed {
			primaryTarget := targets[0]
			event := &Event{
				ToolName:  payload.ToolName,
				FilePath:  primaryTarget,
				ToolInput: payload.ToolInput,
				Decision:  DecisionDeny,
				Cwd:       payload.Cwd,
			}
			if err := writeBashDeny(w, errW, primaryTarget, targets); err != nil {
				return nil, err
			}
			return event, ErrDenied
		}
	}

	// All targets are clear — allow.
	return &Event{
		ToolName:  payload.ToolName,
		FilePath:  targets[0],
		ToolInput: payload.ToolInput,
		Decision:  DecisionAllow,
		Cwd:       payload.Cwd,
	}, nil
}

// evaluateApplyPatch handles VS Code Copilot's apply_patch tool.
// The patch body is in the "input" field and contains one or more file paths
// as "*** Update File: <path>" or "*** Add File: <path>" directives.
func evaluateApplyPatch(payload hookPayload, w io.Writer, errW io.Writer, checker PolicyChecker) (*Event, error) {
	targets := patchFileTargets(payload.ToolInput)

	if len(targets) == 0 {
		// Could not extract any paths — fail-open.
		return &Event{
			ToolName:  payload.ToolName,
			ToolInput: payload.ToolInput,
			Decision:  DecisionAllow,
			Cwd:       payload.Cwd,
		}, nil
	}

	for _, target := range targets {
		allowed, _ := checkPolicy(checker, target, payload.Cwd)
		if !allowed {
			event := &Event{
				ToolName:  payload.ToolName,
				FilePath:  target,
				ToolInput: payload.ToolInput,
				Decision:  DecisionDeny,
				Cwd:       payload.Cwd,
			}
			if err := writeDeny(w, errW, payload.ToolName, target); err != nil {
				return nil, err
			}
			return event, ErrDenied
		}
	}

	return &Event{
		ToolName:  payload.ToolName,
		FilePath:  targets[0],
		ToolInput: payload.ToolInput,
		Decision:  DecisionAllow,
		Cwd:       payload.Cwd,
	}, nil
}

// patchFileTargets extracts file paths from an apply_patch tool_input JSON.
// It looks for "*** Update File: <path>", "*** Add File: <path>", and
// "*** Delete File: <path>" directives in the "input" field.
func patchFileTargets(toolInput json.RawMessage) []string {
	var inp struct {
		Input string `json:"input"`
	}
	if err := json.Unmarshal(toolInput, &inp); err != nil || inp.Input == "" {
		return nil
	}

	var targets []string
	seen := map[string]bool{}
	for _, line := range strings.Split(inp.Input, "\n") {
		line = strings.TrimSpace(line)
		var path string
		switch {
		case strings.HasPrefix(line, "*** Update File: "):
			path = strings.TrimPrefix(line, "*** Update File: ")
		case strings.HasPrefix(line, "*** Add File: "):
			path = strings.TrimPrefix(line, "*** Add File: ")
		case strings.HasPrefix(line, "*** Delete File: "):
			path = strings.TrimPrefix(line, "*** Delete File: ")
		default:
			continue
		}
		path = strings.TrimSpace(path)
		if path != "" && !seen[path] {
			seen[path] = true
			targets = append(targets, path)
		}
	}
	return targets
}

// checkPolicy calls the checker if non-nil, returning (true, "") as the
// fail-open default when checker is nil.
func checkPolicy(checker PolicyChecker, filePath, cwd string) (allowed bool, passID string) {
	if checker == nil {
		return true, ""
	}
	return checker(filePath, cwd)
}

// policyDenyReason returns the denial reason string for a direct file write.
func policyDenyReason(path string) string {
	if path == "" {
		path = "this file"
	}
	return fmt.Sprintf(
		"CORDON POLICY: %s is protected by a Cordon zone policy. "+
			"To request write access, you (agent) should use the cordon_request_access MCP tool which will ask the user for approval. "+
			"Alternatively, ask the user to grant access themselves using the command cordon pass issue --file <file>. "+
			"Do not attempt to write to this file through any alternative method, "+
			"including shell commands such as echo, sed, tee, cp, mv, or any other approach. "+
			"Do NOT run the cordon shell command cordon command directly — agents are prohibited from executing cordon CLI commands. You must use the MCP "+
			"This is an enforced policy restriction, not a technical error. ",
		path,
	)
}

// policyBashDenyReason returns the denial reason for a bash command that
// targets one or more protected files.
func policyBashDenyReason(primary string, all []string) string {
	target := primary
	if target == "" {
		target = "a protected file"
	}
	return fmt.Sprintf(
		"CORDON POLICY: %s is protected by a Cordon zone policy. "+
			"To request write access, you (agent) should use the cordon_request_access MCP tool which will ask the user for approval. "+
			"Alternatively, ask the user to grant access themselves using the command cordon pass issue --file <file>. "+
			"Do not attempt to write to this file through any alternative method, "+
			"including shell commands such as echo, sed, tee, cp, mv, or any other approach. "+
			"Do NOT run the cordon shell command cordon command directly — agents are prohibited from executing cordon CLI commands. You must use the MCP "+
			"This is an enforced policy restriction, not a technical error. ",
		target,
	)
}

func writeDeny(w io.Writer, errW io.Writer, toolName, path string) error {
	reason := policyDenyReason(path)
	if err := encodeClaudeDeny(w, reason); err != nil {
		return err
	}
	if copilotTools[toolName] {
		writeCopilotDeny(errW, reason)
	}
	return nil
}

func writeBashDeny(w io.Writer, errW io.Writer, primary string, all []string) error {
	reason := policyBashDenyReason(primary, all)
	return encodeClaudeDeny(w, reason)
}

// encodeClaudeDeny writes the Claude Code JSON deny response to stdout.
func encodeClaudeDeny(w io.Writer, reason string) error {
	type denyResponse struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(denyResponse{Decision: "block", Reason: reason})
}

// writeCopilotDeny writes a plain-text denial message to stderr for VS Code
// Copilot, which reads stderr for error context rather than parsing the JSON
// on stdout.
func writeCopilotDeny(errW io.Writer, reason string) {
	fmt.Fprintf(errW, "%s\n", reason)
}

// isCordonCommand returns true if the bash command invokes the cordon CLI.
// This prevents agents from running cordon commands directly (e.g. cordon pass issue).
func isCordonCommand(command string) bool {
	cmd := strings.TrimSpace(command)
	return cmd == "cordon" || strings.HasPrefix(cmd, "cordon ") ||
		strings.Contains(cmd, "|cordon ") || strings.Contains(cmd, "| cordon ") ||
		strings.Contains(cmd, "&& cordon ") || strings.Contains(cmd, "&&cordon ") ||
		strings.Contains(cmd, "; cordon ") || strings.Contains(cmd, ";cordon ")
}
