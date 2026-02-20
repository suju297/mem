package embed

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type OllamaProvider struct {
	baseURL string
	model   string
	client  *http.Client
}

func NewOllamaProvider(model string) *OllamaProvider {
	return &OllamaProvider{
		baseURL: resolveOllamaURL(),
		model:   model,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) Embed(texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("embedding text is empty")
		}
	}
	return p.embedBatch(texts)
}

type ollamaEmbeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbeddingResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
	Error      string      `json:"error,omitempty"`
}

func (p *OllamaProvider) embedBatch(texts []string) ([][]float64, error) {
	reqBody, err := json.Marshal(ollamaEmbeddingRequest{
		Model: p.model,
		Input: texts,
	})
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(p.baseURL, "/") + "/api/embed"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ollama embeddings error: %s", strings.TrimSpace(string(body)))
	}

	var payload ollamaEmbeddingResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Error != "" {
		return nil, fmt.Errorf("ollama embed error: %s", payload.Error)
	}
	if len(payload.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed count mismatch: expected %d, got %d", len(texts), len(payload.Embeddings))
	}
	for i, vector := range payload.Embeddings {
		if len(vector) == 0 {
			return nil, fmt.Errorf("ollama embed returned empty vector at index %d", i)
		}
	}
	return payload.Embeddings, nil
}

func resolveOllamaURL() string {
	host := strings.TrimSpace(os.Getenv("OLLAMA_HOST"))
	if host == "" {
		return "http://localhost:11434"
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "http://" + host
	}
	return strings.TrimRight(host, "/")
}
