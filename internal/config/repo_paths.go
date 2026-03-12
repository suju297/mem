package config

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	RepoSupportDirName       = ".mem"
	LegacyRepoSupportDirName = ".mempack"
	RepoIgnoreFileName       = ".memignore"
	LegacyRepoIgnoreFileName = ".mempackignore"
)

func ResolveRepoSupportDir(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		if pathIsDir(RepoSupportDirName) {
			return RepoSupportDirName
		}
		if pathIsDir(LegacyRepoSupportDirName) {
			return LegacyRepoSupportDirName
		}
		return RepoSupportDirName
	}
	if pathIsDir(filepath.Join(root, RepoSupportDirName)) {
		return filepath.Join(root, RepoSupportDirName)
	}
	if pathIsDir(filepath.Join(root, LegacyRepoSupportDirName)) {
		return filepath.Join(root, LegacyRepoSupportDirName)
	}
	return filepath.Join(root, RepoSupportDirName)
}

func ResolveRepoSupportDirName(root string) string {
	return filepath.Base(ResolveRepoSupportDir(root))
}

func ResolveRepoSupportPath(root string, elems ...string) string {
	base := ResolveRepoSupportDir(root)
	if len(elems) == 0 {
		return base
	}
	parts := append([]string{base}, elems...)
	return filepath.Join(parts...)
}

func ResolveRepoIgnorePath(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		if pathExists(RepoIgnoreFileName) {
			return RepoIgnoreFileName
		}
		if pathExists(LegacyRepoIgnoreFileName) {
			return LegacyRepoIgnoreFileName
		}
		return RepoIgnoreFileName
	}
	if pathExists(filepath.Join(root, RepoIgnoreFileName)) {
		return filepath.Join(root, RepoIgnoreFileName)
	}
	if pathExists(filepath.Join(root, LegacyRepoIgnoreFileName)) {
		return filepath.Join(root, LegacyRepoIgnoreFileName)
	}
	return filepath.Join(root, RepoIgnoreFileName)
}

func pathIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
