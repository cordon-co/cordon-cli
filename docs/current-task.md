# Cordon — Current Task

## Summary

**OpenCode Support** — implementing OpenCode agent integration via a JavaScript plugin installed to `.opencode/plugins/cordon-interface.js`. Unlike other agents that use JSON config hooks, OpenCode uses a JS plugin system. The plugin hooks `tool.execute.before`, formats the tool call as a `cordon hook` JSON payload, spawns `cordon hook` as a subprocess, and throws an error on deny.

## Key Files

- `cli/internal/agents/opencode.go` — full Install/Remove/Installed implementation (currently a stub)
- `cli/internal/hook/hook.go` — add OpenCode tool names (`write`, `edit`, `patch`) to `writingTools`; `read` to `readingTools`
- Plugin template embedded in `opencode.go` or a separate file — the JS content written to `.opencode/plugins/cordon-interface.js`

## OpenCode Tool Names (from docs)

Writing: `write`, `edit`, `patch`
Reading: `read`, `grep`
Shell: `bash`
Other: `glob`, `list`, `lsp`, `skill`, `todowrite`, `todoread`, `webfetch`, `websearch`, `question`

## Plugin Hook API

```javascript
export const CordonInterface = async ({ directory }) => {
  return {
    "tool.execute.before": async (input, output) => {
      // input.tool = tool name, output.args = tool arguments
      // Spawn `cordon hook` with JSON payload on stdin
      // Exit 2 = deny (throw Error), Exit 0 = allow
    }
  }
}
```

## Previously Completed

- **Gemini CLI Support**
- **CLI-07 — Interactive Agent Platform Selection on Init**
- **Zone → File Rule Rename**
- **Command Rule Type/Authority Refactor (CMD-07)**
- **File Rule Type / Authority Refactor (FIL-08)**
- **File Rule Read Prevention + Guardrail File Rules**
- **Command Rules (CMD-01 through CMD-06)**
- **Tests** — 17 tests across `cli/internal/store/` and `cli/tests/`

## Next Steps (after this task)

1. **SAF-01 (remaining)** — add destructive command built-in rules
2. **CLI-03/04** — `cordon login` / `cordon logout`
3. **CLI-05** — `cordon status`
4. **INT-01..06** — `cordon check` integrity verification
