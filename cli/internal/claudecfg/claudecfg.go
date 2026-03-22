// Package claudecfg provides helpers for managing JSON config files used by
// Claude Code and VS Code agents. Functions are exported for use by the
// agents package, which owns per-platform install/remove orchestration.
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
	CordonCommand     = "cordon hook"
	CordonMatcher     = "*"
	CordonMCPKey      = "cordon"
	CordonMCPToolPerm = "mcp__cordon__cordon_request_access"
	SettingsRelPath   = ".claude/settings.local.json"
	MCPRelPath        = ".mcp.json"
	VSCodeMCPRelPath  = ".vscode/mcp.json"
	VSCodeHookRelPath = ".github/hooks/cordon.json"
	CursorMCPRelPath   = ".cursor/mcp.json"
	CursorCLIRelPath   = ".cursor/cli.json"
	CursorHookRelPath  = ".cursor/hooks.json"
	CursorMCPToolPerm  = "Mcp(cordon:*)"
)

// ReadSettings reads and unmarshals the settings file into a generic map.
// Returns an empty map if the file does not exist.
func ReadSettings(path string) (map[string]interface{}, error) {
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

// AddHookEntry inserts the Cordon hook group into the PreToolUse array.
// Idempotent: does nothing if a Cordon entry is already present.
func AddHookEntry(data map[string]interface{}) {
	hooks := GetOrCreateMap(data, "hooks")
	preToolUse := GetOrCreateSlice(hooks, "PreToolUse")

	if HasCordonHook(preToolUse) {
		return
	}

	newGroup := map[string]interface{}{
		"matcher": CordonMatcher,
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": CordonCommand,
			},
		},
	}
	hooks["PreToolUse"] = append(preToolUse, newGroup)
	data["hooks"] = hooks
}

// RemoveHookEntry removes the Cordon hook group from the PreToolUse array.
func RemoveHookEntry(data map[string]interface{}) {
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

// WriteVSCodeHookFile writes the VS Code Copilot hook file at the given path.
// The file is a standalone JSON config (not merged into an existing file),
// so it is written atomically and is idempotent.
func WriteVSCodeHookFile(path string) error {
	data := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": CordonCommand,
				},
			},
		},
	}
	return WriteAtomic(path, data)
}

// AddCursorHookEntry inserts the Cordon hook into the preToolUse array
// in a Cursor hooks.json file. Idempotent: does nothing if already present.
// Preserves existing hooks and ensures version field is set.
func AddCursorHookEntry(data map[string]interface{}) {
	// Ensure version field exists.
	if _, ok := data["version"]; !ok {
		data["version"] = float64(1)
	}

	hooks := GetOrCreateMap(data, "hooks")
	preToolUse := GetOrCreateSlice(hooks, "preToolUse")

	if hasCursorCordonHook(preToolUse) {
		return
	}

	newEntry := map[string]interface{}{
		"command": CordonCommand,
	}
	hooks["preToolUse"] = append(preToolUse, newEntry)
	data["hooks"] = hooks
}

// RemoveCursorHookEntry removes the Cordon hook from the preToolUse array
// in a Cursor hooks.json file.
func RemoveCursorHookEntry(data map[string]interface{}) {
	hooksRaw, ok := data["hooks"]
	if !ok {
		return
	}
	hooks, ok := hooksRaw.(map[string]interface{})
	if !ok {
		return
	}

	ptuRaw, ok := hooks["preToolUse"]
	if !ok {
		return
	}
	ptu, ok := ptuRaw.([]interface{})
	if !ok {
		return
	}

	filtered := ptu[:0]
	for _, item := range ptu {
		if isCursorCordonHook(item) {
			continue
		}
		filtered = append(filtered, item)
	}

	if len(filtered) == 0 {
		delete(hooks, "preToolUse")
	} else {
		hooks["preToolUse"] = filtered
	}

	if len(hooks) == 0 {
		delete(data, "hooks")
	} else {
		data["hooks"] = hooks
	}
}

// HasCursorCordonHook reports whether the preToolUse slice contains a
// Cordon hook entry.
func HasCursorCordonHook(ptu []interface{}) bool {
	return hasCursorCordonHook(ptu)
}

func hasCursorCordonHook(ptu []interface{}) bool {
	for _, item := range ptu {
		if isCursorCordonHook(item) {
			return true
		}
	}
	return false
}

func isCursorCordonHook(item interface{}) bool {
	entry, ok := item.(map[string]interface{})
	if !ok {
		return false
	}
	cmd, ok := entry["command"].(string)
	return ok && cmd == CordonCommand
}

// AddVSCodeMCPEntry inserts the Cordon MCP server entry into VS Code's
// .vscode/mcp.json format (uses "servers" key). Idempotent.
func AddVSCodeMCPEntry(data map[string]interface{}) {
	servers := GetOrCreateMap(data, "servers")
	if _, exists := servers[CordonMCPKey]; exists {
		return
	}
	servers[CordonMCPKey] = map[string]interface{}{
		"type":    "stdio",
		"command": "cordon",
		"args":    []interface{}{"--mcp"},
	}
	data["servers"] = servers
}

// RemoveVSCodeMCPEntry removes the Cordon entry from .vscode/mcp.json.
func RemoveVSCodeMCPEntry(data map[string]interface{}) {
	serversRaw, ok := data["servers"]
	if !ok {
		return
	}
	servers, ok := serversRaw.(map[string]interface{})
	if !ok {
		return
	}
	delete(servers, CordonMCPKey)
	if len(servers) == 0 {
		delete(data, "servers")
	} else {
		data["servers"] = servers
	}
}

// AddMCPEntry inserts the Cordon MCP server entry. Idempotent.
func AddMCPEntry(data map[string]interface{}) {
	servers := GetOrCreateMap(data, "mcpServers")
	if _, exists := servers[CordonMCPKey]; exists {
		return
	}
	servers[CordonMCPKey] = map[string]interface{}{
		"type":    "stdio",
		"command": "cordon",
		"args":    []interface{}{"--mcp"},
	}
	data["mcpServers"] = servers
}

// RemoveMCPEntry removes the Cordon MCP server entry.
func RemoveMCPEntry(data map[string]interface{}) {
	serversRaw, ok := data["mcpServers"]
	if !ok {
		return
	}
	servers, ok := serversRaw.(map[string]interface{})
	if !ok {
		return
	}

	delete(servers, CordonMCPKey)

	if len(servers) == 0 {
		delete(data, "mcpServers")
	} else {
		data["mcpServers"] = servers
	}
}

// AddEnabledMCPServer adds "cordon" to the enabledMcpjsonServers array,
// which permits Claude Code to start the MCP server automatically. Idempotent.
func AddEnabledMCPServer(data map[string]interface{}) {
	enabled := GetOrCreateSlice(data, "enabledMcpjsonServers")
	for _, v := range enabled {
		if s, ok := v.(string); ok && s == CordonMCPKey {
			return
		}
	}
	data["enabledMcpjsonServers"] = append(enabled, CordonMCPKey)
}

// RemoveEnabledMCPServer removes "cordon" from enabledMcpjsonServers.
func RemoveEnabledMCPServer(data map[string]interface{}) {
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
		if s, ok := v.(string); ok && s == CordonMCPKey {
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

// AddMCPToolPermission adds the cordon MCP tool to the permissions allow list
// so agents can invoke it without a manual approval prompt. Idempotent.
func AddMCPToolPermission(data map[string]interface{}) {
	addPermissionAllow(data, CordonMCPToolPerm)
}

// RemoveMCPToolPermission removes the cordon MCP tool from the permissions allow list.
func RemoveMCPToolPermission(data map[string]interface{}) {
	removePermissionAllow(data, CordonMCPToolPerm)
}

// AddCursorMCPToolPermission adds the Cursor-format cordon MCP permission
// to the permissions allow list. Idempotent.
func AddCursorMCPToolPermission(data map[string]interface{}) {
	addPermissionAllow(data, CursorMCPToolPerm)
}

// RemoveCursorMCPToolPermission removes the Cursor-format cordon MCP
// permission from the permissions allow list.
func RemoveCursorMCPToolPermission(data map[string]interface{}) {
	removePermissionAllow(data, CursorMCPToolPerm)
}

// addPermissionAllow adds a permission string to the permissions.allow array.
// Idempotent.
func addPermissionAllow(data map[string]interface{}, perm string) {
	perms := GetOrCreateMap(data, "permissions")
	allow := GetOrCreateSlice(perms, "allow")
	for _, v := range allow {
		if s, ok := v.(string); ok && s == perm {
			return
		}
	}
	perms["allow"] = append(allow, perm)
	data["permissions"] = perms
}

// removePermissionAllow removes a permission string from the permissions.allow array.
func removePermissionAllow(data map[string]interface{}, perm string) {
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
		if s, ok := v.(string); ok && s == perm {
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

// HasCordonHook reports whether the PreToolUse slice already contains a
// Cordon hook group (identified by the command string).
func HasCordonHook(ptu []interface{}) bool {
	for _, item := range ptu {
		if isCordonHookGroup(item) {
			return true
		}
	}
	return false
}

// isCordonHookGroup reports whether a PreToolUse array element is the Cordon
// hook group, identified by any inner hook with command == CordonCommand.
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
		if cmd, ok := hm["command"].(string); ok && cmd == CordonCommand {
			return true
		}
	}
	return false
}

// GetOrCreateMap retrieves a map[string]interface{} value from parent by key,
// creating and inserting a new empty map if the key is absent or the wrong type.
func GetOrCreateMap(parent map[string]interface{}, key string) map[string]interface{} {
	if v, ok := parent[key]; ok {
		if m, ok := v.(map[string]interface{}); ok {
			return m
		}
	}
	m := map[string]interface{}{}
	parent[key] = m
	return m
}

// GetOrCreateSlice retrieves a []interface{} value from parent by key,
// creating and inserting a new empty slice if the key is absent or the wrong type.
func GetOrCreateSlice(parent map[string]interface{}, key string) []interface{} {
	if v, ok := parent[key]; ok {
		if s, ok := v.([]interface{}); ok {
			return s
		}
	}
	s := []interface{}{}
	parent[key] = s
	return s
}

// WriteAtomic marshals data and writes it to dst atomically via a temp file
// in the same directory, then renames. Creates the parent directory if needed.
func WriteAtomic(dst string, data map[string]interface{}) error {
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
