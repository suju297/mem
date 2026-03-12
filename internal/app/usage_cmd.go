package app

import (
	"flag"
	"fmt"
	"io"
	"strings"

	"mem/internal/pack"
)

type usageResponse struct {
	RepoID  string           `json:"repo_id"`
	Repo    pack.UsageTotals `json:"repo"`
	Overall pack.UsageTotals `json:"overall"`
}

func runUsage(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("usage", flag.ContinueOnError)
	fs.SetOutput(errOut)
	format := fs.String("format", "json", "Output format: json")
	repoOverride := fs.String("repo", "", "Override repo id or path")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(errOut, "unexpected args: %s\n", strings.Join(fs.Args(), " "))
		return 2
	}
	if strings.ToLower(strings.TrimSpace(*format)) != "json" {
		fmt.Fprintf(errOut, "unsupported format: %s\n", *format)
		return 2
	}

	report, err := loadUsageReport(strings.TrimSpace(*repoOverride), false)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return 1
	}
	return writeJSON(out, errOut, report)
}
