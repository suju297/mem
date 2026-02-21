package reporesolve

import (
	"fmt"
	"os"
	"strings"

	"mempack/internal/config"
	"mempack/internal/pathutil"
	"mempack/internal/repo"
	"mempack/internal/store"
)

type ResolveOptions struct {
	CWD                    string
	RequireRepo            bool
	AllowNonStrictFallback bool
	PersistCache           bool
}

func Resolve(cfg *config.Config, repoOverride string, opts ResolveOptions) (repo.Info, string, error) {
	if cfg == nil {
		return repo.Info{}, "", fmt.Errorf("config is nil")
	}
	repoOverride = strings.TrimSpace(repoOverride)
	if repoOverride != "" {
		if info, err := detectRepoPath(repoOverride); err == nil {
			persistRepoCache(cfg, opts, updateRepoCache(cfg, info))
			return info, "path", nil
		}
		if LooksLikePath(repoOverride) {
			if info, err := repoFromRoot(cfg, repoOverride); err == nil {
				persistRepoCache(cfg, opts, updateRepoCache(cfg, info))
				return info, "db_root", nil
			}
		}
		info, err := repoFromID(cfg, repoOverride)
		if err != nil {
			return repo.Info{}, "", err
		}
		return info, "repo_id", nil
	}

	cwd := strings.TrimSpace(opts.CWD)
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return repo.Info{}, "", err
		}
	}

	if root, repoID := cachedRepoForPath(cfg, cwd); repoID != "" {
		info, err := repo.InfoFromCache(repoID, root, "", "", true)
		if err == nil {
			return info, "cache_cwd", nil
		}
	}

	info, strictErr := repo.DetectBaseStrict(cwd)
	if strictErr == nil {
		if cachedID, ok := cfg.RepoCache[info.GitRoot]; ok && strings.TrimSpace(cachedID) != "" {
			info.ID = strings.TrimSpace(cachedID)
			return info, "cwd", nil
		}

		if existing, err := repoFromRoot(cfg, info.GitRoot); err == nil && strings.TrimSpace(existing.ID) != "" {
			persistRepoCache(cfg, opts, updateRepoCache(cfg, existing))
			return existing, "db_root", nil
		}

		populated, err := repo.PopulateOriginAndID(info)
		if err != nil {
			return repo.Info{}, "", err
		}
		persistRepoCache(cfg, opts, updateRepoCache(cfg, populated))
		return populated, "cwd", nil
	}

	if opts.RequireRepo {
		return repo.Info{}, "", fmt.Errorf("repo not specified and could not detect repo from current directory. Fix: pass repo in the MCP call (workspace root), or start server with mem mcp --repo /path/to/repo")
	}

	if opts.AllowNonStrictFallback {
		if detected, err := repo.Detect(cwd); err == nil {
			persistRepoCache(cfg, opts, updateRepoCache(cfg, detected))
			return detected, "cwd_fallback", nil
		}
	}

	if strings.TrimSpace(cfg.ActiveRepo) != "" {
		info, err := repoFromID(cfg, cfg.ActiveRepo)
		if err == nil {
			return info, "active_repo", nil
		}
	}

	return repo.Info{}, "", strictErr
}

func detectRepoPath(path string) (repo.Info, error) {
	if _, err := os.Stat(path); err != nil {
		return repo.Info{}, err
	}
	info, err := repo.DetectBaseStrict(path)
	if err != nil {
		return repo.Info{}, err
	}
	return repo.PopulateOriginAndID(info)
}

func repoFromRoot(cfg *config.Config, repoPath string) (repo.Info, error) {
	if cfg == nil {
		return repo.Info{}, fmt.Errorf("config is nil")
	}
	cleanPath := pathutil.Canonical(repoPath)
	if cleanPath == "" {
		return repo.Info{}, fmt.Errorf("repo path is empty")
	}
	if _, repoID := cachedRepoForPath(cfg, cleanPath); repoID != "" {
		return repoFromID(cfg, repoID)
	}
	repoID, err := RepoIDFromRoot(cfg.RepoRootDir(), cleanPath)
	if err != nil {
		return repo.Info{}, err
	}
	return repoFromID(cfg, repoID)
}

func repoFromID(cfg *config.Config, repoID string) (repo.Info, error) {
	if cfg == nil {
		return repo.Info{}, fmt.Errorf("config is nil")
	}
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return repo.Info{}, fmt.Errorf("repo id is empty")
	}
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

	info, err := repo.InfoFromCache(repoID, meta.GitRoot, meta.LastHead, meta.LastBranch, true)
	if err != nil {
		return repo.Info{
			ID:      repoID,
			GitRoot: meta.GitRoot,
			Head:    meta.LastHead,
			Branch:  meta.LastBranch,
			HasGit:  meta.LastHead != "" || meta.LastBranch != "",
		}, nil
	}
	return info, nil
}

func cachedRepoForPath(cfg *config.Config, path string) (string, string) {
	if cfg == nil || len(cfg.RepoCache) == 0 {
		return "", ""
	}
	cleanPath := pathutil.Canonical(path)
	bestRoot := ""
	bestID := ""
	sep := string(os.PathSeparator)
	for root, repoID := range cfg.RepoCache {
		repoID = strings.TrimSpace(repoID)
		if repoID == "" {
			continue
		}
		cleanRoot := pathutil.Canonical(root)
		if cleanRoot == "." || cleanRoot == "" {
			continue
		}
		if cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+sep) {
			if len(cleanRoot) > len(bestRoot) {
				bestRoot = cleanRoot
				bestID = repoID
			}
		}
	}
	return bestRoot, bestID
}

func updateRepoCache(cfg *config.Config, info repo.Info) bool {
	if cfg == nil || !info.HasGit || strings.TrimSpace(info.GitRoot) == "" || strings.TrimSpace(info.ID) == "" {
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

func persistRepoCache(cfg *config.Config, opts ResolveOptions, changed bool) {
	if !changed || cfg == nil || !opts.PersistCache {
		return
	}
	_ = cfg.SaveRepoState()
}
