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
	fromID := strings.TrimSpace(link.FromID)
	rel := strings.TrimSpace(link.Rel)
	toID := strings.TrimSpace(link.ToID)
	if fromID == "" || rel == "" || toID == "" {
		return fmt.Errorf("link requires from_id, rel, to_id")
	}

	createdAt := link.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}

	_, err := s.db.Exec(`
		INSERT INTO links (from_id, rel, to_id, weight, created_at)
		SELECT ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1
			FROM links
			WHERE from_id = ? AND rel = ? AND to_id = ?
		)
	`, fromID, rel, toID, link.Weight, createdAt.UTC().Format(time.RFC3339Nano), fromID, rel, toID)
	return err
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
		SELECT from_id, rel, to_id, weight, created_at
		FROM links
		WHERE from_id IN (%s) OR to_id IN (%s)
		ORDER BY created_at DESC
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
