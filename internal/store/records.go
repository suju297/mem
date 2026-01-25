package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

type RepoRow struct {
	RepoID     string
	GitRoot    string
	OriginHash string
	LastHead   string
	LastBranch string
	CreatedAt  time.Time
	LastSeenAt time.Time
}

type Thread struct {
	ThreadID    string
	RepoID      string
	Workspace   string
	Title       string
	TagsJSON    string
	CreatedAt   time.Time
	MemoryCount int
}

type MemorySummary struct {
	ID           string
	ThreadID     string
	Title        string
	Summary      string
	CreatedAt    time.Time
	AnchorCommit string
	SupersededBy string
}

type Artifact struct {
	ID          string
	RepoID      string
	Workspace   string
	Kind        string
	Source      string
	ContentHash string
	CreatedAt   time.Time
}

var ErrNotFound = errors.New("not found")

func (s *Store) GetRepo(repoID string) (RepoRow, error) {
	row := s.db.QueryRow(`
		SELECT repo_id, git_root, origin_hash, last_head, last_branch, created_at, last_seen_at
		FROM repos
		WHERE repo_id = ?
	`, repoID)

	var repo RepoRow
	var createdAt string
	var lastSeen string
	var lastHead sql.NullString
	var lastBranch sql.NullString
	if err := row.Scan(&repo.RepoID, &repo.GitRoot, &repo.OriginHash, &lastHead, &lastBranch, &createdAt, &lastSeen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RepoRow{}, ErrNotFound
		}
		return RepoRow{}, err
	}
	repo.LastHead = lastHead.String
	repo.LastBranch = lastBranch.String
	repo.CreatedAt = parseTime(createdAt)
	repo.LastSeenAt = parseTime(lastSeen)
	return repo, nil
}

func (s *Store) GetMemory(repoID, workspace, id string) (Memory, error) {
	row := s.db.QueryRow(`
		SELECT id, repo_id, workspace, thread_id, title, summary, summary_tokens, tags_json, tags_text, entities_json, entities_text,
			created_at, anchor_commit, superseded_by, deleted_at
		FROM memories
		WHERE repo_id = ? AND workspace = ? AND id = ?
	`, repoID, normalizeWorkspace(workspace), id)

	var mem Memory
	var createdAt string
	var deletedAt sql.NullString
	var threadID sql.NullString
	var summaryTokens sql.NullInt64
	var tagsJSON sql.NullString
	var tagsText sql.NullString
	var entitiesJSON sql.NullString
	var entitiesText sql.NullString
	var anchorCommit sql.NullString
	var supersededBy sql.NullString
	if err := row.Scan(&mem.ID, &mem.RepoID, &mem.Workspace, &threadID, &mem.Title, &mem.Summary, &summaryTokens, &tagsJSON, &tagsText, &entitiesJSON, &entitiesText, &createdAt, &anchorCommit, &supersededBy, &deletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Memory{}, ErrNotFound
		}
		return Memory{}, err
	}
	mem.ThreadID = threadID.String
	if summaryTokens.Valid {
		mem.SummaryTokens = int(summaryTokens.Int64)
	}
	mem.TagsJSON = tagsJSON.String
	mem.TagsText = tagsText.String
	mem.EntitiesJSON = entitiesJSON.String
	mem.EntitiesText = entitiesText.String
	mem.AnchorCommit = anchorCommit.String
	mem.SupersededBy = supersededBy.String
	mem.CreatedAt = parseTime(createdAt)
	if deletedAt.Valid {
		mem.DeletedAt = parseTime(deletedAt.String)
	}
	return mem, nil
}

func (s *Store) GetChunk(repoID, workspace, id string) (Chunk, error) {
	row := s.db.QueryRow(`
		SELECT chunk_id, repo_id, workspace, artifact_id, thread_id, locator,
			text, text_hash, text_tokens, tags_json, tags_text,
			chunk_type, symbol_name, symbol_kind, created_at, deleted_at
		FROM chunks
		WHERE repo_id = ? AND workspace = ? AND chunk_id = ?
	`, repoID, normalizeWorkspace(workspace), id)

	var chunk Chunk
	var createdAt string
	var deletedAt sql.NullString
	var threadID sql.NullString
	var artifactID sql.NullString
	var locator sql.NullString
	var text sql.NullString
	var textHash sql.NullString
	var textTokens sql.NullInt64
	var tagsJSON sql.NullString
	var tagsText sql.NullString
	var chunkType sql.NullString
	var symbolName sql.NullString
	var symbolKind sql.NullString
	if err := row.Scan(&chunk.ID, &chunk.RepoID, &chunk.Workspace, &artifactID, &threadID, &locator,
		&text, &textHash, &textTokens, &tagsJSON, &tagsText,
		&chunkType, &symbolName, &symbolKind, &createdAt, &deletedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Chunk{}, ErrNotFound
		}
		return Chunk{}, err
	}
	chunk.ArtifactID = artifactID.String
	chunk.ThreadID = threadID.String
	chunk.Locator = locator.String
	chunk.Text = text.String
	chunk.TextHash = textHash.String
	if textTokens.Valid {
		chunk.TextTokens = int(textTokens.Int64)
	}
	chunk.TagsJSON = tagsJSON.String
	chunk.TagsText = tagsText.String
	chunk.ChunkType = chunkType.String
	chunk.SymbolName = symbolName.String
	chunk.SymbolKind = symbolKind.String
	chunk.CreatedAt = parseTime(createdAt)
	if deletedAt.Valid {
		chunk.DeletedAt = parseTime(deletedAt.String)
	}
	return chunk, nil
}

func (s *Store) GetMemoriesByIDs(repoID, workspace string, ids []string) ([]Memory, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(ids)+2)
	args = append(args, repoID, normalizeWorkspace(workspace))
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT id, repo_id, workspace, thread_id, title, summary, summary_tokens, tags_json, tags_text, entities_json, entities_text,
			created_at, anchor_commit, superseded_by, deleted_at
		FROM memories
		WHERE repo_id = ? AND workspace = ? AND id IN (%s) AND deleted_at IS NULL
	`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []Memory
	for rows.Next() {
		var mem Memory
		var createdAt string
		var deletedAt sql.NullString
		var threadID sql.NullString
		var summaryTokens sql.NullInt64
		var tagsJSON sql.NullString
		var tagsText sql.NullString
		var entitiesJSON sql.NullString
		var entitiesText sql.NullString
		var anchorCommit sql.NullString
		var supersededBy sql.NullString
		if err := rows.Scan(&mem.ID, &mem.RepoID, &mem.Workspace, &threadID, &mem.Title, &mem.Summary, &summaryTokens, &tagsJSON, &tagsText, &entitiesJSON, &entitiesText, &createdAt, &anchorCommit, &supersededBy, &deletedAt); err != nil {
			return nil, err
		}
		mem.ThreadID = threadID.String
		if summaryTokens.Valid {
			mem.SummaryTokens = int(summaryTokens.Int64)
		}
		mem.TagsJSON = tagsJSON.String
		mem.TagsText = tagsText.String
		mem.EntitiesJSON = entitiesJSON.String
		mem.EntitiesText = entitiesText.String
		mem.AnchorCommit = anchorCommit.String
		mem.SupersededBy = supersededBy.String
		mem.CreatedAt = parseTime(createdAt)
		if deletedAt.Valid {
			mem.DeletedAt = parseTime(deletedAt.String)
		}
		memories = append(memories, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return memories, nil
}

func (s *Store) GetChunksByIDs(repoID, workspace string, ids []string) ([]Chunk, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(ids)+2)
	args = append(args, repoID, normalizeWorkspace(workspace))
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT chunk_id, repo_id, workspace, artifact_id, thread_id, locator,
			text, text_hash, text_tokens, tags_json, tags_text,
			chunk_type, symbol_name, symbol_kind, created_at, deleted_at
		FROM chunks
		WHERE repo_id = ? AND workspace = ? AND chunk_id IN (%s) AND deleted_at IS NULL
	`, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []Chunk
	for rows.Next() {
		var chunk Chunk
		var createdAt string
		var deletedAt sql.NullString
		var textHash sql.NullString
		var textTokens sql.NullInt64
		var tagsJSON sql.NullString
		var tagsText sql.NullString
		var artifactID sql.NullString
		var threadID sql.NullString
		var locator sql.NullString
		var text sql.NullString
		var chunkType sql.NullString
		var symbolName sql.NullString
		var symbolKind sql.NullString
		if err := rows.Scan(&chunk.ID, &chunk.RepoID, &chunk.Workspace, &artifactID, &threadID, &locator,
			&text, &textHash, &textTokens, &tagsJSON, &tagsText,
			&chunkType, &symbolName, &symbolKind, &createdAt, &deletedAt); err != nil {
			return nil, err
		}
		chunk.ArtifactID = artifactID.String
		chunk.ThreadID = threadID.String
		chunk.Locator = locator.String
		chunk.Text = text.String
		chunk.TextHash = textHash.String
		if textTokens.Valid {
			chunk.TextTokens = int(textTokens.Int64)
		}
		chunk.TagsJSON = tagsJSON.String
		chunk.TagsText = tagsText.String
		chunk.ChunkType = chunkType.String
		chunk.SymbolName = symbolName.String
		chunk.SymbolKind = symbolKind.String
		chunk.CreatedAt = parseTime(createdAt)
		if deletedAt.Valid {
			chunk.DeletedAt = parseTime(deletedAt.String)
		}
		chunks = append(chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return chunks, nil
}

func (s *Store) ListRecentActiveThreads(repoID, workspace string, limit int) ([]Thread, error) {
	if limit <= 0 {
		return nil, nil
	}
	workspace = normalizeWorkspace(workspace)
	rows, err := s.db.Query(`
		SELECT t.thread_id, t.repo_id, t.workspace, t.title, t.tags_json, MAX(m.created_at) as last_activity
		FROM threads t
		JOIN memories m
			ON t.thread_id = m.thread_id
			AND t.repo_id = m.repo_id
			AND t.workspace = m.workspace
			AND m.deleted_at IS NULL
		WHERE t.repo_id = ? AND t.workspace = ?
		GROUP BY t.thread_id, t.repo_id, t.workspace
		ORDER BY last_activity DESC
		LIMIT ?
	`, repoID, workspace, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []Thread
	for rows.Next() {
		var thread Thread
		var tagsJSON sql.NullString
		var lastActivity string
		if err := rows.Scan(&thread.ThreadID, &thread.RepoID, &thread.Workspace, &thread.Title, &tagsJSON, &lastActivity); err != nil {
			return nil, err
		}
		thread.TagsJSON = tagsJSON.String
		thread.CreatedAt = parseTime(lastActivity)
		threads = append(threads, thread)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return threads, nil
}

func (s *Store) CountMemories(repoID, workspace string) (int, error) {
	workspace = normalizeWorkspace(workspace)
	row := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM memories
		WHERE repo_id = ? AND workspace = ? AND deleted_at IS NULL
	`, repoID, workspace)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) CountChunks(repoID, workspace string) (int, error) {
	workspace = normalizeWorkspace(workspace)
	row := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM chunks
		WHERE repo_id = ? AND workspace = ? AND deleted_at IS NULL
	`, repoID, workspace)
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ForgetMemory(repoID, workspace, id string, now time.Time) (bool, error) {
	res, err := s.db.Exec(`
		UPDATE memories
		SET deleted_at = ?
		WHERE repo_id = ? AND workspace = ? AND id = ? AND deleted_at IS NULL
	`, now.UTC().Format(time.RFC3339Nano), repoID, normalizeWorkspace(workspace), id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) ForgetChunk(repoID, workspace, id string, now time.Time) (bool, error) {
	res, err := s.db.Exec(`
		UPDATE chunks
		SET deleted_at = ?
		WHERE repo_id = ? AND workspace = ? AND chunk_id = ? AND deleted_at IS NULL
	`, now.UTC().Format(time.RFC3339Nano), repoID, normalizeWorkspace(workspace), id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) MarkMemorySuperseded(repoID, workspace, oldID, newID string) error {
	_, err := s.db.Exec(`
		UPDATE memories
		SET superseded_by = ?
		WHERE repo_id = ? AND workspace = ? AND id = ?
	`, newID, repoID, normalizeWorkspace(workspace), oldID)
	return err
}

func (s *Store) ListThreads(repoID, workspace string) ([]Thread, error) {
	rows, err := s.db.Query(`
		SELECT t.thread_id, t.repo_id, t.workspace, t.title, t.tags_json, t.created_at,
			COUNT(m.id) as memory_count
		FROM threads t
		LEFT JOIN memories m
			ON t.thread_id = m.thread_id
			AND t.repo_id = m.repo_id
			AND t.workspace = m.workspace
			AND m.deleted_at IS NULL
		WHERE t.repo_id = ? AND t.workspace = ?
		GROUP BY t.thread_id, t.repo_id, t.workspace
		ORDER BY t.created_at DESC
	`, repoID, normalizeWorkspace(workspace))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var threads []Thread
	for rows.Next() {
		var thread Thread
		var createdAt string
		var tagsJSON sql.NullString
		if err := rows.Scan(&thread.ThreadID, &thread.RepoID, &thread.Workspace, &thread.Title, &tagsJSON, &createdAt, &thread.MemoryCount); err != nil {
			return nil, err
		}
		thread.TagsJSON = tagsJSON.String
		thread.CreatedAt = parseTime(createdAt)
		threads = append(threads, thread)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return threads, nil
}

func (s *Store) GetThread(repoID, workspace, threadID string) (Thread, error) {
	row := s.db.QueryRow(`
		SELECT t.thread_id, t.repo_id, t.workspace, t.title, t.tags_json, t.created_at,
			COUNT(m.id) as memory_count
		FROM threads t
		LEFT JOIN memories m
			ON t.thread_id = m.thread_id
			AND t.repo_id = m.repo_id
			AND t.workspace = m.workspace
			AND m.deleted_at IS NULL
		WHERE t.repo_id = ? AND t.workspace = ? AND t.thread_id = ?
		GROUP BY t.thread_id, t.repo_id, t.workspace
	`, repoID, normalizeWorkspace(workspace), threadID)

	var thread Thread
	var createdAt string
	var tagsJSON sql.NullString
	if err := row.Scan(&thread.ThreadID, &thread.RepoID, &thread.Workspace, &thread.Title, &tagsJSON, &createdAt, &thread.MemoryCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Thread{}, ErrNotFound
		}
		return Thread{}, err
	}
	thread.TagsJSON = tagsJSON.String
	thread.CreatedAt = parseTime(createdAt)
	return thread, nil
}

func (s *Store) ListMemoriesByThread(repoID, workspace, threadID string, limit int) ([]MemorySummary, error) {
	rows, err := s.db.Query(`
		SELECT id, thread_id, title, summary, created_at, anchor_commit, superseded_by
		FROM memories
		WHERE repo_id = ? AND workspace = ? AND thread_id = ? AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ?
	`, repoID, normalizeWorkspace(workspace), threadID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []MemorySummary
	for rows.Next() {
		var mem MemorySummary
		var createdAt string
		var anchorCommit sql.NullString
		var supersededBy sql.NullString
		if err := rows.Scan(&mem.ID, &mem.ThreadID, &mem.Title, &mem.Summary, &createdAt, &anchorCommit, &supersededBy); err != nil {
			return nil, err
		}
		mem.CreatedAt = parseTime(createdAt)
		mem.AnchorCommit = anchorCommit.String
		mem.SupersededBy = supersededBy.String
		results = append(results, mem)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (s *Store) AddArtifactWithChunks(artifact Artifact, chunks []Chunk) (int, []string, error) {
	workspace := normalizeWorkspace(artifact.Workspace)
	tx, err := s.db.Begin()
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO artifacts (artifact_id, repo_id, workspace, kind, source, content_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, artifact.ID, artifact.RepoID, workspace, artifact.Kind, artifact.Source, artifact.ContentHash, artifact.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, nil, err
	}

	inserted := 0
	insertedIDs := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunkWorkspace := workspace
		if strings.TrimSpace(chunk.Workspace) != "" {
			chunkWorkspace = normalizeWorkspace(chunk.Workspace)
		}
		chunkType := chunk.ChunkType
		if chunkType == "" {
			chunkType = "line"
		}
		textHash := chunk.TextHash
		if textHash == "" {
			textHash = sha256Hex(chunk.Text)
		}
		res, err := tx.Exec(`
			INSERT OR IGNORE INTO chunks (
				chunk_id, repo_id, workspace, artifact_id, thread_id, locator,
				text, text_hash, text_tokens, tags_json, tags_text,
				chunk_type, symbol_name, symbol_kind, created_at, deleted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
		`, chunk.ID, chunk.RepoID, chunkWorkspace, chunk.ArtifactID, chunk.ThreadID, chunk.Locator,
			chunk.Text, textHash, chunk.TextTokens, chunk.TagsJSON, chunk.TagsText,
			chunkType, nullIfEmpty(chunk.SymbolName), nullIfEmpty(chunk.SymbolKind),
			chunk.CreatedAt.UTC().Format(time.RFC3339Nano))
		if err != nil {
			return 0, nil, err
		}
		if res != nil {
			if affected, err := res.RowsAffected(); err == nil {
				inserted += int(affected)
				if affected > 0 {
					insertedIDs = append(insertedIDs, chunk.ID)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, err
	}
	return inserted, insertedIDs, nil
}

func (s *Store) DeleteChunksBySource(repoID, workspace, source string) (int, error) {
	workspace = normalizeWorkspace(workspace)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	rows, err := s.db.Query(`
		SELECT artifact_id FROM artifacts
		WHERE repo_id = ? AND workspace = ? AND source = ?
	`, repoID, workspace, source)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var artifactIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		artifactIDs = append(artifactIDs, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(artifactIDs) == 0 {
		return 0, nil
	}

	placeholders := strings.Repeat("?,", len(artifactIDs))
	placeholders = strings.TrimSuffix(placeholders, ",")

	args := make([]any, 0, len(artifactIDs)+3)
	args = append(args, now, repoID, workspace)
	for _, id := range artifactIDs {
		args = append(args, id)
	}

	result, err := s.db.Exec(fmt.Sprintf(`
		UPDATE chunks SET deleted_at = ?
		WHERE repo_id = ? AND workspace = ? AND artifact_id IN (%s) AND deleted_at IS NULL
	`, placeholders), args...)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

func (s *Store) DeleteArtifactsBySource(repoID, workspace, source string) (int, error) {
	workspace = normalizeWorkspace(workspace)
	result, err := s.db.Exec(`
		DELETE FROM artifacts WHERE repo_id = ? AND workspace = ? AND source = ?
	`, repoID, workspace, source)
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	return int(affected), nil
}

func nullIfEmpty(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

func (s *Store) SetStateCurrent(repoID, workspace, stateJSON string, stateTokens int, updatedAt time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO state_current (repo_id, workspace, state_json, state_tokens, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(repo_id, workspace)
		DO UPDATE SET state_json = excluded.state_json, state_tokens = excluded.state_tokens, updated_at = excluded.updated_at
	`, repoID, workspace, stateJSON, stateTokens, updatedAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) AddStateHistory(stateID, repoID, workspace, stateJSON, reason string, stateTokens int, createdAt time.Time) error {
	_, err := s.db.Exec(`
		INSERT INTO state_history (state_id, repo_id, workspace, state_json, state_tokens, created_at, reason)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, stateID, repoID, workspace, stateJSON, stateTokens, createdAt.UTC().Format(time.RFC3339Nano), reason)
	return err
}
