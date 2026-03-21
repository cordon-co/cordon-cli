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
cordon file add [--guardian] <path>
cordon file list
cordon file remove <path>
cordon pass issue [--file] [--duration]
cordon pass list
cordon pass revoke <pass-id>
cordon version
cordon --mcp                         Run as a stdio MCP server
```

All commands accept `--json` for structured output consumed by the IDE extension.

## Status

The command tree and flag surface are in place and the binary compiles. Business logic is not yet implemented — all commands print `not implemented` and exit cleanly. `--json` output returns valid but empty/stub JSON payloads. Flags documented in `--help` (e.g. `--file`, `--duration`, `--since`, `--export`) are wired to variables but have no effect yet.

## Build

Requires Go 1.22+.

```sh
# current platform
./scripts/build.sh

# all release targets (darwin/linux/windows, arm64/amd64)
./scripts/build.sh all

# or via make
make build
make build-all VERSION=1.0.0
```

Binaries are written to `build/`.

## Dev install

```sh
./scripts/dev-install.sh
# installs to ~/.local/bin/cordon by default
# override with INSTALL_DIR=/usr/local/bin ./scripts/dev-install.sh
```

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
  pass/            pass issue|list|revoke
internal/
  flags/           shared flag state (avoids circular imports between cmd packages)
scripts/
  build.sh
  dev-install.sh
```
