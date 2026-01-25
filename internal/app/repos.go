package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"mempack/internal/store"
)

type RepoListItem struct {
	RepoID     string `json:"repo_id"`
	GitRoot    string `json:"git_root"`
	LastSeenAt string `json:"last_seen_at"`
}

func runRepos(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("repos", flag.ContinueOnError)
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}

	repoDir := cfg.RepoRootDir()
	entries, err := os.ReadDir(repoDir)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(errOut, "repos error: %v\n", err)
		return 1
	}

	var items []RepoListItem
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
		st.Close()
		if err != nil {
			continue
		}
		items = append(items, RepoListItem{
			RepoID:     repoRow.RepoID,
			GitRoot:    repoRow.GitRoot,
			LastSeenAt: repoRow.LastSeenAt.UTC().Format(time.RFC3339Nano),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].LastSeenAt > items[j].LastSeenAt
	})

	encoded, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "json error: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(encoded))
	return 0
}
