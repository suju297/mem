package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"mempack/internal/health"
)

func runDoctor(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id or path")
	jsonOut := fs.Bool("json", false, "Output machine-readable JSON")
	repair := fs.Bool("repair", false, "Attempt repairs (migrations, FTS rebuild, missing dirs)")
	verbose := fs.Bool("verbose", false, "Verbose output")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	opts := health.Options{RepoOverride: strings.TrimSpace(*repoOverride)}
	var report health.Report
	var err error
	if *repair {
		report, err = health.Repair(context.Background(), opts.RepoOverride, opts)
	} else {
		report, err = health.Check(context.Background(), opts.RepoOverride, opts)
	}

	if *jsonOut {
		encoded, encErr := json.MarshalIndent(report, "", "  ")
		if encErr != nil {
			fmt.Fprintf(errOut, "json error: %v\n", encErr)
			return 1
		}
		fmt.Fprintln(out, string(encoded))
		if err != nil {
			fmt.Fprintln(errOut, err.Error())
			return 1
		}
		return 0
	}

	writeDoctorReport(out, report, *verbose)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 1
	}
	return 0
}

func writeDoctorReport(out io.Writer, report health.Report, verbose bool) {
	if report.OK {
		fmt.Fprintln(out, "mempack doctor: ok")
	} else if report.Error != "" {
		fmt.Fprintf(out, "mempack doctor: error: %s\n", report.Error)
	} else {
		fmt.Fprintln(out, "mempack doctor: error")
	}

	if report.Repo.ID != "" {
		fmt.Fprintf(out, "repo: %s\n", report.Repo.ID)
	}
	if verbose {
		if report.ActiveRepo != "" {
			fmt.Fprintf(out, "active_repo: %s\n", report.ActiveRepo)
		}
		if report.Repo.Source != "" {
			fmt.Fprintf(out, "repo_source: %s\n", report.Repo.Source)
		}
		if report.Repo.GitRoot != "" {
			fmt.Fprintf(out, "git_root: %s\n", report.Repo.GitRoot)
		}
	}

	if report.DB.Path != "" {
		if report.DB.Exists {
			fmt.Fprintf(out, "db: %s (%d bytes)\n", report.DB.Path, report.DB.SizeBytes)
		} else {
			fmt.Fprintf(out, "db: %s (missing)\n", report.DB.Path)
		}
	}

	if report.Schema.CurrentVersion > 0 || report.Schema.UserVersion > 0 {
		fmt.Fprintf(out, "schema: v%d (current v%d)\n", report.Schema.UserVersion, report.Schema.CurrentVersion)
		if verbose && report.Schema.LastMigrationAt != "" {
			fmt.Fprintf(out, "last_migration_at: %s\n", report.Schema.LastMigrationAt)
		}
	}

	if report.FTS.Memories || report.FTS.Chunks || report.FTS.Rebuilt {
		status := "ok"
		if !report.FTS.Memories || !report.FTS.Chunks {
			status = "missing"
		}
		if report.FTS.Rebuilt {
			status = "rebuilt"
		}
		fmt.Fprintf(out, "fts: %s\n", status)
	}

	if report.State.Repaired {
		fmt.Fprintln(out, "state_current: repaired -> {}")
	} else if !report.State.Valid && len(report.State.InvalidWorkspaces) > 0 {
		fmt.Fprintf(out, "state_current: invalid (workspace=%s)\n", strings.Join(report.State.InvalidWorkspaces, ","))
	} else if report.State.Valid {
		fmt.Fprintln(out, "state_current: ok")
	}

	if report.Suggestion != "" {
		fmt.Fprintf(out, "suggestion: %s\n", report.Suggestion)
	}
}
