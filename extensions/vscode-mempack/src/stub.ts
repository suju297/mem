import * as vscode from "vscode";
import * as path from "path";
import { getWorkspaceRoot } from "./workspace";
import { appendStub, buildAgentsWithStub, hasStub } from "./stub_logic";

export async function addMempackStub(): Promise<void> {
  const active = vscode.window.activeTextEditor?.document?.uri;
  const root = getWorkspaceRoot(active);
  if (!root) {
    vscode.window.showErrorMessage("Open a folder to use Mem.");
    return;
  }

  const agentsPath = path.join(root, "AGENTS.md");
  let exists = false;
  try {
    await vscode.workspace.fs.stat(vscode.Uri.file(agentsPath));
    exists = true;
  } catch {
    exists = false;
  }

  if (!exists) {
    await writeFile(agentsPath, buildAgentsWithStub());
    vscode.window.showInformationMessage("Created AGENTS.md with Mem stub.");
    return;
  }

  const existing = await readFile(agentsPath);
  if (hasStub(existing)) {
    vscode.window.showInformationMessage("Mem stub already present in AGENTS.md.");
    return;
  }

  const result = appendStub(existing);
  await writeFile(agentsPath, result.updated);
  vscode.window.showInformationMessage("Appended Mem stub to AGENTS.md.");
}

async function readFile(filePath: string): Promise<string> {
  const uri = vscode.Uri.file(filePath);
  const data = await vscode.workspace.fs.readFile(uri);
  return Buffer.from(data).toString("utf8");
}

async function writeFile(filePath: string, content: string): Promise<void> {
  const uri = vscode.Uri.file(filePath);
  await vscode.workspace.fs.writeFile(uri, Buffer.from(content, "utf8"));
}
