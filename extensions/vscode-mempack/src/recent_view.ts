import * as vscode from "vscode";
import { MempackClient } from "./client";
import { RecentMemoryItem } from "./types";

let panel: vscode.WebviewPanel | undefined;

export async function showRecentPanel(
  context: vscode.ExtensionContext,
  client: MempackClient,
  cwd: string
): Promise<void> {
  if (!panel) {
    panel = vscode.window.createWebviewPanel(
      "mempackRecent",
      "Mempack Recent",
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

  const recent = await client.recent(cwd, 50);
  panel.webview.html = renderRecentHtml(context, recent);

  panel.webview.onDidReceiveMessage(async (msg) => {
    if (!panel) {
      return;
    }
    if (msg?.type === "refresh") {
      const updated = await client.recent(cwd, 50);
      panel.webview.postMessage({ type: "recent", recent: updated });
    }
  });
}

function renderRecentHtml(
  context: vscode.ExtensionContext,
  recent: RecentMemoryItem[]
): string {
  const data = { recent };
  const json = JSON.stringify(data);
  const nonce = String(Date.now());
  const csp = panel?.webview.cspSource ?? "";
  const title = "Mempack Recent";

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
    .search {
      width: 100%;
      background: var(--input);
      color: var(--fg);
      border: 1px solid var(--inputBorder);
      border-radius: 6px;
      padding: 6px 8px;
      font-size: 12px;
      margin-bottom: 12px;
    }
    .card {
      border: 1px solid var(--border);
      border-radius: 8px;
      padding: 10px;
      background: var(--card);
      margin-bottom: 10px;
    }
    .muted {
      color: var(--muted);
      font-size: 12px;
    }
  </style>
</head>
<body>
  <div class="header">
    <div class="title">${escapeHtml(title)}</div>
    <button class="btn" id="refresh">Refresh</button>
  </div>
  <input class="search" id="search" placeholder="Filter recent memories..." />
  <div id="list"></div>

  <script nonce="${nonce}">
    const vscode = acquireVsCodeApi();
    const data = ${json};
    let items = Array.isArray(data.recent) ? data.recent : [];

    function escapeHtml(value) {
      return String(value)
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/\"/g, "&quot;")
        .replace(/'/g, "&#39;");
    }

    function render(filter = "") {
      const list = document.getElementById("list");
      const needle = filter.trim().toLowerCase();
      const filtered = items.filter(m => {
        const hay = (m.title + " " + (m.summary || "") + " " + (m.thread_id || "")).toLowerCase();
        return hay.includes(needle);
      });
      if (!filtered.length) {
        list.innerHTML = '<div class="muted">No recent memories.</div>';
        return;
      }
      list.innerHTML = filtered.map(m => {
        const meta = [
          m.thread_id ? "thread=" + m.thread_id : "",
          m.anchor_commit ? "commit=" + m.anchor_commit.slice(0,7) : "",
          m.created_at ? m.created_at : ""
        ].filter(Boolean).join(" · ");
        return \`
          <div class="card">
            <div><strong>\${escapeHtml(m.title || "(untitled)")}</strong></div>
            \${meta ? '<div class="muted">' + escapeHtml(meta) + '</div>' : ""}
            \${m.summary ? '<div style="margin-top:6px;">' + escapeHtml(m.summary) + '</div>' : ""}
          </div>
        \`;
      }).join("");
    }

    document.getElementById("search").addEventListener("input", (e) => {
      render(e.target.value || "");
    });
    document.getElementById("refresh").addEventListener("click", () => {
      vscode.postMessage({ type: "refresh" });
    });

    window.addEventListener("message", (event) => {
      const msg = event.data || {};
      if (msg.type === "recent") {
        items = Array.isArray(msg.recent) ? msg.recent : [];
        render(document.getElementById("search").value || "");
      }
    });

    render();
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
