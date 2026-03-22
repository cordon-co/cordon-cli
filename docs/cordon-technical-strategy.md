# Cordon — Technical Strategy

## What It Does

Cordon provides file-level access policies for AI coding agents (Claude Code, Codex, VS Code Copilot) enforced via each platform's native hook mechanisms, with team-level policy distribution and audit logging.

## Core Concepts

- **Perimeter**: policy boundary for a repository
- **File Rule**: a protected file, folder, or glob pattern. Standard (any member) or guardian (elevated users only)
- **Pass**: temporary access grant to write to a protected file, with configurable duration
- **Demarcation**: agent-registered intent declaration visible to the team via IDE CodeLens and panel

## User Roles

- **Member**: default. Can manage standard file rules and issue standard passes
- **Guardian**: elevated per-repo or org-wide. Can create guardian file rules that members cannot override, issue passes for guardian rules, receive and approve pass requests
- **Admin**: full control including billing, team management, guardian assignment

## Enforcement

| Agent | Mechanism | Level | Runtime Updates |
|-------|-----------|-------|-----------------|
| Claude Code | PreToolUse hook via `cordon hook` binary | Hard block | Immediate |
| VS Code agents (Copilot) | PreToolUse hook via `cordon hook` binary (shared config) | Hard block | Immediate |
| Codex | `model_instructions_file` referencing `.cordon/codex-policy.md` | Soft (model-compliant) | Policy file changes immediate, config.toml requires session restart |
| Any MCP agent | Cordon MCP server | Soft (best-effort) | Immediate |

Hook integration is additive. Cordon appends its matcher group to the existing PreToolUse array in `.claude/settings.local.json` without touching other hooks. The matcher is `Write|Edit|MultiEdit`. Existing user permissions for other tools (Bash, git, npm) are unaffected.

Codex `writable_roots` is an allow-list and cannot be used for deny patterns. Codex also lacks pre-execution hooks. Enforcement relies on model instructions and the `agent-turn-complete` notify hook for violation detection.

VS Code currently ignores hook matchers (hooks fire on all tool invocations regardless). Cordon's hook handles this by checking the tool name internally.

All enforcement mechanisms tested and confirmed working across Claude Code, VS Code Copilot, and Codex.

## Architecture

### Cordon CLI (Go binary)

Single binary serving as CLI, hook engine, and MCP server.

- `cordon init` — sets up `.cordon/` directory, hook entries, Codex config, MCP entry, `.gitignore` additions. Detects installed agent platforms. Non-destructive with existing configs
- `cordon hook` — invoked as PreToolUse hook. Reads JSON from stdin, checks file path against policy database, returns exit 0 (allow) or exit 2 with JSON deny response
- `cordon --mcp` — runs as stdio MCP server. Tools: `cordon_request_access`, `cordon_register_demarcation`
- `cordon login` / `cordon logout` — GitHub OAuth browser flow, token stored in `~/.cordon/credentials.json`
- `cordon file add|remove|list` — file rule management
- `cordon pass issue|revoke|list` — pass management
- `cordon log` — audit log with filtering and export
- `cordon sync` — manual policy sync with cloud
- `cordon status` — auth state, policy summary, integrity check
- `cordon check` — integrity verification of hooks, config, MCP, policy database
- `cordon uninstall` — clean uninstall, removes only Cordon entries from all configs
- All commands support `--json` for structured output

### IDE Extension (VS Code, TypeScript)

Thin UI layer over CLI subprocess calls. Never calls the API directly.

- File rule management panel (add/remove/list via `cordon file` commands)
- Pass management panel (issue/revoke/list via `cordon pass` commands)
- Demarcations panel showing team-wide active agent work
- CodeLens provider for inline demarcation indicators
- Elicitation prompts for agent pass requests
- Auth flow delegates to `cordon login`
- Triggers sync and integrity check on workspace open and periodically
- Auto-detects connected repos and runs init if needed
- CLI detection on activation, prompts install if missing
- Connection status indicator in status bar

### Hosted Service (proprietary, api.cordon.sh)

- Policy distribution and sync across team members
- Team and org management via GitHub org integration
- Guardian notification routing (web, email, Slack, WhatsApp)
- Analytics backend: ingests batched telemetry, aggregates for dashboard
- Audit trail storage and compliance export
- MCP server at mcp.cordon.sh for cloud-connected agents
- REST API consumed by web interface, CLI sync, and extension (via CLI)

### Web Interface (proprietary, app.cordon.sh)

- Team management with GitHub org integration
- Repository management with file browser for file rule creation
- Insights dashboard (org-level and repo-level)
- Pass activity and audit trail views
- Admin settings including billing
- Documented separately

## Data Storage

### In the repository (`.cordon/`)

- `policy.db` — SQLite, file rule definitions only. Present for unauthenticated users. Small, changes infrequently. User decides whether to commit
- `config.json` — basic config (Cordon version, perimeter ID for cloud users)
- `codex-policy.md` — managed instructions file for Codex enforcement

### Per-user (`~/.cordon/`)

- `credentials.json` — auth token from GitHub OAuth
- `config.json` — global user preferences
- `repos/<repo-hash>/policy-cache.db` — cached cloud policy for authenticated users
- `repos/<repo-hash>/data.db` — audit logs, pass state, demarcation history, hook invocation logs. Never committed

### Policy resolution

Hook checks auth state first. If authenticated, reads from `~/.cordon/repos/<repo-hash>/policy-cache.db`. If not, reads from `.cordon/policy.db` in the repo. Offline authenticated users fall back to their last-synced cache. Fails open with logging if policy database is unreadable.

## Permission Elicitation Flow

1. Agent attempts to write to a protected file
2. `cordon hook` denies with message instructing agent to call `cordon_request_access` MCP tool
3. Agent calls MCP tool, which triggers elicitation to the human (IDE prompt or terminal)
4. Human approves or denies
5. If approved, pass is created with configured duration, policy cache updated, agent retries
6. For guardian rules, request routed to guardians via notification channel with approve/deny action

## Curated Safety Hooks

Bundled hook rules enforced through the same `cordon hook` binary alongside file rule enforcement. Configurable per-repo.

- Block destructive commands: `git reset --hard`, `git push --force`, `rm -rf`
- Block writes to files containing detected credential patterns
- Block modifications to CI/CD config (`.github/workflows/`, Terraform, Kubernetes manifests)
- Auto-format on write (configurable: Prettier, Ruff, gofmt)

## Audit & Analytics

Every hook invocation logged: tool name, file path, user, agent, timestamp, permit/deny. All file rule changes, pass events, and integrity checks logged. Analytics module configurable per-org (on by default).

Capabilities: activity heatmaps, most accessed/denied/pass-issued files, user activity breakdown, repeated denial detection, pass activity trends, compliance export.

## Install & Distribution

- `curl cordon.sh/install.sh | sh` — downloads platform binary from GitHub releases, places on PATH
- `irm cordon.sh/install.ps1 | iex` — Windows (WSL required for hooks)
- GitHub Actions builds Go binary for macOS arm64/x64, Linux x64/arm64, Windows x64 on tagged release
- Extension packaged and released alongside CLI, shared version number
- Repo: `github.com/cordon-co/cordon` (CLI + extension, open source when ready)
- Web service: separate private repo

## Open Source Boundary

**Open source**: CLI (Go), IDE extension (TypeScript). Fully functional in local-only mode with `.cordon/policy.db`.

**Proprietary**: hosted service (api.cordon.sh, app.cordon.sh, mcp.cordon.sh). Team sync, web dashboard, analytics backend, notification routing, billing.

The CLI switches from local policy file to cloud sync when the user authenticates. The extension works identically in both modes since it only talks to the CLI.