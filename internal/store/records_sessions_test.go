package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestListAndCountSessionMemories(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	repoID := "r1"
	workspace := "default"

	t1 := time.Now().UTC().Add(-4 * time.Hour)
	t2 := time.Now().UTC().Add(-3 * time.Hour)
	t3 := time.Now().UTC().Add(-2 * time.Hour)
	t4 := time.Now().UTC().Add(-1 * time.Hour)
	t5 := time.Now().UTC()

	_, err = st.AddMemory(AddMemoryInput{
		ID:            "M-1",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T1",
		Title:         "Session 1",
		Summary:       "summary",
		SummaryTokens: 1,
		TagsJSON:      TagsToJSON([]string{"session"}),
		TagsText:      TagsText([]string{"session"}),
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     t1,
	})
	if err != nil {
		t.Fatalf("add session 1: %v", err)
	}

	mem2, err := st.AddMemory(AddMemoryInput{
		ID:            "M-2",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T1",
		Title:         "Session 2",
		Summary:       "",
		SummaryTokens: 0,
		TagsJSON:      TagsToJSON([]string{"session", "needs_summary"}),
		TagsText:      TagsText([]string{"session", "needs_summary"}),
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     t2,
	})
	if err != nil {
		t.Fatalf("add session 2: %v", err)
	}

	_, err = st.AddMemory(AddMemoryInput{
		ID:            "M-3",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T1",
		Title:         "Non-session",
		Summary:       "summary",
		SummaryTokens: 1,
		TagsJSON:      TagsToJSON([]string{"note"}),
		TagsText:      TagsText([]string{"note"}),
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     t3,
	})
	if err != nil {
		t.Fatalf("add non-session: %v", err)
	}

	mem4, err := st.AddMemory(AddMemoryInput{
		ID:            "M-4",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T1",
		Title:         "Superseded session",
		Summary:       "summary",
		SummaryTokens: 1,
		TagsJSON:      TagsToJSON([]string{"session", "needs_summary"}),
		TagsText:      TagsText([]string{"session", "needs_summary"}),
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     t4,
	})
	if err != nil {
		t.Fatalf("add superseded session: %v", err)
	}

	mem5, err := st.AddMemory(AddMemoryInput{
		ID:            "M-5",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T1",
		Title:         "Deleted session",
		Summary:       "summary",
		SummaryTokens: 1,
		TagsJSON:      TagsToJSON([]string{"session", "needs_summary"}),
		TagsText:      TagsText([]string{"session", "needs_summary"}),
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     t5,
	})
	if err != nil {
		t.Fatalf("add deleted session: %v", err)
	}

	if err := st.MarkMemorySuperseded(repoID, workspace, mem4.ID, "M-NEW"); err != nil {
		t.Fatalf("mark superseded: %v", err)
	}

	if ok, err := st.ForgetMemory(repoID, workspace, mem5.ID, time.Now().UTC()); err != nil || !ok {
		t.Fatalf("forget memory: %v", err)
	}

	sessions, err := st.ListSessionMemories(repoID, workspace, 10, false)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}
	if sessions[0].ID != mem2.ID || sessions[1].ID != "M-1" {
		t.Fatalf("unexpected session order: %v", sessions)
	}

	needs, err := st.ListSessionMemories(repoID, workspace, 10, true)
	if err != nil {
		t.Fatalf("list needs_summary sessions: %v", err)
	}
	if len(needs) != 1 || needs[0].ID != mem2.ID {
		t.Fatalf("expected only session with needs_summary, got %v", needs)
	}

	totalCount, err := st.CountSessionMemories(repoID, workspace, false)
	if err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if totalCount != 2 {
		t.Fatalf("expected count 2, got %d", totalCount)
	}

	needsCount, err := st.CountSessionMemories(repoID, workspace, true)
	if err != nil {
		t.Fatalf("count needs_summary sessions: %v", err)
	}
	if needsCount != 1 {
		t.Fatalf("expected needs_summary count 1, got %d", needsCount)
	}

	// Defensive compatibility: treat superseded_by='' the same as NULL.
	if _, err := st.db.Exec(`
		UPDATE memories
		SET superseded_by = ''
		WHERE repo_id = ? AND workspace = ? AND id = ?
	`, repoID, workspace, mem4.ID); err != nil {
		t.Fatalf("set superseded_by empty string: %v", err)
	}

	sessions, err = st.ListSessionMemories(repoID, workspace, 10, false)
	if err != nil {
		t.Fatalf("list sessions after empty-string superseded_by: %v", err)
	}
	if len(sessions) != 3 {
		t.Fatalf("expected 3 sessions after empty-string superseded_by, got %d", len(sessions))
	}

	totalCount, err = st.CountSessionMemories(repoID, workspace, false)
	if err != nil {
		t.Fatalf("count sessions after empty-string superseded_by: %v", err)
	}
	if totalCount != 3 {
		t.Fatalf("expected count 3 after empty-string superseded_by, got %d", totalCount)
	}
}

func TestListSessionMemoriesAllowsNullThreadID(t *testing.T) {
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
	`, "M-SESSION-NULL-THREAD", repoID, workspace, "Session no thread", "legacy row", 2, TagsToJSON([]string{"session"}), TagsText([]string{"session"}), "[]", "", now); err != nil {
		t.Fatalf("insert session memory with NULL thread_id: %v", err)
	}

	sessions, err := st.ListSessionMemories(repoID, workspace, 10, false)
	if err != nil {
		t.Fatalf("list sessions with NULL thread_id: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session memory, got %d", len(sessions))
	}
	if sessions[0].ThreadID != "" {
		t.Fatalf("expected empty thread id for NULL source row, got %q", sessions[0].ThreadID)
	}
}
