package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mempack/internal/config"
	"mempack/internal/repo"
	"mempack/internal/store"
)

type UseResponse struct {
	ActiveRepo string `json:"active_repo"`
	GitRoot    string `json:"git_root,omitempty"`
}

func runUse(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("use", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	arg := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if arg == "" {
		fmt.Fprintln(errOut, "missing repo id or path")
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}

	if info, err := detectRepoPath(arg); err == nil {
		st, err := openStore(cfg, info.ID)
		if err != nil {
			fmt.Fprintf(errOut, "store open error: %v\n", err)
			return 1
		}
		if err := st.EnsureRepo(info); err != nil {
			st.Close()
			fmt.Fprintf(errOut, "store repo error: %v\n", err)
			return 1
		}
		st.Close()

		cfg.ActiveRepo = info.ID
		updateRepoCache(&cfg, info)
		if err := cfg.SaveRepoState(); err != nil {
			fmt.Fprintf(errOut, "config save error: %v\n", err)
			return 1
		}
		return writeJSON(out, errOut, UseResponse{ActiveRepo: info.ID, GitRoot: info.GitRoot})
	}

	repoID := arg
	path := cfg.RepoDBPath(repoID)
	if _, err := os.Stat(path); err != nil {
		matches, err := findReposByName(cfg, repoID)
		if err != nil {
			fmt.Fprintf(errOut, "repo lookup error: %v\n", err)
			return 1
		}
		if len(matches) == 0 {
			fmt.Fprintf(errOut, "repo not found: %s\n", repoID)
			return 1
		}
		if len(matches) > 1 {
			fmt.Fprintf(errOut, "multiple repos named %s:\n", repoID)
			for _, match := range matches {
				fmt.Fprintf(errOut, "  %s (%s)\n", match.RepoID, match.GitRoot)
			}
			fmt.Fprintln(errOut, "use a full path or repo id to disambiguate")
			return 1
		}
		repoID = matches[0].RepoID
		path = cfg.RepoDBPath(repoID)
	}
	cfg.ActiveRepo = repoID
	if err := cfg.SaveRepoState(); err != nil {
		fmt.Fprintf(errOut, "config save error: %v\n", err)
		return 1
	}

	st, err := store.Open(path)
	if err != nil {
		return writeJSON(out, errOut, UseResponse{ActiveRepo: repoID})
	}
	defer st.Close()
	meta, err := st.GetRepo(repoID)
	if err != nil {
		return writeJSON(out, errOut, UseResponse{ActiveRepo: repoID})
	}
	if meta.GitRoot != "" {
		updateRepoCache(&cfg, repo.Info{
			ID:      meta.RepoID,
			GitRoot: meta.GitRoot,
			Head:    meta.LastHead,
			Branch:  meta.LastBranch,
			HasGit:  meta.LastHead != "" || meta.LastBranch != "",
		})
		_ = cfg.SaveRepoState()
	}
	return writeJSON(out, errOut, UseResponse{ActiveRepo: repoID, GitRoot: meta.GitRoot})
}

func detectRepoPath(path string) (repo.Info, error) {
	if _, err := os.Stat(path); err != nil {
		return repo.Info{}, err
	}
	return repo.Detect(path)
}

type repoMatch struct {
	RepoID  string
	GitRoot string
}

func findReposByName(cfg config.Config, name string) ([]repoMatch, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	repoDir := cfg.RepoRootDir()
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	needle := strings.ToLower(name)
	var matches []repoMatch
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		repoID := entry.Name()
		path := filepath.Join(repoDir, repoID, "memory.db")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		st, err := store.Open(path)
		if err != nil {
			continue
		}
		repoRow, err := st.GetRepo(repoID)
		_ = st.Close()
		if err != nil {
			continue
		}
		base := strings.ToLower(filepath.Base(repoRow.GitRoot))
		if base == needle {
			matches = append(matches, repoMatch{RepoID: repoRow.RepoID, GitRoot: repoRow.GitRoot})
		}
	}
	return matches, nil
}
