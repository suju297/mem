package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestForgetAndPurgeMemoryCleanupLinks(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	repoID := "r1"
	workspace := "default"
	now := time.Now().UTC()

	add := func(id string, createdAt time.Time) {
		t.Helper()
		if _, err := st.AddMemory(AddMemoryInput{
			ID:            id,
			RepoID:        repoID,
			Workspace:     workspace,
			ThreadID:      "T-LINK",
			Title:         id,
			Summary:       "summary",
			SummaryTokens: 1,
			TagsJSON:      "[]",
			TagsText:      "",
			EntitiesJSON:  "[]",
			EntitiesText:  "",
			CreatedAt:     createdAt,
		}); err != nil {
			t.Fatalf("add memory %s: %v", id, err)
		}
	}

	add("M-A", now)
	add("M-B", now.Add(time.Second))
	if err := st.AddLink(Link{FromID: "M-A", Rel: "depends_on", ToID: "M-B", Weight: 1, CreatedAt: now.Add(2 * time.Second)}); err != nil {
		t.Fatalf("add link A->B: %v", err)
	}
	if links, err := st.ListLinksForIDs([]string{"M-A", "M-B"}); err != nil || len(links) != 1 {
		t.Fatalf("expected one active link before forget, got len=%d err=%v", len(links), err)
	}
	if ok, err := st.ForgetMemory(repoID, workspace, "M-B", now.Add(3*time.Second)); err != nil || !ok {
		t.Fatalf("forget memory B: ok=%v err=%v", ok, err)
	}
	if links, err := st.ListLinksForIDs([]string{"M-A", "M-B"}); err != nil || len(links) != 0 {
		t.Fatalf("expected no links after forget cleanup, got len=%d err=%v", len(links), err)
	}

	add("M-C", now.Add(4*time.Second))
	add("M-D", now.Add(5*time.Second))
	if err := st.AddLink(Link{FromID: "M-C", Rel: "depends_on", ToID: "M-D", Weight: 1, CreatedAt: now.Add(6 * time.Second)}); err != nil {
		t.Fatalf("add link C->D: %v", err)
	}
	if ok, err := st.PurgeMemory(repoID, workspace, "M-D"); err != nil || !ok {
		t.Fatalf("purge memory D: ok=%v err=%v", ok, err)
	}
	if links, err := st.ListLinksForIDs([]string{"M-C", "M-D"}); err != nil || len(links) != 0 {
		t.Fatalf("expected no links after purge cleanup, got len=%d err=%v", len(links), err)
	}
}

func TestListLinksForIDsFiltersSupersededEndpoints(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	repoID := "r1"
	workspace := "default"
	now := time.Now().UTC()

	if _, err := st.AddMemory(AddMemoryInput{
		ID:            "M-FROM",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T-LINK",
		Title:         "From",
		Summary:       "summary",
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("add from memory: %v", err)
	}
	if _, err := st.AddMemory(AddMemoryInput{
		ID:            "M-TO",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T-LINK",
		Title:         "To",
		Summary:       "summary",
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     now.Add(time.Second),
	}); err != nil {
		t.Fatalf("add to memory: %v", err)
	}
	if err := st.AddLink(Link{FromID: "M-FROM", Rel: "depends_on", ToID: "M-TO", Weight: 1, CreatedAt: now.Add(2 * time.Second)}); err != nil {
		t.Fatalf("add link: %v", err)
	}

	if links, err := st.ListLinksForIDs([]string{"M-FROM", "M-TO"}); err != nil || len(links) != 1 {
		t.Fatalf("expected one active link before supersede, got len=%d err=%v", len(links), err)
	}
	if err := st.MarkMemorySuperseded(repoID, workspace, "M-TO", "M-NEW"); err != nil {
		t.Fatalf("mark memory superseded: %v", err)
	}
	if links, err := st.ListLinksForIDs([]string{"M-FROM", "M-TO"}); err != nil || len(links) != 0 {
		t.Fatalf("expected link to be filtered when endpoint is superseded, got len=%d err=%v", len(links), err)
	}
}
