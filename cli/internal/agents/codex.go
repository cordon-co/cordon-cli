package agents

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cordon-co/cordon-cli/cli/internal/config"
)

// Codex configures OpenAI Codex via a PreToolUse hook in .codex/hooks.json
// and enables the codex_hooks feature flag in .codex/config.toml.
type Codex struct{}

func (c *Codex) ID() string          { return "codex" }
func (c *Codex) DisplayName() string { return "Codex" }
func (c *Codex) Installable() bool   { return true }
func (c *Codex) SupportsMCPElicitation() bool {
	return true
}

func (c *Codex) Install(repoRoot string) error {
	configPath := filepath.Join(repoRoot, config.CodexConfigRelPath)

	// Enable the codex_hooks feature flag in .codex/config.toml.
	if err := config.EnsureCodexFeatureFlag(configPath); err != nil {
		return err
	}

	// Add the MCP server entry to .codex/config.toml.
	if err := config.EnsureCodexMCPServer(configPath); err != nil {
		return err
	}

	// Add the PreToolUse hook to .codex/hooks.json.
	hookPath := filepath.Join(repoRoot, config.CodexHookRelPath)
	hookData, err := config.ReadSettings(hookPath)
	if err != nil {
		return err
	}
	config.AddHookEntry(hookData, "codex")
	return config.WriteAtomic(hookPath, hookData)
}

func (c *Codex) Remove(repoRoot string) error {
	// Remove the PreToolUse hook from .codex/hooks.json.
	hookPath := filepath.Join(repoRoot, config.CodexHookRelPath)
	hookData, err := config.ReadSettings(hookPath)
	if err == nil {
		config.RemoveHookEntry(hookData)
		if err := config.WriteAtomic(hookPath, hookData); err != nil {
			return err
		}
	}

	// Remove cordon entries from .codex/config.toml.
	configPath := filepath.Join(repoRoot, config.CodexConfigRelPath)
	if err := config.RemoveCodexMCPServer(configPath); err != nil {
		return err
	}
	if err := config.RemoveCodexFeatureFlag(configPath); err != nil {
		return err
	}

	// Clean up the legacy codex-policy.md if it exists.
	legacyPath := filepath.Join(repoRoot, ".cordon", "codex-policy.md")
	if err := os.Remove(legacyPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	return nil
}

func (c *Codex) Installed(repoRoot string) bool {
	hookPath := filepath.Join(repoRoot, config.CodexHookRelPath)
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
	ptuRaw, ok := hooks["PreToolUse"]
	if !ok {
		return false
	}
	ptu, ok := ptuRaw.([]interface{})
	if !ok {
		return false
	}
	return config.HasCordonHook(ptu)
}
