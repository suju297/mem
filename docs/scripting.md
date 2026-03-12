# Mem Scripting and Automation

Use this page when Mem is running inside shell scripts, CI jobs, editor integrations, or agent wrappers.

This page focuses on behavior that matters in automation:
- machine-readable output
- stdout vs stderr
- exit codes
- non-interactive behavior
- repo and config resolution

For first-time setup, use `docs/onboarding.md`.
For task-first examples, use `docs/cookbook.md`.
For exact command syntax, use `docs/cli.md`.

## Output Contracts

Prefer commands with JSON output when another tool will parse the result.

Common machine-readable commands:

| Command | Output |
|---|---|
| `mem get "<query>"` | JSON context pack by default |
| `mem get "<query>" --format prompt` | Prompt-formatted text |
| `mem explain "<query>"` | JSON ranking and budget report |
| `mem add --title "<title>" --summary "<summary>"` | JSON with id, thread, and timestamps |
| `mem update ...` | JSON summary of the updated memory |
| `mem show <id> --format json` | JSON memory record |
| `mem repos --format json` | JSON repo list |
| `mem threads --format json` | JSON thread list |
| `mem thread <thread_id> --format json` | JSON thread details |
| `mem recent --format json` | JSON recent memories |
| `mem sessions --format json` | JSON session list |
| `mem doctor --json` | JSON health report |
| `mem embed status` | JSON embedding coverage report |
| `mem ingest-artifact <path> --thread <id>` | JSON ingest counts |
| `mem session upsert ... --format json` | JSON create/update result |

Guidelines:
- Parse stdout, not stderr.
- Expect human-readable text from commands like `mem init`, `mem embed`, and `mem mcp status`.
- Treat JSON field additions as possible over time; avoid brittle exact-field matching when not needed.

## Stdout and Stderr

Mem follows a CLI-friendly split:
- stdout carries the primary result: JSON, prompt text, or normal human-readable output
- stderr carries errors, validation messages, prompts, and debug diagnostics

Examples:
- `mem get --debug` prints timing breakdowns to stderr.
- `mem mcp` prints startup/status lines to stderr before serving stdio.
- usage errors such as missing flags are written to stderr and return exit code `2`.

Automation rule:
- read stdout for the payload
- log stderr for diagnosis
- do not assume stderr is empty on success if you enabled debug output

## Exit Codes

The command handlers consistently follow this contract:
- `0`: success
- `1`: operational/runtime failure such as repo detection, store, config, or provider errors
- `2`: invalid usage, missing required arguments, or unsupported flags/values

Shell example:

```bash
if ! mem doctor --json >/tmp/mem-doctor.json; then
  echo "mem doctor failed" >&2
  exit 1
fi
```

## Non-Interactive Behavior

Some write commands prompt in an interactive terminal if required arguments are missing:
- `mem add`
- `mem supersede`
- `mem checkpoint`
- `mem link`
- `mem session upsert`

Script rule:
- always pass required flags and positional arguments explicitly
- do not rely on prompts in CI or background jobs

Example:

```bash
mem add --title "Release note" --summary "Search behavior now uses repo-scoped ranking."
```

Share import has an explicit non-interactive path:

```bash
printf 'yes\n' | mem share import
```

`mem mcp` is also special:
- it expects an attached JSON-RPC client on stdin/stdout
- in a normal interactive shell, use `mem mcp start|status|stop`
- use `mem mcp --stdio` only when you intentionally want raw stdio mode in a terminal

## Repo Resolution and Scope

Repo resolution order is:
1. explicit repo argument such as `--repo <id|path>` or MCP `repo`
2. Git root from the current working directory
3. `active_repo` fallback

When `--require-repo` is enabled for MCP, the `active_repo` fallback is disabled.

Automation guidance:
- prefer `--repo <path>` when a script may run outside the target repo root
- enable `--require-repo` for MCP clients that move between multiple repos
- use `--workspace <name>` when one repo needs separate memory contexts

Example:

```bash
mem get "release checklist" --repo /path/to/repo --workspace ci
```

## Configuration Precedence

Data directory precedence is:
1. `--data-dir <path>`
2. `MEM_DATA_DIR`
3. `data_dir` in `~/.config/mem/config.toml`

Repo-scoped overrides from `.mem/config.json` apply after the global config is loaded for supported settings such as:
- `mcp_allow_write`
- `mcp_write_mode`
- `embedding_provider`
- `embedding_model`
- `token_budget`
- `default_thread`

Practical rule:
- use `--data-dir` for tests and throwaway runs
- use `MEM_DATA_DIR` for a shell session or CI job
- use config files for stable defaults

## Shell Notes

Quote free-text queries:

```bash
mem get "auth middleware"
mem explain "why is auth middleware not ranked first"
```

Use `jq` when you only need a few fields:

```bash
mem get "auth plan" | jq -r '.top_memories[]?.title'
```

Prefer repo-relative automation when the script already runs in the repo root:

```bash
cd /path/to/repo
mem doctor --json | jq '.ok'
```

Use explicit repo arguments when the caller may run elsewhere:

```bash
mem doctor --json --repo /path/to/repo
```

## Recommended Automation Patterns

Health gate before work:

```bash
mem doctor --json | jq -e '.ok == true' >/dev/null
```

Fetch a prompt pack for another tool:

```bash
mem get "release notes" --format prompt > /tmp/mem-context.txt
```

Capture only the memory ids from a search:

```bash
mem get "auth plan" | jq -r '.top_memories[]?.id'
```

Inspect why retrieval changed:

```bash
mem explain "auth plan" > /tmp/mem-explain.json
```

Backfill embeddings after ingest:

```bash
mem ingest-artifact ./internal --thread T-dev
mem embed --kind chunk
mem embed status | jq '.chunk'
```

## Failure Recovery Checklist

If a script starts failing:
- run `mem doctor --json` first
- confirm the repo scope with `--repo <path>`
- inspect stderr for validation or provider errors
- use `mem explain "<query>"` before changing ranking assumptions
- use `mem embed status` before assuming vector search is active
