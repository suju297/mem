# Memory Testing Process (Sandbox + Benchmarks)

This process gives a repeatable way to test Mempack memory behavior in an isolated sandbox repo, then scale to benchmark-style evaluation.

## 1) Sandbox isolation

Default sandbox paths (all git-ignored):
- `sandbox/memory-eval/repo`
- `sandbox/memory-eval/data`
- `sandbox/memory-eval/evidence`

The sandbox uses a dedicated `MEMPACK_DATA_DIR`, so test writes do not pollute your normal memory store.
This same `MEMPACK_DATA_DIR` is used by both CLI and MCP suites as the single data-root source of truth.
The harness also isolates `XDG_CONFIG_HOME`/`XDG_CACHE_HOME` under sandbox paths so repo-cache state from your global setup cannot leak into tests.

## 2) Tiered evaluation strategy

### Tier 0: deterministic memory invariants (CLI)

Run:

```bash
scripts/memory_sandbox_eval.sh cli
```

What it validates:
- add memory -> retrievable by token
- link memories -> relation appears in `link_trail`
- update memory -> old retrieval token stops matching, new token matches

Output:
- JSON step artifacts and `cli_report.txt` in `sandbox/memory-eval/evidence/<run_id>/`

### Tier 1: MCP write/read parity

Run:

```bash
scripts/memory_sandbox_eval.sh mcp
```

What it validates:
- MCP server startup and tool registration
- read flow (`mempack_get_context`, `mempack_explain`)
- write flow (`mempack_add_memory`, `mempack_update_memory`, `mempack_link_memories`, `mempack_checkpoint`)

Output:
- `mcp_e2e.log` in `sandbox/memory-eval/evidence/<run_id>/`

### Tier 2: benchmark-backed memory quality

Use external datasets/methods to measure quality beyond simple invariants.

Recommended starting points:
- LongMemEval (ICLR 2025): long-context memory benchmark with 500 evaluation instances and support for exact-match/F1 plus LLM-as-a-judge scoring.
  - Paper: [LongMemEval on arXiv](https://arxiv.org/abs/2410.10813)
  - Code/data: [LongMemEval GitHub](https://github.com/xiaowu0162/LongMemEval)
  - Dataset card: [LongMemEval on Hugging Face](https://huggingface.co/datasets/xiaowu0162/LongMemEval)
- LoCoMo (ACL 2024): long-term conversational memory benchmark with category-level analysis (single-hop, temporal, multi-hop, open-domain).
  - Paper: [LoCoMo on arXiv](https://arxiv.org/abs/2402.17753)
  - Dataset/tooling: [LoCoMo GitHub](https://github.com/snap-research/locomo)
- MemoryAgentBench: incremental multi-turn memory benchmark focused on four competencies (accurate retrieval, test-time learning, long-range understanding, conflict resolution).
  - Paper: [Evaluating Memory in LLM Agents via Incremental Multi-Turn Interactions](https://arxiv.org/abs/2507.05257)
  - Code: [MemoryAgentBench GitHub](https://github.com/HUST-AI-HYZ/MemoryAgentBench)

## 3) Pass/fail gates

Minimum bar for a release candidate:
- Tier 0: all deterministic assertions pass
- Tier 1: MCP e2e run passes without tool/confirmation regressions
- Tier 2: no regression versus your last baseline on at least one benchmark split

Suggested tracked metrics:
- retrieval hit rate at top-k (`top_memories` contains expected id)
- answer quality (EM/F1 or benchmark-native score)
- consistency after updates (stale token miss + new token hit)
- memory-link utility (link trail present where expected)
- latency distribution (p50/p95 for `get_context`)

## 4) Running everything in one command

```bash
scripts/memory_sandbox_eval.sh all
```

## 5) Optional: put sandbox outside this repo

If you want sandbox data outside the repo:

```bash
MEMORY_SANDBOX_ROOT="$HOME/mempack-sandbox" scripts/memory_sandbox_eval.sh all
```

## 6) Complete external sandbox scoring run (2026-02-18)

Requested paths:
- main repo: `/Users/sujendragharat/Library/CloudStorage/GoogleDrive-sgharat298@gmail.com/My Drive/MacExternalCloud/Documents/Projects/memory`
- sandbox root: `/Users/sujendragharat/mempack-sandbox`
- sandbox repo: `/Users/sujendragharat/mempack-sandbox/repo`
- mem binary: `/Users/sujendragharat/go/bin/mem`

Command:

```bash
cd "/Users/sujendragharat/Library/CloudStorage/GoogleDrive-sgharat298@gmail.com/My Drive/MacExternalCloud/Documents/Projects/memory"
MEM_BIN=/Users/sujendragharat/go/bin/mem MEMORY_SANDBOX_ROOT="/Users/sujendragharat/mempack-sandbox" scripts/memory_sandbox_eval.sh all
```

Output:

```text
Sandbox repo ready: /Users/sujendragharat/mempack-sandbox/repo
Sandbox data dir:   /Users/sujendragharat/mempack-sandbox/data
Sandbox config dir: /Users/sujendragharat/mempack-sandbox/xdg/config
Evidence root:      /Users/sujendragharat/mempack-sandbox/evidence
CLI suite PASS
CLI evidence: /Users/sujendragharat/mempack-sandbox/evidence/20260218T221215Z
MCP suite PASS
MCP evidence: /Users/sujendragharat/mempack-sandbox/evidence/20260218T221215Z/mcp_e2e.log
```

Command:

```bash
cd "/Users/sujendragharat/mempack-sandbox/repo"
MEMPACK_DATA_DIR="/Users/sujendragharat/mempack-sandbox/data" XDG_CONFIG_HOME="/Users/sujendragharat/mempack-sandbox/xdg/config" XDG_CACHE_HOME="/Users/sujendragharat/mempack-sandbox/xdg/cache" /Users/sujendragharat/go/bin/mem get "sbx-gamma" --format json
```

Key retrieval checks from output:
- `repo.git_root` = `/Users/sujendragharat/mempack-sandbox/repo`
- query = `sbx-gamma`
- newest linked memories for run `20260218T221215Z` are present:
  - `M-20260218-221216-eb63b480` (Sandbox A, gamma token)
  - `M-20260218-221216-137790f7` (Sandbox B)

Evidence validation command:

```bash
LATEST=$(ls -1 /Users/sujendragharat/mempack-sandbox/evidence | sort | tail -n 1); echo "$LATEST"; echo '---'; sed -n '1,200p' "/Users/sujendragharat/mempack-sandbox/evidence/$LATEST/cli_report.txt"; echo '---'; sed -n '1,80p' "/Users/sujendragharat/mempack-sandbox/evidence/$LATEST/mcp_e2e.log"
```

Output:

```text
20260218T221215Z
---
status: PASS
run_id: 20260218T221215Z
thread: T-SANDBOX-EVAL
sandbox_repo: /Users/sujendragharat/mempack-sandbox/repo
sandbox_data_dir: /Users/sujendragharat/mempack-sandbox/data
memory_a: M-20260218-221216-eb63b480
memory_b: M-20260218-221216-137790f7
assertions:
- add memory A/B succeeded
- link relation persisted and appears in retrieval link_trail
- update replaced token and retrieval switched from old token to new token
---
mcp e2e: ok
```

Final score summary:

```json
{
  "reliability_score": 100,
  "isolation_check": "PASS",
  "data_root": "/Users/sujendragharat/mempack-sandbox/data",
  "latest_evidence_dir": "/Users/sujendragharat/mempack-sandbox/evidence/20260218T221215Z",
  "final_status": "PASS",
  "checks": {
    "cli_status": "PASS",
    "mcp_status": "PASS",
    "repo_git_root": "/Users/sujendragharat/mempack-sandbox/repo"
  }
}
```
