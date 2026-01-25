package store

import (
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"mempack/internal/repo"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

//go:embed triggers.sql
var triggersSQL string

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA synchronous=NORMAL;"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA busy_timeout=3000;"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA cache_size=-20000;"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA temp_store=MEMORY;"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA mmap_size=268435456;"); err != nil {
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("schema migration failed: %w", err)
	}
	if _, err := db.Exec(triggersSQL); err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) EnsureRepo(info repo.Info) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	originHash := ""
	if info.Origin != "" {
		originHash = sha256Hex(info.Origin)
	}

	_, err := s.db.Exec(`
		INSERT INTO repos (repo_id, git_root, origin_hash, last_head, last_branch, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repo_id)
		DO UPDATE SET
			git_root = excluded.git_root,
			origin_hash = CASE WHEN excluded.origin_hash = '' THEN repos.origin_hash ELSE excluded.origin_hash END,
			last_head = CASE WHEN excluded.last_head = '' THEN repos.last_head ELSE excluded.last_head END,
			last_branch = CASE WHEN excluded.last_branch = '' THEN repos.last_branch ELSE excluded.last_branch END,
			last_seen_at = excluded.last_seen_at
	`, info.ID, info.GitRoot, originHash, info.Head, info.Branch, now, now)
	return err
}

func (s *Store) GetStateCurrent(repoID, workspace string) (string, int, string, error) {
	row := s.db.QueryRow(`
		SELECT state_json, state_tokens, updated_at
		FROM state_current
		WHERE repo_id = ? AND workspace = ?
	`, repoID, workspace)

	var stateJSON string
	var stateTokens sql.NullInt64
	var updatedAt string
	if err := row.Scan(&stateJSON, &stateTokens, &updatedAt); err != nil {
		return "", 0, "", err
	}
	tokens := 0
	if stateTokens.Valid {
		tokens = int(stateTokens.Int64)
	}
	return stateJSON, tokens, updatedAt, nil
}

func sha256Hex(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}
