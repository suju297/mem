package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mem/internal/repo"
	"mem/internal/store"
)

func TestLoadStateWarnsOnDBError(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".mem")
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

func TestLoadStateFromLegacyMempackDir(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, ".mempack")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}

	stateJSON := []byte(`{"legacy":true}`)
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), stateJSON, 0o644); err != nil {
		t.Fatalf("write state.json: %v", err)
	}

	stateRaw, source, _, err := loadStateFromRepoFiles(root)
	if err != nil {
		t.Fatalf("loadStateFromRepoFiles error: %v", err)
	}
	if source != ".mempack/state.json" {
		t.Fatalf("expected legacy source label, got %q", source)
	}
	if string(stateRaw) != string(stateJSON) {
		t.Fatalf("expected legacy state from repo file, got %s", string(stateRaw))
	}
}
