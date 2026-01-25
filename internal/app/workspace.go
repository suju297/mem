package app

import (
	"strings"

	"mempack/internal/config"
)

func resolveWorkspace(cfg config.Config, workspace string) string {
	ws := strings.TrimSpace(workspace)
	if ws != "" {
		return ws
	}
	ws = strings.TrimSpace(cfg.DefaultWorkspace)
	if ws == "" {
		return "default"
	}
	return ws
}
