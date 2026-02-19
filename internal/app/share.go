package app

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"mempack/internal/store"
	"mempack/internal/token"
)

const (
	shareDefaultDirName       = "mempack-share"
	shareSchemaVersion        = 1
	shareImportTag            = "shared_import"
	shareManifestFileName     = "manifest.json"
	shareMemoriesFileName     = "memories.jsonl"
	shareInstructionsFileName = "README.md"
)

type shareManifest struct {
	SchemaVersion int    `json:"schema_version"`
	ExportedAt    string `json:"exported_at"`
	ToolVersion   string `json:"tool_version"`
	SourceRepoID  string `json:"source_repo_id"`
	SourceGitRoot string `json:"source_git_root"`
	Workspace     string `json:"workspace"`
	MemoryCount   int    `json:"memory_count"`
}

type shareMemoryRecord struct {
	SourceID     string   `json:"source_id"`
	ThreadID     string   `json:"thread_id,omitempty"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	Tags         []string `json:"tags,omitempty"`
	Entities     []string `json:"entities,omitempty"`
	AnchorCommit string   `json:"anchor_commit,omitempty"`
	CreatedAt    string   `json:"created_at"`
}

type shareExportResponse struct {
	Mode      string `json:"mode"`
	RepoID    string `json:"repo_id"`
	Workspace string `json:"workspace"`
	BundleDir string `json:"bundle_dir"`
	Manifest  string `json:"manifest"`
	Memories  string `json:"memories"`
	Count     int    `json:"count"`
}

type shareImportResponse struct {
	Mode              string `json:"mode"`
	RepoID            string `json:"repo_id"`
	Workspace         string `json:"workspace"`
	BundleDir         string `json:"bundle_dir"`
	SourceRepoID      string `json:"source_repo_id"`
	AllowRepoMismatch bool   `json:"allow_repo_mismatch"`
	Replace           bool   `json:"replace"`
	Imported          int    `json:"imported"`
	Updated           int    `json:"updated"`
	Unchanged         int    `json:"unchanged"`
	Deleted           int    `json:"deleted"`
}

func runShare(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: mem share <export|import> [options]")
		return 2
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "export":
		return runShareExport(args[1:], out, errOut)
	case "import":
		return runShareImport(args[1:], out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown share command: %s\n", args[0])
		return 2
	}
}

func runShareExport(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("share export", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id/path")
	workspace := fs.String("workspace", "", "Workspace name")
	outDir := fs.String("out", "", "Output directory (default: mempack-share in repo root)")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"repo":      {RequiresValue: true},
		"workspace": {RequiresValue: true},
		"out":       {RequiresValue: true},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(positional, " "))
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))
	repoInfo, err := resolveRepo(&cfg, strings.TrimSpace(*repoOverride))
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

	memories, err := st.ListActiveMemories(repoInfo.ID, workspaceName)
	if err != nil {
		fmt.Fprintf(errOut, "list memories error: %v\n", err)
		return 1
	}

	records := make([]shareMemoryRecord, 0, len(memories))
	for _, mem := range memories {
		records = append(records, shareMemoryRecord{
			SourceID:     mem.ID,
			ThreadID:     mem.ThreadID,
			Title:        mem.Title,
			Summary:      mem.Summary,
			Tags:         parseJSONList(mem.TagsJSON),
			Entities:     parseJSONList(mem.EntitiesJSON),
			AnchorCommit: mem.AnchorCommit,
			CreatedAt:    mem.CreatedAt.UTC().Format(time.RFC3339Nano),
		})
	}

	bundleDir := resolveShareDir(repoInfo.GitRoot, strings.TrimSpace(*outDir))
	if err := os.RemoveAll(bundleDir); err != nil {
		fmt.Fprintf(errOut, "cleanup bundle dir error: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		fmt.Fprintf(errOut, "create bundle dir error: %v\n", err)
		return 1
	}

	manifest := shareManifest{
		SchemaVersion: shareSchemaVersion,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		ToolVersion:   VersionString(),
		SourceRepoID:  repoInfo.ID,
		SourceGitRoot: repoInfo.GitRoot,
		Workspace:     workspaceName,
		MemoryCount:   len(records),
	}

	manifestPath := filepath.Join(bundleDir, shareManifestFileName)
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		fmt.Fprintf(errOut, "write manifest error: %v\n", err)
		return 1
	}

	memoriesPath := filepath.Join(bundleDir, shareMemoriesFileName)
	if err := writeJSONLines(memoriesPath, records); err != nil {
		fmt.Fprintf(errOut, "write memories error: %v\n", err)
		return 1
	}

	readmePath := filepath.Join(bundleDir, shareInstructionsFileName)
	if err := os.WriteFile(readmePath, []byte(shareBundleReadme()), 0o644); err != nil {
		fmt.Fprintf(errOut, "write readme error: %v\n", err)
		return 1
	}

	resp := shareExportResponse{
		Mode:      "export",
		RepoID:    repoInfo.ID,
		Workspace: workspaceName,
		BundleDir: bundleDir,
		Manifest:  manifestPath,
		Memories:  memoriesPath,
		Count:     len(records),
	}
	return writeJSON(out, errOut, resp)
}

func runShareImport(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("share import", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id/path")
	workspace := fs.String("workspace", "", "Workspace name")
	inDir := fs.String("in", "", "Input directory (default: mempack-share in repo root)")
	replace := fs.Bool("replace", false, "Replace previously imported shared memories from the same source")
	allowRepoMismatch := fs.Bool("allow-repo-mismatch", false, "Allow importing when source_repo_id differs from current repo")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"repo":                {RequiresValue: true},
		"workspace":           {RequiresValue: true},
		"in":                  {RequiresValue: true},
		"replace":             {},
		"allow-repo-mismatch": {},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(positional, " "))
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))
	repoInfo, err := resolveRepo(&cfg, strings.TrimSpace(*repoOverride))
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}

	bundleDir := resolveShareDir(repoInfo.GitRoot, strings.TrimSpace(*inDir))
	manifestPath := filepath.Join(bundleDir, shareManifestFileName)
	memoriesPath := filepath.Join(bundleDir, shareMemoriesFileName)

	manifest, err := readShareManifest(manifestPath)
	if err != nil {
		fmt.Fprintf(errOut, "manifest error: %v\n", err)
		return 1
	}
	if manifest.SchemaVersion != shareSchemaVersion {
		fmt.Fprintf(errOut, "unsupported manifest schema: %d\n", manifest.SchemaVersion)
		return 1
	}
	if strings.TrimSpace(manifest.SourceRepoID) == "" {
		fmt.Fprintln(errOut, "manifest missing source_repo_id")
		return 1
	}
	if manifest.SourceRepoID != repoInfo.ID && !*allowRepoMismatch {
		fmt.Fprintf(errOut, "repo mismatch: bundle source_repo_id=%s current_repo_id=%s (use --allow-repo-mismatch to continue)\n", manifest.SourceRepoID, repoInfo.ID)
		return 1
	}

	records, err := readShareMemoryRecords(memoriesPath)
	if err != nil {
		fmt.Fprintf(errOut, "read memories error: %v\n", err)
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

	counter, err := token.New(cfg.Tokenizer)
	if err != nil {
		fmt.Fprintf(errOut, "tokenizer error: %v\n", err)
		return 1
	}

	sourcePrefix := shareLocalMemoryPrefix(manifest.SourceRepoID)
	sourceTag := shareSourceTag(manifest.SourceRepoID)
	incomingIDs := make(map[string]struct{}, len(records))
	for _, record := range records {
		incomingIDs[shareLocalMemoryID(manifest.SourceRepoID, record.SourceID)] = struct{}{}
	}

	deletedCount := 0
	if *replace {
		existingIDs, err := st.ListActiveMemoryIDsByPrefix(repoInfo.ID, workspaceName, sourcePrefix)
		if err != nil {
			fmt.Fprintf(errOut, "list shared memory ids error: %v\n", err)
			return 1
		}
		for _, id := range existingIDs {
			if _, keep := incomingIDs[id]; keep {
				continue
			}
			ok, err := st.PurgeMemory(repoInfo.ID, workspaceName, id)
			if err != nil {
				fmt.Fprintf(errOut, "purge shared memory error: %v\n", err)
				return 1
			}
			if ok {
				deletedCount++
			}
		}
	}

	importedCount := 0
	updatedCount := 0
	unchangedCount := 0
	for _, record := range records {
		localID := shareLocalMemoryID(manifest.SourceRepoID, record.SourceID)
		title := strings.TrimSpace(record.Title)
		if title == "" {
			title = fmt.Sprintf("Imported memory %s", strings.TrimSpace(record.SourceID))
		}
		summary := strings.TrimSpace(record.Summary)
		summaryTokens := counter.Count(summary)

		threadUsed, _, err := resolveThread(cfg, strings.TrimSpace(record.ThreadID))
		if err != nil {
			fmt.Fprintf(errOut, "thread error for source_id=%s: %v\n", record.SourceID, err)
			return 1
		}

		tags := mergeShareTags(record.Tags, sourceTag)
		entities := store.NormalizeEntities(record.Entities)
		createdAt := parseShareCreatedAt(record.CreatedAt)

		existing, err := st.GetMemory(repoInfo.ID, workspaceName, localID)
		if err == nil {
			if !existing.DeletedAt.IsZero() {
				if _, purgeErr := st.PurgeMemory(repoInfo.ID, workspaceName, localID); purgeErr != nil {
					fmt.Fprintf(errOut, "purge deleted memory error: %v\n", purgeErr)
					return 1
				}
			} else {
				titleCopy := title
				summaryCopy := summary
				updateInput := store.UpdateMemoryInput{
					RepoID:        repoInfo.ID,
					Workspace:     workspaceName,
					ID:            localID,
					Title:         &titleCopy,
					Summary:       &summaryCopy,
					SummaryTokens: &summaryTokens,
					TagsSet:       true,
					Tags:          tags,
					EntitiesSet:   true,
					Entities:      entities,
				}
				mem, changed, updateErr := st.UpdateMemoryWithStatus(updateInput)
				if updateErr != nil {
					fmt.Fprintf(errOut, "update shared memory error: %v\n", updateErr)
					return 1
				}
				if changed {
					if embedErr := maybeEmbedMemory(cfg, st, mem); embedErr != nil {
						fmt.Fprintf(errOut, "embedding warning: %v\n", embedErr)
					}
					updatedCount++
				} else {
					unchangedCount++
				}
				continue
			}
		} else if !errors.Is(err, store.ErrNotFound) {
			fmt.Fprintf(errOut, "read shared memory error: %v\n", err)
			return 1
		}

		mem, err := st.AddMemory(store.AddMemoryInput{
			ID:            localID,
			RepoID:        repoInfo.ID,
			Workspace:     workspaceName,
			ThreadID:      threadUsed,
			Title:         title,
			Summary:       summary,
			SummaryTokens: summaryTokens,
			TagsJSON:      store.TagsToJSON(tags),
			TagsText:      store.TagsText(tags),
			EntitiesJSON:  store.EntitiesToJSON(entities),
			EntitiesText:  store.EntitiesText(entities),
			AnchorCommit:  strings.TrimSpace(record.AnchorCommit),
			CreatedAt:     createdAt,
		})
		if err != nil {
			fmt.Fprintf(errOut, "import shared memory error: %v\n", err)
			return 1
		}
		if embedErr := maybeEmbedMemory(cfg, st, mem); embedErr != nil {
			fmt.Fprintf(errOut, "embedding warning: %v\n", embedErr)
		}
		importedCount++
	}

	resp := shareImportResponse{
		Mode:              "import",
		RepoID:            repoInfo.ID,
		Workspace:         workspaceName,
		BundleDir:         bundleDir,
		SourceRepoID:      manifest.SourceRepoID,
		AllowRepoMismatch: *allowRepoMismatch,
		Replace:           *replace,
		Imported:          importedCount,
		Updated:           updatedCount,
		Unchanged:         unchangedCount,
		Deleted:           deletedCount,
	}
	return writeJSON(out, errOut, resp)
}

func resolveShareDir(repoRoot, raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return filepath.Join(repoRoot, shareDefaultDirName)
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Join(repoRoot, value)
}

func writeJSONFile(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(path, encoded, 0o644)
}

func writeJSONLines(path string, records []shareMemoryRecord) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := bufio.NewWriter(f)
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if _, err := writer.Write(encoded); err != nil {
			return err
		}
		if err := writer.WriteByte('\n'); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func readShareManifest(path string) (shareManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return shareManifest{}, err
	}
	var manifest shareManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return shareManifest{}, err
	}
	return manifest, nil
}

func readShareMemoryRecords(path string) ([]shareMemoryRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 10*1024*1024)
	var records []shareMemoryRecord
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record shareMemoryRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return nil, err
		}
		if strings.TrimSpace(record.SourceID) == "" {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func parseJSONList(raw string) []string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil
	}
	return out
}

func mergeShareTags(tags []string, sourceTag string) []string {
	merged := append([]string{}, tags...)
	merged = append(merged, shareImportTag, sourceTag)
	return store.NormalizeTags(merged)
}

func parseShareCreatedAt(raw string) time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Now().UTC()
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC()
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.UTC()
	}
	return time.Now().UTC()
}

func shareSourceTag(sourceRepoID string) string {
	return "shared_source_" + shortHash(sourceRepoID)
}

func shareLocalMemoryPrefix(sourceRepoID string) string {
	return "MSH-" + shortHash(sourceRepoID) + "-"
}

func shareLocalMemoryID(sourceRepoID, sourceMemoryID string) string {
	return shareLocalMemoryPrefix(sourceRepoID) + sanitizeIDPart(sourceMemoryID)
}

func sanitizeIDPart(value string) string {
	if strings.TrimSpace(value) == "" {
		return shortHash(time.Now().UTC().Format(time.RFC3339Nano))
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return shortHash(value)
	}
	return out
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return hex.EncodeToString(sum[:])[:8]
}

func shareBundleReadme() string {
	return `# mempack-share

This folder is generated by Mempack.

## Files
- manifest.json: metadata and source repo identity.
- memories.jsonl: exported memory records (one JSON object per line).

## Import on teammate machine
Run from the target repo root:

  mem share import

If the bundle came from another repo id (fork/rename/migration), use:

  mem share import --allow-repo-mismatch
`
}
