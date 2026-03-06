package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// FileWatcher polls the vault directory for file changes and notifies the TUI
// when markdown files are created, modified, or deleted.
type FileWatcher struct {
	vaultPath string
	enabled   bool
	debounce  time.Duration
	lastEvent time.Time
	snapshot  map[string]time.Time
}

// fileChangeMsg is sent when the watcher detects file system changes.
type fileChangeMsg struct {
	paths   []string // all paths that changed (created + modified + deleted)
	created []string
	deleted []string
}

// fileWatchTickMsg signals the app to run the next file-watch check.
type fileWatchTickMsg struct{}

// NewFileWatcher creates a FileWatcher for the given vault directory.
// Watching is enabled by default with a 500ms debounce window.
func NewFileWatcher(vaultPath string) *FileWatcher {
	return &FileWatcher{
		vaultPath: vaultPath,
		enabled:   true,
		debounce:  500 * time.Millisecond,
		lastEvent: time.Time{},
		snapshot:  nil,
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

// Start returns a tea.Cmd that builds the initial snapshot and schedules the
// first tick after 2 seconds. Call this from Init().
func (fw *FileWatcher) Start() tea.Cmd {
	fw.snapshot = scanVaultFiles(fw.vaultPath)
	return func() tea.Msg {
		time.Sleep(2 * time.Second)
		return fileWatchTickMsg{}
	}
}

// Check compares the current file system state against the stored snapshot and
// returns a fileChangeMsg if any .md files were created, modified, or deleted.
// The second return value is true when changes were detected.
func (fw *FileWatcher) Check() (fileChangeMsg, bool) {
	if !fw.enabled {
		return fileChangeMsg{}, false
	}

	now := time.Now()
	if now.Sub(fw.lastEvent) < fw.debounce {
		return fileChangeMsg{}, false
	}

	current := scanVaultFiles(fw.vaultPath)
	msg := diffSnapshots(fw.snapshot, current)

	if len(msg.paths) == 0 {
		return fileChangeMsg{}, false
	}

	fw.snapshot = current
	fw.lastEvent = now
	return msg, true
}

// Tick returns a tea.Cmd that schedules the next watch check after 2 seconds.
// Call this after handling fileWatchTickMsg to keep the polling loop alive.
func (fw *FileWatcher) Tick() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(2 * time.Second)
		return fileWatchTickMsg{}
	}
}

// scanVaultFiles walks the vault directory recursively and records the ModTime
// of every .md file. Hidden files/directories and the .granit/ directory are
// skipped.
func scanVaultFiles(root string) map[string]time.Time {
	files := make(map[string]time.Time)

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}

		name := info.Name()

		// Skip hidden files and directories.
		if strings.HasPrefix(name, ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Only track markdown files.
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}

		files[path] = info.ModTime()
		return nil
	})

	return files
}

// diffSnapshots compares an old and new snapshot and returns a fileChangeMsg
// listing every path that was created, modified, or deleted.
func diffSnapshots(old, current map[string]time.Time) fileChangeMsg {
	var msg fileChangeMsg

	// Detect created and modified files.
	for path, modTime := range current {
		oldTime, exists := old[path]
		if !exists {
			msg.created = append(msg.created, path)
			msg.paths = append(msg.paths, path)
		} else if !modTime.Equal(oldTime) {
			msg.paths = append(msg.paths, path)
		}
	}

	// Detect deleted files.
	for path := range old {
		if _, exists := current[path]; !exists {
			msg.deleted = append(msg.deleted, path)
			msg.paths = append(msg.paths, path)
		}
	}

	return msg
}
