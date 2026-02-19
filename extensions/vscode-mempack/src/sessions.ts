import { execFile } from "child_process";
import { promisify } from "util";
import * as vscode from "vscode";
import * as fs from "fs";
import * as path from "path";
import { MempackClient } from "./client";
import { SessionItem } from "./types";
import { getWorkspaceRoot } from "./workspace";
import {
  AutoSessionCaptureEngine,
  AutoSessionConfig,
  AutoSessionIntentSignal
} from "./auto_session_capture";
import { formatShowResult } from "./format";
import {
  clampNumber,
  extractTerminalIntentSignal,
  PrivacyMode
} from "./session_logic";

const execFileAsync = promisify(execFile);

interface GitRepositoryState {
  HEAD?: { commit?: string; name?: string };
  onDidChange: vscode.Event<void>;
}

interface GitRepository {
  rootUri: vscode.Uri;
  state: GitRepositoryState;
}

interface GitAPI {
  repositories: GitRepository[];
  onDidOpenRepository?: vscode.Event<GitRepository>;
  onDidCloseRepository?: vscode.Event<GitRepository>;
}

type SensitivityMode = "low" | "balanced" | "high";

export class SessionManager implements vscode.Disposable {
  private client: MempackClient;
  private output: vscode.OutputChannel;
  private statusItem: vscode.StatusBarItem;
  private state: vscode.Memento;
  private autoCapture: AutoSessionCaptureEngine;
  private gitApi?: GitAPI;
  private lastSeenByRepo = new Map<string, string>();
  private lastNudgedByRepo = new Map<string, string>();
  private recentTerminalIntentByWorkspace = new Map<string, AutoSessionIntentSignal>();
  private inFlight = new Set<string>();
  private repoSubscriptions = new Map<string, vscode.Disposable>();
  private lastAutoSessionAtByRoot = new Map<string, number>();
  private disposables: vscode.Disposable[] = [];
  private pollingTimer?: ReturnType<typeof setInterval>;
  private refreshTimer?: ReturnType<typeof setInterval>;
  private autoCaptureToggleBusy = false;

  constructor(client: MempackClient, output: vscode.OutputChannel, state: vscode.Memento) {
    this.client = client;
    this.output = output;
    this.state = state;
    this.statusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left, 100);
    this.statusItem.command = "mempack.annotateLastSession";
    this.statusItem.tooltip = "Annotate last session";
    this.autoCapture = new AutoSessionCaptureEngine(
      {
        listRecentSessions: (workspaceRoot, limit) =>
          this.client.sessions(workspaceRoot, { limit }),
        resolveThread: (workspaceRoot) => this.resolveThreadForWorkspace(workspaceRoot),
        createSession: async (input) => {
          const created = await this.client.addMemory(
            input.workspaceRoot,
            input.thread,
            input.title,
            "",
            input.tags.join(","),
            input.entities
          );
          return { id: created.id };
        },
        updateSession: async (input) => {
          await this.client.updateMemory(input.workspaceRoot, input.id, {
            title: input.title,
            tagsAdd: input.tagsAdd,
            tagsRemove: input.tagsRemove,
            entities: input.entities,
            entitiesAdd: input.entitiesAdd
          });
        },
        onSessionSaved: async (input) => {
          this.lastAutoSessionAtByRoot.set(this.normalizeRepoPath(input.workspaceRoot), Date.now());
          await this.maybeToastNeedsSummary(input.workspaceRoot, `auto:${input.sessionID}`, input.needsSummary);
          await this.refreshBadge();
        }
      },
      (workspaceRoot) => this.getAutoSessionConfig(workspaceRoot)
    );
  }

  async start(): Promise<void> {
    this.statusItem.hide();
    await this.refreshBadge();
    // One-time consent for auto capture (persisted in extension state).
    this.disposables.push(
      vscode.workspace.onDidChangeConfiguration((e) => {
        if (e.affectsConfiguration("mempack.autoSessionsEnabled")) {
          void this.handleAutoCaptureToggle();
        }
      })
    );
    // If auto capture is already enabled explicitly, treat that as consent.
    void this.maybeBackfillAutoCaptureConsent();
    this.disposables.push(
      vscode.workspace.onDidSaveTextDocument((document) => {
        void this.handleDocumentSave(document);
      })
    );
    this.disposables.push(
      vscode.workspace.onDidCreateFiles((event) => {
        void this.handleFileCreateEvent(event);
      })
    );
    this.disposables.push(
      vscode.workspace.onDidDeleteFiles((event) => {
        void this.handleFileDeleteEvent(event);
      })
    );
    this.disposables.push(
      vscode.workspace.onDidRenameFiles((event) => {
        void this.handleFileRenameEvent(event);
      })
    );
    const windowWithShellExecution = vscode.window as typeof vscode.window & {
      onDidStartTerminalShellExecution?: vscode.Event<vscode.TerminalShellExecutionStartEvent>;
    };
    if (windowWithShellExecution.onDidStartTerminalShellExecution) {
      this.disposables.push(
        windowWithShellExecution.onDidStartTerminalShellExecution((event) => {
          void this.handleTerminalExecutionStart(event);
        })
      );
    }
    if (this.isGitSessionCaptureEnabled()) {
      await this.initGitWatcher();
    }
    this.refreshTimer = setInterval(() => {
      void this.refreshBadge();
    }, 60_000);
  }

  async refreshStatus(): Promise<void> {
    await this.refreshBadge();
  }

  dispose(): void {
    if (this.pollingTimer) {
      clearInterval(this.pollingTimer);
    }
    if (this.refreshTimer) {
      clearInterval(this.refreshTimer);
    }
    for (const disposable of this.repoSubscriptions.values()) {
      disposable.dispose();
    }
    this.repoSubscriptions.clear();
    this.autoCapture.dispose();
    this.recentTerminalIntentByWorkspace.clear();
    this.lastAutoSessionAtByRoot.clear();
    for (const disposable of this.disposables) {
      disposable.dispose();
    }
    this.disposables = [];
    this.statusItem.dispose();
  }

  async annotateLastSession(): Promise<void> {
    const repoRoot = await this.resolveActiveWorkspaceRoot();
    if (!repoRoot) {
      vscode.window.showErrorMessage("Open a workspace to annotate sessions.");
      return;
    }

    let sessions: SessionItem[] = [];
    try {
      sessions = await this.client.sessions(repoRoot, { needsSummary: true, limit: 1 });
    } catch (err: any) {
      this.logError("sessions lookup failed", err);
      vscode.window.showErrorMessage(this.formatError(err));
      return;
    }

    const last = sessions[0];
    if (!last) {
      vscode.window.showInformationMessage("No sessions need a summary.");
      return;
    }

    const summary = await vscode.window.showInputBox({
      prompt: `Why: ${last.title}`,
      placeHolder: "What was the intent behind this change?",
      ignoreFocusOut: true
    });
    if (!summary || summary.trim() === "") {
      return;
    }

    try {
      const nextSummary = await this.buildSessionSummary(repoRoot, last, summary.trim());
      await this.client.updateMemory(repoRoot, last.id, {
        summary: nextSummary,
        tagsAdd: ["session"],
        tagsRemove: ["needs_summary"]
      });
      await this.refreshBadge();
    } catch (err: any) {
      this.logError("session annotate failed", err);
      vscode.window.showErrorMessage(this.formatError(err));
    }
  }

  async annotateSessionFromList(): Promise<void> {
    const repoRoot = await this.resolveActiveWorkspaceRoot();
    if (!repoRoot) {
      vscode.window.showErrorMessage("Open a workspace to annotate sessions.");
      return;
    }

    let sessions: SessionItem[] = [];
    try {
      sessions = await this.client.sessions(repoRoot, { needsSummary: true, limit: 20 });
    } catch (err: any) {
      this.logError("sessions lookup failed", err);
      vscode.window.showErrorMessage(this.formatError(err));
      return;
    }

    if (sessions.length === 0) {
      vscode.window.showInformationMessage("No sessions need a summary.");
      return;
    }

    const pick = await vscode.window.showQuickPick(
      sessions.map((session) => ({
        label: session.title || "(untitled)",
        description: session.created_at ? formatDate(session.created_at) : "",
        session
      })),
      { placeHolder: "Select a session to annotate" }
    );
    if (!pick) {
      return;
    }

    const summary = await vscode.window.showInputBox({
      prompt: `Why: ${pick.session.title}`,
      placeHolder: "What was the intent behind this change?",
      ignoreFocusOut: true
    });
    if (!summary || summary.trim() === "") {
      return;
    }

    try {
      const nextSummary = await this.buildSessionSummary(repoRoot, pick.session, summary.trim());
      await this.client.updateMemory(repoRoot, pick.session.id, {
        summary: nextSummary,
        tagsAdd: ["session"],
        tagsRemove: ["needs_summary"]
      });
      await this.refreshBadge();
    } catch (err: any) {
      this.logError("session annotate failed", err);
      vscode.window.showErrorMessage(this.formatError(err));
    }
  }

  async annotateSession(session: SessionItem): Promise<void> {
    const repoRoot = await this.resolveActiveWorkspaceRoot();
    if (!repoRoot) {
      vscode.window.showErrorMessage("Open a workspace to annotate sessions.");
      return;
    }
    const summary = await vscode.window.showInputBox({
      prompt: `Why: ${session.title}`,
      placeHolder: "What was the intent behind this change?",
      ignoreFocusOut: true
    });
    if (!summary || summary.trim() === "") {
      return;
    }
    try {
      const nextSummary = await this.buildSessionSummary(repoRoot, session, summary.trim());
      await this.client.updateMemory(repoRoot, session.id, {
        summary: nextSummary,
        tagsAdd: ["session"],
        tagsRemove: ["needs_summary"]
      });
      await this.refreshBadge();
    } catch (err: any) {
      this.logError("session annotate failed", err);
      vscode.window.showErrorMessage(this.formatError(err));
    }
  }

  async markSessionReviewed(session: SessionItem): Promise<void> {
    const repoRoot = await this.resolveActiveWorkspaceRoot();
    if (!repoRoot) {
      vscode.window.showErrorMessage("Open a workspace to manage sessions.");
      return;
    }
    try {
      await this.client.updateMemory(repoRoot, session.id, {
        tagsAdd: ["session"],
        tagsRemove: ["needs_summary"]
      });
      await this.refreshBadge();
    } catch (err: any) {
      this.logError("mark reviewed failed", err);
      vscode.window.showErrorMessage(this.formatError(err));
    }
  }

  async copySessionReference(session: SessionItem): Promise<void> {
    const commit = session.anchor_commit ? ` commit:${session.anchor_commit}` : "";
    const text = `mem show ${session.id}${commit ? `  #${commit}` : ""}`.trim();
    await vscode.env.clipboard.writeText(text);
    vscode.window.showInformationMessage("Session reference copied.");
  }

  async openSessionDiff(session: SessionItem): Promise<void> {
    const repoRoot = await this.resolveActiveRepoRoot();
    if (!repoRoot) {
      vscode.window.showErrorMessage("Open a git repository to view diffs.");
      return;
    }
    const sha = session.anchor_commit || "";
    if (sha.trim() === "") {
      vscode.window.showErrorMessage("No commit SHA found for this session.");
      return;
    }
    try {
      const result = await execFileAsync("git", ["-C", repoRoot, "show", "--patch", sha], {
        timeout: 20_000,
        maxBuffer: 50 * 1024 * 1024
      });
      const content = (result.stdout || "").trimEnd();
      const doc = await vscode.workspace.openTextDocument({ content, language: "diff" });
      await vscode.window.showTextDocument(doc, { preview: false });
    } catch (err: any) {
      this.logError("open diff failed", err);
      vscode.window.showErrorMessage(this.formatError(err));
    }
  }

  private async initGitWatcher(): Promise<void> {
    const gitExtension = vscode.extensions.getExtension("vscode.git");
    if (!gitExtension) {
      this.startPolling();
      return;
    }

    try {
      if (!gitExtension.isActive) {
        await gitExtension.activate();
      }
    } catch (err) {
      this.logError("git extension activation failed", err);
    }

    const api = (gitExtension.exports as { getAPI?: (version: number) => GitAPI })?.getAPI?.(1);
    if (!api) {
      this.startPolling();
      return;
    }

    this.gitApi = api;
    for (const repo of api.repositories) {
      this.trackRepository(repo);
    }
    if (api.onDidOpenRepository) {
      this.disposables.push(api.onDidOpenRepository((repo) => this.trackRepository(repo)));
    }
    if (api.onDidCloseRepository) {
      this.disposables.push(api.onDidCloseRepository((repo) => this.untrackRepository(repo)));
    }
  }

  private trackRepository(repo: GitRepository): void {
    const root = repo.rootUri.fsPath;
    if (this.repoSubscriptions.has(root)) {
      return;
    }
    const head = repo.state.HEAD?.commit;
    if (head) {
      this.lastSeenByRepo.set(root, head);
      void this.primeLastProcessedCommit(root, head);
    }
    const disposable = repo.state.onDidChange(() => {
      void this.handleRepoChange(repo);
    });
    this.repoSubscriptions.set(root, disposable);
  }

  private untrackRepository(repo: GitRepository): void {
    const root = repo.rootUri.fsPath;
    const disposable = this.repoSubscriptions.get(root);
    if (disposable) {
      disposable.dispose();
      this.repoSubscriptions.delete(root);
    }
  }

  private async handleRepoChange(repo: GitRepository): Promise<void> {
    if (!this.isIntentCaptureEnabled() || !this.isGitSessionCaptureEnabled()) {
      return;
    }
    const head = repo.state.HEAD?.commit;
    if (!head) {
      return;
    }
    const root = repo.rootUri.fsPath;
    const last = this.lastSeenByRepo.get(root);
    if (last === head) {
      return;
    }
    if (this.inFlight.has(root)) {
      return;
    }
    if (this.getLastProcessedCommit(root) === head) {
      this.lastSeenByRepo.set(root, head);
      return;
    }

    this.lastSeenByRepo.set(root, head);
    this.inFlight.add(root);
    try {
      const branch = repo.state.HEAD?.name || "";
      await this.captureSession(root, head, branch);
    } finally {
      this.inFlight.delete(root);
    }
  }

  private normalizeRepoPath(cwd: string): string {
    const resolved = path.resolve(cwd);
    try {
      return fs.realpathSync(resolved);
    } catch {
      return resolved;
    }
  }

  private lastCommitKey(repoRoot: string): string {
    return `${this.normalizeRepoPath(repoRoot)}:lastCommit`;
  }

  private getLastProcessedCommit(repoRoot: string): string {
    const key = this.lastCommitKey(repoRoot);
    return this.state.get<string>(key, "").trim();
  }

  private async setLastProcessedCommit(repoRoot: string, sha: string): Promise<void> {
    const key = this.lastCommitKey(repoRoot);
    await this.state.update(key, sha.trim());
  }

  private async primeLastProcessedCommit(repoRoot: string, head: string): Promise<void> {
    const normalized = head.trim();
    if (normalized === "") {
      return;
    }
    const key = this.lastCommitKey(repoRoot);
    const current = this.state.get<string>(key, "").trim();
    if (current !== "") {
      return;
    }
    await this.state.update(key, normalized);
  }

  private startPolling(): void {
    if (this.pollingTimer) {
      return;
    }
    this.pollingTimer = setInterval(() => {
      void this.pollActiveRepo();
    }, 2_000);
    void this.pollActiveRepo();
  }

  private async pollActiveRepo(): Promise<void> {
    if (!this.isIntentCaptureEnabled() || !this.isGitSessionCaptureEnabled()) {
      return;
    }
    const repoRoot = await this.resolveActiveRepoRoot();
    if (!repoRoot) {
      return;
    }
    let head = "";
    try {
      head = await this.gitHead(repoRoot);
    } catch {
      return;
    }
    if (head === "") {
      return;
    }
    const last = this.lastSeenByRepo.get(repoRoot);
    if (!last) {
      this.lastSeenByRepo.set(repoRoot, head);
      await this.primeLastProcessedCommit(repoRoot, head);
      return;
    }
    if (last === head) {
      return;
    }
    if (this.inFlight.has(repoRoot)) {
      return;
    }
    if (this.getLastProcessedCommit(repoRoot) === head) {
      this.lastSeenByRepo.set(repoRoot, head);
      return;
    }
    this.lastSeenByRepo.set(repoRoot, head);
    this.inFlight.add(repoRoot);
    try {
      await this.captureSession(repoRoot, head, "");
    } finally {
      this.inFlight.delete(repoRoot);
    }
  }

  private async captureSession(repoRoot: string, sha: string, branchHint: string): Promise<void> {
    if (!this.isIntentCaptureEnabled() || !this.isGitSessionCaptureEnabled()) {
      return;
    }
    const normalizedRoot = this.normalizeRepoPath(repoRoot);
    if (this.isAutoSessionsEnabled() && this.hasAutoCaptureConsent(normalizedRoot)) {
      try {
        await this.autoCapture.flushWorkspace(normalizedRoot, true);
      } catch (err) {
        this.logError("auto session pre-flush failed", err);
      }
    }
    const mergeWindowMs = this.getMergeWindowMs();
    const lastAuto = this.lastAutoSessionAtByRoot.get(normalizedRoot) || 0;
    if (Date.now() - lastAuto < mergeWindowMs) {
      await this.setLastProcessedCommit(normalizedRoot, sha);
      return;
    }
    let subject = "";
    let body = "";
    let branch = normalizeBranch(branchHint);
    try {
      const info = await this.getCommitMessage(normalizedRoot, sha);
      subject = info.subject;
      body = info.body;
      if (!branch) {
        branch = await this.gitBranch(normalizedRoot);
      }
    } catch (err) {
      this.logError("commit lookup failed", err);
      return;
    }

    const title = subject !== "" ? subject : sha.slice(0, 7);
    const commitBody = body.trim();
    let summary = commitBody;
    const thread = this.threadFromBranch(branch);
    const needsSummary = this.shouldMarkNeedsSummary(commitBody);
    const tags = needsSummary ? "session,needs_summary" : "session";

    if (this.getAttachFilesEnabled()) {
      try {
        const files = await this.getChangedFiles(normalizedRoot, sha, 50);
        summary = appendFilesBlock(summary, files);
      } catch (err) {
        this.logError("failed to attach files list", err);
      }
    }

    try {
      await this.client.addMemory(normalizedRoot, thread, title, summary, tags);
      await this.setLastProcessedCommit(normalizedRoot, sha);
      await this.maybeToastNeedsSummary(normalizedRoot, sha, needsSummary);
      await this.refreshBadge();
    } catch (err) {
      this.logError("session write failed", err);
    }
  }

  private async refreshBadge(): Promise<void> {
    if (!this.isIntentCaptureEnabled() || this.getNudgeStyle() === "off") {
      this.statusItem.hide();
      return;
    }
    const roots = await this.resolveRepoRoots();
    if (roots.length === 0) {
      this.statusItem.hide();
      return;
    }
    let total = 0;
    for (const root of roots) {
      try {
        total += await this.client.sessionsCount(root, true);
      } catch (err) {
        this.logError("sessions count failed", err);
      }
    }
    if (total > 0) {
      this.statusItem.text = `📝 ${total}`;
      this.statusItem.show();
    } else {
      this.statusItem.hide();
    }
  }

  private async resolveRepoRoots(): Promise<string[]> {
    const roots = new Set<string>();
    if (this.gitApi && this.gitApi.repositories.length > 0) {
      for (const repo of this.gitApi.repositories) {
        roots.add(repo.rootUri.fsPath);
      }
    }
    if (roots.size === 0) {
      const active = await this.resolveActiveWorkspaceRoot();
      if (active) {
        roots.add(active);
      }
    }
    return Array.from(roots);
  }

  private async resolveActiveWorkspaceRoot(): Promise<string | undefined> {
    const active = vscode.window.activeTextEditor?.document?.uri;
    const workspaceRoot = getWorkspaceRoot(active);
    if (!workspaceRoot) {
      return undefined;
    }
    return this.normalizeRepoPath(workspaceRoot);
  }

  private async resolveActiveRepoRoot(): Promise<string | undefined> {
    const workspaceRoot = await this.resolveActiveWorkspaceRoot();
    if (!workspaceRoot) {
      return undefined;
    }
    const gitRoot = await this.gitTopLevel(workspaceRoot);
    if (!gitRoot) {
      return undefined;
    }
    return gitRoot;
  }

  private async gitTopLevel(cwd: string): Promise<string | undefined> {
    try {
      const result = await execFileAsync("git", ["-C", cwd, "rev-parse", "--show-toplevel"], {
        timeout: 5000,
        maxBuffer: 1024 * 1024
      });
      const value = (result.stdout || "").trim();
      return value === "" ? undefined : value;
    } catch {
      return undefined;
    }
  }

  private async gitHead(cwd: string): Promise<string> {
    const result = await execFileAsync("git", ["-C", cwd, "rev-parse", "HEAD"], {
      timeout: 5000,
      maxBuffer: 1024 * 1024
    });
    return (result.stdout || "").trim();
  }

  private async getCommitMessage(
    repoRoot: string,
    sha: string
  ): Promise<{ subject: string; body: string }> {
    const logResult = await execFileAsync(
      "git",
      ["-C", repoRoot, "log", "-1", "--pretty=format:%s%x00%b", sha],
      { timeout: 5000, maxBuffer: 5 * 1024 * 1024 }
    );
    const output = (logResult.stdout || "").trimEnd();
    const parts = output.split("\u0000");
    const subject = (parts[0] || "").trim();
    const body = (parts[1] || "").trim();
    return { subject, body };
  }

  private async gitBranch(cwd: string): Promise<string> {
    const branchResult = await execFileAsync(
      "git",
      ["-C", cwd, "rev-parse", "--abbrev-ref", "HEAD"],
      { timeout: 5000, maxBuffer: 1024 * 1024 }
    );
    const rawBranch = (branchResult.stdout || "").trim();
    return normalizeBranch(rawBranch);
  }

  private formatError(err: unknown): string {
    const message = err instanceof Error ? err.message : String(err);
    return message.replace(/\s+/g, " ").trim();
  }

  private logError(context: string, err: unknown): void {
    this.output.appendLine(`[sessions] ${context}: ${this.formatError(err)}`);
  }

  private isIntentCaptureEnabled(): boolean {
    return vscode.workspace
      .getConfiguration("mempack")
      .get<boolean>("intentCaptureEnabled", true);
  }

  private isAutoSessionsEnabled(): boolean {
    return vscode.workspace
      .getConfiguration("mempack")
      .get<boolean>("autoSessionsEnabled", false);
  }

  private isGitSessionCaptureEnabled(): boolean {
    return vscode.workspace
      .getConfiguration("mempack")
      .get<boolean>("intentCaptureGitSessions", true);
  }

  private getAttachFilesEnabled(): boolean {
    return vscode.workspace
      .getConfiguration("mempack")
      .get<boolean>("intentCaptureAttachFiles", false);
  }

  private getNudgeStyle(): "badge" | "badgeToast" | "off" {
    return vscode.workspace
      .getConfiguration("mempack")
      .get<"badge" | "badgeToast" | "off">("intentCaptureNudge", "badge");
  }

  private getNeedsSummaryRule(): "emptyBody" | "always" | "never" {
    return vscode.workspace
      .getConfiguration("mempack")
      .get<"emptyBody" | "always" | "never">("intentCaptureNeedsSummary", "emptyBody");
  }

  private useThreadFromBranch(): boolean {
    return vscode.workspace
      .getConfiguration("mempack")
      .get<boolean>("intentCaptureThreadFromBranch", true);
  }

  private getSensitivityMode(): SensitivityMode {
    const mode = vscode.workspace
      .getConfiguration("mempack")
      .get<string>("autoSessionsSensitivity", "balanced")
      .trim()
      .toLowerCase();
    if (mode === "low" || mode === "high") {
      return mode;
    }
    return "balanced";
  }

  private getPrivacyMode(): PrivacyMode {
    const mode = vscode.workspace
      .getConfiguration("mempack")
      .get<string>("autoSessionsPrivacy", "folders_exts")
      .trim()
      .toLowerCase();
    if (mode === "files" || mode === "counts") {
      return mode;
    }
    return "folders_exts";
  }

  private getQuietMs(): number {
    const configured = vscode.workspace
      .getConfiguration("mempack")
      .get<number>("autoSessionsQuietMs", 90_000);
    return clampNumber(configured, 10_000, 600_000, 90_000);
  }

  private getMaxBurstMs(): number {
    const configured = vscode.workspace
      .getConfiguration("mempack")
      .get<number>("autoSessionsMaxBurstMs", 600_000);
    return clampNumber(configured, 60_000, 3_600_000, 600_000);
  }

  private getMergeWindowMs(): number {
    const configured = vscode.workspace
      .getConfiguration("mempack")
      .get<number>("autoSessionsMergeWindowMs", 300_000);
    return clampNumber(configured, 60_000, 3_600_000, 300_000);
  }

  private getNewSessionMinGapMs(): number {
    const configured = vscode.workspace
      .getConfiguration("mempack")
      .get<number>("autoSessionsNewSessionMinGapMs", 300_000);
    return clampNumber(configured, 60_000, 3_600_000, 300_000);
  }

  private getMaxFileBytes(): number {
    const configured = vscode.workspace
      .getConfiguration("mempack")
      .get<number>("autoSessionsMaxFileBytes", 2_000_000);
    return clampNumber(configured, 8_192, 20_000_000, 2_000_000);
  }

  private getMaxFilesPerSession(): number {
    const configured = vscode.workspace
      .getConfiguration("mempack")
      .get<number>("autoSessionsMaxFilesPerSession", 50);
    return clampNumber(configured, 5, 200, 50);
  }

  private getIgnoredPatterns(): string[] {
    const configured = vscode.workspace
      .getConfiguration("mempack")
      .get<string[]>("autoSessionsIgnoreGlobs", []);
    if (!Array.isArray(configured)) {
      return [];
    }
    return configured
      .map((value) => String(value || "").trim())
      .filter((value) => value !== "");
  }

  private getTerminalIntentMaxAgeMs(): number {
    return 10 * 60_000;
  }

  private getTerminalIntentSignal(workspaceRoot: string): AutoSessionIntentSignal | undefined {
    const normalizedRoot = this.normalizeRepoPath(workspaceRoot);
    const signal = this.recentTerminalIntentByWorkspace.get(normalizedRoot);
    if (!signal) {
      return undefined;
    }
    const maxAgeMs = this.getTerminalIntentMaxAgeMs();
    if (Date.now() - signal.observedAtMs > maxAgeMs) {
      this.recentTerminalIntentByWorkspace.delete(normalizedRoot);
      return undefined;
    }
    return signal;
  }

  private getScoreThreshold(): number {
    const mode = this.getSensitivityMode();
    if (mode === "low") {
      return 100;
    }
    if (mode === "high") {
      return 40;
    }
    return 60;
  }

  private getFilesThreshold(): number {
    const mode = this.getSensitivityMode();
    if (mode === "low") {
      return 8;
    }
    if (mode === "high") {
      return 3;
    }
    return 5;
  }

  private getAutoSessionConfig(workspaceRoot: string): AutoSessionConfig {
    const intentSignal = this.getTerminalIntentSignal(workspaceRoot);
    return {
      quietMs: this.getQuietMs(),
      maxBurstMs: this.getMaxBurstMs(),
      scoreThreshold: this.getScoreThreshold(),
      filesThreshold: this.getFilesThreshold(),
      maxFilesPerSession: this.getMaxFilesPerSession(),
      mergeWindowMs: this.getMergeWindowMs(),
      newSessionMinGapMs: this.getNewSessionMinGapMs(),
      maxFileBytes: this.getMaxFileBytes(),
      privacyMode: this.getPrivacyMode(),
      needsSummary: this.shouldMarkNeedsSummary(""),
      ignoredSegments: this.getIgnoredPatterns(),
      intentSignal,
      intentSignalMaxAgeMs: this.getTerminalIntentMaxAgeMs()
    };
  }

  private autoCaptureConsentKey(workspaceRoot: string): string {
    return `mempack.autoCaptureConsent.v1:${this.normalizeRepoPath(workspaceRoot)}`;
  }

  private hasAutoCaptureConsent(workspaceRoot: string): boolean {
    return this.state.get<boolean>(this.autoCaptureConsentKey(workspaceRoot), false) === true;
  }

  private async setAutoCaptureConsent(workspaceRoot: string): Promise<void> {
    await this.state.update(this.autoCaptureConsentKey(workspaceRoot), true);
  }

  private async resolveAnyWorkspaceRoot(): Promise<string | undefined> {
    const active = vscode.window.activeTextEditor?.document?.uri;
    const cwd = getWorkspaceRoot(active);
    if (cwd) {
      return this.normalizeRepoPath(cwd);
    }
    const folders = vscode.workspace.workspaceFolders || [];
    if (folders.length > 0) {
      return this.normalizeRepoPath(folders[0].uri.fsPath);
    }
    return undefined;
  }

  // Ensures the user has acknowledged auto-capture for this workspace/repo.
  // Commands can call this before flipping the setting to avoid a race with the config-change listener.
  async ensureAutoCaptureConsent(): Promise<boolean> {
    const root = await this.resolveAnyWorkspaceRoot();
    if (!root) {
      return false;
    }
    if (this.hasAutoCaptureConsent(root)) {
      return true;
    }

    const choice = await vscode.window.showWarningMessage(
      "Enable auto-capture? Mempack will save lightweight session memories for this repo based on meaningful edits (no prompts). You can turn this off anytime.",
      { modal: true },
      "Enable",
      "Cancel"
    );
    if (choice !== "Enable") {
      return false;
    }

    const folders = vscode.workspace.workspaceFolders || [];
    if (folders.length === 0) {
      await this.setAutoCaptureConsent(root);
      return true;
    }
    for (const folder of folders) {
      await this.setAutoCaptureConsent(folder.uri.fsPath);
    }
    return true;
  }

  private async maybeBackfillAutoCaptureConsent(): Promise<void> {
    if (!this.isAutoSessionsEnabled()) {
      return;
    }
    const folders = vscode.workspace.workspaceFolders || [];
    if (folders.length === 0) {
      const root = await this.resolveAnyWorkspaceRoot();
      if (root && !this.hasAutoCaptureConsent(root)) {
        await this.setAutoCaptureConsent(root);
      }
      return;
    }
    for (const folder of folders) {
      const root = this.normalizeRepoPath(folder.uri.fsPath);
      if (!this.hasAutoCaptureConsent(root)) {
        await this.setAutoCaptureConsent(root);
      }
    }
  }

  private async handleAutoCaptureToggle(): Promise<void> {
    if (this.autoCaptureToggleBusy) {
      return;
    }
    if (!this.isAutoSessionsEnabled()) {
      return;
    }
    const root = await this.resolveAnyWorkspaceRoot();
    if (!root) {
      return;
    }
    if (this.hasAutoCaptureConsent(root)) {
      return;
    }
    const ok = await this.ensureAutoCaptureConsent();
    if (ok) {
      return;
    }

    // User declined consent: revert the toggle.
    const cfg = vscode.workspace.getConfiguration("mempack");
    const inspect = cfg.inspect<boolean>("autoSessionsEnabled");
    const target =
      inspect?.workspaceValue !== undefined
        ? vscode.ConfigurationTarget.Workspace
        : inspect?.globalValue !== undefined
          ? vscode.ConfigurationTarget.Global
          : vscode.ConfigurationTarget.Workspace;
    try {
      this.autoCaptureToggleBusy = true;
      await cfg.update("autoSessionsEnabled", false, target);
    } finally {
      this.autoCaptureToggleBusy = false;
    }
  }

  private async openMemoryByID(repoRoot: string, id: string): Promise<void> {
    const result = await this.client.show(repoRoot, id);
    const content = formatShowResult(result);
    const doc = await vscode.workspace.openTextDocument({ content, language: "markdown" });
    await vscode.window.showTextDocument(doc, { preview: false });
  }

  private threadFromBranch(branch: string): string {
    if (!this.useThreadFromBranch()) {
      return this.client.defaultThread;
    }
    return branchToThread(branch, this.client.defaultThread);
  }

  private shouldMarkNeedsSummary(summary: string): boolean {
    const rule = this.getNeedsSummaryRule();
    if (rule === "always") {
      return true;
    }
    if (rule === "never") {
      return false;
    }
    return summary.trim() === "";
  }

  private async maybeToastNeedsSummary(
    repoRoot: string,
    sha: string,
    needsSummary: boolean
  ): Promise<void> {
    if (!needsSummary) {
      return;
    }
    if (this.getNudgeStyle() !== "badgeToast") {
      return;
    }
    const lastNudged = this.lastNudgedByRepo.get(repoRoot);
    if (lastNudged === sha) {
      return;
    }
    this.lastNudgedByRepo.set(repoRoot, sha);
    const choice = await vscode.window.showInformationMessage(
      "Session saved without a summary.",
      "Annotate now"
    );
    if (choice === "Annotate now") {
      await this.annotateLastSession();
    }
  }

  private async buildSessionSummary(
    repoRoot: string,
    session: SessionItem,
    userSummary: string
  ): Promise<string> {
    let summary = userSummary.trim();
    if (!this.getAttachFilesEnabled()) {
      return summary;
    }
    const sha = session.anchor_commit || "";
    if (sha.trim() === "") {
      return summary;
    }
    try {
      const files = await this.getChangedFiles(repoRoot, sha, 50);
      summary = appendFilesBlock(summary, files);
    } catch (err) {
      this.logError("failed to attach files list", err);
    }
    return summary;
  }

  private async getChangedFiles(repoRoot: string, sha: string, limit: number): Promise<string[]> {
    const result = await execFileAsync(
      "git",
      ["-C", repoRoot, "diff-tree", "--no-commit-id", "--name-only", "-r", sha],
      { timeout: 10_000, maxBuffer: 5 * 1024 * 1024 }
    );
    const lines = (result.stdout || "")
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter((line) => line !== "");
    if (lines.length <= limit) {
      return lines;
    }
    return lines.slice(0, limit);
  }

  private async handleTerminalExecutionStart(
    event: vscode.TerminalShellExecutionStartEvent
  ): Promise<void> {
    if (!this.isAutoSessionsEnabled()) {
      return;
    }
    const commandLine = event.execution.commandLine.value.trim();
    if (commandLine === "") {
      return;
    }
    const signal = extractTerminalIntentSignal(commandLine);
    if (!signal) {
      return;
    }
    const workspaceRoot = this.resolveWorkspaceRootForExecutionEvent(event);
    if (!workspaceRoot) {
      return;
    }
    const normalizedRoot = this.normalizeRepoPath(workspaceRoot);
    this.recentTerminalIntentByWorkspace.set(normalizedRoot, {
      ...signal,
      observedAtMs: Date.now()
    });
  }

  private resolveWorkspaceRootForExecutionEvent(
    event: vscode.TerminalShellExecutionStartEvent
  ): string | undefined {
    const executionCwd = event.execution.cwd;
    if (executionCwd && executionCwd.scheme === "file") {
      return getWorkspaceRoot(executionCwd) || executionCwd.fsPath;
    }

    const terminalOptions = event.terminal.creationOptions as
      | (vscode.TerminalOptions & { cwd?: string | vscode.Uri })
      | vscode.ExtensionTerminalOptions;
    const terminalCwd =
      "cwd" in terminalOptions ? (terminalOptions.cwd as string | vscode.Uri | undefined) : undefined;
    if (typeof terminalCwd === "string" && terminalCwd.trim() !== "") {
      const uri = vscode.Uri.file(terminalCwd);
      return getWorkspaceRoot(uri) || terminalCwd;
    }
    if (terminalCwd instanceof vscode.Uri && terminalCwd.scheme === "file") {
      return getWorkspaceRoot(terminalCwd) || terminalCwd.fsPath;
    }

    const activeUri = vscode.window.activeTextEditor?.document?.uri;
    return getWorkspaceRoot(activeUri);
  }

  private async handleDocumentSave(document: vscode.TextDocument): Promise<void> {
    if (!this.isAutoSessionsEnabled()) {
      return;
    }
    if (document.isUntitled || document.uri.scheme !== "file") {
      return;
    }
    const workspaceRoot = getWorkspaceRoot(document.uri);
    if (!workspaceRoot) {
      return;
    }
    if (!this.hasAutoCaptureConsent(workspaceRoot)) {
      return;
    }
    try {
      await this.autoCapture.recordSave({
        workspaceRoot: this.normalizeRepoPath(workspaceRoot),
        filePath: this.normalizeRepoPath(document.uri.fsPath),
        text: document.getText()
      });
    } catch (err) {
      this.logError("auto session save handling failed", err);
    }
  }

  private async handleFileCreateEvent(event: vscode.FileCreateEvent): Promise<void> {
    if (!this.isAutoSessionsEnabled()) {
      return;
    }
    for (const uri of event.files) {
      if (uri.scheme !== "file") {
        continue;
      }
      const workspaceRoot = getWorkspaceRoot(uri);
      if (!workspaceRoot) {
        continue;
      }
      if (!this.hasAutoCaptureConsent(workspaceRoot)) {
        continue;
      }
      try {
        const text = await this.readTextForAutoSession(uri);
        await this.autoCapture.recordCreate({
          workspaceRoot: this.normalizeRepoPath(workspaceRoot),
          filePath: this.normalizeRepoPath(uri.fsPath),
          text
        });
      } catch (err) {
        this.logError("auto session create event handling failed", err);
      }
    }
  }

  private async handleFileDeleteEvent(event: vscode.FileDeleteEvent): Promise<void> {
    if (!this.isAutoSessionsEnabled()) {
      return;
    }
    for (const uri of event.files) {
      if (uri.scheme !== "file") {
        continue;
      }
      const workspaceRoot = getWorkspaceRoot(uri);
      if (!workspaceRoot) {
        continue;
      }
      if (!this.hasAutoCaptureConsent(workspaceRoot)) {
        continue;
      }
      try {
        await this.autoCapture.recordDelete({
          workspaceRoot: this.normalizeRepoPath(workspaceRoot),
          filePath: this.normalizeRepoPath(uri.fsPath)
        });
      } catch (err) {
        this.logError("auto session delete event handling failed", err);
      }
    }
  }

  private async handleFileRenameEvent(event: vscode.FileRenameEvent): Promise<void> {
    if (!this.isAutoSessionsEnabled()) {
      return;
    }
    for (const entry of event.files) {
      const oldUri = entry.oldUri;
      const newUri = entry.newUri;
      if (oldUri.scheme !== "file" || newUri.scheme !== "file") {
        continue;
      }
      const workspaceRoot = getWorkspaceRoot(newUri) || getWorkspaceRoot(oldUri);
      if (!workspaceRoot) {
        continue;
      }
      if (!this.hasAutoCaptureConsent(workspaceRoot)) {
        continue;
      }
      try {
        const newText = await this.readTextForAutoSession(newUri);
        await this.autoCapture.recordRename({
          workspaceRoot: this.normalizeRepoPath(workspaceRoot),
          oldFilePath: this.normalizeRepoPath(oldUri.fsPath),
          newFilePath: this.normalizeRepoPath(newUri.fsPath),
          newText
        });
      } catch (err) {
        this.logError("auto session rename event handling failed", err);
      }
    }
  }

  private async readTextForAutoSession(uri: vscode.Uri): Promise<string | undefined> {
    try {
      const stat = await fs.promises.stat(uri.fsPath);
      if (!stat.isFile()) {
        return undefined;
      }
      return await fs.promises.readFile(uri.fsPath, "utf8");
    } catch {
      return undefined;
    }
  }

  private async resolveThreadForWorkspace(workspaceRoot: string): Promise<string> {
    if (!this.useThreadFromBranch()) {
      return this.client.defaultThread;
    }
    try {
      const branch = await this.gitBranch(workspaceRoot);
      if (branch.trim() !== "") {
        return branchToThread(branch, this.client.defaultThread);
      }
    } catch {
      // Ignore git lookup failures; default thread is the fallback path.
    }
    return this.client.defaultThread;
  }
}

function normalizeBranch(value: string): string {
  const trimmed = value.trim();
  if (trimmed === "" || trimmed === "HEAD") {
    return "";
  }
  return trimmed.replace(/^refs\/heads\//, "");
}

function branchToThread(branch: string, fallback: string): string {
  if (branch.trim() === "") {
    return fallback;
  }
  const cleaned = branch.replace(/^refs\/heads\//, "").trim();
  const sanitized = cleaned.replace(/[^a-zA-Z0-9]+/g, "-").replace(/^-+|-+$/g, "");
  if (sanitized === "") {
    return fallback;
  }
  return `T-${sanitized}`;
}

function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return date.toLocaleString();
}

function appendFilesBlock(summary: string, files: string[]): string {
  if (!files || files.length === 0) {
    return summary;
  }
  const header = "Files changed:";
  const body = files.map((f) => `- ${f}`).join("\n");
  const block = `${header}\n${body}`.trimEnd();
  const trimmed = summary.trimEnd();
  if (trimmed === "") {
    return block;
  }
  if (trimmed.includes(header)) {
    return trimmed;
  }
  return `${trimmed}\n\n${block}`;
}
