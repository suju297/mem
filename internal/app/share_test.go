package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIShareExportImportIdempotent(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	sourceRepo := setupRepo(t, filepath.Join(base, "source"))
	targetRepo := setupRepo(t, filepath.Join(base, "target"))

	withCwd(t, sourceRepo)
	first := runCLI(t, "add", "--thread", "T-SHARE", "--title", "Share first", "--summary", "First export memory", "--tags", "session", "--entities", "dir_src,file_src_a_ts,ext_ts")
	second := runCLI(t, "add", "--thread", "T-SHARE", "--title", "Share second", "--summary", "Second export memory", "--tags", "session", "--entities", "dir_src,file_src_b_ts,ext_ts")

	var firstAdd addResp
	if err := json.Unmarshal(first, &firstAdd); err != nil {
		t.Fatalf("decode first add response: %v", err)
	}
	var secondAdd addResp
	if err := json.Unmarshal(second, &secondAdd); err != nil {
		t.Fatalf("decode second add response: %v", err)
	}

	exportOut := runCLI(t, "share", "export")
	var exported shareExportResponse
	if err := json.Unmarshal(exportOut, &exported); err != nil {
		t.Fatalf("decode share export response: %v", err)
	}
	if exported.BundleDir == "" {
		t.Fatalf("expected bundle_dir in export response")
	}
	manifestPath := filepath.Join(exported.BundleDir, shareManifestFileName)
	memoriesPath := filepath.Join(exported.BundleDir, shareMemoriesFileName)
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest file: %v", err)
	}
	if _, err := os.Stat(memoriesPath); err != nil {
		t.Fatalf("expected memories file: %v", err)
	}

	exportedRecords, err := readShareMemoryRecords(memoriesPath)
	if err != nil {
		t.Fatalf("read exported records: %v", err)
	}
	if len(exportedRecords) == 0 {
		t.Fatalf("expected exported records")
	}
	recordIDs := make(map[string]struct{}, len(exportedRecords))
	for _, record := range exportedRecords {
		recordIDs[record.SourceID] = struct{}{}
	}
	if _, ok := recordIDs[firstAdd.ID]; !ok {
		t.Fatalf("expected exported records to include source id %s", firstAdd.ID)
	}
	if _, ok := recordIDs[secondAdd.ID]; !ok {
		t.Fatalf("expected exported records to include source id %s", secondAdd.ID)
	}

	withCwd(t, targetRepo)
	errOut := runCLIExpectError(t, "share", "import", "--in", exported.BundleDir)
	if !strings.Contains(errOut, "repo mismatch") {
		t.Fatalf("expected repo mismatch error, got: %s", errOut)
	}

	importOut := runCLI(t, "share", "import", "--in", exported.BundleDir, "--allow-repo-mismatch")
	var imported shareImportResponse
	if err := json.Unmarshal(importOut, &imported); err != nil {
		t.Fatalf("decode share import response: %v", err)
	}
	if imported.Imported != len(exportedRecords) {
		t.Fatalf("expected imported=%d, got %d", len(exportedRecords), imported.Imported)
	}

	sourceTag := shareSourceTag(exported.RepoID)
	for _, sourceID := range []string{firstAdd.ID, secondAdd.ID} {
		localID := shareLocalMemoryID(exported.RepoID, sourceID)
		showOut := runCLI(t, "show", localID)
		var show showResp
		if err := json.Unmarshal(showOut, &show); err != nil {
			t.Fatalf("decode show for %s: %v", localID, err)
		}
		if show.Memory.ID != localID {
			t.Fatalf("expected imported id %s, got %s", localID, show.Memory.ID)
		}
		var tags []string
		if err := json.Unmarshal([]byte(show.Memory.TagsJSON), &tags); err != nil {
			t.Fatalf("decode tags for %s: %v", localID, err)
		}
		if !sliceContains(tags, shareImportTag) {
			t.Fatalf("expected %s tag in %v", shareImportTag, tags)
		}
		if !sliceContains(tags, sourceTag) {
			t.Fatalf("expected %s tag in %v", sourceTag, tags)
		}
	}

	reimportOut := runCLI(t, "share", "import", "--in", exported.BundleDir, "--allow-repo-mismatch")
	var reimported shareImportResponse
	if err := json.Unmarshal(reimportOut, &reimported); err != nil {
		t.Fatalf("decode reimport response: %v", err)
	}
	if reimported.Imported != 0 || reimported.Updated != 0 {
		t.Fatalf("expected idempotent import with no new writes, got imported=%d updated=%d", reimported.Imported, reimported.Updated)
	}
	if reimported.Unchanged != len(exportedRecords) {
		t.Fatalf("expected unchanged=%d on reimport, got %d", len(exportedRecords), reimported.Unchanged)
	}
}

func TestCLIShareImportReplaceRemovesStaleSharedMemories(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	sourceRepo := setupRepo(t, filepath.Join(base, "source"))
	targetRepo := setupRepo(t, filepath.Join(base, "target"))

	withCwd(t, sourceRepo)
	first := runCLI(t, "add", "--thread", "T-SHARE", "--title", "Replace first", "--summary", "Replace memory one", "--entities", "file_src_first_ts,ext_ts")
	second := runCLI(t, "add", "--thread", "T-SHARE", "--title", "Replace second", "--summary", "Replace memory two", "--entities", "file_src_second_ts,ext_ts")

	var firstAdd addResp
	if err := json.Unmarshal(first, &firstAdd); err != nil {
		t.Fatalf("decode first add response: %v", err)
	}
	var secondAdd addResp
	if err := json.Unmarshal(second, &secondAdd); err != nil {
		t.Fatalf("decode second add response: %v", err)
	}

	exportOut := runCLI(t, "share", "export")
	var exported shareExportResponse
	if err := json.Unmarshal(exportOut, &exported); err != nil {
		t.Fatalf("decode share export response: %v", err)
	}

	memoriesPath := filepath.Join(exported.BundleDir, shareMemoriesFileName)
	exportedRecords, err := readShareMemoryRecords(memoriesPath)
	if err != nil {
		t.Fatalf("read exported records: %v", err)
	}
	if len(exportedRecords) < 2 {
		t.Fatalf("expected at least 2 records in export, got %d", len(exportedRecords))
	}

	withCwd(t, targetRepo)
	_ = runCLI(t, "share", "import", "--in", exported.BundleDir, "--allow-repo-mismatch")

	keepRecord := exportedRecords[0]
	if err := writeJSONLines(memoriesPath, []shareMemoryRecord{keepRecord}); err != nil {
		t.Fatalf("rewrite memories.jsonl: %v", err)
	}
	manifestPath := filepath.Join(exported.BundleDir, shareManifestFileName)
	manifest, err := readShareManifest(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest.MemoryCount = 1
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	replaceOut := runCLI(t, "share", "import", "--in", exported.BundleDir, "--allow-repo-mismatch", "--replace")
	var replaceResp shareImportResponse
	if err := json.Unmarshal(replaceOut, &replaceResp); err != nil {
		t.Fatalf("decode replace response: %v", err)
	}
	if replaceResp.Deleted != 1 {
		t.Fatalf("expected deleted=1, got %d", replaceResp.Deleted)
	}

	keptLocalID := shareLocalMemoryID(exported.RepoID, keepRecord.SourceID)
	_ = runCLI(t, "show", keptLocalID)

	for _, sourceID := range []string{firstAdd.ID, secondAdd.ID} {
		if sourceID == keepRecord.SourceID {
			continue
		}
		removedLocalID := shareLocalMemoryID(exported.RepoID, sourceID)
		errOut := runCLIExpectError(t, "show", removedLocalID)
		if !strings.Contains(errOut, "id not found") {
			t.Fatalf("expected removed id %s to be missing, got: %s", removedLocalID, errOut)
		}
	}
}

func TestCLIShareImportReplaceKeepsReceiverLocalMemories(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	sourceRepo := setupRepo(t, filepath.Join(base, "source"))
	targetRepo := setupRepo(t, filepath.Join(base, "target"))

	withCwd(t, sourceRepo)
	first := runCLI(t, "add", "--thread", "T-SHARE", "--title", "Keep first", "--summary", "Keep memory one", "--entities", "file_src_first_ts,ext_ts")
	second := runCLI(t, "add", "--thread", "T-SHARE", "--title", "Keep second", "--summary", "Keep memory two", "--entities", "file_src_second_ts,ext_ts")

	var firstAdd addResp
	if err := json.Unmarshal(first, &firstAdd); err != nil {
		t.Fatalf("decode first add response: %v", err)
	}
	var secondAdd addResp
	if err := json.Unmarshal(second, &secondAdd); err != nil {
		t.Fatalf("decode second add response: %v", err)
	}

	exportOut := runCLI(t, "share", "export")
	var exported shareExportResponse
	if err := json.Unmarshal(exportOut, &exported); err != nil {
		t.Fatalf("decode share export response: %v", err)
	}

	memoriesPath := filepath.Join(exported.BundleDir, shareMemoriesFileName)
	exportedRecords, err := readShareMemoryRecords(memoriesPath)
	if err != nil {
		t.Fatalf("read exported records: %v", err)
	}
	if len(exportedRecords) < 2 {
		t.Fatalf("expected at least 2 records in export, got %d", len(exportedRecords))
	}

	withCwd(t, targetRepo)
	localOnly := runCLI(t, "add", "--title", "Receiver local only", "--summary", "Do not delete me", "--entities", "file_local_only_ts")
	var localOnlyAdd addResp
	if err := json.Unmarshal(localOnly, &localOnlyAdd); err != nil {
		t.Fatalf("decode local-only add response: %v", err)
	}

	_ = runCLI(t, "share", "import", "--in", exported.BundleDir, "--allow-repo-mismatch")

	keepRecord := exportedRecords[0]
	if err := writeJSONLines(memoriesPath, []shareMemoryRecord{keepRecord}); err != nil {
		t.Fatalf("rewrite memories.jsonl: %v", err)
	}
	manifestPath := filepath.Join(exported.BundleDir, shareManifestFileName)
	manifest, err := readShareManifest(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest.MemoryCount = 1
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	_ = runCLI(t, "share", "import", "--in", exported.BundleDir, "--allow-repo-mismatch", "--replace")

	// Local, receiver-authored memory should remain even when replacing shared imports.
	localShow := runCLI(t, "show", localOnlyAdd.ID)
	var localResp showResp
	if err := json.Unmarshal(localShow, &localResp); err != nil {
		t.Fatalf("decode local-only show response: %v", err)
	}
	if localResp.Memory.ID != localOnlyAdd.ID {
		t.Fatalf("expected local memory id %s, got %s", localOnlyAdd.ID, localResp.Memory.ID)
	}

	// One shared record should be removed, one kept.
	keptLocalID := shareLocalMemoryID(exported.RepoID, keepRecord.SourceID)
	_ = runCLI(t, "show", keptLocalID)

	for _, sourceID := range []string{firstAdd.ID, secondAdd.ID} {
		if sourceID == keepRecord.SourceID {
			continue
		}
		removedLocalID := shareLocalMemoryID(exported.RepoID, sourceID)
		errOut := runCLIExpectError(t, "show", removedLocalID)
		if !strings.Contains(errOut, "id not found") {
			t.Fatalf("expected removed id %s to be missing, got: %s", removedLocalID, errOut)
		}
	}
}

func TestCLIShareImportRejectsInvalidManifest(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	sourceRepo := setupRepo(t, filepath.Join(base, "source"))
	targetRepo := setupRepo(t, filepath.Join(base, "target"))

	withCwd(t, sourceRepo)
	_ = runCLI(t, "add", "--title", "Manifest test", "--summary", "manifest", "--entities", "file_manifest_ts")

	exportOut := runCLI(t, "share", "export")
	var exported shareExportResponse
	if err := json.Unmarshal(exportOut, &exported); err != nil {
		t.Fatalf("decode share export response: %v", err)
	}

	manifestPath := filepath.Join(exported.BundleDir, shareManifestFileName)
	manifest, err := readShareManifest(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	manifest.SourceRepoID = ""
	if err := writeJSONFile(manifestPath, manifest); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}

	withCwd(t, targetRepo)
	errOut := runCLIExpectError(t, "share", "import", "--in", exported.BundleDir, "--allow-repo-mismatch")
	if !strings.Contains(errOut, "manifest missing source_repo_id") {
		t.Fatalf("expected source_repo_id validation error, got: %s", errOut)
	}
}

func TestCLIShareExportSkipsSupersededAndDeleted(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	sourceRepo := setupRepo(t, filepath.Join(base, "source"))
	withCwd(t, sourceRepo)

	activeOut := runCLI(t, "add", "--title", "Active keep", "--summary", "Keep me", "--entities", "file_keep_ts")
	replaceOut := runCLI(t, "add", "--title", "Will be superseded", "--summary", "Old", "--entities", "file_old_ts")
	deletedOut := runCLI(t, "add", "--title", "Will be deleted", "--summary", "Delete me", "--entities", "file_deleted_ts")

	var activeAdd addResp
	if err := json.Unmarshal(activeOut, &activeAdd); err != nil {
		t.Fatalf("decode active add response: %v", err)
	}
	var replaceAdd addResp
	if err := json.Unmarshal(replaceOut, &replaceAdd); err != nil {
		t.Fatalf("decode replace add response: %v", err)
	}
	var deletedAdd addResp
	if err := json.Unmarshal(deletedOut, &deletedAdd); err != nil {
		t.Fatalf("decode deleted add response: %v", err)
	}

	supersedeOut := runCLI(t, "supersede", "--title", "Superseded new", "--summary", "New", replaceAdd.ID)
	var supersede supersedeResp
	if err := json.Unmarshal(supersedeOut, &supersede); err != nil {
		t.Fatalf("decode supersede response: %v", err)
	}
	_ = runCLI(t, "forget", deletedAdd.ID)

	exportOut := runCLI(t, "share", "export")
	var exported shareExportResponse
	if err := json.Unmarshal(exportOut, &exported); err != nil {
		t.Fatalf("decode share export response: %v", err)
	}

	memoriesPath := filepath.Join(exported.BundleDir, shareMemoriesFileName)
	exportedRecords, err := readShareMemoryRecords(memoriesPath)
	if err != nil {
		t.Fatalf("read exported records: %v", err)
	}

	recordIDs := make(map[string]struct{}, len(exportedRecords))
	for _, record := range exportedRecords {
		recordIDs[record.SourceID] = struct{}{}
	}

	if _, ok := recordIDs[activeAdd.ID]; !ok {
		t.Fatalf("expected active id %s in export", activeAdd.ID)
	}
	if _, ok := recordIDs[supersede.NewID]; !ok {
		t.Fatalf("expected superseded replacement id %s in export", supersede.NewID)
	}
	if _, ok := recordIDs[supersede.OldID]; ok {
		t.Fatalf("did not expect superseded old id %s in export", supersede.OldID)
	}
	if _, ok := recordIDs[deletedAdd.ID]; ok {
		t.Fatalf("did not expect deleted id %s in export", deletedAdd.ID)
	}
}
