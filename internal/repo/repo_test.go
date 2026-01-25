package repo

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	// Setup temp dir
	tmpDir, err := os.MkdirTemp("", "mempack-test-repo-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Case 1: Not a git repo
	info, err := Detect(tmpDir)
	if err != nil {
		t.Errorf("Detect non-git failed: %v", err)
	}
	if info.HasGit {
		t.Error("Expected HasGit=false for empty dir")
	}

	// Case 2: Git repo
	setupGit(t, tmpDir)
	info, err = Detect(tmpDir)
	if err != nil {
		t.Errorf("Detect git failed: %v", err)
	}
	if !info.HasGit {
		t.Error("Expected HasGit=true for git dir")
	}

	// Edge Case: Subdirectory detection
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	subInfo, err := Detect(subDir)
	if err != nil {
		t.Errorf("Detect subdir failed: %v", err)
	}
	if subInfo.GitRoot != info.GitRoot {
		t.Errorf("Expected GitRoot %s, got %s", info.GitRoot, subInfo.GitRoot)
	}

	// Edge Case: Detached HEAD (no branch)
	runGit(t, tmpDir, "checkout", "--detach", "HEAD")
	dInfo, err := Detect(tmpDir)
	if err != nil {
		t.Errorf("Detect detached failed: %v", err)
	}
	if dInfo.Branch != "HEAD" && dInfo.Branch != "" {
		// Valid implementations might return "HEAD" or empty string when detached, depending on git version/logic
		// Our logic uses --abbrev-ref HEAD. git says "HEAD" if detached.
		if dInfo.Branch != "HEAD" {
			t.Errorf("Expected branch HEAD for detached, got %s", dInfo.Branch)
		}
	} else {
		// If it succeeded, check head matches
		if dInfo.Head == "" {
			t.Error("Expected valid HEAD for detached state")
		}
	}
	runGit(t, tmpDir, "checkout", "main") // restore
}

func TestIsAncestor(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mempack-test-ancestry-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	setupGit(t, tmpDir)

	// Create commit A
	commitA := getHead(t, tmpDir)

	// Create commit B
	makeCommit(t, tmpDir, "file2.txt", "content2")
	commitB := getHead(t, tmpDir)

	// B is ancestor of B? Yes
	is, err := IsAncestor(tmpDir, commitB, commitB)
	if err != nil {
		t.Fatalf("IsAncestor error: %v", err)
	}
	if !is {
		t.Error("Expected commit to be ancestor of itself")
	}

	// A is ancestor of B? Yes
	is, err = IsAncestor(tmpDir, commitA, commitB)
	if err != nil {
		t.Fatalf("IsAncestor error: %v", err)
	}
	if !is {
		t.Error("Expected A to be ancestor of B")
	}

	// B is ancestor of A? No
	is, err = IsAncestor(tmpDir, commitB, commitA)
	if err != nil {
		t.Fatalf("IsAncestor error: %v", err)
	}
	if is {
		t.Error("Expected B NOT to be ancestor of A")
	}

	// Orphan commit (unrelated history)
	// Hard to simulate unrelated history in same repo quickly without checkout --orphan
	// Let's rely on detection logic.
}

func TestInfoFromCache(t *testing.T) {
	// Pure logic test
	info, err := InfoFromCache("id1", "/root", "sha1", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "id1" || info.Head != "sha1" || !info.HasGit {
		t.Error("Cache info mismatch")
	}

	// Test fallback for needsFreshHead=true with invalid root (should fallback)
	info, err = InfoFromCache("id2", "/invalid", "sha2", "dev", true)
	if err != nil {
		t.Fatal(err)
	}
	if info.Head != "sha2" {
		t.Errorf("Expected fallback to cached head 'sha2', got '%s'", info.Head)
	}
}

// Helpers

func setupGit(t *testing.T, dir string) {
	runGit(t, dir, "init", "-q", "--initial-branch=main")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "config", "user.email", "test@example.com")
	makeCommit(t, dir, "README.md", "init")
}

func makeCommit(t *testing.T, dir, file, content string) {
	path := filepath.Join(dir, file)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", file)
	runGit(t, dir, "commit", "-m", "add "+file, "-q")
}

func getHead(t *testing.T, dir string) string {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse failed: %v", err)
	}
	return string(out[:len(out)-1]) // trim newline
}

func runGit(t *testing.T, dir string, args ...string) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v failed: %v", args, err)
	}
}
