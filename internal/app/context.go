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
	if rt := activeMCPRuntime(); rt != nil {
		return rt.configCopy(), nil
	}
	return config.Load()
}

func openStore(cfg config.Config, repoID string) (*store.Store, error) {
	path := cfg.RepoDBPath(repoID)
	return store.Open(path)
}

func openStoreForRequest(cfg config.Config, repoID string) (*store.Store, func(), error) {
	if rt := activeMCPRuntime(); rt != nil {
		st, err := rt.openStore(cfg, repoID)
		if err != nil {
			return nil, nil, err
		}
		return st, func() {}, nil
	}
	st, err := openStore(cfg, repoID)
	if err != nil {
		return nil, nil, err
	}
	return st, func() { _ = st.Close() }, nil
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
	info, _, err := reporesolve.Resolve(cfg, repoOverride, reporesolve.ResolveOptions{
		RequireRepo:            opts.RequireRepo,
		AllowNonStrictFallback: true,
		PersistCache:           true,
	})
	if err != nil {
		return repo.Info{}, err
	}
	finalized, err := finalizeRepo(cfg, info)
	if err != nil {
		return repo.Info{}, err
	}
	if rt := activeMCPRuntime(); rt != nil {
		rt.mergeRepoState(*cfg)
	}
	return finalized, nil
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
