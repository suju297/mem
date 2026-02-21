package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type RepoConfig struct {
	MCPAllowWrite     *bool   `json:"mcp_allow_write,omitempty"`
	MCPWriteMode      *string `json:"mcp_write_mode,omitempty"`
	EmbeddingProvider *string `json:"embedding_provider,omitempty"`
	EmbeddingModel    *string `json:"embedding_model,omitempty"`
	TokenBudget       *int    `json:"token_budget,omitempty"`
	DefaultThread     *string `json:"default_thread,omitempty"`
}

type repoConfigCacheEntry struct {
	LoadedAt time.Time
	ModTime  time.Time
	Size     int64
	Exists   bool
	Config   RepoConfig
}

var repoConfigCache = struct {
	mu      sync.RWMutex
	entries map[string]repoConfigCacheEntry
}{
	entries: map[string]repoConfigCacheEntry{},
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

	stat, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cacheRepoConfig(path, repoConfigCacheEntry{
				LoadedAt: time.Now(),
				Exists:   false,
			})
			return RepoConfig{}, false, nil
		}
		return RepoConfig{}, false, err
	}

	if cached, ok := readRepoConfigCache(path); ok {
		if cached.Exists && cached.ModTime.Equal(stat.ModTime()) && cached.Size == stat.Size() {
			return cached.Config, true, nil
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cacheRepoConfig(path, repoConfigCacheEntry{
				LoadedAt: time.Now(),
				Exists:   false,
			})
			return RepoConfig{}, false, nil
		}
		return RepoConfig{}, false, err
	}

	if len(strings.TrimSpace(string(data))) == 0 {
		cacheRepoConfig(path, repoConfigCacheEntry{
			LoadedAt: time.Now(),
			ModTime:  stat.ModTime(),
			Size:     stat.Size(),
			Exists:   true,
			Config:   RepoConfig{},
		})
		return RepoConfig{}, true, nil
	}
	var cfg RepoConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return RepoConfig{}, false, err
	}
	cacheRepoConfig(path, repoConfigCacheEntry{
		LoadedAt: time.Now(),
		ModTime:  stat.ModTime(),
		Size:     stat.Size(),
		Exists:   true,
		Config:   cfg,
	})
	return cfg, true, nil
}

func readRepoConfigCache(path string) (repoConfigCacheEntry, bool) {
	repoConfigCache.mu.RLock()
	defer repoConfigCache.mu.RUnlock()
	entry, ok := repoConfigCache.entries[path]
	return entry, ok
}

func cacheRepoConfig(path string, entry repoConfigCacheEntry) {
	repoConfigCache.mu.Lock()
	defer repoConfigCache.mu.Unlock()
	repoConfigCache.entries[path] = entry
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
