package app

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// Version and Commit can be overridden at build time:
// go build -ldflags "-X mem/internal/app.Version=v0.2.3 -X mem/internal/app.Commit=abcdef0" ./cmd/mem
var (
	Version = "v0.2.46"
	Commit  = "dev"
)

var readBuildInfo = debug.ReadBuildInfo

func VersionString() string {
	return fmt.Sprintf("mem %s (%s)", Version, effectiveCommit())
}

func effectiveCommit() string {
	commit := strings.TrimSpace(Commit)
	if commit != "" && commit != "dev" {
		return commit
	}
	if vcsCommit := vcsCommitFromBuildInfo(); vcsCommit != "" {
		return vcsCommit
	}
	if commit == "" {
		return "dev"
	}
	return commit
}

func vcsCommitFromBuildInfo() string {
	info, ok := readBuildInfo()
	if !ok || info == nil {
		return ""
	}

	revision := ""
	dirty := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}
	if revision == "" {
		return ""
	}
	if len(revision) > 7 {
		revision = revision[:7]
	}
	if dirty {
		return revision + "-dirty"
	}
	return revision
}
