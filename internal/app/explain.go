package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"mempack/internal/pack"
	"mempack/internal/store"
)

type ExplainReport struct {
	Query          string               `json:"query"`
	Repo           pack.RepoInfo        `json:"repo"`
	Workspace      string               `json:"workspace"`
	StateSource    string               `json:"state_source,omitempty"`
	MatchedThreads []pack.MatchedThread `json:"matched_threads"`
	Memories       []ExplainMemory      `json:"memories"`
	Chunks         []ExplainChunk       `json:"chunks"`
	Vector         VectorExplain        `json:"vector"`
	Budget         pack.BudgetInfo      `json:"budget"`
}

type VectorExplain struct {
	Provider      string  `json:"provider"`
	Model         string  `json:"model"`
	Enabled       bool    `json:"enabled"`
	MinSimilarity float64 `json:"min_similarity"`
	Error         string  `json:"error,omitempty"`
}

type ExplainMemory struct {
	ID           string  `json:"id"`
	ThreadID     string  `json:"thread_id,omitempty"`
	Title        string  `json:"title"`
	AnchorCommit string  `json:"anchor_commit,omitempty"`
	BM25         float64 `json:"bm25"`
	FTSScore     float64 `json:"fts_score"`
	FTSRank      int     `json:"fts_rank"`
	VectorScore  float64 `json:"vector_score"`
	VectorRank   int     `json:"vector_rank"`
	RRFScore     float64 `json:"rrf_score"`
	RecencyBonus float64 `json:"recency_bonus"`
	ThreadBonus  float64 `json:"thread_bonus"`
	Superseded   bool    `json:"superseded"`
	Orphaned     bool    `json:"orphaned"`
	FinalScore   float64 `json:"final_score"`
	Included     bool    `json:"included"`
}

type ExplainChunk struct {
	ID           string  `json:"id"`
	ThreadID     string  `json:"thread_id,omitempty"`
	Locator      string  `json:"locator,omitempty"`
	BM25         float64 `json:"bm25"`
	FTSScore     float64 `json:"fts_score"`
	FTSRank      int     `json:"fts_rank"`
	VectorScore  float64 `json:"vector_score"`
	VectorRank   int     `json:"vector_rank"`
	RRFScore     float64 `json:"rrf_score"`
	RecencyBonus float64 `json:"recency_bonus"`
	ThreadBonus  float64 `json:"thread_bonus"`
	FinalScore   float64 `json:"final_score"`
	Included     bool    `json:"included"`
}

func runExplain(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("explain", flag.ContinueOnError)
	fs.SetOutput(errOut)
	workspace := fs.String("workspace", "", "Workspace name")
	includeOrphans := fs.Bool("include-orphans", false, "Include orphaned memories")
	repoOverride := fs.String("repo", "", "Override repo id")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"workspace":       {RequiresValue: true},
		"include-orphans": {RequiresValue: false},
		"repo":            {RequiresValue: true},
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

	report, err := buildExplainReport(query, ExplainOptions{
		RepoOverride:   *repoOverride,
		Workspace:      *workspace,
		IncludeOrphans: *includeOrphans,
	})
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return 1
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "json error: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(encoded))
	return 0
}
