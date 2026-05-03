package habits

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStreakFor_Daily(t *testing.T) {
	// Five consecutive days completed → streak 5 on day 5.
	logs := []Log{
		{Date: "2026-04-26", Completed: []string{"gym"}},
		{Date: "2026-04-27", Completed: []string{"gym"}},
		{Date: "2026-04-28", Completed: []string{"gym"}},
		{Date: "2026-04-29", Completed: []string{"gym"}},
		{Date: "2026-04-30", Completed: []string{"gym"}},
	}
	if got := StreakFor(logs, "gym", "2026-04-30", "daily"); got != 5 {
		t.Errorf("daily streak: got %d, want 5", got)
	}
	// One missed day breaks the streak.
	logs2 := []Log{
		{Date: "2026-04-27", Completed: []string{"gym"}},
		{Date: "2026-04-28", Completed: []string{}}, // skipped
		{Date: "2026-04-29", Completed: []string{"gym"}},
		{Date: "2026-04-30", Completed: []string{"gym"}},
	}
	if got := StreakFor(logs2, "gym", "2026-04-30", "daily"); got != 2 {
		t.Errorf("broken streak: got %d, want 2", got)
	}
}

func TestStreakFor_Weekdays(t *testing.T) {
	// Habit set to weekdays-only. A Thursday 2026-04-30 streak
	// counting Mon-Fri 2026-04-27..-30 + 2026-04-24 should not
	// break across the weekend (Sat 25, Sun 26 don't require
	// check-ins). Friday 2026-04-24 is a weekday so it counts.
	// Naive "consecutive days done" walk would break here because
	// no log entry exists for the weekend — which is the bug we're
	// fixing.
	logs := []Log{
		{Date: "2026-04-24", Completed: []string{"work-deep"}}, // Fri
		{Date: "2026-04-27", Completed: []string{"work-deep"}}, // Mon
		{Date: "2026-04-28", Completed: []string{"work-deep"}}, // Tue
		{Date: "2026-04-29", Completed: []string{"work-deep"}}, // Wed
		{Date: "2026-04-30", Completed: []string{"work-deep"}}, // Thu
	}
	if got := StreakFor(logs, "work-deep", "2026-04-30", "weekdays"); got != 5 {
		t.Errorf("weekdays streak: got %d, want 5", got)
	}
	// Skip a Monday → streak is just Tue→Thu = 3.
	logs2 := []Log{
		{Date: "2026-04-24", Completed: []string{"work-deep"}}, // Fri
		// Mon skipped
		{Date: "2026-04-28", Completed: []string{"work-deep"}}, // Tue
		{Date: "2026-04-29", Completed: []string{"work-deep"}}, // Wed
		{Date: "2026-04-30", Completed: []string{"work-deep"}}, // Thu
	}
	if got := StreakFor(logs2, "work-deep", "2026-04-30", "weekdays"); got != 3 {
		t.Errorf("weekdays streak with broken Mon: got %d, want 3", got)
	}
}

func TestStreakFor_Weekends(t *testing.T) {
	// Habit set to weekends. Sun 2026-04-26 and Sat 2026-04-25
	// completed; previous Sat/Sun (2026-04-18 / 19) also done.
	// Weekday gaps don't break the streak.
	logs := []Log{
		{Date: "2026-04-18", Completed: []string{"long-run"}}, // Sat
		{Date: "2026-04-19", Completed: []string{"long-run"}}, // Sun
		{Date: "2026-04-25", Completed: []string{"long-run"}}, // Sat
		{Date: "2026-04-26", Completed: []string{"long-run"}}, // Sun
	}
	if got := StreakFor(logs, "long-run", "2026-04-26", "weekends"); got != 4 {
		t.Errorf("weekends streak: got %d, want 4", got)
	}
}

func TestStreakFor_3xWeek(t *testing.T) {
	// 3x-week streak walks 7-day windows back from `today`.
	// today = 2026-04-30 (Thu). Window 1 covers 2026-04-24..-30
	// (Fri..Thu) — Fri+Mon+Tue+Wed = 4 completions ≥ 3 ✓.
	// Window 2 covers 2026-04-17..-23 — Mon (20) + Wed (22) = 2 < 3.
	// Streak = 1.
	logs := []Log{
		{Date: "2026-04-20", Completed: []string{"meditate"}}, // Mon
		{Date: "2026-04-22", Completed: []string{"meditate"}}, // Wed
		{Date: "2026-04-24", Completed: []string{"meditate"}}, // Fri
		{Date: "2026-04-27", Completed: []string{"meditate"}}, // Mon
		{Date: "2026-04-28", Completed: []string{"meditate"}}, // Tue
		{Date: "2026-04-29", Completed: []string{"meditate"}}, // Wed
	}
	if got := StreakFor(logs, "meditate", "2026-04-30", "3x-week"); got != 1 {
		t.Errorf("3x-week streak: got %d, want 1", got)
	}
	// Push more completions into the prior window so the streak
	// extends to 2.
	logs2 := []Log{
		{Date: "2026-04-17", Completed: []string{"meditate"}}, // Fri (in window 2)
		{Date: "2026-04-20", Completed: []string{"meditate"}}, // Mon
		{Date: "2026-04-22", Completed: []string{"meditate"}}, // Wed
		{Date: "2026-04-24", Completed: []string{"meditate"}}, // Fri (window 1)
		{Date: "2026-04-27", Completed: []string{"meditate"}}, // Mon
		{Date: "2026-04-29", Completed: []string{"meditate"}}, // Wed
	}
	if got := StreakFor(logs2, "meditate", "2026-04-30", "3x-week"); got != 2 {
		t.Errorf("3x-week streak (extended): got %d, want 2", got)
	}
	// Drop one in the recent week → falls below target.
	logs3 := []Log{
		{Date: "2026-04-27", Completed: []string{"meditate"}}, // Mon
		{Date: "2026-04-28", Completed: []string{"meditate"}}, // Tue (only 2 this week)
	}
	if got := StreakFor(logs3, "meditate", "2026-04-30", "3x-week"); got != 0 {
		t.Errorf("3x-week underweight: got %d, want 0", got)
	}
}

func TestLongestStreak(t *testing.T) {
	logs := []Log{
		{Date: "2026-04-20", Completed: []string{"read"}},
		{Date: "2026-04-21", Completed: []string{"read"}},
		{Date: "2026-04-22", Completed: []string{"read"}},
		{Date: "2026-04-25", Completed: []string{"read"}},
		{Date: "2026-04-26", Completed: []string{"read"}},
	}
	if got := LongestStreak(logs, "read"); got != 3 {
		t.Errorf("longest streak: got %d, want 3", got)
	}
	if got := LongestStreak(logs, "nope"); got != 0 {
		t.Errorf("absent habit: got %d, want 0", got)
	}
}

func TestPerHabitRate(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	d2 := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	d3 := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	logs := []Log{
		{Date: today, Completed: []string{"x"}},
		{Date: d2, Completed: []string{}},
		{Date: d3, Completed: []string{"x"}},
	}
	got := PerHabitRate(logs, "x", 3)
	if got < 60 || got > 70 {
		t.Errorf("rate over 3 days: got %d, want ~66", got)
	}
}

func TestLoadAndSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	habits := []Entry{
		{Name: "gym", Created: "2026-01-01", Streak: 5},
		{Name: "read 20 pages", Created: "2026-02-15", Streak: 0},
	}
	logs := []Log{
		{Date: "2026-04-29", Completed: []string{"gym"}},
		{Date: "2026-04-30", Completed: []string{"gym", "read 20 pages"}},
	}
	if err := SaveHabitsMD(dir, habits, logs); err != nil {
		t.Fatal(err)
	}
	if err := SaveFrequencies(dir, map[string]string{"gym": "weekdays"}); err != nil {
		t.Fatal(err)
	}
	d := Load(dir)
	if len(d.Habits) != 2 {
		t.Errorf("habits: got %d, want 2", len(d.Habits))
	}
	if len(d.Logs) != 2 {
		t.Errorf("logs: got %d, want 2", len(d.Logs))
	}
	if d.Frequencies["gym"] != "weekdays" {
		t.Errorf("frequencies missing")
	}
}

func TestSidecarsAtomic(t *testing.T) {
	// No leftover .tmp files after sidecar saves.
	dir := t.TempDir()
	_ = SaveFrequencies(dir, map[string]string{"a": "daily"})
	_ = SaveCategories(dir, map[string]string{"a": "Health"})
	entries, _ := os.ReadDir(filepath.Join(dir, ".granit"))
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("leftover tmp file: %s", e.Name())
		}
	}
}

func TestHeatmap90Flat(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	logs := []Log{
		{Date: today, Completed: []string{"x"}},
	}
	got := Heatmap90Flat(logs, "x")
	if len(got) != 90 {
		t.Errorf("len: got %d, want 90", len(got))
	}
	if !got[89] {
		t.Errorf("today (last cell) should be true")
	}
	if got[0] {
		t.Errorf("oldest cell should be false")
	}
}

func TestNextDueAt(t *testing.T) {
	// Daily → today is always due.
	if got := NextDueAt("daily", "2026-04-30"); got != "2026-04-30" {
		t.Errorf("daily next due: got %q", got)
	}
	// Weekdays → Saturday 2026-04-25 next due is Mon 2026-04-27.
	if got := NextDueAt("weekdays", "2026-04-25"); got != "2026-04-27" {
		t.Errorf("weekdays next due: got %q, want 2026-04-27", got)
	}
	// Weekends → Wednesday 2026-04-29 next due is Sat 2026-05-02.
	if got := NextDueAt("weekends", "2026-04-29"); got != "2026-05-02" {
		t.Errorf("weekends next due: got %q, want 2026-05-02", got)
	}
}
