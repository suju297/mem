import * as os from "os";
import * as path from "path";

export type EmbeddingConfig = {
  provider: string;
  model: string;
};

export type McpWritesMode = "off" | "ask" | "auto";
export type McpWritesSource = "default" | "global" | "repo";

export type McpWritesConfig = {
  mcp_allow_write?: boolean;
  mcp_write_mode?: string;
};

export type EffectiveMcpWrites = {
  mode: McpWritesMode;
  source: McpWritesSource;
  allowWrite: boolean;
  configuredMode: McpWritesMode;
  invalidConfig: boolean;
};

export type RepoConfig = {
  mcp_allow_write?: boolean;
  mcp_write_mode?: string;
  embedding_provider?: string;
  embedding_model?: string;
  token_budget?: number;
};

export function getConfigPath(): string {
  const configHome = process.env.XDG_CONFIG_HOME || path.join(os.homedir(), ".config");
  return path.join(configHome, "mempack", "config.toml");
}

export function getRepoConfigPath(cwd: string): string {
  return path.join(cwd, ".mempack", "config.json");
}

export function parseEmbeddingConfig(content: string): EmbeddingConfig {
  const provider = readTomlString(content, "embedding_provider") || "auto";
  const model = readTomlString(content, "embedding_model") || "nomic-embed-text";
  return { provider, model };
}

export function parseTokenBudgetConfig(content: string): number | undefined {
  const value = readTomlNumber(content, "token_budget");
  if (typeof value === "number" && Number.isFinite(value) && value > 0) {
    return Math.floor(value);
  }
  return undefined;
}

export function parseMcpWritesConfig(content: string): McpWritesConfig {
  return {
    mcp_allow_write: readTomlBoolean(content, "mcp_allow_write"),
    mcp_write_mode: readTomlString(content, "mcp_write_mode")
  };
}

export function resolveMcpWrites(
  globalCfg: McpWritesConfig = {},
  repoCfg: RepoConfig = {}
): EffectiveMcpWrites {
  const repoHasAllow = typeof repoCfg.mcp_allow_write === "boolean";
  const repoModeRaw = typeof repoCfg.mcp_write_mode === "string" ? repoCfg.mcp_write_mode.trim() : "";
  const repoHasMode = repoModeRaw !== "";
  const globalHasAllow = typeof globalCfg.mcp_allow_write === "boolean";
  const globalModeRaw =
    typeof globalCfg.mcp_write_mode === "string" ? globalCfg.mcp_write_mode.trim() : "";
  const globalHasMode = globalModeRaw !== "";

  const source: McpWritesSource =
    repoHasAllow || repoHasMode
      ? "repo"
      : globalHasAllow || globalHasMode
        ? "global"
        : "default";

  const allowWrite = repoHasAllow
    ? repoCfg.mcp_allow_write === true
    : globalHasAllow
      ? globalCfg.mcp_allow_write === true
      : true;

  const modeRaw = repoHasMode ? repoModeRaw : globalHasMode ? globalModeRaw : "ask";
  const configuredMode = normalizeMcpWriteMode(modeRaw);
  const invalidConfig = !allowWrite && configuredMode !== "off";
  const mode: McpWritesMode = invalidConfig
    ? "off"
    : configuredMode === "off" || !allowWrite
      ? "off"
      : configuredMode;

  return {
    mode,
    source,
    allowWrite,
    configuredMode,
    invalidConfig
  };
}

export function parseRepoConfig(content: string): RepoConfig {
  if (content.trim() === "") {
    return {};
  }
  const data = JSON.parse(content);
  if (!data || typeof data !== "object") {
    return {};
  }
  const cfg: RepoConfig = {};
  if (typeof data.mcp_allow_write === "boolean") {
    cfg.mcp_allow_write = data.mcp_allow_write;
  }
  if (typeof data.mcp_write_mode === "string") {
    cfg.mcp_write_mode = data.mcp_write_mode;
  }
  if (typeof data.embedding_provider === "string") {
    cfg.embedding_provider = data.embedding_provider;
  }
  if (typeof data.embedding_model === "string") {
    cfg.embedding_model = data.embedding_model;
  }
  if (typeof data.token_budget === "number" && Number.isFinite(data.token_budget)) {
    cfg.token_budget = Math.floor(data.token_budget);
  }
  return cfg;
}

export function updateRepoConfig(content: string, updates: Partial<RepoConfig>): string {
  const base = parseRepoConfig(content);
  const merged: RepoConfig = { ...base, ...updates };
  return serializeRepoConfig(merged);
}

function serializeRepoConfig(cfg: RepoConfig): string {
  const ordered: Record<string, unknown> = {};
  if (cfg.mcp_allow_write !== undefined) {
    ordered.mcp_allow_write = cfg.mcp_allow_write;
  }
  if (cfg.mcp_write_mode !== undefined) {
    ordered.mcp_write_mode = cfg.mcp_write_mode;
  }
  if (cfg.embedding_provider !== undefined) {
    ordered.embedding_provider = cfg.embedding_provider;
  }
  if (cfg.embedding_model !== undefined) {
    ordered.embedding_model = cfg.embedding_model;
  }
  if (cfg.token_budget !== undefined) {
    ordered.token_budget = cfg.token_budget;
  }
  return JSON.stringify(ordered, null, 2) + "\n";
}

export function setTomlString(content: string, key: string, value: string): string {
  const eol = content.includes("\r\n") ? "\r\n" : "\n";
  const hadTrailingEol = content.endsWith("\n") || content.endsWith("\r\n");
  const lines = content.split(/\r?\n/);
  const filtered = lines.filter((line) => !new RegExp(`^\\s*${escapeRegex(key)}\\s*=`, "i").test(line));
  filtered.push(`${key} = "${value}"`);
  const joined = filtered.join(eol);
  return hadTrailingEol ? joined + eol : joined;
}

export function setTomlNumber(content: string, key: string, value: number): string {
  const eol = content.includes("\r\n") ? "\r\n" : "\n";
  const hadTrailingEol = content.endsWith("\n") || content.endsWith("\r\n");
  const lines = content.split(/\r?\n/);
  const filtered = lines.filter((line) => !new RegExp(`^\\s*${escapeRegex(key)}\\s*=`, "i").test(line));
  filtered.push(`${key} = ${value}`);
  const joined = filtered.join(eol);
  return hadTrailingEol ? joined + eol : joined;
}

export function setTomlBoolean(content: string, key: string, value: boolean): string {
  const eol = content.includes("\r\n") ? "\r\n" : "\n";
  const hadTrailingEol = content.endsWith("\n") || content.endsWith("\r\n");
  const lines = content.split(/\r?\n/);
  const filtered = lines.filter((line) => !new RegExp(`^\\s*${escapeRegex(key)}\\s*=`, "i").test(line));
  filtered.push(`${key} = ${value ? "true" : "false"}`);
  const joined = filtered.join(eol);
  return hadTrailingEol ? joined + eol : joined;
}

function readTomlString(content: string, key: string): string | undefined {
  const pattern = new RegExp(`^\\s*${escapeRegex(key)}\\s*=\\s*(.+?)\\s*$`, "i");
  for (const line of content.split(/\r?\n/)) {
    const match = line.match(pattern);
    if (!match) {
      continue;
    }
    const raw = match[1].trim();
    if (raw.startsWith("\"") && raw.endsWith("\"")) {
      return raw.slice(1, -1);
    }
    if (raw.startsWith("'") && raw.endsWith("'")) {
      return raw.slice(1, -1);
    }
    return raw;
  }
  return undefined;
}

function readTomlNumber(content: string, key: string): number | undefined {
  const pattern = new RegExp(`^\\s*${escapeRegex(key)}\\s*=\\s*(.+?)\\s*$`, "i");
  for (const line of content.split(/\r?\n/)) {
    const match = line.match(pattern);
    if (!match) {
      continue;
    }
    const raw = match[1].trim().replace(/#,.*$/, "").replace(/;.*$/, "").trim();
    const value = Number(raw);
    if (Number.isFinite(value)) {
      return value;
    }
  }
  return undefined;
}

function readTomlBoolean(content: string, key: string): boolean | undefined {
  const pattern = new RegExp(`^\\s*${escapeRegex(key)}\\s*=\\s*(.+?)\\s*$`, "i");
  for (const line of content.split(/\r?\n/)) {
    const match = line.match(pattern);
    if (!match) {
      continue;
    }
    const raw = match[1].trim().replace(/#.*$/, "").replace(/;.*$/, "").trim();
    const normalized = raw.toLowerCase();
    if (normalized === "true") {
      return true;
    }
    if (normalized === "false") {
      return false;
    }
  }
  return undefined;
}

function normalizeMcpWriteMode(value: string): McpWritesMode {
  const mode = value.trim().toLowerCase();
  if (mode === "off" || mode === "auto" || mode === "ask") {
    return mode;
  }
  return "ask";
}

function escapeRegex(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
