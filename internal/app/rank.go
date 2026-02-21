package app

import (
	"math"
	"sort"
	"strings"
	"time"

	"mempack/internal/pack"
	"mempack/internal/repo"
	"mempack/internal/store"
)

type RankedMemory struct {
	Memory        store.Memory
	BM25          float64
	FTSScore      float64
	FTSRank       int
	VectorScore   float64
	VectorRank    int
	RRFScore      float64
	RecencyBonus  float64
	ThreadBonus   float64
	SafetyPenalty float64
	FinalScore    float64
	Orphaned      bool
	Superseded    bool
}

type RankedChunk struct {
	Chunk         store.Chunk
	BM25          float64
	FTSScore      float64
	FTSRank       int
	VectorScore   float64
	VectorRank    int
	RRFScore      float64
	RecencyBonus  float64
	ThreadBonus   float64
	SafetyPenalty float64
	FinalScore    float64
}

type RankStats struct {
	ReachabilityChecks int
	OrphansFiltered    int
	OrphanFilterTime   time.Duration
	ThreadMatchTime    time.Duration
}

const maxOrphanChecks = 200
const defaultRRFK = 60
const defaultRRFWeight = 60.0

type RankOptions struct {
	IncludeOrphans      bool
	VectorResults       []VectorResult
	VectorMinSimilarity float64
	RRFK                int
	RRFWeight           float64
	RecencyMultiplier   float64
	TimeFilter          *time.Time
}

func rankMemories(query string, results []store.MemoryResult, vectorOnly []store.Memory, repoInfo repo.Info, opts RankOptions) ([]RankedMemory, []pack.MatchedThread, map[string]struct{}, RankStats, error) {
	opts = normalizeRankOptions(opts)
	vectorResults := prepareVectorResults(opts.VectorResults, opts.VectorMinSimilarity)
	candidates := make([]RankedMemory, 0, len(results))
	reachability := make(map[string]bool)
	stats := RankStats{}
	seenIDs := make(map[string]struct{})
	ftsOrder := make([]string, 0, len(results))
	reachabilityChecks := 0

	filterStart := time.Now()
	checkOrphan := func(anchor string) (bool, error) {
		if !repoInfo.HasGit || anchor == "" || reachabilityChecks >= maxOrphanChecks {
			return false, nil
		}
		reachable, ok := reachability[anchor]
		if !ok {
			isAncestor, err := repo.IsAncestor(repoInfo.GitRoot, anchor, repoInfo.Head)
			if err != nil {
				return false, err
			}
			reachable = isAncestor
			reachability[anchor] = reachable
			reachabilityChecks++
			stats.ReachabilityChecks++
		}
		return !reachable, nil
	}

	for _, res := range results {
		orphaned, err := checkOrphan(res.AnchorCommit)
		if err != nil {
			return nil, nil, nil, stats, err
		}

		if orphaned && !opts.IncludeOrphans {
			stats.OrphansFiltered++
			continue
		}

		candidates = append(candidates, RankedMemory{Memory: res.Memory, BM25: res.BM25, Orphaned: orphaned})
		ftsOrder = append(ftsOrder, res.Memory.ID)
		seenIDs[res.Memory.ID] = struct{}{}
	}

	for _, mem := range vectorOnly {
		if _, ok := seenIDs[mem.ID]; ok {
			continue
		}
		orphaned, err := checkOrphan(mem.AnchorCommit)
		if err != nil {
			return nil, nil, nil, stats, err
		}
		if orphaned && !opts.IncludeOrphans {
			stats.OrphansFiltered++
			continue
		}
		candidates = append(candidates, RankedMemory{Memory: mem, Orphaned: orphaned})
		seenIDs[mem.ID] = struct{}{}
	}
	stats.OrphanFilterTime = time.Since(filterStart)

	threadStart := time.Now()
	matchedThreads, matchedThreadIDs := matchThreads(query, candidates)
	stats.ThreadMatchTime = time.Since(threadStart)

	ftsRanks := make(map[string]int, len(ftsOrder))
	for idx, id := range ftsOrder {
		ftsRanks[id] = idx + 1
	}
	vectorRanks := make(map[string]int, len(vectorResults))
	vectorScores := make(map[string]float64, len(vectorResults))
	for idx, res := range vectorResults {
		if _, ok := vectorRanks[res.ID]; ok {
			continue
		}
		vectorRanks[res.ID] = idx + 1
		vectorScores[res.ID] = res.Score
	}

	now := time.Now().UTC()
	recencyMult := opts.RecencyMultiplier
	if recencyMult <= 0 {
		recencyMult = 1.0
	}
	for i := range candidates {
		mem := &candidates[i]
		mem.FTSScore = -mem.BM25
		mem.FTSRank = ftsRanks[mem.Memory.ID]
		mem.VectorRank = vectorRanks[mem.Memory.ID]
		mem.VectorScore = vectorScores[mem.Memory.ID]
		mem.RRFScore = (rrfScore(mem.FTSRank, opts.RRFK) + rrfScore(mem.VectorRank, opts.RRFK)) * opts.RRFWeight
		mem.RecencyBonus = recencyBonus(now, mem.Memory.CreatedAt) * recencyMult
		if _, ok := matchedThreadIDs[mem.Memory.ThreadID]; ok {
			mem.ThreadBonus = 0.10
		}
		if mem.Memory.SupersededBy != "" {
			mem.Superseded = true
		}
		if containsPromptInjectionPhrase(mem.Memory.Title) || containsPromptInjectionPhrase(mem.Memory.Summary) {
			mem.SafetyPenalty = -100.0
		}
		mem.FinalScore = mem.RRFScore + mem.RecencyBonus + mem.ThreadBonus + mem.SafetyPenalty
		if opts.TimeFilter != nil && mem.Memory.CreatedAt.Before(*opts.TimeFilter) {
			mem.FinalScore -= 2.0
		}
		if mem.Superseded {
			mem.FinalScore -= 5.0
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].FinalScore != candidates[j].FinalScore {
			return candidates[i].FinalScore > candidates[j].FinalScore
		}
		if !candidates[i].Memory.CreatedAt.Equal(candidates[j].Memory.CreatedAt) {
			return candidates[i].Memory.CreatedAt.After(candidates[j].Memory.CreatedAt)
		}
		return candidates[i].Memory.ID < candidates[j].Memory.ID
	})

	return candidates, matchedThreads, matchedThreadIDs, stats, nil
}

func rankChunks(results []store.ChunkResult, vectorOnly []store.Chunk, vectorResults []VectorResult, matchedThreadIDs map[string]struct{}, opts RankOptions) []RankedChunk {
	opts = normalizeRankOptions(opts)
	vectorResults = prepareVectorResults(vectorResults, opts.VectorMinSimilarity)
	now := time.Now().UTC()
	recencyMult := opts.RecencyMultiplier
	if recencyMult <= 0 {
		recencyMult = 1.0
	}
	candidates := make([]RankedChunk, 0, len(results))
	ftsOrder := make([]string, 0, len(results))
	seenIDs := make(map[string]struct{})

	for _, res := range results {
		chunk := RankedChunk{Chunk: res.Chunk, BM25: res.BM25}
		chunk.FTSScore = -chunk.BM25
		chunk.RecencyBonus = recencyBonus(now, chunk.Chunk.CreatedAt) * recencyMult
		if _, ok := matchedThreadIDs[chunk.Chunk.ThreadID]; ok {
			chunk.ThreadBonus = 0.10
		}

		if containsPromptInjectionPhrase(chunk.Chunk.Text) {
			chunk.SafetyPenalty = -100.0
		}
		candidates = append(candidates, chunk)
		ftsOrder = append(ftsOrder, res.Chunk.ID)
		seenIDs[res.Chunk.ID] = struct{}{}
	}

	for _, res := range vectorOnly {
		if _, ok := seenIDs[res.ID]; ok {
			continue
		}
		chunk := RankedChunk{Chunk: res}
		chunk.FTSScore = -chunk.BM25
		chunk.RecencyBonus = recencyBonus(now, chunk.Chunk.CreatedAt) * recencyMult
		if _, ok := matchedThreadIDs[chunk.Chunk.ThreadID]; ok {
			chunk.ThreadBonus = 0.10
		}

		if containsPromptInjectionPhrase(chunk.Chunk.Text) {
			chunk.SafetyPenalty = -100.0
		}
		candidates = append(candidates, chunk)
		seenIDs[res.ID] = struct{}{}
	}

	ftsRanks := make(map[string]int, len(ftsOrder))
	for idx, id := range ftsOrder {
		ftsRanks[id] = idx + 1
	}
	vectorRanks := make(map[string]int, len(vectorResults))
	vectorScores := make(map[string]float64, len(vectorResults))
	for idx, res := range vectorResults {
		if _, ok := vectorRanks[res.ID]; ok {
			continue
		}
		vectorRanks[res.ID] = idx + 1
		vectorScores[res.ID] = res.Score
	}

	for i := range candidates {
		chunk := &candidates[i]
		chunk.FTSRank = ftsRanks[chunk.Chunk.ID]
		chunk.VectorRank = vectorRanks[chunk.Chunk.ID]
		chunk.VectorScore = vectorScores[chunk.Chunk.ID]
		chunk.RRFScore = (rrfScore(chunk.FTSRank, opts.RRFK) + rrfScore(chunk.VectorRank, opts.RRFK)) * opts.RRFWeight
		chunk.FinalScore = chunk.RRFScore + chunk.RecencyBonus + chunk.ThreadBonus + chunk.SafetyPenalty
		if opts.TimeFilter != nil && chunk.Chunk.CreatedAt.Before(*opts.TimeFilter) {
			chunk.FinalScore -= 2.0
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].FinalScore != candidates[j].FinalScore {
			return candidates[i].FinalScore > candidates[j].FinalScore
		}
		if !candidates[i].Chunk.CreatedAt.Equal(candidates[j].Chunk.CreatedAt) {
			return candidates[i].Chunk.CreatedAt.After(candidates[j].Chunk.CreatedAt)
		}
		return candidates[i].Chunk.ID < candidates[j].Chunk.ID
	})

	return candidates
}

func matchThreads(query string, memories []RankedMemory) ([]pack.MatchedThread, map[string]struct{}) {
	matchedIDs := make(map[string]struct{})
	reasons := make(map[string]string)
	counts := make(map[string]int)
	lowerQuery := strings.ToLower(query)

	for _, mem := range memories {
		threadID := mem.Memory.ThreadID
		if threadID == "" {
			continue
		}
		counts[threadID]++
		if lowerQuery != "" && strings.Contains(lowerQuery, strings.ToLower(threadID)) {
			reasons[threadID] = "query matched thread id"
		}
	}

	type pair struct {
		id    string
		count int
	}
	pairs := make([]pair, 0, len(counts))
	for id, count := range counts {
		pairs = append(pairs, pair{id: id, count: count})
	}

	sort.SliceStable(pairs, func(i, j int) bool {
		if pairs[i].count != pairs[j].count {
			return pairs[i].count > pairs[j].count
		}
		return pairs[i].id < pairs[j].id
	})

	matched := make([]pack.MatchedThread, 0, 3)
	for _, p := range pairs {
		reason, ok := reasons[p.id]
		if !ok {
			reason = "top retrieved thread"
		}
		matched = append(matched, pack.MatchedThread{ThreadID: p.id, Why: reason})
		matchedIDs[p.id] = struct{}{}
		if len(matched) >= 3 {
			break
		}
	}

	for id, reason := range reasons {
		if _, ok := matchedIDs[id]; ok {
			continue
		}
		matched = append(matched, pack.MatchedThread{ThreadID: id, Why: reason})
		matchedIDs[id] = struct{}{}
	}

	return matched, matchedIDs
}

func recencyBonus(now time.Time, createdAt time.Time) float64 {
	if createdAt.IsZero() {
		return 0
	}
	ageDays := now.Sub(createdAt).Hours() / 24
	return 0.15 * math.Exp(-ageDays/14)
}

func normalizeRankOptions(opts RankOptions) RankOptions {
	if opts.RRFK <= 0 {
		opts.RRFK = defaultRRFK
	}
	if opts.RRFWeight <= 0 {
		opts.RRFWeight = defaultRRFWeight
	}
	if opts.VectorMinSimilarity < 0 {
		opts.VectorMinSimilarity = 0
	}
	return opts
}

func prepareVectorResults(results []VectorResult, minSimilarity float64) []VectorResult {
	if len(results) == 0 {
		return results
	}
	if minSimilarity < 0 {
		minSimilarity = 0
	}
	filtered := make([]VectorResult, 0, len(results))
	for _, res := range results {
		if res.Score >= minSimilarity {
			filtered = append(filtered, res)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	sorted := append([]VectorResult(nil), filtered...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Score != sorted[j].Score {
			return sorted[i].Score > sorted[j].Score
		}
		return sorted[i].ID < sorted[j].ID
	})
	return sorted
}

func rrfScore(rank int, k int) float64 {
	if rank <= 0 {
		return 0
	}
	return 1.0 / float64(k+rank)
}
