import * as path from "path";

export type PrivacyMode = "folders_exts" | "files" | "counts";
export type SessionUpsertAction = "update_latest" | "create_new";
export type IntentKind = "test" | "build" | "train" | "eval" | "migrate" | "deploy" | "commit";

const exactLcsWorkLimit = 1_000_000;

export interface TerminalIntentSignal {
  headline: string;
  entities: string[];
}

export interface SessionUpsertDecisionInput {
  nowMs: number;
  latestExists: boolean;
  latestIsAuto: boolean;
  latestCreatedAtMs: number;
  mergeWindowMs: number;
  minGapMs: number;
}

export function isAutoSessionTitle(title: string): boolean {
  return title.trim().toLowerCase().startsWith("session:");
}

export function clampNumber(value: number, min: number, max: number, fallback: number): number {
  if (!Number.isFinite(value)) {
    return fallback;
  }
  const rounded = Math.round(value);
  if (rounded < min) {
    return min;
  }
  if (rounded > max) {
    return max;
  }
  return rounded;
}

export function toRepoRelative(workspaceRoot: string, absolutePath: string): string {
  const rel = path.relative(workspaceRoot, absolutePath).replace(/\\/g, "/");
  if (rel.startsWith("..") || path.isAbsolute(rel)) {
    return "";
  }
  return rel.trim();
}

export function normalizeExt(relPath: string): string {
  return path.extname(relPath).replace(/^\./, "").trim().toLowerCase();
}

export function isWhitespaceSensitiveExt(ext: string, relPath: string): boolean {
  const file = path.basename(relPath).toLowerCase();
  if (file === "makefile") {
    return true;
  }
  const sensitive = new Set(["py", "yaml", "yml", "toml", "mk"]);
  return sensitive.has(ext);
}

export function normalizeForDiff(text: string, whitespaceSensitive: boolean): string {
  const raw = text.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
  const lines = raw.split("\n").map((line) => line.replace(/[ \t]+$/g, ""));
  if (whitespaceSensitive) {
    return lines.join("\n");
  }
  return lines.map((line) => line.replace(/[ \t]+/g, " ")).join("\n");
}

export function compactMeaningfulText(text: string): string {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line !== "")
    .join("\n");
}

export function estimateDiffStats(before: string, after: string): {
  linesAdded: number;
  linesRemoved: number;
  deltaLines: number;
  deltaChars: number;
} {
  const beforeLines = before.split("\n");
  const afterLines = after.split("\n");
  const n = beforeLines.length;
  const m = afterLines.length;
  let common = 0;

  // Exact LCS for typical edit sizes.
  const lcsWork = n * m;
  if (lcsWork <= exactLcsWorkLimit) {
    const shortLines = n <= m ? beforeLines : afterLines;
    const longLines = n <= m ? afterLines : beforeLines;
    const width = shortLines.length + 1;
    let prev = new Uint32Array(width);
    let curr = new Uint32Array(width);

    for (const longLine of longLines) {
      for (let j = 1; j < width; j += 1) {
        if (longLine === shortLines[j - 1]) {
          curr[j] = prev[j - 1] + 1;
        } else {
          curr[j] = Math.max(prev[j], curr[j - 1]);
        }
      }
      const tmp = prev;
      prev = curr;
      curr = tmp;
      curr.fill(0);
    }
    common = prev[width - 1] || 0;
  } else {
    // Fallback for very large files: multiset overlap by line content.
    const counts = new Map<string, number>();
    for (const line of beforeLines) {
      counts.set(line, (counts.get(line) || 0) + 1);
    }
    for (const line of afterLines) {
      const count = counts.get(line) || 0;
      if (count <= 0) {
        continue;
      }
      common += 1;
      if (count === 1) {
        counts.delete(line);
      } else {
        counts.set(line, count - 1);
      }
    }
  }

  const linesRemoved = Math.max(0, n - common);
  const linesAdded = Math.max(0, m - common);
  return {
    linesAdded,
    linesRemoved,
    deltaLines: linesAdded + linesRemoved,
    deltaChars: Math.abs(after.length - before.length)
  };
}

export function computeSemanticBonus(before: string, after: string, relPath: string): number {
  if (!supportsSemanticSignals(relPath)) {
    return 0;
  }
  const beforeSignatures = extractSignatureLines(before, relPath);
  const afterSignatures = extractSignatureLines(after, relPath);
  const beforeSet = new Set(beforeSignatures);
  const afterSet = new Set(afterSignatures);
  const signatureChanges = symmetricDiffCount(beforeSet, afterSet);
  if (signatureChanges === 0) {
    return 0;
  }

  const beforeExported = new Set(
    beforeSignatures.filter((line) => isLikelyExported(line, relPath))
  );
  const afterExported = new Set(
    afterSignatures.filter((line) => isLikelyExported(line, relPath))
  );
  const exportedChanges = symmetricDiffCount(beforeExported, afterExported);
  const functionCountDelta = Math.abs(
    countFunctionDeclarations(beforeSignatures) - countFunctionDeclarations(afterSignatures)
  );

  const bonus = exportedChanges * 20 + signatureChanges * 10 + functionCountDelta * 5;
  return clampNumber(bonus, 0, 120, 0);
}

export function computeSignificanceScore(
  deltaLines: number,
  deltaChars: number,
  relPath: string,
  semanticBonus = 0
): number {
  if (deltaLines <= 0 && deltaChars <= 0) {
    return 0;
  }
  const base = Math.min(100, deltaLines * 2 + deltaChars / 50 + semanticBonus);
  const weight = fileWeight(relPath);
  return Math.max(0, Math.round(base * weight));
}

export function fileWeight(relPath: string): number {
  const normalized = relPath.replace(/\\/g, "/").toLowerCase();
  const ext = normalizeExt(normalized);
  const base = path.basename(normalized);
  if (base.includes(".test.") || base.includes(".spec.") || normalized.includes("/test/")) {
    return 1.2;
  }
  if (
    base === ".env" ||
    base.startsWith(".env.") ||
    ext === "toml" ||
    ext === "yaml" ||
    ext === "yml" ||
    ext === "json" ||
    ext === "jsonc"
  ) {
    return 2.0;
  }
  const docs = new Set(["md", "txt", "rst", "adoc"]);
  if (docs.has(ext)) {
    return 0.7;
  }
  const code = new Set([
    "go",
    "ts",
    "tsx",
    "js",
    "jsx",
    "py",
    "rs",
    "java",
    "kt",
    "swift",
    "c",
    "cc",
    "cpp",
    "h",
    "hpp",
    "cs",
    "php",
    "rb",
    "sh",
    "bash",
    "zsh",
    "sql"
  ]);
  if (code.has(ext)) {
    return 1.5;
  }
  return 1.0;
}

export function buildSessionTitle(
  workspaceRoot: string,
  filePaths: string[],
  intentHeadline = ""
): string {
  const fallback = sanitizeToken(path.basename(workspaceRoot) || "workspace");
  const folderCounts = new Map<string, number>();
  const extCounts = new Map<string, number>();
  for (const relPath of filePaths) {
    const cleanPath = relPath.replace(/\\/g, "/");
    const folder = cleanPath.includes("/") ? cleanPath.split("/")[0] : fallback;
    if (folder.trim() !== "") {
      folderCounts.set(folder, (folderCounts.get(folder) || 0) + 1);
    }
    const ext = normalizeExt(cleanPath);
    if (ext !== "") {
      extCounts.set(ext, (extCounts.get(ext) || 0) + 1);
    }
  }
  const topFolder = topNByCount(folderCounts, 1)[0] || fallback;
  const topExts = topNByCount(extCounts, 2);
  const count = filePaths.length;
  const countSuffix = count === 1 ? "file" : "files";
  const extSuffix = topExts.length > 0 ? `, ${topExts.join(", ")}` : "";
  const normalizedHeadline = intentHeadline.trim();
  if (normalizedHeadline !== "") {
    return `Session: ${normalizedHeadline} in ${topFolder} (${count} ${countSuffix}${extSuffix})`;
  }
  return `Session: Worked in ${topFolder} (${count} ${countSuffix}${extSuffix})`;
}

export function buildSessionEntities(filePaths: string[], privacyMode: PrivacyMode): string[] {
  const dirSet = new Set<string>();
  const extSet = new Set<string>();
  const fileSet = new Set<string>();

  for (const relPath of filePaths) {
    const cleanPath = relPath.replace(/\\/g, "/");
    const firstSegment = cleanPath.includes("/") ? cleanPath.split("/")[0] : "";
    if (firstSegment !== "") {
      dirSet.add(`dir_${sanitizeToken(firstSegment)}`);
    }
    const ext = normalizeExt(cleanPath);
    if (ext !== "") {
      extSet.add(`ext_${sanitizeToken(ext)}`);
    }
    if (privacyMode === "files") {
      fileSet.add(`file_${sanitizeToken(cleanPath)}`);
    }
  }

  const dirs = Array.from(dirSet).sort().slice(0, 20);
  const exts = Array.from(extSet).sort().slice(0, 10);
  if (privacyMode === "counts") {
    return [
      `count_files_${filePaths.length}`,
      `count_dirs_${dirSet.size}`,
      `count_exts_${extSet.size}`
    ];
  }
  const files = privacyMode === "files" ? Array.from(fileSet).sort().slice(0, 50) : [];
  return [...dirs, ...exts, ...files];
}

export function parseTimeMs(value: string): number {
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

export function decideSessionUpsertAction(input: SessionUpsertDecisionInput): SessionUpsertAction {
  if (!input.latestExists || !input.latestIsAuto) {
    return "create_new";
  }
  const withinMergeWindow =
    input.latestCreatedAtMs > 0 && input.nowMs - input.latestCreatedAtMs <= input.mergeWindowMs;
  const withinGap =
    input.latestCreatedAtMs > 0 && input.nowMs - input.latestCreatedAtMs <= input.minGapMs;
  if (withinMergeWindow || withinGap) {
    return "update_latest";
  }
  return "create_new";
}

export function extractTerminalIntentSignal(commandLine: string): TerminalIntentSignal | undefined {
  const raw = commandLine.replace(/\s+/g, " ").trim();
  if (raw === "") {
    return undefined;
  }
  if (looksSensitiveCommand(raw)) {
    return undefined;
  }

  const commitMessage = extractGitCommitMessage(raw);
  if (commitMessage) {
    return {
      headline: `commit prep: ${commitMessage}`,
      entities: ["intent_commit", "cmd_git"]
    };
  }

  const lower = raw.toLowerCase();
  const kind = detectIntentKind(lower);
  if (!kind) {
    return undefined;
  }

  const modelHint = extractModelHint(lower);
  const metricHint = extractMetricHint(lower);
  const headline = buildIntentHeadline(kind, modelHint, metricHint);
  const entities = new Set<string>();
  entities.add(`intent_${kind}`);

  const firstToken = raw.split(/\s+/)[0] || "";
  const cmd = sanitizeToken(path.basename(firstToken));
  if (cmd !== "") {
    entities.add(`cmd_${cmd}`);
  }
  if (modelHint.token !== "") {
    entities.add(modelHint.token);
  }
  if (metricHint.token !== "") {
    entities.add(metricHint.token);
  }

  return {
    headline,
    entities: Array.from(entities).sort().slice(0, 12)
  };
}

function topNByCount(counts: Map<string, number>, n: number): string[] {
  return Array.from(counts.entries())
    .sort((left, right) => right[1] - left[1] || left[0].localeCompare(right[0]))
    .slice(0, n)
    .map(([value]) => value);
}

function sanitizeToken(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "")
    .replace(/_+/g, "_");
}

function looksSensitiveCommand(commandLine: string): boolean {
  return /\b(password|passwd|secret|token|apikey|api_key|private_key|authorization)\b/i.test(commandLine);
}

function extractGitCommitMessage(commandLine: string): string {
  if (!/\bgit\s+commit\b/i.test(commandLine)) {
    return "";
  }
  const quoted = commandLine.match(/(?:^|\s)-m\s+["']([^"']{1,120})["']/i);
  if (quoted && quoted[1]) {
    return quoted[1].trim();
  }
  const plain = commandLine.match(/(?:^|\s)-m\s+([^\s][^-]{2,120})/i);
  if (plain && plain[1]) {
    return plain[1].trim();
  }
  return "git commit";
}

function detectIntentKind(commandLower: string): IntentKind | undefined {
  if (
    /\b(pytest|go test|npm test|pnpm test|yarn test|jest|vitest|cargo test|ctest|gradle test)\b/.test(
      commandLower
    )
  ) {
    return "test";
  }
  if (
    /\b(go build|npm run build|pnpm build|yarn build|cargo build|gradle build|cmake --build|docker build)\b/.test(
      commandLower
    )
  ) {
    return "build";
  }
  if (
    /\b(train|trainer|fit|fine[-_ ]?tune|epoch|tensorflow|pytorch|torchrun|keras)\b/.test(commandLower)
  ) {
    return "train";
  }
  if (/\b(eval|evaluate|validation|inference|benchmark|score)\b/.test(commandLower)) {
    return "eval";
  }
  if (
    /\b(prisma migrate|alembic upgrade|flyway|dbmate|knex migrate|sequelize db:migrate|liquibase)\b/.test(
      commandLower
    )
  ) {
    return "migrate";
  }
  if (
    /\b(deploy|release|helm upgrade|kubectl apply|terraform apply|vercel|netlify deploy)\b/.test(
      commandLower
    )
  ) {
    return "deploy";
  }
  return undefined;
}

function buildIntentHeadline(
  kind: IntentKind,
  modelHint: { label: string; token: string },
  metricHint: { label: string; token: string }
): string {
  const metricSuffix = metricHint.label !== "" ? ` (${metricHint.label})` : "";
  if (kind === "train") {
    if (modelHint.label !== "") {
      return `model training for ${modelHint.label}${metricSuffix}`;
    }
    return `model training${metricSuffix}`;
  }
  if (kind === "eval") {
    if (modelHint.label !== "") {
      return `model evaluation for ${modelHint.label}${metricSuffix}`;
    }
    return `model evaluation${metricSuffix}`;
  }
  if (kind === "test") {
    return "test run";
  }
  if (kind === "build") {
    return "build run";
  }
  if (kind === "migrate") {
    return "database migration";
  }
  if (kind === "deploy") {
    return "deployment run";
  }
  return "session";
}

function extractMetricHint(commandLower: string): { label: string; token: string } {
  const patterns: Array<{ name: string; regex: RegExp }> = [
    { name: "accuracy", regex: /\b(?:acc|accuracy)\s*(?:=|:)?\s*(\d+(?:\.\d+)?%?)/ },
    { name: "f1", regex: /\bf1(?:[_ -]?score)?\s*(?:=|:)?\s*(\d+(?:\.\d+)?%?)/ },
    { name: "auc", regex: /\bauc\s*(?:=|:)?\s*(\d+(?:\.\d+)?%?)/ }
  ];
  for (const pattern of patterns) {
    const match = commandLower.match(pattern.regex);
    if (!match || !match[1]) {
      continue;
    }
    const value = match[1];
    const tokenValue = sanitizeToken(value.replace(/%/g, "_pct"));
    if (tokenValue === "") {
      continue;
    }
    return {
      label: `${pattern.name} ${value}`,
      token: `metric_${pattern.name}_${tokenValue}`
    };
  }
  return { label: "", token: "" };
}

function extractModelHint(commandLower: string): { label: string; token: string } {
  const patterns: Array<{ label: string; token: string; regex: RegExp }> = [
    { label: "neural network", token: "model_neural_network", regex: /\b(neural[_ -]?network|cnn|rnn|lstm|transformer|bert|pytorch|tensorflow|keras)\b/ },
    { label: "naive bayes", token: "model_naive_bayes", regex: /\b(naive[_ -]?bayes|sklearn\.naive_bayes)\b/ },
    { label: "xgboost", token: "model_xgboost", regex: /\bxgboost\b/ },
    { label: "random forest", token: "model_random_forest", regex: /\brandom[_ -]?forest\b/ }
  ];
  for (const pattern of patterns) {
    if (pattern.regex.test(commandLower)) {
      return { label: pattern.label, token: pattern.token };
    }
  }
  return { label: "", token: "" };
}

function supportsSemanticSignals(relPath: string): boolean {
  const ext = normalizeExt(relPath);
  const supported = new Set([
    "go",
    "ts",
    "tsx",
    "js",
    "jsx",
    "py",
    "rs",
    "java",
    "kt",
    "cs",
    "swift"
  ]);
  return supported.has(ext);
}

function extractSignatureLines(text: string, relPath: string): string[] {
  const ext = normalizeExt(relPath);
  const lines = text.split("\n");
  const signatures: string[] = [];
  for (const raw of lines) {
    const line = raw.trim();
    if (line === "") {
      continue;
    }
    if (looksLikeSignature(line, ext)) {
      signatures.push(line.replace(/\s+/g, " "));
    }
  }
  return signatures;
}

function looksLikeSignature(line: string, ext: string): boolean {
  if (ext === "go") {
    return /^func\s+(\([^)]*\)\s*)?[A-Za-z_][A-Za-z0-9_]*\s*\(/.test(line);
  }
  if (ext === "py") {
    return /^(def|class)\s+[A-Za-z_][A-Za-z0-9_]*\s*(\(|:)/.test(line);
  }
  if (ext === "rs") {
    return /^(pub\s+)?(async\s+)?fn\s+[A-Za-z_][A-Za-z0-9_]*\s*\(/.test(line);
  }
  if (ext === "java" || ext === "kt" || ext === "cs" || ext === "swift") {
    return /\b(class|interface|enum|record)\b/.test(line) || /\b(public|private|protected)\b.*\(/.test(line);
  }
  return (
    /^export\s+/.test(line) ||
    /^(async\s+)?function\s+[A-Za-z_][A-Za-z0-9_]*\s*\(/.test(line) ||
    /^class\s+[A-Za-z_][A-Za-z0-9_]*\b/.test(line) ||
    /^(const|let|var)\s+[A-Za-z_][A-Za-z0-9_]*\s*=\s*(async\s*)?\(/.test(line) ||
    /^(interface|type|enum)\s+[A-Za-z_][A-Za-z0-9_]*\b/.test(line)
  );
}

function isLikelyExported(signatureLine: string, relPath: string): boolean {
  const ext = normalizeExt(relPath);
  if (ext === "go") {
    const match = signatureLine.match(/^func\s+(\([^)]*\)\s*)?([A-Za-z_][A-Za-z0-9_]*)\s*\(/);
    if (!match) {
      return false;
    }
    const name = match[2] || "";
    return /^[A-Z]/.test(name);
  }
  if (ext === "py") {
    const match = signatureLine.match(/^(def|class)\s+([A-Za-z_][A-Za-z0-9_]*)/);
    if (!match) {
      return false;
    }
    const name = match[2] || "";
    return !name.startsWith("_");
  }
  if (ext === "rs") {
    return signatureLine.startsWith("pub ");
  }
  return (
    signatureLine.startsWith("export ") ||
    /\bpublic\b/.test(signatureLine) ||
    /\bpub\b/.test(signatureLine)
  );
}

function countFunctionDeclarations(signatures: string[]): number {
  let count = 0;
  for (const line of signatures) {
    if (
      /\bfunction\b/.test(line) ||
      /\bfunc\b/.test(line) ||
      /^\s*def\b/.test(line) ||
      /\bfn\b/.test(line)
    ) {
      count += 1;
    }
  }
  return count;
}

function symmetricDiffCount(left: Set<string>, right: Set<string>): number {
  let count = 0;
  for (const item of left) {
    if (!right.has(item)) {
      count += 1;
    }
  }
  for (const item of right) {
    if (!left.has(item)) {
      count += 1;
    }
  }
  return count;
}
