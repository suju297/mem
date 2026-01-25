package token

import (
	"testing"
)

func TestToken(t *testing.T) {
	// cl100k_base
	c, err := New("cl100k_base")
	if err != nil {
		t.Fatalf("Failed to create tokenizer: %v", err)
	}

	text := "Hello world"
	count := c.Count(text)
	if count != 2 {
		t.Errorf("Expected 2 tokens for 'Hello world', got %d", count)
	}

	longText := "token "
	for i := 0; i < 50; i++ {
		longText += "token "
	}
	// "token " is 1 or 2 tokens depending on spacing, "token" is 1. " " is 1.
	// "token " -> token(5) + space(1)? No, cl100k usually merges.
	// Let's rely on basic count check > 0
	count = c.Count(longText)
	if count <= 10 {
		t.Errorf("Expected > 10 tokens, got %d", count)
	}

	// Test Truncate
	truncated, tCount := c.Truncate(longText, 10)
	if tCount > 10 {
		t.Errorf("Truncated count %d > limit 10", tCount)
	}
	// Verify it actually truncated (string length shorter)
	if len(truncated) >= len(longText) {
		t.Errorf("Truncated text length %d >= original %d", len(truncated), len(longText))
	}

	// Boundary case
	trunc2, count2 := c.Truncate("short", 100)
	if trunc2 != "short" {
		t.Errorf("Truncate changed short string")
	}
	if count2 != c.Count("short") {
		t.Errorf("Truncate returned wrong count for short string")
	}
	// Edge Case: Empty string
	if count := c.Count(""); count != 0 {
		t.Errorf("Expected 0 tokens for empty string, got %d", count)
	}

	// Edge Case: Unicode
	// Unicode chars often take >=1 tokens or merge differently.
	// "Hello 🌍" -> Hello(1) + space(1) + earth(1)? or merged?
	// Just verify it doesn't crash and returns > 0
	uniText := "Hello 🌍"
	if count := c.Count(uniText); count == 0 {
		t.Error("Expected > 0 tokens for unicode text")
	}

	// Edge Case: Truncate with unicode
	// Ensure we don't split multi-byte characters in a way that creates invalid utf8?
	// Tiktoken handles this, but let's verify truncation logic behaves sane.
	// "🌍" is 4 bytes.
	// If we truncate, we expect valid string back.
	truncUni, _ := c.Truncate("Test 🌍", 10) // should fit
	if truncUni != "Test 🌍" {
		t.Errorf("Truncate failed on short unicode string")
	}
}
