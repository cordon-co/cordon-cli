# Cordon — Current Task

## Summary

**Agent identification in hooks + improved `cordon log`**

1. Added `--agent` flag to `cordon hook` so each agent platform self-identifies in audit logs. Each agent's hook installation now writes `cordon hook --agent <id>` (e.g. `claude-code`, `cursor`, `gemini-cli`, `vs-copilot`, `opencode`). The agent value is stored in the existing `agent` column of `hook_log` and `audit_log`.

2. Improved `cordon log` with new filtering and streaming capabilities:
   - Default time window: last 24 hours (was unlimited)
   - `--date 2026-03-22` — filter to a specific calendar date
   - `--agent claude-code` — filter by agent platform
   - `--follow` / `-f` — stream new entries in real-time (1s poll)
   - All flags work with `--json` (follow mode uses NDJSON)

## Key Files

- `cli/cmd/hook.go` — `--agent` flag on hook command, flows into `HookLogEntry.Agent`
- `cli/cmd/log.go` — new `--date`, `--agent`, `--follow` flags; default 24h window; follow polling loop
- `cli/internal/claudecfg/claudecfg.go` — parameterized hook command (`CordonHookCommand(agent)`), prefix-based detection for backwards compat
- `cli/internal/store/logview.go` — `Agent` and `Until` fields on `LogFilter`, applied in both query functions
- `cli/internal/agents/*.go` — each agent passes its ID to hook entry functions
- `cli/internal/agents/opencode.go` — plugin JS updated with `--agent opencode` spawn args

## Previously Completed

- **OpenCode Support / Uninstall Robustness** (INS-04, INS-05)
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
