package app

import (
	"math"
	"sort"

	"mempack/internal/pack"
)

const (
	clusterSimilarityThreshold = 0.75
	minClusterSize             = 2
	maxClusters                = 10
)

type MemoryCluster struct {
	Representative pack.MemoryItem
	Members        []pack.MemoryItem
	Similarity     float64
}

// ClusterMemories groups similar memories based on embedding similarity.
// Returns clusters (grouped) + unclustered (standalone) memories.
func ClusterMemories(memories []pack.MemoryItem, embeddings map[string][]float64) ([]MemoryCluster, []pack.MemoryItem) {
	if len(memories) < minClusterSize || len(embeddings) == 0 {
		return nil, memories
	}

	n := len(memories)
	sim := make([][]float64, n)
	for i := range sim {
		sim[i] = make([]float64, n)
	}

	for i := 0; i < n; i++ {
		vecI := embeddings[memories[i].ID]
		if len(vecI) == 0 {
			continue
		}
		for j := i + 1; j < n; j++ {
			vecJ := embeddings[memories[j].ID]
			if len(vecJ) == 0 {
				continue
			}
			s := cosineSimilarityPair(vecI, vecJ)
			if s == 0 {
				continue
			}
			sim[i][j] = s
			sim[j][i] = s
		}
	}

	assigned := make([]int, n) // cluster ID for each memory, 0 = unassigned
	clusterID := 1

	for {
		bestI, bestJ, bestSim := -1, -1, 0.0
		for i := 0; i < n; i++ {
			if assigned[i] != 0 {
				continue
			}
			for j := i + 1; j < n; j++ {
				if assigned[j] != 0 {
					continue
				}
				if sim[i][j] > bestSim {
					bestI, bestJ, bestSim = i, j, sim[i][j]
				}
			}
		}

		if bestSim < clusterSimilarityThreshold {
			break
		}

		assigned[bestI] = clusterID
		assigned[bestJ] = clusterID

		for k := 0; k < n; k++ {
			if assigned[k] != 0 {
				continue
			}
			if sim[k][bestI] >= clusterSimilarityThreshold && sim[k][bestJ] >= clusterSimilarityThreshold {
				assigned[k] = clusterID
			}
		}

		clusterID++
		if clusterID > maxClusters {
			break
		}
	}

	clusterMap := make(map[int][]int)
	for i, cid := range assigned {
		if cid > 0 {
			clusterMap[cid] = append(clusterMap[cid], i)
		}
	}

	clusters := make([]MemoryCluster, 0, len(clusterMap))
	unclustered := make([]pack.MemoryItem, 0, len(memories))
	processed := make(map[int]struct{}, len(clusterMap))

	for i := 0; i < n; i++ {
		cid := assigned[i]
		if cid == 0 {
			continue
		}
		if _, ok := processed[cid]; ok {
			continue
		}
		processed[cid] = struct{}{}

		indices := clusterMap[cid]
		sort.Ints(indices)
		if len(indices) < minClusterSize {
			for _, idx := range indices {
				assigned[idx] = 0
			}
			continue
		}

		members := make([]pack.MemoryItem, len(indices))
		for j, idx := range indices {
			members[j] = memories[idx]
		}

		avgSim := 0.0
		count := 0
		for a := 0; a < len(indices); a++ {
			for b := a + 1; b < len(indices); b++ {
				avgSim += sim[indices[a]][indices[b]]
				count++
			}
		}
		if count > 0 {
			avgSim /= float64(count)
		}

		clusters = append(clusters, MemoryCluster{
			Representative: members[0],
			Members:        members,
			Similarity:     avgSim,
		})
	}

	for i, cid := range assigned {
		if cid == 0 {
			unclustered = append(unclustered, memories[i])
		}
	}

	return clusters, unclustered
}

func cosineSimilarityPair(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	dot := 0.0
	normA := 0.0
	normB := 0.0
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
