package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mempack/internal/config"
	"mempack/internal/repo"
	"mempack/internal/store"
)

func loadConfig() (config.Config, error) {
	return config.Load()
}

func openStore(cfg config.Config, repoID string) (*store.Store, error) {
	path := cfg.RepoDBPath(repoID)
	return store.Open(path)
}

func resolveRepo(cfg config.Config, repoOverride string) (repo.Info, error) {
	if repoOverride != "" {
		return repoFromID(cfg, repoOverride)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return repo.Info{}, err
	}

	// Fast path: check if cwd is inside a known cached git_root
	// This avoids the expensive git rev-parse --show-toplevel call
	if root, repoID := cachedRepoForCwd(cfg, cwd); repoID != "" {
		// Use InfoFromCache with needsFreshHead=true for 1 git call
		// instead of DetectFromRoot which also does 1 git call but this is clearer
		info, err := repo.InfoFromCache(repoID, root, "", "", true)
		if err == nil {
			return info, nil
		}
		// Fall through to full detection on error
	}

	// Need to detect git root - this is 1 git call
	info, err := repo.DetectBaseStrict(cwd)
	if err == nil {
		// Check if we have this root cached - skip origin lookup entirely
		if cachedID, ok := cfg.RepoCache[info.GitRoot]; ok && cachedID != "" {
			info.ID = cachedID
			return info, nil
		}

		// New repo - need origin lookup for ID computation
		info, err = repo.PopulateOriginAndID(info)
		if err != nil {
			return repo.Info{}, err
		}
		if updateRepoCache(&cfg, info) {
			_ = cfg.Save()
		}
		return info, nil
	}

	if cfg.ActiveRepo != "" {
		return repoFromID(cfg, cfg.ActiveRepo)
	}

	return repo.Detect(cwd)
}

func repoFromID(cfg config.Config, repoID string) (repo.Info, error) {
	path := cfg.RepoDBPath(repoID)
	if _, err := os.Stat(path); err != nil {
		return repo.Info{}, fmt.Errorf("repo %s not found", repoID)
	}
	st, err := store.Open(path)
	if err != nil {
		return repo.Info{}, err
	}
	defer st.Close()

	meta, err := st.GetRepo(repoID)
	if err != nil {
		return repo.Info{}, err
	}

	// Use InfoFromCache with needsFreshHead=true - makes 1 git call for current HEAD
	// Falls back to cached values if git fails (e.g., if not in repo directory)
	info, err := repo.InfoFromCache(repoID, meta.GitRoot, meta.LastHead, meta.LastBranch, true)
	if err != nil {
		// Should not happen since InfoFromCache handles errors internally
		return repo.Info{
			ID:      repoID,
			GitRoot: meta.GitRoot,
			Head:    meta.LastHead,
			Branch:  meta.LastBranch,
			HasGit:  meta.LastHead != "" || meta.LastBranch != "",
		}, nil
	}
	if updateRepoCache(&cfg, info) {
		_ = cfg.Save()
	}
	return info, nil
}

func cachedRepoForCwd(cfg config.Config, cwd string) (string, string) {
	if len(cfg.RepoCache) == 0 {
		return "", ""
	}
	cleanCwd := filepath.Clean(cwd)
	bestRoot := ""
	bestID := ""
	sep := string(os.PathSeparator)
	for root, repoID := range cfg.RepoCache {
		if repoID == "" {
			continue
		}
		cleanRoot := filepath.Clean(root)
		if cleanRoot == "." || cleanRoot == "" {
			continue
		}
		if cleanCwd == cleanRoot || strings.HasPrefix(cleanCwd, cleanRoot+sep) {
			if len(cleanRoot) > len(bestRoot) {
				bestRoot = cleanRoot
				bestID = repoID
			}
		}
	}
	return bestRoot, bestID
}

func updateRepoCache(cfg *config.Config, info repo.Info) bool {
	if !info.HasGit || info.GitRoot == "" || info.ID == "" {
		return false
	}
	if cfg.RepoCache == nil {
		cfg.RepoCache = map[string]string{}
	}
	if existing, ok := cfg.RepoCache[info.GitRoot]; ok && existing == info.ID {
		return false
	}
	cfg.RepoCache[info.GitRoot] = info.ID
	return true
}
