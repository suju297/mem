package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	EmbeddingKindMemory = "memory"
	EmbeddingKindChunk  = "chunk"
)

type Embedding struct {
	RepoID      string
	Workspace   string
	Kind        string
	ItemID      string
	Model       string
	ContentHash string
	Vector      []float64
	VectorDim   int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (s *Store) UpsertEmbedding(embedding Embedding) error {
	workspace := normalizeWorkspace(embedding.Workspace)
	kind := strings.TrimSpace(embedding.Kind)
	itemID := strings.TrimSpace(embedding.ItemID)
	model := strings.TrimSpace(embedding.Model)
	contentHash := strings.TrimSpace(embedding.ContentHash)
	if embedding.RepoID == "" || kind == "" || itemID == "" || model == "" {
		return fmt.Errorf("embedding requires repo_id, kind, item_id, model")
	}
	if contentHash == "" {
		return fmt.Errorf("embedding requires content_hash")
	}
	if len(embedding.Vector) == 0 {
		return fmt.Errorf("embedding vector is empty")
	}
	vectorJSON, err := json.Marshal(embedding.Vector)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	createdAt := embedding.CreatedAt
	if createdAt.IsZero() {
		createdAt = now
	}
	updatedAt := embedding.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = now
	}

	_, err = s.db.Exec(`
		INSERT INTO embeddings (
			repo_id, workspace, kind, item_id, model, content_hash, vector_json, vector_dim, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_id, workspace, kind, item_id, model)
		DO UPDATE SET
			content_hash = excluded.content_hash,
			vector_json = excluded.vector_json,
			vector_dim = excluded.vector_dim,
			updated_at = excluded.updated_at
	`, embedding.RepoID, workspace, kind, itemID, model, contentHash, string(vectorJSON), len(embedding.Vector), createdAt.UTC().Format(time.RFC3339Nano), updatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListEmbeddingsForSearch(repoID, workspace, kind, model string) ([]Embedding, int, error) {
	workspace = normalizeWorkspace(workspace)
	kind = strings.TrimSpace(kind)
	model = strings.TrimSpace(model)
	if repoID == "" || kind == "" || model == "" {
		return nil, 0, fmt.Errorf("embedding search requires repo_id, kind, model")
	}

	var rows *sql.Rows
	var err error
	switch kind {
	case EmbeddingKindMemory:
		rows, err = s.db.Query(`
			SELECT e.item_id, e.content_hash, e.vector_json, e.vector_dim, e.created_at, e.updated_at,
				m.title, m.summary, m.tags_text, m.entities_text
			FROM embeddings e
			JOIN memories m
				ON m.id = e.item_id
				AND m.repo_id = e.repo_id
				AND m.workspace = e.workspace
				AND m.deleted_at IS NULL
			WHERE e.repo_id = ? AND e.workspace = ? AND e.kind = ? AND e.model = ?
		`, repoID, workspace, kind, model)
	case EmbeddingKindChunk:
		rows, err = s.db.Query(`
			SELECT e.item_id, e.content_hash, e.vector_json, e.vector_dim, e.created_at, e.updated_at,
				c.locator, c.text, c.tags_text
			FROM embeddings e
			JOIN chunks c
				ON c.chunk_id = e.item_id
				AND c.repo_id = e.repo_id
				AND c.workspace = e.workspace
				AND c.deleted_at IS NULL
			WHERE e.repo_id = ? AND e.workspace = ? AND e.kind = ? AND e.model = ?
		`, repoID, workspace, kind, model)
	default:
		return nil, 0, fmt.Errorf("unsupported embedding kind: %s", kind)
	}
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var embeddings []Embedding
	stale := 0
	for rows.Next() {
		var itemID string
		var contentHash string
		var vectorJSON string
		var vectorDim int
		var createdAt string
		var updatedAt string
		switch kind {
		case EmbeddingKindMemory:
			var title sql.NullString
			var summary sql.NullString
			var tagsText sql.NullString
			var entitiesText sql.NullString
			if err := rows.Scan(&itemID, &contentHash, &vectorJSON, &vectorDim, &createdAt, &updatedAt, &title, &summary, &tagsText, &entitiesText); err != nil {
				return nil, 0, err
			}
			expected := embeddingContentHash(kind, title.String, summary.String, tagsText.String, entitiesText.String, "", "")
			if contentHash == "" || expected != contentHash {
				stale++
				continue
			}
		case EmbeddingKindChunk:
			var locator sql.NullString
			var text sql.NullString
			var tagsText sql.NullString
			if err := rows.Scan(&itemID, &contentHash, &vectorJSON, &vectorDim, &createdAt, &updatedAt, &locator, &text, &tagsText); err != nil {
				return nil, 0, err
			}
			expected := embeddingContentHash(kind, "", "", tagsText.String, "", locator.String, text.String)
			if contentHash == "" || expected != contentHash {
				stale++
				continue
			}
		}
		var vector []float64
		if err := json.Unmarshal([]byte(vectorJSON), &vector); err != nil {
			return nil, 0, err
		}
		embeddings = append(embeddings, Embedding{
			RepoID:      repoID,
			Workspace:   workspace,
			Kind:        kind,
			ItemID:      itemID,
			Model:       model,
			ContentHash: contentHash,
			Vector:      vector,
			VectorDim:   vectorDim,
			CreatedAt:   parseTime(createdAt),
			UpdatedAt:   parseTime(updatedAt),
		})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return embeddings, stale, nil
}

func (s *Store) ListMemoryEmbeddingsByIDs(repoID, workspace, model string, ids []string) (map[string][]float64, error) {
	if repoID == "" || strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("embedding lookup requires repo_id and model")
	}
	if len(ids) == 0 {
		return map[string][]float64{}, nil
	}
	workspace = normalizeWorkspace(workspace)
	model = strings.TrimSpace(model)

	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(ids)+4)
	args = append(args, repoID, workspace, EmbeddingKindMemory, model)
	for _, id := range ids {
		args = append(args, strings.TrimSpace(id))
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT e.item_id, e.content_hash, e.vector_json, e.vector_dim,
			m.title, m.summary, m.tags_text, m.entities_text
		FROM embeddings e
		JOIN memories m
			ON m.id = e.item_id
			AND m.repo_id = e.repo_id
			AND m.workspace = e.workspace
			AND m.deleted_at IS NULL
		WHERE e.repo_id = ? AND e.workspace = ? AND e.kind = ? AND e.model = ?
			AND e.item_id IN (%s)
	`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	embeddings := make(map[string][]float64, len(ids))
	for rows.Next() {
		var itemID string
		var contentHash string
		var vectorJSON string
		var vectorDim int
		var title sql.NullString
		var summary sql.NullString
		var tagsText sql.NullString
		var entitiesText sql.NullString
		if err := rows.Scan(&itemID, &contentHash, &vectorJSON, &vectorDim, &title, &summary, &tagsText, &entitiesText); err != nil {
			return nil, err
		}
		expected := embeddingContentHash(EmbeddingKindMemory, title.String, summary.String, tagsText.String, entitiesText.String, "", "")
		if contentHash == "" || expected != contentHash {
			continue
		}
		var vector []float64
		if err := json.Unmarshal([]byte(vectorJSON), &vector); err != nil {
			return nil, err
		}
		if len(vector) == 0 || (vectorDim > 0 && len(vector) != vectorDim) {
			continue
		}
		embeddings[itemID] = vector
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return embeddings, nil
}

func (s *Store) ListMemoriesMissingEmbedding(repoID, workspace, model string, limit int) ([]Memory, error) {
	workspace = normalizeWorkspace(workspace)
	model = strings.TrimSpace(model)
	if repoID == "" || model == "" {
		return nil, fmt.Errorf("missing embedding requires repo_id and model")
	}

	query := `
		SELECT m.id, m.repo_id, m.workspace, m.thread_id, m.title, m.summary, m.tags_text, m.entities_text, m.created_at,
			e.content_hash
		FROM memories m
		LEFT JOIN embeddings e
			ON e.repo_id = m.repo_id
			AND e.workspace = m.workspace
			AND e.kind = ?
			AND e.item_id = m.id
			AND e.model = ?
		WHERE m.repo_id = ? AND m.workspace = ? AND m.deleted_at IS NULL
		ORDER BY m.created_at DESC
	`
	args := []any{EmbeddingKindMemory, model, repoID, workspace}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var mem Memory
		var threadID sql.NullString
		var tagsText sql.NullString
		var entitiesText sql.NullString
		var createdAt string
		var contentHash sql.NullString
		if err := rows.Scan(&mem.ID, &mem.RepoID, &mem.Workspace, &threadID, &mem.Title, &mem.Summary, &tagsText, &entitiesText, &createdAt, &contentHash); err != nil {
			return nil, err
		}
		mem.ThreadID = threadID.String
		mem.TagsText = tagsText.String
		mem.EntitiesText = entitiesText.String
		mem.CreatedAt = parseTime(createdAt)
		expectedHash := EmbeddingContentHash(MemoryEmbeddingText(mem))
		if contentHash.String != "" && contentHash.String == expectedHash {
			continue
		}
		memories = append(memories, mem)
		if limit > 0 && len(memories) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memories, nil
}

func (s *Store) ListChunksMissingEmbedding(repoID, workspace, model string, limit int) ([]Chunk, error) {
	workspace = normalizeWorkspace(workspace)
	model = strings.TrimSpace(model)
	if repoID == "" || model == "" {
		return nil, fmt.Errorf("missing embedding requires repo_id and model")
	}

	query := `
		SELECT c.chunk_id, c.repo_id, c.workspace, c.artifact_id, c.thread_id, c.locator, c.text, c.tags_text, c.created_at,
			e.content_hash
		FROM chunks c
		LEFT JOIN embeddings e
			ON e.repo_id = c.repo_id
			AND e.workspace = c.workspace
			AND e.kind = ?
			AND e.item_id = c.chunk_id
			AND e.model = ?
		WHERE c.repo_id = ? AND c.workspace = ? AND c.deleted_at IS NULL
		ORDER BY c.created_at DESC
	`
	args := []any{EmbeddingKindChunk, model, repoID, workspace}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var chunk Chunk
		var artifactID sql.NullString
		var threadID sql.NullString
		var locator sql.NullString
		var createdAt string
		var tagsText sql.NullString
		var contentHash sql.NullString
		if err := rows.Scan(&chunk.ID, &chunk.RepoID, &chunk.Workspace, &artifactID, &threadID, &locator, &chunk.Text, &tagsText, &createdAt, &contentHash); err != nil {
			return nil, err
		}
		chunk.ArtifactID = artifactID.String
		chunk.ThreadID = threadID.String
		chunk.Locator = locator.String
		chunk.TagsText = tagsText.String
		chunk.CreatedAt = parseTime(createdAt)
		expectedHash := EmbeddingContentHash(ChunkEmbeddingText(chunk))
		if contentHash.String != "" && contentHash.String == expectedHash {
			continue
		}
		chunks = append(chunks, chunk)
		if limit > 0 && len(chunks) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}

type EmbeddingCoverage struct {
	WithEmbeddings int
	Total          int
	Stale          int
	DimMismatch    int
	LastUpdated    time.Time
}

func (s *Store) EmbeddingCoverage(repoID, workspace, kind, model string) (EmbeddingCoverage, error) {
	workspace = normalizeWorkspace(workspace)
	model = strings.TrimSpace(model)
	kind = strings.TrimSpace(kind)
	if repoID == "" || model == "" || kind == "" {
		return EmbeddingCoverage{}, fmt.Errorf("embedding coverage requires repo_id, kind, model")
	}

	total, err := s.countItems(repoID, workspace, kind)
	if err != nil {
		return EmbeddingCoverage{}, err
	}

	rows, err := s.db.Query(`
		SELECT e.content_hash, e.vector_dim, e.updated_at,
			m.title, m.summary, m.tags_text, m.entities_text,
			c.locator, c.text, c.tags_text
		FROM embeddings e
		LEFT JOIN memories m
			ON e.kind = ? AND m.id = e.item_id AND m.repo_id = e.repo_id AND m.workspace = e.workspace AND m.deleted_at IS NULL
		LEFT JOIN chunks c
			ON e.kind = ? AND c.chunk_id = e.item_id AND c.repo_id = e.repo_id AND c.workspace = e.workspace AND c.deleted_at IS NULL
		WHERE e.repo_id = ? AND e.workspace = ? AND e.kind = ? AND e.model = ?
	`, EmbeddingKindMemory, EmbeddingKindChunk, repoID, workspace, kind, model)
	if err != nil {
		return EmbeddingCoverage{}, err
	}
	defer rows.Close()

	coverage := EmbeddingCoverage{Total: total}
	firstDim := 0
	for rows.Next() {
		var contentHash string
		var vectorDim int
		var updatedAt string
		var title sql.NullString
		var summary sql.NullString
		var memTags sql.NullString
		var entities sql.NullString
		var locator sql.NullString
		var text sql.NullString
		var chunkTags sql.NullString
		if err := rows.Scan(&contentHash, &vectorDim, &updatedAt, &title, &summary, &memTags, &entities, &locator, &text, &chunkTags); err != nil {
			return EmbeddingCoverage{}, err
		}

		expected := ""
		if kind == EmbeddingKindMemory {
			expected = embeddingContentHash(kind, title.String, summary.String, memTags.String, entities.String, "", "")
		} else {
			expected = embeddingContentHash(kind, "", "", chunkTags.String, "", locator.String, text.String)
		}
		if expected == "" || contentHash == "" || expected != contentHash {
			coverage.Stale++
		} else {
			coverage.WithEmbeddings++
		}

		if firstDim == 0 {
			firstDim = vectorDim
		} else if vectorDim != firstDim {
			coverage.DimMismatch++
		}
		if ts := parseTime(updatedAt); !ts.IsZero() {
			if coverage.LastUpdated.IsZero() || ts.After(coverage.LastUpdated) {
				coverage.LastUpdated = ts
			}
		}
	}
	if err := rows.Err(); err != nil {
		return EmbeddingCoverage{}, err
	}
	return coverage, nil
}

func (s *Store) countItems(repoID, workspace, kind string) (int, error) {
	query := ""
	switch kind {
	case EmbeddingKindMemory:
		query = `SELECT COUNT(*) FROM memories WHERE repo_id = ? AND workspace = ? AND deleted_at IS NULL`
	case EmbeddingKindChunk:
		query = `SELECT COUNT(*) FROM chunks WHERE repo_id = ? AND workspace = ? AND deleted_at IS NULL`
	default:
		return 0, fmt.Errorf("unsupported embedding kind: %s", kind)
	}
	row := s.db.QueryRow(query, repoID, workspace)
	var total int
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func EmbeddingContentHash(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	return sha256Hex(text)
}

func MemoryEmbeddingText(mem Memory) string {
	return memoryEmbeddingTextFromFields(mem.Title, mem.Summary, mem.TagsText, mem.EntitiesText)
}

func ChunkEmbeddingText(chunk Chunk) string {
	return chunkEmbeddingTextFromFields(chunk.Locator, chunk.Text, chunk.TagsText)
}

func memoryEmbeddingTextFromFields(title, summary, tagsText, entitiesText string) string {
	parts := make([]string, 0, 4)
	if strings.TrimSpace(title) != "" {
		parts = append(parts, strings.TrimSpace(title))
	}
	if strings.TrimSpace(summary) != "" {
		parts = append(parts, strings.TrimSpace(summary))
	}
	if strings.TrimSpace(tagsText) != "" {
		parts = append(parts, "Tags: "+strings.TrimSpace(tagsText))
	}
	if strings.TrimSpace(entitiesText) != "" {
		parts = append(parts, "Entities: "+strings.TrimSpace(entitiesText))
	}
	return strings.Join(parts, "\n")
}

func chunkEmbeddingTextFromFields(locator, text, tagsText string) string {
	parts := make([]string, 0, 3)
	if strings.TrimSpace(locator) != "" {
		parts = append(parts, "Locator: "+strings.TrimSpace(locator))
	}
	if strings.TrimSpace(text) != "" {
		parts = append(parts, text)
	}
	if strings.TrimSpace(tagsText) != "" {
		parts = append(parts, "Tags: "+strings.TrimSpace(tagsText))
	}
	return strings.Join(parts, "\n")
}

func embeddingContentHash(kind, title, summary, tagsText, entitiesText, locator, text string) string {
	switch kind {
	case EmbeddingKindMemory:
		return EmbeddingContentHash(memoryEmbeddingTextFromFields(title, summary, tagsText, entitiesText))
	case EmbeddingKindChunk:
		return EmbeddingContentHash(chunkEmbeddingTextFromFields(locator, text, tagsText))
	default:
		return ""
	}
}
