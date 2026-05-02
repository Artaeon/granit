// Package timetracker is the shared time-tracking store used by the
// granit TUI's clock-in overlay and the web's task-row play button.
//
// On-disk format: <vault>/.granit/timetracker.json — flat array of
// completed sessions. The "active timer" is held in memory only; the
// caller decides where (per-process for the TUI, server-side for the
// web). When the timer stops, the package appends a session and
// persists.
package timetracker

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Entry is one completed work session. Same JSON shape the TUI's
// timetracker.go writes — both surfaces read+write the same file.
type Entry struct {
	NotePath  string        `json:"note_path"`
	TaskText  string        `json:"task_text"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Pomodoros int           `json:"pomodoros,omitempty"`
	Date      string        `json:"date"` // YYYY-MM-DD
	// TaskID points at granit's stable task ID (sidecar) when the
	// session was clock-in'd from a tracked task. Lets the web roll
	// up "minutes spent on task X" without doing fuzzy matching on
	// task text. Optional — TUI sessions often only have free-form
	// task text.
	TaskID string `json:"task_id,omitempty"`
}

// Active is the in-memory state of a running timer. Owned by the
// caller (TUI keeps it on the overlay struct; the web keeps it on
// the Server). Survives only as long as the process — restarting
// granit web while a timer is running drops the timer (the user can
// reissue clock-in).
type Active struct {
	NotePath  string
	TaskText  string
	TaskID    string
	StartTime time.Time
}

// Path returns the canonical timetracker.json path for a vault.
func Path(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "timetracker.json")
}

// Load reads all completed entries. Missing file → nil slice with no
// error (the common fresh-vault case).
func Load(vaultRoot string) ([]Entry, error) {
	data, err := os.ReadFile(Path(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Append writes a new entry to the store. Loads, appends, atomically
// writes back — race-safe across concurrent goroutines via the
// package-level mutex (single vault server, one file).
func Append(vaultRoot string, e Entry) error {
	mu.Lock()
	defer mu.Unlock()
	all, err := Load(vaultRoot)
	if err != nil {
		return err
	}
	all = append(all, e)
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(Path(vaultRoot), data)
}

var mu sync.Mutex

// MinutesByTaskID rolls up total tracked minutes per task ID. Used by
// the web's TaskCard to show "1h 23m" next to a task once it's been
// clocked.
func MinutesByTaskID(entries []Entry) map[string]int {
	out := make(map[string]int, len(entries))
	for _, e := range entries {
		if e.TaskID == "" {
			continue
		}
		out[e.TaskID] += int(e.Duration.Minutes())
	}
	return out
}

// MinutesToday sums minutes for sessions ending today (local time).
// Used by the dashboard "today's focus" widget.
func MinutesToday(entries []Entry) int {
	today := time.Now().Format("2006-01-02")
	total := 0
	for _, e := range entries {
		if e.Date != today {
			continue
		}
		total += int(e.Duration.Minutes())
	}
	return total
}
