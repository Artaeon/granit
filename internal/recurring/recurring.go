// Package recurring is the shared recurring-task store + scheduler used
// by both the granit TUI and the web server. Source of truth: a single
// JSON file at <vault>/.granit/recurring.json.
//
// The TUI's recurringtasks.go owns the rich overlay UI; this package
// holds the data model + load/save + "is this task due today?" rule
// so the web server can answer the same questions and write the same
// file on a cron-like loop without depending on the TUI.
package recurring

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Task is one recurring rule. Same JSON shape the TUI writes — both
// surfaces read+write the same file.
type Task struct {
	Text        string `json:"text"`
	Frequency   string `json:"frequency"`    // "daily" | "weekly" | "monthly"
	DayOfWeek   int    `json:"day_of_week"`  // 0-6 (Sun-Sat), used for weekly
	DayOfMonth  int    `json:"day_of_month"` // 1-31, used for monthly
	LastCreated string `json:"last_created"` // YYYY-MM-DD; updated when an instance is created
	Enabled     bool   `json:"enabled"`
}

// Path returns the canonical recurring-task config file location for
// a given vault. Single point so both surfaces always agree.
func Path(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit", "recurring.json")
}

// Load reads the rules. Missing file is not an error — returns nil so
// the caller can render an empty list. A corrupt file returns the
// parse error so the TUI/web can surface it instead of silently
// resetting (matches the TUI's existing behaviour after we logged the
// "resetting" message — explicit error is friendlier).
func Load(vaultRoot string) ([]Task, error) {
	data, err := os.ReadFile(Path(vaultRoot))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Task
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Save writes the rules atomically. Mkdirs the .granit/ folder if
// missing so a fresh vault doesn't fail the first save.
func Save(vaultRoot string, tasks []Task) error {
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteState(Path(vaultRoot), data)
}

// IsDue reports whether the given task should be created on the given
// day. Mirrors the TUI's isDue(): checks LastCreated to avoid double-
// generation on the same day, then matches frequency rules.
func IsDue(task Task, today time.Time) bool {
	todayStr := today.Format("2006-01-02")
	if task.LastCreated == todayStr {
		return false
	}
	switch task.Frequency {
	case "daily":
		return true
	case "weekly":
		return int(today.Weekday()) == task.DayOfWeek
	case "monthly":
		return today.Day() == task.DayOfMonth
	}
	return false
}

// MarkCreated stamps the task's LastCreated to today and persists. The
// caller is expected to have already added the task line to the vault
// — MarkCreated only updates the rule's bookkeeping.
func MarkCreated(vaultRoot string, idx int, today time.Time) error {
	all, err := Load(vaultRoot)
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(all) {
		return errors.New("recurring: index out of range")
	}
	all[idx].LastCreated = today.Format("2006-01-02")
	return Save(vaultRoot, all)
}
