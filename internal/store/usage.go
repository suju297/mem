package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	UsageScopeOverall = "overall"
	UsageScopeRepo    = "repo"
)

type UsageDelta struct {
	RequestCount    int
	CandidateTokens int
	UsedTokens      int
	SavedTokens     int
	TruncatedTokens int
	DroppedTokens   int
}

type UsageRollup struct {
	Scope           string
	RepoID          string
	RequestCount    int
	CandidateTokens int
	UsedTokens      int
	SavedTokens     int
	TruncatedTokens int
	DroppedTokens   int
	UpdatedAt       time.Time
}

type UsageSnapshot struct {
	Repo    UsageRollup
	Overall UsageRollup
}

func OpenUsage(path string) (*Store, error) {
	db, err := openSQLite(path)
	if err != nil {
		return nil, err
	}
	if err := migrateUsage(db); err != nil {
		return nil, fmt.Errorf("usage migration failed: %w", err)
	}
	return &Store{db: db}, nil
}

func migrateUsage(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS usage_rollups (
			scope TEXT NOT NULL,
			repo_id TEXT NOT NULL DEFAULT '',
			request_count INTEGER NOT NULL DEFAULT 0,
			candidate_tokens INTEGER NOT NULL DEFAULT 0,
			used_tokens INTEGER NOT NULL DEFAULT 0,
			saved_tokens INTEGER NOT NULL DEFAULT 0,
			truncated_tokens INTEGER NOT NULL DEFAULT 0,
			dropped_tokens INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (scope, repo_id)
		)
	`); err != nil {
		return err
	}
	_, err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_usage_rollups_repo
		ON usage_rollups (repo_id)
		WHERE scope = 'repo'
	`)
	return err
}

func (s *Store) IncrementUsage(repoID string, delta UsageDelta, now time.Time) error {
	repoID = strings.TrimSpace(repoID)
	if repoID == "" {
		return fmt.Errorf("repo_id is required")
	}
	updatedAt := now.UTC().Format(time.RFC3339Nano)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	if err := upsertUsageRollup(tx, UsageScopeRepo, repoID, delta, updatedAt); err != nil {
		return err
	}
	if err := upsertUsageRollup(tx, UsageScopeOverall, "", delta, updatedAt); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func upsertUsageRollup(tx *sql.Tx, scope, repoID string, delta UsageDelta, updatedAt string) error {
	_, err := tx.Exec(`
		INSERT INTO usage_rollups (
			scope, repo_id, request_count, candidate_tokens, used_tokens,
			saved_tokens, truncated_tokens, dropped_tokens, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope, repo_id) DO UPDATE SET
			request_count = usage_rollups.request_count + excluded.request_count,
			candidate_tokens = usage_rollups.candidate_tokens + excluded.candidate_tokens,
			used_tokens = usage_rollups.used_tokens + excluded.used_tokens,
			saved_tokens = usage_rollups.saved_tokens + excluded.saved_tokens,
			truncated_tokens = usage_rollups.truncated_tokens + excluded.truncated_tokens,
			dropped_tokens = usage_rollups.dropped_tokens + excluded.dropped_tokens,
			updated_at = excluded.updated_at
	`, scope, repoID, delta.RequestCount, delta.CandidateTokens, delta.UsedTokens, delta.SavedTokens, delta.TruncatedTokens, delta.DroppedTokens, updatedAt)
	return err
}

func (s *Store) GetUsageSnapshot(repoID string) (UsageSnapshot, error) {
	repoID = strings.TrimSpace(repoID)
	repo, err := s.GetUsageRollup(UsageScopeRepo, repoID)
	if err != nil {
		return UsageSnapshot{}, err
	}
	overall, err := s.GetUsageRollup(UsageScopeOverall, "")
	if err != nil {
		return UsageSnapshot{}, err
	}
	return UsageSnapshot{
		Repo:    repo,
		Overall: overall,
	}, nil
}

func (s *Store) GetUsageRollup(scope, repoID string) (UsageRollup, error) {
	scope = normalizeUsageScope(scope)
	repoID = strings.TrimSpace(repoID)
	if scope == UsageScopeOverall {
		repoID = ""
	}
	row := s.db.QueryRow(`
		SELECT scope, repo_id, request_count, candidate_tokens, used_tokens,
			saved_tokens, truncated_tokens, dropped_tokens, updated_at
		FROM usage_rollups
		WHERE scope = ? AND repo_id = ?
	`, scope, repoID)

	var usage UsageRollup
	var updatedAt string
	if err := row.Scan(
		&usage.Scope,
		&usage.RepoID,
		&usage.RequestCount,
		&usage.CandidateTokens,
		&usage.UsedTokens,
		&usage.SavedTokens,
		&usage.TruncatedTokens,
		&usage.DroppedTokens,
		&updatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return UsageRollup{Scope: scope, RepoID: repoID}, nil
		}
		return UsageRollup{}, err
	}
	usage.UpdatedAt = parseTime(updatedAt)
	return usage, nil
}

func normalizeUsageScope(scope string) string {
	switch strings.ToLower(strings.TrimSpace(scope)) {
	case UsageScopeRepo:
		return UsageScopeRepo
	default:
		return UsageScopeOverall
	}
}
