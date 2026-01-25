package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"mempack/internal/token"
)

const (
	initialContextThreadLimit    = 5
	initialContextStateMaxTokens = 120
	initialContextFallbackChars  = 800
)

type InitialContext struct {
	RepoID        string   `json:"repo_id"`
	Workspace     string   `json:"workspace"`
	HasState      bool     `json:"has_state"`
	StateSummary  string   `json:"state_summary,omitempty"`
	RecentThreads []string `json:"recent_threads,omitempty"`
	MemoryCount   int      `json:"memory_count"`
	ChunkCount    int      `json:"chunk_count"`
	Suggestion    string   `json:"suggestion"`
}

func handleGetInitialContext(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoOverride := strings.TrimSpace(request.GetString("repo", ""))
	workspaceOverride := strings.TrimSpace(request.GetString("workspace", ""))

	cfg, err := loadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("config error: %v", err)), nil
	}
	workspace := resolveWorkspace(cfg, workspaceOverride)

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

	stateRaw, stateTokens, _, err := loadState(repoInfo, workspace, st)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("state error: %v", err)), nil
	}
	stateSummary, hasState := summarizeState(cfg.Tokenizer, stateRaw, stateTokens)

	threads, err := st.ListRecentActiveThreads(repoInfo.ID, workspace, initialContextThreadLimit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("threads error: %v", err)), nil
	}
	recentThreads := make([]string, 0, len(threads))
	for _, thread := range threads {
		recentThreads = append(recentThreads, thread.ThreadID)
	}

	memoryCount, err := st.CountMemories(repoInfo.ID, workspace)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("memory count error: %v", err)), nil
	}
	chunkCount, err := st.CountChunks(repoInfo.ID, workspace)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("chunk count error: %v", err)), nil
	}

	result := InitialContext{
		RepoID:        repoInfo.ID,
		Workspace:     workspace,
		HasState:      hasState,
		StateSummary:  stateSummary,
		RecentThreads: recentThreads,
		MemoryCount:   memoryCount,
		ChunkCount:    chunkCount,
		Suggestion:    buildInitialSuggestion(hasState, memoryCount, chunkCount),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: formatInitialContextText(result)},
		},
		StructuredContent: result,
	}, nil
}

func summarizeState(tokenizer string, stateRaw json.RawMessage, stateTokens int) (string, bool) {
	stateText := strings.TrimSpace(string(stateRaw))
	if stateText == "" || stateText == "{}" {
		return "", false
	}

	compacted := stateText
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(stateText)); err == nil {
		compacted = buf.String()
	}
	if compacted == "" || compacted == "{}" {
		return "", false
	}

	if stateTokens > 0 && stateTokens <= initialContextStateMaxTokens {
		return compacted, true
	}

	counter, err := token.New(tokenizer)
	if err != nil {
		return truncateString(compacted, initialContextFallbackChars), true
	}
	truncated, _ := counter.Truncate(compacted, initialContextStateMaxTokens)
	if truncated == "" {
		return truncateString(compacted, initialContextFallbackChars), true
	}
	return truncated, true
}

func buildInitialSuggestion(hasState bool, memoryCount, chunkCount int) string {
	if memoryCount == 0 && chunkCount == 0 {
		if hasState {
			return "No memories or chunks yet. Add memories or ingest artifacts, then call mempack.get_context."
		}
		return "No memories or chunks yet. Add memories or ingest artifacts; consider setting state with mempack.checkpoint."
	}
	if !hasState {
		return "Call mempack.get_context with a short query; consider setting state with mempack.checkpoint."
	}
	return "Call mempack.get_context with a short, specific query to fetch relevant memories and chunks."
}

func formatInitialContextText(result InitialContext) string {
	stateLabel := "none"
	if result.HasState {
		stateLabel = "present"
	}
	lines := []string{
		fmt.Sprintf("Initial context: repo=%s workspace=%s", result.RepoID, result.Workspace),
		fmt.Sprintf("state=%s memories=%d chunks=%d recent_threads=%d", stateLabel, result.MemoryCount, result.ChunkCount, len(result.RecentThreads)),
	}
	if result.StateSummary != "" {
		lines = append(lines, fmt.Sprintf("state_summary: %s", result.StateSummary))
	}
	if len(result.RecentThreads) > 0 {
		lines = append(lines, fmt.Sprintf("recent_threads: %s", strings.Join(result.RecentThreads, ", ")))
	}
	if result.Suggestion != "" {
		lines = append(lines, fmt.Sprintf("suggestion: %s", result.Suggestion))
	}
	return strings.Join(lines, "\n")
}

func truncateString(value string, max int) string {
	if value == "" || max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
