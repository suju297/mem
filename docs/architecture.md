# Mempack Architecture

This document explains the runtime architecture behind Mempack CLI, MCP tools, and extension integration.

## 1) Runtime components

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

## 2) Package responsibilities

- `cmd/mem`: program entrypoint.
- `internal/app`: command routing, MCP handlers, context construction, ranking, budgeting.
- `internal/store`: schema + migrations + record APIs + FTS queries.
- `internal/embed`: provider resolution and vector embedding calls.
- `internal/config`: XDG config handling and data dir resolution.
- `extensions/vscode-mempack`: extension UI, one-shot MCP calls, auto-session capture.

## 3) Retrieval flow

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

## 4) Persistence model

Global config path:
- `XDG_CONFIG_HOME/mempack/config.toml` (default `~/.config/mempack/config.toml`)

Repo DB path:
- `<data_dir>/repos/<repo_id>/memory.db`

Main tables:
- `repos`
- `state_current`, `state_history`
- `threads`, `memories`
- `artifacts`, `chunks`
- `embeddings`, `embedding_queue`
- `links`
- `meta`

FTS tables:
- `memories_fts`
- `chunks_fts`

## 5) MCP runtime modes

- One-shot stdio: extension spawns `mem mcp ...` per tool call and tears it down after the call.
- Local daemon: `mem mcp start|status|stop` with PID/log files.
- Manager mode: `mem mcp manager` runs TCP control-plane on `127.0.0.1:<port>` and coordinates daemon lifecycle.

## 6) Repo scoping

Resolution order:
1. Explicit repo override (`--repo` or MCP `repo` argument)
2. Git root from current working directory
3. `active_repo` fallback (when strict mode is off)

Set `--require-repo` / `mcp_require_repo=true` to disable fallback and fail fast.

