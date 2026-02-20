# Mempack: Repo-Scoped Memory for Coding Agents

**Mempack** is a fast, local-first CLI that gives coding agents persistent, **repo-scoped memory**.

It stores:

* **State**: current status/context you want the agent to remember
* **Memories**: short decisions or summaries
* **Evidence**: indexed code chunks from your repo

All data stays in a local SQLite database. Search is keyword-based by default, with optional vector search if embeddings are enabled.

---

## Setup (first time)

1. Install mempack (see **Installation** below).

2. Initialize in your repo:

```bash
mem init
```

**AGENTS.md behavior**

* If `AGENTS.md` does **not** exist, `mem init` writes it.
* If `AGENTS.md` **already exists**, `mem init` **does not overwrite it**.

  * It writes `.mempack/AGENTS.md` (helper stub)
  * It prints **two lines** you should paste into your existing `AGENTS.md`

> Note: Most assistants only read repo-root instruction files like `AGENTS.md`.
> `.mempack/AGENTS.md` is a convenience stub, not a guarantee the agent will read it.

3. Connect Codex to the local MCP server (recommended):

```bash
codex mcp add mempack -- mem mcp
codex mcp list
```

One-liner:

```bash
codex mcp add mempack -- mem mcp
```

Codex MCP configuration details live in the Codex repo.

4. Start the MCP server when you want implicit memory:

```bash
mem mcp
```

That's it. The agent will call `mempack_get_context("<task>")` at task start.

For session startup, call `mempack_get_initial_context` once to get a short summary (state, recent threads, counts).

MCP write tools are enabled by default in ask mode (requires user approval).
To disable writes: `mem mcp --write-mode off` or set `mcp_allow_write=false` in config or `.mempack/config.json`.
To auto-write (no confirmation): `mem mcp --write-mode auto` or set `mcp_write_mode=auto`.

Helper commands (for manual/background use):

```bash
mem mcp start
mem mcp status
mem mcp stop
```

---

## Important: MCP repo selection (avoid "wrong repo")

MCP servers may be started from a global location (not inside your repo). If Mempack can't resolve the repo from the MCP server's current working directory, it may fall back to `active_repo` (unless strict mode is enabled). That's how you end up "reading the wrong repo."

### Reliable ways to make repo selection correct

**Option A (best, VS Code extension should do this): pass `repo` on every MCP call**
If the client knows your workspace root, always pass `repo=<workspaceRoot>` (path). This is deterministic even if MCP runs globally.

**Option B: start MCP locked to a repo**

```bash
mem mcp --repo /path/to/repo
```

**Option C: strict mode (recommended for global Codex setup)**

* CLI:

  ```bash
  mem mcp --require-repo
  ```
* Or config:

  ```toml
  mcp_require_repo = true
  ```

Strict mode fails fast instead of silently picking `active_repo`.

### Recommended defaults

* **VS Code extension**: always pass `repo=workspaceRoot` on every tool call.
* **Codex global MCP config**: prefer `--require-repo` to prevent wrong-repo reads.

---

## Quick Start (manual CLI)

```bash
mem add --thread T-auth --title "Auth refactor plan" --summary "Move auth logic to middleware; invalid token returns 401."
mem get "auth middleware" --format prompt
```

---

## Architecture (Detailed)

Mempack is split into small internal modules so CLI, MCP, and extension flows share the same core logic.

### Core packages

* `cmd/mem`: program entrypoint.
* `internal/app`: command dispatch, MCP handlers, retrieval orchestration, ranking, token budgeting.
* `internal/store`: SQLite schema, migrations, FTS, records API, link persistence.
* `internal/embed`: embedding provider resolution + optional vector generation/search support.
* `internal/config`: XDG config loading, data dir resolution, repo state persistence.
* `extensions/vscode-mempack`: VS Code/Cursor UI, one-shot MCP tool calls, auto session capture.

### High-level system diagram (ASCII)

```text
+-------------------------+          +--------------------------+
| User / Agent / Scripts  |          | VS Code / Cursor Ext     |
| - mem CLI commands      |          | - Sidebar / Commands     |
+------------+------------+          +------------+-------------+
             |                                    |
             | mem ...                            | one-shot MCP tool calls
             v                                    v
      +------+------------------------------------+------+
      |            mem CLI (internal/app)                |
      | - global flag parsing / command dispatch         |
      | - repo + workspace resolution                    |
      +------+-------------------------------+------------+
             |                               |
             | retrieval path                | MCP tool handlers
             v                               v
 +-----------+-------------------+   +-------+------------------+
 | Context Builder / Rank /      |   | mempack_get_context      |
 | Budget                         |   | mempack_explain          |
 | - query parse + hints          |   | mempack_add/update/link  |
 | - BM25 + optional vectors      |   | mempack_checkpoint       |
 +-----------+-------------------+   +-----------+--------------+
             |                                   |
             +-------------------+---------------+
                                 v
                    +------------+-------------+
                    | internal/store (SQLite)  |
                    | repos/<repo_id>/memory.db|
                    +------------+-------------+
                                 |
                                 v
                    +------------+-------------+
                    | internal/embed (optional)|
                    | Ollama / vectors         |
                    +--------------------------+
```

### Retrieval pipeline (ASCII)

```text
query
  -> validate + parse (intent/time/entity hints)
  -> BM25 search (memories + chunks via FTS)
  -> optional vector search (if embeddings available)
  -> merge + rank (RRF-style fusion, recency boosts, safety filtering)
  -> orphan filtering (git reachability unless include-orphans)
  -> token budgeting (state + memories + chunks under hard cap)
  -> context pack (prompt + structured JSON)
```

### Data model and storage layout

Global config:
* `~/.config/mempack/config.toml` (or `XDG_CONFIG_HOME/mempack/config.toml`)

Per-repo DB path:
* `<data_dir>/repos/<repo_id>/memory.db`

Primary tables:
* `repos`
* `state_current`, `state_history`
* `threads`, `memories`
* `artifacts`, `chunks`
* `embeddings`, `embedding_queue`
* `links`
* `meta`

FTS indexes:
* `memories_fts`
* `chunks_fts`

### MCP runtime modes

Mempack supports three practical MCP run modes:

* One-shot stdio server: used by the extension for tool calls (`mem mcp ...` spawned per request, then exits).
* Local daemon: `mem mcp start|status|stop` keeps a background process with PID/log files.
* Manager mode (optional): `mem mcp manager` runs a local TCP control plane (`127.0.0.1:<port>`) that coordinates daemon lifecycle.

### Repo scoping behavior

Repo resolution order for reads/writes:

1. Explicit repo override (`--repo` or MCP `repo` argument)
2. Git root from current working directory
3. `active_repo` fallback (only when strict mode is off)

Use `--require-repo` / `mcp_require_repo=true` to disable fallback and fail fast.

---

## Ingest (watch mode)

Use watch mode to keep artifacts indexed as you edit files:

```bash
mem ingest-artifact ./internal --thread T-dev --watch
```

The watcher logs create/modify/delete events and stops cleanly on Ctrl+C.

---

## Retrieval tips

* Search is keyword-based; for top results, word order usually doesn't matter.
* Rewrites (e.g., `delta99 -> delta 99`) only run when the base query has **zero hits** and are reported in `search_meta`.
* Use `mem explain "<query>"` when results look wrong.
* Use exact words from titles/summaries when possible; avoid misspellings.
* Intent/time hints (recent, today, file paths, thread IDs, symbols) are detected and shown in `search_meta`.

---

## Workspaces

* Memories, threads, artifacts, and chunks are isolated per workspace.
* Use `--workspace <name>` (CLI) or `workspace` (MCP) to target a workspace.
* Default workspace is `default` and can be set via `default_workspace` in `XDG_CONFIG_HOME/mempack/config.toml` (defaults to `~/.config/mempack/config.toml`).
* Default thread is `T-SESSION` and can be set via `default_thread` in `XDG_CONFIG_HOME/mempack/config.toml`.

---

## Embeddings (default: auto)

* `embedding_provider = "auto"` tries Ollama locally. If it's not available, Mempack falls back to keyword-only search and reports a warning in `search_meta`.
* Mempack never downloads models for you. To enable vectors, run:

  * `ollama pull <model>`
* Default model is `nomic-embed-text` (override with `embedding_model`).
* `mem embed status` shows whether embeddings are working and how to fix issues.
* Backfill existing data with:

  * `mem embed --kind all` (or `--kind chunk` for chunks).
* Recommended models: `nomic-embed-text` (default), `mxbai-embed-large`, `bge-small-en`, `bge-base-en`, `bge-large-en`.

---

## Memory clustering (optional)

* Use `mem get "<query>" --cluster` or MCP `cluster=true` to group similar memories.
* Requires embeddings; if embeddings are unavailable, results stay unclustered.
* Clustered entries include `is_cluster`, `cluster_size`, `cluster_ids`, and `similarity`.

---

## MCP output contract (current)

* `mempack_get_initial_context` returns a short startup summary (state, recent threads, counts).
* `mempack_get_context` returns a readable prompt string **plus** structured JSON (`ContextPack`).
* For MCP calls, structured JSON is always present; `format=prompt` only changes the human-readable text.
* `search_meta` includes: `mode_used`, `fallback_reason`, `rewrites_applied`, `warnings`, `intent`, `time_hint`, `recency_boost`.
* Ordering is deterministic; ties are broken by recency (newer first).
* Duplicate chunks collapse into one entry with `sources[]` populated.
* Workspace isolation applies to memories, threads, artifacts, chunks, and state.
* When clustering is enabled, `search_meta.clusters_formed` is populated and `top_memories` may include cluster metadata.

---

## Retrieval behavior (current)

* Keyword search is the default; word order usually does not change top results.
* Rewrites only apply when the base query returns zero hits.
* Vector fallback is used when keyword search returns nothing (see `fallback_reason=bm25_empty` and warnings).
* Empty retrievals still return state.
* Recency hints boost scores and populate `intent`, `time_hint`, and `recency_boost`.

---

## If something looks missing

* If `mempack_get_initial_context` or `--cluster` is missing, you're likely running an older `mem` binary. Use the repo build or reinstall.
* `search_meta` only shows up in JSON. Use `mem get "<query>" --format json` or `mem explain "<query>"` to inspect rewrites/fallbacks.
* MCP always returns structured JSON; some clients only display that. If you expect a prompt string, check the tool's text content.
* Rewrites only run when the base query has zero hits. If you already got results, no rewrite will appear.

---

## CLI parsing note

Flags can appear before or after positional args:

```bash
mem thread T-auth --limit 20
mem show M-123 --repo r_deadbeef
```

---

## Implicit memory via MCP (no copy/paste)

Start the local MCP server:

```bash
mem mcp
```

Tools exposed:

* `mempack_get_initial_context`
* `mempack_get_context`
* `mempack_explain`
* `mempack_add_memory`
* `mempack_update_memory`
* `mempack_link_memories`
* `mempack_checkpoint`

Write tools are gated by write mode (`ask|auto|off`). Default behavior is `ask` when writes are enabled.

Example MCP structured payload (JSON):

```json
{
  "search_meta": {
    "mode_used": "bm25",
    "rewrites_applied": ["delta99 -> delta 99"],
    "warnings": ["vectors_configured_but_unavailable"]
  },
  "top_chunks": [
    {
      "locator": "file:docs/mcp_example_source.txt#L1-L3",
      "sources": [{"chunk_id": "C-..."}]
    }
  ]
}
```

---

## Acceptance Proof (Git repo)

* Init in a git repo; add two memories anchored to commit A and commit B.
* On commit B, `delta99` returns the commit B memory with rewrite metadata.
* After checkout to commit A, `delta99` returns no memories (orphan filtered).
* With `--include-orphans`, the commit B memory returns.
* Duplicate chunk ingestion collapses to one chunk with two `sources[]`.

---

## Writes via MCP

Write tools are enabled by default in ask mode (requires `confirmed=true` after user approval).
To disable writes: `mem mcp --write-mode off` or set `mcp_allow_write=false`.
To auto-write (no confirmation): `mem mcp --write-mode auto`.

* `mempack_add_memory`
* `mempack_update_memory`
* `mempack_link_memories`
* `mempack_checkpoint`

Write modes:

* `ask` — requires `confirmed=true` after user approval (default)
* `auto` — no approval required
* `off` — disable writes

Repo overrides:

* Disable writes: `mcp_allow_write=false` in `.mempack/config.json`
* Auto-write: `mcp_write_mode=auto` in `.mempack/config.json`

---

## Health checks

```bash
mem doctor
```

If you want auto-repair at MCP startup:

* set `mcp_auto_repair = true` in config, or
* run `mem mcp --repair`

---

## Config (optional)

Global config: `XDG_CONFIG_HOME/mempack/config.toml` (default `~/.config/mempack/config.toml`)
Repo overrides: `.mempack/config.json`

Data directory overrides (highest priority first):

* `--data-dir <path>` (CLI)
* `MEMPACK_DATA_DIR=<path>` (env)
* `data_dir = "..."` in `config.toml`

Example:

```bash
export MEMPACK_DATA_DIR="$PWD/.mempack_data"
mem init
mem embed status
```

```toml
default_workspace = "default"
default_thread = "T-SESSION"
mcp_auto_repair = false
mcp_allow_write = true
mcp_write_mode = "ask"
mcp_require_repo = false
embedding_provider = "auto"
embedding_model = "nomic-embed-text"
embedding_min_similarity = 0.6
```

Notes:

* `mcp_require_repo` disables `active_repo` fallback; requests must resolve from explicit `repo` or current git repo (also available as `--require-repo`).
* `default_thread` is used when `--thread` is omitted.
* `embedding_min_similarity` drops low-similarity vector-only matches.

To build embeddings:

```bash
mem embed --kind all
```

---

## Wire mempack MCP into Codex (CLI + IDE share config)

Codex stores MCP server config in `~/.codex/config.toml`, shared between the CLI and IDE extension.

Option A — CLI:

```bash
codex mcp add mempack -- mem mcp
codex mcp list
```

**Recommended if you use Codex globally across many repos** (prevents wrong-repo reads):

```bash
codex mcp add mempack -- mem mcp --require-repo
```

Write tools are enabled by default in ask mode. To auto-write:

```bash
codex mcp add mempack -- mem mcp --write-mode auto
codex mcp list
```

Option B — config.toml:

```toml
[mcp_servers.mempack]
command = "mem"
args = ["mcp"]
enabled_tools = ["mempack_get_context", "mempack_explain"]
```

With write tools (ask mode by default):

```toml
[mcp_servers.mempack]
command = "mem"
args = ["mcp"]
enabled_tools = [
  "mempack_get_context",
  "mempack_explain",
  "mempack_add_memory",
  "mempack_update_memory",
  "mempack_link_memories",
  "mempack_checkpoint",
]
```

---

## Installation

### Prebuilt (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/<release>/scripts/install.sh | sh -s -- --repo <owner>/<repo> --version <release>
```

### From source

```bash
go build -o mem ./cmd/mem
```

Optional (Go install):

```bash
go install github.com/<owner>/<repo>/cmd/mem@<release>
```

---

## Core Commands

### Setup & Info

* `mem init [--no-agents] [--assistants agents|claude|gemini|all]`
* `mem doctor [--json] [--repair] [--verbose]`
* `mem version` / `mem --version`
* `mem repos`
* `mem use <repo_id|path>`

### Memory Management

* `mem add --title <str> --summary <str> [--thread <id>] [--tags ...] [--workspace <name>]`
* `mem update <id> [--title <str>] [--summary <str>] [--tags <csv>] [--tags-add <csv>] [--tags-remove <csv>] [--entities <csv>] [--entities-add <csv>] [--entities-remove <csv>] [--workspace <name>] [--repo <id>]`
* `mem show <id>`
* `mem forget <id>`
* `mem supersede <id> --title <str> --summary <str> [--thread <id>]`
* `mem link --from <id> --rel <relation> --to <id> [--workspace <name>] [--repo <id>]`
* `mem checkpoint --reason <str> --state-file <path>|--state-json <json> [--thread <id>]`

### Retrieval

* `mem get "<query>" --format prompt|json [--workspace <name>] [--cluster] [--include-orphans]`
* `mem explain "<query>" [--workspace <name>]`
* `mem threads [--workspace <name>]`
* `mem thread <thread_id> [--limit 20] [--workspace <name>]`
* `mem recent [--limit 20] [--workspace <name>]`
* `mem sessions [--needs-summary] [--count] [--limit 20] [--workspace <name>]`

### Ingestion & Embeddings

* `mem ingest-artifact <path> --thread <id> [--watch] [--workspace <name>]`
* `mem embed [--kind memory|chunk|all] [--workspace <name>]`
* `mem embed status`

### MCP Server

* `mem mcp [--repo <id|path>] [--require-repo] [--allow-write] [--write-mode ask|auto|off] [--repair]`
* `mem mcp start|stop|status`

### Templates

* `mem template agents [--write] [--assistants agents|claude|gemini|all] [--no-memory]`

---

## Features

* **Git Awareness**: Automatically filters out "orphaned" memories from unreachable commits.
* **Artifact Ingestion**: Ingest code files with `ingest-artifact`.
* **Token Budgeting**: Strict hard cap on output tokens (default 2500).
  * User perspective: higher budgets include more context in `mem get` output, lower budgets keep prompts shorter.
* **Security**:
  * Chunks with adversarial phrases (e.g. "ignore previous instructions") are automatically downranked.
  * Evidence output is clearly labeled as data.
* **Hybrid Retrieval (optional)**: Enable an embedder and run `mem embed` to fuse vector search with BM25.
* Full feature reference: `docs/features.md`.

---

## VS Code Extension (Implementation Guide)

A fast, hotkey-first UI for Mempack is available as a VS Code extension (`extensions/vscode-mempack/`).

**Correctness requirement (do this):**

* Always pass `repo = workspaceRoot` on every MCP tool call.
* Pass `workspace` if set.
* If spawning MCP, start it with `cwd = workspaceRoot`.

### Installation

```bash
cd extensions/vscode-mempack
npm install
npm run compile
npx vsce package
# Install: Extensions > ... > Install from VSIX > select mempack-x.x.x.vsix
```

### Features

* **Save Selection as Memory**: Highlight text, press `Cmd+Shift+M` (Mac) / `Ctrl+Shift+M` (Win/Linux)
* **Get Context**: Press `Cmd+Shift+G` to search and retrieve context
* **Sidebar**: Browse memory threads, recent memories, MCP status, and embeddings
* **Status node**: Read-only MCP runtime visibility (daemon/manager state + last spawned MCP call metadata)
* **Memory Actions**: View details, copy summary, copy as prompt snippet, delete

### Commands

* `Mempack: Init (in this repo)`
* `Mempack: Save Selection as Memory`
* `Mempack: Save Checkpoint`
* `Mempack: Get Context for Query`
* `Mempack: Doctor`
* `Mempack: Toggle MCP Writes`
* `Mempack: Configure Embeddings`

### Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| `mempack.binaryPath` | `"mem"` | Path to the mem binary |
| `mempack.workspace` | `""` | Optional workspace name |
| `mempack.defaultThread` | `"T-SESSION"` | Default thread for new memories |
| `mempack.recentLimit` | `10` | Number of recent memories to show |
| `mempack.commandTimeoutMs` | `10000` | Timeout for CLI commands |

---

## Performance

Benchmarks (Apple M4, 1k memories + 6.6k chunks, ~14MB DB):

* **End-to-end retrieval**: p50 27ms / p95 28ms
* **Repo detection**: ~9ms (was 100ms before caching optimization)
* **Tokenizer init**: 0ms on warm runs (lazy loading)
* **MCP get_context**: ~7ms (handler only), ~13ms via stdio

---

## License

Mempack is licensed under the MIT License.
See [LICENSE](LICENSE).

---

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for release notes.
