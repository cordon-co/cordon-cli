# Technical Concepts & Design Decisions

## Why Hooks Over Other Enforcement Mechanisms

We evaluated multiple approaches to enforcing file-level write restrictions on AI agents.

**OS-level sandboxing (sandbox-exec, seccomp-bpf, Landlock):** Policies are loaded at process launch and are immutable at runtime. Changing a file rule or issuing a pass would require restarting the agent. More critically, IDE-embedded agents like VS Code Copilot run inside the VS Code process itself — sandboxing the agent means sandboxing the entire IDE.

**Separate Unix user:** Running agents under a restricted user (e.g. `cordon-agent`) and using filesystem permissions to enforce boundaries. Technically sound but requires elevated setup permissions and doesn't work for IDE-embedded agents that share the VS Code process.

**FUSE overlay:** Mounting a restricted filesystem view for agent processes. Same problem as sandboxing — cannot distinguish between IDE-embedded agent writes and human writes within the same process.

**Git hooks (pre-commit):** The agent can still write the file; the hook only blocks the commit. Since humans typically commit agent work (not the agent itself), the hook cannot distinguish human-authored from agent-authored changes.

**MCP-only enforcement:** Relies on the agent checking permissions before acting. This is the honour system — the agent must call the MCP, and must respect the response. No hard enforcement.

**Native agent hooks (chosen approach):** Claude Code and VS Code agents support PreToolUse hooks that intercept file write operations before execution. The hook runs external code, receives the tool input as JSON, and can deny the operation. This is real pre-execution enforcement that works for both CLI agents and IDE-embedded agents, supports runtime policy updates (the hook reads policy on each invocation), and integrates additively with existing hooks.

The tradeoff is that enforcement strength varies by platform. Claude Code and VS Code agents get hard enforcement. Codex gets soft enforcement via model instructions because it lacks pre-execution hooks. This is a platform limitation, not a design choice.

## Why a Single Go Binary

The `cordon` binary serves as CLI, hook engine, and MCP server. This was chosen over separate components because:

- The hook (`cordon hook`) must start and complete fast. A compiled Go binary has near-zero startup time. A Node.js or Python script has a cold-start penalty that would add latency to every tool call.
- No runtime dependencies. The binary runs on any machine without requiring Node, Python, or any toolchain. This matters for the install script (`curl | sh`) and for trust — users can inspect a single binary rather than a node_modules tree.
- The MCP server (`cordon --mcp`) runs as a long-lived stdio process. Go handles this naturally with goroutines for concurrent MCP requests.
- Cross-compilation is trivial. `GOOS=darwin GOARCH=arm64 go build` produces a macOS arm64 binary from any platform. GitHub Actions builds all targets in a single workflow.
- One binary means one version number, one release artifact per platform, and one thing to install.

## Why the Extension Delegates to the CLI

The IDE extension calls CLI subcommands with `--json` output rather than implementing business logic or calling the API directly. This decision has several consequences:

- The extension works identically for open source (local policy) and cloud (synced policy) users. The CLI handles the policy source internally.
- All business logic is testable in Go without VS Code. The extension is a rendering layer.
- Community extensions for other IDEs (JetBrains, Neovim) can be built as thin layers over the same CLI without reimplementing anything.
- Auth is handled once in the CLI. The extension triggers `cordon login` and reads state from `cordon status --json`.
- The JSON output schema of each CLI command is the contract between the CLI and the extension. This schema is treated as a stable API surface — changes are backwards-compatible.

The tradeoff is subprocess spawning latency on each interaction. For a Go binary this is milliseconds, which is acceptable for v1. If real-time streaming is needed later (e.g. live demarcation updates), a `cordon serve` long-running mode with local socket communication can be added.

## Why SQLite for Local Storage

Policy and audit data are stored in SQLite databases rather than flat files (JSON, TOML, YAML).

- The hook subprocess and the CLI can read from the same database concurrently without corruption. SQLite handles this with WAL mode.
- Audit logs can grow large. SQLite handles millions of rows efficiently. A JSON file would need to be fully parsed on every read.
- Querying (filter by file, by date, by denied-only) is SQL rather than application code.
- The `.cordon/policy.db` in the repo is a single file that can be committed if the user chooses. It's inspectable with any SQLite client.
- Go has mature SQLite bindings (modernc.org/sqlite is a pure Go implementation, no CGO required, which simplifies cross-compilation).

## Why Two Databases

Policy definitions and operational data are separated into different databases in different locations.

**`.cordon/policy.db` (repo-level, unauthenticated users):** Contains only file rule definitions. Changes infrequently (only when rules are added or removed). Small enough to optionally commit to the repo. Other developers who clone the repo get the policy.

**`~/.cordon/repos/<repo-hash>/data.db` (user-level):** Contains audit logs, pass state, demarcation history, hook invocation logs. Changes on every hook invocation. Never committed to the repo. For authenticated users, synced to the cloud.

For authenticated users, policy comes from the cloud and is cached in `~/.cordon/repos/<repo-hash>/policy-cache.db`. The repo-level `policy.db` is not present or is ignored.

This separation means the repo stays clean (no constantly-changing database files in source control) while policy can still travel with the repo for open source users.

## Why Hooks Are Additive

Cordon never modifies or replaces existing hook configurations. It appends a new matcher group to the PreToolUse array in `.claude/settings.local.json`.

Claude Code supports multiple matcher groups in the same hook event array, and multiple hooks within a matcher group run in parallel. If a customer has an existing Prettier hook on `Write|Edit` and Cordon adds its own hook on `Write|Edit|MultiEdit`, both run independently. If Cordon denies, the write is blocked regardless of the other hook's result. If Cordon allows, the other hook still runs.

Cordon writes to `settings.local.json` (the local override file, not committed) rather than `settings.json` (the project file, committed). This means Cordon's hook wiring never appears in the team's committed config — it's managed locally by the Cordon CLI and extension.

Uninstall (`cordon uninstall`) parses the settings file and removes only the Cordon entries, leaving everything else intact.

## Why Fail-Open

When the policy database is unreadable, the cloud is unreachable, or the integrity check fails, Cordon allows the operation and logs the failure rather than blocking it.

Cordon is a trust-based system. Users can modify any files they choose and commit any work they choose. Agent changes are an extension of that. Hard blocking on infrastructure failure would create frustration and erode trust in the tool. Silent failure would be worse.

Fail-open with logging means: the developer is not blocked, the guardian sees the enforcement gap in the audit trail, and the integrity check surfaces the issue for repair. This matches the product's philosophy — Cordon provides control and visibility, not lockdown.

## Why Codex Uses Model Instructions

Codex lacks pre-execution hooks. Its only hook is `agent-turn-complete` (notify, post-execution). Its `writable_roots` config is an allow-list, not a deny-list, and cannot be practically inverted for deny-pattern enforcement. Config changes also do not hot-reload during a running session.

The `model_instructions_file` approach works because:

- Codex reliably follows explicit instructions in its model instructions file. Tested and confirmed: Codex refuses to write to denied files and tells the user to change the policy.
- The instructions file is a plain markdown file at `.cordon/codex-policy.md` that Cordon regenerates whenever file rules change. The file content is read by Codex on each turn, so deny list changes take effect without session restart (even though the config.toml reference itself requires restart).
- The `agent-turn-complete` notify hook allows Cordon to check for violations after each turn and alert the user if the model ignored the instructions.

This is soft enforcement. The model can theoretically ignore the instructions. This is documented honestly in the enforcement matrix and competitive positioning.

## Why the MCP Server Is Stdio

The Cordon MCP server runs as `cordon --mcp` via stdio rather than as a hosted HTTP MCP endpoint (for the open source / local mode).

- Stdio MCP servers are spawned by the agent client (Claude Code, etc.) as a subprocess. No port management, no daemon, no process to keep alive.
- The MCP process has direct access to the local policy database. No network call required to check a file rule or register a demarcation.
- The agent's MCP config is the same regardless of whether the user is authenticated or not: `{"command": "cordon", "args": ["--mcp"]}`. The binary handles the policy source internally.
- For cloud-connected teams, the hosted MCP at mcp.cordon.sh exists as an alternative, but the local stdio MCP is the default.

## Why Guardian Rules Are Trust-Based

A developer can technically bypass guardian rule enforcement by editing the Codex instructions file, deleting the hook entry, or modifying the policy database. Cordon does not attempt to make this impossible.

This mirrors every other access control system in software development. Developers can bypass branch protection, skip PR reviews, ignore CODEOWNERS. The purpose of these systems is to establish norms, make violations visible, and create accountability — not to be tamper-proof against malicious insiders.

Cordon provides hard enforcement for Claude Code and VS Code agents (hooks block writes at the tool level — circumvention requires actively deleting or modifying config). Codex enforcement is model-compliant. In all cases, the audit trail captures when enforcement stops working for a user (their agent writing to protected files without passes), making circumvention visible to guardians.

## Why the Hook Checks the Tool Name Internally

The hook matcher in the config is set to `Write|Edit|MultiEdit`, but the `cordon hook` binary also checks the tool name from the JSON payload internally. This is because:

- VS Code currently ignores matchers and fires hooks on all tool invocations regardless. Without the internal check, Cordon would process read operations and other non-write tools unnecessarily.
- Future agent platforms may have different matcher semantics. The internal check ensures correct behaviour regardless of how the host platform handles matchers.
- The internal check adds negligible overhead (a string comparison before the policy database lookup).

## Why the Install Is a Curl Script

`curl cordon.sh/install.sh | sh` downloads a prebuilt binary from GitHub releases. Not `npm install -g`, not `brew install`, not building from source.

- No runtime dependency. The user does not need Node, Go, Homebrew, or any toolchain.
- Prebuilt binaries are produced by GitHub Actions on tagged releases. Cross-compilation in Go is a single environment variable, not a complex toolchain.
- The install script detects the platform (OS + architecture), downloads the correct binary, and places it on PATH. This is the pattern used by Deno, Fly, Hugo, and Rust.
- Building from source would require Go installed on the user's machine. Most developers do not have Go unless they are Go developers.
- The script is hosted at cordon.sh and is inspectable before running.
