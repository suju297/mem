package app

import (
	"strings"
	"time"

	"mempack/internal/config"
	"mempack/internal/embed"
	"mempack/internal/store"
)

func maybeEmbedMemory(cfg config.Config, st *store.Store, mem store.Memory) error {
	provider := strings.TrimSpace(strings.ToLower(cfg.EmbeddingProvider))
	if provider == "" || provider == "none" {
		return nil
	}
	model := effectiveEmbeddingModel(cfg)
	if model == "" {
		return nil
	}
	queueItem := store.EmbeddingQueueItem{
		RepoID:    mem.RepoID,
		Workspace: mem.Workspace,
		Kind:      store.EmbeddingKindMemory,
		ItemID:    mem.ID,
		Model:     model,
		CreatedAt: time.Now().UTC(),
	}
	return st.EnqueueEmbedding(queueItem)
}

func effectiveEmbeddingModel(cfg config.Config) string {
	provider := strings.TrimSpace(strings.ToLower(cfg.EmbeddingProvider))
	model := strings.TrimSpace(cfg.EmbeddingModel)
	if provider == "auto" && model == "" {
		return embed.DefaultAutoModel
	}
	return model
}
