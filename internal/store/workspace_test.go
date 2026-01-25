package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestWorkspaceIsolationMemoriesAndThreads(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	createdAt := time.Now().UTC()
	_, err = st.AddMemory(AddMemoryInput{
		ID:            "M-A",
		RepoID:        "r1",
		Workspace:     "A",
		ThreadID:      "T1",
		Title:         "Alpha",
		Summary:       "alpha summary",
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     createdAt,
	})
	if err != nil {
		t.Fatalf("add memory A: %v", err)
	}

	_, err = st.AddMemory(AddMemoryInput{
		ID:            "M-B",
		RepoID:        "r1",
		Workspace:     "B",
		ThreadID:      "T1",
		Title:         "Bravo",
		Summary:       "bravo summary",
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     createdAt,
	})
	if err != nil {
		t.Fatalf("add memory B: %v", err)
	}

	resultsA, _, err := st.SearchMemories("r1", "A", "alpha", 10)
	if err != nil {
		t.Fatalf("search memories A: %v", err)
	}
	if len(resultsA) != 1 {
		t.Fatalf("expected 1 result in workspace A, got %d", len(resultsA))
	}

	resultsA, _, err = st.SearchMemories("r1", "A", "bravo", 10)
	if err != nil {
		t.Fatalf("search memories A for bravo: %v", err)
	}
	if len(resultsA) != 0 {
		t.Fatalf("expected 0 results for bravo in workspace A, got %d", len(resultsA))
	}

	resultsB, _, err := st.SearchMemories("r1", "B", "bravo", 10)
	if err != nil {
		t.Fatalf("search memories B: %v", err)
	}
	if len(resultsB) != 1 {
		t.Fatalf("expected 1 result in workspace B, got %d", len(resultsB))
	}

	threadsA, err := st.ListThreads("r1", "A")
	if err != nil {
		t.Fatalf("list threads A: %v", err)
	}
	if len(threadsA) != 1 {
		t.Fatalf("expected 1 thread in workspace A, got %d", len(threadsA))
	}
	if threadsA[0].Workspace != "A" {
		t.Fatalf("expected workspace A, got %s", threadsA[0].Workspace)
	}

	threadsB, err := st.ListThreads("r1", "B")
	if err != nil {
		t.Fatalf("list threads B: %v", err)
	}
	if len(threadsB) != 1 {
		t.Fatalf("expected 1 thread in workspace B, got %d", len(threadsB))
	}
	if threadsB[0].Workspace != "B" {
		t.Fatalf("expected workspace B, got %s", threadsB[0].Workspace)
	}
}
