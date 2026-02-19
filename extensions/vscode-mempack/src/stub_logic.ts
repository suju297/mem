export const MARKER_START = "<!-- mempack:start -->";
export const MARKER_END = "<!-- mempack:end -->";

export const STUB_BLOCK = `${MARKER_START}
Use Mempack:
- Prefer MCP tool: mempack_get_context at task start.
- Fallback: run \`mem get "<task>" --format prompt\`.
${MARKER_END}`;

export const MINIMAL_AGENTS = `# Mempack Agent Policy

Before starting any task, fetch repo memory:
- Prefer MCP: call mempack_get_context with the user's task as the query.

If MCP is unavailable, ask the user to run:
\`mem get "<task>" --format prompt\`

Full instructions: \`.mempack/MEMORY.md\`
`;

export function hasStub(content: string): boolean {
  return content.includes(MARKER_START) && content.includes(MARKER_END);
}

export function appendStub(existing: string): { updated: string; changed: boolean } {
  if (hasStub(existing)) {
    return { updated: existing, changed: false };
  }
  const trimmed = existing.trimEnd();
  const separator = trimmed === "" ? "" : "\n\n";
  const updated = trimmed + separator + STUB_BLOCK + "\n";
  return { updated, changed: true };
}

export function buildAgentsWithStub(): string {
  return MINIMAL_AGENTS + "\n" + STUB_BLOCK + "\n";
}
