package app

import (
	"flag"
	"fmt"
	"io"
	"strings"
)

func runTemplate(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: mem template <agents|...>")
		return 2
	}

	subCmd := strings.ToLower(args[0])
	switch subCmd {
	case "agents":
		return runTemplateAgents(args[1:], out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown template type: %s\n", subCmd)
		return 2
	}
}

func runTemplateAgents(args []string, out, errOut io.Writer) int {
	// Optional flags for future expansion (e.g. style=cursor/codex)
	// For now, simple output
	fs := flag.NewFlagSet("template agents", flag.ContinueOnError)
	fs.SetOutput(errOut)
	write := fs.Bool("write", false, "Write AGENTS.md and .mempack/MEMORY.md to the current directory")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *write {
		result, err := writeAgentFiles("")
		if err != nil {
			fmt.Fprintf(errOut, "failed to write agent files: %v\n", err)
			return 1
		}
		if result.WroteAlternate {
			fmt.Fprintln(out, "Wrote .mempack/MEMORY.md and .mempack/AGENTS.md")
			fmt.Fprintln(out, "AGENTS.md already exists; add the following 2 lines:")
			for _, line := range agentsStubHintLines() {
				fmt.Fprintln(out, line)
			}
		} else {
			fmt.Fprintln(out, "Wrote AGENTS.md and .mempack/MEMORY.md")
		}
		return 0
	}
	fmt.Fprint(out, memoryInstructionsContent())
	return 0
}
