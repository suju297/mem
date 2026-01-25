package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

type ShowResponse struct {
	Kind   string        `json:"kind"`
	Memory *MemoryDetail `json:"memory,omitempty"`
	Chunk  *ChunkDetail  `json:"chunk,omitempty"`
}

type MemoryDetail struct {
	ID           string `json:"id"`
	RepoID       string `json:"repo_id"`
	ThreadID     string `json:"thread_id,omitempty"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	TagsJSON     string `json:"tags_json,omitempty"`
	EntitiesJSON string `json:"entities_json,omitempty"`
	CreatedAt    string `json:"created_at"`
	AnchorCommit string `json:"anchor_commit,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`
	DeletedAt    string `json:"deleted_at,omitempty"`
}

type ChunkDetail struct {
	ID         string `json:"id"`
	RepoID     string `json:"repo_id"`
	ArtifactID string `json:"artifact_id,omitempty"`
	ThreadID   string `json:"thread_id,omitempty"`
	Locator    string `json:"locator,omitempty"`
	Text       string `json:"text"`
	TagsJSON   string `json:"tags_json,omitempty"`
	CreatedAt  string `json:"created_at"`
	DeletedAt  string `json:"deleted_at,omitempty"`
}

func runShow(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	fs.SetOutput(errOut)
	repoOverride := fs.String("repo", "", "Override repo id")
	workspace := fs.String("workspace", "", "Workspace name")
	positional, flagArgs, err := splitFlagArgs(args, map[string]flagSpec{
		"repo":      {RequiresValue: true},
		"workspace": {RequiresValue: true},
	})
	if err != nil {
		fmt.Fprintln(errOut, err.Error())
		return 2
	}
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}

	id := strings.TrimSpace(strings.Join(positional, " "))
	if id == "" {
		fmt.Fprintln(errOut, "missing id")
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		fmt.Fprintf(errOut, "config error: %v\n", err)
		return 1
	}
	workspaceName := resolveWorkspace(cfg, strings.TrimSpace(*workspace))

	repoInfo, err := resolveRepo(cfg, strings.TrimSpace(*repoOverride))
	if err != nil {
		fmt.Fprintf(errOut, "repo detection error: %v\n", err)
		return 1
	}

	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		fmt.Fprintf(errOut, "store open error: %v\n", err)
		return 1
	}
	defer st.Close()

	var resp ShowResponse
	isMemoryPref := strings.HasPrefix(id, "M")
	isChunkPref := strings.HasPrefix(id, "C")

	if !isChunkPref {
		mem, err := st.GetMemory(repoInfo.ID, workspaceName, id)
		if err == nil {
			resp.Kind = "memory"
			resp.Memory = &MemoryDetail{
				ID:           mem.ID,
				RepoID:       mem.RepoID,
				ThreadID:     mem.ThreadID,
				Title:        mem.Title,
				Summary:      mem.Summary,
				TagsJSON:     mem.TagsJSON,
				EntitiesJSON: mem.EntitiesJSON,
				CreatedAt:    mem.CreatedAt.UTC().Format(time.RFC3339Nano),
				AnchorCommit: mem.AnchorCommit,
				SupersededBy: mem.SupersededBy,
				DeletedAt:    formatTime(mem.DeletedAt),
			}
			return writeJSON(out, errOut, resp)
		}
	}

	if !isMemoryPref {
		chunk, err := st.GetChunk(repoInfo.ID, workspaceName, id)
		if err == nil {
			resp.Kind = "chunk"
			resp.Chunk = &ChunkDetail{
				ID:         chunk.ID,
				RepoID:     chunk.RepoID,
				ArtifactID: chunk.ArtifactID,
				ThreadID:   chunk.ThreadID,
				Locator:    chunk.Locator,
				Text:       chunk.Text,
				TagsJSON:   chunk.TagsJSON,
				CreatedAt:  chunk.CreatedAt.UTC().Format(time.RFC3339Nano),
				DeletedAt:  formatTime(chunk.DeletedAt),
			}
			return writeJSON(out, errOut, resp)
		}
	}

	fmt.Fprintf(errOut, "id not found: %s\n", id)
	return 1
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func writeJSON(out, errOut io.Writer, value any) int {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "json error: %v\n", err)
		return 1
	}
	fmt.Fprintln(out, string(encoded))
	return 0
}
