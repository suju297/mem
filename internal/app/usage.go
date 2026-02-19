package app

import (
	"io"
	"os"
)

func writeUsage(w io.Writer) {
	useColor := shouldColorize(w)
	title := colorize(useColor, "mempack - repo-scoped memory CLI")
	usage := colorize(useColor, "Usage:")
	commands := colorize(useColor, "Commands:")

	io.WriteString(w, title+"\n\n")
	io.WriteString(w, usage+"\n")
	io.WriteString(w, "  mem [--data-dir <path>] <command> [options]\n\n")
	io.WriteString(w, colorize(useColor, "Global options:")+"\n")
	io.WriteString(w, "  --data-dir <path>  Override data dir (MEMPACK_DATA_DIR)\n\n")
	io.WriteString(w, "Version:\n")
	io.WriteString(w, "  mem version | mem --version | mem -v\n\n")
	io.WriteString(w, commands+"\n")
	io.WriteString(w, "  init            mem init [--no-agents] [--assistants agents|claude|gemini|all]\n")
	io.WriteString(w, "  get             mem get \"<query>\" [--workspace <name>] [--include-orphans] [--cluster] [--repo <id>] [--debug]\n")
	io.WriteString(w, "  add             mem add --title <title> --summary <summary> [--thread <id>] [--tags tag1,tag2] [--entities <csv>] [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "  update          mem update <id> [--title <title>] [--summary <summary>] [--tags <csv>] [--tags-add <csv>] [--tags-remove <csv>] [--entities <csv>] [--entities-add <csv>] [--entities-remove <csv>] [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "  explain         mem explain \"<query>\" [--workspace <name>] [--include-orphans] [--repo <id>]\n")
	io.WriteString(w, "  show            mem show <id> [--format json] [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "  forget          mem forget <id> [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "  supersede       mem supersede <id> --title <title> --summary <summary> [--thread <id>] [--tags tag1,tag2] [--entities <csv>] [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "  link            mem link --from <id> --rel <relation> --to <id> [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "  checkpoint      mem checkpoint --reason \"<...>\" --state-file <path>|--state-json <json> [--thread <id>] [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "  ingest          mem ingest <path> --thread <id> [--watch] [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "                 mem ingest-artifact <path> --thread <id> [--watch] [--workspace <name>] [--repo <id>] (alias)\n")
	io.WriteString(w, "  embed           mem embed [--kind memory|chunk|all] [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "                 mem embed status [--workspace <name>] [--repo <id>]\n")
	io.WriteString(w, "  repos           mem repos\n")
	io.WriteString(w, "  use             mem use <repo_id|path>\n")
	io.WriteString(w, "  threads         mem threads [--workspace <name>] [--repo <id>] [--format json]\n")
	io.WriteString(w, "  thread          mem thread <thread_id> [--limit 20] [--workspace <name>] [--repo <id>] [--format json]\n")
	io.WriteString(w, "  recent          mem recent [--limit 20] [--workspace <name>] [--repo <id>] [--format json]\n")
	io.WriteString(w, "  sessions        mem sessions [--needs-summary] [--count] [--limit 20] [--workspace <name>] [--repo <id>] [--format json]\n")
	io.WriteString(w, "  session         mem session upsert --title <title> [--summary <summary>] [--thread <id>] [--tags <csv>] [--entities <csv>] [--merge-window-ms <n>] [--min-gap-ms <n>] [--workspace <name>] [--repo <id>] [--format json]\n")
	io.WriteString(w, "  share           mem share export [--repo <id|path>] [--workspace <name>] [--out <dir>]\n")
	io.WriteString(w, "                 mem share import [--repo <id|path>] [--workspace <name>] [--in <dir>] [--replace] [--allow-repo-mismatch]\n")
	io.WriteString(w, "  mcp             mem mcp [start|stop|status] [--repo <id|path>] [--require-repo] [--allow-write] [--write-mode ask|auto|off] [--debug] [--repair]\n")
	io.WriteString(w, "                 mem mcp manager [--port <n>] [--token <token>] [--idle-seconds <n>]\n")
	io.WriteString(w, "                 mem mcp manager status [--json]\n")
	io.WriteString(w, "  doctor          mem doctor [--repo <id|path>] [--json] [--repair] [--verbose]\n")
	io.WriteString(w, "  template        mem template [agents] [--write] [--assistants agents|claude|gemini|all] [--memory|--no-memory]\n")
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
	const purple = "\x1b[35m"
	const bold = "\x1b[1m"
	const reset = "\x1b[0m"
	return bold + purple + text + reset
}
