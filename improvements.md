# Mempack Improvements: Detailed Implementation Plan

## Overview

This document provides concrete implementation plans for improving Mempack's integration with coding assistants. Each section includes schema changes, Go code, and migration strategies.

---

## 1. Semantic Code Chunking (High Priority)

### Problem
Current chunking splits code at arbitrary line boundaries based purely on token counts. This breaks functions mid-body, separates related code, and produces chunks that lack semantic coherence.

### Solution
Add AST-aware chunking that respects code structure boundaries.

### Schema Changes
```sql
-- Add to schema.sql (schema version 9)
ALTER TABLE chunks ADD COLUMN chunk_type TEXT DEFAULT 'line';
-- chunk_type: 'line' (legacy), 'function', 'class', 'block', 'file'

ALTER TABLE chunks ADD COLUMN symbol_name TEXT;
-- For function/class chunks: the symbol name

ALTER TABLE chunks ADD COLUMN symbol_kind TEXT;
-- 'function', 'method', 'class', 'struct', 'interface', 'const', 'var'

ALTER TABLE chunks ADD COLUMN parent_chunk_id TEXT;
-- For nested symbols (method inside class)

CREATE INDEX IF NOT EXISTS idx_chunks_symbol ON chunks (repo_id, workspace, symbol_name);
```

### Implementation

**New file: `internal/chunker/chunker.go`**
```go
package chunker

import (
    "go/ast"
    "go/parser"
    "go/token"
    "strings"
)

type ChunkType string

const (
    ChunkTypeLine     ChunkType = "line"
    ChunkTypeFunction ChunkType = "function"
    ChunkTypeClass    ChunkType = "class"
    ChunkTypeBlock    ChunkType = "block"
    ChunkTypeFile     ChunkType = "file"
)

type SemanticChunk struct {
    Text          string
    StartLine     int
    EndLine       int
    ChunkType     ChunkType
    SymbolName    string
    SymbolKind    string
    ParentChunkID string
}

type Chunker interface {
    Chunk(filename string, content []byte, maxTokens int) ([]SemanticChunk, error)
}

// Registry of chunkers by file extension
var chunkers = map[string]Chunker{
    ".go":  &GoChunker{},
    ".py":  &PythonChunker{},
    ".js":  &JSChunker{},
    ".ts":  &TSChunker{},
    ".tsx": &TSChunker{},
}

func GetChunker(ext string) Chunker {
    if c, ok := chunkers[strings.ToLower(ext)]; ok {
        return c
    }
    return &LineChunker{} // Fallback
}

// GoChunker extracts functions, methods, types from Go files
type GoChunker struct{}

func (c *GoChunker) Chunk(filename string, content []byte, maxTokens int) ([]SemanticChunk, error) {
    fset := token.NewFileSet()
    f, err := parser.ParseFile(fset, filename, content, parser.ParseComments)
    if err != nil {
        // Fall back to line chunking on parse error
        return (&LineChunker{}).Chunk(filename, content, maxTokens)
    }

    lines := strings.Split(string(content), "\n")
    var chunks []SemanticChunk

    // Extract package-level declarations
    for _, decl := range f.Decls {
        switch d := decl.(type) {
        case *ast.FuncDecl:
            chunk := extractFuncChunk(fset, d, lines)
            chunks = append(chunks, chunk)

        case *ast.GenDecl:
            for _, spec := range d.Specs {
                switch s := spec.(type) {
                case *ast.TypeSpec:
                    chunk := extractTypeChunk(fset, d, s, lines)
                    chunks = append(chunks, chunk)
                }
            }
        }
    }

    // If no semantic chunks found, fall back to line chunking
    if len(chunks) == 0 {
        return (&LineChunker{}).Chunk(filename, content, maxTokens)
    }

    // Split oversized chunks
    return splitOversizedChunks(chunks, maxTokens), nil
}

func extractFuncChunk(fset *token.FileSet, fn *ast.FuncDecl, lines []string) SemanticChunk {
    startPos := fset.Position(fn.Pos())
    endPos := fset.Position(fn.End())
    
    symbolKind := "function"
    symbolName := fn.Name.Name
    if fn.Recv != nil {
        symbolKind = "method"
        // Extract receiver type for method name
        if len(fn.Recv.List) > 0 {
            if t, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
                if ident, ok := t.X.(*ast.Ident); ok {
                    symbolName = ident.Name + "." + fn.Name.Name
                }
            } else if ident, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
                symbolName = ident.Name + "." + fn.Name.Name
            }
        }
    }

    text := strings.Join(lines[startPos.Line-1:endPos.Line], "\n")
    
    return SemanticChunk{
        Text:       text,
        StartLine:  startPos.Line,
        EndLine:    endPos.Line,
        ChunkType:  ChunkTypeFunction,
        SymbolName: symbolName,
        SymbolKind: symbolKind,
    }
}

func extractTypeChunk(fset *token.FileSet, gen *ast.GenDecl, spec *ast.TypeSpec, lines []string) SemanticChunk {
    startPos := fset.Position(gen.Pos())
    endPos := fset.Position(gen.End())
    
    symbolKind := "type"
    switch spec.Type.(type) {
    case *ast.StructType:
        symbolKind = "struct"
    case *ast.InterfaceType:
        symbolKind = "interface"
    }

    text := strings.Join(lines[startPos.Line-1:endPos.Line], "\n")
    
    return SemanticChunk{
        Text:       text,
        StartLine:  startPos.Line,
        EndLine:    endPos.Line,
        ChunkType:  ChunkTypeClass,
        SymbolName: spec.Name.Name,
        SymbolKind: symbolKind,
    }
}

func splitOversizedChunks(chunks []SemanticChunk, maxTokens int) []SemanticChunk {
    // Implementation: if a chunk exceeds maxTokens, split it
    // while preserving the semantic metadata
    var result []SemanticChunk
    for _, chunk := range chunks {
        // Estimate tokens (rough: 4 chars per token)
        estimatedTokens := len(chunk.Text) / 4
        if estimatedTokens <= maxTokens {
            result = append(result, chunk)
            continue
        }
        
        // Split large chunks at logical boundaries (empty lines, closing braces)
        subChunks := splitAtBoundaries(chunk, maxTokens)
        result = append(result, subChunks...)
    }
    return result
}

// LineChunker is the fallback for unsupported languages
type LineChunker struct{}

func (c *LineChunker) Chunk(filename string, content []byte, maxTokens int) ([]SemanticChunk, error) {
    // Existing line-based chunking logic from ingest.go
    // Returns chunks with ChunkType = ChunkTypeLine
    return nil, nil // Placeholder
}
```

**Python Chunker (using tree-sitter or regex fallback):**
```go
// internal/chunker/python.go
package chunker

import (
    "regexp"
    "strings"
)

type PythonChunker struct{}

var (
    pyFuncPattern  = regexp.MustCompile(`(?m)^(async\s+)?def\s+(\w+)\s*\(`)
    pyClassPattern = regexp.MustCompile(`(?m)^class\s+(\w+)`)
)

func (c *PythonChunker) Chunk(filename string, content []byte, maxTokens int) ([]SemanticChunk, error) {
    lines := strings.Split(string(content), "\n")
    var chunks []SemanticChunk
    
    // Find all function and class definitions
    type marker struct {
        line       int
        name       string
        kind       string
        indent     int
    }
    
    var markers []marker
    for i, line := range lines {
        if match := pyFuncPattern.FindStringSubmatch(line); match != nil {
            indent := len(line) - len(strings.TrimLeft(line, " \t"))
            markers = append(markers, marker{
                line:   i,
                name:   match[2],
                kind:   "function",
                indent: indent,
            })
        } else if match := pyClassPattern.FindStringSubmatch(line); match != nil {
            indent := len(line) - len(strings.TrimLeft(line, " \t"))
            markers = append(markers, marker{
                line:   i,
                name:   match[1],
                kind:   "class",
                indent: indent,
            })
        }
    }
    
    // Extract chunks between markers
    for i, m := range markers {
        endLine := len(lines)
        if i+1 < len(markers) {
            // Find where this block ends (next definition at same or lower indent)
            for j := i + 1; j < len(markers); j++ {
                if markers[j].indent <= m.indent {
                    endLine = markers[j].line
                    break
                }
            }
        }
        
        // Trim trailing empty lines
        for endLine > m.line && strings.TrimSpace(lines[endLine-1]) == "" {
            endLine--
        }
        
        text := strings.Join(lines[m.line:endLine], "\n")
        chunkType := ChunkTypeFunction
        if m.kind == "class" {
            chunkType = ChunkTypeClass
        }
        
        chunks = append(chunks, SemanticChunk{
            Text:       text,
            StartLine:  m.line + 1,
            EndLine:    endLine,
            ChunkType:  chunkType,
            SymbolName: m.name,
            SymbolKind: m.kind,
        })
    }
    
    if len(chunks) == 0 {
        return (&LineChunker{}).Chunk(filename, content, maxTokens)
    }
    
    return splitOversizedChunks(chunks, maxTokens), nil
}
```

**Update ingest.go to use semantic chunking:**
```go
// In ingest.go, replace the chunking logic

import "mempack/internal/chunker"

func processFile(path string) error {
    // ... existing file reading code ...
    
    ext := filepath.Ext(path)
    c := chunker.GetChunker(ext)
    
    semanticChunks, err := c.Chunk(path, data, *chunkTokens)
    if err != nil {
        return err
    }
    
    chunks := make([]store.Chunk, 0, len(semanticChunks))
    for _, sc := range semanticChunks {
        chunkHash := sha256.Sum256([]byte(sc.Text))
        locator := formatSemanticLocator(repoInfo, relPath, sc)
        
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
            ChunkType:  string(sc.ChunkType),
            SymbolName: sc.SymbolName,
            SymbolKind: sc.SymbolKind,
            TagsJSON:   "[]",
            TagsText:   "",
            CreatedAt:  time.Now().UTC(),
        })
    }
    
    // ... rest of function ...
}

func formatSemanticLocator(info repo.Info, relPath string, chunk chunker.SemanticChunk) string {
    base := ""
    if info.HasGit && info.Head != "" {
        base = fmt.Sprintf("git:%s:%s", info.Head, relPath)
    } else {
        base = fmt.Sprintf("file:%s", relPath)
    }
    
    if chunk.SymbolName != "" {
        return fmt.Sprintf("%s#%s", base, chunk.SymbolName)
    }
    return fmt.Sprintf("%s#L%d-L%d", base, chunk.StartLine, chunk.EndLine)
}
```

### Migration
```go
// In migrate.go, add migration to version 9
func migrateToV9(db *sql.DB) error {
    columns := []struct {
        name string
        typ  string
    }{
        {"chunk_type", "TEXT DEFAULT 'line'"},
        {"symbol_name", "TEXT"},
        {"symbol_kind", "TEXT"},
        {"parent_chunk_id", "TEXT"},
    }
    
    for _, col := range columns {
        if err := ensureColumn(db, "chunks", col.name, col.typ); err != nil {
            return err
        }
    }
    
    _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_chunks_symbol ON chunks (repo_id, workspace, symbol_name)`)
    return err
}
```

---

## 2. Auto-Context on MCP Initialize (High Priority)

### Problem
Agents must explicitly call `get_context` at task start. This is easy to forget and adds latency.

### Solution
Add optional auto-context that returns initial context during MCP session initialization.

### Implementation

**Update mcp.go:**
```go
// Add to mcp.go

type mcpSessionState struct {
    RepoID       string
    Workspace    string
    InitialPack  *pack.ContextPack
    LastQuery    string
    LastQueryAt  time.Time
}

var sessionState *mcpSessionState

func runMCP(args []string, out, errOut io.Writer) int {
    // ... existing flag parsing ...
    
    autoContext := fs.Bool("auto-context", false, "Fetch initial context on session start")
    autoContextQuery := fs.String("auto-context-query", "", "Query for auto-context (default: recent work)")
    
    // ... existing health check ...
    
    // Initialize session state
    sessionState = &mcpSessionState{
        RepoID:    report.Repo.ID,
        Workspace: resolveWorkspace(cfg, ""),
    }
    
    // Auto-fetch initial context if enabled
    if *autoContext {
        query := strings.TrimSpace(*autoContextQuery)
        if query == "" {
            query = buildAutoContextQuery(cfg, report.Repo.ID)
        }
        
        initialPack, err := buildContextPack(query, ContextOptions{
            Workspace:      sessionState.Workspace,
            IncludeOrphans: false,
        }, nil)
        if err == nil {
            sessionState.InitialPack = &initialPack
        }
    }
    
    srv := server.NewMCPServer(*name, *version, server.WithToolCapabilities(false))
    
    // Register resource for initial context
    if sessionState.InitialPack != nil {
        registerInitialContextResource(srv, sessionState.InitialPack)
    }
    
    registerMCPTools(srv, writeCfg)
    
    // ... rest of function ...
}

func buildAutoContextQuery(cfg config.Config, repoID string) string {
    // Build query from recent activity
    st, err := openStore(cfg, repoID)
    if err != nil {
        return "recent work"
    }
    defer st.Close()
    
    // Get most recent threads
    threads, err := st.ListRecentThreads(repoID, cfg.DefaultWorkspace, 3)
    if err != nil || len(threads) == 0 {
        return "recent work"
    }
    
    // Build query from recent thread titles/IDs
    var parts []string
    for _, t := range threads {
        if t.Title != "" {
            parts = append(parts, t.Title)
        } else {
            parts = append(parts, t.ThreadID)
        }
    }
    
    return strings.Join(parts, " ")
}

func registerInitialContextResource(srv *server.MCPServer, initialPack *pack.ContextPack) {
    // Register as an MCP resource that clients can read
    resource := mcp.NewResource(
        "mempack://context/initial",
        "Initial repository context",
        "application/json",
    )
    
    srv.AddResource(resource, func(ctx context.Context, req mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
        encoded, err := json.Marshal(initialPack)
        if err != nil {
            return nil, err
        }
        
        return &mcp.ReadResourceResult{
            Contents: []mcp.ResourceContents{
                mcp.TextResourceContents{
                    URI:      "mempack://context/initial",
                    MimeType: "application/json",
                    Text:     string(encoded),
                },
            },
        }, nil
    })
}
```

**Add to store for recent threads:**
```go
// In records.go or threads.go

func (s *Store) ListRecentThreads(repoID, workspace string, limit int) ([]Thread, error) {
    rows, err := s.db.Query(`
        SELECT t.thread_id, t.repo_id, t.workspace, t.title, t.tags_json, t.created_at
        FROM threads t
        JOIN (
            SELECT thread_id, MAX(created_at) as last_activity
            FROM memories
            WHERE repo_id = ? AND workspace = ? AND deleted_at IS NULL
            GROUP BY thread_id
        ) m ON t.thread_id = m.thread_id AND t.repo_id = ? AND t.workspace = ?
        ORDER BY m.last_activity DESC
        LIMIT ?
    `, repoID, workspace, repoID, workspace, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()
    
    var threads []Thread
    for rows.Next() {
        var t Thread
        if err := rows.Scan(&t.ThreadID, &t.RepoID, &t.Workspace, &t.Title, &t.TagsJSON, &t.CreatedAt); err != nil {
            return nil, err
        }
        threads = append(threads, t)
    }
    return threads, rows.Err()
}
```

**Update AGENTS.md generator:**
```go
// In agents_content.go, update instructions

func memoryInstructionsContent() string {
    return `# Mempack Instructions (Repo Memory)

## Auto-Context (when enabled)
If the MCP server was started with --auto-context, initial context is available
at resource URI: mempack://context/initial

Read this resource at session start instead of calling get_context manually.

## Manual Context Fetch (required when auto-context is disabled)
1) Fetch context for the current task.
   - MCP (preferred): call ` + "`" + `mempack.get_context(query="<task>")` + "`" + `
   
// ... rest of content ...
`
}
```

---

## 3. Memory Clustering and Summarization (High Priority)

### Problem
When memory count grows, individual memories compete for the token budget. Related memories about the same topic are shown separately, wasting tokens.

### Solution
Add automatic clustering of related memories with AI-generated summaries.

### Schema Changes
```sql
-- Schema version 10
CREATE TABLE IF NOT EXISTS memory_clusters (
    cluster_id TEXT PRIMARY KEY,
    repo_id TEXT NOT NULL,
    workspace TEXT NOT NULL DEFAULT 'default',
    thread_id TEXT,
    title TEXT NOT NULL,
    summary TEXT NOT NULL,
    summary_tokens INTEGER,
    memory_count INTEGER NOT NULL,
    first_created_at TEXT NOT NULL,
    last_created_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS memory_cluster_members (
    cluster_id TEXT NOT NULL,
    memory_id TEXT NOT NULL,
    added_at TEXT NOT NULL,
    PRIMARY KEY (cluster_id, memory_id)
);

CREATE INDEX IF NOT EXISTS idx_clusters_repo ON memory_clusters (repo_id, workspace);
CREATE INDEX IF NOT EXISTS idx_cluster_members_memory ON memory_cluster_members (memory_id);
```

### Implementation

**New file: `internal/cluster/cluster.go`**
```go
package cluster

import (
    "fmt"
    "sort"
    "strings"
    "time"
    
    "mempack/internal/store"
)

const (
    MinClusterSize       = 3
    MaxClusterSize       = 10
    SimilarityThreshold  = 0.7
    ClusterMaxAgeHours   = 24 * 7 // Re-cluster weekly
)

type ClusterConfig struct {
    MinSize            int
    MaxSize            int
    SimilarityThreshold float64
}

type MemoryCluster struct {
    ClusterID      string
    RepoID         string
    Workspace      string
    ThreadID       string
    Title          string
    Summary        string
    SummaryTokens  int
    MemoryCount    int
    MemoryIDs      []string
    FirstCreatedAt time.Time
    LastCreatedAt  time.Time
}

// ClusterMemories groups related memories
func ClusterMemories(memories []store.Memory, embeddings map[string][]float64, cfg ClusterConfig) []MemoryCluster {
    if len(memories) < cfg.MinSize {
        return nil
    }
    
    // Build similarity matrix
    n := len(memories)
    similarity := make([][]float64, n)
    for i := range similarity {
        similarity[i] = make([]float64, n)
    }
    
    for i := 0; i < n; i++ {
        for j := i + 1; j < n; j++ {
            sim := cosineSimilarity(embeddings[memories[i].ID], embeddings[memories[j].ID])
            similarity[i][j] = sim
            similarity[j][i] = sim
        }
    }
    
    // Greedy clustering: start with highest similarity pairs
    type pair struct {
        i, j int
        sim  float64
    }
    var pairs []pair
    for i := 0; i < n; i++ {
        for j := i + 1; j < n; j++ {
            if similarity[i][j] >= cfg.SimilarityThreshold {
                pairs = append(pairs, pair{i, j, similarity[i][j]})
            }
        }
    }
    sort.Slice(pairs, func(a, b int) bool {
        return pairs[a].sim > pairs[b].sim
    })
    
    // Assign to clusters
    assigned := make(map[int]int) // memory index -> cluster index
    var clusters [][]int
    
    for _, p := range pairs {
        ci, ciOK := assigned[p.i]
        cj, cjOK := assigned[p.j]
        
        if !ciOK && !cjOK {
            // Neither assigned: create new cluster
            clusterIdx := len(clusters)
            clusters = append(clusters, []int{p.i, p.j})
            assigned[p.i] = clusterIdx
            assigned[p.j] = clusterIdx
        } else if ciOK && !cjOK {
            // Add j to i's cluster if not too large
            if len(clusters[ci]) < cfg.MaxSize {
                clusters[ci] = append(clusters[ci], p.j)
                assigned[p.j] = ci
            }
        } else if !ciOK && cjOK {
            // Add i to j's cluster if not too large
            if len(clusters[cj]) < cfg.MaxSize {
                clusters[cj] = append(clusters[cj], p.i)
                assigned[p.i] = cj
            }
        }
        // If both assigned to different clusters, don't merge
    }
    
    // Convert to MemoryCluster structs
    var result []MemoryCluster
    for i, indices := range clusters {
        if len(indices) < cfg.MinSize {
            continue
        }
        
        clusterMems := make([]store.Memory, len(indices))
        for j, idx := range indices {
            clusterMems[j] = memories[idx]
        }
        
        cluster := buildCluster(fmt.Sprintf("CL-%d", i), clusterMems)
        result = append(result, cluster)
    }
    
    return result
}

func buildCluster(clusterID string, memories []store.Memory) MemoryCluster {
    // Find common thread
    threadCounts := make(map[string]int)
    for _, m := range memories {
        threadCounts[m.ThreadID]++
    }
    var commonThread string
    maxCount := 0
    for tid, count := range threadCounts {
        if count > maxCount {
            maxCount = count
            commonThread = tid
        }
    }
    
    // Find time range
    var first, last time.Time
    for i, m := range memories {
        if i == 0 || m.CreatedAt.Before(first) {
            first = m.CreatedAt
        }
        if i == 0 || m.CreatedAt.After(last) {
            last = m.CreatedAt
        }
    }
    
    // Build title from common words
    title := extractCommonTheme(memories)
    
    // Build summary (placeholder - ideally use LLM)
    var summaryParts []string
    for _, m := range memories {
        summaryParts = append(summaryParts, m.Title)
    }
    summary := fmt.Sprintf("Cluster of %d related memories: %s", len(memories), strings.Join(summaryParts, "; "))
    
    var memIDs []string
    for _, m := range memories {
        memIDs = append(memIDs, m.ID)
    }
    
    return MemoryCluster{
        ClusterID:      clusterID,
        RepoID:         memories[0].RepoID,
        Workspace:      memories[0].Workspace,
        ThreadID:       commonThread,
        Title:          title,
        Summary:        summary,
        MemoryCount:    len(memories),
        MemoryIDs:      memIDs,
        FirstCreatedAt: first,
        LastCreatedAt:  last,
    }
}

func extractCommonTheme(memories []store.Memory) string {
    // Simple: use most common title words
    wordCounts := make(map[string]int)
    for _, m := range memories {
        words := strings.Fields(strings.ToLower(m.Title))
        for _, w := range words {
            if len(w) > 3 { // Skip short words
                wordCounts[w]++
            }
        }
    }
    
    type wc struct {
        word  string
        count int
    }
    var sorted []wc
    for w, c := range wordCounts {
        sorted = append(sorted, wc{w, c})
    }
    sort.Slice(sorted, func(i, j int) bool {
        return sorted[i].count > sorted[j].count
    })
    
    if len(sorted) == 0 {
        return "Related memories"
    }
    if len(sorted) == 1 {
        return strings.Title(sorted[0].word)
    }
    return strings.Title(sorted[0].word) + " & " + strings.Title(sorted[1].word)
}

func cosineSimilarity(a, b []float64) float64 {
    if len(a) != len(b) || len(a) == 0 {
        return 0
    }
    var dot, normA, normB float64
    for i := range a {
        dot += a[i] * b[i]
        normA += a[i] * a[i]
        normB += b[i] * b[i]
    }
    if normA == 0 || normB == 0 {
        return 0
    }
    return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
```

**CLI command for clustering:**
```go
// New file: cluster.go in app package

func runCluster(args []string, out, errOut io.Writer) int {
    fs := flag.NewFlagSet("cluster", flag.ContinueOnError)
    fs.SetOutput(errOut)
    repoOverride := fs.String("repo", "", "Override repo id")
    workspace := fs.String("workspace", "", "Workspace name")
    minSize := fs.Int("min-size", 3, "Minimum cluster size")
    dryRun := fs.Bool("dry-run", false, "Show clusters without saving")
    
    if err := fs.Parse(args); err != nil {
        return 2
    }
    
    cfg, err := loadConfig()
    if err != nil {
        fmt.Fprintf(errOut, "config error: %v\n", err)
        return 1
    }
    workspaceName := resolveWorkspace(cfg, *workspace)
    
    repoInfo, err := resolveRepo(cfg, *repoOverride)
    if err != nil {
        fmt.Fprintf(errOut, "repo error: %v\n", err)
        return 1
    }
    
    st, err := openStore(cfg, repoInfo.ID)
    if err != nil {
        fmt.Fprintf(errOut, "store error: %v\n", err)
        return 1
    }
    defer st.Close()
    
    // Load all memories
    memories, err := st.ListAllMemories(repoInfo.ID, workspaceName)
    if err != nil {
        fmt.Fprintf(errOut, "memory list error: %v\n", err)
        return 1
    }
    
    // Load embeddings
    embeddings, _, err := st.ListEmbeddingsForSearch(repoInfo.ID, workspaceName, store.EmbeddingKindMemory, cfg.EmbeddingModel)
    if err != nil {
        fmt.Fprintf(errOut, "embedding error: %v\n", err)
        return 1
    }
    
    embeddingMap := make(map[string][]float64)
    for _, e := range embeddings {
        embeddingMap[e.ItemID] = e.Vector
    }
    
    // Run clustering
    clusters := cluster.ClusterMemories(memories, embeddingMap, cluster.ClusterConfig{
        MinSize:            *minSize,
        MaxSize:            cluster.MaxClusterSize,
        SimilarityThreshold: cluster.SimilarityThreshold,
    })
    
    if *dryRun {
        for _, c := range clusters {
            fmt.Fprintf(out, "Cluster: %s (%d memories)\n", c.Title, c.MemoryCount)
            for _, id := range c.MemoryIDs {
                fmt.Fprintf(out, "  - %s\n", id)
            }
        }
        return 0
    }
    
    // Save clusters
    for _, c := range clusters {
        if err := st.SaveCluster(c); err != nil {
            fmt.Fprintf(errOut, "save cluster error: %v\n", err)
            return 1
        }
    }
    
    fmt.Fprintf(out, "Created %d clusters\n", len(clusters))
    return 0
}
```

**Update context_builder.go to use clusters:**
```go
// In buildContextPack, after ranking memories

// Check if we should use clusters instead of individual memories
if len(rankedMemories) > cfg.MemoriesK*2 {
    clusters, err := st.ListClusters(repoInfo.ID, workspace)
    if err == nil && len(clusters) > 0 {
        // Replace some individual memories with cluster summaries
        budget.Memories = mergeWithClusters(budget.Memories, clusters, cfg.MemoriesK)
    }
}

func mergeWithClusters(memories []pack.MemoryItem, clusters []store.MemoryCluster, limit int) []pack.MemoryItem {
    // Keep top N/2 individual memories
    // Fill remaining slots with cluster summaries
    keepIndividual := limit / 2
    if keepIndividual > len(memories) {
        keepIndividual = len(memories)
    }
    
    result := memories[:keepIndividual]
    
    // Add cluster summaries for remaining slots
    clusterSlots := limit - len(result)
    for i := 0; i < clusterSlots && i < len(clusters); i++ {
        c := clusters[i]
        result = append(result, pack.MemoryItem{
            ID:       c.ClusterID,
            ThreadID: c.ThreadID,
            Title:    fmt.Sprintf("[Cluster] %s", c.Title),
            Summary:  c.Summary,
            IsCluster: true,
            ClusterSize: c.MemoryCount,
        })
    }
    
    return result
}
```

---

## 4. Smarter Query Understanding (Medium Priority)

### Problem
Queries are passed directly to FTS5. Natural language queries like "why did we choose postgres" don't work well.

### Solution
Add query parsing to extract intent and entities before search.

### Implementation

**New file: `internal/query/parser.go`**
```go
package query

import (
    "regexp"
    "strings"
)

type Intent string

const (
    IntentRecall    Intent = "recall"     // Find existing information
    IntentDecision  Intent = "decision"   // Find why something was decided
    IntentHowTo     Intent = "howto"      // Find instructions
    IntentStatus    Intent = "status"     // Find current state
    IntentRecent    Intent = "recent"     // Find recent activity
    IntentUnknown   Intent = "unknown"
)

type ParsedQuery struct {
    Original  string
    Intent    Intent
    Entities  []string
    Keywords  []string
    TimeHint  string  // "recent", "old", "last week", etc.
    Negations []string
}

var (
    decisionPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)^why\s+(did|do|does|should|would|could)\s+`),
        regexp.MustCompile(`(?i)\b(decision|chose|picked|selected|decided)\b`),
        regexp.MustCompile(`(?i)\b(reason|rationale|because)\b`),
    }
    
    howtoPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)^how\s+(do|to|can|should)\s+`),
        regexp.MustCompile(`(?i)^what('s| is) the (way|process|steps)\b`),
    }
    
    statusPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)^(what('s| is) the )?(current|latest|status)\b`),
        regexp.MustCompile(`(?i)\b(now|currently|today)\b`),
    }
    
    recentPatterns = []*regexp.Regexp{
        regexp.MustCompile(`(?i)\b(recent|latest|last|yesterday|today|this week)\b`),
    }
    
    entityPattern = regexp.MustCompile(`(?i)\b([A-Z][a-z]+(?:\s+[A-Z][a-z]+)*)\b`) // CamelCase or Title Case
    techPattern   = regexp.MustCompile(`(?i)\b(postgres|mysql|redis|kafka|docker|kubernetes|k8s|aws|gcp|azure|react|vue|angular|node|python|go|rust|java)\b`)
)

func Parse(query string) ParsedQuery {
    result := ParsedQuery{
        Original: query,
        Intent:   IntentUnknown,
    }
    
    // Detect intent
    for _, p := range decisionPatterns {
        if p.MatchString(query) {
            result.Intent = IntentDecision
            break
        }
    }
    if result.Intent == IntentUnknown {
        for _, p := range howtoPatterns {
            if p.MatchString(query) {
                result.Intent = IntentHowTo
                break
            }
        }
    }
    if result.Intent == IntentUnknown {
        for _, p := range statusPatterns {
            if p.MatchString(query) {
                result.Intent = IntentStatus
                break
            }
        }
    }
    if result.Intent == IntentUnknown {
        for _, p := range recentPatterns {
            if p.MatchString(query) {
                result.Intent = IntentRecent
                break
            }
        }
    }
    if result.Intent == IntentUnknown {
        result.Intent = IntentRecall
    }
    
    // Extract time hints
    for _, p := range recentPatterns {
        if matches := p.FindStringSubmatch(query); len(matches) > 0 {
            result.TimeHint = strings.ToLower(matches[0])
            break
        }
    }
    
    // Extract entities (tech terms)
    techMatches := techPattern.FindAllString(query, -1)
    for _, m := range techMatches {
        result.Entities = append(result.Entities, strings.ToLower(m))
    }
    
    // Extract proper nouns / capitalized words
    entityMatches := entityPattern.FindAllString(query, -1)
    for _, m := range entityMatches {
        lower := strings.ToLower(m)
        if !contains(result.Entities, lower) && !isStopWord(lower) {
            result.Entities = append(result.Entities, lower)
        }
    }
    
    // Extract keywords (non-stop words)
    words := strings.Fields(strings.ToLower(query))
    for _, w := range words {
        w = strings.Trim(w, "?.,!\"'")
        if len(w) > 2 && !isStopWord(w) && !contains(result.Entities, w) {
            result.Keywords = append(result.Keywords, w)
        }
    }
    
    return result
}

func (p ParsedQuery) ToFTSQuery() string {
    var parts []string
    
    // Entities get higher weight (exact match)
    for _, e := range p.Entities {
        parts = append(parts, fmt.Sprintf(`"%s"`, e))
    }
    
    // Keywords with prefix matching
    for _, k := range p.Keywords {
        if len(k) >= 3 {
            parts = append(parts, k+"*")
        } else {
            parts = append(parts, k)
        }
    }
    
    if len(parts) == 0 {
        return `""`
    }
    
    return strings.Join(parts, " OR ")
}

func (p ParsedQuery) ShouldBoostRecent() bool {
    return p.Intent == IntentRecent || p.Intent == IntentStatus || p.TimeHint != ""
}

func (p ParsedQuery) ShouldSearchDecisions() bool {
    return p.Intent == IntentDecision
}

var stopWords = map[string]bool{
    "the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
    "in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
    "with": true, "by": true, "from": true, "as": true, "is": true, "was": true,
    "are": true, "were": true, "been": true, "be": true, "have": true, "has": true,
    "had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
    "could": true, "should": true, "may": true, "might": true, "must": true,
    "can": true, "this": true, "that": true, "these": true, "those": true,
    "i": true, "you": true, "he": true, "she": true, "it": true, "we": true, "they": true,
    "what": true, "which": true, "who": true, "whom": true, "why": true, "how": true,
    "when": true, "where": true,
}

func isStopWord(w string) bool {
    return stopWords[w]
}

func contains(slice []string, item string) bool {
    for _, s := range slice {
        if s == item {
            return true
        }
    }
    return false
}
```

**Update context_builder.go to use parsed queries:**
```go
import "mempack/internal/query"

func buildContextPack(queryStr string, opts ContextOptions, timings *getTimings) (pack.ContextPack, error) {
    // Parse the query for intent and entities
    parsed := query.Parse(queryStr)
    
    // Build FTS query from parsed components
    ftsQuery := parsed.ToFTSQuery()
    
    // ... existing code ...
    
    // Modify ranking based on intent
    rankOpts := RankOptions{
        IncludeOrphans: opts.IncludeOrphans,
        VectorResults:  vectorMemResults,
    }
    
    if parsed.ShouldBoostRecent() {
        rankOpts.RecencyMultiplier = 2.0  // Double recency bonus
    }
    
    if parsed.ShouldSearchDecisions() {
        // Filter to memories with decision-related tags
        memResults = filterDecisionMemories(memResults)
    }
    
    // ... rest of function ...
}

func filterDecisionMemories(results []store.MemoryResult) []store.MemoryResult {
    var filtered []store.MemoryResult
    decisionTags := []string{"decision", "architecture", "design", "choice", "rationale"}
    
    for _, r := range results {
        // Keep all results but boost decision-tagged ones
        isDecision := false
        lowerTags := strings.ToLower(r.TagsText)
        for _, tag := range decisionTags {
            if strings.Contains(lowerTags, tag) {
                isDecision = true
                break
            }
        }
        if isDecision {
            r.BM25 *= 1.5 // Boost decision memories
        }
        filtered = append(filtered, r)
    }
    
    return filtered
}
```

---

## 5. File Watch Mode for Continuous Indexing (Medium Priority)

### Problem
Users must manually run `mem ingest-artifact` when files change. This is easy to forget.

### Solution
Add a `--watch` mode that monitors file changes and auto-indexes.

### Implementation

**New file: `internal/watcher/watcher.go`**
```go
package watcher

import (
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"
    
    "github.com/fsnotify/fsnotify"
)

type FileChange struct {
    Path      string
    Op        string // "create", "modify", "delete"
    Timestamp time.Time
}

type Watcher struct {
    root       string
    fsWatcher  *fsnotify.Watcher
    changes    chan FileChange
    ignorer    func(string) bool
    debounce   time.Duration
    pending    map[string]FileChange
    pendingMu  sync.Mutex
    stop       chan struct{}
}

func New(root string, ignorer func(string) bool) (*Watcher, error) {
    fsw, err := fsnotify.NewWatcher()
    if err != nil {
        return nil, err
    }
    
    w := &Watcher{
        root:      root,
        fsWatcher: fsw,
        changes:   make(chan FileChange, 100),
        ignorer:   ignorer,
        debounce:  500 * time.Millisecond,
        pending:   make(map[string]FileChange),
        stop:      make(chan struct{}),
    }
    
    return w, nil
}

func (w *Watcher) Start() error {
    // Add root and all subdirectories
    err := filepath.WalkDir(w.root, func(path string, d os.DirEntry, err error) error {
        if err != nil {
            return err
        }
        if d.IsDir() {
            relPath, _ := filepath.Rel(w.root, path)
            if w.ignorer(relPath) {
                return filepath.SkipDir
            }
            return w.fsWatcher.Add(path)
        }
        return nil
    })
    if err != nil {
        return err
    }
    
    // Start event processing goroutine
    go w.processEvents()
    
    // Start debounce flusher
    go w.flushPending()
    
    return nil
}

func (w *Watcher) Stop() {
    close(w.stop)
    w.fsWatcher.Close()
}

func (w *Watcher) Changes() <-chan FileChange {
    return w.changes
}

func (w *Watcher) processEvents() {
    for {
        select {
        case <-w.stop:
            return
        case event, ok := <-w.fsWatcher.Events:
            if !ok {
                return
            }
            
            relPath, err := filepath.Rel(w.root, event.Name)
            if err != nil {
                continue
            }
            
            if w.ignorer(relPath) {
                continue
            }
            
            // Determine operation
            var op string
            switch {
            case event.Op&fsnotify.Create != 0:
                op = "create"
                // If it's a new directory, watch it
                if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
                    w.fsWatcher.Add(event.Name)
                }
            case event.Op&fsnotify.Write != 0:
                op = "modify"
            case event.Op&fsnotify.Remove != 0:
                op = "delete"
            case event.Op&fsnotify.Rename != 0:
                op = "delete" // Treat rename as delete (new name will trigger create)
            default:
                continue
            }
            
            // Add to pending (debounce)
            w.pendingMu.Lock()
            w.pending[event.Name] = FileChange{
                Path:      event.Name,
                Op:        op,
                Timestamp: time.Now(),
            }
            w.pendingMu.Unlock()
            
        case err, ok := <-w.fsWatcher.Errors:
            if !ok {
                return
            }
            // Log error but continue
            _ = err
        }
    }
}

func (w *Watcher) flushPending() {
    ticker := time.NewTicker(w.debounce)
    defer ticker.Stop()
    
    for {
        select {
        case <-w.stop:
            return
        case <-ticker.C:
            w.pendingMu.Lock()
            now := time.Now()
            for path, change := range w.pending {
                if now.Sub(change.Timestamp) >= w.debounce {
                    select {
                    case w.changes <- change:
                    default:
                        // Channel full, skip
                    }
                    delete(w.pending, path)
                }
            }
            w.pendingMu.Unlock()
        }
    }
}
```

**Update ingest.go to support watch mode:**
```go
func runIngest(args []string, out, errOut io.Writer) int {
    fs := flag.NewFlagSet("ingest-artifact", flag.ContinueOnError)
    // ... existing flags ...
    watch := fs.Bool("watch", false, "Watch for file changes and auto-ingest")
    
    // ... existing setup code ...
    
    if *watch {
        return runIngestWatch(pathArg, repoInfo, workspaceName, *threadID, *maxFileMB, *chunkTokens, *overlapTokens, out, errOut)
    }
    
    // ... existing single-run code ...
}

func runIngestWatch(path string, repoInfo repo.Info, workspace, threadID string, maxFileMB, chunkTokens, overlapTokens int, out, errOut io.Writer) int {
    cfg, _ := loadConfig()
    
    root := repoInfo.GitRoot
    if root == "" {
        root = path
    }
    
    matcher := loadIgnoreMatcher(root)
    ignorer := func(relPath string) bool {
        if strings.HasPrefix(relPath, ".git") {
            return true
        }
        return matcher.Matches(relPath)
    }
    
    w, err := watcher.New(root, ignorer)
    if err != nil {
        fmt.Fprintf(errOut, "watcher error: %v\n", err)
        return 1
    }
    
    if err := w.Start(); err != nil {
        fmt.Fprintf(errOut, "watcher start error: %v\n", err)
        return 1
    }
    defer w.Stop()
    
    fmt.Fprintf(out, "Watching %s for changes (Ctrl+C to stop)\n", root)
    
    // Handle interrupt
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
    
    counter, _ := token.New(cfg.Tokenizer)
    
    for {
        select {
        case <-sigCh:
            fmt.Fprintln(out, "\nStopping watcher...")
            return 0
            
        case change := <-w.Changes():
            relPath, _ := filepath.Rel(root, change.Path)
            
            switch change.Op {
            case "create", "modify":
                if !allowedExtension(change.Path) {
                    continue
                }
                
                st, err := openStore(cfg, repoInfo.ID)
                if err != nil {
                    fmt.Fprintf(errOut, "store error: %v\n", err)
                    continue
                }
                
                resp, err := ingestSingleFile(st, counter, repoInfo, workspace, threadID, change.Path, relPath, root, maxFileMB, chunkTokens, overlapTokens)
                st.Close()
                
                if err != nil {
                    fmt.Fprintf(errOut, "ingest error for %s: %v\n", relPath, err)
                    continue
                }
                
                fmt.Fprintf(out, "[%s] %s: %d chunks\n", change.Op, relPath, resp.ChunksAdded)
                
            case "delete":
                st, err := openStore(cfg, repoInfo.ID)
                if err != nil {
                    continue
                }
                
                // Mark chunks from this file as deleted
                deleted, _ := st.DeleteChunksBySource(repoInfo.ID, workspace, relPath)
                st.Close()
                
                if deleted > 0 {
                    fmt.Fprintf(out, "[delete] %s: removed %d chunks\n", relPath, deleted)
                }
            }
        }
    }
}

func ingestSingleFile(st *store.Store, counter token.Counter, repoInfo repo.Info, workspace, threadID, fullPath, relPath, root string, maxFileMB, chunkTokens, overlapTokens int) (IngestResponse, error) {
    // Extract the file processing logic from runIngest into a reusable function
    // ... implementation similar to processFile but returns IngestResponse ...
    return IngestResponse{}, nil
}
```

**Add to store for deleting chunks by source:**
```go
// In records.go

func (s *Store) DeleteChunksBySource(repoID, workspace, source string) (int, error) {
    now := time.Now().UTC().Format(time.RFC3339Nano)
    
    result, err := s.db.Exec(`
        UPDATE chunks 
        SET deleted_at = ?
        WHERE repo_id = ? 
          AND workspace = ? 
          AND artifact_id IN (
              SELECT artifact_id FROM artifacts 
              WHERE repo_id = ? AND workspace = ? AND source = ?
          )
          AND deleted_at IS NULL
    `, now, repoID, workspace, repoID, workspace, source)
    if err != nil {
        return 0, err
    }
    
    affected, _ := result.RowsAffected()
    return int(affected), nil
}
```

---

## 6. Memory Access Tracking and Importance Decay (Lower Priority)

### Problem
All memories are treated equally regardless of how often they're accessed or how old they are.

### Solution
Track memory access patterns and decay importance over time.

### Schema Changes
```sql
-- Schema version 11
ALTER TABLE memories ADD COLUMN importance_score REAL DEFAULT 1.0;
ALTER TABLE memories ADD COLUMN last_accessed_at TEXT;
ALTER TABLE memories ADD COLUMN access_count INTEGER DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_memories_importance ON memories (repo_id, workspace, importance_score DESC);
```

### Implementation

**Update store to track access:**
```go
// In records.go

func (s *Store) RecordMemoryAccess(repoID, workspace string, memoryIDs []string) error {
    if len(memoryIDs) == 0 {
        return nil
    }
    
    now := time.Now().UTC().Format(time.RFC3339Nano)
    
    // Build placeholders
    placeholders := make([]string, len(memoryIDs))
    args := make([]any, 0, len(memoryIDs)+3)
    args = append(args, now, repoID, workspace)
    for i, id := range memoryIDs {
        placeholders[i] = "?"
        args = append(args, id)
    }
    
    query := fmt.Sprintf(`
        UPDATE memories 
        SET last_accessed_at = ?,
            access_count = COALESCE(access_count, 0) + 1
        WHERE repo_id = ? AND workspace = ? AND id IN (%s)
    `, strings.Join(placeholders, ","))
    
    _, err := s.db.Exec(query, args...)
    return err
}

func (s *Store) UpdateImportanceScores(repoID, workspace string) error {
    // Decay formula: importance = base_importance * exp(-days_since_access / 30) * log(access_count + 1)
    _, err := s.db.Exec(`
        UPDATE memories
        SET importance_score = 
            COALESCE(importance_score, 1.0) 
            * EXP(-JULIANDAY('now') - JULIANDAY(COALESCE(last_accessed_at, created_at)) / 30.0)
            * (1.0 + LOG(COALESCE(access_count, 0) + 1) / 10.0)
        WHERE repo_id = ? AND workspace = ? AND deleted_at IS NULL
    `, repoID, workspace)
    return err
}
```

**Update ranking to use importance:**
```go
// In rank.go, update FinalScore calculation

func rankMemories(...) {
    // ... existing code ...
    
    for i := range candidates {
        mem := &candidates[i]
        // ... existing scoring ...
        
        // Add importance factor
        importanceBonus := 0.0
        if mem.Memory.ImportanceScore > 0 {
            importanceBonus = mem.Memory.ImportanceScore * 0.1
        }
        
        mem.FinalScore = mem.RRFScore + mem.RecencyBonus + mem.ThreadBonus + importanceBonus
        // ... rest of scoring ...
    }
}
```

**Update context_builder to record access:**
```go
// In context_builder.go, after building the result

// Record access for returned memories
if len(budget.Memories) > 0 {
    memIDs := make([]string, len(budget.Memories))
    for i, m := range budget.Memories {
        memIDs[i] = m.ID
    }
    _ = st.RecordMemoryAccess(repoInfo.ID, workspace, memIDs)
}
```

---

## 7. Proactive Staleness Detection (Lower Priority)

### Problem
Agents don't know when memories reference deleted files or outdated code.

### Solution
Cross-reference memories/chunks with current file system state.

### Implementation

**New file: `internal/staleness/detector.go`**
```go
package staleness

import (
    "os"
    "path/filepath"
    "regexp"
    "strings"
    "time"
    
    "mempack/internal/store"
)

type StalenessWarning struct {
    ItemID      string
    ItemType    string // "memory", "chunk"
    Reason      string
    Severity    string // "info", "warning", "error"
    Suggestion  string
}

var fileRefPattern = regexp.MustCompile(`(?i)\b([a-zA-Z0-9_/.-]+\.(go|py|js|ts|java|rs|c|cpp|h))\b`)

func DetectStaleItems(gitRoot string, memories []store.Memory, chunks []store.Chunk) []StalenessWarning {
    var warnings []StalenessWarning
    
    // Check memories for file references
    for _, mem := range memories {
        refs := extractFileRefs(mem.Summary + " " + mem.Title)
        for _, ref := range refs {
            fullPath := filepath.Join(gitRoot, ref)
            if _, err := os.Stat(fullPath); os.IsNotExist(err) {
                warnings = append(warnings, StalenessWarning{
                    ItemID:     mem.ID,
                    ItemType:   "memory",
                    Reason:     fmt.Sprintf("References deleted file: %s", ref),
                    Severity:   "warning",
                    Suggestion: fmt.Sprintf("Consider updating or superseding memory %s", mem.ID),
                })
            }
        }
        
        // Check age
        age := time.Since(mem.CreatedAt)
        if age > 90*24*time.Hour { // 90 days
            warnings = append(warnings, StalenessWarning{
                ItemID:     mem.ID,
                ItemType:   "memory",
                Reason:     fmt.Sprintf("Memory is %d days old", int(age.Hours()/24)),
                Severity:   "info",
                Suggestion: "Consider reviewing if this is still accurate",
            })
        }
    }
    
    // Check chunks for file existence
    for _, chunk := range chunks {
        // Extract file path from locator
        filePath := extractFileFromLocator(chunk.Locator)
        if filePath != "" {
            fullPath := filepath.Join(gitRoot, filePath)
            if _, err := os.Stat(fullPath); os.IsNotExist(err) {
                warnings = append(warnings, StalenessWarning{
                    ItemID:     chunk.ID,
                    ItemType:   "chunk",
                    Reason:     fmt.Sprintf("Source file deleted: %s", filePath),
                    Severity:   "error",
                    Suggestion: "Run: mem forget --chunk " + chunk.ID,
                })
            }
        }
    }
    
    return warnings
}

func extractFileRefs(text string) []string {
    matches := fileRefPattern.FindAllString(text, -1)
    seen := make(map[string]bool)
    var unique []string
    for _, m := range matches {
        if !seen[m] {
            seen[m] = true
            unique = append(unique, m)
        }
    }
    return unique
}

func extractFileFromLocator(locator string) string {
    // Locator format: "git:<commit>:<path>#L1-L10" or "file:<path>#L1-L10"
    if strings.HasPrefix(locator, "git:") {
        parts := strings.SplitN(locator, ":", 3)
        if len(parts) >= 3 {
            pathPart := parts[2]
            if idx := strings.Index(pathPart, "#"); idx != -1 {
                return pathPart[:idx]
            }
            return pathPart
        }
    }
    if strings.HasPrefix(locator, "file:") {
        pathPart := strings.TrimPrefix(locator, "file:")
        if idx := strings.Index(pathPart, "#"); idx != -1 {
            return pathPart[:idx]
        }
        return pathPart
    }
    return ""
}
```

**Add warnings to context pack:**
```go
// In pack/types.go

type ContextPack struct {
    // ... existing fields ...
    Warnings []ContextWarning `json:"warnings,omitempty"`
}

type ContextWarning struct {
    ItemID     string `json:"item_id"`
    ItemType   string `json:"item_type"`
    Message    string `json:"message"`
    Severity   string `json:"severity"`
    Suggestion string `json:"suggestion,omitempty"`
}
```

**Update context_builder to include warnings:**
```go
// In context_builder.go

import "mempack/internal/staleness"

func buildContextPack(...) (pack.ContextPack, error) {
    // ... existing code ...
    
    // Detect stale items
    staleWarnings := staleness.DetectStaleItems(
        repoInfo.GitRoot,
        extractMemoriesFromBudget(budget.Memories),
        extractChunksFromBudget(budget.Chunks),
    )
    
    var contextWarnings []pack.ContextWarning
    for _, sw := range staleWarnings {
        contextWarnings = append(contextWarnings, pack.ContextWarning{
            ItemID:     sw.ItemID,
            ItemType:   sw.ItemType,
            Message:    sw.Reason,
            Severity:   sw.Severity,
            Suggestion: sw.Suggestion,
        })
    }
    
    result := pack.ContextPack{
        // ... existing fields ...
        Warnings: contextWarnings,
    }
    
    return result, nil
}
```

---

## 8. Memory Export/Import for Team Sharing (Lower Priority)

### Implementation

**New CLI commands:**
```go
// export.go
func runExport(args []string, out, errOut io.Writer) int {
    fs := flag.NewFlagSet("export", flag.ContinueOnError)
    repoOverride := fs.String("repo", "", "Override repo id")
    workspace := fs.String("workspace", "", "Workspace name")
    output := fs.String("output", "", "Output file path (default: stdout)")
    includeEmbeddings := fs.Bool("include-embeddings", false, "Include vector embeddings")
    
    // ... parse args ...
    
    cfg, _ := loadConfig()
    repoInfo, _ := resolveRepo(cfg, *repoOverride)
    st, _ := openStore(cfg, repoInfo.ID)
    defer st.Close()
    
    export := MemoryExport{
        Version:   "1.0",
        ExportedAt: time.Now().UTC(),
        RepoID:    repoInfo.ID,
        Workspace: *workspace,
    }
    
    // Load all memories
    memories, _ := st.ListAllMemories(repoInfo.ID, *workspace)
    for _, m := range memories {
        export.Memories = append(export.Memories, ExportedMemory{
            ID:           m.ID,
            ThreadID:     m.ThreadID,
            Title:        m.Title,
            Summary:      m.Summary,
            Tags:         parseTags(m.TagsJSON),
            Entities:     parseTags(m.EntitiesJSON),
            AnchorCommit: m.AnchorCommit,
            CreatedAt:    m.CreatedAt,
        })
    }
    
    // Load state
    state, _, _, _ := st.GetStateCurrent(repoInfo.ID, *workspace)
    export.State = json.RawMessage(state)
    
    // Optionally load embeddings
    if *includeEmbeddings {
        embeddings, _, _ := st.ListEmbeddingsForSearch(repoInfo.ID, *workspace, "", cfg.EmbeddingModel)
        for _, e := range embeddings {
            export.Embeddings = append(export.Embeddings, ExportedEmbedding{
                ItemID: e.ItemID,
                Kind:   e.Kind,
                Vector: e.Vector,
            })
        }
    }
    
    encoded, _ := json.MarshalIndent(export, "", "  ")
    
    if *output != "" {
        return writeFile(*output, encoded)
    }
    fmt.Fprintln(out, string(encoded))
    return 0
}

type MemoryExport struct {
    Version    string            `json:"version"`
    ExportedAt time.Time         `json:"exported_at"`
    RepoID     string            `json:"repo_id"`
    Workspace  string            `json:"workspace"`
    State      json.RawMessage   `json:"state,omitempty"`
    Memories   []ExportedMemory  `json:"memories"`
    Embeddings []ExportedEmbedding `json:"embeddings,omitempty"`
}

type ExportedMemory struct {
    ID           string    `json:"id"`
    ThreadID     string    `json:"thread_id"`
    Title        string    `json:"title"`
    Summary      string    `json:"summary"`
    Tags         []string  `json:"tags,omitempty"`
    Entities     []string  `json:"entities,omitempty"`
    AnchorCommit string    `json:"anchor_commit,omitempty"`
    CreatedAt    time.Time `json:"created_at"`
}

type ExportedEmbedding struct {
    ItemID string    `json:"item_id"`
    Kind   string    `json:"kind"`
    Vector []float64 `json:"vector"`
}

// import.go
func runImport(args []string, out, errOut io.Writer) int {
    fs := flag.NewFlagSet("import", flag.ContinueOnError)
    input := fs.String("input", "", "Input file path")
    repoOverride := fs.String("repo", "", "Override repo id")
    workspace := fs.String("workspace", "", "Workspace name")
    merge := fs.String("merge", "skip", "Conflict resolution: skip|replace|rename")
    
    // ... parse args ...
    
    data, _ := os.ReadFile(*input)
    var export MemoryExport
    json.Unmarshal(data, &export)
    
    cfg, _ := loadConfig()
    repoInfo, _ := resolveRepo(cfg, *repoOverride)
    st, _ := openStore(cfg, repoInfo.ID)
    defer st.Close()
    
    counter, _ := token.New(cfg.Tokenizer)
    
    imported := 0
    skipped := 0
    
    for _, em := range export.Memories {
        // Check if exists
        existing, err := st.GetMemory(repoInfo.ID, *workspace, em.ID)
        if err == nil && existing.ID != "" {
            switch *merge {
            case "skip":
                skipped++
                continue
            case "replace":
                st.ForgetMemory(repoInfo.ID, *workspace, em.ID, time.Now())
            case "rename":
                em.ID = store.NewID("M")
            }
        }
        
        // Import memory
        _, err = st.AddMemory(store.AddMemoryInput{
            ID:            em.ID,
            RepoID:        repoInfo.ID,
            Workspace:     *workspace,
            ThreadID:      em.ThreadID,
            Title:         em.Title,
            Summary:       em.Summary,
            SummaryTokens: counter.Count(em.Summary),
            TagsJSON:      store.TagsToJSON(em.Tags),
            TagsText:      store.TagsText(em.Tags),
            EntitiesJSON:  store.TagsToJSON(em.Entities),
            EntitiesText:  store.TagsText(em.Entities),
            AnchorCommit:  em.AnchorCommit,
            CreatedAt:     em.CreatedAt,
        })
        if err == nil {
            imported++
        }
    }
    
    fmt.Fprintf(out, "Imported %d memories, skipped %d\n", imported, skipped)
    return 0
}
```

---

## Summary: Implementation Priority

| Feature | Complexity | Impact | Recommended Order |
|---------|------------|--------|-------------------|
| Semantic Code Chunking | Medium | High | 1 |
| Auto-Context on MCP Init | Low | High | 2 |
| File Watch Mode | Medium | High | 3 |
| Query Understanding | Medium | Medium | 4 |
| Memory Clustering | High | Medium | 5 |
| Staleness Detection | Low | Medium | 6 |
| Importance Decay | Low | Low | 7 |
| Export/Import | Low | Low | 8 |

**Recommended Sprint Plan:**

**Sprint 1 (1-2 weeks):**
- Semantic Code Chunking (Go + Python)
- Auto-Context on MCP Init

**Sprint 2 (1-2 weeks):**
- File Watch Mode
- Query Understanding

**Sprint 3 (2-3 weeks):**
- Memory Clustering
- Staleness Detection

**Sprint 4 (1 week):**
- Importance Decay
- Export/Import


Corrected Implementation: Semantic Code Chunking
1. Schema version bump + migration:
go// migrate.go - update schemaVersion
const schemaVersion = 9 // was 8

// Add to migrate() switch
case 8:
    if err := migrateV8ToV9(db); err != nil {
        return err
    }
    fallthrough

func migrateV8ToV9(db *sql.DB) error {
    columns := []struct{ name, typ string }{
        {"chunk_type", "TEXT DEFAULT 'line'"},
        {"symbol_name", "TEXT"},
        {"symbol_kind", "TEXT"},
    }
    for _, col := range columns {
        if err := ensureColumn(db, "chunks", col.name, col.typ); err != nil {
            return err
        }
    }
    // Index for symbol lookup
    _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_chunks_symbol ON chunks (repo_id, workspace, symbol_name) WHERE symbol_name IS NOT NULL`)
    return err
}
2. Update store.Chunk type (memory.go):
gotype Chunk struct {
    ID         string
    RepoID     string
    Workspace  string
    ArtifactID string
    ThreadID   string
    Locator    string
    Text       string
    TextHash   string
    TextTokens int
    TagsJSON   string
    TagsText   string
    ChunkType  string // NEW: "line", "function", "class", "block"
    SymbolName string // NEW: e.g., "Store.AddMemory"
    SymbolKind string // NEW: "function", "method", "struct"
    CreatedAt  time.Time
    DeletedAt  time.Time
}
3. Update chunk scanning (records.go):
go// In scanChunk helper or wherever chunks are scanned
func (s *Store) scanChunk(rows *sql.Rows) (Chunk, error) {
    var c Chunk
    var chunkType, symbolName, symbolKind sql.NullString
    var deletedAt sql.NullString
    
    err := rows.Scan(
        &c.ID, &c.RepoID, &c.Workspace, &c.ArtifactID, &c.ThreadID,
        &c.Locator, &c.Text, &c.TextHash, &c.TextTokens,
        &c.TagsJSON, &c.TagsText,
        &chunkType, &symbolName, &symbolKind, // NEW
        &c.CreatedAt, &deletedAt,
    )
    if err != nil {
        return c, err
    }
    c.ChunkType = chunkType.String
    c.SymbolName = symbolName.String
    c.SymbolKind = symbolKind.String
    if deletedAt.Valid {
        c.DeletedAt, _ = time.Parse(time.RFC3339Nano, deletedAt.String)
    }
    return c, nil
}

// Update AddArtifactWithChunks INSERT to include new columns
_, err = tx.Exec(`
    INSERT INTO chunks (chunk_id, repo_id, workspace, artifact_id, thread_id, locator, text, text_hash, text_tokens, tags_json, tags_text, chunk_type, symbol_name, symbol_kind, created_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT (repo_id, workspace, locator, text_hash, thread_id) DO NOTHING
`, chunk.ID, chunk.RepoID, chunkWorkspace, chunk.ArtifactID, chunk.ThreadID, chunk.Locator, chunk.Text, chunk.TextHash, chunk.TextTokens, chunk.TagsJSON, chunk.TagsText, chunk.ChunkType, chunk.SymbolName, chunk.SymbolKind, createdAt)
4. Chunker with real token counting (new file chunker.go in app package):
gopackage app

import (
    "go/ast"
    "go/parser"
    "go/token"
    "path/filepath"
    "strings"

    "mempack/internal/token"
)

type SemanticChunk struct {
    Text       string
    StartLine  int
    EndLine    int
    ChunkType  string
    SymbolName string
    SymbolKind string
}

func chunkFile(path string, content []byte, maxTokens int, counter token.Counter) ([]SemanticChunk, error) {
    ext := strings.ToLower(filepath.Ext(path))
    
    switch ext {
    case ".go":
        return chunkGo(path, content, maxTokens, counter)
    default:
        return chunkLines(content, maxTokens, counter)
    }
}

func chunkGo(path string, content []byte, maxTokens int, counter token.Counter) ([]SemanticChunk, error) {
    fset := token.NewFileSet()
    f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
    if err != nil {
        // Parse failed, fall back to line chunking
        return chunkLines(content, maxTokens, counter)
    }

    lines := strings.Split(string(content), "\n")
    var chunks []SemanticChunk

    for _, decl := range f.Decls {
        switch d := decl.(type) {
        case *ast.FuncDecl:
            chunk := extractGoFunc(fset, d, lines, counter, maxTokens)
            chunks = append(chunks, chunk...)
        case *ast.GenDecl:
            for _, spec := range d.Specs {
                if ts, ok := spec.(*ast.TypeSpec); ok {
                    chunk := extractGoType(fset, d, ts, lines, counter, maxTokens)
                    chunks = append(chunks, chunk...)
                }
            }
        }
    }

    if len(chunks) == 0 {
        return chunkLines(content, maxTokens, counter)
    }
    return chunks, nil
}

func extractGoFunc(fset *token.FileSet, fn *ast.FuncDecl, lines []string, counter token.Counter, maxTokens int) []SemanticChunk {
    startPos := fset.Position(fn.Pos())
    endPos := fset.Position(fn.End())
    
    startLine := startPos.Line - 1
    endLine := endPos.Line
    if endLine > len(lines) {
        endLine = len(lines)
    }
    
    text := strings.Join(lines[startLine:endLine], "\n")
    tokens := counter.Count(text)
    
    symbolKind := "function"
    symbolName := fn.Name.Name
    if fn.Recv != nil && len(fn.Recv.List) > 0 {
        symbolKind = "method"
        if t, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
            if ident, ok := t.X.(*ast.Ident); ok {
                symbolName = ident.Name + "." + fn.Name.Name
            }
        } else if ident, ok := fn.Recv.List[0].Type.(*ast.Ident); ok {
            symbolName = ident.Name + "." + fn.Name.Name
        }
    }

    // If within budget, return as single chunk
    if tokens <= maxTokens {
        return []SemanticChunk{{
            Text:       text,
            StartLine:  startLine + 1,
            EndLine:    endLine,
            ChunkType:  "function",
            SymbolName: symbolName,
            SymbolKind: symbolKind,
        }}
    }

    // Oversized: split into line chunks but preserve symbol metadata
    return splitWithMetadata(lines[startLine:endLine], startLine+1, symbolName, symbolKind, counter, maxTokens)
}

func splitWithMetadata(lines []string, baseLineNum int, symbolName, symbolKind string, counter token.Counter, maxTokens int) []SemanticChunk {
    var chunks []SemanticChunk
    var buf []string
    bufTokens := 0
    bufStart := baseLineNum

    for i, line := range lines {
        lineTokens := counter.Count(line)
        
        if bufTokens > 0 && bufTokens+lineTokens > maxTokens {
            // Flush buffer
            chunks = append(chunks, SemanticChunk{
                Text:       strings.Join(buf, "\n"),
                StartLine:  bufStart,
                EndLine:    baseLineNum + i - 1,
                ChunkType:  "block",
                SymbolName: symbolName,
                SymbolKind: symbolKind,
            })
            buf = nil
            bufTokens = 0
            bufStart = baseLineNum + i
        }
        
        buf = append(buf, line)
        bufTokens += lineTokens
    }

    if len(buf) > 0 {
        chunks = append(chunks, SemanticChunk{
            Text:       strings.Join(buf, "\n"),
            StartLine:  bufStart,
            EndLine:    baseLineNum + len(lines) - 1,
            ChunkType:  "block",
            SymbolName: symbolName,
            SymbolKind: symbolKind,
        })
    }

    return chunks
}

func chunkLines(content []byte, maxTokens int, counter token.Counter) ([]SemanticChunk, error) {
    // Existing line-based logic from ingest.go, wrapped to return SemanticChunk
    lines := strings.Split(string(content), "\n")
    lineTokens := make([]int, len(lines))
    for i, line := range lines {
        lineTokens[i] = counter.Count(line)
    }

    ranges := chunkRanges(lines, lineTokens, maxTokens, maxTokens/8) // 12.5% overlap
    var chunks []SemanticChunk
    for _, r := range ranges {
        text := strings.Join(lines[r.Start:r.End], "\n")
        chunks = append(chunks, SemanticChunk{
            Text:      text,
            StartLine: r.Start + 1,
            EndLine:   r.End,
            ChunkType: "line",
        })
    }
    return chunks, nil
}
5. Update ingest.go to use chunker (preserve locator format):
go// In processFile, replace the chunking section:

semanticChunks, err := chunkFile(path, data, *chunkTokens, counter)
if err != nil {
    return err
}

chunks := make([]store.Chunk, 0, len(semanticChunks))
for _, sc := range semanticChunks {
    chunkHash := sha256.Sum256([]byte(sc.Text))
    
    // Preserve existing locator format: git:<commit>:<path>#L<start>-L<end>
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
6. Update schema.sql for reference (though migrations handle runtime):
sql-- Add after existing chunks columns
-- chunk_type TEXT DEFAULT 'line',
-- symbol_name TEXT,
-- symbol_kind TEXT,

This is one feature done correctly. The other 7 need similar treatment—each requires tracing through all the touch points (schema, types, inserts, reads, FTS, tests, prompt rendering)

# Semantic Chunking: Correct Implementation

This is a complete, compilable implementation that matches the existing codebase patterns.

---

## 1. Update schema version and add migration (internal/store/migrate.go)

```go
// Change line 10:
const schemaVersion = 9  // was 8

// Add to ensureColumns() function after line 240:
func ensureColumns(db *sql.DB) error {
    // ... existing columns ...
    
    // New chunk semantic fields (schema v9)
    if err := ensureColumn(db, "chunks", "chunk_type", "TEXT"); err != nil {
        return err
    }
    if err := ensureColumn(db, "chunks", "symbol_name", "TEXT"); err != nil {
        return err
    }
    if err := ensureColumn(db, "chunks", "symbol_kind", "TEXT"); err != nil {
        return err
    }
    return nil
}

// Add new function after ensureWorkspaceIndexes():
func ensureChunkSymbolIndex(db *sql.DB) error {
    _, err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_chunks_symbol 
        ON chunks (repo_id, workspace, symbol_name) 
        WHERE symbol_name IS NOT NULL AND symbol_name != ''
    `)
    return err
}

// Update migrate() to call the new function, add after line 40:
func migrate(db *sql.DB) error {
    // ... existing code ...
    
    if err := ensureChunkSymbolIndex(db); err != nil {
        return err
    }
    
    // ... rest of function ...
}
```

---

## 2. Update Chunk struct (internal/store/memory.go)

```go
// Replace lines 394-408:
type Chunk struct {
    ID         string
    RepoID     string
    Workspace  string
    ArtifactID string
    ThreadID   string
    Locator    string
    Text       string
    TextHash   string
    TextTokens int
    TagsJSON   string
    TagsText   string
    ChunkType  string    // NEW: "line", "function", "class", "block"
    SymbolName string    // NEW: e.g., "Store.AddMemory"
    SymbolKind string    // NEW: "function", "method", "struct", "interface"
    CreatedAt  time.Time
    DeletedAt  time.Time
}
```

---

## 3. Update searchChunksWithQuery (internal/store/memory.go)

```go
// Replace lines 335-372:
    fetchStart := time.Now()
    querySQL := fmt.Sprintf(`
        SELECT rowid, chunk_id, repo_id, workspace, artifact_id, thread_id, locator, 
               text, text_hash, text_tokens, tags_json, tags_text,
               chunk_type, symbol_name, symbol_kind, created_at
        FROM chunks
        WHERE rowid IN (%s)
        AND repo_id = ?
        AND workspace = ?
        AND deleted_at IS NULL
    `, placeholders)
    fetchRows, err := s.db.Query(querySQL, args...)
    if err != nil {
        return nil, stats, err
    }
    defer fetchRows.Close()

    details := make(map[int64]Chunk, len(rowIDs))
    for fetchRows.Next() {
        var rowid int64
        var chunk Chunk
        var createdAt string
        var artifactID sql.NullString
        var threadID sql.NullString
        var locator sql.NullString
        var text sql.NullString
        var textHash sql.NullString
        var textTokens sql.NullInt64
        var tagsJSON sql.NullString
        var tagsText sql.NullString
        var chunkType sql.NullString
        var symbolName sql.NullString
        var symbolKind sql.NullString
        if err := fetchRows.Scan(&rowid, &chunk.ID, &chunk.RepoID, &chunk.Workspace, 
            &artifactID, &threadID, &locator, &text, &textHash, &textTokens, 
            &tagsJSON, &tagsText, &chunkType, &symbolName, &symbolKind, &createdAt); err != nil {
            return nil, stats, err
        }
        chunk.ArtifactID = artifactID.String
        chunk.ThreadID = threadID.String
        chunk.Locator = locator.String
        chunk.Text = text.String
        chunk.TextHash = textHash.String
        if textTokens.Valid {
            chunk.TextTokens = int(textTokens.Int64)
        }
        chunk.TagsJSON = tagsJSON.String
        chunk.TagsText = tagsText.String
        chunk.ChunkType = chunkType.String
        chunk.SymbolName = symbolName.String
        chunk.SymbolKind = symbolKind.String
        chunk.CreatedAt = parseTime(createdAt)
        details[rowid] = chunk
    }
```

---

## 4. Update GetChunk (internal/store/records.go)

```go
// Replace lines 120-159:
func (s *Store) GetChunk(repoID, workspace, id string) (Chunk, error) {
    row := s.db.QueryRow(`
        SELECT chunk_id, repo_id, workspace, artifact_id, thread_id, locator, 
               text, text_hash, text_tokens, tags_json, tags_text,
               chunk_type, symbol_name, symbol_kind, created_at, deleted_at
        FROM chunks
        WHERE repo_id = ? AND workspace = ? AND chunk_id = ?
    `, repoID, normalizeWorkspace(workspace), id)

    var chunk Chunk
    var createdAt string
    var deletedAt sql.NullString
    var threadID sql.NullString
    var artifactID sql.NullString
    var locator sql.NullString
    var text sql.NullString
    var textHash sql.NullString
    var textTokens sql.NullInt64
    var tagsJSON sql.NullString
    var tagsText sql.NullString
    var chunkType sql.NullString
    var symbolName sql.NullString
    var symbolKind sql.NullString
    if err := row.Scan(&chunk.ID, &chunk.RepoID, &chunk.Workspace, &artifactID, &threadID, 
        &locator, &text, &textHash, &textTokens, &tagsJSON, &tagsText,
        &chunkType, &symbolName, &symbolKind, &createdAt, &deletedAt); err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return Chunk{}, ErrNotFound
        }
        return Chunk{}, err
    }
    chunk.ArtifactID = artifactID.String
    chunk.ThreadID = threadID.String
    chunk.Locator = locator.String
    chunk.Text = text.String
    chunk.TextHash = textHash.String
    if textTokens.Valid {
        chunk.TextTokens = int(textTokens.Int64)
    }
    chunk.TagsJSON = tagsJSON.String
    chunk.TagsText = tagsText.String
    chunk.ChunkType = chunkType.String
    chunk.SymbolName = symbolName.String
    chunk.SymbolKind = symbolKind.String
    chunk.CreatedAt = parseTime(createdAt)
    if deletedAt.Valid {
        chunk.DeletedAt = parseTime(deletedAt.String)
    }
    return chunk, nil
}
```

---

## 5. Update GetChunksByIDs (internal/store/records.go)

```go
// Replace lines 222-278:
func (s *Store) GetChunksByIDs(repoID, workspace string, ids []string) ([]Chunk, error) {
    if len(ids) == 0 {
        return nil, nil
    }
    placeholders := strings.Repeat("?,", len(ids))
    placeholders = strings.TrimSuffix(placeholders, ",")
    args := make([]any, 0, len(ids)+2)
    args = append(args, repoID, normalizeWorkspace(workspace))
    for _, id := range ids {
        args = append(args, id)
    }

    rows, err := s.db.Query(fmt.Sprintf(`
        SELECT chunk_id, repo_id, workspace, artifact_id, thread_id, locator, 
               text, text_hash, text_tokens, tags_json, tags_text,
               chunk_type, symbol_name, symbol_kind, created_at, deleted_at
        FROM chunks
        WHERE repo_id = ? AND workspace = ? AND chunk_id IN (%s) AND deleted_at IS NULL
    `, placeholders), args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var chunks []Chunk
    for rows.Next() {
        var chunk Chunk
        var createdAt string
        var deletedAt sql.NullString
        var textHash sql.NullString
        var textTokens sql.NullInt64
        var tagsJSON sql.NullString
        var tagsText sql.NullString
        var artifactID sql.NullString
        var threadID sql.NullString
        var locator sql.NullString
        var chunkType sql.NullString
        var symbolName sql.NullString
        var symbolKind sql.NullString
        if err := rows.Scan(&chunk.ID, &chunk.RepoID, &chunk.Workspace, &artifactID, &threadID, 
            &locator, &chunk.Text, &textHash, &textTokens, &tagsJSON, &tagsText,
            &chunkType, &symbolName, &symbolKind, &createdAt, &deletedAt); err != nil {
            return nil, err
        }
        chunk.ArtifactID = artifactID.String
        chunk.ThreadID = threadID.String
        chunk.Locator = locator.String
        chunk.TextHash = textHash.String
        if textTokens.Valid {
            chunk.TextTokens = int(textTokens.Int64)
        }
        chunk.TagsJSON = tagsJSON.String
        chunk.TagsText = tagsText.String
        chunk.ChunkType = chunkType.String
        chunk.SymbolName = symbolName.String
        chunk.SymbolKind = symbolKind.String
        chunk.CreatedAt = parseTime(createdAt)
        if deletedAt.Valid {
            chunk.DeletedAt = parseTime(deletedAt.String)
        }
        chunks = append(chunks, chunk)
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }
    return chunks, nil
}
```

---

## 6. Update AddArtifactWithChunks (internal/store/records.go)

```go
// Replace lines 446-451:
        res, err := tx.Exec(`
            INSERT OR IGNORE INTO chunks (
                chunk_id, repo_id, workspace, artifact_id, thread_id, locator, 
                text, text_hash, text_tokens, tags_json, tags_text,
                chunk_type, symbol_name, symbol_kind, created_at, deleted_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
        `, chunk.ID, chunk.RepoID, chunkWorkspace, chunk.ArtifactID, chunk.ThreadID, 
           chunk.Locator, chunk.Text, textHash, chunk.TextTokens, chunk.TagsJSON, chunk.TagsText,
           nullIfEmpty(chunk.ChunkType), nullIfEmpty(chunk.SymbolName), nullIfEmpty(chunk.SymbolKind),
           chunk.CreatedAt.UTC().Format(time.RFC3339Nano))

// Add helper function at end of file:
func nullIfEmpty(s string) interface{} {
    if s == "" {
        return nil
    }
    return s
}
```

---

## 7. Update schema.sql for reference

```sql
-- Add to chunks table definition (for documentation, migrations handle runtime):
-- chunk_type TEXT,
-- symbol_name TEXT,
-- symbol_kind TEXT,

-- Add index:
CREATE INDEX IF NOT EXISTS idx_chunks_symbol ON chunks (repo_id, workspace, symbol_name) WHERE symbol_name IS NOT NULL AND symbol_name != '';
```

---

## 8. New file: chunker.go (in app package)

```go
package app

import (
    "go/ast"
    "go/parser"
    "go/token"
    "path/filepath"
    "strings"

    memtoken "mempack/internal/token"
)

// SemanticChunk represents a chunk with optional semantic metadata
type SemanticChunk struct {
    Text       string
    StartLine  int
    EndLine    int
    ChunkType  string // "line", "function", "class", "block"
    SymbolName string // e.g., "Store.AddMemory"
    SymbolKind string // "function", "method", "struct", "interface"
}

// chunkFile splits file content into semantic chunks where possible
func chunkFile(path string, content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
    ext := strings.ToLower(filepath.Ext(path))
    
    switch ext {
    case ".go":
        return chunkGo(path, content, maxTokens, overlapTokens, counter)
    default:
        return chunkLines(content, maxTokens, overlapTokens, counter)
    }
}

// chunkGo parses Go source and extracts functions/types as chunks
func chunkGo(path string, content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
    fset := token.NewFileSet()
    f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
    if err != nil {
        // Parse failed, fall back to line chunking
        return chunkLines(content, maxTokens, overlapTokens, counter)
    }

    lines := strings.Split(string(content), "\n")
    var chunks []SemanticChunk

    for _, decl := range f.Decls {
        switch d := decl.(type) {
        case *ast.FuncDecl:
            funcChunks := extractGoFunc(fset, d, lines, counter, maxTokens, overlapTokens)
            chunks = append(chunks, funcChunks...)
        case *ast.GenDecl:
            for _, spec := range d.Specs {
                if ts, ok := spec.(*ast.TypeSpec); ok {
                    typeChunks := extractGoType(fset, d, ts, lines, counter, maxTokens, overlapTokens)
                    chunks = append(chunks, typeChunks...)
                }
            }
        }
    }

    if len(chunks) == 0 {
        return chunkLines(content, maxTokens, overlapTokens, counter)
    }
    return chunks, nil
}

// extractGoFunc extracts a function/method as a chunk, splitting if too large
func extractGoFunc(fset *token.FileSet, fn *ast.FuncDecl, lines []string, counter *memtoken.Counter, maxTokens, overlapTokens int) []SemanticChunk {
    startPos := fset.Position(fn.Pos())
    endPos := fset.Position(fn.End())
    
    startLine := startPos.Line - 1 // 0-indexed
    endLine := endPos.Line
    if startLine < 0 {
        startLine = 0
    }
    if endLine > len(lines) {
        endLine = len(lines)
    }
    
    text := strings.Join(lines[startLine:endLine], "\n")
    tokens := counter.Count(text)
    
    // Determine symbol kind and name
    symbolKind := "function"
    symbolName := fn.Name.Name
    if fn.Recv != nil && len(fn.Recv.List) > 0 {
        symbolKind = "method"
        // Extract receiver type
        switch t := fn.Recv.List[0].Type.(type) {
        case *ast.StarExpr:
            if ident, ok := t.X.(*ast.Ident); ok {
                symbolName = ident.Name + "." + fn.Name.Name
            }
        case *ast.Ident:
            symbolName = t.Name + "." + fn.Name.Name
        }
    }

    // If within budget, return as single chunk
    if tokens <= maxTokens {
        return []SemanticChunk{{
            Text:       text,
            StartLine:  startLine + 1, // 1-indexed for locator
            EndLine:    endLine,
            ChunkType:  "function",
            SymbolName: symbolName,
            SymbolKind: symbolKind,
        }}
    }

    // Oversized: split into blocks but preserve symbol metadata
    return splitWithMetadata(lines[startLine:endLine], startLine+1, symbolName, symbolKind, counter, maxTokens, overlapTokens)
}

// extractGoType extracts a type declaration as a chunk
func extractGoType(fset *token.FileSet, gen *ast.GenDecl, spec *ast.TypeSpec, lines []string, counter *memtoken.Counter, maxTokens, overlapTokens int) []SemanticChunk {
    startPos := fset.Position(gen.Pos())
    endPos := fset.Position(gen.End())
    
    startLine := startPos.Line - 1
    endLine := endPos.Line
    if startLine < 0 {
        startLine = 0
    }
    if endLine > len(lines) {
        endLine = len(lines)
    }
    
    text := strings.Join(lines[startLine:endLine], "\n")
    tokens := counter.Count(text)
    
    symbolKind := "type"
    switch spec.Type.(type) {
    case *ast.StructType:
        symbolKind = "struct"
    case *ast.InterfaceType:
        symbolKind = "interface"
    }
    symbolName := spec.Name.Name

    if tokens <= maxTokens {
        return []SemanticChunk{{
            Text:       text,
            StartLine:  startLine + 1,
            EndLine:    endLine,
            ChunkType:  "class",
            SymbolName: symbolName,
            SymbolKind: symbolKind,
        }}
    }

    return splitWithMetadata(lines[startLine:endLine], startLine+1, symbolName, symbolKind, counter, maxTokens, overlapTokens)
}

// splitWithMetadata splits oversized content into chunks while preserving semantic info
func splitWithMetadata(lines []string, baseLineNum int, symbolName, symbolKind string, counter *memtoken.Counter, maxTokens, overlapTokens int) []SemanticChunk {
    var chunks []SemanticChunk
    var buf []string
    bufTokens := 0
    bufStart := baseLineNum

    for i, line := range lines {
        lineTokens := counter.Count(line)
        
        // If adding this line would exceed budget and we have content, flush
        if bufTokens > 0 && bufTokens+lineTokens > maxTokens {
            chunks = append(chunks, SemanticChunk{
                Text:       strings.Join(buf, "\n"),
                StartLine:  bufStart,
                EndLine:    baseLineNum + i - 1,
                ChunkType:  "block",
                SymbolName: symbolName,
                SymbolKind: symbolKind,
            })
            
            // Calculate overlap: keep last N tokens worth of lines
            overlapLines := 0
            overlapCount := 0
            for j := len(buf) - 1; j >= 0 && overlapCount < overlapTokens; j-- {
                overlapCount += counter.Count(buf[j])
                overlapLines++
            }
            
            if overlapLines > 0 && overlapLines < len(buf) {
                buf = buf[len(buf)-overlapLines:]
                bufTokens = overlapCount
                bufStart = baseLineNum + i - overlapLines
            } else {
                buf = nil
                bufTokens = 0
                bufStart = baseLineNum + i
            }
        }
        
        buf = append(buf, line)
        bufTokens += lineTokens
    }

    // Flush remaining
    if len(buf) > 0 {
        chunks = append(chunks, SemanticChunk{
            Text:       strings.Join(buf, "\n"),
            StartLine:  bufStart,
            EndLine:    baseLineNum + len(lines) - 1,
            ChunkType:  "block",
            SymbolName: symbolName,
            SymbolKind: symbolKind,
        })
    }

    return chunks
}

// chunkLines is the fallback line-based chunker (existing logic adapted)
func chunkLines(content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
    lines := strings.Split(string(content), "\n")
    lineTokens := make([]int, len(lines))
    for i, line := range lines {
        lineTokens[i] = counter.Count(line)
    }

    ranges := chunkRanges(lines, lineTokens, maxTokens, overlapTokens)
    var chunks []SemanticChunk
    for _, r := range ranges {
        text := strings.Join(lines[r.Start:r.End], "\n")
        chunks = append(chunks, SemanticChunk{
            Text:      text,
            StartLine: r.Start + 1, // 1-indexed
            EndLine:   r.End,
            ChunkType: "line",
            // SymbolName and SymbolKind left empty for line chunks
        })
    }
    return chunks, nil
}
```

---

## 9. Update ingest.go to use the chunker

```go
// Replace the chunking section in processFile (around lines 154-196):

    // Use semantic chunking
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
        // Preserve existing locator format: git:<commit>:<path>#L<start>-L<end>
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
```

---

## 10. Verification checklist

After applying these changes:

1. **Build**: `go build ./...` should pass
2. **Test existing**: `go test ./...` should pass (existing tests don't touch new columns)
3. **Migration test**: Create a DB with v8 schema, run the app, verify columns exist:
   ```sql
   PRAGMA table_info(chunks);  -- should show chunk_type, symbol_name, symbol_kind
   PRAGMA user_version;        -- should be 9
   ```
4. **Ingest test**: Run `mem ingest-artifact ./app --thread T-test` on a Go package
5. **Verify chunks**: 
   ```sql
   SELECT chunk_id, chunk_type, symbol_name, symbol_kind, locator 
   FROM chunks 
   WHERE symbol_name IS NOT NULL 
   LIMIT 10;
   ```

---

## Key differences from the original plan

| Issue | Original | Corrected |
|-------|----------|-----------|
| token.Counter type | `token.Counter` | `*memtoken.Counter` (pointer) |
| Missing extractGoType | Referenced but undefined | Implemented |
| Overlap tokens | Hardcoded maxTokens/8 | Uses `overlapTokens` parameter |
| createdAt in INSERT | Undefined variable | Uses `chunk.CreatedAt.UTC().Format(...)` |
| Empty ChunkType | Would be "" | Uses `nullIfEmpty()` helper |
| SearchChunks SELECT | Not updated | Includes new columns |
| GetChunksByIDs | Not updated | Includes new columns |
| Package location | Ambiguous (app vs internal/chunker) | Consistently in `app` |
| Schema/migration | Incomplete | Full migrate.go changes |


# Mempack Improvements: Consolidated Implementation Plan

This document contains ONE correct implementation for each feature. Earlier drafts had conflicts—this is the authoritative version.

---

## Feature 1: Semantic Code Chunking

**Status: Ready to implement**

### 1.1 Schema Changes

**migrate.go** - Update version and add columns:

```go
// Line 10: bump version
const schemaVersion = 9

// Add to ensureColumns() after existing chunk columns (around line 214):
    if err := ensureColumn(db, "chunks", "chunk_type", "TEXT DEFAULT 'line'"); err != nil {
        return err
    }
    if err := ensureColumn(db, "chunks", "symbol_name", "TEXT"); err != nil {
        return err
    }
    if err := ensureColumn(db, "chunks", "symbol_kind", "TEXT"); err != nil {
        return err
    }

// Add new function:
func ensureChunkSymbolIndex(db *sql.DB) error {
    _, err := db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_chunks_symbol 
        ON chunks (repo_id, workspace, symbol_name) 
        WHERE symbol_name IS NOT NULL AND symbol_name != ''
    `)
    return err
}

// In migrate(), add after ensureLinksIndexes() call (around line 40):
    if err := ensureChunkSymbolIndex(db); err != nil {
        return err
    }
```

**schema.sql** - Add for documentation (migrations handle runtime):

```sql
-- In chunks table, add after tags_text:
--     chunk_type TEXT DEFAULT 'line',
--     symbol_name TEXT,
--     symbol_kind TEXT,

-- Add index:
CREATE INDEX IF NOT EXISTS idx_chunks_symbol ON chunks (repo_id, workspace, symbol_name) WHERE symbol_name IS NOT NULL AND symbol_name != '';
```

### 1.2 Update Chunk Struct

**memory.go** - Replace Chunk struct (lines 394-408):

```go
type Chunk struct {
    ID         string
    RepoID     string
    Workspace  string
    ArtifactID string
    ThreadID   string
    Locator    string
    Text       string
    TextHash   string
    TextTokens int
    TagsJSON   string
    TagsText   string
    ChunkType  string    // "line", "function", "class", "block"
    SymbolName string    // e.g., "Store.AddMemory"
    SymbolKind string    // "function", "method", "struct", "interface"
    CreatedAt  time.Time
    DeletedAt  time.Time
}
```

### 1.3 Update Chunk Queries

**memory.go** - Update searchChunksWithQuery (replace lines 335-376):

```go
    fetchStart := time.Now()
    querySQL := fmt.Sprintf(`
        SELECT rowid, chunk_id, repo_id, workspace, artifact_id, thread_id, locator, 
               text, text_hash, text_tokens, tags_json, tags_text,
               chunk_type, symbol_name, symbol_kind, created_at
        FROM chunks
        WHERE rowid IN (%s)
        AND repo_id = ?
        AND workspace = ?
        AND deleted_at IS NULL
    `, placeholders)
    fetchRows, err := s.db.Query(querySQL, args...)
    if err != nil {
        return nil, stats, err
    }
    defer fetchRows.Close()

    details := make(map[int64]Chunk, len(rowIDs))
    for fetchRows.Next() {
        var rowid int64
        var chunk Chunk
        var createdAt string
        var artifactID, threadID, locator, text, textHash sql.NullString
        var textTokens sql.NullInt64
        var tagsJSON, tagsText sql.NullString
        var chunkType, symbolName, symbolKind sql.NullString
        if err := fetchRows.Scan(&rowid, &chunk.ID, &chunk.RepoID, &chunk.Workspace,
            &artifactID, &threadID, &locator, &text, &textHash, &textTokens,
            &tagsJSON, &tagsText, &chunkType, &symbolName, &symbolKind, &createdAt); err != nil {
            return nil, stats, err
        }
        chunk.ArtifactID = artifactID.String
        chunk.ThreadID = threadID.String
        chunk.Locator = locator.String
        chunk.Text = text.String
        chunk.TextHash = textHash.String
        if textTokens.Valid {
            chunk.TextTokens = int(textTokens.Int64)
        }
        chunk.TagsJSON = tagsJSON.String
        chunk.TagsText = tagsText.String
        chunk.ChunkType = chunkType.String
        chunk.SymbolName = symbolName.String
        chunk.SymbolKind = symbolKind.String
        chunk.CreatedAt = parseTime(createdAt)
        details[rowid] = chunk
    }
    if err := fetchRows.Err(); err != nil {
        return nil, stats, err
    }
    stats.FetchTime = time.Since(fetchStart)
```

**records.go** - Update GetChunk (replace lines 120-159):

```go
func (s *Store) GetChunk(repoID, workspace, id string) (Chunk, error) {
    row := s.db.QueryRow(`
        SELECT chunk_id, repo_id, workspace, artifact_id, thread_id, locator, 
               text, text_hash, text_tokens, tags_json, tags_text,
               chunk_type, symbol_name, symbol_kind, created_at, deleted_at
        FROM chunks
        WHERE repo_id = ? AND workspace = ? AND chunk_id = ?
    `, repoID, normalizeWorkspace(workspace), id)

    var chunk Chunk
    var createdAt string
    var deletedAt, artifactID, threadID, locator, text, textHash sql.NullString
    var textTokens sql.NullInt64
    var tagsJSON, tagsText sql.NullString
    var chunkType, symbolName, symbolKind sql.NullString
    
    if err := row.Scan(&chunk.ID, &chunk.RepoID, &chunk.Workspace, 
        &artifactID, &threadID, &locator, &text, &textHash, &textTokens, 
        &tagsJSON, &tagsText, &chunkType, &symbolName, &symbolKind, 
        &createdAt, &deletedAt); err != nil {
        if errors.Is(err, sql.ErrNoRows) {
            return Chunk{}, ErrNotFound
        }
        return Chunk{}, err
    }
    chunk.ArtifactID = artifactID.String
    chunk.ThreadID = threadID.String
    chunk.Locator = locator.String
    chunk.Text = text.String
    chunk.TextHash = textHash.String
    if textTokens.Valid {
        chunk.TextTokens = int(textTokens.Int64)
    }
    chunk.TagsJSON = tagsJSON.String
    chunk.TagsText = tagsText.String
    chunk.ChunkType = chunkType.String
    chunk.SymbolName = symbolName.String
    chunk.SymbolKind = symbolKind.String
    chunk.CreatedAt = parseTime(createdAt)
    if deletedAt.Valid {
        chunk.DeletedAt = parseTime(deletedAt.String)
    }
    return chunk, nil
}
```

**records.go** - Update GetChunksByIDs (replace lines 222-278):

```go
func (s *Store) GetChunksByIDs(repoID, workspace string, ids []string) ([]Chunk, error) {
    if len(ids) == 0 {
        return nil, nil
    }
    placeholders := strings.Repeat("?,", len(ids))
    placeholders = strings.TrimSuffix(placeholders, ",")
    args := make([]any, 0, len(ids)+2)
    args = append(args, repoID, normalizeWorkspace(workspace))
    for _, id := range ids {
        args = append(args, id)
    }

    rows, err := s.db.Query(fmt.Sprintf(`
        SELECT chunk_id, repo_id, workspace, artifact_id, thread_id, locator, 
               text, text_hash, text_tokens, tags_json, tags_text,
               chunk_type, symbol_name, symbol_kind, created_at, deleted_at
        FROM chunks
        WHERE repo_id = ? AND workspace = ? AND chunk_id IN (%s) AND deleted_at IS NULL
    `, placeholders), args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var chunks []Chunk
    for rows.Next() {
        var chunk Chunk
        var createdAt string
        var deletedAt, artifactID, threadID, locator, text, textHash sql.NullString
        var textTokens sql.NullInt64
        var tagsJSON, tagsText sql.NullString
        var chunkType, symbolName, symbolKind sql.NullString
        
        if err := rows.Scan(&chunk.ID, &chunk.RepoID, &chunk.Workspace, 
            &artifactID, &threadID, &locator, &text, &textHash, &textTokens, 
            &tagsJSON, &tagsText, &chunkType, &symbolName, &symbolKind,
            &createdAt, &deletedAt); err != nil {
            return nil, err
        }
        chunk.ArtifactID = artifactID.String
        chunk.ThreadID = threadID.String
        chunk.Locator = locator.String
        chunk.Text = text.String
        chunk.TextHash = textHash.String
        if textTokens.Valid {
            chunk.TextTokens = int(textTokens.Int64)
        }
        chunk.TagsJSON = tagsJSON.String
        chunk.TagsText = tagsText.String
        chunk.ChunkType = chunkType.String
        chunk.SymbolName = symbolName.String
        chunk.SymbolKind = symbolKind.String
        chunk.CreatedAt = parseTime(createdAt)
        if deletedAt.Valid {
            chunk.DeletedAt = parseTime(deletedAt.String)
        }
        chunks = append(chunks, chunk)
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }
    return chunks, nil
}
```

**records.go** - Update AddArtifactWithChunks INSERT (replace lines 446-451):

```go
        // Ensure chunk_type has a value (default to "line" if empty)
        chunkType := chunk.ChunkType
        if chunkType == "" {
            chunkType = "line"
        }
        
        res, err := tx.Exec(`
            INSERT OR IGNORE INTO chunks (
                chunk_id, repo_id, workspace, artifact_id, thread_id, locator, 
                text, text_hash, text_tokens, tags_json, tags_text,
                chunk_type, symbol_name, symbol_kind, created_at, deleted_at
            ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
        `, chunk.ID, chunk.RepoID, chunkWorkspace, chunk.ArtifactID, chunk.ThreadID,
           chunk.Locator, chunk.Text, textHash, chunk.TextTokens, chunk.TagsJSON, chunk.TagsText,
           chunkType, nullIfEmpty(chunk.SymbolName), nullIfEmpty(chunk.SymbolKind),
           chunk.CreatedAt.UTC().Format(time.RFC3339Nano))
```

**records.go** - Add helper at end of file:

```go
func nullIfEmpty(s string) interface{} {
    if s == "" {
        return nil
    }
    return s
}
```

### 1.4 New Chunker (app/chunker.go)

```go
package app

import (
    "go/ast"
    "go/parser"
    "go/token"
    "path/filepath"
    "strings"

    memtoken "mempack/internal/token"
)

// SemanticChunk represents a chunk with optional semantic metadata
type SemanticChunk struct {
    Text       string
    StartLine  int
    EndLine    int
    ChunkType  string // "line", "function", "class", "block"
    SymbolName string // e.g., "Store.AddMemory"
    SymbolKind string // "function", "method", "struct", "interface"
}

// chunkFile splits file content into semantic chunks where possible.
// Uses existing chunkRanges from ingest.go for line-based fallback.
func chunkFile(path string, content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
    ext := strings.ToLower(filepath.Ext(path))

    switch ext {
    case ".go":
        return chunkGo(path, content, maxTokens, overlapTokens, counter)
    default:
        return chunkLines(content, maxTokens, overlapTokens, counter)
    }
}

// chunkGo parses Go source and extracts functions/types as chunks
func chunkGo(path string, content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
    fset := token.NewFileSet()
    f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
    if err != nil {
        // Parse failed, fall back to line chunking
        return chunkLines(content, maxTokens, overlapTokens, counter)
    }

    lines := strings.Split(string(content), "\n")
    var chunks []SemanticChunk
    covered := make(map[int]bool) // track which lines are covered

    for _, decl := range f.Decls {
        switch d := decl.(type) {
        case *ast.FuncDecl:
            funcChunks := extractGoFunc(fset, d, lines, counter, maxTokens, overlapTokens)
            for _, c := range funcChunks {
                for i := c.StartLine; i <= c.EndLine; i++ {
                    covered[i] = true
                }
            }
            chunks = append(chunks, funcChunks...)

        case *ast.GenDecl:
            for _, spec := range d.Specs {
                if ts, ok := spec.(*ast.TypeSpec); ok {
                    typeChunks := extractGoType(fset, ts, lines, counter, maxTokens, overlapTokens)
                    for _, c := range typeChunks {
                        for i := c.StartLine; i <= c.EndLine; i++ {
                            covered[i] = true
                        }
                    }
                    chunks = append(chunks, typeChunks...)
                }
            }
        }
    }

    if len(chunks) == 0 {
        return chunkLines(content, maxTokens, overlapTokens, counter)
    }
    return chunks, nil
}

// extractGoFunc extracts a function/method as a chunk, splitting if too large
func extractGoFunc(fset *token.FileSet, fn *ast.FuncDecl, lines []string, counter *memtoken.Counter, maxTokens, overlapTokens int) []SemanticChunk {
    startPos := fset.Position(fn.Pos())
    endPos := fset.Position(fn.End())

    startLine := startPos.Line - 1 // 0-indexed
    endLine := endPos.Line
    if startLine < 0 {
        startLine = 0
    }
    if endLine > len(lines) {
        endLine = len(lines)
    }

    text := strings.Join(lines[startLine:endLine], "\n")
    tokens := counter.Count(text)

    // Determine symbol kind and name
    symbolKind := "function"
    symbolName := fn.Name.Name
    if fn.Recv != nil && len(fn.Recv.List) > 0 {
        symbolKind = "method"
        switch t := fn.Recv.List[0].Type.(type) {
        case *ast.StarExpr:
            if ident, ok := t.X.(*ast.Ident); ok {
                symbolName = ident.Name + "." + fn.Name.Name
            }
        case *ast.Ident:
            symbolName = t.Name + "." + fn.Name.Name
        }
    }

    // If within budget, return as single chunk
    if tokens <= maxTokens {
        return []SemanticChunk{{
            Text:       text,
            StartLine:  startLine + 1, // 1-indexed for locator
            EndLine:    endLine,
            ChunkType:  "function",
            SymbolName: symbolName,
            SymbolKind: symbolKind,
        }}
    }

    // Oversized: split into blocks but preserve symbol metadata
    return splitWithMetadata(lines[startLine:endLine], startLine+1, symbolName, symbolKind, counter, maxTokens, overlapTokens)
}

// extractGoType extracts a single type declaration as a chunk.
// Uses spec.Pos()/spec.End() to scope to just this type, not the whole GenDecl.
func extractGoType(fset *token.FileSet, spec *ast.TypeSpec, lines []string, counter *memtoken.Counter, maxTokens, overlapTokens int) []SemanticChunk {
    startPos := fset.Position(spec.Pos())
    endPos := fset.Position(spec.End())

    startLine := startPos.Line - 1
    endLine := endPos.Line
    if startLine < 0 {
        startLine = 0
    }
    if endLine > len(lines) {
        endLine = len(lines)
    }

    text := strings.Join(lines[startLine:endLine], "\n")
    tokens := counter.Count(text)

    symbolKind := "type"
    switch spec.Type.(type) {
    case *ast.StructType:
        symbolKind = "struct"
    case *ast.InterfaceType:
        symbolKind = "interface"
    }
    symbolName := spec.Name.Name

    if tokens <= maxTokens {
        return []SemanticChunk{{
            Text:       text,
            StartLine:  startLine + 1,
            EndLine:    endLine,
            ChunkType:  "class",
            SymbolName: symbolName,
            SymbolKind: symbolKind,
        }}
    }

    return splitWithMetadata(lines[startLine:endLine], startLine+1, symbolName, symbolKind, counter, maxTokens, overlapTokens)
}

// splitWithMetadata splits oversized content into chunks while preserving semantic info
func splitWithMetadata(lines []string, baseLineNum int, symbolName, symbolKind string, counter *memtoken.Counter, maxTokens, overlapTokens int) []SemanticChunk {
    var chunks []SemanticChunk
    var buf []string
    bufTokens := 0
    bufStart := baseLineNum

    for i, line := range lines {
        lineTokens := counter.Count(line)

        // If adding this line would exceed budget and we have content, flush
        if bufTokens > 0 && bufTokens+lineTokens > maxTokens {
            chunks = append(chunks, SemanticChunk{
                Text:       strings.Join(buf, "\n"),
                StartLine:  bufStart,
                EndLine:    baseLineNum + i - 1,
                ChunkType:  "block",
                SymbolName: symbolName,
                SymbolKind: symbolKind,
            })

            // Calculate overlap: keep last N tokens worth of lines
            overlapLines := 0
            overlapCount := 0
            for j := len(buf) - 1; j >= 0 && overlapCount < overlapTokens; j-- {
                overlapCount += counter.Count(buf[j])
                overlapLines++
            }

            if overlapLines > 0 && overlapLines < len(buf) {
                buf = buf[len(buf)-overlapLines:]
                bufTokens = overlapCount
                bufStart = baseLineNum + i - overlapLines
            } else {
                buf = nil
                bufTokens = 0
                bufStart = baseLineNum + i
            }
        }

        buf = append(buf, line)
        bufTokens += lineTokens
    }

    // Flush remaining
    if len(buf) > 0 {
        chunks = append(chunks, SemanticChunk{
            Text:       strings.Join(buf, "\n"),
            StartLine:  bufStart,
            EndLine:    baseLineNum + len(lines) - 1,
            ChunkType:  "block",
            SymbolName: symbolName,
            SymbolKind: symbolKind,
        })
    }

    return chunks
}

// chunkLines is the fallback line-based chunker using existing chunkRanges
func chunkLines(content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
    lines := strings.Split(string(content), "\n")
    lineTokens := make([]int, len(lines))
    for i, line := range lines {
        lineTokens[i] = counter.Count(line)
    }

    // chunkRanges is defined in ingest.go (same package)
    ranges := chunkRanges(lines, lineTokens, maxTokens, overlapTokens)
    var chunks []SemanticChunk
    for _, r := range ranges {
        text := strings.Join(lines[r.Start:r.End], "\n")
        chunks = append(chunks, SemanticChunk{
            Text:      text,
            StartLine: r.Start + 1, // 1-indexed
            EndLine:   r.End,
            ChunkType: "line",
            // SymbolName and SymbolKind left empty for line chunks
        })
    }
    return chunks, nil
}
```

### 1.5 Update ingest.go

Replace the chunking section in `processFile` (around lines 154-196):

```go
    // Use semantic chunking
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
        // Preserve existing locator format: git:<commit>:<path>#L<start>-L<end>
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
```

### 1.6 Verification

```bash
# 1. Build
go build ./...

# 2. Run tests
go test ./...

# 3. Check migration (on existing DB)
sqlite3 ~/.local/share/mempack/repos/*/memory.db "PRAGMA user_version; PRAGMA table_info(chunks);"
# Should show: user_version=9, and chunk_type/symbol_name/symbol_kind columns

# 4. Test ingest
mem ingest-artifact ./app --thread T-test

# 5. Verify chunks have semantic data
sqlite3 ~/.local/share/mempack/repos/*/memory.db \
  "SELECT chunk_id, chunk_type, symbol_name, symbol_kind FROM chunks WHERE symbol_name IS NOT NULL LIMIT 5;"
```

---

## Feature 2: Auto-Context on MCP Initialize

**Status: Needs design work**

### Issues to resolve before implementation:
1. MCP currently exposes no resources—need to implement `list_resources` handler
2. `sessionState` as global won't work with multiple clients
3. Need to define client expectations (do they auto-read resources on connect?)
4. Alternative: Add a `mempack.get_initial_context` tool that's cheaper to call

### Recommended approach:
Add `--preload-context` flag that prints initial context to stderr on startup (for logging) and makes first `get_context` call faster by pre-warming caches. Defer resource API until MCP client support is clearer.

---

## Feature 3: File Watch Mode

**Status: Needs additional methods**

### Missing pieces:
1. `Store.DeleteChunksBySource()` method needed
2. Path normalization for cross-platform support
3. `.gitignore` stacking (currently only root is honored)
4. Signal handling for graceful shutdown

### Store method to add (records.go):

```go
func (s *Store) DeleteChunksBySource(repoID, workspace, source string) (int, error) {
    now := time.Now().UTC().Format(time.RFC3339Nano)
    
    result, err := s.db.Exec(`
        UPDATE chunks 
        SET deleted_at = ?
        WHERE repo_id = ? 
          AND workspace = ? 
          AND artifact_id IN (
              SELECT artifact_id FROM artifacts 
              WHERE repo_id = ? AND workspace = ? AND source = ?
          )
          AND deleted_at IS NULL
    `, now, repoID, normalizeWorkspace(workspace), repoID, normalizeWorkspace(workspace), source)
    if err != nil {
        return 0, err
    }
    
    affected, _ := result.RowsAffected()
    return int(affected), nil
}
```

Full implementation deferred until design decisions are made.

---

## Feature 4: Query Understanding

**Status: Needs integration plan**

### Issues:
1. Current behavior uses AND+NEAR+rewrite; new plan uses OR/prefix—breaking change
2. `RecencyMultiplier` doesn't exist in `RankOptions`
3. Need to decide: augment existing sanitizeQuery or replace it?

### Recommended approach:
Add query parsing as an optional enhancement that augments (not replaces) the existing FTS query. Add `RecencyMultiplier` to `RankOptions` with default 1.0.

---

## Feature 5: Memory Clustering

**Status: Sketch only**

### Missing pieces:
1. New tables: `memory_clusters`, `memory_cluster_members`
2. `pack.MemoryItem.IsCluster` and `ClusterSize` fields
3. Prompt rendering changes for clusters
4. Summarization strategy (LLM vs deterministic)
5. CLI command and/or background job

Not ready for implementation without further design.

---

## Feature 6: Staleness Detection

**Status: Sketch only**

### Missing pieces:
1. `pack.ContextPack.Warnings` field
2. Prompt rendering for warnings
3. JSON serialization updates
4. Test coverage

Lower priority—defer.

---

## Feature 7: Importance Decay

**Status: Sketch only**

### Issues:
1. SQL formula has math errors and uses `LOG()` which may not exist in all SQLite builds
2. New columns needed: `importance_score`, `last_accessed_at`, `access_count`
3. Ranking changes needed

Lower priority—defer.

---

## Feature 8: Export/Import

**Status: Sketch only**

### Missing pieces:
1. `ListAllMemories()` method doesn't exist
2. Ignores threads/artifacts/chunks/state integrity
3. Needs transaction handling for imports
4. Conflict resolution strategy incomplete

Lower priority—defer.

---

## Implementation Order

| Priority | Feature | Status | Effort |
|----------|---------|--------|--------|
| 1 | Semantic Chunking | Ready | 1-2 days |
| 2 | Watch Mode | Needs store method | 1 day |
| 3 | Auto-Context | Needs design | 2-3 days |
| 4 | Query Understanding | Needs integration plan | 2-3 days |
| 5 | Staleness Detection | Sketch | 1 day |
| 6 | Memory Clustering | Sketch | 1 week |
| 7 | Importance Decay | Sketch | 2-3 days |
| 8 | Export/Import | Sketch | 2-3 days |

**Recommended Sprint 1:** Semantic Chunking only (it's the only feature ready to implement correctly).