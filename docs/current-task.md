# Cordon — Current Task

## Summary

Implementing `cordon log` — the user-facing audit log viewer (AUD-04, AUD-05).

## In Progress

**Goal:** `cordon log` with pager-style output (like `git log`), filtering flags, and CSV export.

**Key files:**
- `cli/internal/store/logview.go` — NEW: unified query over `hook_log` + `audit_log`, `LogFilter`, `UnifiedEntry`
- `cli/cmd/log.go` — REWRITE: pager, ANSI colours, flag wiring, JSON, CSV

**Design decisions:**
- `hook_log` (raw hook invocations) and `audit_log` (zone/pass/integrity events) are queried separately and merged in memory, sorted newest-first.
- `--denied-only` filters `hook_log` to `decision='deny'` only; `audit_log` is excluded (zone/pass events have no allow/deny concept).
- `--file <substr>` is a substring match on `file_path` in both tables.
- `--since` accepts standard Go durations (`24h`, `90m`) plus a `d` shorthand (`7d`).
- Paged display via `less -RFX` when stdout is a TTY; respects `$PAGER`.
- ANSI colour badges: `DENY` (bold red), `ALLOW` (green), `ZONE+` (yellow), `ZONE-` (red), `PASS+` (cyan), `PASS-` (red), `PASS!` (dim).

## Previously Completed

- **store/schema.go** — added `MigratePolicyDB()` (zones table) and extended `MigrateDataDB()` (passes + audit_log tables)
- **store/match.go** — `pathMatchesZone` helper (exact, glob, directory-prefix)
- **store/policy.go** — `Zone` struct + `AddZone`, `ListZones`, `RemoveZone`, `ZoneForPath`
- **store/passes.go** — `Pass` struct + `IssuePass`, `ListPasses`, `RevokePass`, `ActivePassForPath`, `ExpireStale`
- **store/audit.go** — `AuditEntry` + `InsertAudit`, `ListAudit`
- **internal/codexpolicy/codexpolicy.go** — `Generate()` writes `.cordon/codex-policy.md`
- **internal/hook/hook.go** — added `PolicyChecker` type; replaced global deny with zone+pass check
- **cmd/hook.go** — builds `PolicyChecker` (opens policy/data DBs from cwd, fail-open on error)
- **cmd/zone/add.go** — `cordon zone add [--guardian] <pattern>`
- **cmd/zone/list.go** — `cordon zone list`
- **cmd/zone/zoneremove.go** — `cordon zone remove <pattern>`
- **cmd/pass/issue.go** — `cordon pass issue --file <path> --duration <dur>`
- **cmd/pass/list.go** — `cordon pass list` (auto-expires stale passes)
- **cmd/pass/revoke.go** — `cordon pass revoke <pass-id>`
- **cmd/init.go** — added `MigratePolicyDB()` + codex-policy.md generation

## Next Steps (after this task)

- **HOK-06** — `cordon init` writes `.codex/config.toml` with `model_instructions_file` reference
- **CLI-03/04** — `cordon login` / `cordon logout` (GitHub OAuth)
- **CLI-05** — `cordon status` with auth state, policy summary, integrity check
- **MCP-01/03/05** — `cordon --mcp` stdio MCP server
- **INT-01..06** — `cordon check` integrity verification
