package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitDoesNotOverwriteAgents(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	sentinel := "KEEP THIS"
	writeFile(t, repoDir, "AGENTS.md", sentinel)

	_ = runCLI(t, "init")

	data, err := os.ReadFile(filepath.Join(repoDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if strings.TrimSpace(string(data)) != sentinel {
		t.Fatalf("AGENTS.md was modified unexpectedly")
	}

	if _, err := os.Stat(filepath.Join(repoDir, ".mempack", "AGENTS.md")); err != nil {
		t.Fatalf("expected .mempack/AGENTS.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".mempack", "MEMORY.md")); err != nil {
		t.Fatalf("expected .mempack/MEMORY.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("expected CLAUDE.md to be absent by default")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "GEMINI.md")); !os.IsNotExist(err) {
		t.Fatalf("expected GEMINI.md to be absent by default")
	}
}

func TestInitWithAssistantsAllCreatesCompatibilityFiles(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "init", "--assistants", "all")

	if _, err := os.Stat(filepath.Join(repoDir, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "GEMINI.md")); err != nil {
		t.Fatalf("expected GEMINI.md to exist: %v", err)
	}
}

func TestInitDoesNotOverwriteClaudeAndGeminiWhenSelected(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	claudeSentinel := "KEEP CLAUDE"
	geminiSentinel := "KEEP GEMINI"
	writeFile(t, repoDir, "CLAUDE.md", claudeSentinel)
	writeFile(t, repoDir, "GEMINI.md", geminiSentinel)

	_ = runCLI(t, "init", "--assistants", "claude,gemini")

	claudeData, err := os.ReadFile(filepath.Join(repoDir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if strings.TrimSpace(string(claudeData)) != claudeSentinel {
		t.Fatalf("CLAUDE.md was modified unexpectedly")
	}

	geminiData, err := os.ReadFile(filepath.Join(repoDir, "GEMINI.md"))
	if err != nil {
		t.Fatalf("read GEMINI.md: %v", err)
	}
	if strings.TrimSpace(string(geminiData)) != geminiSentinel {
		t.Fatalf("GEMINI.md was modified unexpectedly")
	}
}

func TestTemplateAgentsNoMemoryWritesSelectedTargetsOnly(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "template", "agents", "--write", "--assistants", "gemini", "--no-memory")

	if _, err := os.Stat(filepath.Join(repoDir, "GEMINI.md")); err != nil {
		t.Fatalf("expected GEMINI.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected AGENTS.md to be absent")
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".mempack", "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("expected .mempack/MEMORY.md to be absent")
	}
}
