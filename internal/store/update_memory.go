package store

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func (s *Store) UpdateMemory(input UpdateMemoryInput) (Memory, error) {
	mem, _, err := s.UpdateMemoryWithStatus(input)
	return mem, err
}

func (s *Store) UpdateMemoryWithStatus(input UpdateMemoryInput) (Memory, bool, error) {
	if strings.TrimSpace(input.RepoID) == "" || strings.TrimSpace(input.ID) == "" {
		return Memory{}, false, fmt.Errorf("update requires repo_id and id")
	}
	workspace := normalizeWorkspace(input.Workspace)

	mem, err := s.GetMemory(input.RepoID, workspace, input.ID)
	if err != nil {
		return Memory{}, false, err
	}
	if !mem.DeletedAt.IsZero() {
		return Memory{}, false, ErrNotFound
	}

	changed := false

	newTitle := mem.Title
	if input.Title != nil {
		trimmed := strings.TrimSpace(*input.Title)
		newTitle = trimmed
		if newTitle != mem.Title {
			changed = true
		}
	}

	newSummary := mem.Summary
	newSummaryTokens := mem.SummaryTokens
	if input.Summary != nil {
		if input.SummaryTokens == nil {
			return Memory{}, false, fmt.Errorf("summary_tokens required when updating summary")
		}
		trimmed := strings.TrimSpace(*input.Summary)
		newSummary = trimmed
		newSummaryTokens = *input.SummaryTokens
		if newSummary != mem.Summary || newSummaryTokens != mem.SummaryTokens {
			changed = true
		}
	}

	newTagsJSON := mem.TagsJSON
	newTagsText := mem.TagsText
	if input.TagsSet || len(input.TagsAdd) > 0 || len(input.TagsRemove) > 0 {
		existingTags := parseStringListJSON(mem.TagsJSON)
		if len(existingTags) == 0 && strings.TrimSpace(mem.TagsText) != "" {
			existingTags = strings.Fields(mem.TagsText)
		}
		existingTags = NormalizeTags(existingTags)

		var nextTags []string
		if input.TagsSet {
			nextTags = NormalizeTags(input.Tags)
		} else {
			nextTags = applyAddRemove(existingTags, NormalizeTags(input.TagsAdd), NormalizeTags(input.TagsRemove))
		}
		if !reflect.DeepEqual(existingTags, nextTags) {
			newTagsJSON = TagsToJSON(nextTags)
			newTagsText = TagsText(nextTags)
			changed = true
		}
	}

	newEntitiesJSON := mem.EntitiesJSON
	newEntitiesText := mem.EntitiesText
	if input.EntitiesSet || len(input.EntitiesAdd) > 0 || len(input.EntitiesRemove) > 0 {
		existingEntities := parseStringListJSON(mem.EntitiesJSON)
		if len(existingEntities) == 0 && strings.TrimSpace(mem.EntitiesText) != "" {
			existingEntities = strings.Fields(mem.EntitiesText)
		}
		existingEntities = NormalizeEntities(existingEntities)

		var nextEntities []string
		if input.EntitiesSet {
			nextEntities = NormalizeEntities(input.Entities)
		} else {
			nextEntities = applyAddRemove(existingEntities, NormalizeEntities(input.EntitiesAdd), NormalizeEntities(input.EntitiesRemove))
		}
		if !reflect.DeepEqual(existingEntities, nextEntities) {
			newEntitiesJSON = EntitiesToJSON(nextEntities)
			newEntitiesText = EntitiesText(nextEntities)
			changed = true
		}
	}

	if !changed {
		return mem, false, nil
	}

	_, err = s.db.Exec(`
		UPDATE memories
		SET title = ?, summary = ?, summary_tokens = ?, tags_json = ?, tags_text = ?, entities_json = ?, entities_text = ?
		WHERE id = ? AND repo_id = ? AND workspace = ?
	`, newTitle, newSummary, newSummaryTokens, newTagsJSON, newTagsText, newEntitiesJSON, newEntitiesText, mem.ID, mem.RepoID, workspace)
	if err != nil {
		return Memory{}, false, err
	}

	mem.Title = newTitle
	mem.Summary = newSummary
	mem.SummaryTokens = newSummaryTokens
	mem.TagsJSON = newTagsJSON
	mem.TagsText = newTagsText
	mem.EntitiesJSON = newEntitiesJSON
	mem.EntitiesText = newEntitiesText
	return mem, true, nil
}

func parseStringListJSON(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(value), &out); err != nil {
		return nil
	}
	return out
}

func applyAddRemove(base, add, remove []string) []string {
	removeSet := make(map[string]struct{}, len(remove))
	for _, item := range remove {
		removeSet[item] = struct{}{}
	}
	seen := make(map[string]struct{}, len(base))
	var out []string
	for _, item := range base {
		if _, ok := removeSet[item]; ok {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	for _, item := range add {
		if _, ok := removeSet[item]; ok {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
