# cordon CLI

Go binary that serves as the CLI, hook enforcement engine, and MCP server for [Cordon](https://cordon.sh).

## Commands

```
cordon init                          Initialise Cordon in the current repository
cordon remove                        Uninstall Cordon from the current repository
cordon status                        Show auth state, policy summary, and integrity check
cordon login                         Authenticate via GitHub OAuth
cordon logout                        Clear stored credentials
cordon sync                          Sync policy and audit data with Cordon Cloud
cordon hook                          Evaluate a PreToolUse hook payload (reads JSON from stdin)
cordon log [--file] [--denied-only] [--since] [--export csv]
cordon file add [--guardian] [--allow] [--prevent-read] <pattern>
cordon file list
cordon file remove <pattern>
cordon command add [--allow] <pattern>
cordon command list
cordon command remove <pattern>
cordon pass issue --file <path> [--duration 60m|24h|7d|1w|indefinite]
cordon pass list [--all]
cordon pass revoke <pass-id>
cordon version
cordon --mcp                         Run as a stdio MCP server
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

## Dev install

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

## Release

Tagged pushes (`v*`) trigger the GitHub Actions release workflow, which cross-compiles all targets and attaches the binaries to the GitHub release.

Version is injected at build time:

```sh
make build VERSION=1.2.3
# or
go build -ldflags "-X github.com/cordon-co/cordon/cmd.Version=1.2.3" -o build/cordon .
```

## Project layout

```
main.go
cmd/
  root.go          root command, --json and --mcp flags
  init.go
  remove.go
  login.go / logout.go
  status.go / sync.go
  hook.go          invoked by agent PreToolUse hook config (hidden from help)
  log.go
  version.go
  file/            file add|list|remove
  command/         command add|list|remove
  pass/            pass issue|list|revoke
internal/
  store/           SQLite database layer (policy.db and data.db)
  hook/            hook evaluation logic
  reporoot/        repository root detection
  claudecfg/       .claude/settings.local.json management
  codexpolicy/     Codex policy file generation
  flags/           shared flag state
scripts/
  build.sh
  dev-install.sh
  test.sh
tests/
  cli integration tests
```
