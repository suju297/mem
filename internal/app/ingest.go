package app

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mempack/internal/repo"
	"mempack/internal/store"
	"mempack/internal/token"

	ignore "github.com/sabhiram/go-gitignore"
)

type IngestResponse struct {
	FilesIngested int `json:"files_ingested"`
	ChunksAdded   int `json:"chunks_added"`
	FilesSkipped  int `json:"files_skipped"`
}

type ignoreMatcher struct {
	matchers []*ignore.GitIgnore
}

func (m ignoreMatcher) Matches(path string) bool {
	for _, matcher := range m.matchers {
		if matcher != nil && matcher.MatchesPath(path) {
			return true
		}
	}
	return false
}

func runIngest(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("ingest-artifact", flag.ContinueOnError)
	fs.SetOutput(errOut)
	threadID := fs.String("thread", "", "Thread id")
	repoOverride := fs.String("repo", "", "Override repo id")
	workspace := fs.String("workspace", "", "Workspace name")
	maxFileMB := fs.Int("max-file-mb", 2, "Max file size (MB)")
	chunkTokens := fs.Int("chunk-tokens", 320, "Chunk size (tokens)")
	overlapTokens := fs.Int("overlap-tokens", 40, "Chunk overlap (tokens)")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"thread":         {RequiresValue: true},
		"repo":           {RequiresValue: true},
		"workspace":      {RequiresValue: true},
		"max-file-mb":    {RequiresValue: true},
		"chunk-tokens":   {RequiresValue: true},
		"overlap-tokens": {RequiresValue: true},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	pathArg := strings.TrimSpace(strings.Join(positional, " "))
	if pathArg == "" {
		fmt.Fprintln(errOut, "missing path")
		return 2
	}
	if strings.TrimSpace(*threadID) == "" {
		fmt.Fprintln(errOut, "missing --thread")
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	repoInfo, err := resolveRepo(cfg, strings.TrimSpace(*repoOverride))
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

	counter, err := token.New(cfg.Tokenizer)
	if err != nil {
		fmt.Fprintf(errOut, "tokenizer error: %v\n", err)
		return 1
	}

	info, err := os.Stat(pathArg)
	if err != nil {
		fmt.Fprintf(errOut, "path error: %v\n", err)
		return 1
	}

	root := repoInfo.GitRoot
	if root == "" {
		if info.IsDir() {
			root = pathArg
		} else {
			root = filepath.Dir(pathArg)
		}
	}
	matcher := loadIgnoreMatcher(root)
	maxBytes := int64(*maxFileMB) * 1024 * 1024

	var resp IngestResponse
	processFile := func(path string) error {
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Size() > maxBytes {
			resp.FilesSkipped++
			return nil
		}

		relPath := path
		if root != "" {
			if rel, err := filepath.Rel(root, path); err == nil {
				relPath = rel
			}
		}
		relPath = filepath.ToSlash(relPath)
		if matcher.Matches(relPath) {
			resp.FilesSkipped++
			return nil
		}
		if !allowedExtension(path) {
			resp.FilesSkipped++
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		semanticChunks, err := chunkFile(path, data, *chunkTokens, *overlapTokens, counter)
		if err != nil {
			return err
		}
		if len(semanticChunks) == 0 {
			resp.FilesSkipped++
			return nil
		}

		hash := sha256.Sum256(data)
		artifact := store.Artifact{
			ID:          store.NewID("A"),
			RepoID:      repoInfo.ID,
			Workspace:   workspaceName,
			Kind:        "file",
			Source:      relPath,
			ContentHash: hex.EncodeToString(hash[:]),
			CreatedAt:   time.Now().UTC(),
		}

		chunks := make([]store.Chunk, 0, len(semanticChunks))
		for _, sc := range semanticChunks {
			chunkHash := sha256.Sum256([]byte(sc.Text))
			locator := formatLocator(repoInfo, relPath, sc.StartLine, sc.EndLine)
			chunks = append(chunks, store.Chunk{
				ID:         store.NewID("C"),
				RepoID:     repoInfo.ID,
				Workspace:  workspaceName,
				ArtifactID: artifact.ID,
				ThreadID:   strings.TrimSpace(*threadID),
				Locator:    locator,
				Text:       sc.Text,
				TextHash:   hex.EncodeToString(chunkHash[:]),
				TextTokens: counter.Count(sc.Text),
				ChunkType:  sc.ChunkType,
				SymbolName: sc.SymbolName,
				SymbolKind: sc.SymbolKind,
				TagsJSON:   "[]",
				TagsText:   "",
				CreatedAt:  time.Now().UTC(),
			})
		}

		inserted, _, err := st.AddArtifactWithChunks(artifact, chunks)
		if err != nil {
			return err
		}

		resp.FilesIngested++
		resp.ChunksAdded += inserted
		return nil
	}

	if info.IsDir() {
		err := filepath.WalkDir(pathArg, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				relPath := path
				if root != "" {
					if rel, err := filepath.Rel(root, path); err == nil {
						relPath = rel
					}
				}
				relPath = filepath.ToSlash(relPath)
				if d.Name() == ".git" || matcher.Matches(relPath) {
					return filepath.SkipDir
				}
				return nil
			}
			return processFile(path)
		})
		if err != nil {
			fmt.Fprintf(errOut, "ingest error: %v\n", err)
			return 1
		}
	} else {
		if err := processFile(pathArg); err != nil {
			fmt.Fprintf(errOut, "ingest error: %v\n", err)
			return 1
		}
	}

	return writeJSON(out, errOut, resp)
}

type chunkRange struct {
	Start int
	End   int
}

func chunkRanges(lines []string, lineTokens []int, maxTokens, overlapTokens int) []chunkRange {
	if maxTokens <= 0 {
		return nil
	}
	var ranges []chunkRange
	start := 0
	for start < len(lines) {
		tokens := 0
		end := start
		for end < len(lines) {
			lineTok := lineTokens[end]
			if tokens > 0 && tokens+lineTok > maxTokens {
				break
			}
			tokens += lineTok
			end++
			if tokens >= maxTokens {
				break
			}
		}
		if end == start {
			end = start + 1
		}
		ranges = append(ranges, chunkRange{Start: start, End: end})
		if end >= len(lines) {
			break
		}

		overlap := 0
		nextStart := end
		for nextStart > start && overlap < overlapTokens {
			nextStart--
			overlap += lineTokens[nextStart]
		}
		if nextStart == start {
			nextStart = end
		}
		start = nextStart
	}
	return ranges
}

func loadIgnoreMatcher(root string) ignoreMatcher {
	matchers := []*ignore.GitIgnore{}
	matchers = append(matchers, ignore.CompileIgnoreLines(defaultIgnoreLines()...))

	gitignorePath := filepath.Join(root, ".gitignore")
	if matcher, err := ignore.CompileIgnoreFile(gitignorePath); err == nil {
		matchers = append(matchers, matcher)
	}
	mempackIgnorePath := filepath.Join(root, ".mempackignore")
	if matcher, err := ignore.CompileIgnoreFile(mempackIgnorePath); err == nil {
		matchers = append(matchers, matcher)
	}
	return ignoreMatcher{matchers: matchers}
}

func defaultIgnoreLines() []string {
	return []string{
		".git/",
		"node_modules/",
		"venv/",
		".venv/",
		"dist/",
		"build/",
		"out/",
		"vendor/",
		"target/",
		".gradle/",
		"__pycache__/",
		"*.png",
		"*.jpg",
		"*.jpeg",
		"*.gif",
		"*.pdf",
		"*.zip",
		"*.jar",
		"*.class",
		"*.so",
		"*.dylib",
		"*.exe",
		".DS_Store",
	}
}

func allowedExtension(path string) bool {
	lower := strings.ToLower(filepath.Ext(path))
	switch lower {
	case ".md", ".txt", ".rst", ".log", ".json", ".yaml", ".yml", ".toml":
		return true
	case ".py", ".go", ".js", ".ts", ".tsx", ".java", ".kt", ".rs", ".c", ".cpp", ".h", ".cs", ".sql", ".sh":
		return true
	default:
		return false
	}
}

func formatLocator(info repo.Info, relPath string, startLine, endLine int) string {
	if info.HasGit && info.Head != "" {
		return fmt.Sprintf("git:%s:%s#L%d-L%d", info.Head, relPath, startLine, endLine)
	}
	return fmt.Sprintf("file:%s#L%d-L%d", relPath, startLine, endLine)
}
