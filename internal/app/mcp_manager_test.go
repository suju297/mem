package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"mempack/internal/config"
)

func TestMCPManagerPingAndStatus(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}

	done := startManager(t, cfg, "test-token", 2)

	resp, err := sendManagerCommand(cfg, managerRequest{Cmd: "ping"})
	if err != nil {
		t.Fatalf("ping error: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ping ok, got %+v", resp)
	}

	resp, err = sendManagerCommand(cfg, managerRequest{Cmd: "status"})
	if err != nil {
		t.Fatalf("status error: %v", err)
	}
	if resp.ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d (resp=%+v)", resp.ExitCode, resp)
	}
	if !strings.Contains(strings.ToLower(resp.Message), "not running") {
		t.Fatalf("expected not running message, got %q", resp.Message)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runMCPStatus(&out, &errOut)
	if code != 1 {
		t.Fatalf("expected status code 1, got %d", code)
	}
	if !strings.Contains(strings.ToLower(errOut.String()), "not running") {
		t.Fatalf("expected status message in stderr, got %q", errOut.String())
	}

	waitForManagerExit(t, done, 4*time.Second)
}

func TestMCPManagerUnauthorized(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}

	done := startManager(t, cfg, "good-token", 2)
	info := waitForManagerInfo(t, cfg)

	addr := fmt.Sprintf("127.0.0.1:%d", info.Port)
	resp, err := sendRawManagerRequest(addr, managerRequest{
		Cmd:   "ping",
		Token: "bad-token",
	})
	if err != nil {
		t.Fatalf("unauthorized request error: %v", err)
	}
	if resp.Ok {
		t.Fatalf("expected unauthorized response, got %+v", resp)
	}
	if !strings.Contains(strings.ToLower(resp.Message), "unauthorized") {
		t.Fatalf("expected unauthorized message, got %q", resp.Message)
	}

	waitForManagerExit(t, done, 4*time.Second)
}

func TestMCPManagerServesParallelConnections(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}

	done := startManager(t, cfg, "parallel-token", 2)
	info := waitForManagerInfo(t, cfg)
	addr := fmt.Sprintf("127.0.0.1:%d", info.Port)

	slowConn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("dial slow connection: %v", err)
	}
	defer slowConn.Close()

	// Keep one connection open without sending a request body.
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	resp, err := sendManagerCommand(cfg, managerRequest{Cmd: "ping"})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("ping while slow connection open: %v", err)
	}
	if !resp.Ok {
		t.Fatalf("expected ping ok, got %+v", resp)
	}
	if elapsed > 1200*time.Millisecond {
		t.Fatalf("expected ping to return quickly, took %v", elapsed)
	}

	_ = slowConn.Close()
	waitForManagerExit(t, done, 7*time.Second)
}

func TestMCPManagerIdleShutdown(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}

	done := startManager(t, cfg, "idle-token", 1)
	waitForManagerInfo(t, cfg)
	waitForManagerExit(t, done, 4*time.Second)
}

func TestMCPManagerStatusJSONNotRunning(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runMCPManagerStatus([]string{"--json"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("expected exit code 1, got %d", code)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr, got %q", errOut.String())
	}
	var status managerStatus
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("expected JSON output: %v", err)
	}
	if status.Running {
		t.Fatalf("expected running=false, got %+v", status)
	}
}

func TestMCPManagerStatusJSONRunning(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	done := startManager(t, cfg, "status-token", 2)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runMCPManagerStatus([]string{"--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if errOut.Len() != 0 {
		t.Fatalf("expected no stderr, got %q", errOut.String())
	}
	var status managerStatus
	if err := json.Unmarshal(out.Bytes(), &status); err != nil {
		t.Fatalf("expected JSON output: %v", err)
	}
	if !status.Running || status.PID == 0 || status.Port == 0 {
		t.Fatalf("expected running status with pid/port, got %+v", status)
	}

	waitForManagerExit(t, done, 4*time.Second)
}

func TestMCPManagerStatusTextRunning(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	done := startManager(t, cfg, "text-status-token", 2)

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runMCPManagerStatus(nil, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	text := strings.ToLower(out.String())
	if !strings.Contains(text, "mcp manager running") {
		t.Fatalf("expected running text, got %q", out.String())
	}

	waitForManagerExit(t, done, 4*time.Second)
}

func TestMCPManagerCleansInfoFile(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}

	done := startManager(t, cfg, "cleanup-token", 1)
	infoPath := managerInfoPath(cfg)
	waitForManagerInfo(t, cfg)
	waitForManagerExit(t, done, 4*time.Second)
	waitForManagerInfoRemoved(t, infoPath, 2*time.Second)
}

func TestWriteManagerInfoUsesOwnerOnlyPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions are not portable on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.manager.json")
	err := writeManagerInfo(path, managerInfo{
		Port:  4242,
		Token: "token",
		PID:   7,
	})
	if err != nil {
		t.Fatalf("write manager info: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat manager info: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected permissions 0600, got %04o", got)
	}
}

func TestMCPManagerFallbackOnCorruptInfo(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	if err := os.MkdirAll(cfg.ConfigDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(managerInfoPath(cfg), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write corrupt info: %v", err)
	}

	called := false
	prev := ensureManagerFunc
	ensureManagerFunc = func(cfg config.Config, errOut io.Writer) error {
		called = true
		return errors.New("disabled")
	}
	t.Cleanup(func() {
		ensureManagerFunc = prev
	})

	var out bytes.Buffer
	var errOut bytes.Buffer
	code := runMCPStatus(&out, &errOut)
	if !called {
		t.Fatalf("expected ensureManager to be called")
	}
	if code != 1 {
		t.Fatalf("expected status code 1, got %d", code)
	}
	if !strings.Contains(strings.ToLower(out.String()), "not running") {
		t.Fatalf("expected local fallback output, got %q", out.String())
	}
}

func TestAppendDataDirArg(t *testing.T) {
	got := appendDataDirArg([]string{"--allow-write"}, "/tmp/mempack-data")
	want := []string{"--allow-write", "--data-dir", "/tmp/mempack-data"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected args: want=%v got=%v", want, got)
	}

	already := appendDataDirArg([]string{"--data-dir", "/tmp/custom", "--allow-write"}, "/tmp/mempack-data")
	wantAlready := []string{"--data-dir", "/tmp/custom", "--allow-write"}
	if !reflect.DeepEqual(already, wantAlready) {
		t.Fatalf("unexpected args when already set: want=%v got=%v", wantAlready, already)
	}
}

func startManager(t testing.TB, cfg config.Config, token string, idleSeconds int) <-chan int {
	t.Helper()
	var errOut bytes.Buffer
	done := make(chan int, 1)
	args := []string{"--port", "0", "--token", token, "--idle-seconds", strconv.Itoa(idleSeconds)}
	go func() {
		code := runMCPManager(args, io.Discard, &errOut)
		if errOut.Len() > 0 && code != 0 {
			// Keep stderr for debugging if the test fails.
			fmt.Fprintln(io.Discard, errOut.String())
		}
		done <- code
	}()
	waitForManagerInfo(t, cfg)
	return done
}

func waitForManagerInfo(t testing.TB, cfg config.Config) managerInfo {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		info, err := readManagerInfo(managerInfoPath(cfg))
		if err == nil {
			return info
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("manager info not ready")
	return managerInfo{}
}

func waitForManagerExit(t testing.TB, done <-chan int, timeout time.Duration) {
	t.Helper()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("manager exit code %d", code)
		}
	case <-time.After(timeout):
		t.Fatalf("manager did not exit within %v", timeout)
	}
}

func waitForManagerInfoRemoved(t testing.TB, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("manager info file still present: %s", path)
}

func sendRawManagerRequest(addr string, req managerRequest) (managerResponse, error) {
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return managerResponse{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return managerResponse{}, err
	}
	var resp managerResponse
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return managerResponse{}, err
	}
	return resp, nil
}
