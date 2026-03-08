package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Initialization
// ---------------------------------------------------------------------------

func TestNewTimeTracker(t *testing.T) {
	tt := NewTimeTracker()
	if tt.active {
		t.Error("expected new tracker to be inactive")
	}
	if tt.timerRunning {
		t.Error("expected no running timer on init")
	}
	if tt.activeTimer != nil {
		t.Error("expected nil activeTimer on init")
	}
	if tt.phase != 0 {
		t.Errorf("expected phase 0, got %d", tt.phase)
	}
	if len(tt.entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(tt.entries))
	}
	if tt.viewDate.IsZero() {
		t.Error("expected viewDate to be set to now")
	}
}

func TestTimeTrackerSetSize(t *testing.T) {
	tt := NewTimeTracker()
	tt.SetSize(120, 40)
	if tt.width != 120 {
		t.Errorf("expected width 120, got %d", tt.width)
	}
	if tt.height != 40 {
		t.Errorf("expected height 40, got %d", tt.height)
	}
}

func TestOpenAndClose(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()

	tt.Open(tmp)
	if !tt.active {
		t.Error("expected tracker to be active after Open")
	}
	if tt.vaultRoot != tmp {
		t.Errorf("expected vaultRoot %q, got %q", tmp, tt.vaultRoot)
	}
	if tt.phase != 0 {
		t.Errorf("expected phase 0 after Open, got %d", tt.phase)
	}

	tt.Close()
	if tt.active {
		t.Error("expected tracker to be inactive after Close")
	}
}

func TestIsActive(t *testing.T) {
	tt := NewTimeTracker()
	if tt.IsActive() {
		t.Error("expected IsActive() false on new tracker")
	}
	tt.active = true
	if !tt.IsActive() {
		t.Error("expected IsActive() true after setting active")
	}
}

// ---------------------------------------------------------------------------
// Timer Start/Stop
// ---------------------------------------------------------------------------

func TestStartTimer(t *testing.T) {
	tt := NewTimeTracker()
	tt.vaultRoot = t.TempDir()

	tt.StartTimer("notes/project.md", "implement feature")

	if !tt.timerRunning {
		t.Error("expected timer to be running after StartTimer")
	}
	if !tt.IsTimerRunning() {
		t.Error("expected IsTimerRunning() true")
	}
	if tt.activeTimer == nil {
		t.Fatal("expected activeTimer to be set")
	}
	if tt.activeTimer.NotePath != "notes/project.md" {
		t.Errorf("expected NotePath %q, got %q", "notes/project.md", tt.activeTimer.NotePath)
	}
	if tt.activeTimer.TaskText != "implement feature" {
		t.Errorf("expected TaskText %q, got %q", "implement feature", tt.activeTimer.TaskText)
	}
	if tt.currentNote != "notes/project.md" {
		t.Errorf("expected currentNote %q, got %q", "notes/project.md", tt.currentNote)
	}
	if tt.currentTask != "implement feature" {
		t.Errorf("expected currentTask %q, got %q", "implement feature", tt.currentTask)
	}
	if tt.activeTimer.Date != time.Now().Format("2006-01-02") {
		t.Errorf("expected date to be today, got %q", tt.activeTimer.Date)
	}
	if tt.timerElapsed != 0 {
		t.Errorf("expected timerElapsed 0, got %v", tt.timerElapsed)
	}
}

func TestStopTimer(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	tt.StartTimer("notes/test.md", "write tests")
	// Simulate a short delay by manipulating start time.
	tt.activeTimer.StartTime = time.Now().Add(-5 * time.Minute)
	tt.timerStart = tt.activeTimer.StartTime

	tt.StopTimer()

	if tt.timerRunning {
		t.Error("expected timer to be stopped")
	}
	if tt.IsTimerRunning() {
		t.Error("expected IsTimerRunning() false")
	}
	if tt.activeTimer != nil {
		t.Error("expected activeTimer to be nil after stop")
	}
	if tt.currentNote != "" {
		t.Errorf("expected currentNote to be empty, got %q", tt.currentNote)
	}
	if tt.currentTask != "" {
		t.Errorf("expected currentTask to be empty, got %q", tt.currentTask)
	}
	if len(tt.entries) != 1 {
		t.Fatalf("expected 1 entry after stop, got %d", len(tt.entries))
	}

	entry := tt.entries[0]
	if entry.NotePath != "notes/test.md" {
		t.Errorf("expected NotePath %q, got %q", "notes/test.md", entry.NotePath)
	}
	if entry.Duration < 4*time.Minute || entry.Duration > 6*time.Minute {
		t.Errorf("expected duration ~5m, got %v", entry.Duration)
	}
	if entry.EndTime.Before(entry.StartTime) {
		t.Error("expected EndTime to be after StartTime")
	}
}

func TestStopTimerWhenNotRunning(t *testing.T) {
	tt := NewTimeTracker()
	tt.vaultRoot = t.TempDir()

	// Should be a no-op, not panic.
	tt.StopTimer()
	if len(tt.entries) != 0 {
		t.Errorf("expected no entries, got %d", len(tt.entries))
	}
}

func TestStopTimerWithNilActiveTimer(t *testing.T) {
	tt := NewTimeTracker()
	tt.vaultRoot = t.TempDir()
	tt.timerRunning = true
	tt.activeTimer = nil

	// Should be a no-op since activeTimer is nil.
	tt.StopTimer()
	if len(tt.entries) != 0 {
		t.Errorf("expected no entries, got %d", len(tt.entries))
	}
}

func TestGetTimerStatusWhenRunning(t *testing.T) {
	tt := NewTimeTracker()
	tt.timerRunning = true
	tt.currentNote = "vault/deep/notes/my-note.md"
	tt.timerElapsed = 3 * time.Minute

	name, elapsed := tt.GetTimerStatus()
	if name != "my-note" {
		t.Errorf("expected name %q, got %q", "my-note", name)
	}
	if elapsed != 3*time.Minute {
		t.Errorf("expected elapsed 3m, got %v", elapsed)
	}
}

func TestGetTimerStatusWhenNotRunning(t *testing.T) {
	tt := NewTimeTracker()

	name, elapsed := tt.GetTimerStatus()
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
	if elapsed != 0 {
		t.Errorf("expected 0 elapsed, got %v", elapsed)
	}
}

// ---------------------------------------------------------------------------
// Time Entry Recording
// ---------------------------------------------------------------------------

func TestMultipleTimerSessions(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	// Session 1
	tt.StartTimer("a.md", "task-a")
	tt.activeTimer.StartTime = time.Now().Add(-10 * time.Minute)
	tt.StopTimer()

	// Session 2
	tt.StartTimer("b.md", "task-b")
	tt.activeTimer.StartTime = time.Now().Add(-20 * time.Minute)
	tt.StopTimer()

	if len(tt.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(tt.entries))
	}
	if tt.entries[0].NotePath != "a.md" {
		t.Errorf("expected first entry for a.md, got %q", tt.entries[0].NotePath)
	}
	if tt.entries[1].NotePath != "b.md" {
		t.Errorf("expected second entry for b.md, got %q", tt.entries[1].NotePath)
	}
}

// ---------------------------------------------------------------------------
// Pomodoro Counting
// ---------------------------------------------------------------------------

func TestRecordPomodoroExistingEntry(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	// Create an entry for today
	today := time.Now().Format("2006-01-02")
	tt.entries = []timeEntry{
		{
			NotePath:  "notes/study.md",
			TaskText:  "review",
			StartTime: time.Now().Add(-30 * time.Minute),
			EndTime:   time.Now(),
			Duration:  30 * time.Minute,
			Pomodoros: 0,
			Date:      today,
		},
	}

	tt.RecordPomodoro("notes/study.md", "review")

	if tt.entries[0].Pomodoros != 1 {
		t.Errorf("expected 1 pomodoro, got %d", tt.entries[0].Pomodoros)
	}

	// Record another
	tt.RecordPomodoro("notes/study.md", "review")
	if tt.entries[0].Pomodoros != 2 {
		t.Errorf("expected 2 pomodoros, got %d", tt.entries[0].Pomodoros)
	}
}

func TestRecordPomodoroCreatesNewEntry(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	tt.RecordPomodoro("notes/new.md", "fresh task")

	if len(tt.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(tt.entries))
	}

	entry := tt.entries[0]
	if entry.NotePath != "notes/new.md" {
		t.Errorf("expected NotePath %q, got %q", "notes/new.md", entry.NotePath)
	}
	if entry.TaskText != "fresh task" {
		t.Errorf("expected TaskText %q, got %q", "fresh task", entry.TaskText)
	}
	if entry.Pomodoros != 1 {
		t.Errorf("expected 1 pomodoro, got %d", entry.Pomodoros)
	}
	if entry.Duration != 0 {
		t.Errorf("expected 0 duration for stub entry, got %v", entry.Duration)
	}
	if entry.Date != time.Now().Format("2006-01-02") {
		t.Errorf("expected today's date, got %q", entry.Date)
	}
}

func TestRecordPomodoroMatchesMostRecentEntry(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	today := time.Now().Format("2006-01-02")
	// Two entries for the same note/task on the same day
	tt.entries = []timeEntry{
		{
			NotePath:  "notes/study.md",
			TaskText:  "review",
			Pomodoros: 0,
			Date:      today,
		},
		{
			NotePath:  "notes/study.md",
			TaskText:  "review",
			Pomodoros: 3,
			Date:      today,
		},
	}

	tt.RecordPomodoro("notes/study.md", "review")

	// Should increment the last (most recent) matching entry
	if tt.entries[1].Pomodoros != 4 {
		t.Errorf("expected 4 pomodoros on last entry, got %d", tt.entries[1].Pomodoros)
	}
	if tt.entries[0].Pomodoros != 0 {
		t.Errorf("expected 0 pomodoros on first entry, got %d", tt.entries[0].Pomodoros)
	}
}

func TestRecordPomodoroDoesNotMatchDifferentDay(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tt.entries = []timeEntry{
		{
			NotePath:  "notes/old.md",
			TaskText:  "old task",
			Pomodoros: 5,
			Date:      yesterday,
		},
	}

	tt.RecordPomodoro("notes/old.md", "old task")

	// Should create a new entry for today, not increment yesterday's
	if len(tt.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(tt.entries))
	}
	if tt.entries[0].Pomodoros != 5 {
		t.Errorf("expected yesterday's entry unchanged at 5 pomos, got %d", tt.entries[0].Pomodoros)
	}
	if tt.entries[1].Pomodoros != 1 {
		t.Errorf("expected new entry with 1 pomo, got %d", tt.entries[1].Pomodoros)
	}
}

// ---------------------------------------------------------------------------
// Daily Report Generation
// ---------------------------------------------------------------------------

func TestTodaySummary(t *testing.T) {
	tt := NewTimeTracker()

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	tt.entries = []timeEntry{
		{NotePath: "a.md", Duration: 30 * time.Minute, Pomodoros: 2, Date: today},
		{NotePath: "b.md", Duration: 15 * time.Minute, Pomodoros: 1, Date: today},
		{NotePath: "c.md", Duration: 60 * time.Minute, Pomodoros: 3, Date: yesterday},
	}

	tt.todaySummary()

	if len(tt.todayEntries) != 2 {
		t.Errorf("expected 2 today entries, got %d", len(tt.todayEntries))
	}
	if tt.totalToday != 45*time.Minute {
		t.Errorf("expected totalToday 45m, got %v", tt.totalToday)
	}
	if tt.pomodorosToday != 3 {
		t.Errorf("expected 3 pomodoros today, got %d", tt.pomodorosToday)
	}
}

func TestTodaySummaryNoEntries(t *testing.T) {
	tt := NewTimeTracker()
	tt.entries = nil
	tt.todaySummary()

	if len(tt.todayEntries) != 0 {
		t.Errorf("expected 0 today entries, got %d", len(tt.todayEntries))
	}
	if tt.totalToday != 0 {
		t.Errorf("expected 0 totalToday, got %v", tt.totalToday)
	}
	if tt.pomodorosToday != 0 {
		t.Errorf("expected 0 pomodoros today, got %d", tt.pomodorosToday)
	}
}

func TestRefreshDailyView(t *testing.T) {
	tt := NewTimeTracker()

	specificDate := time.Date(2026, 3, 5, 12, 0, 0, 0, time.Local)
	tt.viewDate = specificDate

	tt.entries = []timeEntry{
		{NotePath: "a.md", Duration: 20 * time.Minute, Pomodoros: 1, Date: "2026-03-05"},
		{NotePath: "b.md", Duration: 40 * time.Minute, Pomodoros: 2, Date: "2026-03-05"},
		{NotePath: "c.md", Duration: 10 * time.Minute, Pomodoros: 0, Date: "2026-03-06"},
	}

	tt.refreshDailyView()

	if len(tt.todayEntries) != 2 {
		t.Errorf("expected 2 entries for 2026-03-05, got %d", len(tt.todayEntries))
	}
	if tt.totalToday != 60*time.Minute {
		t.Errorf("expected totalToday 60m, got %v", tt.totalToday)
	}
	if tt.pomodorosToday != 3 {
		t.Errorf("expected 3 pomodoros, got %d", tt.pomodorosToday)
	}
}

// ---------------------------------------------------------------------------
// Weekly Report Data
// ---------------------------------------------------------------------------

func TestWeekSummary(t *testing.T) {
	tt := NewTimeTracker()

	// Use a known Monday (2026-03-02 is a Monday)
	now := time.Now()
	today := now.Format("2006-01-02")

	// Calculate this week's Monday
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	mondayOffset := int(weekday) - int(time.Monday)
	monday := now.AddDate(0, 0, -mondayOffset)
	mondayStr := monday.Format("2006-01-02")
	tuesdayStr := monday.AddDate(0, 0, 1).Format("2006-01-02")
	lastWeekStr := monday.AddDate(0, 0, -3).Format("2006-01-02")

	tt.entries = []timeEntry{
		{NotePath: "a.md", Duration: 30 * time.Minute, Pomodoros: 2, Date: mondayStr},
		{NotePath: "b.md", Duration: 45 * time.Minute, Pomodoros: 3, Date: tuesdayStr},
		{NotePath: "c.md", Duration: 60 * time.Minute, Pomodoros: 4, Date: lastWeekStr},
	}

	// Ensure today falls within the week range for the test to make sense.
	_ = today

	tt.weekSummary()

	if len(tt.weekEntries) != 2 {
		t.Errorf("expected 2 week entries, got %d", len(tt.weekEntries))
	}
	if tt.totalWeek != 75*time.Minute {
		t.Errorf("expected totalWeek 75m, got %v", tt.totalWeek)
	}
	if tt.pomodorosWeek != 5 {
		t.Errorf("expected 5 pomodoros this week, got %d", tt.pomodorosWeek)
	}
}

func TestRefreshWeeklyView(t *testing.T) {
	tt := NewTimeTracker()

	// Set viewDate to a known Wednesday (2026-03-04)
	tt.viewDate = time.Date(2026, 3, 4, 12, 0, 0, 0, time.Local)

	// That week's Monday is 2026-03-02, Sunday is 2026-03-08
	tt.entries = []timeEntry{
		{NotePath: "a.md", Duration: 20 * time.Minute, Pomodoros: 1, Date: "2026-03-02"},
		{NotePath: "b.md", Duration: 30 * time.Minute, Pomodoros: 2, Date: "2026-03-04"},
		{NotePath: "c.md", Duration: 40 * time.Minute, Pomodoros: 3, Date: "2026-03-08"},
		{NotePath: "d.md", Duration: 50 * time.Minute, Pomodoros: 4, Date: "2026-03-09"}, // next week
		{NotePath: "e.md", Duration: 60 * time.Minute, Pomodoros: 5, Date: "2026-03-01"}, // prev week
	}

	tt.refreshWeeklyView()

	if len(tt.weekEntries) != 3 {
		t.Errorf("expected 3 week entries, got %d", len(tt.weekEntries))
	}
	if tt.totalWeek != 90*time.Minute {
		t.Errorf("expected totalWeek 90m, got %v", tt.totalWeek)
	}
	if tt.pomodorosWeek != 6 {
		t.Errorf("expected 6 pomodoros, got %d", tt.pomodorosWeek)
	}
}

func TestWeekSummaryExcludesLastWeek(t *testing.T) {
	tt := NewTimeTracker()

	// All entries from last week
	now := time.Now()
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	mondayOffset := int(weekday) - int(time.Monday)
	lastMonday := now.AddDate(0, 0, -mondayOffset-7)

	tt.entries = []timeEntry{
		{NotePath: "old.md", Duration: 2 * time.Hour, Pomodoros: 8, Date: lastMonday.Format("2006-01-02")},
	}

	tt.weekSummary()

	if len(tt.weekEntries) != 0 {
		t.Errorf("expected 0 week entries for last week's data, got %d", len(tt.weekEntries))
	}
	if tt.totalWeek != 0 {
		t.Errorf("expected totalWeek 0, got %v", tt.totalWeek)
	}
}

// ---------------------------------------------------------------------------
// Per-Note Time Aggregation
// ---------------------------------------------------------------------------

func TestNoteHistory(t *testing.T) {
	tt := NewTimeTracker()

	tt.entries = []timeEntry{
		{NotePath: "notes/alpha.md", Duration: 10 * time.Minute, Date: "2026-03-01"},
		{NotePath: "notes/beta.md", Duration: 20 * time.Minute, Date: "2026-03-02"},
		{NotePath: "notes/alpha.md", Duration: 30 * time.Minute, Date: "2026-03-03"},
		{NotePath: "notes/gamma.md", Duration: 40 * time.Minute, Date: "2026-03-04"},
		{NotePath: "notes/alpha.md", Duration: 50 * time.Minute, Date: "2026-03-05"},
	}

	result := tt.noteHistory("notes/alpha.md")
	if len(result) != 3 {
		t.Fatalf("expected 3 entries for alpha.md, got %d", len(result))
	}
	for _, e := range result {
		if e.NotePath != "notes/alpha.md" {
			t.Errorf("expected all entries for alpha.md, got %q", e.NotePath)
		}
	}
}

func TestNoteHistoryNoMatches(t *testing.T) {
	tt := NewTimeTracker()
	tt.entries = []timeEntry{
		{NotePath: "a.md", Date: "2026-03-01"},
	}

	result := tt.noteHistory("nonexistent.md")
	if len(result) != 0 {
		t.Errorf("expected 0 entries for nonexistent note, got %d", len(result))
	}
}

func TestTopNotesByTime(t *testing.T) {
	entries := []timeEntry{
		{NotePath: "a.md", Duration: 10 * time.Minute, Pomodoros: 1},
		{NotePath: "b.md", Duration: 30 * time.Minute, Pomodoros: 2},
		{NotePath: "a.md", Duration: 20 * time.Minute, Pomodoros: 1},
		{NotePath: "c.md", Duration: 5 * time.Minute, Pomodoros: 0},
		{NotePath: "b.md", Duration: 15 * time.Minute, Pomodoros: 1},
	}

	result := topNotesByTime(entries, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	// b.md should be first (45m), then a.md (30m), then c.md (5m)
	if result[0].NotePath != "b.md" {
		t.Errorf("expected b.md first, got %q", result[0].NotePath)
	}
	if result[0].TotalTime != 45*time.Minute {
		t.Errorf("expected 45m for b.md, got %v", result[0].TotalTime)
	}
	if result[0].TotalPomos != 3 {
		t.Errorf("expected 3 pomos for b.md, got %d", result[0].TotalPomos)
	}
	if result[0].Sessions != 2 {
		t.Errorf("expected 2 sessions for b.md, got %d", result[0].Sessions)
	}

	if result[1].NotePath != "a.md" {
		t.Errorf("expected a.md second, got %q", result[1].NotePath)
	}
	if result[1].TotalTime != 30*time.Minute {
		t.Errorf("expected 30m for a.md, got %v", result[1].TotalTime)
	}

	if result[2].NotePath != "c.md" {
		t.Errorf("expected c.md third, got %q", result[2].NotePath)
	}
}

func TestTopNotesByTimeLimitN(t *testing.T) {
	entries := []timeEntry{
		{NotePath: "a.md", Duration: 10 * time.Minute},
		{NotePath: "b.md", Duration: 20 * time.Minute},
		{NotePath: "c.md", Duration: 30 * time.Minute},
		{NotePath: "d.md", Duration: 40 * time.Minute},
	}

	result := topNotesByTime(entries, 2)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].NotePath != "d.md" {
		t.Errorf("expected d.md first, got %q", result[0].NotePath)
	}
	if result[1].NotePath != "c.md" {
		t.Errorf("expected c.md second, got %q", result[1].NotePath)
	}
}

func TestTopNotesByTimeEmpty(t *testing.T) {
	result := topNotesByTime(nil, 5)
	if len(result) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(result))
	}
}

// ---------------------------------------------------------------------------
// Per-Task Time Aggregation
// ---------------------------------------------------------------------------

func TestTopNotesByTimeGroupsByNotePath(t *testing.T) {
	// Different tasks on the same note should be aggregated under the note
	entries := []timeEntry{
		{NotePath: "project.md", TaskText: "design", Duration: 15 * time.Minute, Pomodoros: 1},
		{NotePath: "project.md", TaskText: "implement", Duration: 25 * time.Minute, Pomodoros: 2},
		{NotePath: "project.md", TaskText: "test", Duration: 10 * time.Minute, Pomodoros: 1},
	}

	result := topNotesByTime(entries, 10)
	if len(result) != 1 {
		t.Fatalf("expected 1 aggregate, got %d", len(result))
	}
	if result[0].TotalTime != 50*time.Minute {
		t.Errorf("expected 50m total, got %v", result[0].TotalTime)
	}
	if result[0].TotalPomos != 4 {
		t.Errorf("expected 4 pomos total, got %d", result[0].TotalPomos)
	}
	if result[0].Sessions != 3 {
		t.Errorf("expected 3 sessions, got %d", result[0].Sessions)
	}
}

func TestRecordPomodoroDistinguishesTasks(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	today := time.Now().Format("2006-01-02")
	tt.entries = []timeEntry{
		{NotePath: "a.md", TaskText: "task-x", Pomodoros: 2, Date: today},
		{NotePath: "a.md", TaskText: "task-y", Pomodoros: 1, Date: today},
	}

	tt.RecordPomodoro("a.md", "task-x")

	// Should increment the last matching entry for task-x (entry 0, scanned from end)
	// Actually, it scans from end. task-y is at index 1, task-x is at index 0.
	// It looks for matching notePath AND taskText, so it finds entry 0.
	if tt.entries[0].Pomodoros != 3 {
		t.Errorf("expected task-x entry to have 3 pomos, got %d", tt.entries[0].Pomodoros)
	}
	if tt.entries[1].Pomodoros != 1 {
		t.Errorf("expected task-y entry unchanged at 1 pomo, got %d", tt.entries[1].Pomodoros)
	}
}

// ---------------------------------------------------------------------------
// Daily Totals
// ---------------------------------------------------------------------------

func TestDailyTotals(t *testing.T) {
	tt := NewTimeTracker()

	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	twoDaysAgo := time.Now().AddDate(0, 0, -2).Format("2006-01-02")

	tt.entries = []timeEntry{
		{NotePath: "a.md", Duration: 30 * time.Minute, Pomodoros: 2, Date: today},
		{NotePath: "b.md", Duration: 15 * time.Minute, Pomodoros: 1, Date: today},
		{NotePath: "c.md", Duration: 60 * time.Minute, Pomodoros: 4, Date: yesterday},
		{NotePath: "d.md", Duration: 45 * time.Minute, Pomodoros: 3, Date: twoDaysAgo},
	}

	result := tt.dailyTotals(3)

	if len(result) != 3 {
		t.Fatalf("expected 3 day totals, got %d", len(result))
	}

	// Result is ordered oldest to newest
	if result[0].Date != twoDaysAgo {
		t.Errorf("expected first day %q, got %q", twoDaysAgo, result[0].Date)
	}
	if result[0].Duration != 45*time.Minute {
		t.Errorf("expected 45m for two days ago, got %v", result[0].Duration)
	}
	if result[0].Pomos != 3 {
		t.Errorf("expected 3 pomos two days ago, got %d", result[0].Pomos)
	}

	if result[1].Date != yesterday {
		t.Errorf("expected second day %q, got %q", yesterday, result[1].Date)
	}
	if result[1].Duration != 60*time.Minute {
		t.Errorf("expected 60m yesterday, got %v", result[1].Duration)
	}

	if result[2].Date != today {
		t.Errorf("expected third day %q, got %q", today, result[2].Date)
	}
	if result[2].Duration != 45*time.Minute {
		t.Errorf("expected 45m today, got %v", result[2].Duration)
	}
}

func TestDailyTotalsZeroDaysWithData(t *testing.T) {
	tt := NewTimeTracker()

	// Request 5 days but only have data for 1
	today := time.Now().Format("2006-01-02")
	tt.entries = []timeEntry{
		{NotePath: "a.md", Duration: 20 * time.Minute, Pomodoros: 1, Date: today},
	}

	result := tt.dailyTotals(5)
	if len(result) != 5 {
		t.Fatalf("expected 5 day totals, got %d", len(result))
	}

	// All days except today should have zero duration
	for i, dt := range result {
		if dt.Date == today {
			if dt.Duration != 20*time.Minute {
				t.Errorf("expected 20m for today, got %v", dt.Duration)
			}
		} else {
			if dt.Duration != 0 {
				t.Errorf("expected 0 duration for day %d (%s), got %v", i, dt.Date, dt.Duration)
			}
		}
	}
}

func TestDailyTotalsNoEntries(t *testing.T) {
	tt := NewTimeTracker()
	tt.entries = nil

	result := tt.dailyTotals(7)
	if len(result) != 7 {
		t.Fatalf("expected 7 day totals, got %d", len(result))
	}
	for _, dt := range result {
		if dt.Duration != 0 {
			t.Errorf("expected 0 duration for %s, got %v", dt.Date, dt.Duration)
		}
	}
}

// ---------------------------------------------------------------------------
// JSON Persistence
// ---------------------------------------------------------------------------

func TestSaveAndLoadEntries(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	now := time.Now()
	tt.entries = []timeEntry{
		{
			NotePath:  "notes/alpha.md",
			TaskText:  "write chapter 1",
			StartTime: now.Add(-2 * time.Hour),
			EndTime:   now.Add(-1 * time.Hour),
			Duration:  1 * time.Hour,
			Pomodoros: 4,
			Date:      now.Format("2006-01-02"),
		},
		{
			NotePath:  "notes/beta.md",
			TaskText:  "review feedback",
			StartTime: now.Add(-30 * time.Minute),
			EndTime:   now,
			Duration:  30 * time.Minute,
			Pomodoros: 2,
			Date:      now.Format("2006-01-02"),
		},
	}

	tt.saveEntries()

	// Verify file exists
	storagePath := filepath.Join(tmp, ".granit", "timetracker.json")
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		t.Fatal("expected timetracker.json to be created")
	}

	// Load into a new tracker
	tt2 := NewTimeTracker()
	tt2.vaultRoot = tmp
	tt2.loadEntries()

	if len(tt2.entries) != 2 {
		t.Fatalf("expected 2 entries after load, got %d", len(tt2.entries))
	}
	if tt2.entries[0].NotePath != "notes/alpha.md" {
		t.Errorf("expected first entry NotePath %q, got %q", "notes/alpha.md", tt2.entries[0].NotePath)
	}
	if tt2.entries[0].TaskText != "write chapter 1" {
		t.Errorf("expected first entry TaskText %q, got %q", "write chapter 1", tt2.entries[0].TaskText)
	}
	if tt2.entries[0].Pomodoros != 4 {
		t.Errorf("expected 4 pomodoros, got %d", tt2.entries[0].Pomodoros)
	}
	if tt2.entries[0].Duration != 1*time.Hour {
		t.Errorf("expected 1h duration, got %v", tt2.entries[0].Duration)
	}
	if tt2.entries[1].NotePath != "notes/beta.md" {
		t.Errorf("expected second entry NotePath %q, got %q", "notes/beta.md", tt2.entries[1].NotePath)
	}
}

func TestStoragePath(t *testing.T) {
	tt := NewTimeTracker()
	tt.vaultRoot = "/my/vault"
	expected := filepath.Join("/my/vault", ".granit", "timetracker.json")
	if got := tt.storagePath(); got != expected {
		t.Errorf("expected storagePath %q, got %q", expected, got)
	}
}

func TestLoadEntriesMissingFile(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	// Should not panic with missing file
	tt.loadEntries()
	if tt.entries != nil {
		t.Errorf("expected nil entries when file missing, got %v", tt.entries)
	}
}

func TestLoadEntriesInvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	granitDir := filepath.Join(tmp, ".granit")
	if err := os.MkdirAll(granitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(granitDir, "timetracker.json"), []byte("not valid json{{{"), 0644); err != nil {
		t.Fatal(err)
	}

	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	// Should not panic with invalid JSON
	tt.loadEntries()
	// entries should remain nil/empty since unmarshal failed
	if len(tt.entries) != 0 {
		t.Errorf("expected 0 entries with invalid JSON, got %d", len(tt.entries))
	}
}

func TestSaveEntriesCreatesDirectory(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	tt.entries = []timeEntry{
		{NotePath: "test.md", Date: "2026-03-08"},
	}

	tt.saveEntries()

	granitDir := filepath.Join(tmp, ".granit")
	if _, err := os.Stat(granitDir); os.IsNotExist(err) {
		t.Error("expected .granit directory to be created")
	}
}

func TestSaveEntriesJSONFormat(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	tt.entries = []timeEntry{
		{
			NotePath:  "notes/test.md",
			TaskText:  "task one",
			Pomodoros: 3,
			Duration:  25 * time.Minute,
			Date:      "2026-03-08",
		},
	}

	tt.saveEntries()

	data, err := os.ReadFile(filepath.Join(tmp, ".granit", "timetracker.json"))
	if err != nil {
		t.Fatal(err)
	}

	var loaded []timeEntry
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("saved JSON is not valid: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 entry in JSON, got %d", len(loaded))
	}
	if loaded[0].NotePath != "notes/test.md" {
		t.Errorf("expected NotePath %q in JSON, got %q", "notes/test.md", loaded[0].NotePath)
	}
}

func TestStopTimerPersistsEntry(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	tt.StartTimer("persist.md", "test persistence")
	tt.activeTimer.StartTime = time.Now().Add(-3 * time.Minute)
	tt.StopTimer()

	// Verify persistence by loading from disk
	tt2 := NewTimeTracker()
	tt2.vaultRoot = tmp
	tt2.loadEntries()

	if len(tt2.entries) != 1 {
		t.Fatalf("expected 1 persisted entry, got %d", len(tt2.entries))
	}
	if tt2.entries[0].NotePath != "persist.md" {
		t.Errorf("expected persisted entry NotePath %q, got %q", "persist.md", tt2.entries[0].NotePath)
	}
}

// ---------------------------------------------------------------------------
// Edge Cases
// ---------------------------------------------------------------------------

func TestNoEntries(t *testing.T) {
	tt := NewTimeTracker()
	tt.entries = nil

	tt.todaySummary()
	tt.weekSummary()

	if tt.totalToday != 0 {
		t.Errorf("expected 0 totalToday, got %v", tt.totalToday)
	}
	if tt.totalWeek != 0 {
		t.Errorf("expected 0 totalWeek, got %v", tt.totalWeek)
	}
	if tt.pomodorosToday != 0 {
		t.Errorf("expected 0 pomodorosToday, got %d", tt.pomodorosToday)
	}
	if tt.pomodorosWeek != 0 {
		t.Errorf("expected 0 pomodorosWeek, got %d", tt.pomodorosWeek)
	}
	if len(tt.todayEntries) != 0 {
		t.Errorf("expected 0 todayEntries, got %d", len(tt.todayEntries))
	}
	if len(tt.weekEntries) != 0 {
		t.Errorf("expected 0 weekEntries, got %d", len(tt.weekEntries))
	}

	result := tt.noteHistory("anything.md")
	if len(result) != 0 {
		t.Errorf("expected 0 noteHistory, got %d", len(result))
	}

	top := topNotesByTime(tt.entries, 5)
	if len(top) != 0 {
		t.Errorf("expected 0 topNotes, got %d", len(top))
	}

	dailyData := tt.dailyTotals(7)
	if len(dailyData) != 7 {
		t.Errorf("expected 7 day totals even with no data, got %d", len(dailyData))
	}
}

func TestTimerAcrossMidnight(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	// Simulate a timer started before midnight
	yesterday := time.Now().AddDate(0, 0, -1)
	yesterdayLate := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 30, 0, 0, time.Local)

	tt.activeTimer = &timeEntry{
		NotePath:  "late-night.md",
		TaskText:  "midnight work",
		StartTime: yesterdayLate,
		Date:      yesterdayLate.Format("2006-01-02"),
	}
	tt.timerRunning = true
	tt.timerStart = yesterdayLate

	tt.StopTimer()

	if len(tt.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(tt.entries))
	}

	entry := tt.entries[0]
	// The entry's date is from when the timer started (yesterday)
	if entry.Date != yesterdayLate.Format("2006-01-02") {
		t.Errorf("expected date from start time %q, got %q", yesterdayLate.Format("2006-01-02"), entry.Date)
	}
	// Duration should span across midnight
	if entry.Duration < 30*time.Minute {
		t.Errorf("expected duration > 30m (spanning midnight), got %v", entry.Duration)
	}
	if entry.EndTime.Before(entry.StartTime) {
		t.Error("EndTime should be after StartTime even across midnight")
	}
}

func TestVeryLongSession(t *testing.T) {
	tmp := t.TempDir()
	tt := NewTimeTracker()
	tt.vaultRoot = tmp

	// Simulate a timer running for 12 hours
	now := time.Now()
	tt.activeTimer = &timeEntry{
		NotePath:  "marathon.md",
		TaskText:  "long session",
		StartTime: now.Add(-12 * time.Hour),
		Date:      now.Add(-12 * time.Hour).Format("2006-01-02"),
	}
	tt.timerRunning = true
	tt.timerStart = now.Add(-12 * time.Hour)

	tt.StopTimer()

	if len(tt.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(tt.entries))
	}

	entry := tt.entries[0]
	if entry.Duration < 11*time.Hour || entry.Duration > 13*time.Hour {
		t.Errorf("expected duration ~12h, got %v", entry.Duration)
	}
}

func TestStartTimerOverwritesPrevious(t *testing.T) {
	tt := NewTimeTracker()
	tt.vaultRoot = t.TempDir()

	tt.StartTimer("first.md", "first task")
	tt.StartTimer("second.md", "second task")

	if tt.currentNote != "second.md" {
		t.Errorf("expected currentNote %q, got %q", "second.md", tt.currentNote)
	}
	if tt.activeTimer.NotePath != "second.md" {
		t.Errorf("expected activeTimer for second.md, got %q", tt.activeTimer.NotePath)
	}
}

func TestOpenLoadsExistingEntries(t *testing.T) {
	tmp := t.TempDir()

	// Pre-populate entries on disk
	granitDir := filepath.Join(tmp, ".granit")
	if err := os.MkdirAll(granitDir, 0755); err != nil {
		t.Fatal(err)
	}

	entries := []timeEntry{
		{NotePath: "saved.md", TaskText: "saved task", Pomodoros: 5, Date: time.Now().Format("2006-01-02")},
	}
	data, _ := json.MarshalIndent(entries, "", "  ")
	if err := os.WriteFile(filepath.Join(granitDir, "timetracker.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	tt := NewTimeTracker()
	tt.Open(tmp)

	if len(tt.entries) != 1 {
		t.Fatalf("expected 1 entry loaded from disk, got %d", len(tt.entries))
	}
	if tt.entries[0].NotePath != "saved.md" {
		t.Errorf("expected NotePath %q, got %q", "saved.md", tt.entries[0].NotePath)
	}
}

// ---------------------------------------------------------------------------
// Formatting Helpers
// ---------------------------------------------------------------------------

func TestTtFormatDuration(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{0, "0s"},
		{-1 * time.Second, "0s"},
		{30 * time.Second, "30s"},
		{1 * time.Minute, "1m 0s"},
		{1*time.Minute + 30*time.Second, "1m 30s"},
		{1 * time.Hour, "1h 0m"},
		{1*time.Hour + 30*time.Minute, "1h 30m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
		{24 * time.Hour, "24h 0m"},
	}

	for _, tc := range tests {
		got := ttFormatDuration(tc.input)
		if got != tc.want {
			t.Errorf("ttFormatDuration(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestFormatDurationShort(t *testing.T) {
	tests := []struct {
		input time.Duration
		want  string
	}{
		{0, "0m"},
		{-1 * time.Second, "0m"},
		{30 * time.Second, "30s"},
		{1 * time.Minute, "1m"},
		{1*time.Minute + 30*time.Second, "1m"},
		{1 * time.Hour, "1h"},
		{1*time.Hour + 30*time.Minute, "1h30m"},
		{3 * time.Hour, "3h"},
	}

	for _, tc := range tests {
		got := formatDurationShort(tc.input)
		if got != tc.want {
			t.Errorf("formatDurationShort(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRenderPomoIcons(t *testing.T) {
	// 0 pomodoros should return empty
	result := renderPomoIcons(0)
	if result != "" {
		t.Errorf("expected empty for 0 pomos, got %q", result)
	}

	// Negative should return empty
	result = renderPomoIcons(-1)
	if result != "" {
		t.Errorf("expected empty for -1 pomos, got %q", result)
	}

	// Should not panic for large values
	result = renderPomoIcons(100)
	if result == "" {
		t.Error("expected non-empty for 100 pomos")
	}
}

func TestTtPadRight(t *testing.T) {
	tests := []struct {
		s     string
		width int
		want  string
	}{
		{"abc", 6, "abc   "},
		{"hello", 5, "hello"},
		{"long string", 5, "long string"},
		{"", 3, "   "},
		{"a", 1, "a"},
	}

	for _, tc := range tests {
		got := ttPadRight(tc.s, tc.width)
		if got != tc.want {
			t.Errorf("ttPadRight(%q, %d) = %q, want %q", tc.s, tc.width, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Scroll Adjustment
// ---------------------------------------------------------------------------

func TestAdjustScroll(t *testing.T) {
	tt := NewTimeTracker()
	tt.height = 20
	tt.cursor = 15

	tt.adjustScroll()

	// visH = height - 12 = 8
	// cursor (15) >= scroll (0) + visH (8), so scroll = 15 - 8 + 1 = 8
	if tt.scroll != 8 {
		t.Errorf("expected scroll 8, got %d", tt.scroll)
	}
}

func TestAdjustScrollSmallHeight(t *testing.T) {
	tt := NewTimeTracker()
	tt.height = 5 // visH would be negative, clamped to 1
	tt.cursor = 3

	tt.adjustScroll()

	// visH = max(5-12, 1) = 1
	// cursor (3) >= scroll (0) + 1, so scroll = 3 - 1 + 1 = 3
	if tt.scroll != 3 {
		t.Errorf("expected scroll 3, got %d", tt.scroll)
	}
}
