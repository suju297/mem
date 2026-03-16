package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpIncludesASCIILogo(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	code := Run([]string{"help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("help returned %d: %s", code, errOut.String())
	}

	got := out.String()
	wantPrefix := renderMemLogo(false) + "\n\nmem repo-scoped memory CLI\n\n"
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("expected help output to start with logo and title, got %q", got)
	}
}

func TestInitIncludesASCIILogo(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	var out bytes.Buffer
	var errOut bytes.Buffer

	code := Run([]string{"init", "--no-agents"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("init returned %d: %s", code, errOut.String())
	}

	got := out.String()
	if !strings.Contains(got, renderMemLogo(false)) {
		t.Fatalf("expected init output to include logo, got %q", got)
	}
	if !strings.Contains(got, "Initialized memory for repo:") {
		t.Fatalf("expected init output to include init summary, got %q", got)
	}
}

func TestRenderMemLogoColorUsesPurpleAccent(t *testing.T) {
	got := renderMemLogo(true)
	if !strings.Contains(got, "\x1b[35m") {
		t.Fatalf("expected purple accent in colored logo, got %q", got)
	}
	if strings.Contains(got, "\x1b[95m") || strings.Contains(got, "\x1b[96m") {
		t.Fatalf("expected single-color logo accent, got %q", got)
	}
}
