package config

import "strings"

// Cursor config paths.
const (
	CursorMCPRelPath  = ".cursor/mcp.json"
	CursorCLIRelPath  = ".cursor/cli.json"
	CursorHookRelPath = ".cursor/hooks.json"
	CursorMCPToolPerm = "Mcp(cordon:*)"
)

// AddCursorHookEntry inserts the Cordon hook into the preToolUse array
// in a Cursor hooks.json file. Idempotent: does nothing if already present.
// Preserves existing hooks and ensures version field is set.
func AddCursorHookEntry(data map[string]interface{}, agent string) {
	cmd := CordonHookCommand(agent)

	// Ensure version field exists.
	if _, ok := data["version"]; !ok {
		data["version"] = float64(1)
	}

	hooks := GetOrCreateMap(data, "hooks")
	preToolUse := GetOrCreateSlice(hooks, "preToolUse")

	if updateCursorCordonHookCommand(preToolUse, cmd) {
		return
	}

	newEntry := map[string]interface{}{
		"command": cmd,
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
	for _, item := range ptu {
		if isCursorCordonHook(item) {
			return true
		}
	}
	return false
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

func isCursorCordonHook(item interface{}) bool {
	entry, ok := item.(map[string]interface{})
	if !ok {
		return false
	}
	cmd, ok := entry["command"].(string)
	return ok && strings.HasPrefix(cmd, cordonHookBase)
}

func updateCursorCordonHookCommand(ptu []interface{}, cmd string) bool {
	for _, item := range ptu {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		c, ok := entry["command"].(string)
		if ok && strings.HasPrefix(c, cordonHookBase) {
			entry["command"] = cmd
			return true
		}
	}
	return false
}
