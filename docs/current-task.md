# Cordon — Current Task

## Summary

Implementing `cordon --mcp` — a stdio MCP server (MCP-01, MCP-03, MCP-05) using the
`mcp-go` framework. The initial tool is `cordon_request_access`: an agent calls this
when denied a write, provides a file path and reason, and receives a temporary pass.

## Key Files

- **cli/cmd/root.go** — replace stub `runMCPServer()` with a real call
- **cli/internal/mcpserver/mcpserver.go** — NEW: MCP server + `cordon_request_access` tool
- **cli/go.mod / go.sum** — add `github.com/mark3labs/mcp-go`

## Approach

1. `go get github.com/mark3labs/mcp-go` to add the dependency
2. Create `cli/internal/mcpserver/` package
3. `Run(ctx)` opens policy.db + data.db from the repo root (same pattern as `cmd/hook.go`),
   creates an `mcp.NewServer`, registers `cordon_request_access`, and calls
   `server.ServeStdio()` which blocks until client disconnects
4. `cordon_request_access` accepts `file_path` (string, required) and `reason` (string, optional):
   - Resolve repo root via `reporoot.Find()`
   - Open policy.db → `ZoneForPath` → error if not in any zone
   - Issue pass (60m default) via `store.IssuePass`
   - Audit log via `store.InsertAudit`
   - Return pass ID and expiry as text
5. Wire `runMCPServer()` in root.go to call `mcpserver.Run(ctx)`

## Requirement IDs

MCP-01, MCP-03, MCP-05

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

## Next Steps (after this task)

- **HOK-06** — `cordon init` writes `.codex/config.toml`
- **CLI-03/04** — `cordon login` / `cordon logout`
- **CLI-05** — `cordon status`
- **INT-01..06** — `cordon check` integrity verification
