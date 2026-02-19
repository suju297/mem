package embed

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	autoCheckTTL     = 30 * time.Second
	autoCheckTimeout = 500 * time.Millisecond
)

var (
	autoCheckMu        sync.Mutex
	autoCheckUntil     time.Time
	autoCheckAvailable bool
	autoCheckError     string
	autoCheckModel     string
)

func checkOllamaAvailable(model string) (bool, string) {
	now := time.Now()
	autoCheckMu.Lock()
	if now.Before(autoCheckUntil) && autoCheckModel == model {
		available := autoCheckAvailable
		errMsg := autoCheckError
		autoCheckMu.Unlock()
		return available, errMsg
	}
	autoCheckMu.Unlock()

	available, errMsg := probeOllama(model)

	autoCheckMu.Lock()
	if now.Before(autoCheckUntil) && autoCheckModel == model {
		available = autoCheckAvailable
		errMsg = autoCheckError
		autoCheckMu.Unlock()
		return available, errMsg
	}
	autoCheckAvailable = available
	autoCheckError = errMsg
	autoCheckUntil = time.Now().Add(autoCheckTTL)
	autoCheckModel = model
	autoCheckMu.Unlock()

	return available, errMsg
}

func probeOllama(model string) (bool, string) {
	baseURL := strings.TrimRight(resolveOllamaURL(), "/")
	url := baseURL + "/api/tags"
	client := &http.Client{Timeout: autoCheckTimeout}

	resp, err := client.Get(url)
	if err != nil {
		return false, fmt.Sprintf("ollama unavailable at %s", baseURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Sprintf("embeddings_unavailable: ollama unavailable (status %d)", resp.StatusCode)
	}

	var payload ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, "embeddings_unavailable: failed to read ollama tags"
	}
	if strings.TrimSpace(model) != "" && !ollamaHasModel(payload, model) {
		return false, fmt.Sprintf("embeddings_unavailable: ollama model not found (%s). Run: ollama pull %s", model, model)
	}
	return true, ""
}

type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func ollamaHasModel(payload ollamaTagsResponse, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, entry := range payload.Models {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		if name == model || strings.HasPrefix(name, model+":") {
			return true
		}
	}
	return false
}
