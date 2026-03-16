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
	cordonCommand      = "cordon hook"
	cordonMatcher      = "*"
	cordonMCPKey       = "cordon"
	cordonMCPToolPerm  = "mcp__cordon__cordon_request_access"
	settingsRelPath    = ".claude/settings.local.json"
	mcpRelPath         = ".mcp.json"
)

// AddCordonEntries writes the Cordon PreToolUse hook into
// <repoRoot>/.claude/settings.local.json and the MCP server entry into
// <repoRoot>/.mcp.json. Files are created if they do not exist. Existing
// entries are preserved. The operation is idempotent.
func AddCordonEntries(repoRoot string) error {
	// Hook goes in .claude/settings.local.json
	settingsPath := filepath.Join(repoRoot, settingsRelPath)
	settingsData, err := readSettings(settingsPath)
	if err != nil {
		return err
	}
	addHookEntry(settingsData)
	addEnabledMCPServer(settingsData)
	addMCPToolPermission(settingsData)
	// Remove any legacy MCP entry from settings.local.json
	removeMCPEntry(settingsData)
	if err := writeAtomic(settingsPath, settingsData); err != nil {
		return err
	}

	// MCP server goes in .mcp.json (Claude Code reads MCP configs from here)
	mcpPath := filepath.Join(repoRoot, mcpRelPath)
	mcpData, err := readSettings(mcpPath)
	if err != nil {
		return err
	}
	addMCPEntry(mcpData)
	return writeAtomic(mcpPath, mcpData)
}

// RemoveCordonEntries removes the Cordon PreToolUse hook from
// <repoRoot>/.claude/settings.local.json and the MCP server entry from
// <repoRoot>/.mcp.json. If the files do not exist the function returns nil.
// All other content is preserved.
func RemoveCordonEntries(repoRoot string) error {
	// Remove hook from settings.local.json
	settingsPath := filepath.Join(repoRoot, settingsRelPath)
	settingsData, err := readSettings(settingsPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if settingsData != nil {
		removeHookEntry(settingsData)
		removeMCPEntry(settingsData) // clean up any legacy entry too
		removeEnabledMCPServer(settingsData)
		removeMCPToolPermission(settingsData)
		if err := writeAtomic(settingsPath, settingsData); err != nil {
			return err
		}
	}

	// Remove MCP entry from .mcp.json
	mcpPath := filepath.Join(repoRoot, mcpRelPath)
	mcpData, err := readSettings(mcpPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if mcpData != nil {
		removeMCPEntry(mcpData)
		if err := writeAtomic(mcpPath, mcpData); err != nil {
			return err
		}
	}

	return nil
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
		"type":    "stdio",
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

// addEnabledMCPServer adds "cordon" to the enabledMcpjsonServers array,
// which permits Claude Code to start the MCP server automatically. Idempotent.
func addEnabledMCPServer(data map[string]interface{}) {
	enabled := getOrCreateSlice(data, "enabledMcpjsonServers")
	for _, v := range enabled {
		if s, ok := v.(string); ok && s == cordonMCPKey {
			return
		}
	}
	data["enabledMcpjsonServers"] = append(enabled, cordonMCPKey)
}

// removeEnabledMCPServer removes "cordon" from enabledMcpjsonServers.
func removeEnabledMCPServer(data map[string]interface{}) {
	raw, ok := data["enabledMcpjsonServers"]
	if !ok {
		return
	}
	slice, ok := raw.([]interface{})
	if !ok {
		return
	}
	filtered := slice[:0]
	for _, v := range slice {
		if s, ok := v.(string); ok && s == cordonMCPKey {
			continue
		}
		filtered = append(filtered, v)
	}
	if len(filtered) == 0 {
		delete(data, "enabledMcpjsonServers")
	} else {
		data["enabledMcpjsonServers"] = filtered
	}
}

// addMCPToolPermission adds the cordon MCP tool to the permissions allow list
// so agents can invoke it without a manual approval prompt. Idempotent.
func addMCPToolPermission(data map[string]interface{}) {
	perms := getOrCreateMap(data, "permissions")
	allow := getOrCreateSlice(perms, "allow")
	for _, v := range allow {
		if s, ok := v.(string); ok && s == cordonMCPToolPerm {
			return
		}
	}
	perms["allow"] = append(allow, cordonMCPToolPerm)
	data["permissions"] = perms
}

// removeMCPToolPermission removes the cordon MCP tool from the permissions allow list.
func removeMCPToolPermission(data map[string]interface{}) {
	permsRaw, ok := data["permissions"]
	if !ok {
		return
	}
	perms, ok := permsRaw.(map[string]interface{})
	if !ok {
		return
	}
	allowRaw, ok := perms["allow"]
	if !ok {
		return
	}
	allow, ok := allowRaw.([]interface{})
	if !ok {
		return
	}
	filtered := allow[:0]
	for _, v := range allow {
		if s, ok := v.(string); ok && s == cordonMCPToolPerm {
			continue
		}
		filtered = append(filtered, v)
	}
	if len(filtered) == 0 {
		delete(perms, "allow")
	} else {
		perms["allow"] = filtered
	}
	if len(perms) == 0 {
		delete(data, "permissions")
	} else {
		data["permissions"] = perms
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
