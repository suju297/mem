package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"mempack/internal/config"
	"mempack/internal/pack"
)

func TestAcceptanceGitAnchoringAndOrphans(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	writeTestConfig(t, base, func(cfg *config.Config) {
		cfg.EmbeddingProvider = "none"
	})

	runCLI(t, "init", "--no-agents")

	commitA := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "HEAD"))

	addAOut := runCLI(t, "add", "--thread", "T-accept", "--title", "Commit A", "--summary", "alpha-note")
	var addA addResp
	if err := json.Unmarshal(addAOut, &addA); err != nil {
		t.Fatalf("decode add A: %v", err)
	}
	if addA.AnchorCommit != commitA {
		t.Fatalf("expected commit A anchor %s, got %s", commitA, addA.AnchorCommit)
	}

	writeFile(t, repoDir, "file.txt", "content\nbeta\n")
	runGit(t, repoDir, "add", "file.txt")
	runGit(t, repoDir, "commit", "-m", "commit B")
	commitB := strings.TrimSpace(runGitOutput(t, repoDir, "rev-parse", "HEAD"))

	addBOut := runCLI(t, "add", "--thread", "T-accept", "--title", "Commit B", "--summary", "delta-99")
	var addB addResp
	if err := json.Unmarshal(addBOut, &addB); err != nil {
		t.Fatalf("decode add B: %v", err)
	}
	if addB.AnchorCommit != commitB {
		t.Fatalf("expected commit B anchor %s, got %s", commitB, addB.AnchorCommit)
	}

	if err := os.MkdirAll(filepath.Join(repoDir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	writeFile(t, repoDir, filepath.Join("docs", "acceptance.txt"), "mempack acceptance chunk\nDelta-99 for rewrite coverage.\n")
	runCLI(t, "ingest-artifact", "docs/acceptance.txt", "--thread", "acceptance-chunks-a")
	runCLI(t, "ingest-artifact", "docs/acceptance.txt", "--thread", "acceptance-chunks-b")

	out := runCLI(t, "get", "delta99", "--format", "json")
	var ctx pack.ContextPack
	if err := json.Unmarshal(out, &ctx); err != nil {
		t.Fatalf("decode delta99: %v", err)
	}
	if ctx.SearchMeta.ModeUsed != "bm25" {
		t.Fatalf("expected mode_used bm25, got %s", ctx.SearchMeta.ModeUsed)
	}
	if len(ctx.TopMemories) == 0 {
		t.Fatalf("expected delta99 memory at commit B")
	}
	if ctx.TopMemories[0].AnchorCommit != commitB {
		t.Fatalf("expected delta99 anchor %s, got %s", commitB, ctx.TopMemories[0].AnchorCommit)
	}
	if !containsString(ctx.SearchMeta.RewritesApplied, "delta99 -> delta 99") {
		t.Fatalf("expected delta99 rewrite metadata, got %+v", ctx.SearchMeta.RewritesApplied)
	}
	if len(ctx.TopChunks) != 1 {
		t.Fatalf("expected 1 deduped chunk, got %d", len(ctx.TopChunks))
	}
	if len(ctx.TopChunks[0].Sources) != 2 {
		t.Fatalf("expected 2 chunk sources, got %d", len(ctx.TopChunks[0].Sources))
	}

	out = runCLI(t, "get", "delta-99", "--format", "json")
	ctx = pack.ContextPack{}
	if err := json.Unmarshal(out, &ctx); err != nil {
		t.Fatalf("decode delta-99: %v", err)
	}
	if len(ctx.SearchMeta.RewritesApplied) != 0 {
		t.Fatalf("expected no rewrites for delta-99, got %+v", ctx.SearchMeta.RewritesApplied)
	}

	runGit(t, repoDir, "checkout", commitA)

	out = runCLI(t, "get", "delta99", "--format", "json")
	ctx = pack.ContextPack{}
	if err := json.Unmarshal(out, &ctx); err != nil {
		t.Fatalf("decode delta99 on commit A: %v", err)
	}
	if len(ctx.TopMemories) != 0 {
		t.Fatalf("expected no delta99 memories after checkout to commit A")
	}

	out = runCLI(t, "get", "delta99", "--format", "json", "--include-orphans")
	ctx = pack.ContextPack{}
	if err := json.Unmarshal(out, &ctx); err != nil {
		t.Fatalf("decode delta99 include-orphans: %v", err)
	}
	if len(ctx.TopMemories) == 0 || ctx.TopMemories[0].AnchorCommit != commitB {
		t.Fatalf("expected commit B memory when including orphans")
	}

	out = runCLI(t, "get", "alpha-note", "--format", "json")
	ctx = pack.ContextPack{}
	if err := json.Unmarshal(out, &ctx); err != nil {
		t.Fatalf("decode alpha-note: %v", err)
	}
	if len(ctx.TopMemories) == 0 || ctx.TopMemories[0].AnchorCommit != commitA {
		t.Fatalf("expected commit A memory after checkout, got %+v", ctx.TopMemories)
	}
}

func TestAcceptanceMCPContextMetaAndSources(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	writeTestConfig(t, base, func(cfg *config.Config) {
		cfg.EmbeddingProvider = "none"
	})

	runCLI(t, "init", "--no-agents")
	runCLI(t, "add", "--thread", "T-mcp", "--title", "Delta Entry", "--summary", "delta-99")

	if err := os.MkdirAll(filepath.Join(repoDir, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	writeFile(t, repoDir, filepath.Join("docs", "mcp-acceptance.txt"), "mcp acceptance chunk\nDelta-99 for rewrite coverage.\n")
	runCLI(t, "ingest-artifact", "docs/mcp-acceptance.txt", "--thread", "mcp-chunks-a")
	runCLI(t, "ingest-artifact", "docs/mcp-acceptance.txt", "--thread", "mcp-chunks-b")

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack_get_context",
			Arguments: map[string]any{"query": "delta99", "format": "json"},
		},
	}
	res, err := handleGetContext(context.Background(), req, false)
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
	if packRes.SearchMeta.ModeUsed != "bm25" {
		t.Fatalf("expected mode_used bm25, got %s", packRes.SearchMeta.ModeUsed)
	}
	if packRes.SearchMeta.FallbackReason != "" {
		t.Fatalf("expected empty fallback_reason, got %s", packRes.SearchMeta.FallbackReason)
	}
	if !containsString(packRes.SearchMeta.RewritesApplied, "delta99 -> delta 99") {
		t.Fatalf("expected rewrite metadata, got %+v", packRes.SearchMeta.RewritesApplied)
	}
	if len(packRes.TopChunks) == 0 {
		t.Fatalf("expected chunks for delta99")
	}
	foundMultiSource := false
	for _, chunk := range packRes.TopChunks {
		if len(chunk.Sources) >= 2 {
			foundMultiSource = true
			break
		}
	}
	if !foundMultiSource {
		t.Fatalf("expected deduped chunk sources, got %+v", packRes.TopChunks)
	}
}
