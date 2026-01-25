package app

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"mempack/internal/pack"
	"mempack/internal/store"
	"mempack/internal/token"
)

type ExplainOptions struct {
	RepoOverride   string
	Workspace      string
	IncludeOrphans bool
}

func buildExplainReport(query string, opts ExplainOptions) (ExplainReport, error) {
	cfg, err := loadConfig()
	if err != nil {
		return ExplainReport{}, fmt.Errorf("config error: %v", err)
	}
	workspace := resolveWorkspace(cfg, opts.Workspace)

	repoInfo, err := resolveRepo(cfg, strings.TrimSpace(opts.RepoOverride))
	if err != nil {
		return ExplainReport{}, fmt.Errorf("repo detection error: %v", err)
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		return ExplainReport{}, fmt.Errorf("store open error: %v", err)
	}
	defer st.Close()

	if err := st.EnsureRepo(repoInfo); err != nil {
		return ExplainReport{}, fmt.Errorf("store repo error: %v", err)
	}

	stateRaw, stateTokens, _, err := loadState(repoInfo, workspace, st)
	if err != nil {
		return ExplainReport{}, fmt.Errorf("state error: %v", err)
	}

	parsed := store.ParseQuery(query)

	memResults, _, err := st.SearchMemories(repoInfo.ID, workspace, query, cfg.MemoriesK*5)
	if err != nil {
		return ExplainReport{}, fmt.Errorf("memory search error: %v", err)
	}

	chunkResults, _, err := st.SearchChunks(repoInfo.ID, workspace, query, cfg.ChunksK*5)
	if err != nil {
		return ExplainReport{}, fmt.Errorf("chunk search error: %v", err)
	}

	bm25Empty := len(memResults) == 0 && len(chunkResults) == 0
	vectorMinSimilarity := cfg.EmbeddingMinSimilarity
	vectorMemLimit := cfg.MemoriesK * 5
	vectorChunkLimit := cfg.ChunksK * 5
	if bm25Empty {
		vectorMinSimilarity = math.Max(0, vectorMinSimilarity-0.1)
		vectorMemLimit *= 2
		vectorChunkLimit *= 2
	}

	vectorMemResults, vectorStatus := vectorSearchMemories(cfg, st, repoInfo.ID, workspace, query, vectorMemLimit)
	vectorMemFiltered := filterVectorResults(vectorMemResults, vectorMinSimilarity)
	vectorMemOnly, err := loadVectorOnlyMemories(st, repoInfo.ID, workspace, memResults, vectorMemFiltered)
	if err != nil {
		return ExplainReport{}, fmt.Errorf("vector memory load error: %v", err)
	}

	rankOpts := RankOptions{
		IncludeOrphans:    opts.IncludeOrphans,
		VectorResults:     vectorMemResults,
		RecencyMultiplier: parsed.BoostRecency,
	}
	if parsed.TimeHint != nil {
		rankOpts.TimeFilter = &parsed.TimeHint.After
	}
	rankedMemories, matchedThreads, matchedThreadIDs, _, err := rankMemories(query, memResults, vectorMemOnly, repoInfo, rankOpts)
	if err != nil {
		return ExplainReport{}, fmt.Errorf("ranking error: %v", err)
	}
	vectorChunkResults, _ := vectorSearchChunks(cfg, st, repoInfo.ID, workspace, query, vectorChunkLimit)
	vectorChunkFiltered := filterVectorResults(vectorChunkResults, vectorMinSimilarity)
	vectorChunkOnly, err := loadVectorOnlyChunks(st, repoInfo.ID, workspace, chunkResults, vectorChunkFiltered)
	if err != nil {
		return ExplainReport{}, fmt.Errorf("vector chunk load error: %v", err)
	}
	chunkRankOpts := RankOptions{
		VectorResults:     vectorChunkResults,
		RecencyMultiplier: parsed.BoostRecency,
	}
	if parsed.TimeHint != nil {
		chunkRankOpts.TimeFilter = &parsed.TimeHint.After
	}
	rankedChunks := rankChunks(chunkResults, vectorChunkOnly, vectorChunkResults, matchedThreadIDs, chunkRankOpts)

	var counter TokenCounter
	budget, err := applyBudget(cfg, counter, stateRaw, stateTokens, rankedMemories, rankedChunks)
	if errors.Is(err, ErrTokenizerRequired) {
		counter, err = token.New(cfg.Tokenizer)
		if err != nil {
			return ExplainReport{}, fmt.Errorf("tokenizer error: %v", err)
		}
		budget, err = applyBudget(cfg, counter, stateRaw, stateTokens, rankedMemories, rankedChunks)
	}
	if err != nil {
		return ExplainReport{}, fmt.Errorf("budget error: %v", err)
	}

	memExplain := make([]ExplainMemory, 0, len(rankedMemories))
	for _, mem := range rankedMemories {
		_, included := budget.IncludedMemoryIDs[mem.Memory.ID]
		memExplain = append(memExplain, ExplainMemory{
			ID:           mem.Memory.ID,
			ThreadID:     mem.Memory.ThreadID,
			Title:        mem.Memory.Title,
			AnchorCommit: mem.Memory.AnchorCommit,
			BM25:         mem.BM25,
			FTSScore:     mem.FTSScore,
			FTSRank:      mem.FTSRank,
			VectorScore:  mem.VectorScore,
			VectorRank:   mem.VectorRank,
			RRFScore:     mem.RRFScore,
			RecencyBonus: mem.RecencyBonus,
			ThreadBonus:  mem.ThreadBonus,
			Superseded:   mem.Superseded,
			Orphaned:     mem.Orphaned,
			FinalScore:   mem.FinalScore,
			Included:     included,
		})
	}

	chunkExplain := make([]ExplainChunk, 0, len(rankedChunks))
	for _, chunk := range rankedChunks {
		_, included := budget.IncludedChunkIDs[chunk.Chunk.ID]
		chunkExplain = append(chunkExplain, ExplainChunk{
			ID:           chunk.Chunk.ID,
			ThreadID:     chunk.Chunk.ThreadID,
			Locator:      chunk.Chunk.Locator,
			BM25:         chunk.BM25,
			FTSScore:     chunk.FTSScore,
			FTSRank:      chunk.FTSRank,
			VectorScore:  chunk.VectorScore,
			VectorRank:   chunk.VectorRank,
			RRFScore:     chunk.RRFScore,
			RecencyBonus: chunk.RecencyBonus,
			ThreadBonus:  chunk.ThreadBonus,
			FinalScore:   chunk.FinalScore,
			Included:     included,
		})
	}

	report := ExplainReport{
		Query:          query,
		Repo:           pack.RepoInfo{RepoID: repoInfo.ID, GitRoot: repoInfo.GitRoot, Head: repoInfo.Head, Branch: repoInfo.Branch},
		Workspace:      workspace,
		MatchedThreads: matchedThreads,
		Memories:       memExplain,
		Chunks:         chunkExplain,
		Vector: VectorExplain{
			Provider:      vectorStatus.Provider,
			Model:         vectorStatus.Model,
			Enabled:       vectorStatus.Enabled,
			MinSimilarity: vectorStatus.MinSimilarity,
			Error:         vectorStatus.Error,
		},
		Budget: pack.BudgetInfo{
			Tokenizer:   cfg.Tokenizer,
			TargetTotal: cfg.TokenBudget,
			UsedTotal:   budget.UsedTokens,
		},
	}

	return report, nil
}
