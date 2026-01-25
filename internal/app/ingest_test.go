package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestChunkRangesOverlap(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e", "f"}
	lineTokens := []int{1, 1, 1, 1, 1, 1}

	ranges := chunkRanges(lines, lineTokens, 3, 1)
	expected := []chunkRange{
		{Start: 0, End: 3},
		{Start: 2, End: 5},
		{Start: 4, End: 6},
	}

	if !reflect.DeepEqual(ranges, expected) {
		t.Fatalf("expected ranges %+v, got %+v", expected, ranges)
	}
}

func TestLoadIgnoreMatcher(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".mempackignore"), []byte("skip/\n"), 0o644); err != nil {
		t.Fatalf("write mempackignore: %v", err)
	}

	matcher := loadIgnoreMatcher(root)
	if !matcher.Matches("ignored.txt") {
		t.Fatalf("expected ignored.txt to match")
	}
	if !matcher.Matches("skip/file.txt") {
		t.Fatalf("expected skip/ to match")
	}
	if !matcher.Matches(".git/config") {
		t.Fatalf("expected .git/ to match")
	}
	if matcher.Matches("keep.txt") {
		t.Fatalf("expected keep.txt not to match")
	}
}
