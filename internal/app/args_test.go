package app

import (
	"reflect"
	"testing"
)

func TestSplitGlobalFlagsSkipsDoubleDash(t *testing.T) {
	out, globals, err := splitGlobalFlags([]string{"--data-dir", "/tmp/mempack", "--", "mcp", "start"})
	if err != nil {
		t.Fatalf("splitGlobalFlags error: %v", err)
	}
	if globals.DataDir != "/tmp/mempack" {
		t.Fatalf("unexpected data dir: %q", globals.DataDir)
	}
	want := []string{"mcp", "start"}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("unexpected args: want=%v got=%v", want, out)
	}
}

func TestSplitGlobalFlagsDoubleDashOnly(t *testing.T) {
	out, globals, err := splitGlobalFlags([]string{"--"})
	if err != nil {
		t.Fatalf("splitGlobalFlags error: %v", err)
	}
	if globals.DataDir != "" {
		t.Fatalf("unexpected data dir: %q", globals.DataDir)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty args after --, got %v", out)
	}
}
