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
	DefaultWorkspace       string            `toml:"default_workspace"`
	EmbeddingProvider      string            `toml:"embedding_provider"`
	EmbeddingModel         string            `toml:"embedding_model"`
	EmbeddingMinSimilarity float64           `toml:"embedding_min_similarity"`
}

var dataDirOverride string

func SetDataDirOverride(path string) {
	dataDirOverride = strings.TrimSpace(path)
}

func Default() (Config, error) {
	configHome, dataHome, cacheHome, err := xdgHomes()
	if err != nil {
		return Config{}, err
	}

	return Config{
		ConfigDir:              filepath.Join(configHome, "mempack"),
		DataDir:                filepath.Join(dataHome, "mempack"),
		CacheDir:               filepath.Join(cacheHome, "mempack"),
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
		DefaultWorkspace:       "default",
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
	cfg.DataDir = dataDir

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
			if len(existing.RepoCache) > 0 {
				if c.RepoCache == nil {
					c.RepoCache = map[string]string{}
				}
				for key, value := range existing.RepoCache {
					if _, ok := c.RepoCache[key]; !ok {
						c.RepoCache[key] = value
					}
				}
			}
		}
	}
	tmpPath := path + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(c); err != nil {
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
	if env := strings.TrimSpace(os.Getenv("MEMPACK_DATA_DIR")); env != "" {
		return env
	}
	if strings.TrimSpace(cfg.DataDir) != "" {
		return cfg.DataDir
	}
	return filepath.Join(".", "mempack")
}
