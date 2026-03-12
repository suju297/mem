package app

import "fmt"

// Version and Commit can be overridden at build time:
// go build -ldflags "-X mem/internal/app.Version=v0.2.3 -X mem/internal/app.Commit=abcdef0" ./cmd/mem
var (
	Version = "v0.2.41"
	Commit  = "dev"
)

func VersionString() string {
	return fmt.Sprintf("mem %s (%s)", Version, Commit)
}
