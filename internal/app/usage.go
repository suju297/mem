package app

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

func writeUsage(w io.Writer) {
	useColor := shouldColorize(w)
	title := colorize(useColor, "mem")
	subtitle := colorizeSubtle(useColor, "repo-scoped memory CLI")
	usage := colorize(useColor, "Usage")
	sections := colorize(useColor, "Common Commands")
	examples := colorize(useColor, "Examples")
	notes := colorize(useColor, "Notes")

	writeMemLogo(w)
	io.WriteString(w, "\n\n"+title+" "+subtitle+"\n\n")
	io.WriteString(w, usage+"\n")
	io.WriteString(w, "  mem [--data-dir <path>] <command> [options]\n\n")
	io.WriteString(w, colorize(useColor, "Global Options")+"\n")
	io.WriteString(w, "  --data-dir <path>  Override data dir (MEM_DATA_DIR)\n\n")
	io.WriteString(w, colorize(useColor, "Version")+"\n")
	io.WriteString(w, "  mem version | mem --version | mem -v\n\n")
	io.WriteString(w, sections+"\n")
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "  init\tInitialize memory in current repo")
	fmt.Fprintln(tw, "  get\tRetrieve context by query")
	fmt.Fprintln(tw, "  usage\tShow cumulative token usage and savings")
	fmt.Fprintln(tw, "  add\tSave a memory")
	fmt.Fprintln(tw, "  update\tUpdate a memory")
	fmt.Fprintln(tw, "  repos\tList known repos")
	fmt.Fprintln(tw, "  share export\tExport memories to mem-share/")
	fmt.Fprintln(tw, "  share import\tImport from mem-share/")
	fmt.Fprintln(tw, "  mcp start|status|stop\tManage local MCP daemon")
	fmt.Fprintln(tw, "  mcp manager\tRun MCP manager control plane")
	fmt.Fprintln(tw, "  doctor\tRun health checks")
	fmt.Fprintln(tw, "  template\tGenerate assistant template files")
	_ = tw.Flush()

	io.WriteString(w, "\n"+examples+"\n")
	io.WriteString(w, "  mem init\n")
	io.WriteString(w, "  mem add --title \"Auth plan\" --summary \"Use middleware\"\n")
	io.WriteString(w, "  mem get \"auth plan\" --format prompt\n")
	io.WriteString(w, "  mem usage\n")
	io.WriteString(w, "  mem usage --me\n")
	io.WriteString(w, "  mem doctor --json\n")

	io.WriteString(w, "\n"+notes+"\n")
	io.WriteString(w, "  - Run 'mem <command> --help' for command-specific flags.\n")
	io.WriteString(w, "  - 'mem mcp' is raw stdio mode for MCP clients; in terminals use 'mem mcp start|status|stop'.\n")
	io.WriteString(w, "  - First-time setup: docs/onboarding.md.\n")
	io.WriteString(w, "  - Common workflows: docs/cookbook.md.\n")
	io.WriteString(w, "  - Scripting and automation: docs/scripting.md.\n")
	io.WriteString(w, "  - Full CLI reference: docs/cli.md.\n")
}

func shouldColorize(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func colorize(enabled bool, text string) string {
	if !enabled {
		return text
	}
	const cyan = "\x1b[36m"
	const bold = "\x1b[1m"
	const reset = "\x1b[0m"
	return bold + cyan + text + reset
}

func colorizeSubtle(enabled bool, text string) string {
	if !enabled {
		return text
	}
	const dim = "\x1b[2m"
	const reset = "\x1b[0m"
	return dim + text + reset
}
