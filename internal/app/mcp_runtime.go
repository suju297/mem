package app

import (
	"fmt"
	"sync"

	"mempack/internal/config"
	"mempack/internal/store"
)

type mcpRuntime struct {
	mu      sync.Mutex
	baseCfg config.Config
	stores  map[string]*store.Store
	closed  bool
}

var (
	mcpRuntimeMu sync.RWMutex
	mcpRuntimeV  *mcpRuntime
)

func newMCPRuntime(cfg config.Config) *mcpRuntime {
	return &mcpRuntime{
		baseCfg: cloneConfig(cfg),
		stores:  map[string]*store.Store{},
	}
}

func setActiveMCPRuntime(rt *mcpRuntime) {
	mcpRuntimeMu.Lock()
	defer mcpRuntimeMu.Unlock()
	mcpRuntimeV = rt
}

func activeMCPRuntime() *mcpRuntime {
	mcpRuntimeMu.RLock()
	defer mcpRuntimeMu.RUnlock()
	return mcpRuntimeV
}

func (rt *mcpRuntime) configCopy() config.Config {
	if rt == nil {
		return config.Config{}
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return cloneConfig(rt.baseCfg)
}

func (rt *mcpRuntime) openStore(cfg config.Config, repoID string) (*store.Store, error) {
	if rt == nil {
		return nil, fmt.Errorf("runtime is nil")
	}

	rt.mu.Lock()
	if rt.closed {
		rt.mu.Unlock()
		return nil, fmt.Errorf("mcp runtime closed")
	}
	if st, ok := rt.stores[repoID]; ok {
		rt.mu.Unlock()
		return st, nil
	}
	rt.mu.Unlock()

	opened, err := store.Open(cfg.RepoDBPath(repoID))
	if err != nil {
		return nil, err
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.closed {
		_ = opened.Close()
		return nil, fmt.Errorf("mcp runtime closed")
	}
	if st, ok := rt.stores[repoID]; ok {
		_ = opened.Close()
		return st, nil
	}
	rt.stores[repoID] = opened
	return opened, nil
}

func (rt *mcpRuntime) close() error {
	if rt == nil {
		return nil
	}
	rt.mu.Lock()
	if rt.closed {
		rt.mu.Unlock()
		return nil
	}
	rt.closed = true
	stores := make([]*store.Store, 0, len(rt.stores))
	for _, st := range rt.stores {
		stores = append(stores, st)
	}
	rt.stores = map[string]*store.Store{}
	rt.mu.Unlock()

	var firstErr error
	for _, st := range stores {
		if err := st.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (rt *mcpRuntime) mergeRepoState(cfg config.Config) {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.baseCfg.ActiveRepo = cfg.ActiveRepo
	if rt.baseCfg.RepoCache == nil {
		rt.baseCfg.RepoCache = map[string]string{}
	}
	for key, value := range cfg.RepoCache {
		if key == "" || value == "" {
			continue
		}
		rt.baseCfg.RepoCache[key] = value
	}
}

func cloneConfig(cfg config.Config) config.Config {
	out := cfg
	if cfg.RepoCache == nil {
		out.RepoCache = map[string]string{}
		return out
	}
	out.RepoCache = make(map[string]string, len(cfg.RepoCache))
	for key, value := range cfg.RepoCache {
		out.RepoCache[key] = value
	}
	return out
}
