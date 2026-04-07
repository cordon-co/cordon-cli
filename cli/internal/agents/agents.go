// Package agents provides a registry of AI coding agent platforms that cordon
// can configure. Each agent implements the Agent interface with self-contained
// install/remove logic.
package agents

// Agent represents an installable agent platform.
type Agent interface {
	// ID returns a stable machine-readable identifier (e.g. "claude-code").
	ID() string

	// DisplayName returns a human-readable name (e.g. "Claude Code").
	DisplayName() string

	// Installable reports whether this agent can be installed currently,
	// as opposed to being a placeholder stub.
	Installable() bool

	// Install configures this agent platform in the given repo root.
	// Must be idempotent.
	Install(repoRoot string) error

	// Remove removes this agent platform's configuration from the given repo root.
	// Must be idempotent and safe to call if the agent was never installed.
	Remove(repoRoot string) error

	// Installed reports whether this agent is currently configured in the repo.
	Installed(repoRoot string) bool

	// SupportsMCPElicitation reports whether this agent platform supports MCP
	// elicitation flows (interactive access request prompts).
	SupportsMCPElicitation() bool
}

var registry = []Agent{
	&ClaudeCode{},
	&Cursor{},
	&VSCopilot{},
	&GeminiCLI{},
	&OpenCode{},
	&Codex{},
}

// All returns the full ordered list of known agents.
func All() []Agent {
	return registry
}

// Installable returns only agents where Installable() == true.
func Installable() []Agent {
	var result []Agent
	for _, a := range registry {
		if a.Installable() {
			result = append(result, a)
		}
	}
	return result
}

// ByID looks up an agent by its stable identifier.
func ByID(id string) (Agent, bool) {
	for _, a := range registry {
		if a.ID() == id {
			return a, true
		}
	}
	return nil, false
}

// InstallSelected installs a subset of agents by ID. Returns the first error
// encountered; agents installed before the error are left in place.
func InstallSelected(repoRoot string, ids []string) error {
	for _, id := range ids {
		a, ok := ByID(id)
		if !ok {
			continue
		}
		if err := a.Install(repoRoot); err != nil {
			return err
		}
	}
	return nil
}

// RemoveAll removes all known agent configurations from a repo.
// Errors are collected but all agents are attempted.
func RemoveAll(repoRoot string) error {
	var firstErr error
	for _, a := range registry {
		if err := a.Remove(repoRoot); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
