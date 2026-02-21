package config

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	ConfigDir              string            `toml:"config_dir"`
	DataDir                string            `toml:"data_dir"`
	CacheDir               string            `toml:"cache_dir"`
	ActiveRepo             string            `toml:"active_repo"`
	RepoCache              map[string]string `toml:"repo_cache"`
	Tokenizer              string            `toml:"tokenizer"`
	TokenBudget            int               `toml:"token_budget"`
	StateMax               int               `toml:"state_max"`
	MemoryMaxEach          int               `toml:"memory_max_each"`
	MemoriesK              int               `toml:"memories_k"`
	ChunksK                int               `toml:"chunks_k"`
	ChunkMaxEach           int               `toml:"chunk_max_each"`
	MCPAutoRepair          bool              `toml:"mcp_auto_repair"`
	MCPAllowWrite          bool              `toml:"mcp_allow_write"`
	MCPWriteMode           string            `toml:"mcp_write_mode"`
	MCPRequireRepo         bool              `toml:"mcp_require_repo"`
	DefaultWorkspace       string            `toml:"default_workspace"`
	DefaultThread          string            `toml:"default_thread"`
	EmbeddingProvider      string            `toml:"embedding_provider"`
	EmbeddingModel         string            `toml:"embedding_model"`
	EmbeddingMinSimilarity float64           `toml:"embedding_min_similarity"`
}

var dataDirOverride string

const (
	appDirName       = "mem"
	legacyAppDirName = "mempack"
)

func SetDataDirOverride(path string) {
	dataDirOverride = strings.TrimSpace(path)
}

func Default() (Config, error) {
	configHome, dataHome, cacheHome, err := xdgHomes()
	if err != nil {
		return Config{}, err
	}

	return Config{
		ConfigDir:              preferredAppDir(configHome),
		DataDir:                preferredAppDir(dataHome),
		CacheDir:               preferredAppDir(cacheHome),
		ActiveRepo:             "",
		RepoCache:              map[string]string{},
		Tokenizer:              "cl100k_base",
		TokenBudget:            2500,
		StateMax:               600,
		MemoryMaxEach:          80,
		MemoriesK:              10,
		ChunksK:                4,
		ChunkMaxEach:           320,
		MCPAutoRepair:          false,
		MCPAllowWrite:          true,
		MCPWriteMode:           "ask",
		MCPRequireRepo:         false,
		DefaultWorkspace:       "default",
		DefaultThread:          "T-SESSION",
		EmbeddingProvider:      "auto",
		EmbeddingModel:         "nomic-embed-text",
		EmbeddingMinSimilarity: 0.6,
	}, nil
}

func Load() (Config, error) {
	cfg, err := Default()
	if err != nil {
		return Config{}, err
	}

	path := filepath.Join(cfg.ConfigDir, "config.toml")
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &cfg); err != nil {
			return Config{}, err
		}
	}
	if cfg.RepoCache == nil {
		cfg.RepoCache = map[string]string{}
	}

	dataDir := resolveDataDir(cfg)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) RepoDBPath(repoID string) string {
	return filepath.Join(resolveDataDir(c), "repos", repoID, "memory.db")
}

func (c Config) RepoRootDir() string {
	return filepath.Join(resolveDataDir(c), "repos")
}

func (c Config) Save() error {
	if err := os.MkdirAll(c.ConfigDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(c.ConfigDir, "config.toml")
	if _, err := os.Stat(path); err == nil {
		var existing Config
		if _, err := toml.DecodeFile(path, &existing); err == nil {
			c.RepoCache = mergeRepoCache(existing.RepoCache, c.RepoCache)
		}
	}
	if c.RepoCache == nil {
		c.RepoCache = map[string]string{}
	}
	return writeConfigFile(path, c)
}

// SaveRepoState persists only repo-routing state (active_repo and repo_cache).
// It intentionally preserves all other persisted settings to avoid leaking
// runtime-effective overrides (e.g. --data-dir / MEM_DATA_DIR) into config.toml.
func (c Config) SaveRepoState() error {
	if err := os.MkdirAll(c.ConfigDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(c.ConfigDir, "config.toml")

	persisted, err := Default()
	if err != nil {
		return err
	}
	persisted.ConfigDir = c.ConfigDir

	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &persisted); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	persisted.ActiveRepo = c.ActiveRepo
	persisted.RepoCache = mergeRepoCache(persisted.RepoCache, c.RepoCache)
	return writeConfigFile(path, persisted)
}

func mergeRepoCache(existing map[string]string, updates map[string]string) map[string]string {
	merged := map[string]string{}
	for key, value := range existing {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		merged[key] = value
	}
	for key, value := range updates {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		if value == "" {
			delete(merged, key)
			continue
		}
		merged[key] = value
	}
	return merged
}

func writeConfigFile(path string, cfg Config) error {
	tmpPath := path + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(cfg); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func xdgHomes() (string, string, string, error) {
	configHome := os.Getenv("XDG_CONFIG_HOME")
	dataHome := os.Getenv("XDG_DATA_HOME")
	cacheHome := os.Getenv("XDG_CACHE_HOME")

	if configHome != "" && dataHome != "" && cacheHome != "" {
		return configHome, dataHome, cacheHome, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", "", err
	}

	if configHome == "" {
		configHome = filepath.Join(home, ".config")
	}
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	if cacheHome == "" {
		cacheHome = filepath.Join(home, ".cache")
	}

	return configHome, dataHome, cacheHome, nil
}

func resolveDataDir(cfg Config) string {
	if dataDirOverride != "" {
		return dataDirOverride
	}
	if env := strings.TrimSpace(os.Getenv("MEM_DATA_DIR")); env != "" {
		return env
	}
	if env := strings.TrimSpace(os.Getenv("MEMPACK_DATA_DIR")); env != "" {
		return env
	}
	if strings.TrimSpace(cfg.DataDir) != "" {
		return cfg.DataDir
	}
	return preferredAppDir(".")
}

func preferredAppDir(root string) string {
	primary := filepath.Join(root, appDirName)
	legacy := filepath.Join(root, legacyAppDirName)
	if pathExists(primary) {
		return primary
	}
	if pathExists(legacy) {
		return legacy
	}
	return primary
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
