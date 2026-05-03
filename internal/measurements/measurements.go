// Package measurements is the canonical schema + IO for granit's
// numeric tracking. Companion to internal/habits — habits are yes/no
// checkboxes ("did I run today?"), measurements are numeric values
// ("how heavy was I today?", "how many hours did I sleep?",
// "how many push-ups did I do?"). Two different shapes, two
// different files.
//
// Series + Entries: a Series is the metric definition (Name, Unit,
// optional Target). Entries are the time-stamped values logged
// against a series. One file each — entries grow unbounded over
// years, but the series file stays tiny.
//
// State at <vault>/.granit/measurements/{series,entries}.json so
// the TUI gets the same source of truth.
//
// Pure data + IO only. The "current value" / "trend" / "vs target"
// derivations live in serveapi handlers.
package measurements

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Series is a metric definition: what's being tracked, in what unit,
// optionally with a target.
//
// Direction: "up" or "down" — does the user want this value to go
// up over time (push-ups, miles run) or down (weight, body fat)?
// Drives how the UI colours "you went up 2 lbs" — green or red.
// Defaulted at the handler so missing/old data renders sensibly.
type Series struct {
	ID        string    `json:"id"`         // ULID, lowercase
	Name      string    `json:"name"`       // "Weight", "Sleep", "Push-ups"
	Unit      string    `json:"unit"`       // "kg", "lbs", "hours", "count"
	Target    *float64  `json:"target,omitempty"`     // optional target value
	Direction string    `json:"direction,omitempty"`  // "up" or "down" — see comment above
	Notes     string    `json:"notes,omitempty"`
	Archived  bool      `json:"archived,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NormalizeDirection collapses arbitrary input to one of the two
// canonical values. Empty defaults to "up" since that's the more
// common case (push-ups, miles, savings).
func NormalizeDirection(s string) string {
	switch s {
	case "up", "down":
		return s
	default:
		return "up"
	}
}

// Entry is one logged value against a series at a point in time.
// Date is YYYY-MM-DD; multiple entries on the same date are allowed
// (e.g. weighed in morning + evening).
type Entry struct {
	ID        string    `json:"id"`
	SeriesID  string    `json:"series_id"`
	Date      string    `json:"date"`     // YYYY-MM-DD
	Value     float64   `json:"value"`
	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func dir(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "measurements")
}
func SeriesPath(v string) string  { return filepath.Join(dir(v), "series.json") }
func EntriesPath(v string) string { return filepath.Join(dir(v), "entries.json") }

func loadAll[T any](path string) []T {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []T
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

func saveAll[T any](vaultRoot, path string, items []T) error {
	if vaultRoot == "" {
		return errors.New("measurements: empty vault root")
	}
	if err := os.MkdirAll(dir(vaultRoot), 0o755); err != nil {
		return err
	}
	if items == nil {
		items = []T{}
	}
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(path, data)
}

func LoadSeries(v string) []Series          { return loadAll[Series](SeriesPath(v)) }
func SaveSeries(v string, x []Series) error { return saveAll(v, SeriesPath(v), x) }
func LoadEntries(v string) []Entry          { return loadAll[Entry](EntriesPath(v)) }
func SaveEntries(v string, x []Entry) error { return saveAll(v, EntriesPath(v), x) }

// SortSeries: archived to the bottom; otherwise alpha by Name with
// ID as tiebreak so the order is stable across reloads.
func SortSeries(xs []Series) []Series {
	out := make([]Series, len(xs))
	copy(out, xs)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Archived != out[j].Archived {
			return !out[i].Archived
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// EntriesForSeries returns only entries belonging to seriesID, sorted
// newest-date-first with newest CreatedAt as tiebreak (so the
// last-logged-today entry surfaces first when the user weighed
// themselves twice).
func EntriesForSeries(entries []Entry, seriesID string) []Entry {
	out := []Entry{}
	for _, e := range entries {
		if e.SeriesID == seriesID {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Date != out[j].Date {
			return out[i].Date > out[j].Date
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

// LatestForSeries is a convenience for "what's the current value of
// X" (used by the dashboard widget + the series detail card). Returns
// the entry pointer + true if any entry exists, else (Entry{}, false).
func LatestForSeries(entries []Entry, seriesID string) (Entry, bool) {
	xs := EntriesForSeries(entries, seriesID)
	if len(xs) == 0 {
		return Entry{}, false
	}
	return xs[0], true
}
