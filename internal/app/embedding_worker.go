package app

import (
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

func startEmbeddingWorker(cfg config.Config, repoID string) {
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

	go func() {
		for {
			embedder, status := embed.Resolve(cfg)
			if embedder == nil || !status.Enabled {
				time.Sleep(embedQueueErrorDelay)
				continue
			}

			st, err := openStore(cfg, repoID)
			if err != nil {
				time.Sleep(embedQueueErrorDelay)
				continue
			}

			items, err := st.ListEmbeddingQueue(repoID, model, embedQueueBatchSize)
			if err != nil {
				recordEmbeddingWorkerStatus(st, model, err.Error())
				_ = st.Close()
				time.Sleep(embedQueueErrorDelay)
				continue
			}
			if len(items) == 0 {
				_ = st.Close()
				time.Sleep(embedQueueIdleDelay)
				continue
			}

			if err := processEmbeddingQueue(embedder, st, items); err != nil {
				recordEmbeddingWorkerStatus(st, model, err.Error())
				_ = st.Close()
				time.Sleep(embedQueueErrorDelay)
				continue
			}
			recordEmbeddingWorkerStatus(st, model, "")
			_ = st.Close()
		}
	}()
}

func processEmbeddingQueue(embedder embed.Provider, st *store.Store, items []store.EmbeddingQueueItem) error {
	processed := make([]int64, 0, len(items))
	var queue []queuedMemory

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
			queue = append(queue, queuedMemory{QueueID: item.QueueID, Memory: mem, Model: item.Model})
		default:
			processed = append(processed, item.QueueID)
		}
	}

	for i := 0; i < len(queue); i += embedBatchSize {
		end := i + embedBatchSize
		if end > len(queue) {
			end = len(queue)
		}
		texts := make([]string, 0, end-i)
		batch := make([]queuedMemory, 0, end-i)
		for _, entry := range queue[i:end] {
			text := store.MemoryEmbeddingText(entry.Memory)
			if strings.TrimSpace(text) == "" {
				processed = append(processed, entry.QueueID)
				continue
			}
			texts = append(texts, text)
			batch = append(batch, entry)
		}
		if len(texts) == 0 {
			continue
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
			text := store.MemoryEmbeddingText(entry.Memory)
			embedding := store.Embedding{
				RepoID:      entry.Memory.RepoID,
				Workspace:   entry.Memory.Workspace,
				Kind:        store.EmbeddingKindMemory,
				ItemID:      entry.Memory.ID,
				Model:       entry.Model,
				ContentHash: store.EmbeddingContentHash(text),
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

type queuedMemory struct {
	QueueID int64
	Memory  store.Memory
	Model   string
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
