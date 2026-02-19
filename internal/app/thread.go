package app

import (
	"fmt"
	"strings"

	"mempack/internal/config"
)

func resolveThread(cfg config.Config, thread string) (string, bool, error) {
	thread = strings.TrimSpace(thread)
	if thread != "" {
		return thread, false, nil
	}
	defaultThread := strings.TrimSpace(cfg.DefaultThread)
	if defaultThread == "" {
		return "", false, fmt.Errorf("missing thread (pass --thread or set default_thread in config.toml or .mempack/config.json)")
	}
	return defaultThread, true, nil
}
