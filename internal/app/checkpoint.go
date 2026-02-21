package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"mempack/internal/store"
	"mempack/internal/token"
)

type CheckpointResponse struct {
	StateID   string `json:"state_id"`
	Workspace string `json:"workspace"`
	Reason    string `json:"reason"`
	MemoryID  string `json:"memory_id,omitempty"`
}

func runCheckpoint(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("checkpoint", flag.ContinueOnError)
	fs.SetOutput(errOut)
	reason := fs.String("reason", "", "Checkpoint reason")
	workspace := fs.String("workspace", "", "Workspace name")
	stateFile := fs.String("state-file", "", "Path to state JSON/markdown")
	stateJSON := fs.String("state-json", "", "Inline state JSON")
	threadID := fs.String("thread", "", "Thread id (optional; defaults to default_thread or T-SESSION)")
	repoOverride := fs.String("repo", "", "Override repo id")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"reason":     {RequiresValue: true},
		"workspace":  {RequiresValue: true},
		"state-file": {RequiresValue: true},
		"state-json": {RequiresValue: true},
		"thread":     {RequiresValue: true},
		"repo":       {RequiresValue: true},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	reasonWasSet := flagWasSet(args, "reason")
	stateFileWasSet := flagWasSet(args, "state-file")
	stateJSONWasSet := flagWasSet(args, "state-json")
	remaining := append([]string{}, positional...)
	if !reasonWasSet && len(remaining) > 0 {
		*reason = remaining[0]
		remaining = remaining[1:]
	}
	if !stateFileWasSet && !stateJSONWasSet && len(remaining) > 0 {
		*stateJSON = remaining[0]
		remaining = remaining[1:]
	}
	if len(remaining) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(remaining, " "))
		return 2
	}

	reasonText := strings.TrimSpace(*reason)
	if reasonText == "" && isInteractiveTerminal(os.Stdin) {
		promptedReason, promptErr := promptText(os.Stdin, errOut, "Checkpoint reason", false)
		if promptErr != nil {
			fmt.Fprintf(errOut, "reason prompt error: %v\n", promptErr)
			return 1
		}
		reasonText = strings.TrimSpace(promptedReason)
	}
	if reasonText == "" {
		fmt.Fprintln(errOut, "missing reason (use --reason or first positional argument)")
		return 2
	}

	stateFileValue := strings.TrimSpace(*stateFile)
	stateJSONValue := strings.TrimSpace(*stateJSON)
	if stateFileValue == "" && stateJSONValue == "" && isInteractiveTerminal(os.Stdin) {
		promptedState, promptErr := promptText(os.Stdin, errOut, "State JSON/text (blank for {})", true)
		if promptErr != nil {
			fmt.Fprintf(errOut, "state prompt error: %v\n", promptErr)
			return 1
		}
		stateJSONValue = strings.TrimSpace(promptedState)
		if stateJSONValue == "" {
			stateJSONValue = "{}"
		}
	}

	state, err := loadStatePayload(stateFileValue, stateJSONValue)
	if err != nil {
		fmt.Fprintf(errOut, "state error: %v\n", err)
		return 1
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	counter, err := token.New(cfg.Tokenizer)
	if err != nil {
		fmt.Fprintf(errOut, "tokenizer error: %v\n", err)
		return 1
	}

	repoInfo, err := resolveRepo(&cfg, strings.TrimSpace(*repoOverride))
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}
	threadUsed, threadDefaulted, err := resolveThread(cfg, strings.TrimSpace(*threadID))
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		fmt.Fprintf(errOut, "store open error: %v\n", err)
		return 1
	}
	defer st.Close()

	now := time.Now().UTC()
	stateID := store.NewID("S")
	stateTokens := counter.Count(state)
	if err := st.AddStateHistory(stateID, repoInfo.ID, workspaceName, state, reasonText, stateTokens, now); err != nil {
		fmt.Fprintf(errOut, "state history error: %v\n", err)
		return 1
	}
	if err := st.SetStateCurrent(repoInfo.ID, workspaceName, state, stateTokens, now); err != nil {
		fmt.Fprintf(errOut, "state current error: %v\n", err)
		return 1
	}

	resp := CheckpointResponse{
		StateID:   stateID,
		Workspace: workspaceName,
		Reason:    reasonText,
	}

	anchorCommit := ""
	if repoInfo.HasGit {
		anchorCommit = repoInfo.Head
	}
	reasonTokens := counter.Count(reasonText)
	mem, err := st.AddMemory(store.AddMemoryInput{
		RepoID:        repoInfo.ID,
		ThreadID:      threadUsed,
		Workspace:     workspaceName,
		Title:         "Checkpoint",
		Summary:       reasonText,
		SummaryTokens: reasonTokens,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		AnchorCommit:  anchorCommit,
		CreatedAt:     now,
	})
	if err != nil {
		fmt.Fprintf(errOut, "memory add error: %v\n", err)
		return 1
	}
	if err := maybeEmbedMemory(cfg, st, mem); err != nil {
		fmt.Fprintf(errOut, "embedding warning: %v\n", err)
	}
	resp.MemoryID = mem.ID

	respWithThread := map[string]any{
		"state_id":         resp.StateID,
		"workspace":        resp.Workspace,
		"reason":           resp.Reason,
		"memory_id":        resp.MemoryID,
		"thread_used":      threadUsed,
		"thread_defaulted": threadDefaulted,
	}
	return writeJSON(out, errOut, respWithThread)
}

func loadStatePayload(path, inline string) (string, error) {
	if inline == "" && path == "" {
		return "", fmt.Errorf("state payload required (use --state-file or --state-json)")
	}

	var data []byte
	if path != "" {
		fileData, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		data = fileData
	} else {
		data = []byte(inline)
	}

	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return "{}", nil
	}

	if json.Valid([]byte(trimmed)) {
		return trimmed, nil
	}

	wrapped, err := json.Marshal(map[string]string{"raw": trimmed})
	if err != nil {
		return "", err
	}
	return string(wrapped), nil
}
