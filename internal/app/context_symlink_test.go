package app

import (
	"os"
	"path/filepath"
	"testing"

	"mempack/internal/config"
	"mempack/internal/pathutil"
)

func TestCachedRepoForCwdCanonicalizesSymlinks(t *testing.T) {
	base := t.TempDir()
	realRoot := filepath.Join(base, "real-repo")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatalf("mkdir realRoot: %v", err)
	}
	linkRoot := filepath.Join(base, "link-repo")
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	canonicalRoot := pathutil.Canonical(realRoot)
	cfg := config.Config{
		RepoCache: map[string]string{
			canonicalRoot: "p_test",
		},
	}

	gotRoot, gotID := cachedRepoForCwd(&cfg, linkRoot)
	if gotID != "p_test" {
		t.Fatalf("expected repo id p_test, got %q (root=%q)", gotID, gotRoot)
	}
	if gotRoot != canonicalRoot {
		t.Fatalf("expected root %q, got %q", canonicalRoot, gotRoot)
	}
}
