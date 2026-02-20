import { spawn } from "child_process";
import * as vscode from "vscode";
import * as fs from "fs";
import * as path from "path";
import { callMcpTool, McpToolCallResult } from "./mcp_stdio";
import {
  AddMemoryResponse,
  ContextPack,
  DoctorReport,
  EmbedStatusResponse,
  RecentMemoryItem,
  SessionCount,
  SessionItem,
  ShowResponse,
  ThreadItem,
  ThreadShowResponse,
  UpdateMemoryResponse
} from "./types";

export type LastMcpSpawn = {
  pid?: number;
  tool: string;
  repo: string;
  startedAtMs: number;
  endedAtMs: number;
  durationMs: number;
  ok: boolean;
  error?: string;
};

function execFileStrict(
  binary: string,
  args: string[],
  options: { cwd: string; timeoutMs: number; maxBuffer: number }
): Promise<{ stdout: string; stderr: string }> {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, args, {
      cwd: options.cwd,
      stdio: ["ignore", "pipe", "pipe"],
      windowsHide: true
    });

    let stdout = "";
    let stderr = "";
    let done = false;

    const finish = (err?: any, code?: number | null, signal?: NodeJS.Signals | null) => {
      if (done) {
        return;
      }
      done = true;
      clearTimeout(timer);
      child.removeAllListeners();
      child.stdout?.removeAllListeners();
      child.stderr?.removeAllListeners();

      if (err) {
        const e: any = err instanceof Error ? err : new Error(String(err));
        if (typeof e.code === "undefined") {
          e.code = code;
        }
        e.signal = signal;
        e.stdout = stdout;
        e.stderr = stderr;
        reject(e);
        return;
      }

      if (code !== 0) {
        const e: any = new Error(`exit ${code}`);
        e.code = code;
        e.signal = signal;
        e.stdout = stdout;
        e.stderr = stderr;
        reject(e);
        return;
      }

      resolve({ stdout, stderr });
    };

    const timer = setTimeout(() => {
      // Reject even if the process is stuck in uninterruptible IO (SIGKILL won't work).
      const e: any = new Error(`timeout after ${options.timeoutMs}ms`);
      e.code = "ETIMEDOUT";
      e.stdout = stdout;
      e.stderr = stderr;
      try {
        child.kill("SIGKILL");
      } catch {
        // ignore
      }
      finish(e, null, null);
    }, options.timeoutMs);

    const append = (kind: "stdout" | "stderr", chunk: Buffer) => {
      const text = chunk.toString("utf8");
      if (kind === "stdout") {
        stdout += text;
      } else {
        stderr += text;
      }
      if (stdout.length+stderr.length > options.maxBuffer) {
        const e: any = new Error("maxBuffer exceeded");
        e.code = "EMAXBUFFER";
        e.stdout = stdout;
        e.stderr = stderr;
        try {
          child.kill("SIGKILL");
        } catch {
          // ignore
        }
        finish(e, null, null);
      }
    };

    child.on("error", (err) => finish(err, null, null));
    child.stdout?.on("data", (chunk: Buffer) => append("stdout", chunk));
    child.stderr?.on("data", (chunk: Buffer) => append("stderr", chunk));
    child.on("close", (code, signal) => finish(undefined, code, signal));
  });
}

export class MempackClient {
  private output: vscode.OutputChannel;
  private lastMcpSpawn?: LastMcpSpawn;

  constructor(output: vscode.OutputChannel) {
    this.output = output;
  }

  dispose(): void {
    // no-op
  }

  get binaryPath(): string {
    return vscode.workspace.getConfiguration("mempack").get<string>("binaryPath") || "mem";
  }

  get workspaceName(): string {
    return vscode.workspace.getConfiguration("mempack").get<string>("workspace") || "";
  }

  get defaultThread(): string {
    return vscode.workspace.getConfiguration("mempack").get<string>("defaultThread") || "T-SESSION";
  }

  get recentLimit(): number {
    return vscode.workspace.getConfiguration("mempack").get<number>("recentLimit") || 10;
  }

  get timeoutMs(): number {
    return vscode.workspace.getConfiguration("mempack").get<number>("commandTimeoutMs") || 10000;
  }

  get writeTransport(): "mcp_first" | "cli" {
    const mode = vscode.workspace
      .getConfiguration("mempack")
      .get<string>("writeTransport", "mcp_first")
      .trim()
      .toLowerCase();
    if (mode === "cli") {
      return "cli";
    }
    return "mcp_first";
  }

  withWorkspace(args: string[]): string[] {
    if (this.workspaceName.trim() === "") {
      return args;
    }
    return [...args, "--workspace", this.workspaceName.trim()];
  }

  private normalizeRepoPath(cwd: string): string {
    const resolved = path.resolve(cwd);
    try {
      return fs.realpathSync(resolved);
    } catch {
      return resolved;
    }
  }

  withRepo(args: string[], cwd: string): string[] {
    if (args.includes("--repo")) {
      return args;
    }
    const repo = this.normalizeRepoPath(cwd);
    return [...args, "--repo", repo];
  }

  private async mcpToolCall(
    cwd: string,
    toolName: string,
    toolArgs: Record<string, any>
  ): Promise<McpToolCallResult> {
    const binary = this.binaryPath;
    const repo = this.normalizeRepoPath(cwd);
    const serverArgs = ["mcp", "--require-repo", "--repo", repo];
    this.output.appendLine(`${binary} ${serverArgs.join(" ")}  # MCP tool: ${toolName}`);
    const startedAtMs = Date.now();
    let spawnedPid: number | undefined;

    try {
      const result = await callMcpTool(binary, serverArgs, toolName, toolArgs, {
        cwd: repo,
        timeoutMs: this.timeoutMs,
        onLog: (line) => this.output.appendLine(line),
        onSpawn: (pid) => {
          spawnedPid = pid;
          this.output.appendLine(`[mcp spawn] pid=${pid ?? "unknown"} tool=${toolName}`);
        }
      });
      const endedAtMs = Date.now();
      this.lastMcpSpawn = {
        pid: spawnedPid,
        tool: toolName,
        repo,
        startedAtMs,
        endedAtMs,
        durationMs: Math.max(0, endedAtMs - startedAtMs),
        ok: true
      };
      return result;
    } catch (err) {
      const endedAtMs = Date.now();
      this.lastMcpSpawn = {
        pid: spawnedPid,
        tool: toolName,
        repo,
        startedAtMs,
        endedAtMs,
        durationMs: Math.max(0, endedAtMs - startedAtMs),
        ok: false,
        error: err instanceof Error ? err.message : String(err)
      };
      throw err;
    }
  }

  getLastMcpSpawn(): LastMcpSpawn | undefined {
    return this.lastMcpSpawn;
  }

  private extractFirstText(result: McpToolCallResult): string {
    const content = Array.isArray(result.content) ? result.content : [];
    for (const item of content) {
      if (item && item.type === "text" && typeof item.text === "string") {
        return item.text;
      }
    }
    return "";
  }

  private extractStructured<T>(result: McpToolCallResult): T | undefined {
    if (result.structuredContent && typeof result.structuredContent === "object") {
      return result.structuredContent as T;
    }
    const content = Array.isArray(result.content) ? result.content : [];
    for (const item of content) {
      if (!item || item.type !== "text" || typeof item.text !== "string") {
        continue;
      }
      const raw = item.text.trim();
      if (raw === "" || (!raw.startsWith("{") && !raw.startsWith("["))) {
        continue;
      }
      try {
        return JSON.parse(raw) as T;
      } catch {
        continue;
      }
    }
    return undefined;
  }

  async doctor(cwd: string): Promise<DoctorReport> {
    return this.runJson<DoctorReport>(this.withRepo(["doctor", "--json"], cwd), cwd);
  }

  async embedStatus(cwd: string): Promise<EmbedStatusResponse> {
    return this.runJson<EmbedStatusResponse>(
      this.withRepo(this.withWorkspace(["embed", "status"]), cwd),
      cwd
    );
  }

  async threads(cwd: string): Promise<ThreadItem[]> {
    return this.runJson<ThreadItem[]>(
      this.withRepo(this.withWorkspace(["threads"]), cwd),
      cwd
    );
  }

  async thread(cwd: string, threadId: string): Promise<ThreadShowResponse> {
    return this.runJson<ThreadShowResponse>(
      this.withRepo(this.withWorkspace(["thread", threadId]), cwd),
      cwd
    );
  }

  async recent(cwd: string, limit: number): Promise<RecentMemoryItem[]> {
    return this.runJson<RecentMemoryItem[]>(
      this.withRepo(this.withWorkspace(["recent", "--limit", String(limit)]), cwd),
      cwd
    );
  }

  async sessions(
    cwd: string,
    options?: { limit?: number; needsSummary?: boolean }
  ): Promise<SessionItem[]> {
    const args = ["sessions", "--format", "json"];
    if (options?.needsSummary) {
      args.push("--needs-summary");
    }
    if (typeof options?.limit === "number") {
      args.push("--limit", String(options.limit));
    }
    return this.runJson<SessionItem[]>(this.withRepo(this.withWorkspace(args), cwd), cwd);
  }

  async sessionsCount(cwd: string, needsSummary: boolean): Promise<number> {
    const args = ["sessions", "--format", "json", "--count"];
    if (needsSummary) {
      args.push("--needs-summary");
    }
    const result = await this.runJson<SessionCount>(this.withRepo(this.withWorkspace(args), cwd), cwd);
    return result.count;
  }

  async addMemory(
    cwd: string,
    threadId: string | undefined,
    title: string,
    summary: string,
    tags?: string,
    entities?: string[]
  ): Promise<AddMemoryResponse> {
    if (this.writeTransport === "mcp_first") {
      try {
        const repo = this.normalizeRepoPath(cwd);
        const toolArgs: Record<string, any> = {
          title,
          summary,
          repo,
          confirmed: true
        };
        if (this.workspaceName.trim() !== "") {
          toolArgs.workspace = this.workspaceName.trim();
        }
        if (threadId && threadId.trim() !== "") {
          toolArgs.thread = threadId.trim();
        }
        if (tags && tags.trim() !== "") {
          toolArgs.tags = tags.trim();
        }
        const entitiesCsv = joinCsv(entities);
        if (entitiesCsv) {
          toolArgs.entities = entitiesCsv;
        }
        const res = await this.mcpToolCall(cwd, "mempack_add_memory", toolArgs);
        if (res.isError) {
          throw new Error(this.extractFirstText(res) || "MCP add_memory failed");
        }
        const structured = this.extractStructured<AddMemoryResponse>(res);
        if (structured && structured.id) {
          return structured;
        }
        throw new Error("MCP add_memory returned no structured content");
      } catch (err) {
        this.output.appendLine(
          `[mcp fallback] add_memory failed: ${err instanceof Error ? err.message : String(err)}`
        );
      }
    }

    const args = ["add", "--title", title, "--summary", summary];
    if (threadId && threadId.trim() !== "") {
      args.push("--thread", threadId.trim());
    }
    if (tags && tags.trim() !== "") {
      args.push("--tags", tags.trim());
    }
    const entitiesCsv = joinCsv(entities);
    if (entitiesCsv) {
      args.push("--entities", entitiesCsv);
    }
    return this.runJson<AddMemoryResponse>(this.withRepo(this.withWorkspace(args), cwd), cwd);
  }

  async supersede(
    cwd: string,
    id: string,
    title: string,
    summary: string,
    tags?: string
  ): Promise<void> {
    const args = ["supersede", id, "--title", title, "--summary", summary];
    if (tags && tags.trim() !== "") {
      args.push("--tags", tags.trim());
    }
    await this.runText(this.withRepo(this.withWorkspace(args), cwd), cwd);
  }

  async updateMemory(
    cwd: string,
    id: string,
    options: {
      title?: string;
      summary?: string;
      tags?: string[];
      tagsAdd?: string[];
      tagsRemove?: string[];
      entities?: string[];
      entitiesAdd?: string[];
      entitiesRemove?: string[];
    }
  ): Promise<UpdateMemoryResponse> {
    if (this.writeTransport === "mcp_first") {
      try {
        const repo = this.normalizeRepoPath(cwd);
        const toolArgs: Record<string, any> = {
          id,
          repo,
          confirmed: true
        };
        if (this.workspaceName.trim() !== "") {
          toolArgs.workspace = this.workspaceName.trim();
        }
        if (typeof options.title === "string") {
          toolArgs.title = options.title;
        }
        if (typeof options.summary === "string") {
          toolArgs.summary = options.summary;
        }
        const tags = joinCsv(options.tags);
        if (tags) {
          toolArgs.tags = tags;
        }
        const tagsAdd = joinCsv(options.tagsAdd);
        if (tagsAdd) {
          toolArgs.tags_add = tagsAdd;
        }
        const tagsRemove = joinCsv(options.tagsRemove);
        if (tagsRemove) {
          toolArgs.tags_remove = tagsRemove;
        }
        const entities = joinCsv(options.entities);
        if (entities) {
          toolArgs.entities = entities;
        }
        const entitiesAdd = joinCsv(options.entitiesAdd);
        if (entitiesAdd) {
          toolArgs.entities_add = entitiesAdd;
        }
        const entitiesRemove = joinCsv(options.entitiesRemove);
        if (entitiesRemove) {
          toolArgs.entities_remove = entitiesRemove;
        }
        const res = await this.mcpToolCall(cwd, "mempack_update_memory", toolArgs);
        if (res.isError) {
          throw new Error(this.extractFirstText(res) || "MCP update_memory failed");
        }
        const structured = this.extractStructured<UpdateMemoryResponse>(res);
        if (structured && structured.id) {
          return structured;
        }
        throw new Error("MCP update_memory returned no structured content");
      } catch (err) {
        this.output.appendLine(
          `[mcp fallback] update_memory failed: ${err instanceof Error ? err.message : String(err)}`
        );
      }
    }

    const args = ["update", id];
    if (typeof options.title === "string") {
      args.push("--title", options.title);
    }
    if (typeof options.summary === "string") {
      args.push("--summary", options.summary);
    }
    if (options.tags && options.tags.length > 0) {
      args.push("--tags", options.tags.join(","));
    }
    if (options.tagsAdd && options.tagsAdd.length > 0) {
      args.push("--tags-add", options.tagsAdd.join(","));
    }
    if (options.tagsRemove && options.tagsRemove.length > 0) {
      args.push("--tags-remove", options.tagsRemove.join(","));
    }
    if (options.entities && options.entities.length > 0) {
      args.push("--entities", options.entities.join(","));
    }
    if (options.entitiesAdd && options.entitiesAdd.length > 0) {
      args.push("--entities-add", options.entitiesAdd.join(","));
    }
    if (options.entitiesRemove && options.entitiesRemove.length > 0) {
      args.push("--entities-remove", options.entitiesRemove.join(","));
    }
    return this.runJson<UpdateMemoryResponse>(this.withRepo(this.withWorkspace(args), cwd), cwd);
  }

  async checkpoint(cwd: string, reason: string, stateJson: string, threadId?: string): Promise<void> {
    if (this.writeTransport === "mcp_first") {
      try {
        const repo = this.normalizeRepoPath(cwd);
        const toolArgs: Record<string, any> = {
          reason,
          state_json: stateJson,
          repo,
          confirmed: true
        };
        if (this.workspaceName.trim() !== "") {
          toolArgs.workspace = this.workspaceName.trim();
        }
        if (threadId && threadId.trim() !== "") {
          toolArgs.thread = threadId.trim();
        }
        const res = await this.mcpToolCall(cwd, "mempack_checkpoint", toolArgs);
        if (res.isError) {
          throw new Error(this.extractFirstText(res) || "MCP checkpoint failed");
        }
        return;
      } catch (err) {
        this.output.appendLine(
          `[mcp fallback] checkpoint failed: ${err instanceof Error ? err.message : String(err)}`
        );
      }
    }

    const args = ["checkpoint", "--reason", reason, "--state-json", stateJson];
    if (threadId && threadId.trim() !== "") {
      args.push("--thread", threadId.trim());
    }
    await this.runText(this.withRepo(this.withWorkspace(args), cwd), cwd);
  }

  async getContextPack(cwd: string, query: string): Promise<{ pack: ContextPack; prompt: string }> {
    const repo = this.normalizeRepoPath(cwd);
    const workspace = this.workspaceName.trim();
    const args: Record<string, any> = { query, repo, format: "json" };
    if (workspace !== "") {
      args.workspace = workspace;
    }
    try {
      const res = await this.mcpToolCall(cwd, "mempack_get_context", args);
      if (res.isError) {
        throw new Error(this.extractFirstText(res) || "MCP get_context failed");
      }
      const prompt = this.extractFirstText(res);
      const pack = this.extractStructured<ContextPack>(res);
      if (!pack) {
        throw new Error("MCP get_context returned no structured content");
      }
      return { pack, prompt };
    } catch (err) {
      this.output.appendLine(`[mcp fallback] get_context failed: ${err instanceof Error ? err.message : String(err)}`);
      const pack = await this.runJson<ContextPack>(
        this.withRepo(this.withWorkspace(["get", query, "--format", "json"]), cwd),
        cwd
      );
      const prompt = await this.runText(
        this.withRepo(this.withWorkspace(["get", query, "--format", "prompt"]), cwd),
        cwd
      );
      return { pack, prompt };
    }
  }

  async getContext(cwd: string, query: string): Promise<string> {
    const res = await this.getContextPack(cwd, query);
    return res.prompt;
  }

  async explainReport(cwd: string, query: string): Promise<any> {
    const repo = this.normalizeRepoPath(cwd);
    const workspace = this.workspaceName.trim();
    const args: Record<string, any> = { query, repo };
    if (workspace !== "") {
      args.workspace = workspace;
    }
    try {
      const res = await this.mcpToolCall(cwd, "mempack_explain", args);
      if (res.isError) {
        throw new Error(this.extractFirstText(res) || "MCP explain failed");
      }
      const report = this.extractStructured<any>(res);
      if (!report) {
        throw new Error("MCP explain returned no structured content");
      }
      return report;
    } catch (err) {
      this.output.appendLine(`[mcp fallback] explain failed: ${err instanceof Error ? err.message : String(err)}`);
      return this.runJson<any>(this.withRepo(this.withWorkspace(["explain", query]), cwd), cwd);
    }
  }

  async init(cwd: string, noAgents: boolean, assistants: string[] = []): Promise<string> {
    const args = ["init"];
    if (noAgents) {
      args.push("--no-agents");
    } else if (assistants.length > 0) {
      args.push("--assistants", assistants.join(","));
    }
    const stdout = await this.runText(args, cwd);
    if (stdout.trim() !== "") {
      this.output.appendLine(stdout.trimEnd());
    }
    return stdout;
  }

  async writeAssistantFiles(cwd: string, assistants: string[], includeMemory = false): Promise<string> {
    if (assistants.length === 0) {
      return "";
    }
    const args = ["template", "agents", "--write", "--assistants", assistants.join(",")];
    if (!includeMemory) {
      args.push("--no-memory");
    }
    const stdout = await this.runText(args, cwd);
    if (stdout.trim() !== "") {
      this.output.appendLine(stdout.trimEnd());
    }
    return stdout;
  }

  async show(cwd: string, id: string): Promise<ShowResponse> {
    return this.runJson<ShowResponse>(
      this.withRepo(this.withWorkspace(["show", id]), cwd),
      cwd
    );
  }

  async forget(cwd: string, id: string): Promise<void> {
    await this.runText(this.withRepo(this.withWorkspace(["forget", id]), cwd), cwd);
  }

  async mcpStatus(cwd: string): Promise<{ running: boolean; pid?: number; message?: string }> {
    const binary = this.binaryPath;
    const args = ["mcp", "status"];
    this.output.appendLine(`${binary} ${args.join(" ")}`);
    try {
      const result = await execFileStrict(binary, args, {
        cwd,
        timeoutMs: this.timeoutMs,
        maxBuffer: 10 * 1024 * 1024
      });
      const text = (result.stdout || "").trim();
      return { running: true, pid: parsePid(text), message: text };
    } catch (err: any) {
      const stderr = typeof err?.stderr === "string" ? err.stderr : "";
      const stdout = typeof err?.stdout === "string" ? err.stdout : "";
      const detail = (stderr || stdout || err?.message || "Unknown error").trim();
      if (detail.toLowerCase().includes("not running")) {
        return { running: false, message: detail };
      }
      throw new Error(`mem command failed${typeof err?.code === "number" ? ` (code ${err.code})` : ""}: ${detail}`);
    }
  }

  async mcpManagerStatus(
    cwd: string
  ): Promise<{ running: boolean; pid?: number; port?: number; message?: string }> {
    const args = ["mcp", "manager", "status", "--json"];
    try {
      const result = await this.runJson<{ running: boolean; pid?: number; port?: number }>(
        args,
        cwd
      );
      return { running: result.running, pid: result.pid, port: result.port };
    } catch (err: any) {
      const message = err instanceof Error ? err.message : String(err);
      return { running: false, message };
    }
  }

  async mcpStart(cwd: string): Promise<string> {
    const binary = this.binaryPath;
    const repo = this.normalizeRepoPath(cwd);
    const args = ["mcp", "start", "--require-repo", "--repo", repo];
    this.output.appendLine(`${binary} ${args.join(" ")}`);
    return this.runText(args, cwd);
  }

  async mcpStop(cwd: string): Promise<string> {
    return this.runText(["mcp", "stop"], cwd);
  }

  private async runJson<T>(args: string[], cwd: string): Promise<T> {
    const stdout = await this.runText(args, cwd);
    try {
      return JSON.parse(stdout) as T;
    } catch (err) {
      throw new Error(`Failed to parse JSON from mem: ${String(err)}`);
    }
  }

  private async runText(args: string[], cwd: string): Promise<string> {
    const binary = this.binaryPath;
    this.output.appendLine(`${binary} ${args.join(" ")}`);
    try {
      const result = await execFileStrict(binary, args, {
        cwd,
        timeoutMs: this.timeoutMs,
        maxBuffer: 10 * 1024 * 1024
      });
      if (result.stderr && result.stderr.trim() !== "") {
        this.output.appendLine(result.stderr.trim());
      }
      return (result.stdout || "").trimEnd();
    } catch (err: any) {
      const stderr = typeof err?.stderr === "string" ? err.stderr : "";
      const stdout = typeof err?.stdout === "string" ? err.stdout : "";
      const detail = (stderr || stdout || err?.message || "Unknown error").trim();
      const code = typeof err?.code === "number" ? ` (code ${err.code})` : "";
      throw new Error(`mem command failed${code}: ${detail}`);
    }
  }
}

function parsePid(text: string): number | undefined {
  const match = text.match(/pid=(\d+)/i);
  if (!match) {
    return undefined;
  }
  const pid = Number(match[1]);
  return Number.isFinite(pid) ? pid : undefined;
}

function joinCsv(values?: string[]): string {
  if (!values || values.length === 0) {
    return "";
  }
  return values
    .map((value) => String(value || "").trim())
    .filter((value) => value !== "")
    .join(",");
}
