package pack

import "encoding/json"

type ContextPack struct {
	Version        string          `json:"version"`
	Tool           string          `json:"tool"`
	Repo           RepoInfo        `json:"repo"`
	Workspace      string          `json:"workspace"`
	SearchMeta     SearchMeta      `json:"search_meta,omitempty"`
	State          json.RawMessage `json:"state"`
	MatchedThreads []MatchedThread `json:"matched_threads"`
	TopMemories    []MemoryItem    `json:"top_memories"`
	TopChunks      []ChunkItem     `json:"top_chunks"`
	TopChunksRaw   []ChunkItem     `json:"top_chunks_raw,omitempty"`
	LinkTrail      []LinkTrail     `json:"link_trail"`
	Rules          []string        `json:"rules"`
	Budget         BudgetInfo      `json:"budget"`
}

type RepoInfo struct {
	RepoID  string `json:"repo_id"`
	GitRoot string `json:"git_root"`
	Head    string `json:"head,omitempty"`
	Branch  string `json:"branch,omitempty"`
}

type MatchedThread struct {
	ThreadID string `json:"thread_id"`
	Why      string `json:"why"`
}

type MemoryItem struct {
	ID           string   `json:"id"`
	ThreadID     string   `json:"thread_id,omitempty"`
	Title        string   `json:"title"`
	Summary      string   `json:"summary"`
	AnchorCommit string   `json:"anchor_commit,omitempty"`
	Links        []string `json:"links,omitempty"`
}

type ChunkItem struct {
	ChunkID    string        `json:"chunk_id"`
	ArtifactID string        `json:"artifact_id,omitempty"`
	ThreadID   string        `json:"thread_id,omitempty"`
	Locator    string        `json:"locator,omitempty"`
	Text       string        `json:"text"`
	Sources    []ChunkSource `json:"sources,omitempty"`
}

type ChunkSource struct {
	ChunkID    string `json:"chunk_id"`
	ArtifactID string `json:"artifact_id,omitempty"`
	ThreadID   string `json:"thread_id,omitempty"`
	Locator    string `json:"locator,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}

type LinkTrail struct {
	From string `json:"from"`
	Rel  string `json:"rel"`
	To   string `json:"to"`
}

type BudgetInfo struct {
	Tokenizer   string `json:"tokenizer"`
	TargetTotal int    `json:"target_total"`
	UsedTotal   int    `json:"used_total"`
}

type SearchMeta struct {
	Mode            string   `json:"mode"`
	ModeUsed        string   `json:"mode_used"`
	VectorUsed      bool     `json:"vector_used"`
	RewrittenQuery  string   `json:"rewritten_query,omitempty"`
	RewritesApplied []string `json:"rewrites_applied,omitempty"`
	FallbackReason  string   `json:"fallback_reason,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}
