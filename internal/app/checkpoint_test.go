package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStatePayloadWrapsNonJSON(t *testing.T) {
	payload, err := loadStatePayload("", "plain text")
	if err != nil {
		t.Fatalf("load state payload: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if decoded["raw"] != "plain text" {
		t.Fatalf("expected raw field to be preserved")
	}
}

func TestLoadStatePayloadFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, []byte(`{"goal":"test"}`), 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	payload, err := loadStatePayload(path, "")
	if err != nil {
		t.Fatalf("load state payload: %v", err)
	}

	var decoded map[string]string
	if err := json.Unmarshal([]byte(payload), &decoded); err == nil {
		if _, ok := decoded["goal"]; ok {
			return
		}
	}

	if payload != `{"goal":"test"}` {
		t.Fatalf("expected payload to match file contents")
	}
}
