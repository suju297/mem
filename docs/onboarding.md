# Mem

Repo-scoped memory for coding agents, stored locally.

This page is the shortest path to first success.

Use the rest of the docs like this:
- stay here for installation, setup, and first troubleshooting
- move to `docs/cookbook.md` for common workflows and copy-pasteable examples
- move to `docs/scripting.md` for CI, shell scripts, and machine-readable behavior
- move to `docs/cli.md` when you need exact syntax

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

1. Install `mem`:

Homebrew (macOS):

```bash
brew tap suju297/mem
brew install suju297/mem/mem-cli
```

The Homebrew formula is named `mem-cli`; the installed binary is still `mem`.

GitHub Releases installer (macOS/Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/suju297/mem/main/scripts/install.sh | sh -s -- --repo suju297/mem
```

Add PATH automatically during install (macOS/Linux):

```bash
curl -fsSL https://raw.githubusercontent.com/suju297/mem/main/scripts/install.sh | sh -s -- --repo suju297/mem --add-to-path
```

Windows (PowerShell):

```powershell
iwr https://raw.githubusercontent.com/suju297/mem/main/scripts/install.ps1 -OutFile $env:TEMP\\mem-install.ps1; & $env:TEMP\\mem-install.ps1 -Repo suju297/mem
```

Windows PATH behavior:
- By default, installer updates user PATH (`-AddToPath $true`).
- To skip PATH update: `-AddToPath $false`.

If release assets are unavailable, installers fall back to source build (Go toolchain required).

2. Initialize in your repo:

```bash
mem init
```

By default, `mem init` writes the repo memory instructions plus `AGENTS.md` when those files are missing.
Use `mem init --agents`, `mem init --claude`, `mem init --gemini`, or `mem init --all` when you want to choose which assistant stub files are created.
On the first interactive `mem init`, Mem also asks whether you want local embeddings. If you opt in, it can offer an Ollama install and then prompt for a recommended embedding model.
If you need to undo repo setup later, run `mem delete --yes`.

3. Connect an MCP client to Mem:

```bash
# Codex
codex mcp add mem -- mem mcp --require-repo
codex mcp list

# Claude Code
claude mcp add --transport stdio mem -- mem mcp --require-repo
```

4. Start MCP stdio:

```bash
mem mcp
```

If the host launches outside the repo, `--require-repo` still allows MCP startup; pass `repo=<workspace root>` on tool calls until the repo is resolved.

Background daemon lifecycle is separate:

```bash
mem mcp start
mem mcp status
mem mcp stop
```

`mem mcp manager` remains a separate control-plane process.

5. Seed one memory:

```bash
mem add --title "Auth plan" --summary "Use middleware; invalid token returns 401."
```

6. Pick the next doc surface:
- Common workflows: `docs/cookbook.md`
- Automation and CI: `docs/scripting.md`
- Full syntax reference: `docs/cli.md`

---

## Mental Model

How Mem works:
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
# Codex
codex mcp add mem -- mem mcp --require-repo

# Claude Code
claude mcp add --transport stdio mem -- mem mcp --require-repo

mem mcp
```
What happens:
- The agent can call `mem_get_context` automatically.
- Use `mem mcp start|status|stop` only for the local daemon lifecycle.
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
- Default: `none`
- Description: Keeps keyword search as the baseline. Set to `ollama` when you want local vector search.
- When to change it: Use the first interactive `mem init` prompt, or set it explicitly if you run a specific embedding stack.

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

Q: How do I verify what Mem is returning?
A: Use `mem explain "<query>"` and check `search_meta`.

---

## Contribution & Support

- Start from `ReadMe.md` for the high-level command/reference map.
- See `../ARCHITECTURE.md` for runtime/data-flow internals.
- Use `docs/memory-testing-process.md` for reproducible sandbox validation.
