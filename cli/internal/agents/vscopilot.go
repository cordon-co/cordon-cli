package agents

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/cordon-co/cordon-cli/internal/claudecfg"
)

// VSCopilot configures VS Code Copilot via .vscode/mcp.json and
// .github/hooks/cordon.json.
type VSCopilot struct{}

func (v *VSCopilot) ID() string          { return "vs-copilot" }
func (v *VSCopilot) DisplayName() string { return "VS Code Chat" }
func (v *VSCopilot) Installable() bool   { return true }

func (v *VSCopilot) Install(repoRoot string) error {
	// MCP server in .vscode/mcp.json
	vscodeMCPPath := filepath.Join(repoRoot, claudecfg.VSCodeMCPRelPath)
	vscodeMCPData, err := claudecfg.ReadSettings(vscodeMCPPath)
	if err != nil {
		return err
	}
	claudecfg.AddVSCodeMCPEntry(vscodeMCPData)
	if err := claudecfg.WriteAtomic(vscodeMCPPath, vscodeMCPData); err != nil {
		return err
	}

	// Hook in .github/hooks/cordon.json
	vscodeHookPath := filepath.Join(repoRoot, claudecfg.VSCodeHookRelPath)
	return claudecfg.WriteVSCodeHookFile(vscodeHookPath, "vs-copilot")
}

func (v *VSCopilot) Remove(repoRoot string) error {
	// Remove VS Code hook file
	vscodeHookPath := filepath.Join(repoRoot, claudecfg.VSCodeHookRelPath)
	if err := os.Remove(vscodeHookPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	// Remove MCP entry from .vscode/mcp.json
	vscodeMCPPath := filepath.Join(repoRoot, claudecfg.VSCodeMCPRelPath)
	vscodeMCPData, err := claudecfg.ReadSettings(vscodeMCPPath)
	if err != nil {
		return nil
	}
	claudecfg.RemoveVSCodeMCPEntry(vscodeMCPData)
	return claudecfg.WriteAtomic(vscodeMCPPath, vscodeMCPData)
}

func (v *VSCopilot) Installed(repoRoot string) bool {
	vscodeHookPath := filepath.Join(repoRoot, claudecfg.VSCodeHookRelPath)
	_, err := os.Stat(vscodeHookPath)
	return err == nil
}
