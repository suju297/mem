package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mempack/internal/store"
)

type testEmbedProvider struct {
	calls int
}

func (p *testEmbedProvider) Name() string {
	return "test"
}

func (p *testEmbedProvider) Embed(texts []string) ([][]float64, error) {
	p.calls++
	vectors := make([][]float64, 0, len(texts))
	for i := range texts {
		vectors = append(vectors, []float64{float64(i + 1), 1})
	}
	return vectors, nil
}

func TestProcessEmbeddingQueueEmbedsMemoryAndChunk(t *testing.T) {
	st, repoID, workspace, model := setupEmbeddingStore(t)
	now := time.Now().UTC()

	mem, err := st.AddMemory(store.AddMemoryInput{
		ID:            "M-QUEUE-1",
		RepoID:        repoID,
		Workspace:     workspace,
		ThreadID:      "T-QUEUE",
		Title:         "Memory title",
		Summary:       "Memory summary",
		SummaryTokens: 2,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		CreatedAt:     now,
	})
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}

	artifact := store.Artifact{
		ID:          "A-QUEUE-1",
		RepoID:      repoID,
		Workspace:   workspace,
		Kind:        "file",
		Source:      "queue.txt",
		ContentHash: "hash",
		CreatedAt:   now,
	}
	chunk := store.Chunk{
		ID:         "C-QUEUE-1",
		RepoID:     repoID,
		Workspace:  workspace,
		ArtifactID: artifact.ID,
		ThreadID:   "T-QUEUE",
		Locator:    "queue.txt#L1",
		Text:       "chunk content",
		TagsJSON:   "[]",
		TagsText:   "",
		CreatedAt:  now,
	}
	if _, _, err := st.AddArtifactWithChunks(artifact, []store.Chunk{chunk}); err != nil {
		t.Fatalf("add artifact/chunk: %v", err)
	}

	if err := st.EnqueueEmbedding(store.EmbeddingQueueItem{
		RepoID:    repoID,
		Workspace: workspace,
		Kind:      store.EmbeddingKindMemory,
		ItemID:    mem.ID,
		Model:     model,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("enqueue memory: %v", err)
	}
	if err := st.EnqueueEmbedding(store.EmbeddingQueueItem{
		RepoID:    repoID,
		Workspace: workspace,
		Kind:      store.EmbeddingKindChunk,
		ItemID:    chunk.ID,
		Model:     model,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("enqueue chunk: %v", err)
	}

	items, err := st.ListEmbeddingQueue(repoID, model, 10)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}

	provider := &testEmbedProvider{}
	if err := processEmbeddingQueue(provider, st, items); err != nil {
		t.Fatalf("process queue: %v", err)
	}

	queueDepth, err := st.CountEmbeddingQueue(repoID, model)
	if err != nil {
		t.Fatalf("count queue: %v", err)
	}
	if queueDepth != 0 {
		t.Fatalf("expected queue depth 0, got %d", queueDepth)
	}

	memEmbeddings, _, err := st.ListEmbeddingsForSearch(repoID, workspace, store.EmbeddingKindMemory, model)
	if err != nil {
		t.Fatalf("list memory embeddings: %v", err)
	}
	if len(memEmbeddings) != 1 {
		t.Fatalf("expected 1 memory embedding, got %d", len(memEmbeddings))
	}

	chunkEmbeddings, _, err := st.ListEmbeddingsForSearch(repoID, workspace, store.EmbeddingKindChunk, model)
	if err != nil {
		t.Fatalf("list chunk embeddings: %v", err)
	}
	if len(chunkEmbeddings) != 1 {
		t.Fatalf("expected 1 chunk embedding, got %d", len(chunkEmbeddings))
	}
}

func TestProcessEmbeddingQueueRejectsUnknownKind(t *testing.T) {
	st, repoID, workspace, model := setupEmbeddingStore(t)
	if err := st.EnqueueEmbedding(store.EmbeddingQueueItem{
		RepoID:    repoID,
		Workspace: workspace,
		Kind:      "unknown-kind",
		ItemID:    "X-1",
		Model:     model,
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("enqueue unknown kind: %v", err)
	}

	items, err := st.ListEmbeddingQueue(repoID, model, 10)
	if err != nil {
		t.Fatalf("list queue: %v", err)
	}

	provider := &testEmbedProvider{}
	err = processEmbeddingQueue(provider, st, items)
	if err == nil || !strings.Contains(err.Error(), "unsupported embedding queue kind") {
		t.Fatalf("expected unsupported kind error, got: %v", err)
	}

	queueDepth, err := st.CountEmbeddingQueue(repoID, model)
	if err != nil {
		t.Fatalf("count queue: %v", err)
	}
	if queueDepth != 1 {
		t.Fatalf("expected unknown item to remain queued, got depth=%d", queueDepth)
	}
}

func TestEmbedMissingMemoriesFetchesInBatches(t *testing.T) {
	st, repoID, workspace, model := setupEmbeddingStore(t)
	total := embedFetchLimit + 11
	now := time.Now().UTC()

	for i := 0; i < total; i++ {
		_, err := st.AddMemory(store.AddMemoryInput{
			ID:            fmt.Sprintf("M-BATCH-%03d", i),
			RepoID:        repoID,
			Workspace:     workspace,
			ThreadID:      "T-BATCH",
			Title:         fmt.Sprintf("Memory %03d", i),
			Summary:       fmt.Sprintf("summary %03d", i),
			SummaryTokens: 2,
			TagsJSON:      "[]",
			TagsText:      "",
			EntitiesJSON:  "[]",
			EntitiesText:  "",
			CreatedAt:     now.Add(time.Duration(i) * time.Second),
		})
		if err != nil {
			t.Fatalf("add memory %d: %v", i, err)
		}
	}

	provider := &testEmbedProvider{}
	embedded, err := embedMissingMemories(provider, st, repoID, workspace, model)
	if err != nil {
		t.Fatalf("embed missing memories: %v", err)
	}
	if embedded != total {
		t.Fatalf("expected %d embeddings, got %d", total, embedded)
	}
	if provider.calls < 2 {
		t.Fatalf("expected multiple provider calls, got %d", provider.calls)
	}

	remaining, err := st.ListMemoriesMissingEmbedding(repoID, workspace, model, 0)
	if err != nil {
		t.Fatalf("list missing memories: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no missing memory embeddings, got %d", len(remaining))
	}
}

func TestEmbedMissingChunksFetchesInBatches(t *testing.T) {
	st, repoID, workspace, model := setupEmbeddingStore(t)
	total := embedFetchLimit + 11
	now := time.Now().UTC()
	artifact := store.Artifact{
		ID:          "A-BATCH-CHUNKS",
		RepoID:      repoID,
		Workspace:   workspace,
		Kind:        "file",
		Source:      "chunks.txt",
		ContentHash: "hash",
		CreatedAt:   now,
	}
	chunks := make([]store.Chunk, 0, total)
	for i := 0; i < total; i++ {
		chunks = append(chunks, store.Chunk{
			ID:         fmt.Sprintf("C-BATCH-%03d", i),
			RepoID:     repoID,
			Workspace:  workspace,
			ArtifactID: artifact.ID,
			ThreadID:   "T-BATCH",
			Locator:    fmt.Sprintf("chunks.txt#L%d", i+1),
			Text:       fmt.Sprintf("chunk text %03d", i),
			TagsJSON:   "[]",
			TagsText:   "",
			CreatedAt:  now.Add(time.Duration(i) * time.Second),
		})
	}
	if _, _, err := st.AddArtifactWithChunks(artifact, chunks); err != nil {
		t.Fatalf("add chunks: %v", err)
	}

	provider := &testEmbedProvider{}
	embedded, err := embedMissingChunks(provider, st, repoID, workspace, model)
	if err != nil {
		t.Fatalf("embed missing chunks: %v", err)
	}
	if embedded != total {
		t.Fatalf("expected %d chunk embeddings, got %d", total, embedded)
	}
	if provider.calls < 2 {
		t.Fatalf("expected multiple provider calls, got %d", provider.calls)
	}

	remaining, err := st.ListChunksMissingEmbedding(repoID, workspace, model, 0)
	if err != nil {
		t.Fatalf("list missing chunks: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no missing chunk embeddings, got %d", len(remaining))
	}
}

func setupEmbeddingStore(t testing.TB) (*store.Store, string, string, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "memory.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
	})
	return st, "r-test", "default", "model-test"
}
