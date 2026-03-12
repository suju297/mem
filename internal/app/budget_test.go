package app

import (
	"strings"
	"testing"
	"time"

	"mem/internal/config"
	"mem/internal/store"
)

type fakeCounter struct{}

func (fakeCounter) Count(text string) int {
	return len(strings.Fields(text))
}

func (fakeCounter) Truncate(text string, maxTokens int) (string, int) {
	parts := strings.Fields(text)
	if maxTokens <= 0 {
		return "", 0
	}
	if len(parts) <= maxTokens {
		return text, len(parts)
	}
	return strings.Join(parts[:maxTokens], " "), maxTokens
}

func TestBudgetDropsLowestScore(t *testing.T) {
	cfg := config.Config{
		TokenBudget:   11,
		StateMax:      2,
		MemoryMaxEach: 5,
		MemoriesK:     3,
		ChunksK:       0,
		ChunkMaxEach:  0,
	}

	state := []byte("state")
	memories := []RankedMemory{
		{
			Memory:     store.Memory{ID: "M-1", Summary: "one two three four five", Title: "A", CreatedAt: time.Unix(10, 0)},
			FinalScore: 2,
		},
		{
			Memory:     store.Memory{ID: "M-2", Summary: "one two three four five", Title: "B", CreatedAt: time.Unix(9, 0)},
			FinalScore: 1,
		},
		{
			Memory:     store.Memory{ID: "M-3", Summary: "one two three four five", Title: "C", CreatedAt: time.Unix(11, 0)},
			FinalScore: 3,
		},
	}

	result, err := applyBudget(cfg, fakeCounter{}, state, 0, memories, nil)
	if err != nil {
		t.Fatalf("apply budget error: %v", err)
	}
	if result.UsedTokens > cfg.TokenBudget {
		t.Fatalf("expected budget <= %d, got %d", cfg.TokenBudget, result.UsedTokens)
	}
	if result.CandidateTokens != 16 {
		t.Fatalf("expected candidate tokens 16, got %d", result.CandidateTokens)
	}
	if result.PreBudgetTokens != 16 {
		t.Fatalf("expected pre-budget tokens 16, got %d", result.PreBudgetTokens)
	}
	if result.TruncatedTokens != 0 {
		t.Fatalf("expected truncated tokens 0, got %d", result.TruncatedTokens)
	}
	if result.DroppedTokens != 5 {
		t.Fatalf("expected dropped tokens 5, got %d", result.DroppedTokens)
	}
	if result.SavedTokens != 5 {
		t.Fatalf("expected saved tokens 5, got %d", result.SavedTokens)
	}
	if len(result.Memories) != 2 {
		t.Fatalf("expected 2 memories, got %d", len(result.Memories))
	}

	included := map[string]struct{}{}
	for _, mem := range result.Memories {
		included[mem.ID] = struct{}{}
	}
	if _, ok := included["M-3"]; !ok {
		t.Fatalf("expected M-3 to be included")
	}
	if _, ok := included["M-1"]; !ok {
		t.Fatalf("expected M-1 to be included")
	}
	if _, ok := included["M-2"]; ok {
		t.Fatalf("expected M-2 to be dropped")
	}
}

func TestBudgetTracksTruncationSavings(t *testing.T) {
	cfg := config.Config{
		TokenBudget:   100,
		StateMax:      10,
		MemoryMaxEach: 3,
		MemoriesK:     1,
		ChunksK:       0,
		ChunkMaxEach:  0,
	}

	memories := []RankedMemory{
		{
			Memory:     store.Memory{ID: "M-1", Summary: "one two three four five", Title: "A", CreatedAt: time.Unix(10, 0)},
			FinalScore: 1,
		},
	}

	result, err := applyBudget(cfg, fakeCounter{}, []byte("{}"), 0, memories, nil)
	if err != nil {
		t.Fatalf("apply budget error: %v", err)
	}
	if result.CandidateTokens != 7 {
		t.Fatalf("expected candidate tokens 7, got %d", result.CandidateTokens)
	}
	if result.PreBudgetTokens != 5 {
		t.Fatalf("expected pre-budget tokens 5, got %d", result.PreBudgetTokens)
	}
	if result.TruncatedTokens != 2 {
		t.Fatalf("expected truncated tokens 2, got %d", result.TruncatedTokens)
	}
	if result.DroppedTokens != 0 {
		t.Fatalf("expected dropped tokens 0, got %d", result.DroppedTokens)
	}
	if result.SavedTokens != 2 {
		t.Fatalf("expected saved tokens 2, got %d", result.SavedTokens)
	}
}

func BenchmarkBudgetPack(b *testing.B) {
	cfg := config.Config{
		TokenBudget:   2500,
		StateMax:      600,
		MemoryMaxEach: 80,
		MemoriesK:     100,
		ChunksK:       50,
		ChunkMaxEach:  320,
	}

	memories := make([]RankedMemory, 0, 100)
	for i := 0; i < 100; i++ {
		memories = append(memories, RankedMemory{
			Memory:     store.Memory{ID: "M-test", Summary: strings.Repeat("token ", 50), Title: "Title"},
			FinalScore: float64(100 - i),
		})
	}
	chunks := make([]RankedChunk, 0, 50)
	for i := 0; i < 50; i++ {
		chunks = append(chunks, RankedChunk{
			Chunk:      store.Chunk{ID: "C-test", Text: strings.Repeat("token ", 200)},
			FinalScore: float64(50 - i),
		})
	}

	state := []byte(`{"goal":"bench"}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := applyBudget(cfg, fakeCounter{}, state, 0, memories, chunks); err != nil {
			b.Fatalf("apply budget error: %v", err)
		}
	}
}
