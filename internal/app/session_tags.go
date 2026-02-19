package app

import "strings"

func hasSessionTag(tags []string) bool {
	for _, tag := range tags {
		if strings.EqualFold(tag, "session") || strings.EqualFold(tag, "needs_summary") {
			return true
		}
	}
	return false
}
