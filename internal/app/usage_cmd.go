package app

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"mem/internal/pack"
)

type usageResponse struct {
	Scope   string            `json:"scope"`
	RepoID  string            `json:"repo_id,omitempty"`
	Repo    *pack.UsageTotals `json:"repo,omitempty"`
	Overall pack.UsageTotals  `json:"overall"`
}

func runUsage(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	fs.SetOutput(errOut)
	format := fs.String("format", "text", "Output format: text|json")
	repoOverride := fs.String("repo", "", "Override repo id or path")
	profile := fs.Bool("me", false, "Show profile-wide usage totals")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	outputFormat := strings.ToLower(strings.TrimSpace(*format))
	if outputFormat != "json" && outputFormat != "text" {
		fmt.Fprintf(errOut, "unsupported format: %s\n", *format)
		return 2
	}
	if *profile && strings.TrimSpace(*repoOverride) != "" {
		fmt.Fprintln(errOut, "cannot combine --me with --repo")
		return 2
	}

	var (
		report usageResponse
		err    error
	)
	if *profile {
		report, err = loadProfileUsageReport()
	} else {
		report, err = loadUsageReport(strings.TrimSpace(*repoOverride), true)
	}
	if err != nil {
		fmt.Fprintf(errOut, "%s\n", formatUsageError(err, strings.TrimSpace(*repoOverride), *profile))
		return 1
	}
	if outputFormat == "json" {
		return writeJSON(out, errOut, report)
	}
	writeUsageSummary(out, report)
	return 0
}

func formatUsageError(err error, repoOverride string, profile bool) string {
	if err == nil {
		return ""
	}
	if profile {
		return err.Error()
	}
	if strings.TrimSpace(repoOverride) == "" && strings.Contains(err.Error(), "repo not specified and could not detect repo from current directory") {
		return "current directory is not inside a repo. Run 'mem usage --repo /path/to/repo' for repo usage, or 'mem usage --me' for profile totals."
	}
	return err.Error()
}

func writeUsageSummary(w io.Writer, report usageResponse) {
	if report.Scope == "profile" {
		fmt.Fprintf(w, "All your repos: %s\n", formatSavedTokens(report.Overall.SavedTokens))
		if report.Overall.RequestCount == 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w, "No saved-token history yet.")
		}
		return
	}

	repoTotals := report.Repo
	if repoTotals == nil {
		repoTotals = &pack.UsageTotals{}
	}

	fmt.Fprintf(w, "This repo: %s\n", formatSavedTokens(repoTotals.SavedTokens))
	fmt.Fprintf(w, "All your repos: %s\n", formatSavedTokens(report.Overall.SavedTokens))
	if repoTotals.RequestCount == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "No saved-token history for this repo yet.")
	}
}

func formatSavedTokens(tokens int) string {
	if tokens == 1 {
		return "1 token saved"
	}
	return fmt.Sprintf("%d tokens saved", tokens)
}
