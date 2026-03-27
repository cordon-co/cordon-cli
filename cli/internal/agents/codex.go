package agents

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cordon-co/cordon-cli/cli/internal/claudecfg"
)

// Codex configures OpenAI Codex via a PreToolUse hook in .codex/hooks.json
// and enables the codex_hooks feature flag in .codex/config.toml.
type Codex struct{}

func (c *Codex) ID() string          { return "codex" }
func (c *Codex) DisplayName() string { return "Codex" }
func (c *Codex) Installable() bool   { return true }

func (c *Codex) Install(repoRoot string) error {
	// Enable the codex_hooks feature flag in .codex/config.toml.
	configPath := filepath.Join(repoRoot, claudecfg.CodexConfigRelPath)
	if err := claudecfg.EnsureCodexFeatureFlag(configPath); err != nil {
		return err
	}

	// Add the PreToolUse hook to .codex/hooks.json.
	hookPath := filepath.Join(repoRoot, claudecfg.CodexHookRelPath)
	hookData, err := claudecfg.ReadSettings(hookPath)
	if err != nil {
		return err
	}
	claudecfg.AddHookEntry(hookData, "codex")
	return claudecfg.WriteAtomic(hookPath, hookData)
}

func (c *Codex) Remove(repoRoot string) error {
	// Remove the PreToolUse hook from .codex/hooks.json.
	hookPath := filepath.Join(repoRoot, claudecfg.CodexHookRelPath)
	hookData, err := claudecfg.ReadSettings(hookPath)
	if err == nil {
		claudecfg.RemoveHookEntry(hookData)
		if err := claudecfg.WriteAtomic(hookPath, hookData); err != nil {
			return err
		}
	}

	// Remove the codex_hooks feature flag from .codex/config.toml.
	configPath := filepath.Join(repoRoot, claudecfg.CodexConfigRelPath)
	if err := claudecfg.RemoveCodexFeatureFlag(configPath); err != nil {
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
	hookPath := filepath.Join(repoRoot, claudecfg.CodexHookRelPath)
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
	ptuRaw, ok := hooks["PreToolUse"]
	if !ok {
		return false
	}
	ptu, ok := ptuRaw.([]interface{})
	if !ok {
		return false
	}
	return claudecfg.HasCordonHook(ptu)
}
