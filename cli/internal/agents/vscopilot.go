package agents

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cordon-co/cordon-cli/cli/internal/config"
)

// VSCopilot configures VS Code Copilot via .vscode/mcp.json and
// .github/hooks/cordon.json.
type VSCopilot struct{}

func (v *VSCopilot) ID() string          { return "vs-copilot" }
func (v *VSCopilot) DisplayName() string { return "VS Code Chat" }
func (v *VSCopilot) Installable() bool   { return true }
func (v *VSCopilot) SupportsMCPElicitation() bool {
	return true
}

func (v *VSCopilot) Install(repoRoot string) error {
	// MCP server in .vscode/mcp.json
	vscodeMCPPath := filepath.Join(repoRoot, config.VSCodeMCPRelPath)
	vscodeMCPData, err := config.ReadSettings(vscodeMCPPath)
	if err != nil {
		return err
	}
	config.AddVSCodeMCPEntry(vscodeMCPData)
	if err := config.WriteAtomic(vscodeMCPPath, vscodeMCPData); err != nil {
		return err
	}

	// Hook in .github/hooks/cordon.json
	vscodeHookPath := filepath.Join(repoRoot, config.VSCodeHookRelPath)
	return config.WriteVSCodeHookFile(vscodeHookPath, "vs-copilot")
}

func (v *VSCopilot) Remove(repoRoot string) error {
	// Remove VS Code hook file
	vscodeHookPath := filepath.Join(repoRoot, config.VSCodeHookRelPath)
	if err := os.Remove(vscodeHookPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// Remove MCP entry from .vscode/mcp.json
	vscodeMCPPath := filepath.Join(repoRoot, config.VSCodeMCPRelPath)
	vscodeMCPData, err := config.ReadSettings(vscodeMCPPath)
	if err != nil {
		return nil
	}
	config.RemoveVSCodeMCPEntry(vscodeMCPData)
	return config.WriteAtomic(vscodeMCPPath, vscodeMCPData)
}

func (v *VSCopilot) Installed(repoRoot string) bool {
	vscodeHookPath := filepath.Join(repoRoot, config.VSCodeHookRelPath)
	_, err := os.Stat(vscodeHookPath)
	return err == nil
}
