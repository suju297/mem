package store

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"
)

const schemaVersion = 10

func migrate(db *sql.DB) error {
	version, err := getUserVersion(db)
	if err != nil {
		return err
	}

	if err := ensureColumns(db); err != nil {
		return err
	}
	if err := ensureEmbeddingsTable(db); err != nil {
		return err
	}
	if err := ensureEmbeddingColumns(db); err != nil {
		return err
	}
	if err := ensureEmbeddingsIndexes(db); err != nil {
		return err
	}
	if err := ensureEmbeddingQueueTable(db); err != nil {
		return err
	}
	if err := ensureEmbeddingQueueIndexes(db); err != nil {
		return err
	}
	if err := ensureLinksTable(db); err != nil {
		return err
	}
	if err := ensureLinksIndexes(db); err != nil {
		return err
	}
	if err := ensureChunkSymbolIndex(db); err != nil {
		return err
	}

	if version < 5 {
		if err := rebuildThreadsTable(db); err != nil {
			return err
		}
		if err := backfillWorkspaceColumns(db); err != nil {
			return err
		}
		if err := ensureWorkspaceIndexes(db); err != nil {
			return err
		}
		if err := backfillChunkHashes(db); err != nil {
			return err
		}
		if err := dedupeChunks(db); err != nil {
			return err
		}
		if err := ensureChunkUniqueIndex(db); err != nil {
			return err
		}
	}

	if version < 9 {
		if err := backfillChunkType(db); err != nil {
			return err
		}
	}
	if version < schemaVersion {
		if err := rebuildFTS(db); err != nil {
			return err
		}
		if err := setUserVersion(db, schemaVersion); err != nil {
			return err
		}
		if err := setLastMigrationAt(db); err != nil {
			return err
		}
	}

	if err := ensureMeta(db); err != nil {
		return err
	}

	return nil
}

func getUserVersion(db *sql.DB) (int, error) {
	row := db.QueryRow("PRAGMA user_version;")
	var version int
	if err := row.Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func setUserVersion(db *sql.DB, version int) error {
	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d;", version))
	return err
}

func setLastMigrationAt(db *sql.DB) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.Exec(`
		INSERT INTO meta (key, value)
		VALUES ('last_migration_at', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, now)
	return err
}

func ensureMeta(db *sql.DB) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return ensureMetaKey(db, "last_migration_at", now)
}

func ensureEmbeddingsTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS embeddings (
			repo_id TEXT NOT NULL,
			workspace TEXT NOT NULL DEFAULT 'default',
			kind TEXT NOT NULL,
			item_id TEXT NOT NULL,
			model TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			vector_json TEXT NOT NULL,
			vector_dim INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (repo_id, workspace, kind, item_id, model)
		)
	`)
	return err
}

func ensureEmbeddingsIndexes(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_embeddings_kind_model
		ON embeddings (repo_id, workspace, kind, model)
	`)
	return err
}

func ensureEmbeddingQueueTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS embedding_queue (
			queue_id INTEGER PRIMARY KEY AUTOINCREMENT,
			repo_id TEXT NOT NULL,
			workspace TEXT NOT NULL DEFAULT 'default',
			kind TEXT NOT NULL,
			item_id TEXT NOT NULL,
			model TEXT NOT NULL,
			created_at TEXT NOT NULL
		)
	`)
	return err
}

func ensureEmbeddingQueueIndexes(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_embedding_queue_unique
		ON embedding_queue (repo_id, workspace, kind, item_id, model)
	`)
	return err
}

func ensureEmbeddingColumns(db *sql.DB) error {
	if err := ensureColumn(db, "embeddings", "content_hash", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	return nil
}

func ensureLinksTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS links (
			from_id TEXT NOT NULL,
			rel TEXT NOT NULL,
			to_id TEXT NOT NULL,
			weight REAL,
			created_at TEXT NOT NULL
		)
	`)
	return err
}

func ensureLinksIndexes(db *sql.DB) error {
	if _, err := db.Exec(`
		DELETE FROM links
		WHERE rowid NOT IN (
			SELECT MIN(rowid)
			FROM links
			GROUP BY from_id, rel, to_id
		)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_links_from ON links (from_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_links_to ON links (to_id)`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_links_unique ON links (from_id, rel, to_id)`); err != nil {
		return err
	}
	return nil
}

func ensureChunkSymbolIndex(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_chunks_symbol
		ON chunks (repo_id, workspace, symbol_name)
		WHERE symbol_name IS NOT NULL AND symbol_name != ''
	`)
	return err
}

func ensureColumns(db *sql.DB) error {
	if err := ensureColumn(db, "memories", "tags_text", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "memories", "entities_text", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "memories", "summary_tokens", "INTEGER"); err != nil {
		return err
	}
	if err := ensureColumn(db, "chunks", "tags_json", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "chunks", "tags_text", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "chunks", "text_hash", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "chunks", "text_tokens", "INTEGER"); err != nil {
		return err
	}
	if err := ensureColumn(db, "chunks", "deleted_at", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "chunks", "chunk_type", "TEXT DEFAULT 'line'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "chunks", "symbol_name", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "chunks", "symbol_kind", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "state_current", "state_tokens", "INTEGER"); err != nil {
		return err
	}
	if err := ensureColumn(db, "state_history", "state_tokens", "INTEGER"); err != nil {
		return err
	}
	if err := ensureColumn(db, "repos", "last_head", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "repos", "last_branch", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "threads", "workspace", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "memories", "workspace", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "artifacts", "workspace", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	if err := ensureColumn(db, "chunks", "workspace", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	return nil
}

func backfillChunkType(db *sql.DB) error {
	_, err := db.Exec(`UPDATE chunks SET chunk_type = 'line' WHERE chunk_type IS NULL OR chunk_type = ''`)
	return err
}

func ensureColumn(db *sql.DB, table, column, columnType string) error {
	exists, err := columnExists(db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	_, err = db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnType))
	return err
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func rebuildFTS(db *sql.DB) error {
	if err := recreateFTSTables(db); err != nil {
		return err
	}
	if _, err := db.Exec("DELETE FROM memories_fts"); err != nil {
		return err
	}
	_, err := db.Exec(`
		INSERT INTO memories_fts (rowid, title, summary, tags, entities, repo_id, workspace, mem_id)
		SELECT rowid, title, summary, COALESCE(tags_text, ''), COALESCE(entities_text, ''), repo_id, workspace, id
		FROM memories
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return err
	}

	if _, err := db.Exec("DELETE FROM chunks_fts"); err != nil {
		return err
	}
	_, err = db.Exec(`
	INSERT INTO chunks_fts (rowid, locator, text, tags, repo_id, workspace, chunk_id, thread_id)
	SELECT rowid, locator, text, COALESCE(tags_text, ''), repo_id, workspace, chunk_id, thread_id
	FROM chunks
	WHERE deleted_at IS NULL
	`)
	return err
}

func recreateFTSTables(db *sql.DB) error {
	if _, err := db.Exec("DROP TABLE IF EXISTS memories_fts"); err != nil {
		return err
	}
	if _, err := db.Exec("DROP TABLE IF EXISTS chunks_fts"); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5 (
			title,
			summary,
			tags,
			entities,
			repo_id UNINDEXED,
			workspace UNINDEXED,
			mem_id UNINDEXED,
			tokenize = 'porter unicode61'
		)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5 (
			locator,
			text,
			tags,
			repo_id UNINDEXED,
			workspace UNINDEXED,
			chunk_id UNINDEXED,
			thread_id UNINDEXED,
			tokenize = 'porter unicode61'
		)
	`); err != nil {
		return err
	}
	return nil
}

func backfillChunkHashes(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT chunk_id, text
		FROM chunks
		WHERE text_hash IS NULL OR text_hash = ''
	`)
	if err != nil {
		return err
	}
	defer rows.Close()

	type entry struct {
		id   string
		hash string
	}
	var updates []entry
	for rows.Next() {
		var id string
		var text sql.NullString
		if err := rows.Scan(&id, &text); err != nil {
			return err
		}
		hash := sha256.Sum256([]byte(text.String))
		updates = append(updates, entry{id: id, hash: fmt.Sprintf("%x", hash[:])})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, u := range updates {
		if _, err := tx.Exec(`UPDATE chunks SET text_hash = ? WHERE chunk_id = ?`, u.hash, u.id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func dedupeChunks(db *sql.DB) error {
	_, err := db.Exec(`
		DELETE FROM chunks
		WHERE rowid NOT IN (
			SELECT MIN(rowid)
			FROM chunks
			GROUP BY repo_id, workspace, locator, text_hash, thread_id
		)
	`)
	return err
}

func ensureChunkUniqueIndex(db *sql.DB) error {
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_chunks_unique`); err != nil {
		return err
	}
	_, err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS idx_chunks_unique
		ON chunks (repo_id, workspace, locator, text_hash, thread_id)
	`)
	return err
}

func backfillWorkspaceColumns(db *sql.DB) error {
	if _, err := db.Exec(`UPDATE threads SET workspace = 'default' WHERE workspace IS NULL OR workspace = ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE memories SET workspace = 'default' WHERE workspace IS NULL OR workspace = ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE artifacts SET workspace = 'default' WHERE workspace IS NULL OR workspace = ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE chunks SET workspace = 'default' WHERE workspace IS NULL OR workspace = ''`); err != nil {
		return err
	}
	return nil
}

func ensureWorkspaceIndexes(db *sql.DB) error {
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_memories_repo_created`); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_memories_thread`); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_chunks_repo_created`); err != nil {
		return err
	}
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_chunks_thread`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_memories_repo_created
		ON memories (repo_id, workspace, created_at)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_memories_thread
		ON memories (repo_id, workspace, thread_id)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_chunks_repo_created
		ON chunks (repo_id, workspace, created_at)
	`); err != nil {
		return err
	}
	if _, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_chunks_thread
		ON chunks (repo_id, workspace, thread_id)
	`); err != nil {
		return err
	}
	return nil
}

func rebuildThreadsTable(db *sql.DB) error {
	columnsAdded, err := columnExists(db, "threads", "workspace")
	if err != nil {
		return err
	}
	if !columnsAdded {
		if err := ensureColumn(db, "threads", "workspace", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
			return err
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DROP TABLE IF EXISTS threads_new`); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		CREATE TABLE IF NOT EXISTS threads_new (
			thread_id TEXT NOT NULL,
			repo_id TEXT NOT NULL,
			workspace TEXT NOT NULL DEFAULT 'default',
			title TEXT,
			tags_json TEXT,
			created_at TEXT NOT NULL,
			PRIMARY KEY (thread_id, repo_id, workspace)
		)
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`
		INSERT INTO threads_new (thread_id, repo_id, workspace, title, tags_json, created_at)
		SELECT thread_id, repo_id, workspace, title, tags_json, created_at
		FROM threads
	`); err != nil {
		return err
	}

	if _, err := tx.Exec(`DROP TABLE threads`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE threads_new RENAME TO threads`); err != nil {
		return err
	}

	return tx.Commit()
}
