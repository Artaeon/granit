package serveapi

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Caches for the ICS pipeline. Two layers:
//
//   parseICSFileCached  — per-file parse result, keyed by (path, mtime, size).
//                         Eliminates the read + tokenise + RRULE-prep cost
//                         on every /calendar request when the file hasn't
//                         changed on disk.
//
//   icsListSourcesCached — per-vault file enumeration, keyed by the mtimes
//                          of the three source roots. Cheaper than the
//                          parse cache but called from three handlers,
//                          so it's still worth eliminating the 3× ReadDir.
//
// Both caches use mtime guards rather than watcher invalidation so they
// stay correct even if the file changes outside the watcher's scope
// (e.g. a sync agent dropping a new .ics in `calendars/`). The watcher
// fires watcher events too, but we don't need it — stat is enough.

// ---------- parseICSFile cache ----------

type parseICSCacheEntry struct {
	mtime  time.Time
	size   int64
	events []icsEvent
	err    error
}

var (
	parseICSCacheMu sync.Mutex
	parseICSCache   = map[string]parseICSCacheEntry{}
)

// parseICSFileCached returns parseICSFile's output, memoised by file
// (path, mtime, size). The cached value is shared by reference — callers
// must NOT mutate the returned slice or its event entries. Existing
// callers (icsScan, handlers_ics_events) treat it read-only via copy or
// per-event tagging, so this contract is safe.
func parseICSFileCached(path string) ([]icsEvent, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	mtime := info.ModTime()
	size := info.Size()

	parseICSCacheMu.Lock()
	entry, ok := parseICSCache[path]
	parseICSCacheMu.Unlock()
	if ok && entry.mtime.Equal(mtime) && entry.size == size {
		return entry.events, entry.err
	}

	events, perr := parseICSFile(path)
	parseICSCacheMu.Lock()
	parseICSCache[path] = parseICSCacheEntry{mtime: mtime, size: size, events: events, err: perr}
	parseICSCacheMu.Unlock()
	return events, perr
}

// ---------- icsListSources cache ----------

type icsSourcesCacheEntry struct {
	// mtimes for vaultRoot, calendars/, Calendars/ — the three dirs
	// icsListSources walks. Zero time records "directory did not
	// exist" so a later create flips the cache to miss.
	rootMtime    time.Time
	writableMtime time.Time
	calendarsMtime time.Time
	sources       []icsSource
}

var (
	icsSourcesCacheMu sync.Mutex
	icsSourcesCache   = map[string]icsSourcesCacheEntry{}
)

// icsListSourcesCached memoises icsListSources by vault root and the
// mtimes of the three source dirs it walks. A file added/removed in any
// of those dirs bumps the parent dir's mtime, so the next call misses
// the cache and re-walks. File CONTENT changes don't bump the dir's
// mtime — but icsListSources only reads dir entries, not content, so
// that's the right granularity.
func icsListSourcesCached(vaultRoot string) []icsSource {
	root := vaultRoot
	writable := filepath.Join(vaultRoot, "calendars")
	calendars := filepath.Join(vaultRoot, "Calendars")
	rMt := statMtimeFile(root)
	wMt := statMtimeFile(writable)
	cMt := statMtimeFile(calendars)

	icsSourcesCacheMu.Lock()
	entry, ok := icsSourcesCache[vaultRoot]
	icsSourcesCacheMu.Unlock()
	if ok && entry.rootMtime.Equal(rMt) && entry.writableMtime.Equal(wMt) && entry.calendarsMtime.Equal(cMt) {
		return entry.sources
	}

	sources := icsListSources(vaultRoot)
	icsSourcesCacheMu.Lock()
	icsSourcesCache[vaultRoot] = icsSourcesCacheEntry{
		rootMtime:      rMt,
		writableMtime:  wMt,
		calendarsMtime: cMt,
		sources:        sources,
	}
	icsSourcesCacheMu.Unlock()
	return sources
}

// statMtimeFile is the file-stat sibling of config.statMtime; duplicated
// here so the serveapi package doesn't have to import its own config
// dependency for one helper. Returns zero time when the path doesn't
// exist or stat fails — those cases get cached too and invalidate as
// soon as the path appears.
func statMtimeFile(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
