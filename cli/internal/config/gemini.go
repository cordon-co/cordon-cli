package config

// Gemini CLI config paths.
const (
	GeminiSettingsRelPath = ".gemini/settings.json"
)

// AddGeminiHookEntry inserts the Cordon hook group into the BeforeTool array
// of a .gemini/settings.json file. Idempotent: does nothing if already present.
func AddGeminiHookEntry(data map[string]interface{}, agent string) {
	cmd := CordonHookCommand(agent)
	hooks := GetOrCreateMap(data, "hooks")
	beforeTool := GetOrCreateSlice(hooks, "BeforeTool")

	if updateCordonHookGroupCommand(beforeTool, cmd) {
		return
	}

	newGroup := map[string]interface{}{
		"hooks": []interface{}{
			map[string]interface{}{
				"name":    "cordon-hook",
				"type":    "command",
				"command": cmd,
			},
		},
	}
	hooks["BeforeTool"] = append(beforeTool, newGroup)
	data["hooks"] = hooks
}

// RemoveGeminiHookEntry removes the Cordon hook group from the BeforeTool array
// of a .gemini/settings.json file.
func RemoveGeminiHookEntry(data map[string]interface{}) {
	hooksRaw, ok := data["hooks"]
	if !ok {
		return
	}
	hooks, ok := hooksRaw.(map[string]interface{})
	if !ok {
		return
	}

	btRaw, ok := hooks["BeforeTool"]
	if !ok {
		return
	}
	bt, ok := btRaw.([]interface{})
	if !ok {
		return
	}

	filtered := bt[:0]
	for _, item := range bt {
		if !isCordonHookGroup(item) {
			filtered = append(filtered, item)
		}
	}

	if len(filtered) == 0 {
		delete(hooks, "BeforeTool")
	} else {
		hooks["BeforeTool"] = filtered
	}

	if len(hooks) == 0 {
		delete(data, "hooks")
	} else {
		data["hooks"] = hooks
	}
}

// HasGeminiCordonHook reports whether the BeforeTool slice already contains
// a Cordon hook group.
func HasGeminiCordonHook(bt []interface{}) bool {
	for _, item := range bt {
		if isCordonHookGroup(item) {
			return true
		}
	}
	return false
}
