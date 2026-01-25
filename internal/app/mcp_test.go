package app

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"mempack/internal/pack"
	"mempack/internal/store"
)

func TestMCPGetContextStructuredAndPrompt(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	seedMemory(t, "decision", "Decision summary")

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack.get_context",
			Arguments: map[string]any{"query": "decision", "format": "json"},
		},
	}
	res, err := handleGetContext(context.Background(), req)
	if err != nil {
		t.Fatalf("get_context error: %v", err)
	}
	if res.StructuredContent == nil {
		t.Fatalf("expected structured content")
	}
	packRes, ok := res.StructuredContent.(pack.ContextPack)
	if !ok {
		t.Fatalf("expected ContextPack, got %T", res.StructuredContent)
	}
	if len(packRes.TopMemories) == 0 {
		t.Fatalf("expected at least one memory in context pack")
	}
	if len(res.Content) == 0 {
		t.Fatalf("expected text content for get_context")
	}
	text, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	if !bytes.Contains([]byte(text.Text), []byte("Context from Memory")) {
		t.Fatalf("expected prompt content to include Context from Memory header")
	}
	if len(res.Content) < 2 {
		t.Fatalf("expected JSON fallback content for format=json")
	}
	jsonText, ok := res.Content[1].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected JSON text content, got %T", res.Content[1])
	}
	var decoded pack.ContextPack
	if err := json.Unmarshal([]byte(jsonText.Text), &decoded); err != nil {
		t.Fatalf("expected JSON fallback to be valid: %v", err)
	}

	reqPrompt := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack.get_context",
			Arguments: map[string]any{"query": "decision", "format": "prompt"},
		},
	}
	resPrompt, err := handleGetContext(context.Background(), reqPrompt)
	if err != nil {
		t.Fatalf("get_context prompt error: %v", err)
	}
	if len(resPrompt.Content) == 0 {
		t.Fatalf("expected prompt text content")
	}
	text, ok = resPrompt.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", resPrompt.Content[0])
	}
	if !bytes.Contains([]byte(text.Text), []byte("Context from Memory")) {
		t.Fatalf("expected prompt content to include Context from Memory header")
	}
}

func TestMCPExplainDeterministic(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	seedMemory(t, "decision", "Deterministic decision")

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack.explain",
			Arguments: map[string]any{"query": "decision"},
		},
	}
	first, err := handleExplain(context.Background(), req)
	if err != nil {
		t.Fatalf("explain error: %v", err)
	}
	second, err := handleExplain(context.Background(), req)
	if err != nil {
		t.Fatalf("explain error: %v", err)
	}
	if len(first.Content) == 0 || len(second.Content) == 0 {
		t.Fatalf("expected explain text content")
	}

	reportA, ok := first.StructuredContent.(ExplainReport)
	if !ok {
		t.Fatalf("expected ExplainReport, got %T", first.StructuredContent)
	}
	reportB, ok := second.StructuredContent.(ExplainReport)
	if !ok {
		t.Fatalf("expected ExplainReport, got %T", second.StructuredContent)
	}

	a, err := json.Marshal(reportA)
	if err != nil {
		t.Fatalf("marshal report A: %v", err)
	}
	b, err := json.Marshal(reportB)
	if err != nil {
		t.Fatalf("marshal report B: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("expected deterministic explain output")
	}
}

func TestMCPAddMemoryRequiresConfirmation(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	args := map[string]any{
		"thread":  "T-1",
		"title":   "Decision",
		"summary": "A short summary.",
	}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack.add_memory",
			Arguments: args,
		},
	}
	res, err := handleAddMemory(context.Background(), req, mcpWriteConfig{Allowed: true, Mode: writeModeAsk})
	if err != nil {
		t.Fatalf("add_memory error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected add_memory to require confirmation")
	}

	args["confirmed"] = true
	res, err = handleAddMemory(context.Background(), req, mcpWriteConfig{Allowed: true, Mode: writeModeAsk})
	if err != nil {
		t.Fatalf("add_memory error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected add_memory to succeed with confirmation")
	}

	payload, ok := res.StructuredContent.(map[string]string)
	if !ok {
		t.Fatalf("expected structured content map, got %T", res.StructuredContent)
	}
	if payload["id"] == "" {
		t.Fatalf("expected memory id in response")
	}
}

func TestMCPCheckpointRequiresConfirmation(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	ckArgs := map[string]any{
		"reason":     "Checkpoint reason",
		"state_json": `{"goal":"ship"}`,
	}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack.checkpoint",
			Arguments: ckArgs,
		},
	}
	res, err := handleCheckpoint(context.Background(), req, mcpWriteConfig{Allowed: true, Mode: writeModeAsk})
	if err != nil {
		t.Fatalf("checkpoint error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected checkpoint to require confirmation")
	}

	ckArgs["confirmed"] = true
	res, err = handleCheckpoint(context.Background(), req, mcpWriteConfig{Allowed: true, Mode: writeModeAsk})
	if err != nil {
		t.Fatalf("checkpoint error: %v", err)
	}
	if res == nil || res.IsError {
		t.Fatalf("expected checkpoint to succeed with confirmation")
	}

	payload, ok := res.StructuredContent.(map[string]string)
	if !ok {
		t.Fatalf("expected structured content map, got %T", res.StructuredContent)
	}
	if payload["state_id"] == "" {
		t.Fatalf("expected state_id in response")
	}
}

func TestMCPCheckpointRejectsInvalidJSON(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	ckArgs := map[string]any{
		"reason":     "Checkpoint reason",
		"state_json": "{bad json}",
		"confirmed":  true,
	}
	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack.checkpoint",
			Arguments: ckArgs,
		},
	}
	res, err := handleCheckpoint(context.Background(), req, mcpWriteConfig{Allowed: true, Mode: writeModeAsk})
	if err != nil {
		t.Fatalf("checkpoint error: %v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("expected checkpoint to reject invalid JSON")
	}
}

func TestMCPHealthRepairToggle(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	repoInfo, err := resolveRepo(cfg, "")
	if err != nil {
		t.Fatalf("repo detection error: %v", err)
	}
	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		t.Fatalf("store open error: %v", err)
	}
	if err := st.EnsureRepo(repoInfo); err != nil {
		t.Fatalf("store repo error: %v", err)
	}
	if err := st.SetStateCurrent(repoInfo.ID, "default", "{bad json}", 0, time.Now().UTC()); err != nil {
		t.Fatalf("seed invalid state: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("store close error: %v", err)
	}

	report, err := checkMCPHealth("", false)
	if err == nil {
		t.Fatalf("expected mcp health to fail without repair")
	}
	msg := formatMCPHealthError(report, err)
	if msg != "invalid workspace state JSON (workspace=default). Run: mem doctor --repair" {
		t.Fatalf("unexpected error message: %s", msg)
	}

	if _, err := checkMCPHealth("", true); err != nil {
		t.Fatalf("expected mcp health to succeed with repair: %v", err)
	}
}

func seedMemory(t testing.TB, title, summary string) {
	t.Helper()
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	repoInfo, err := resolveRepo(cfg, "")
	if err != nil {
		t.Fatalf("repo detection error: %v", err)
	}
	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		t.Fatalf("store open error: %v", err)
	}
	defer st.Close()
	if err := st.EnsureRepo(repoInfo); err != nil {
		t.Fatalf("store repo error: %v", err)
	}

	createdAt := time.Unix(0, 0)
	_, err = st.AddMemory(store.AddMemoryInput{
		ID:            "M-TEST",
		RepoID:        repoInfo.ID,
		Workspace:     "default",
		ThreadID:      "T-TEST",
		Title:         title,
		Summary:       summary,
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		AnchorCommit:  repoInfo.Head,
		CreatedAt:     createdAt,
	})
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}
}
