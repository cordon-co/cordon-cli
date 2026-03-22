package agents

import (
	"path/filepath"

	"github.com/cordon-co/cordon-cli/internal/claudecfg"
)

// Cursor configures Cursor IDE via .cursor/hooks.json, .cursor/mcp.json,
// and .cursor/cli.json. The hook is always installed in .cursor/hooks.json
// so Cursor enforcement is self-contained (independent of Claude Code).
type Cursor struct{}

func (c *Cursor) ID() string          { return "cursor" }
func (c *Cursor) DisplayName() string { return "Cursor" }
func (c *Cursor) Installable() bool   { return true }

func (c *Cursor) Install(repoRoot string) error {
	// Hook in .cursor/hooks.json
	hookPath := filepath.Join(repoRoot, claudecfg.CursorHookRelPath)
	hookData, err := claudecfg.ReadSettings(hookPath)
	if err != nil {
		return err
	}
	claudecfg.AddCursorHookEntry(hookData)
	if err := claudecfg.WriteAtomic(hookPath, hookData); err != nil {
		return err
	}

	// MCP server in .cursor/mcp.json
	mcpPath := filepath.Join(repoRoot, claudecfg.CursorMCPRelPath)
	mcpData, err := claudecfg.ReadSettings(mcpPath)
	if err != nil {
		return err
	}
	claudecfg.AddMCPEntry(mcpData)
	if err := claudecfg.WriteAtomic(mcpPath, mcpData); err != nil {
		return err
	}

	// MCP tool permission in .cursor/cli.json
	cliPath := filepath.Join(repoRoot, claudecfg.CursorCLIRelPath)
	cliData, err := claudecfg.ReadSettings(cliPath)
	if err != nil {
		return err
	}
	claudecfg.AddCursorMCPToolPermission(cliData)
	return claudecfg.WriteAtomic(cliPath, cliData)
}

func (c *Cursor) Remove(repoRoot string) error {
	// Remove hook from .cursor/hooks.json
	hookPath := filepath.Join(repoRoot, claudecfg.CursorHookRelPath)
	hookData, err := claudecfg.ReadSettings(hookPath)
	if err == nil {
		claudecfg.RemoveCursorHookEntry(hookData)
		if err := claudecfg.WriteAtomic(hookPath, hookData); err != nil {
			return err
		}
	}

	// Remove MCP entry from .cursor/mcp.json
	mcpPath := filepath.Join(repoRoot, claudecfg.CursorMCPRelPath)
	mcpData, err := claudecfg.ReadSettings(mcpPath)
	if err == nil {
		claudecfg.RemoveMCPEntry(mcpData)
		if err := claudecfg.WriteAtomic(mcpPath, mcpData); err != nil {
			return err
		}
	}

	// Remove permission from .cursor/cli.json
	cliPath := filepath.Join(repoRoot, claudecfg.CursorCLIRelPath)
	cliData, err := claudecfg.ReadSettings(cliPath)
	if err == nil {
		claudecfg.RemoveCursorMCPToolPermission(cliData)
		if err := claudecfg.WriteAtomic(cliPath, cliData); err != nil {
			return err
		}
	}

	return nil
}

func (c *Cursor) Installed(repoRoot string) bool {
	hookPath := filepath.Join(repoRoot, claudecfg.CursorHookRelPath)
	data, err := claudecfg.ReadSettings(hookPath)
	if err != nil {
		return false
	}
	hooksRaw, ok := data["hooks"]
	if !ok {
		return false
	}
	hooks, ok := hooksRaw.(map[string]interface{})
	if !ok {
		return false
	}
	ptuRaw, ok := hooks["preToolUse"]
	if !ok {
		return false
	}
	ptu, ok := ptuRaw.([]interface{})
	if !ok {
		return false
	}
	return claudecfg.HasCursorCordonHook(ptu)
}
