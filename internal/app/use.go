package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

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
		if err := cfg.Save(); err != nil {
			fmt.Fprintf(errOut, "config save error: %v\n", err)
			return 1
		}
		return writeJSON(out, errOut, UseResponse{ActiveRepo: info.ID, GitRoot: info.GitRoot})
	}

	repoID := arg
	path := cfg.RepoDBPath(repoID)
	if _, err := os.Stat(path); err != nil {
		fmt.Fprintf(errOut, "repo not found: %s\n", repoID)
		return 1
	}
	cfg.ActiveRepo = repoID
	if err := cfg.Save(); err != nil {
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
	return writeJSON(out, errOut, UseResponse{ActiveRepo: repoID, GitRoot: meta.GitRoot})
}

func detectRepoPath(path string) (repo.Info, error) {
	if _, err := os.Stat(path); err != nil {
		return repo.Info{}, err
	}
	return repo.Detect(path)
}
