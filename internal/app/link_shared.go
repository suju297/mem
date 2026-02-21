package app

import (
	"fmt"
	"strings"
	"unicode"

	"mempack/internal/store"
)

func normalizeLinkRelation(raw string) (string, error) {
	rel := strings.TrimSpace(strings.ToLower(raw))
	rel = strings.ReplaceAll(rel, " ", "_")
	if rel == "" {
		return "", fmt.Errorf("missing relation")
	}
	if len(rel) > 64 {
		return "", fmt.Errorf("relation too long (max 64 chars)")
	}
	for _, r := range rel {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == ':' {
			continue
		}
		return "", fmt.Errorf("relation contains invalid character: %q", r)
	}
	return rel, nil
}

func ensureMemoryExistsForLink(st *store.Store, repoID, workspace, id, side string) (store.Memory, error) {
	mem, err := st.GetMemory(repoID, workspace, id)
	if err != nil {
		return store.Memory{}, fmt.Errorf("%s memory not found: %s", side, id)
	}
	if !mem.DeletedAt.IsZero() {
		return store.Memory{}, fmt.Errorf("%s memory is deleted: %s", side, id)
	}
	if strings.TrimSpace(mem.SupersededBy) != "" {
		return store.Memory{}, fmt.Errorf("%s memory is superseded: %s", side, id)
	}
	return mem, nil
}
