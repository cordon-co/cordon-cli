<p align="center">
  <a href="https://cordon.sh">
    <img src="docs/assets/Banner.png" alt="Cordon" />
  </a>
</p>

<h3 align="center">
  <a href="https://cordon.sh">cordon.sh</a>
</h3>

<p align="center">
  Team-wide access policies and visibility for AI coding agents.
</p>

<p align="center">
  <a href="https://cordon.sh"><img src="https://img.shields.io/badge/website-cordon.sh-blue" alt="Website" /></a>
  <a href="https://github.com/cordon-co/cordon-cli/releases/latest"><img src="https://img.shields.io/github/v/release/cordon-co/cordon-cli" alt="Latest Release" /></a>
  <a href="https://goreportcard.com/report/github.com/cordon-co/cordon-cli"><img src="https://goreportcard.com/badge/github.com/cordon-co/cordon-cli" alt="Go Report Card" /></a>
  <a href="https://github.com/cordon-co/cordon-cli/actions/workflows/test.yml"><img src="https://github.com/cordon-co/cordon-cli/actions/workflows/test.yml/badge.svg" alt="Tests" /></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-BSL--1.1-orange" alt="License" /></a>
  <a href="https://github.com/cordon-co/cordon-cli/actions/workflows/codeql.yml"><img src="https://github.com/cordon-co/cordon-cli/actions/workflows/codeql.yml/badge.svg" alt="CodeQL" /></a>
  <a href="https://github.com/cordon-co/cordon-cli"><img src="https://img.shields.io/github/languages/top/cordon-co/cordon-cli" alt="Top Language" /></a>
  <a href="https://github.com/cordon-co/cordon-cli/releases"><img src="https://img.shields.io/github/downloads/cordon-co/cordon-cli/total" alt="Downloads" /></a>
  <a href="https://github.com/cordon-co/cordon-cli/blob/main/go.mod"><img src="https://img.shields.io/github/go-mod/go-version/cordon-co/cordon-cli" alt="Go Version" /></a>
</p>

---

## Installation

**Quick install:**

```sh
curl -fsSL cordon.sh/install.sh | sh
```

**From GitHub directly:**

```sh
curl -fsSL https://raw.githubusercontent.com/cordon-co/cordon-cli/main/scripts/install.sh | sh
```

**With Go:**

```sh
go install github.com/cordon-co/cordon-cli@latest
```

## Quick Start

**1. Initialise Cordon in your repository:**

```sh
cd your-repo
cordon init
```

The interactive setup will detect installed agents and let you select which ones to enforce policies on.

**2. Commit or ignore the config:**

- To share policies with your team, commit the `.cordon/` directory and any agent config changes (e.g. `.claude/settings.local.json`, `.codex/`).
- For personal use only, add `.cordon/` to your `.gitignore`.

## Supported Agents

| Agent | Support | Mechanism | Hook Based Enforcement | MCP Elicitation Support |
|-------|---------|-----------|---------|-----------|
| Claude Code | First class | Yes ✓ | Yes ✓ |
| Cursor | First class | Yes ✓ | Yes ✓ |
| VS Code Chat (Copilot) | First class | Yes ✓ | Yes ✓ |
| Gemini CLI | Effective | Yes ✓ | No ⤫ |
| OpenCode | Effective | Yes ✓ | No ⤫ |
| Codex | Limited | No ⤫ | No ⤫ |

## Commands

```
cordon init                          Initialise Cordon in the current repository
cordon uninstall                     Uninstall Cordon from the current repository
cordon version

cordon log [--since] [--date] [--agent] [--file] [--allow] [--deny] [--granted] [--pass] [--follow] [--export csv]

cordon file add [--allow] [--prevent-read] <pattern>
cordon file list
cordon file remove <pattern>

cordon command add [--allow] <pattern>
cordon command list
cordon command remove <pattern>

cordon pass issue --file <path> [--duration 60m|24h|7d|1w|indefinite]
cordon pass list [--all]
cordon pass revoke <pass-id>
```

All commands accept `--json` for structured output consumed by the IDE extension.

## Build

Requires Go 1.22+.

```sh
# current platform
make build

# all release targets (darwin/linux/windows, arm64/amd64)
make build-all VERSION=1.0.0
```

Binaries are written to `build/`.

## Dev Install

```sh
./scripts/dev-install.sh
# installs to ~/.local/bin/cordon by default
# override with INSTALL_DIR=/usr/local/bin ./scripts/dev-install.sh
```

## Test

```sh
./scripts/test.sh
```

Runs both store-level unit tests and CLI integration tests.

## License

[Business Source License 1.1](LICENSE) — see the LICENSE file for details.
