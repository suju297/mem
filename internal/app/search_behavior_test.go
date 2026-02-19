package app

import (
	"strings"
	"testing"
	"time"

	"mempack/internal/config"
	"mempack/internal/pack"
	"mempack/internal/store"
)

func TestQueryRewriteDigits(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	writeTestConfig(t, base, func(cfg *config.Config) {
		cfg.EmbeddingProvider = "none"
	})

	addMemory(t, "M-DELTA", "Delta Entry", "delta-99")

	pack, err := buildContextPack("delta99", ContextOptions{}, nil)
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if !memorySummaryContains(pack.TopMemories, "delta-99") {
		t.Fatalf("expected delta-99 memory to be returned")
	}
	if !containsString(pack.SearchMeta.RewritesApplied, "delta99 -> delta 99") {
		t.Fatalf("expected rewrite delta99 -> delta 99, got %+v", pack.SearchMeta.RewritesApplied)
	}
}

func TestQueryOrderInsensitive(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	writeTestConfig(t, base, func(cfg *config.Config) {
		cfg.EmbeddingProvider = "none"
	})

	addMemory(t, "M-ONE", "Order A", "one two")
	addMemory(t, "M-TWO", "Order B", "two one")

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

	resultsA, _, err := st.SearchMemories(repoInfo.ID, "default", "one two", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	resultsB, _, err := st.SearchMemories(repoInfo.ID, "default", "two one", 10)
	if err != nil {
		t.Fatalf("search memories: %v", err)
	}
	if len(resultsA) == 0 || len(resultsB) == 0 {
		t.Fatalf("expected results for both queries")
	}
	if resultsA[0].ID != resultsB[0].ID {
		t.Fatalf("expected same top result, got %s vs %s", resultsA[0].ID, resultsB[0].ID)
	}
}

func TestSearchMetaFieldsDeterministic(t *testing.T) {
	base := t.TempDir()
	setXDGEnv(t, base)

	repoDir := setupRepo(t, base)
	withCwd(t, repoDir)

	writeTestConfig(t, base, func(cfg *config.Config) {
		cfg.EmbeddingProvider = "none"
	})

	addMemory(t, "M-DELTA", "Delta Entry", "delta-99")

	pack, err := buildContextPack("delta99", ContextOptions{}, nil)
	if err != nil {
		t.Fatalf("build context: %v", err)
	}
	if pack.SearchMeta.ModeUsed != "bm25" || pack.SearchMeta.Mode != "bm25" {
		t.Fatalf("expected mode_used=bm25, got mode=%s mode_used=%s", pack.SearchMeta.Mode, pack.SearchMeta.ModeUsed)
	}
	if pack.SearchMeta.VectorUsed {
		t.Fatalf("expected vector_used=false")
	}
	if len(pack.SearchMeta.RewritesApplied) == 0 {
		t.Fatalf("expected rewrites_applied to be populated")
	}
}

func addMemory(t testing.TB, id, title, summary string) {
	t.Helper()
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
	if err := st.EnsureRepo(repoInfo); err != nil {
		t.Fatalf("store repo error: %v", err)
	}

	_, err = st.AddMemory(store.AddMemoryInput{
		ID:            id,
		RepoID:        repoInfo.ID,
		Workspace:     "default",
		ThreadID:      "T-TEST",
		Title:         title,
		Summary:       summary,
		SummaryTokens: 1,
		TagsJSON:      "[]",
		TagsText:      "",
		EntitiesJSON:  "[]",
		EntitiesText:  "",
		AnchorCommit:  repoInfo.Head,
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("add memory: %v", err)
	}
}

func writeTestConfig(t testing.TB, _ string, update func(cfg *config.Config)) {
	t.Helper()
	cfg, err := config.Default()
	if err != nil {
		t.Fatalf("config default error: %v", err)
	}
	if update != nil {
		update(&cfg)
	}
	if err := cfg.Save(); err != nil {
		t.Fatalf("config save error: %v", err)
	}
}

func memorySummaryContains(items []pack.MemoryItem, needle string) bool {
	for _, item := range items {
		if strings.Contains(item.Summary, needle) {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
