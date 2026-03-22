# Cordon — Current Task

## Summary

Completed: **Gemini CLI Support** — implemented full Gemini CLI agent integration. Installs a `BeforeTool` hook in `.gemini/settings.json` using the nested group format Gemini CLI expects. Added `write_file` and `replace` to the hook enforcement's `writingTools` map so writes via Gemini CLI are correctly blocked by file rules.

## Key Files

- `cli/internal/agents/geminicli.go` — full Install/Remove/Installed implementation (was a stub)
- `cli/internal/claudecfg/claudecfg.go` — added `GeminiSettingsRelPath` constant and `AddGeminiHookEntry`/`RemoveGeminiHookEntry`/`HasGeminiCordonHook`
- `cli/internal/hook/hook.go` — added `write_file`, `replace` to `writingTools`; `read_file` already covered

## Previously Completed

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
