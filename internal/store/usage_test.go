package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenUsageCreatesRollupTableFreshAndExisting(t *testing.T) {
	cases := []struct {
		name         string
		prepareDB    func(t *testing.T, path string)
		expectedRows int
	}{
		{
			name:      "fresh",
			prepareDB: func(t *testing.T, path string) {},
		},
		{
			name: "existing",
			prepareDB: func(t *testing.T, path string) {
				db, err := sql.Open("sqlite", path)
				if err != nil {
					t.Fatalf("open sqlite: %v", err)
				}
				defer db.Close()
				if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS sentinel (id INTEGER PRIMARY KEY)`); err != nil {
					t.Fatalf("create sentinel: %v", err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "usage.db")
			tc.prepareDB(t, path)

			st, err := OpenUsage(path)
			if err != nil {
				t.Fatalf("open usage store: %v", err)
			}
			defer st.Close()

			row := st.db.QueryRow(`
				SELECT COUNT(*)
				FROM sqlite_master
				WHERE type = 'table' AND name = 'usage_rollups'
			`)
			var count int
			if err := row.Scan(&count); err != nil {
				t.Fatalf("scan table count: %v", err)
			}
			if count != 1 {
				t.Fatalf("expected usage_rollups table to exist, got count %d", count)
			}
		})
	}
}

func TestUsageRollupsTrackRepoAndOverall(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	st, err := OpenUsage(path)
	if err != nil {
		t.Fatalf("open usage store: %v", err)
	}
	defer st.Close()

	if err := st.IncrementUsage("repo-a", UsageDelta{
		RequestCount:    1,
		CandidateTokens: 20,
		UsedTokens:      11,
		SavedTokens:     9,
		TruncatedTokens: 4,
		DroppedTokens:   5,
	}, time.Unix(10, 0)); err != nil {
		t.Fatalf("increment repo-a first: %v", err)
	}
	if err := st.IncrementUsage("repo-a", UsageDelta{
		RequestCount:    1,
		CandidateTokens: 12,
		UsedTokens:      7,
		SavedTokens:     5,
		TruncatedTokens: 2,
		DroppedTokens:   3,
	}, time.Unix(20, 0)); err != nil {
		t.Fatalf("increment repo-a second: %v", err)
	}
	if err := st.IncrementUsage("repo-b", UsageDelta{
		RequestCount:    1,
		CandidateTokens: 30,
		UsedTokens:      18,
		SavedTokens:     12,
		TruncatedTokens: 6,
		DroppedTokens:   6,
	}, time.Unix(30, 0)); err != nil {
		t.Fatalf("increment repo-b: %v", err)
	}

	snapshotA, err := st.GetUsageSnapshot("repo-a")
	if err != nil {
		t.Fatalf("get usage snapshot repo-a: %v", err)
	}
	if snapshotA.Repo.RequestCount != 2 {
		t.Fatalf("expected repo-a request_count 2, got %d", snapshotA.Repo.RequestCount)
	}
	if snapshotA.Repo.CandidateTokens != 32 {
		t.Fatalf("expected repo-a candidate_tokens 32, got %d", snapshotA.Repo.CandidateTokens)
	}
	if snapshotA.Repo.UsedTokens != 18 {
		t.Fatalf("expected repo-a used_tokens 18, got %d", snapshotA.Repo.UsedTokens)
	}
	if snapshotA.Repo.SavedTokens != 14 {
		t.Fatalf("expected repo-a saved_tokens 14, got %d", snapshotA.Repo.SavedTokens)
	}
	if snapshotA.Repo.TruncatedTokens != 6 {
		t.Fatalf("expected repo-a truncated_tokens 6, got %d", snapshotA.Repo.TruncatedTokens)
	}
	if snapshotA.Repo.DroppedTokens != 8 {
		t.Fatalf("expected repo-a dropped_tokens 8, got %d", snapshotA.Repo.DroppedTokens)
	}
	if snapshotA.Overall.RequestCount != 3 {
		t.Fatalf("expected overall request_count 3, got %d", snapshotA.Overall.RequestCount)
	}
	if snapshotA.Overall.SavedTokens != 26 {
		t.Fatalf("expected overall saved_tokens 26, got %d", snapshotA.Overall.SavedTokens)
	}

	snapshotMissing, err := st.GetUsageSnapshot("repo-missing")
	if err != nil {
		t.Fatalf("get usage snapshot missing repo: %v", err)
	}
	if snapshotMissing.Repo.RequestCount != 0 {
		t.Fatalf("expected missing repo request_count 0, got %d", snapshotMissing.Repo.RequestCount)
	}
	if snapshotMissing.Overall.RequestCount != 3 {
		t.Fatalf("expected overall request_count 3, got %d", snapshotMissing.Overall.RequestCount)
	}
}
