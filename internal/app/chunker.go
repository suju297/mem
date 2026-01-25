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
	StartLine  int
	EndLine    int
	ChunkType  string
	SymbolName string
	SymbolKind string
}

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

func chunkGo(path string, content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return nil, err
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
					typeChunks := extractGoType(fset, ts, lines, counter, maxTokens, overlapTokens)
					chunks = append(chunks, typeChunks...)
				}
			}
		}
	}

	return chunks, nil
}

func extractGoFunc(fset *token.FileSet, fn *ast.FuncDecl, lines []string, counter *memtoken.Counter, maxTokens, overlapTokens int) []SemanticChunk {
	startPos := fset.Position(fn.Pos())
	endPos := fset.Position(fn.End())

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

	return splitWithMetadata(lines[startLine:endLine], startLine+1, symbolName, symbolKind, counter, maxTokens, overlapTokens)
}

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

func splitWithMetadata(lines []string, baseLineNum int, symbolName, symbolKind string, counter *memtoken.Counter, maxTokens, overlapTokens int) []SemanticChunk {
	var chunks []SemanticChunk
	var buf []string
	bufTokens := 0
	bufStart := baseLineNum

	for i, line := range lines {
		lineTokens := counter.Count(line)

		if bufTokens > 0 && bufTokens+lineTokens > maxTokens {
			chunks = append(chunks, SemanticChunk{
				Text:       strings.Join(buf, "\n"),
				StartLine:  bufStart,
				EndLine:    baseLineNum + i - 1,
				ChunkType:  "block",
				SymbolName: symbolName,
				SymbolKind: symbolKind,
			})

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
			StartLine: r.Start + 1,
			EndLine:   r.End,
			ChunkType: "line",
		})
	}
	return chunks, nil
}
