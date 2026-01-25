package app

import (
	"encoding/json"
	"os"
	"path/filepath"

	"mempack/internal/repo"
	"mempack/internal/store"
)

func loadState(repoInfo repo.Info, workspace string, st *store.Store) (json.RawMessage, int, string, error) {
	if st != nil {
		stateJSON, stateTokens, updatedAt, err := st.GetStateCurrent(repoInfo.ID, workspace)
		if err == nil {
			return json.RawMessage(stateJSON), stateTokens, updatedAt, nil
		}
	}

	stateFromRepo, updatedAt, err := loadStateFromRepoFiles(repoInfo.GitRoot)
	if err != nil {
		return json.RawMessage("{}"), 0, "", nil
	}
	if stateFromRepo != nil {
		return stateFromRepo, 0, updatedAt, nil
	}

	return json.RawMessage("{}"), 0, "", nil
}

func loadStateFromRepoFiles(root string) (json.RawMessage, string, error) {
	stateJSONPath := filepath.Join(root, ".mempack", "state.json")
	if data, err := os.ReadFile(stateJSONPath); err == nil {
		if json.Valid(data) {
			info, statErr := os.Stat(stateJSONPath)
			updatedAt := ""
			if statErr == nil {
				updatedAt = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
			}
			return json.RawMessage(data), updatedAt, nil
		}
		wrapped, err := json.Marshal(map[string]string{
			"raw": string(data),
		})
		return json.RawMessage(wrapped), "", err
	}

	stateMDPath := filepath.Join(root, "STATE.md")
	if data, err := os.ReadFile(stateMDPath); err == nil {
		wrapped, err := json.Marshal(map[string]string{
			"raw_markdown": string(data),
		})
		if err != nil {
			return nil, "", err
		}
		info, statErr := os.Stat(stateMDPath)
		updatedAt := ""
		if statErr == nil {
			updatedAt = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
		}
		return json.RawMessage(wrapped), updatedAt, nil
	}

	return nil, "", nil
}
