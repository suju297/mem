package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"mempack/internal/store"
	"mempack/internal/token"
)

type mcpWriteConfig struct {
	Allowed bool
	Mode    string
	Source  string
}

const (
	writeModeOff  = "off"
	writeModeAsk  = "ask"
	writeModeAuto = "auto"
)

func parseWriteConfig(allowWrite bool, repoOptIn bool, modeFlag string) (mcpWriteConfig, error) {
	mode := strings.ToLower(strings.TrimSpace(modeFlag))
	if mode == "" {
		if allowWrite || repoOptIn {
			mode = writeModeAsk
		} else {
			mode = writeModeOff
		}
	}
	switch mode {
	case writeModeOff, writeModeAsk, writeModeAuto:
	default:
		return mcpWriteConfig{}, fmt.Errorf("invalid --write-mode: %s (use ask|auto|off)", mode)
	}
	if !allowWrite && !repoOptIn && mode != writeModeOff {
		return mcpWriteConfig{}, fmt.Errorf("write mode requires --allow-write or mempack.allow_write=true in .mempack/MEMORY.md")
	}

	allowed := (allowWrite || repoOptIn) && mode != writeModeOff
	source := "off"
	if allowed {
		if allowWrite {
			source = "flag"
		} else {
			source = "repo"
		}
	}
	return mcpWriteConfig{Allowed: allowed, Mode: mode, Source: source}, nil
}

func handleAddMemory(ctx context.Context, request mcp.CallToolRequest, writeCfg mcpWriteConfig) (*mcp.CallToolResult, error) {
	if !writeCfg.Allowed {
		return mcp.NewToolResultError("write tools disabled (use --allow-write or set mempack.allow_write=true in .mempack/MEMORY.md)"), nil
	}

	threadID := strings.TrimSpace(request.GetString("thread", ""))
	title := strings.TrimSpace(request.GetString("title", ""))
	summary := strings.TrimSpace(request.GetString("summary", ""))
	tags := strings.TrimSpace(request.GetString("tags", ""))
	workspace := strings.TrimSpace(request.GetString("workspace", ""))
	repoOverride := strings.TrimSpace(request.GetString("repo", ""))

	if threadID == "" {
		return mcp.NewToolResultError("missing thread"), nil
	}
	if title == "" {
		return mcp.NewToolResultError("missing title"), nil
	}
	if summary == "" {
		return mcp.NewToolResultError("missing summary"), nil
	}
	if err := requireWriteConfirmation(request, writeCfg); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if pattern, ok := detectSensitive(summary); ok {
		return mcp.NewToolResultError(fmt.Sprintf("potential secret detected (%s); redact and retry", pattern)), nil
	}

	untrusted := containsInjection(summary)

	cfg, err := loadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("config error: %v", err)), nil
	}
	workspace = resolveWorkspace(cfg, workspace)

	counter, err := token.New(cfg.Tokenizer)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("tokenizer error: %v", err)), nil
	}

	repoInfo, err := resolveRepo(cfg, repoOverride)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("repo detection error: %v", err)), nil
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("store open error: %v", err)), nil
	}
	defer st.Close()

	if err := st.EnsureRepo(repoInfo); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("store repo error: %v", err)), nil
	}

	tagList := store.ParseTags(tags)
	tagList = store.NormalizeTags(tagList)
	if untrusted {
		tagList = append(tagList, "untrusted")
		tagList = store.NormalizeTags(tagList)
	}
	tagsJSON := store.TagsToJSON(tagList)
	tagsText := store.TagsText(tagList)

	anchorCommit := ""
	if repoInfo.HasGit {
		anchorCommit = repoInfo.Head
	}

	createdAt := time.Now().UTC()
	summaryTokens := counter.Count(summary)
	memory, err := st.AddMemory(store.AddMemoryInput{
		RepoID:        repoInfo.ID,
		Workspace:     workspace,
		ThreadID:      threadID,
		Title:         title,
		Summary:       summary,
		SummaryTokens: summaryTokens,
		TagsJSON:      tagsJSON,
		TagsText:      tagsText,
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		AnchorCommit:  anchorCommit,
		CreatedAt:     createdAt,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("add memory error: %v", err)), nil
	}
	_ = maybeEmbedMemory(cfg, st, memory)

	result := map[string]string{
		"id":            memory.ID,
		"thread_id":     memory.ThreadID,
		"title":         memory.Title,
		"anchor_commit": memory.AnchorCommit,
		"created_at":    memory.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Memory saved: %s", memory.ID)},
		},
		StructuredContent: result,
	}, nil
}

func handleCheckpoint(ctx context.Context, request mcp.CallToolRequest, writeCfg mcpWriteConfig) (*mcp.CallToolResult, error) {
	if !writeCfg.Allowed {
		return mcp.NewToolResultError("write tools disabled (use --allow-write or set mempack.allow_write=true in .mempack/MEMORY.md)"), nil
	}

	reason := strings.TrimSpace(request.GetString("reason", ""))
	stateJSON := strings.TrimSpace(request.GetString("state_json", ""))
	threadID := strings.TrimSpace(request.GetString("thread", ""))
	workspace := strings.TrimSpace(request.GetString("workspace", ""))
	repoOverride := strings.TrimSpace(request.GetString("repo", ""))

	if reason == "" {
		return mcp.NewToolResultError("missing reason"), nil
	}
	if stateJSON == "" {
		return mcp.NewToolResultError("missing state_json"), nil
	}
	if !json.Valid([]byte(stateJSON)) {
		return mcp.NewToolResultError("state_json must be valid JSON"), nil
	}
	if err := requireWriteConfirmation(request, writeCfg); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if pattern, ok := detectSensitive(stateJSON); ok {
		return mcp.NewToolResultError(fmt.Sprintf("potential secret detected (%s); redact and retry", pattern)), nil
	}
	if containsInjection(stateJSON) {
		return mcp.NewToolResultError("state_json contains unsafe phrases; remove and retry"), nil
	}

	state, err := loadStatePayload("", stateJSON)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("state error: %v", err)), nil
	}

	cfg, err := loadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("config error: %v", err)), nil
	}
	workspace = resolveWorkspace(cfg, workspace)

	counter, err := token.New(cfg.Tokenizer)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("tokenizer error: %v", err)), nil
	}

	repoInfo, err := resolveRepo(cfg, repoOverride)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("repo detection error: %v", err)), nil
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("store open error: %v", err)), nil
	}
	defer st.Close()

	now := time.Now().UTC()
	stateID := store.NewID("S")
	stateTokens := counter.Count(state)
	if err := st.AddStateHistory(stateID, repoInfo.ID, workspace, state, reason, stateTokens, now); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("state history error: %v", err)), nil
	}
	if err := st.SetStateCurrent(repoInfo.ID, workspace, state, stateTokens, now); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("state current error: %v", err)), nil
	}

	resp := map[string]string{
		"state_id":  stateID,
		"workspace": workspace,
		"reason":    reason,
	}

	if threadID != "" {
		anchorCommit := ""
		if repoInfo.HasGit {
			anchorCommit = repoInfo.Head
		}
		reasonTokens := counter.Count(reason)
		mem, err := st.AddMemory(store.AddMemoryInput{
			RepoID:        repoInfo.ID,
			ThreadID:      threadID,
			Workspace:     workspace,
			Title:         "Checkpoint",
			Summary:       reason,
			SummaryTokens: reasonTokens,
			TagsJSON:      "[]",
			TagsText:      "",
			EntitiesJSON:  "[]",
			EntitiesText:  "",
			AnchorCommit:  anchorCommit,
			CreatedAt:     now,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("memory add error: %v", err)), nil
		}
		_ = maybeEmbedMemory(cfg, st, mem)
		resp["memory_id"] = mem.ID
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Checkpoint saved: %s", stateID)},
		},
		StructuredContent: resp,
	}, nil
}

func requireWriteConfirmation(request mcp.CallToolRequest, writeCfg mcpWriteConfig) error {
	if writeCfg.Mode != writeModeAsk {
		return nil
	}
	confirmed := request.GetBool("confirmed", false)
	if !confirmed {
		return fmt.Errorf("write_mode=ask requires confirmed=true after user approval")
	}
	return nil
}

func detectSensitive(text string) (string, bool) {
	lower := strings.ToLower(text)
	for _, pattern := range []string{
		"-----begin private key-----",
		"-----begin openssh private key-----",
		"aws_access_key_id",
		"aws_secret_access_key",
		"github_pat_",
		"ghp_",
		"gho_",
		"xoxb-",
		"xoxp-",
		"sk_live_",
		"sk_test_",
		"secret_key",
		"api_key",
	} {
		if strings.Contains(lower, pattern) {
			return pattern, true
		}
	}
	return "", false
}

func containsInjection(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range []string{"ignore previous instructions", "system prompt", "you are an ai", "jailbreak"} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
