package agents

import (
	"path/filepath"

	"github.com/cordon-co/cordon-cli/cli/internal/config"
)

// Cursor configures Cursor IDE via .cursor/hooks.json, .cursor/mcp.json,
// and .cursor/cli.json. The hook is always installed in .cursor/hooks.json
// so Cursor enforcement is self-contained (independent of Claude Code).
type Cursor struct{}

func (c *Cursor) ID() string          { return "cursor" }
func (c *Cursor) DisplayName() string { return "Cursor" }
func (c *Cursor) Installable() bool   { return true }
func (c *Cursor) SupportsMCPElicitation() bool {
	return true
}

func (c *Cursor) Install(repoRoot string) error {
	// Hook in .cursor/hooks.json
	hookPath := filepath.Join(repoRoot, config.CursorHookRelPath)
	hookData, err := config.ReadSettings(hookPath)
	if err != nil {
		return err
	}
	// Pass empty agent: the hook command must be identical to Claude Code's
	// ("cordon hook" with no --agent flag) so Cursor deduplicates them into a
	// single hook call. Agent identity is inferred from the payload instead.
	// See hook.go:inferAgent.
	config.AddCursorHookEntry(hookData, "")
	if err := config.WriteAtomic(hookPath, hookData); err != nil {
		return err
	}

	// MCP server in .cursor/mcp.json
	mcpPath := filepath.Join(repoRoot, config.CursorMCPRelPath)
	mcpData, err := config.ReadSettings(mcpPath)
	if err != nil {
		return err
	}
	config.AddMCPEntry(mcpData)
	if err := config.WriteAtomic(mcpPath, mcpData); err != nil {
		return err
	}

	// MCP tool permission in .cursor/cli.json
	cliPath := filepath.Join(repoRoot, config.CursorCLIRelPath)
	cliData, err := config.ReadSettings(cliPath)
	if err != nil {
		return err
	}
	config.AddCursorMCPToolPermission(cliData)
	return config.WriteAtomic(cliPath, cliData)
}

func (c *Cursor) Remove(repoRoot string) error {
	// Remove hook from .cursor/hooks.json
	hookPath := filepath.Join(repoRoot, config.CursorHookRelPath)
	hookData, err := config.ReadSettings(hookPath)
	if err == nil {
		config.RemoveCursorHookEntry(hookData)
		if err := config.WriteAtomic(hookPath, hookData); err != nil {
			return err
		}
	}

	// Remove MCP entry from .cursor/mcp.json
	mcpPath := filepath.Join(repoRoot, config.CursorMCPRelPath)
	mcpData, err := config.ReadSettings(mcpPath)
	if err == nil {
		config.RemoveMCPEntry(mcpData)
		if err := config.WriteAtomic(mcpPath, mcpData); err != nil {
			return err
		}
	}

	// Remove permission from .cursor/cli.json
	cliPath := filepath.Join(repoRoot, config.CursorCLIRelPath)
	cliData, err := config.ReadSettings(cliPath)
	if err == nil {
		config.RemoveCursorMCPToolPermission(cliData)
		if err := config.WriteAtomic(cliPath, cliData); err != nil {
			return err
		}
	}

	return nil
}

func (c *Cursor) Installed(repoRoot string) bool {
	hookPath := filepath.Join(repoRoot, config.CursorHookRelPath)
	data, err := config.ReadSettings(hookPath)
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
	return config.HasCursorCordonHook(ptu)
}
