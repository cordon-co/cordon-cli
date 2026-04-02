package config

// Claude Code config paths.
const (
	SettingsRelPath   = ".claude/settings.local.json"
	MCPRelPath        = ".mcp.json"
	CordonMCPToolPerm = "mcp__cordon__cordon_request_access"
)

// AddHookEntry inserts the Cordon hook group into the PreToolUse array
// of a Claude Code settings.local.json file. If a Cordon entry already exists,
// its command is updated. Otherwise a new entry is created.
// Also used by Codex, which shares the same hooks.json format.
func AddHookEntry(data map[string]interface{}, agent string) {
	cmd := CordonHookCommand(agent)
	hooks := GetOrCreateMap(data, "hooks")
	preToolUse := GetOrCreateSlice(hooks, "PreToolUse")

	if updateCordonHookGroupCommand(preToolUse, cmd) {
		return
	}

	newGroup := map[string]interface{}{
		"matcher": CordonMatcher,
		"hooks": []interface{}{
			map[string]interface{}{
				"type":    "command",
				"command": cmd,
			},
		},
	}
	hooks["PreToolUse"] = append(preToolUse, newGroup)
	data["hooks"] = hooks
}

// RemoveHookEntry removes the Cordon hook group from the PreToolUse array.
// Also used by Codex.
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
