const assert = require("assert");
const fs = require("fs/promises");
const os = require("os");
const path = require("path");

const {
  appendStub,
  buildAgentsWithStub,
  hasStub,
  STUB_BLOCK,
  MINIMAL_AGENTS
} = require("../dist/stub_logic");

const {
  formatDoctorReport,
  formatShowResult,
  suggestSummary,
  suggestTitle,
  truncate
} = require("../dist/format");

const {
  buildSessionEntities,
  buildSessionTitle,
  compactMeaningfulText,
  computeSemanticBonus,
  computeSignificanceScore,
  decideSessionUpsertAction,
  estimateDiffStats,
  extractTerminalIntentSignal,
  isAutoSessionTitle,
  isWhitespaceSensitiveExt,
  normalizeForDiff,
  parseTimeMs,
  toRepoRelative
} = require("../dist/session_logic");

const { AutoSessionCaptureEngine } = require("../dist/auto_session_capture");

function testStubHelpers() {
  const empty = "";
  const built = buildAgentsWithStub();
  assert.ok(built.includes(MINIMAL_AGENTS));
  assert.ok(built.includes(STUB_BLOCK));

  const res = appendStub("Existing content");
  assert.ok(res.changed);
  assert.ok(res.updated.includes("Existing content"));
  assert.ok(res.updated.includes(STUB_BLOCK));

  const res2 = appendStub(STUB_BLOCK);
  assert.ok(!res2.changed);
  assert.strictEqual(res2.updated, STUB_BLOCK);

  assert.ok(hasStub(STUB_BLOCK));
  assert.ok(!hasStub(empty));
}

function testFormatHelpers() {
  const title = suggestTitle(" First line\nSecond line");
  assert.strictEqual(title, "First line");

  const summary = suggestSummary("This is a\nlong summary.");
  assert.ok(summary.includes("This is a long summary"));

  const truncated = truncate("a".repeat(200), 80);
  assert.ok(truncated.length <= 80);

  const doctor = formatDoctorReport({ ok: true, repo: { id: "r1" } });
  assert.ok(doctor.includes("Status: Ready"));

  const memoryResult = formatShowResult({
    kind: "memory",
    memory: {
      id: "M1",
      repo_id: "r1",
      title: "Title",
      summary: "Summary",
      created_at: "now"
    }
  });
  assert.ok(memoryResult.includes("# Title"));

  const chunkResult = formatShowResult({
    kind: "chunk",
    chunk: {
      id: "C1",
      repo_id: "r1",
      text: "chunk text",
      created_at: "now"
    }
  });
  assert.ok(chunkResult.includes("# Chunk C1"));
}

function testSessionLogicHelpers() {
  const normalizedA = normalizeForDiff("a  b\r\nc \n\n", false);
  const normalizedB = normalizeForDiff("a b\nc\n", false);
  assert.strictEqual(compactMeaningfulText(normalizedA), compactMeaningfulText(normalizedB));

  const pySensitive = isWhitespaceSensitiveExt("py", "src/main.py");
  assert.strictEqual(pySensitive, true);
  const normalizedPyA = normalizeForDiff("x  = 1\n", true);
  const normalizedPyB = normalizeForDiff("x = 1\n", true);
  assert.notStrictEqual(normalizedPyA, normalizedPyB);

  const compactA = compactMeaningfulText("alpha\n\n beta \n");
  const compactB = compactMeaningfulText("alpha\nbeta");
  assert.strictEqual(compactA, compactB);

  const stats = estimateDiffStats("a\nb\n", "a\nb\nc\n");
  assert.strictEqual(stats.linesAdded >= 1, true);
  assert.strictEqual(stats.deltaLines >= 1, true);
  const insertTopStats = estimateDiffStats(
    "line1\nline2\nline3\n",
    "header\nline1\nline2\nline3\n"
  );
  assert.strictEqual(insertTopStats.linesAdded, 1);
  assert.strictEqual(insertTopStats.linesRemoved, 0);

  const codeScore = computeSignificanceScore(12, 400, "src/app.ts");
  const docsScore = computeSignificanceScore(12, 400, "docs/readme.md");
  assert.strictEqual(codeScore > docsScore, true);
  const semanticBoost = computeSemanticBonus(
    "export function loadData() { return 1; }",
    "export function loadData(input: string) { return 1; }",
    "src/app.ts"
  );
  const baselineScore = computeSignificanceScore(2, 20, "src/app.ts");
  const boostedScore = computeSignificanceScore(2, 20, "src/app.ts", semanticBoost);
  assert.strictEqual(semanticBoost > 0, true);
  assert.strictEqual(boostedScore > baselineScore, true);

  const title = buildSessionTitle("/repo", [
    "internal/state/store.go",
    "internal/state/store_test.go",
    "src/ui/view.ts"
  ]);
  assert.strictEqual(title.startsWith("Session: Worked in internal"), true);
  assert.ok(title.includes("3 files"));

  const intentTitle = buildSessionTitle(
    "/repo",
    ["internal/state/store.go", "src/ui/view.ts"],
    "model training for neural network (accuracy 98%)"
  );
  assert.strictEqual(
    intentTitle.startsWith("Session: model training for neural network (accuracy 98%) in internal"),
    true
  );

  const entitiesDefault = buildSessionEntities(
    ["internal/state/store.go", "src/ui/view.ts"],
    "folders_exts"
  );
  assert.ok(entitiesDefault.includes("dir_internal"));
  assert.ok(entitiesDefault.includes("ext_go"));
  assert.strictEqual(entitiesDefault.some((entry) => entry.startsWith("file_")), false);

  const entitiesFiles = buildSessionEntities(
    ["internal/state/store.go", "src/ui/view.ts"],
    "files"
  );
  assert.strictEqual(entitiesFiles.some((entry) => entry.startsWith("file_")), true);

  const entitiesCounts = buildSessionEntities(
    ["internal/state/store.go", "src/ui/view.ts"],
    "counts"
  );
  assert.ok(entitiesCounts.includes("count_files_2"));
  assert.ok(entitiesCounts.includes("count_dirs_2"));
  assert.ok(entitiesCounts.includes("count_exts_2"));

  const rel = toRepoRelative("/repo", "/repo/src/app.ts");
  assert.strictEqual(rel, "src/app.ts");
  const outside = toRepoRelative("/repo", "/other/path.ts");
  assert.strictEqual(outside, "");

  assert.strictEqual(isAutoSessionTitle("Session: src (+3 files) [ts]"), true);
  assert.strictEqual(isAutoSessionTitle("Session: Worked in src (3 files, ts)"), true);
  assert.strictEqual(isAutoSessionTitle("Feature work"), false);
  assert.strictEqual(parseTimeMs("2026-01-01T00:00:00Z") > 0, true);
  assert.strictEqual(parseTimeMs("not-a-time"), 0);

  const intentSignal = extractTerminalIntentSignal(
    "python train.py --model naive_bayes --accuracy 98%"
  );
  assert.ok(intentSignal);
  assert.strictEqual(intentSignal.headline.includes("model training"), true);
  assert.strictEqual(intentSignal.headline.includes("naive bayes"), true);
  assert.strictEqual(intentSignal.entities.includes("intent_train"), true);
  assert.strictEqual(intentSignal.entities.includes("model_naive_bayes"), true);
  assert.strictEqual(
    intentSignal.entities.some((entry) => entry.startsWith("metric_accuracy_")),
    true
  );

  const commitSignal = extractTerminalIntentSignal("git commit -m \"reach 98% accuracy\"");
  assert.ok(commitSignal);
  assert.strictEqual(commitSignal.headline.includes("reach 98% accuracy"), true);
  assert.strictEqual(commitSignal.entities.includes("intent_commit"), true);

  const sensitiveSignal = extractTerminalIntentSignal("export OPENAI_API_KEY=sk-test");
  assert.strictEqual(sensitiveSignal, undefined);
}

function testSessionMergeDecision() {
  const base = {
    nowMs: 1_700_000_000_000,
    latestExists: true,
    latestIsAuto: true,
    latestCreatedAtMs: 1_700_000_000_000 - 30_000,
    mergeWindowMs: 300_000,
    lastAutoSessionAtMs: 1_700_000_000_000 - 200_000,
    minGapMs: 300_000
  };

  assert.strictEqual(
    decideSessionUpsertAction({ ...base, latestExists: false }),
    "create_new"
  );
  assert.strictEqual(
    decideSessionUpsertAction({ ...base, latestIsAuto: false }),
    "create_new"
  );
  assert.strictEqual(
    decideSessionUpsertAction(base),
    "update_latest"
  );
  assert.strictEqual(
    decideSessionUpsertAction({
      ...base,
      latestCreatedAtMs: base.nowMs - 600_000,
      lastAutoSessionAtMs: base.nowMs - 120_000
    }),
    "update_latest"
  );
  assert.strictEqual(
    decideSessionUpsertAction({
      ...base,
      latestCreatedAtMs: base.nowMs - 600_000,
      lastAutoSessionAtMs: base.nowMs - 600_000
    }),
    "create_new"
  );
  assert.strictEqual(
    decideSessionUpsertAction({
      ...base,
      latestCreatedAtMs: 0,
      lastAutoSessionAtMs: 0
    }),
    "create_new"
  );
}

async function testAutoSessionCaptureIntegration() {
  let nowMs = 1_700_000_000_000;
  const created = [];
  const updated = [];
  const saved = [];
  const recent = [];
  let intentSignal = undefined;

  const scheduler = {
    now: () => nowMs,
    setTimeout: () => 0,
    clearTimeout: () => {}
  };

  async function settleAsyncFlush() {
    await new Promise((resolve) => setImmediate(resolve));
    await new Promise((resolve) => setImmediate(resolve));
  }

  const persistence = {
    async listRecentSessions() {
      return recent.slice(0, 1);
    },
    async resolveThread() {
      return "T-SESSION";
    },
    async createSession(input) {
      const id = `M-${created.length + 1}`;
      created.push({ id, ...input });
      recent.unshift({
        id,
        title: input.title,
        created_at: new Date(nowMs).toISOString()
      });
      return { id };
    },
    async updateSession(input) {
      updated.push({ ...input });
      const session = recent.find((entry) => entry.id === input.id);
      if (session && input.title) {
        session.title = input.title;
      }
    },
    async onSessionSaved(input) {
      saved.push(input);
    }
  };

  const engine = new AutoSessionCaptureEngine(
    persistence,
    () => ({
      quietMs: 60_000,
      maxBurstMs: 600_000,
      scoreThreshold: 2,
      filesThreshold: 5,
      maxFilesPerSession: 50,
      mergeWindowMs: 300_000,
      newSessionMinGapMs: 300_000,
      maxFileBytes: 2_000_000,
      privacyMode: "folders_exts",
      needsSummary: true,
      intentSignal
    }),
    scheduler
  );

  await engine.recordSave({
    workspaceRoot: "/repo",
    filePath: "/repo/src/app.ts",
    text: "const a = 1;\n"
  });
  await engine.recordSave({
    workspaceRoot: "/repo",
    filePath: "/repo/src/app.ts",
    text: "const  a = 1;   \n"
  });
  await settleAsyncFlush();
  assert.strictEqual(created.length, 0);

  await engine.recordSave({
    workspaceRoot: "/repo",
    filePath: "/repo/src/app.ts",
    text: "const a = 1;\nconst b = 2;\nconst c = 3;\n"
  });
  await settleAsyncFlush();
  assert.strictEqual(created.length, 1);
  assert.strictEqual(created[0].thread, "T-SESSION");
  assert.strictEqual(created[0].tags.includes("needs_summary"), true);
  assert.strictEqual(Array.isArray(created[0].entities) && created[0].entities.length > 0, true);

  intentSignal = {
    headline: "model training for neural network (accuracy 98%)",
    entities: ["intent_train", "model_neural_network", "metric_accuracy_98"],
    observedAtMs: nowMs
  };

  const createdAfterFirst = created.length;
  const updatedAfterFirst = updated.length;
  await engine.recordSave({
    workspaceRoot: "/repo",
    filePath: "/repo/src/app.ts",
    text: "const a = 1;\nconst b = 2;\nconst c = 3;\n"
  });
  await settleAsyncFlush();
  assert.strictEqual(created.length, createdAfterFirst);
  assert.strictEqual(updated.length, updatedAfterFirst);

  nowMs += 60_000;
  await engine.recordSave({
    workspaceRoot: "/repo",
    filePath: "/repo/src/app.ts",
    text: "const a = 1;\nconst b = 2;\nconst c = 3;\nconst d = 4;\n"
  });
  await settleAsyncFlush();
  assert.strictEqual(created.length, createdAfterFirst);
  assert.strictEqual(updated.length > updatedAfterFirst, true);
  assert.strictEqual(updated[updated.length - 1].title.includes("model training for neural network"), true);
  assert.strictEqual(updated[updated.length - 1].entitiesAdd.includes("intent_train"), true);

  nowMs += 700_000;
  await engine.recordSave({
    workspaceRoot: "/repo",
    filePath: "/repo/src/app.ts",
    text: "const a = 1;\nconst b = 2;\nconst c = 3;\nconst d = 4;\nconst e = 5;\n"
  });
  await settleAsyncFlush();
  assert.strictEqual(created.length, createdAfterFirst + 1);

  recent[0].title = "Manual note";
  nowMs += 60_000;
  await engine.recordSave({
    workspaceRoot: "/repo",
    filePath: "/repo/src/app.ts",
    text: "const a = 1;\nconst b = 2;\nconst c = 3;\nconst d = 4;\nconst e = 5;\nconst f = 6;\n"
  });
  await settleAsyncFlush();
  assert.strictEqual(created.length, createdAfterFirst + 2);

  engine.dispose();
}

async function testAutoSessionFlushPreservesPendingChanges() {
  let nowMs = 1_700_000_000_000;
  const created = [];
  const recent = [];
  let firstFlushStarted = false;
  let releaseFirstFlush = () => {};
  const firstFlushGate = new Promise((resolve) => {
    releaseFirstFlush = resolve;
  });

  const scheduler = {
    now: () => nowMs,
    setTimeout: () => 0,
    clearTimeout: () => {}
  };

  async function settleAsyncFlush() {
    await new Promise((resolve) => setImmediate(resolve));
    await new Promise((resolve) => setImmediate(resolve));
    await new Promise((resolve) => setImmediate(resolve));
  }

  async function waitForCreatedCount(expectedCount) {
    const deadline = Date.now() + 2_000;
    while (Date.now() < deadline) {
      if (created.length >= expectedCount) {
        return;
      }
      await settleAsyncFlush();
      await new Promise((resolve) => setTimeout(resolve, 5));
    }
    throw new Error(`timed out waiting for created count ${expectedCount}, got ${created.length}`);
  }

  const engine = new AutoSessionCaptureEngine(
    {
      async listRecentSessions() {
        return recent.slice(0, 1);
      },
      async resolveThread() {
        return "T-SESSION";
      },
      async createSession(input) {
        const createdAtMs = nowMs;
        if (!firstFlushStarted) {
          firstFlushStarted = true;
          await firstFlushGate;
        }
        const id = `M-${created.length + 1}`;
        created.push({ id, ...input });
        recent.unshift({
          id,
          title: input.title,
          created_at: new Date(createdAtMs).toISOString()
        });
        return { id };
      },
      async updateSession() {
        throw new Error("unexpected update path");
      }
    },
    () => ({
      quietMs: 60_000,
      maxBurstMs: 600_000,
      scoreThreshold: 1,
      filesThreshold: 5,
      maxFilesPerSession: 50,
      mergeWindowMs: 0,
      newSessionMinGapMs: 0,
      maxFileBytes: 2_000_000,
      privacyMode: "folders_exts",
      needsSummary: false
    }),
    scheduler
  );

  try {
    await engine.recordSave({
      workspaceRoot: "/repo",
      filePath: "/repo/src/app.ts",
      text: "const a = 1;\n"
    });

    await engine.recordSave({
      workspaceRoot: "/repo",
      filePath: "/repo/src/app.ts",
      text: "const a = 1;\nconst b = 2;\n"
    });
    await settleAsyncFlush();
    assert.strictEqual(firstFlushStarted, true);

    nowMs += 1_000;
    await engine.recordSave({
      workspaceRoot: "/repo",
      filePath: "/repo/src/app.ts",
      text: "const a = 1;\nconst b = 2;\nconst c = 3;\n"
    });

    releaseFirstFlush();
    await waitForCreatedCount(2);
    assert.strictEqual(created.length, 2);
  } finally {
    engine.dispose();
  }
}

async function testAutoSessionLifecycleAndIgnore() {
  const tmpRoot = await fs.mkdtemp(path.join(os.tmpdir(), "mempack-auto-session-"));
  const workspaceRoot = path.join(tmpRoot, "repo");
  await fs.mkdir(path.join(workspaceRoot, "src"), { recursive: true });
  await fs.writeFile(path.join(workspaceRoot, ".mempackignore"), "ignored/**\n");
  await fs.mkdir(path.join(workspaceRoot, "ignored"), { recursive: true });

  let nowMs = 1_700_000_000_000;
  const created = [];
  const updated = [];
  const recent = [];

  const scheduler = {
    now: () => nowMs,
    setTimeout: () => 0,
    clearTimeout: () => {}
  };

  async function settleAsyncFlush() {
    await new Promise((resolve) => setImmediate(resolve));
    await new Promise((resolve) => setImmediate(resolve));
  }

  const engine = new AutoSessionCaptureEngine(
    {
      async listRecentSessions() {
        return recent.slice(0, 1);
      },
      async resolveThread() {
        return "T-SESSION";
      },
      async createSession(input) {
        const id = `M-${created.length + 1}`;
        created.push({ id, ...input });
        recent.unshift({
          id,
          title: input.title,
          created_at: new Date(nowMs).toISOString()
        });
        return { id };
      },
      async updateSession(input) {
        updated.push({ ...input });
      }
    },
    () => ({
      quietMs: 60_000,
      maxBurstMs: 600_000,
      scoreThreshold: 30,
      filesThreshold: 5,
      maxFilesPerSession: 50,
      mergeWindowMs: 300_000,
      newSessionMinGapMs: 300_000,
      maxFileBytes: 2_000_000,
      privacyMode: "counts",
      needsSummary: false,
      ignoredSegments: []
    }),
    scheduler
  );

  const createdPath = path.join(workspaceRoot, "src", "created.ts");
  await fs.writeFile(createdPath, "export function createdOne() { return 1; }\n");
  const createdText = await fs.readFile(createdPath, "utf8");
  await engine.recordCreate({
    workspaceRoot,
    filePath: createdPath,
    text: createdText
  });
  await settleAsyncFlush();
  assert.strictEqual(created.length, 1);
  assert.strictEqual(
    Array.isArray(created[0].entities) && created[0].entities.includes("count_files_1"),
    true
  );

  const renamedPath = path.join(workspaceRoot, "src", "renamed.ts");
  await fs.rename(createdPath, renamedPath);
  const renamedText = await fs.readFile(renamedPath, "utf8");
  nowMs += 60_000;
  await engine.recordRename({
    workspaceRoot,
    oldFilePath: createdPath,
    newFilePath: renamedPath,
    newText: renamedText
  });
  await settleAsyncFlush();
  assert.strictEqual(updated.length > 0, true);

  await fs.unlink(renamedPath);
  nowMs += 60_000;
  await engine.recordDelete({
    workspaceRoot,
    filePath: renamedPath
  });
  await settleAsyncFlush();
  assert.strictEqual(updated.length >= 2, true);

  const ignoredPath = path.join(workspaceRoot, "ignored", "skip.ts");
  await fs.writeFile(ignoredPath, "export function skipped() { return 1; }\n");
  const ignoredText = await fs.readFile(ignoredPath, "utf8");
  const createdBeforeIgnore = created.length;
  const updatedBeforeIgnore = updated.length;
  nowMs += 60_000;
  await engine.recordSave({
    workspaceRoot,
    filePath: ignoredPath,
    text: ignoredText
  });
  await engine.recordSave({
    workspaceRoot,
    filePath: ignoredPath,
    text: `${ignoredText}\nexport const x = 1;\n`
  });
  await settleAsyncFlush();
  assert.strictEqual(created.length, createdBeforeIgnore);
  assert.strictEqual(updated.length, updatedBeforeIgnore);

  engine.dispose();
}

async function runTests() {
  try {
    testStubHelpers();
    testFormatHelpers();
    testSessionLogicHelpers();
    testSessionMergeDecision();
    await testAutoSessionCaptureIntegration();
    await testAutoSessionFlushPreservesPendingChanges();
    await testAutoSessionLifecycleAndIgnore();
    console.log("Extension unit tests: ok");
  } catch (err) {
    console.error("Extension unit tests: failed", err);
    process.exit(1);
  }
}

void runTests();
