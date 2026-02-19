package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	memBin := os.Getenv("MEM_BIN")
	repoDir := os.Getenv("REPO_DIR")
	query := os.Getenv("QUERY")
	if memBin == "" || repoDir == "" {
		fmt.Fprintln(os.Stderr, "MEM_BIN and REPO_DIR are required")
		os.Exit(1)
	}
	if query == "" {
		query = "decision"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	env := []string{}
	if v := os.Getenv("MEMPACK_DATA_DIR"); v != "" {
		env = append(env, "MEMPACK_DATA_DIR="+v)
	}
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" {
		env = append(env, "XDG_CONFIG_HOME="+v)
	}
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		env = append(env, "XDG_DATA_HOME="+v)
	}
	if v := os.Getenv("XDG_CACHE_HOME"); v != "" {
		env = append(env, "XDG_CACHE_HOME="+v)
	}

	cmdArgs := []string{"mcp", "--allow-write", "--write-mode", "ask"}
	cmdArgs = append(cmdArgs, "--require-repo", "--repo", repoDir)
	stdio := transport.NewStdioWithOptions(
		memBin,
		env,
		cmdArgs,
		transport.WithCommandFunc(func(ctx context.Context, command string, env []string, args []string) (*exec.Cmd, error) {
			cmd := exec.CommandContext(ctx, command, args...)
			cmd.Dir = repoDir
			cmd.Env = append(os.Environ(), env...)
			return cmd, nil
		}),
	)

	c := client.NewClient(stdio)
	if err := c.Start(ctx); err != nil {
		fail("start client", err)
	}
	defer c.Close()

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "mcp-e2e", Version: "1.0"}
	initReq.Params.Capabilities = mcp.ClientCapabilities{}

	initRes, err := c.Initialize(ctx, initReq)
	if err != nil {
		fail("initialize", err)
	}
	if initRes.ServerInfo.Name == "" {
		fail("initialize", fmt.Errorf("server name missing"))
	}

	if err := c.Ping(ctx); err != nil {
		fail("ping", err)
	}

	toolsRes, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		fail("list tools", err)
	}
	requireTool(toolsRes.Tools, "mempack_get_context")
	requireTool(toolsRes.Tools, "mempack_get_initial_context")
	requireTool(toolsRes.Tools, "mempack_explain")
	requireTool(toolsRes.Tools, "mempack_add_memory")
	requireTool(toolsRes.Tools, "mempack_update_memory")
	requireTool(toolsRes.Tools, "mempack_link_memories")
	requireTool(toolsRes.Tools, "mempack_checkpoint")

	initCtxRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "mempack_get_initial_context", Arguments: map[string]any{"repo": repoDir}},
	})
	if err != nil {
		fail("get_initial_context", err)
	}
	if initCtxRes.IsError {
		fail("get_initial_context", fmt.Errorf("tool error"))
	}
	initPayload := asMap(initCtxRes.StructuredContent)
	if initPayload["repo_id"] == nil {
		fail("get_initial_context", fmt.Errorf("missing repo_id"))
	}

	ctxRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack_get_context",
			Arguments: map[string]any{"query": query, "format": "json", "repo": repoDir},
		},
	})
	if err != nil {
		fail("get_context", err)
	}
	if ctxRes.IsError {
		fail("get_context", fmt.Errorf("tool error"))
	}
	ctxPayload := asMap(ctxRes.StructuredContent)
	if ctxPayload["top_memories"] == nil {
		fail("get_context", fmt.Errorf("missing top_memories"))
	}

	explainRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "mempack_explain", Arguments: map[string]any{"query": query, "repo": repoDir}},
	})
	if err != nil {
		fail("explain", err)
	}
	if explainRes.IsError {
		fail("explain", fmt.Errorf("tool error"))
	}

	addRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "mempack_add_memory",
			Arguments: map[string]any{
				"thread":    "T-E2E",
				"title":     "MCP E2E",
				"summary":   "MCP tool write test",
				"entities":  "file_src_old_ts,ext_ts",
				"repo":      repoDir,
				"confirmed": true,
			},
		},
	})
	if err != nil {
		fail("add_memory", err)
	}
	if addRes.IsError {
		fail("add_memory", fmt.Errorf("tool error"))
	}
	addPayload := asMap(addRes.StructuredContent)
	id, _ := addPayload["id"].(string)
	if id == "" {
		fail("add_memory", fmt.Errorf("missing id in add response"))
	}

	oldQueryRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack_get_context",
			Arguments: map[string]any{"query": "file_src_old_ts", "format": "json", "repo": repoDir},
		},
	})
	if err != nil {
		fail("get_context old token", err)
	}
	if oldQueryRes.IsError {
		fail("get_context old token", fmt.Errorf("tool error"))
	}
	if !containsMemoryID(asMap(oldQueryRes.StructuredContent), id) {
		fail("get_context old token", fmt.Errorf("expected memory %s in results", id))
	}

	updateRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "mempack_update_memory",
			Arguments: map[string]any{
				"id":        id,
				"entities":  "file_src_new_ts,ext_ts",
				"repo":      repoDir,
				"confirmed": true,
			},
		},
	})
	if err != nil {
		fail("update_memory", err)
	}
	if updateRes.IsError {
		fail("update_memory", fmt.Errorf("tool error"))
	}
	updatePayload := asMap(updateRes.StructuredContent)
	updatedID, _ := updatePayload["id"].(string)
	if updatedID != id {
		fail("update_memory", fmt.Errorf("expected in-place update id %s, got %s", id, updatedID))
	}

	oldAfterUpdateRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack_get_context",
			Arguments: map[string]any{"query": "file_src_old_ts", "format": "json", "repo": repoDir},
		},
	})
	if err != nil {
		fail("get_context old token after update", err)
	}
	if oldAfterUpdateRes.IsError {
		fail("get_context old token after update", fmt.Errorf("tool error"))
	}
	if containsMemoryID(asMap(oldAfterUpdateRes.StructuredContent), id) {
		fail("get_context old token after update", fmt.Errorf("old token still matched memory %s", id))
	}

	newAfterUpdateRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack_get_context",
			Arguments: map[string]any{"query": "file_src_new_ts", "format": "json", "repo": repoDir},
		},
	})
	if err != nil {
		fail("get_context new token after update", err)
	}
	if newAfterUpdateRes.IsError {
		fail("get_context new token after update", fmt.Errorf("tool error"))
	}
	if !containsMemoryID(asMap(newAfterUpdateRes.StructuredContent), id) {
		fail("get_context new token after update", fmt.Errorf("new token did not match memory %s", id))
	}

	relatedRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "mempack_add_memory",
			Arguments: map[string]any{
				"thread":    "T-E2E",
				"title":     "MCP E2E Related",
				"summary":   "Linked memory target",
				"entities":  "file_src_support_ts",
				"repo":      repoDir,
				"confirmed": true,
			},
		},
	})
	if err != nil {
		fail("add_memory related", err)
	}
	if relatedRes.IsError {
		fail("add_memory related", fmt.Errorf("tool error"))
	}
	relatedPayload := asMap(relatedRes.StructuredContent)
	relatedID, _ := relatedPayload["id"].(string)
	if relatedID == "" {
		fail("add_memory related", fmt.Errorf("missing id in add response"))
	}

	linkRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "mempack_link_memories",
			Arguments: map[string]any{
				"from_id":   id,
				"rel":       "supersedes",
				"to_id":     relatedID,
				"repo":      repoDir,
				"confirmed": true,
			},
		},
	})
	if err != nil {
		fail("link_memories", err)
	}
	if linkRes.IsError {
		fail("link_memories", fmt.Errorf("tool error"))
	}

	unscopedRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack_get_context",
			Arguments: map[string]any{"query": "file_src_new_ts", "format": "json"},
		},
	})
	if err != nil {
		fail("get_context unscoped", err)
	}
	if unscopedRes.IsError {
		fail("get_context unscoped", fmt.Errorf("expected unscoped call in current repo to succeed"))
	}
	if !containsMemoryID(asMap(unscopedRes.StructuredContent), id) {
		fail("get_context unscoped", fmt.Errorf("expected unscoped call to return memory %s", id))
	}

	ckRes, err := c.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name: "mempack_checkpoint",
			Arguments: map[string]any{
				"reason":     "MCP E2E",
				"state_json": "{\"goal\":\"ship\"}",
				"thread":     "T-E2E",
				"repo":       repoDir,
				"confirmed":  true,
			},
		},
	})
	if err != nil {
		fail("checkpoint", err)
	}
	if ckRes.IsError {
		fail("checkpoint", fmt.Errorf("tool error"))
	}

	fmt.Println("mcp e2e: ok")
}

func requireTool(tools []mcp.Tool, name string) {
	for _, tool := range tools {
		if tool.Name == name {
			return
		}
	}
	fail("list tools", fmt.Errorf("missing tool %s", name))
}

func asMap(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func containsMemoryID(payload map[string]any, id string) bool {
	if id == "" {
		return false
	}
	raw, ok := payload["top_memories"]
	if !ok {
		return false
	}
	items, ok := raw.([]any)
	if !ok {
		return false
	}
	for _, item := range items {
		mem, ok := item.(map[string]any)
		if !ok {
			continue
		}
		got, _ := mem["id"].(string)
		if got == id {
			return true
		}
	}
	return false
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "mcp e2e failed (%s): %v\n", step, err)
	os.Exit(1)
}
