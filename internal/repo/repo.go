package repo

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type Info struct {
	ID      string
	GitRoot string
	Head    string
	Branch  string
	Origin  string
	HasGit  bool
}

func Detect(cwd string) (Info, error) {
	info, err := DetectBase(cwd)
	if err != nil {
		return Info{}, err
	}
	if !info.HasGit {
		return info, nil
	}
	return PopulateOriginAndID(info)
}

func DetectStrict(cwd string) (Info, error) {
	info, err := DetectBaseStrict(cwd)
	if err != nil {
		return Info{}, err
	}
	return PopulateOriginAndID(info)
}

func DetectBase(cwd string) (Info, error) {
	root, head, branch, err := gitRootHeadBranch(cwd)
	if err != nil {
		return fallbackInfo(cwd), nil
	}
	return Info{
		GitRoot: root,
		Head:    head,
		Branch:  branch,
		HasGit:  true,
	}, nil
}

func DetectBaseStrict(cwd string) (Info, error) {
	root, head, branch, err := gitRootHeadBranch(cwd)
	if err != nil {
		return Info{}, err
	}
	return Info{
		GitRoot: root,
		Head:    head,
		Branch:  branch,
		HasGit:  true,
	}, nil
}

func DetectFromRoot(root string) (Info, error) {
	head, branch, err := gitHeadBranch(root)
	if err != nil {
		return Info{GitRoot: root, HasGit: false}, err
	}
	return Info{
		GitRoot: root,
		Head:    head,
		Branch:  branch,
		HasGit:  true,
	}, nil
}

// InfoFromCache creates an Info entirely from cached metadata without any git calls.
// Use this when you have valid cached data and want to avoid subprocess overhead.
// If needsFreshHead is true, it will make one git call to get current HEAD/branch.
func InfoFromCache(id, gitRoot, cachedHead, cachedBranch string, needsFreshHead bool) (Info, error) {
	if needsFreshHead {
		head, branch, err := gitHeadBranch(gitRoot)
		if err != nil {
			// Fall back to cached values if git fails
			return Info{
				ID:      id,
				GitRoot: gitRoot,
				Head:    cachedHead,
				Branch:  cachedBranch,
				HasGit:  cachedHead != "" || cachedBranch != "",
			}, nil
		}
		return Info{
			ID:      id,
			GitRoot: gitRoot,
			Head:    head,
			Branch:  branch,
			HasGit:  true,
		}, nil
	}
	// No git calls at all - use purely cached data
	return Info{
		ID:      id,
		GitRoot: gitRoot,
		Head:    cachedHead,
		Branch:  cachedBranch,
		HasGit:  cachedHead != "" || cachedBranch != "",
	}, nil
}

func PopulateOriginAndID(info Info) (Info, error) {
	if !info.HasGit {
		info.ID = hashID("p_", info.GitRoot)
		return info, nil
	}

	origin, _ := gitOutput(info.GitRoot, "config", "--get", "remote.origin.url")
	info.Origin = strings.TrimSpace(origin)

	firstCommit := ""
	if info.Origin == "" {
		commit, _ := gitOutput(info.GitRoot, "rev-list", "--max-parents=0", "HEAD")
		firstCommit = strings.TrimSpace(commit)
	}

	info.ID = computeID(info, firstCommit)
	return info, nil
}

func IsAncestor(repoRoot, commit, head string) (bool, error) {
	if commit == "" || head == "" {
		return true, nil
	}

	if commit == head {
		return true, nil
	}

	_, err := gitOutput(repoRoot, "merge-base", "--is-ancestor", commit, head)
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return false, nil
	}

	return false, err
}

func fallbackInfo(cwd string) Info {
	root := filepath.Clean(cwd)
	id := hashID("p_", root)
	return Info{
		ID:      id,
		GitRoot: root,
		HasGit:  false,
	}
}

func computeID(info Info, firstCommit string) string {
	if info.Origin != "" {
		return hashID("r_", info.Origin)
	}

	if info.HasGit && firstCommit != "" {
		seed := info.GitRoot + ":" + firstCommit
		return hashID("r_", seed)
	}

	return hashID("p_", info.GitRoot)
}

func hashID(prefix, input string) string {
	h := sha256.Sum256([]byte(input))
	hexDigest := hex.EncodeToString(h[:])
	return fmt.Sprintf("%s%s", prefix, hexDigest[:8])
}

func gitOutput(repoRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repoRoot}, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", err
	}
	return stdout.String(), nil
}

func gitRootHeadBranch(cwd string) (string, string, string, error) {
	// Note: --abbrev-ref applies to subsequent args, so we put the SHA HEAD first
	output, err := gitOutput(cwd, "rev-parse", "--show-toplevel", "HEAD", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", "", err
	}
	lines := splitLines(output)
	if len(lines) < 3 {
		return "", "", "", fmt.Errorf("unexpected rev-parse output")
	}
	root := strings.TrimSpace(lines[0])
	head := strings.TrimSpace(lines[1])   // SHA
	branch := strings.TrimSpace(lines[2]) // Branch
	return root, head, branch, nil
}

func gitHeadBranch(root string) (string, string, error) {
	// Note: --abbrev-ref applies to subsequent args, so we put the SHA HEAD first
	output, err := gitOutput(root, "rev-parse", "HEAD", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", "", err
	}
	lines := splitLines(output)
	if len(lines) < 2 {
		return "", "", fmt.Errorf("unexpected rev-parse output")
	}
	head := strings.TrimSpace(lines[0])   // SHA
	branch := strings.TrimSpace(lines[1]) // Branch
	return head, branch, nil
}

func splitLines(output string) []string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}
