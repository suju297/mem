package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestStateCurrentNormalizesWorkspaceOnWriteAndRead(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	now := time.Now().UTC()
	if err := st.SetStateCurrent("r1", "", `{"goal":"ship"}`, 2, now); err != nil {
		t.Fatalf("set state current: %v", err)
	}

	stateJSON, tokens, updatedAt, err := st.GetStateCurrent("r1", "default")
	if err != nil {
		t.Fatalf("get state current with default workspace: %v", err)
	}
	if stateJSON != `{"goal":"ship"}` {
		t.Fatalf("unexpected state json: %q", stateJSON)
	}
	if tokens != 2 {
		t.Fatalf("unexpected token count: %d", tokens)
	}
	if updatedAt == "" {
		t.Fatalf("expected updated_at to be set")
	}

	var emptyWorkspaceCount int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM state_current WHERE repo_id = ? AND workspace = ''`, "r1").Scan(&emptyWorkspaceCount); err != nil {
		t.Fatalf("count empty-workspace rows: %v", err)
	}
	if emptyWorkspaceCount != 0 {
		t.Fatalf("expected no empty-workspace state_current rows, got %d", emptyWorkspaceCount)
	}
}

func TestAddStateHistoryNormalizesWorkspace(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	now := time.Now().UTC()
	if err := st.AddStateHistory("S-1", "r1", "", `{"goal":"ship"}`, "manual", 2, now); err != nil {
		t.Fatalf("add state history: %v", err)
	}

	var defaultWorkspaceCount int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM state_history WHERE repo_id = ? AND workspace = ?`, "r1", "default").Scan(&defaultWorkspaceCount); err != nil {
		t.Fatalf("count default-workspace history rows: %v", err)
	}
	if defaultWorkspaceCount != 1 {
		t.Fatalf("expected one default-workspace state_history row, got %d", defaultWorkspaceCount)
	}

	var emptyWorkspaceCount int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM state_history WHERE repo_id = ? AND workspace = ''`, "r1").Scan(&emptyWorkspaceCount); err != nil {
		t.Fatalf("count empty-workspace history rows: %v", err)
	}
	if emptyWorkspaceCount != 0 {
		t.Fatalf("expected no empty-workspace state_history rows, got %d", emptyWorkspaceCount)
	}
}

func TestEnsureLinksIndexesDedupesBeforeUniqueIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE links (
			from_id TEXT NOT NULL,
			rel TEXT NOT NULL,
			to_id TEXT NOT NULL,
			weight REAL,
			created_at TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("create legacy links table: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`INSERT INTO links (from_id, rel, to_id, weight, created_at) VALUES ('M-1', 'relates_to', 'M-2', 1, ?), ('M-1', 'relates_to', 'M-2', 1, ?)`, now, now); err != nil {
		t.Fatalf("insert duplicate legacy links: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy db: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open store with migration: %v", err)
	}
	defer st.Close()

	row := st.db.QueryRow(`SELECT COUNT(*) FROM links WHERE from_id = 'M-1' AND rel = 'relates_to' AND to_id = 'M-2'`)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("count deduped links: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected deduped count 1, got %d", count)
	}

	if _, err := st.db.Exec(`INSERT INTO links (from_id, rel, to_id, weight, created_at) VALUES ('M-1', 'relates_to', 'M-2', 1, ?)`, now); err == nil {
		t.Fatalf("expected unique index to reject duplicate link")
	}
}
