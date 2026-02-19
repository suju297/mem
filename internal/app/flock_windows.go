//go:build windows

package app

import "os"

// Windows fallback: keep behavior functional and compile-safe.
// Locking is best-effort on this platform until a native lock implementation is added.
func flockExclusive(_ *os.File) error {
	return nil
}

func flockShared(_ *os.File) error {
	return nil
}

func flockUnlock(_ *os.File) error {
	return nil
}
