package store

import "strings"

func normalizeWorkspace(workspace string) string {
	ws := strings.TrimSpace(workspace)
	if ws == "" {
		return "default"
	}
	return ws
}
