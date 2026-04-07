package agents

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	openCodePluginRelDir  = ".opencode/plugins"
	openCodePluginFile    = "cordon-interface.js"
	openCodePluginMarker  = "cordon.sh"
	openCodeConfigRelPath = ".opencode/opencode.jsonc"
	openCodeMCPKey        = "cordon"
)

// pluginContent is the JS plugin written to .opencode/plugins/cordon-interface.js.
// It hooks tool.execute.before, spawns "cordon hook" with the tool payload on
// stdin, and throws an Error on deny (exit code 2) to block execution.
const pluginContent = `// Cordon enforcement plugin for OpenCode — do not edit.
// Managed by cordon (https://cordon.sh).
export const CordonEnforcement = async ({ $, directory }) => {
  const extractToolInput = (input, output) => {
    // OpenCode tool hook payloads can surface args under different keys.
    const candidate =
      output?.args ??
      output?.arguments ??
      input?.args ??
      input?.arguments ??
      input?.input ??
      {};
    return candidate && typeof candidate === "object" ? candidate : {};
  };

  return {
    "tool.execute.before": async (input, output) => {
      const toolInput = extractToolInput(input, output);
      const payload = JSON.stringify({
        tool_name: input.tool,
        tool_input: toolInput,
        cwd: directory,
      });
      try {
        const proc = Bun.spawn(["cordon", "hook", "--agent", "opencode"], {
          stdin: new Blob([payload]),
          stdout: "pipe",
          stderr: "pipe",
        });
        const exitCode = await proc.exited;
        if (exitCode === 2) {
          let reason = "Blocked by Cordon policy";
          try {
            const stdout = await new Response(proc.stdout).text();
            const parsed = JSON.parse(stdout);
            if (parsed.reason) reason = parsed.reason;
          } catch {}
          const denyError = new Error(reason);
          denyError.name = "CordonPolicyError";
          throw denyError;
        }
      } catch (e) {
        if (e?.name === "CordonPolicyError") throw e;
        // Fail-open on infrastructure errors
      }
    },
  }
}
`

// OpenCode configures the OpenCode agent via a JS plugin at
// .opencode/plugins/cordon-interface.js. The plugin hooks tool.execute.before
// to enforce Cordon file and command rules.
type OpenCode struct{}

func (o *OpenCode) ID() string          { return "opencode" }
func (o *OpenCode) DisplayName() string { return "OpenCode" }
func (o *OpenCode) Installable() bool   { return true }
func (o *OpenCode) SupportsMCPElicitation() bool {
	return false
}

func (o *OpenCode) Install(repoRoot string) error {
	return o.installPlugin(repoRoot)
}

func (o *OpenCode) Remove(repoRoot string) error {
	if err := o.removePlugin(repoRoot); err != nil {
		return err
	}
	return o.removeMCP(repoRoot)
}

func (o *OpenCode) Installed(repoRoot string) bool {
	return o.pluginInstalled(repoRoot)
}

// --- plugin management ---

func (o *OpenCode) installPlugin(repoRoot string) error {
	dir := filepath.Join(repoRoot, openCodePluginRelDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("opencode: create plugins directory: %w", err)
	}
	dest := filepath.Join(dir, openCodePluginFile)
	tmp := dest + ".tmp"
	if err := os.WriteFile(tmp, []byte(pluginContent), 0o644); err != nil {
		return fmt.Errorf("opencode: write plugin: %w", err)
	}
	if err := os.Rename(tmp, dest); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("opencode: install plugin: %w", err)
	}
	return nil
}

func (o *OpenCode) removePlugin(repoRoot string) error {
	path := filepath.Join(repoRoot, openCodePluginRelDir, openCodePluginFile)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// Clean up empty plugins directory.
	dir := filepath.Join(repoRoot, openCodePluginRelDir)
	entries, err := os.ReadDir(dir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
	return nil
}

func (o *OpenCode) pluginInstalled(repoRoot string) bool {
	path := filepath.Join(repoRoot, openCodePluginRelDir, openCodePluginFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), openCodePluginMarker)
}

// --- MCP config management (.opencode/opencode.jsonc) ---

// stripJSONC removes single-line comments (//) and trailing commas to produce
// valid JSON from a JSONC input while preserving comment-like text inside
// string literals (for example "https://...").
func stripJSONC(raw []byte) []byte {
	return stripJSONCTrailingCommas(stripJSONCComments(raw))
}

func stripJSONCComments(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString := false
	escape := false
	inLineComment := false
	inBlockComment := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
				out = append(out, ch)
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < len(raw) && raw[i+1] == '/' {
				inBlockComment = false
				i++
				continue
			}
			if ch == '\n' {
				out = append(out, ch)
			}
			continue
		}

		if inString {
			out = append(out, ch)
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}

		if ch == '/' && i+1 < len(raw) {
			next := raw[i+1]
			if next == '/' {
				inLineComment = true
				i++
				continue
			}
			if next == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		out = append(out, ch)
	}

	return out
}

func stripJSONCTrailingCommas(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	inString := false
	escape := false

	for i := 0; i < len(raw); i++ {
		ch := raw[i]

		if inString {
			out = append(out, ch)
			if escape {
				escape = false
				continue
			}
			if ch == '\\' {
				escape = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}

		if ch == ',' {
			j := i + 1
			for j < len(raw) {
				switch raw[j] {
				case ' ', '\t', '\n', '\r':
					j++
				default:
					goto nextToken
				}
			}
		nextToken:
			if j < len(raw) && (raw[j] == '}' || raw[j] == ']') {
				continue
			}
		}

		out = append(out, ch)
	}

	return out
}

func readOpenCodeConfig(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("opencode: read %s: %w", path, err)
	}
	clean := stripJSONC(raw)
	var data map[string]interface{}
	if err := json.Unmarshal(clean, &data); err != nil {
		return nil, fmt.Errorf("opencode: parse %s: %w", path, err)
	}
	return data, nil
}

func writeOpenCodeConfig(path string, data map[string]interface{}) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("opencode: create config directory: %w", err)
	}
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("opencode: marshal config: %w", err)
	}
	out = append(out, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return fmt.Errorf("opencode: write config: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("opencode: install config: %w", err)
	}
	return nil
}

func (o *OpenCode) installMCP(repoRoot string) error {
	cfgPath := filepath.Join(repoRoot, openCodeConfigRelPath)
	data, err := readOpenCodeConfig(cfgPath)
	if err != nil {
		return err
	}

	// Get or create the "mcp" map.
	mcpRaw, ok := data["mcp"]
	var mcp map[string]interface{}
	if ok {
		mcp, ok = mcpRaw.(map[string]interface{})
		if !ok {
			mcp = map[string]interface{}{}
		}
	} else {
		mcp = map[string]interface{}{}
	}

	// Idempotent: skip if already present.
	if _, exists := mcp[openCodeMCPKey]; exists {
		return nil
	}

	mcp[openCodeMCPKey] = map[string]interface{}{
		"type":    "local",
		"command": []interface{}{"cordon", "--mcp"},
	}
	data["mcp"] = mcp
	return writeOpenCodeConfig(cfgPath, data)
}

func (o *OpenCode) removeMCP(repoRoot string) error {
	cfgPath := filepath.Join(repoRoot, openCodeConfigRelPath)

	// Do not materialize a new config file during cleanup when one doesn't exist.
	if _, err := os.Stat(cfgPath); errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	data, err := readOpenCodeConfig(cfgPath)
	if err != nil {
		return err
	}

	mcpRaw, ok := data["mcp"]
	if !ok {
		return nil
	}
	mcp, ok := mcpRaw.(map[string]interface{})
	if !ok {
		return nil
	}
	delete(mcp, openCodeMCPKey)
	if len(mcp) == 0 {
		delete(data, "mcp")
	} else {
		data["mcp"] = mcp
	}

	// If nothing remains after removing the Cordon MCP entry, remove the file.
	if len(data) == 0 {
		if err := os.Remove(cfgPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}

	return writeOpenCodeConfig(cfgPath, data)
}
