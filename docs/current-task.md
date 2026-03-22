# Cordon — Current Task

## Summary

**OpenCode Support / Uninstall Robustness** — completed a bugfix for `cordon uninstall` where OpenCode JSONC parsing failed on valid string content containing `//` (for example schema URLs like `https://...`). `cli/internal/agents/opencode.go` now performs quote-aware comment stripping and trailing-comma removal before JSON parsing, so uninstall cleanly removes Cordon-managed OpenCode config entries without corrupting non-Cordon values.

Relevant requirements: **INS-04**, **INS-05**

## Key Files

- `cli/internal/agents/opencode.go` — JSONC sanitization fix used by OpenCode config read during uninstall
- `cli/internal/agents/opencode_test.go` — regression tests for URL-preserving JSONC parsing and trailing comma/comment handling

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

## Next Steps

1. **SAF-01 (remaining)** — add destructive command built-in rules
2. **CLI-03/04** — `cordon login` / `cordon logout`
3. **CLI-05** — `cordon status`
4. **INT-01..06** — `cordon check` integrity verification
