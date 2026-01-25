# Mempack: Repo-Scoped Memory for Coding Agents

**Mempack** is a fast, local-first CLI that gives coding agents persistent, **repo-scoped memory**.
It stores **state**, **memories** (decisions/summaries), and **evidence** (indexed code chunks) in an embedded SQLite DB, retrievable via FTS5 (BM25) and git reachability filtering.

## Setup (first time)

1) Install mempack (see Installation below).
2) Initialize in your repo:

```bash
mem init
```

If `AGENTS.md` already exists, `mem init` writes `.mempack/AGENTS.md` instead
and prints the two lines to add manually.

3) Connect Codex to the local MCP server (recommended):

```bash
codex mcp add mempack -- mem mcp
codex mcp list
```

One-liner (Codex):
```bash
codex mcp add mempack -- mem mcp
```
Codex MCP configuration details live in the Codex repo.

4) Start the MCP server when you want implicit memory:

```bash
mem mcp
```

That is it. The agent will call `mempack.get_context("<task>")` at task start.

If you want the agent to save memories via MCP, enable writes (ask mode requires user approval):

```bash
mem mcp --allow-write --write-mode ask
```

Repo opt-in alternative: add `mempack.allow_write=true` to `.mempack/MEMORY.md`
to allow write tools without passing `--allow-write`.

Helper commands (for manual/background use):

```bash
mem mcp start --allow-write --write-mode ask
mem mcp status
mem mcp stop
```

## Quick Start (manual CLI)

```bash
mem add --thread T-auth --title "Auth refactor plan" --summary "Move auth logic to middleware; invalid token returns 401."
mem get "auth middleware" --format prompt
```

## Retrieval tips

- Default search is lexical AND + NEAR boost; top results are order-insensitive.
- Rewrites (for example `delta99 -> delta 99`) only apply when the base query has zero hits and are reported in `search_meta`.
- Use `mem explain "<query>"` to debug mode, ranking, and fallbacks.
- Prefer exact tokens from titles/summaries; avoid misspellings when possible.

## Workspaces

- Memories, threads, artifacts, and chunks are isolated per workspace.
- Use `--workspace <name>` (CLI) or `workspace` (MCP) to target a workspace.
- Default workspace is `default` and can be set via `default_workspace` in `XDG_CONFIG_HOME/mempack/config.toml` (defaults to `~/.config/mempack/config.toml`).

## Embeddings (default: auto)

- `embedding_provider = "auto"` tries Ollama locally; if unavailable or the model is missing, Mempack falls back to BM25-only with warnings in `search_meta`.
- Vectors are configured by default; availability depends on Ollama + the model being present.
- If vectors are unavailable, `mem embed status` shows the reason and fix steps.
- Mempack never auto-downloads models; to enable vectors, run `ollama pull <model>` yourself.
- Default model for Ollama is `nomic-embed-text` (override with `embedding_model`).
- Auto-embeddings are queued for memories and processed in the MCP server background.
- Backfill embeddings for existing data with `mem embed --kind all` (or `--kind chunk` for chunks).

## MCP output contract (current)

- `mempack.get_context` returns a prompt string plus structured `ContextPack`; `format=json` also includes a JSON text fallback.
- `search_meta` reports `mode_used`, `fallback_reason`, `rewrites_applied`, and `warnings`.
- Ordering is deterministic; ties are broken by recency (newer first).
- Duplicate chunks collapse into one entry with `sources[]` populated.
- Workspace isolation applies to memories, threads, artifacts, chunks, and state.

## Retrieval behavior (current)

- Lexical AND + NEAR boost is the default; top results are order-insensitive.
- Conditional rewrites apply only when the base query returns zero hits.
- Vector fallback is used when BM25 is empty (see `fallback_reason=bm25_empty` and warnings).
- Empty retrievals still return state.

## CLI parsing note

Flags can appear before or after positional args. For example:

```bash
mem thread T-auth --limit 20
mem show M-123 --repo r_deadbeef
```

## Implicit memory via MCP (no copy/paste)

Start the local MCP server:

```bash
mem mcp
```

Tools exposed (read-only by default):
- `mempack.get_context`: returns a prompt-friendly Context Pack + JSON
- `mempack.explain`: explains ranking and filtering decisions

Example MCP structured payload (JSON) in `docs/mcp_example.json` (captured via `mempack.get_context`). The tool response also includes a prompt string; the structured JSON is the richer payload:

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

## Acceptance Proof (Git repo)
- Init in a git repo; add two memories anchored to commit A and commit B.
- On commit B, `delta99` returns the commit B memory with rewrite metadata.
- After checkout to commit A, `delta99` returns no memories (orphan filtered).
- With `--include-orphans`, the commit B memory returns as expected.
- Duplicate chunk ingestion collapses to one chunk with two `sources[]`.

To expose a **save** tool, start MCP with writes enabled (or opt-in via `.mempack/MEMORY.md`):

```bash
mem mcp --allow-write --write-mode ask
```

Write tools (enabled by `--allow-write` or repo opt-in; `write-mode=ask` requires user approval):
- `mempack.add_memory`: saves a short decision/summary
- `mempack.checkpoint`: saves the current state JSON

Health checks:

```bash
mem doctor
```

Note: MCP config changes are not required for these updates. If you want auto-repair at MCP startup, either set `mcp_auto_repair = true` in the mempack config or add `--repair` to the MCP args in your client config.

## Config (optional)

Config file: `XDG_CONFIG_HOME/mempack/config.toml` (defaults to `~/.config/mempack/config.toml`).

Data directory overrides (highest priority first):
- `--data-dir <path>` (CLI)
- `MEMPACK_DATA_DIR=<path>` (env)
- `data_dir = "..."` in `config.toml`

Example for restricted environments:

```bash
export MEMPACK_DATA_DIR="$PWD/.mempack_data"
mem init
mem embed status
```

```toml
default_workspace = "default"
mcp_auto_repair = false
embedding_provider = "auto"
embedding_model = "nomic-embed-text"
embedding_min_similarity = 0.6
```

- `default_workspace` is used when no workspace is provided.
- `mcp_auto_repair` makes `mem mcp` behave like `mem mcp --repair`.
- `embedding_provider` enables hybrid retrieval (`auto`, `none`, `ollama`, `python`, `onnx`).
- `embedding_model` selects the embedder's model (provider-specific).
- `embedding_min_similarity` drops low-similarity vector-only matches.

To build embeddings after enabling a provider:

```bash
mem embed --kind all
```

### Wire mempack MCP into Codex (CLI + IDE share config)

Codex stores MCP server config in `~/.codex/config.toml`, shared between the CLI and IDE extension.

IDE setup (one time):
1) Install the Codex IDE extension for your editor (VS Code/Cursor/Windsurf).
2) Sign in when prompted.
3) Open `~/.codex/config.toml` from the Codex gear menu: Codex Settings > Open config.toml.
4) Restart the IDE if you do not see Codex in the sidebar.

Option A - CLI (recommended)

```bash
codex mcp add mempack -- mem mcp
codex mcp list
```

Codex's MCP CLI supports adding a stdio server by passing the server command after `--`.

To enable write tools (ask mode), register with write args:

```bash
codex mcp add mempack -- mem mcp --allow-write --write-mode ask
codex mcp list
```

Option B - config.toml

Add this to `~/.codex/config.toml`:

```toml
[mcp_servers.mempack]
command = "mem"
args = ["mcp"]
# Optional: show only mempack tools
enabled_tools = ["mempack.get_context", "mempack.explain"]
```

Codex supports `[mcp_servers.<name>]` with `command`/`args` for stdio servers, plus `enabled_tools` for tool allow-listing.

To expose write tools, add the write args and allow-list them:

```toml
[mcp_servers.mempack]
command = "mem"
args = ["mcp", "--allow-write", "--write-mode", "ask"]
enabled_tools = [
  "mempack.get_context",
  "mempack.explain",
  "mempack.add_memory",
  "mempack.checkpoint",
]
```

What happens after setup

Codex launches configured MCP servers automatically and exposes their tools alongside built-ins.

Other MCP clients

Codex supports stdio and streamable HTTP MCP servers.

If a client supports stdio MCP, configure it with:

```txt
command: mem
args: ["mcp"]
```

If a client only supports HTTP MCP, it will not connect to a stdio server directly; you'd need an HTTP bridge or an HTTP mode later.

## Performance

Measured on Apple M4, local SSD (20 runs) with 1k memories + 6.6k chunks (~14MB DB):

- **Fixed overhead:** ~8ms (repo_detect ~6.6ms, tokenizer 0ms)
- **End-to-end retrieval:** **p50 27ms / p95 28ms**
- **MCP stdio tool call:** **p50 13ms / p95 14ms**

## Installation

### Prebuilt (recommended)

```bash
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/v0.2.0/scripts/install.sh | sh -s -- --repo <owner>/<repo> --version v0.2.0
```

Release assets are expected to be named like:
`mempack_<os>_<arch>.tar.gz` and contain the `mem` binary.
Default install dir is `~/.local/bin` (override with `--install-dir`).

### From source

```bash
go build -o mem ./cmd/mem
```

Optional (Go install, with version pinning):

```bash
go install github.com/<owner>/<repo>/cmd/mem@v0.2.0
```

## Core Commands

- `mem init [--no-agents]`
- `mem get "<query>" --format prompt|json [--workspace <name>]`
- `mem add --thread <id> --title <str> --summary <str> [--workspace <name>]`
- `mem ingest-artifact <path> --thread <id> [--workspace <name>]`
- `mem embed [--kind memory|chunk|all] [--workspace <name>]`
- `mem mcp [--repo <id|path>] [--allow-write] [--write-mode ask|auto|off] [--repair]`
- `mem doctor [--json] [--repair] [--verbose]`

## Features
- **Git Awareness**: Automatically filters out "orphaned" memories from unreachable commits.
- **Artifact Ingestion**: Ingest code files with `ingest-artifact`.
- **Token Budgeting**: Strict hard cap on output tokens (default 2500).
- **Security**:
  - Chunks with adversarial phrases (e.g. "ignore previous instructions") are automatically downranked.
  - Evidence output is clearly labeled as data.
- **Hybrid Retrieval (optional)**: Enable an embedder and run `mem embed` to fuse vector search with BM25.
- Full feature reference: `docs/features.md`.
