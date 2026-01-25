package app

import (
	"fmt"
	"io"
	"strings"

	"mempack/internal/config"
)

func Run(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		writeUsage(out)
		return 2
	}

	parsedArgs, globals, err := splitGlobalFlags(args)
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		writeUsage(errOut)
		return 2
	}
	if strings.TrimSpace(globals.DataDir) != "" {
		config.SetDataDirOverride(globals.DataDir)
	}
	args = parsedArgs
	if len(args) == 0 {
		writeUsage(out)
		return 2
	}

	if isVersionCommand(args[0]) {
		fmt.Fprintln(out, VersionString())
		return 0
	}

	cmd := strings.ToLower(args[0])
	switch cmd {
	case "init":
		return runInit(args[1:], out, errOut)
	case "get":
		return runGet(args[1:], out, errOut)
	case "add":
		return runAdd(args[1:], out, errOut)
	case "explain":
		return runExplain(args[1:], out, errOut)
	case "show":
		return runShow(args[1:], out, errOut)
	case "forget":
		return runForget(args[1:], out, errOut)
	case "supersede":
		return runSupersede(args[1:], out, errOut)
	case "checkpoint":
		return runCheckpoint(args[1:], out, errOut)
	case "repos":
		return runRepos(args[1:], out, errOut)
	case "use":
		return runUse(args[1:], out, errOut)
	case "threads":
		return runThreads(args[1:], out, errOut)
	case "thread":
		return runThreadShow(args[1:], out, errOut)
	case "ingest-artifact":
		return runIngest(args[1:], out, errOut)
	case "embed":
		return runEmbed(args[1:], out, errOut)
	case "template":
		return runTemplate(args[1:], out, errOut)
	case "mcp":
		return runMCP(args[1:], out, errOut)
	case "doctor":
		return runDoctor(args[1:], out, errOut)
	case "help", "-h", "--help":
		writeUsage(out)
		return 0
	default:
		fmt.Fprintf(errOut, "unknown command: %s\n", cmd)
		writeUsage(errOut)
		return 2
	}
}

func isVersionCommand(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "version", "--version", "-v":
		return true
	default:
		return false
	}
}
