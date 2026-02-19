package app

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"mempack/internal/config"
	"mempack/internal/embed"
	"mempack/internal/store"
)

type VectorResult struct {
	ID    string
	Score float64
}

type VectorSearchStatus struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	Enabled       bool    `json:"enabled"`
	MinSimilarity float64 `json:"min_similarity"`
	Error         string  `json:"error,omitempty"`
}

func vectorSearchMemories(cfg config.Config, st *store.Store, repoID, workspace, query string, limit int) ([]VectorResult, VectorSearchStatus) {
	provider, status := resolveVectorProvider(cfg)
	if !status.Enabled || provider == nil {
		return nil, status
	}

	model := strings.TrimSpace(status.Model)
	if model == "" {
		status.Error = "embedding model is not configured"
		return nil, status
	}
	embeddings, _, err := st.ListEmbeddingsForSearch(repoID, workspace, store.EmbeddingKindMemory, model)
	if err != nil {
		status.Error = fmt.Sprintf("embedding lookup failed: %v", err)
		return nil, status
	}
	if len(embeddings) == 0 {
		hasItems, err := st.HasEmbeddableItems(repoID, workspace, store.EmbeddingKindMemory)
		if err != nil {
			status.Error = fmt.Sprintf("embedding lookup failed: %v", err)
			return nil, status
		}
		if hasItems {
			status.Error = fmt.Sprintf("no embeddings stored for model %s (run: mem embed)", model)
		}
		return nil, status
	}

	queryVectors, err := provider.Embed([]string{query})
	if err != nil {
		status.Error = fmt.Sprintf("embedding query failed: %v", err)
		return nil, status
	}
	if len(queryVectors) == 0 {
		status.Error = "embedding query returned empty vector"
		return nil, status
	}

	results := scoreEmbeddings(queryVectors[0], embeddings, limit)
	return results, status
}

func vectorSearchChunks(cfg config.Config, st *store.Store, repoID, workspace, query string, limit int) ([]VectorResult, VectorSearchStatus) {
	provider, status := resolveVectorProvider(cfg)
	if !status.Enabled || provider == nil {
		return nil, status
	}

	model := strings.TrimSpace(status.Model)
	if model == "" {
		status.Error = "embedding model is not configured"
		return nil, status
	}
	embeddings, _, err := st.ListEmbeddingsForSearch(repoID, workspace, store.EmbeddingKindChunk, model)
	if err != nil {
		status.Error = fmt.Sprintf("embedding lookup failed: %v", err)
		return nil, status
	}
	if len(embeddings) == 0 {
		hasItems, err := st.HasEmbeddableItems(repoID, workspace, store.EmbeddingKindChunk)
		if err != nil {
			status.Error = fmt.Sprintf("embedding lookup failed: %v", err)
			return nil, status
		}
		if hasItems {
			status.Error = fmt.Sprintf("no embeddings stored for model %s (run: mem embed)", model)
		}
		return nil, status
	}

	queryVectors, err := provider.Embed([]string{query})
	if err != nil {
		status.Error = fmt.Sprintf("embedding query failed: %v", err)
		return nil, status
	}
	if len(queryVectors) == 0 {
		status.Error = "embedding query returned empty vector"
		return nil, status
	}

	results := scoreEmbeddings(queryVectors[0], embeddings, limit)
	return results, status
}

func resolveVectorProvider(cfg config.Config) (embed.Provider, VectorSearchStatus) {
	provider, status := embed.Resolve(cfg)
	minSimilarity := cfg.EmbeddingMinSimilarity
	if minSimilarity < 0 {
		minSimilarity = 0
	}
	return provider, VectorSearchStatus{
		Provider:      strings.TrimSpace(status.Provider),
		Model:         strings.TrimSpace(status.Model),
		Enabled:       status.Enabled,
		MinSimilarity: minSimilarity,
		Error:         status.Error,
	}
}

func filterVectorResults(results []VectorResult, minSimilarity float64) []VectorResult {
	if len(results) == 0 {
		return results
	}
	filtered := make([]VectorResult, 0, len(results))
	for _, res := range results {
		if res.Score >= minSimilarity {
			filtered = append(filtered, res)
		}
	}
	return filtered
}

func scoreEmbeddings(query []float64, embeddings []store.Embedding, limit int) []VectorResult {
	queryNorm := vectorNorm(query)
	if queryNorm == 0 {
		return nil
	}

	results := make([]VectorResult, 0, len(embeddings))
	for _, embedding := range embeddings {
		if embedding.VectorDim != len(query) {
			continue
		}
		score := cosineSimilarity(query, queryNorm, embedding.Vector)
		results = append(results, VectorResult{ID: embedding.ItemID, Score: score})
	}
	if len(results) == 0 {
		return nil
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].ID < results[j].ID
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func cosineSimilarity(query []float64, queryNorm float64, candidate []float64) float64 {
	if len(candidate) != len(query) {
		return 0
	}
	dot := 0.0
	candidateNorm := 0.0
	for i, value := range query {
		dot += value * candidate[i]
		candidateNorm += candidate[i] * candidate[i]
	}
	if dot == 0 || candidateNorm == 0 {
		return 0
	}
	return dot / (queryNorm * math.Sqrt(candidateNorm))
}

func vectorNorm(vector []float64) float64 {
	sum := 0.0
	for _, value := range vector {
		sum += value * value
	}
	if sum == 0 {
		return 0
	}
	return math.Sqrt(sum)
}

func loadVectorOnlyMemories(st *store.Store, repoID, workspace string, ftsResults []store.MemoryResult, vectorResults []VectorResult) ([]store.Memory, error) {
	if len(vectorResults) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(ftsResults))
	for _, res := range ftsResults {
		seen[res.Memory.ID] = struct{}{}
	}
	ids := vectorOnlyIDs(seen, vectorResults)
	if len(ids) == 0 {
		return nil, nil
	}
	return st.GetMemoriesByIDs(repoID, workspace, ids)
}

func loadVectorOnlyChunks(st *store.Store, repoID, workspace string, ftsResults []store.ChunkResult, vectorResults []VectorResult) ([]store.Chunk, error) {
	if len(vectorResults) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(ftsResults))
	for _, res := range ftsResults {
		seen[res.Chunk.ID] = struct{}{}
	}
	ids := vectorOnlyIDs(seen, vectorResults)
	if len(ids) == 0 {
		return nil, nil
	}
	return st.GetChunksByIDs(repoID, workspace, ids)
}

func vectorOnlyIDs(seen map[string]struct{}, vectorResults []VectorResult) []string {
	ids := make([]string, 0, len(vectorResults))
	for _, res := range vectorResults {
		if _, ok := seen[res.ID]; ok {
			continue
		}
		seen[res.ID] = struct{}{}
		ids = append(ids, res.ID)
	}
	return ids
}
