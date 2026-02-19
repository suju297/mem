package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type RecentMemoryItem struct {
	ID           string `json:"id"`
	ThreadID     string `json:"thread_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	CreatedAt    string `json:"created_at"`
	AnchorCommit string `json:"anchor_commit,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`
}

func runRecent(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("recent", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id")
	workspace := fs.String("workspace", "", "Workspace name")
	limit := fs.Int("limit", 20, "Max memories to show")
	format := fs.String("format", "json", "Output format: json")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"repo":      {RequiresValue: true},
		"workspace": {RequiresValue: true},
		"limit":     {RequiresValue: true},
		"format":    {RequiresValue: true},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(positional) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(positional, " "))
		return 2
	}
	if strings.TrimSpace(*format) != "json" {
		fmt.Fprintf(errOut, "unsupported format: %s\n", *format)
		return 2
	}
	if *limit < 0 {
		fmt.Fprintln(errOut, "limit must be >= 0")
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	repoInfo, err := resolveRepo(&cfg, strings.TrimSpace(*repoOverride))
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		fmt.Fprintf(errOut, "store open error: %v\n", err)
		return 1
	}
	defer st.Close()

	memories, err := st.ListRecentMemories(repoInfo.ID, workspaceName, *limit)
	if err != nil {
		fmt.Fprintf(errOut, "recent error: %v\n", err)
		return 1
	}

	items := make([]RecentMemoryItem, 0, len(memories))
	for _, mem := range memories {
		items = append(items, RecentMemoryItem{
			ID:           mem.ID,
			ThreadID:     mem.ThreadID,
			Title:        mem.Title,
			Summary:      mem.Summary,
			CreatedAt:    mem.CreatedAt.UTC().Format(time.RFC3339Nano),
			AnchorCommit: mem.AnchorCommit,
			SupersededBy: mem.SupersededBy,
		})
	}

	encoded, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "json error: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(encoded))
	return 0
}
