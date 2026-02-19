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
	write := fs.Bool("write", false, "Write .mempack/MEMORY.md and assistant stub files to the current directory")
	assistantsFlag := fs.String("assistants", "agents", "Comma-separated assistant stubs to write: agents,claude,gemini,all")
	noMemory := fs.Bool("no-memory", false, "Skip writing .mempack/MEMORY.md")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	targets, err := parseAssistantStubTargets(*assistantsFlag)
	if err != nil {
		fmt.Fprintf(errOut, "invalid --assistants: %v\n", err)
		return 2
	}

	if *write {
		result, err := writeAgentFiles("", targets, !*noMemory)
		if err != nil {
			fmt.Fprintf(errOut, "failed to write agent files: %v\n", err)
			return 1
		}
		if *noMemory {
			fmt.Fprintf(out, "Wrote assistant stubs: %s (when missing).\n", assistantStubTargetsLabel(targets))
		} else {
			fmt.Fprintf(out, "Wrote .mempack/MEMORY.md and assistant stubs: %s (when missing).\n", assistantStubTargetsLabel(targets))
		}
		if result.WroteAlternate {
			fmt.Fprintln(out, "AGENTS.md already exists; wrote .mempack/AGENTS.md. Add the following 2 lines to AGENTS.md:")
			for _, line := range agentsStubHintLines() {
				fmt.Fprintln(out, line)
			}
		}
		return 0
	}
	fmt.Fprint(out, memoryInstructionsContent())
	return 0
}
