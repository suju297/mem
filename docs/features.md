# Mempack Feature Reference

This document lists the user-facing features available in the current Mempack implementation.

## Core Concepts
- Repo-scoped, local-first memory stored in a per-repo SQLite database; repo ids are derived from git origin/first commit or fallback to the repo path.
- Data types include state (current + checkpoints), memories (threaded summaries), evidence (chunks) with artifacts, and link trails for supersedes.
- Workspaces isolate all records (state, memories, chunks, threads, artifacts) per repo; the default workspace is configurable.
- Context packs are the stable retrieval output for both CLI and MCP (JSON or prompt formats).
- Config/data/cache directories follow XDG paths with overrides via `--data-dir` or `MEMPACK_DATA_DIR`.

## CLI Commands
| Command | Purpose | Notes |
| --- | --- | --- |
| `mem init` | Initialize memory for the current repo | Writes `.mempack/MEMORY.md` + `AGENTS.md` (or `.mempack/AGENTS.md` if `AGENTS.md` exists), sets the active repo, adds a welcome memory; `--no-agents` skips files |
| `mem template agents` | Print agent instructions | `--write` writes `.mempack/MEMORY.md` and `AGENTS.md` to the current directory |
| `mem get "<query>"` | Retrieve a context pack | `--format json|prompt`, `--workspace`, `--include-orphans`, `--repo`, `--debug` |
| `mem explain "<query>"` | Explain ranking and budget decisions | `--workspace`, `--include-orphans`, `--repo` |
| `mem add` | Add a memory | Requires `--thread`, `--title`, `--summary`; optional `--tags`, `--workspace`, `--repo` |
| `mem supersede <id>` | Replace a memory | Creates a new memory, marks the old one as superseded, adds link trail; `--title`, `--summary`, optional `--thread`, `--tags` |
| `mem checkpoint` | Save state (current + history) | Requires `--reason` and `--state-file` or `--state-json`; optional `--thread` to create a reason memory |
| `mem show <id>` | Show a memory or chunk | Returns full record; supports `M*` and `C*` ids |
| `mem forget <id>` | Soft-delete a memory or chunk | Marks records as deleted without removing them from the DB |
| `mem ingest-artifact <path>` | Ingest files as evidence chunks | Requires `--thread`; supports `--max-file-mb`, `--chunk-tokens`, `--overlap-tokens` |
| `mem embed` | Backfill embeddings | `--kind memory|chunk|all`; uses configured embed provider |
| `mem embed status` | Show embedding status | Reports provider availability, coverage, queue depth, and worker status |
| `mem threads` | List threads | Returns thread summaries and memory counts |
| `mem thread <thread_id>` | Show a thread | `--limit` controls how many memories are returned |
| `mem repos` | List known repos | Reads repo metadata from the data directory |
| `mem use <repo_id|path>` | Set the active repo | Accepts a repo id or a path to detect |
| `mem mcp` | Run MCP server | `--allow-write`, `--write-mode ask|auto|off`, `--repo`, `--debug`, `--repair`, `--name`, `--version` |
| `mem mcp start|stop|status` | Manage background MCP server | Uses a PID file and log under the config dir |
| `mem doctor` | Health checks and repair | `--json`, `--verbose`, `--repair` |
| `mem version` / `mem --version` | Print version | Includes semver + commit when built with ldflags |

## MCP Server and Tools
- Local stdio MCP server (`mem mcp`) with read tools enabled by default.
- Tools: `mempack.get_context`, `mempack.explain`, and write tools `mempack.add_memory`, `mempack.checkpoint`.
- Write tools are gated by `--allow-write` or `mempack.allow_write=true` in `.mempack/MEMORY.md`.
- Write modes: `ask` (default when allowed), `auto`, `off`; `ask` requires `confirmed=true`.
- Startup health check validates repo, DB, schema, and FTS; `--debug` prints a JSON report and `--repair` can fix invalid state.

## Configuration and Defaults
- Config file lives under `~/.config/mempack/config.toml` by default (XDG paths honored).
- Tokenization defaults to `cl100k_base` with `token_budget=2500`, `state_max=600`, `memories_k=10`, `memory_max_each=80`, `chunks_k=4`, `chunk_max_each=320`.
- `default_workspace` sets the workspace used when none is provided.
- Embedding settings include `embedding_provider`, `embedding_model`, and `embedding_min_similarity`.
- `active_repo`, `repo_cache`, and `mcp_auto_repair` are persisted in config for faster repo detection and MCP behavior.

## Retrieval Pipeline
- FTS5 + BM25 search over memory titles, summaries, tags, entities, and chunk locator/text/tags.
- Query sanitization and rewrite for token variants (hyphen/underscore/concat), NEAR boosts, and prefix search for single-token queries.
- Optional vector search using embeddings with RRF fusion; vector-only hits are filtered by `embedding_min_similarity`.
- Search metadata includes mode (bm25/vector/hybrid), rewrites applied, and warnings when fallback behavior is used.
- Git reachability filtering removes orphaned memories by default; `--include-orphans` overrides this.

## Ranking and Budgeting
- Ranking signals include BM25/FTS score, vector rank, recency bonus, thread bonus, and supersede penalties.
- Chunks containing prompt-injection phrases are heavily down-ranked.
- Token budgeting enforces `token_budget` with caps for state, memories, and chunks; low-ranked items are dropped.
- State, memory, and chunk text are truncated to fit token limits with cached token counts.
- `mem get --debug` prints timings and counts for the retrieval pipeline.

## Context Pack Output
- JSON output includes repo/workspace, state, matched threads, top memories, top chunks, link trail, rules, budget, and search metadata.
- Prompt format adds a structured, copy-ready output with headings and "Evidence (Data Only)" blocks.
- Raw chunks are included in `top_chunks_raw` for prompt mode; JSON output includes deduped chunks with sources.

## State and Memory Management
- State is stored as `state_current` with append-only checkpoint history.
- `mem checkpoint` accepts JSON or Markdown; non-JSON is wrapped as `{ "raw": "..." }`.
- If the DB has no state, mempack reads `.mempack/state.json` or `STATE.md` from the repo.
- `mem supersede` creates bidirectional links (`supersedes` / `superseded_by`) and down-ranks old memories.
- Tags are normalized, stored, and indexed for search.

## Evidence Ingestion
- Ingests files or directories with `.gitignore`, `.mempackignore`, and built-in ignore patterns.
- File allowlist: `.md .txt .rst .log .json .yaml .yml .toml .py .go .js .ts .tsx .java .kt .rs .c .cpp .h .cs .sql .sh`.
- Chunking is token-based with overlap; locators include git commit + line ranges when available.
- Chunks are deduplicated by hash and locator; artifacts record source and content hash.

## Embeddings and Hybrid Retrieval
- Config options: `embedding_provider` (auto/none/ollama/python/onnx), `embedding_model`, `embedding_min_similarity`.
- `auto` uses Ollama when available (respects `OLLAMA_HOST`); no auto-downloads, BM25-only fallback with warnings.
- `mem embed` backfills missing embeddings for memories and chunks.
- `mem embed status` reports provider availability, coverage (missing/stale/dim mismatch), queue depth, and worker status.
- New memories enqueue embeddings; the MCP background worker drains the queue when embeddings are enabled.

## Safety and Governance
- Local-only, repo-scoped storage; no hosted service required.
- Evidence is labeled as data; suspicious phrases in chunks are down-ranked.
- MCP write tools enforce opt-in and can require explicit user confirmation (ask mode).
- MCP writes scan for secrets and prompt-injection phrases; unsafe content is rejected or tagged as untrusted.

## Health and Maintenance
- `mem doctor` checks repo resolution, DB existence, schema version, FTS tables, and state validity.
- `--repair` can rebuild FTS and normalize invalid state; `mcp_auto_repair` enables this on MCP start.
- `mem repos` lists known repos and last-seen timestamps; `mem use` updates the active repo.
- SQLite runs in WAL mode with a busy timeout for concurrency.
