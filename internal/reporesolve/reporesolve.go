package reporesolve

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mempack/internal/pathutil"
	"mempack/internal/store"
)

func LooksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	if filepath.IsAbs(value) {
		return true
	}
	if strings.HasPrefix(value, ".") || strings.HasPrefix(value, "~") {
		return true
	}
	if strings.ContainsRune(value, os.PathSeparator) {
		return true
	}
	if strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return true
	}
	if len(value) >= 2 && value[1] == ':' {
		return true
	}
	return false
}

func RepoIDFromRoot(repoDir, repoPath string) (string, error) {
	cleanPath := pathutil.Canonical(repoPath)
	if cleanPath == "" {
		return "", fmt.Errorf("repo path is empty")
	}

	entries, err := os.ReadDir(repoDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("repo not found: %s", repoPath)
		}
		return "", err
	}

	bestLen := -1
	bestID := ""
	sep := string(os.PathSeparator)
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
		if strings.TrimSpace(repoRow.GitRoot) == "" {
			continue
		}
		cleanRoot := pathutil.Canonical(repoRow.GitRoot)
		if cleanRoot == "." || cleanRoot == "" {
			continue
		}
		if cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+sep) {
			if len(cleanRoot) > bestLen {
				bestLen = len(cleanRoot)
				bestID = repoRow.RepoID
			}
		}
	}

	if bestLen == -1 {
		return "", fmt.Errorf("repo not found: %s", repoPath)
	}
	return bestID, nil
}
