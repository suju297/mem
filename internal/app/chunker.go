package app

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"unicode"

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
	case ".py":
		chunks, err := chunkPython(content, maxTokens, overlapTokens, counter)
		if err != nil || len(chunks) == 0 {
			return chunkLinesSemanticWrap(content, maxTokens, overlapTokens, counter)
		}
		return chunks, nil
	case ".js", ".jsx", ".ts", ".tsx", ".mts", ".cts":
		chunks, err := chunkTypeScript(content, maxTokens, overlapTokens, counter)
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

type pythonDecl struct {
	StartLine  int
	EndLine    int
	ChunkType  string
	SymbolName string
	SymbolKind string
}

func chunkPython(content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
	lines := strings.Split(string(content), "\n")
	decls := collectPythonDecls(lines)
	chunks := make([]SemanticChunk, 0, len(decls))

	for _, decl := range decls {
		if decl.StartLine < 0 || decl.StartLine >= len(lines) {
			continue
		}
		if decl.EndLine <= decl.StartLine || decl.EndLine > len(lines) {
			continue
		}

		declLines := lines[decl.StartLine:decl.EndLine]
		text := strings.Join(declLines, "\n")
		if strings.TrimSpace(text) == "" {
			continue
		}

		tokens := counter.Count(text)
		if tokens <= maxTokens {
			chunks = append(chunks, SemanticChunk{
				Text:       text,
				StartLine:  decl.StartLine + 1,
				EndLine:    decl.EndLine,
				ChunkType:  decl.ChunkType,
				SymbolName: decl.SymbolName,
				SymbolKind: decl.SymbolKind,
			})
			continue
		}

		chunks = append(chunks, splitWithMetadata(declLines, decl.StartLine+1, decl.SymbolName, decl.SymbolKind, counter, maxTokens, overlapTokens)...)
	}

	return chunks, nil
}

func collectPythonDecls(lines []string) []pythonDecl {
	decls := make([]pythonDecl, 0)

	for i := 0; i < len(lines); {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || leadingIndentWidth(lines[i]) != 0 {
			i++
			continue
		}

		startLine := i
		headerLine := i
		symbolKind := ""
		symbolName := ""
		chunkType := ""
		ok := false

		if strings.HasPrefix(trimmed, "@") {
			var found bool
			headerLine, symbolKind, symbolName, chunkType, found = findPythonDecoratedHeader(lines, i)
			if !found {
				i++
				continue
			}
			ok = true
		} else {
			symbolKind, symbolName, chunkType, ok = parsePythonDeclHeader(strings.TrimSpace(lines[headerLine]))
		}

		if !ok {
			i++
			continue
		}

		endLine := findPythonDeclEnd(lines, headerLine)
		if endLine <= headerLine {
			endLine = headerLine + 1
		}
		if endLine > len(lines) {
			endLine = len(lines)
		}

		decls = append(decls, pythonDecl{
			StartLine:  startLine,
			EndLine:    endLine,
			ChunkType:  chunkType,
			SymbolName: symbolName,
			SymbolKind: symbolKind,
		})
		i = endLine
	}

	return decls
}

func findPythonDecoratedHeader(lines []string, startLine int) (headerLine int, symbolKind, symbolName, chunkType string, ok bool) {
	j := startLine
	for j < len(lines) {
		candidate := strings.TrimSpace(lines[j])
		if candidate == "" || strings.HasPrefix(candidate, "#") {
			j++
			continue
		}
		if leadingIndentWidth(lines[j]) != 0 {
			return 0, "", "", "", false
		}
		if !strings.HasPrefix(candidate, "@") {
			break
		}

		next := consumePythonDecorator(lines, j)
		if next <= j {
			return 0, "", "", "", false
		}
		j = next
	}

	for ; j < len(lines); j++ {
		candidate := strings.TrimSpace(lines[j])
		if candidate == "" || strings.HasPrefix(candidate, "#") {
			continue
		}
		if leadingIndentWidth(lines[j]) != 0 {
			return 0, "", "", "", false
		}
		symbolKind, symbolName, chunkType, ok := parsePythonDeclHeader(candidate)
		if ok {
			return j, symbolKind, symbolName, chunkType, true
		}
		return 0, "", "", "", false
	}

	return 0, "", "", "", false
}

func parsePythonDeclHeader(line string) (symbolKind, symbolName, chunkType string, ok bool) {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "async def "):
		name := readIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(line, "async def ")), false)
		if name == "" {
			return "", "", "", false
		}
		return "function", name, "function", true
	case strings.HasPrefix(line, "def "):
		name := readIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(line, "def ")), false)
		if name == "" {
			return "", "", "", false
		}
		return "function", name, "function", true
	case strings.HasPrefix(line, "class "):
		name := readIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(line, "class ")), false)
		if name == "" {
			return "", "", "", false
		}
		return "class", name, "class", true
	default:
		return "", "", "", false
	}
}

func findPythonDeclEnd(lines []string, headerLine int) int {
	if headerLine < 0 || headerLine >= len(lines) {
		return 0
	}

	baseIndent := leadingIndentWidth(lines[headerLine])
	endLine := headerLine + 1
	sawBody := false

	for i := headerLine + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if sawBody {
				endLine = i + 1
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			if sawBody {
				endLine = i + 1
			}
			continue
		}

		indent := leadingIndentWidth(lines[i])
		if indent <= baseIndent {
			break
		}
		sawBody = true
		endLine = i + 1
	}

	return endLine
}

func consumePythonDecorator(lines []string, start int) int {
	if start < 0 || start >= len(lines) {
		return start
	}

	trimmed := strings.TrimSpace(lines[start])
	if !strings.HasPrefix(trimmed, "@") {
		return start
	}

	depth, continued := pythonDecoratorLineState(lines[start])
	i := start + 1

	for i < len(lines) && (depth > 0 || continued) {
		delta, lineContinued := pythonDecoratorLineState(lines[i])
		depth += delta
		if depth < 0 {
			depth = 0
		}
		continued = lineContinued
		i++
	}

	return i
}

func pythonDecoratorLineState(line string) (parenDelta int, continued bool) {
	inSingle := false
	inDouble := false
	escaped := false
	lastNonSpace := byte(0)

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if inSingle {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}

		if inDouble {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}

		if ch == '#' {
			break
		}

		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '(', '[', '{':
			parenDelta++
		case ')', ']', '}':
			parenDelta--
		}

		if !unicode.IsSpace(rune(ch)) {
			lastNonSpace = ch
		}
	}

	return parenDelta, lastNonSpace == '\\'
}

type typeScriptDecl struct {
	StartLine             int
	EndLine               int
	ChunkType             string
	SymbolName            string
	SymbolKind            string
	RequireFunctionMarker bool
}

type typeScriptLineScan struct {
	StartDepth   int
	EndDepth     int
	HasOpenBrace bool
}

func chunkTypeScript(content []byte, maxTokens, overlapTokens int, counter *memtoken.Counter) ([]SemanticChunk, error) {
	lines := strings.Split(string(content), "\n")
	scan := scanTypeScriptLines(lines)
	decls := collectTypeScriptDecls(lines, scan)
	chunks := make([]SemanticChunk, 0, len(decls))

	for _, decl := range decls {
		if decl.StartLine < 0 || decl.StartLine >= len(lines) {
			continue
		}
		if decl.EndLine <= decl.StartLine || decl.EndLine > len(lines) {
			continue
		}

		declLines := lines[decl.StartLine:decl.EndLine]
		text := strings.Join(declLines, "\n")
		if strings.TrimSpace(text) == "" {
			continue
		}
		if decl.RequireFunctionMarker && !looksLikeTypeScriptFunction(text) {
			continue
		}

		tokens := counter.Count(text)
		if tokens <= maxTokens {
			chunks = append(chunks, SemanticChunk{
				Text:       text,
				StartLine:  decl.StartLine + 1,
				EndLine:    decl.EndLine,
				ChunkType:  decl.ChunkType,
				SymbolName: decl.SymbolName,
				SymbolKind: decl.SymbolKind,
			})
			continue
		}

		chunks = append(chunks, splitWithMetadata(declLines, decl.StartLine+1, decl.SymbolName, decl.SymbolKind, counter, maxTokens, overlapTokens)...)
	}

	return chunks, nil
}

func collectTypeScriptDecls(lines []string, scan []typeScriptLineScan) []typeScriptDecl {
	decls := make([]typeScriptDecl, 0)

	for i := 0; i < len(lines); {
		if i >= len(scan) || scan[i].StartDepth != 0 {
			i++
			continue
		}

		trimmed := strings.TrimSpace(stripTypeScriptLineComment(lines[i]))
		if trimmed == "" {
			i++
			continue
		}

		startLine := i
		headerLine := i
		symbolKind := ""
		symbolName := ""
		chunkType := ""
		requireFunctionMarker := false
		ok := false

		if strings.HasPrefix(trimmed, "@") {
			var found bool
			headerLine, symbolKind, symbolName, chunkType, requireFunctionMarker, found = findTypeScriptDecoratedHeader(lines, scan, i)
			if !found {
				i++
				continue
			}
			ok = true
		} else {
			symbolKind, symbolName, chunkType, requireFunctionMarker, ok = parseTypeScriptDeclHeader(strings.TrimSpace(stripTypeScriptLineComment(lines[headerLine])))
		}

		if !ok {
			i++
			continue
		}

		endLine := findTypeScriptDeclEnd(lines, scan, headerLine)
		if endLine <= headerLine {
			endLine = headerLine + 1
		}
		if endLine > len(lines) {
			endLine = len(lines)
		}

		decls = append(decls, typeScriptDecl{
			StartLine:             startLine,
			EndLine:               endLine,
			ChunkType:             chunkType,
			SymbolName:            symbolName,
			SymbolKind:            symbolKind,
			RequireFunctionMarker: requireFunctionMarker,
		})
		i = endLine
	}

	return decls
}

func findTypeScriptDecoratedHeader(lines []string, scan []typeScriptLineScan, startLine int) (headerLine int, symbolKind, symbolName, chunkType string, requireFunctionMarker, ok bool) {
	for j := startLine + 1; j < len(lines); j++ {
		if j >= len(scan) {
			break
		}

		candidate := strings.TrimSpace(stripTypeScriptLineComment(lines[j]))
		if candidate == "" {
			continue
		}
		if scan[j].StartDepth != 0 {
			continue
		}
		if strings.HasPrefix(candidate, "@") {
			continue
		}
		if isTypeScriptDecoratorContinuationLine(candidate) {
			continue
		}

		symbolKind, symbolName, chunkType, requireFunctionMarker, ok := parseTypeScriptDeclHeader(candidate)
		if ok {
			return j, symbolKind, symbolName, chunkType, requireFunctionMarker, true
		}
		break
	}
	return 0, "", "", "", false, false
}

func parseTypeScriptDeclHeader(line string) (symbolKind, symbolName, chunkType string, requireFunctionMarker, ok bool) {
	line = strings.TrimSpace(line)
	line = normalizeTypeScriptDeclPrefixes(line)
	if line == "" {
		return "", "", "", false, false
	}

	switch {
	case strings.HasPrefix(line, "class "):
		name := readIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(line, "class ")), true)
		if name == "" {
			return "", "", "", false, false
		}
		return "class", name, "class", false, true
	case strings.HasPrefix(line, "interface "):
		name := readIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(line, "interface ")), true)
		if name == "" {
			return "", "", "", false, false
		}
		return "interface", name, "interface", false, true
	case strings.HasPrefix(line, "enum "):
		name := readIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(line, "enum ")), true)
		if name == "" {
			return "", "", "", false, false
		}
		return "enum", name, "enum", false, true
	case strings.HasPrefix(line, "type "):
		name := readIdentifierPrefix(strings.TrimSpace(strings.TrimPrefix(line, "type ")), true)
		if name == "" {
			return "", "", "", false, false
		}
		return "type", name, "type", false, true
	case strings.HasPrefix(line, "function "), strings.HasPrefix(line, "function*"):
		rest := strings.TrimSpace(strings.TrimPrefix(line, "function"))
		rest = strings.TrimPrefix(rest, "*")
		name := readIdentifierPrefix(strings.TrimSpace(rest), true)
		if name == "" {
			return "", "", "", false, false
		}
		return "function", name, "function", false, true
	}

	for _, prefix := range []string{"const ", "let ", "var "} {
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		rest := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		name := readIdentifierPrefix(rest, true)
		if name == "" {
			return "", "", "", false, false
		}

		afterName := strings.TrimSpace(strings.TrimPrefix(rest, name))
		if strings.HasPrefix(afterName, "!") {
			afterName = strings.TrimSpace(strings.TrimPrefix(afterName, "!"))
		}
		if strings.HasPrefix(afterName, ":") {
			eqIdx := strings.Index(afterName, "=")
			if eqIdx < 0 {
				return "", "", "", false, false
			}
			afterName = strings.TrimSpace(afterName[eqIdx:])
		}
		if !strings.HasPrefix(afterName, "=") {
			return "", "", "", false, false
		}

		rhs := strings.TrimSpace(strings.TrimPrefix(afterName, "="))
		if strings.HasPrefix(rhs, "function") || strings.HasPrefix(rhs, "async function") || strings.Contains(rhs, "=>") {
			return "function", name, "function", false, true
		}
		if rhs == "" || strings.HasPrefix(rhs, "async ") || strings.HasPrefix(rhs, "(") || strings.HasPrefix(rhs, "<") {
			return "function", name, "function", true, true
		}
		return "", "", "", false, false
	}

	return "", "", "", false, false
}

func findTypeScriptDeclEnd(lines []string, scan []typeScriptLineScan, headerLine int) int {
	if headerLine < 0 || headerLine >= len(lines) || headerLine >= len(scan) {
		return 0
	}

	baseDepth := scan[headerLine].StartDepth
	endLine := headerLine + 1
	sawBody := false

	for i := headerLine; i < len(lines); i++ {
		if i >= len(scan) {
			break
		}
		trimmed := strings.TrimSpace(stripTypeScriptLineComment(lines[i]))

		if scan[i].HasOpenBrace || scan[i].StartDepth > baseDepth || scan[i].EndDepth > baseDepth {
			sawBody = true
		}

		if sawBody {
			endLine = i + 1
			if scan[i].EndDepth == baseDepth {
				return endLine
			}
			continue
		}

		if hasTypeScriptTerminator(trimmed) {
			return i + 1
		}
	}

	return endLine
}

func isTypeScriptDecoratorContinuationLine(line string) bool {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, ")"),
		strings.HasPrefix(line, "]"),
		strings.HasPrefix(line, "}"),
		strings.HasPrefix(line, ","):
		return true
	default:
		return false
	}
}

func scanTypeScriptLines(lines []string) []typeScriptLineScan {
	scan := make([]typeScriptLineScan, len(lines))
	depth := 0

	inBlockComment := false
	inSingle := false
	inDouble := false
	inTemplate := false
	escaped := false

	for i, line := range lines {
		scan[i].StartDepth = depth
		inLineComment := false

		for j := 0; j < len(line); j++ {
			ch := line[j]
			next := byte(0)
			if j+1 < len(line) {
				next = line[j+1]
			}

			if inLineComment {
				continue
			}
			if inBlockComment {
				if ch == '*' && next == '/' {
					inBlockComment = false
					j++
				}
				continue
			}
			if inSingle {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '\'' {
					inSingle = false
				}
				continue
			}
			if inDouble {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					inDouble = false
				}
				continue
			}
			if inTemplate {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '`' {
					inTemplate = false
				}
				continue
			}

			if ch == '/' && next == '/' {
				inLineComment = true
				continue
			}
			if ch == '/' && next == '*' {
				inBlockComment = true
				j++
				continue
			}

			switch ch {
			case '\'':
				inSingle = true
			case '"':
				inDouble = true
			case '`':
				inTemplate = true
			case '{':
				depth++
				scan[i].HasOpenBrace = true
			case '}':
				if depth > 0 {
					depth--
				}
			}
		}

		scan[i].EndDepth = depth
	}

	return scan
}

func normalizeTypeScriptDeclPrefixes(line string) string {
	line = strings.TrimSpace(line)
	for {
		switch {
		case strings.HasPrefix(line, "export "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		case strings.HasPrefix(line, "default "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "default "))
		case strings.HasPrefix(line, "declare "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "declare "))
		case strings.HasPrefix(line, "abstract "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "abstract "))
		case strings.HasPrefix(line, "async "):
			line = strings.TrimSpace(strings.TrimPrefix(line, "async "))
		default:
			return line
		}
	}
}

func stripTypeScriptLineComment(line string) string {
	inSingle := false
	inDouble := false
	inTemplate := false
	escaped := false

	for i := 0; i < len(line); i++ {
		ch := line[i]
		next := byte(0)
		if i+1 < len(line) {
			next = line[i+1]
		}

		if inSingle {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '\'' {
				inSingle = false
			}
			continue
		}

		if inDouble {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inDouble = false
			}
			continue
		}

		if inTemplate {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '`' {
				inTemplate = false
			}
			continue
		}

		if ch == '/' && next == '/' {
			return line[:i]
		}

		switch ch {
		case '\'':
			inSingle = true
		case '"':
			inDouble = true
		case '`':
			inTemplate = true
		}
	}

	return line
}

func hasTypeScriptTerminator(line string) bool {
	return strings.Contains(line, ";")
}

func looksLikeTypeScriptFunction(text string) bool {
	return strings.Contains(text, "=>") || strings.Contains(text, "function")
}

func leadingIndentWidth(line string) int {
	width := 0
	for _, r := range line {
		switch r {
		case ' ':
			width++
		case '\t':
			width += 4
		default:
			return width
		}
	}
	return width
}

func readIdentifierPrefix(raw string, allowDollar bool) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	started := false
	var b strings.Builder
	for _, r := range s {
		if !started {
			if !isIdentifierStart(r, allowDollar) {
				return ""
			}
			started = true
			b.WriteRune(r)
			continue
		}
		if !isIdentifierPart(r, allowDollar) {
			break
		}
		b.WriteRune(r)
	}

	return b.String()
}

func isIdentifierStart(r rune, allowDollar bool) bool {
	if r == '_' || unicode.IsLetter(r) {
		return true
	}
	return allowDollar && r == '$'
}

func isIdentifierPart(r rune, allowDollar bool) bool {
	return isIdentifierStart(r, allowDollar) || unicode.IsDigit(r)
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
