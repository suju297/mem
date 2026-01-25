package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mempack/internal/repo"
	"mempack/internal/store"
)

func TestReachabilityFilter(t *testing.T) {
	repoDir := t.TempDir()
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.name", "Test")
	runGit(t, repoDir, "config", "user.email", "test@example.com")

	writeFile(t, repoDir, "file.txt", "first")
	runGit(t, repoDir, "add", "file.txt")
	runGit(t, repoDir, "commit", "-m", "commit A")
	shaA := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "HEAD"))

	writeFile(t, repoDir, "file.txt", "second")
	runGit(t, repoDir, "add", "file.txt")
	runGit(t, repoDir, "commit", "-m", "commit B")
	shaB := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "HEAD"))

	runGit(t, repoDir, "checkout", shaA)

	repoInfo, err := repo.Detect(repoDir)
	if err != nil {
		t.Fatalf("detect repo: %v", err)
	}

	results := []store.MemoryResult{
		{Memory: store.Memory{ID: "M-A", AnchorCommit: shaA, CreatedAt: time.Unix(10, 0)}, BM25: 1},
		{Memory: store.Memory{ID: "M-B", AnchorCommit: shaB, CreatedAt: time.Unix(11, 0)}, BM25: 1},
	}

	ranked, _, _, _, err := rankMemories("query", results, nil, repoInfo, RankOptions{IncludeOrphans: false})
	if err != nil {
		t.Fatalf("rank memories: %v", err)
	}
	if len(ranked) != 1 {
		t.Fatalf("expected 1 memory after reachability filter, got %d", len(ranked))
	}
	if ranked[0].Memory.AnchorCommit != shaA {
		t.Fatalf("expected anchor %s, got %s", shaA, ranked[0].Memory.AnchorCommit)
	}
}

func runGit(t testing.TB, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, output)
	}
}

func runGitOutput(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v (%s)", args, err, output)
	}
	return string(output)
}

func writeFile(t testing.TB, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
