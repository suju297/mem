package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mempack/internal/repo"
	"mempack/internal/store"
)

func loadState(repoInfo repo.Info, workspace string, st *store.Store) (json.RawMessage, int, string, string, error) {
	warning := ""
	if st != nil {
		stateJSON, stateTokens, updatedAt, err := st.GetStateCurrent(repoInfo.ID, workspace)
		if err == nil {
			return json.RawMessage(stateJSON), stateTokens, updatedAt, "", nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			warning = formatStateWarning("state_db_error", err)
		}
	}

	stateFromRepo, updatedAt, err := loadStateFromRepoFiles(repoInfo.GitRoot)
	if err != nil {
		warning = joinStateWarnings(warning, formatStateWarning("state_repo_error", err))
		return json.RawMessage("{}"), 0, "", warning, nil
	}
	if stateFromRepo != nil {
		return stateFromRepo, 0, updatedAt, warning, nil
	}

	return json.RawMessage("{}"), 0, "", warning, nil
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

func formatStateWarning(prefix string, err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", prefix, msg)
}

func joinStateWarnings(existing, next string) string {
	if next == "" {
		return existing
	}
	if existing == "" {
		return next
	}
	return existing + ";" + next
}
