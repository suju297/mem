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
	noAgents := fs.Bool("no-agents", false, "Skip writing .mempack/MEMORY.md and assistant stub files")
	assistantsFlag := fs.String("assistants", "agents", "Comma-separated assistant stubs to write: agents,claude,gemini,all")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	targets := []assistantStubTarget{assistantTargetAgents}
	if !*noAgents {
		var err error
		targets, err = parseAssistantStubTargets(*assistantsFlag)
		if err != nil {
			fmt.Fprintf(errOut, "invalid --assistants: %v\n", err)
			return 2
		}
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
	repoInfo, err := resolveRepo(&cfg, "")
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
	if err := cfg.SaveRepoState(); err != nil {
		fmt.Fprintf(errOut, "config save error: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "Initialized memory for repo: %s\n", repoInfo.ID)
	fmt.Fprintf(out, "Root: %s\n\n", repoInfo.GitRoot)
	fmt.Fprintln(out, "Try these commands:")
	fmt.Fprintln(out, "  mem add --title \"My Feature\" --summary \"Planning the API\"")
	fmt.Fprintln(out, "  mem get \"planning\"")
	fmt.Fprintln(out, "  mem repos")

	if !*noAgents {
		result, err := writeAgentFiles(repoInfo.GitRoot, targets, true)
		if err != nil {
			fmt.Fprintf(errOut, "warning: failed to write agent instructions: %v\n", err)
		} else {
			fmt.Fprintf(out, "Generated .mempack/MEMORY.md and assistant stubs: %s (when missing).\n", assistantStubTargetsLabel(targets))
			if result.WroteAlternate {
				fmt.Fprintln(out, "AGENTS.md already exists; wrote .mempack/AGENTS.md. Add the following 2 lines to AGENTS.md:")
				for _, line := range agentsStubHintLines() {
					fmt.Fprintln(out, line)
				}
			}
		}
	}

	return 0
}

type agentFilesResult struct {
	AgentsPath     string
	WroteAlternate bool
}

type assistantStubTarget string

const (
	assistantTargetAgents assistantStubTarget = "agents"
	assistantTargetClaude assistantStubTarget = "claude"
	assistantTargetGemini assistantStubTarget = "gemini"
)

var assistantTargetOrder = []assistantStubTarget{
	assistantTargetAgents,
	assistantTargetClaude,
	assistantTargetGemini,
}

func parseAssistantStubTargets(raw string) ([]assistantStubTarget, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return []assistantStubTarget{assistantTargetAgents}, nil
	}

	seen := map[assistantStubTarget]bool{}
	targets := make([]assistantStubTarget, 0, len(assistantTargetOrder))
	add := func(target assistantStubTarget) {
		if seen[target] {
			return
		}
		seen[target] = true
		targets = append(targets, target)
	}

	for _, token := range strings.Split(trimmed, ",") {
		value := strings.TrimSpace(token)
		if value == "" {
			continue
		}
		switch value {
		case "all":
			for _, target := range assistantTargetOrder {
				add(target)
			}
		case "agents", "agent", "agents.md":
			add(assistantTargetAgents)
		case "claude", "claude.md":
			add(assistantTargetClaude)
		case "gemini", "gemini.md":
			add(assistantTargetGemini)
		default:
			return nil, fmt.Errorf("unknown target %q (use agents, claude, gemini, all)", value)
		}
	}

	if len(targets) == 0 {
		return []assistantStubTarget{assistantTargetAgents}, nil
	}
	return targets, nil
}

func assistantStubTargetsLabel(targets []assistantStubTarget) string {
	labels := make([]string, 0, len(targets))
	for _, target := range targets {
		switch target {
		case assistantTargetAgents:
			labels = append(labels, "AGENTS.md")
		case assistantTargetClaude:
			labels = append(labels, "CLAUDE.md")
		case assistantTargetGemini:
			labels = append(labels, "GEMINI.md")
		}
	}
	if len(labels) == 0 {
		return "none"
	}
	return strings.Join(labels, ", ")
}

func writeAgentFiles(root string, targets []assistantStubTarget, includeMemory bool) (agentFilesResult, error) {
	if includeMemory {
		if err := writeMemoryInstructions(root); err != nil {
			return agentFilesResult{}, err
		}
	}
	return writeAssistantStubs(root, targets)
}

func writeAssistantStubs(root string, targets []assistantStubTarget) (agentFilesResult, error) {
	var result agentFilesResult
	for _, target := range targets {
		switch target {
		case assistantTargetAgents:
			agentsResult, err := writeAgentsStub(root)
			if err != nil {
				return agentFilesResult{}, err
			}
			if agentsResult.AgentsPath != "" {
				result.AgentsPath = agentsResult.AgentsPath
			}
			if agentsResult.WroteAlternate {
				result.WroteAlternate = true
			}
		case assistantTargetClaude:
			if err := writeFileIfMissing(resolveRootPath(root, "CLAUDE.md"), claudeStubContent()); err != nil {
				return agentFilesResult{}, err
			}
		case assistantTargetGemini:
			if err := writeFileIfMissing(resolveRootPath(root, "GEMINI.md"), geminiStubContent()); err != nil {
				return agentFilesResult{}, err
			}
		}
	}
	return result, nil
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

func writeFileIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func resolveRootPath(root, name string) string {
	if root != "" {
		return filepath.Join(root, name)
	}
	return name
}

func isConstraintError(err error) bool {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code()&0xff == sqlite3.SQLITE_CONSTRAINT
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "constraint failed") || strings.Contains(msg, "constraint violation")
}

func maybeUpdateAgentFiles(root string) {
	if strings.TrimSpace(root) == "" {
		return
	}
	if err := maybeRefreshMemoryInstructions(root); err != nil {
		return
	}
	if err := maybeRefreshAgentsStub(root); err != nil {
		return
	}
	if err := maybeRefreshCompatibilityStubs(root); err != nil {
		return
	}
}

func maybeRefreshMemoryInstructions(root string) error {
	path := filepath.Join(root, ".mempack", "MEMORY.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)
	if !isManagedMemoryInstructions(content) {
		return nil
	}
	return writeMemoryInstructions(root)
}

func maybeRefreshAgentsStub(root string) error {
	rootPath := filepath.Join(root, "AGENTS.md")
	data, err := os.ReadFile(rootPath)
	if err == nil {
		if isManagedAgentsStub(string(data)) {
			if err := os.WriteFile(rootPath, []byte(agentsStubContent()), 0644); err != nil {
				return err
			}
		}
	}
	altPath := filepath.Join(root, ".mempack", "AGENTS.md")
	altData, err := os.ReadFile(altPath)
	if err != nil {
		return nil
	}
	if isManagedAgentsStub(string(altData)) {
		return os.WriteFile(altPath, []byte(agentsStubContent()), 0644)
	}
	return nil
}

func maybeRefreshCompatibilityStubs(root string) error {
	if err := maybeRefreshStubFile(filepath.Join(root, "CLAUDE.md"), claudeStubContent(), isManagedClaudeStub); err != nil {
		return err
	}
	if err := maybeRefreshStubFile(filepath.Join(root, "GEMINI.md"), geminiStubContent(), isManagedGeminiStub); err != nil {
		return err
	}
	return nil
}

func maybeRefreshStubFile(path, content string, managedFn func(string) bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if !managedFn(string(data)) {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func isManagedMemoryInstructions(content string) bool {
	normalized := strings.ToLower(content)
	if strings.Contains(normalized, memoryManagedMarker) {
		return true
	}
	if strings.Contains(normalized, "mempack instructions") &&
		strings.Contains(normalized, "do not edit this file") &&
		(strings.Contains(normalized, "mempack_get_context") || strings.Contains(normalized, "mempack.get_context")) {
		return true
	}
	return false
}

func isManagedAgentsStub(content string) bool {
	normalized := normalizeStub(content)
	stub := normalizeStub(agentsStubContent())
	if normalized == stub {
		return true
	}
	return false
}

func isManagedClaudeStub(content string) bool {
	return normalizeStub(content) == normalizeStub(claudeStubContent())
}

func isManagedGeminiStub(content string) bool {
	return normalizeStub(content) == normalizeStub(geminiStubContent())
}

func normalizeStub(content string) string {
	return strings.TrimSpace(strings.ReplaceAll(content, "\r\n", "\n"))
}
