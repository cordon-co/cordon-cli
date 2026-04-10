# Cordon CLI Reference

This reference summarizes the `cordon` command tree, common workflows, and flags based on the current CLI implementation.

## Global usage

```bash
cordon [command] [flags]
```

Global flags:

- `--json` — output structured JSON (supported across user-facing commands)
- `--mcp` — run Cordon as a stdio MCP server (root command mode)

---

## Top-level commands

### `cordon init`
Initializes Cordon in the current repository.

What it does:
- Creates `.cordon/policy.db`
- Ensures user-level data DB in `~/.cordon/repos/<perimeter-id>/data.db`
- Configures selected agent hooks/MCP integration
- Optionally adds standard guardrails during first setup

Flags:
- `-y, --yes` — non-interactive defaults
- `--agent` — reconfigure managed agent integrations for an existing installation

Examples:

```bash
cordon init
cordon init --yes
cordon init --agent
```

### `cordon status`
Shows repository, policy, auth, perimeter, and agent integration status.

Examples:

```bash
cordon status
cordon status --json
```

### `cordon log`
Displays audit log entries with filter/export/stream options.

Flags:
- `--agent <id>`
- `--allow`
- `--deny`
- `--granted`
- `--pass`
- `--file <substring>`
- `--since <duration>` (for example `24h`, `7d`)
- `--until <rfc3339-or-date>`
- `--date <YYYY-MM-DD>`
- `-f, --follow`
- `-i, --interactive`
- `--limit <n>`
- `--export <csv-path>`

Notes:
- Some flags are mutually exclusive (`--date` vs `--since/--until`, `--follow` vs `--export`, etc.).
- `--interactive` cannot be combined with `--json`.

Examples:

```bash
cordon log --deny --since 24h
cordon log --agent codex --file secrets --limit 100
cordon log --export ./audit.csv
```

### `cordon sync`
Syncs policy and audit data with Cordon Cloud.

Flags:
- `--background` — run detached sync with lockfile coordination

Examples:

```bash
cordon sync
cordon sync --background
```

### `cordon uninstall`
Removes Cordon from the current repository.

What it removes:
- Cordon-managed hook and MCP config entries
- Local `.cordon/` directory

What it does not remove:
- User-level data under `~/.cordon/`

Example:

```bash
cordon uninstall
```

### `cordon version`
Prints the CLI version.

Example:

```bash
cordon version
```

---

## Rule management

### `cordon file`
Manage protected file rules.

Subcommands:
- `cordon file add <pattern> [--allow] [--prevent-read]`
- `cordon file list`
- `cordon file remove <pattern>`

Examples:

```bash
cordon file add "**/*.env" --prevent-read
cordon file add "docs/**" --allow
cordon file list
cordon file remove "**/*.env"
```

### `cordon command`
Manage protected command rules.

Subcommands:
- `cordon command add <pattern> [--allow]`
- `cordon command list`
- `cordon command remove <pattern>`

Examples:

```bash
cordon command add "git push --force*"
cordon command add "npm test*" --allow
cordon command list
cordon command remove "git push --force*"
```

---

## Pass management

### `cordon pass issue`
Issues temporary access to a protected target.

Usage:

```bash
cordon pass issue <target> [--duration 60m|24h|7d|1w|indefinite]
```

Flags:
- `--duration` (default `60m`)
- `--target` (alternative to positional target)

### `cordon pass list`
Lists active passes.

Flags:
- `--all` — include expired and revoked

### `cordon pass revoke`
Revokes an active pass.

Usage:

```bash
cordon pass revoke <pass-id>
```

Examples:

```bash
cordon pass issue "src/secrets.go" --duration 24h
cordon pass list
cordon pass list --all
cordon pass revoke 31d15ecd-19e3-41be-a20f-b74fe167013a
```

---

## Authentication

### `cordon auth login`
Authenticate via device OAuth or machine token.

Flags:
- `--token <machine-token>`

### `cordon auth status`
Check current auth status and token validity.

### `cordon auth logout`
Clear local credentials and attempt server-side revocation.

Examples:

```bash
cordon auth login
cordon auth login --token "$CORDON_TOKEN"
cordon auth status
cordon auth logout
```

---

## Hidden/internal commands

The CLI also includes hidden/internal commands used by integrations and background tasks:

- `cordon hook` — hook entrypoint for pre-tool enforcement
- `cordon sessions extract` — transcript/session extraction utility

These are intentionally hidden from normal command help output.
