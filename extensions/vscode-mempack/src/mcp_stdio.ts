import { spawn } from "child_process";
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

interface McpClientOptions {
  cwd: string;
  timeoutMs: number;
  onLog?: (line: string) => void;
  onSpawn?: (pid: number | undefined) => void;
  onExit?: (code: number | null, signal: NodeJS.Signals | null) => void;
}

function isObject(value: unknown): value is Record<string, any> {
  return typeof value === "object" && value !== null;
}

function formatJsonRpcError(error: JsonRpcError): string {
  const msg = error.message || "Unknown error";
  const code = typeof error.code === "number" ? ` (code ${error.code})` : "";
  return `${msg}${code}`.trim();
}

function withTimeout<T>(
  promise: Promise<T>,
  timeoutMs: number,
  onTimeout: () => void,
  label: string
): Promise<T> {
  if (timeoutMs <= 0) {
    return promise;
  }
  return new Promise<T>((resolve, reject) => {
    const timer = setTimeout(() => {
      try {
        onTimeout();
      } catch {
        // ignore
      }
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

export async function callMcpTool(
  binary: string,
  serverArgs: string[],
  toolName: string,
  toolArgs: Record<string, any>,
  options: McpClientOptions
): Promise<McpToolCallResult> {
  const log = options.onLog || (() => undefined);

  const child = spawn(binary, serverArgs, {
    cwd: options.cwd,
    stdio: ["pipe", "pipe", "pipe"]
  });
  try {
    options.onSpawn?.(child.pid);
  } catch {
    // ignore callback failures
  }

  const pending = new Map<number, { resolve: (result: any) => void; reject: (err: Error) => void }>();
  let nextId = 1;
  let stderr = "";
  let closed = false;

  const kill = () => {
    if (closed) {
      return;
    }
    closed = true;
    try {
      child.stdin?.end();
    } catch {
      // ignore
    }
    try {
      child.kill();
    } catch {
      // ignore
    }
  };

  const rejectAll = (err: Error) => {
    for (const entry of pending.values()) {
      entry.reject(err);
    }
    pending.clear();
  };

  child.on("error", (err) => {
    rejectAll(new Error(`Failed to start MCP server: ${String(err)}`));
  });

  child.on("exit", (code, signal) => {
    try {
      options.onExit?.(code, signal);
    } catch {
      // ignore callback failures
    }
    const tail = stderr.trim();
    const detail = tail ? `: ${tail}` : "";
    const reason =
      typeof code === "number"
        ? `exit code ${code}`
        : signal
          ? `signal ${signal}`
          : "unknown exit";
    rejectAll(new Error(`MCP server exited (${reason})${detail}`));
  });

  if (child.stderr) {
    const rlErr = readline.createInterface({ input: child.stderr });
    rlErr.on("line", (line) => {
      stderr = `${stderr}${stderr ? "\n" : ""}${line}`;
      log(`[mcp stderr] ${line}`);
    });
    rlErr.on("close", () => {
      // no-op
    });
  }

  if (!child.stdin || !child.stdout) {
    kill();
    throw new Error("Failed to start MCP server (stdio unavailable).");
  }

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

    const entry = pending.get(id);
    if (!entry) {
      return;
    }
    pending.delete(id);

    const resp = msg as JsonRpcResponse;
    if (resp.error) {
      entry.reject(new Error(formatJsonRpcError(resp.error)));
      return;
    }
    entry.resolve(resp.result);
  });

  const sendRequest = (method: string, params?: any): Promise<any> => {
    const id = nextId++;
    const payload = {
      jsonrpc: "2.0",
      id,
      method,
      params
    };
    const text = JSON.stringify(payload);
    log(`[mcp ->] ${method} (#${id})`);
    return new Promise<any>((resolve, reject) => {
      pending.set(id, { resolve, reject });
      try {
        child.stdin.write(`${text}\n`, "utf8");
      } catch (err) {
        pending.delete(id);
        reject(new Error(`Failed to write MCP request: ${String(err)}`));
      }
    });
  };

  const sendNotification = (method: string, params?: any): void => {
    const payload = {
      jsonrpc: "2.0",
      method,
      params
    };
    const text = JSON.stringify(payload);
    log(`[mcp ->] ${method} (notification)`);
    try {
      child.stdin.write(`${text}\n`, "utf8");
    } catch {
      // ignore
    }
  };

  try {
    await withTimeout(
      sendRequest("initialize", {
        protocolVersion: "2025-06-18",
        capabilities: {},
        clientInfo: { name: "vscode-mempack", version: "0.0.0" }
      }),
      options.timeoutMs,
      kill,
      "MCP initialize"
    );
    sendNotification("notifications/initialized");

    const result = await withTimeout(
      sendRequest("tools/call", { name: toolName, arguments: toolArgs }),
      options.timeoutMs,
      kill,
      `MCP tools/call ${toolName}`
    );
    return (result || {}) as McpToolCallResult;
  } catch (err) {
    const tail = stderr.trim();
    const detail = tail ? `\n${tail}` : "";
    throw new Error(`${err instanceof Error ? err.message : String(err)}${detail}`);
  } finally {
    kill();
    rlOut.close();
  }
}
