package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

type Memory struct {
	ID            string
	RepoID        string
	Workspace     string
	ThreadID      string
	Title         string
	Summary       string
	SummaryTokens int
	TagsJSON      string
	TagsText      string
	EntitiesJSON  string
	EntitiesText  string
	CreatedAt     time.Time
	AnchorCommit  string
	SupersededBy  string
	DeletedAt     time.Time
}

type MemoryResult struct {
	Memory
	BM25 float64
}

type SearchStats struct {
	CandidateTime  time.Duration
	FetchTime      time.Duration
	CandidateCount int
	ResultCount    int
	BaselineCount  int
	SanitizedQuery string
	Rewritten      bool
	Rewrites       []string
	RewriteMatched bool
}

type AddMemoryInput struct {
	ID            string
	RepoID        string
	Workspace     string
	ThreadID      string
	Title         string
	Summary       string
	SummaryTokens int
	TagsJSON      string
	TagsText      string
	EntitiesJSON  string
	EntitiesText  string
	AnchorCommit  string
	CreatedAt     time.Time
}

type UpdateMemoryInput struct {
	RepoID         string
	Workspace      string
	ID             string
	Title          *string
	Summary        *string
	SummaryTokens  *int
	TagsSet        bool
	Tags           []string
	TagsAdd        []string
	TagsRemove     []string
	EntitiesSet    bool
	Entities       []string
	EntitiesAdd    []string
	EntitiesRemove []string
}

func (s *Store) AddMemory(input AddMemoryInput) (Memory, error) {
	createdAt := input.CreatedAt.UTC().Format(time.RFC3339Nano)
	id := input.ID
	if id == "" {
		id = NewID("M")
	}
	workspace := normalizeWorkspace(input.Workspace)

	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO threads (thread_id, repo_id, workspace, title, tags_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, input.ThreadID, input.RepoID, workspace, input.ThreadID, "[]", createdAt)
	if err != nil {
		return Memory{}, err
	}

	_, err = s.db.Exec(`
		INSERT INTO memories (
			id, repo_id, workspace, thread_id, title, summary, summary_tokens, tags_json, tags_text, entities_json, entities_text,
			created_at, anchor_commit, superseded_by, deleted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)
	`, id, input.RepoID, workspace, input.ThreadID, input.Title, input.Summary, input.SummaryTokens, input.TagsJSON, input.TagsText, input.EntitiesJSON, input.EntitiesText, createdAt, input.AnchorCommit)
	if err != nil {
		return Memory{}, err
	}

	return Memory{
		ID:            id,
		RepoID:        input.RepoID,
		Workspace:     workspace,
		ThreadID:      input.ThreadID,
		Title:         input.Title,
		Summary:       input.Summary,
		SummaryTokens: input.SummaryTokens,
		TagsJSON:      input.TagsJSON,
		TagsText:      input.TagsText,
		EntitiesJSON:  input.EntitiesJSON,
		EntitiesText:  input.EntitiesText,
		CreatedAt:     input.CreatedAt.UTC(),
		AnchorCommit:  input.AnchorCommit,
	}, nil
}

func (s *Store) SearchMemories(repoID, workspace, query string, limit int) ([]MemoryResult, SearchStats, error) {
	if limit <= 0 {
		return nil, SearchStats{}, nil
	}
	workspace = normalizeWorkspace(workspace)
	parsed := ParseQuery(query)
	baseQuery, _ := buildQueryFromParsed(parsed, false)
	baseResults, baseStats, err := s.searchMemoriesWithQuery(repoID, workspace, baseQuery, limit)
	if err != nil {
		return nil, SearchStats{}, err
	}
	if baseStats.ResultCount > 0 {
		return baseResults, baseStats, nil
	}

	expandedQuery, rewriteMeta := buildQueryFromParsed(parsed, true)
	if !rewriteMeta.Rewritten || expandedQuery == baseQuery {
		return baseResults, baseStats, nil
	}

	expandedResults, expandedStats, err := s.searchMemoriesWithQuery(repoID, workspace, expandedQuery, limit)
	if err != nil {
		return nil, SearchStats{}, err
	}
	expandedStats.BaselineCount = baseStats.ResultCount
	expandedStats.Rewritten = rewriteMeta.Rewritten
	expandedStats.Rewrites = rewriteMeta.Rewrites
	expandedStats.RewriteMatched = baseStats.ResultCount == 0 && expandedStats.ResultCount > 0
	return expandedResults, expandedStats, nil
}

func (s *Store) searchMemoriesWithQuery(repoID, workspace, query string, limit int) ([]MemoryResult, SearchStats, error) {
	candidateLimit := limit
	if candidateLimit < 200 {
		candidateLimit = 200
	}

	candidateStart := time.Now()
	rows, err := s.db.Query(`
		SELECT rowid, bm25(memories_fts, 5.0, 3.0, 2.0, 2.0, 0.0, 0.0, 0.0)
		FROM memories_fts
		WHERE memories_fts MATCH ?
		AND repo_id = ?
		AND workspace = ?
		ORDER BY bm25(memories_fts, 5.0, 3.0, 2.0, 2.0, 0.0, 0.0, 0.0)
		LIMIT ?
	`, query, repoID, workspace, candidateLimit)
	if err != nil {
		return nil, SearchStats{}, err
	}
	defer rows.Close()

	rowIDs := make([]int64, 0, candidateLimit)
	scores := make(map[int64]float64)
	for rows.Next() {
		var rowid int64
		var bm25 float64
		if err := rows.Scan(&rowid, &bm25); err != nil {
			return nil, SearchStats{}, err
		}
		rowIDs = append(rowIDs, rowid)
		scores[rowid] = bm25
	}
	if err := rows.Err(); err != nil {
		return nil, SearchStats{}, err
	}
	stats := SearchStats{
		CandidateTime:  time.Since(candidateStart),
		CandidateCount: len(rowIDs),
		SanitizedQuery: query,
	}
	if len(rowIDs) == 0 {
		return nil, stats, nil
	}

	placeholders := strings.Repeat("?,", len(rowIDs))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(rowIDs)+2)
	for _, rowid := range rowIDs {
		args = append(args, rowid)
	}
	args = append(args, repoID)
	args = append(args, workspace)

	fetchStart := time.Now()
	querySQL := fmt.Sprintf(`
		SELECT rowid, id, repo_id, workspace, thread_id, title, summary, summary_tokens, tags_json, entities_json, created_at, anchor_commit, superseded_by
		FROM memories
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

	details := make(map[int64]Memory, len(rowIDs))
	for fetchRows.Next() {
		var rowid int64
		mem, err := scanMemorySearchFields(func(dest ...any) error {
			args := append([]any{&rowid}, dest...)
			return fetchRows.Scan(args...)
		})
		if err != nil {
			return nil, stats, err
		}
		details[rowid] = mem
	}
	if err := fetchRows.Err(); err != nil {
		return nil, stats, err
	}
	stats.FetchTime = time.Since(fetchStart)

	results := make([]MemoryResult, 0, limit)
	for _, rowid := range rowIDs {
		mem, ok := details[rowid]
		if !ok {
			continue
		}
		results = append(results, MemoryResult{Memory: mem, BM25: scores[rowid]})
		if len(results) >= limit {
			break
		}
	}
	stats.ResultCount = len(results)
	return results, stats, nil
}

func (s *Store) SearchChunks(repoID, workspace, query string, limit int) ([]ChunkResult, SearchStats, error) {
	if limit <= 0 {
		return nil, SearchStats{}, nil
	}
	workspace = normalizeWorkspace(workspace)
	parsed := ParseQuery(query)
	baseQuery, _ := buildQueryFromParsed(parsed, false)
	baseResults, baseStats, err := s.searchChunksWithQuery(repoID, workspace, baseQuery, limit)
	if err != nil {
		return nil, SearchStats{}, err
	}
	if baseStats.ResultCount > 0 {
		return baseResults, baseStats, nil
	}

	expandedQuery, rewriteMeta := buildQueryFromParsed(parsed, true)
	if !rewriteMeta.Rewritten || expandedQuery == baseQuery {
		return baseResults, baseStats, nil
	}

	expandedResults, expandedStats, err := s.searchChunksWithQuery(repoID, workspace, expandedQuery, limit)
	if err != nil {
		return nil, SearchStats{}, err
	}
	expandedStats.BaselineCount = baseStats.ResultCount
	expandedStats.Rewritten = rewriteMeta.Rewritten
	expandedStats.Rewrites = rewriteMeta.Rewrites
	expandedStats.RewriteMatched = baseStats.ResultCount == 0 && expandedStats.ResultCount > 0
	return expandedResults, expandedStats, nil
}

func (s *Store) searchChunksWithQuery(repoID, workspace, query string, limit int) ([]ChunkResult, SearchStats, error) {
	candidateLimit := limit
	if candidateLimit < 200 {
		candidateLimit = 200
	}

	candidateStart := time.Now()
	rows, err := s.db.Query(`
		SELECT rowid, bm25(chunks_fts, 1.0, 3.0, 2.0, 0.0, 0.0, 0.0, 0.0)
		FROM chunks_fts
		WHERE chunks_fts MATCH ?
		AND repo_id = ?
		AND workspace = ?
		ORDER BY bm25(chunks_fts, 1.0, 3.0, 2.0, 0.0, 0.0, 0.0, 0.0)
		LIMIT ?
	`, query, repoID, workspace, candidateLimit)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, SearchStats{}, nil
		}
		return nil, SearchStats{}, err
	}
	defer rows.Close()

	rowIDs := make([]int64, 0, candidateLimit)
	scores := make(map[int64]float64)
	for rows.Next() {
		var rowid int64
		var bm25 float64
		if err := rows.Scan(&rowid, &bm25); err != nil {
			return nil, SearchStats{}, err
		}
		rowIDs = append(rowIDs, rowid)
		scores[rowid] = bm25
	}
	if err := rows.Err(); err != nil {
		return nil, SearchStats{}, err
	}
	stats := SearchStats{
		CandidateTime:  time.Since(candidateStart),
		CandidateCount: len(rowIDs),
		SanitizedQuery: query,
	}
	if len(rowIDs) == 0 {
		return nil, stats, nil
	}

	placeholders := strings.Repeat("?,", len(rowIDs))
	placeholders = strings.TrimSuffix(placeholders, ",")
	args := make([]any, 0, len(rowIDs)+2)
	for _, rowid := range rowIDs {
		args = append(args, rowid)
	}
	args = append(args, repoID)
	args = append(args, workspace)

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
		chunk, err := scanChunkSearchFields(func(dest ...any) error {
			args := append([]any{&rowid}, dest...)
			return fetchRows.Scan(args...)
		})
		if err != nil {
			return nil, stats, err
		}
		details[rowid] = chunk
	}
	if err := fetchRows.Err(); err != nil {
		return nil, stats, err
	}
	stats.FetchTime = time.Since(fetchStart)

	results := make([]ChunkResult, 0, limit)
	for _, rowid := range rowIDs {
		chunk, ok := details[rowid]
		if !ok {
			continue
		}
		results = append(results, ChunkResult{Chunk: chunk, BM25: scores[rowid]})
		if len(results) >= limit {
			break
		}
	}
	stats.ResultCount = len(results)
	return results, stats, nil
}

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
	ChunkType  string
	SymbolName string
	SymbolKind string
	CreatedAt  time.Time
	DeletedAt  time.Time
}

type ChunkResult struct {
	Chunk
	BM25 float64
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func TagsToJSON(tags []string) string {
	if len(tags) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(tags)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func NormalizeTags(tags []string) []string {
	seen := make(map[string]struct{})
	var normalized []string
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		normalized = append(normalized, tag)
	}
	return normalized
}

func ParseTags(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	return NormalizeTags(parts)
}

func TagsText(tags []string) string {
	return strings.Join(tags, " ")
}

func EntitiesToJSON(entities []string) string {
	if len(entities) == 0 {
		return "[]"
	}
	encoded, err := json.Marshal(entities)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func NormalizeEntities(entities []string) []string {
	seen := make(map[string]struct{})
	var normalized []string
	for _, entity := range entities {
		entity = strings.TrimSpace(entity)
		if entity == "" {
			continue
		}
		if _, ok := seen[entity]; ok {
			continue
		}
		seen[entity] = struct{}{}
		normalized = append(normalized, entity)
	}
	return normalized
}

func ParseEntities(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	return NormalizeEntities(parts)
}

func EntitiesText(entities []string) string {
	return strings.Join(entities, " ")
}

const maxQueryLength = 4096

func EnsureValidQuery(query string) error {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return errors.New("query must not be empty")
	}
	if len(trimmed) > maxQueryLength {
		return fmt.Errorf("query is too long (max %d characters)", maxQueryLength)
	}
	return nil
}

func SanitizeQuery(q string) string {
	sanitized, _ := sanitizeQueryWithMeta(q, true)
	return sanitized
}

const maxTokenVariants = 6

type queryRewriteMeta struct {
	Rewritten bool
	Rewrites  []string
}

func sanitizeQueryWithMeta(q string, expand bool) (string, queryRewriteMeta) {
	trimmed := strings.TrimSpace(q)
	if trimmed == "" {
		return "\"\"", queryRewriteMeta{}
	}

	tokens := strings.Fields(trimmed)
	if len(tokens) == 0 {
		return "\"\"", queryRewriteMeta{}
	}

	allowPrefix := len(tokens) == 1
	terms := make([]string, 0, len(tokens))
	var rewrites []string

	for _, token := range tokens {
		variants := []string{token}
		if expand {
			tokenVariants, rewrite := expandTokenVariants(token)
			if len(tokenVariants) > 0 {
				variants = tokenVariants
			}
			if rewrite != "" {
				rewrites = append(rewrites, rewrite)
			}
		}
		if allowPrefix {
			variants = append(variants, prefixVariants(variants)...)
		}
		terms = append(terms, buildTokenExpression(variants))
	}

	andExpr := strings.Join(terms, " AND ")
	boostExpr := buildNearExpression(tokens)
	if boostExpr != "" {
		andExpr = fmt.Sprintf("(%s) OR (%s)", andExpr, boostExpr)
	}

	rewrites = uniqueQueryTerms(rewrites)
	return andExpr, queryRewriteMeta{
		Rewritten: expand && len(rewrites) > 0,
		Rewrites:  rewrites,
	}
}

func buildTokenExpression(variants []string) string {
	unique := uniqueQueryTerms(variants)
	if len(unique) == 0 {
		return "\"\""
	}
	terms := make([]string, 0, len(unique))
	for _, variant := range unique {
		if len(terms) >= maxTokenVariants {
			break
		}
		if term := formatFTSVariant(variant); term != "" {
			terms = append(terms, term)
		}
	}
	if len(terms) == 0 {
		return "\"\""
	}
	if len(terms) == 1 {
		return terms[0]
	}
	return "(" + strings.Join(terms, " OR ") + ")"
}

func buildNearExpression(tokens []string) string {
	if len(tokens) < 2 {
		return ""
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if term := formatFTSVariant(token); term != "" {
			parts = append(parts, term)
		}
	}
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts, " NEAR ")
}

func expandTokenVariants(token string) ([]string, string) {
	segments := splitTokenSegments(token)
	if len(segments) <= 1 {
		return []string{token}, ""
	}
	spaceVariant := strings.Join(segments, " ")
	variants := []string{
		token,
		spaceVariant,
		strings.Join(segments, "-"),
		strings.Join(segments, "_"),
		strings.Join(segments, ""),
	}
	variants = uniqueQueryTerms(variants)
	if len(variants) > maxTokenVariants {
		variants = variants[:maxTokenVariants]
	}
	rewrite := ""
	if spaceVariant != token {
		rewrite = fmt.Sprintf("%s -> %s", token, spaceVariant)
	}
	return variants, rewrite
}

func prefixVariants(variants []string) []string {
	out := make([]string, 0, len(variants))
	for _, variant := range variants {
		term := strings.TrimSpace(variant)
		if term == "" || strings.Contains(term, " ") || len(term) < 3 {
			continue
		}
		if !isPrefixSafe(term) {
			continue
		}
		out = append(out, term+"*")
	}
	return out
}

func splitTokenSegments(token string) []string {
	var segments []string
	var buf []rune
	prevClass := 0
	flush := func() {
		if len(buf) == 0 {
			return
		}
		segments = append(segments, string(buf))
		buf = buf[:0]
	}

	for _, r := range token {
		if r == '-' || r == '_' {
			flush()
			prevClass = 0
			continue
		}
		class := classifyRune(r)
		if class == 0 {
			flush()
			prevClass = 0
			continue
		}
		if prevClass != 0 && class != prevClass {
			flush()
		}
		buf = append(buf, r)
		prevClass = class
	}
	flush()
	return segments
}

func classifyRune(r rune) int {
	switch {
	case unicode.IsLetter(r):
		return 1
	case unicode.IsDigit(r):
		return 2
	default:
		return 0
	}
}

func formatFTSVariant(value string) string {
	term := strings.TrimSpace(value)
	if term == "" {
		return ""
	}
	if strings.HasSuffix(term, "*") && !strings.Contains(term, " ") && isPrefixSafe(strings.TrimSuffix(term, "*")) {
		return term
	}
	return fmt.Sprintf("\"%s\"", escapeFTS(term))
}

func escapeFTS(value string) string {
	return strings.ReplaceAll(value, "\"", "\"\"")
}

func isPrefixSafe(value string) bool {
	for _, r := range value {
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func uniqueQueryTerms(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
