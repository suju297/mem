package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mem/internal/config"
	"mem/internal/pathutil"
	"mem/internal/repo"
)

type repoDeletePlan struct {
	RepoID           string
	GitRoot          string
	RepoDataDir      string
	SupportPaths     []string
	ManagedStubPaths []string
	SkippedStubPaths []string
	ConfigCleanup    bool
}

var deletePromptInteractive = func() bool {
	return isInteractiveTerminal(os.Stdin)
}

func runDelete(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("delete", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id or path")
	yes := fs.Bool("yes", false, "Delete without interactive confirmation")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}

	repoInfo, err := resolveRepoWithOptions(&cfg, strings.TrimSpace(*repoOverride), repoResolveOptions{RequireRepo: true})
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}

	configPath := filepath.Join(cfg.ConfigDir, "config.toml")
	configExisted := false
	if _, err := os.Stat(configPath); err == nil {
		configExisted = true
	}

	plan := buildRepoDeletePlan(cfg, repoInfo, configExisted)
	if !plan.HasTargets() {
		fmt.Fprintf(out, "No Mem setup found for repo: %s\n", repoInfo.GitRoot)
		return 0
	}

	if !*yes {
		if !deletePromptInteractive() {
			fmt.Fprintln(errOut, "refusing to delete Mem setup in non-interactive mode without --yes")
			return 2
		}
		describeRepoDeletePlan(errOut, plan)
		ok, err := promptYesNo(os.Stdin, errOut, "Delete this Mem setup?", false)
		if err != nil {
			fmt.Fprintf(errOut, "delete aborted: %v\n", err)
			return 1
		}
		if !ok {
			fmt.Fprintln(errOut, "Aborted.")
			return 0
		}
	}

	if err := executeRepoDeletePlan(plan); err != nil {
		fmt.Fprintf(errOut, "delete error: %v\n", err)
		return 1
	}
	if err := clearRepoStateForDelete(&cfg, repoInfo, configExisted); err != nil {
		fmt.Fprintf(errOut, "delete warning: repo files removed, but config cleanup failed: %v\n", err)
		return 1
	}

	fmt.Fprintf(out, "Deleted Mem setup for repo: %s\n", plan.RepoID)
	fmt.Fprintf(out, "Root: %s\n", plan.GitRoot)
	if plan.RepoDataDir != "" {
		fmt.Fprintf(out, "Removed repo data: %s\n", plan.RepoDataDir)
	}
	for _, path := range plan.SupportPaths {
		fmt.Fprintf(out, "Removed repo support path: %s\n", path)
	}
	if len(plan.ManagedStubPaths) > 0 {
		fmt.Fprintf(out, "Removed managed stub files: %s\n", strings.Join(plan.ManagedStubPaths, ", "))
	}
	if len(plan.SkippedStubPaths) > 0 {
		fmt.Fprintf(out, "Left unchanged (not Mem-managed): %s\n", strings.Join(plan.SkippedStubPaths, ", "))
	}
	return 0
}

func buildRepoDeletePlan(cfg config.Config, info repo.Info, configExisted bool) repoDeletePlan {
	plan := repoDeletePlan{
		RepoID:  strings.TrimSpace(info.ID),
		GitRoot: strings.TrimSpace(info.GitRoot),
	}

	dataDir := filepath.Dir(cfg.RepoDBPath(info.ID))
	if pathExistsForDelete(dataDir) {
		plan.RepoDataDir = dataDir
	}

	for _, name := range []string{config.RepoSupportDirName, config.LegacyRepoSupportDirName} {
		path := filepath.Join(info.GitRoot, name)
		if pathExistsForDelete(path) {
			plan.SupportPaths = append(plan.SupportPaths, path)
		}
	}

	managed, skipped := managedRootStubPaths(info.GitRoot)
	plan.ManagedStubPaths = managed
	plan.SkippedStubPaths = skipped
	plan.ConfigCleanup = configNeedsRepoCleanup(cfg, info, configExisted)
	return plan
}

func (p repoDeletePlan) HasTargets() bool {
	return p.RepoDataDir != "" ||
		len(p.SupportPaths) > 0 ||
		len(p.ManagedStubPaths) > 0 ||
		p.ConfigCleanup
}

func describeRepoDeletePlan(out io.Writer, plan repoDeletePlan) {
	fmt.Fprintf(out, "This will remove Mem setup for repo: %s\n", plan.GitRoot)
	if plan.RepoDataDir != "" {
		fmt.Fprintf(out, "  repo data: %s\n", plan.RepoDataDir)
	}
	for _, path := range plan.SupportPaths {
		fmt.Fprintf(out, "  repo support path: %s\n", path)
	}
	for _, path := range plan.ManagedStubPaths {
		fmt.Fprintf(out, "  managed stub: %s\n", path)
	}
	for _, path := range plan.SkippedStubPaths {
		fmt.Fprintf(out, "  keep unchanged (not Mem-managed): %s\n", path)
	}
	if plan.ConfigCleanup {
		fmt.Fprintln(out, "  config cleanup: active_repo/repo_cache entries for this repo")
	}
}

func executeRepoDeletePlan(plan repoDeletePlan) error {
	for _, path := range plan.ManagedStubPaths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for _, path := range plan.SupportPaths {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	if plan.RepoDataDir != "" {
		if err := os.RemoveAll(plan.RepoDataDir); err != nil {
			return err
		}
	}
	return nil
}

func clearRepoStateForDelete(cfg *config.Config, info repo.Info, configExisted bool) error {
	if cfg == nil {
		return nil
	}
	if !configNeedsRepoCleanup(*cfg, info, configExisted) {
		return nil
	}
	if cfg.RepoCache == nil {
		cfg.RepoCache = map[string]string{}
	}
	targetRoot := pathutil.Canonical(info.GitRoot)
	for root, repoID := range cfg.RepoCache {
		if repoID == info.ID || pathutil.Canonical(root) == targetRoot {
			cfg.RepoCache[root] = ""
		}
	}
	if strings.TrimSpace(cfg.ActiveRepo) == info.ID {
		cfg.ActiveRepo = ""
	}
	return cfg.SaveRepoState()
}

func configNeedsRepoCleanup(cfg config.Config, info repo.Info, configExisted bool) bool {
	if !configExisted {
		return false
	}
	if strings.TrimSpace(cfg.ActiveRepo) == info.ID {
		return true
	}
	targetRoot := pathutil.Canonical(info.GitRoot)
	for root, repoID := range cfg.RepoCache {
		if repoID == info.ID || pathutil.Canonical(root) == targetRoot {
			return true
		}
	}
	return false
}

func managedRootStubPaths(root string) (managed []string, skipped []string) {
	type stubCheck struct {
		Path    string
		Managed func(string) bool
	}
	checks := []stubCheck{
		{Path: filepath.Join(root, "AGENTS.md"), Managed: isManagedAgentsStub},
		{Path: filepath.Join(root, "CLAUDE.md"), Managed: isManagedClaudeStub},
		{Path: filepath.Join(root, "GEMINI.md"), Managed: isManagedGeminiStub},
	}
	for _, check := range checks {
		data, err := os.ReadFile(check.Path)
		if err != nil {
			continue
		}
		if check.Managed(string(data)) {
			managed = append(managed, check.Path)
			continue
		}
		skipped = append(skipped, check.Path)
	}
	return managed, skipped
}

func pathExistsForDelete(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
