# Cordon — Current Task

## Summary

No active task. Ready for next assignment.

## Previously Completed

- **Pass traceability in hook audit logs** — `pass_id` column added to `hook_log` table, `HookLogEntry` struct, `InsertHookLog`, `logHookEvent`, and `queryHookLog` SELECT/scan. Pass-authorized hook decisions are now fully traceable via `cordon log --json`


- **Agent identification in hooks** — `--agent` flag on `cordon hook`, each agent install writes its ID
- **Improved `cordon log`** — default 24h, `--date`, `--agent`, `--follow`/`-f`, NDJSON streaming
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
