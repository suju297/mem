import * as vscode from "vscode";
import { MempackClient } from "./client";
import { ThreadItem, ThreadShowResponse } from "./types";

let panel: vscode.WebviewPanel | undefined;

export async function showThreadsPanel(
  context: vscode.ExtensionContext,
  client: MempackClient,
  cwd: string
): Promise<void> {
  if (!panel) {
    panel = vscode.window.createWebviewPanel(
      "mempackThreads",
      "Mem Threads",
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

  const threads = await client.threads(cwd);
  panel.webview.html = renderThreadsHtml(context, threads);

  panel.webview.onDidReceiveMessage(async (msg) => {
    if (!panel) {
      return;
    }
    if (msg?.type === "refresh") {
      const updated = await client.threads(cwd);
      panel.webview.postMessage({ type: "threads", threads: updated });
      return;
    }
    if (msg?.type === "newThread") {
      const threadId = await vscode.window.showInputBox({
        prompt: "New thread ID",
        placeHolder: "e.g. T-auth",
        ignoreFocusOut: true
      });
      if (!threadId || threadId.trim() === "") {
        return;
      }
      const title = await vscode.window.showInputBox({
        prompt: "Thread title",
        value: threadId.trim(),
        ignoreFocusOut: true
      });
      if (!title || title.trim() === "") {
        return;
      }
      const summary = await vscode.window.showInputBox({
        prompt: "Thread summary (first memory)",
        placeHolder: "Short summary for this thread",
        ignoreFocusOut: true
      });
      if (!summary || summary.trim() === "") {
        return;
      }
      try {
        await client.addMemory(cwd, threadId.trim(), title.trim(), summary.trim(), "");
        const updated = await client.threads(cwd);
        panel.webview.postMessage({ type: "threads", threads: updated });
      } catch (err: any) {
        panel.webview.postMessage({
          type: "threadError",
          message: String(err?.message || err)
        });
      }
      return;
    }
    if (msg?.type === "openThread" && typeof msg.threadId === "string") {
      try {
        const detail = await client.thread(cwd, msg.threadId);
        panel.webview.postMessage({ type: "threadDetail", detail });
      } catch (err: any) {
        panel.webview.postMessage({
          type: "threadError",
          message: String(err?.message || err)
        });
      }
    }
  });
}

function renderThreadsHtml(context: vscode.ExtensionContext, threads: ThreadItem[]): string {
  const data = { threads };
  const json = JSON.stringify(data);
  const nonce = String(Date.now());
  const csp = panel?.webview.cspSource ?? "";
  const title = "Mem Threads";

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
      --input: var(--vscode-input-background);
      --inputBorder: var(--vscode-input-border);
    }
    body {
      margin: 0;
      padding: 16px;
      background: var(--bg);
      color: var(--fg);
      font-family: var(--vscode-font-family);
      font-size: var(--vscode-font-size);
    }
    .header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 12px;
    }
    .title {
      font-size: 18px;
      font-weight: 600;
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
      margin-right: 8px;
    }
    .layout {
      display: grid;
      grid-template-columns: 280px 1fr;
      gap: 12px;
      min-height: 60vh;
    }
    .sidebar {
      border: 1px solid var(--border);
      border-radius: 10px;
      overflow: hidden;
    }
    .sidebar-header {
      padding: 8px;
      border-bottom: 1px solid var(--border);
      background: var(--card);
    }
    .search {
      width: 100%;
      background: var(--input);
      color: var(--fg);
      border: 1px solid var(--inputBorder);
      border-radius: 6px;
      padding: 6px 8px;
      font-size: 12px;
    }
    .thread-list {
      max-height: 70vh;
      overflow: auto;
    }
    .thread-item {
      padding: 8px 10px;
      border-bottom: 1px solid var(--border);
      cursor: pointer;
    }
    .thread-item:hover {
      background: var(--card);
    }
    .thread-item.active {
      background: var(--accent);
      color: var(--accentText);
    }
    .thread-title {
      font-weight: 600;
    }
    .thread-meta {
      color: var(--muted);
      font-size: 11px;
    }
    .content {
      border: 1px solid var(--border);
      border-radius: 10px;
      padding: 12px;
      background: var(--card);
    }
    .card {
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 10px;
      background: var(--bg);
      margin-bottom: 10px;
    }
    .card.default {
      border-color: var(--accent);
      box-shadow: 0 0 0 1px var(--accent);
    }
    .pill {
      display: inline-block;
      padding: 2px 8px;
      border-radius: 999px;
      font-size: 11px;
      background: var(--badge);
      color: var(--badgeText);
      margin-left: 6px;
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
  </style>
</head>
<body>
  <div class="header">
    <div class="title">${escapeHtml(title)}</div>
    <div>
      <button class="btn secondary" id="newThread">New Thread</button>
      <button class="btn" id="refresh">Refresh</button>
    </div>
  </div>

  <div class="layout">
    <div class="sidebar">
      <div class="sidebar-header">
        <input class="search" id="search" placeholder="Search threads..." />
      </div>
      <div class="thread-list" id="threadList"></div>
    </div>
    <div class="content" id="content">
      <div class="muted">Select a thread to view its memories.</div>
    </div>
  </div>

  <script nonce="${nonce}">
    const vscode = acquireVsCodeApi();
    const data = ${json};
    let threads = Array.isArray(data.threads) ? data.threads : [];
    let activeThreadId = "";

    function escapeHtml(value) {
      return String(value)
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/\"/g, "&quot;")
        .replace(/'/g, "&#39;");
    }

    function renderThreads(filter = "") {
      const list = document.getElementById("threadList");
      const needle = filter.trim().toLowerCase();
      const filtered = threads.filter(t => {
        const hay = (t.thread_id + " " + (t.title || "")).toLowerCase();
        return hay.includes(needle);
      });
      const defaultThreadId = "T-SESSION";
      const defaultThread = filtered.find(t => t.thread_id === defaultThreadId);
      const others = filtered.filter(t => t.thread_id !== defaultThreadId);
      const ordered = defaultThread ? [defaultThread, ...others] : others;
      if (filtered.length === 0) {
        list.innerHTML = '<div class="thread-item"><div class="muted">No threads.</div></div>';
        return;
      }
      list.innerHTML = ordered.map(t => {
        const active = t.thread_id === activeThreadId ? "active" : "";
        const count = t.memory_count ? t.memory_count : 0;
        const isDefault = t.thread_id === defaultThreadId;
        const title = isDefault ? "Default Thread (T-SESSION)" : (t.title || t.thread_id);
        return \`
          <div class="thread-item \${active}" data-id="\${escapeHtml(t.thread_id)}">
            <div class="thread-title">\${escapeHtml(title)}</div>
            <div class="thread-meta">\${escapeHtml(t.thread_id)} · \${count} memories\${isDefault ? " · default" : ""}</div>
          </div>
        \`;
      }).join("");

      list.querySelectorAll(".thread-item").forEach((el) => {
        const id = el.getAttribute("data-id");
        if (!id) return;
        el.addEventListener("click", () => {
          activeThreadId = id;
          vscode.postMessage({ type: "openThread", threadId: id });
          renderThreads(document.getElementById("search").value);
        });
      });
    }

    function renderThreadDetail(detail) {
      const container = document.getElementById("content");
      if (!detail || !detail.thread) {
        container.innerHTML = '<div class="muted">No thread detail.</div>';
        return;
      }
      const thread = detail.thread;
      const memories = Array.isArray(detail.memories) ? detail.memories : [];
      const isDefault = thread.thread_id === "T-SESSION";
      const title = isDefault ? "Default Thread (T-SESSION)" : (thread.title || thread.thread_id);
      const header = \`
        <div class="card\${isDefault ? " default" : ""}">
          <div><strong>\${escapeHtml(title)}</strong>\${isDefault ? '<span class="pill">default</span>' : ""}</div>
          <div class="muted">\${escapeHtml(thread.thread_id)} · \${memories.length} memories</div>
          \${isDefault ? '<div class="muted">Used when you do not specify a thread.</div>' : ""}
        </div>\`;
      const items = memories.map(mem => {
        const meta = [
          mem.created_at ? mem.created_at : "",
          mem.anchor_commit ? "commit=" + mem.anchor_commit.slice(0,7) : "",
          mem.superseded_by ? "superseded" : ""
        ].filter(Boolean).join(" · ");
        return \`
          <div class="card">
            <div><strong>\${escapeHtml(mem.title || "(untitled)")}</strong></div>
            \${meta ? '<div class="muted">' + escapeHtml(meta) + '</div>' : ""}
            \${mem.summary ? '<div style="margin-top:6px;">' + escapeHtml(mem.summary) + '</div>' : ""}
          </div>\`;
      }).join("");
      container.innerHTML = header + (items || '<div class="muted">No memories in this thread.</div>');
    }

    document.getElementById("search").addEventListener("input", (e) => {
      renderThreads(e.target.value || "");
    });
    document.getElementById("newThread").addEventListener("click", () => {
      vscode.postMessage({ type: "newThread" });
    });
    document.getElementById("refresh").addEventListener("click", () => {
      vscode.postMessage({ type: "refresh" });
    });

    window.addEventListener("message", (event) => {
      const msg = event.data || {};
      if (msg.type === "threads") {
        threads = Array.isArray(msg.threads) ? msg.threads : [];
        renderThreads(document.getElementById("search").value || "");
      }
      if (msg.type === "threadDetail") {
        renderThreadDetail(msg.detail);
      }
      if (msg.type === "threadError") {
        const container = document.getElementById("content");
        container.innerHTML = '<div class="muted">Error: ' + escapeHtml(msg.message || "Unknown") + '</div>';
      }
    });

    renderThreads();
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
