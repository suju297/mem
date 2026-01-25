package store

import "database/sql"

func SchemaVersion() int {
	return schemaVersion
}

func (s *Store) UserVersion() (int, error) {
	return getUserVersion(s.db)
}

func (s *Store) HasFTSTables() (bool, bool, error) {
	rows, err := s.db.Query(`
		SELECT name
		FROM sqlite_master
		WHERE type = 'table' AND name IN ('memories_fts', 'chunks_fts')
	`)
	if err != nil {
		return false, false, err
	}
	defer rows.Close()

	var memories bool
	var chunks bool
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, false, err
		}
		switch name {
		case "memories_fts":
			memories = true
		case "chunks_fts":
			chunks = true
		}
	}
	if err := rows.Err(); err != nil {
		return false, false, err
	}
	return memories, chunks, nil
}

func (s *Store) RebuildFTS() error {
	return rebuildFTS(s.db)
}

func ensureMetaKey(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO meta (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO NOTHING
	`, key, value)
	return err
}
