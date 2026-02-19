package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestListRecentMemoriesOrdersAndFiltersDeleted(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	repoID := "r1"
	workspace := "default"

	t1 := time.Now().UTC().Add(-2 * time.Hour)
	t2 := time.Now().UTC().Add(-1 * time.Hour)
	t3 := time.Now().UTC()

	_, err = st.AddMemory(AddMemoryInput{
		ID:            "M-1",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T1",
		Title:         "Old",
		Summary:       "old summary",
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     t1,
	})
	if err != nil {
		t.Fatalf("add memory 1: %v", err)
	}

	mem2, err := st.AddMemory(AddMemoryInput{
		ID:            "M-2",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T1",
		Title:         "Middle",
		Summary:       "middle summary",
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     t2,
	})
	if err != nil {
		t.Fatalf("add memory 2: %v", err)
	}

	_, err = st.AddMemory(AddMemoryInput{
		ID:            "M-3",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T1",
		Title:         "Newest",
		Summary:       "new summary",
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     t3,
	})
	if err != nil {
		t.Fatalf("add memory 3: %v", err)
	}

	if ok, err := st.ForgetMemory(repoID, workspace, mem2.ID, time.Now().UTC()); err != nil || !ok {
		t.Fatalf("forget memory 2: %v", err)
	}

	recent, err := st.ListRecentMemories(repoID, workspace, 10)
	if err != nil {
		t.Fatalf("list recent: %v", err)
	}
	if len(recent) != 2 {
		t.Fatalf("expected 2 recent memories, got %d", len(recent))
	}
	if recent[0].ID != "M-3" || recent[1].ID != "M-1" {
		t.Fatalf("unexpected order: %v, %v", recent[0].ID, recent[1].ID)
	}

	limited, err := st.ListRecentMemories(repoID, workspace, 1)
	if err != nil {
		t.Fatalf("list recent limit: %v", err)
	}
	if len(limited) != 1 || limited[0].ID != "M-3" {
		t.Fatalf("expected newest memory, got %v", limited)
	}
}

func TestListRecentActiveThreadsExcludesSupersededMemories(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	repoID := "r1"
	workspace := "default"
	now := time.Now().UTC()

	// Thread T-SUPER has only a superseded memory and should be excluded.
	_, err = st.AddMemory(AddMemoryInput{
		ID:            "M-SUPER",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T-SUPER",
		Title:         "Superseded",
		Summary:       "old",
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("add superseded memory: %v", err)
	}
	if err := st.MarkMemorySuperseded(repoID, workspace, "M-SUPER", "M-NEW"); err != nil {
		t.Fatalf("mark superseded: %v", err)
	}

	// Thread T-ACTIVE has a non-superseded memory and should remain.
	_, err = st.AddMemory(AddMemoryInput{
		ID:            "M-ACTIVE",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T-ACTIVE",
		Title:         "Active",
		Summary:       "still valid",
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     now.Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("add active memory: %v", err)
	}

	threads, err := st.ListRecentActiveThreads(repoID, workspace, 10)
	if err != nil {
		t.Fatalf("list recent active threads: %v", err)
	}
	if len(threads) != 1 {
		t.Fatalf("expected only one active thread, got %d", len(threads))
	}
	if threads[0].ThreadID != "T-ACTIVE" {
		t.Fatalf("expected T-ACTIVE, got %s", threads[0].ThreadID)
	}
	if threads[0].MemoryCount != 1 {
		t.Fatalf("expected memory_count=1 for T-ACTIVE, got %d", threads[0].MemoryCount)
	}
}

func TestListRecentMemoriesAllowsNullThreadID(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	repoID := "r1"
	workspace := "default"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := st.db.Exec(`
		INSERT INTO memories (
			id, repo_id, workspace, thread_id, title, summary, summary_tokens,
			tags_json, tags_text, entities_json, entities_text, created_at
		)
		VALUES (?, ?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "M-NULL-THREAD", repoID, workspace, "No thread", "legacy row", 2, "[]", "", "[]", "", now); err != nil {
		t.Fatalf("insert memory with NULL thread_id: %v", err)
	}

	recent, err := st.ListRecentMemories(repoID, workspace, 10)
	if err != nil {
		t.Fatalf("list recent with NULL thread_id: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent memory, got %d", len(recent))
	}
	if recent[0].ThreadID != "" {
		t.Fatalf("expected empty thread id for NULL source row, got %q", recent[0].ThreadID)
	}
}
