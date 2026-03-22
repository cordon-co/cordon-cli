package agents

import (
	"path/filepath"

	"github.com/cordon-co/cordon-cli/internal/claudecfg"
)

// GeminiCLI configures Google Gemini CLI via .gemini/settings.json.
// It installs a BeforeTool hook so Cordon enforcement runs before every
// tool call made by the Gemini CLI agent.
type GeminiCLI struct{}

func (g *GeminiCLI) ID() string          { return "gemini-cli" }
func (g *GeminiCLI) DisplayName() string { return "Gemini CLI" }
func (g *GeminiCLI) Installable() bool   { return true }

func (g *GeminiCLI) Install(repoRoot string) error {
	settingsPath := filepath.Join(repoRoot, claudecfg.GeminiSettingsRelPath)
	data, err := claudecfg.ReadSettings(settingsPath)
	if err != nil {
		return err
	}
	claudecfg.AddGeminiHookEntry(data)
	return claudecfg.WriteAtomic(settingsPath, data)
}

func (g *GeminiCLI) Remove(repoRoot string) error {
	settingsPath := filepath.Join(repoRoot, claudecfg.GeminiSettingsRelPath)
	data, err := claudecfg.ReadSettings(settingsPath)
	if err != nil {
		return err
	}
	claudecfg.RemoveGeminiHookEntry(data)
	return claudecfg.WriteAtomic(settingsPath, data)
}

func (g *GeminiCLI) Installed(repoRoot string) bool {
	settingsPath := filepath.Join(repoRoot, claudecfg.GeminiSettingsRelPath)
	data, err := claudecfg.ReadSettings(settingsPath)
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
	btRaw, ok := hooks["BeforeTool"]
	if !ok {
		return false
	}
	bt, ok := btRaw.([]interface{})
	if !ok {
		return false
	}
	return claudecfg.HasGeminiCordonHook(bt)
}
