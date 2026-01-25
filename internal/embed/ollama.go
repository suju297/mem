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
	results := make([][]float64, 0, len(texts))
	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("embedding text is empty")
		}
		embedding, err := p.embedOne(text)
		if err != nil {
			return nil, err
		}
		results = append(results, embedding)
	}
	return results, nil
}

type ollamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type ollamaEmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
	Error     string    `json:"error,omitempty"`
}

func (p *OllamaProvider) embedOne(text string) ([]float64, error) {
	reqBody, err := json.Marshal(ollamaEmbeddingRequest{
		Model:  p.model,
		Prompt: text,
	})
	if err != nil {
		return nil, err
	}

	url := strings.TrimRight(p.baseURL, "/") + "/api/embeddings"
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
		return nil, fmt.Errorf("ollama embeddings error: %s", payload.Error)
	}
	if len(payload.Embedding) == 0 {
		return nil, fmt.Errorf("ollama embeddings returned empty vector")
	}
	return payload.Embedding, nil
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
