package app

import (
	"os"
	"path/filepath"
	"testing"

	"mempack/internal/config"
	"mempack/internal/repo"
)

func TestResolveRepoPrefersCwdOverActiveRepo(t *testing.T) {
	temp := t.TempDir()
	cfg := config.Config{
		ConfigDir:     filepath.Join(temp, "config"),
		DataDir:       filepath.Join(temp, "data"),
		CacheDir:      filepath.Join(temp, "cache"),
		Tokenizer:     "cl100k_base",
		TokenBudget:   2500,
		StateMax:      600,
		MemoryMaxEach: 80,
		MemoriesK:     10,
		ChunksK:       4,
		ChunkMaxEach:  320,
	}

	repoRoot := filepath.Join(temp, "repo-root")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}

	info, err := repo.Detect(repoRoot)
	if err != nil {
		t.Fatalf("detect repo: %v", err)
	}

	st, err := openStore(cfg, info.ID)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.EnsureRepo(info); err != nil {
		st.Close()
		t.Fatalf("ensure repo: %v", err)
	}
	st.Close()

	cfg.ActiveRepo = info.ID

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer os.Chdir(cwd)

	nonRepo := filepath.Join(temp, "non-repo")
	if err := os.MkdirAll(nonRepo, 0o755); err != nil {
		t.Fatalf("mkdir non-repo: %v", err)
	}
	if err := os.Chdir(nonRepo); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	cwdNow, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	expected, err := repo.Detect(cwdNow)
	if err != nil {
		t.Fatalf("detect non-repo: %v", err)
	}

	resolved, err := resolveRepo(&cfg, "")
	if err != nil {
		t.Fatalf("resolve repo: %v", err)
	}
	if resolved.ID != expected.ID {
		t.Fatalf("expected repo id %s, got %s", expected.ID, resolved.ID)
	}
}

func TestResolveRepoOverrideMissing(t *testing.T) {
	temp := t.TempDir()
	cfg := config.Config{
		ConfigDir:     filepath.Join(temp, "config"),
		DataDir:       filepath.Join(temp, "data"),
		CacheDir:      filepath.Join(temp, "cache"),
		Tokenizer:     "cl100k_base",
		TokenBudget:   2500,
		StateMax:      600,
		MemoryMaxEach: 80,
		MemoriesK:     10,
		ChunksK:       4,
		ChunkMaxEach:  320,
	}

	if _, err := resolveRepo(&cfg, "missing"); err == nil {
		t.Fatalf("expected error for missing repo override")
	}
}

func TestResolveRepoOverrideFallsBackToDBRoot(t *testing.T) {
	temp := t.TempDir()
	cfg := config.Config{
		ConfigDir:     filepath.Join(temp, "config"),
		DataDir:       filepath.Join(temp, "data"),
		CacheDir:      filepath.Join(temp, "cache"),
		Tokenizer:     "cl100k_base",
		TokenBudget:   2500,
		StateMax:      600,
		MemoryMaxEach: 80,
		MemoriesK:     10,
		ChunksK:       4,
		ChunkMaxEach:  320,
	}

	repoRoot := filepath.Join(temp, "repo-root")
	if err := os.MkdirAll(repoRoot, 0o755); err != nil {
		t.Fatalf("mkdir repo root: %v", err)
	}

	info, err := repo.Detect(repoRoot)
	if err != nil {
		t.Fatalf("detect repo: %v", err)
	}

	st, err := openStore(cfg, info.ID)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.EnsureRepo(info); err != nil {
		st.Close()
		t.Fatalf("ensure repo: %v", err)
	}
	st.Close()

	if err := os.RemoveAll(repoRoot); err != nil {
		t.Fatalf("remove repo root: %v", err)
	}

	resolved, err := resolveRepoWithOptions(&cfg, repoRoot, repoResolveOptions{})
	if err != nil {
		t.Fatalf("resolve repo: %v", err)
	}
	if resolved.ID != info.ID {
		t.Fatalf("expected repo id %s, got %s", info.ID, resolved.ID)
	}
}
