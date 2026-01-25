package embed

import (
	"fmt"
	"strings"

	"mempack/internal/config"
)

type Provider interface {
	Name() string
	Embed(texts []string) ([][]float64, error)
}

type Status struct {
	Provider string
	Model    string
	Enabled  bool
	Error    string
}

const DefaultAutoModel = "nomic-embed-text"

func Resolve(cfg config.Config) (Provider, Status) {
	name := strings.TrimSpace(strings.ToLower(cfg.EmbeddingProvider))
	if name == "" || name == "none" {
		return nil, Status{Provider: "none", Enabled: false}
	}

	switch name {
	case "auto":
		model := strings.TrimSpace(cfg.EmbeddingModel)
		if model == "" {
			model = DefaultAutoModel
		}
		available, errMsg := checkOllamaAvailable(model)
		if !available {
			return nil, Status{
				Provider: "ollama",
				Model:    model,
				Enabled:  false,
				Error:    errMsg,
			}
		}
		return NewOllamaProvider(model), Status{
			Provider: "ollama",
			Model:    model,
			Enabled:  true,
		}
	case "ollama":
		model := strings.TrimSpace(cfg.EmbeddingModel)
		if model == "" {
			return nil, Status{
				Provider: name,
				Model:    model,
				Enabled:  false,
				Error:    "embedding_model is required for ollama",
			}
		}
		available, errMsg := checkOllamaAvailable(model)
		if !available {
			return nil, Status{
				Provider: name,
				Model:    model,
				Enabled:  false,
				Error:    errMsg,
			}
		}
		return NewOllamaProvider(model), Status{
			Provider: name,
			Model:    model,
			Enabled:  true,
		}
	case "python", "onnx":
		model := strings.TrimSpace(cfg.EmbeddingModel)
		return nil, Status{
			Provider: name,
			Model:    model,
			Enabled:  false,
			Error:    fmt.Sprintf("embedding provider not implemented: %s", name),
		}
	default:
		model := strings.TrimSpace(cfg.EmbeddingModel)
		return nil, Status{
			Provider: name,
			Model:    model,
			Enabled:  false,
			Error:    fmt.Sprintf("unknown embedding provider: %s", name),
		}
	}
}
