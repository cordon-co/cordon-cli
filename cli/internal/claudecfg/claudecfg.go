// Package claudecfg manages the .claude/settings.local.json file used by
// Claude Code and VS Code agents. Entries are added and removed additively —
// no existing content is destroyed.
package claudecfg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	cordonCommand   = "cordon hook"
	cordonMatcher   = "Write|Edit|MultiEdit"
	cordonMCPKey    = "cordon"
	settingsRelPath = ".claude/settings.local.json"
)

// AddCordonEntries writes the Cordon PreToolUse hook and MCP server entries
// into <repoRoot>/.claude/settings.local.json. The file is created if it does
// not exist. Existing entries are preserved. The operation is idempotent.
func AddCordonEntries(repoRoot string) error {
	path := filepath.Join(repoRoot, settingsRelPath)

	data, err := readSettings(path)
	if err != nil {
		return err
	}

	addHookEntry(data)
	addMCPEntry(data)

	return writeAtomic(path, data)
}

// RemoveCordonEntries removes the Cordon PreToolUse hook and MCP server entries
// from <repoRoot>/.claude/settings.local.json. If the file does not exist the
// function returns nil. All other content is preserved.
func RemoveCordonEntries(repoRoot string) error {
	path := filepath.Join(repoRoot, settingsRelPath)

	data, err := readSettings(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	removeHookEntry(data)
	removeMCPEntry(data)

	return writeAtomic(path, data)
}

// readSettings reads and unmarshals the settings file into a generic map.
// Returns an empty map if the file does not exist.
func readSettings(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("claudecfg: read %s: %w", path, err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("claudecfg: parse %s: %w", path, err)
	}
	return data, nil
}

// addHookEntry inserts the Cordon hook group into the PreToolUse array.
// Idempotent: does nothing if a Cordon entry is already present.
func addHookEntry(data map[string]interface{}) {
	hooks := getOrCreateMap(data, "hooks")
	preToolUse := getOrCreateSlice(hooks, "PreToolUse")

	if hasCordonHook(preToolUse) {
		return
	}

	newGroup := map[string]interface{}{
		"matcher": cordonMatcher,
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": cordonCommand,
			},
		},
	}
	hooks["PreToolUse"] = append(preToolUse, newGroup)
	data["hooks"] = hooks
}

// addMCPEntry inserts the Cordon MCP server entry. Idempotent.
func addMCPEntry(data map[string]interface{}) {
	servers := getOrCreateMap(data, "mcpServers")
	if _, exists := servers[cordonMCPKey]; exists {
		return
	}
	servers[cordonMCPKey] = map[string]interface{}{
		"command": "cordon",
		"args":    []interface{}{"--mcp"},
	}
	data["mcpServers"] = servers
}

// removeHookEntry removes the Cordon hook group from the PreToolUse array.
func removeHookEntry(data map[string]interface{}) {
	hooksRaw, ok := data["hooks"]
	if !ok {
		return
	}
	hooks, ok := hooksRaw.(map[string]interface{})
	if !ok {
		return
	}

	ptuRaw, ok := hooks["PreToolUse"]
	if !ok {
		return
	}
	ptu, ok := ptuRaw.([]interface{})
	if !ok {
		return
	}

	filtered := ptu[:0]
	for _, item := range ptu {
		if !isCordonHookGroup(item) {
			filtered = append(filtered, item)
		}
	}

	if len(filtered) == 0 {
		delete(hooks, "PreToolUse")
	} else {
		hooks["PreToolUse"] = filtered
	}

	if len(hooks) == 0 {
		delete(data, "hooks")
	} else {
		data["hooks"] = hooks
	}
}

// removeMCPEntry removes the Cordon MCP server entry.
func removeMCPEntry(data map[string]interface{}) {
	serversRaw, ok := data["mcpServers"]
	if !ok {
		return
	}
	servers, ok := serversRaw.(map[string]interface{})
	if !ok {
		return
	}

	delete(servers, cordonMCPKey)

	if len(servers) == 0 {
		delete(data, "mcpServers")
	} else {
		data["mcpServers"] = servers
	}
}

// hasCordonHook reports whether the PreToolUse slice already contains a
// Cordon hook group (identified by the command string).
func hasCordonHook(ptu []interface{}) bool {
	for _, item := range ptu {
		if isCordonHookGroup(item) {
			return true
		}
	}
	return false
}

// isCordonHookGroup reports whether a PreToolUse array element is the Cordon
// hook group, identified by any inner hook with command == cordonCommand.
func isCordonHookGroup(item interface{}) bool {
	group, ok := item.(map[string]interface{})
	if !ok {
		return false
	}
	hooksRaw, ok := group["hooks"]
	if !ok {
		return false
	}
	innerHooks, ok := hooksRaw.([]interface{})
	if !ok {
		return false
	}
	for _, h := range innerHooks {
		hm, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		if cmd, ok := hm["command"].(string); ok && cmd == cordonCommand {
			return true
		}
	}
	return false
}

// getOrCreateMap retrieves a map[string]interface{} value from parent by key,
// creating and inserting a new empty map if the key is absent or the wrong type.
func getOrCreateMap(parent map[string]interface{}, key string) map[string]interface{} {
	if v, ok := parent[key]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
	}
	m := map[string]interface{}{}
	parent[key] = m
	return m
}

// getOrCreateSlice retrieves a []interface{} value from parent by key,
// creating and inserting a new empty slice if the key is absent or the wrong type.
func getOrCreateSlice(parent map[string]interface{}, key string) []interface{} {
	if v, ok := parent[key]; ok {
		if s, ok := v.([]interface{}); ok {
			return s
		}
	}
	s := []interface{}{}
	parent[key] = s
	return s
}

// writeAtomic marshals data and writes it to dst atomically via a temp file
// in the same directory, then renames. Creates the parent directory if needed.
func writeAtomic(dst string, data map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("claudecfg: create directory: %w", err)
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("claudecfg: marshal: %w", err)
	}
	content = append(content, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("claudecfg: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("claudecfg: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("claudecfg: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("claudecfg: rename to %s: %w", dst, err)
	}

	return nil
}
