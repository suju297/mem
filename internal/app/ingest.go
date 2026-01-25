package app

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"mempack/internal/repo"
	"mempack/internal/store"
	"mempack/internal/token"
	"mempack/internal/watcher"

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
	watch := fs.Bool("watch", false, "Watch for file changes and auto-ingest")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"thread":         {RequiresValue: true},
		"repo":           {RequiresValue: true},
		"workspace":      {RequiresValue: true},
		"max-file-mb":    {RequiresValue: true},
		"chunk-tokens":   {RequiresValue: true},
		"overlap-tokens": {RequiresValue: true},
		"watch":          {RequiresValue: false},
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
	if absRoot, err := filepath.Abs(root); err == nil {
		root = absRoot
	}
	if absPath, err := filepath.Abs(pathArg); err == nil {
		pathArg = absPath
	}

	matcher := loadIgnoreMatcher(root)
	maxBytes := int64(*maxFileMB) * 1024 * 1024

	ingestParams := ingestPathParams{
		path:          pathArg,
		root:          root,
		matcher:       matcher,
		repoInfo:      repoInfo,
		workspace:     workspaceName,
		threadID:      strings.TrimSpace(*threadID),
		maxBytes:      maxBytes,
		chunkTokens:   *chunkTokens,
		overlapTokens: *overlapTokens,
		st:            st,
		counter:       counter,
	}

	if *watch {
		resp, err := ingestPath(ingestParams)
		if err != nil {
			fmt.Fprintf(errOut, "ingest error: %v\n", err)
			return 1
		}
		if resp.FilesIngested > 0 || resp.ChunksAdded > 0 || resp.FilesSkipped > 0 {
			fmt.Fprintf(out, "Initial ingest: files=%d chunks=%d skipped=%d\n",
				resp.FilesIngested, resp.ChunksAdded, resp.FilesSkipped)
		}

		watchFile := ""
		if !info.IsDir() {
			watchFile = relPathFor(root, pathArg)
		}
		return runIngestWatch(runIngestWatchParams{
			root:          root,
			watchFile:     watchFile,
			repoInfo:      repoInfo,
			workspace:     workspaceName,
			threadID:      strings.TrimSpace(*threadID),
			maxBytes:      maxBytes,
			chunkTokens:   *chunkTokens,
			overlapTokens: *overlapTokens,
			st:            st,
			counter:       counter,
			matcher:       matcher,
			out:           out,
			errOut:        errOut,
		})
	}

	resp, err := ingestPath(ingestParams)
	if err != nil {
		fmt.Fprintf(errOut, "ingest error: %v\n", err)
		return 1
	}
	return writeJSON(out, errOut, resp)
}

type ingestPathParams struct {
	path           string
	root           string
	matcher        ignoreMatcher
	repoInfo       repo.Info
	workspace      string
	threadID       string
	maxBytes       int64
	chunkTokens    int
	overlapTokens  int
	st             *store.Store
	counter        *token.Counter
	deleteExisting bool
}

type ingestSingleFileParams struct {
	path           string
	relPath        string
	repoInfo       repo.Info
	workspace      string
	threadID       string
	maxBytes       int64
	chunkTokens    int
	overlapTokens  int
	st             *store.Store
	counter        *token.Counter
	deleteExisting bool
}

type runIngestWatchParams struct {
	root          string
	watchFile     string
	repoInfo      repo.Info
	workspace     string
	threadID      string
	maxBytes      int64
	chunkTokens   int
	overlapTokens int
	st            *store.Store
	counter       *token.Counter
	matcher       ignoreMatcher
	out           io.Writer
	errOut        io.Writer
}

func ingestPath(p ingestPathParams) (IngestResponse, error) {
	info, err := os.Stat(p.path)
	if err != nil {
		return IngestResponse{}, err
	}
	var resp IngestResponse

	processFile := func(path string) error {
		relPath := relPathFor(p.root, path)
		if p.matcher.Matches(relPath) {
			resp.FilesSkipped++
			return nil
		}
		if !allowedExtension(path) {
			resp.FilesSkipped++
			return nil
		}

		fileResp, err := ingestSingleFile(ingestSingleFileParams{
			path:           path,
			relPath:        relPath,
			repoInfo:       p.repoInfo,
			workspace:      p.workspace,
			threadID:       p.threadID,
			maxBytes:       p.maxBytes,
			chunkTokens:    p.chunkTokens,
			overlapTokens:  p.overlapTokens,
			st:             p.st,
			counter:        p.counter,
			deleteExisting: p.deleteExisting,
		})
		if err != nil {
			return err
		}
		resp.FilesIngested += fileResp.FilesIngested
		resp.ChunksAdded += fileResp.ChunksAdded
		resp.FilesSkipped += fileResp.FilesSkipped
		return nil
	}

	if info.IsDir() {
		if err := filepath.WalkDir(p.path, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				relPath := relPathFor(p.root, path)
				if d.Name() == ".git" || p.matcher.Matches(relPath) {
					return filepath.SkipDir
				}
				return nil
			}
			return processFile(path)
		}); err != nil {
			return resp, err
		}
	} else {
		if err := processFile(p.path); err != nil {
			return resp, err
		}
	}

	return resp, nil
}

func ingestSingleFile(p ingestSingleFileParams) (IngestResponse, error) {
	var resp IngestResponse

	info, err := os.Stat(p.path)
	if err != nil {
		return resp, err
	}
	if info.IsDir() {
		return resp, nil
	}
	if info.Size() > p.maxBytes {
		resp.FilesSkipped++
		return resp, nil
	}

	data, err := os.ReadFile(p.path)
	if err != nil {
		return resp, err
	}

	semanticChunks, err := chunkFile(p.path, data, p.chunkTokens, p.overlapTokens, p.counter)
	if err != nil {
		return resp, err
	}
	if len(semanticChunks) == 0 {
		resp.FilesSkipped++
		return resp, nil
	}

	if p.deleteExisting {
		if _, err := p.st.DeleteChunksBySource(p.repoInfo.ID, p.workspace, p.relPath); err != nil {
			return resp, err
		}
		if _, err := p.st.DeleteArtifactsBySource(p.repoInfo.ID, p.workspace, p.relPath); err != nil {
			return resp, err
		}
	}

	hash := sha256.Sum256(data)
	artifact := store.Artifact{
		ID:          store.NewID("A"),
		RepoID:      p.repoInfo.ID,
		Workspace:   p.workspace,
		Kind:        "file",
		Source:      p.relPath,
		ContentHash: hex.EncodeToString(hash[:]),
		CreatedAt:   time.Now().UTC(),
	}

	chunks := make([]store.Chunk, 0, len(semanticChunks))
	for _, sc := range semanticChunks {
		chunkHash := sha256.Sum256([]byte(sc.Text))
		locator := formatLocator(p.repoInfo, p.relPath, sc.StartLine, sc.EndLine)
		chunks = append(chunks, store.Chunk{
			ID:         store.NewID("C"),
			RepoID:     p.repoInfo.ID,
			Workspace:  p.workspace,
			ArtifactID: artifact.ID,
			ThreadID:   p.threadID,
			Locator:    locator,
			Text:       sc.Text,
			TextHash:   hex.EncodeToString(chunkHash[:]),
			TextTokens: p.counter.Count(sc.Text),
			ChunkType:  sc.ChunkType,
			SymbolName: sc.SymbolName,
			SymbolKind: sc.SymbolKind,
			TagsJSON:   "[]",
			TagsText:   "",
			CreatedAt:  time.Now().UTC(),
		})
	}

	inserted, _, err := p.st.AddArtifactWithChunks(artifact, chunks)
	if err != nil {
		return resp, err
	}

	resp.FilesIngested++
	resp.ChunksAdded += inserted
	return resp, nil
}

func runIngestWatch(p runIngestWatchParams) int {
	ignorer := func(relPath string) bool {
		if relPath == ".git" || strings.HasPrefix(relPath, ".git/") {
			return true
		}
		return p.matcher.Matches(relPath)
	}

	w, err := watcher.New(p.root, ignorer)
	if err != nil {
		fmt.Fprintf(p.errOut, "watcher error: %v\n", err)
		return 1
	}
	if err := w.Start(); err != nil {
		fmt.Fprintf(p.errOut, "watcher start error: %v\n", err)
		return 1
	}
	defer w.Stop()

	fmt.Fprintf(p.out, "Watching %s for changes (Ctrl+C to stop)...\n", p.root)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-sigCh:
			fmt.Fprintln(p.out, "\nStopping watcher...")
			return 0
		case event, ok := <-w.Events():
			if !ok {
				return 0
			}
			if p.watchFile != "" && event.RelPath != p.watchFile {
				continue
			}
			if p.matcher.Matches(event.RelPath) {
				continue
			}

			switch event.Op {
			case watcher.OpCreate, watcher.OpModify:
				if !allowedExtension(event.Path) {
					continue
				}
				resp, err := ingestSingleFile(ingestSingleFileParams{
					path:           event.Path,
					relPath:        event.RelPath,
					repoInfo:       p.repoInfo,
					workspace:      p.workspace,
					threadID:       p.threadID,
					maxBytes:       p.maxBytes,
					chunkTokens:    p.chunkTokens,
					overlapTokens:  p.overlapTokens,
					st:             p.st,
					counter:        p.counter,
					deleteExisting: true,
				})
				if err != nil {
					if os.IsNotExist(err) {
						continue
					}
					fmt.Fprintf(p.errOut, "[%s] %s: error: %v\n", event.Op, event.RelPath, err)
					continue
				}
				if resp.ChunksAdded > 0 || resp.FilesIngested > 0 {
					fmt.Fprintf(p.out, "[%s] %s: %d chunks\n", event.Op, event.RelPath, resp.ChunksAdded)
				}
			case watcher.OpDelete:
				deleted, err := p.st.DeleteChunksBySource(p.repoInfo.ID, p.workspace, event.RelPath)
				if err != nil {
					fmt.Fprintf(p.errOut, "[delete] %s: error: %v\n", event.RelPath, err)
					continue
				}
				if deleted > 0 {
					fmt.Fprintf(p.out, "[delete] %s: removed %d chunks\n", event.RelPath, deleted)
				}
				_, _ = p.st.DeleteArtifactsBySource(p.repoInfo.ID, p.workspace, event.RelPath)
			}
		}
	}
}

func relPathFor(root, path string) string {
	if root == "" {
		return filepath.ToSlash(path)
	}
	rootPath := root
	target := path
	if filepath.IsAbs(rootPath) && !filepath.IsAbs(target) {
		if abs, err := filepath.Abs(target); err == nil {
			target = abs
		}
	}
	if !filepath.IsAbs(rootPath) && filepath.IsAbs(target) {
		if abs, err := filepath.Abs(rootPath); err == nil {
			rootPath = abs
		}
	}
	if rel, err := filepath.Rel(rootPath, target); err == nil {
		if rel != "." && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
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
