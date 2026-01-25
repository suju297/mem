package app

import (
	"encoding/json"
	"errors"
	"sort"
	"time"

	"mempack/internal/config"
	"mempack/internal/pack"
)

type BudgetResult struct {
	State             json.RawMessage
	StateTokens       int
	Memories          []pack.MemoryItem
	Chunks            []pack.ChunkItem
	UsedTokens        int
	IncludedMemoryIDs map[string]struct{}
	IncludedChunkIDs  map[string]struct{}
}

type TokenCounter interface {
	Count(text string) int
	Truncate(text string, maxTokens int) (string, int)
}

var ErrTokenizerRequired = errors.New("tokenizer required")

type budgetItem struct {
	Kind      string
	Index     int
	Tokens    int
	Score     float64
	CreatedAt time.Time
	ID        string
}

func applyBudget(cfg config.Config, counter TokenCounter, state json.RawMessage, stateTokens int, memories []RankedMemory, chunks []RankedChunk) (BudgetResult, error) {
	stateJSON, stateTokens, err := normalizeState(cfg, counter, state, stateTokens)
	if err != nil {
		return BudgetResult{}, err
	}

	memCount := cfg.MemoriesK
	if memCount > len(memories) {
		memCount = len(memories)
	}
	chunkCount := cfg.ChunksK
	if chunkCount > len(chunks) {
		chunkCount = len(chunks)
	}

	memItems := make([]pack.MemoryItem, 0, memCount)
	memTokens := make([]int, 0, memCount)
	for i := 0; i < memCount; i++ {
		mem := memories[i]
		truncated, tokens, err := summarizeMemory(counter, mem.Memory.Summary, mem.Memory.SummaryTokens, cfg.MemoryMaxEach)
		if err != nil {
			return BudgetResult{}, err
		}
		memItems = append(memItems, pack.MemoryItem{
			ID:           mem.Memory.ID,
			ThreadID:     mem.Memory.ThreadID,
			Title:        mem.Memory.Title,
			Summary:      truncated,
			AnchorCommit: mem.Memory.AnchorCommit,
		})
		memTokens = append(memTokens, tokens)
	}

	chunkItems := make([]pack.ChunkItem, 0, chunkCount)
	chunkTokens := make([]int, 0, chunkCount)
	for i := 0; i < chunkCount; i++ {
		chunk := chunks[i]
		truncated, tokens, err := summarizeChunk(counter, chunk.Chunk.Text, chunk.Chunk.TextTokens, cfg.ChunkMaxEach)
		if err != nil {
			return BudgetResult{}, err
		}
		chunkItems = append(chunkItems, pack.ChunkItem{
			ChunkID:  chunk.Chunk.ID,
			ThreadID: chunk.Chunk.ThreadID,
			Locator:  chunk.Chunk.Locator,
			Text:     truncated,
		})
		chunkTokens = append(chunkTokens, tokens)
	}

	usedTokens := stateTokens
	for _, tokens := range memTokens {
		usedTokens += tokens
	}
	for _, tokens := range chunkTokens {
		usedTokens += tokens
	}

	items := make([]budgetItem, 0, len(memItems)+len(chunkItems))
	for i, mem := range memItems {
		items = append(items, budgetItem{
			Kind:      "memory",
			Index:     i,
			Tokens:    memTokens[i],
			Score:     memories[i].FinalScore,
			CreatedAt: memories[i].Memory.CreatedAt,
			ID:        mem.ID,
		})
	}
	for i, chunk := range chunkItems {
		items = append(items, budgetItem{
			Kind:      "chunk",
			Index:     i,
			Tokens:    chunkTokens[i],
			Score:     chunks[i].FinalScore,
			CreatedAt: chunks[i].Chunk.CreatedAt,
			ID:        chunk.ChunkID,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if !items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].CreatedAt.After(items[j].CreatedAt)
		}
		return items[i].ID < items[j].ID
	})

	keepMemory := make(map[int]bool)
	keepChunk := make(map[int]bool)
	for _, item := range items {
		switch item.Kind {
		case "memory":
			keepMemory[item.Index] = true
		case "chunk":
			keepChunk[item.Index] = true
		}
	}

	for usedTokens > cfg.TokenBudget && len(items) > 0 {
		last := items[len(items)-1]
		usedTokens -= last.Tokens
		switch last.Kind {
		case "memory":
			delete(keepMemory, last.Index)
		case "chunk":
			delete(keepChunk, last.Index)
		}
		items = items[:len(items)-1]
	}

	selectedMemories := make([]pack.MemoryItem, 0, len(memItems))
	includedMemIDs := make(map[string]struct{})
	for i, item := range memItems {
		if !keepMemory[i] {
			continue
		}
		selectedMemories = append(selectedMemories, item)
		includedMemIDs[item.ID] = struct{}{}
	}

	selectedChunks := make([]pack.ChunkItem, 0, len(chunkItems))
	includedChunkIDs := make(map[string]struct{})
	for i, item := range chunkItems {
		if !keepChunk[i] {
			continue
		}
		selectedChunks = append(selectedChunks, item)
		includedChunkIDs[item.ChunkID] = struct{}{}
	}

	return BudgetResult{
		State:             stateJSON,
		StateTokens:       stateTokens,
		Memories:          selectedMemories,
		Chunks:            selectedChunks,
		UsedTokens:        usedTokens,
		IncludedMemoryIDs: includedMemIDs,
		IncludedChunkIDs:  includedChunkIDs,
	}, nil
}

func normalizeState(cfg config.Config, counter TokenCounter, state json.RawMessage, stateTokens int) (json.RawMessage, int, error) {
	if len(state) == 0 {
		state = json.RawMessage("{}")
	}
	if !json.Valid(state) {
		wrapped, err := json.Marshal(map[string]string{"raw": string(state)})
		if err == nil {
			state = wrapped
		}
		stateTokens = 0
	}

	stateStr := string(state)
	// Optimization: empty JSON object is 2 tokens in cl100k_base.
	// This avoids tokenizer init for the common empty state case.
	if stateTokens <= 0 && stateStr == "{}" {
		stateTokens = 2
	}

	if stateTokens <= 0 {
		if counter == nil {
			return state, 0, ErrTokenizerRequired
		}
		stateTokens = counter.Count(stateStr)
	}
	if stateTokens <= cfg.StateMax {
		return state, stateTokens, nil
	}
	if counter == nil {
		return state, stateTokens, ErrTokenizerRequired
	}

	limit := cfg.StateMax
	for limit > 0 {
		truncated, _ := counter.Truncate(stateStr, limit)
		wrapped, err := json.Marshal(map[string]any{
			"raw":       truncated,
			"truncated": true,
		})
		if err != nil {
			break
		}
		stateTokens = counter.Count(string(wrapped))
		state = wrapped
		if stateTokens <= cfg.StateMax || limit <= 10 {
			break
		}
		limit -= 10
	}

	return state, stateTokens, nil
}

func summarizeMemory(counter TokenCounter, summary string, summaryTokens, maxTokens int) (string, int, error) {
	if maxTokens <= 0 {
		return "", 0, nil
	}
	tokens := summaryTokens
	if tokens <= 0 {
		if counter == nil {
			return "", 0, ErrTokenizerRequired
		}
		tokens = counter.Count(summary)
	}
	if tokens <= maxTokens {
		return summary, tokens, nil
	}
	if counter == nil {
		return "", 0, ErrTokenizerRequired
	}
	truncated, truncatedTokens := counter.Truncate(summary, maxTokens)
	return truncated, truncatedTokens, nil
}

func summarizeChunk(counter TokenCounter, text string, textTokens, maxTokens int) (string, int, error) {
	if maxTokens <= 0 {
		return "", 0, nil
	}
	tokens := textTokens
	if tokens <= 0 {
		if counter == nil {
			return "", 0, ErrTokenizerRequired
		}
		tokens = counter.Count(text)
	}
	if tokens <= maxTokens {
		return text, tokens, nil
	}
	if counter == nil {
		return "", 0, ErrTokenizerRequired
	}
	truncated, truncatedTokens := counter.Truncate(text, maxTokens)
	return truncated, truncatedTokens, nil
}
