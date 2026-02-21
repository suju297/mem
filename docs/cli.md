# Mempack CLI

## Usage

```text
mem [global-flags] <command> [args] [flags]
mem <command> --help
mem --version
```

## Global Flags

- `--data-dir <path>`

## Shared Flag Sets

- `[scope]` = `[--repo <id|path>] [--workspace <name>]`
- `[write-meta]` = `[--thread <id>] [--tags <csv>] [--entities <csv>]`
- `[json]` = `[--format json]`

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
mem init [--no-agents] [--assistants agents|claude|gemini|all]
mem doctor [--repo <id|path>] [--json] [--repair] [--verbose]
mem repos [--format table|json] [--full-paths]
mem use <repo_id|path>
mem version | mem --version | mem -v
```

### Retrieval

```text
mem get <query> [--include-orphans] [--cluster] [--debug] [scope]
mem explain <query> [--include-orphans] [scope]
mem show <id> [json] [scope]
mem threads [json] [scope]
mem thread <thread_id> [--limit <n>] [json] [scope]
mem recent [--limit <n>] [json] [scope]
mem sessions [--needs-summary] [--count] [--limit <n>] [json] [scope]
```

### Writes

```text
mem add <title> [summary] [write-meta] [scope]
mem add --title <title> --summary <summary> [write-meta] [scope]
mem update <id> [--title <title>] [--summary <summary>] [--tags <csv>|--tags-add <csv>|--tags-remove <csv>] [--entities <csv>|--entities-add <csv>|--entities-remove <csv>] [scope]
mem supersede <id> [title] [summary] [write-meta] [scope]
mem supersede <id> --title <title> --summary <summary> [write-meta] [scope]
mem link <from_id> <relation> <to_id> [scope]
mem link --from <id> --rel <relation> --to <id> [scope]
mem checkpoint <reason> [state_json] [--state-file <path>] [--thread <id>] [scope]
mem checkpoint --reason <text> (--state-file <path> | --state-json <json>) [--thread <id>] [scope]
mem forget <id> [scope]
```

### Ingest and Embeddings

```text
mem ingest <path> --thread <id> [--watch] [scope]
mem ingest-artifact <path> --thread <id> [--watch] [scope]
mem embed [--kind memory|chunk|all] [scope]
mem embed status [scope]
```

### Session and Sharing

```text
mem session upsert <title> [summary] [write-meta] [--merge-window-ms <n>] [--min-gap-ms <n>] [json] [scope]
mem session upsert --title <title> [--summary <summary>] [write-meta] [--merge-window-ms <n>] [--min-gap-ms <n>] [json] [scope]
mem share export [--out <dir>] [scope]
mem share import [--in <dir>] [--replace] [scope]
```

`share import` prompts for confirmation if `source_repo_id` differs from the current repo in interactive terminals.

### MCP

```text
mem mcp [--stdio] [mcp-runtime] [mcp-write]
mem mcp start [mcp-runtime] [mcp-write]
mem mcp stop
mem mcp status
mem mcp manager [--port <n>] [--token <token>] [--idle-seconds <n>]
mem mcp manager status [--json]
```

Where:
- `[mcp-runtime]` = `[--repo <id|path>] [--require-repo[=true|false]] [--debug] [--repair]`
- `[mcp-write]` = `[--allow-write] [--write-mode ask|auto|off]`

### Templates

```text
mem template [agents] [--write] [--assistants agents|claude|gemini|all] [--memory|--no-memory]
```

## Notes

- Use `mem --help` for the latest command list.
- Use `mem <command> --help` for command-specific flags and examples.
