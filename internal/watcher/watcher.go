package watcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Op string

const (
	OpCreate Op = "create"
	OpModify Op = "modify"
	OpDelete Op = "delete"
)

type Event struct {
	Path      string
	RelPath   string
	Op        Op
	Timestamp time.Time
}

type Watcher struct {
	root      string
	fsWatcher *fsnotify.Watcher
	events    chan Event
	ignorer   func(relPath string) bool
	debounce  time.Duration
	pending   map[string]Event
	stop      chan struct{}
	stopped   chan struct{}
}

func New(root string, ignorer func(relPath string) bool) (*Watcher, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("root path required")
	}
	absRoot, err := filepath.Abs(root)
	if err == nil {
		root = absRoot
	}

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		root:      root,
		fsWatcher: fsw,
		events:    make(chan Event, 100),
		ignorer:   ignorer,
		debounce:  500 * time.Millisecond,
		pending:   make(map[string]Event),
		stop:      make(chan struct{}),
		stopped:   make(chan struct{}),
	}, nil
}

func (w *Watcher) Start() error {
	if err := w.addDirRecursive(w.root); err != nil {
		return err
	}
	go w.run()
	return nil
}

func (w *Watcher) Stop() {
	close(w.stop)
	_ = w.fsWatcher.Close()
	<-w.stopped
}

func (w *Watcher) Events() <-chan Event {
	return w.events
}

func (w *Watcher) run() {
	defer close(w.events)
	defer close(w.stopped)

	ticker := time.NewTicker(w.debounce / 2)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case ev, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.handleFsEvent(ev)
		case _, ok := <-w.fsWatcher.Errors:
			if !ok {
				continue
			}
		case now := <-ticker.C:
			w.flushPending(now)
		}
	}
}

func (w *Watcher) handleFsEvent(ev fsnotify.Event) {
	if strings.TrimSpace(ev.Name) == "" {
		return
	}
	relPath, ok := w.relPath(ev.Name)
	if !ok || relPath == "." {
		return
	}
	if w.ignorer != nil && w.ignorer(relPath) {
		return
	}

	if ev.Op&fsnotify.Create != 0 {
		if w.isDir(ev.Name) {
			_ = w.addDirRecursive(ev.Name)
			return
		}
		w.queue(relPath, ev.Name, OpCreate)
	}
	if ev.Op&fsnotify.Write != 0 {
		if w.isDir(ev.Name) {
			return
		}
		w.queue(relPath, ev.Name, OpModify)
	}
	if ev.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		w.dropPendingPath(relPath)
		w.emit(Event{Path: ev.Name, RelPath: relPath, Op: OpDelete, Timestamp: time.Now()})
	}
}

func (w *Watcher) queue(relPath, path string, op Op) {
	now := time.Now()
	if existing, ok := w.pending[relPath]; ok {
		if existing.Op == OpCreate && op == OpModify {
			op = OpCreate
		} else if op == OpCreate && existing.Op == OpModify {
			op = existing.Op
		}
	}
	w.pending[relPath] = Event{
		Path:      path,
		RelPath:   relPath,
		Op:        op,
		Timestamp: now,
	}
}

func (w *Watcher) flushPending(now time.Time) {
	for relPath, event := range w.pending {
		if now.Sub(event.Timestamp) < w.debounce {
			continue
		}
		delete(w.pending, relPath)
		w.emit(event)
	}
}

func (w *Watcher) dropPendingPath(relPath string) {
	if _, ok := w.pending[relPath]; ok {
		delete(w.pending, relPath)
	}
}

func (w *Watcher) emit(event Event) {
	w.events <- event
}

func (w *Watcher) addDirRecursive(path string) error {
	return filepath.WalkDir(path, func(next string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		relPath, ok := w.relPath(next)
		if !ok {
			return filepath.SkipDir
		}
		if relPath != "." && w.ignorer != nil && w.ignorer(relPath) {
			return filepath.SkipDir
		}
		return w.fsWatcher.Add(next)
	})
}

func (w *Watcher) relPath(path string) (string, bool) {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "" {
		return "", false
	}
	return rel, true
}

func (w *Watcher) isDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
