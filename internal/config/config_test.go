package config

import (
	"os"
	"path/filepath"
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
	if cfg.DefaultWorkspace != "default" {
		t.Errorf("Expected default workspace default, got %s", cfg.DefaultWorkspace)
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
	cfg.DefaultWorkspace = "workspace-a"
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
	if cfg2.DefaultWorkspace != "workspace-a" {
		t.Errorf("Expected default workspace workspace-a, got %s", cfg2.DefaultWorkspace)
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
	if _, err := os.Stat(filepath.Join(tmpDir, "new-config", "mempack", "config.toml")); err != nil {
		t.Error("Config file not created in new dir")
	}
}
