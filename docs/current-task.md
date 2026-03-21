# Cordon — Current Task

## Summary

Completed: **CLI-07 — Interactive Agent Platform Selection on Init** — rearchitect `cordon init` to present an interactive checkbox TUI where users select which AI coding agents to configure. Introduces an agent registry pattern so each platform (Claude, Copilot, Codex, Cursor, Gemini CLI, KiloCode, OpenCode) has a self-contained installer. Stores selected agents in `perimeter_meta` so cordon knows which agents are configured for ongoing operations. Adds "already initialised" detection with informative message.

## Key Files

- `cli/cmd/init.go` — init command orchestration, TUI interaction
- `cli/internal/agentcfg/` — new package: agent registry, per-agent installers
- `cli/internal/agentcfg/claude.go` — Claude Code installer (extracted from claudecfg)
- `cli/internal/agentcfg/copilot.go` — VS Code Copilot installer (extracted from claudecfg)
- `cli/internal/agentcfg/codex.go` — Codex installer (new: writes .codex/config.toml)
- `cli/internal/agentcfg/cursor.go` — Cursor placeholder stub
- `cli/internal/agentcfg/gemini.go` — Gemini CLI placeholder stub
- `cli/internal/agentcfg/kilocode.go` — KiloCode placeholder stub
- `cli/internal/agentcfg/opencode.go` — OpenCode placeholder stub
- `cli/internal/claudecfg/claudecfg.go` — existing; will be refactored into agentcfg
- `cli/internal/store/schema.go` — perimeter_meta stores `installed_agents`
- `cli/internal/store/store.go` — new functions for reading/writing installed agents
- `cli/internal/tui/checkbox.go` — new: raw ANSI checkbox selector (no external deps)

## Relevant Requirement IDs

- CLI-07: Platform detection on init
- HOK-06: `cordon init` writes `.codex/config.toml`
- CLI-02: `cordon init` (enhancement)

## Previously Completed

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
