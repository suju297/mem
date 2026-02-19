//go:build !windows

package app

import (
	"os"
	"syscall"
)

func flockExclusive(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
}

func flockShared(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_SH)
}

func flockUnlock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
}
