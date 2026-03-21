# cordon — CLI source

Go module for the cordon binary. See the [root README](../README.md) for product documentation.

## Requirements

Go 1.22+. No CGo required — SQLite is bundled via `modernc.org/sqlite`.

## Build

```sh
# from repo root
./scripts/build.sh              # current platform → cli/build/cordon
./scripts/build.sh all          # all release targets

# or directly
cd cli && make build
```

## Test

```sh
# from repo root
./scripts/test.sh

# or directly
cd cli && go test ./... -count=1 -v
```

## Package layout

```
main.go
cmd/
  root.go          root command, --json and --mcp flags
  init.go
  hook.go          PreToolUse hook enforcement (hidden from help)
  log.go
  file/            file add|list|remove
  command/         command add|list|remove
  pass/            pass issue|list|revoke
internal/
  store/           SQLite layer — policy.db (repo) and data.db (user)
  hook/            hook evaluation logic
  reporoot/        walks up to find .cordon/
  claudecfg/       .claude/settings.local.json management
  codexpolicy/     .cordon/codex-policy.md generation
  flags/           shared flag state (avoids circular imports)
tests/
  CLI integration tests — build binary, exercise via subprocess
```
