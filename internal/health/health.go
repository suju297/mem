package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mempack/internal/config"
	"mempack/internal/pathutil"
	"mempack/internal/repo"
	"mempack/internal/reporesolve"
	"mempack/internal/store"
	"mempack/internal/token"
)

type Options struct {
	RepoOverride string
	Cwd          string
	Repair       bool
	RequireRepo  bool
}

type Report struct {
	OK         bool         `json:"ok"`
	Repo       RepoReport   `json:"repo"`
	DB         DBReport     `json:"db"`
	Schema     SchemaReport `json:"schema"`
	FTS        FTSReport    `json:"fts"`
	State      StateReport  `json:"state"`
	ActiveRepo string       `json:"active_repo,omitempty"`
	Error      string       `json:"error,omitempty"`
	Suggestion string       `json:"suggestion,omitempty"`
}

type RepoReport struct {
	ID      string `json:"id"`
	GitRoot string `json:"git_root"`
	Source  string `json:"source"`
	HasGit  bool   `json:"has_git"`
}

type DBReport struct {
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"size_bytes"`
}

type SchemaReport struct {
	UserVersion     int    `json:"user_version"`
	CurrentVersion  int    `json:"current_version"`
	LastMigrationAt string `json:"last_migration_at,omitempty"`
}

type FTSReport struct {
	Memories bool `json:"memories"`
	Chunks   bool `json:"chunks"`
	Rebuilt  bool `json:"rebuilt,omitempty"`
}

type StateReport struct {
	Valid             bool     `json:"valid"`
	InvalidWorkspaces []string `json:"invalid_workspaces,omitempty"`
	Repaired          bool     `json:"repaired,omitempty"`
}

type CheckError struct {
	Message    string
	Suggestion string
	Err        error
}

func (e *CheckError) Error() string {
	if e.Suggestion == "" {
		return e.Message
	}
	return fmt.Sprintf("%s. %s", e.Message, e.Suggestion)
}

func Check(ctx context.Context, repoRef string, opts Options) (Report, error) {
	return check(ctx, repoRef, opts)
}

func Repair(ctx context.Context, repoRef string, opts Options) (Report, error) {
	opts.Repair = true
	return check(ctx, repoRef, opts)
}

func check(ctx context.Context, repoRef string, opts Options) (Report, error) {
	_ = ctx
	report := Report{}

	cfg, err := config.Load()
	if err != nil {
		return reportError(report, "config error", "Check config.toml", err)
	}
	report.ActiveRepo = cfg.ActiveRepo

	cwd := opts.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return reportError(report, "failed to get cwd", "", err)
		}
	}

	info, source, err := resolveRepo(cfg, repoRef, cwd, opts.RequireRepo)
	if err != nil {
		if ce, ok := err.(*CheckError); ok {
			return reportError(report, ce.Message, ce.Suggestion, ce.Err)
		}
		return reportError(report, "repo resolution error", "Run: mem init", err)
	}

	report.Repo = RepoReport{
		ID:      info.ID,
		GitRoot: info.GitRoot,
		Source:  source,
		HasGit:  info.HasGit,
	}

	dbPath := cfg.RepoDBPath(info.ID)
	report.DB.Path = dbPath

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		msg, hint := mapDBError(err)
		return reportError(report, msg, hint, err)
	}

	fi, err := os.Stat(dbPath)
	if err != nil {
		if os.IsNotExist(err) {
			report.DB.Exists = false
			if !opts.Repair {
				return reportError(report, "DB not initialized for this repo", "Run: mem init", err)
			}
		} else {
			msg, hint := mapDBError(err)
			return reportError(report, msg, hint, err)
		}
	} else {
		report.DB.Exists = true
		report.DB.SizeBytes = fi.Size()
	}

	st, err := store.Open(dbPath)
	if err != nil {
		msg, hint := mapDBError(err)
		return reportError(report, msg, hint, err)
	}
	defer st.Close()

	if fi, err := os.Stat(dbPath); err == nil {
		report.DB.Exists = true
		report.DB.SizeBytes = fi.Size()
	}

	userVersion, err := st.UserVersion()
	if err != nil {
		return reportError(report, "schema check failed", "Try: mem doctor --verbose", err)
	}
	report.Schema.UserVersion = userVersion
	report.Schema.CurrentVersion = store.SchemaVersion()

	if lastMigration, err := st.GetMeta("last_migration_at"); err == nil {
		report.Schema.LastMigrationAt = formatTimeRFC3339(lastMigration)
	}

	invalidRows, err := invalidStateCurrent(st, info.ID)
	if err != nil {
		return reportError(report, "state check failed", "Try: mem doctor --verbose", err)
	}
	invalidWorkspaces := workspacesFromRows(invalidRows)
	if len(invalidWorkspaces) > 0 {
		report.State.Valid = false
		report.State.InvalidWorkspaces = invalidWorkspaces
		if opts.Repair {
			repaired, err := repairInvalidStateCurrent(st, info.ID, invalidRows, cfg.Tokenizer)
			if err != nil {
				return reportError(report, "state repair failed", "Try: mem doctor --repair", err)
			}
			report.State.Repaired = repaired
			report.State.Valid = true
		} else {
			msg := fmt.Sprintf("invalid workspace state JSON (workspace=%s)", strings.Join(invalidWorkspaces, ","))
			return reportError(report, msg, "Run: mem doctor --repair", errors.New("invalid state JSON"))
		}
	} else {
		report.State.Valid = true
	}

	memFTS, chunkFTS, err := st.HasFTSTables()
	if err != nil {
		return reportError(report, "FTS check failed", "Try: mem doctor --verbose", err)
	}
	if !memFTS || !chunkFTS {
		if err := st.RebuildFTS(); err != nil {
			return reportError(report, "FTS index missing", "Run: mem doctor --repair", err)
		}
		report.FTS.Rebuilt = true
		memFTS, chunkFTS, err = st.HasFTSTables()
		if err != nil {
			return reportError(report, "FTS check failed", "Try: mem doctor --verbose", err)
		}
	}

	report.FTS.Memories = memFTS
	report.FTS.Chunks = chunkFTS
	if !memFTS || !chunkFTS {
		return reportError(report, "FTS index missing", "Run: mem doctor --repair", errors.New("fts missing"))
	}

	report.OK = true
	return report, nil
}

func invalidStateCurrent(st *store.Store, repoID string) ([]store.StateCurrentRow, error) {
	rows, err := st.ListStateCurrent(repoID)
	if err != nil {
		return nil, err
	}
	invalid := make([]store.StateCurrentRow, 0)
	for _, row := range rows {
		if !json.Valid([]byte(row.StateJSON)) {
			invalid = append(invalid, row)
		}
	}
	return invalid, nil
}

func workspacesFromRows(rows []store.StateCurrentRow) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Workspace)
	}
	return out
}

func repairInvalidStateCurrent(st *store.Store, repoID string, rows []store.StateCurrentRow, tokenizer string) (bool, error) {
	stateJSON := "{}"
	stateTokens := 0
	if counter, err := token.New(tokenizer); err == nil {
		stateTokens = counter.Count(stateJSON)
	}
	reason := "repair: invalid state_current JSON"
	now := time.Now().UTC()
	repaired := false

	for _, row := range rows {
		if strings.TrimSpace(row.StateJSON) == stateJSON {
			continue
		}
		skipHistory := false
		if last, err := st.GetLatestStateHistory(repoID, row.Workspace); err == nil {
			if strings.TrimSpace(last.StateJSON) == stateJSON && last.Reason == reason {
				skipHistory = true
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return repaired, err
		}
		if !skipHistory {
			stateID := store.NewID("S")
			if err := st.AddStateHistory(stateID, repoID, row.Workspace, stateJSON, reason, stateTokens, now); err != nil {
				return repaired, err
			}
		}
		if err := st.SetStateCurrent(repoID, row.Workspace, stateJSON, stateTokens, now); err != nil {
			return repaired, err
		}
		repaired = true
	}
	return repaired, nil
}

func resolveRepo(cfg config.Config, repoRef, cwd string, requireRepo bool) (repo.Info, string, error) {
	if repoRef != "" {
		if info, err := detectRepoPathStrict(repoRef); err == nil {
			return info, "path", nil
		} else if reporesolve.LooksLikePath(repoRef) {
			if info, err := repoFromRoot(cfg, repoRef); err == nil {
				return info, "db_root", nil
			}
		} else if pathExists(repoRef) {
			if isGitNotFound(err) {
				return repo.Info{}, "", &CheckError{
					Message:    "git not found",
					Suggestion: "Install git or pass --repo <id|path>",
					Err:        err,
				}
			}
			return repo.Info{}, "", &CheckError{
				Message:    fmt.Sprintf("repo detection failed for %s", repoRef),
				Suggestion: "Run: mem use <path> or pass --repo <id>",
				Err:        err,
			}
		}

		info, err := repoFromID(cfg, repoRef)
		if err != nil {
			return repo.Info{}, "", &CheckError{
				Message:    fmt.Sprintf("repo not found: %s", repoRef),
				Suggestion: "Run: mem repos or mem init",
				Err:        err,
			}
		}
		return info, "repo_id", nil
	}

	info, err := detectRepoCwdStrict(cwd)
	if err == nil {
		return info, "cwd", nil
	}
	if isGitNotFound(err) {
		return repo.Info{}, "", &CheckError{
			Message:    "git not found",
			Suggestion: "Install git or pass --repo <id|path>",
			Err:        err,
		}
	}
	if requireRepo {
		return repo.Info{}, "", &CheckError{
			Message:    "repo not specified and could not detect repo from current directory",
			Suggestion: "Pass --repo <id|path> or start mem mcp --repo /path/to/repo",
			Err:        err,
		}
	}

	if cfg.ActiveRepo != "" {
		meta, metaErr := repoMetaFromID(cfg, cfg.ActiveRepo)
		if metaErr == nil && meta.GitRoot != "" && pathExists(meta.GitRoot) {
			info, infoErr := repo.InfoFromCache(meta.RepoID, meta.GitRoot, meta.LastHead, meta.LastBranch, true)
			if infoErr == nil {
				return info, "active_repo", nil
			}
		}
	}

	return repo.Info{}, "", &CheckError{
		Message:    "no active repo",
		Suggestion: "Run: mem init (in your repo) or: mem use <path>",
		Err:        err,
	}
}

func repoFromRoot(cfg config.Config, repoPath string) (repo.Info, error) {
	cleanPath := pathutil.Canonical(repoPath)
	if cleanPath == "" {
		return repo.Info{}, fmt.Errorf("repo path is empty")
	}

	if _, repoID := cachedRepoForPath(cfg, cleanPath); repoID != "" {
		return repoFromID(cfg, repoID)
	}
	repoID, err := reporesolve.RepoIDFromRoot(cfg.RepoRootDir(), cleanPath)
	if err != nil {
		return repo.Info{}, err
	}
	return repoFromID(cfg, repoID)
}

func cachedRepoForPath(cfg config.Config, path string) (string, string) {
	if len(cfg.RepoCache) == 0 {
		return "", ""
	}
	cleanPath := pathutil.Canonical(path)
	bestRoot := ""
	bestID := ""
	sep := string(os.PathSeparator)
	for root, repoID := range cfg.RepoCache {
		if repoID == "" {
			continue
		}
		cleanRoot := pathutil.Canonical(root)
		if cleanRoot == "." || cleanRoot == "" {
			continue
		}
		if cleanPath == cleanRoot || strings.HasPrefix(cleanPath, cleanRoot+sep) {
			if len(cleanRoot) > len(bestRoot) {
				bestRoot = cleanRoot
				bestID = repoID
			}
		}
	}
	return bestRoot, bestID
}

func detectRepoCwdStrict(cwd string) (repo.Info, error) {
	info, err := repo.DetectBaseStrict(cwd)
	if err != nil {
		return repo.Info{}, err
	}
	return repo.PopulateOriginAndID(info)
}

func detectRepoPathStrict(path string) (repo.Info, error) {
	if _, err := os.Stat(path); err != nil {
		return repo.Info{}, err
	}
	info, err := repo.DetectBaseStrict(path)
	if err != nil {
		return repo.Info{}, err
	}
	return repo.PopulateOriginAndID(info)
}

func repoMetaFromID(cfg config.Config, repoID string) (store.RepoRow, error) {
	dbPath := cfg.RepoDBPath(repoID)
	if _, err := os.Stat(dbPath); err != nil {
		return store.RepoRow{}, err
	}
	st, err := store.Open(dbPath)
	if err != nil {
		return store.RepoRow{}, err
	}
	defer st.Close()
	return st.GetRepo(repoID)
}

func repoFromID(cfg config.Config, repoID string) (repo.Info, error) {
	meta, err := repoMetaFromID(cfg, repoID)
	if err != nil {
		return repo.Info{}, err
	}
	return repo.InfoFromCache(meta.RepoID, meta.GitRoot, meta.LastHead, meta.LastBranch, true)
}

func mapDBError(err error) (string, string) {
	if isDBLocked(err) {
		return "database is locked", "Close other mempack process and retry (busy_timeout=3000ms)"
	}
	if isReadOnly(err) {
		return "cannot create DB under XDG path", "Check permissions or set XDG_DATA_HOME"
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "schema migration failed") {
		reason := strings.TrimSpace(strings.TrimPrefix(err.Error(), "schema migration failed:"))
		if reason == "" {
			reason = err.Error()
		}
		return fmt.Sprintf("schema migration failed: %s", reason), "Try: mem doctor --verbose"
	}
	return "DB open error", "Run: mem doctor --verbose"
}

func reportError(report Report, message, suggestion string, err error) (Report, error) {
	report.OK = false
	report.Error = message
	report.Suggestion = suggestion
	return report, &CheckError{Message: message, Suggestion: suggestion, Err: err}
}

func isGitNotFound(err error) bool {
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "executable file not found") || strings.Contains(msg, "git: not found")
}

func isDBLocked(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "database is busy")
}

func isReadOnly(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "read-only") || strings.Contains(msg, "readonly") || strings.Contains(msg, "permission denied")
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func formatTimeRFC3339(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts.Format(time.RFC3339Nano)
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts.Format(time.RFC3339)
	}
	return value
}
