package app

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	origVersion := Version
	origCommit := Commit
	origReadBuildInfo := readBuildInfo
	Version = "v0.2.3"
	Commit = "abc1234"
	t.Cleanup(func() {
		Version = origVersion
		Commit = origCommit
		readBuildInfo = origReadBuildInfo
	})
	readBuildInfo = debug.ReadBuildInfo

	var out bytes.Buffer
	code := Run([]string{"--version"}, &out, &out)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	got := strings.TrimSpace(out.String())
	want := "mem v0.2.3 (abc1234)"
	if got != want {
		t.Fatalf("unexpected version output: %q (want %q)", got, want)
	}
}

func TestVersionStringFallsBackToBuildInfoCommit(t *testing.T) {
	origVersion := Version
	origCommit := Commit
	origReadBuildInfo := readBuildInfo
	Version = "v0.2.3"
	Commit = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef1234567890"},
				{Key: "vcs.modified", Value: "false"},
			},
		}, true
	}
	t.Cleanup(func() {
		Version = origVersion
		Commit = origCommit
		readBuildInfo = origReadBuildInfo
	})

	got := VersionString()
	want := "mem v0.2.3 (abcdef1)"
	if got != want {
		t.Fatalf("unexpected version output: %q (want %q)", got, want)
	}
}

func TestVersionStringMarksDirtyBuildInfo(t *testing.T) {
	origVersion := Version
	origCommit := Commit
	origReadBuildInfo := readBuildInfo
	Version = "v0.2.3"
	Commit = "dev"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abcdef1234567890"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	}
	t.Cleanup(func() {
		Version = origVersion
		Commit = origCommit
		readBuildInfo = origReadBuildInfo
	})

	got := VersionString()
	want := "mem v0.2.3 (abcdef1-dirty)"
	if got != want {
		t.Fatalf("unexpected version output: %q (want %q)", got, want)
	}
}
