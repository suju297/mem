import * as fs from "fs/promises";
import * as path from "path";
import {
  buildSessionEntities,
  buildSessionTitle,
  compactMeaningfulText,
  computeSemanticBonus,
  computeSignificanceScore,
  decideSessionUpsertAction,
  estimateDiffStats,
  isAutoSessionTitle,
  isWhitespaceSensitiveExt,
  normalizeExt,
  normalizeForDiff,
  parseTimeMs,
  toRepoRelative,
  PrivacyMode
} from "./session_logic";

interface BurstFileStats {
  path: string;
  score: number;
  linesAdded: number;
  linesRemoved: number;
  charsDelta: number;
}

interface BurstState {
  startAt: number;
  lastMeaningfulAt: number;
  totalScore: number;
  lifecycleEvents: number;
  files: Map<string, BurstFileStats>;
  quietTimer?: unknown;
}

export interface AutoSessionIntentSignal {
  headline: string;
  entities: string[];
  observedAtMs: number;
}

export interface AutoSessionConfig {
  quietMs: number;
  maxBurstMs: number;
  scoreThreshold: number;
  filesThreshold: number;
  maxFilesPerSession: number;
  mergeWindowMs: number;
  newSessionMinGapMs: number;
  maxFileBytes: number;
  privacyMode: PrivacyMode;
  needsSummary: boolean;
  ignoredSegments?: string[];
  intentSignal?: AutoSessionIntentSignal;
  intentSignalMaxAgeMs?: number;
}

export interface AutoSessionRecent {
  id: string;
  title: string;
  created_at: string;
}

export interface AutoSessionCreateInput {
  workspaceRoot: string;
  title: string;
  tags: string[];
  thread: string;
  entities?: string[];
}

export interface AutoSessionUpdateInput {
  workspaceRoot: string;
  id: string;
  title?: string;
  tagsAdd?: string[];
  tagsRemove?: string[];
  entities?: string[];
  entitiesAdd?: string[];
}

export interface AutoSessionPersistence {
  listRecentSessions(workspaceRoot: string, limit: number): Promise<AutoSessionRecent[]>;
  resolveThread(workspaceRoot: string): Promise<string>;
  createSession(input: AutoSessionCreateInput): Promise<{ id: string }>;
  updateSession(input: AutoSessionUpdateInput): Promise<void>;
  onSessionSaved?(input: {
    workspaceRoot: string;
    sessionID: string;
    needsSummary: boolean;
    action: "created" | "updated";
    title: string;
  }): Promise<void>;
}

export interface AutoSessionScheduler {
  now(): number;
  setTimeout(fn: () => void, ms: number): unknown;
  clearTimeout(handle: unknown): void;
}

export interface AutoSessionSaveInput {
  workspaceRoot: string;
  filePath: string;
  text: string;
}

export interface AutoSessionFileCreateInput {
  workspaceRoot: string;
  filePath: string;
  text?: string;
}

export interface AutoSessionFileDeleteInput {
  workspaceRoot: string;
  filePath: string;
}

export interface AutoSessionFileRenameInput {
  workspaceRoot: string;
  oldFilePath: string;
  newFilePath: string;
  newText?: string;
}

interface IgnoreRule {
  regex: RegExp;
  negated: boolean;
}

interface IgnoreCacheEntry {
  rules: IgnoreRule[];
  gitIgnoreMtime: number;
  mempackIgnoreMtime: number;
}

const DEFAULT_IGNORED_SEGMENTS = [
  ".git/",
  ".mempack/",
  "node_modules/",
  "dist/",
  "build/",
  "vendor/",
  ".next/",
  "coverage/"
];

export class AutoSessionCaptureEngine {
  private readonly snapshotsByWorkspace = new Map<string, Map<string, string>>();
  private readonly burstByWorkspace = new Map<string, BurstState>();
  private readonly ignoreCacheByWorkspace = new Map<string, IgnoreCacheEntry>();
  private readonly flushInFlight = new Set<string>();
  private readonly lastAutoSessionAtByRoot = new Map<string, number>();

  constructor(
    private readonly persistence: AutoSessionPersistence,
    private readonly getConfig: (workspaceRoot: string) => AutoSessionConfig,
    private readonly scheduler: AutoSessionScheduler = defaultScheduler()
  ) {}

  async recordSave(input: AutoSessionSaveInput): Promise<void> {
    const workspaceRoot = normalizePath(input.workspaceRoot);
    const filePath = normalizePath(input.filePath);
    const relPath = toRepoRelative(workspaceRoot, filePath);
    if (!relPath) {
      return;
    }
    const config = this.getConfig(workspaceRoot);
    if (await this.shouldIgnore(workspaceRoot, relPath, config.ignoredSegments)) {
      return;
    }
    if (Buffer.byteLength(input.text, "utf8") > config.maxFileBytes) {
      return;
    }

    const ext = normalizeExt(relPath);
    const whitespaceSensitive = isWhitespaceSensitiveExt(ext, relPath);
    const normalized = normalizeForDiff(input.text, whitespaceSensitive);

    const snapshots = this.ensureSnapshots(workspaceRoot);

    const previous = snapshots.get(relPath);
    snapshots.set(relPath, normalized);
    if (typeof previous !== "string") {
      return;
    }
    if (previous === normalized) {
      return;
    }
    if (!whitespaceSensitive && compactMeaningfulText(previous) === compactMeaningfulText(normalized)) {
      return;
    }

    const stats = estimateDiffStats(previous, normalized);
    if (stats.deltaLines <= 0 && stats.deltaChars <= 0) {
      return;
    }
    const semanticBonus = computeSemanticBonus(previous, normalized, relPath);
    const score = computeSignificanceScore(stats.deltaLines, stats.deltaChars, relPath, semanticBonus);
    if (score <= 0) {
      return;
    }

    this.recordBurstChange(
      workspaceRoot,
      relPath,
      stats.linesAdded,
      stats.linesRemoved,
      stats.deltaChars,
      score,
      false
    );
  }

  async recordCreate(input: AutoSessionFileCreateInput): Promise<void> {
    const workspaceRoot = normalizePath(input.workspaceRoot);
    const filePath = normalizePath(input.filePath);
    const relPath = toRepoRelative(workspaceRoot, filePath);
    if (!relPath) {
      return;
    }
    const config = this.getConfig(workspaceRoot);
    if (await this.shouldIgnore(workspaceRoot, relPath, config.ignoredSegments)) {
      return;
    }
    const text = typeof input.text === "string" ? input.text : "";
    if (Buffer.byteLength(text, "utf8") > config.maxFileBytes) {
      return;
    }

    const ext = normalizeExt(relPath);
    const whitespaceSensitive = isWhitespaceSensitiveExt(ext, relPath);
    const normalized = normalizeForDiff(text, whitespaceSensitive);
    const snapshots = this.ensureSnapshots(workspaceRoot);
    snapshots.set(relPath, normalized);

    const deltaLines = Math.max(1, normalized.split("\n").filter((line) => line.trim() !== "").length);
    const semanticBonus = computeSemanticBonus("", normalized, relPath);
    const score = computeSignificanceScore(
      Math.max(6, deltaLines),
      Math.max(80, normalized.length),
      relPath,
      semanticBonus + 10
    );
    this.recordBurstChange(workspaceRoot, relPath, deltaLines, 0, normalized.length, score, true);
  }

  async recordDelete(input: AutoSessionFileDeleteInput): Promise<void> {
    const workspaceRoot = normalizePath(input.workspaceRoot);
    const filePath = normalizePath(input.filePath);
    const relPath = toRepoRelative(workspaceRoot, filePath);
    if (!relPath) {
      return;
    }
    const config = this.getConfig(workspaceRoot);
    if (await this.shouldIgnore(workspaceRoot, relPath, config.ignoredSegments)) {
      return;
    }
    const snapshots = this.ensureSnapshots(workspaceRoot);
    const previous = snapshots.get(relPath) || "";
    snapshots.delete(relPath);
    const deltaLines = Math.max(1, previous.split("\n").filter((line) => line.trim() !== "").length);
    const score = computeSignificanceScore(
      Math.max(6, deltaLines),
      Math.max(80, previous.length),
      relPath,
      10
    );
    this.recordBurstChange(workspaceRoot, relPath, 0, deltaLines, previous.length, score, true);
  }

  async recordRename(input: AutoSessionFileRenameInput): Promise<void> {
    const workspaceRoot = normalizePath(input.workspaceRoot);
    const oldFilePath = normalizePath(input.oldFilePath);
    const newFilePath = normalizePath(input.newFilePath);
    const oldRelPath = toRepoRelative(workspaceRoot, oldFilePath);
    const newRelPath = toRepoRelative(workspaceRoot, newFilePath);
    if (!oldRelPath || !newRelPath) {
      return;
    }
    const config = this.getConfig(workspaceRoot);
    const oldIgnored = await this.shouldIgnore(workspaceRoot, oldRelPath, config.ignoredSegments);
    const newIgnored = await this.shouldIgnore(workspaceRoot, newRelPath, config.ignoredSegments);
    if (oldIgnored && newIgnored) {
      return;
    }

    const snapshots = this.ensureSnapshots(workspaceRoot);
    const previous = snapshots.get(oldRelPath) || "";
    snapshots.delete(oldRelPath);

    const nextText = typeof input.newText === "string" ? input.newText : previous;
    const newExt = normalizeExt(newRelPath);
    const whitespaceSensitive = isWhitespaceSensitiveExt(newExt, newRelPath);
    const normalized = normalizeForDiff(nextText, whitespaceSensitive);
    if (!newIgnored) {
      snapshots.set(newRelPath, normalized);
    }
    if (oldIgnored || newIgnored) {
      return;
    }

    const semanticBonus = computeSemanticBonus(previous, normalized, newRelPath);
    const score = computeSignificanceScore(6, Math.max(40, normalized.length), newRelPath, semanticBonus + 10);
    const firstScore = Math.max(1, Math.floor(score / 2));
    const secondScore = Math.max(1, score - firstScore);
    this.recordBurstChange(workspaceRoot, oldRelPath, 1, 1, 0, firstScore, true);
    this.recordBurstChange(workspaceRoot, newRelPath, 1, 1, 0, secondScore, true);
  }

  async flushWorkspace(workspaceRoot: string, force = true): Promise<void> {
    await this.flushBurst(normalizePath(workspaceRoot), force);
  }

  dispose(): void {
    for (const burst of this.burstByWorkspace.values()) {
      if (burst.quietTimer !== undefined) {
        this.scheduler.clearTimeout(burst.quietTimer);
      }
    }
    this.burstByWorkspace.clear();
    this.snapshotsByWorkspace.clear();
    this.ignoreCacheByWorkspace.clear();
    this.flushInFlight.clear();
  }

  private ensureSnapshots(workspaceRoot: string): Map<string, string> {
    let snapshots = this.snapshotsByWorkspace.get(workspaceRoot);
    if (!snapshots) {
      snapshots = new Map<string, string>();
      this.snapshotsByWorkspace.set(workspaceRoot, snapshots);
    }
    return snapshots;
  }

  private async shouldIgnore(
    workspaceRoot: string,
    relPath: string,
    extraIgnored?: string[]
  ): Promise<boolean> {
    if (shouldIgnoreBySegment(relPath, extraIgnored)) {
      return true;
    }
    if (matchesIgnoreRules(relPath, parseInlineIgnoreRules(extraIgnored || []))) {
      return true;
    }
    const rules = await this.getIgnoreRules(workspaceRoot);
    return matchesIgnoreRules(relPath, rules);
  }

  private async getIgnoreRules(workspaceRoot: string): Promise<IgnoreRule[]> {
    const gitIgnorePath = path.join(workspaceRoot, ".gitignore");
    const mempackIgnorePath = path.join(workspaceRoot, ".mempackignore");
    const [gitIgnoreMtime, mempackIgnoreMtime] = await Promise.all([
      mtimeMs(gitIgnorePath),
      mtimeMs(mempackIgnorePath)
    ]);

    const cached = this.ignoreCacheByWorkspace.get(workspaceRoot);
    if (
      cached &&
      cached.gitIgnoreMtime === gitIgnoreMtime &&
      cached.mempackIgnoreMtime === mempackIgnoreMtime
    ) {
      return cached.rules;
    }

    const [gitIgnoreRaw, mempackIgnoreRaw] = await Promise.all([
      readTextIfExists(gitIgnorePath),
      readTextIfExists(mempackIgnorePath)
    ]);
    const rules = [
      ...parseIgnoreFile(gitIgnoreRaw),
      ...parseIgnoreFile(mempackIgnoreRaw)
    ];
    this.ignoreCacheByWorkspace.set(workspaceRoot, {
      rules,
      gitIgnoreMtime,
      mempackIgnoreMtime
    });
    return rules;
  }

  private recordBurstChange(
    workspaceRoot: string,
    relPath: string,
    linesAdded: number,
    linesRemoved: number,
    charsDelta: number,
    score: number,
    lifecycleEvent: boolean
  ): void {
    const now = this.scheduler.now();
    const config = this.getConfig(workspaceRoot);

    let burst = this.burstByWorkspace.get(workspaceRoot);
    if (!burst) {
      burst = this.newBurst(now);
      this.burstByWorkspace.set(workspaceRoot, burst);
    }

    burst.lastMeaningfulAt = now;
    burst.totalScore += score;
    if (lifecycleEvent) {
      burst.lifecycleEvents += 1;
    }
    const existing = burst.files.get(relPath);
    if (existing) {
      existing.linesAdded += linesAdded;
      existing.linesRemoved += linesRemoved;
      existing.charsDelta += charsDelta;
      existing.score += score;
    } else {
      burst.files.set(relPath, {
        path: relPath,
        score,
        linesAdded,
        linesRemoved,
        charsDelta
      });
    }

    if (burst.quietTimer !== undefined) {
      this.scheduler.clearTimeout(burst.quietTimer);
    }
    burst.quietTimer = this.scheduler.setTimeout(() => {
      void this.flushBurst(workspaceRoot, false);
    }, config.quietMs);

    if (burst.totalScore >= config.scoreThreshold || burst.files.size >= config.filesThreshold) {
      void this.flushBurst(workspaceRoot, true);
      return;
    }
    if (now - burst.startAt >= config.maxBurstMs) {
      void this.flushBurst(workspaceRoot, true);
    }
  }

  private async flushBurst(workspaceRoot: string, force: boolean): Promise<void> {
    if (this.flushInFlight.has(workspaceRoot)) {
      return;
    }
    const burst = this.burstByWorkspace.get(workspaceRoot);
    if (!burst) {
      return;
    }

    const config = this.getConfig(workspaceRoot);
    const now = this.scheduler.now();

    if (!force) {
      const sinceLast = now - burst.lastMeaningfulAt;
      if (sinceLast < config.quietMs) {
        if (burst.quietTimer !== undefined) {
          this.scheduler.clearTimeout(burst.quietTimer);
        }
        burst.quietTimer = this.scheduler.setTimeout(() => {
          void this.flushBurst(workspaceRoot, false);
        }, config.quietMs - sinceLast);
        return;
      }
    }

    if (!this.shouldPersistBurst(burst, config)) {
      this.clearBurst(workspaceRoot);
      return;
    }

    if (burst.quietTimer !== undefined) {
      this.scheduler.clearTimeout(burst.quietTimer);
      burst.quietTimer = undefined;
    }

    const flushingBurst = burst;
    this.burstByWorkspace.set(workspaceRoot, this.newBurst(now));
    this.flushInFlight.add(workspaceRoot);
    try {
      await this.upsertBurstSession(workspaceRoot, flushingBurst, config);
    } finally {
      this.flushInFlight.delete(workspaceRoot);
      const pending = this.burstByWorkspace.get(workspaceRoot);
      if (!pending) {
        return;
      }
      if (!this.hasBurstChanges(pending)) {
        this.clearBurst(workspaceRoot);
        return;
      }
      if (this.shouldPersistBurst(pending, config)) {
        void this.flushBurst(workspaceRoot, true);
      }
    }
  }

  private clearBurst(workspaceRoot: string): void {
    const burst = this.burstByWorkspace.get(workspaceRoot);
    if (burst?.quietTimer !== undefined) {
      this.scheduler.clearTimeout(burst.quietTimer);
    }
    this.burstByWorkspace.delete(workspaceRoot);
  }

  private newBurst(now: number): BurstState {
    return {
      startAt: now,
      lastMeaningfulAt: now,
      totalScore: 0,
      lifecycleEvents: 0,
      files: new Map<string, BurstFileStats>()
    };
  }

  private hasBurstChanges(burst: BurstState): boolean {
    return burst.totalScore > 0 || burst.lifecycleEvents > 0 || burst.files.size > 0;
  }

  private shouldPersistBurst(burst: BurstState, config: AutoSessionConfig): boolean {
    const lifecycleThreshold = Math.max(30, Math.round(config.scoreThreshold / 2));
    return (
      burst.totalScore >= config.scoreThreshold ||
      burst.files.size >= config.filesThreshold ||
      (burst.lifecycleEvents > 0 && burst.totalScore >= lifecycleThreshold)
    );
  }

  private async upsertBurstSession(
    workspaceRoot: string,
    burst: BurstState,
    config: AutoSessionConfig
  ): Promise<void> {
    const now = this.scheduler.now();
    const files = Array.from(burst.files.values())
      .sort((left, right) => right.score - left.score || left.path.localeCompare(right.path))
      .slice(0, config.maxFilesPerSession);
    if (files.length === 0) {
      return;
    }

    const filePaths = files.map((file) => file.path);
    const intentSignal = resolveIntentSignal(config, now);
    const title = buildSessionTitle(workspaceRoot, filePaths, intentSignal?.headline || "");
    const entities = mergeEntities(
      buildSessionEntities(filePaths, config.privacyMode),
      intentSignal?.entities || []
    );
    const tags = config.needsSummary ? ["session", "needs_summary"] : ["session"];
    const tagsRemove = config.needsSummary ? [] : ["needs_summary"];

    let recent: AutoSessionRecent[] = [];
    try {
      recent = await this.persistence.listRecentSessions(workspaceRoot, 1);
    } catch {
      recent = [];
    }

    const latest = recent[0];
    const action = decideSessionUpsertAction({
      nowMs: now,
      latestExists: Boolean(latest),
      latestIsAuto: latest ? isAutoSessionTitle(latest.title) : false,
      latestCreatedAtMs: latest ? parseTimeMs(latest.created_at) : 0,
      mergeWindowMs: config.mergeWindowMs,
      lastAutoSessionAtMs: this.lastAutoSessionAtByRoot.get(workspaceRoot) || 0,
      minGapMs: config.newSessionMinGapMs
    });

    let sessionID = "";
    let savedAction: "created" | "updated" = "created";
    if (latest && action === "update_latest") {
      await this.persistence.updateSession({
        workspaceRoot,
        id: latest.id,
        title: title !== latest.title ? title : undefined,
        tagsAdd: tags,
        tagsRemove,
        entitiesAdd: entities
      });
      sessionID = latest.id;
      savedAction = "updated";
    } else {
      const thread = await this.persistence.resolveThread(workspaceRoot);
      const created = await this.persistence.createSession({
        workspaceRoot,
        title,
        tags,
        thread,
        entities
      });
      sessionID = created.id;
      savedAction = "created";
    }

    this.lastAutoSessionAtByRoot.set(workspaceRoot, now);
    if (sessionID && this.persistence.onSessionSaved) {
      await this.persistence.onSessionSaved({
        workspaceRoot,
        sessionID,
        needsSummary: config.needsSummary,
        action: savedAction,
        title
      });
    }
  }
}

function resolveIntentSignal(
  config: AutoSessionConfig,
  nowMs: number
): AutoSessionIntentSignal | undefined {
  const signal = config.intentSignal;
  if (!signal) {
    return undefined;
  }
  const maxAgeMs =
    typeof config.intentSignalMaxAgeMs === "number" && Number.isFinite(config.intentSignalMaxAgeMs)
      ? Math.max(30_000, Math.round(config.intentSignalMaxAgeMs))
      : 600_000;
  if (nowMs - signal.observedAtMs > maxAgeMs) {
    return undefined;
  }
  return signal;
}

function mergeEntities(base: string[], extra: string[]): string[] {
  const merged = new Set<string>();
  for (const value of [...base, ...extra]) {
    const token = String(value || "").trim();
    if (token === "") {
      continue;
    }
    if (token.length > 120) {
      continue;
    }
    merged.add(token);
  }
  return Array.from(merged).sort().slice(0, 60);
}

function shouldIgnoreBySegment(relPath: string, extraIgnored?: string[]): boolean {
  const normalized = relPath.replace(/\\/g, "/");
  if (normalized.trim() === "") {
    return true;
  }
  const lower = normalized.toLowerCase();
  const ignored = [...DEFAULT_IGNORED_SEGMENTS, ...(extraIgnored || [])];
  for (const segment of ignored) {
    if (lower.includes(segment.toLowerCase())) {
      return true;
    }
  }
  return false;
}

function matchesIgnoreRules(relPath: string, rules: IgnoreRule[]): boolean {
  if (rules.length === 0) {
    return false;
  }
  const normalized = relPath.replace(/\\/g, "/").replace(/^\/+/, "");
  let ignored = false;
  for (const rule of rules) {
    if (rule.regex.test(normalized)) {
      ignored = !rule.negated;
    }
  }
  return ignored;
}

function parseIgnoreFile(content: string): IgnoreRule[] {
  const rules: IgnoreRule[] = [];
  if (content.trim() === "") {
    return rules;
  }
  for (const rawLine of content.split(/\r?\n/)) {
    let line = rawLine.trim();
    if (line === "" || line.startsWith("#")) {
      continue;
    }
    let negated = false;
    if (line.startsWith("!")) {
      negated = true;
      line = line.slice(1).trim();
    }
    if (line === "") {
      continue;
    }
    const regex = ignorePatternToRegex(line);
    if (!regex) {
      continue;
    }
    rules.push({ regex, negated });
  }
  return rules;
}

function parseInlineIgnoreRules(patterns: string[]): IgnoreRule[] {
  const rules: IgnoreRule[] = [];
  for (const rawPattern of patterns) {
    const pattern = String(rawPattern || "").trim();
    if (pattern === "") {
      continue;
    }
    const regex = ignorePatternToRegex(pattern);
    if (!regex) {
      continue;
    }
    rules.push({ regex, negated: false });
  }
  return rules;
}

function ignorePatternToRegex(pattern: string): RegExp | undefined {
  let normalized = pattern.replace(/\\/g, "/");
  if (normalized === "") {
    return undefined;
  }
  const anchored = normalized.startsWith("/");
  if (anchored) {
    normalized = normalized.slice(1);
  }
  const directoryOnly = normalized.endsWith("/");
  if (directoryOnly) {
    normalized = normalized.slice(0, -1);
  }
  if (normalized === "") {
    return undefined;
  }

  const globRegex = globToRegexBody(normalized);
  if (!globRegex) {
    return undefined;
  }
  if (directoryOnly) {
    if (anchored) {
      return new RegExp(`^${globRegex}(?:/.*)?$`);
    }
    return new RegExp(`(?:^|.*/)${globRegex}(?:/.*)?$`);
  }
  if (anchored) {
    return new RegExp(`^${globRegex}$`);
  }
  if (!normalized.includes("/")) {
    return new RegExp(`(?:^|.*/)${globRegex}(?:$|/.*)`);
  }
  return new RegExp(`(?:^|.*/)${globRegex}$`);
}

function globToRegexBody(glob: string): string {
  let out = "";
  for (let i = 0; i < glob.length; i += 1) {
    const ch = glob[i];
    if (ch === "*") {
      const next = glob[i + 1];
      if (next === "*") {
        out += ".*";
        i += 1;
      } else {
        out += "[^/]*";
      }
      continue;
    }
    if (ch === "?") {
      out += "[^/]";
      continue;
    }
    if ("\\.^$+{}()[]|".includes(ch)) {
      out += `\\${ch}`;
      continue;
    }
    out += ch;
  }
  return out;
}

async function readTextIfExists(filePath: string): Promise<string> {
  try {
    return await fs.readFile(filePath, "utf8");
  } catch {
    return "";
  }
}

async function mtimeMs(filePath: string): Promise<number> {
  try {
    const stat = await fs.stat(filePath);
    return Math.round(stat.mtimeMs);
  } catch {
    return 0;
  }
}

function normalizePath(value: string): string {
  return path.resolve(value).replace(/\\/g, "/");
}

function defaultScheduler(): AutoSessionScheduler {
  return {
    now: () => Date.now(),
    setTimeout: (fn: () => void, ms: number) => setTimeout(fn, ms),
    clearTimeout: (handle: unknown) => clearTimeout(handle as NodeJS.Timeout)
  };
}
