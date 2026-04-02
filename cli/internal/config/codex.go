package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Codex config paths.
const (
	CodexHookRelPath   = ".codex/hooks.json"
	CodexConfigRelPath = ".codex/config.toml"
)

// --- Feature flag helpers ---

// EnsureCodexFeatureFlag reads the config.toml at the given path and ensures
// [features] codex_hooks = true is present. Preserves all other content.
// Creates the file and parent directories if they do not exist.
func EnsureCodexFeatureFlag(path string) error {
	content, err := readToml(path)
	if err != nil {
		return err
	}

	updated := ensureTomlSection(content, "[features]", "codex_hooks = true")
	if updated == content {
		return nil
	}

	return writeToml(path, updated)
}

// RemoveCodexFeatureFlag removes codex_hooks = true from the [features] section
// of the config.toml at the given path. Removes the [features] header if the
// section becomes empty. Returns nil if the file does not exist.
func RemoveCodexFeatureFlag(path string) error {
	content, err := readToml(path)
	if err != nil || content == "" {
		return err
	}

	updated := removeTomlKey(content, "[features]", "codex_hooks")
	if updated == content {
		return nil
	}

	return writeOrRemoveToml(path, updated)
}

// --- MCP server helpers ---

// EnsureCodexMCPServer ensures the [mcp_servers.cordon] section exists in the
// config.toml with command = "cordon" and args = ["--mcp"].
// Preserves all other content.
func EnsureCodexMCPServer(path string) error {
	content, err := readToml(path)
	if err != nil {
		return err
	}

	updated := ensureTomlKeyValue(content, "[mcp_servers.cordon]", "command = \"cordon\"")
	updated = ensureTomlKeyValue(updated, "[mcp_servers.cordon]", "args = [\"--mcp\"]")
	updated = ensureTomlKeyValue(updated, "[mcp_servers.cordon.tools.cordon_request_access]", "approval_mode = \"approve\"")

	return writeToml(path, updated)
}

// RemoveCodexMCPServer removes the [mcp_servers.cordon] section and all its
// keys from the config.toml. Returns nil if the file does not exist.
func RemoveCodexMCPServer(path string) error {
	content, err := readToml(path)
	if err != nil || content == "" {
		return err
	}

	updated := removeTomlSectionBlock(content, "[mcp_servers.cordon]")
	updated = removeTomlSectionBlock(updated, "[mcp_servers.cordon.tools.cordon_request_access]")
	if updated == content {
		return nil
	}

	return writeOrRemoveToml(path, updated)
}

// --- TOML file I/O ---

func readToml(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("codexcfg: read %s: %w", path, err)
	}
	return string(raw), nil
}

func writeToml(path string, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("codexcfg: create directory for %s: %w", path, err)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeOrRemoveToml(path string, content string) error {
	if strings.TrimSpace(content) == "" {
		return os.Remove(path)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// --- TOML manipulation helpers ---

// ensureTomlSection ensures a key=value line exists under the given section
// header. If the section doesn't exist, it is appended.
func ensureTomlSection(content, header, keyValue string) string {
	lines := strings.Split(content, "\n")
	key := strings.SplitN(keyValue, "=", 2)[0]
	key = strings.TrimSpace(key)

	inSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			inSection = true
			continue
		}
		if inSection {
			if strings.HasPrefix(trimmed, "[") {
				break
			}
			if strings.HasPrefix(trimmed, key) && strings.Contains(trimmed, "true") {
				return content // already present
			}
		}
	}

	if inSection {
		// Section exists but missing the key. Insert after header.
		var result []string
		inserted := false
		for _, line := range lines {
			result = append(result, line)
			if !inserted && strings.TrimSpace(line) == header {
				result = append(result, keyValue)
				inserted = true
			}
		}
		return strings.Join(result, "\n")
	}

	// Section doesn't exist. Append it.
	sep := ""
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		sep = "\n"
	}
	return content + sep + "\n" + header + "\n" + keyValue + "\n"
}

// ensureTomlKeyValue ensures a key=value line exists under the given section
// header. If the key exists with a different value, it is replaced. If the
// section doesn't exist, it is appended.
func ensureTomlKeyValue(content, header, keyValue string) string {
	lines := strings.Split(content, "\n")
	key := strings.TrimSpace(strings.SplitN(keyValue, "=", 2)[0])

	inSection := false
	foundHeader := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == header {
			inSection = true
			foundHeader = true
			continue
		}
		if inSection && strings.HasPrefix(trimmed, "[") {
			inSection = false
		}
		if inSection && strings.HasPrefix(trimmed, key) {
			if trimmed == keyValue {
				return content
			}
			lines[i] = keyValue
			return strings.Join(lines, "\n")
		}
	}

	if foundHeader {
		var result []string
		inserted := false
		for _, line := range lines {
			result = append(result, line)
			if !inserted && strings.TrimSpace(line) == header {
				result = append(result, keyValue)
				inserted = true
			}
		}
		return strings.Join(result, "\n")
	}

	sep := ""
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		sep = "\n"
	}
	return content + sep + "\n" + header + "\n" + keyValue + "\n"
}

// removeTomlKey removes lines matching the given key prefix from the specified
// section. If the section becomes empty (no keys, only blank/comment lines),
// the section header is removed too.
func removeTomlKey(content, header, keyPrefix string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inSection := false
	headerIdx := -1
	sectionHasOtherKeys := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == header {
			inSection = true
			headerIdx = len(result)
			result = append(result, line)
			continue
		}

		if inSection && strings.HasPrefix(trimmed, "[") {
			inSection = false
		}

		if inSection && strings.HasPrefix(trimmed, keyPrefix) {
			continue // remove this key
		}

		if inSection && trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			sectionHasOtherKeys = true
		}

		result = append(result, line)
	}

	// If section is now empty, remove the header.
	if headerIdx >= 0 && !sectionHasOtherKeys {
		filtered := make([]string, 0, len(result))
		for i, line := range result {
			if i == headerIdx {
				continue
			}
			filtered = append(filtered, line)
		}
		result = filtered
	}

	return strings.Join(result, "\n")
}

// removeTomlSectionBlock removes an entire TOML section (header + all keys
// up to the next section header or EOF).
func removeTomlSectionBlock(content, header string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == header {
			inSection = true
			continue
		}

		if inSection && strings.HasPrefix(trimmed, "[") {
			inSection = false
		}

		if inSection {
			continue
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}
