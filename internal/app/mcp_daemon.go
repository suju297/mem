package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

func runMCPStart(args []string, out, errOut io.Writer) int {
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

	if pid, running := readPID(pidPath); running {
		fmt.Fprintf(out, "mempack mcp already running (pid=%d)\n", pid)
		return 0
	}

	bin, err := exec.LookPath(os.Args[0])
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

	childArgs := append([]string{"mcp"}, args...)
	cmd := exec.Command(bin, childArgs...)
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		fmt.Fprintf(errOut, "failed to start mcp: %v\n", err)
		return 1
	}

	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o644); err != nil {
		_ = cmd.Process.Kill()
		_ = logFile.Close()
		fmt.Fprintf(errOut, "failed to write pid file: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "mempack mcp started (pid=%d, log=%s)\n", cmd.Process.Pid, logPath)
	return 0
}

func runMCPStop(out, errOut io.Writer) int {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	pidPath := filepath.Join(cfg.ConfigDir, "mcp.pid")

	pid, running := readPID(pidPath)
	if pid == 0 || !running {
		fmt.Fprintln(out, "mempack mcp not running")
		_ = os.Remove(pidPath)
		return 0
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		fmt.Fprintf(errOut, "failed to find process: %v\n", err)
		return 1
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		fmt.Fprintf(errOut, "failed to stop mcp: %v\n", err)
		return 1
	}
	_ = os.Remove(pidPath)
	fmt.Fprintf(out, "mempack mcp stopped (pid=%d)\n", pid)
	return 0
}

func runMCPStatus(out, errOut io.Writer) int {
	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	pidPath := filepath.Join(cfg.ConfigDir, "mcp.pid")
	pid, running := readPID(pidPath)
	if pid == 0 || !running {
		fmt.Fprintln(out, "mempack mcp not running")
		return 1
	}
	fmt.Fprintf(out, "mempack mcp running (pid=%d)\n", pid)
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
	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return pid, false
	}
	return pid, true
}
