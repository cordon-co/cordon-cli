# Cordon — Current Task

## Summary

Completed: **Cursor Agent Support** — implemented full Cursor IDE agent integration. Cursor already uses `.claude/settings.local.json` for hook enforcement (shared with Claude Code), so only the MCP server entry needed Cursor-specific configuration. Added `.cursor/mcp.json` support using the `mcpServers` key format. Cursor now appears as an installable agent in `cordon init` TUI and is properly cleaned up by `cordon remove`.

## Key Files

- `cli/internal/agents/cursor.go` — full Install/Remove/Installed implementation (was a stub)
- `cli/internal/claudecfg/claudecfg.go` — added `CursorMCPRelPath` constant

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
