import { ContextPack, ShowResponse } from "./types";

export function suggestTitle(text: string): string {
  const lines = text
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter((line) => line.length > 0);
  if (lines.length === 0) {
    return "Memory";
  }
  return truncate(lines[0], 80);
}

export function suggestSummary(text: string): string {
  const clean = text.replace(/\s+/g, " ").trim();
  if (clean === "") {
    return "";
  }
  return truncate(clean, 160);
}

export function truncate(value: string, max: number): string {
  if (value.length <= max) {
    return value;
  }
  return value.slice(0, max - 3) + "...";
}

export function formatDoctorReport(report: any): string {
  const ok = report?.ok === true;
  const status = ok ? "Ready" : report?.error ? "Error" : "Needs setup";
  const lines = [
    "# Mempack Doctor",
    "",
    `- Status: ${status}`,
    report?.error ? `- Error: ${report.error}` : "",
    report?.suggestion ? `- Suggestion: ${report.suggestion}` : "",
    report?.repo?.id ? `- Repo: ${report.repo.id}` : "",
    report?.repo?.git_root ? `- Git root: ${report.repo.git_root}` : "",
    "",
    "## Raw JSON",
    "```json",
    JSON.stringify(report, null, 2),
    "```"
  ];
  return lines.filter((line) => line !== "").join("\n");
}

export function formatShowResult(result: ShowResponse): string {
  if (result.kind === "chunk" && result.chunk) {
    const chunk = result.chunk;
    return [
      `# Chunk ${chunk.id}`,
      "",
      `- Repo: ${chunk.repo_id}`,
      chunk.thread_id ? `- Thread: ${chunk.thread_id}` : "",
      chunk.locator ? `- Locator: ${chunk.locator}` : "",
      chunk.created_at ? `- Created: ${chunk.created_at}` : "",
      chunk.deleted_at ? `- Deleted: ${chunk.deleted_at}` : "",
      "",
      "## Text",
      "```",
      chunk.text || "",
      "```"
    ]
      .filter((line) => line !== "")
      .join("\n");
  }

  const mem = result.memory;
  if (!mem) {
    return "No memory found.";
  }
  const tags = mem.tags_json || "[]";
  const entities = mem.entities_json || "[]";

  return [
    `# ${mem.title}`,
    "",
    `- ID: ${mem.id}`,
    `- Repo: ${mem.repo_id}`,
    mem.thread_id ? `- Thread: ${mem.thread_id}` : "",
    mem.anchor_commit ? `- Anchor commit: ${mem.anchor_commit}` : "",
    mem.superseded_by ? `- Superseded by: ${mem.superseded_by}` : "",
    mem.created_at ? `- Created: ${mem.created_at}` : "",
    mem.deleted_at ? `- Deleted: ${mem.deleted_at}` : "",
    "",
    "## Summary",
    mem.summary || "",
    "",
    "## Tags",
    "```json",
    tags,
    "```",
    "",
    "## Entities",
    "```json",
    entities,
    "```"
  ]
    .filter((line) => line !== "")
    .join("\n");
}

export function formatContextPack(pack: ContextPack, promptText: string): string {
  const repoRoot = pack?.repo?.git_root || "";
  const repoID = pack?.repo?.repo_id || "";
  const workspace = pack?.workspace || "";
  const query = pack?.search_meta?.query || "";
  const mode = pack?.search_meta?.mode_used || pack?.search_meta?.mode || "";
  const warnings = pack?.search_meta?.warnings || [];
  const rewrites = pack?.search_meta?.rewrites_applied || [];
  const intent = pack?.search_meta?.intent || "";
  const timeHint = pack?.search_meta?.time_hint || "";
  const recencyBoost =
    typeof pack?.search_meta?.recency_boost === "number"
      ? String(pack.search_meta.recency_boost)
      : "";
  const clusters =
    typeof pack?.search_meta?.clusters_formed === "number"
      ? String(pack.search_meta.clusters_formed)
      : "";
  const memoriesCount = Array.isArray(pack.top_memories) ? pack.top_memories.length : 0;
  const chunksCount = Array.isArray(pack.top_chunks) ? pack.top_chunks.length : 0;
  const threadsCount = Array.isArray(pack.matched_threads) ? pack.matched_threads.length : 0;

  const lines: Array<string | null> = [];
  const push = (...items: Array<string | null>) => {
    for (const item of items) {
      lines.push(item === undefined ? null : item);
    }
  };

  push(`# Mempack Context${query ? ` — ${query}` : ""}`);
  push("");
  push(
    "| Repo | Workspace | Mode | Results |",
    "| --- | --- | --- | --- |",
    `| ${repoRoot || repoID || "unknown"} | ${workspace || "default"} | ${mode || "bm25"} | memories ${memoriesCount} · chunks ${chunksCount} · threads ${threadsCount} |`
  );
  push("");
  push(
    "| Signal | Value |",
    "| --- | --- |"
  );
  push(`| intent | ${intent || "—"} |`);
  push(`| time hint | ${timeHint || "—"} |`);
  push(`| recency boost | ${recencyBoost || "—"} |`);
  push(`| clusters | ${clusters || "—"} |`);
  if (rewrites.length > 0) {
    push(`| rewrites | ${rewrites.join(", ")} |`);
  }
  if (warnings.length > 0) {
    push(`| warnings | ${warnings.join(", ")} |`);
  }

  if (Array.isArray(pack.matched_threads) && pack.matched_threads.length > 0) {
    push("", "## Matched Threads");
    for (const thread of pack.matched_threads) {
      const why = thread.why ? ` — ${thread.why}` : "";
      push(`- ${thread.thread_id}${why}`);
    }
  }

  if (Array.isArray(pack.top_memories) && pack.top_memories.length > 0) {
    push("", "## Top Memories");
    let idx = 1;
    for (const mem of pack.top_memories) {
      const head = `${idx}. **${mem.title || "(untitled)"}** — \`${mem.id}\``;
      const metaParts = [
        mem.thread_id ? `thread=${mem.thread_id}` : "",
        mem.anchor_commit ? `commit=${mem.anchor_commit.slice(0, 7)}` : "",
        mem.is_cluster ? `cluster=${mem.cluster_size || 0}` : "",
        typeof mem.similarity === "number" ? `sim=${mem.similarity.toFixed(2)}` : ""
      ].filter((v) => v !== "");
      push(metaParts.length > 0 ? `${head}  \n_${metaParts.join(" · ")}_` : head);
      const summary = (mem.summary || "").trim();
      if (summary !== "") {
        push(`> ${summary.replace(/\s+/g, " ").trim()}`);
      }
      idx += 1;
    }
  }

  if (Array.isArray(pack.top_chunks) && pack.top_chunks.length > 0) {
    push("", "## Top Evidence");
    let idx = 1;
    for (const chunk of pack.top_chunks) {
      const loc = chunk.locator || chunk.chunk_id;
      const thread = chunk.thread_id ? ` · thread=${chunk.thread_id}` : "";
      const sources = Array.isArray(chunk.sources) ? chunk.sources.length : 0;
      const sourceLabel = sources > 0 ? ` · sources=${sources}` : "";
      push(`${idx}. \`${loc}\`${thread}${sourceLabel}`);
      const text = (chunk.text || "").trim();
      if (text !== "") {
        push("```", truncate(text, 280), "```");
      }
      idx += 1;
    }
  }

  if (Array.isArray(pack.rules) && pack.rules.length > 0) {
    push("", "## Rules");
    for (const rule of pack.rules) {
      push(`- ${rule}`);
    }
  }

  const budget = pack?.budget;
  if (budget?.tokenizer) {
    push(
      "",
      `**Token budget**: ${budget.used_total}/${budget.target_total} (${budget.tokenizer})`
    );
  }

  if (pack?.state) {
    push(
      "",
      "<details>",
      "<summary>State (JSON)</summary>",
      "",
      "```json",
      JSON.stringify(pack.state, null, 2),
      "```",
      "</details>"
    );
  }

  if (pack?.search_meta) {
    push(
      "",
      "<details>",
      "<summary>Search Meta (JSON)</summary>",
      "",
      "```json",
      JSON.stringify(pack.search_meta || {}, null, 2),
      "```",
      "</details>"
    );
  }

  if (promptText.trim() !== "") {
    const trimmed = promptText.trimEnd();
    const fence = pickFence(trimmed);
    push(
      "",
      "<details>",
      "<summary>Prompt (copied to clipboard)</summary>",
      "",
      fence,
      trimmed,
      fence,
      "</details>"
    );
  }

  return lines.filter((line) => line !== null).join("\n");
}

function pickFence(text: string): string {
  let max = 0;
  let current = 0;
  for (const ch of text) {
    if (ch === "`") {
      current += 1;
      if (current > max) {
        max = current;
      }
    } else {
      current = 0;
    }
  }
  const count = Math.max(3, max + 1);
  return "`".repeat(count);
}
