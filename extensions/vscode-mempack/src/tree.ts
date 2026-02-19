import * as vscode from "vscode";
import * as fs from "fs/promises";
import { MempackClient } from "./client";
import { EmbedStatusResponse, RecentMemoryItem, SessionItem, ThreadItem, ThreadMemoryBrief } from "./types";
import { getWorkspaceRoot } from "./workspace";
import {
  getConfigPath,
  getRepoConfigPath,
  parseMcpWritesConfig,
  parseRepoConfig,
  parseTokenBudgetConfig,
  resolveMcpWrites,
  McpWritesMode
} from "./config";

const BRAND_COLOR = new vscode.ThemeColor("mempack.brand");

function brandIcon(name: string): vscode.ThemeIcon {
  return new vscode.ThemeIcon(name, BRAND_COLOR);
}

export class MempackTreeProvider implements vscode.TreeDataProvider<MempackNode> {
  private client: MempackClient;
  private context: vscode.ExtensionContext;
  private _onDidChangeTreeData = new vscode.EventEmitter<MempackNode | undefined>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private doctorCache?: { at: number; report: any };
  private healthCache?: { at: number; node: HealthNode };
  private repoCache?: { at: number; node: RepoNode };

  private mcpCache?: { at: number; node: McpServerNode; running: boolean };
  private mcpWritesCache?: { at: number; node: McpWritesNode };
  private embeddingCache?: { at: number; node: EmbeddingsNode };
  private intentCaptureCache?: { at: number; node: SessionsOnCommitNode };
  private tokenBudgetCache?: { at: number; node: TokenBudgetNode };
  private workspaceCache?: { at: number; node: WorkspaceNode };
  private defaultThreadCache?: { at: number; node: DefaultThreadNode };
  private needsSummaryCache?: { at: number; items: SessionItem[] };
  private recentSessionsCache?: { at: number; items: SessionItem[] };
  private threadsCache?: { at: number; items: ThreadItem[] };
  private recentCache?: { at: number; items: RecentMemoryItem[] };

  constructor(client: MempackClient, context: vscode.ExtensionContext) {
    this.client = client;
    this.context = context;
  }

  refresh(): void {
    this.doctorCache = undefined;
    this.healthCache = undefined;
    this.repoCache = undefined;
    this.mcpCache = undefined;
    this.mcpWritesCache = undefined;
    this.embeddingCache = undefined;
    this.intentCaptureCache = undefined;
    this.tokenBudgetCache = undefined;
    this.workspaceCache = undefined;
    this.defaultThreadCache = undefined;
    this.needsSummaryCache = undefined;
    this.recentSessionsCache = undefined;
    this.threadsCache = undefined;
    this.recentCache = undefined;
    this._onDidChangeTreeData.fire(undefined);
  }

  getTreeItem(element: MempackNode): vscode.TreeItem {
    return element;
  }

  async getChildren(element?: MempackNode): Promise<MempackNode[]> {
    const cwd = this.getCwd();
    if (!cwd) {
      return [new HealthNode("Health: Unavailable", "Mempack needs a workspace")];
    }

    if (!element) {
      return [
        new StatusRootNode(),
        new SettingsRootNode(),
        new SearchRootNode(),
        new RecentSessionsRootNode(),
        new NeedsSummaryRootNode(await this.getNeedsSummaryCount(cwd)),
        new ThreadsRootNode(),
        new RecentRootNode()
      ];
    }

    if (element instanceof StatusRootNode) {
      return [
        await this.getHealthNode(cwd),
        await this.getRepoNode(cwd),
        await this.getMcpNode(cwd),
        await this.getEmbeddingsNode(cwd)
      ];
    }

    if (element instanceof SettingsRootNode) {
      return [
        await this.getMcpWritesNode(cwd),
        await this.getSessionsOnCommitNode(),
        await this.getWorkspaceNode(),
        await this.getDefaultThreadNode(),
        await this.getTokenBudgetNode()
      ];
    }

    if (element instanceof SearchRootNode) {
      return [new SearchContextNode(), new ExplainSearchNode()];
    }

    if (element instanceof RecentSessionsRootNode) {
      try {
        const limit = this.client.recentLimit;
        const sessions = await this.getRecentSessions(cwd, limit);
        return sessions.map((s) => new SessionNode(s));
      } catch (err: any) {
        return [buildErrorNode("Recent Sessions", err)];
      }
    }

    if (element instanceof ThreadsRootNode) {
      try {
        const threads = await this.getThreads(cwd);
        return threads.map((thread) => new ThreadNode(thread));
      } catch (err: any) {
        return [buildErrorNode("Threads", err)];
      }
    }

    if (element instanceof ThreadNode) {
      try {
        const thread = await this.client.thread(cwd, element.thread.thread_id);
        return (thread.memories || []).map((mem) => new MemoryNode(mem));
      } catch (err: any) {
        return [buildErrorNode("Thread", err)];
      }
    }

    if (element instanceof RecentRootNode) {
      try {
        const limit = this.client.recentLimit;
        const recent = await this.getRecent(cwd, limit);
        return recent.map((mem) => new MemoryNode(mem));
      } catch (err: any) {
        return [buildErrorNode("Recent", err)];
      }
    }

    if (element instanceof NeedsSummaryRootNode) {
      try {
        const sessions = await this.getNeedsSummarySessions(cwd, 20);
        return sessions.map((s) => new SessionNode(s));
      } catch (err: any) {
        return [buildErrorNode("Needs Summary", err)];
      }
    }

    return [];
  }

  private getCwd(): string | undefined {
    const active = vscode.window.activeTextEditor?.document?.uri;
    return getWorkspaceRoot(active);
  }

  private async getDoctorReport(cwd: string): Promise<any> {
    const now = Date.now();
    if (this.doctorCache && now - this.doctorCache.at < 5000) {
      return this.doctorCache.report;
    }
    const report = await this.client.doctor(cwd);
    this.doctorCache = { at: now, report };
    return report;
  }

  private async getHealthNode(cwd: string): Promise<HealthNode> {
    const now = Date.now();
    if (this.healthCache && now - this.healthCache.at < 5000) {
      return this.healthCache.node;
    }
    try {
      const report = await this.getDoctorReport(cwd);
      const label = report.ok ? "Health: OK" : "Health: Issues";
      const detail = report.error || report.suggestion || "";
      const node = new HealthNode(label, detail, now);
      this.healthCache = { at: now, node };
      return node;
    } catch (err: any) {
      const node = new HealthNode("Health: Unavailable", formatErrorMessage(err), now);
      this.healthCache = { at: now, node };
      return node;
    }
  }

  private async getRepoNode(cwd: string): Promise<RepoNode> {
    const now = Date.now();
    if (this.repoCache && now - this.repoCache.at < 5000) {
      return this.repoCache.node;
    }
    try {
      const report = await this.getDoctorReport(cwd);
      const root = report?.repo?.git_root ? String(report.repo.git_root) : cwd;
      const repoID = report?.repo?.id ? String(report.repo.id) : "";
      const memoryDBPath = report?.db?.path ? String(report.db.path) : undefined;
      const memoryDBSizeBytes =
        typeof report?.db?.size_bytes === "number" ? Number(report.db.size_bytes) : undefined;
      const memoryDBExists =
        typeof report?.db?.exists === "boolean" ? Boolean(report.db.exists) : undefined;
      const node = new RepoNode(root, repoID, memoryDBPath, memoryDBSizeBytes, memoryDBExists);
      this.repoCache = { at: now, node };
      return node;
    } catch (err: any) {
      const node = new RepoNode(cwd, "", formatErrorMessage(err));
      this.repoCache = { at: now, node };
      return node;
    }
  }

  private async getMcpWritesNode(cwd: string): Promise<McpWritesNode> {
    const now = Date.now();
    if (this.mcpWritesCache && now - this.mcpWritesCache.at < 5000) {
      return this.mcpWritesCache.node;
    }
    const repoConfigPath = getRepoConfigPath(cwd);
    const globalConfigPath = getConfigPath();
    let repoCfg = {};
    let globalCfg = {};

    try {
      const content = await fs.readFile(repoConfigPath, "utf8");
      repoCfg = parseRepoConfig(content);
    } catch (err: any) {
      if (err?.code !== "ENOENT") {
        const node = new McpWritesNode(undefined, repoConfigPath, "Unavailable");
        this.mcpWritesCache = { at: now, node };
        return node;
      }
    }

    try {
      const content = await fs.readFile(globalConfigPath, "utf8");
      globalCfg = parseMcpWritesConfig(content);
    } catch (err: any) {
      if (err?.code !== "ENOENT") {
        const node = new McpWritesNode(undefined, globalConfigPath, "Unavailable");
        this.mcpWritesCache = { at: now, node };
        return node;
      }
    }

    const effective = resolveMcpWrites(globalCfg, repoCfg);
    const sourceLabel =
      effective.source === "repo" ? "Repo" : effective.source === "global" ? "Global" : "Default";
    const detail = effective.invalidConfig
      ? `${sourceLabel} (invalid; forced Off)`
      : sourceLabel;
    const sourcePath = effective.source === "repo" ? repoConfigPath : globalConfigPath;
    const node = new McpWritesNode(effective.mode, sourcePath, detail);
    this.mcpWritesCache = { at: now, node };
    return node;
  }

  private async getMcpNode(cwd: string): Promise<McpServerNode> {
    const now = Date.now();
    if (this.mcpCache && now - this.mcpCache.at < 5000) {
      return this.mcpCache.node;
    }
    try {
      const status = await this.client.mcpStatus(cwd);
      await vscode.commands.executeCommand("setContext", "mempack.mcpRunning", status.running);
      const node = new McpServerNode(status.running, status.pid, status.message);
      this.mcpCache = { at: now, node, running: status.running };
      return node;
    } catch (err: any) {
      await vscode.commands.executeCommand("setContext", "mempack.mcpRunning", false);
      const node = new McpServerNode(false, undefined, formatErrorMessage(err), true);
      this.mcpCache = { at: now, node, running: false };
      return node;
    }
  }

  private async getEmbeddingsNode(cwd: string): Promise<EmbeddingsNode> {
    const now = Date.now();
    if (this.embeddingCache && now - this.embeddingCache.at < 5000) {
      return this.embeddingCache.node;
    }
    try {
      const status = await this.client.embedStatus(cwd);
      const node = new EmbeddingsNode(status);
      this.embeddingCache = { at: now, node };
      return node;
    } catch (err: any) {
      const node = new EmbeddingsNode(undefined, formatErrorMessage(err));
      this.embeddingCache = { at: now, node };
      return node;
    }
  }

  private async getSessionsOnCommitNode(): Promise<SessionsOnCommitNode> {
    const now = Date.now();
    if (this.intentCaptureCache && now - this.intentCaptureCache.at < 5000) {
      return this.intentCaptureCache.node;
    }
    const cfg = vscode.workspace.getConfiguration("mempack");
    const enabled = cfg.get<boolean>("autoSessionsEnabled", false);
    const node = new SessionsOnCommitNode(enabled);
    this.intentCaptureCache = { at: now, node };
    return node;
  }

  private async getTokenBudgetNode(): Promise<TokenBudgetNode> {
    const now = Date.now();
    if (this.tokenBudgetCache && now - this.tokenBudgetCache.at < 5000) {
      return this.tokenBudgetCache.node;
    }
    const active = vscode.window.activeTextEditor?.document?.uri;
    const cwd = getWorkspaceRoot(active);
    if (!cwd) {
      const node = new TokenBudgetNode(undefined, getConfigPath(), "Unavailable");
      this.tokenBudgetCache = { at: now, node };
      return node;
    }
    const repoConfigPath = getRepoConfigPath(cwd);
    try {
      const repoContent = await fs.readFile(repoConfigPath, "utf8");
      const repoCfg = parseRepoConfig(repoContent);
      if (typeof repoCfg.token_budget === "number" && repoCfg.token_budget > 0) {
        const node = new TokenBudgetNode(repoCfg.token_budget, repoConfigPath, "Repo");
        this.tokenBudgetCache = { at: now, node };
        return node;
      }
    } catch (err: any) {
      if (err?.code !== "ENOENT") {
        const node = new TokenBudgetNode(undefined, repoConfigPath, "Unavailable");
        this.tokenBudgetCache = { at: now, node };
        return node;
      }
    }

    const configPath = getConfigPath();
    try {
      const content = await fs.readFile(configPath, "utf8");
      const budget = parseTokenBudgetConfig(content);
      if (typeof budget === "number") {
        const node = new TokenBudgetNode(budget, configPath, "Global");
        this.tokenBudgetCache = { at: now, node };
        return node;
      }
    } catch {
      // fall through to default
    }

    const node = new TokenBudgetNode(2500, repoConfigPath, "Default");
    this.tokenBudgetCache = { at: now, node };
    return node;
  }

  private async getWorkspaceNode(): Promise<WorkspaceNode> {
    const now = Date.now();
    if (this.workspaceCache && now - this.workspaceCache.at < 5000) {
      return this.workspaceCache.node;
    }
    const value = vscode.workspace.getConfiguration("mempack").get<string>("workspace") || "";
    const node = new WorkspaceNode(value);
    this.workspaceCache = { at: now, node };
    return node;
  }

  private async getDefaultThreadNode(): Promise<DefaultThreadNode> {
    const now = Date.now();
    if (this.defaultThreadCache && now - this.defaultThreadCache.at < 5000) {
      return this.defaultThreadCache.node;
    }
    const value =
      vscode.workspace.getConfiguration("mempack").get<string>("defaultThread") || "T-SESSION";
    const node = new DefaultThreadNode(value);
    this.defaultThreadCache = { at: now, node };
    return node;
  }

  private async getThreads(cwd: string): Promise<ThreadItem[]> {
    const now = Date.now();
    if (this.threadsCache && now - this.threadsCache.at < 5000) {
      return this.threadsCache.items;
    }
    const items = await this.client.threads(cwd);
    this.threadsCache = { at: now, items };
    return items;
  }

  private async getRecent(cwd: string, limit: number): Promise<RecentMemoryItem[]> {
    const now = Date.now();
    if (this.recentCache && now - this.recentCache.at < 5000) {
      return this.recentCache.items.slice(0, limit);
    }
    const items = await this.client.recent(cwd, limit);
    this.recentCache = { at: now, items };
    return items;
  }

  private async getRecentSessions(cwd: string, limit: number): Promise<SessionItem[]> {
    const now = Date.now();
    if (this.recentSessionsCache && now - this.recentSessionsCache.at < 5000) {
      return this.recentSessionsCache.items.slice(0, limit);
    }
    const items = await this.client.sessions(cwd, { limit });
    this.recentSessionsCache = { at: now, items };
    return items;
  }

  private async getNeedsSummarySessions(cwd: string, limit: number): Promise<SessionItem[]> {
    const now = Date.now();
    if (this.needsSummaryCache && now - this.needsSummaryCache.at < 5000) {
      return this.needsSummaryCache.items.slice(0, limit);
    }
    const items = await this.client.sessions(cwd, { needsSummary: true, limit });
    this.needsSummaryCache = { at: now, items };
    return items;
  }

  private async getNeedsSummaryCount(cwd: string): Promise<number> {
    try {
      return await this.client.sessionsCount(cwd, true);
    } catch {
      return 0;
    }
  }
}

export type MempackNode =
  | StatusRootNode
  | SettingsRootNode
  | SearchRootNode
  | SearchContextNode
  | ExplainSearchNode
  | HealthNode
  | RepoNode
  | McpServerNode
  | McpWritesNode
  | EmbeddingsNode
  | SessionsOnCommitNode
  | TokenBudgetNode
  | WorkspaceNode
  | DefaultThreadNode
  | RecentSessionsRootNode
  | NeedsSummaryRootNode
  | ThreadsRootNode
  | ThreadNode
  | RecentRootNode
  | MemoryNode
  | SessionNode
  | MessageNode;

class StatusRootNode extends vscode.TreeItem {
  constructor() {
    super("Status", vscode.TreeItemCollapsibleState.Expanded);
    this.contextValue = "mempackStatusRoot";
    this.iconPath = brandIcon("pulse");
  }
}

class SettingsRootNode extends vscode.TreeItem {
  constructor() {
    super("Settings", vscode.TreeItemCollapsibleState.Expanded);
    this.contextValue = "mempackSettingsRoot";
    this.iconPath = brandIcon("gear");
  }
}

class SearchRootNode extends vscode.TreeItem {
  constructor() {
    super("Search", vscode.TreeItemCollapsibleState.Collapsed);
    this.contextValue = "mempackSearchRoot";
    this.iconPath = brandIcon("search");
  }
}

class SearchContextNode extends vscode.TreeItem {
  constructor() {
    super("Get Context", vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackSearchContext";
    this.iconPath = brandIcon("search");
    this.command = { command: "mempack.getContext", title: "Get Context" };
    this.tooltip = "Search memories and chunks to build agent context.";
  }
}

class ExplainSearchNode extends vscode.TreeItem {
  constructor() {
    super("Explain Search", vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackExplainSearch";
    this.iconPath = brandIcon("debug");
    this.command = { command: "mempack.explain", title: "Explain Search" };
    this.tooltip = "Explain ranking and budget decisions for a query.";
  }
}

class HealthNode extends vscode.TreeItem {
  constructor(label: string, detail?: string, checkedAt?: number) {
    super(label, vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackHealth";
    this.command = { command: "mempack.doctor", title: "Doctor" };
    this.description = detail || "";
    const iconName = label.toLowerCase().includes("ok")
      ? "check"
      : label.toLowerCase().includes("issues")
        ? "warning"
        : "circle-slash";
    this.iconPath = iconName === "check" ? brandIcon(iconName) : new vscode.ThemeIcon(iconName);
    const checked = checkedAt ? new Date(checkedAt).toLocaleTimeString() : "";
    this.tooltip = `${label}${detail ? `\n${detail}` : ""}${checked ? `\nLast checked: ${checked}` : ""}`;
  }
}

class RepoNode extends vscode.TreeItem {
  constructor(
    repoRoot: string,
    repoID: string,
    memoryDBPathOrDetail?: string,
    memoryDBSizeBytes?: number,
    memoryDBExists?: boolean,
    detail?: string
  ) {
    const memoryDBPath =
      memoryDBSizeBytes === undefined && memoryDBExists === undefined && detail === undefined
        ? undefined
        : memoryDBPathOrDetail;
    const fallbackDetail =
      memoryDBSizeBytes === undefined && memoryDBExists === undefined && detail === undefined
        ? memoryDBPathOrDetail
        : detail;

    const name = basenameSafe(repoRoot);
    super(`Repo: ${name}`, vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackRepo";
    this.iconPath = brandIcon("repo");
    this.command = { command: "mempack.refresh", title: "Refresh" };
    const lines = [`Repo root: ${repoRoot}`];
    if (repoID) {
      lines.push(`Repo id: ${repoID}`);
    }
    if (memoryDBPath) {
      lines.push(`Memory DB: ${memoryDBPath}`);
    }
    if (memoryDBSizeBytes !== undefined) {
      lines.push(`Memory size: ${formatBytes(memoryDBSizeBytes)}`);
      this.description = `${formatBytes(memoryDBSizeBytes)} · ${compactPath(memoryDBPath || "memory.db")}`;
    } else if (memoryDBExists === false) {
      lines.push("Memory size: DB not initialized");
      this.description = "No memory DB";
    } else if (memoryDBPath) {
      this.description = compactPath(memoryDBPath);
    }
    if (fallbackDetail) {
      lines.push(fallbackDetail);
    }
    this.tooltip = lines.join("\n");
  }
}

class McpWritesNode extends vscode.TreeItem {
  constructor(mode: McpWritesMode | undefined, configPath: string, detail?: string) {
    super("MCP Writes", vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackMcpWrites";
    this.command = { command: "mempack.configureMcpWrites", title: "Configure MCP Writes" };
    if (!mode) {
      this.description = detail || "Unavailable";
      this.iconPath = new vscode.ThemeIcon("warning");
      this.tooltip = detail || "Unable to read repo config.";
      return;
    }
    const label = mode === "off" ? "Off" : mode === "ask" ? "Ask" : "Auto";
    this.description = `${label}${detail ? ` · ${detail}` : ""}`;
    this.iconPath = mode === "off" ? new vscode.ThemeIcon("lock") : brandIcon("unlock");
    this.tooltip = `Controls whether MCP write tools are allowed.\nConfig: ${configPath}`;
  }
}

class McpServerNode extends vscode.TreeItem {
  constructor(running: boolean, pid?: number, detail?: string, unavailable = false) {
    super("Status", vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackMcpServer";
    if (unavailable) {
      this.description = "Unavailable";
      this.iconPath = new vscode.ThemeIcon("warning");
      this.tooltip = detail || "Unable to check MCP status.";
      this.command = { command: "mempack.startMcpServer", title: "Start MCP Server" };
      return;
    }
    this.description = running ? "Running" : "Stopped";
    this.iconPath = running ? brandIcon("play-circle") : new vscode.ThemeIcon("circle-slash");
    this.tooltip = detail || (running ? `Running${pid ? ` (pid=${pid})` : ""}` : "Not running");
    this.command = running
      ? { command: "mempack.stopMcpServer", title: "Stop MCP Server" }
      : { command: "mempack.startMcpServer", title: "Start MCP Server" };
  }
}



class EmbeddingsNode extends vscode.TreeItem {
  constructor(status?: EmbedStatusResponse, error?: string) {
    super("Embeddings", vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackEmbeddings";
    this.command = { command: "mempack.configureEmbeddings", title: "Configure Embeddings" };
    if (!status) {
      this.description = "Unavailable";
      this.iconPath = new vscode.ThemeIcon("warning");
      this.tooltip = error || "Unable to query embedding status.";
      return;
    }

    const provider = (status.vectors?.provider_configured || status.provider || "").trim();
    if (provider.toLowerCase() === "none") {
      this.description = "Off";
      this.iconPath = new vscode.ThemeIcon("circle-slash");
      this.tooltip = "Embeddings are disabled.";
      return;
    }
    const available = status.vectors?.available === true;
    const providerLabel = provider === "" ? "Auto" : provider;
    this.description = `${providerLabel} (${available ? "Available" : "Unavailable"})`;
    this.iconPath = available ? brandIcon("check") : new vscode.ThemeIcon("warning");
    const configured = status.vectors?.configured ? "Yes" : "No";
    const enabled = status.vectors?.enabled ? "Yes" : "No";
    const reason = status.vectors?.reason ? `Reason: ${status.vectors.reason}` : "";
    const how = Array.isArray(status.vectors?.how_to_fix) ? status.vectors.how_to_fix.join("\n") : "";
    this.tooltip = [
      `Provider: ${providerLabel}`,
      status.model ? `Model: ${status.model}` : status.vectors?.model_configured ? `Model: ${status.vectors.model_configured}` : "",
      `Configured: ${configured}`,
      `Available: ${available ? "Yes" : "No"}`,
      `Used: ${enabled}`,
      reason,
      how
    ]
      .filter((line) => line && line.trim() !== "")
      .join("\n");
  }
}

class SessionsOnCommitNode extends vscode.TreeItem {
  constructor(enabled: boolean) {
    super("Auto Capture", vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackSessionsOnCommit";
    this.command = { command: "mempack.configureIntentCapture", title: "Configure Intent Capture" };
    this.description = enabled ? "On" : "Off";
    this.iconPath = enabled ? brandIcon("record") : new vscode.ThemeIcon("circle-slash");
    this.tooltip = enabled
      ? "Creates session memories automatically from meaningful edits in this repo."
      : "Auto-capture is disabled.";
  }
}

class TokenBudgetNode extends vscode.TreeItem {
  constructor(value: number | undefined, configPath: string, detail?: string) {
    super("Token Budget", vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackTokenBudget";
    this.command = { command: "mempack.configureTokenBudget", title: "Configure Token Budget" };
    if (value === undefined) {
      this.description = detail || "Unavailable";
      this.iconPath = new vscode.ThemeIcon("warning");
      this.tooltip = detail || "Unable to read config.";
      return;
    }
    if (detail) {
      this.description = `${value} · ${detail}`;
    } else {
      this.description = `${value}`;
    }
    this.iconPath = brandIcon("dashboard");
    this.tooltip = `Token budget: ${value}\nControls max tokens in context output.\n${configPath}`;
  }
}

class WorkspaceNode extends vscode.TreeItem {
  constructor(value: string) {
    super("Workspace", vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackWorkspace";
    this.command = { command: "mempack.configureWorkspace", title: "Configure Workspace" };
    const trimmed = value.trim();
    this.description = trimmed === "" ? "Default" : trimmed;
    this.iconPath = brandIcon("folder");
    this.tooltip =
      trimmed === ""
        ? "Using mempack default workspace."
        : `Using workspace: ${trimmed}`;
  }
}

class DefaultThreadNode extends vscode.TreeItem {
  constructor(value: string) {
    super("Default Thread", vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackDefaultThread";
    this.command = { command: "mempack.configureDefaultThread", title: "Configure Default Thread" };
    const trimmed = value.trim();
    this.description = trimmed === "" ? "T-SESSION" : trimmed;
    this.iconPath = brandIcon("tag");
    this.tooltip = `Default thread for new memories: ${trimmed === "" ? "T-SESSION" : trimmed}`;
  }
}

class ThreadsRootNode extends vscode.TreeItem {
  constructor() {
    super("Memory Threads", vscode.TreeItemCollapsibleState.Collapsed);
    this.contextValue = "mempackThreads";
    this.iconPath = brandIcon("comment-discussion");
    this.command = { command: "mempack.openThreadsUI", title: "Open Threads UI" };
  }
}

class RecentSessionsRootNode extends vscode.TreeItem {
  constructor() {
    super("Recent Sessions", vscode.TreeItemCollapsibleState.Collapsed);
    this.contextValue = "mempackRecentSessions";
    this.iconPath = brandIcon("book");
  }
}

class RecentRootNode extends vscode.TreeItem {
  constructor() {
    super("Recent", vscode.TreeItemCollapsibleState.Collapsed);
    this.contextValue = "mempackRecent";
    this.iconPath = brandIcon("history");
    this.command = { command: "mempack.openRecentUI", title: "Open Recent UI" };
  }
}

class NeedsSummaryRootNode extends vscode.TreeItem {
  constructor(count: number) {
    super("Needs Summary", vscode.TreeItemCollapsibleState.Collapsed);
    this.contextValue = "mempackNeedsSummary";
    this.description = count > 0 ? `${count}` : "";
    this.iconPath = brandIcon("checklist");
    this.tooltip =
      count > 0 ? `${count} sessions need a summary.` : "No sessions need a summary.";
  }
}

class ThreadNode extends vscode.TreeItem {
  thread: ThreadItem;
  constructor(thread: ThreadItem) {
    super(formatThreadLabel(thread), vscode.TreeItemCollapsibleState.Collapsed);
    this.thread = thread;
    this.description = thread.memory_count ? `${thread.memory_count}` : "";
    this.contextValue = "mempackThread";
    this.iconPath = brandIcon("comment");
  }
}

class MessageNode extends vscode.TreeItem {
  constructor(message: string, description?: string, iconName = "error") {
    super(message, vscode.TreeItemCollapsibleState.None);
    this.contextValue = "mempackMessage";
    this.description = description || "";
    this.iconPath = new vscode.ThemeIcon(iconName);
  }
}

export type MemoryNodeData = ThreadMemoryBrief | RecentMemoryItem;

export class MemoryNode extends vscode.TreeItem {
  memory: MemoryNodeData;

  constructor(memory: MemoryNodeData) {
    super(memory.title || "(untitled)", vscode.TreeItemCollapsibleState.None);
    this.memory = memory;
    this.description = truncateSummary(memory.summary || "", 60);
    this.tooltip = buildMemoryTooltip(memory);
    this.contextValue = "mempackMemory";
    this.iconPath = brandIcon("note");
    this.command = {
      command: "mempack.openMemory",
      title: "Open Memory",
      arguments: [this]
    };
  }
}

export class SessionNode extends vscode.TreeItem {
  session: SessionItem;

  constructor(session: SessionItem) {
    super(session.title || "(untitled)", vscode.TreeItemCollapsibleState.None);
    this.session = session;
    this.description = formatSessionDescription(session);
    this.tooltip = buildSessionTooltip(session);
    this.contextValue = "mempackSession";
    this.iconPath = brandIcon("book");
    this.command = {
      command: "mempack.openSessionDiff",
      title: "Open Diff",
      arguments: [this]
    };
  }
}

function formatThreadLabel(thread: ThreadItem): string {
  if (thread.title && thread.title !== thread.thread_id) {
    return `${thread.thread_id} — ${thread.title}`;
  }
  return thread.thread_id;
}


function truncateSummary(summary: string, max: number): string {
  const clean = summary.replace(/\s+/g, " ").trim();
  if (clean.length <= max) {
    return clean;
  }
  return clean.slice(0, max - 3) + "...";
}

function buildMemoryTooltip(memory: MemoryNodeData): string {
  const summary = memory.summary ? memory.summary.trim() : "";
  const lines = [memory.title];
  if ("thread_id" in memory) {
    lines.push(memory.thread_id);
  }
  if (summary !== "") {
    lines.push(summary);
  }
  return lines.join("\n");
}

function buildSessionTooltip(session: SessionItem): string {
  const lines = [session.title];
  if (session.thread_id) {
    lines.push(session.thread_id);
  }
  if (session.anchor_commit) {
    lines.push(session.anchor_commit);
  }
  if (session.summary && session.summary.trim() !== "") {
    lines.push(session.summary.trim());
  }
  if (session.created_at) {
    lines.push(session.created_at);
  }
  return lines.join("\n");
}

function buildErrorNode(section: string, err: unknown): MessageNode {
  const message = formatErrorMessage(err);
  const hint = inferErrorHint(message);
  const node = new MessageNode(`${section} unavailable`, hint, "warning");
  node.tooltip = message;
  return node;
}

function formatErrorMessage(err: unknown): string {
  const message = err instanceof Error ? err.message : String(err);
  return message.replace(/\s+/g, " ").trim();
}

function inferErrorHint(message: string): string {
  const lower = message.toLowerCase();
  if (lower.includes("repo detection") || lower.includes("db not initialized")) {
    return "Run: Mempack Init";
  }
  if (lower.includes("mem command failed")) {
    return "Check mem binary path";
  }
  if (lower.includes("not found") && lower.includes("mem")) {
    return "Set mempack.binaryPath";
  }
  return "See Output: Mempack";
}

function basenameSafe(value: string): string {
  const cleaned = String(value || "").replace(/\\/g, "/").replace(/\/+$/, "");
  const parts = cleaned.split("/");
  const last = parts[parts.length - 1];
  return last && last.trim() !== "" ? last : cleaned || "(unknown)";
}

function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) {
    return "Unknown";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  let size = value;
  let unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit += 1;
  }
  if (unit === 0) {
    return `${Math.round(size)} ${units[unit]}`;
  }
  return `${size.toFixed(1)} ${units[unit]}`;
}

function compactPath(value: string): string {
  const path = String(value || "").replace(/\\/g, "/");
  if (path.length <= 48) {
    return path;
  }
  return `...${path.slice(-45)}`;
}

function formatSessionDescription(session: SessionItem): string {
  const parts: string[] = [];
  if (session.thread_id) {
    parts.push(session.thread_id);
  }
  if (session.anchor_commit) {
    parts.push(session.anchor_commit.slice(0, 7));
  }
  const rel = session.created_at ? formatRelativeTime(session.created_at) : "";
  if (rel) {
    parts.push(rel);
  }
  return parts.join(" · ");
}

function formatRelativeTime(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  const diffMs = Date.now() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  if (diffSec < 60) {
    return `${diffSec}s`;
  }
  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) {
    return `${diffMin}m`;
  }
  const diffHr = Math.floor(diffMin / 60);
  if (diffHr < 48) {
    return `${diffHr}h`;
  }
  const diffDay = Math.floor(diffHr / 24);
  return `${diffDay}d`;
}
