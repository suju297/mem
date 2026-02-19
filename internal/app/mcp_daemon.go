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

	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0o644); err != nil {
		_ = cmd.Process.Kill()
		_ = logFile.Close()
		fmt.Fprintf(errOut, "failed to write pid file: %v\n", err)
		return 1
	}
	_ = logFile.Close()
	_ = cmd.Process.Release()

	fmt.Fprintf(out, "mempack mcp started (pid=%d, log=%s)\n", cmd.Process.Pid, logPath)
	return 0
}

func buildMCPChildArgs(args []string, fallbackDataDir string) ([]string, error) {
	parsedArgs, globals, err := splitGlobalFlags(args)
	if err != nil {
		return nil, err
	}
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
