package app

import (
	"sort"
	"testing"

	"mempack/internal/pack"
)

func TestClusterMemories(t *testing.T) {
	memories := []pack.MemoryItem{
		{ID: "M-1", Title: "Auth A"},
		{ID: "M-2", Title: "Auth B"},
		{ID: "M-3", Title: "Auth C"},
		{ID: "M-4", Title: "Database X"},
		{ID: "M-5", Title: "Database Y"},
	}

	embeddings := map[string][]float64{
		"M-1": {1.0, 0.0, 0.0},
		"M-2": {0.95, 0.05, 0.0},
		"M-3": {0.9, 0.1, 0.0},
		"M-4": {0.0, 1.0, 0.0},
		"M-5": {0.0, 0.95, 0.05},
	}

	clusters, unclustered := ClusterMemories(memories, embeddings)

	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}
	if len(unclustered) != 0 {
		t.Fatalf("expected 0 unclustered, got %d", len(unclustered))
	}

	sizes := []int{len(clusters[0].Members), len(clusters[1].Members)}
	sort.Ints(sizes)
	if sizes[0] != 2 || sizes[1] != 3 {
		t.Fatalf("expected cluster sizes [2 3], got %v", sizes)
	}
}

func TestClusterMemoriesNoEmbeddings(t *testing.T) {
	memories := []pack.MemoryItem{
		{ID: "M-1", Title: "Auth A"},
		{ID: "M-2", Title: "Auth B"},
	}

	clusters, unclustered := ClusterMemories(memories, nil)

	if len(clusters) != 0 {
		t.Fatalf("expected 0 clusters without embeddings, got %d", len(clusters))
	}
	if len(unclustered) != 2 {
		t.Fatalf("expected 2 unclustered, got %d", len(unclustered))
	}
}

func TestClusterMemoriesBelowThreshold(t *testing.T) {
	memories := []pack.MemoryItem{
		{ID: "M-1"},
		{ID: "M-2"},
	}

	embeddings := map[string][]float64{
		"M-1": {1.0, 0.0, 0.0},
		"M-2": {0.0, 1.0, 0.0},
	}

	clusters, unclustered := ClusterMemories(memories, embeddings)

	if len(clusters) != 0 {
		t.Fatalf("expected 0 clusters for dissimilar embeddings, got %d", len(clusters))
	}
	if len(unclustered) != 2 {
		t.Fatalf("expected 2 unclustered, got %d", len(unclustered))
	}
}
