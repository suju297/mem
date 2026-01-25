package app

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func BenchmarkMCPGetContext(b *testing.B) {
	base := b.TempDir()
	setXDGEnv(b, base)

	repoDir := setupRepo(b, base)
	withCwd(b, repoDir)
	seedMemory(b, "decision", "Decision summary")

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack.get_context",
			Arguments: map[string]any{"query": "decision", "format": "json"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := handleGetContext(context.Background(), req); err != nil {
			b.Fatalf("get_context error: %v", err)
		}
	}
}

func BenchmarkMCPExplain(b *testing.B) {
	base := b.TempDir()
	setXDGEnv(b, base)

	repoDir := setupRepo(b, base)
	withCwd(b, repoDir)
	seedMemory(b, "decision", "Decision summary")

	req := mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      "mempack.explain",
			Arguments: map[string]any{"query": "decision"},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := handleExplain(context.Background(), req); err != nil {
			b.Fatalf("explain error: %v", err)
		}
	}
}
