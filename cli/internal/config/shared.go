// Package claudecfg provides helpers for managing agent config files.
// Each supported agent platform has its own file in this package containing
// platform-specific hook, MCP, and permission helpers. Shared utilities
// (JSON read/write, map/slice helpers) live in this file.
package config

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
	cordonHookBase = "cordon hook"
	CordonMatcher  = "*"
	CordonMCPKey   = "cordon"
)

// CordonHookCommand returns the hook command string for the given agent.
// If agent is empty, returns the base command without an --agent flag.
func CordonHookCommand(agent string) string {
	if agent == "" {
		return cordonHookBase
	}
	return cordonHookBase + " --agent " + agent
}

// ReadSettings reads and unmarshals a JSON settings file into a generic map.
// Returns an empty map if the file does not exist.
func ReadSettings(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]interface{}{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return data, nil
}

// WriteAtomic marshals data and writes it to dst atomically via a temp file
// in the same directory, then renames. Creates the parent directory if needed.
func WriteAtomic(dst string, data map[string]interface{}) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("config: create directory: %w", err)
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("config: marshal: %w", err)
	}
	content = append(content, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(dst), ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("config: create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("config: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: rename to %s: %w", dst, err)
	}

	return nil
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

// --- Shared hook identification helpers ---
// These are used by agents that share the Claude Code hook JSON format
// (matcher group with inner hooks array): Claude Code, Codex, Gemini.

// HasCordonHook reports whether the given slice already contains a
// Cordon hook group (identified by the command string).
func HasCordonHook(ptu []interface{}) bool {
	for _, item := range ptu {
		if isCordonHookGroup(item) {
			return true
		}
	}
	return false
}

// isCordonHookGroup reports whether a hook array element is the Cordon
// hook group, identified by any inner hook whose command starts with "cordon hook".
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
		if cmd, ok := hm["command"].(string); ok && strings.HasPrefix(cmd, cordonHookBase) {
			return true
		}
	}
	return false
}

// updateCordonHookGroupCommand finds the existing Cordon hook group and updates
// its inner hook command string. Returns true if found (regardless of whether
// the command changed).
func updateCordonHookGroupCommand(ptu []interface{}, cmd string) bool {
	for _, item := range ptu {
		group, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		hooksRaw, ok := group["hooks"]
		if !ok {
			continue
		}
		innerHooks, ok := hooksRaw.([]interface{})
		if !ok {
			continue
		}
		for _, h := range innerHooks {
			hm, ok := h.(map[string]interface{})
			if !ok {
				continue
			}
			if c, ok := hm["command"].(string); ok && strings.HasPrefix(c, cordonHookBase) {
				hm["command"] = cmd
				return true
			}
		}
	}
	return false
}

// --- Shared MCP helpers ---
// Used by agents that store MCP config in JSON under "mcpServers" (Claude Code, Cursor).

// AddMCPEntry inserts the Cordon MCP server entry under "mcpServers". Idempotent.
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

// RemoveMCPEntry removes the Cordon MCP server entry from "mcpServers".
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

// --- Shared permission helpers ---

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
