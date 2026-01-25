package store

import (
	"fmt"
	"strings"
	"time"
)

type EmbeddingQueueItem struct {
	QueueID   int64
	RepoID    string
	Workspace string
	Kind      string
	ItemID    string
	Model     string
	CreatedAt time.Time
}

func (s *Store) EnqueueEmbedding(item EmbeddingQueueItem) error {
	if item.RepoID == "" || item.ItemID == "" || item.Kind == "" || strings.TrimSpace(item.Model) == "" {
		return fmt.Errorf("embedding queue requires repo_id, kind, item_id, model")
	}
	workspace := normalizeWorkspace(item.Workspace)
	createdAt := item.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO embedding_queue (
			repo_id, workspace, kind, item_id, model, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, item.RepoID, workspace, strings.TrimSpace(item.Kind), strings.TrimSpace(item.ItemID), strings.TrimSpace(item.Model), createdAt.UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) ListEmbeddingQueue(repoID, model string, limit int) ([]EmbeddingQueueItem, error) {
	if repoID == "" || strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("embedding queue list requires repo_id and model")
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT queue_id, repo_id, workspace, kind, item_id, model, created_at
		FROM embedding_queue
		WHERE repo_id = ? AND model = ?
		ORDER BY queue_id
		LIMIT ?
	`, repoID, strings.TrimSpace(model), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []EmbeddingQueueItem
	for rows.Next() {
		var item EmbeddingQueueItem
		var createdAt string
		if err := rows.Scan(&item.QueueID, &item.RepoID, &item.Workspace, &item.Kind, &item.ItemID, &item.Model, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(createdAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) CountEmbeddingQueue(repoID, model string) (int, error) {
	if repoID == "" || strings.TrimSpace(model) == "" {
		return 0, fmt.Errorf("embedding queue count requires repo_id and model")
	}
	row := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM embedding_queue
		WHERE repo_id = ? AND model = ?
	`, repoID, strings.TrimSpace(model))
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) DeleteEmbeddingQueue(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	query := fmt.Sprintf(`DELETE FROM embedding_queue WHERE queue_id IN (%s)`, placeholders)
	_, err := s.db.Exec(query, args...)
	return err
}
