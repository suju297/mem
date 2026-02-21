package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
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
	titleWasSet := flagWasSet(args, "title")
	summaryWasSet := flagWasSet(args, "summary")
	remaining := append([]string{}, positional...)
	id := ""
	if len(remaining) > 0 {
		id = remaining[0]
		remaining = remaining[1:]
	}
	if !titleWasSet && len(remaining) > 0 {
		*title = remaining[0]
		remaining = remaining[1:]
	}
	if !summaryWasSet && len(remaining) > 0 {
		*summary = remaining[0]
		remaining = remaining[1:]
	}
	if len(remaining) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(remaining, " "))
		return 2
	}
	id = strings.TrimSpace(id)
	if id == "" && isInteractiveTerminal(os.Stdin) {
		promptedID, promptErr := promptText(os.Stdin, errOut, "Memory id to supersede", false)
		if promptErr != nil {
			fmt.Fprintf(errOut, "id prompt error: %v\n", promptErr)
			return 1
		}
		id = strings.TrimSpace(promptedID)
	}
	if id == "" {
		fmt.Fprintln(errOut, "missing id")
		return 2
	}
	titleText := strings.TrimSpace(*title)
	if titleText == "" && isInteractiveTerminal(os.Stdin) {
		promptedTitle, promptErr := promptText(os.Stdin, errOut, "New title", false)
		if promptErr != nil {
			fmt.Fprintf(errOut, "title prompt error: %v\n", promptErr)
			return 1
		}
		titleText = strings.TrimSpace(promptedTitle)
	}
	if titleText == "" {
		fmt.Fprintln(errOut, "missing title (use --title or second positional argument)")
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
	entityList := store.NormalizeEntities(store.ParseEntities(*entities))
	if len(entityList) == 0 {
		entityList = parseEntitiesJSON(oldMem.EntitiesJSON)
	}
	entityList = store.NormalizeEntities(entityList)
	if summaryText == "" && !hasSessionTag(tagList) {
		if isInteractiveTerminal(os.Stdin) {
			promptedSummary, promptErr := promptText(os.Stdin, errOut, "New summary", false)
			if promptErr != nil {
				fmt.Fprintf(errOut, "summary prompt error: %v\n", promptErr)
				return 1
			}
			summaryText = strings.TrimSpace(promptedSummary)
		}
		if summaryText == "" {
			fmt.Fprintln(errOut, "missing summary (use --summary or third positional argument)")
			return 2
		}
	}
	tagsJSON := store.TagsToJSON(tagList)
	tagsText := store.TagsText(tagList)
	entitiesJSON := store.EntitiesToJSON(entityList)
	entitiesText := store.EntitiesText(entityList)

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
		Title:         titleText,
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

func parseEntitiesJSON(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var entities []string
	if err := json.Unmarshal([]byte(raw), &entities); err != nil {
		return nil
	}
	return entities
}
