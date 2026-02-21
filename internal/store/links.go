package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Link struct {
	FromID    string
	Rel       string
	ToID      string
	Weight    float64
	CreatedAt time.Time
}

func (s *Store) AddLink(link Link) error {
	_, err := s.AddLinkIfMissing(link)
	return err
}

func (s *Store) AddLinkIfMissing(link Link) (bool, error) {
	fromID := strings.TrimSpace(link.FromID)
	rel := strings.TrimSpace(link.Rel)
	toID := strings.TrimSpace(link.ToID)
	if fromID == "" || rel == "" || toID == "" {
		return false, fmt.Errorf("link requires from_id, rel, to_id")
	}

	createdAt := link.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	res, err := s.db.Exec(`
		INSERT INTO links (from_id, rel, to_id, weight, created_at)
		SELECT ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1
			FROM links
			WHERE from_id = ? AND rel = ? AND to_id = ?
		)
	`, fromID, rel, toID, link.Weight, createdAt.UTC().Format(time.RFC3339Nano), fromID, rel, toID)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *Store) WouldCreateLinkCycle(repoID, workspace, fromID, toID string) (bool, error) {
	repoID = strings.TrimSpace(repoID)
	workspace = normalizeWorkspace(workspace)
	fromID = strings.TrimSpace(fromID)
	toID = strings.TrimSpace(toID)
	if repoID == "" || fromID == "" || toID == "" {
		return false, nil
	}
	if fromID == toID {
		return true, nil
	}

	row := s.db.QueryRow(`
		WITH RECURSIVE walk(id) AS (
			SELECT ?
			UNION
			SELECT l.to_id
			FROM links l
			JOIN walk w ON l.from_id = w.id
			JOIN memories m_from
				ON m_from.id = l.from_id
				AND m_from.repo_id = ?
				AND m_from.workspace = ?
				AND m_from.deleted_at IS NULL
				AND (m_from.superseded_by IS NULL OR m_from.superseded_by = '')
			JOIN memories m_to
				ON m_to.id = l.to_id
				AND m_to.repo_id = ?
				AND m_to.workspace = ?
				AND m_to.deleted_at IS NULL
				AND (m_to.superseded_by IS NULL OR m_to.superseded_by = '')
		)
		SELECT 1
		FROM walk
		WHERE id = ?
		LIMIT 1
	`, toID, repoID, workspace, repoID, workspace, fromID)
	var found int
	if err := row.Scan(&found); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return found == 1, nil
}

func (s *Store) ListLinksForIDs(ids []string) ([]Link, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(ids)*2)
	for _, id := range ids {
		args = append(args, id)
	}
	for _, id := range ids {
		args = append(args, id)
	}

	rows, err := s.db.Query(fmt.Sprintf(`
		SELECT l.from_id, l.rel, l.to_id, l.weight, l.created_at
		FROM links l
		JOIN memories m_from
			ON m_from.id = l.from_id
			AND m_from.deleted_at IS NULL
			AND (m_from.superseded_by IS NULL OR m_from.superseded_by = '')
		JOIN memories m_to
			ON m_to.id = l.to_id
			AND m_to.deleted_at IS NULL
			AND (m_to.superseded_by IS NULL OR m_to.superseded_by = '')
			AND m_to.repo_id = m_from.repo_id
			AND m_to.workspace = m_from.workspace
		WHERE l.from_id IN (%s) OR l.to_id IN (%s)
		ORDER BY l.created_at DESC
	`, placeholders, placeholders), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []Link
	for rows.Next() {
		var link Link
		var weight sql.NullFloat64
		var createdAt string
		if err := rows.Scan(&link.FromID, &link.Rel, &link.ToID, &weight, &createdAt); err != nil {
			return nil, err
		}
		if weight.Valid {
			link.Weight = weight.Float64
		}
		link.CreatedAt = parseTime(createdAt)
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return links, nil
}

func (s *Store) DeleteLinksForMemoryID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	_, err := s.db.Exec(`
		DELETE FROM links
		WHERE from_id = ? OR to_id = ?
	`, id, id)
	return err
}
