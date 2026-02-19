package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestUpdateMemoryRecomputesFTS(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	createdAt := time.Now().UTC()
	mem, err := st.AddMemory(AddMemoryInput{
		RepoID:        "r1",
		Workspace:     "default",
		ThreadID:      "t1",
		Title:         "Session",
		Summary:       "",
		SummaryTokens: 0,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  EntitiesToJSON([]string{"file_src_index_ts"}),
		EntitiesText:  "file_src_index_ts",
		AnchorCommit:  "",
		CreatedAt:     createdAt,
	})
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}

	results, _, err := st.SearchMemories("r1", "default", "file_src_index_ts", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	_, changed, err := st.UpdateMemoryWithStatus(UpdateMemoryInput{
		RepoID:      "r1",
		Workspace:   "default",
		ID:          mem.ID,
		EntitiesSet: true,
		Entities:    []string{"file_src_app_ts"},
	})
	if err != nil {
		t.Fatalf("update memory: %v", err)
	}
	if !changed {
		t.Fatalf("expected update to be marked changed")
	}

	results, _, err = st.SearchMemories("r1", "default", "file_src_index_ts", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after update, got %d", len(results))
	}

	results, _, err = st.SearchMemories("r1", "default", "file_src_app_ts", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for updated entities, got %d", len(results))
	}
}

func TestUpdateMemoryTagsMerge(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	mem, err := st.AddMemory(AddMemoryInput{
		RepoID:        "r1",
		Workspace:     "default",
		ThreadID:      "t1",
		Title:         "Session",
		Summary:       "",
		SummaryTokens: 0,
		TagsJSON:      TagsToJSON([]string{"session"}),
		TagsText:      TagsText([]string{"session"}),
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}

	if _, _, err := st.UpdateMemoryWithStatus(UpdateMemoryInput{
		RepoID:     "r1",
		Workspace:  "default",
		ID:         mem.ID,
		TagsAdd:    []string{"needs_summary"},
		TagsRemove: []string{},
	}); err != nil {
		t.Fatalf("update memory: %v", err)
	}

	updated, err := st.GetMemory("r1", "default", mem.ID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	var tags []string
	if err := json.Unmarshal([]byte(updated.TagsJSON), &tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}

	if _, _, err := st.UpdateMemoryWithStatus(UpdateMemoryInput{
		RepoID:     "r1",
		Workspace:  "default",
		ID:         mem.ID,
		TagsRemove: []string{"session"},
	}); err != nil {
		t.Fatalf("update memory: %v", err)
	}
	updated, err = st.GetMemory("r1", "default", mem.ID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	tags = nil
	if err := json.Unmarshal([]byte(updated.TagsJSON), &tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "needs_summary" {
		t.Fatalf("expected tags to be [needs_summary], got %v", tags)
	}

	if _, _, err := st.UpdateMemoryWithStatus(UpdateMemoryInput{
		RepoID:     "r1",
		Workspace:  "default",
		ID:         mem.ID,
		TagsSet:    true,
		Tags:       []string{"session"},
		TagsAdd:    []string{"needs_summary"},
		TagsRemove: []string{"session"},
	}); err != nil {
		t.Fatalf("update memory: %v", err)
	}
	updated, err = st.GetMemory("r1", "default", mem.ID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	tags = nil
	if err := json.Unmarshal([]byte(updated.TagsJSON), &tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	if len(tags) != 1 || tags[0] != "session" {
		t.Fatalf("expected tags to be [session], got %v", tags)
	}
}

func TestUpdateMemorySummaryTokens(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	mem, err := st.AddMemory(AddMemoryInput{
		RepoID:        "r1",
		Workspace:     "default",
		ThreadID:      "t1",
		Title:         "Session",
		Summary:       "",
		SummaryTokens: 0,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}

	tokens := 2
	if _, _, err := st.UpdateMemoryWithStatus(UpdateMemoryInput{
		RepoID:        "r1",
		Workspace:     "default",
		ID:            mem.ID,
		Summary:       stringPtr("hello world"),
		SummaryTokens: &tokens,
	}); err != nil {
		t.Fatalf("update memory: %v", err)
	}

	updated, err := st.GetMemory("r1", "default", mem.ID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if updated.SummaryTokens != tokens {
		t.Fatalf("expected summary tokens %d, got %d", tokens, updated.SummaryTokens)
	}
	if updated.Summary != "hello world" {
		t.Fatalf("expected summary to be updated, got %q", updated.Summary)
	}
}

func stringPtr(value string) *string {
	return &value
}
