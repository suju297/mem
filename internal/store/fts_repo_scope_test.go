package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestSearchMemoriesRepoScopedFTS(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "memory.db")
	st, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	now := time.Now().UTC()
	_, err = st.AddMemory(AddMemoryInput{
		RepoID:       "r1",
		Workspace:    "default",
		ThreadID:     "T-1",
		Title:        "Memory One",
		Summary:      "shared term",
		TagsJSON:     "[]",
		TagsText:     "",
		EntitiesJSON: "[]",
		EntitiesText: "",
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("add memory r1: %v", err)
	}
	_, err = st.AddMemory(AddMemoryInput{
		RepoID:       "r2",
		Workspace:    "default",
		ThreadID:     "T-2",
		Title:        "Memory Two",
		Summary:      "shared term",
		TagsJSON:     "[]",
		TagsText:     "",
		EntitiesJSON: "[]",
		EntitiesText: "",
		CreatedAt:    now,
	})
	if err != nil {
		t.Fatalf("add memory r2: %v", err)
	}

	results, stats, err := st.SearchMemories("r1", "default", "shared", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if stats.CandidateCount != 1 {
		t.Fatalf("expected candidate count 1 for repo-scoped FTS, got %d", stats.CandidateCount)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].RepoID != "r1" {
		t.Fatalf("expected repo_id r1, got %s", results[0].RepoID)
	}
}
