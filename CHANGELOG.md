# Changelog

## v0.2.0

- MCP server (`mem mcp`) exposing `mempack.get_context` and `mempack.explain`; write tools gated by `--allow-write`.
- Agent onboarding: `mem init` writes `.mempack/MEMORY.md` plus a thin `AGENTS.md` stub (use `--no-agents` to skip).
- Performance: **p50 27ms / p95 28ms** on 1k memories + 6.6k chunks (~14MB DB).
- Repo detection caching + lazy tokenizer (0ms init on warm runs).
- Dedupe + migrations + deterministic output tests (CLI + MCP).
- Installer + README guidance for implicit memory and MCP usage.
- `mem --version` / `mem version` outputs semver + commit SHA.
