package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	origVersion := Version
	origCommit := Commit
	Version = "v0.2.0"
	Commit = "abc1234"
	t.Cleanup(func() {
		Version = origVersion
		Commit = origCommit
	})

	var out bytes.Buffer
	code := Run([]string{"--version"}, &out, &out)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	got := strings.TrimSpace(out.String())
	want := "mempack v0.2.0 (abc1234)"
	if got != want {
		t.Fatalf("unexpected version output: %q (want %q)", got, want)
	}
}
