package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// startFileWatcher watches the vault directory for changes and emits
// Wails events so the frontend can refresh automatically.
func (a *GranitApp) startFileWatcher() {
	if a.vaultRoot == "" || a.ctx == nil {
		return
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}

	// Stop any previous watcher
	if a.watcher != nil {
		a.watcher.Close()
	}
	a.watcher = watcher

	// Walk vault and add directories
	filepath.Walk(a.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			// Skip hidden dirs and common noise
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "_blog" {
				return filepath.SkipDir
			}
			watcher.Add(path)
		}
		return nil
	})

	// Debounce: collect events over 500ms then emit one refresh
	go func() {
		var debounceTimer *time.Timer
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Only care about markdown files
				if !strings.HasSuffix(event.Name, ".md") {
					continue
				}
				// Debounce
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
					// Re-scan vault and rebuild index
					if a.vault != nil {
						a.vault.Scan()
						if a.index != nil {
							a.index.Build()
						}
					}
					// Notify frontend
					wailsRuntime.EventsEmit(a.ctx, "vault:changed")
				})
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

// stopFileWatcher stops the file watcher goroutine.
func (a *GranitApp) stopFileWatcher() {
	if a.watcher != nil {
		a.watcher.Close()
		a.watcher = nil
	}
}
