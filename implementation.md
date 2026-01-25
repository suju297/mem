# Implementation Notes

## Performance Optimizations

- **Repo Detection**: Reduced from ~100ms to **~9-15ms (warm)**.
  - Implemented `InfoFromCache` to skip git detection entirely for known repos.
  - Collapsed git operations to a single `git rev-parse HEAD --abbrev-ref HEAD` call per run.
- **Tokenizer Initialization**: Reduced from ~90ms to **0ms (warm)**.
  - Implemented strict lazy loading: tokenizer is only initialized if tokens are missing.
  - Optimized `normalizeState` to handle empty state `{}` without tokenizer.
  - Added caching of calculated state tokens to `state_current` DB to prevent future initialization.
  - State token cache timestamps now parse RFC3339Nano/RFC3339 to avoid cache misses.
- **Benchmarks**:
  - `repo_detect`: ~9ms vs 100ms baseline.
  - `tokenizer_init`: 0ms vs 90ms baseline.
  - Fixed overhead: **~15ms**; end-to-end retrieval **p50 44ms / p95 75ms** (see perf section below).

## What was done

- **Core CLI**: Expanded `mem` to cover `show`, `forget`, `supersede`, `checkpoint`, `repos`, `use`, `threads`, `thread`, and `ingest-artifact`.
- **UX Features**:
  - `mem init`: Bootstraps config, repo, and welcome thread (writes agent files by default).
  - `mem get --format prompt`: Outputs context in a paste-ready LLM format.
  - `mem` help output formatted into a command table with optional purple headings (TTY-only, respects `NO_COLOR`).
  - `mem mcp`: Starts a local MCP server (stdio) with read tools by default and optional write tools gated by `--allow-write`.
  - `mem --version` / `mem version`: Prints semver + commit SHA.
  - `mem doctor`: Health check with optional repair + JSON output.
- **Security**: Added downranking for adversarial phrases in chunks and clear labeling of evidence as data.
- **Repo Scoping**: Added `--repo` overrides and persisted `active_repo`.
- **Ingestion**: Implemented artifact ingestion with ignore rules, token-aware chunking, and deduplication.
- **Assistant Instructions**:
  - Moved instructions to `.mempack/MEMORY.md` to avoid filename collisions.
  - `mem init` writes both `.mempack/MEMORY.md` and a thin `AGENTS.md` stub by default (use `--no-agents` to skip).
  - `mem template agents --write` writes both files in the current directory.
  - Guidance now emphasizes: never edit the instruction file, default save behavior, and only ask for init if no trace exists.
  - `AGENTS.md` is intentionally minimal (MCP-first + fallback only).
- **Distribution**: Added `scripts/install.sh` + README prebuilt install guidance (GitHub Releases).
  - Optional version-pinned `go install` instructions in README.
  - `scripts/release_build.sh` for cross-compiled artifacts + checksums.
  - `CHANGELOG.md` with a short v0.2.0 release summary.
  - `scripts/install.sh` verifies `checksums.txt` when available.
- **FTS & Storage**:
  - FTS5 virtual tables (`memories_fts`, `chunks_fts`) with sanitization fixes for special chars.
  - Sync triggers and migration steps.
  - Token counts stored for all text to enable budget packing without re-tokenizing.
  - Added `meta` table to track `last_migration_at` for health reporting.
- **Correctness**:
  - Fixed `anchor_commit` bug where branch names were stored instead of SHAs (Test 2 pass).
  - Fixed `timings` variable scope bug.
- **Tests**: Added integration tests (reachability, budget), tokenizer timestamp parsing test (`get_test.go`), MCP tool tests (`mcp_test.go`), and performance benchmarks (`full_test.sh`).

## Layout

- `.mempack/MEMORY.md`: Agent instructions for using mempack in the repo.
- `AGENTS.md`: Thin instruction stub pointing to `.mempack/MEMORY.md` (MCP-first).
- `CHANGELOG.md`: Release notes.
- `cmd/mem/main.go`: CLI entry point.
- `internal/app/`: command handlers, ranking, budgeting, state loading, ingestion.
- `internal/app/version.go`: Version + commit string (overridable via ldflags).
- `internal/health/`: Repo/DB/schema/FTS health checks shared by MCP + doctor.
- `internal/repo/`: git detection and ancestor checks.
- `internal/store/`: schema, triggers, migrations, and store methods.
- `internal/store/schema.sql`: base schema.
- `internal/store/triggers.sql`: FTS sync triggers.
- `internal/store/migrate.go`: user_version + FTS rebuild on upgrade.
- `internal/store/meta.go`: meta table helpers (last migration timestamp).
- `internal/token/`: tokenizer wrapper (tiktoken).
- `internal/pack/`: Context Pack structs.
- `scripts/install.sh`: GitHub Releases installer (prebuilt binaries).
- `scripts/release_build.sh`: Build release artifacts + checksums.

## Commands implemented

- `mem get "<query>" [--workspace default] [--include-orphans] [--format json] [--repo <id>] [--debug]`
- `mem add --thread <id> --title <title> --summary <summary> [--tags tag1,tag2] [--repo <id>]`
- `mem explain "<query>" [--workspace default] [--include-orphans] [--repo <id>]`
- `mem show <id> [--repo <id>]`
- `mem forget <id> [--repo <id>]`
- `mem supersede <id> --title <title> --summary <summary> [--thread <id>] [--tags tag1,tag2] [--repo <id>]`
- `mem checkpoint --reason "<...>" --state-file <path>|--state-json <json> [--thread <id>] [--repo <id>]`
- `mem ingest-artifact <path> --thread <id> [--repo <id>]`
- `mem embed [--kind memory|chunk|all] [--repo <id>]`
- `mem repos`
- `mem use <repo_id|path>`
- `mem threads [--repo <id>]`
- `mem thread <thread_id> [--limit 20] [--repo <id>]`
- `mem mcp [--repo <id|path>] [--allow-write] [--write-mode ask|auto|off] [--debug]`
- `mem --version` / `mem version`
- `mem doctor [--repo <id|path>] [--json] [--repair] [--verbose]`

## Storage and indexing

- Schema: `internal/store/schema.sql` (repos, state, threads, memories, artifacts, chunks, embeddings, links, meta).
- Added columns for text indexing, token counts, and soft deletion:

  - `memories.tags_text`, `memories.entities_text`
  - `memories.summary_tokens`
  - `chunks.text_hash`, `chunks.text_tokens`, `chunks.tags_json`, `chunks.tags_text`, `chunks.deleted_at`

- FTS5 virtual tables: `memories_fts` and `chunks_fts`.
- Unique index: `chunks(repo_id, locator, text_hash, thread_id)`.
- FTS sync triggers in `internal/store/triggers.sql` for insert/update/delete.
- FTS queries sanitized (`SanitizeQuery`) to prevent syntax errors (e.g. hyphens).
- Migrations via `PRAGMA user_version` and an FTS rebuild when upgrading.
- DB settings: WAL mode, `synchronous=NORMAL`, and `busy_timeout=3000`.
- Repo DB path from config: `~/.local/share/mempack/repos/<repo_id>/memory.db`.

## Ranking and budgets

- FTS5 bm25 weights (memories: title 5, summary 3, tags 2, entities 2; chunks: locator 1, text 3, tags 2).
- Recency bonus: `0.15 * exp(-days/14)`.
- Thread bonus: `+0.10` for matched thread.
- Orphan filtering uses git ancestry checks only on the top 200 memory candidates (configurable via code).
- Budget defaults: 2500 total, state 600, memories 10x80, chunks 4x320.
- Hard cap enforcement drops lowest-ranked items until within budget.

## Dependencies

- `github.com/BurntSushi/toml` for config parsing.
- `modernc.org/sqlite` for embedded SQLite.
- `github.com/pkoukk/tiktoken-go` for token counting (`cl100k_base`).
- `github.com/sabhiram/go-gitignore` for `.gitignore`/`.mempackignore` handling.
- `github.com/mark3labs/mcp-go` for MCP server implementation.

## Tests and build status

- Tests: `go test ./...`
- Build: `go build ./cmd/mem`
  - Perf (scripts/full_test.sh, 2026-01-22):
  - Fixed overhead: **~8ms** (repo_detect ~6.58ms, tokenizer 0ms).
  - End-to-end retrieval (1k memories + 6.6k chunks, db ~13.9MB): **p50 27.18ms / p95 27.63ms**.
  - MCP handler benchmarks (no stdio, Apple M4): get_context ~7.43ms/op, explain ~7.69ms/op.
  - MCP stdio benchmark (Apple M4): get_context p50 13.04ms / p95 13.89ms (20 runs).
  - Test 1 (200 docs): p50 14.56ms, p95 16.56ms, cold 16.68ms.
  - Test 2: Reachability filter passing correctly (orphan check works, anchor commit uses SHA).

## Deep Test Report (2026-01-22)

Repo: `wordtolatex-server` (user local), MCP configured via `~/.codex/config.toml` with write tools enabled.

Observations:
- Retrieval is phrase-sensitive: exact title/summary substrings work; re-phrased or misspelled queries often miss. Use `mem explain` to debug.
- MCP `get_context` mirrors the same behavior; narrow queries return the created memory/state, broader requests do not.
- CLI parsing: `mem thread --limit 20 <thread_id>` works; `mem thread <thread_id> --limit 20` fails (flags must precede args).
- Ingested chunks are retrievable by exact text (e.g., `AGENTS.md` title).

## v0.2.0 Release Checklist

- `mem --version` prints semver and commit SHA (e.g., `mempack v0.2.0 (abcdef0)`).
- `mem init` creates:
  - `.mempack/MEMORY.md`
  - `AGENTS.md` (thin stub)
- `AGENTS.md` is minimal (MCP-first + fallback only).
- `mem get --format prompt` is stable (no debug fields).
- `mem mcp` starts without config (uses active repo or cwd repo).
- `--allow-write` enables MCP write tools; write-mode defaults to `ask` and requires `confirmed=true`.
- `NO_COLOR` respected for CLI output.

## MCP Plan (v0.2+)

### Step 1: Read-only MCP (fast + safe)
Expose only:
- `mempack.get_context(query, repo?, workspace?, format?, budget?)`
- `mempack.explain(query, repo?)`

Rationale: enables implicit retrieval with minimal risk.

### Step 2: Write MCP with guardrails
Expose (gated by `mem mcp --allow-write`):
- `mempack.add_memory(thread, title, summary, tags?, confirmed?)`
- `mempack.checkpoint(reason, state_json?, thread?, confirmed?)`

Guardrails:
- Default to read-only unless repo opts in via `.mempack/MEMORY.md`, or
- Require a `--allow-write` flag at MCP server start (write-mode defaults to `ask`).

### Minimal MCP server spec
- One command: `mempack mcp` (runs local MCP server)
- Tools: `mempack.get_context`, `mempack.explain`
- Write tools gated by `--allow-write` with write-mode `ask` by default:
  - `mempack.add_memory`, `mempack.checkpoint`
- Startup health check prints a single-line status (`repo`, `db`, `schema`, `fts`, tools) and fails fast on missing setup (use `--debug` for JSON report).
- No hosting, no auth server, no accounts.

### MCP tool behavior (v0.2)
- `get_context` returns prompt text plus structured context pack JSON (and JSON text fallback).
- `explain` returns a compact text summary plus full JSON report (structured content).
- `add_memory` returns a short confirmation text plus a structured payload (id, thread, title, created_at).
- `checkpoint` returns a short confirmation text plus a structured payload (state_id, workspace, reason, memory_id?).

## Distribution Plan (Minimum Viable)

- GitHub Releases + checksums (`checksums.txt`).
- `scripts/install.sh` downloads the right binary by OS/arch.
- Later: brew/scoop.
- Consider GoReleaser to automate builds and release assets.

## Roadmap

- HTTP MCP mode (bridge or native) for clients that don't support stdio.

### Why MCP
- MCP is an emerging standard for agent tooling.
- "Built an MCP server for repo-scoped memory + retrieval" is a strong, relevant project story.
