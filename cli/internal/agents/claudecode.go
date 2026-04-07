package agents

import (
	"path/filepath"

	"github.com/cordon-co/cordon-cli/cli/internal/config"
)

// ClaudeCode configures Claude Code via .claude/settings.local.json and .mcp.json.
type ClaudeCode struct{}

func (c *ClaudeCode) ID() string          { return "claude-code" }
func (c *ClaudeCode) DisplayName() string { return "Claude Code" }
func (c *ClaudeCode) Installable() bool   { return true }
func (c *ClaudeCode) SupportsMCPElicitation() bool {
	return true
}

func (c *ClaudeCode) Install(repoRoot string) error {
	// Hook + MCP permissions in .claude/settings.local.json
	settingsPath := filepath.Join(repoRoot, config.SettingsRelPath)
	settingsData, err := config.ReadSettings(settingsPath)
	if err != nil {
		return err
	}
	// Pass empty agent: Cursor reads .claude/settings.local.json hooks too,
	// so both Claude Code and Cursor must emit the same "cordon hook" command
	// (no --agent flag). This lets Cursor deduplicate to a single hook call.
	// Agent identity is detected from the payload instead (conversation_id
	// presence → Cursor, otherwise Claude Code). See hook.go:inferAgent.
	config.AddHookEntry(settingsData, "")
	config.AddEnabledMCPServer(settingsData)
	config.AddMCPToolPermission(settingsData)
	config.RemoveMCPEntry(settingsData) // clean up any legacy MCP entry
	if err := config.WriteAtomic(settingsPath, settingsData); err != nil {
		return err
	}

	// MCP server in .mcp.json
	mcpPath := filepath.Join(repoRoot, config.MCPRelPath)
	mcpData, err := config.ReadSettings(mcpPath)
	if err != nil {
		return err
	}
	config.AddMCPEntry(mcpData)
	return config.WriteAtomic(mcpPath, mcpData)
}

func (c *ClaudeCode) Remove(repoRoot string) error {
	// Remove from .claude/settings.local.json
	settingsPath := filepath.Join(repoRoot, config.SettingsRelPath)
	settingsData, err := config.ReadSettings(settingsPath)
	if err != nil {
		return nil // file doesn't exist, nothing to remove
	}
	config.RemoveHookEntry(settingsData)
	config.RemoveMCPEntry(settingsData)
	config.RemoveEnabledMCPServer(settingsData)
	config.RemoveMCPToolPermission(settingsData)
	if err := config.WriteAtomic(settingsPath, settingsData); err != nil {
		return err
	}

	// Remove from .mcp.json
	mcpPath := filepath.Join(repoRoot, config.MCPRelPath)
	mcpData, err := config.ReadSettings(mcpPath)
	if err != nil {
		return nil
	}
	config.RemoveMCPEntry(mcpData)
	return config.WriteAtomic(mcpPath, mcpData)
}

func (c *ClaudeCode) Installed(repoRoot string) bool {
	settingsPath := filepath.Join(repoRoot, config.SettingsRelPath)
	data, err := config.ReadSettings(settingsPath)
	if err != nil {
		return false
	}
	hooks, ok := data["hooks"]
	if !ok {
		return false
	}
	hooksMap, ok := hooks.(map[string]interface{})
	if !ok {
		return false
	}
	ptuRaw, ok := hooksMap["PreToolUse"]
	if !ok {
		return false
	}
	ptu, ok := ptuRaw.([]interface{})
	if !ok {
		return false
	}
	return config.HasCordonHook(ptu)
}
