package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type SessionItem struct {
	ID           string `json:"id"`
	ThreadID     string `json:"thread_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	CreatedAt    string `json:"created_at"`
	AnchorCommit string `json:"anchor_commit,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`
}

type SessionCount struct {
	Count int `json:"count"`
}

func runSessions(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id")
	workspace := fs.String("workspace", "", "Workspace name")
	limit := fs.Int("limit", 20, "Max sessions to show")
	format := fs.String("format", "json", "Output format: json")
	needsSummary := fs.Bool("needs-summary", false, "Only sessions tagged needs_summary")
	countOnly := fs.Bool("count", false, "Return count only")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"repo":          {RequiresValue: true},
		"workspace":     {RequiresValue: true},
		"limit":         {RequiresValue: true},
		"format":        {RequiresValue: true},
		"needs-summary": {RequiresValue: false},
		"count":         {RequiresValue: false},
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

	if *countOnly {
		total, err := st.CountSessionMemories(repoInfo.ID, workspaceName, *needsSummary)
		if err != nil {
			fmt.Fprintf(errOut, "sessions count error: %v\n", err)
			return 1
		}
		encoded, err := json.MarshalIndent(SessionCount{Count: total}, "", "  ")
		if err != nil {
			fmt.Fprintf(errOut, "json error: %v\n", err)
			return 1
		}
		fmt.Fprintln(out, string(encoded))
		return 0
	}

	sessions, err := st.ListSessionMemories(repoInfo.ID, workspaceName, *limit, *needsSummary)
	if err != nil {
		fmt.Fprintf(errOut, "sessions error: %v\n", err)
		return 1
	}

	items := make([]SessionItem, 0, len(sessions))
	for _, mem := range sessions {
		items = append(items, SessionItem{
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
