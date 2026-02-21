package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"mempack/internal/store"
	"mempack/internal/token"
)

type sessionUpsertResponse struct {
	ID        string `json:"id"`
	Action    string `json:"action"`
	Created   bool   `json:"created"`
	Updated   bool   `json:"updated"`
	ThreadID  string `json:"thread_id"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	UpdatedAt string `json:"updated_at"`
}

func runSession(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "missing session subcommand (supported: upsert)")
		return 2
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "upsert":
		return runSessionUpsert(args[1:], out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown session subcommand: %s\n", args[0])
		return 2
	}
}

func runSessionUpsert(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("session upsert", flag.ContinueOnError)
	fs.SetOutput(errOut)
	title := fs.String("title", "", "Session title")
	summary := fs.String("summary", "", "Session summary")
	threadID := fs.String("thread", "", "Thread id")
	tags := fs.String("tags", "session", "Session tags (comma-separated)")
	entities := fs.String("entities", "", "Session entities (comma-separated)")
	workspace := fs.String("workspace", "", "Workspace name")
	repoOverride := fs.String("repo", "", "Override repo id")
	format := fs.String("format", "json", "Output format: json")
	mergeWindowMS := fs.Int("merge-window-ms", 300000, "Merge into latest session if created within this window")
	minGapMS := fs.Int("min-gap-ms", 300000, "Minimum gap to create a new session")
	onlyAutoLatest := fs.Bool("only-auto-latest", true, "Only merge if latest session title starts with 'Session:'")

	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"title":            {RequiresValue: true},
		"summary":          {RequiresValue: true},
		"thread":           {RequiresValue: true},
		"tags":             {RequiresValue: true},
		"entities":         {RequiresValue: true},
		"workspace":        {RequiresValue: true},
		"repo":             {RequiresValue: true},
		"format":           {RequiresValue: true},
		"merge-window-ms":  {RequiresValue: true},
		"min-gap-ms":       {RequiresValue: true},
		"only-auto-latest": {RequiresValue: false},
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
	if !titleWasSet && len(positional) > 0 {
		*title = positional[0]
		positional = positional[1:]
	}
	if !summaryWasSet && len(positional) > 0 {
		*summary = positional[0]
		positional = positional[1:]
	}
	if len(positional) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(positional, " "))
		return 2
	}
	if strings.TrimSpace(*format) != "json" {
		fmt.Fprintf(errOut, "unsupported format: %s\n", *format)
		return 2
	}
	titleValue := strings.TrimSpace(*title)
	if titleValue == "" && isInteractiveTerminal(os.Stdin) {
		promptedTitle, promptErr := promptText(os.Stdin, errOut, "Session title", false)
		if promptErr != nil {
			fmt.Fprintf(errOut, "title prompt error: %v\n", promptErr)
			return 1
		}
		titleValue = strings.TrimSpace(promptedTitle)
	}
	if titleValue == "" {
		fmt.Fprintln(errOut, "missing title (use --title or first positional argument)")
		return 2
	}
	if *mergeWindowMS < 0 || *minGapMS < 0 {
		fmt.Fprintln(errOut, "merge-window-ms and min-gap-ms must be >= 0")
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

	now := time.Now().UTC()
	summaryValue := strings.TrimSpace(*summary)
	tagList := store.NormalizeTags(store.ParseTags(*tags))
	if !hasSessionTag(tagList) {
		tagList = append(tagList, "session")
		tagList = store.NormalizeTags(tagList)
	}
	entityList := store.NormalizeEntities(store.ParseEntities(*entities))

	latestList, err := st.ListSessionMemories(repoInfo.ID, workspaceName, 1, false)
	if err != nil {
		fmt.Fprintf(errOut, "session lookup error: %v\n", err)
		return 1
	}

	if len(latestList) > 0 {
		latest := latestList[0]
		latestIsAuto := strings.HasPrefix(strings.ToLower(strings.TrimSpace(latest.Title)), "session:")
		withinMergeWindow := *mergeWindowMS > 0 && now.Sub(latest.CreatedAt) <= time.Duration(*mergeWindowMS)*time.Millisecond
		withinGap := *minGapMS > 0 && now.Sub(latest.CreatedAt) <= time.Duration(*minGapMS)*time.Millisecond
		shouldMerge := withinMergeWindow || withinGap
		if *onlyAutoLatest && !latestIsAuto {
			shouldMerge = false
		}
		if shouldMerge {
			updateInput := store.UpdateMemoryInput{
				RepoID:      repoInfo.ID,
				Workspace:   workspaceName,
				ID:          latest.ID,
				Title:       &titleValue,
				TagsAdd:     tagList,
				EntitiesAdd: entityList,
			}
			if flagWasSet(args, "summary") {
				counter, err := token.New(cfg.Tokenizer)
				if err != nil {
					fmt.Fprintf(errOut, "tokenizer error: %v\n", err)
					return 1
				}
				tokens := counter.Count(summaryValue)
				updateInput.Summary = &summaryValue
				updateInput.SummaryTokens = &tokens
			}
			mem, changed, err := st.UpdateMemoryWithStatus(updateInput)
			if err != nil {
				fmt.Fprintf(errOut, "session update error: %v\n", err)
				return 1
			}
			if changed {
				if err := maybeEmbedMemory(cfg, st, mem); err != nil {
					fmt.Fprintf(errOut, "embedding warning: %v\n", err)
				}
			}
			return writeJSON(out, errOut, sessionUpsertResponse{
				ID:        mem.ID,
				Action:    "updated",
				Created:   false,
				Updated:   true,
				ThreadID:  mem.ThreadID,
				Title:     mem.Title,
				Summary:   mem.Summary,
				UpdatedAt: now.Format(time.RFC3339Nano),
			})
		}
	}

	threadUsed, _, err := resolveThread(cfg, strings.TrimSpace(*threadID))
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	counter, err := token.New(cfg.Tokenizer)
	if err != nil {
		fmt.Fprintf(errOut, "tokenizer error: %v\n", err)
		return 1
	}
	anchorCommit := ""
	if repoInfo.HasGit {
		anchorCommit = repoInfo.Head
	}
	mem, err := st.AddMemory(store.AddMemoryInput{
		RepoID:        repoInfo.ID,
		Workspace:     workspaceName,
		ThreadID:      threadUsed,
		Title:         titleValue,
		Summary:       summaryValue,
		SummaryTokens: counter.Count(summaryValue),
		TagsJSON:      store.TagsToJSON(tagList),
		TagsText:      store.TagsText(tagList),
		EntitiesJSON:  store.EntitiesToJSON(entityList),
		EntitiesText:  store.EntitiesText(entityList),
		AnchorCommit:  anchorCommit,
		CreatedAt:     now,
	})
	if err != nil {
		fmt.Fprintf(errOut, "session create error: %v\n", err)
		return 1
	}
	if err := maybeEmbedMemory(cfg, st, mem); err != nil {
		fmt.Fprintf(errOut, "embedding warning: %v\n", err)
	}
	return writeJSON(out, errOut, sessionUpsertResponse{
		ID:        mem.ID,
		Action:    "created",
		Created:   true,
		Updated:   false,
		ThreadID:  mem.ThreadID,
		Title:     mem.Title,
		Summary:   mem.Summary,
		UpdatedAt: now.Format(time.RFC3339Nano),
	})
}
