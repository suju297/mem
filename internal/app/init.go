package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mempack/internal/store"
	"mempack/internal/token"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

func runInit(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(errOut)
	noAgents := fs.Bool("no-agents", false, "Skip writing .mempack/MEMORY.md and AGENTS.md")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		// If error is strictly file missing, we proceed with default.
		// But loadConfig might return error on parse failure.
		// For init, we usually assume we can overwrite or create?
		// But loadConfig currently returns default + nil error if missing.
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, "")

	// Detect current repo
	repoInfo, err := resolveRepo(cfg, "")
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		fmt.Fprintf(errOut, "store open error: %v\n", err)
		return 1
	}
	defer st.Close()

	if err := st.EnsureRepo(repoInfo); err != nil {
		fmt.Fprintf(errOut, "store repo error: %v\n", err)
		return 1
	}

	// Initialize tokenizer for adding welcome memory
	counter, err := token.New(cfg.Tokenizer)
	if err != nil {
		fmt.Fprintf(errOut, "tokenizer error: %v\n", err)
		return 1
	}

	// Create example thread/memory if empty?
	// But let's just add a welcome memory always?
	// "Creates an example thread".
	// Thread is created implicitly by AddMemory.
	welcomeID := "M-WELCOME"
	welcomeTitle := "Welcome to Memory"
	welcomeSummary := "This is your first memory repository. Use 'mem add' to store context and 'mem get' to retrieve it."

	// Create Thread record explicitly? Not strictly needed by schema but good for metadata.
	// Store.AddMemory handles thread creation in 'threads' table.

	_, err = st.AddMemory(store.AddMemoryInput{
		ID:            welcomeID,
		RepoID:        repoInfo.ID,
		Workspace:     workspaceName,
		ThreadID:      "T-WELCOME",
		Title:         welcomeTitle,
		Summary:       welcomeSummary,
		SummaryTokens: counter.Count(welcomeSummary),
		TagsJSON:      "[\"setup\"]",
		TagsText:      "setup",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		AnchorCommit:  repoInfo.Head, // Might be empty if no git, harmless
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		if !isConstraintError(err) {
			fmt.Fprintf(errOut, "add welcome memory error: %v\n", err)
			return 1
		}
	}

	// Set active repo
	cfg.ActiveRepo = repoInfo.ID
	if err := cfg.Save(); err != nil {
		fmt.Fprintf(errOut, "config save error: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "Initialized memory for repo: %s\n", repoInfo.ID)
	fmt.Fprintf(out, "Root: %s\n\n", repoInfo.GitRoot)
	fmt.Fprintln(out, "Try these commands:")
	fmt.Fprintln(out, "  mem add --thread T-1 --title \"My Feature\" --summary \"Planning the API\"")
	fmt.Fprintln(out, "  mem get \"planning\"")
	fmt.Fprintln(out, "  mem repos")

	if !*noAgents {
		result, err := writeAgentFiles(repoInfo.GitRoot)
		if err != nil {
			fmt.Fprintf(errOut, "warning: failed to write agent instructions: %v\n", err)
		} else if result.WroteAlternate {
			fmt.Fprintln(out, "Generated .mempack/MEMORY.md and .mempack/AGENTS.md")
			fmt.Fprintln(out, "AGENTS.md already exists; add the following 2 lines:")
			for _, line := range agentsStubHintLines() {
				fmt.Fprintln(out, line)
			}
		} else {
			fmt.Fprintln(out, "Generated .mempack/MEMORY.md and AGENTS.md")
		}
	}

	return 0
}

type agentFilesResult struct {
	AgentsPath     string
	WroteAlternate bool
}

func writeAgentFiles(root string) (agentFilesResult, error) {
	if err := writeMemoryInstructions(root); err != nil {
		return agentFilesResult{}, err
	}
	return writeAgentsStub(root)
}

func writeMemoryInstructions(root string) error {
	dir := ".mempack"
	path := filepath.Join(dir, "MEMORY.md")
	if root != "" {
		dir = filepath.Join(root, ".mempack")
		path = filepath.Join(dir, "MEMORY.md")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, []byte(memoryInstructionsContent()), 0644)
}

func writeAgentsStub(root string) (agentFilesResult, error) {
	path := "AGENTS.md"
	if root != "" {
		path = filepath.Join(root, "AGENTS.md")
	}
	if _, err := os.Stat(path); err == nil {
		altDir := ".mempack"
		if root != "" {
			altDir = filepath.Join(root, ".mempack")
		}
		if err := os.MkdirAll(altDir, 0o755); err != nil {
			return agentFilesResult{}, err
		}
		altPath := filepath.Join(altDir, "AGENTS.md")
		if err := os.WriteFile(altPath, []byte(agentsStubContent()), 0644); err != nil {
			return agentFilesResult{}, err
		}
		return agentFilesResult{AgentsPath: altPath, WroteAlternate: true}, nil
	}

	if err := os.WriteFile(path, []byte(agentsStubContent()), 0644); err != nil {
		return agentFilesResult{}, err
	}
	return agentFilesResult{AgentsPath: path}, nil
}

func isConstraintError(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code()&0xff == sqlite3.SQLITE_CONSTRAINT
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "constraint failed") || strings.Contains(msg, "constraint violation")
}
