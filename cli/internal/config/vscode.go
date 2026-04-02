package config

// VS Code Copilot config paths.
const (
	VSCodeMCPRelPath  = ".vscode/mcp.json"
	VSCodeHookRelPath = ".github/hooks/cordon.json"
)

// WriteVSCodeHookFile writes the VS Code Copilot hook file at the given path.
// The file is a standalone JSON config (not merged into an existing file),
// so it is written atomically and is idempotent.
func WriteVSCodeHookFile(path string, agent string) error {
	data := map[string]interface{}{
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{
					"type":    "command",
					"command": CordonHookCommand(agent),
				},
			},
		},
	}
	return WriteAtomic(path, data)
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
