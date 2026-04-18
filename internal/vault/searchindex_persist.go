package vault

// Persistence for SearchIndex. The index is rebuilt at app startup
// today; on a vault with thousands of notes that's a measurable wait
// before content search becomes useful. Saving the built index to disk
// lets the next launch pick up an instantly-usable index — a background
// rebuild then catches anything that changed externally.
//
// Storage format is gob: it round-trips Go's nested maps without the
// per-key string-conversion overhead JSON would impose, and the file is
// machine-internal so we don't need a human-readable format.

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
)

// indexSnapshot is the on-disk representation. Field names are exported
// because gob requires that.
type indexSnapshot struct {
	InvertedIndex map[string]map[string]bool
	Positions     map[string]map[string][]int
	DocWordCount  map[string]int
	DocLines      map[string][]string
	TotalDocs     int
}

// Save writes the current index state to path atomically. Returns an
// error so callers can decide whether to surface it; persistence
// failure should never block search.
func (si *SearchIndex) Save(path string) error {
	si.mu.RLock()
	snap := indexSnapshot{
		InvertedIndex: si.invertedIndex,
		Positions:     si.positions,
		DocWordCount:  si.docWordCount,
		DocLines:      si.docLines,
		TotalDocs:     si.totalDocs,
	}
	si.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("search index: mkdir: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("search index: create: %w", err)
	}
	if err := gob.NewEncoder(f).Encode(&snap); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("search index: encode: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("search index: close: %w", err)
	}
	return os.Rename(tmp, path)
}

// Load replaces the index state with the snapshot stored at path. Marks
// the index ready so search can proceed immediately. Returns nil and
// leaves the existing state untouched when the file doesn't exist.
//
// The caller is responsible for kicking off a background Build() to
// reconcile changes that happened while granit was off — Load only
// promises "instantly usable," not "fully fresh."
func (si *SearchIndex) Load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("search index: open: %w", err)
	}
	defer f.Close()

	var snap indexSnapshot
	if err := gob.NewDecoder(f).Decode(&snap); err != nil {
		return fmt.Errorf("search index: decode: %w", err)
	}
	if snap.InvertedIndex == nil {
		// Empty / corrupt file — treat as missing rather than silently
		// installing a useless index.
		return nil
	}

	si.mu.Lock()
	si.invertedIndex = snap.InvertedIndex
	si.positions = snap.Positions
	si.docWordCount = snap.DocWordCount
	si.docLines = snap.DocLines
	si.totalDocs = snap.TotalDocs
	si.ready = true
	si.mu.Unlock()
	return nil
}
