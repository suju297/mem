package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// Canonical returns a cleaned, symlink-resolved path when possible.
// It is best-effort: if the full path doesn't exist, it will try to resolve the deepest existing parent.
func Canonical(path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return clean
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return filepath.Clean(resolved)
	}

	// If the full path doesn't exist, resolve symlinks for the deepest existing parent
	// and then join the remaining suffix. This keeps canonicalization stable for
	// paths under macOS symlinks like /tmp -> /private/tmp and /var -> /private/var.
	prefix := clean
	suffix := ""
	for {
		if _, err := os.Lstat(prefix); err == nil {
			if resolvedPrefix, err := filepath.EvalSymlinks(prefix); err == nil {
				if suffix == "" {
					return filepath.Clean(resolvedPrefix)
				}
				return filepath.Clean(filepath.Join(resolvedPrefix, suffix))
			}
			break
		}
		dir := filepath.Dir(prefix)
		if dir == prefix {
			break
		}
		base := filepath.Base(prefix)
		if suffix == "" {
			suffix = base
		} else {
			suffix = filepath.Join(base, suffix)
		}
		prefix = dir
	}

	return clean
}
