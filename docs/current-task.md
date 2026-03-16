# Cordon — Current Task

## Summary

Completed: SQLite schemas for zones and passes, zone/pass CLI commands, hook policy checking,
codex-policy.md generation, and audit logging for zone/pass events.

## Last Completed

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

## Next Steps (potential)

- **HOK-06** — `cordon init` writes `.codex/config.toml` with `model_instructions_file` reference
- **CLI-03/04** — `cordon login` / `cordon logout` (GitHub OAuth)
- **CLI-05** — `cordon status` with auth state, policy summary, integrity check
- **AUD-04/05** — `cordon log` with filtering and CSV export
- **MCP-01/03/05** — `cordon --mcp` stdio MCP server
- **INT-01..06** — `cordon check` integrity verification
