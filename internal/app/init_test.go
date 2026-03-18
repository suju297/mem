package app

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mem/internal/config"
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

	if _, err := os.Stat(filepath.Join(repoDir, ".mem", "AGENTS.md")); err != nil {
		t.Fatalf("expected .mem/AGENTS.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".mem", "MEMORY.md")); err != nil {
		t.Fatalf("expected .mem/MEMORY.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("expected CLAUDE.md to be absent by default")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "GEMINI.md")); !os.IsNotExist(err) {
		t.Fatalf("expected GEMINI.md to be absent by default")
	}
}

func TestInitWithAllCreatesCompatibilityFiles(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "init", "--all")

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

func TestInitWithClaudeOnlyCreatesClaudeStub(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "init", "--claude")

	if _, err := os.Stat(filepath.Join(repoDir, "CLAUDE.md")); err != nil {
		t.Fatalf("expected CLAUDE.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected AGENTS.md to be absent when only --claude is selected")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "GEMINI.md")); !os.IsNotExist(err) {
		t.Fatalf("expected GEMINI.md to be absent when only --claude is selected")
	}
}

func TestInitWithAgentsOnlyCreatesAgentsStub(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "init", "--agents")

	if _, err := os.Stat(filepath.Join(repoDir, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatalf("expected CLAUDE.md to be absent when only --agents is selected")
	}
	if _, err := os.Stat(filepath.Join(repoDir, "GEMINI.md")); !os.IsNotExist(err) {
		t.Fatalf("expected GEMINI.md to be absent when only --agents is selected")
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

	_ = runCLI(t, "init", "--claude", "--gemini")

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

	if _, err := os.Stat(filepath.Join(repoDir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected AGENTS.md to be absent when only --claude and --gemini are selected")
	}
}

func TestInitUsesLegacyMempackDirWhenPresent(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	if err := os.MkdirAll(filepath.Join(repoDir, ".mempack"), 0o755); err != nil {
		t.Fatalf("mkdir .mempack: %v", err)
	}

	_ = runCLI(t, "init")

	if _, err := os.Stat(filepath.Join(repoDir, ".mempack", "MEMORY.md")); err != nil {
		t.Fatalf("expected .mempack/MEMORY.md to exist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".mempack", "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected .mempack/AGENTS.md to be absent when AGENTS.md is written at repo root")
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".mem", "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("expected .mem/MEMORY.md to be absent for legacy repo")
	}

	agentsData, err := os.ReadFile(filepath.Join(repoDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agentsData), ".mempack/MEMORY.md") {
		t.Fatalf("expected AGENTS.md to reference .mempack/MEMORY.md")
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
	if _, err := os.Stat(filepath.Join(repoDir, ".mem", "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("expected .mem/MEMORY.md to be absent")
	}
}

func TestInitFirstRunEmbeddingSetupCanDisableEmbeddings(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	origInteractive := initEmbeddingPromptInteractive
	initEmbeddingPromptInteractive = func() bool { return true }
	t.Cleanup(func() {
		initEmbeddingPromptInteractive = origInteractive
	})

	_ = runCLIWithInput(t, "n\n", "init", "--no-agents")

	configPath := filepath.Join(base, "config", "mem", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `embedding_provider = "none"`) {
		t.Fatalf("expected embedding_provider none in config, got:\n%s", text)
	}
	if !strings.Contains(text, `embedding_setup_complete = true`) {
		t.Fatalf("expected embedding_setup_complete true in config, got:\n%s", text)
	}
}

func TestInitFirstRunEmbeddingSetupEnablesOllamaModel(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	origInteractive := initEmbeddingPromptInteractive
	origLookPath := initEmbeddingLookPath
	origCheck := initEmbeddingCheckOllamaAvailable
	origPull := initEmbeddingPullModel
	initEmbeddingPromptInteractive = func() bool { return true }
	initEmbeddingLookPath = func(file string) (string, error) { return "/usr/bin/ollama", nil }
	initEmbeddingCheckOllamaAvailable = func(model string) (bool, string) { return true, "" }
	pulledModel := ""
	initEmbeddingPullModel = func(model string, out io.Writer) error {
		pulledModel = model
		return nil
	}
	t.Cleanup(func() {
		initEmbeddingPromptInteractive = origInteractive
		initEmbeddingLookPath = origLookPath
		initEmbeddingCheckOllamaAvailable = origCheck
		initEmbeddingPullModel = origPull
	})

	_ = runCLIWithInput(t, "y\n1\ny\n", "init", "--no-agents")

	configPath := filepath.Join(base, "config", "mem", "config.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config.toml: %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `embedding_provider = "ollama"`) {
		t.Fatalf("expected embedding_provider ollama in config, got:\n%s", text)
	}
	if !strings.Contains(text, `embedding_model = "nomic-embed-text"`) {
		t.Fatalf("expected embedding_model nomic-embed-text in config, got:\n%s", text)
	}
	if !strings.Contains(text, `embedding_setup_complete = true`) {
		t.Fatalf("expected embedding_setup_complete true in config, got:\n%s", text)
	}
	if pulledModel != "nomic-embed-text" {
		t.Fatalf("expected pulled model nomic-embed-text, got %q", pulledModel)
	}
}

func TestShouldPromptForEmbeddingSetup(t *testing.T) {
	tests := []struct {
		name         string
		cfg          config.Config
		configExists bool
		want         bool
	}{
		{
			name: "new install default prompts",
			cfg: config.Config{
				EmbeddingProvider:      "none",
				EmbeddingSetupComplete: false,
			},
			configExists: false,
			want:         true,
		},
		{
			name: "completed setup skips",
			cfg: config.Config{
				EmbeddingProvider:      "none",
				EmbeddingSetupComplete: true,
			},
			configExists: true,
			want:         false,
		},
		{
			name: "legacy auto config skips",
			cfg: config.Config{
				EmbeddingProvider:      "auto",
				EmbeddingSetupComplete: false,
			},
			configExists: true,
			want:         false,
		},
		{
			name: "existing none config still prompts until answered",
			cfg: config.Config{
				EmbeddingProvider:      "none",
				EmbeddingSetupComplete: false,
			},
			configExists: true,
			want:         true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldPromptForEmbeddingSetup(tc.cfg, tc.configExists); got != tc.want {
				t.Fatalf("shouldPromptForEmbeddingSetup() = %v, want %v", got, tc.want)
			}
		})
	}
}
