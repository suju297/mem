# Mempack Improvements: Implementation Plan

**Version:** 1.0 (consolidated)  
**Status:** This is the single authoritative plan. All previous drafts are superseded.

---

## Feature 1: Semantic Code Chunking

**Status:** Ready to implement  
**Effort:** 1-2 days

### 1.1 Schema Changes

**internal/store/migrate.go:**

```go
// Line 10: bump version
const schemaVersion = 9

// In ensureColumns(), add after line 240 (after existing chunk columns):
    if err := ensureColumn(db, "chunks", "chunk_type", "TEXT DEFAULT 'line'"); err != nil {
        return err
    }
    if err := ensureColumn(db, "chunks", "symbol_name", "TEXT"); err != nil {
        return err
    }
    if err := ensureColumn(db, "chunks", "symbol_kind", "TEXT"); err != nil {
        return err
    }

// In migrate(), add after ensureLinksIndexes() call (around line 40):
    if err := ensureChunkSymbolIndex(db); err != nil {
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

// Optional: backfill existing rows to make chunk_type explicit
func backfillChunkType(db *sql.DB) error {
    _, err := db.Exec(`UPDATE chunks SET chunk_type = 'line' WHERE chunk_type IS NULL OR chunk_type = ''`)
    return err
}
```

Call `backfillChunkType(db)` in `migrate()` after `ensureChunkSymbolIndex()` if you want existing rows explicitly set.

**Note on schema.sql:** `schema.sql` is executed on every open, so avoid adding indexes that reference columns only added by migrations (it will fail on older DBs before `ensureColumns()` runs). Keep `idx_chunks_symbol` creation in `ensureChunkSymbolIndex()` only. If you want schema.sql to reflect the v9 schema for documentation purposes, add only the columns:

```sql
-- In chunks table definition, after tags_text:
    chunk_type TEXT DEFAULT 'line',
    symbol_name TEXT,
    symbol_kind TEXT,

```

### 1.2 Update Chunk Struct

**internal/store/memory.go** - Replace lines 394-408:

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

### 1.3 Update searchChunksWithQuery

**internal/store/memory.go** - Replace lines 335-376:

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

### 1.4 Update GetChunk

**internal/store/records.go** - Replace lines 120-159:

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

### 1.5 Update GetChunksByIDs

**internal/store/records.go** - Replace lines 222-278:

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

### 1.6 Update AddArtifactWithChunks

**internal/store/records.go** - Replace lines 446-451:

```go
        // Ensure chunk_type has a value (use default if empty)
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

**internal/store/records.go** - Add helper at end of file:

```go
func nullIfEmpty(s string) interface{} {
    if s == "" {
        return nil
    }
    return s
}
```

### 1.7 New File: app/chunker.go

Create new file `chunker.go` in the `app` package (same directory as `ingest.go`):

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

// SemanticChunk represents a chunk with optional semantic metadata.
type SemanticChunk struct {
    Text       string
    StartLine  int    // 1-indexed
    EndLine    int    // 1-indexed, inclusive
    ChunkType  string // "line", "function", "class", "block"
    SymbolName string // e.g., "Store.AddMemory"
    SymbolKind string // "function", "method", "struct", "interface"
}

// chunkFile splits file content into semantic chunks where possible.
// Falls back to line-based chunking for unsupported languages.
func chunkFile(path string, content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
    ext := strings.ToLower(filepath.Ext(path))

    switch ext {
    case ".go":
        chunks, err := chunkGo(path, content, maxTokens, overlapTokens, counter)
        if err != nil || len(chunks) == 0 {
            return chunkLinesSemanticWrap(content, maxTokens, overlapTokens, counter)
        }
        return chunks, nil
    default:
        return chunkLinesSemanticWrap(content, maxTokens, overlapTokens, counter)
    }
}

// chunkGo parses Go source and extracts functions/types as chunks.
func chunkGo(path string, content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
    fset := token.NewFileSet()
    f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
    if err != nil {
        return nil, err // Caller will fall back to line chunking
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
                    // Use spec boundaries, not gen boundaries
                    typeChunks := extractGoType(fset, ts, lines, counter, maxTokens, overlapTokens)
                    chunks = append(chunks, typeChunks...)
                }
            }
        }
    }

    return chunks, nil
}

// extractGoFunc extracts a function/method as chunk(s).
func extractGoFunc(fset *token.FileSet, fn *ast.FuncDecl, lines []string, counter *memtoken.Counter, maxTokens, overlapTokens int) []SemanticChunk {
    startPos := fset.Position(fn.Pos())
    endPos := fset.Position(fn.End())

    startLine := startPos.Line - 1 // Convert to 0-indexed
    endLine := endPos.Line         // End is exclusive for slice
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
            StartLine:  startLine + 1, // Convert back to 1-indexed
            EndLine:    endLine,
            ChunkType:  "function",
            SymbolName: symbolName,
            SymbolKind: symbolKind,
        }}
    }

    // Oversized: split into blocks preserving metadata
    return splitWithMetadata(lines[startLine:endLine], startLine+1, symbolName, symbolKind, counter, maxTokens, overlapTokens)
}

// extractGoType extracts a single type spec as chunk(s).
// Uses spec.Pos()/spec.End() to scope to just this type.
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

// splitWithMetadata splits oversized content into chunks while preserving semantic info.
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

// chunkLinesSemanticWrap wraps the existing chunkRanges into SemanticChunk format.
// chunkRanges is defined in ingest.go (same package).
func chunkLinesSemanticWrap(content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
    lines := strings.Split(string(content), "\n")
    lineTokens := make([]int, len(lines))
    for i, line := range lines {
        lineTokens[i] = counter.Count(line)
    }

    ranges := chunkRanges(lines, lineTokens, maxTokens, overlapTokens)
    chunks := make([]SemanticChunk, 0, len(ranges))
    for _, r := range ranges {
        text := strings.Join(lines[r.Start:r.End], "\n")
        chunks = append(chunks, SemanticChunk{
            Text:      text,
            StartLine: r.Start + 1, // 1-indexed
            EndLine:   r.End,
            ChunkType: "line",
            // SymbolName and SymbolKind empty for line chunks
        })
    }
    return chunks, nil
}
```

### 1.8 Update ingest.go

Replace chunking section in `processFile` (around lines 154-196):

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

### 1.9 Verification

```bash
go build ./...
go test ./...

# Check migration
sqlite3 ~/.local/share/mempack/repos/*/memory.db \
  "PRAGMA user_version; PRAGMA table_info(chunks);"

# Test ingest
mem ingest-artifact ./app --thread T-test

# Verify semantic data
sqlite3 ~/.local/share/mempack/repos/*/memory.db \
  "SELECT chunk_type, symbol_name, symbol_kind FROM chunks WHERE symbol_name IS NOT NULL LIMIT 5;"
```

### 1.10 FTS Scope Note

The `idx_chunks_symbol` index enables direct lookups by symbol name (e.g., `WHERE symbol_name = 'Store.AddMemory'`). However, **symbol_name is NOT searchable via FTS** with this implementation.

If you want symbol names searchable via the existing FTS queries (e.g., searching "AddMemory" finds the chunk), you would need to:
1. Add `symbol_name` column to `chunks_fts` virtual table in `recreateFTSTables()`
2. Update triggers in `triggers.sql` to include `symbol_name` in INSERT/UPDATE
3. Update `rebuildFTS()` to populate the new column

This is optional—the current implementation stores the metadata for display/filtering but doesn't make it FTS-searchable.

---

## Other Features (Not Ready)

| Feature | Blocker |
|---------|---------|
| Auto-Context | MCP resources not implemented; needs design |
| Watch Mode | Needs `DeleteChunksBySource` method; path normalization |
| Query Understanding | Would break existing AND+NEAR behavior; needs integration plan |
| Memory Clustering | New tables, pack fields, summarization strategy TBD |
| Staleness Detection | Missing ContextPack.Warnings, prompt rendering |
| Importance Decay | SQL math errors, missing columns |
| Export/Import | Missing `ListAllMemories`, transaction handling |

These require further design before implementation.
