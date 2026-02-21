# Mempack: Repo-Scoped Memory for Coding Agents

Mempack is a local-first memory system for coding workflows.

It stores three things per repo:
- State: current project context.
- Memories: short durable decisions.
- Evidence: indexed code/document chunks.

Data is persisted in local SQLite under your configured `data_dir`.

## Documentation Map

Reference:
- Getting started + troubleshooting: `docs/onboarding.md`
- Full CLI syntax reference: `docs/cli.md`
- Storage layout, schema, and artifacts: `docs/storage.md`
- Architecture + runtime diagrams: `ARCHITECTURE.md`
- Sandbox evaluation/testing process: `docs/memory-testing-process.md`
- VS Code/Cursor extension: `extensions/vscode-mempack/README.md`

## Quick Start

1. Install CLI from GitHub Releases:

macOS/Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/suju297/mempack/main/scripts/install.sh | sh -s -- --repo suju297/mempack
```

Add PATH automatically during install (macOS/Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/suju297/mempack/main/scripts/install.sh | sh -s -- --repo suju297/mempack --add-to-path
```

Windows (PowerShell):

```powershell
iwr https://raw.githubusercontent.com/suju297/mempack/main/scripts/install.ps1 -OutFile $env:TEMP\\mempack-install.ps1; & $env:TEMP\\mempack-install.ps1 -Repo suju297/mempack
```

Windows PATH behavior:
- By default, installer updates user PATH (`-AddToPath $true`).
- To skip PATH update: `-AddToPath $false`.

If release assets are unavailable, installers fall back to source build (Go toolchain required).

Verify:

```bash
mem --version
```

2. Initialize in your repo:

```bash
mem init
```

3. Save and retrieve one memory:

```bash
mem add --thread T-setup --title "Mempack ready" --summary "Initialized memory for this repo"
mem get "Mempack ready" --format prompt
```

4. Connect Codex MCP:

```bash
codex mcp add mempack -- mem mcp --require-repo
codex mcp list
```

5. Daemon lifecycle commands:

```bash
mem mcp start
mem mcp status
mem mcp stop
```

## Repo Scoping

Repo scoping options:
- Pass `repo=<workspaceRoot>` on every MCP tool call.
- Start MCP with `--repo /path/to/repo`.
- Enable strict mode with `--require-repo` (or `mcp_require_repo = true`).

Resolution order is:
1. Explicit repo argument (`--repo` / MCP `repo`)
2. Git root from current working directory
3. `active_repo` fallback (disabled in require-repo mode)

## CLI Reference

Basic form:

```text
mem [--data-dir <path>] <command> [options]
mem <command> --help
mem --version
```

Command groups:

| Group | Commands |
|---|---|
| Setup | `init`, `doctor`, `repos`, `use`, `version` |
| Retrieval | `get`, `explain`, `show`, `threads`, `thread`, `recent`, `sessions` |
| Writes | `add`, `update`, `supersede`, `link`, `checkpoint`, `forget` |
| Ingest/Embed | `ingest`, `ingest-artifact`, `embed` |
| Session/Share | `session upsert`, `share export`, `share import` |
| MCP | `mcp`, `mcp start|stop|status`, `mcp manager`, `mcp manager status` |
| Templates | `template` |

Common options:
- `--data-dir <path>`: override data root.
- `--repo <id|path>`: explicit repo scope.
- `--workspace <name>`: workspace scope.
- `--format json`: machine-readable output where supported.

Examples:

```bash
mem get "auth middleware" --format json
mem add --title "Auth plan" --summary "Use middleware"
mem mcp status
```

Full command syntax: `docs/cli.md`

## MCP Tool Surface

Primary tools:
- `mempack_get_initial_context`
- `mempack_get_context`
- `mempack_explain`
- `mempack_add_memory`
- `mempack_update_memory`
- `mempack_link_memories`
- `mempack_checkpoint`

Write mode behavior:
- `ask`: default when writes are enabled; requires explicit confirmation
- `auto`: writes without confirmation
- `off`: disables write tools

## Configuration

Global config path:
- `XDG_CONFIG_HOME/mempack/config.toml` (default `~/.config/mempack/config.toml`)

Repo override path:
- `.mempack/config.json`

Data directory precedence:
1. `--data-dir <path>`
2. `MEMPACK_DATA_DIR=<path>`
3. `data_dir` in `config.toml`

Minimal example:

```toml
default_workspace = "default"
default_thread = "T-SESSION"
mcp_allow_write = true
mcp_write_mode = "ask"
mcp_require_repo = true
embedding_provider = "auto"
embedding_model = "nomic-embed-text"
```

## Architecture

Detailed diagrams and architecture contracts live in `ARCHITECTURE.md`.

```mermaid
flowchart TD
    CLI["CLI<br/>`mem ...`"]
    MCP["MCP Tools<br/>`mempack_*`"]
    EXT["VS Code/Cursor<br/>Extension"]

    APP["App Orchestrator<br/>`internal/app`"]
    CFG["Config + Repo Resolution<br/>`internal/config`, `internal/repo*`"]
    STORE["SQLite Store<br/>`internal/store`"]
    FTS["FTS Search Index<br/>SQLite FTS5"]
    EMBED["Embedding Layer<br/>`internal/embed` (optional)"]

    CLI --> APP
    MCP --> APP
    EXT --> APP
    APP --> CFG
    APP --> STORE
    STORE --> FTS
    APP --> EMBED
    EMBED -. vectors .-> STORE

    classDef edge fill:#E8F1FF,stroke:#2D6CDF,color:#0B2545,stroke-width:1.5px;
    classDef core fill:#E9FFF3,stroke:#1B8A5A,color:#0C3B2E,stroke-width:1.5px;
    classDef data fill:#FFF3E8,stroke:#C96A12,color:#4A2A00,stroke-width:1.5px;

    class CLI,MCP,EXT edge;
    class APP,CFG core;
    class STORE,FTS,EMBED data;
```

## VS Code/Cursor Extension

Extension implementation and usage are documented in:
- `extensions/vscode-mempack/README.md`

Extension control model:
- Extension status reflects CLI/daemon state.
- MCP lifecycle control uses CLI-backed commands.

## Development

Run tests:

```bash
go test ./...
```

Build CLI:

```bash
go build -o mem ./cmd/mem
```

Build extension:

```bash
cd extensions/vscode-mempack
npm install
npm run compile
npx @vscode/vsce package
```

## License

Mempack is licensed under MIT. See `LICENSE`.

## Release History

Release details are available in git tags and commit history.
