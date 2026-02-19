package app

import "strings"

var promptInjectionPhrases = []string{
	"ignore previous instructions",
	"ignore all previous instructions",
	"disregard previous instructions",
	"jailbreak",
	"bypass safety",
	"do anything now",
}

func containsPromptInjectionPhrase(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range promptInjectionPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}
