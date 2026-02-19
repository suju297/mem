package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteReposTableCompactsLongPathByDefault(t *testing.T) {
	t.Setenv("COLUMNS", "80")

	longPath := "/Users/example/Library/CloudStorage/GoogleDrive-user@example.com/My Drive/Documents/Projects/very-long-project-name/with/many/segments"
	items := []RepoListItem{
		{
			RepoID:     "r_12345678",
			RootName:   "very-long-project-name",
			GitRoot:    longPath,
			LastSeenAt: "2026-02-19T21:19:11.468398Z",
		},
	}

	var out bytes.Buffer
	writeReposTable(&out, items, false)
	text := out.String()

	if !strings.Contains(text, "REPO ID") || !strings.Contains(text, "GIT ROOT") {
		t.Fatalf("expected table headers, got %q", text)
	}
	if !strings.Contains(text, "+") || !strings.Contains(text, "|") {
		t.Fatalf("expected ascii table borders, got %q", text)
	}
	if strings.Contains(text, longPath) {
		t.Fatalf("expected long path to be compacted in table mode")
	}
	if !strings.Contains(text, "...") {
		t.Fatalf("expected compacted path to include ellipsis, got %q", text)
	}
}

func TestWriteReposTableFullPathsShowsRawPath(t *testing.T) {
	t.Setenv("COLUMNS", "80")

	longPath := "/Users/example/Library/CloudStorage/GoogleDrive-user@example.com/My Drive/Documents/Projects/very-long-project-name/with/many/segments"
	items := []RepoListItem{
		{
			RepoID:     "r_12345678",
			RootName:   "very-long-project-name",
			GitRoot:    longPath,
			LastSeenAt: "2026-02-19T21:19:11.468398Z",
		},
	}

	var out bytes.Buffer
	writeReposTable(&out, items, true)
	text := out.String()
	if !strings.Contains(text, longPath) {
		t.Fatalf("expected full-path table mode to include full git root")
	}
}

func TestTruncateMiddle(t *testing.T) {
	got := truncateMiddle("abcdefghijklmnopqrstuvwxyz", 12)
	if len(got) > 12 {
		t.Fatalf("expected truncated output length <= 12, got %d", len(got))
	}
	if !strings.Contains(got, "...") {
		t.Fatalf("expected ellipsis in truncated output, got %q", got)
	}

	if same := truncateMiddle("short", 12); same != "short" {
		t.Fatalf("expected short values to remain unchanged, got %q", same)
	}
}
