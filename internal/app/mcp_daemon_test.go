package app

import (
	"reflect"
	"testing"
)

func TestBuildMCPChildArgsUsesFallbackDataDir(t *testing.T) {
	got, err := buildMCPChildArgs([]string{"--allow-write"}, "/tmp/mempack-data")
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	want := []string{"--data-dir", "/tmp/mempack-data", "mcp", "--allow-write", "--require-repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected child args:\nwant=%v\ngot=%v", want, got)
	}
}

func TestBuildMCPChildArgsPrefersExplicitDataDir(t *testing.T) {
	got, err := buildMCPChildArgs([]string{"--data-dir", "/tmp/override", "--allow-write"}, "/tmp/mempack-data")
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	want := []string{"--data-dir", "/tmp/override", "mcp", "--allow-write", "--require-repo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected child args:\nwant=%v\ngot=%v", want, got)
	}
}

func TestBuildMCPChildArgsKeepsExplicitRequireRepoValue(t *testing.T) {
	got, err := buildMCPChildArgs([]string{"--allow-write", "--require-repo=false"}, "/tmp/mempack-data")
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	want := []string{"--data-dir", "/tmp/mempack-data", "mcp", "--allow-write", "--require-repo=false"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected child args:\nwant=%v\ngot=%v", want, got)
	}
}
