# Mempack CLI

![Setup](https://img.shields.io/badge/Setup-Group-0EA5E9?style=flat-square) ![Retrieval](https://img.shields.io/badge/Retrieval-Group-4F46E5?style=flat-square) ![Writes](https://img.shields.io/badge/Writes-Group-10B981?style=flat-square) ![Ingest/Embed](https://img.shields.io/badge/Ingest%2FEmbed-Group-F59E0B?style=flat-square) ![Session/Share](https://img.shields.io/badge/Session%2FShare-Group-EC4899?style=flat-square) ![MCP](https://img.shields.io/badge/MCP-Group-EF4444?style=flat-square) ![Templates](https://img.shields.io/badge/Templates-Group-6B7280?style=flat-square)

## Usage

```text
mem [global-flags] <command> [args] [flags]
mem <command> --help
mem --version
```

## Global Flags ![Global](https://img.shields.io/badge/Global-8B5CF6?style=flat-square)

- `--data-dir <path>`

## Shared Flag Sets ![Shared](https://img.shields.io/badge/Shared-8B5CF6?style=flat-square)

- ![scope](https://img.shields.io/badge/scope-6366F1?style=flat-square) `[scope]` = `[--repo <id|path>] [--workspace <name>]`
- ![write-meta](https://img.shields.io/badge/write--meta-10B981?style=flat-square) `[write-meta]` = `[--thread <id>] [--tags <csv>] [--entities <csv>]`
- ![json](https://img.shields.io/badge/json-0EA5E9?style=flat-square) `[json]` = `[--format json]`

## Command Catalog

| Color | Group | Commands |
|---|---|---|
| ![Setup](https://img.shields.io/badge/-0EA5E9?style=flat-square) | Setup | `init`, `doctor`, `repos`, `use`, `version` |
| ![Retrieval](https://img.shields.io/badge/-4F46E5?style=flat-square) | Retrieval | `get`, `explain`, `show`, `threads`, `thread`, `recent`, `sessions` |
| ![Writes](https://img.shields.io/badge/-10B981?style=flat-square) | Writes | `add`, `update`, `supersede`, `link`, `checkpoint`, `forget` |
| ![Ingest](https://img.shields.io/badge/-F59E0B?style=flat-square) | Ingest/Embed | `ingest`, `ingest-artifact`, `embed` |
| ![Session](https://img.shields.io/badge/-EC4899?style=flat-square) | Session/Share | `session upsert`, `share export`, `share import` |
| ![MCP](https://img.shields.io/badge/-EF4444?style=flat-square) | MCP | `mcp`, `mcp start`, `mcp stop`, `mcp status`, `mcp manager`, `mcp manager status` |
| ![Templates](https://img.shields.io/badge/-6B7280?style=flat-square) | Templates | `template` |

## Detailed Syntax

### Setup ![Setup](https://img.shields.io/badge/Setup-0EA5E9?style=flat-square)

```text
mem init [--no-agents] [--assistants agents|claude|gemini|all]
mem doctor [--repo <id|path>] [--json] [--repair] [--verbose]
mem repos [--format table|json] [--full-paths]
mem use <repo_id|path>
mem version | mem --version | mem -v
```

### Retrieval ![Retrieval](https://img.shields.io/badge/Retrieval-4F46E5?style=flat-square)

```text
mem get <query> [--include-orphans] [--cluster] [--debug] [scope]
mem explain <query> [--include-orphans] [scope]
mem show <id> [json] [scope]
mem threads [json] [scope]
mem thread <thread_id> [--limit <n>] [json] [scope]
mem recent [--limit <n>] [json] [scope]
mem sessions [--needs-summary] [--count] [--limit <n>] [json] [scope]
```

### Writes ![Writes](https://img.shields.io/badge/Writes-10B981?style=flat-square)

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

### Ingest and Embeddings ![Ingest/Embed](https://img.shields.io/badge/Ingest%2FEmbed-F59E0B?style=flat-square)

```text
mem ingest <path> --thread <id> [--watch] [scope]
mem ingest-artifact <path> --thread <id> [--watch] [scope]
mem embed [--kind memory|chunk|all] [scope]
mem embed status [scope]
```

### Session and Sharing ![Session/Share](https://img.shields.io/badge/Session%2FShare-EC4899?style=flat-square)

```text
mem session upsert <title> [summary] [write-meta] [--merge-window-ms <n>] [--min-gap-ms <n>] [json] [scope]
mem session upsert --title <title> [--summary <summary>] [write-meta] [--merge-window-ms <n>] [--min-gap-ms <n>] [json] [scope]
mem share export [--out <dir>] [scope]
mem share import [--in <dir>] [--replace] [scope]
```

`share import` prompts for confirmation if `source_repo_id` differs from the current repo in interactive terminals.

### MCP ![MCP](https://img.shields.io/badge/MCP-EF4444?style=flat-square)

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

### Templates ![Templates](https://img.shields.io/badge/Templates-6B7280?style=flat-square)

```text
mem template [agents] [--write] [--assistants agents|claude|gemini|all] [--memory|--no-memory]
```

## Notes

- Use `mem --help` for the latest command list.
- Use `mem <command> --help` for command-specific flags and examples.
