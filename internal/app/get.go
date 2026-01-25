package app

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"mempack/internal/pack"
	"mempack/internal/store"
)

type getTimings struct {
	ConfigLoad           time.Duration
	RepoDetect           time.Duration
	StoreOpen            time.Duration
	StateLoad            time.Duration
	FTSMemoriesCandidate time.Duration
	FTSMemoriesFetch     time.Duration
	FTSChunksCandidate   time.Duration
	FTSChunksFetch       time.Duration
	OrphanFilter         time.Duration
	ThreadMatch          time.Duration
	TokenizerInit        time.Duration
	Budget               time.Duration
	JSONEncode           time.Duration
	JSONWrite            time.Duration
	JSONFlush            time.Duration
	MemoryCount          int
	ChunkCount           int
	MemoryCandidates     int
	ChunkCandidates      int
	OrphanChecks         int
	OrphansFiltered      int
}

func runGet(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("get", flag.ContinueOnError)
	fs.SetOutput(errOut)
	format := fs.String("format", "json", "Output format: json|prompt")
	repoOverride := fs.String("repo", "", "Override repo id")
	workspace := fs.String("workspace", "", "Workspace name")
	includeOrphans := fs.Bool("include-orphans", false, "Include orphaned memories")
	debug := fs.Bool("debug", false, "Print timing breakdown to stderr")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"format":          {RequiresValue: true},
		"repo":            {RequiresValue: true},
		"workspace":       {RequiresValue: true},
		"include-orphans": {RequiresValue: false},
		"debug":           {RequiresValue: false},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	query := strings.TrimSpace(strings.Join(positional, " "))
	if err := store.EnsureValidQuery(query); err != nil {
		fmt.Fprintf(errOut, "invalid query: %v\n", err)
		return 2
	}
	if *format != "json" && *format != "prompt" {
		fmt.Fprintf(errOut, "unsupported format: %s\n", *format)
		return 2
	}

	includeRawChunks := strings.TrimSpace(*format) == "prompt"
	var timings getTimings
	packJSON, err := buildContextPack(query, ContextOptions{
		RepoOverride:     *repoOverride,
		Workspace:        *workspace,
		IncludeOrphans:   *includeOrphans,
		IncludeRawChunks: includeRawChunks,
	}, &timings)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return 1
	}

	encodeStart := time.Now()
	encoded, err := json.MarshalIndent(packJSON, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "json error: %v\n", err)
		return 1
	}
	timings.JSONEncode = time.Since(encodeStart)

	writeStart := time.Now()
	if *format == "prompt" {
		renderPrompt(out, packJSON)
	} else {
		// Default JSON
		fmt.Fprintln(out, string(encoded))
	}
	timings.JSONWrite = time.Since(writeStart)
	timings.JSONFlush = flushOutput(out, *debug)

	if *debug {
		writeTimings(errOut, timings)
	}
	return 0
}

func writeTimings(out io.Writer, timings getTimings) {
	ms := func(d time.Duration) float64 {
		return float64(d.Microseconds()) / 1000.0
	}
	fmt.Fprintf(out, "debug timings (ms): config_load=%.2f repo_detect=%.2f store_open=%.2f state_load=%.2f fts_memories_candidate=%.2f fts_memories_fetch=%.2f fts_chunks_candidate=%.2f fts_chunks_fetch=%.2f orphan_filter=%.2f thread_match=%.2f tokenizer_init=%.2f budget=%.2f json_encode=%.2f json_write=%.2f json_flush=%.2f\n",
		ms(timings.ConfigLoad),
		ms(timings.RepoDetect),
		ms(timings.StoreOpen),
		ms(timings.StateLoad),
		ms(timings.FTSMemoriesCandidate),
		ms(timings.FTSMemoriesFetch),
		ms(timings.FTSChunksCandidate),
		ms(timings.FTSChunksFetch),
		ms(timings.OrphanFilter),
		ms(timings.ThreadMatch),
		ms(timings.TokenizerInit),
		ms(timings.Budget),
		ms(timings.JSONEncode),
		ms(timings.JSONWrite),
		ms(timings.JSONFlush),
	)
	fmt.Fprintf(out, "debug counts: mem_candidates=%d mem_results=%d chunk_candidates=%d chunk_results=%d orphan_checks=%d orphans_filtered=%d\n",
		timings.MemoryCandidates,
		timings.MemoryCount,
		timings.ChunkCandidates,
		timings.ChunkCount,
		timings.OrphanChecks,
		timings.OrphansFiltered,
	)
}

func renderPromptString(p pack.ContextPack) string {
	var buf bytes.Buffer
	renderPrompt(&buf, p)
	return buf.String()
}

func parseStateUpdatedAt(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts, true
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

func flushOutput(out io.Writer, enabled bool) time.Duration {
	if !enabled {
		return 0
	}
	syncer, ok := out.(interface{ Sync() error })
	if !ok {
		return 0
	}
	start := time.Now()
	_ = syncer.Sync()
	return time.Since(start)
}

func renderPrompt(out io.Writer, p pack.ContextPack) {
	fmt.Fprintf(out, "# Context from Memory (Repo: %s)\n", p.Repo.RepoID)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Agent rule: If you do not have this pack for the current task, ask the user to run:")
	fmt.Fprintln(out, "`mempack get \"<task>\" --format prompt`")
	fmt.Fprintln(out)

	if len(p.State) > 0 && string(p.State) != "{}" {
		fmt.Fprintln(out, "## State")
		fmt.Fprintln(out, "```json")
		fmt.Fprintln(out, string(p.State))
		fmt.Fprintln(out, "```")
		fmt.Fprintln(out)
	}

	if len(p.TopMemories) > 0 {
		fmt.Fprintln(out, "## Memories")
		for _, m := range p.TopMemories {
			fmt.Fprintf(out, "- **%s**: %s\n", m.Title, m.Summary)
		}
		fmt.Fprintln(out)
	}

	chunks := p.TopChunks
	if len(p.TopChunksRaw) > 0 {
		chunks = p.TopChunksRaw
	}
	if len(chunks) > 0 {
		fmt.Fprintln(out, "## Evidence (Data Only)")
		for _, c := range chunks {
			fmt.Fprintf(out, "### %s\n", c.Locator)
			fmt.Fprintln(out, "```")
			fmt.Fprintln(out, c.Text)
			fmt.Fprintln(out, "```")
			fmt.Fprintln(out)
		}
	}

	if len(p.Rules) > 0 {
		fmt.Fprintln(out, "## Rules")
		for _, r := range p.Rules {
			fmt.Fprintf(out, "- %s\n", r)
		}
		fmt.Fprintln(out)
	}
}
