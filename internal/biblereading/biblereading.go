// Package biblereading tracks the days the user has opened scripture
// in Granit. Pure storage + a tiny RecordRead helper; the streak
// arithmetic itself reuses daily.ComputeStreak so the "consecutive-
// day habit" semantics stay identical to the daily-note streak the
// editor surfaces (forgiving today-in-progress rule, future-date
// drop, leap-year-safe boundary math).
//
// Store shape:
//   <vault>/.granit/bible-reading-log.json
//   { "version": 1, "dates": ["2026-05-09", "2026-05-10", ...] }
//
// Why dates-only (vs full read events): the streak is a habit
// indicator, not an analytics ledger. Storing one entry per
// passage-opened would multiply the file size by ~10× for the same
// streak number, and the user can't query "what did I read on day
// X" through this surface anyway (bookmarks cover that). One date
// per day, deduped on add.
package biblereading

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Log is the on-disk envelope. Dates are RFC3339 calendar-only
// strings (YYYY-MM-DD), sorted ascending. Version lets a future
// schema change migrate cleanly.
type Log struct {
	Version int      `json:"version"`
	Dates   []string `json:"dates"`
}

const currentVersion = 1

// logMu serialises read-modify-write — RecordRead is the only
// mutating path and gets called once per session, but a user
// flipping rapidly between bible tabs in two browser windows would
// otherwise race the same way annotations/sidecars do.
var logMu sync.Mutex

func statePath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "bible-reading-log.json")
}

// Load reads the log. Returns an empty Log on a fresh vault (a
// never-read vault is a valid state); errors only on parse failure
// so the caller can decide whether to surface or fall back.
func Load(vaultRoot string) (Log, error) {
	raw, err := os.ReadFile(statePath(vaultRoot))
	if errors.Is(err, fs.ErrNotExist) {
		return Log{Version: currentVersion}, nil
	}
	if err != nil {
		return Log{}, err
	}
	var l Log
	if err := json.Unmarshal(raw, &l); err != nil {
		return Log{}, err
	}
	if l.Version == 0 {
		l.Version = currentVersion
	}
	return l, nil
}

// Save writes the log atomically. Sorts + dedupes so the JSON is
// stable across saves (a diff-friendly file matters with the
// autocommit pattern other granit sidecars follow).
func Save(vaultRoot string, l Log) error {
	if l.Version == 0 {
		l.Version = currentVersion
	}
	l.Dates = sortDedupe(l.Dates)
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(statePath(vaultRoot), raw)
}

// RecordRead marks the given calendar date as read. Idempotent —
// calling repeatedly on the same date doesn't grow the log. Returns
// whether the call actually added a new date (so callers can avoid
// emitting a "saved" toast on a no-op).
func RecordRead(vaultRoot, date string) (added bool, err error) {
	if !validDate(date) {
		return false, errors.New("biblereading: date must be YYYY-MM-DD")
	}
	logMu.Lock()
	defer logMu.Unlock()
	l, err := Load(vaultRoot)
	if err != nil {
		return false, err
	}
	for _, d := range l.Dates {
		if d == date {
			return false, nil
		}
	}
	l.Dates = append(l.Dates, date)
	return true, Save(vaultRoot, l)
}

// Snapshot returns the date list (sorted, deduped) so callers can
// feed it to daily.ComputeStreak without touching the storage layer.
func Snapshot(vaultRoot string) ([]string, error) {
	l, err := Load(vaultRoot)
	if err != nil {
		return nil, err
	}
	if len(l.Dates) == 0 {
		return nil, nil
	}
	out := make([]string, len(l.Dates))
	copy(out, l.Dates)
	return sortDedupe(out), nil
}

func sortDedupe(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, d := range in {
		if d == "" {
			continue
		}
		if _, dup := seen[d]; dup {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}

func validDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
