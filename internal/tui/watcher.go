package tui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// FileWatcher uses fsnotify to watch the vault directory for external file
// changes and sends debounced fileChangeMsg events to the TUI.
type FileWatcher struct {
	vaultPath string
	enabled   bool
	watcher   *fsnotify.Watcher

	// Debounce: accumulate events for 500ms before firing a single message.
	mu        sync.Mutex
	pending   map[string]fsnotify.Op
	timer     *time.Timer
	debounce  time.Duration
	eventChan chan fileChangeMsg
	stopChan  chan struct{}
}

// fileChangeMsg is sent when the watcher detects file system changes.
type fileChangeMsg struct {
	paths   []string // all paths that changed (created + modified + deleted)
	created []string
	deleted []string
}

// NewFileWatcher creates a FileWatcher for the given vault directory.
// Watching is enabled by default with a 500ms debounce window.
func NewFileWatcher(vaultPath string) *FileWatcher {
	return &FileWatcher{
		vaultPath: vaultPath,
		enabled:   true,
		debounce:  500 * time.Millisecond,
		pending:   make(map[string]fsnotify.Op),
		eventChan: make(chan fileChangeMsg, 1),
		stopChan:  make(chan struct{}),
	}
}

// SetEnabled enables or disables file watching.
func (fw *FileWatcher) SetEnabled(enabled bool) {
	fw.enabled = enabled
}

// IsEnabled returns whether file watching is currently active.
func (fw *FileWatcher) IsEnabled() bool {
	return fw.enabled
}

// Start initialises the fsnotify watcher, adds all vault subdirectories, and
// returns a tea.Cmd that blocks until the first change event arrives. Call this
// from Init().
func (fw *FileWatcher) Start() tea.Cmd {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		// Silently degrade — watcher will be nil and no events fire.
		return nil
	}
	fw.watcher = w

	// Walk the vault to add every directory (fsnotify is not recursive).
	_ = filepath.Walk(fw.vaultPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if shouldIgnoreDir(name) {
				return filepath.SkipDir
			}
			_ = w.Add(path)
		}
		return nil
	})

	// Background goroutine: read fsnotify events, filter, debounce, and push
	// fileChangeMsg values onto eventChan.
	go fw.loop()

	return fw.waitForEvent()
}

// Stop closes the underlying fsnotify watcher and signals the background
// goroutine to exit. Safe to call multiple times.
func (fw *FileWatcher) Stop() {
	select {
	case <-fw.stopChan:
		// Already stopped.
	default:
		close(fw.stopChan)
	}
	if fw.watcher != nil {
		fw.watcher.Close()
	}
}

// waitForEvent returns a tea.Cmd that blocks until a debounced change event
// is ready, then delivers it as a tea.Msg.
func (fw *FileWatcher) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-fw.eventChan:
			return msg
		case <-fw.stopChan:
			return nil
		}
	}
}

// loop is the background goroutine that reads raw fsnotify events, filters
// irrelevant paths, and debounces bursts into a single fileChangeMsg.
func (fw *FileWatcher) loop() {
	for {
		select {
		case <-fw.stopChan:
			return

		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			if !fw.enabled {
				continue
			}
			if shouldIgnorePath(event.Name) {
				continue
			}

			// When a new directory is created, start watching it too.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if !shouldIgnoreDir(info.Name()) {
						_ = fw.watcher.Add(event.Name)
					}
					continue // directory creation is not a note change
				}
			}

			// Only care about markdown files.
			if !strings.HasSuffix(strings.ToLower(event.Name), ".md") {
				continue
			}

			fw.mu.Lock()
			fw.pending[event.Name] = event.Op

			// Reset or start the debounce timer.
			if fw.timer != nil {
				fw.timer.Stop()
			}
			fw.timer = time.AfterFunc(fw.debounce, fw.flush)
			fw.mu.Unlock()

		case _, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			// Silently ignore watcher errors.
		}
	}
}

// flush is called by the debounce timer. It drains the pending map and pushes
// a single fileChangeMsg onto eventChan.
func (fw *FileWatcher) flush() {
	fw.mu.Lock()
	if len(fw.pending) == 0 {
		fw.mu.Unlock()
		return
	}
	msg := fileChangeMsg{}
	for path, op := range fw.pending {
		msg.paths = append(msg.paths, path)
		if op.Has(fsnotify.Create) {
			msg.created = append(msg.created, path)
		}
		if op.Has(fsnotify.Remove) || op.Has(fsnotify.Rename) {
			msg.deleted = append(msg.deleted, path)
		}
	}
	fw.pending = make(map[string]fsnotify.Op)
	fw.mu.Unlock()

	// Non-blocking send: drop if channel already has an event queued.
	select {
	case fw.eventChan <- msg:
	default:
	}
}

// shouldIgnoreDir returns true for directories whose contents should never
// trigger change events.
func shouldIgnoreDir(name string) bool {
	switch name {
	case ".git", ".granit-trash", ".granit", ".obsidian", "node_modules":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// shouldIgnorePath returns true for temporary and system files that editors
// create during save operations.
func shouldIgnorePath(path string) bool {
	base := filepath.Base(path)

	// Swap files, backups, macOS metadata.
	if strings.HasSuffix(base, ".swp") ||
		strings.HasSuffix(base, ".swo") ||
		strings.HasSuffix(base, "~") ||
		strings.HasPrefix(base, ".#") ||
		strings.HasPrefix(base, "#") ||
		base == ".DS_Store" ||
		base == "4913" { // vim temp probe file
		return true
	}

	return false
}
