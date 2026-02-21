package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptTextRequiresValue(t *testing.T) {
	var out bytes.Buffer
	value, err := promptText(strings.NewReader("\nhello\n"), &out, "Title", false)
	if err != nil {
		t.Fatalf("promptText error: %v", err)
	}
	if value != "hello" {
		t.Fatalf("expected value hello, got %q", value)
	}
	if !strings.Contains(out.String(), "This value is required.") {
		t.Fatalf("expected required message, got %q", out.String())
	}
}

func TestPromptTextAllowsEmpty(t *testing.T) {
	var out bytes.Buffer
	value, err := promptText(strings.NewReader("\n"), &out, "Optional", true)
	if err != nil {
		t.Fatalf("promptText error: %v", err)
	}
	if value != "" {
		t.Fatalf("expected empty value, got %q", value)
	}
}
