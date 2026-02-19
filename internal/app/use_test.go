package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUseByRepoName(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := createRepoAt(t, filepath.Join(base, "alpha"))
	withCwd(t, repoDir)
	runCLI(t, "init", "--no-agents")

	other := filepath.Join(base, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}
	withCwd(t, other)

	out := runCLI(t, "use", "alpha")
	var resp UseResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("decode use response: %v", err)
	}
	expectedRoot := repoDir
	gotRoot := resp.GitRoot
	if resolved, err := filepath.EvalSymlinks(repoDir); err == nil {
		expectedRoot = resolved
	}
	if resolved, err := filepath.EvalSymlinks(resp.GitRoot); err == nil {
		gotRoot = resolved
	}
	if expectedRoot != gotRoot {
		t.Fatalf("expected git_root %s, got %s", expectedRoot, gotRoot)
	}
}

func TestUseByRepoNameDisambiguates(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDirA := createRepoAt(t, filepath.Join(base, "group1", "project"))
	withCwd(t, repoDirA)
	runCLI(t, "init", "--no-agents")

	repoDirB := createRepoAt(t, filepath.Join(base, "group2", "project"))
	withCwd(t, repoDirB)
	runCLI(t, "init", "--no-agents")

	other := filepath.Join(base, "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}
	withCwd(t, other)

	errOut := runCLIExpectError(t, "use", "project")
	if !strings.Contains(errOut, "multiple repos named project") {
		t.Fatalf("expected disambiguation error, got: %s", errOut)
	}
}

func runCLIExpectError(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run(args, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected command to fail, got success")
	}
	return errOut.String()
}

func createRepoAt(t testing.TB, repoDir string) string {
	t.Helper()
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.name", "Test")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	writeFile(t, repoDir, "file.txt", "content")
	runGit(t, repoDir, "add", "file.txt")
	runGit(t, repoDir, "commit", "-m", "init")
	return repoDir
}
