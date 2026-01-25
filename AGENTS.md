# Mempack Agent Policy

Before starting any task, fetch repo memory:
- Prefer MCP: call `mempack.get_context` with the user's task as the query.

If MCP is unavailable, ask the user to run:
`mempack get "<task>" --format prompt`

Full instructions: `.mempack/MEMORY.md`
