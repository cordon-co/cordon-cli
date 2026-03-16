# Cordon — Current Task

## Summary

Completed: `cordon --mcp` stdio MCP server with `cordon_request_access` tool (MCP-01, MCP-03, MCP-05).

## Last Completed

- **cli/internal/mcpserver/mcpserver.go** — NEW: stdio MCP server using `mcp-go`. Registers
  `cordon_request_access` tool — issues a 60-minute pass for a zoned file, audit-logs via
  `store.InsertAudit`, returns pass ID + expiry as text.
- **cli/cmd/root.go** — `runMCPServer()` stub replaced with `mcpserver.Run(ctx)`.
- **cli/go.mod / go.sum** — added `github.com/mark3labs/mcp-go v0.45.0`.

## Previously Completed

- **store/logview.go** — NEW: `LogFilter`, `UnifiedEntry`, `ListUnifiedLog`
- **cmd/log.go** — REWRITE: pager, ANSI colour badges, filters, `--export csv`
- **store/schema.go** — `MigratePolicyDB()` + extended `MigrateDataDB()`
- **store/match.go** — `pathMatchesZone` helper
- **store/policy.go** — Zone CRUD + `ZoneForPath`
- **store/passes.go** — Pass CRUD + `ActivePassForPath`, `ExpireStale`
- **store/audit.go** — `AuditEntry` + `InsertAudit`, `ListAudit`
- **internal/codexpolicy/codexpolicy.go** — `Generate()`
- **internal/hook/hook.go** — `PolicyChecker` + zone+pass enforcement
- **cmd/hook.go** — builds `PolicyChecker`
- **cmd/zone/add.go** — `cordon zone add [--guardian] <pattern>`
- **cmd/zone/list.go** — `cordon zone list`
- **cmd/zone/zoneremove.go** — `cordon zone remove <pattern>`
- **cmd/pass/issue.go** — `cordon pass issue`
- **cmd/pass/list.go** — `cordon pass list`
- **cmd/pass/revoke.go** — `cordon pass revoke`
- **cmd/init.go** — init with DB migration + codex policy

## Next Steps

- **HOK-06** — `cordon init` writes `.codex/config.toml` with `model_instructions_file`
- **CLI-03/04** — `cordon login` / `cordon logout` (GitHub OAuth)
- **CLI-05** — `cordon status`
- **MCP-03 elicitation** — surface pass requests to the human via VS Code extension
- **INT-01..06** — `cordon check` integrity verification
