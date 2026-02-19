package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type ForgetResponse struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

func runForget(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("forget", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id")
	workspace := fs.String("workspace", "", "Workspace name")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"repo":      {RequiresValue: true},
		"workspace": {RequiresValue: true},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	id := strings.TrimSpace(strings.Join(positional, " "))
	if id == "" {
		fmt.Fprintln(errOut, "missing id")
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	repoInfo, err := resolveRepo(&cfg, strings.TrimSpace(*repoOverride))
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		fmt.Fprintf(errOut, "store open error: %v\n", err)
		return 1
	}
	defer st.Close()

	now := time.Now().UTC()
	isMemoryPref := strings.HasPrefix(id, "M")
	isChunkPref := strings.HasPrefix(id, "C")

	if !isChunkPref {
		ok, err := st.ForgetMemory(repoInfo.ID, workspaceName, id, now)
		if err != nil {
			fmt.Fprintf(errOut, "forget error: %v\n", err)
			return 1
		}
		if ok {
			return writeJSON(out, errOut, ForgetResponse{ID: id, Kind: "memory", Status: "forgotten"})
		}
	}

	if !isMemoryPref {
		ok, err := st.ForgetChunk(repoInfo.ID, workspaceName, id, now)
		if err != nil {
			fmt.Fprintf(errOut, "forget error: %v\n", err)
			return 1
		}
		if ok {
			return writeJSON(out, errOut, ForgetResponse{ID: id, Kind: "chunk", Status: "forgotten"})
		}
	}

	encoded, _ := json.MarshalIndent(ForgetResponse{ID: id, Kind: "unknown", Status: "not found"}, "", "  ")
	fmt.Fprintln(out, string(encoded))
	return 1
}
