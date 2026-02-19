package app

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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
		return mcpWriteConfig{}, fmt.Errorf("invalid write mode: %s (use ask|auto|off)", mode)
	}
	if !allowWrite && !repoOptIn && mode != writeModeOff {
		return mcpWriteConfig{}, fmt.Errorf("write mode requires --allow-write, mcp_allow_write=true in config, or .mempack/config.json")
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

func handleAddMemory(_ context.Context, request mcp.CallToolRequest, writeCfg mcpWriteConfig, requireRepo bool) (*mcp.CallToolResult, error) {
	if !writeCfg.Allowed {
		return mcp.NewToolResultError("write tools disabled (use --allow-write or set mcp_allow_write in config or .mempack/config.json)"), nil
	}

	title := strings.TrimSpace(request.GetString("title", ""))
	summary := strings.TrimSpace(request.GetString("summary", ""))
	tags := strings.TrimSpace(request.GetString("tags", ""))
	entities := strings.TrimSpace(request.GetString("entities", ""))
	workspace := strings.TrimSpace(request.GetString("workspace", ""))
	repoOverride := strings.TrimSpace(request.GetString("repo", ""))

	if title == "" {
		return mcp.NewToolResultError("missing title"), nil
	}
	if err := requireWriteConfirmation(request, writeCfg); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if pattern, ok := detectSensitive(title); ok {
		return mcp.NewToolResultError(fmt.Sprintf("potential secret detected (%s); redact and retry", pattern)), nil
	}
	if pattern, ok := detectSensitive(summary); ok {
		return mcp.NewToolResultError(fmt.Sprintf("potential secret detected (%s); redact and retry", pattern)), nil
	}
	if containsInjection(summary) || containsInjection(title) {
		return mcp.NewToolResultError("title/summary contains unsafe phrases; remove and retry"), nil
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

	repoInfo, err := resolveRepoWithOptions(&cfg, repoOverride, repoResolveOptions{
		RequireRepo: requireRepo,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("repo detection error: %v", err)), nil
	}
	threadUsed, threadDefaulted, err := resolveThread(cfg, strings.TrimSpace(request.GetString("thread", "")))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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
	tagsJSON := store.TagsToJSON(tagList)
	tagsText := store.TagsText(tagList)
	entityList := store.ParseEntities(entities)
	entityList = store.NormalizeEntities(entityList)
	entitiesJSON := store.EntitiesToJSON(entityList)
	entitiesText := store.EntitiesText(entityList)

	anchorCommit := ""
	if repoInfo.HasGit {
		anchorCommit = repoInfo.Head
	}

	createdAt := time.Now().UTC()
	summaryTokens := counter.Count(summary)
	memory, err := st.AddMemory(store.AddMemoryInput{
		RepoID:        repoInfo.ID,
		Workspace:     workspace,
		ThreadID:      threadUsed,
		Title:         title,
		Summary:       summary,
		SummaryTokens: summaryTokens,
		TagsJSON:      tagsJSON,
		TagsText:      tagsText,
		EntitiesJSON:  entitiesJSON,
		EntitiesText:  entitiesText,
		AnchorCommit:  anchorCommit,
		CreatedAt:     createdAt,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("add memory error: %v", err)), nil
	}
	_ = maybeEmbedMemory(cfg, st, memory)

	result := map[string]any{
		"id":               memory.ID,
		"thread_id":        memory.ThreadID,
		"thread_used":      memory.ThreadID,
		"thread_defaulted": threadDefaulted,
		"title":            memory.Title,
		"anchor_commit":    memory.AnchorCommit,
		"created_at":       memory.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Memory saved: %s", memory.ID)},
		},
		StructuredContent: result,
	}, nil
}

func handleUpdateMemory(_ context.Context, request mcp.CallToolRequest, writeCfg mcpWriteConfig, requireRepo bool) (*mcp.CallToolResult, error) {
	if !writeCfg.Allowed {
		return mcp.NewToolResultError("write tools disabled (use --allow-write or set mcp_allow_write in config or .mempack/config.json)"), nil
	}

	id := strings.TrimSpace(request.GetString("id", ""))
	if id == "" {
		return mcp.NewToolResultError("missing id"), nil
	}

	flags := updateFieldFlagsFromMCPRequest(request)
	if !flags.Any() {
		return mcp.NewToolResultError("no update fields provided"), nil
	}

	if err := requireWriteConfirmation(request, writeCfg); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	title := strings.TrimSpace(request.GetString("title", ""))
	summary := strings.TrimSpace(request.GetString("summary", ""))
	tags := strings.TrimSpace(request.GetString("tags", ""))
	tagsAdd := strings.TrimSpace(request.GetString("tags_add", ""))
	tagsRemove := strings.TrimSpace(request.GetString("tags_remove", ""))
	entities := strings.TrimSpace(request.GetString("entities", ""))
	entitiesAdd := strings.TrimSpace(request.GetString("entities_add", ""))
	entitiesRemove := strings.TrimSpace(request.GetString("entities_remove", ""))
	workspace := strings.TrimSpace(request.GetString("workspace", ""))
	repoOverride := strings.TrimSpace(request.GetString("repo", ""))

	cfg, err := loadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("config error: %v", err)), nil
	}
	workspace = resolveWorkspace(cfg, workspace)

	repoInfo, err := resolveRepoWithOptions(&cfg, repoOverride, repoResolveOptions{
		RequireRepo: requireRepo,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("repo detection error: %v", err)), nil
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("store open error: %v", err)), nil
	}
	defer st.Close()

	updateInput, err := makeUpdateMemoryInput(
		repoInfo.ID,
		workspace,
		id,
		cfg.Tokenizer,
		flags,
		updateFieldValues{
			Title:          title,
			Summary:        summary,
			Tags:           tags,
			TagsAdd:        tagsAdd,
			TagsRemove:     tagsRemove,
			Entities:       entities,
			EntitiesAdd:    entitiesAdd,
			EntitiesRemove: entitiesRemove,
		},
	)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("tokenizer error: %v", err)), nil
	}

	mem, changed, err := st.UpdateMemoryWithStatus(updateInput)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("update memory error: %v", err)), nil
	}
	if changed {
		_ = maybeEmbedMemory(cfg, st, mem)
	}

	result := map[string]any{
		"id":           mem.ID,
		"thread_id":    mem.ThreadID,
		"title":        mem.Title,
		"summary":      mem.Summary,
		"operation_at": time.Now().UTC().Format(time.RFC3339Nano),
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Memory updated: %s", mem.ID)},
		},
		StructuredContent: result,
	}, nil
}

func handleLinkMemories(_ context.Context, request mcp.CallToolRequest, writeCfg mcpWriteConfig, requireRepo bool) (*mcp.CallToolResult, error) {
	if !writeCfg.Allowed {
		return mcp.NewToolResultError("write tools disabled (use --allow-write or set mcp_allow_write in config or .mempack/config.json)"), nil
	}

	fromID := strings.TrimSpace(request.GetString("from_id", ""))
	toID := strings.TrimSpace(request.GetString("to_id", ""))
	relRaw := request.GetString("rel", "")
	workspace := strings.TrimSpace(request.GetString("workspace", ""))
	repoOverride := strings.TrimSpace(request.GetString("repo", ""))

	if fromID == "" {
		return mcp.NewToolResultError("missing from_id"), nil
	}
	if toID == "" {
		return mcp.NewToolResultError("missing to_id"), nil
	}
	if fromID == toID {
		return mcp.NewToolResultError("from_id and to_id must differ"), nil
	}
	rel, err := normalizeLinkRelation(relRaw)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid rel: %v", err)), nil
	}
	if err := requireWriteConfirmation(request, writeCfg); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	cfg, err := loadConfig()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("config error: %v", err)), nil
	}
	workspace = resolveWorkspace(cfg, workspace)

	repoInfo, err := resolveRepoWithOptions(&cfg, repoOverride, repoResolveOptions{
		RequireRepo: requireRepo,
	})
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
	if _, err := ensureMemoryExistsForLink(st, repoInfo.ID, workspace, fromID, "from"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if _, err := ensureMemoryExistsForLink(st, repoInfo.ID, workspace, toID, "to"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	createdAt := time.Now().UTC()
	if err := st.AddLink(store.Link{
		FromID:    fromID,
		Rel:       rel,
		ToID:      toID,
		Weight:    1,
		CreatedAt: createdAt,
	}); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("link error: %v", err)), nil
	}

	result := map[string]any{
		"from_id":    fromID,
		"rel":        rel,
		"to_id":      toID,
		"weight":     1,
		"created_at": createdAt.Format(time.RFC3339Nano),
		"status":     "linked",
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{Type: "text", Text: fmt.Sprintf("Memory link saved: %s --%s--> %s", fromID, rel, toID)},
		},
		StructuredContent: result,
	}, nil
}

func handleCheckpoint(_ context.Context, request mcp.CallToolRequest, writeCfg mcpWriteConfig, requireRepo bool) (*mcp.CallToolResult, error) {
	if !writeCfg.Allowed {
		return mcp.NewToolResultError("write tools disabled (use --allow-write or set mcp_allow_write in config or .mempack/config.json)"), nil
	}

	reason := strings.TrimSpace(request.GetString("reason", ""))
	stateJSON := strings.TrimSpace(request.GetString("state_json", ""))
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

	repoInfo, err := resolveRepoWithOptions(&cfg, repoOverride, repoResolveOptions{
		RequireRepo: requireRepo,
	})
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("repo detection error: %v", err)), nil
	}
	threadUsed, threadDefaulted, err := resolveThread(cfg, strings.TrimSpace(request.GetString("thread", "")))
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
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

	anchorCommit := ""
	if repoInfo.HasGit {
		anchorCommit = repoInfo.Head
	}
	reasonTokens := counter.Count(reason)
	mem, err := st.AddMemory(store.AddMemoryInput{
		RepoID:        repoInfo.ID,
		ThreadID:      threadUsed,
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

	resp := map[string]any{
		"state_id":         stateID,
		"workspace":        workspace,
		"reason":           reason,
		"memory_id":        mem.ID,
		"thread_used":      threadUsed,
		"thread_defaulted": threadDefaulted,
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

func requestHasArg(request mcp.CallToolRequest, key string) bool {
	args := request.GetArguments()
	if args == nil {
		return false
	}
	_, ok := args[key]
	return ok
}

var sensitiveAssignmentPatterns = []struct {
	label string
	re    *regexp.Regexp
}{
	{
		label: "secret_key",
		re:    regexp.MustCompile(`(?i)\bsecret[_-]?key\b["']?\s*[:=]\s*["']?[A-Za-z0-9_\-]{8,}`),
	},
	{
		label: "api_key",
		re:    regexp.MustCompile(`(?i)\bapi[_-]?key\b["']?\s*[:=]\s*["']?[A-Za-z0-9_\-]{8,}`),
	},
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
	} {
		if strings.Contains(lower, pattern) {
			return pattern, true
		}
	}
	for _, pattern := range sensitiveAssignmentPatterns {
		if pattern.re.MatchString(text) {
			return pattern.label, true
		}
	}
	return "", false
}

func containsInjection(text string) bool {
	return containsPromptInjectionPhrase(text)
}
