package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"mempack/internal/store"
)

type RepoListItem struct {
	RepoID     string `json:"repo_id"`
	RootName   string `json:"root_name"`
	GitRoot    string `json:"git_root"`
	LastSeenAt string `json:"last_seen_at"`
}

func runRepos(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("repos", flag.ContinueOnError)
	fs.SetOutput(errOut)
	format := fs.String("format", "table", "Output format: table|json")
	fullPaths := fs.Bool("full-paths", false, "Table mode: show full git_root without truncation")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	formatValue := strings.ToLower(strings.TrimSpace(*format))
	if formatValue != "table" && formatValue != "json" {
		fmt.Fprintf(errOut, "unsupported format: %s\n", *format)
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}

	repoDir := cfg.RepoRootDir()
	entries, err := os.ReadDir(repoDir)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(errOut, "repos error: %v\n", err)
		return 1
	}

	var items []RepoListItem
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		repoID := entry.Name()
		path := filepath.Join(repoDir, repoID, "memory.db")
		if _, err := os.Stat(path); err != nil {
			continue
		}

		st, err := store.Open(path)
		if err != nil {
			continue
		}
		repoRow, err := st.GetRepo(repoID)
		st.Close()
		if err != nil {
			continue
		}
		items = append(items, RepoListItem{
			RepoID:     repoRow.RepoID,
			RootName:   repoRootName(repoRow.GitRoot),
			GitRoot:    repoRow.GitRoot,
			LastSeenAt: repoRow.LastSeenAt.UTC().Format(time.RFC3339Nano),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].LastSeenAt > items[j].LastSeenAt
	})

	if formatValue == "json" {
		return writeJSON(out, errOut, items)
	}
	writeReposTable(out, items, *fullPaths)
	return 0
}

func writeReposTable(out io.Writer, items []RepoListItem, fullPaths bool) {
	headers := []string{"REPO ID", "ROOT", "LAST SEEN", "GIT ROOT"}
	rows := make([][]string, 0, len(items))
	maxRoot, maxGitRoot := tableColumnWidths()
	for _, item := range items {
		rootName := strings.TrimSpace(item.RootName)
		if rootName == "" {
			rootName = "-"
		}
		rootDisplay := rootName
		gitRootDisplay := item.GitRoot
		if !fullPaths {
			rootDisplay = truncateMiddle(rootName, maxRoot)
			gitRootDisplay = truncateMiddle(item.GitRoot, maxGitRoot)
		}
		rows = append(rows, []string{item.RepoID, rootDisplay, item.LastSeenAt, gitRootDisplay})
	}

	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, col := range row {
			if len(col) > widths[i] {
				widths[i] = len(col)
			}
		}
	}

	border := asciiBorder(widths)
	fmt.Fprintln(out, border)
	writeASCIIRow(out, widths, headers)
	fmt.Fprintln(out, border)
	for _, row := range rows {
		writeASCIIRow(out, widths, row)
	}
	fmt.Fprintln(out, border)
}

func tableColumnWidths() (int, int) {
	const (
		defaultRootWidth = 24
		defaultGitWidth  = 72
		minGitWidth      = 28
		maxGitWidth      = 120
	)
	cols := terminalColumns()
	if cols <= 0 {
		return defaultRootWidth, defaultGitWidth
	}

	// Approximate width reserved by non-path columns + separators.
	reserved := 10 + defaultRootWidth + 25
	gitWidth := cols - reserved
	if gitWidth < minGitWidth {
		gitWidth = minGitWidth
	}
	if gitWidth > maxGitWidth {
		gitWidth = maxGitWidth
	}
	return defaultRootWidth, gitWidth
}

func terminalColumns() int {
	raw := strings.TrimSpace(os.Getenv("COLUMNS"))
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func truncateMiddle(value string, max int) string {
	text := strings.TrimSpace(value)
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	left := (max - 3) / 3
	if left < 1 {
		left = 1
	}
	right := max - 3 - left
	if right < 1 {
		right = 1
	}
	if left+right+3 > len(text) {
		return text
	}
	return text[:left] + "..." + text[len(text)-right:]
}

func asciiBorder(widths []int) string {
	var b strings.Builder
	b.WriteByte('+')
	for _, w := range widths {
		b.WriteString(strings.Repeat("-", w+2))
		b.WriteByte('+')
	}
	return b.String()
}

func writeASCIIRow(out io.Writer, widths []int, cols []string) {
	fmt.Fprint(out, "|")
	for i, width := range widths {
		value := ""
		if i < len(cols) {
			value = cols[i]
		}
		fmt.Fprintf(out, " %-*s |", width, value)
	}
	fmt.Fprintln(out)
}

func repoRootName(gitRoot string) string {
	root := strings.TrimSpace(gitRoot)
	if root == "" {
		return ""
	}
	clean := filepath.Clean(root)
	if clean == string(filepath.Separator) {
		return clean
	}
	base := filepath.Base(clean)
	if base == "." || base == string(filepath.Separator) {
		return clean
	}
	return base
}
