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
	threadID := fs.String("thread", "", "Thread id (optional; defaults to default_thread or T-SESSION)")
	title := fs.String("title", "", "Memory title")
	summary := fs.String("summary", "", "Memory summary")
	tags := fs.String("tags", "", "Comma-separated tags")
	entities := fs.String("entities", "", "Comma-separated entities")
	workspace := fs.String("workspace", "", "Workspace name")
	repoOverride := fs.String("repo", "", "Override repo id")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"thread":    {RequiresValue: true},
		"title":     {RequiresValue: true},
		"summary":   {RequiresValue: true},
		"tags":      {RequiresValue: true},
		"entities":  {RequiresValue: true},
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

	if strings.TrimSpace(*title) == "" {
		fmt.Fprintln(errOut, "missing --title")
		return 2
	}
	summaryText := strings.TrimSpace(*summary)

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

	repoInfo, err := resolveRepo(&cfg, strings.TrimSpace(*repoOverride))
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}
	threadUsed, threadDefaulted, err := resolveThread(cfg, strings.TrimSpace(*threadID))
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
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
	if summaryText == "" && !hasSessionTag(tagList) {
		fmt.Fprintln(errOut, "missing --summary")
		return 2
	}
	tagsJSON := store.TagsToJSON(tagList)
	tagsText := store.TagsText(tagList)
	entityList := store.ParseEntities(*entities)
	entityList = store.NormalizeEntities(entityList)
	entitiesJSON := store.EntitiesToJSON(entityList)
	entitiesText := store.EntitiesText(entityList)

	createdAt := time.Now().UTC()
	anchorCommit := ""
	if repoInfo.HasGit {
		anchorCommit = repoInfo.Head
	}

	summaryTokens := counter.Count(summaryText)
	memory, err := st.AddMemory(store.AddMemoryInput{
		RepoID:        repoInfo.ID,
		Workspace:     workspaceName,
		ThreadID:      threadUsed,
		Title:         strings.TrimSpace(*title),
		Summary:       summaryText,
		SummaryTokens: summaryTokens,
		TagsJSON:      tagsJSON,
		TagsText:      tagsText,
		EntitiesJSON:  entitiesJSON,
		EntitiesText:  entitiesText,
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

	resp := map[string]any{
		"id":               memory.ID,
		"thread_id":        memory.ThreadID,
		"thread_used":      memory.ThreadID,
		"thread_defaulted": threadDefaulted,
		"title":            memory.Title,
		"anchor_commit":    memory.AnchorCommit,
		"created_at":       memory.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "json error: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(encoded))
	return 0
}
