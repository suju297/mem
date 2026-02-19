package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mempack/internal/config"
	"mempack/internal/embed"
	"mempack/internal/store"
)

const (
	embedQueueBatchSize  = 16
	embedQueueIdleDelay  = 3 * time.Second
	embedQueueErrorDelay = 10 * time.Second
)

const (
	embedWorkerMetaLastRun   = "embedding_worker_last_run"
	embedWorkerMetaLastError = "embedding_worker_last_error"
	embedWorkerMetaModel     = "embedding_worker_model"
)

func startEmbeddingWorker(ctx context.Context, cfg config.Config, repoID string) {
	provider := strings.TrimSpace(strings.ToLower(cfg.EmbeddingProvider))
	if provider == "" || provider == "none" {
		return
	}
	model := effectiveEmbeddingModel(cfg)
	if model == "" {
		return
	}
	if repoID == "" {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}

	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			embedder, status := embed.Resolve(cfg)
			if embedder == nil || !status.Enabled {
				if !sleepWithContext(ctx, embedQueueErrorDelay) {
					return
				}
				continue
			}

			delay := runEmbeddingWorkerIteration(embedder, cfg, repoID, model)
			if delay > 0 && !sleepWithContext(ctx, delay) {
				return
			}
		}
	}()
}

func runEmbeddingWorkerIteration(embedder embed.Provider, cfg config.Config, repoID, model string) (delay time.Duration) {
	st, err := openStore(cfg, repoID)
	if err != nil {
		return embedQueueErrorDelay
	}
	defer st.Close()
	defer func() {
		if recovered := recover(); recovered != nil {
			recordEmbeddingWorkerStatus(st, model, fmt.Sprintf("panic: %v", recovered))
			delay = embedQueueErrorDelay
		}
	}()

	items, err := st.ListEmbeddingQueue(repoID, model, embedQueueBatchSize)
	if err != nil {
		recordEmbeddingWorkerStatus(st, model, err.Error())
		return embedQueueErrorDelay
	}
	if len(items) == 0 {
		return embedQueueIdleDelay
	}

	if err := processEmbeddingQueue(embedder, st, items); err != nil {
		recordEmbeddingWorkerStatus(st, model, err.Error())
		return embedQueueErrorDelay
	}
	recordEmbeddingWorkerStatus(st, model, "")
	return 0
}

func processEmbeddingQueue(embedder embed.Provider, st *store.Store, items []store.EmbeddingQueueItem) error {
	processed := make([]int64, 0, len(items))
	var queue []queuedEmbedding

	for _, item := range items {
		switch item.Kind {
		case store.EmbeddingKindMemory:
			mem, err := st.GetMemory(item.RepoID, item.Workspace, item.ItemID)
			if err != nil {
				if err == store.ErrNotFound {
					processed = append(processed, item.QueueID)
					continue
				}
				_ = st.DeleteEmbeddingQueue(processed)
				return err
			}
			if !mem.DeletedAt.IsZero() {
				processed = append(processed, item.QueueID)
				continue
			}
			text := store.MemoryEmbeddingText(mem)
			if strings.TrimSpace(text) == "" {
				processed = append(processed, item.QueueID)
				continue
			}
			queue = append(queue, queuedEmbedding{
				QueueID:   item.QueueID,
				RepoID:    mem.RepoID,
				Workspace: mem.Workspace,
				Kind:      store.EmbeddingKindMemory,
				ItemID:    mem.ID,
				Model:     item.Model,
				Text:      text,
			})
		case store.EmbeddingKindChunk:
			chunk, err := st.GetChunk(item.RepoID, item.Workspace, item.ItemID)
			if err != nil {
				if err == store.ErrNotFound {
					processed = append(processed, item.QueueID)
					continue
				}
				_ = st.DeleteEmbeddingQueue(processed)
				return err
			}
			if !chunk.DeletedAt.IsZero() {
				processed = append(processed, item.QueueID)
				continue
			}
			text := store.ChunkEmbeddingText(chunk)
			if strings.TrimSpace(text) == "" {
				processed = append(processed, item.QueueID)
				continue
			}
			queue = append(queue, queuedEmbedding{
				QueueID:   item.QueueID,
				RepoID:    chunk.RepoID,
				Workspace: chunk.Workspace,
				Kind:      store.EmbeddingKindChunk,
				ItemID:    chunk.ID,
				Model:     item.Model,
				Text:      text,
			})
		default:
			_ = st.DeleteEmbeddingQueue(processed)
			return fmt.Errorf("unsupported embedding queue kind: %s", item.Kind)
		}
	}

	for i := 0; i < len(queue); i += embedBatchSize {
		end := i + embedBatchSize
		if end > len(queue) {
			end = len(queue)
		}
		texts := make([]string, 0, end-i)
		batch := queue[i:end]
		for _, entry := range batch {
			texts = append(texts, entry.Text)
		}
		vectors, err := embedder.Embed(texts)
		if err != nil {
			_ = st.DeleteEmbeddingQueue(processed)
			return err
		}
		if len(vectors) != len(batch) {
			_ = st.DeleteEmbeddingQueue(processed)
			return fmt.Errorf("embedding count mismatch: got %d, want %d", len(vectors), len(batch))
		}
		for idx, entry := range batch {
			embedding := store.Embedding{
				RepoID:      entry.RepoID,
				Workspace:   entry.Workspace,
				Kind:        entry.Kind,
				ItemID:      entry.ItemID,
				Model:       entry.Model,
				ContentHash: store.EmbeddingContentHash(entry.Text),
				Vector:      vectors[idx],
			}
			if err := st.UpsertEmbedding(embedding); err != nil {
				_ = st.DeleteEmbeddingQueue(processed)
				return err
			}
			processed = append(processed, entry.QueueID)
		}
	}

	return st.DeleteEmbeddingQueue(processed)
}

type queuedEmbedding struct {
	QueueID   int64
	RepoID    string
	Workspace string
	Kind      string
	ItemID    string
	Model     string
	Text      string
}

func recordEmbeddingWorkerStatus(st *store.Store, model, errMsg string) {
	if st == nil || strings.TrimSpace(model) == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_ = st.SetMeta(embedWorkerMetaLastRun, now)
	_ = st.SetMeta(embedWorkerMetaModel, model)
	_ = st.SetMeta(embedWorkerMetaLastError, strings.TrimSpace(errMsg))
}

func sleepWithContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
