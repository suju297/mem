# Mempack CLI

## Usage

```text
mem [--data-dir <path>] <command> [options]
mem <command> --help
mem --version
```

## Global Option

- `--data-dir <path>`: overrides data root (`MEMPACK_DATA_DIR` / config).

## Command Catalog

| Group | Commands |
|---|---|
| Setup | `init`, `doctor`, `repos`, `use`, `version` |
| Retrieval | `get`, `explain`, `show`, `threads`, `thread`, `recent`, `sessions` |
| Writes | `add`, `update`, `supersede`, `link`, `checkpoint`, `forget` |
| Ingest/Embed | `ingest`, `ingest-artifact`, `embed` |
| Session/Share | `session upsert`, `share export`, `share import` |
| MCP | `mcp`, `mcp start`, `mcp stop`, `mcp status`, `mcp manager`, `mcp manager status` |
| Templates | `template` |

## Detailed Syntax

### Setup

```text
mem [--data-dir <path>] init [--no-agents] [--assistants agents|claude|gemini|all]
mem [--data-dir <path>] doctor [--repo <id|path>] [--json] [--repair] [--verbose]
mem [--data-dir <path>] repos [--format table|json] [--full-paths]
mem [--data-dir <path>] use <repo_id|path>
mem version | mem --version | mem -v
```

### Retrieval

```text
mem [--data-dir <path>] get <query> [--workspace <name>] [--include-orphans] [--cluster] [--repo <id|path>] [--debug]
mem [--data-dir <path>] explain <query> [--workspace <name>] [--include-orphans] [--repo <id|path>]
mem [--data-dir <path>] show <id> [--format json] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] threads [--workspace <name>] [--repo <id|path>] [--format json]
mem [--data-dir <path>] thread <thread_id> [--limit <n>] [--workspace <name>] [--repo <id|path>] [--format json]
mem [--data-dir <path>] recent [--limit <n>] [--workspace <name>] [--repo <id|path>] [--format json]
mem [--data-dir <path>] sessions [--needs-summary] [--count] [--limit <n>] [--workspace <name>] [--repo <id|path>] [--format json]
```

### Writes

```text
mem [--data-dir <path>] add --title <title> --summary <summary> [--thread <id>] [--tags <csv>] [--entities <csv>] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] add <title> [summary] [--thread <id>] [--tags <csv>] [--entities <csv>] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] update <id> [--title <title>] [--summary <summary>] [--tags <csv>] [--tags-add <csv>] [--tags-remove <csv>] [--entities <csv>] [--entities-add <csv>] [--entities-remove <csv>] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] supersede <id> --title <title> --summary <summary> [--thread <id>] [--tags <csv>] [--entities <csv>] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] supersede <id> [title] [summary] [--thread <id>] [--tags <csv>] [--entities <csv>] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] link --from <id> --rel <relation> --to <id> [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] link <from_id> <relation> <to_id> [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] checkpoint --reason <text> (--state-file <path> | --state-json <json>) [--thread <id>] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] checkpoint <reason> [state_json] [--state-file <path>] [--thread <id>] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] forget <id> [--workspace <name>] [--repo <id|path>]
```

### Ingest and Embeddings

```text
mem [--data-dir <path>] ingest <path> --thread <id> [--watch] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] ingest-artifact <path> --thread <id> [--watch] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] embed [--kind memory|chunk|all] [--workspace <name>] [--repo <id|path>]
mem [--data-dir <path>] embed status [--workspace <name>] [--repo <id|path>]
```

### Session and Sharing

```text
mem [--data-dir <path>] session upsert --title <title> [--summary <summary>] [--thread <id>] [--tags <csv>] [--entities <csv>] [--merge-window-ms <n>] [--min-gap-ms <n>] [--workspace <name>] [--repo <id|path>] [--format json]
mem [--data-dir <path>] session upsert <title> [summary] [--thread <id>] [--tags <csv>] [--entities <csv>] [--merge-window-ms <n>] [--min-gap-ms <n>] [--workspace <name>] [--repo <id|path>] [--format json]
mem [--data-dir <path>] share export [--repo <id|path>] [--workspace <name>] [--out <dir>]
mem [--data-dir <path>] share import [--repo <id|path>] [--workspace <name>] [--in <dir>] [--replace]
```

Notes:
- If `source_repo_id` differs from the current repo, interactive terminals prompt for confirmation.
- For non-interactive runs, pipe an answer (`printf 'yes\n' | mem share import`).

### MCP

```text
mem [--data-dir <path>] mcp [--stdio] [--repo <id|path>] [--require-repo] [--allow-write] [--write-mode ask|auto|off] [--debug] [--repair]
mem [--data-dir <path>] mcp start [--repo <id|path>] [--require-repo] [--allow-write] [--write-mode ask|auto|off] [--debug] [--repair]
mem [--data-dir <path>] mcp stop [--repo <id|path>] [--require-repo] [--allow-write] [--write-mode ask|auto|off] [--debug] [--repair]
mem [--data-dir <path>] mcp status [--repo <id|path>] [--require-repo] [--allow-write] [--write-mode ask|auto|off] [--debug] [--repair]
mem [--data-dir <path>] mcp manager [--port <n>] [--token <token>] [--idle-seconds <n>]
mem [--data-dir <path>] mcp manager status [--json]
```

Notes:
- `mem mcp` is raw stdio mode for MCP clients.
- For manual terminal control, use `mem mcp start`, `mem mcp status`, and `mem mcp stop`.

### Templates

```text
mem [--data-dir <path>] template [agents] [--write] [--assistants agents|claude|gemini|all] [--memory|--no-memory]
```

## Notes

- Use `mem --help` for the latest command list.
- Use `mem <command> --help` for command-specific flags and examples.
