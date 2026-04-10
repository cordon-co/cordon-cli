# Project Overview and Concepts

This document gives a practical overview of the Cordon CLI project and its core concepts.

## What Cordon is

Cordon is a policy enforcement layer for AI coding agents. It lets teams define repository-level protections and enforce them through native agent hooks before risky operations execute.

In this repository:
- `cli/` is the core product (policy logic, storage, hook enforcement, MCP server)

## Core goals

- **Prevent unsafe file and command operations by default**
- **Allow controlled, temporary exceptions through passes**
- **Provide audit visibility into allow/deny decisions**
- **Work across multiple agent ecosystems with minimal user friction**

## Primary concepts

See the project README for canonical concept definitions and terminology.

## Policy enforcement model (high level)

Cordon enforces two related but different policy types:

- **File policy enforcement**
  - Protects files, folders, and globs in the repository.
  - Evaluates whether a tool operation would read from or write to protected paths.

- **Command policy enforcement**
  - Protects shell command intent (for example destructive operations or sensitive commands).
  - Evaluates each shell command segment against command rules before execution.

For command rules, matching is intentionally layered:

- **`string` matching**
  - Backward-compatible command text matching (exact/glob/prefix style).
  - Useful for straightforward command patterns.

- **`argv` matching**
  - Token-aware command matching that looks at command words/flags.
  - Helps catch equivalent commands where option order differs (for example force flags appearing later in the command).
  - Common shell wrappers (`sh -c`, `bash -lc`, etc.) are unwrapped so equivalent inner commands are evaluated consistently.

In practice, this means file and command enforcement share the same policy system but use different matching models suited to their domain.

File pattern support includes recursive glob usage (`**`) for nested path matching.

## High-level runtime architecture

1. User runs a command (`init`, `file add`, `pass issue`, etc.)
2. CLI resolves repo root and opens/migrates SQLite databases
3. Policy mutation or query executes through internal store APIs
4. Agent hooks or MCP server consume the same policy model at runtime
5. Audit events are recorded to user-level data DB for log inspection and sync

## Command groups and responsibilities

- `init`, `uninstall`, `status`, `version`: lifecycle and diagnostics
- `file`, `command`: policy authoring
- `pass`: temporary overrides
- `log`: audit visibility/export
- `sync`, `auth`: cloud identity and policy/audit synchronization
- hidden/internal commands (`hook`, `sessions extract`): integration/runtime internals

## Design principles visible in this repo

- **CLI-first architecture:** business logic lives in Go CLI, not extension clients
- **Native integrations:** each agent platform is integrated through its own config/hook mechanism
- **Safety defaults:** explicit policy and pass lifecycle, with auditable decisions
- **Scriptable interface:** `--json` support for machine-readable CLI consumption
- **Incremental compatibility:** hooks are appended without destroying unrelated user config

## Suggested reading order for contributors

1. Root `README.md` for product-level behavior and command map
2. `cli/README.md` for package layout
3. `cli/cmd/` for command entrypoints and flag contracts
4. `cli/internal/store/` for schema and policy semantics
5. `cli/internal/hook/` and `cli/internal/agents/` for enforcement/integration behavior
