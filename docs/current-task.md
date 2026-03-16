# Cordon — Current Task

## Summary

Implementing the first real capabilities: `cordon init` sets up the repo (creates `.cordon/policy.db`,
writes the hook entry into `.claude/settings.local.json`), `cordon hook` parses stdin JSON and returns
a mock denial response, and the two SQLite stores get their schema stubs. Also adding `dev-uninstall.sh`.

## Relevant Requirements

- **CLI-02** — `cordon init` creates `.cordon/` with `config.json` and `policy.db`
- **HOK-01** — `cordon hook` reads JSON payload from stdin and checks file path against policy
- **HOK-02** — Returns exit 0 (allow) or exit 2 with JSON deny response including guidance
- **HOK-03** — `cordon init` writes PreToolUse hook entry to `.claude/settings.local.json`
- **HOK-04** — Hook integration is additive (appends, does not overwrite existing entries)
- **HOK-08** — Hook queries local policy database
- **MCP-04** — `cordon init` adds MCP server entry to `.claude/settings.local.json`
- **INS-04** / **INS-05** — `cordon remove` undoes the above changes surgically (hook + MCP entries only)

## Key Files

- `cli/internal/claudecfg/settings.go` — read/write `.claude/settings.local.json` additively
- `cli/internal/store/policy.go` — open/create `.cordon/policy.db` (SQLite, WAL mode)
- `cli/internal/store/data.go` — open/create `~/.cordon/repos/<repo-hash>/data.db` (SQLite, WAL mode)
- `cli/internal/hook/hook.go` — parse stdin JSON payload, evaluate, write allow/deny response
- `cli/cmd/init.go` — wire up the above into a working init flow
- `cli/cmd/hook.go` — call into internal/hook
- `cli/scripts/dev-uninstall.sh` — remove the dev-installed binary

## Notes

- SQLite driver: `modernc.org/sqlite` (pure Go, no CGO — required for cross-compilation)
- Repo identity for data.db path: SHA-256 of the absolute path to the repo root (the directory
  containing `.cordon/`), hex-encoded, used as `~/.cordon/repos/<hash>/`
- Hook JSON input comes from Claude Code PreToolUse; format is `{"tool_name":"Write","tool_input":{"path":"..."}}`
- Hook denial exit code is 2; the JSON body is `{"decision":"block","reason":"..."}`
- `.claude/settings.local.json` may not exist; must be created with correct structure if absent
- Cordon entries in settings must be identifiable for surgical removal by `cordon remove`
- Schema can be left empty for now (tables will be added when zone/pass logic is built)
- dev-uninstall.sh only removes the binary; it does not touch repo config
