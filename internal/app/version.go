package app

import "fmt"

// Version and Commit can be overridden at build time:
// go build -ldflags "-X mempack/internal/app.Version=v0.2.3 -X mempack/internal/app.Commit=abcdef0" ./cmd/mem
var (
	Version = "v0.2.35"
	Commit  = "dev"
)

func VersionString() string {
	return fmt.Sprintf("mem %s (%s)", Version, Commit)
}
