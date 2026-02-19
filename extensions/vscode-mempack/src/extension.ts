import * as vscode from "vscode";
import * as fs from "fs/promises";
import * as path from "path";
import { MempackClient } from "./client";
import { MempackTreeProvider, MemoryNode, SessionNode } from "./tree";
import { getWorkspaceRoot } from "./workspace";
import { addMempackStub } from "./stub";
import { showContextPanel } from "./context_view";
import { showThreadsPanel } from "./threads_view";
import { showRecentPanel } from "./recent_view";
import { formatContextPack, formatDoctorReport, formatShowResult, suggestSummary, suggestTitle } from "./format";
import { SessionManager } from "./sessions";
import {
  getConfigPath,
  getRepoConfigPath,
  parseEmbeddingConfig,
  parseMcpWritesConfig,
  parseRepoConfig,
  parseTokenBudgetConfig,
  resolveMcpWrites,
  setTomlBoolean,
  setTomlNumber,
  setTomlString,
  updateRepoConfig
} from "./config";

export function activate(context: vscode.ExtensionContext): void {
  const output = vscode.window.createOutputChannel("Mempack");
  const client = new MempackClient(output);
  const tree = new MempackTreeProvider(client, context);
  const sessions = new SessionManager(client, output, context.globalState);

  context.subscriptions.push(output);
  context.subscriptions.push({ dispose: () => client.dispose() });
  context.subscriptions.push(sessions);
  void sessions.start();
  void vscode.commands.executeCommand("setContext", "mempack.mcpRunning", false);
  context.subscriptions.push(vscode.window.registerTreeDataProvider("mempackView", tree));
  context.subscriptions.push(
    vscode.workspace.onDidChangeConfiguration((e) => {
      if (e.affectsConfiguration("mempack")) {
        tree.refresh();
        void sessions.refreshStatus();
      }
    })
  );

  const mcpWatchInterval = setInterval(() => {
    tree.refresh();
  }, 5000);
  context.subscriptions.push({ dispose: () => clearInterval(mcpWatchInterval) });

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.refresh", () => {
      tree.refresh();
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.annotateLastSession", async () => {
      await sessions.annotateLastSession();
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.annotateSessionFromList", async () => {
      await sessions.annotateSessionFromList();
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.openSessionDiff", async (node?: SessionNode) => {
      if (!node) {
        return;
      }
      await sessions.openSessionDiff(node.session);
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.annotateSession", async (node?: SessionNode) => {
      if (!node) {
        return;
      }
      await sessions.annotateSession(node.session);
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.markSessionReviewed", async (node?: SessionNode) => {
      if (!node) {
        return;
      }
      await sessions.markSessionReviewed(node.session);
      tree.refresh();
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.copySessionReference", async (node?: SessionNode) => {
      if (!node) {
        return;
      }
      await sessions.copySessionReference(node.session);
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.toggleIntentCapture", async () => {
      const config = vscode.workspace.getConfiguration("mempack");
      const current = config.get<boolean>("autoSessionsEnabled", false);
      const scope = await vscode.window.showQuickPick<
        { label: string; value: vscode.ConfigurationTarget; description?: string }
      >(
        [
          { label: "Workspace", value: vscode.ConfigurationTarget.Workspace, description: "Only this repo/workspace." },
          { label: "User", value: vscode.ConfigurationTarget.Global, description: "Default for all repos." }
        ],
        { placeHolder: "Where should this setting apply?" }
      );
      if (!scope) {
        return;
      }
      try {
        if (!current) {
          const ok = await sessions.ensureAutoCaptureConsent();
          if (!ok) {
            return;
          }
        }
        await config.update("autoSessionsEnabled", !current, scope.value);
        tree.refresh();
        await sessions.refreshStatus();
        vscode.window.showInformationMessage(
          current
            ? "Auto-capture is Off. Mempack will not write session memories automatically."
            : "Auto-capture is On. Mempack will write lightweight session memories from meaningful edits."
        );
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.configureIntentCapture", async () => {
      const config = vscode.workspace.getConfiguration("mempack");
      const scope = await vscode.window.showQuickPick<
        { label: string; value: vscode.ConfigurationTarget; description?: string }
      >(
        [
          { label: "Workspace", value: vscode.ConfigurationTarget.Workspace, description: "Only this repo/workspace." },
          { label: "User", value: vscode.ConfigurationTarget.Global, description: "Default for all repos." }
        ],
        { placeHolder: "Where should these auto-capture settings apply?" }
      );
      if (!scope) {
        return;
      }

      const enabled = config.get<boolean>("autoSessionsEnabled", false);
      const enabledPick = await vscode.window.showQuickPick(
        [
          {
            label: "Auto-capture: On",
            value: true,
            description: "Automatically saves lightweight session memories from meaningful edits (no prompts)."
          },
          {
            label: "Auto-capture: Off",
            value: false,
            description: "Disables passive session capture. Manual saves still work."
          }
        ],
        { placeHolder: `Auto-capture work sessions (currently ${enabled ? "On" : "Off"})` }
      );
      if (!enabledPick) {
        return;
      }

      if (enabledPick.value) {
        const ok = await sessions.ensureAutoCaptureConsent();
        if (!ok) {
          return;
        }
      }

      try {
        await config.update("autoSessionsEnabled", enabledPick.value, scope.value);
        tree.refresh();
        await sessions.refreshStatus();
        vscode.window.showInformationMessage(
          enabledPick.value
            ? "Auto-capture is On. Mempack will write session memories from meaningful edits."
            : "Auto-capture is Off. Mempack will not write session memories automatically."
        );
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.startMcpServer", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        const message = await client.mcpStart(cwd);
        tree.refresh();
        vscode.window.showInformationMessage(message || "MCP server started.");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.stopMcpServer", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        const message = await client.mcpStop(cwd);
        tree.refresh();
        vscode.window.showInformationMessage(message || "MCP server stopped.");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.restartMcpServer", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        await client.mcpStop(cwd).catch(() => "");
        const message = await client.mcpStart(cwd);
        tree.refresh();
        vscode.window.showInformationMessage(message || "MCP server restarted.");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.init", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      const choice = await vscode.window.showQuickPick<{
        label: string;
        value: "write" | "no-agents";
        description?: string;
      }>(
        [
          {
            label: "Init (recommended)",
            value: "write",
            description:
              "Runs `mem init`. Writes `.mempack/MEMORY.md` + `AGENTS.md` and only detected assistant files when relevant."
          },
          {
            label: "Init (no repo files)",
            value: "no-agents",
            description:
              "Runs `mem init --no-agents`. Initializes the repo DB and welcome memory, but writes no files into this repo."
          }
        ],
        { placeHolder: "Initialize Mempack in this repo" }
      );
      if (!choice) {
        return;
      }
      try {
        output.show(true);
        await vscode.window.withProgress(
          { location: vscode.ProgressLocation.Notification, title: "Initializing Mempack", cancellable: false },
          async () => {
            await client.init(cwd, choice.value === "no-agents");
          }
        );
        tree.refresh();

        // Resolve repo root for follow-up actions (init writes agent files to repo root, which may differ from cwd).
        let repoRoot = cwd;
        try {
          const report = await client.doctor(cwd);
          const root = report?.repo?.git_root ? String(report.repo.git_root) : "";
          if (root.trim() !== "") {
            repoRoot = root;
          }
        } catch {
          // ignore
        }

        const message =
          choice.value === "no-agents"
            ? "Mempack initialized. No repo files were written (as requested)."
            : "Mempack initialized. Agent instruction files were written.";

        const actions: Array<{ label: string; filePath?: string }> = [];
        if (choice.value !== "no-agents") {
          await addOpenActionIfExists(actions, "Open .mempack/MEMORY.md", path.join(repoRoot, ".mempack", "MEMORY.md"));
          await addOpenActionIfExists(actions, "Open AGENTS.md", path.join(repoRoot, "AGENTS.md"));
          await addOpenActionIfExists(actions, "Open CLAUDE.md", path.join(repoRoot, "CLAUDE.md"));
          await addOpenActionIfExists(actions, "Open GEMINI.md", path.join(repoRoot, "GEMINI.md"));
          await addOpenActionIfExists(actions, "Open .mempack/AGENTS.md", path.join(repoRoot, ".mempack", "AGENTS.md"));
        }
        actions.push({ label: "Open Output" });

        const pick = await vscode.window.showInformationMessage(message, ...actions.map((a) => a.label));
        if (pick) {
          if (pick === "Open Output") {
            output.show(true);
          } else {
            const action = actions.find((a) => a.label === pick);
            if (action?.filePath) {
              try {
                const uri = vscode.Uri.file(action.filePath);
                await vscode.workspace.fs.stat(uri);
                await vscode.window.showTextDocument(uri, { preview: false });
              } catch {
                // ignore missing file (e.g. AGENTS.md already existed so init wrote .mempack/AGENTS.md instead)
              }
            }
          }
        }

        await maybePromptMcpWrites(context, tree);
        await maybePromptStartMcpServer(context, client, tree);
        await maybePromptAssistantFiles(context, client, repoRoot);
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.configureAssistantFiles", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      const repoRoot = await resolveRepoRoot(client, cwd);
      const detected = detectAssistantUsage();
      const items: Array<{
        label: string;
        target: AssistantTarget;
        description?: string;
        picked?: boolean;
      }> = [
        {
          label: "AGENTS.md",
          target: "agents",
          description: (await fileExists(path.join(repoRoot, "AGENTS.md"))) ? "Exists" : "Missing"
        },
        {
          label: "CLAUDE.md",
          target: "claude",
          description: (await fileExists(path.join(repoRoot, "CLAUDE.md")))
            ? "Exists"
            : detected.claude
              ? "Missing (detected)"
              : "Missing",
          picked: detected.claude
        },
        {
          label: "GEMINI.md",
          target: "gemini",
          description: (await fileExists(path.join(repoRoot, "GEMINI.md")))
            ? "Exists"
            : detected.gemini
              ? "Missing (detected)"
              : "Missing",
          picked: detected.gemini
        }
      ];
      const picks = await vscode.window.showQuickPick(items, {
        canPickMany: true,
        placeHolder: "Select assistant stubs to create (existing files are left unchanged)."
      });
      if (!picks || picks.length === 0) {
        return;
      }
      try {
        await client.writeAssistantFiles(repoRoot, picks.map((item) => item.target), false);
        vscode.window.showInformationMessage(`Assistant stubs updated: ${picks.map((item) => item.label).join(", ")}`);
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.configureMcpWrites", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        const settings = await loadMcpWritesSettings(cwd);
        const currentLabel =
          settings.effective.mode === "off"
            ? "Off"
            : settings.effective.mode === "auto"
              ? "Auto"
              : "Ask";

        const modePick = await vscode.window.showQuickPick<
          { label: string; allow: boolean; mode: string; description?: string }
        >(
          [
            { label: "Off", allow: false, mode: "off", description: "Disable MCP write tools." },
            { label: "Ask", allow: true, mode: "ask", description: "Allow writes, but require confirmation." },
            { label: "Auto", allow: true, mode: "auto", description: "Allow writes without confirmation." }
          ],
          { placeHolder: `MCP Writes (currently ${currentLabel})` }
        );
        if (!modePick) {
          return;
        }

        const scopePick = await vscode.window.showQuickPick<
          { label: string; value: "repo" | "global"; description?: string }
        >(
          [
            { label: "Repo override", value: "repo", description: settings.repoConfigPath },
            { label: "Global default", value: "global", description: settings.globalConfigPath }
          ],
          { placeHolder: "Where should this write setting apply?" }
        );
        if (!scopePick) {
          return;
        }

        if (scopePick.value === "repo") {
          const nextContent = updateRepoConfig(settings.repoContent || "{}", {
            mcp_allow_write: modePick.allow,
            mcp_write_mode: modePick.mode
          });
          await fs.mkdir(path.dirname(settings.repoConfigPath), { recursive: true });
          await fs.writeFile(settings.repoConfigPath, nextContent, "utf8");
        } else {
          let nextGlobal = setTomlBoolean(settings.globalContent || "", "mcp_allow_write", modePick.allow);
          nextGlobal = setTomlString(nextGlobal, "mcp_write_mode", modePick.mode);
          await fs.mkdir(path.dirname(settings.globalConfigPath), { recursive: true });
          await fs.writeFile(settings.globalConfigPath, nextGlobal, "utf8");
        }

        tree.refresh();
        vscode.window.showInformationMessage(`MCP writes set to ${modePick.label}.`);
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.toggleWriteOptIn", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        const settings = await loadMcpWritesSettings(cwd);
        const enabled = settings.effective.mode !== "off";
        const actionLabel = enabled ? "Disable" : "Enable";
        const confirm = await vscode.window.showWarningMessage(
          `${actionLabel} MCP writes for this repo override?`,
          { modal: true },
          actionLabel
        );
        if (confirm !== actionLabel) {
          return;
        }

        const nextAllow = !enabled;
        const nextMode = nextAllow
          ? settings.effective.configuredMode === "auto"
            ? "auto"
            : "ask"
          : "off";
        const nextContent = updateRepoConfig(settings.repoContent || "{}", {
          mcp_allow_write: nextAllow,
          mcp_write_mode: nextMode
        });
        await fs.mkdir(path.dirname(settings.repoConfigPath), { recursive: true });
        await fs.writeFile(settings.repoConfigPath, nextContent, "utf8");
        tree.refresh();
        vscode.window.showInformationMessage(`MCP writes ${enabled ? "disabled" : "enabled"}.`);
      } catch (writeErr: any) {
        showError(writeErr);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.configureEmbeddings", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      const repoConfigPath = getRepoConfigPath(cwd);
      let repoContent = "";
      try {
        repoContent = await fs.readFile(repoConfigPath, "utf8");
      } catch (err: any) {
        if (err?.code !== "ENOENT") {
          showError(err);
          return;
        }
      }
      let repoCfg;
      try {
        repoCfg = parseRepoConfig(repoContent || "{}");
      } catch (err: any) {
        showError(err);
        return;
      }
      let globalCfg: { provider: string; model: string } | null = null;
      if (!repoCfg.embedding_provider || !repoCfg.embedding_model) {
        try {
          const globalContent = await fs.readFile(getConfigPath(), "utf8");
          globalCfg = parseEmbeddingConfig(globalContent);
        } catch {
          globalCfg = null;
        }
      }
      const currentProvider = repoCfg.embedding_provider || globalCfg?.provider || "auto";
      const currentModel = repoCfg.embedding_model || globalCfg?.model || "nomic-embed-text";
      const enabled = currentProvider.toLowerCase() !== "none";
      const choice = await vscode.window.showQuickPick<
        { label: string; value: string; description?: string }
      >(
        [
          {
            label: enabled ? "Disable embeddings" : "Enable embeddings",
            value: "toggle",
            description: enabled ? "Set embedding_provider = none" : "Set embedding_provider = auto"
          },
          {
            label: "Choose embedding model",
            value: "model",
            description: `Current: ${currentModel}`
          }
        ],
        { placeHolder: "Configure embeddings" }
      );
      if (!choice) {
        return;
      }

      let next = repoContent || "{}";
      if (choice.value === "toggle") {
        next = updateRepoConfig(next, {
          embedding_provider: enabled ? "none" : "auto",
          embedding_model: currentModel
        });
      }

      if (choice.value === "model") {
        const modelPick = await vscode.window.showQuickPick(
          [
            { label: "nomic-embed-text", description: "Default (balanced)" },
            { label: "mxbai-embed-large", description: "High quality" },
            { label: "bge-small-en", description: "Fast, smaller" },
            { label: "bge-base-en", description: "Balanced" },
            { label: "bge-large-en", description: "High quality" },
            { label: "Custom...", description: "Enter a model name" }
          ],
          { placeHolder: "Select embedding model" }
        );
        if (!modelPick) {
          return;
        }
        let model = modelPick.label;
        if (modelPick.label === "Custom...") {
          const custom = await vscode.window.showInputBox({
            prompt: "Embedding model",
            value: currentModel,
            ignoreFocusOut: true
          });
          if (!custom || custom.trim() === "") {
            return;
          }
          model = custom.trim();
        }
        next = updateRepoConfig(next, {
          embedding_model: model,
          embedding_provider: "auto"
        });
      }

      try {
        await fs.mkdir(path.dirname(repoConfigPath), { recursive: true });
        await fs.writeFile(repoConfigPath, next, "utf8");
        tree.refresh();
        vscode.window.showInformationMessage("Embedding settings updated.");
        await maybePromptOllamaInstall(context, client);
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.configureTokenBudget", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      const repoConfigPath = getRepoConfigPath(cwd);
      let repoContent = "";
      try {
        repoContent = await fs.readFile(repoConfigPath, "utf8");
      } catch (err: any) {
        if (err?.code !== "ENOENT") {
          showError(err);
          return;
        }
      }

      let repoCfg;
      try {
        repoCfg = parseRepoConfig(repoContent || "{}");
      } catch (err: any) {
        showError(err);
        return;
      }

      let globalBudget: number | undefined;
      if (repoCfg.token_budget === undefined) {
        try {
          const globalContent = await fs.readFile(getConfigPath(), "utf8");
          globalBudget = parseTokenBudgetConfig(globalContent);
        } catch {
          globalBudget = undefined;
        }
      }

      const currentBudget = repoCfg.token_budget || globalBudget || 2500;
      const scopeChoice = await vscode.window.showQuickPick<
        { label: string; value: "repo" | "global"; description?: string }
      >(
        [
          { label: "Repo override", value: "repo", description: repoConfigPath },
          { label: "Global default", value: "global", description: getConfigPath() }
        ],
        { placeHolder: "Where should this token budget apply?" }
      );
      if (!scopeChoice) {
        return;
      }

      const input = await vscode.window.showInputBox({
        prompt: "Token budget (total tokens for context output)",
        value: String(currentBudget),
        ignoreFocusOut: true
      });
      if (!input || input.trim() === "") {
        return;
      }
      const nextBudget = Number(input.trim());
      if (!Number.isFinite(nextBudget) || nextBudget <= 0) {
        vscode.window.showErrorMessage("Token budget must be a positive number.");
        return;
      }

      if (scopeChoice.value === "repo") {
        const next = updateRepoConfig(repoContent || "{}", {
          token_budget: Math.floor(nextBudget)
        });
        try {
          await fs.mkdir(path.dirname(repoConfigPath), { recursive: true });
          await fs.writeFile(repoConfigPath, next, "utf8");
          tree.refresh();
          vscode.window.showInformationMessage("Token budget updated for this repo.");
        } catch (err: any) {
          showError(err);
        }
        return;
      }

      const configPath = getConfigPath();
      let globalContent = "";
      try {
        globalContent = await fs.readFile(configPath, "utf8");
      } catch (err: any) {
        if (err?.code !== "ENOENT") {
          showError(err);
          return;
        }
      }
      const nextGlobal = setTomlNumber(globalContent || "", "token_budget", Math.floor(nextBudget));
      try {
        await fs.mkdir(path.dirname(configPath), { recursive: true });
        await fs.writeFile(configPath, nextGlobal, "utf8");
        tree.refresh();
        vscode.window.showInformationMessage("Token budget updated globally.");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.configureWorkspace", async () => {
      const config = vscode.workspace.getConfiguration("mempack");
      const scope = await vscode.window.showQuickPick<
        { label: string; value: vscode.ConfigurationTarget }
      >(
        [
          { label: "Workspace", value: vscode.ConfigurationTarget.Workspace },
          { label: "User", value: vscode.ConfigurationTarget.Global }
        ],
        { placeHolder: "Where should this workspace setting apply?" }
      );
      if (!scope) {
        return;
      }
      const current = config.get<string>("workspace") || "";
      const next = await vscode.window.showInputBox({
        prompt: "Mempack workspace name (blank = default)",
        value: current,
        ignoreFocusOut: true
      });
      if (next === undefined) {
        return;
      }
      try {
        await config.update("workspace", next.trim(), scope.value);
        tree.refresh();
        vscode.window.showInformationMessage("Workspace setting updated.");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.configureDefaultThread", async () => {
      const config = vscode.workspace.getConfiguration("mempack");
      const scope = await vscode.window.showQuickPick<
        { label: string; value: vscode.ConfigurationTarget }
      >(
        [
          { label: "Workspace", value: vscode.ConfigurationTarget.Workspace },
          { label: "User", value: vscode.ConfigurationTarget.Global }
        ],
        { placeHolder: "Where should this default thread setting apply?" }
      );
      if (!scope) {
        return;
      }
      const current = config.get<string>("defaultThread") || "T-SESSION";
      const next = await vscode.window.showInputBox({
        prompt: "Default thread ID",
        value: current,
        ignoreFocusOut: true
      });
      if (!next || next.trim() === "") {
        return;
      }
      try {
        await config.update("defaultThread", next.trim(), scope.value);
        tree.refresh();
        vscode.window.showInformationMessage("Default thread updated.");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  void (async () => {
    await maybePromptOllamaInstall(context, client);
    await maybePromptMcpWrites(context, tree);
    await maybePromptStartMcpServer(context, client, tree);
    const cwd = getWorkspaceRoot(vscode.window.activeTextEditor?.document?.uri);
    if (cwd) {
      await maybePromptAssistantFiles(context, client, await resolveRepoRoot(client, cwd));
    }
  })();

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.addStub", async () => {
      try {
        await addMempackStub();
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.doctor", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        const report = await client.doctor(cwd);
        const content = formatDoctorReport(report);
        await openTextDocument("Mempack Doctor", content, "markdown");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.getContext", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      const query = await vscode.window.showInputBox({
        prompt: "Enter a Mempack query",
        placeHolder: "e.g. auth middleware",
        ignoreFocusOut: true
      });
      if (!query || query.trim() === "") {
        return;
      }
      try {
        const { pack, prompt } = await client.getContextPack(cwd, query.trim());
        showContextPanel(context, pack, prompt);
        await vscode.env.clipboard.writeText(prompt);
        vscode.window.showInformationMessage("Mempack context copied to clipboard.");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.explain", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      const query = await vscode.window.showInputBox({
        prompt: "Enter a query to explain",
        placeHolder: "e.g. auth middleware",
        ignoreFocusOut: true
      });
      if (!query || query.trim() === "") {
        return;
      }
      try {
        const report = await client.explainReport(cwd, query.trim());
        const text = JSON.stringify(report, null, 2);
        await openTextDocument("Mempack Explain", text, "json");
        await vscode.env.clipboard.writeText(text);
        vscode.window.showInformationMessage("Mempack explain copied to clipboard.");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.searchMemories", async () => {
      await vscode.commands.executeCommand("mempack.getContext");
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.openThreadsUI", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        await showThreadsPanel(context, client, cwd);
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.openRecentUI", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        await showRecentPanel(context, client, cwd);
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.saveSelection", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      const editor = vscode.window.activeTextEditor;
      if (!editor) {
        vscode.window.showErrorMessage("No active editor.");
        return;
      }
      const selection = editor.selection;
      const selectedText = editor.document.getText(selection).trim();
      if (selectedText === "") {
        vscode.window.showErrorMessage("Select some text to save as memory.");
        return;
      }

      const lastThread = context.workspaceState.get<string>("mempack.lastThread");
      const thread = await vscode.window.showInputBox({
        prompt: "Thread ID",
        value: lastThread || client.defaultThread,
        ignoreFocusOut: true
      });
      if (!thread || thread.trim() === "") {
        return;
      }

      const title = await vscode.window.showInputBox({
        prompt: "Title",
        value: suggestTitle(selectedText),
        ignoreFocusOut: true
      });
      if (!title || title.trim() === "") {
        return;
      }

      const summary = await vscode.window.showInputBox({
        prompt: "Summary",
        value: suggestSummary(selectedText),
        ignoreFocusOut: true
      });
      if (!summary || summary.trim() === "") {
        return;
      }

      const tags = await vscode.window.showInputBox({
        prompt: "Tags (comma separated, optional)",
        ignoreFocusOut: true
      });

      try {
        const combinedSummary = buildSelectionSummary(summary, selectedText);
        await client.addMemory(cwd, thread.trim(), title.trim(), combinedSummary, tags || "");
        await context.workspaceState.update("mempack.lastThread", thread.trim());
        tree.refresh();
        vscode.window.showInformationMessage(`Saved to Mempack: ${thread.trim()}`);
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.checkpoint", async () => {
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      const reason = await vscode.window.showInputBox({
        prompt: "Checkpoint reason",
        ignoreFocusOut: true
      });
      if (!reason || reason.trim() === "") {
        return;
      }

      const stateJsonInput = await vscode.window.showInputBox({
        prompt: "State JSON (optional)",
        value: "{}",
        ignoreFocusOut: true
      });
      const stateJson = stateJsonInput && stateJsonInput.trim() !== "" ? stateJsonInput.trim() : "{}";
      if (!isValidJson(stateJson)) {
        vscode.window.showErrorMessage("State JSON must be valid JSON.");
        return;
      }

      const thread = await vscode.window.showInputBox({
        prompt: "Thread ID (optional)",
        value: context.workspaceState.get<string>("mempack.lastThread") || "",
        ignoreFocusOut: true
      });

      try {
        await client.checkpoint(cwd, reason.trim(), stateJson, thread || "");
        if (thread && thread.trim() !== "") {
          await context.workspaceState.update("mempack.lastThread", thread.trim());
        }
        tree.refresh();
        vscode.window.showInformationMessage("Checkpoint saved.");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.openMemory", async (node?: MemoryNode) => {
      if (!node) {
        return;
      }
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        const result = await client.show(cwd, node.memory.id);
        const content = formatShowResult(result);
        await openTextDocument("Mempack Memory", content, "markdown");
      } catch (err: any) {
        showError(err);
      }
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.copySummary", async (node?: MemoryNode) => {
      if (!node) {
        return;
      }
      const summary = node.memory.summary || "";
      await vscode.env.clipboard.writeText(summary);
      vscode.window.showInformationMessage("Summary copied to clipboard.");
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.copyPromptSnippet", async (node?: MemoryNode) => {
      if (!node) {
        return;
      }
      const title = node.memory.title || "";
      const summary = node.memory.summary || "";
      const snippet = `- **${title}**: ${summary}`.trim();
      await vscode.env.clipboard.writeText(snippet);
      vscode.window.showInformationMessage("Prompt snippet copied to clipboard.");
    })
  );

  context.subscriptions.push(
    vscode.commands.registerCommand("mempack.deleteMemory", async (node?: MemoryNode) => {
      if (!node) {
        return;
      }
      const confirm = await vscode.window.showWarningMessage(
        `Delete memory "${node.memory.title}"?`,
        { modal: true },
        "Delete"
      );
      if (confirm !== "Delete") {
        return;
      }
      const cwd = requireWorkspace();
      if (!cwd) {
        return;
      }
      try {
        await client.forget(cwd, node.memory.id);
        tree.refresh();
        vscode.window.showInformationMessage("Memory deleted.");
      } catch (err: any) {
        showError(err);
      }
    })
  );
}

export function deactivate(): void {}

type AssistantTarget = "agents" | "claude" | "gemini";

type AssistantUsage = {
  claude: boolean;
  gemini: boolean;
};

function requireWorkspace(): string | undefined {
  const active = vscode.window.activeTextEditor?.document?.uri;
  const cwd = getWorkspaceRoot(active);
  if (!cwd) {
    vscode.window.showErrorMessage("Open a folder to use Mempack.");
    return undefined;
  }
  return cwd;
}

async function resolveRepoRoot(client: MempackClient, cwd: string): Promise<string> {
  try {
    const report = await client.doctor(cwd);
    const root = report?.repo?.git_root ? String(report.repo.git_root).trim() : "";
    if (root !== "") {
      return root;
    }
  } catch {
    // Ignore and fall back to workspace path.
  }
  return cwd;
}

function detectAssistantUsage(): AssistantUsage {
  const appName = (vscode.env.appName || "").toLowerCase();
  let claude = appName.includes("claude");
  let gemini = appName.includes("gemini");

  for (const ext of vscode.extensions.all) {
    const id = (ext.id || "").toLowerCase();
    if (id === "anthropic.claude-code" || id.includes("claude-code")) {
      claude = true;
    }
    if (id.includes("gemini") || id.includes("geminicodeassist")) {
      gemini = true;
    }
  }

  return { claude, gemini };
}

async function addOpenActionIfExists(
  actions: Array<{ label: string; filePath?: string }>,
  label: string,
  filePath: string
): Promise<void> {
  if (await fileExists(filePath)) {
    actions.push({ label, filePath });
  }
}

async function fileExists(filePath: string): Promise<boolean> {
  try {
    await fs.stat(filePath);
    return true;
  } catch (err: any) {
    if (err?.code === "ENOENT") {
      return false;
    }
    throw err;
  }
}

async function openTextDocument(title: string, content: string, language: string): Promise<void> {
  const doc = await vscode.workspace.openTextDocument({ content, language });
  await vscode.window.showTextDocument(doc, { preview: false });
  vscode.window.setStatusBarMessage(title, 1500);
}

function showError(err: any): void {
  const message = err instanceof Error ? err.message : String(err);
  vscode.window.showErrorMessage(message);
}

function buildSelectionSummary(summary: string, selection: string): string {
  const summaryText = summary.trim();
  const selectionText = selection.trim();
  if (selectionText === "") {
    return summaryText;
  }
  if (summaryText === "") {
    return selectionText;
  }
  if (summaryText.includes(selectionText)) {
    return summaryText;
  }
  return `${summaryText}\n\nSelection:\n${selectionText}`;
}

function isValidJson(value: string): boolean {
  try {
    JSON.parse(value);
    return true;
  } catch {
    return false;
  }
}


async function maybePromptOllamaInstall(
  context: vscode.ExtensionContext,
  client: MempackClient
): Promise<void> {
  const active = vscode.window.activeTextEditor?.document?.uri;
  const cwd = getWorkspaceRoot(active);
  if (!cwd) {
    return;
  }

  const repoConfigPath = getRepoConfigPath(cwd);
  const suppressKey = `mempack.ollamaPromptSuppressed:${repoConfigPath}`;
  if (context.globalState.get<boolean>(suppressKey)) {
    return;
  }

  const lastPromptKey = `mempack.ollamaPromptedAt:${repoConfigPath}`;
  const lastPrompt = context.globalState.get<number>(lastPromptKey) || 0;
  if (Date.now() - lastPrompt < 24 * 60 * 60 * 1000) {
    return;
  }

  try {
    const status = await client.embedStatus(cwd);
    const reason = status.vectors?.reason || "";
    const provider = status.vectors?.provider_configured || status.provider || "";
    if (provider.toLowerCase() === "none") {
      return;
    }
    if (reason !== "ollama_not_reachable") {
      return;
    }
  } catch {
    return;
  }

  const choice = await vscode.window.showInformationMessage(
    "Embeddings are recommended, but Ollama isn't available. Install Ollama?",
    "Install Ollama",
    "Disable embeddings",
    "Later",
    "Don't ask again"
  );

  if (!choice || choice === "Later") {
    await context.globalState.update(lastPromptKey, Date.now());
    return;
  }

  if (choice === "Don't ask again") {
    await context.globalState.update(suppressKey, true);
    return;
  }

  if (choice === "Install Ollama") {
    await context.globalState.update(lastPromptKey, Date.now());
    await vscode.env.openExternal(vscode.Uri.parse("https://ollama.com/download"));
    return;
  }

  if (choice === "Disable embeddings") {
    try {
      const content = await fs.readFile(repoConfigPath, "utf8").catch(() => "");
      const next = updateRepoConfig(content || "{}", { embedding_provider: "none" });
      await fs.mkdir(path.dirname(repoConfigPath), { recursive: true });
      await fs.writeFile(repoConfigPath, next, "utf8");
      await context.globalState.update(lastPromptKey, Date.now());
      vscode.window.showInformationMessage("Embeddings disabled.");
    } catch (err: any) {
      showError(err);
    }
  }
}

type McpWriteSettings = {
  repoConfigPath: string;
  globalConfigPath: string;
  repoContent: string;
  globalContent: string;
  repoCfg: ReturnType<typeof parseRepoConfig>;
  effective: ReturnType<typeof resolveMcpWrites>;
};

async function loadMcpWritesSettings(cwd: string): Promise<McpWriteSettings> {
  const repoConfigPath = getRepoConfigPath(cwd);
  const globalConfigPath = getConfigPath();

  let repoContent = "";
  try {
    repoContent = await fs.readFile(repoConfigPath, "utf8");
  } catch (err: any) {
    if (err?.code !== "ENOENT") {
      throw err;
    }
  }
  const repoCfg = parseRepoConfig(repoContent || "{}");

  let globalContent = "";
  let globalCfg = {};
  try {
    globalContent = await fs.readFile(globalConfigPath, "utf8");
    globalCfg = parseMcpWritesConfig(globalContent);
  } catch (err: any) {
    if (err?.code !== "ENOENT") {
      throw err;
    }
  }

  return {
    repoConfigPath,
    globalConfigPath,
    repoContent,
    globalContent,
    repoCfg,
    effective: resolveMcpWrites(globalCfg, repoCfg)
  };
}

async function maybePromptMcpWrites(
  context: vscode.ExtensionContext,
  tree: MempackTreeProvider
): Promise<void> {
  const active = vscode.window.activeTextEditor?.document?.uri;
  const cwd = getWorkspaceRoot(active);
  if (!cwd) {
    return;
  }

  const repoConfigPath = getRepoConfigPath(cwd);
  const promptKey = `mempack.mcpWritesPrompted:${repoConfigPath}`;
  if (context.globalState.get<boolean>(promptKey)) {
    return;
  }

  try {
    const mempackDir = path.join(cwd, ".mempack");
    await fs.stat(mempackDir);
  } catch (err: any) {
    if (err?.code === "ENOENT") {
      return;
    }
    showError(err);
    return;
  }

  let settings: McpWriteSettings;
  try {
    settings = await loadMcpWritesSettings(cwd);
  } catch (err: any) {
    showError(err);
    return;
  }

  const sourceLabel =
    settings.effective.source === "repo"
      ? "repo override"
      : settings.effective.source === "global"
        ? "global config"
        : "built-in default";

  const message = settings.effective.mode === "off"
    ? `MCP writes are currently Off (${sourceLabel}). You can change this via "Mempack: Configure MCP Writes".`
    : `MCP writes are currently ${settings.effective.mode === "auto" ? "Auto" : "Ask"} (${sourceLabel}). You can change this anytime via "Mempack: Configure MCP Writes".`;

  const pick = await vscode.window.showInformationMessage(message, "Configure", "OK");
  if (pick === "Configure") {
    await vscode.commands.executeCommand("mempack.configureMcpWrites");
  }

  tree.refresh();
  await context.globalState.update(promptKey, true);
}

async function maybePromptAssistantFiles(
  context: vscode.ExtensionContext,
  client: MempackClient,
  repoRoot: string
): Promise<void> {
  if (repoRoot.trim() === "") {
    return;
  }
  try {
    await fs.stat(path.join(repoRoot, ".mempack"));
  } catch (err: any) {
    if (err?.code === "ENOENT") {
      return;
    }
    showError(err);
    return;
  }

  const usage = detectAssistantUsage();
  const missingTargets: AssistantTarget[] = [];
  if (usage.claude && !(await fileExists(path.join(repoRoot, "CLAUDE.md")))) {
    missingTargets.push("claude");
  }
  if (usage.gemini && !(await fileExists(path.join(repoRoot, "GEMINI.md")))) {
    missingTargets.push("gemini");
  }
  if (missingTargets.length === 0) {
    return;
  }

  const repoConfigPath = getRepoConfigPath(repoRoot);
  const suppressKey = `mempack.assistantFilesPromptSuppressed:${repoConfigPath}`;
  if (context.globalState.get<boolean>(suppressKey)) {
    return;
  }

  const promptKey = `mempack.assistantFilesPromptedAt:${repoConfigPath}`;
  const lastPrompt = context.globalState.get<number>(promptKey) || 0;
  if (Date.now() - lastPrompt < 24 * 60 * 60 * 1000) {
    return;
  }

  const fileList = missingTargets.map((target) => `${target.toUpperCase()}.md`).join(", ");
  const pick = await vscode.window.showInformationMessage(
    `Detected assistant tooling for this repo. Create ${fileList} with Mempack policy?`,
    "Create",
    "Later",
    "Don't ask again"
  );

  if (pick === "Don't ask again") {
    await context.globalState.update(suppressKey, true);
    await context.globalState.update(promptKey, Date.now());
    return;
  }
  if (!pick || pick === "Later") {
    await context.globalState.update(promptKey, Date.now());
    return;
  }

  if (pick === "Create") {
    try {
      await client.writeAssistantFiles(repoRoot, missingTargets, false);
      vscode.window.showInformationMessage(`Created ${fileList}.`);
      await context.globalState.update(promptKey, Date.now());
    } catch (err: any) {
      showError(err);
    }
  }
}

async function maybePromptStartMcpServer(
  context: vscode.ExtensionContext,
  client: MempackClient,
  tree: MempackTreeProvider
): Promise<void> {
  const enabled = vscode.workspace
    .getConfiguration("mempack")
    .get<boolean>("promptStartMcpServer", true);
  if (!enabled) {
    return;
  }

  const active = vscode.window.activeTextEditor?.document?.uri;
  const cwd = getWorkspaceRoot(active);
  if (!cwd) {
    return;
  }

  const repoConfigPath = getRepoConfigPath(cwd);
  const suppressKey = `mempack.mcpStartPromptSuppressed:${repoConfigPath}`;
  if (context.globalState.get<boolean>(suppressKey)) {
    return;
  }

  const promptKey = `mempack.mcpStartPrompted:${repoConfigPath}`;
  if (context.globalState.get<boolean>(promptKey)) {
    return;
  }

  try {
    const mempackDir = path.join(cwd, ".mempack");
    await fs.stat(mempackDir);
  } catch (err: any) {
    if (err?.code === "ENOENT") {
      return;
    }
    showError(err);
    return;
  }

  try {
    const status = await client.mcpStatus(cwd);
    if (status.running) {
      await context.globalState.update(promptKey, true);
      return;
    }
  } catch {
    // Fall through and offer start prompt.
  }

  const pick = await vscode.window.showInformationMessage(
    "MCP server is not running. Start it now? (recommended for MCP-first reads/writes)",
    "Start MCP",
    "Later",
    "Don't ask again"
  );

  if (pick === "Don't ask again") {
    await context.globalState.update(suppressKey, true);
    await context.globalState.update(promptKey, true);
    return;
  }
  if (pick === "Later" || !pick) {
    await context.globalState.update(promptKey, true);
    return;
  }
  if (pick === "Start MCP") {
    try {
      const message = await client.mcpStart(cwd);
      tree.refresh();
      vscode.window.showInformationMessage(message || "MCP server started.");
      await context.globalState.update(promptKey, true);
    } catch (err: any) {
      showError(err);
    }
  }
}
