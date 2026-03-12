package app

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"testing"
)

func TestBuildMCPChildArgsUsesFallbackDataDir(t *testing.T) {
	got, err := buildMCPChildArgs([]string{"--allow-write"}, "/tmp/mem-data")
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	want := []string{"--data-dir", "/tmp/mem-data", "mcp", "--allow-write", "--daemon", "--require-repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected child args:\nwant=%v\ngot=%v", want, got)
	}
}

func TestBuildMCPChildArgsPrefersExplicitDataDir(t *testing.T) {
	got, err := buildMCPChildArgs([]string{"--data-dir", "/tmp/override", "--allow-write"}, "/tmp/mem-data")
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	want := []string{"--data-dir", "/tmp/override", "mcp", "--allow-write", "--daemon", "--require-repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected child args:\nwant=%v\ngot=%v", want, got)
	}
}

func TestBuildMCPChildArgsKeepsExplicitRequireRepoValue(t *testing.T) {
	got, err := buildMCPChildArgs([]string{"--allow-write", "--require-repo=false"}, "/tmp/mem-data")
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	want := []string{"--data-dir", "/tmp/mem-data", "mcp", "--allow-write", "--require-repo=false", "--daemon"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected child args:\nwant=%v\ngot=%v", want, got)
	}
}

func TestBuildMCPChildArgsForcesDaemonWhenDisabled(t *testing.T) {
	got, err := buildMCPChildArgs([]string{"--allow-write", "--daemon=false"}, "/tmp/mem-data")
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	want := []string{"--data-dir", "/tmp/mem-data", "mcp", "--allow-write", "--daemon=false", "--daemon", "--require-repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected child args:\nwant=%v\ngot=%v", want, got)
	}
}

func TestMCPLocalLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("daemon lifecycle integration test is not supported on windows")
	}

	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)
	runCLI(t, "init", "--no-agents")

	bin := buildMemBinary(t)
	withMCPExecPath(t, bin)

	var startOut bytes.Buffer
	var startErr bytes.Buffer
	if code := runMCPStartLocal([]string{"--repo", repoDir}, &startOut, &startErr); code != 0 {
		t.Fatalf("start failed (%d): %s", code, startErr.String())
	}
	t.Cleanup(func() {
		var out bytes.Buffer
		var errOut bytes.Buffer
		_ = runMCPStopLocal(&out, &errOut)
	})

	pid, ok := parsePID(t, startOut.String())
	if !ok || pid < 0 {
		t.Fatalf("expected non-negative pid in start output, got %q", startOut.String())
	}

	var statusOut bytes.Buffer
	var statusErr bytes.Buffer
	if code := runMCPStatusLocal(&statusOut, &statusErr); code != 0 {
		t.Fatalf("status failed (%d): %s", code, statusErr.String())
	}
	if !bytes.Contains(statusOut.Bytes(), []byte(fmt.Sprintf("pid=%d", pid))) {
		t.Fatalf("expected status to report pid %d, got %q", pid, statusOut.String())
	}

	var stopOut bytes.Buffer
	var stopErr bytes.Buffer
	if code := runMCPStopLocal(&stopOut, &stopErr); code != 0 {
		t.Fatalf("stop failed (%d): %s", code, stopErr.String())
	}
	if !bytes.Contains(stopOut.Bytes(), []byte(fmt.Sprintf("pid=%d", pid))) {
		t.Fatalf("expected stop to report pid %d, got %q", pid, stopOut.String())
	}

	statusOut.Reset()
	statusErr.Reset()
	if code := runMCPStatusLocal(&statusOut, &statusErr); code != 1 {
		t.Fatalf("expected status after stop to fail, got %d (%s)", code, statusErr.String())
	}
	if !bytes.Contains(statusOut.Bytes(), []byte("mem mcp not running")) {
		t.Fatalf("expected not running after stop, got %q", statusOut.String())
	}
}

func TestMCPStartLocalFailsWhenChildExitsImmediately(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	bin := buildFailingBinary(t)
	withMCPExecPath(t, bin)

	var startOut bytes.Buffer
	var startErr bytes.Buffer
	if code := runMCPStartLocal([]string{"--repo", repoDir}, &startOut, &startErr); code == 0 {
		t.Fatalf("expected start to fail, got success: %q", startOut.String())
	}
	if !bytes.Contains(startErr.Bytes(), []byte("failed to start mcp daemon")) {
		t.Fatalf("expected daemon startup failure, got %q", startErr.String())
	}

	var statusOut bytes.Buffer
	var statusErr bytes.Buffer
	if code := runMCPStatusLocal(&statusOut, &statusErr); code != 1 {
		t.Fatalf("expected status to report not running, got %d (%s)", code, statusErr.String())
	}
	if !bytes.Contains(statusOut.Bytes(), []byte("mem mcp not running")) {
		t.Fatalf("expected not running after failed start, got %q", statusOut.String())
	}
}

func buildMemBinary(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime caller unavailable")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	bin := filepath.Join(t.TempDir(), "mem")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", bin, "./cmd/mem")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build mem binary: %v\n%s", err, output)
	}
	return bin
}

func buildFailingBinary(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	source := filepath.Join(workDir, "main.go")
	if err := os.WriteFile(source, []byte("package main\nimport \"os\"\nfunc main(){ os.Exit(1) }\n"), 0o644); err != nil {
		t.Fatalf("write failing binary source: %v", err)
	}
	bin := filepath.Join(workDir, "fail")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failing binary: %v\n%s", err, output)
	}
	return bin
}

func withMCPExecPath(t *testing.T, bin string) {
	t.Helper()
	orig := mcpExecPath
	mcpExecPath = func() (string, error) {
		return bin, nil
	}
	t.Cleanup(func() {
		mcpExecPath = orig
	})
}

func parsePID(t *testing.T, text string) (int, bool) {
	t.Helper()
	match := regexp.MustCompile(`pid=(\d+)`).FindStringSubmatch(text)
	if len(match) != 2 {
		return 0, false
	}
	pid, err := strconv.Atoi(match[1])
	if err != nil {
		t.Fatalf("parse pid: %v", err)
	}
	return pid, true
}
