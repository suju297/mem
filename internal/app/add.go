package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"mempack/internal/store"
	"mempack/internal/token"
)

func runAdd(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	fs.SetOutput(errOut)
	threadID := fs.String("thread", "", "Thread id")
	title := fs.String("title", "", "Memory title")
	summary := fs.String("summary", "", "Memory summary")
	tags := fs.String("tags", "", "Comma-separated tags")
	workspace := fs.String("workspace", "", "Workspace name")
	repoOverride := fs.String("repo", "", "Override repo id")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"thread":    {RequiresValue: true},
		"title":     {RequiresValue: true},
		"summary":   {RequiresValue: true},
		"tags":      {RequiresValue: true},
		"workspace": {RequiresValue: true},
		"repo":      {RequiresValue: true},
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

	if strings.TrimSpace(*threadID) == "" {
		fmt.Fprintln(errOut, "missing --thread")
		return 2
	}
	if strings.TrimSpace(*title) == "" {
		fmt.Fprintln(errOut, "missing --title")
		return 2
	}
	summaryText := strings.TrimSpace(*summary)
	if summaryText == "" {
		fmt.Fprintln(errOut, "missing --summary")
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	counter, err := token.New(cfg.Tokenizer)
	if err != nil {
		fmt.Fprintf(errOut, "tokenizer error: %v\n", err)
		return 1
	}

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

	if err := st.EnsureRepo(repoInfo); err != nil {
		fmt.Fprintf(errOut, "store repo error: %v\n", err)
		return 1
	}

	tagList := store.ParseTags(*tags)
	tagList = store.NormalizeTags(tagList)
	tagsJSON := store.TagsToJSON(tagList)
	tagsText := store.TagsText(tagList)

	createdAt := time.Now().UTC()
	anchorCommit := ""
	if repoInfo.HasGit {
		anchorCommit = repoInfo.Head
	}

	summaryTokens := counter.Count(summaryText)
	memory, err := st.AddMemory(store.AddMemoryInput{
		RepoID:        repoInfo.ID,
		Workspace:     workspaceName,
		ThreadID:      strings.TrimSpace(*threadID),
		Title:         strings.TrimSpace(*title),
		Summary:       summaryText,
		SummaryTokens: summaryTokens,
		TagsJSON:      tagsJSON,
		TagsText:      tagsText,
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		AnchorCommit:  anchorCommit,
		CreatedAt:     createdAt,
	})
	if err != nil {
		fmt.Fprintf(errOut, "add memory error: %v\n", err)
		return 1
	}
	if err := maybeEmbedMemory(cfg, st, memory); err != nil {
		fmt.Fprintf(errOut, "embedding warning: %v\n", err)
	}

	resp := map[string]string{
		"id":            memory.ID,
		"thread_id":     memory.ThreadID,
		"title":         memory.Title,
		"anchor_commit": memory.AnchorCommit,
		"created_at":    memory.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "json error: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(encoded))
	return 0
}
