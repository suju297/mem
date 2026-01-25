package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mempack/internal/repo"
)

type addResp struct {
	ID           string `json:"id"`
	ThreadID     string `json:"thread_id"`
	Title        string `json:"title"`
	AnchorCommit string `json:"anchor_commit"`
}

type showResp struct {
	Kind   string       `json:"kind"`
	Memory memoryDetail `json:"memory"`
}

type memoryDetail struct {
	ID           string `json:"id"`
	ThreadID     string `json:"thread_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	SupersededBy string `json:"superseded_by"`
	DeletedAt    string `json:"deleted_at"`
}

type forgetResp struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type supersedeResp struct {
	OldID  string `json:"old_id"`
	NewID  string `json:"new_id"`
	Status string `json:"status"`
}

type checkpointResp struct {
	StateID  string `json:"state_id"`
	MemoryID string `json:"memory_id"`
}

func TestCLIShowForgetSupersedeCheckpoint(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	addOut := runCLI(t, "add", "--thread", "T1", "--title", "First", "--summary", "Initial decision")
	var add addResp
	if err := json.Unmarshal(addOut, &add); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if add.ID == "" {
		t.Fatalf("expected add id to be set")
	}

	showOut := runCLI(t, "show", add.ID)
	var show showResp
	if err := json.Unmarshal(showOut, &show); err != nil {
		t.Fatalf("decode show response: %v", err)
	}
	if show.Kind != "memory" {
		t.Fatalf("expected kind memory, got %s", show.Kind)
	}
	if show.Memory.Title != "First" {
		t.Fatalf("expected title First, got %s", show.Memory.Title)
	}

	supOut := runCLI(t, "supersede", "--title", "Second", "--summary", "Updated decision", add.ID)
	var sup supersedeResp
	if err := json.Unmarshal(supOut, &sup); err != nil {
		t.Fatalf("decode supersede response: %v", err)
	}
	if sup.NewID == "" {
		t.Fatalf("expected new id to be set")
	}

	oldShowOut := runCLI(t, "show", sup.OldID)
	var oldShow showResp
	if err := json.Unmarshal(oldShowOut, &oldShow); err != nil {
		t.Fatalf("decode old show: %v", err)
	}
	if oldShow.Memory.SupersededBy != sup.NewID {
		t.Fatalf("expected superseded_by %s, got %s", sup.NewID, oldShow.Memory.SupersededBy)
	}

	newShowOut := runCLI(t, "show", sup.NewID)
	var newShow showResp
	if err := json.Unmarshal(newShowOut, &newShow); err != nil {
		t.Fatalf("decode new show: %v", err)
	}
	if newShow.Memory.Title != "Second" {
		t.Fatalf("expected new title Second, got %s", newShow.Memory.Title)
	}

	forgetOut := runCLI(t, "forget", sup.NewID)
	var forget forgetResp
	if err := json.Unmarshal(forgetOut, &forget); err != nil {
		t.Fatalf("decode forget response: %v", err)
	}
	if forget.Status != "forgotten" {
		t.Fatalf("expected forgotten status, got %s", forget.Status)
	}

	forgottenShow := runCLI(t, "show", sup.NewID)
	var forgotten showResp
	if err := json.Unmarshal(forgottenShow, &forgotten); err != nil {
		t.Fatalf("decode forgotten show: %v", err)
	}
	if forgotten.Memory.DeletedAt == "" {
		t.Fatalf("expected deleted_at to be set")
	}

	ckOut := runCLI(t, "checkpoint", "--reason", "Snapshot", "--state-json", `{"goal":"ship"}`, "--thread", "T1")
	var ck checkpointResp
	if err := json.Unmarshal(ckOut, &ck); err != nil {
		t.Fatalf("decode checkpoint response: %v", err)
	}
	if ck.StateID == "" || ck.MemoryID == "" {
		t.Fatalf("expected state_id and memory_id to be set")
	}

	ckShow := runCLI(t, "show", ck.MemoryID)
	var ckMem showResp
	if err := json.Unmarshal(ckShow, &ckMem); err != nil {
		t.Fatalf("decode checkpoint memory: %v", err)
	}
	if ckMem.Memory.Summary != "Snapshot" {
		t.Fatalf("expected checkpoint summary Snapshot, got %s", ckMem.Memory.Summary)
	}
}

func runCLI(t *testing.T, args ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run(args, &out, &errOut)
	if code != 0 {
		t.Fatalf("command failed (%d): %s", code, errOut.String())
	}
	return bytes.TrimSpace(out.Bytes())
}

func setupRepo(t testing.TB, base string) string {
	t.Helper()
	repoDir := filepath.Join(base, "repo")
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

func setXDGEnv(t testing.TB, base string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
}

func withCwd(t testing.TB, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

func TestCLIReposAndUse(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	info, err := repo.Detect(repoDir)
	if err != nil {
		t.Fatalf("detect repo: %v", err)
	}

	_ = runCLI(t, "add", "--thread", "T1", "--title", "Seed", "--summary", "Seed")

	useOut := runCLI(t, "use", info.ID)
	if len(useOut) == 0 {
		t.Fatalf("expected use output")
	}

	reposOut := runCLI(t, "repos")
	if len(reposOut) == 0 {
		t.Fatalf("expected repos output")
	}
}

func TestCLIThreadFlagsAfterID(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "add", "--thread", "T1", "--title", "Seed", "--summary", "Seed")

	out := runCLI(t, "thread", "T1", "--limit", "20")
	var resp ThreadShowResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("decode thread response: %v", err)
	}
	if resp.Thread.ThreadID != "T1" {
		t.Fatalf("expected thread T1, got %s", resp.Thread.ThreadID)
	}
	if resp.Thread.MemoryCount != 1 {
		t.Fatalf("expected memory_count 1, got %d", resp.Thread.MemoryCount)
	}
}

func TestCLIThreadMemoryCountMatchesTotal(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "add", "--thread", "T1", "--title", "First", "--summary", "First summary")
	_ = runCLI(t, "add", "--thread", "T1", "--title", "Second", "--summary", "Second summary")

	out := runCLI(t, "thread", "T1", "--limit", "1")
	var resp ThreadShowResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("decode thread response: %v", err)
	}
	if len(resp.Memories) != 1 {
		t.Fatalf("expected 1 memory in response, got %d", len(resp.Memories))
	}
	if resp.Thread.MemoryCount != 2 {
		t.Fatalf("expected memory_count 2, got %d", resp.Thread.MemoryCount)
	}
	if resp.Thread.MemoryCount < len(resp.Memories) {
		t.Fatalf("expected memory_count >= memories length, got %d < %d", resp.Thread.MemoryCount, len(resp.Memories))
	}
}

func TestGetDeterministicOutput(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "add", "--thread", "T1", "--title", "First", "--summary", "Initial decision")

	first := runCLI(t, "get", "decision")
	second := runCLI(t, "get", "decision")
	if !bytes.Equal(first, second) {
		t.Fatalf("expected byte-identical get output")
	}
}
