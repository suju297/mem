package app

import (
	"fmt"
	"strings"
	"time"

	"mem/internal/config"
	"mem/internal/pack"
	"mem/internal/store"
)

func openUsageStore(cfg config.Config) (*store.Store, func(), error) {
	st, err := store.OpenUsage(cfg.UsageDBPath())
	if err != nil {
		return nil, nil, err
	}
	return st, func() { _ = st.Close() }, nil
}

func attachUsageSnapshot(result *pack.ContextPack) error {
	if result == nil {
		return fmt.Errorf("context pack is nil")
	}
	repoID := strings.TrimSpace(result.Repo.RepoID)
	if repoID == "" {
		return fmt.Errorf("repo_id is required for usage tracking")
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config error: %v", err)
	}
	st, release, err := openUsageStore(cfg)
	if err != nil {
		return fmt.Errorf("usage store error: %v", err)
	}
	defer release()

	delta := store.UsageDelta{
		RequestCount:    1,
		CandidateTokens: result.Budget.CandidateTotal,
		UsedTokens:      result.Budget.UsedTotal,
		SavedTokens:     result.Budget.SavedTotal,
		TruncatedTokens: result.Budget.TruncatedTotal,
		DroppedTokens:   result.Budget.DroppedTotal,
	}
	if err := st.IncrementUsage(repoID, delta, time.Now().UTC()); err != nil {
		return fmt.Errorf("usage increment error: %v", err)
	}

	snapshot, err := st.GetUsageSnapshot(repoID)
	if err != nil {
		return fmt.Errorf("usage snapshot error: %v", err)
	}
	result.Usage = usageSnapshotToPack(snapshot)
	return nil
}

func loadUsageReport(repoOverride string, requireRepo bool) (usageResponse, error) {
	cfg, err := loadConfig()
	if err != nil {
		return usageResponse{}, fmt.Errorf("config error: %v", err)
	}
	repoInfo, err := resolveRepoWithOptions(&cfg, strings.TrimSpace(repoOverride), repoResolveOptions{RequireRepo: requireRepo})
	if err != nil {
		return usageResponse{}, fmt.Errorf("repo detection error: %v", err)
	}

	st, release, err := openUsageStore(cfg)
	if err != nil {
		return usageResponse{}, fmt.Errorf("usage store error: %v", err)
	}
	defer release()

	snapshot, err := st.GetUsageSnapshot(repoInfo.ID)
	if err != nil {
		return usageResponse{}, fmt.Errorf("usage snapshot error: %v", err)
	}

	packSnapshot := usageSnapshotToPack(snapshot)
	return usageResponse{
		Scope:   "repo",
		RepoID:  repoInfo.ID,
		Repo:    &packSnapshot.Repo,
		Overall: packSnapshot.Overall,
	}, nil
}

func loadProfileUsageReport() (usageResponse, error) {
	cfg, err := loadConfig()
	if err != nil {
		return usageResponse{}, fmt.Errorf("config error: %v", err)
	}

	st, release, err := openUsageStore(cfg)
	if err != nil {
		return usageResponse{}, fmt.Errorf("usage store error: %v", err)
	}
	defer release()

	overall, err := st.GetUsageRollup(store.UsageScopeOverall, "")
	if err != nil {
		return usageResponse{}, fmt.Errorf("usage snapshot error: %v", err)
	}

	return usageResponse{
		Scope:   "profile",
		Overall: usageTotalsToPack(overall),
	}, nil
}

func usageSnapshotToPack(snapshot store.UsageSnapshot) *pack.UsageSnapshot {
	return &pack.UsageSnapshot{
		Repo:    usageTotalsToPack(snapshot.Repo),
		Overall: usageTotalsToPack(snapshot.Overall),
	}
}

func usageTotalsToPack(rollup store.UsageRollup) pack.UsageTotals {
	return pack.UsageTotals{
		RequestCount:    rollup.RequestCount,
		CandidateTokens: rollup.CandidateTokens,
		UsedTokens:      rollup.UsedTokens,
		SavedTokens:     rollup.SavedTokens,
		TruncatedTokens: rollup.TruncatedTokens,
		DroppedTokens:   rollup.DroppedTokens,
		UpdatedAt:       formatTime(rollup.UpdatedAt),
	}
}
