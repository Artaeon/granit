package serveapi

import (
	"sync"
	"time"

	"github.com/artaeon/granit/internal/vault"
)

// Habits-section parse cache, keyed by daily-note path + mtime. The
// /habits handler walks ~90 daily notes per request and re-parses each
// one's `## Habits` section. The parse is O(lines × tiny-regexes) per
// file, but the real cost is the EnsureLoaded that reads file content
// from disk on a watcher-triggered rescan (ScanFast wipes the in-memory
// notes map; subsequent EnsureLoaded calls hit disk).
//
// Cache key is (relPath, mtime). When the daily note hasn't changed on
// disk since we last parsed it, we reuse the cached map without any
// disk I/O — both EnsureLoaded and parseHabitsSection are skipped. On
// a 90-daily-window vault that hasn't been edited, /habits drops from
// "stat + parse 90 files" to "stat + 90 map lookups".

type habitsParsedEntry struct {
	mtime  time.Time
	habits map[string]bool // nil when the note has no Habits section
}

var (
	habitsCacheMu sync.RWMutex
	habitsCache   = map[string]habitsParsedEntry{}
)

// parseHabitsSectionCached returns the Habits-section map for a daily
// note. On a (path, mtime) cache miss, EnsureLoaded reads the body and
// parseHabitsSection extracts the checkbox state; both results land in
// the cache for the next call. Returns nil + false when the note has
// no `## Habits` section (caller should skip the date).
//
// The returned map is the SAME reference shared across callers. Treat
// it as read-only — concurrent /habits requests would otherwise race.
func (s *Server) parseHabitsSectionCached(n *vault.Note) (map[string]bool, bool) {
	habitsCacheMu.RLock()
	e, ok := habitsCache[n.RelPath]
	habitsCacheMu.RUnlock()
	if ok && e.mtime.Equal(n.ModTime) {
		return e.habits, e.habits != nil
	}
	if !s.cfg.Vault.EnsureLoaded(n.RelPath) {
		// Read failed — record a negative-cache entry keyed by the
		// current mtime so we don't retry every request. Watcher
		// events bump the mtime and invalidate this naturally.
		habitsCacheMu.Lock()
		habitsCache[n.RelPath] = habitsParsedEntry{mtime: n.ModTime, habits: nil}
		habitsCacheMu.Unlock()
		return nil, false
	}
	parsed := parseHabitsSection(n.Content)
	var cached map[string]bool
	if len(parsed) > 0 {
		cached = parsed
	}
	habitsCacheMu.Lock()
	habitsCache[n.RelPath] = habitsParsedEntry{mtime: n.ModTime, habits: cached}
	habitsCacheMu.Unlock()
	return cached, cached != nil
}
