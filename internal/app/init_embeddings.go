package app

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"mem/internal/config"
	"mem/internal/embed"
)

type embeddingModelChoice struct {
	Label       string
	Model       string
	Description string
}

var recommendedEmbeddingModels = []embeddingModelChoice{
	{
		Label:       "nomic-embed-text",
		Model:       "nomic-embed-text",
		Description: "Balanced default for repo search",
	},
	{
		Label:       "mxbai-embed-large",
		Model:       "mxbai-embed-large",
		Description: "Higher quality, heavier model",
	},
	{
		Label:       "all-minilm",
		Model:       "all-minilm",
		Description: "Smaller and faster model",
	},
}

var (
	initEmbeddingPromptInteractive = func() bool {
		return isInteractiveTerminal(os.Stdin)
	}
	initEmbeddingLookPath             = exec.LookPath
	initEmbeddingCheckOllamaAvailable = embedCheckOllamaAvailable
	initEmbeddingInstallOllama        = installOllama
	initEmbeddingPullModel            = pullOllamaModel
)

func maybeRunInitEmbeddingSetup(cfg *config.Config, out, errOut io.Writer, configExisted bool) {
	if cfg == nil {
		return
	}
	if !shouldPromptForEmbeddingSetup(*cfg, configExisted) {
		return
	}
	if !initEmbeddingPromptInteractive() {
		return
	}

	fmt.Fprintln(errOut)
	fmt.Fprintln(errOut, "Embeddings are optional. Keyword search works without them.")

	reader := bufio.NewReader(os.Stdin)

	enableEmbeddings, err := promptYesNo(reader, errOut, "Enable local embeddings for semantic search and clustering?", false)
	if err != nil {
		fmt.Fprintf(errOut, "embedding setup skipped: %v\n", err)
		return
	}

	cfg.EmbeddingSetupComplete = true
	if !enableEmbeddings {
		cfg.EmbeddingProvider = "none"
		if saveErr := cfg.SaveEmbeddingState(); saveErr != nil {
			fmt.Fprintf(errOut, "warning: failed to save embedding preference: %v\n", saveErr)
			return
		}
		fmt.Fprintf(errOut, "Embeddings stay disabled. You can enable them later in %s.\n", filepath.Join(cfg.ConfigDir, "config.toml"))
		return
	}

	cfg.EmbeddingProvider = "ollama"

	hasOllama := ollamaInstalled()
	ollamaReachable, _ := initEmbeddingCheckOllamaAvailable("")
	if !hasOllama {
		fmt.Fprintln(errOut, "Mem uses Ollama for local embeddings.")
		installNow, promptErr := promptYesNo(reader, errOut, "Ollama is not installed. Install it now for you?", false)
		if promptErr != nil {
			fmt.Fprintf(errOut, "embedding setup skipped: %v\n", promptErr)
			return
		}
		if installNow {
			if err := initEmbeddingInstallOllama(errOut); err != nil {
				fmt.Fprintf(errOut, "warning: Ollama install failed: %v\n", err)
				fmt.Fprintln(errOut, "You can install it later from https://ollama.com/download")
			} else {
				hasOllama = true
				ollamaReachable, _ = initEmbeddingCheckOllamaAvailable("")
			}
		} else {
			fmt.Fprintln(errOut, "You can install Ollama later from https://ollama.com/download")
		}
	} else if !ollamaReachable {
		fmt.Fprintln(errOut, "Ollama is installed but not reachable right now. You can still save the embedding choice and start Ollama later.")
	}

	model, err := promptEmbeddingModelChoice(reader, errOut)
	if err != nil {
		fmt.Fprintf(errOut, "embedding setup skipped: %v\n", err)
		return
	}
	cfg.EmbeddingModel = model

	if saveErr := cfg.SaveEmbeddingState(); saveErr != nil {
		fmt.Fprintf(errOut, "warning: failed to save embedding settings: %v\n", saveErr)
		return
	}

	if hasOllama {
		pullNow, promptErr := promptYesNo(reader, errOut, fmt.Sprintf("Pull %s now?", model), true)
		if promptErr != nil {
			fmt.Fprintf(errOut, "warning: failed to read pull confirmation: %v\n", promptErr)
		} else if pullNow {
			if err := initEmbeddingPullModel(model, errOut); err != nil {
				fmt.Fprintf(errOut, "warning: failed to pull %s: %v\n", model, err)
				fmt.Fprintf(errOut, "You can pull it later with: ollama pull %s\n", model)
			}
		}
	}

	fmt.Fprintf(errOut, "Saved embedding settings: provider=%s model=%s\n", cfg.EmbeddingProvider, cfg.EmbeddingModel)
	if !hasOllama {
		fmt.Fprintf(errOut, "After installing Ollama, run: ollama pull %s\n", model)
	}
	fmt.Fprintln(errOut, "When you want to backfill vectors for existing data, run: mem embed")
}

func shouldPromptForEmbeddingSetup(cfg config.Config, configExisted bool) bool {
	if cfg.EmbeddingSetupComplete {
		return false
	}

	provider := strings.ToLower(strings.TrimSpace(cfg.EmbeddingProvider))
	switch provider {
	case "auto", "ollama":
		return false
	case "none", "":
		return true
	default:
		return !configExisted
	}
}

func promptEmbeddingModelChoice(in io.Reader, out io.Writer) (string, error) {
	fmt.Fprintln(out, "Choose an embedding model:")
	for i, option := range recommendedEmbeddingModels {
		fmt.Fprintf(out, "  %d) %s - %s\n", i+1, option.Label, option.Description)
	}
	customChoice := strconv.Itoa(len(recommendedEmbeddingModels) + 1)
	fmt.Fprintf(out, "  %s) custom - enter a model name yourself\n", customChoice)

	for {
		value, err := promptText(in, out, "Model choice", false)
		if err != nil {
			return "", err
		}

		trimmed := strings.TrimSpace(value)
		if trimmed == customChoice {
			return promptText(in, out, "Custom Ollama embedding model", false)
		}
		for i, option := range recommendedEmbeddingModels {
			if trimmed == strconv.Itoa(i+1) {
				return option.Model, nil
			}
		}

		for _, option := range recommendedEmbeddingModels {
			if strings.EqualFold(value, option.Label) || strings.EqualFold(value, option.Model) {
				return option.Model, nil
			}
		}
		fmt.Fprintf(out, "Choose 1-%d or enter one of the listed model names.\n", len(recommendedEmbeddingModels)+1)
	}
}

func ollamaInstalled() bool {
	_, err := initEmbeddingLookPath("ollama")
	return err == nil
}

func installOllama(out io.Writer) error {
	switch runtime.GOOS {
	case "darwin", "linux":
		cmd := exec.Command("sh", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
		cmd.Stdin = os.Stdin
		cmd.Stdout = out
		cmd.Stderr = out
		return cmd.Run()
	case "windows":
		cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm https://ollama.com/install.ps1 | iex")
		cmd.Stdin = os.Stdin
		cmd.Stdout = out
		cmd.Stderr = out
		return cmd.Run()
	default:
		return fmt.Errorf("automatic Ollama install is not supported on %s; use https://ollama.com/download", runtime.GOOS)
	}
}

func pullOllamaModel(model string, out io.Writer) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return fmt.Errorf("embedding model is required")
	}
	cmd := exec.Command("ollama", "pull", model)
	cmd.Stdin = os.Stdin
	cmd.Stdout = out
	cmd.Stderr = out
	return cmd.Run()
}

func embedCheckOllamaAvailable(model string) (bool, string) {
	return embed.CheckOllamaAvailable(model)
}
