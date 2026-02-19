package app

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"mempack/internal/config"
	"mempack/internal/embed"
	"mempack/internal/store"
)

const (
	embedBatchSize  = 8
	embedFetchLimit = 64
)

func runEmbed(args []string, out, errOut io.Writer) int {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		sub := strings.ToLower(strings.TrimSpace(args[0]))
		if sub == "status" {
			return runEmbedStatus(args[1:], out, errOut)
		}
	}

	fs := flag.NewFlagSet("embed", flag.ContinueOnError)
	fs.SetOutput(errOut)
	kind := fs.String("kind", "all", "Embed kind: memory|chunk|all")
	workspace := fs.String("workspace", "", "Workspace name")
	repoOverride := fs.String("repo", "", "Override repo id")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}

	provider, status := embed.Resolve(cfg)
	if provider == nil || !status.Enabled {
		msg := status.Error
		if strings.TrimSpace(msg) == "" {
			msg = "embedding provider disabled"
		}
		fmt.Fprintf(errOut, "embedding provider unavailable: %s\n", msg)
		return 1
	}

	kindValue := strings.ToLower(strings.TrimSpace(*kind))
	if kindValue == "" {
		kindValue = "all"
	}
	if kindValue != "memory" && kindValue != "chunk" && kindValue != "all" {
		fmt.Fprintf(errOut, "invalid --kind: %s\n", kindValue)
		return 2
	}

	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))
	repoInfo, err := resolveRepo(&cfg, strings.TrimSpace(*repoOverride))
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		fmt.Fprintf(errOut, "store open error: %v\n", err)
		return 1
	}
	defer st.Close()

	if err := st.EnsureRepo(repoInfo); err != nil {
		fmt.Fprintf(errOut, "store repo error: %v\n", err)
		return 1
	}

	model := strings.TrimSpace(status.Model)
	if model == "" {
		model = strings.TrimSpace(cfg.EmbeddingModel)
	}
	totalMem := 0
	totalChunks := 0

	if kindValue == "memory" || kindValue == "all" {
		embedded, err := embedMissingMemories(provider, st, repoInfo.ID, workspaceName, model)
		if err != nil {
			fmt.Fprintf(errOut, "memory embedding error: %v\n", err)
			return 1
		}
		totalMem = embedded
	}

	if kindValue == "chunk" || kindValue == "all" {
		embedded, err := embedMissingChunks(provider, st, repoInfo.ID, workspaceName, model)
		if err != nil {
			fmt.Fprintf(errOut, "chunk embedding error: %v\n", err)
			return 1
		}
		totalChunks = embedded
	}

	fmt.Fprintf(out, "Embedded memories=%d chunks=%d (provider=%s model=%s)\n", totalMem, totalChunks, status.Provider, model)
	return 0
}

type EmbedStatusResponse struct {
	RepoID     string              `json:"repo_id"`
	Workspace  string              `json:"workspace"`
	Provider   string              `json:"provider"`
	Model      string              `json:"model,omitempty"`
	Enabled    bool                `json:"enabled"`
	Error      string              `json:"error,omitempty"`
	Note       string              `json:"note"`
	Vectors    VectorStatus        `json:"vectors"`
	Memory     EmbedCoverageStatus `json:"memory"`
	Chunk      EmbedCoverageStatus `json:"chunk"`
	QueueDepth int                 `json:"queue_depth"`
	Worker     EmbedWorkerStatus   `json:"worker"`
}

type VectorStatus struct {
	ProviderConfigured string   `json:"provider_configured"`
	ModelConfigured    string   `json:"model_configured,omitempty"`
	Configured         bool     `json:"configured"`
	Available          bool     `json:"available"`
	Enabled            bool     `json:"enabled"`
	Reason             string   `json:"reason,omitempty"`
	HowToFix           []string `json:"how_to_fix,omitempty"`
	EffectiveProvider  string   `json:"effective_provider,omitempty"`
	EffectiveModel     string   `json:"effective_model,omitempty"`
}

type EmbedCoverageStatus struct {
	WithEmbeddings int `json:"with_embeddings"`
	Missing        int `json:"missing"`
	Total          int `json:"total"`
	Stale          int `json:"stale"`
	DimMismatch    int `json:"dim_mismatch"`
}

type EmbedWorkerStatus struct {
	LastRun   string `json:"last_run,omitempty"`
	LastError string `json:"last_error,omitempty"`
	Model     string `json:"model,omitempty"`
}

func runEmbedStatus(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("embed status", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id")
	workspace := fs.String("workspace", "", "Workspace name")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"repo":      {RequiresValue: true},
		"workspace": {RequiresValue: true},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(positional, " "))
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	repoInfo, err := resolveRepo(&cfg, strings.TrimSpace(*repoOverride))
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		fmt.Fprintf(errOut, "store open error: %v\n", err)
		return 1
	}
	defer st.Close()

	if err := st.EnsureRepo(repoInfo); err != nil {
		fmt.Fprintf(errOut, "store repo error: %v\n", err)
		return 1
	}

	_, status := embed.Resolve(cfg)
	model := strings.TrimSpace(status.Model)
	if model == "" {
		model = effectiveEmbeddingModel(cfg)
	}
	vectorStatus := buildVectorStatus(cfg, status, model)

	queueDepth := 0
	if model != "" {
		queueDepth, err = st.CountEmbeddingQueue(repoInfo.ID, model)
		if err != nil {
			fmt.Fprintf(errOut, "queue error: %v\n", err)
			return 1
		}
	}

	memCoverage := store.EmbeddingCoverage{}
	chunkCoverage := store.EmbeddingCoverage{}
	if model != "" {
		memCoverage, err = st.EmbeddingCoverage(repoInfo.ID, workspaceName, store.EmbeddingKindMemory, model)
		if err != nil {
			fmt.Fprintf(errOut, "memory coverage error: %v\n", err)
			return 1
		}
		chunkCoverage, err = st.EmbeddingCoverage(repoInfo.ID, workspaceName, store.EmbeddingKindChunk, model)
		if err != nil {
			fmt.Fprintf(errOut, "chunk coverage error: %v\n", err)
			return 1
		}
	}

	worker := EmbedWorkerStatus{}
	if value, err := st.GetMeta(embedWorkerMetaLastRun); err == nil {
		worker.LastRun = value
	} else if err != store.ErrNotFound {
		fmt.Fprintf(errOut, "worker status error: %v\n", err)
		return 1
	}
	if value, err := st.GetMeta(embedWorkerMetaLastError); err == nil {
		worker.LastError = value
	} else if err != store.ErrNotFound {
		fmt.Fprintf(errOut, "worker status error: %v\n", err)
		return 1
	}
	if value, err := st.GetMeta(embedWorkerMetaModel); err == nil {
		worker.Model = value
	} else if err != store.ErrNotFound {
		fmt.Fprintf(errOut, "worker status error: %v\n", err)
		return 1
	}

	memMissing := memCoverage.Total - memCoverage.WithEmbeddings
	if memMissing < 0 {
		memMissing = 0
	}
	chunkMissing := chunkCoverage.Total - chunkCoverage.WithEmbeddings
	if chunkMissing < 0 {
		chunkMissing = 0
	}

	note := "queue_depth counts pending embeddings; the worker drains it only when the provider is enabled."
	resp := EmbedStatusResponse{
		RepoID:     repoInfo.ID,
		Workspace:  workspaceName,
		Provider:   status.Provider,
		Model:      model,
		Enabled:    status.Enabled,
		Error:      status.Error,
		Note:       note,
		Vectors:    vectorStatus,
		QueueDepth: queueDepth,
		Memory: EmbedCoverageStatus{
			WithEmbeddings: memCoverage.WithEmbeddings,
			Missing:        memMissing,
			Total:          memCoverage.Total,
			Stale:          memCoverage.Stale,
			DimMismatch:    memCoverage.DimMismatch,
		},
		Chunk: EmbedCoverageStatus{
			WithEmbeddings: chunkCoverage.WithEmbeddings,
			Missing:        chunkMissing,
			Total:          chunkCoverage.Total,
			Stale:          chunkCoverage.Stale,
			DimMismatch:    chunkCoverage.DimMismatch,
		},
		Worker: worker,
	}
	return writeJSON(out, errOut, resp)
}

func buildVectorStatus(cfg config.Config, status embed.Status, model string) VectorStatus {
	providerConfigured := strings.TrimSpace(strings.ToLower(cfg.EmbeddingProvider))
	modelConfigured := strings.TrimSpace(cfg.EmbeddingModel)
	configured := providerConfigured != "" && providerConfigured != "none"
	available := status.Enabled
	enabled := status.Enabled
	reason := ""
	var fixes []string

	if !configured {
		reason = "provider_off"
	} else if !available {
		errLower := strings.ToLower(status.Error)
		switch {
		case strings.Contains(errLower, "ollama unavailable"):
			reason = "ollama_not_reachable"
			fixes = append(fixes, "Start Ollama (ollama serve)")
		case strings.Contains(errLower, "model not found"):
			reason = "model_missing"
		case strings.Contains(errLower, "embedding_model is required"):
			reason = "misconfigured"
			fixes = append(fixes, "Set embedding_model in config.toml")
		case strings.Contains(errLower, "not implemented"):
			reason = "provider_unsupported"
		case strings.Contains(errLower, "unknown embedding provider"):
			reason = "misconfigured"
			fixes = append(fixes, "Set embedding_provider to auto or ollama")
		default:
			reason = "unavailable"
		}
		if reason == "model_missing" && model != "" {
			fixes = append(fixes, fmt.Sprintf("Pull model (ollama pull %s)", model))
		}
		if reason == "ollama_not_reachable" && model != "" {
			fixes = append(fixes, fmt.Sprintf("Pull model (ollama pull %s)", model))
		}
	}

	return VectorStatus{
		ProviderConfigured: providerConfigured,
		ModelConfigured:    modelConfigured,
		Configured:         configured,
		Available:          available,
		Enabled:            enabled,
		Reason:             reason,
		HowToFix:           fixes,
		EffectiveProvider:  strings.TrimSpace(status.Provider),
		EffectiveModel:     model,
	}
}

func embedMissingMemories(provider embed.Provider, st *store.Store, repoID, workspace, model string) (int, error) {
	embedded := 0
	for {
		memories, err := st.ListMemoriesMissingEmbedding(repoID, workspace, model, embedFetchLimit)
		if err != nil {
			return embedded, err
		}
		if len(memories) == 0 {
			return embedded, nil
		}
		progress := 0
		now := time.Now().UTC()
		for i := 0; i < len(memories); i += embedBatchSize {
			end := i + embedBatchSize
			if end > len(memories) {
				end = len(memories)
			}

			texts := make([]string, 0, end-i)
			batch := make([]store.Memory, 0, end-i)
			for _, mem := range memories[i:end] {
				text := store.MemoryEmbeddingText(mem)
				if strings.TrimSpace(text) == "" {
					continue
				}
				texts = append(texts, text)
				batch = append(batch, mem)
			}
			if len(texts) == 0 {
				continue
			}
			vectors, err := provider.Embed(texts)
			if err != nil {
				return embedded, err
			}
			if len(vectors) != len(batch) {
				return embedded, fmt.Errorf("embedding count mismatch: got %d, want %d", len(vectors), len(batch))
			}

			for idx, vec := range vectors {
				mem := batch[idx]
				text := texts[idx]
				if err := st.UpsertEmbedding(store.Embedding{
					RepoID:      repoID,
					Workspace:   workspace,
					Kind:        store.EmbeddingKindMemory,
					ItemID:      mem.ID,
					Model:       model,
					ContentHash: store.EmbeddingContentHash(text),
					Vector:      vec,
					CreatedAt:   now,
					UpdatedAt:   now,
				}); err != nil {
					return embedded, err
				}
				embedded++
				progress++
			}
		}
		if progress == 0 {
			return embedded, nil
		}
	}
}

func embedMissingChunks(provider embed.Provider, st *store.Store, repoID, workspace, model string) (int, error) {
	embedded := 0
	for {
		chunks, err := st.ListChunksMissingEmbedding(repoID, workspace, model, embedFetchLimit)
		if err != nil {
			return embedded, err
		}
		if len(chunks) == 0 {
			return embedded, nil
		}
		progress := 0
		now := time.Now().UTC()
		for i := 0; i < len(chunks); i += embedBatchSize {
			end := i + embedBatchSize
			if end > len(chunks) {
				end = len(chunks)
			}

			texts := make([]string, 0, end-i)
			batch := make([]store.Chunk, 0, end-i)
			for _, chunk := range chunks[i:end] {
				text := store.ChunkEmbeddingText(chunk)
				if strings.TrimSpace(text) == "" {
					continue
				}
				texts = append(texts, text)
				batch = append(batch, chunk)
			}
			if len(texts) == 0 {
				continue
			}
			vectors, err := provider.Embed(texts)
			if err != nil {
				return embedded, err
			}
			if len(vectors) != len(batch) {
				return embedded, fmt.Errorf("embedding count mismatch: got %d, want %d", len(vectors), len(batch))
			}

			for idx, vec := range vectors {
				chunk := batch[idx]
				text := texts[idx]
				if err := st.UpsertEmbedding(store.Embedding{
					RepoID:      repoID,
					Workspace:   workspace,
					Kind:        store.EmbeddingKindChunk,
					ItemID:      chunk.ID,
					Model:       model,
					ContentHash: store.EmbeddingContentHash(text),
					Vector:      vec,
					CreatedAt:   now,
					UpdatedAt:   now,
				}); err != nil {
					return embedded, err
				}
				embedded++
				progress++
			}
		}
		if progress == 0 {
			return embedded, nil
		}
	}
}
