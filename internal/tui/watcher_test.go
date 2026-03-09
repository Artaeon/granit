package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// shouldIgnorePath / shouldIgnoreDir
// ---------------------------------------------------------------------------

func TestFileWatcher_IgnoredPaths(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		// Directories that should be ignored.
		{".git", true},
		{".granit-trash", true},
		{".obsidian", true},
		{"node_modules", true},
		{".granit", true},
		{".hidden", true}, // any dot-prefixed directory

		// Directories that should NOT be ignored.
		{"notes", false},
		{"daily", false},
		{"projects", false},
	}
	for _, tt := range tests {
		got := shouldIgnoreDir(tt.path)
		if got != tt.want {
			t.Errorf("shouldIgnoreDir(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}

	pathTests := []struct {
		path string
		want bool
	}{
		// Temp/swap files that should be ignored.
		{"/vault/notes/.file.md.swp", true},
		{"/vault/notes/file.swo", true},
		{"/vault/notes/file~", true},
		{"/vault/.#lockfile", true},
		{"/vault/#autosave#", true},
		{"/vault/.DS_Store", true},
		{"/vault/4913", true}, // vim temp probe

		// Normal files that should NOT be ignored.
		{"/vault/notes/hello.md", false},
		{"/vault/readme.txt", false},
		{"/vault/notes/deep/path/note.md", false},
	}
	for _, tt := range pathTests {
		got := shouldIgnorePath(tt.path)
		if got != tt.want {
			t.Errorf("shouldIgnorePath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Debounce coalescing
// ---------------------------------------------------------------------------

func TestFileWatcher_DebounceCoalesces(t *testing.T) {
	dir := t.TempDir()

	fw := NewFileWatcher(dir)
	fw.debounce = 200 * time.Millisecond
	cmd := fw.Start()
	if cmd == nil {
		t.Fatal("Start returned nil cmd — fsnotify failed to initialise")
	}
	defer fw.Stop()

	// Create several markdown files in rapid succession.
	for i := 0; i < 5; i++ {
		name := filepath.Join(dir, "note"+string(rune('a'+i))+".md")
		if err := os.WriteFile(name, []byte("# Note"), 0644); err != nil {
			t.Fatalf("failed to create file: %v", err)
		}
	}

	// Wait for debounce to fire (debounce + margin).
	time.Sleep(500 * time.Millisecond)

	// The event channel should have exactly one coalesced message.
	select {
	case msg := <-fw.eventChan:
		if len(msg.paths) == 0 {
			t.Error("expected at least one path in coalesced message")
		}
		t.Logf("coalesced %d paths into one message", len(msg.paths))
	default:
		t.Error("expected a coalesced message on eventChan, got none")
	}

	// Channel should be drained — no second message.
	select {
	case extra := <-fw.eventChan:
		t.Errorf("expected no second message, got one with %d paths", len(extra.paths))
	default:
		// Good — no extra message.
	}
}

// ---------------------------------------------------------------------------
// Start / Stop — goroutine leak check
// ---------------------------------------------------------------------------

func TestFileWatcher_StartStop(t *testing.T) {
	dir := t.TempDir()

	// Snapshot goroutine count before starting the watcher.
	before := runtime.NumGoroutine()

	fw := NewFileWatcher(dir)
	cmd := fw.Start()
	if cmd == nil {
		t.Fatal("Start returned nil cmd")
	}

	// Give the background goroutine a moment to launch.
	time.Sleep(50 * time.Millisecond)

	fw.Stop()

	// Allow goroutines to wind down.
	time.Sleep(200 * time.Millisecond)

	after := runtime.NumGoroutine()

	// Allow a small margin (other tests, GC finalizers, etc. may add a
	// goroutine or two), but the watcher should not leak.
	if after > before+2 {
		t.Errorf("possible goroutine leak: before=%d, after=%d", before, after)
	}

	// Calling Stop a second time should be safe.
	fw.Stop()
}

// ---------------------------------------------------------------------------
// New subdirectory gets watched
// ---------------------------------------------------------------------------

func TestFileWatcher_NewDirectoryWatched(t *testing.T) {
	dir := t.TempDir()

	fw := NewFileWatcher(dir)
	fw.debounce = 200 * time.Millisecond
	cmd := fw.Start()
	if cmd == nil {
		t.Fatal("Start returned nil cmd")
	}
	defer fw.Stop()

	// Create a new subdirectory — the watcher should pick it up.
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("failed to create subdir: %v", err)
	}

	// Give the watcher time to notice the new directory and add it.
	time.Sleep(100 * time.Millisecond)

	// Now create a markdown file inside the new subdirectory.
	notePath := filepath.Join(subdir, "nested.md")
	if err := os.WriteFile(notePath, []byte("# Nested"), 0644); err != nil {
		t.Fatalf("failed to create nested file: %v", err)
	}

	// Wait for debounce.
	time.Sleep(500 * time.Millisecond)

	select {
	case msg := <-fw.eventChan:
		found := false
		for _, p := range msg.paths {
			if p == notePath {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in changed paths, got %v", notePath, msg.paths)
		}
	default:
		t.Error("expected a change message for file in new subdirectory, got none")
	}
}
