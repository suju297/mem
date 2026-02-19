package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type RepoConfig struct {
	MCPAllowWrite     *bool   `json:"mcp_allow_write,omitempty"`
	MCPWriteMode      *string `json:"mcp_write_mode,omitempty"`
	EmbeddingProvider *string `json:"embedding_provider,omitempty"`
	EmbeddingModel    *string `json:"embedding_model,omitempty"`
	TokenBudget       *int    `json:"token_budget,omitempty"`
	DefaultThread     *string `json:"default_thread,omitempty"`
}

func RepoConfigPath(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	return filepath.Join(root, ".mempack", "config.json")
}

func LoadRepoConfig(root string) (RepoConfig, bool, error) {
	path := RepoConfigPath(root)
	if path == "" {
		return RepoConfig{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RepoConfig{}, false, nil
		}
		return RepoConfig{}, false, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return RepoConfig{}, true, nil
	}
	var cfg RepoConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return RepoConfig{}, false, err
	}
	return cfg, true, nil
}

func ApplyRepoOverrides(cfg *Config, root string) error {
	if cfg == nil {
		return nil
	}
	repoCfg, ok, err := LoadRepoConfig(root)
	if err != nil || !ok {
		return err
	}
	if repoCfg.MCPAllowWrite != nil {
		cfg.MCPAllowWrite = *repoCfg.MCPAllowWrite
	}
	if repoCfg.MCPWriteMode != nil {
		mode := strings.TrimSpace(*repoCfg.MCPWriteMode)
		if mode != "" {
			cfg.MCPWriteMode = mode
		}
	}
	if repoCfg.EmbeddingProvider != nil {
		provider := strings.TrimSpace(*repoCfg.EmbeddingProvider)
		if provider != "" {
			cfg.EmbeddingProvider = provider
		}
	}
	if repoCfg.EmbeddingModel != nil {
		model := strings.TrimSpace(*repoCfg.EmbeddingModel)
		if model != "" {
			cfg.EmbeddingModel = model
		}
	}
	if repoCfg.TokenBudget != nil && *repoCfg.TokenBudget > 0 {
		cfg.TokenBudget = *repoCfg.TokenBudget
	}
	if repoCfg.DefaultThread != nil {
		cfg.DefaultThread = strings.TrimSpace(*repoCfg.DefaultThread)
	}
	return nil
}
