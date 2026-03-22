package agents

import (
	"path/filepath"

	"github.com/cordon-co/cordon-cli/internal/claudecfg"
)

// ClaudeCode configures Claude Code via .claude/settings.local.json and .mcp.json.
type ClaudeCode struct{}

func (c *ClaudeCode) ID() string          { return "claude-code" }
func (c *ClaudeCode) DisplayName() string { return "Claude Code" }
func (c *ClaudeCode) Installable() bool   { return true }

func (c *ClaudeCode) Install(repoRoot string) error {
	// Hook + MCP permissions in .claude/settings.local.json
	settingsPath := filepath.Join(repoRoot, claudecfg.SettingsRelPath)
	settingsData, err := claudecfg.ReadSettings(settingsPath)
	if err != nil {
		return err
	}
	claudecfg.AddHookEntry(settingsData, "claude-code")
	claudecfg.AddEnabledMCPServer(settingsData)
	claudecfg.AddMCPToolPermission(settingsData)
	claudecfg.RemoveMCPEntry(settingsData) // clean up any legacy MCP entry
	if err := claudecfg.WriteAtomic(settingsPath, settingsData); err != nil {
		return err
	}

	// MCP server in .mcp.json
	mcpPath := filepath.Join(repoRoot, claudecfg.MCPRelPath)
	mcpData, err := claudecfg.ReadSettings(mcpPath)
	if err != nil {
		return err
	}
	claudecfg.AddMCPEntry(mcpData)
	return claudecfg.WriteAtomic(mcpPath, mcpData)
}

func (c *ClaudeCode) Remove(repoRoot string) error {
	// Remove from .claude/settings.local.json
	settingsPath := filepath.Join(repoRoot, claudecfg.SettingsRelPath)
	settingsData, err := claudecfg.ReadSettings(settingsPath)
	if err != nil {
		return nil // file doesn't exist, nothing to remove
	}
	claudecfg.RemoveHookEntry(settingsData)
	claudecfg.RemoveMCPEntry(settingsData)
	claudecfg.RemoveEnabledMCPServer(settingsData)
	claudecfg.RemoveMCPToolPermission(settingsData)
	if err := claudecfg.WriteAtomic(settingsPath, settingsData); err != nil {
		return err
	}

	// Remove from .mcp.json
	mcpPath := filepath.Join(repoRoot, claudecfg.MCPRelPath)
	mcpData, err := claudecfg.ReadSettings(mcpPath)
	if err != nil {
		return nil
	}
	claudecfg.RemoveMCPEntry(mcpData)
	return claudecfg.WriteAtomic(mcpPath, mcpData)
}

func (c *ClaudeCode) Installed(repoRoot string) bool {
	settingsPath := filepath.Join(repoRoot, claudecfg.SettingsRelPath)
	data, err := claudecfg.ReadSettings(settingsPath)
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
	return claudecfg.HasCordonHook(ptu)
}
