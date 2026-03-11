package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	mcpReadyTimeout      = 3 * time.Second
	mcpReadyStableFor    = 500 * time.Millisecond
	mcpReadyPollInterval = 50 * time.Millisecond
	mcpStopTimeout       = 3 * time.Second
)

var mcpExecPath = func() (string, error) {
	return exec.LookPath(os.Args[0])
}

func runMCPStartLocal(args []string, out, errOut io.Writer) int {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(cfg.ConfigDir, 0o755); err != nil {
		fmt.Fprintf(errOut, "config dir error: %v\n", err)
		return 1
	}

	pidPath := filepath.Join(cfg.ConfigDir, "mcp.pid")
	logPath := filepath.Join(cfg.ConfigDir, "mcp.log")

	lockFile, err := lockPIDFile(pidPath)
	if err != nil {
		fmt.Fprintf(errOut, "failed to lock pid file: %v\n", err)
		return 1
	}
	defer unlockPIDFile(lockFile)

	if pid, running := readPID(pidPath); running {
		fmt.Fprintf(out, "mem mcp already running (pid=%d)\n", pid)
		return 0
	}

	bin, err := mcpExecPath()
	if err != nil {
		fmt.Fprintf(errOut, "failed to find mem binary: %v\n", err)
		return 1
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_RDONLY, 0)
	if err != nil {
		fmt.Fprintf(errOut, "failed to open %s: %v\n", os.DevNull, err)
		return 1
	}
	defer devNull.Close()

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(errOut, "failed to open log file: %v\n", err)
		return 1
	}

	fallbackDataDir := filepath.Dir(cfg.RepoRootDir())
	childArgs, err := buildMCPChildArgs(args, fallbackDataDir)
	if err != nil {
		_ = logFile.Close()
		fmt.Fprintf(errOut, "invalid mcp start args: %v\n", err)
		return 2
	}
	cmd := exec.Command(bin, childArgs...)
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		fmt.Fprintf(errOut, "failed to start mcp: %v\n", err)
		return 1
	}
	pid := cmd.Process.Pid

	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", pid)), 0o644); err != nil {
		_ = cmd.Process.Kill()
		_ = logFile.Close()
		fmt.Fprintf(errOut, "failed to write pid file: %v\n", err)
		return 1
	}
	_ = logFile.Close()
	if !waitForMCPReady(pidPath, pid, mcpReadyTimeout) {
		_ = os.Remove(pidPath)
		_ = cmd.Process.Kill()
		_ = cmd.Process.Release()
		fmt.Fprintf(errOut, "failed to start mcp daemon; see log: %s\n", logPath)
		return 1
	}
	_ = cmd.Process.Release()

	fmt.Fprintf(out, "mem mcp started (pid=%d, log=%s)\n", pid, logPath)
	return 0
}

func buildMCPChildArgs(args []string, fallbackDataDir string) ([]string, error) {
	parsedArgs, globals, err := splitGlobalFlags(args)
	if err != nil {
		return nil, err
	}
	parsedArgs = applyMCPStartDefaults(parsedArgs)
	dataDir := strings.TrimSpace(globals.DataDir)
	if dataDir == "" {
		dataDir = strings.TrimSpace(fallbackDataDir)
	}
	childArgs := make([]string, 0, len(parsedArgs)+3)
	if dataDir != "" {
		childArgs = append(childArgs, "--data-dir", dataDir)
	}
	childArgs = append(childArgs, "mcp")
	childArgs = append(childArgs, parsedArgs...)
	return childArgs, nil
}

func applyMCPStartDefaults(args []string) []string {
	out := append([]string{}, args...)
	if !hasMCPDaemonFlag(args) {
		out = append(out, "--daemon")
	}
	if !hasMCPRequireRepoFlag(args) {
		out = append(out, "--require-repo")
	}
	return out
}

func hasMCPDaemonFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--daemon" || arg == "--daemon=true" {
			return true
		}
	}
	return false
}

func hasMCPRequireRepoFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--require-repo" || strings.HasPrefix(arg, "--require-repo=") {
			return true
		}
	}
	return false
}

func runMCPStopLocal(out, errOut io.Writer) int {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	pidPath := filepath.Join(cfg.ConfigDir, "mcp.pid")

	lockFile, err := lockPIDFile(pidPath)
	if err != nil {
		fmt.Fprintf(errOut, "failed to lock pid file: %v\n", err)
		return 1
	}
	defer unlockPIDFile(lockFile)

	pid, running := readPID(pidPath)
	if pid == 0 || !running {
		fmt.Fprintln(out, "mem mcp not running")
		_ = os.Remove(pidPath)
		return 0
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(errOut, "failed to find process: %v\n", err)
		return 1
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if isMCPProcessRunning(pid) {
			fmt.Fprintf(errOut, "failed to stop mcp: %v\n", err)
			return 1
		}
	}
	if !waitForProcessExit(pid, mcpStopTimeout) {
		fmt.Fprintf(errOut, "timed out stopping mcp (pid=%d)\n", pid)
		return 1
	}
	_ = os.Remove(pidPath)
	fmt.Fprintf(out, "mem mcp stopped (pid=%d)\n", pid)
	return 0
}

func runMCPStatusLocal(out, errOut io.Writer) int {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	pidPath := filepath.Join(cfg.ConfigDir, "mcp.pid")
	lockFile, err := lockPIDFileShared(pidPath)
	if err != nil {
		fmt.Fprintf(errOut, "failed to lock pid file: %v\n", err)
		return 1
	}
	defer unlockPIDFile(lockFile)

	pid, running := readPID(pidPath)
	if pid == 0 || !running {
		fmt.Fprintln(out, "mem mcp not running")
		return 1
	}
	fmt.Fprintf(out, "mem mcp running (pid=%d)\n", pid)
	return 0
}

func readPID(path string) (int, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		return 0, false
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, isMCPProcessRunning(pid)
}

func isMCPProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	if processIsZombie(pid) {
		return false
	}
	return looksLikeMCPProcess(pid)
}

func waitForMCPReady(pidPath string, pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	var readySince time.Time
	for {
		currentPID, running := readPID(pidPath)
		now := time.Now()
		if running && currentPID == pid {
			if readySince.IsZero() {
				readySince = now
			}
			if now.Sub(readySince) >= mcpReadyStableFor {
				return true
			}
		} else {
			readySince = time.Time{}
		}
		if !now.Before(deadline) {
			return false
		}
		time.Sleep(mcpReadyPollInterval)
	}
}

func waitForProcessExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !isMCPProcessRunning(pid) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(mcpReadyPollInterval)
	}
}

func looksLikeMCPProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Best-effort stale PID detection. On unsupported systems/tools, keep legacy behavior.
	if runtime.GOOS == "windows" {
		return true
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "command=").Output()
	if err != nil {
		return true
	}
	cmdline := strings.ToLower(strings.TrimSpace(string(out)))
	if cmdline == "" {
		return false
	}
	bin := strings.ToLower(filepath.Base(os.Args[0]))
	if bin != "" && strings.Contains(cmdline, bin) && strings.Contains(cmdline, " mcp") && strings.Contains(cmdline, "--daemon") {
		return true
	}
	return strings.Contains(cmdline, " mcp ") && strings.Contains(cmdline, "--daemon")
}

func processIsZombie(pid int) bool {
	if pid <= 0 || runtime.GOOS == "windows" {
		return false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "stat=").Output()
	if err != nil {
		return false
	}
	status := strings.TrimSpace(string(out))
	return status != "" && status[0] == 'Z'
}

func lockPIDFile(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := flockExclusive(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func lockPIDFileShared(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	if err := flockShared(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func unlockPIDFile(file *os.File) {
	if file == nil {
		return
	}
	_ = flockUnlock(file)
	_ = file.Close()
}
