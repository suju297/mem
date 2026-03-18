package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mem/internal/config"
	"mem/internal/repo"
)

func TestDeleteRemovesRepoSetupAndDB(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "init")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	info, err := repo.Detect(repoDir)
	if err != nil {
		t.Fatalf("detect repo: %v", err)
	}
	repoDataDir := filepath.Dir(cfg.RepoDBPath(info.ID))
	if _, err := os.Stat(repoDataDir); err != nil {
		t.Fatalf("expected repo data dir to exist: %v", err)
	}

	_ = runCLI(t, "delete", "--yes")

	if _, err := os.Stat(repoDataDir); !os.IsNotExist(err) {
		t.Fatalf("expected repo data dir to be deleted, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".mem")); !os.IsNotExist(err) {
		t.Fatalf("expected .mem dir to be deleted, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected managed AGENTS.md to be deleted, got err=%v", err)
	}

	cfgAfter, err := config.Load()
	if err != nil {
		t.Fatalf("reload config: %v", err)
	}
	if cfgAfter.ActiveRepo != "" {
		t.Fatalf("expected active repo to be cleared, got %q", cfgAfter.ActiveRepo)
	}
	if repoID, ok := cfgAfter.RepoCache[repoDir]; ok && strings.TrimSpace(repoID) != "" {
		t.Fatalf("expected repo cache entry to be cleared, got %q", repoID)
	}
}

func TestDeletePreservesUserOwnedAgentsStub(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	sentinel := "KEEP THIS"
	writeFile(t, repoDir, "AGENTS.md", sentinel)

	_ = runCLI(t, "init")

	if _, err := os.Stat(filepath.Join(repoDir, ".mem", "AGENTS.md")); err != nil {
		t.Fatalf("expected alternate .mem/AGENTS.md to exist: %v", err)
	}

	_ = runCLI(t, "delete", "--yes")

	data, err := os.ReadFile(filepath.Join(repoDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if strings.TrimSpace(string(data)) != sentinel {
		t.Fatalf("expected root AGENTS.md to be preserved")
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".mem")); !os.IsNotExist(err) {
		t.Fatalf("expected .mem dir to be deleted, got err=%v", err)
	}
}

func TestDeleteSkipsModifiedManagedCompatibilityStub(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "init", "--claude")

	claudePath := filepath.Join(repoDir, "CLAUDE.md")
	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("read CLAUDE.md: %v", err)
	}
	if err := os.WriteFile(claudePath, append(data, []byte("\nLocal note\n")...), 0o644); err != nil {
		t.Fatalf("rewrite CLAUDE.md: %v", err)
	}

	_ = runCLI(t, "delete", "--yes")

	if _, err := os.Stat(claudePath); err != nil {
		t.Fatalf("expected modified CLAUDE.md to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".mem")); !os.IsNotExist(err) {
		t.Fatalf("expected .mem dir to be deleted, got err=%v", err)
	}
}

func TestDeletePromptCanAbort(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "init")

	origInteractive := deletePromptInteractive
	deletePromptInteractive = func() bool { return true }
	t.Cleanup(func() {
		deletePromptInteractive = origInteractive
	})

	_ = runCLIWithInput(t, "n\n", "delete")

	if _, err := os.Stat(filepath.Join(repoDir, ".mem")); err != nil {
		t.Fatalf("expected .mem dir to remain after abort: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md to remain after abort: %v", err)
	}
}

func TestDeleteRequiresYesWhenNonInteractive(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "init")

	errOut := runCLIExpectError(t, "delete")
	if !strings.Contains(errOut, "without --yes") {
		t.Fatalf("expected non-interactive refusal, got: %s", errOut)
	}
}
