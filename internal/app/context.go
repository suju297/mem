package app

import (
	"fmt"
	"os"
	"strings"

	"mempack/internal/config"
	"mempack/internal/pathutil"
	"mempack/internal/repo"
	"mempack/internal/reporesolve"
	"mempack/internal/store"
)

func loadConfig() (config.Config, error) {
	return config.Load()
}

func openStore(cfg config.Config, repoID string) (*store.Store, error) {
	path := cfg.RepoDBPath(repoID)
	return store.Open(path)
}

type repoResolveOptions struct {
	RequireRepo bool
}

func resolveRepo(cfg *config.Config, repoOverride string) (repo.Info, error) {
	return resolveRepoWithOptions(cfg, repoOverride, repoResolveOptions{})
}

func resolveRepoWithOptions(cfg *config.Config, repoOverride string, opts repoResolveOptions) (repo.Info, error) {
	if cfg == nil {
		return repo.Info{}, fmt.Errorf("config is nil")
	}
	if repoOverride != "" {
		if info, err := detectRepoPath(repoOverride); err == nil {
			if updateRepoCache(cfg, info) {
				_ = cfg.SaveRepoState()
			}
			return finalizeRepo(cfg, info)
		}
		if reporesolve.LooksLikePath(repoOverride) {
			if info, err := repoFromRoot(cfg, repoOverride); err == nil {
				if updateRepoCache(cfg, info) {
					_ = cfg.SaveRepoState()
				}
				return finalizeRepo(cfg, info)
			}
		}
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
			return finalizeRepo(cfg, info)
		}
		// Fall through to full detection on error
	}

	// Need to detect git root - this is 1 git call
	info, err := repo.DetectBaseStrict(cwd)
	if err == nil {
		// Check if we have this root cached - skip origin lookup entirely
		if cachedID, ok := cfg.RepoCache[info.GitRoot]; ok && cachedID != "" {
			info.ID = cachedID
			return finalizeRepo(cfg, info)
		}

		// If cache is missing, attempt to locate an existing repo DB by root.
		// This prevents repo_id churn when paths differ only by symlinks (e.g. /tmp vs /private/tmp),
		// and also makes new installs find existing repos without requiring mem use.
		if existing, err := repoFromRoot(cfg, info.GitRoot); err == nil && existing.ID != "" {
			if updateRepoCache(cfg, existing) {
				_ = cfg.SaveRepoState()
			}
			return finalizeRepo(cfg, existing)
		}

		// New repo - need origin lookup for ID computation
		info, err = repo.PopulateOriginAndID(info)
		if err != nil {
			return repo.Info{}, err
		}
		if updateRepoCache(cfg, info) {
			_ = cfg.SaveRepoState()
		}
		return finalizeRepo(cfg, info)
	}

	if opts.RequireRepo {
		return repo.Info{}, fmt.Errorf("repo not specified and could not detect repo from current directory. Fix: pass repo in the MCP call (workspace root), or start server with mem mcp --repo /path/to/repo")
	}

	// Fall back to non-strict detection (allows non-git dirs) before using active repo.
	info, err = repo.Detect(cwd)
	if err == nil {
		if updateRepoCache(cfg, info) {
			_ = cfg.SaveRepoState()
		}
		return finalizeRepo(cfg, info)
	}

	if cfg.ActiveRepo != "" {
		return repoFromID(cfg, cfg.ActiveRepo)
	}

	return repo.Info{}, err
}

func repoFromID(cfg *config.Config, repoID string) (repo.Info, error) {
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
		fallbackInfo := repo.Info{
			ID:      repoID,
			GitRoot: meta.GitRoot,
			Head:    meta.LastHead,
			Branch:  meta.LastBranch,
			HasGit:  meta.LastHead != "" || meta.LastBranch != "",
		}
		return finalizeRepo(cfg, fallbackInfo)
	}
	if updateRepoCache(cfg, info) {
		_ = cfg.SaveRepoState()
	}
	return finalizeRepo(cfg, info)
}

func cachedRepoForCwd(cfg *config.Config, cwd string) (string, string) {
	if len(cfg.RepoCache) == 0 {
		return "", ""
	}
	cleanCwd := pathutil.Canonical(cwd)
	bestRoot := ""
	bestID := ""
	sep := string(os.PathSeparator)
	for root, repoID := range cfg.RepoCache {
		if repoID == "" {
			continue
		}
		cleanRoot := pathutil.Canonical(root)
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

func repoFromRoot(cfg *config.Config, repoPath string) (repo.Info, error) {
	if cfg == nil {
		return repo.Info{}, fmt.Errorf("config is nil")
	}
	cleanPath := pathutil.Canonical(repoPath)
	if cleanPath == "" {
		return repo.Info{}, fmt.Errorf("repo path is empty")
	}

	if _, repoID := cachedRepoForCwd(cfg, cleanPath); repoID != "" {
		return repoFromID(cfg, repoID)
	}
	repoID, err := reporesolve.RepoIDFromRoot(cfg.RepoRootDir(), cleanPath)
	if err != nil {
		return repo.Info{}, err
	}
	return repoFromID(cfg, repoID)
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

func finalizeRepo(cfg *config.Config, info repo.Info) (repo.Info, error) {
	if err := config.ApplyRepoOverrides(cfg, info.GitRoot); err != nil {
		return repo.Info{}, err
	}
	if info.GitRoot != "" {
		maybeUpdateAgentFiles(info.GitRoot)
	}
	return info, nil
}
