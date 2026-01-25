package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMemoryFTSSyncOnUpdateAndDelete(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	createdAt := time.Now().UTC()
	mem, err := st.AddMemory(AddMemoryInput{
		RepoID:       "r1",
		Workspace:    "default",
		ThreadID:     "t1",
		Title:        "Auth decision",
		Summary:      "old summary",
		TagsJSON:     "[]",
		TagsText:     "auth",
		EntitiesJSON: "[]",
		EntitiesText: "",
		CreatedAt:    createdAt,
	})
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}

	results, _, err := st.SearchMemories("r1", "default", "old", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if _, err := st.db.Exec(`UPDATE memories SET summary = ?, tags_text = ? WHERE id = ?`, "new summary", "newtag", mem.ID); err != nil {
		t.Fatalf("update memory: %v", err)
	}

	results, _, err = st.SearchMemories("r1", "default", "old", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after update, got %d", len(results))
	}

	results, _, err = st.SearchMemories("r1", "default", "new", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result for updated summary, got %d", len(results))
	}

	ok, err := st.ForgetMemory("r1", "default", mem.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("forget memory: %v", err)
	}
	if !ok {
		t.Fatalf("expected memory to be forgotten")
	}

	results, _, err = st.SearchMemories("r1", "default", "new", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after delete, got %d", len(results))
	}
}

func TestChunkFTSSyncOnDelete(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	artifact := Artifact{
		ID:          NewID("A"),
		RepoID:      "r1",
		Workspace:   "default",
		Kind:        "file",
		Source:      "file.txt",
		ContentHash: "hash",
		CreatedAt:   time.Now().UTC(),
	}
	chunk := Chunk{
		ID:         NewID("C"),
		RepoID:     "r1",
		Workspace:  "default",
		ArtifactID: artifact.ID,
		ThreadID:   "t1",
		Locator:    "file:file.txt#L1-L2",
		Text:       "needle content",
		TagsJSON:   "[]",
		TagsText:   "",
		CreatedAt:  time.Now().UTC(),
	}

	if _, _, err := st.AddArtifactWithChunks(artifact, []Chunk{chunk}); err != nil {
		t.Fatalf("add artifact chunks: %v", err)
	}

	results, _, err := st.SearchChunks("r1", "default", "needle", 10)
	if err != nil {
		t.Fatalf("search chunks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(results))
	}

	ok, err := st.ForgetChunk("r1", "default", chunk.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("forget chunk: %v", err)
	}
	if !ok {
		t.Fatalf("expected chunk to be forgotten")
	}

	results, _, err = st.SearchChunks("r1", "default", "needle", 10)
	if err != nil {
		t.Fatalf("search chunks: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 chunks after delete, got %d", len(results))
	}
}

func TestChunkDeduplication(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	artifact1 := Artifact{
		ID:          NewID("A"),
		RepoID:      "r1",
		Workspace:   "default",
		Kind:        "file",
		Source:      "file.txt",
		ContentHash: "hash",
		CreatedAt:   time.Now().UTC(),
	}
	chunk := Chunk{
		ID:         NewID("C"),
		RepoID:     "r1",
		Workspace:  "default",
		ArtifactID: artifact1.ID,
		ThreadID:   "t1",
		Locator:    "file:file.txt#L1-L2",
		Text:       "duplicate content",
		TagsJSON:   "[]",
		TagsText:   "",
		CreatedAt:  time.Now().UTC(),
	}

	inserted1, _, err := st.AddArtifactWithChunks(artifact1, []Chunk{chunk})
	if err != nil {
		t.Fatalf("add artifact chunks: %v", err)
	}
	if inserted1 != 1 {
		t.Fatalf("expected 1 chunk inserted, got %d", inserted1)
	}

	artifact2 := Artifact{
		ID:          NewID("A"),
		RepoID:      "r1",
		Workspace:   "default",
		Kind:        "file",
		Source:      "file.txt",
		ContentHash: "hash",
		CreatedAt:   time.Now().UTC(),
	}
	chunk2 := Chunk{
		ID:         NewID("C"),
		RepoID:     "r1",
		Workspace:  "default",
		ArtifactID: artifact2.ID,
		ThreadID:   "t1",
		Locator:    "file:file.txt#L1-L2",
		Text:       "duplicate content",
		TagsJSON:   "[]",
		TagsText:   "",
		CreatedAt:  time.Now().UTC(),
	}

	inserted2, _, err := st.AddArtifactWithChunks(artifact2, []Chunk{chunk2})
	if err != nil {
		t.Fatalf("add artifact chunks: %v", err)
	}
	if inserted2 != 0 {
		t.Fatalf("expected duplicate chunk to be skipped, got %d", inserted2)
	}
}

func TestUserVersionSet(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	row := st.db.QueryRow("PRAGMA user_version;")
	var version int
	if err := row.Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != schemaVersion {
		t.Fatalf("expected user_version %d, got %d", schemaVersion, version)
	}
}
