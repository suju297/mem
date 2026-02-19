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

type SupersedeResponse struct {
	OldID  string `json:"old_id"`
	NewID  string `json:"new_id"`
	Status string `json:"status"`
}

func runSupersede(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("supersede", flag.ContinueOnError)
	fs.SetOutput(errOut)
	threadID := fs.String("thread", "", "Thread id (default: old thread)")
	title := fs.String("title", "", "New memory title")
	summary := fs.String("summary", "", "New memory summary")
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

	id := strings.TrimSpace(strings.Join(positional, " "))
	if id == "" {
		fmt.Fprintln(errOut, "missing id")
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

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		fmt.Fprintf(errOut, "store open error: %v\n", err)
		return 1
	}
	defer st.Close()

	oldMem, err := st.GetMemory(repoInfo.ID, workspaceName, id)
	if err != nil {
		fmt.Fprintf(errOut, "memory not found: %s\n", id)
		return 1
	}

	thread := strings.TrimSpace(*threadID)
	if thread == "" {
		thread = oldMem.ThreadID
	}
	if thread == "" {
		fmt.Fprintln(errOut, "missing --thread (no existing thread to inherit)")
		return 2
	}

	tagList := store.ParseTags(*tags)
	if len(tagList) == 0 {
		tagList = parseTagsJSON(oldMem.TagsJSON)
	}
	tagList = store.NormalizeTags(tagList)
	if summaryText == "" && !hasSessionTag(tagList) {
		fmt.Fprintln(errOut, "missing --summary")
		return 2
	}
	tagsJSON := store.TagsToJSON(tagList)
	tagsText := store.TagsText(tagList)

	createdAt := time.Now().UTC()
	anchorCommit := strings.TrimSpace(oldMem.AnchorCommit)
	if anchorCommit == "" && repoInfo.HasGit {
		anchorCommit = repoInfo.Head
	}

	summaryTokens := counter.Count(summaryText)
	newMem, err := st.AddMemory(store.AddMemoryInput{
		RepoID:        repoInfo.ID,
		Workspace:     workspaceName,
		ThreadID:      thread,
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
		fmt.Fprintf(errOut, "supersede error: %v\n", err)
		return 1
	}

	if err := st.MarkMemorySuperseded(repoInfo.ID, workspaceName, oldMem.ID, newMem.ID); err != nil {
		fmt.Fprintf(errOut, "supersede link error: %v\n", err)
		return 1
	}

	linkTime := createdAt
	if err := st.AddLink(store.Link{
		FromID:    oldMem.ID,
		Rel:       "superseded_by",
		ToID:      newMem.ID,
		Weight:    1,
		CreatedAt: linkTime,
	}); err != nil {
		fmt.Fprintf(errOut, "supersede link error: %v\n", err)
		return 1
	}
	if err := st.AddLink(store.Link{
		FromID:    newMem.ID,
		Rel:       "supersedes",
		ToID:      oldMem.ID,
		Weight:    1,
		CreatedAt: linkTime,
	}); err != nil {
		fmt.Fprintf(errOut, "supersede link error: %v\n", err)
		return 1
	}

	return writeJSON(out, errOut, SupersedeResponse{OldID: oldMem.ID, NewID: newMem.ID, Status: "superseded"})
}

func parseTagsJSON(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return nil
	}
	return tags
}
