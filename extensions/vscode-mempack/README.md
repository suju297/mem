# Mem VS Code Extension (MVP)

This extension provides a fast, hotkey-first UI for the `mem` CLI:

- Save Selection as Memory
- Save Checkpoint
- Get Context (prompt format)
- Auto-capture commit sessions + annotate last session
- Browse threads and recent memories
- View memory details and delete memories
- Editor context menu actions (save selection / get context)
- Command Palette actions for refresh/search

Selection text is appended to the summary when saving a memory.
Embeddings are enabled by default; use "Mem: Configure Embeddings" to toggle or change models.
If Ollama is missing, the extension will prompt you to install it (recommended).
MCP write settings follow CLI config precedence: `.mempack/config.json` (repo override) over `~/.config/mempack/config.toml` (global default).

## Requirements

- `mem` binary installed and on PATH (or set `mempack.binaryPath`).
- A git repo with `mem init` run at least once.

## Commands

- Mem: Save Selection as Memory
- Mem: Save Checkpoint
- Mem: Annotate Last Session
- Mem: Get Context for Query
- Mem: Doctor
- Mem: Init (in this repo)
- Mem: Add Mem Stub
- Mem: Toggle MCP Writes
- Mem: Configure Embeddings
- Mem: Configure Token Budget
- Mem: Configure Workspace
- Mem: Configure Default Thread
- Mem: Toggle Intent Capture
- Mem: Annotate Session from List
- Mem: Configure Intent Capture
- Mem: Open Session Diff
- Mem: Mark Session as Reviewed
- Mem: Copy Session Reference

## MCP Writes (User Experience)

On first run in a repo with `.mempack/`, the extension shows the current effective MCP write mode from CLI config.
Use **Mem: Configure MCP Writes** to set:

- **Repo override** (`.mempack/config.json`)
- **Global default** (`~/.config/mempack/config.toml`)

Token budget controls how large the `mem get` context output can be. Higher values include more context but produce longer prompts.

Intent capture options:
- Auto-capture sessions on commit (on/off)
- Nudge style: badge only, badge + toast, or off
- Needs-summary rule: empty commit body, always, or never
- Thread mapping: use branch name as thread (on/off)
- Attach changed files list to session (on/off)

## Hotkeys

- Annotate last session: Cmd/Ctrl+Shift+M

## Configuration

- `mempack.binaryPath`
- `mempack.workspace`
- `mempack.defaultThread`
- `mempack.recentLimit`
- `mempack.commandTimeoutMs`

## Build

```bash
npm install
npm run compile
```

## License

The extension is licensed under the MIT License.
See [LICENSE](LICENSE).
