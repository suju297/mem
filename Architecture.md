# Architecture

## Overview
Mempack is an MCP-first, local memory system for coding agents. The primary
interface is the MCP server (`mem mcp`) that exposes stable tools for retrieval
and optional writes. The CLI remains fully featured for setup, maintenance, and
manual use. Mempack stores state, memories (decisions/summaries), and evidence
(indexed chunks from artifacts) in an embedded SQLite database per repo, then
retrieves context using BM25 with vector search when available (BM25-only
fallback).

## Design Goals
- MCP-first integration with stable tool APIs for agents.
- Repo-scoped, local data; no hosted service required.
- Fast retrieval with predictable latency.
- Deterministic, inspectable ranking via `explain`.
- CLI parity for setup, debugging, and manual workflows.
- Safe evidence handling (chunks are labeled as data, not instructions).

## High-Level Architecture

```
+---------+    +-------------------+    +------------------+    +--------------------+
| Agents  | -> | MCP Server        | -> | App Layer        | -> | Store (SQLite/FTS)  |
| (MCP)   |    | (mem mcp)         |    | (internal/app)   |    | (internal/store)   |
+---------+    +-------------------+    +------------------+    +--------------------+
                        ^                        |                         |
                        |                        |                         +-- Schema + migrations
                   +---------+                   |                         +-- FTS5 + triggers
                   | CLI     |                   |                         +-- Repo-scoped DB
                   | (mem)   |                   |
                   +---------+                   +-- Repo detection (internal/repo)
                                                  +-- Config (internal/config)
                                                  +-- Tokenization (internal/token)
                                                  +-- Embeddings (internal/embed)
                                                  +-- Context pack (internal/pack)
```

## Core Packages
- `cmd/mem`: CLI and MCP server entry point and command routing.
- `internal/app`: Command handlers, retrieval pipeline, ranking, budgeting, MCP.
- `internal/store`: SQLite schema, migrations, and CRUD/search methods.
- `internal/repo`: Git root detection and reachability checks.
- `internal/config`: XDG-backed config and repo cache.
- `internal/token`: Token counting and truncation (tiktoken).
- `internal/embed`: Optional embedding providers (Ollama implemented).
- `internal/health`: Health checks and repairs used by `doctor` and MCP.
- `internal/pack`: Context pack types returned by CLI/MCP.

## Data Model (SQLite)
The schema is defined in `internal/store/schema.sql` and migrated via
`internal/store/migrate.go`.

Key entities (tables):
- `repos`: repo metadata (git root, last head/branch).
- `state_current`: latest JSON state per repo/workspace.
- `state_history`: append-only state checkpoints.
- `threads`: thread metadata.
- `memories`: user summaries/decisions (threaded, anchor_commit).
- `artifacts`: ingested sources (files, etc.).
- `chunks`: artifact excerpts with locators and token counts.
- `embeddings`: optional vector embeddings for memories/chunks.
- `links`: typed links between memories.
- `meta`: internal metadata (e.g., last migration time).

Workspaces are a first-class namespace: all user data is scoped by
`repo_id + workspace`.

## Storage and Indexing
- SQLite DB stored at: `~/.local/share/mempack/repos/<repo_id>/memory.db`
  (via XDG directories).
- FTS5 virtual tables: `memories_fts`, `chunks_fts`.
- Triggers keep FTS tables in sync on insert/update/delete.
- WAL mode and a busy timeout are configured for good concurrency.
- Chunk deduplication uses `(repo_id, locator, text_hash, thread_id)` uniqueness.

## Retrieval Pipeline (MCP `get_context` primary, CLI `mem get` parity)
The MCP server and CLI share the same retrieval pipeline; MCP is the default
integration path for agents.

1) Load config and resolve repo/workspace.
2) Open store and ensure repo metadata exists.
3) Load current state JSON and cached token counts.
4) Run FTS searches:
   - Memories: title/summary/tags/entities.
   - Chunks: locator/text/tags.
5) Optional vector search (if embedding provider configured):
   - Embed query text.
   - Score stored embeddings with cosine similarity.
   - Apply `embedding_min_similarity` only to vector-only additions.
6) Merge FTS and vector-only results.
7) Rank results:
   - RRF fusion of FTS + vector ranks.
   - Recency bonus and thread bonus.
   - Git reachability filter for orphans (unless `--include-orphans`).
   - Safety penalty for suspicious phrases in chunks.
8) Apply token budget:
   - Truncate state, memory summaries, and chunk text as needed.
   - Drop lowest-ranked items until within budget.
9) Emit context pack (JSON or prompt format).

The context pack includes state, top memories, top chunks, matched threads,
and an explicit rule set: "state is authoritative."

## Ranking Details
- BM25 scores from FTS5 are combined with vector ranks using RRF.
- Recency and thread bonuses bias results toward fresh, relevant threads.
- Superseded memories are down-ranked.
- Chunks that contain prompt-injection phrases receive a large penalty.

## Ingestion Pipeline (`mem ingest-artifact`)

1) Resolve repo/workspace and open the store.
2) Read file(s) from a path; apply ignore rules:
   - `.gitignore` and `.mempackignore`.
   - File size and extension filters.
3) Tokenize content line-by-line and compute chunk ranges.
4) Create an `artifact` record and chunk records with locators and hashes.
5) Insert chunks with deduplication (unique index).

## Embeddings and Hybrid Retrieval
- Embeddings are default-auto via config:
  `embedding_provider = "auto"` uses Ollama when available, otherwise BM25-only.
- `embedding_model` defaults to `nomic-embed-text`; `embedding_min_similarity`
  gates vector-only additions.
- New memories are queued for embedding and processed by the MCP server in the background.
- `mem embed` backfills missing embeddings for memories/chunks.
- Vector search runs in-memory scoring using cosine similarity.
- RRF merges vector and FTS results for robust hybrid ranking.

## MCP Server Architecture
- `mem mcp` runs a local stdio MCP server (mcp-go).
- Read tools are always available:
  - `mempack.get_context`
  - `mempack.explain`
- Write tools are gated by `--allow-write` and `write-mode`:
  - `mempack.add_memory`
  - `mempack.checkpoint`
- A health check runs at startup to verify DB, schema, state, and FTS.

## Health and Repair (`mem doctor`)
- Validates repo resolution, DB existence, schema version, and FTS tables.
- Detects invalid JSON in `state_current`.
- Optional repairs rebuild FTS and normalize invalid state.

## Configuration and Repo Scoping
- Config stored in XDG config directory as `config.toml`.
- Each repo has an ID derived from git root + origin data.
- `RepoCache` accelerates repo detection to avoid extra git calls.
- Workspaces isolate memories/chunks/state for parallel projects.

## Output Contract (Context Pack)
The `ContextPack` type in `internal/pack` is the stable output format for CLI
and MCP clients. It includes repo info, workspace, state, ranked items, and
budget metadata to support deterministic downstream behavior. The `search_meta`
field reports whether retrieval was BM25-only, vector-only, or hybrid, plus any
vector availability warnings.
