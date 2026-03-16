# Cordon — Current Task

## Summary

Initial Go CLI scaffolding. Creating the `cli/` Go project with the full command structure, cross-platform build support, and a GitHub Actions release workflow. Commands are stubbed (not yet implemented) — the goal is a compilable binary with the correct command tree and `--json` flag wiring throughout.

## Relevant Requirements

- **CLI-01** — Go project scaffolding with cross-platform build targets (macOS arm64/x64, Linux x64/arm64, Windows x64)
- **INS-02** — Install script supports macOS (arm64, x64), Linux (x64, arm64)
- **INS-04** — `cordon remove` cleanly uninstalls Cordon config from a repo (stub structure in place)
- **INS-05** — Uninstall leaves all non-Cordon hooks intact (implied by INS-04 implementation)

## Key Files

- `cli/main.go` — entry point
- `cli/cmd/root.go` — root command, `--mcp` flag, `--json` flag
- `cli/cmd/` — all top-level command stubs (init, remove, login, logout, status, sync, hook, log)
- `cli/cmd/zone/` — zone subcommand tree (add, list, remove)
- `cli/cmd/pass/` — pass subcommand tree (issue, list, revoke)
- `cli/go.mod` — module definition with cobra dependency
- `cli/Makefile` — local build targets per platform
- `.github/workflows/release.yml` — cross-compile and attach binaries on tagged release

## Notes

- `cordon --mcp` is a flag on the root command, not a subcommand
- All commands must accept `--json` for structured output (wired up in root, inherited)
- `cordon remove` must surgically remove only Cordon entries from `.claude/settings.local.json` and `.codex/config.toml`, leaving everything else intact
- Build system is intentionally minimal — no custom build tooling beyond `go build` and a thin Makefile
