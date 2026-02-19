package app

import (
	"bytes"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"mempack/internal/config"
)

const (
	managerInfoFile  = "mcp.manager.json"
	managerIdleSecs  = 300
	managerDialDelay = 100 * time.Millisecond
)

type managerInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}

type managerRequest struct {
	Cmd   string   `json:"cmd"`
	Token string   `json:"token"`
	Args  []string `json:"args,omitempty"`
}

type managerResponse struct {
	Ok       bool   `json:"ok"`
	Message  string `json:"message,omitempty"`
	ExitCode int    `json:"exit_code"`
}

func runMCPStart(args []string, out, errOut io.Writer) int {
	return runMCPManaged("start", args, out, errOut)
}

func runMCPStop(out, errOut io.Writer) int {
	return runMCPManaged("stop", nil, out, errOut)
}

func runMCPStatus(out, errOut io.Writer) int {
	return runMCPManaged("status", nil, out, errOut)
}

func runMCPManaged(cmd string, args []string, out, errOut io.Writer) int {
	if code, ok := runMCPViaManager(cmd, args, out, errOut); ok {
		return code
	}
	switch cmd {
	case "start":
		return runMCPStartLocal(args, out, errOut)
	case "stop":
		return runMCPStopLocal(out, errOut)
	case "status":
		return runMCPStatusLocal(out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown mcp command: %s\n", cmd)
		return 2
	}
}

var ensureManagerFunc = ensureManager

func runMCPViaManager(cmd string, args []string, out, errOut io.Writer) (int, bool) {
	cfg, err := loadConfig()
	if err != nil {
		return 0, false
	}
	if err := os.MkdirAll(cfg.ConfigDir, 0o755); err != nil {
		return 0, false
	}

	cmdArgs := append([]string{}, args...)
	if strings.EqualFold(strings.TrimSpace(cmd), "start") {
		dataDir := filepath.Dir(cfg.RepoRootDir())
		cmdArgs = appendDataDirArg(cmdArgs, dataDir)
	}

	resp, err := sendManagerCommand(cfg, managerRequest{Cmd: cmd, Args: cmdArgs})
	if err != nil {
		if ensureManagerFunc(cfg, errOut) != nil {
			return 0, false
		}
		resp, err = sendManagerCommand(cfg, managerRequest{Cmd: cmd, Args: cmdArgs})
		if err != nil {
			return 0, false
		}
	}

	if resp.Message != "" {
		if resp.Ok {
			fmt.Fprintln(out, resp.Message)
		} else {
			fmt.Fprintln(errOut, resp.Message)
		}
	}
	return resp.ExitCode, true
}

func appendDataDirArg(args []string, dataDir string) []string {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		return append([]string{}, args...)
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--data-dir" || strings.HasPrefix(arg, "--data-dir=") {
			return append([]string{}, args...)
		}
	}
	out := append([]string{}, args...)
	out = append(out, "--data-dir", dataDir)
	return out
}

func runMCPManager(args []string, _, errOut io.Writer) int {
	fs := flag.NewFlagSet("mcp-manager", flag.ContinueOnError)
	fs.SetOutput(errOut)
	port := fs.Int("port", 0, "Port to listen on (default: random)")
	tokenFlag := fs.String("token", "", "Auth token")
	idleSeconds := fs.Int("idle-seconds", managerIdleSecs, "Exit after idle seconds")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(cfg.ConfigDir, 0o755); err != nil {
		fmt.Fprintf(errOut, "config dir error: %v\n", err)
		return 1
	}

	token := strings.TrimSpace(*tokenFlag)
	if token == "" {
		token, err = generateToken()
		if err != nil {
			fmt.Fprintf(errOut, "token error: %v\n", err)
			return 1
		}
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(errOut, "manager listen error: %v\n", err)
		return 1
	}
	defer listener.Close()

	actualPort := listener.Addr().(*net.TCPAddr).Port
	info := managerInfo{
		Port:  actualPort,
		Token: token,
		PID:   os.Getpid(),
	}
	infoPath := managerInfoPath(cfg)
	if err := writeManagerInfo(infoPath, info); err != nil {
		fmt.Fprintf(errOut, "manager info error: %v\n", err)
		return 1
	}
	defer os.Remove(infoPath)

	idleFor := time.Duration(*idleSeconds) * time.Second
	if idleFor <= 0 {
		idleFor = time.Duration(managerIdleSecs) * time.Second
	}

	var lastActivityAt atomic.Int64
	lastActivityAt.Store(time.Now().UnixNano())
	var activeConns atomic.Int32
	for {
		if tcp, ok := listener.(*net.TCPListener); ok {
			_ = tcp.SetDeadline(time.Now().Add(1 * time.Second))
		}
		conn, err := listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				lastActivity := time.Unix(0, lastActivityAt.Load())
				if activeConns.Load() == 0 && time.Since(lastActivity) > idleFor {
					return 0
				}
				continue
			}
			fmt.Fprintf(errOut, "manager accept error: %v\n", err)
			return 1
		}
		lastActivityAt.Store(time.Now().UnixNano())
		activeConns.Add(1)
		go func(c net.Conn) {
			defer activeConns.Add(-1)
			defer lastActivityAt.Store(time.Now().UnixNano())
			handleManagerConn(c, token)
		}(conn)
	}
}

func runMCPManagerStatus(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("mcp-manager-status", flag.ContinueOnError)
	fs.SetOutput(errOut)
	jsonOut := fs.Bool("json", false, "Output JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}

	info, err := readManagerInfo(managerInfoPath(cfg))
	if err != nil {
		if *jsonOut {
			writeManagerStatusJSON(out, managerStatus{Running: false})
		} else {
			fmt.Fprintln(out, "mcp manager not running")
		}
		return 1
	}

	if ok := pingManager(cfg); !ok {
		_ = os.Remove(managerInfoPath(cfg))
		if *jsonOut {
			writeManagerStatusJSON(out, managerStatus{Running: false})
		} else {
			fmt.Fprintln(out, "mcp manager not running")
		}
		return 1
	}

	status := managerStatus{
		Running: true,
		PID:     info.PID,
		Port:    info.Port,
	}
	if *jsonOut {
		writeManagerStatusJSON(out, status)
	} else {
		fmt.Fprintf(out, "mcp manager running (pid=%d port=%d)\n", info.PID, info.Port)
	}
	return 0
}

type managerStatus struct {
	Running bool `json:"running"`
	PID     int  `json:"pid,omitempty"`
	Port    int  `json:"port,omitempty"`
}

func writeManagerStatusJSON(out io.Writer, status managerStatus) {
	enc := json.NewEncoder(out)
	_ = enc.Encode(status)
}

func handleManagerConn(conn net.Conn, token string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	var req managerRequest
	dec := json.NewDecoder(io.LimitReader(conn, 64*1024))
	if err := dec.Decode(&req); err != nil {
		writeManagerResponse(conn, managerResponse{
			Ok:       false,
			Message:  fmt.Sprintf("invalid request: %v", err),
			ExitCode: 2,
		})
		return
	}
	if strings.TrimSpace(req.Token) == "" || !tokensEqual(req.Token, token) {
		writeManagerResponse(conn, managerResponse{
			Ok:       false,
			Message:  "unauthorized",
			ExitCode: 2,
		})
		return
	}

	switch strings.ToLower(strings.TrimSpace(req.Cmd)) {
	case "ping":
		writeManagerResponse(conn, managerResponse{Ok: true, Message: "ok", ExitCode: 0})
		return
	case "start":
		writeManagerResponse(conn, captureCommand(func(out, errOut io.Writer) int {
			return runMCPStartLocal(req.Args, out, errOut)
		}))
		return
	case "stop":
		writeManagerResponse(conn, captureCommand(func(out, errOut io.Writer) int {
			return runMCPStopLocal(out, errOut)
		}))
		return
	case "status":
		writeManagerResponse(conn, captureCommand(func(out, errOut io.Writer) int {
			return runMCPStatusLocal(out, errOut)
		}))
		return
	default:
		writeManagerResponse(conn, managerResponse{
			Ok:       false,
			Message:  "unsupported command",
			ExitCode: 2,
		})
		return
	}
}

func captureCommand(run func(io.Writer, io.Writer) int) managerResponse {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(&stdout, &stderr)
	message := strings.TrimSpace(stdout.String())
	if message == "" {
		message = strings.TrimSpace(stderr.String())
	}
	return managerResponse{
		Ok:       code == 0,
		Message:  message,
		ExitCode: code,
	}
}

func writeManagerResponse(conn net.Conn, resp managerResponse) {
	enc := json.NewEncoder(conn)
	_ = enc.Encode(resp)
}

func ensureManager(cfg config.Config, errOut io.Writer) error {
	if ok := pingManager(cfg); ok {
		return nil
	}
	_ = os.Remove(managerInfoPath(cfg))

	bin, err := exec.LookPath(os.Args[0])
	if err != nil {
		return err
	}

	cmd := exec.Command(bin, "mcp", "manager")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ok := pingManager(cfg); ok {
			return nil
		}
		time.Sleep(managerDialDelay)
	}
	return errors.New("mcp manager not ready")
}

func sendManagerCommand(cfg config.Config, req managerRequest) (managerResponse, error) {
	info, err := readManagerInfo(managerInfoPath(cfg))
	if err != nil {
		return managerResponse{}, err
	}
	req.Token = info.Token

	addr := fmt.Sprintf("127.0.0.1:%d", info.Port)
	conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		_ = os.Remove(managerInfoPath(cfg))
		return managerResponse{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))

	enc := json.NewEncoder(conn)
	if err := enc.Encode(req); err != nil {
		return managerResponse{}, err
	}
	var resp managerResponse
	dec := json.NewDecoder(io.LimitReader(conn, 64*1024))
	if err := dec.Decode(&resp); err != nil {
		return managerResponse{}, err
	}
	return resp, nil
}

func pingManager(cfg config.Config) bool {
	resp, err := sendManagerCommand(cfg, managerRequest{Cmd: "ping"})
	return err == nil && resp.Ok
}

func managerInfoPath(cfg config.Config) string {
	return filepath.Join(cfg.ConfigDir, managerInfoFile)
}

func readManagerInfo(path string) (managerInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return managerInfo{}, err
	}
	var info managerInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return managerInfo{}, err
	}
	if info.Port <= 0 || strings.TrimSpace(info.Token) == "" {
		return managerInfo{}, errors.New("invalid manager info")
	}
	return info, nil
}

func writeManagerInfo(path string, info managerInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func tokensEqual(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func generateToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
