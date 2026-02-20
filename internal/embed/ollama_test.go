package embed

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestOllamaProviderEmbedBatch(t *testing.T) {
	var gotPath string
	var gotBody ollamaEmbeddingRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollamaEmbeddingResponse{
			Embeddings: [][]float64{
				{1, 2},
				{3, 4},
			},
		})
	}))
	defer server.Close()

	provider := &OllamaProvider{
		baseURL: server.URL,
		model:   "nomic-embed-text",
		client:  server.Client(),
	}

	vectors, err := provider.Embed([]string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if gotPath != "/api/embed" {
		t.Fatalf("expected path /api/embed, got %s", gotPath)
	}
	if gotBody.Model != "nomic-embed-text" {
		t.Fatalf("unexpected model: %s", gotBody.Model)
	}
	if !reflect.DeepEqual(gotBody.Input, []string{"alpha", "beta"}) {
		t.Fatalf("unexpected input: %#v", gotBody.Input)
	}
	if len(vectors) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(vectors))
	}
	if !reflect.DeepEqual(vectors[0], []float64{1, 2}) || !reflect.DeepEqual(vectors[1], []float64{3, 4}) {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}

func TestOllamaProviderEmbedBatchCountMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ollamaEmbeddingResponse{
			Embeddings: [][]float64{{1, 2}},
		})
	}))
	defer server.Close()

	provider := &OllamaProvider{
		baseURL: server.URL,
		model:   "nomic-embed-text",
		client:  server.Client(),
	}

	_, err := provider.Embed([]string{"alpha", "beta"})
	if err == nil {
		t.Fatal("expected count mismatch error")
	}
	if !strings.Contains(err.Error(), "count mismatch") {
		t.Fatalf("expected count mismatch error, got: %v", err)
	}
}
