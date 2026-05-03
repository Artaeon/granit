// Package habits is the canonical schema + IO for the habit tracker.
//
// On-disk layout (the TUI's source of truth, lifted unchanged):
//
//	<vault>/Habits/habits.md          — markdown table of habits + log
//	<vault>/.granit/habits-frequency.json   — per-habit cadence
//	<vault>/.granit/habits-times.json       — per-habit reminder time-of-day
//	<vault>/.granit/habits-categories.json  — per-habit category
//	<vault>/.granit/habits-notes.json       — per-day per-habit free-text note
//	<vault>/.granit/habits-archived.json    — archived-name set
//
// The web API previously parsed `## Habits` checkbox sections inside
// daily notes — a separate data model that showed zero rows for users
// who lived in the TUI. Both surfaces now read this package, eliminating
// the divorce.
//
// Pure data + IO only. No Bubbletea, no lipgloss, no rendering.
package habits

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/atomicio"
)

// Frequency is the cadence target for a habit's streak math.
type Frequency string

const (
	FrequencyDaily    Frequency = "daily"
	FrequencyWeekdays Frequency = "weekdays"
	FrequencyWeekends Frequency = "weekends"
	Frequency3xWeek   Frequency = "3x-week"
)

// Entry is a single tracked habit.
type Entry struct {
	Name    string
	Created string // YYYY-MM-DD
	Streak  int
}

// Log is a date's completed-habit list.
type Log struct {
	Date      string // YYYY-MM-DD
	Completed []string
}

// Data is the loaded habit state — habits.md + every sidecar JSON merged
// into one struct the consumer can reason about.
type Data struct {
	Habits      []Entry
	Logs        []Log
	Frequencies map[string]string // habit → "daily"|"weekdays"|"weekends"|"3x-week"
	Times       map[string]string // habit → reminder period or HH:MM
	Categories  map[string]string // habit → category name
	Notes       map[string]string // "habit|YYYY-MM-DD" → free text
	Archived    map[string]bool   // habit → archived
}

// HabitsDir returns the path to <vault>/Habits/.
func HabitsDir(vaultRoot string) string {
	return filepath.Join(vaultRoot, "Habits")
}

// HabitsMDPath returns the canonical path to habits.md.
func HabitsMDPath(vaultRoot string) string {
	return filepath.Join(HabitsDir(vaultRoot), "habits.md")
}

func sidecarPath(vaultRoot, name string) string {
	return filepath.Join(vaultRoot, ".granit", name)
}

// Load reads habits.md plus every sidecar JSON. Missing or malformed
// files become empty data — never an error — so the TUI/web can always
// render a clean empty state. Callers receive a fully populated Data
// struct (maps are non-nil) so call sites don't need nil-checks.
func Load(vaultRoot string) Data {
	d := Data{
		Frequencies: map[string]string{},
		Times:       map[string]string{},
		Categories:  map[string]string{},
		Notes:       map[string]string{},
		Archived:    map[string]bool{},
	}
	d.Habits, d.Logs = loadHabitsMD(vaultRoot)
	loadJSONInto(sidecarPath(vaultRoot, "habits-frequency.json"), &d.Frequencies)
	loadJSONInto(sidecarPath(vaultRoot, "habits-times.json"), &d.Times)
	loadJSONInto(sidecarPath(vaultRoot, "habits-categories.json"), &d.Categories)
	loadJSONInto(sidecarPath(vaultRoot, "habits-notes.json"), &d.Notes)
	loadJSONInto(sidecarPath(vaultRoot, "habits-archived.json"), &d.Archived)
	return d
}

// loadHabitsMD parses the markdown table at <vault>/Habits/habits.md.
// The format is preserved exactly as the TUI writes it — moving the
// parser here means both surfaces see identical data without a hidden
// migration step.
func loadHabitsMD(vaultRoot string) ([]Entry, []Log) {
	data, err := os.ReadFile(HabitsMDPath(vaultRoot))
	if err != nil {
		return nil, nil
	}
	var habits []Entry
	var logs []Log
	section := "" // "habits" or "log"
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Habits" {
			section = "habits"
			continue
		}
		if trimmed == "## Log" {
			section = "log"
			continue
		}
		if !strings.HasPrefix(trimmed, "|") || strings.Contains(trimmed, "---") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		var cells []string
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				cells = append(cells, p)
			}
		}
		switch section {
		case "habits":
			if len(cells) >= 3 && cells[0] != "Habit" {
				streak, _ := strconv.Atoi(strings.TrimSpace(cells[2]))
				habits = append(habits, Entry{
					Name:    strings.TrimSpace(cells[0]),
					Created: strings.TrimSpace(cells[1]),
					Streak:  streak,
				})
			}
		case "log":
			if len(cells) >= 2 && cells[0] != "Date" {
				var completed []string
				for _, c := range strings.Split(cells[1], ",") {
					c = strings.TrimSpace(c)
					if c != "" {
						completed = append(completed, c)
					}
				}
				logs = append(logs, Log{
					Date:      strings.TrimSpace(cells[0]),
					Completed: completed,
				})
			}
		}
	}
	return habits, logs
}

func loadJSONInto(path string, into interface{}) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, into)
}

// SaveHabitsMD persists the habits + log back to habits.md. Used by the
// TUI on every check-in / habit-add. Layout matches loadHabitsMD exactly
// so the round-trip is stable.
func SaveHabitsMD(vaultRoot string, habits []Entry, logs []Log) error {
	if vaultRoot == "" {
		return errors.New("habits: empty vault root")
	}
	dir := HabitsDir(vaultRoot)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("---\ntype: habits\n---\n# Daily Habits\n\n")
	b.WriteString("## Habits\n")
	b.WriteString("| Habit | Created | Streak |\n")
	b.WriteString("|-------|---------|--------|\n")
	for _, h := range habits {
		b.WriteString("| " + h.Name + " | " + h.Created + " | " + strconv.Itoa(h.Streak) + " |\n")
	}
	b.WriteString("\n## Log\n")
	b.WriteString("| Date | Completed |\n")
	b.WriteString("|------|-----------|\n")
	for _, log := range logs {
		b.WriteString("| " + log.Date + " | " + strings.Join(log.Completed, ", ") + " |\n")
	}
	return atomicio.WriteNote(HabitsMDPath(vaultRoot), b.String())
}

// SaveFrequencies persists the per-habit cadence map.
func SaveFrequencies(vaultRoot string, m map[string]string) error {
	return saveSidecar(vaultRoot, "habits-frequency.json", m)
}

// SaveTimes persists the per-habit reminder map.
func SaveTimes(vaultRoot string, m map[string]string) error {
	return saveSidecar(vaultRoot, "habits-times.json", m)
}

// SaveCategories persists the per-habit category map.
func SaveCategories(vaultRoot string, m map[string]string) error {
	return saveSidecar(vaultRoot, "habits-categories.json", m)
}

// SaveNotes persists the per-day per-habit notes map.
func SaveNotes(vaultRoot string, m map[string]string) error {
	return saveSidecar(vaultRoot, "habits-notes.json", m)
}

// SaveArchived persists the archived-name set.
func SaveArchived(vaultRoot string, m map[string]bool) error {
	return saveSidecar(vaultRoot, "habits-archived.json", m)
}

// saveSidecar is a tiny helper that mkdir's .granit and atomic-writes
// indented JSON, identical to the pattern the TUI was using inline.
func saveSidecar(vaultRoot, name string, v interface{}) error {
	if vaultRoot == "" {
		return errors.New("habits: empty vault root")
	}
	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicio.WriteNote(filepath.Join(dir, name), string(data))
}

// SidecarPaths returns the relative paths of every habit sidecar plus
// the habits.md file. Used by the WS broadcaster so callers can fan a
// `state.changed` event out for every habit-related write without
// hardcoding the list at each call site.
func SidecarPaths() []string {
	return []string{
		"Habits/habits.md",
		".granit/habits-frequency.json",
		".granit/habits-times.json",
		".granit/habits-categories.json",
		".granit/habits-notes.json",
		".granit/habits-archived.json",
	}
}

// FreqIsRequired reports whether a habit's frequency demands a check-in
// on `date`. Lifts the TUI's logic verbatim so streak math agrees with
// the TUI on the day "weekdays" habits don't break their streak on
// Saturday. "3x-week" returns false here — the streak is computed by
// StreakFor walking the weekly path instead.
func FreqIsRequired(freq, date string) bool {
	if freq == "" || freq == string(FrequencyDaily) {
		return true
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return true
	}
	wd := t.Weekday()
	switch freq {
	case string(FrequencyWeekdays):
		return wd >= time.Monday && wd <= time.Friday
	case string(FrequencyWeekends):
		return wd == time.Saturday || wd == time.Sunday
	case string(Frequency3xWeek):
		// Daily check-in not required; the streak counts in
		// week-buckets. See StreakFor.
		return false
	}
	return true
}

// StreakFor returns the current streak for `habit` on `today`,
// honouring the cadence in `freq`. The web previously used a naive
// "consecutive days done" walk that gave wrong streaks for weekend /
// weekday / 3x-week habits — this is the single function that fixes it.
//
//   - daily / unset: consecutive days completed.
//   - weekdays / weekends: skip non-required days without breaking the streak.
//   - 3x-week: count consecutive ISO weeks where ≥3 completions land.
//
// today is a YYYY-MM-DD string so callers don't need to thread time.Now
// through a chain — a fixed clock is just a fixed string.
func StreakFor(logs []Log, habit, today, freq string) int {
	if freq == string(Frequency3xWeek) {
		return streakWeekly(logs, habit, today, 3)
	}
	d, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0
	}
	streak := 0
	// Walk back at most ~2 years so a corrupt stream can't loop.
	for guard := 0; guard < 730; guard++ {
		ds := d.Format("2006-01-02")
		if !FreqIsRequired(freq, ds) {
			d = d.AddDate(0, 0, -1)
			continue
		}
		if !isCompletedOn(logs, habit, ds) {
			break
		}
		streak++
		d = d.AddDate(0, 0, -1)
	}
	return streak
}

func streakWeekly(logs []Log, habit, today string, target int) int {
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0
	}
	streak := 0
	cursor := t
	for guard := 0; guard < 104; guard++ {
		count := 0
		for offset := 0; offset < 7; offset++ {
			d := cursor.AddDate(0, 0, -offset).Format("2006-01-02")
			if isCompletedOn(logs, habit, d) {
				count++
			}
		}
		if count < target {
			break
		}
		streak++
		cursor = cursor.AddDate(0, 0, -7)
	}
	return streak
}

// isCompletedOn checks whether a habit was logged on a given date.
func isCompletedOn(logs []Log, habit, date string) bool {
	for _, log := range logs {
		if log.Date == date {
			for _, c := range log.Completed {
				if c == habit {
					return true
				}
			}
			return false
		}
	}
	return false
}

// LongestStreak returns the longest run of consecutive calendar days
// the habit was completed across the entire log. Cadence-naive — the
// "longest" metric is a milestone count, not a target-adjusted figure.
func LongestStreak(logs []Log, habit string) int {
	var dates []string
	for _, log := range logs {
		for _, c := range log.Completed {
			if c == habit {
				dates = append(dates, log.Date)
				break
			}
		}
	}
	if len(dates) == 0 {
		return 0
	}
	sort.Strings(dates)
	longest := 1
	current := 1
	for i := 1; i < len(dates); i++ {
		prev, _ := time.Parse("2006-01-02", dates[i-1])
		curr, _ := time.Parse("2006-01-02", dates[i])
		if curr.Equal(prev.AddDate(0, 0, 1)) {
			current++
			if current > longest {
				longest = current
			}
		} else if !curr.Equal(prev) {
			current = 1
		}
	}
	return longest
}

// CompletionRate returns the rate (0-100) of "habit-days completed" /
// "habit-days possible" across the last `days` days. Both surfaces use
// it for the last-7 / last-30 percentages.
func CompletionRate(habits []Entry, logs []Log, days int) float64 {
	if len(habits) == 0 || days <= 0 {
		return 0
	}
	today, _ := time.Parse("2006-01-02", time.Now().Format("2006-01-02"))
	total := 0
	done := 0
	for i := 0; i < days; i++ {
		d := today.AddDate(0, 0, -i)
		ds := d.Format("2006-01-02")
		total += len(habits)
		for _, log := range logs {
			if log.Date == ds {
				done += len(log.Completed)
				break
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(done) * 100.0 / float64(total)
}

// PerHabitRate returns the per-habit rate (0-100) over the last `days`
// days. Web's last7Pct / last30Pct delegate here so each habit shows
// its own consistency, not the dashboard average.
func PerHabitRate(logs []Log, habit string, days int) int {
	if days <= 0 {
		return 0
	}
	today, err := time.Parse("2006-01-02", time.Now().Format("2006-01-02"))
	if err != nil {
		return 0
	}
	done := 0
	for i := 0; i < days; i++ {
		d := today.AddDate(0, 0, -i).Format("2006-01-02")
		if isCompletedOn(logs, habit, d) {
			done++
		}
	}
	return int(float64(done) / float64(days) * 100)
}

// WeeklyCompletions returns per-week completion counts for the last 12
// weeks (oldest → newest). Used by the TUI's per-row sparkline.
func WeeklyCompletions(logs []Log, habit string) []int {
	now := time.Now()
	weeks := make([]int, 12)
	for w := 0; w < 12; w++ {
		weekEnd := now.AddDate(0, 0, -7*(11-w))
		count := 0
		for offset := 0; offset < 7; offset++ {
			d := weekEnd.AddDate(0, 0, -offset).Format("2006-01-02")
			if isCompletedOn(logs, habit, d) {
				count++
			}
		}
		weeks[w] = count
	}
	return weeks
}

// Heatmap90 returns a 7-row × 13-col grid of date strings (YYYY-MM-DD,
// or empty string for cells off the 90-day window). Bottom-right is
// today; rows are Sun..Sat (top..bottom). Both the TUI heatmap and the
// web heatmap can render directly off this grid + a per-date count
// derived from the logs.
func Heatmap90(logs []Log) (grid [7][13]string, counts map[string]int) {
	counts = map[string]int{}
	now := time.Now()
	cutoff := now.AddDate(0, 0, -90).Format("2006-01-02")
	for _, log := range logs {
		if log.Date >= cutoff {
			counts[log.Date] = len(log.Completed)
		}
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	d := today
	for col := 12; col >= 0; col-- {
		for row := 6; row >= 0; row-- {
			if d.Before(today.AddDate(0, 0, -90)) {
				continue
			}
			grid[row][col] = d.Format("2006-01-02")
			d = d.AddDate(0, 0, -1)
		}
	}
	return grid, counts
}

// Heatmap90Flat returns a flat slice of bools, one per day in the 90-day
// window oldest → newest. The web /habits page sends this to the
// browser as a compact done/not-done strip per habit.
func Heatmap90Flat(logs []Log, habit string) []bool {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	out := make([]bool, 90)
	for i := 0; i < 90; i++ {
		d := today.AddDate(0, 0, -(89 - i)).Format("2006-01-02")
		out[i] = isCompletedOn(logs, habit, d)
	}
	return out
}

// NextDueAt returns the next date a habit is required on, walking
// forward from `from` honouring its cadence. Returns "" if the habit
// has no useful "next due" notion (e.g. weekly target — every day
// counts). The web uses this for reminders; the TUI surfaces it via
// the time-of-day chip.
func NextDueAt(freq, from string) string {
	if freq == "" || freq == string(FrequencyDaily) {
		return from
	}
	if freq == string(Frequency3xWeek) {
		// Every day counts toward the weekly target — "next due"
		// is just today.
		return from
	}
	d, err := time.Parse("2006-01-02", from)
	if err != nil {
		return ""
	}
	for guard := 0; guard < 14; guard++ {
		if FreqIsRequired(freq, d.Format("2006-01-02")) {
			return d.Format("2006-01-02")
		}
		d = d.AddDate(0, 0, 1)
	}
	return ""
}

// NoteKey is the stable composite key for the per-check-in note map.
func NoteKey(habit, date string) string { return habit + "|" + date }

// Today returns today's date as YYYY-MM-DD in the local zone.
func Today() string { return time.Now().Format("2006-01-02") }
