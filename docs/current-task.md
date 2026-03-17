# Cordon — Current Task

## Summary

Completed: **Command Rules** — policy-based enforcement of shell commands (CMD-01 through CMD-06, SAF-01 in progress).

## Strategy: Command Rules

### Problem

Cordon currently enforces two things in the Bash hook:
1. **Zone policy** — file-write detection via regex patterns (redirections, tee, sed -i, cp, mv), checked against zones in policy.db.
2. **Hardcoded cordon block** — `isCordonCommand()` is a special case baked into hook.go.

This doesn't scale. SAF-01 requires blocking destructive commands (`git reset --hard`, `git push --force`, `rm -rf`). Users will want to add their own command restrictions. The `cordon` block is currently a hardcoded special case that should be a built-in rule, not a one-off.

### Design: Two Enforcement Axes in evaluateBash

The Bash hook needs to evaluate commands against two independent policy axes:

| Axis | What it protects | Storage | Example |
|------|-----------------|---------|---------|
| **Zones** (existing) | Files — blocks shell writes to zoned paths | `zones` table in policy.db | `*.env`, `src/core/` |
| **Command Rules** (new) | Commands — blocks dangerous or prohibited shell commands | `command_rules` table in policy.db | `cordon *`, `git reset --hard*`, `rm -rf *` |

A Bash command can be denied by either axis independently. The evaluation order is:
1. Check command rules first (cheap string/glob match, no DB needed for built-ins)
2. If allowed, check file-write targets against zones (existing flow)

### Command Rule Model

```
CommandRule {
    ID        string    // UUID
    Pattern   string    // glob-style match against the command string
    RuleType  string    // "builtin" or "custom"
    Severity  string    // "block" (hard deny) or "warn" (log + allow)
    Reason    string    // human-readable explanation shown in deny message
    CreatedBy string
    CreatedAt string
}
```

**Built-in rules** are not stored in the database. They are compiled into the binary and always active:
- `cordon *` — agents must not run cordon CLI directly
- (SAF-01, future) `git reset --hard*`, `git push --force*`, `rm -rf /*`

**Custom rules** are stored in `command_rules` table in policy.db, managed via `cordon rule add` / `cordon rule list` / `cordon rule remove`.

### Pattern Matching

Command rule patterns use glob-style matching against the full command string:
- `cordon *` matches `cordon pass issue --file foo`
- `git reset --hard*` matches `git reset --hard` and `git reset --hard HEAD~3`
- `rm -rf /*` matches `rm -rf /usr/local`

For compound commands (`cmd1 && cmd2`, `cmd1; cmd2`, `cmd1 | cmd2`), each segment is checked independently.

### Schema Change (policy.db)

```sql
CREATE TABLE IF NOT EXISTS command_rules (
    id         TEXT PRIMARY KEY,
    pattern    TEXT NOT NULL,
    rule_type  TEXT NOT NULL DEFAULT 'custom' CHECK(rule_type IN ('builtin','custom')),
    severity   TEXT NOT NULL DEFAULT 'block' CHECK(severity IN ('block','warn')),
    reason     TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_command_rules_pattern ON command_rules(pattern);
```

### Hook Changes

`evaluateBash` in hook.go becomes:

```
1. Parse command string
2. Split compound commands into segments
3. For each segment:
   a. Check against built-in command rules (hardcoded list)
   b. Check against custom command rules (from policy.db)
   c. If matched: deny with rule-specific reason
4. If no command rule matched:
   a. Extract write targets (existing bashWriteTargets logic)
   b. Check targets against zones (existing flow)
```

The `isCordonCommand()` function and its hardcoded deny message get replaced by the built-in rule mechanism.

### PolicyChecker Evolution

Currently `PolicyChecker` only checks file paths. We need a second checker type:

```go
type CommandChecker func(command, cwd string) (allowed bool, reason string)
```

`Evaluate` accepts both checkers. `cmd/hook.go` builds both from the policy database.

### CLI Commands

- `cordon rule add <pattern> [--reason "..."]` — add a custom command rule
- `cordon rule list` — list all command rules (built-in + custom)
- `cordon rule remove <pattern>` — remove a custom command rule

### Deny Message

Command rule denials use a distinct message format:

```
CORDON POLICY: This command is prohibited by a Cordon command rule.
Rule: <pattern>
Reason: <reason>
This is an enforced policy restriction, not a technical error.
```

### Key Files

- **cli/internal/store/schema.go** — add `command_rules` table to `MigratePolicyDB`
- **cli/internal/store/rules.go** — NEW: CommandRule CRUD + `MatchCommandRule`
- **cli/internal/hook/hook.go** — add `CommandChecker`, refactor `evaluateBash`
- **cli/internal/hook/rules_builtin.go** — NEW: built-in command rules list
- **cli/cmd/hook.go** — build `CommandChecker` alongside `PolicyChecker`
- **cli/cmd/rule/rule.go** — NEW: `cordon rule` subcommand group
- **cli/cmd/rule/add.go** — NEW: `cordon rule add`
- **cli/cmd/rule/list.go** — NEW: `cordon rule list`
- **cli/cmd/rule/remove.go** — NEW: `cordon rule remove`
- **cli/cmd/root.go** — register rule subcommand

### Requirement IDs

- **SAF-01** — built-in destructive command blocking (partial: `cordon *` rule replaces hardcoded block)
- New requirement needed for custom command rules (suggest **POL-01**: User-defined command rules)

## Last Completed

- **cli/internal/store/schema.go** — `command_rules` table added to `MigratePolicyDB`
- **cli/internal/store/rules.go** — NEW: `CommandRule` struct, `AddRule`, `ListRules`, `RemoveRule`, `MatchCommandRule`
- **cli/internal/hook/commandrule.go** — NEW: `CommandChecker` type, `MatchedRule`, built-in rules, `CheckBuiltinRules`, `BuiltinRulesAsStore`, `splitCompoundCommand`, `commandRuleDenyReason`
- **cli/internal/hook/hook.go** — `Evaluate` takes `CommandChecker`; `evaluateBash` uses splitter + built-in + custom rule checks; `isCordonCommand` removed
- **cli/cmd/hook.go** — `buildCommandChecker` added and wired into `Evaluate`
- **cli/cmd/rule/rule.go** — NEW: `cordon rule` parent command
- **cli/cmd/rule/add.go** — NEW: `cordon rule add <pattern> [--reason] [--severity]`
- **cli/cmd/rule/list.go** — NEW: `cordon rule list` (built-in + custom)
- **cli/cmd/rule/remove.go** — NEW: `cordon rule remove <pattern>`
- **cli/cmd/root.go** — `rule.Cmd` registered

## Previously Completed

- MCP elicitation with "Pass Approved" field naming
- Copilot hook support (.github/hooks/cordon.json, .vscode/mcp.json)
- Copilot deny message format (stderr for Copilot agents)
- `cordon` command prohibition in Bash hook
- `cordon pass list` shows active only, `--all` flag for history
- VS Code Copilot `apply_patch` path parsing

## Previously Completed

- **cli/internal/mcpserver/mcpserver.go** — MCP server with `cordon_request_access` + elicitation
- **cli/internal/claudecfg/claudecfg.go** — `enabledMcpjsonServers` + MCP permission entries
- **store/logview.go** — `LogFilter`, `UnifiedEntry`, `ListUnifiedLog`
- **cmd/log.go** — pager, ANSI colour badges, filters, `--export csv`
- **store/schema.go** — `MigratePolicyDB()` + extended `MigrateDataDB()`
- **store/match.go** — `pathMatchesZone` helper
- **store/policy.go** — Zone CRUD + `ZoneForPath`
- **store/passes.go** — Pass CRUD + `ActivePassForPath`, `ExpireStale`
- **store/audit.go** — `AuditEntry` + `InsertAudit`, `ListAudit`
- **internal/codexpolicy/codexpolicy.go** — `Generate()`
- **internal/hook/hook.go** — `PolicyChecker` + zone+pass enforcement
- **cmd/hook.go** — builds `PolicyChecker`
- **cmd/zone/** — zone CRUD commands
- **cmd/pass/** — pass CRUD commands
- **cmd/init.go** — init with DB migration + codex policy

## Next Steps

1. **SAF-01 (remaining)** — add destructive command built-in rules (`git reset --hard*`, `git push --force*`, `rm -rf /*`)
2. **HOK-06** — `cordon init` writes `.codex/config.toml`
3. **CLI-03/04** — `cordon login` / `cordon logout`
4. **CLI-05** — `cordon status`
5. **INT-01..06** — `cordon check` integrity verification
