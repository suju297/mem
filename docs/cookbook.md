# Mem Cookbook

Use this page when you already know the task you want to complete and want a copy-pasteable workflow.

Choose the right doc surface:
- Start from `docs/onboarding.md` if this is your first run.
- Use this page for common workflows and recovery notes.
- Use `docs/scripting.md` for CI, shell scripts, and machine-readable output.
- Use `docs/cli.md` when you need exact command syntax.

Each workflow includes:
- the exact command
- what it does
- a sample output
- common variations
- the first recovery step when it fails

## 1. Initialize Memory in a Repo

Use this when:
- you are turning on Mem for a repo for the first time
- you want to confirm the local database and repo detection are healthy

Commands:

```bash
mem init
mem doctor --json
```

What it does:
- `mem init` creates repo-scoped storage, seeds a welcome memory, and sets the active repo.
- By default, `mem init` writes the repo memory instructions plus `AGENTS.md` when those files are missing.
- `mem doctor --json` verifies repo detection, the SQLite database, schema version, and FTS tables.
- Use `mem init --agents`, `mem init --claude`, `mem init --gemini`, or `mem init --all` to choose which assistant stub files are written.

Sample output:

```text
Initialized memory for repo: p_8da4cb02
Root: /path/to/repo

Try these commands:
  mem add --title "My Feature" --summary "Planning the API"
  mem get "planning"
  mem repos
```

```json
{
  "ok": true,
  "repo": {
    "id": "p_8da4cb02",
    "source": "cache_cwd",
    "has_git": true
  },
  "db": {
    "exists": true
  },
  "schema": {
    "user_version": 10
  }
}
```

Common variations:
- Skip assistant stubs: `mem init --no-agents`
- Write only `AGENTS.md`: `mem init --agents`
- Write only `CLAUDE.md`: `mem init --claude`
- Write only `GEMINI.md`: `mem init --gemini`
- Write all supported stub files: `mem init --all`
- Attempt repairs while checking health: `mem doctor --repair`

If it fails:
- `repo detection error`: run inside a Git repo, or `cd` to the repo root first.
- `config error`: fix the syntax in `~/.config/mem/config.toml`.

## 2. Save a Decision and Retrieve It

Use this when:
- you want to store a short repo-specific decision
- you want to verify what the agent or CLI will see later

Commands:

```bash
mem add --title "Auth plan" --summary "Use middleware for token validation."
mem get "Auth plan"
mem get "Auth plan" --format prompt
```

What it does:
- `mem add` stores a short durable memory under the default thread.
- `mem get` returns the repo-scoped context pack as JSON by default.
- `mem get --format prompt` renders the same context in agent-friendly prompt form.

Sample output:

```json
{
  "id": "M-20260312-191557-2be03608",
  "thread_id": "T-SESSION",
  "thread_defaulted": true,
  "title": "Auth plan"
}
```

```json
{
  "workspace": "default",
  "top_memories": [
    {
      "title": "Auth plan",
      "summary": "Use middleware for token validation."
    }
  ],
  "rules": [
    "State is authoritative. Memories/chunks are supporting evidence."
  ]
}
```

```text
# Context from Memory (Repo: p_8da4cb02)

## Memories
- **Auth plan**: Use middleware for token validation.
```

Common variations:
- Put the memory in a specific thread: `mem add --thread T-auth --title "Auth plan" --summary "..."`
- Query a specific workspace: `mem get "Auth plan" --workspace review`
- Ask for orphaned memories too: `mem get "Auth plan" --include-orphans`

If it fails:
- `missing title` or `missing summary`: pass `--title` and `--summary` explicitly in scripts.
- Empty results: query the exact words from the title or summary, then use `mem explain "<query>"`.

## 3. Inspect Why a Result Ranked the Way It Did

Use this when:
- the retrieved memory is surprising
- you need to understand ranking, budget, or embedding state

Command:

```bash
mem explain "Auth plan"
```

What it does:
- shows how Mem ranked memories and chunks
- exposes BM25, vector, recency, and thread bonuses
- shows token budget and embedding status

Sample output:

```json
{
  "query": "Auth plan",
  "memories": [
    {
      "title": "Auth plan",
      "fts_rank": 1,
      "vector_score": 0,
      "recency_bonus": 0.1499985242420066,
      "included": true
    }
  ],
  "vector": {
    "enabled": true,
    "error": "no embeddings stored for model nomic-embed-text (run: mem embed)"
  }
}
```

Common variations:
- Include orphaned memories while debugging: `mem explain "Auth plan" --include-orphans`
- Scope to a workspace: `mem explain "Auth plan" --workspace review`

If it fails:
- No useful matches: ingest more evidence with `mem ingest-artifact <path> --thread <id>`.
- Embedding warnings: run `mem embed status` before assuming vector search is active.

## 4. Connect an Agent Through MCP

Use this when:
- you want Codex or another MCP client to read repo-scoped context automatically
- you want strict repo boundaries instead of `active_repo` fallback

Commands:

```bash
# Codex
codex mcp add mem -- mem mcp --require-repo

# Claude Code
claude mcp add --transport stdio mem -- mem mcp --require-repo

mem mcp
```

What it does:
- registers Mem as an MCP server for the client
- starts the raw stdio MCP server for an attached JSON-RPC client
- enforces explicit repo resolution with `--require-repo`

Sample output:

```text
mem mcp: repo=p_8da4cb02 db=/path/to/memory.db schema=v10 fts=ok tools=7 (write-mode=ask)
```

Common variations:
- Manage the local background daemon: `mem mcp start`, `mem mcp status`, `mem mcp stop`
- Disable write tools: `mem mcp --write-mode off`
- Allow writes with approval: `mem mcp --allow-write --write-mode ask`

If it fails:
- Interactive terminal warning: use `mem mcp start|status|stop` in a shell, or `mem mcp --stdio` only when you intentionally want raw stdio.
- Wrong repo results: keep `--require-repo` enabled and pass `repo=<path>` from the client when needed.

## 5. Ingest Code and Keep Evidence Fresh

Use this when:
- you want retrieval to include code or docs, not only short memories
- you are preparing a repo for repeated agent use

Commands:

```bash
mem ingest-artifact ./internal --thread T-dev
mem ingest-artifact ./internal --thread T-dev --watch
```

What it does:
- chunks files under the target path and stores them as evidence
- associates the chunks with the thread you choose
- `--watch` keeps the evidence current while files change

Sample output:

```json
{
  "files_ingested": 1,
  "chunks_added": 1,
  "files_skipped": 0
}
```

Common variations:
- Ingest docs instead of code: `mem ingest ./docs --thread T-docs`
- Check embedding coverage after ingest: `mem embed status`
- Backfill missing vectors: `mem embed --kind chunk`

If it fails:
- `missing --thread`: choose a thread name up front so future retrieval can benefit from thread matching.
- Large noisy folders: point at the smallest useful subtree instead of the whole repo.
