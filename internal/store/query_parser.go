package store

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

type QueryIntent string

const (
	IntentSearch QueryIntent = "search"
	IntentRecent QueryIntent = "recent"
	IntentThread QueryIntent = "thread"
	IntentSymbol QueryIntent = "symbol"
	IntentFile   QueryIntent = "file"
)

type ParsedQuery struct {
	Original     string
	Intent       QueryIntent
	Entities     []Entity
	TimeHint     *TimeHint
	Keywords     []string
	FTSQuery     string
	BoostRecency float64
}

type Entity struct {
	Type  string
	Value string
	Raw   string
}

type TimeHint struct {
	Relative string
	After    time.Time
}

var (
	threadIDPattern = regexp.MustCompile(`\bT-[A-Za-z0-9_-]+\b`)
	filePathPattern = regexp.MustCompile(`\b[\w./\\-]+\.(go|py|ts|js|tsx|jsx|md|json|yaml|yml|sql|sh)\b`)
	symbolPattern   = regexp.MustCompile(`\b[A-Z][a-zA-Z0-9]*\.[A-Z][a-zA-Z0-9]*\b`)
)

var stopWords = map[string]bool{
	"show": true, "me": true, "the": true, "a": true, "an": true,
	"find": true, "search": true, "get": true, "recent": true,
	"latest": true, "new": true, "from": true, "in": true,
	"about": true, "for": true, "what": true, "where": true,
	"how": true, "when": true, "why": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true,
	"today": true, "yesterday": true, "this": true, "last": true,
	"week": true, "month": true, "year": true,
}

// ParseQuery analyzes a query and extracts intent, entities, and search terms.
// It preserves the original FTS query generation but adds semantic metadata.
func ParseQuery(q string) ParsedQuery {
	q = strings.TrimSpace(q)
	parsed := ParsedQuery{
		Original:     q,
		Intent:       IntentSearch,
		BoostRecency: 1.0,
		Keywords:     []string{},
		Entities:     []Entity{},
	}
	if q == "" {
		parsed.FTSQuery = "\"\""
		return parsed
	}

	lower := strings.ToLower(q)
	parsed.TimeHint, parsed.BoostRecency = detectTimeHint(lower)
	parsed.Intent = detectIntent(lower, q)
	parsed.Entities = extractEntities(q)
	parsed.Keywords = extractKeywords(q, parsed.Entities)
	parsed.FTSQuery, _ = buildQueryFromParsed(parsed, false)

	return parsed
}

func detectTimeHint(lower string) (*TimeHint, float64) {
	now := time.Now().UTC()

	patterns := []struct {
		signal   string
		relative string
		after    time.Time
		boost    float64
	}{
		{"today", "today", now.Truncate(24 * time.Hour), 3.0},
		{"yesterday", "yesterday", now.Add(-24 * time.Hour).Truncate(24 * time.Hour), 2.5},
		{"this week", "this week", now.Add(-7 * 24 * time.Hour), 2.0},
		{"last week", "last week", now.Add(-14 * 24 * time.Hour), 1.8},
		{"recent", "recent", now.Add(-7 * 24 * time.Hour), 2.0},
		{"latest", "recent", now.Add(-3 * 24 * time.Hour), 2.5},
		{"just", "recent", now.Add(-24 * time.Hour), 2.5},
		{"new", "recent", now.Add(-7 * 24 * time.Hour), 1.5},
	}

	for _, p := range patterns {
		if strings.Contains(lower, p.signal) {
			return &TimeHint{Relative: p.relative, After: p.after}, p.boost
		}
	}
	return nil, 1.0
}

func detectIntent(lower, original string) QueryIntent {
	if threadIDPattern.MatchString(original) || containsAny(lower, []string{"thread", "conversation", "discussion", "chat"}) {
		return IntentThread
	}
	if symbolPattern.MatchString(original) || containsAny(lower, []string{"function", "method", "struct", "type ", "class", "interface", "func ", "def ", "fn "}) {
		return IntentSymbol
	}
	if filePathPattern.MatchString(original) || containsAny(lower, []string{"file", "in file", "from file", ".go", ".py", ".ts", ".js", ".md"}) {
		return IntentFile
	}
	if containsAny(lower, []string{"recent", "latest", "new", "today", "yesterday"}) {
		return IntentRecent
	}
	return IntentSearch
}

func extractEntities(q string) []Entity {
	var entities []Entity

	for _, match := range threadIDPattern.FindAllString(q, -1) {
		entities = append(entities, Entity{Type: "thread", Value: match, Raw: match})
	}
	for _, match := range filePathPattern.FindAllString(q, -1) {
		entities = append(entities, Entity{Type: "file", Value: match, Raw: match})
	}
	for _, match := range symbolPattern.FindAllString(q, -1) {
		entities = append(entities, Entity{Type: "symbol", Value: match, Raw: match})
	}

	return entities
}

func extractKeywords(q string, entities []Entity) []string {
	cleaned := q
	for _, e := range entities {
		cleaned = strings.ReplaceAll(cleaned, e.Raw, " ")
	}

	tokens := strings.Fields(cleaned)
	keywords := make([]string, 0, len(tokens))
	for _, token := range tokens {
		clean := strings.Trim(token, " \t\r\n.,;:!?()[]{}<>\"'")
		if clean == "" {
			continue
		}
		lower := strings.ToLower(clean)
		if stopWords[lower] {
			continue
		}
		if len(clean) < 2 {
			continue
		}
		keywords = append(keywords, clean)
	}
	if len(keywords) == 0 {
		for _, token := range tokens {
			clean := strings.Trim(token, " \t\r\n.,;:!?()[]{}<>\"'")
			if clean == "" {
				continue
			}
			if len(clean) < 2 {
				continue
			}
			keywords = append(keywords, clean)
		}
	}
	return keywords
}

func buildQueryFromParsed(parsed ParsedQuery, expand bool) (string, queryRewriteMeta) {
	if !needsEnhancedQuery(parsed) {
		return sanitizeQueryWithMeta(parsed.Original, expand)
	}
	return buildEnhancedFTSQuery(parsed, expand)
}

func needsEnhancedQuery(parsed ParsedQuery) bool {
	if parsed.Intent != IntentSearch {
		return true
	}
	if parsed.TimeHint != nil {
		return true
	}
	return len(parsed.Entities) > 0
}

func buildEnhancedFTSQuery(parsed ParsedQuery, expand bool) (string, queryRewriteMeta) {
	var parts []string
	var rewrites []string

	for _, e := range parsed.Entities {
		switch e.Type {
		case "thread":
			if term := formatFTSVariant(e.Value); term != "" {
				parts = append(parts, term)
			}
		case "symbol":
			if term := buildSymbolQuery(e.Value); term != "" {
				parts = append(parts, term)
			}
		case "file":
			if term := formatFTSVariant(e.Value); term != "" {
				parts = append(parts, term)
			}
		}
	}

	allowPrefix := len(parsed.Keywords) == 1 && len(parsed.Entities) == 0
	for _, kw := range parsed.Keywords {
		variants := []string{kw}
		if expand {
			tokenVariants, rewrite := expandTokenVariants(kw)
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
		term := buildTokenExpression(variants)
		if term != "" && term != "\"\"" {
			parts = append(parts, term)
		}
	}

	rewrites = uniqueQueryTerms(rewrites)
	meta := queryRewriteMeta{
		Rewritten: expand && len(rewrites) > 0,
		Rewrites:  rewrites,
	}

	if len(parts) == 0 {
		return "\"\"", meta
	}

	andExpr := strings.Join(parts, " AND ")
	if len(parsed.Keywords) >= 2 {
		nearParts := make([]string, 0, len(parsed.Keywords))
		for _, kw := range parsed.Keywords {
			if term := formatFTSVariant(kw); term != "" {
				nearParts = append(nearParts, term)
			}
		}
		if len(nearParts) >= 2 {
			nearExpr := strings.Join(nearParts, " NEAR ")
			andExpr = fmt.Sprintf("(%s) OR (%s)", andExpr, nearExpr)
		}
	}

	return andExpr, meta
}

func buildSymbolQuery(symbol string) string {
	symbolTerm := formatFTSVariant(symbol)
	if symbolTerm == "" {
		return ""
	}
	parts := strings.Split(symbol, ".")
	if len(parts) == 2 {
		left := formatFTSVariant(parts[0])
		right := formatFTSVariant(parts[1])
		if left != "" && right != "" {
			return fmt.Sprintf("(%s AND %s) OR %s", left, right, symbolTerm)
		}
	}
	return symbolTerm
}

func containsAny(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
