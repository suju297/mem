Run a strict end-to-end validation for Mempack v0.2.20 (CLI + extension), including the `mem share export/import` flow.

Hard requirements:
- Do NOT change source code.
- Do NOT commit.
- Use MCP-first for writes wherever possible.
- For MCP write calls in ask mode, use `confirmed=true`.
- Pass `repo` explicitly on every MCP tool call.
- CLI may be used for assertions, read checks, and `mem share` commands.
- Save all evidence files and transcripts.
- Parse extension versions from installed extension `package.json` files, not from extension IDs/folder names.

Paths:
- Repo under test: /Users/sujendragharat/Library/CloudStorage/GoogleDrive-sgharat298@gmail.com/My Drive/MacExternalCloud/Documents/Projects/memory
- Temp repos:
  - A: /tmp/mempack-e2e-0220-A
  - B: /tmp/mempack-e2e-0220-B
- Data dir: /tmp/mempack-e2e-0220-data
- Evidence dir: /tmp/mempack-e2e-0220-evidence
- Final reports:
  - /tmp/mempack-e2e-0220-report.json
  - /tmp/mempack-e2e-0220-PASS_FAIL_REPORT.md

Scenarios (must-pass unless marked WARN):
S0 Preconditions (must-pass)
- Verify `mem --version` semver is `0.2.20` (ignore optional build suffix like `(dev)`).
- Verify installed VS Code and Cursor `mempack.mempack` extension versions by reading installed extension `package.json` files.
- Fail if CLI semver and target extension semver differ.

S1 Repo setup (must-pass)
- Clean/create repo A and B.
- `git init` both, leave both with no commits.
- Run `mem init --no-agents` in each repo.
- Verify `.mempack/config.json` exists in each repo.
- Treat `.mempack/MEMORY.md` as optional in `--no-agents` mode. If present, validate managed marker; if absent, record as expected.
- Run `mem doctor --json` twice per repo; assert stable repo_id + git_root.

S2 MCP strict scoping (must-pass)
- Start MCP stdio session with `--repo <A> --require-repo --write-mode ask`.
- Unscoped `mempack_get_context` must error with repo-required message.
- Scoped `mempack_get_context` must succeed and match repo A id/root.

S3 MCP ask-mode write gating (must-pass)
- `mempack_add_memory` without confirmed => must fail.
- Same call with `confirmed=true` => must succeed and return memory id.

S4 MCP update in place (must-pass)
- Add memory with old entity token.
- Update same memory via `mempack_update_memory` replacing entities.
- Assert ID unchanged.
- Assert old token query returns 0 hits in repo A.
- Assert new token query returns >=1 hit in repo A.

S5 Tags + summary semantics (must-pass)
- On same memory, test tags replace, remove, add.
- Update summary with unique token.
- Verify unique summary token is searchable in repo A.

S6 Repo isolation (must-pass)
- Write unique token memory in repo B via MCP (repo override).
- Query token in repo B => hit.
- Query same token in repo A => 0 hits.

S7 Share export/import/idempotency (must-pass)
- In repo A, ensure at least 2 active memories exist.
- Run `mem share export` (default folder `mempack-share`).
- In repo B:
  - `printf '\n' | mem share import --in <A/mempack-share>` => must fail (repo mismatch / canceled).
  - `printf 'yes\n' | mem share import --in <A/mempack-share>` => must import.
  - re-import same bundle => no duplicates; unchanged count should increase; imported should be 0.

S8 Share replace behavior (must-pass)
- In repo B create one local (non-shared) memory.
- Edit exported bundle (`memories.jsonl` + manifest memory_count) to keep only one shared record.
- Run `printf 'yes\n' | mem share import --replace`.
- Assert:
  - stale shared records removed,
  - kept shared record still present,
  - local non-shared repo B memory is still present.

S9 Extension runtime/default checks (must-pass)
- Verify extension defaults from installed package:
  - fastMcpEnabled=true
  - writeTransport=mcp_first
  - promptStartMcpServer=true
  - autoSessionsEnabled=false

S10 GUI-only checks (WARN allowed)
- If terminal-only runtime cannot observe GUI behavior, mark WARN with explicit blocker and manual checklist.

Output format requirements:
1) JSON report with:
- overall_status: PASS | PASS_WITH_WARNINGS | FAIL
- environment block
- scenarios[] with:
  - id, name, must_pass, status, observed_values, evidence_files, notes
- failed_must_pass_scenarios[]
- warn_scenarios[]

2) Markdown pass/fail report:
- clear scenario-by-scenario PASS/FAIL/WARN
- links/paths to evidence
- explicit summary of blockers and next actions

Decision rule:
- If any must-pass scenario fails => overall_status=FAIL.
- If all must-pass pass and any WARN exists => PASS_WITH_WARNINGS.
- Else PASS.
