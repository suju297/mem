package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type ThreadItem struct {
	ThreadID    string `json:"thread_id"`
	Title       string `json:"title,omitempty"`
	TagsJSON    string `json:"tags_json,omitempty"`
	CreatedAt   string `json:"created_at"`
	MemoryCount int    `json:"memory_count"`
}

type ThreadShowResponse struct {
	Thread   ThreadItem          `json:"thread"`
	Memories []ThreadMemoryBrief `json:"memories"`
}

type ThreadMemoryBrief struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	CreatedAt    string `json:"created_at"`
	AnchorCommit string `json:"anchor_commit,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`
}

func runThreads(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("threads", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id")
	workspace := fs.String("workspace", "", "Workspace name")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"repo":      {RequiresValue: true},
		"workspace": {RequiresValue: true},
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

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	repoInfo, err := resolveRepo(cfg, strings.TrimSpace(*repoOverride))
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

	threads, err := st.ListThreads(repoInfo.ID, workspaceName)
	if err != nil {
		fmt.Fprintf(errOut, "threads error: %v\n", err)
		return 1
	}

	items := make([]ThreadItem, 0, len(threads))
	for _, thread := range threads {
		items = append(items, ThreadItem{
			ThreadID:    thread.ThreadID,
			Title:       thread.Title,
			TagsJSON:    thread.TagsJSON,
			CreatedAt:   thread.CreatedAt.UTC().Format(time.RFC3339Nano),
			MemoryCount: thread.MemoryCount,
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

func runThreadShow(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("thread", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id")
	limit := fs.Int("limit", 20, "Max memories to show")
	workspace := fs.String("workspace", "", "Workspace name")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"repo":      {RequiresValue: true},
		"limit":     {RequiresValue: true},
		"workspace": {RequiresValue: true},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	threadID := strings.TrimSpace(strings.Join(positional, " "))
	if threadID == "" {
		fmt.Fprintln(errOut, "missing thread id")
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	repoInfo, err := resolveRepo(cfg, strings.TrimSpace(*repoOverride))
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

	thread, err := st.GetThread(repoInfo.ID, workspaceName, threadID)
	if err != nil {
		fmt.Fprintf(errOut, "thread not found: %s\n", threadID)
		return 1
	}

	memories, err := st.ListMemoriesByThread(repoInfo.ID, workspaceName, threadID, *limit)
	if err != nil {
		fmt.Fprintf(errOut, "thread memories error: %v\n", err)
		return 1
	}

	memoryCount := thread.MemoryCount
	if memoryCount < len(memories) {
		memoryCount = len(memories)
	}

	resp := ThreadShowResponse{
		Thread: ThreadItem{
			ThreadID:    thread.ThreadID,
			Title:       thread.Title,
			TagsJSON:    thread.TagsJSON,
			CreatedAt:   thread.CreatedAt.UTC().Format(time.RFC3339Nano),
			MemoryCount: memoryCount,
		},
	}

	for _, mem := range memories {
		resp.Memories = append(resp.Memories, ThreadMemoryBrief{
			ID:           mem.ID,
			Title:        mem.Title,
			Summary:      mem.Summary,
			CreatedAt:    mem.CreatedAt.UTC().Format(time.RFC3339Nano),
			AnchorCommit: mem.AnchorCommit,
			SupersededBy: mem.SupersededBy,
		})
	}

	encoded, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "json error: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(encoded))
	return 0
}
