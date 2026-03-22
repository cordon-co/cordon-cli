package agents

import (
	"path/filepath"

	"github.com/cordon-co/cordon/internal/claudecfg"
)

// Cursor configures Cursor IDE via .cursor/mcp.json.
// Hook enforcement is handled via .claude/settings.local.json (shared with
// Claude Code), so only the MCP server entry needs Cursor-specific config.
type Cursor struct{}

func (c *Cursor) ID() string          { return "cursor" }
func (c *Cursor) DisplayName() string { return "Cursor" }
func (c *Cursor) Installable() bool   { return true }

func (c *Cursor) Install(repoRoot string) error {
	mcpPath := filepath.Join(repoRoot, claudecfg.CursorMCPRelPath)
	mcpData, err := claudecfg.ReadSettings(mcpPath)
	if err != nil {
		return err
	}
	claudecfg.AddMCPEntry(mcpData)
	return claudecfg.WriteAtomic(mcpPath, mcpData)
}

func (c *Cursor) Remove(repoRoot string) error {
	mcpPath := filepath.Join(repoRoot, claudecfg.CursorMCPRelPath)
	mcpData, err := claudecfg.ReadSettings(mcpPath)
	if err != nil {
		return nil // file doesn't exist, nothing to remove
	}
	claudecfg.RemoveMCPEntry(mcpData)
	return claudecfg.WriteAtomic(mcpPath, mcpData)
}

func (c *Cursor) Installed(repoRoot string) bool {
	mcpPath := filepath.Join(repoRoot, claudecfg.CursorMCPRelPath)
	data, err := claudecfg.ReadSettings(mcpPath)
	if err != nil {
		return false
	}
	servers, ok := data["mcpServers"]
	if !ok {
		return false
	}
	serversMap, ok := servers.(map[string]interface{})
	if !ok {
		return false
	}
	_, exists := serversMap[claudecfg.CordonMCPKey]
	return exists
}
