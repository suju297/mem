package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type updateResponse struct {
	ID          string `json:"id"`
	ThreadID    string `json:"thread_id"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	OperationAt string `json:"operation_at"`
}

func runUpdate(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(errOut)
	title := fs.String("title", "", "Memory title")
	summary := fs.String("summary", "", "Memory summary")
	tags := fs.String("tags", "", "Replace tags (comma-separated)")
	tagsAdd := fs.String("tags-add", "", "Add tags (comma-separated)")
	tagsRemove := fs.String("tags-remove", "", "Remove tags (comma-separated)")
	entities := fs.String("entities", "", "Replace entities (comma-separated)")
	entitiesAdd := fs.String("entities-add", "", "Add entities (comma-separated)")
	entitiesRemove := fs.String("entities-remove", "", "Remove entities (comma-separated)")
	workspace := fs.String("workspace", "", "Workspace name")
	repoOverride := fs.String("repo", "", "Override repo id")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"title":           {RequiresValue: true},
		"summary":         {RequiresValue: true},
		"tags":            {RequiresValue: true},
		"tags-add":        {RequiresValue: true},
		"tags-remove":     {RequiresValue: true},
		"entities":        {RequiresValue: true},
		"entities-add":    {RequiresValue: true},
		"entities-remove": {RequiresValue: true},
		"workspace":       {RequiresValue: true},
		"repo":            {RequiresValue: true},
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

	flags := updateFieldFlagsFromCLIArgs(args)
	if !flags.Any() {
		fmt.Fprintln(errOut, "no update fields provided")
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

	updateInput, err := makeUpdateMemoryInput(
		repoInfo.ID,
		workspaceName,
		id,
		cfg.Tokenizer,
		flags,
		updateFieldValues{
			Title:          *title,
			Summary:        *summary,
			Tags:           *tags,
			TagsAdd:        *tagsAdd,
			TagsRemove:     *tagsRemove,
			Entities:       *entities,
			EntitiesAdd:    *entitiesAdd,
			EntitiesRemove: *entitiesRemove,
		},
	)
	if err != nil {
		fmt.Fprintf(errOut, "tokenizer error: %v\n", err)
		return 1
	}

	mem, changed, err := st.UpdateMemoryWithStatus(updateInput)
	if err != nil {
		fmt.Fprintf(errOut, "update memory error: %v\n", err)
		return 1
	}
	if changed {
		if err := maybeEmbedMemory(cfg, st, mem); err != nil {
			fmt.Fprintf(errOut, "embedding warning: %v\n", err)
		}
	}

	resp := updateResponse{
		ID:          mem.ID,
		ThreadID:    mem.ThreadID,
		Title:       mem.Title,
		Summary:     mem.Summary,
		OperationAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	encoded, err := json.MarshalIndent(resp, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "json error: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(encoded))
	return 0
}
