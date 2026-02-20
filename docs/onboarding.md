# Mempack

Repo-scoped memory for coding agents, stored locally.

Why it exists:
- Keeps context inside the repo instead of a generic conversation log.
- Stores data on your machine (SQLite), not a remote service.

Who it’s for:
- Developers using coding agents in real repos.
- Teams who want repeatable, repo-specific context.

Who it’s not for:
- People who want cloud sync or multi-device sharing.
- Use cases without a Git repo or local filesystem access.

---

## Quick Start (MCP first, under 5 minutes)

```bash
# Install
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/<release>/scripts/install.sh | sh -s -- --repo <owner>/<repo> --version <release>

# Initialize in your repo
mem init

# Connect Codex to MCP
codex mcp add mempack -- mem mcp --require-repo
codex mcp list

# Start the MCP server
mem mcp

# Minimal example (seed one memory)
mem add --title "Auth plan" --summary "Use middleware; invalid token returns 401."
```

---

## Mental Model

How Mempack works:
- State: current truth you want the agent to remember.
- Memories: short decisions or summaries.
- Evidence: ingested code/text chunks from your repo.

Important invariants:
- Everything is repo-scoped and stored locally (SQLite).
- Retrieval is deterministic; ties break by recency.
- Rewrites only apply when a query returns zero hits.
- State is always returned, even when nothing matches.

---

## Core Usage (90% of use cases)

Common tasks:
- Use MCP with your agent
- Add a memory
- Get context (CLI)
- Ingest code

Task: Use MCP with your agent
Command:
```
codex mcp add mempack -- mem mcp --require-repo
mem mcp
```
What happens:
- The agent can call `mempack_get_context` automatically.
Common mistakes:
- Running MCP globally without `--require-repo`.
- Forgetting to start the MCP server.

Task: Add a memory
Command:
```
mem add --title "Auth plan" --summary "Use middleware; invalid token returns 401."
```
What happens:
- A short memory is saved under the default thread.
- It becomes retrievable via `mem get`.
Common mistakes:
- Using long titles (harder to search).
- Writing multi-paragraph summaries (harder to scan).

Task: Get context
Command:
```
mem get "auth plan" --format prompt
```
What happens:
- Retrieves state, plus top memories and evidence.
Common mistakes:
- Vague queries with no matching words.
- Expecting rewrites when results already exist.

Task: Ingest code
Command:
```
mem ingest-artifact ./internal --thread T-dev --watch
```
What happens:
- Indexes files as evidence chunks.
- Keeps them up to date while you edit.
Common mistakes:
- Forgetting `--watch` during active work.
- Pointing to huge folders you don’t need.

---

## Configuration & Options (reference)

`mcp_require_repo`
- Type: boolean
- Default: false
- Description: Fails if the repo cannot be resolved; prevents wrong-repo reads.
- When to change it: Set to `true` if you use multiple repos.

`mcp_allow_write`
- Type: boolean
- Default: true
- Description: Enables write tools for MCP.
- When to change it: Turn off if you do not want the agent to save memories.

`mcp_write_mode`
- Type: string (`ask` | `auto` | `off`)
- Default: `ask`
- Description: Controls whether write tools require approval.
- When to change it: Use `ask` for safety, `auto` for trusted agents.

`default_workspace`
- Type: string
- Default: `default`
- Description: Isolates memories per workspace name.
- When to change it: Use different workspaces for separate projects or contexts.

`default_thread`
- Type: string
- Default: `T-SESSION`
- Description: Thread used when `--thread` is omitted.
- When to change it: Set a project- or team-specific default.

`embedding_provider`
- Type: string
- Default: `auto`
- Description: Enables embeddings when available, falls back to keyword search.
- When to change it: Set explicitly if you run a specific embedding stack.

`embedding_model`
- Type: string
- Default: `nomic-embed-text`
- Description: Model used for vector embeddings.
- When to change it: If you want a different model or performance tradeoff.

`embedding_min_similarity`
- Type: float
- Default: 0.6
- Description: Drops low-similarity vector-only matches.
- When to change it: Increase to reduce noise, lower to increase recall.

---

## Error Handling & Debugging

Common errors

Error: Empty results
Cause: Query words don’t appear in memories or evidence.
Fix: Use exact words from titles/summaries; run `mem explain "<query>"`.

Error: Wrong repo results
Cause: MCP couldn’t resolve the repo and fell back to a different one.
Fix: Start MCP with `--require-repo`; then use current repo (cwd) or pass `repo=<path>` when needed.

Error: Embeddings unavailable
Cause: Embedder not installed or not running.
Fix: Run `mem embed status` for the specific fix.

Debugging tips:
- Use `mem explain "<query>"` to see rewrites and fallbacks.
- Check `search_meta` in JSON output for warnings.

---

## Advanced Usage

- Disable write tools: `mem mcp --write-mode off`
- Require repo for MCP: `mem mcp --require-repo`
- Include orphaned memories: `mem get "<query>" --include-orphans`
- Cluster results: `mem get "<query>" --cluster` (requires embeddings)
- Separate contexts: use named workspaces (`--workspace <name>`)

---

## Design Decisions

Why it’s built this way:
- Local-first storage keeps data under your control.
- Repo-scoped memory avoids cross-project contamination.
- Keyword search is the default for predictability.
- A hard token budget prevents runaway prompts.

---

## FAQ

Q: Do I need embeddings?
A: No. Keyword search works by default. Embeddings are optional.

Q: Why am I seeing context from the wrong repo?
A: Start MCP with `--require-repo`; then use current repo (cwd) or pass `repo=<path>` when needed.

Q: Can I keep multiple contexts?
A: Yes. Use named workspaces and switch with `--workspace <name>`.

Q: How do I verify what Mempack is returning?
A: Use `mem explain "<query>"` and check `search_meta`.

---

## Contribution & Support

- Start from `ReadMe.md` for the high-level command/reference map.
- See `../ARCHITECTURE.md` for runtime/data-flow internals.
- Use `docs/memory-testing-process.md` for reproducible sandbox validation.
