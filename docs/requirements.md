# Cordon — Requirements

## Install & Uninstall

| # | Requirement | Progress |
|---|-------------|--------|
| INS-01 | `curl cordon.sh/install.sh \| sh` downloads correct platform binary and places on PATH | None |
| INS-02 | Install script supports macOS (arm64, x64), Linux (x64, arm64) | In Progress |
| INS-03 | PowerShell install script (`irm cordon.sh/install.ps1 \| iex`) for Windows (WSL documented as required for hooks) | None |
| INS-04 | `cordon remove` cleanly uninstalls all Cordon configuration from a repo: removes hook entries from settings.local.json, removes .codex/config.toml modifications, removes .cordon/ directory | In Progress |
| INS-05 | Uninstall leaves all non-Cordon hooks and config intact | In Progress |

## CLI Core

| # | Requirement | Progress |
|---|-------------|--------|
| CLI-01 | Go project scaffolding with cross-platform build targets (macOS arm64/x64, Linux x64/arm64, Windows x64) | In Progress |
| CLI-02 | `cordon init` creates `.cordon/` directory with `config.json` and `policy.db` in the current repo | In Progress |
| CLI-03 | `cordon login` authenticates via GitHub OAuth browser flow and stores token in `~/.cordon/credentials.json` | None |
| CLI-04 | `cordon logout` clears stored credentials | None |
| CLI-05 | `cordon status` displays auth state, repo policy summary, sync state, and integrity check result | None |
| CLI-06 | All commands support `--json` flag for structured output consumed by the IDE extension | None |
| CLI-07 | Platform detection on init: identify installed agent platforms (Claude Code, Codex, VS Code agents) and configure enforcement for each | None |

## Zone Management

| # | Requirement | Progress |
|---|-------------|--------|
| ZON-01 | `cordon zone add <file\|folder\|glob>` creates a deny zone (standard authority) in policy.db | Done |
| ZON-02 | `cordon zone add --guardian <file\|folder\|glob>` creates a zone with guardian authority (requires guardian/admin role when authenticated) | Done |
| ZON-03 | `cordon zone list` displays all active zones with type, creator, and scope | Done |
| ZON-04 | `cordon zone remove <file\|folder\|glob>` removes a zone (guardian zones require guardian/admin role) | Done |
| ZON-05 | Zones stored in `.cordon/policy.db` (SQLite) for unauthenticated users | Done |
| ZON-06 | Zones cached in `~/.cordon/repos/<repo-hash>/policy-cache.db` for authenticated users, synced from cloud | None |
| ZON-07 | `cordon zone add --prevent-read` also blocks agent read access (Read, Grep, Bash cat/head/tail/etc.) for credential and secret files | Done |
| ZON-08 | Zone types: deny (blocks access, default) and allow (permits access, overrides deny zones). Zone authority: standard (any member) or guardian (admin only). `cordon zone add --allow` creates an allow zone | Done |

## Command Rule Management

| # | Requirement | Progress |
|---|-------------|--------|
| CMD-01 | Built-in command rules compiled into the binary block agents from running `cordon *` commands directly | Done |
| CMD-02 | `cordon rule add <pattern> [--reason] [--severity block\|warn]` adds a custom command rule to policy.db | Done |
| CMD-03 | `cordon rule list` displays built-in and custom command rules with severity and reason | Done |
| CMD-04 | `cordon rule remove <pattern>` removes a custom command rule (built-ins cannot be removed) | Done |
| CMD-05 | Hook evaluates each segment of compound commands (`&&`, `\|\|`, `;`, `\|`) independently against rules | Done |
| CMD-06 | `cordon hook` is exempt from the built-in cordon block (it is the hook runner, not an agent command) | Done |

## Pass Management

| # | Requirement | Progress |
|---|-------------|--------|
| PAS-01 | `cordon pass issue --file <path> --duration <duration>` issues a temporary pass | Done |
| PAS-02 | Pass durations: short-term (e.g. 60m), medium-term (e.g. 1w), indefinite | Done |
| PAS-03 | `cordon pass list` displays active and recent passes with status, issuer, and expiry | Done |
| PAS-04 | `cordon pass revoke <pass-id>` revokes an active pass | Done |
| PAS-05 | Pass state stored in `~/.cordon/repos/<repo-hash>/data.db` | Done |
| PAS-06 | Guardian zone passes restricted to guardian/admin issuance when authenticated | None |

## Hook Enforcement

| # | Requirement | Progress |
|---|-------------|--------|
| HOK-01 | `cordon hook` subcommand reads JSON payload from stdin and checks file path against policy | Done |
| HOK-02 | Returns exit code 0 (allow) or exit code 2 with JSON deny response including guidance to request a pass | Done |
| HOK-03 | `cordon init` writes PreToolUse hook entry to `.claude/settings.local.json` with `Write|Edit|MultiEdit` matcher pointing to `cordon hook` | Done |
| HOK-04 | Hook integration is additive: appends to existing hooks array without modifying other entries | Done |
| HOK-05 | `cordon init` writes `.cordon/codex-policy.md` with deny list for Codex enforcement | Done |
| HOK-06 | `cordon init` writes `.codex/config.toml` with `model_instructions_file` reference to Codex policy file | None |
| HOK-07 | Codex policy file regenerated automatically when zones change | Done |
| HOK-08 | Hook queries local policy database (policy.db or policy-cache.db depending on auth state) | Done |

## MCP Server

| # | Requirement | Progress |
|---|-------------|--------|
| MCP-01 | `cordon --mcp` runs as a stdio an MCP server (MCP Go - https://github.com/mark3labs/mcp-go) for agent integration | In Progress |
| MCP-03 | MCP tool: `cordon_request_access` — agent requests a pass, triggers elicitation to the human | In Progress |
| MCP-04 | `cordon init` adds MCP server entry to `.claude/settings.local.json` with `cordon --mcp` command | In Progress |
| MCP-05 | MCP reads from the same local policy database as the hook | In Progress |

## Audit & Logging

| # | Requirement | Progress |
|---|-------------|--------|
| AUD-01 | Every hook invocation logged to `~/.cordon/repos/<repo-hash>/data.db`: tool name, file path, user, agent, timestamp, permit/deny | Done |
| AUD-02 | All zone changes logged: creation, modification, removal, by whom, timestamp | Done |
| AUD-03 | All pass events logged: issuance, approval, denial, expiry, revocation | Done |
| AUD-04 | `cordon log` displays audit log with filtering options (--file, --denied-only, --since) | Done |
| AUD-05 | `cordon log --export csv` exports audit data | Done |

## IDE Extension

| # | Requirement | Progress |
|---|-------------|--------|
| EXT-01 | VS Code extension scaffolding (TypeScript) | None |
| EXT-02 | CLI detection: check for `cordon` on PATH, prompt user to install if missing | None |
| EXT-03 | Auth: "Sign in" button triggers `cordon login` subprocess, reads auth state from `cordon status --json` | None |
| EXT-04 | Zone management panel: list zones, add/remove zones via `cordon zone` commands | None |
| EXT-05 | Pass management panel: list passes, issue/revoke via `cordon pass` commands | None |
| EXT-06 | Demarcations panel: display active agent work across the team | None |
| EXT-07 | CodeLens provider: inline demarcation indicators on files with active agent work | None |
| EXT-08 | Elicitation prompts: surface pass request notifications from agents | None |
| EXT-09 | Connection status indicator in status bar | None |
| EXT-10 | Trigger policy sync on workspace open and periodically | None |
| EXT-11 | Trigger integrity check on workspace open | None |
| EXT-12 | Repo setup: detect connected repo and run `cordon init` equivalent if not initialised | None |
| EXT-13 | All extension data sourced from CLI subprocess calls with `--json` output | None |

## Integrity

| # | Requirement | Progress |
|---|-------------|--------|
| INT-01 | `cordon check` verifies hook entries exist in settings.local.json | None |
| INT-02 | Verify Codex config.toml still references Cordon policy file | None |
| INT-03 | Verify MCP entry exists in agent config | None |
| INT-04 | Verify policy database exists and is readable | None |
| INT-05 | Auto-repair for simple failures (missing hook entry, missing MCP entry) | None |
| INT-06 | Log integrity check results to audit database | None |

## Curated Safety Hooks

| # | Requirement | Progress |
|---|-------------|--------|
| SAF-01 | Built-in hook rule: block destructive commands (git reset --hard, git push --force, rm -rf) | In Progress |
| SAF-02 | Standard guardrail zones for credential files (.env, credentials.json, *.pem, etc.) seeded on `cordon init` with read+write prevention | In Progress |
| SAF-03 | Built-in hook rule: block modifications to CI/CD config files (.github/workflows/) | None |
| SAF-04 | Safety hooks configurable per-repo (enable/disable individual rules) | None |
| SAF-05 | Safety hooks managed through the same `cordon hook` binary (no separate scripts) | None |
| SAF-06 | Safety hook state stored in policy database alongside zone data | None |

## CI/CD & Release

| # | Requirement | Progress |
|---|-------------|--------|
| REL-01 | GitHub Actions workflow: cross-compile Go binary for all targets on tagged release | None |
| REL-02 | GitHub Actions workflow: package VS Code extension on tagged release | None |
| REL-03 | Binaries attached to GitHub release for install script consumption | None |
| REL-04 | Versioning: CLI and extension share a single version number | None |

## Policy Sync (Cordon Cloud Integration)

| # | Requirement | Progress |
|---|-------------|--------|
| SYN-01 | `cordon sync` pulls policy from api.cordon.sh and writes to local policy cache | None |
| SYN-02 | `cordon sync` pushes local audit data and demarcation state to the cloud | None |
| SYN-03 | Sync is two-way: local changes push up, cloud changes pull down, cloud-wins on conflict | None |
| SYN-04 | Offline resilience: hook enforces last-known cached policy when cloud is unreachable, fails open with logging | None |
| SYN-05 | Telemetry batched and compressed before upload | None |

