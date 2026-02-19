package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mempack/internal/repo"
)

type addResp struct {
	ID           string `json:"id"`
	ThreadID     string `json:"thread_id"`
	Title        string `json:"title"`
	AnchorCommit string `json:"anchor_commit"`
}

type showResp struct {
	Kind   string       `json:"kind"`
	Memory memoryDetail `json:"memory"`
}

type memoryDetail struct {
	ID           string `json:"id"`
	ThreadID     string `json:"thread_id"`
	Title        string `json:"title"`
	Summary      string `json:"summary"`
	TagsJSON     string `json:"tags_json"`
	EntitiesJSON string `json:"entities_json"`
	AnchorCommit string `json:"anchor_commit"`
	SupersededBy string `json:"superseded_by"`
	DeletedAt    string `json:"deleted_at"`
}

type forgetResp struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type supersedeResp struct {
	OldID  string `json:"old_id"`
	NewID  string `json:"new_id"`
	Status string `json:"status"`
}

type checkpointResp struct {
	StateID  string `json:"state_id"`
	MemoryID string `json:"memory_id"`
}

type linkResp struct {
	FromID string `json:"from_id"`
	Rel    string `json:"rel"`
	ToID   string `json:"to_id"`
	Status string `json:"status"`
}

type recentResp struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
}

type sessionResp struct {
	ID        string `json:"id"`
	ThreadID  string `json:"thread_id"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
}

type sessionCountResp struct {
	Count int `json:"count"`
}

type sessionUpsertResp struct {
	ID      string `json:"id"`
	Action  string `json:"action"`
	Created bool   `json:"created"`
	Updated bool   `json:"updated"`
}

func TestCLIShowForgetSupersedeCheckpoint(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	addOut := runCLI(t, "add", "--thread", "T1", "--title", "First", "--summary", "Initial decision")
	var add addResp
	if err := json.Unmarshal(addOut, &add); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if add.ID == "" {
		t.Fatalf("expected add id to be set")
	}
	if add.AnchorCommit == "" {
		t.Fatalf("expected anchor_commit to be set")
	}

	showOut := runCLI(t, "show", add.ID)
	var show showResp
	if err := json.Unmarshal(showOut, &show); err != nil {
		t.Fatalf("decode show response: %v", err)
	}
	if show.Kind != "memory" {
		t.Fatalf("expected kind memory, got %s", show.Kind)
	}
	if show.Memory.Title != "First" {
		t.Fatalf("expected title First, got %s", show.Memory.Title)
	}

	// Move HEAD to a new commit to ensure supersede preserves the original anchor_commit.
	writeFile(t, repoDir, "file.txt", "content2")
	runGit(t, repoDir, "add", "file.txt")
	runGit(t, repoDir, "commit", "-m", "next")

	supOut := runCLI(t, "supersede", "--title", "Second", "--summary", "Updated decision", add.ID)
	var sup supersedeResp
	if err := json.Unmarshal(supOut, &sup); err != nil {
		t.Fatalf("decode supersede response: %v", err)
	}
	if sup.NewID == "" {
		t.Fatalf("expected new id to be set")
	}

	oldShowOut := runCLI(t, "show", sup.OldID)
	var oldShow showResp
	if err := json.Unmarshal(oldShowOut, &oldShow); err != nil {
		t.Fatalf("decode old show: %v", err)
	}
	if oldShow.Memory.SupersededBy != sup.NewID {
		t.Fatalf("expected superseded_by %s, got %s", sup.NewID, oldShow.Memory.SupersededBy)
	}

	newShowOut := runCLI(t, "show", sup.NewID)
	var newShow showResp
	if err := json.Unmarshal(newShowOut, &newShow); err != nil {
		t.Fatalf("decode new show: %v", err)
	}
	if newShow.Memory.Title != "Second" {
		t.Fatalf("expected new title Second, got %s", newShow.Memory.Title)
	}
	if newShow.Memory.AnchorCommit != add.AnchorCommit {
		t.Fatalf("expected anchor_commit %s, got %s", add.AnchorCommit, newShow.Memory.AnchorCommit)
	}

	forgetOut := runCLI(t, "forget", sup.NewID)
	var forget forgetResp
	if err := json.Unmarshal(forgetOut, &forget); err != nil {
		t.Fatalf("decode forget response: %v", err)
	}
	if forget.Status != "forgotten" {
		t.Fatalf("expected forgotten status, got %s", forget.Status)
	}

	forgottenShow := runCLI(t, "show", sup.NewID)
	var forgotten showResp
	if err := json.Unmarshal(forgottenShow, &forgotten); err != nil {
		t.Fatalf("decode forgotten show: %v", err)
	}
	if forgotten.Memory.DeletedAt == "" {
		t.Fatalf("expected deleted_at to be set")
	}

	ckOut := runCLI(t, "checkpoint", "--reason", "Snapshot", "--state-json", `{"goal":"ship"}`, "--thread", "T1")
	var ck checkpointResp
	if err := json.Unmarshal(ckOut, &ck); err != nil {
		t.Fatalf("decode checkpoint response: %v", err)
	}
	if ck.StateID == "" || ck.MemoryID == "" {
		t.Fatalf("expected state_id and memory_id to be set")
	}

	ckShow := runCLI(t, "show", ck.MemoryID)
	var ckMem showResp
	if err := json.Unmarshal(ckShow, &ckMem); err != nil {
		t.Fatalf("decode checkpoint memory: %v", err)
	}
	if ckMem.Memory.Summary != "Snapshot" {
		t.Fatalf("expected checkpoint summary Snapshot, got %s", ckMem.Memory.Summary)
	}
}

func TestCLIUpdateMemory(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	addOut := runCLI(t, "add", "--title", "First", "--summary", "Initial decision")
	var add addResp
	if err := json.Unmarshal(addOut, &add); err != nil {
		t.Fatalf("decode add response: %v", err)
	}

	_ = runCLI(t, "update", add.ID, "--summary", "Updated decision", "--tags-add", "needs_summary")

	showOut := runCLI(t, "show", add.ID)
	var show showResp
	if err := json.Unmarshal(showOut, &show); err != nil {
		t.Fatalf("decode show response: %v", err)
	}
	if show.Memory.Summary != "Updated decision" {
		t.Fatalf("expected updated summary, got %s", show.Memory.Summary)
	}
	var tags []string
	if err := json.Unmarshal([]byte(show.Memory.TagsJSON), &tags); err != nil {
		t.Fatalf("decode tags: %v", err)
	}
	if !sliceContains(tags, "needs_summary") {
		t.Fatalf("expected needs_summary tag, got %v", tags)
	}
}

func TestCLILinkMemories(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	firstOut := runCLI(t, "add", "--title", "First", "--summary", "First summary")
	var first addResp
	if err := json.Unmarshal(firstOut, &first); err != nil {
		t.Fatalf("decode first add response: %v", err)
	}

	secondOut := runCLI(t, "add", "--title", "Second", "--summary", "Second summary")
	var second addResp
	if err := json.Unmarshal(secondOut, &second); err != nil {
		t.Fatalf("decode second add response: %v", err)
	}

	linkOut := runCLI(t, "link", "--from", first.ID, "--rel", "depends_on", "--to", second.ID)
	var link linkResp
	if err := json.Unmarshal(linkOut, &link); err != nil {
		t.Fatalf("decode link response: %v", err)
	}
	if link.FromID != first.ID || link.ToID != second.ID || link.Rel != "depends_on" {
		t.Fatalf("unexpected link response: %+v", link)
	}
	if link.Status != "linked" {
		t.Fatalf("expected linked status, got %s", link.Status)
	}

	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("config error: %v", err)
	}
	repoInfo, err := resolveRepo(&cfg, "")
	if err != nil {
		t.Fatalf("repo detection error: %v", err)
	}
	st, err := openStore(cfg, repoInfo.ID)
	if err != nil {
		t.Fatalf("store open error: %v", err)
	}
	defer st.Close()

	links, err := st.ListLinksForIDs([]string{first.ID, second.ID})
	if err != nil {
		t.Fatalf("list links: %v", err)
	}
	found := false
	for _, edge := range links {
		if edge.FromID == first.ID && edge.Rel == "depends_on" && edge.ToID == second.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected depends_on link from %s to %s", first.ID, second.ID)
	}
}

func TestCLIAddEntities(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	addOut := runCLI(t, "add", "--title", "First", "--summary", "Initial decision", "--entities", "file_src_a_ts,ext_ts")
	var add addResp
	if err := json.Unmarshal(addOut, &add); err != nil {
		t.Fatalf("decode add response: %v", err)
	}

	showOut := runCLI(t, "show", add.ID)
	var show showResp
	if err := json.Unmarshal(showOut, &show); err != nil {
		t.Fatalf("decode show response: %v", err)
	}
	var entities []string
	if err := json.Unmarshal([]byte(show.Memory.EntitiesJSON), &entities); err != nil {
		t.Fatalf("decode entities: %v", err)
	}
	if !sliceContains(entities, "file_src_a_ts") {
		t.Fatalf("expected file_src_a_ts entity, got %v", entities)
	}
	if !sliceContains(entities, "ext_ts") {
		t.Fatalf("expected ext_ts entity, got %v", entities)
	}
}

func sliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestCLIAddDefaultsThread(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	addOut := runCLI(t, "add", "--title", "First", "--summary", "Initial decision")
	var add addResp
	if err := json.Unmarshal(addOut, &add); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if add.ThreadID != "T-SESSION" {
		t.Fatalf("expected default thread T-SESSION, got %s", add.ThreadID)
	}
}

func TestCLIRecentAndFormatFlags(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	runCLI(t, "add", "--thread", "T1", "--title", "First", "--summary", "First summary")
	time.Sleep(10 * time.Millisecond)
	runCLI(t, "add", "--thread", "T1", "--title", "Second", "--summary", "Second summary")

	recentOut := runCLI(t, "recent", "--limit", "1", "--format", "json")
	var recent []recentResp
	if err := json.Unmarshal(recentOut, &recent); err != nil {
		t.Fatalf("decode recent response: %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("expected 1 recent item, got %d", len(recent))
	}
	if recent[0].Title != "Second" {
		t.Fatalf("expected most recent title Second, got %s", recent[0].Title)
	}
	if recent[0].ThreadID != "T1" {
		t.Fatalf("expected thread T1, got %s", recent[0].ThreadID)
	}

	_ = runCLI(t, "threads", "--format", "json")
	_ = runCLI(t, "thread", "T1", "--format", "json")
}

func TestCLISessions(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	runCLI(t, "add", "--thread", "T1", "--title", "Session one", "--summary", "Body", "--tags", "session")
	time.Sleep(10 * time.Millisecond)
	runCLI(t, "add", "--thread", "T1", "--title", "Session two", "--summary", "Body", "--tags", "session,needs_summary")

	sessionsOut := runCLI(t, "sessions", "--needs-summary", "--limit", "1", "--format", "json")
	var sessions []sessionResp
	if err := json.Unmarshal(sessionsOut, &sessions); err != nil {
		t.Fatalf("decode sessions response: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session item, got %d", len(sessions))
	}
	if sessions[0].Title != "Session two" {
		t.Fatalf("expected most recent needs_summary session, got %s", sessions[0].Title)
	}

	countOut := runCLI(t, "sessions", "--needs-summary", "--count", "--format", "json")
	var count sessionCountResp
	if err := json.Unmarshal(countOut, &count); err != nil {
		t.Fatalf("decode sessions count response: %v", err)
	}
	if count.Count != 1 {
		t.Fatalf("expected needs_summary count 1, got %d", count.Count)
	}
}

func TestCLISessionUpsert(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	firstOut := runCLI(
		t,
		"session",
		"upsert",
		"--title",
		"Session: src (+1 files) [ts]",
		"--tags",
		"session,needs_summary",
		"--entities",
		"dir_src,ext_ts,file_src_app_ts",
		"--format",
		"json",
	)
	var first sessionUpsertResp
	if err := json.Unmarshal(firstOut, &first); err != nil {
		t.Fatalf("decode first session upsert: %v", err)
	}
	if !first.Created || first.Action != "created" {
		t.Fatalf("expected first upsert to create, got %+v", first)
	}

	secondOut := runCLI(
		t,
		"session",
		"upsert",
		"--title",
		"Session: src (+2 files) [ts]",
		"--entities",
		"file_src_routes_ts",
		"--format",
		"json",
	)
	var second sessionUpsertResp
	if err := json.Unmarshal(secondOut, &second); err != nil {
		t.Fatalf("decode second session upsert: %v", err)
	}
	if !second.Updated || second.Action != "updated" {
		t.Fatalf("expected second upsert to update, got %+v", second)
	}
	if second.ID != first.ID {
		t.Fatalf("expected second upsert to update first session id %s, got %s", first.ID, second.ID)
	}

	showOut := runCLI(t, "show", first.ID)
	var show showResp
	if err := json.Unmarshal(showOut, &show); err != nil {
		t.Fatalf("decode show response: %v", err)
	}
	if show.Memory.Title != "Session: src (+2 files) [ts]" {
		t.Fatalf("expected merged title to update, got %s", show.Memory.Title)
	}
	if !strings.Contains(show.Memory.TagsJSON, "needs_summary") {
		t.Fatalf("expected needs_summary tag to remain, got %s", show.Memory.TagsJSON)
	}

	manual := runCLI(t, "add", "--title", "Manual note", "--summary", "manual", "--tags", "session")
	var manualAdd addResp
	if err := json.Unmarshal(manual, &manualAdd); err != nil {
		t.Fatalf("decode manual add response: %v", err)
	}
	if manualAdd.ID == "" {
		t.Fatalf("expected manual add id")
	}

	thirdOut := runCLI(
		t,
		"session",
		"upsert",
		"--title",
		"Session: src (+3 files) [ts]",
		"--entities",
		"file_src_api_ts",
		"--format",
		"json",
	)
	var third sessionUpsertResp
	if err := json.Unmarshal(thirdOut, &third); err != nil {
		t.Fatalf("decode third session upsert: %v", err)
	}
	if !third.Created || third.Action != "created" {
		t.Fatalf("expected third upsert to create because latest is manual, got %+v", third)
	}
}

func runCLI(t *testing.T, args ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	var errOut bytes.Buffer
	code := Run(args, &out, &errOut)
	if code != 0 {
		t.Fatalf("command failed (%d): %s", code, errOut.String())
	}
	return bytes.TrimSpace(out.Bytes())
}

func setupRepo(t testing.TB, base string) string {
	t.Helper()
	repoDir := filepath.Join(base, "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	runGit(t, repoDir, "init")
	runGit(t, repoDir, "config", "user.name", "Test")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	writeFile(t, repoDir, "file.txt", "content")
	runGit(t, repoDir, "add", "file.txt")
	runGit(t, repoDir, "commit", "-m", "init")
	return repoDir
}

func setXDGEnv(t testing.TB, base string) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(base, "cache"))
}

func withCwd(t testing.TB, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
}

func TestCLIReposAndUse(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	info, err := repo.Detect(repoDir)
	if err != nil {
		t.Fatalf("detect repo: %v", err)
	}

	_ = runCLI(t, "add", "--thread", "T1", "--title", "Seed", "--summary", "Seed")

	useOut := runCLI(t, "use", info.ID)
	if len(useOut) == 0 {
		t.Fatalf("expected use output")
	}

	reposOut := runCLI(t, "repos")
	if len(reposOut) == 0 {
		t.Fatalf("expected repos output")
	}
}

func TestCLIThreadFlagsAfterID(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "add", "--thread", "T1", "--title", "Seed", "--summary", "Seed")

	out := runCLI(t, "thread", "T1", "--limit", "20")
	var resp ThreadShowResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("decode thread response: %v", err)
	}
	if resp.Thread.ThreadID != "T1" {
		t.Fatalf("expected thread T1, got %s", resp.Thread.ThreadID)
	}
	if resp.Thread.MemoryCount != 1 {
		t.Fatalf("expected memory_count 1, got %d", resp.Thread.MemoryCount)
	}
}

func TestCLIThreadMemoryCountMatchesTotal(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "add", "--thread", "T1", "--title", "First", "--summary", "First summary")
	_ = runCLI(t, "add", "--thread", "T1", "--title", "Second", "--summary", "Second summary")

	out := runCLI(t, "thread", "T1", "--limit", "1")
	var resp ThreadShowResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("decode thread response: %v", err)
	}
	if len(resp.Memories) != 1 {
		t.Fatalf("expected 1 memory in response, got %d", len(resp.Memories))
	}
	if resp.Thread.MemoryCount != 2 {
		t.Fatalf("expected memory_count 2, got %d", resp.Thread.MemoryCount)
	}
	if resp.Thread.MemoryCount < len(resp.Memories) {
		t.Fatalf("expected memory_count >= memories length, got %d < %d", resp.Thread.MemoryCount, len(resp.Memories))
	}
}

func TestGetDeterministicOutput(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	_ = runCLI(t, "add", "--thread", "T1", "--title", "First", "--summary", "Initial decision")

	first := runCLI(t, "get", "decision")
	second := runCLI(t, "get", "decision")
	if !bytes.Equal(first, second) {
		t.Fatalf("expected byte-identical get output")
	}
}
