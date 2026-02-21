import * as vscode from "vscode";
import { ContextPack } from "./types";

let panel: vscode.WebviewPanel | undefined;

export function showContextPanel(
  context: vscode.ExtensionContext,
  pack: ContextPack,
  prompt: string
): void {
  if (!panel) {
    panel = vscode.window.createWebviewPanel(
      "mempackContext",
      "Mem Context",
      vscode.ViewColumn.Active,
      {
        enableScripts: true,
        retainContextWhenHidden: true
      }
    );
    panel.onDidDispose(() => {
      panel = undefined;
    });
  } else {
    panel.reveal(vscode.ViewColumn.Active);
  }

  panel.webview.html = renderContextHtml(context, pack, prompt);
}

function renderContextHtml(
  context: vscode.ExtensionContext,
  pack: ContextPack,
  prompt: string
): string {
  const data = {
    pack,
    prompt
  };
  const json = JSON.stringify(data);
  const nonce = String(Date.now());
  const csp = panel?.webview.cspSource ?? "";
  const title = "Mem Context";

  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src ${csp} https: data:; style-src ${csp} 'unsafe-inline'; script-src 'nonce-${nonce}';" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>${escapeHtml(title)}</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: var(--vscode-editor-background);
      --fg: var(--vscode-editor-foreground);
      --muted: var(--vscode-descriptionForeground);
      --card: var(--vscode-editorWidget-background);
      --border: var(--vscode-editorWidget-border);
      --accent: var(--vscode-button-background);
      --accentText: var(--vscode-button-foreground);
      --badge: var(--vscode-badge-background);
      --badgeText: var(--vscode-badge-foreground);
      --code: var(--vscode-textCodeBlock-background);
    }
    body {
      margin: 0;
      padding: 20px;
      background: var(--bg);
      color: var(--fg);
      font-family: var(--vscode-font-family);
      font-size: var(--vscode-font-size);
    }
    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      margin-bottom: 16px;
    }
    .title {
      font-size: 20px;
      font-weight: 600;
    }
    .meta {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin: 8px 0 16px;
    }
    .badge {
      background: var(--badge);
      color: var(--badgeText);
      padding: 4px 8px;
      border-radius: 999px;
      font-size: 12px;
    }
    .actions {
      display: flex;
      gap: 8px;
      align-items: center;
    }
    .tabs {
      display: flex;
      gap: 8px;
      margin-bottom: 12px;
    }
    .tab {
      border: 1px solid var(--border);
      padding: 6px 10px;
      border-radius: 6px;
      cursor: pointer;
      background: transparent;
      color: var(--fg);
      font-size: 12px;
    }
    .tab.active {
      background: var(--accent);
      color: var(--accentText);
      border-color: transparent;
    }
    .panel {
      display: none;
    }
    .panel.active {
      display: block;
    }
    .card {
      border: 1px solid var(--border);
      background: var(--card);
      padding: 12px;
      border-radius: 10px;
      margin-bottom: 12px;
    }
    .row {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
    }
    .muted {
      color: var(--muted);
      font-size: 12px;
    }
    .code {
      background: var(--code);
      padding: 8px;
      border-radius: 8px;
      white-space: pre-wrap;
      font-family: var(--vscode-editor-font-family);
      font-size: 12px;
    }
    .btn {
      background: var(--accent);
      color: var(--accentText);
      border: none;
      padding: 6px 10px;
      border-radius: 6px;
      cursor: pointer;
      font-size: 12px;
    }
    .btn.secondary {
      background: transparent;
      border: 1px solid var(--border);
      color: var(--fg);
    }
    .list {
      display: grid;
      gap: 10px;
    }
  </style>
</head>
<body>
  <div class="header">
    <div>
      <div class="title">${escapeHtml(title)}</div>
      <div class="meta" id="meta"></div>
    </div>
    <div class="actions">
      <button class="btn" id="copyPrompt">Copy Prompt</button>
      <button class="btn secondary" id="copyJson">Copy JSON</button>
    </div>
  </div>

  <div class="tabs">
    <button class="tab active" data-tab="memories">Memories</button>
    <button class="tab" data-tab="evidence">Evidence</button>
    <button class="tab" data-tab="threads">Threads</button>
    <button class="tab" data-tab="meta">Meta</button>
  </div>

  <div class="panel active" id="panel-memories"></div>
  <div class="panel" id="panel-evidence"></div>
  <div class="panel" id="panel-threads"></div>
  <div class="panel" id="panel-meta"></div>

  <script nonce="${nonce}">
    const data = ${json};
    const pack = data.pack || {};
    const prompt = data.prompt || "";

    const meta = document.getElementById("meta");
    const repo = pack.repo?.git_root || pack.repo?.repo_id || "unknown";
    const workspace = pack.workspace || "default";
    const mode = pack.search_meta?.mode_used || pack.search_meta?.mode || "bm25";
    const query = pack.search_meta?.query || "";
    const counts = {
      memories: Array.isArray(pack.top_memories) ? pack.top_memories.length : 0,
      chunks: Array.isArray(pack.top_chunks) ? pack.top_chunks.length : 0,
      threads: Array.isArray(pack.matched_threads) ? pack.matched_threads.length : 0
    };
    const badges = [
      { label: "Repo", value: repo },
      { label: "Workspace", value: workspace },
      { label: "Mode", value: mode },
      { label: "Query", value: query },
      { label: "Memories", value: String(counts.memories) },
      { label: "Chunks", value: String(counts.chunks) },
      { label: "Threads", value: String(counts.threads) }
    ];
    meta.innerHTML = badges
      .filter(b => b.value && b.value !== "unknown")
      .map(b => '<span class="badge">' + escapeHtml(b.label + ": " + b.value) + "</span>")
      .join("");

    function escapeHtml(value) {
      return String(value)
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/\"/g, "&quot;")
        .replace(/'/g, "&#39;");
    }

    function renderMemories() {
      const container = document.getElementById("panel-memories");
      const list = Array.isArray(pack.top_memories) ? pack.top_memories : [];
      if (!list.length) {
        container.innerHTML = '<div class="muted">No memories found.</div>';
        return;
      }
      container.innerHTML = list.map((mem, idx) => {
        const meta = [
          mem.thread_id ? "thread=" + mem.thread_id : "",
          mem.anchor_commit ? "commit=" + mem.anchor_commit.slice(0,7) : "",
          mem.is_cluster ? "cluster=" + (mem.cluster_size || 0) : "",
          typeof mem.similarity === "number" ? "sim=" + mem.similarity.toFixed(2) : ""
        ].filter(Boolean).join(" · ");
        return \`
          <div class="card">
            <div class="row">
              <div><strong>\${escapeHtml(mem.title || "(untitled)")}</strong></div>
              <div class="muted">\${escapeHtml(mem.id || "")}</div>
            </div>
            \${meta ? '<div class="muted">' + escapeHtml(meta) + "</div>" : ""}
            \${mem.summary ? '<div style="margin-top:6px;">' + escapeHtml(mem.summary) + "</div>" : ""}
          </div>
        \`;
      }).join("");
    }

    function renderEvidence() {
      const container = document.getElementById("panel-evidence");
      const list = Array.isArray(pack.top_chunks) ? pack.top_chunks : [];
      if (!list.length) {
        container.innerHTML = '<div class="muted">No evidence chunks found.</div>';
        return;
      }
      container.innerHTML = list.map((chunk) => {
        const loc = chunk.locator || chunk.chunk_id || "";
        const meta = [
          chunk.thread_id ? "thread=" + chunk.thread_id : "",
          Array.isArray(chunk.sources) ? "sources=" + chunk.sources.length : ""
        ].filter(Boolean).join(" · ");
        return \`
          <div class="card">
            <div class="row">
              <div><strong>\${escapeHtml(loc)}</strong></div>
              <div class="muted">\${escapeHtml(meta)}</div>
            </div>
            \${chunk.text ? '<div class="code" style="margin-top:8px;">' + escapeHtml(chunk.text) + "</div>" : ""}
          </div>
        \`;
      }).join("");
    }

    function renderThreads() {
      const container = document.getElementById("panel-threads");
      const list = Array.isArray(pack.matched_threads) ? pack.matched_threads : [];
      if (!list.length) {
        container.innerHTML = '<div class="muted">No matched threads.</div>';
        return;
      }
      container.innerHTML = list.map((thread) => {
        const why = thread.why ? thread.why : "";
        return \`
          <div class="card">
            <div class="row">
              <div><strong>\${escapeHtml(thread.thread_id || "")}</strong></div>
            </div>
            \${why ? '<div class="muted">' + escapeHtml(why) + '</div>' : ""}
          </div>
        \`;
      }).join("");
    }

    function renderMeta() {
      const container = document.getElementById("panel-meta");
      const rules = Array.isArray(pack.rules) ? pack.rules : [];
      const searchMeta = pack.search_meta || {};
      const state = pack.state || {};
      container.innerHTML = \`
        <div class="card">
          <div class="row"><strong>Rules</strong></div>
          \${rules.length ? rules.map(r => '<div>• ' + escapeHtml(r) + '</div>').join("") : '<div class="muted">No rules.</div>'}
        </div>
        <div class="card">
          <div class="row"><strong>Search Meta</strong></div>
          <div class="code">\${escapeHtml(JSON.stringify(searchMeta, null, 2))}</div>
        </div>
        <div class="card">
          <div class="row"><strong>State</strong></div>
          <div class="code">\${escapeHtml(JSON.stringify(state, null, 2))}</div>
        </div>
      \`;
    }

    document.querySelectorAll(".tab").forEach((tab) => {
      tab.addEventListener("click", () => {
        document.querySelectorAll(".tab").forEach(t => t.classList.remove("active"));
        document.querySelectorAll(".panel").forEach(p => p.classList.remove("active"));
        tab.classList.add("active");
        const key = tab.getAttribute("data-tab");
        const panel = document.getElementById("panel-" + key);
        if (panel) panel.classList.add("active");
      });
    });

    document.getElementById("copyPrompt").addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(prompt);
      } catch {}
    });
    document.getElementById("copyJson").addEventListener("click", async () => {
      try {
        await navigator.clipboard.writeText(JSON.stringify(pack, null, 2));
      } catch {}
    });

    renderMemories();
    renderEvidence();
    renderThreads();
    renderMeta();
  </script>
</body>
</html>`;
}

function escapeHtml(value: string): string {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\"/g, "&quot;")
    .replace(/'/g, "&#39;");
}
