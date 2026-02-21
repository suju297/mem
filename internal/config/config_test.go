package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSave(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mempack-test-config-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Set env vars to point to tmpDir
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "data"))

	// Load default (should succeed with defaults even if file missing)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load default failed: %v", err)
	}
	if cfg.TokenBudget != 2500 {
		t.Errorf("Expected default budget 2500, got %d", cfg.TokenBudget)
	}
	if cfg.MCPAutoRepair {
		t.Errorf("Expected default mcp_auto_repair false, got true")
	}
	if !cfg.MCPAllowWrite {
		t.Errorf("Expected default mcp_allow_write true, got false")
	}
	if cfg.MCPWriteMode != "ask" {
		t.Errorf("Expected default mcp_write_mode ask, got %s", cfg.MCPWriteMode)
	}
	if cfg.DefaultWorkspace != "default" {
		t.Errorf("Expected default workspace default, got %s", cfg.DefaultWorkspace)
	}
	if cfg.DefaultThread != "T-SESSION" {
		t.Errorf("Expected default thread T-SESSION, got %s", cfg.DefaultThread)
	}
	if cfg.EmbeddingProvider != "auto" {
		t.Errorf("Expected default embedding provider auto, got %s", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "nomic-embed-text" {
		t.Errorf("Expected default embedding model nomic-embed-text, got %s", cfg.EmbeddingModel)
	}
	if cfg.EmbeddingMinSimilarity != 0.6 {
		t.Errorf("Expected default embedding min similarity 0.6, got %v", cfg.EmbeddingMinSimilarity)
	}

	// Modify and Save
	cfg.ActiveRepo = "test-repo"
	cfg.TokenBudget = 5000
	cfg.MCPAutoRepair = true
	cfg.MCPAllowWrite = true
	cfg.MCPWriteMode = "auto"
	cfg.DefaultWorkspace = "workspace-a"
	cfg.DefaultThread = "T-DEFAULT"
	cfg.EmbeddingProvider = "ollama"
	cfg.EmbeddingModel = "nomic-embed-text"
	cfg.EmbeddingMinSimilarity = 0.7
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load again
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Reload failed: %v", err)
	}
	if cfg2.ActiveRepo != "test-repo" {
		t.Errorf("Expected ActiveRepo test-repo, got %s", cfg2.ActiveRepo)
	}
	if cfg2.TokenBudget != 5000 {
		t.Errorf("Expected Budget 5000, got %d", cfg2.TokenBudget)
	}
	if !cfg2.MCPAutoRepair {
		t.Errorf("Expected mcp_auto_repair true, got false")
	}
	if !cfg2.MCPAllowWrite {
		t.Errorf("Expected mcp_allow_write true, got false")
	}
	if cfg2.MCPWriteMode != "auto" {
		t.Errorf("Expected mcp_write_mode auto, got %s", cfg2.MCPWriteMode)
	}
	if cfg2.DefaultWorkspace != "workspace-a" {
		t.Errorf("Expected default workspace workspace-a, got %s", cfg2.DefaultWorkspace)
	}
	if cfg2.DefaultThread != "T-DEFAULT" {
		t.Errorf("Expected default thread T-DEFAULT, got %s", cfg2.DefaultThread)
	}
	if cfg2.EmbeddingProvider != "ollama" {
		t.Errorf("Expected embedding provider ollama, got %s", cfg2.EmbeddingProvider)
	}
	if cfg2.EmbeddingModel != "nomic-embed-text" {
		t.Errorf("Expected embedding model nomic-embed-text, got %s", cfg2.EmbeddingModel)
	}
	if cfg2.EmbeddingMinSimilarity != 0.7 {
		t.Errorf("Expected embedding min similarity 0.7, got %v", cfg2.EmbeddingMinSimilarity)
	}

	// Edge Case: Missing config dir should be created on Save
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "new-config"))
	cfg3, _ := Load() // Load default
	if err := cfg3.Save(); err != nil {
		t.Errorf("Expected Save to create dir, got error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "new-config", "mem", "config.toml")); err != nil {
		t.Error("Config file not created in new dir")
	}
}

func TestSaveRepoStateDoesNotPersistDataDirOverride(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mempack-test-config-state-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(tmpDir, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(tmpDir, "cache"))

	SetDataDirOverride("")
	defer SetDataDirOverride("")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	persistedDataDir := filepath.Join(tmpDir, "persisted-data")
	cfg.DataDir = persistedDataDir
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	overrideDir := filepath.Join(tmpDir, "override-data")
	SetDataDirOverride(overrideDir)

	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Reload with override failed: %v", err)
	}
	cfg2.ActiveRepo = "repo-after"
	cfg2.RepoCache = map[string]string{"/tmp/repo": "r_test1234"}
	if err := cfg2.SaveRepoState(); err != nil {
		t.Fatalf("SaveRepoState failed: %v", err)
	}

	configPath := filepath.Join(cfg2.ConfigDir, "config.toml")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Read config failed: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `data_dir = "`+persistedDataDir+`"`) {
		t.Fatalf("expected persisted data_dir %q in config.toml", persistedDataDir)
	}
	if strings.Contains(text, overrideDir) {
		t.Fatalf("runtime override data_dir %q leaked into config.toml", overrideDir)
	}
	if !strings.Contains(text, `active_repo = "repo-after"`) {
		t.Fatalf("expected active_repo to be persisted by SaveRepoState")
	}
}

func TestApplyRepoOverrides(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mempack-test-repo-config-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repoDir := filepath.Join(tmpDir, "repo")
	if err := os.MkdirAll(filepath.Join(repoDir, ".mempack"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(repoDir, ".mempack", "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "mcp_allow_write": true,
  "mcp_write_mode": "auto",
  "embedding_provider": "none",
  "embedding_model": "nomic-embed-text",
  "token_budget": 3200,
  "default_thread": "T-REPO"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyRepoOverrides(&cfg, repoDir); err != nil {
		t.Fatalf("ApplyRepoOverrides failed: %v", err)
	}
	if !cfg.MCPAllowWrite {
		t.Errorf("Expected mcp_allow_write true, got false")
	}
	if cfg.MCPWriteMode != "auto" {
		t.Errorf("Expected mcp_write_mode auto, got %s", cfg.MCPWriteMode)
	}
	if cfg.EmbeddingProvider != "none" {
		t.Errorf("Expected embedding_provider none, got %s", cfg.EmbeddingProvider)
	}
	if cfg.EmbeddingModel != "nomic-embed-text" {
		t.Errorf("Expected embedding_model nomic-embed-text, got %s", cfg.EmbeddingModel)
	}
	if cfg.TokenBudget != 3200 {
		t.Errorf("Expected token_budget 3200, got %d", cfg.TokenBudget)
	}
	if cfg.DefaultThread != "T-REPO" {
		t.Errorf("Expected default_thread T-REPO, got %s", cfg.DefaultThread)
	}
}
