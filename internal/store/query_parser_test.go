package store

import (
	"strings"
	"testing"
)

func TestQueryIntentDetection(t *testing.T) {
	tests := []struct {
		query  string
		intent QueryIntent
		boost  float64
	}{
		{"auth login", IntentSearch, 1.0},
		{"recent auth changes", IntentRecent, 2.0},
		{"show me today's work", IntentRecent, 3.0},
		{"what's new in auth", IntentRecent, 1.5},
		{"create new memory", IntentSearch, 1.0},
		{"new architecture proposal", IntentSearch, 1.0},
		{"thread T-AUTH", IntentThread, 1.0},
		{"Store.AddMemory function", IntentSymbol, 1.0},
		{"store.Open function", IntentSymbol, 1.0},
		{"changes in auth.go", IntentFile, 1.0},
	}

	for _, tt := range tests {
		parsed := ParseQuery(tt.query)
		if parsed.Intent != tt.intent {
			t.Errorf("%q: expected intent %s, got %s", tt.query, tt.intent, parsed.Intent)
		}
		if parsed.BoostRecency != tt.boost {
			t.Errorf("%q: expected boost %.1f, got %.1f", tt.query, tt.boost, parsed.BoostRecency)
		}
	}
}

func TestParseQueryNewRecencyHeuristics(t *testing.T) {
	positive := []string{
		"new",
		"new changes in auth",
		"show me new updates",
		"what is new in auth",
	}
	for _, q := range positive {
		parsed := ParseQuery(q)
		if parsed.TimeHint == nil || parsed.BoostRecency <= 1.0 {
			t.Fatalf("%q: expected recent time hint and boost > 1.0", q)
		}
	}

	negative := []string{
		"create new endpoint",
		"new architecture proposal",
		"adjust build pipeline",
		"renew auth token",
	}
	for _, q := range negative {
		parsed := ParseQuery(q)
		if parsed.TimeHint != nil || parsed.BoostRecency != 1.0 {
			t.Fatalf("%q: expected no recency hint", q)
		}
	}
}

func TestEntityExtraction(t *testing.T) {
	tests := []struct {
		query    string
		entities int
	}{
		{"update T-AUTH thread", 1},
		{"Store.AddMemory implementation", 1},
		{"call ctx.Done safely", 1},
		{"changes in auth.go and user.go", 2},
		{"T-AUTH Store.Get auth.go", 3},
	}

	for _, tt := range tests {
		parsed := ParseQuery(tt.query)
		if len(parsed.Entities) != tt.entities {
			t.Errorf("%q: expected %d entities, got %d", tt.query, tt.entities, len(parsed.Entities))
		}
	}
}

func TestBackwardCompatibility(t *testing.T) {
	queries := []string{
		"authentication",
		"user login",
		"delta99",
		"AddMemory function",
	}

	for _, q := range queries {
		parsed := ParseQuery(q)
		if len(parsed.Keywords) > 1 && !strings.Contains(parsed.FTSQuery, "AND") {
			t.Errorf("%q: expected AND in FTS query", q)
		}
		if parsed.BoostRecency != 1.0 {
			t.Errorf("%q: expected boost 1.0, got %.1f", q, parsed.BoostRecency)
		}
	}
}

func TestParseQueryPreservesBaseQuery(t *testing.T) {
	queries := []string{
		"authentication",
		"user login",
		"delta99",
	}
	for _, q := range queries {
		parsed := ParseQuery(q)
		baseQuery, _ := sanitizeQueryWithMeta(q, false)
		if parsed.FTSQuery != baseQuery {
			t.Errorf("%q: expected base FTS query %q, got %q", q, baseQuery, parsed.FTSQuery)
		}
	}
}
