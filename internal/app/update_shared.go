package app

import (
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"mempack/internal/store"
	"mempack/internal/token"
)

type updateFieldFlags struct {
	Title          bool
	Summary        bool
	Tags           bool
	TagsAdd        bool
	TagsRemove     bool
	Entities       bool
	EntitiesAdd    bool
	EntitiesRemove bool
}

type updateFieldValues struct {
	Title          string
	Summary        string
	Tags           string
	TagsAdd        string
	TagsRemove     string
	Entities       string
	EntitiesAdd    string
	EntitiesRemove string
}

func updateFieldFlagsFromCLIArgs(args []string) updateFieldFlags {
	return updateFieldFlags{
		Title:          flagWasSet(args, "title"),
		Summary:        flagWasSet(args, "summary"),
		Tags:           flagWasSet(args, "tags"),
		TagsAdd:        flagWasSet(args, "tags-add"),
		TagsRemove:     flagWasSet(args, "tags-remove"),
		Entities:       flagWasSet(args, "entities"),
		EntitiesAdd:    flagWasSet(args, "entities-add"),
		EntitiesRemove: flagWasSet(args, "entities-remove"),
	}
}

func updateFieldFlagsFromMCPRequest(request mcp.CallToolRequest) updateFieldFlags {
	return updateFieldFlags{
		Title:          requestHasArg(request, "title"),
		Summary:        requestHasArg(request, "summary"),
		Tags:           requestHasArg(request, "tags"),
		TagsAdd:        requestHasArg(request, "tags_add"),
		TagsRemove:     requestHasArg(request, "tags_remove"),
		Entities:       requestHasArg(request, "entities"),
		EntitiesAdd:    requestHasArg(request, "entities_add"),
		EntitiesRemove: requestHasArg(request, "entities_remove"),
	}
}

func (flags updateFieldFlags) Any() bool {
	return flags.Title ||
		flags.Summary ||
		flags.Tags ||
		flags.TagsAdd ||
		flags.TagsRemove ||
		flags.Entities ||
		flags.EntitiesAdd ||
		flags.EntitiesRemove
}

func makeUpdateMemoryInput(
	repoID string,
	workspace string,
	id string,
	tokenizerEncoding string,
	flags updateFieldFlags,
	values updateFieldValues,
) (store.UpdateMemoryInput, error) {
	trimmedTitle := strings.TrimSpace(values.Title)
	trimmedSummary := strings.TrimSpace(values.Summary)

	var titlePtr *string
	if flags.Title {
		titlePtr = &trimmedTitle
	}

	var summaryPtr *string
	var summaryTokens *int
	if flags.Summary {
		counter, err := token.New(tokenizerEncoding)
		if err != nil {
			return store.UpdateMemoryInput{}, err
		}
		count := counter.Count(trimmedSummary)
		summaryPtr = &trimmedSummary
		summaryTokens = &count
	}

	return store.UpdateMemoryInput{
		RepoID:         repoID,
		Workspace:      workspace,
		ID:             id,
		Title:          titlePtr,
		Summary:        summaryPtr,
		SummaryTokens:  summaryTokens,
		TagsSet:        flags.Tags,
		Tags:           store.ParseTags(values.Tags),
		TagsAdd:        store.ParseTags(values.TagsAdd),
		TagsRemove:     store.ParseTags(values.TagsRemove),
		EntitiesSet:    flags.Entities,
		Entities:       store.ParseEntities(values.Entities),
		EntitiesAdd:    store.ParseEntities(values.EntitiesAdd),
		EntitiesRemove: store.ParseEntities(values.EntitiesRemove),
	}, nil
}
