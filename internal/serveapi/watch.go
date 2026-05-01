package serveapi

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type fsKind int

const (
	fsCreate fsKind = iota
	fsWrite
	fsRemove
	fsRename
)

type fsEvent struct {
	Kind fsKind
	Path string
}

type watcher struct {
	root string
	log  *slog.Logger
	w    *fsnotify.Watcher
	out  chan fsEvent

	mu       sync.Mutex
	pending  map[string]fsKind
	flushIn  time.Duration
	closeOne sync.Once
	done     chan struct{}
}

func newWatcher(root string, log *slog.Logger) (*watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &watcher{
		root:    root,
		log:     log,
		w:       fw,
		out:     make(chan fsEvent, 64),
		pending: map[string]fsKind{},
		flushIn: 150 * time.Millisecond,
		done:    make(chan struct{}),
	}
	if err := w.addTree(root); err != nil {
		fw.Close()
		return nil, err
	}
	go w.loop()
	return w, nil
}

func (w *watcher) Events() <-chan fsEvent { return w.out }

func (w *watcher) Close() error {
	w.closeOne.Do(func() {
		close(w.done)
		w.w.Close()
		close(w.out)
	})
	return nil
}

func (w *watcher) addTree(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if path != root && strings.HasPrefix(base, ".") {
			return fs.SkipDir
		}
		return w.w.Add(path)
	})
}

func (w *watcher) loop() {
	t := time.NewTimer(time.Hour)
	t.Stop()
	for {
		select {
		case <-w.done:
			return
		case ev, ok := <-w.w.Events:
			if !ok {
				return
			}
			k, useful := classify(ev.Op)
			if !useful {
				continue
			}
			if ev.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					base := filepath.Base(ev.Name)
					if !strings.HasPrefix(base, ".") {
						_ = w.w.Add(ev.Name)
					}
				}
			}
			w.mu.Lock()
			w.pending[ev.Name] = k
			w.mu.Unlock()
			t.Reset(w.flushIn)
		case err, ok := <-w.w.Errors:
			if !ok {
				return
			}
			w.log.Warn("fsnotify error", "err", err)
		case <-t.C:
			w.flush()
		}
	}
}

func (w *watcher) flush() {
	w.mu.Lock()
	pending := w.pending
	w.pending = map[string]fsKind{}
	w.mu.Unlock()
	for path, k := range pending {
		w.out <- fsEvent{Kind: k, Path: path}
	}
}

func classify(op fsnotify.Op) (fsKind, bool) {
	switch {
	case op&fsnotify.Remove != 0:
		return fsRemove, true
	case op&fsnotify.Rename != 0:
		return fsRename, true
	case op&fsnotify.Create != 0:
		return fsCreate, true
	case op&fsnotify.Write != 0:
		return fsWrite, true
	}
	return 0, false
}
