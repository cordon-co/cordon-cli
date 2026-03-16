# Cordon Project Instructions

## Project Overview

Cordon (cordon.sh) is a developer tool that provides team-wide access policies and visibility for AI coding agents. It enforces file-level write restrictions across Claude Code, Codex, and VS Code Copilot using each tool's native hook mechanisms, with team-level policy distribution and audit logging.

## Repository Structure

This repo contains two packages:
- `cli/` — Go binary that serves as the CLI, hook enforcement engine, and MCP server
- `vs-code-extension/` — VS Code extension (TypeScript) that provides the IDE interface

The CLI is the core of the product. The extension is a thin UI layer that calls CLI commands with `--json` output.

## Core Concepts

- **Perimeter**: the top-level policy boundary for a repository
- **Zone**: a file, folder, or glob pattern protected by an access policy. Standard zones (any member) or guardian zones (guardian/admin only)
- **Pass**: a temporary access grant allowing an agent to write to a zoned file. Configured with a duration
- **Demarcation**: a registered declaration of what an agent is currently working on, visible to the team via CodeLens and the demarcations panel

## Key Architecture Decisions

- The CLI binary handles all business logic. The extension never calls the API directly — it calls CLI subcommands
- `cordon hook` is invoked as a PreToolUse hook by Claude Code and VS Code agents. It reads JSON from stdin, checks policy, returns allow/deny
- `cordon --mcp` runs as a stdio MCP server providing zone checks, pass requests, and demarcation registration
- Policy is stored in SQLite: `.cordon/policy.db` in the repo for unauthenticated users, `~/.cordon/repos/<repo-hash>/policy-cache.db` for authenticated users synced from the cloud
- Operational data (audit logs, pass state, demarcation history) is stored in `~/.cordon/repos/<repo-hash>/data.db` and never committed to the repo
- User credentials and global preferences are stored in `~/.cordon/`
- Hook integration is additive: Cordon appends its entries to existing hook configs without modifying other hooks
- Codex enforcement uses a managed `model_instructions_file` at `.cordon/codex-policy.md`

## Enforcement Matrix

| Agent | Mechanism | Enforcement Level |
|-------|-----------|-------------------|
| Claude Code | PreToolUse hook via `cordon hook` | Hard (pre-execution block) |
| VS Code agents (Copilot) | PreToolUse hook via `cordon hook` | Hard (pre-execution block) |
| Codex | model_instructions_file + notify hook | Soft (model-compliant) |
| Any MCP agent | Cordon MCP server | Soft (best-effort) |

## Task Management

Before starting work:
1. Read `docs/current_task.md` for context on any in-progress work
2. Read `docs/requirements.md` to understand the full scope when needed
3. Update `docs/current_task.md` with a summary of the work you are about to perform, the key files involved, and the relevant requirement IDs

After completing work:
1. Update the relevant requirement progress in `docs/requirements.md` (None → In Progress → Done)
2. Update `docs/current_task.md` to reflect completion and clear the task, or update it with the next steps if work is ongoing

## Code Conventions

- Go code in `cli/`: standard Go project layout, `go fmt`, no external dependencies unless necessary
- TypeScript code in `extension/`: standard VS Code extension patterns
- All CLI commands must support `--json` for structured output
- All user-facing output should be clean and minimal
- Error messages should be actionable
