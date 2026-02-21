package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mempack/internal/repo"
	"mempack/internal/store"
)

func TestLoadStateWarnsOnDBError(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".mempack")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}

	stateJSON := []byte(`{"ok":true}`)
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), stateJSON, 0o644); err != nil {
		t.Fatalf("write state.json: %v", err)
	}

	dbPath := filepath.Join(root, "memory.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	stateRaw, _, _, _, warning, err := loadState(repo.Info{ID: "r1", GitRoot: root}, "default", st)
	if err != nil {
		t.Fatalf("loadState error: %v", err)
	}
	if warning == "" || !strings.Contains(warning, "state_db_error:") {
		t.Fatalf("expected state_db_error warning, got %q", warning)
	}
	if string(stateRaw) != string(stateJSON) {
		t.Fatalf("expected state from repo file, got %s", string(stateRaw))
	}
}
