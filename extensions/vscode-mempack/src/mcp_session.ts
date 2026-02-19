import { spawn, ChildProcessWithoutNullStreams } from "child_process";
import * as readline from "readline";

export interface McpToolCallResult {
  content?: Array<{ type: string; text?: string }>;
  structuredContent?: any;
  isError?: boolean;
}

interface JsonRpcError {
  code: number;
  message: string;
  data?: any;
}

interface JsonRpcResponse {
  jsonrpc: string;
  id: number;
  result?: any;
  error?: JsonRpcError;
}

interface JsonRpcNotification {
  jsonrpc: string;
  method: string;
  params?: any;
}

export type McpSessionOptions = {
  binary: string;
  serverArgs: string[];
  cwd: string;
  timeoutMs: number;
  onLog?: (line: string) => void;
};

function isObject(value: unknown): value is Record<string, any> {
  return typeof value === "object" && value !== null;
}

function formatJsonRpcError(error: JsonRpcError): string {
  const msg = error.message || "Unknown error";
  const code = typeof error.code === "number" ? ` (code ${error.code})` : "";
  return `${msg}${code}`.trim();
}

export class McpStdioSession {
  private options: McpSessionOptions;
  private child?: ChildProcessWithoutNullStreams;
  private stderr = "";
  private closed = false;
  private pending = new Map<number, { resolve: (result: any) => void; reject: (err: Error) => void }>();
  private nextId = 1;
  private initPromise?: Promise<void>;

  constructor(options: McpSessionOptions) {
    this.options = options;
  }

  isRunning(): boolean {
    return Boolean(this.child) && !this.closed;
  }

  async start(): Promise<void> {
    if (this.initPromise) {
      return this.initPromise;
    }
    this.initPromise = this.startImpl();
    return this.initPromise;
  }

  private async startImpl(): Promise<void> {
    const log = this.options.onLog || (() => undefined);

    this.child = spawn(this.options.binary, this.options.serverArgs, {
      cwd: this.options.cwd,
      stdio: ["pipe", "pipe", "pipe"]
    });
    this.closed = false;
    this.stderr = "";

    const child = this.child;

    const rejectAll = (err: Error) => {
      for (const entry of this.pending.values()) {
        entry.reject(err);
      }
      this.pending.clear();
    };

    child.on("error", (err) => {
      rejectAll(new Error(`Failed to start MCP server: ${String(err)}`));
    });

    child.on("exit", (code, signal) => {
      const tail = this.stderr.trim();
      const detail = tail ? `: ${tail}` : "";
      const reason =
        typeof code === "number"
          ? `exit code ${code}`
          : signal
            ? `signal ${signal}`
            : "unknown exit";
      rejectAll(new Error(`MCP server exited (${reason})${detail}`));
      this.closed = true;
    });

    const rlErr = readline.createInterface({ input: child.stderr });
    rlErr.on("line", (line) => {
      this.stderr = `${this.stderr}${this.stderr ? "\n" : ""}${line}`;
      log(`[mcp stderr] ${line}`);
    });
    rlErr.on("close", () => {
      // no-op
    });

    const rlOut = readline.createInterface({ input: child.stdout });
    rlOut.on("line", (line) => {
      const trimmed = line.trim();
      if (trimmed === "") {
        return;
      }
      let msg: unknown;
      try {
        msg = JSON.parse(trimmed);
      } catch {
        return;
      }
      if (!isObject(msg)) {
        return;
      }

      const id = msg["id"];
      if (typeof id !== "number") {
        const maybeMethod = msg["method"];
        if (typeof maybeMethod === "string") {
          const note = msg as JsonRpcNotification;
          log(`[mcp notification] ${note.method}`);
        }
        return;
      }

      const entry = this.pending.get(id);
      if (!entry) {
        return;
      }
      this.pending.delete(id);

      const resp = msg as JsonRpcResponse;
      if (resp.error) {
        entry.reject(new Error(formatJsonRpcError(resp.error)));
        return;
      }
      entry.resolve(resp.result);
    });

    await this.withTimeout(
      this.sendRequest("initialize", {
        protocolVersion: "2025-06-18",
        capabilities: {},
        clientInfo: { name: "vscode-mempack", version: "0.0.0" }
      }),
      "MCP initialize"
    );
    this.sendNotification("notifications/initialized");
  }

  async callTool(toolName: string, toolArgs: Record<string, any>): Promise<McpToolCallResult> {
    await this.start();
    const result = await this.withTimeout(
      this.sendRequest("tools/call", { name: toolName, arguments: toolArgs }),
      `MCP tools/call ${toolName}`
    );
    return (result || {}) as McpToolCallResult;
  }

  dispose(): void {
    if (this.closed) {
      return;
    }
    this.closed = true;
    try {
      this.child?.stdin?.end();
    } catch {
      // ignore
    }
    try {
      this.child?.kill();
    } catch {
      // ignore
    }
    this.child = undefined;
    const err = new Error("MCP session disposed");
    for (const entry of this.pending.values()) {
      entry.reject(err);
    }
    this.pending.clear();
  }

  private sendRequest(method: string, params?: any): Promise<any> {
    if (!this.child || this.closed) {
      return Promise.reject(new Error("MCP session is not running"));
    }
    const log = this.options.onLog || (() => undefined);
    const id = this.nextId++;
    const payload = {
      jsonrpc: "2.0",
      id,
      method,
      params
    };
    const text = JSON.stringify(payload);
    log(`[mcp ->] ${method} (#${id})`);

    return new Promise<any>((resolve, reject) => {
      this.pending.set(id, { resolve, reject });
      try {
        this.child!.stdin.write(`${text}\n`, "utf8");
      } catch (err) {
        this.pending.delete(id);
        reject(new Error(`Failed to write MCP request: ${String(err)}`));
      }
    });
  }

  private sendNotification(method: string, params?: any): void {
    if (!this.child || this.closed) {
      return;
    }
    const log = this.options.onLog || (() => undefined);
    const payload = {
      jsonrpc: "2.0",
      method,
      params
    };
    const text = JSON.stringify(payload);
    log(`[mcp ->] ${method} (notification)`);
    try {
      this.child.stdin.write(`${text}\n`, "utf8");
    } catch {
      // ignore
    }
  }

  private async withTimeout<T>(promise: Promise<T>, label: string): Promise<T> {
    const timeoutMs = this.options.timeoutMs;
    if (timeoutMs <= 0) {
      return promise;
    }
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.dispose();
        reject(new Error(`${label} timed out after ${timeoutMs}ms`));
      }, timeoutMs);
      promise
        .then((value) => {
          clearTimeout(timer);
          resolve(value);
        })
        .catch((err) => {
          clearTimeout(timer);
          reject(err);
        });
    });
  }
}

