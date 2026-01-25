# Mempack: Repo-Scoped Memory for Coding Agents

**Mempack** is a high-performance CLI tool that gives AI coding agents persistent, repo-scoped memory. It stores context (summaries), evidence (code chunks), and rules in a local embedded database, retrievable via semantic search (FTS + BM25) and git reachability filtering.

## Performance
- **Fixed Overhead**: **~10-15ms**.
  (Repo detection, tokenizer lazy-init, config load, DB open).
- **End-to-End Latency**:
  - ~37ms (p50) / ~45ms (p95) on 1k memories + 6.6k chunks.
  - Scaleable via FTS5 and hard token budgets.

## Installation

### Prebuilt (recommended)

```bash
# Download and install from GitHub Releases
curl -fsSL https://raw.githubusercontent.com/<owner>/<repo>/main/scripts/install.sh | sh -s -- --repo <owner>/<repo>

# Or, if you already cloned the repo:
./scripts/install.sh --repo <owner>/<repo>

# Pin a version:
./scripts/install.sh --repo <owner>/<repo> --version v0.2.0
```

Release assets are expected to be named like:
`mempack_<os>_<arch>.tar.gz` and contain the `mem` binary.
Default install dir is `~/.local/bin` (override with `--install-dir`).

### From source

```bash
# Build binary as 'mempack'
go build -o mempack ./cmd/mem

# Recommended: alias as 'mem' for convenience
alias mem='./mempack'
```

Optional (Go install, with version pinning):

```bash
go install github.com/<owner>/<repo>/cmd/mem@v0.2.0
```

## Quick Start

```bash
# Initialize memory in your current repo
mempack init

# Add a memory
mempack add --thread T-123 --title "Auth Refactor" --summary "Moving auth logic to middleware"

# Retrieve context for a query and output as prompt
mempack get "auth middleware" --format prompt
```

## Implicit memory via MCP

Start the local MCP server:

```bash
mem mcp
```

In an MCP-capable client, enable these tools:
- `mempack.get_context("<task>")` at task start
- `mempack.explain("<query>")` when debugging retrieval

No hosting, local-only, read-only in v0.2.
MCP-capable clients can connect via stdio.

## Features
- **Git Awareness**: Automatically filters out "orphaned" memories from unreachable commits.
- **Artifact Ingestion**: ingest code files with `ingest-artifact`.
- **Token Budgeting**: Strict hard cap on output tokens (default 2500).
- **Security**:
  - Chunks with adversarial phrases (e.g. "ignore previous instructions") are automatically downranked.
  - Evidence output is clearly labeled as data.

## Commands
- `init`: Setup config and repo.
- `add`: Store a memory.
- `get`: Retrieve context (JSON or Prompt format).
- `ingest-artifact`: Index chunks from files.
- `show`, `forget`, `supersede`, `checkpoint`, `repos`, `use`.
