# Cordon — Current Task

## Summary

Previously completed: **Command Rule Type/Authority Refactor (CMD-07)** — mirrored the zone allow/deny + standard/guardian split onto command rules. `CommandRule` struct now has `RuleType` (allow/deny) and `RuleAuthority` (standard/guardian). Allow rules supersede deny rules in `MatchCommandRule`. Built-in rules remain absolute (guardian-authority deny, short-circuit before DB rules). `cordon command add --allow` creates allow rules. `cordon command list` shows TYPE, PATTERN, CREATED BY, CREATED AT. Removed `reason` column from command rules. Removed all backward-compat migrations (pre-release, no deployments to migrate).

Previously completed: **Zone Type / Zone Authority Refactor** — split `ZoneType` into allow/deny access control and `ZoneAuthority` for standard/guardian authorization (ZON-08).

Previously completed: **Zone Read Prevention + Guardrail Zones** — `prevent_read`/`prevent_write` columns, hook enforcement for read tools, `--prevent-read` flag, standard guardrail zones (ZON-07, SAF-02).

Previously completed: **Command Rules** — policy-based enforcement of shell commands (CMD-01 through CMD-06, SAF-01 in progress).

## Strategy: Zone Read Prevention + Guardrail Zones

### Problem

Zones currently only prevent **writes**. There is no way to prevent agents from **reading** sensitive files (e.g. `.env`, `credentials.json`) into their context. Additionally, `cordon init` offers guardrail command rules but no guardrail zones — users have to manually add zones for obvious secrets files.

### Design

**Two new boolean columns on `zones`:**

| Column | Default | Meaning |
|--------|---------|---------|
| `prevent_write` | `1` (true) | Blocks write tools (existing behaviour, now explicit) |
| `prevent_read` | `0` (false) | Also blocks read tools when enabled |

`prevent_write` is always true for now. `--prevent-read` sets both to true when adding a zone.

**Hook read enforcement:**

A new `ReadChecker` type (same signature as `PolicyChecker`) is consulted for "reading tools":
- `Read`, `NotebookRead`, `Grep` (Claude Code)
- Basic Bash read commands: `cat`, `head`, `tail`, `less`, `more`

If a file is in a zone with `prevent_read=true` and no active pass exists, the read is denied with an appropriate reason message.

**Standard guardrail zones (added on `cordon init` alongside command rules):**

| Pattern | Reason |
|---------|--------|
| `.env` | Credential exposure |
| `.env.*` | Credential exposure (.env.local, .env.production, etc.) |
| `.envrc` | Credential exposure (direnv) |
| `credentials.json` | Credential exposure |
| `secrets.json` | Credential exposure |
| `service-account.json` | Credential exposure |
| `*.pem` | Private key / certificate |
| `*.key` | Private key |
| `*.p12` | PKCS#12 certificate |
| `*.pfx` | PKCS#12 certificate |

All seeded with `prevent_read=true` (and `prevent_write=true` implicitly).

### Key Files

- **cli/internal/store/schema.go** — add `prevent_write`, `prevent_read` to `zones` CREATE + ALTER TABLE migration
- **cli/internal/store/policy.go** — add fields to `Zone` struct; update `AddZone` signature; update scan in `ListZones`
- **cli/internal/hook/hook.go** — add `ReadChecker` type; add `readingTools` map; extend `Evaluate` to check reads; add `bashReadTargets` and check against `ReadChecker` in `evaluateBash`
- **cli/cmd/hook.go** — add `buildReadChecker()`; pass to `Evaluate`
- **cli/cmd/zone/add.go** — add `--prevent-read` flag
- **cli/cmd/zone/list.go** — display `prevent_read` column
- **cli/internal/store/rules.go** (or policy.go) — add `StandardGuardrailZones`
- **cli/cmd/init.go** — `promptAndAddGuardrails` also seeds guardrail zones
- **docs/requirements.md** — add ZON-07; update SAF-02

### Requirement IDs

- **ZON-07** (new) — Zones support `--prevent-read` to block agent read access to sensitive files
- **SAF-02** (partial) — Standard guardrail zones for credential files seeded on `cordon init`

---

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

**Command Rule Type / Authority Refactor (CMD-07)**
- **cli/internal/store/schema.go** — added `rule_access` and `rule_authority` columns to `command_rules` with migration + backfill from legacy `rule_type` (custom→standard, builtin→guardian)
- **cli/internal/store/rules.go** — `CommandRule` struct split: `RuleType` (allow/deny), `RuleAuthority` (standard/guardian); `AddRule` takes both; `MatchCommandRule` implements allow-supersedes-deny logic
- **cli/internal/hook/commandrule.go** — `MatchedRule` struct gains `RuleAuthority`; `BuiltinRulesAsStore` returns deny+guardian; `CheckBuiltinRules` returns deny+guardian matched rules
- **cli/cmd/command/add.go** — added `--allow` flag; updated display labels
- **cli/cmd/command/list.go** — table format: TYPE, PATTERN, REASON, CREATED BY, CREATED AT
- **cli/cmd/hook.go** — `buildCommandChecker` propagates `RuleAuthority` field
- **cli/cmd/init.go** — guardrail rules use `"deny"`, `"standard"` parameters

**Zone Type / Zone Authority Refactor (ZON-08)**
- **cli/internal/store/schema.go** — added `zone_access` and `zone_authority` columns with migration + backfill from legacy `zone_type`
- **cli/internal/store/policy.go** — `Zone` struct split: `ZoneType` (allow/deny), `ZoneAuthority` (standard/guardian); `AddZone` takes both; `ZoneForPath` implements allow-supersedes-deny logic
- **cli/cmd/zone/add.go** — added `--allow` flag; validation for `--allow` + `--prevent-read`; updated display labels
- **cli/cmd/init.go** — guardrail zones use `"deny"`, `"standard"` parameters
- **cli/internal/codexpolicy/codexpolicy.go** — allow zones omitted from deny list; guardian label uses `ZoneAuthority`

## Previously Completed

- **Command Rules** (CMD-01–CMD-06): `command_rules` table, `CommandChecker`, built-in rules, `cordon rule` CLI

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
