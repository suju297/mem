package app

import (
	"mempack/internal/pack"
)

type ExplainOptions struct {
	RepoOverride   string
	Workspace      string
	IncludeOrphans bool
	RequireRepo    bool
}

func buildExplainReport(query string, opts ExplainOptions) (ExplainReport, error) {
	trace := retrievalTrace{}
	contextPack, err := buildContextPackWithTrace(query, ContextOptions{
		RepoOverride:   opts.RepoOverride,
		Workspace:      opts.Workspace,
		IncludeOrphans: opts.IncludeOrphans,
		RequireRepo:    opts.RequireRepo,
	}, nil, &trace)
	if err != nil {
		return ExplainReport{}, err
	}

	memExplain := make([]ExplainMemory, 0, len(trace.RankedMemories))
	for _, mem := range trace.RankedMemories {
		_, included := trace.Budget.IncludedMemoryIDs[mem.Memory.ID]
		memExplain = append(memExplain, ExplainMemory{
			ID:            mem.Memory.ID,
			ThreadID:      mem.Memory.ThreadID,
			Title:         mem.Memory.Title,
			AnchorCommit:  mem.Memory.AnchorCommit,
			BM25:          mem.BM25,
			FTSScore:      mem.FTSScore,
			FTSRank:       mem.FTSRank,
			VectorScore:   mem.VectorScore,
			VectorRank:    mem.VectorRank,
			RRFScore:      mem.RRFScore,
			RecencyBonus:  mem.RecencyBonus,
			ThreadBonus:   mem.ThreadBonus,
			SafetyPenalty: mem.SafetyPenalty,
			Superseded:    mem.Superseded,
			Orphaned:      mem.Orphaned,
			FinalScore:    mem.FinalScore,
			Included:      included,
		})
	}

	chunkExplain := make([]ExplainChunk, 0, len(trace.RankedChunks))
	for _, chunk := range trace.RankedChunks {
		_, included := trace.Budget.IncludedChunkIDs[chunk.Chunk.ID]
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
		Repo:           pack.RepoInfo{RepoID: contextPack.Repo.RepoID, GitRoot: contextPack.Repo.GitRoot, Head: contextPack.Repo.Head, Branch: contextPack.Repo.Branch},
		Workspace:      contextPack.Workspace,
		StateSource:    trace.StateSource,
		MatchedThreads: trace.MatchedThreads,
		Memories:       memExplain,
		Chunks:         chunkExplain,
		Vector: VectorExplain{
			Provider:      trace.VectorMemStatus.Provider,
			Model:         trace.VectorMemStatus.Model,
			Enabled:       trace.VectorMemStatus.Enabled,
			MinSimilarity: trace.VectorMemStatus.MinSimilarity,
			Error:         trace.VectorMemStatus.Error,
		},
		Budget: contextPack.Budget,
	}

	return report, nil
}
