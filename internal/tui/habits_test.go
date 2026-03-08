package tui

import (
	"testing"
	"time"
)

// ── Initialization ───────────────────────────────────────────────

func TestNewHabitTracker(t *testing.T) {
	ht := NewHabitTracker()
	if ht.active {
		t.Fatal("new HabitTracker should not be active")
	}
	if ht.goalExpanded != -1 {
		t.Fatalf("goalExpanded should be -1, got %d", ht.goalExpanded)
	}
	if ht.habits != nil {
		t.Fatal("habits should be nil on init")
	}
	if ht.goals != nil {
		t.Fatal("goals should be nil on init")
	}
	if ht.logs != nil {
		t.Fatal("logs should be nil on init")
	}
	if ht.tab != 0 {
		t.Fatalf("tab should be 0, got %d", ht.tab)
	}
}

func TestHabitTracker_IsActive(t *testing.T) {
	ht := NewHabitTracker()
	if ht.IsActive() {
		t.Fatal("should not be active after construction")
	}
	ht.active = true
	if !ht.IsActive() {
		t.Fatal("should be active after setting active=true")
	}
}

func TestHabitTracker_SetSize(t *testing.T) {
	ht := NewHabitTracker()
	ht.SetSize(120, 40)
	if ht.width != 120 {
		t.Fatalf("expected width 120, got %d", ht.width)
	}
	if ht.height != 40 {
		t.Fatalf("expected height 40, got %d", ht.height)
	}
}

func TestHabitTracker_Close(t *testing.T) {
	ht := NewHabitTracker()
	ht.active = true
	ht.Close()
	if ht.active {
		t.Fatal("Close should set active to false")
	}
}

// ── Habit Creation ───────────────────────────────────────────────

func TestHabitCreation_DifferentFrequencies(t *testing.T) {
	// Habits are stored as simple entries. Test that creating multiple
	// habits with different names works correctly.
	habits := []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
		{Name: "Read 30 min", Created: "2026-03-02", Streak: 0},
		{Name: "Meditate", Created: "2026-03-03", Streak: 0},
		{Name: "Journal", Created: "2026-03-04", Streak: 0},
		{Name: "Drink Water", Created: "2026-03-05", Streak: 0},
	}

	ht := NewHabitTracker()
	ht.habits = habits

	if len(ht.habits) != 5 {
		t.Fatalf("expected 5 habits, got %d", len(ht.habits))
	}

	for i, h := range habits {
		if ht.habits[i].Name != h.Name {
			t.Errorf("habit %d: expected name %q, got %q", i, h.Name, ht.habits[i].Name)
		}
		if ht.habits[i].Created != h.Created {
			t.Errorf("habit %d: expected created %q, got %q", i, h.Created, ht.habits[i].Created)
		}
		if ht.habits[i].Streak != 0 {
			t.Errorf("habit %d: expected streak 0, got %d", i, ht.habits[i].Streak)
		}
	}
}

func TestHabitEntry_Fields(t *testing.T) {
	h := habitEntry{
		Name:    "Daily Walk",
		Created: "2026-01-15",
		Streak:  7,
	}
	if h.Name != "Daily Walk" {
		t.Errorf("expected name 'Daily Walk', got %q", h.Name)
	}
	if h.Created != "2026-01-15" {
		t.Errorf("expected created '2026-01-15', got %q", h.Created)
	}
	if h.Streak != 7 {
		t.Errorf("expected streak 7, got %d", h.Streak)
	}
}

// ── Daily Checkbox Toggling ──────────────────────────────────────

func TestToggleToday_AddCompletion(t *testing.T) {
	ht := NewHabitTracker()
	ht.vaultRoot = t.TempDir()
	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
	}

	today := todayStr()

	// Initially no logs
	if ht.isTodayCompleted("Exercise") {
		t.Fatal("should not be completed before toggle")
	}

	ht.toggleToday("Exercise")

	if !ht.isTodayCompleted("Exercise") {
		t.Fatal("should be completed after toggle")
	}

	// Verify the log entry was created
	found := false
	for _, log := range ht.logs {
		if log.Date == today {
			for _, c := range log.Completed {
				if c == "Exercise" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatal("expected Exercise in today's log")
	}
}

func TestToggleToday_RemoveCompletion(t *testing.T) {
	ht := NewHabitTracker()
	ht.vaultRoot = t.TempDir()
	today := todayStr()

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
	}
	ht.logs = []habitLog{
		{Date: today, Completed: []string{"Exercise"}},
	}

	// Should be completed
	if !ht.isTodayCompleted("Exercise") {
		t.Fatal("should be completed initially")
	}

	// Toggle off
	ht.toggleToday("Exercise")

	if ht.isTodayCompleted("Exercise") {
		t.Fatal("should not be completed after untoggle")
	}
}

func TestToggleToday_MultipleHabits(t *testing.T) {
	ht := NewHabitTracker()
	ht.vaultRoot = t.TempDir()

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
		{Name: "Read", Created: "2026-03-01", Streak: 0},
		{Name: "Meditate", Created: "2026-03-01", Streak: 0},
	}

	// Toggle first two
	ht.toggleToday("Exercise")
	ht.toggleToday("Read")

	if !ht.isTodayCompleted("Exercise") {
		t.Fatal("Exercise should be completed")
	}
	if !ht.isTodayCompleted("Read") {
		t.Fatal("Read should be completed")
	}
	if ht.isTodayCompleted("Meditate") {
		t.Fatal("Meditate should not be completed")
	}

	// Toggle Exercise off
	ht.toggleToday("Exercise")
	if ht.isTodayCompleted("Exercise") {
		t.Fatal("Exercise should be uncompleted after second toggle")
	}
	if !ht.isTodayCompleted("Read") {
		t.Fatal("Read should still be completed")
	}
}

func TestToggleToday_RemoveLastCompletionDeletesLogEntry(t *testing.T) {
	ht := NewHabitTracker()
	ht.vaultRoot = t.TempDir()
	today := todayStr()

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
	}
	ht.logs = []habitLog{
		{Date: today, Completed: []string{"Exercise"}},
	}

	ht.toggleToday("Exercise")

	// The entire log entry should be removed since it was the only completion
	for _, log := range ht.logs {
		if log.Date == today {
			t.Fatal("log entry for today should be removed when last habit is untoggled")
		}
	}
}

// ── 7-Day Streak Calculation ─────────────────────────────────────

func TestRecalcStreaks_ConsecutiveDays(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()
	todayTime, _ := time.Parse("2006-01-02", today)

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
	}

	// Build 7 consecutive days of logs ending today
	var logs []habitLog
	for i := 6; i >= 0; i-- {
		d := todayTime.AddDate(0, 0, -i)
		logs = append(logs, habitLog{
			Date:      d.Format("2006-01-02"),
			Completed: []string{"Exercise"},
		})
	}
	ht.logs = logs

	ht.recalcStreaks()

	if ht.habits[0].Streak != 7 {
		t.Fatalf("expected streak of 7, got %d", ht.habits[0].Streak)
	}
}

func TestRecalcStreaks_BrokenStreak(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()
	todayTime, _ := time.Parse("2006-01-02", today)

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
	}

	// Today + yesterday completed, then gap, then 3 more days
	ht.logs = []habitLog{
		{Date: todayTime.Format("2006-01-02"), Completed: []string{"Exercise"}},
		{Date: todayTime.AddDate(0, 0, -1).Format("2006-01-02"), Completed: []string{"Exercise"}},
		// Gap at -2
		{Date: todayTime.AddDate(0, 0, -3).Format("2006-01-02"), Completed: []string{"Exercise"}},
		{Date: todayTime.AddDate(0, 0, -4).Format("2006-01-02"), Completed: []string{"Exercise"}},
		{Date: todayTime.AddDate(0, 0, -5).Format("2006-01-02"), Completed: []string{"Exercise"}},
	}

	ht.recalcStreaks()

	// Streak should be 2 (today + yesterday), broken at day -2
	if ht.habits[0].Streak != 2 {
		t.Fatalf("expected streak of 2 (broken by gap), got %d", ht.habits[0].Streak)
	}
}

func TestRecalcStreaks_NothingToday(t *testing.T) {
	ht := NewHabitTracker()
	todayTime, _ := time.Parse("2006-01-02", todayStr())

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
	}

	// Only yesterday completed
	ht.logs = []habitLog{
		{Date: todayTime.AddDate(0, 0, -1).Format("2006-01-02"), Completed: []string{"Exercise"}},
	}

	ht.recalcStreaks()

	// Streak should be 0 because today is not completed
	if ht.habits[0].Streak != 0 {
		t.Fatalf("expected streak of 0 (not done today), got %d", ht.habits[0].Streak)
	}
}

func TestRecalcStreaks_MultipleHabits(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()
	todayTime, _ := time.Parse("2006-01-02", today)

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
		{Name: "Read", Created: "2026-03-01", Streak: 0},
	}

	// Exercise: 3-day streak. Read: 1-day streak (only today).
	ht.logs = []habitLog{
		{Date: todayTime.Format("2006-01-02"), Completed: []string{"Exercise", "Read"}},
		{Date: todayTime.AddDate(0, 0, -1).Format("2006-01-02"), Completed: []string{"Exercise"}},
		{Date: todayTime.AddDate(0, 0, -2).Format("2006-01-02"), Completed: []string{"Exercise"}},
	}

	ht.recalcStreaks()

	if ht.habits[0].Streak != 3 {
		t.Fatalf("Exercise: expected streak 3, got %d", ht.habits[0].Streak)
	}
	if ht.habits[1].Streak != 1 {
		t.Fatalf("Read: expected streak 1, got %d", ht.habits[1].Streak)
	}
}

// ── Streak Blocks (7-day visual) ─────────────────────────────────

func TestStreakBlocks_AllCompleted(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()
	todayTime, _ := time.Parse("2006-01-02", today)

	// Create logs for all 7 days
	for i := 6; i >= 0; i-- {
		d := todayTime.AddDate(0, 0, -i)
		ht.logs = append(ht.logs, habitLog{
			Date:      d.Format("2006-01-02"),
			Completed: []string{"Exercise"},
		})
	}

	blocks := ht.streakBlocks("Exercise")
	if blocks == "" {
		t.Fatal("streakBlocks should not return empty string")
	}
	// The result contains ANSI color codes, so just verify it's non-empty
	// and has content (7 blocks joined without separator)
}

func TestStreakBlocks_NoneCompleted(t *testing.T) {
	ht := NewHabitTracker()
	// No logs at all
	blocks := ht.streakBlocks("Exercise")
	if blocks == "" {
		t.Fatal("streakBlocks should return blocks even with no completions")
	}
}

// ── Goal Creation with Milestones ────────────────────────────────

func TestGoalEntry_GoalProgress_NoMilestones(t *testing.T) {
	g := goalEntry{
		Title:      "Learn Go",
		TargetDate: "2026-06-01",
	}
	if g.goalProgress() != 0 {
		t.Fatalf("expected 0%% progress with no milestones, got %d%%", g.goalProgress())
	}
}

func TestGoalEntry_GoalProgress_AllComplete(t *testing.T) {
	g := goalEntry{
		Title:      "Learn Go",
		TargetDate: "2026-06-01",
		Milestones: []milestone{
			{Text: "Read tutorial", Done: true},
			{Text: "Build project", Done: true},
			{Text: "Write tests", Done: true},
		},
	}
	if g.goalProgress() != 100 {
		t.Fatalf("expected 100%% progress, got %d%%", g.goalProgress())
	}
}

func TestGoalEntry_GoalProgress_Partial(t *testing.T) {
	g := goalEntry{
		Title:      "Learn Go",
		TargetDate: "2026-06-01",
		Milestones: []milestone{
			{Text: "Read tutorial", Done: true},
			{Text: "Build project", Done: false},
			{Text: "Write tests", Done: false},
			{Text: "Deploy", Done: false},
		},
	}
	// 1 out of 4 = 25%
	if g.goalProgress() != 25 {
		t.Fatalf("expected 25%% progress, got %d%%", g.goalProgress())
	}
}

func TestGoalEntry_GoalProgress_Half(t *testing.T) {
	g := goalEntry{
		Title:      "Learn Go",
		TargetDate: "2026-06-01",
		Milestones: []milestone{
			{Text: "Step 1", Done: true},
			{Text: "Step 2", Done: true},
			{Text: "Step 3", Done: false},
			{Text: "Step 4", Done: false},
		},
	}
	if g.goalProgress() != 50 {
		t.Fatalf("expected 50%% progress, got %d%%", g.goalProgress())
	}
}

func TestGoalEntry_GoalProgress_OneOfThree(t *testing.T) {
	g := goalEntry{
		Title:      "Goal",
		TargetDate: "2026-06-01",
		Milestones: []milestone{
			{Text: "A", Done: true},
			{Text: "B", Done: false},
			{Text: "C", Done: false},
		},
	}
	// 1/3 = 33% (integer division)
	if g.goalProgress() != 33 {
		t.Fatalf("expected 33%% progress (1/3), got %d%%", g.goalProgress())
	}
}

func TestGoalEntry_GoalProgress_TwoOfThree(t *testing.T) {
	g := goalEntry{
		Title:      "Goal",
		TargetDate: "2026-06-01",
		Milestones: []milestone{
			{Text: "A", Done: true},
			{Text: "B", Done: true},
			{Text: "C", Done: false},
		},
	}
	// 2/3 = 66% (integer division)
	if g.goalProgress() != 66 {
		t.Fatalf("expected 66%% progress (2/3), got %d%%", g.goalProgress())
	}
}

func TestGoalCreation_WithMilestones(t *testing.T) {
	ht := NewHabitTracker()
	ht.goals = []goalEntry{
		{
			Title:      "Ship v1.0",
			TargetDate: "2026-12-31",
			Milestones: []milestone{
				{Text: "Design spec", Done: true},
				{Text: "Implement core", Done: false},
				{Text: "Write tests", Done: false},
				{Text: "Documentation", Done: false},
			},
		},
	}

	if len(ht.goals) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(ht.goals))
	}
	if ht.goals[0].Title != "Ship v1.0" {
		t.Fatalf("expected title 'Ship v1.0', got %q", ht.goals[0].Title)
	}
	if len(ht.goals[0].Milestones) != 4 {
		t.Fatalf("expected 4 milestones, got %d", len(ht.goals[0].Milestones))
	}
	if ht.goals[0].goalProgress() != 25 {
		t.Fatalf("expected 25%% progress, got %d%%", ht.goals[0].goalProgress())
	}
}

// ── Progress Bar Calculation ─────────────────────────────────────

func TestHabitProgressBar_Zero(t *testing.T) {
	bar := habitProgressBar(0, 20)
	if bar == "" {
		t.Fatal("progress bar should not be empty")
	}
	// Should start with '[' and end with ']'
	if bar[0] != '[' {
		t.Error("progress bar should start with '['")
	}
	// The last character may be after ANSI codes, so check contains ']'
}

func TestHabitProgressBar_Full(t *testing.T) {
	bar := habitProgressBar(100, 20)
	if bar == "" {
		t.Fatal("progress bar should not be empty")
	}
}

func TestHabitProgressBar_Clamping(t *testing.T) {
	// Negative should clamp to 0
	barNeg := habitProgressBar(-10, 10)
	barZero := habitProgressBar(0, 10)
	if barNeg != barZero {
		t.Error("negative percentage should clamp to 0 and match 0%% bar")
	}

	// Over 100 should clamp to 100
	barOver := habitProgressBar(150, 10)
	barHundred := habitProgressBar(100, 10)
	if barOver != barHundred {
		t.Error("percentage over 100 should clamp to 100 and match 100%% bar")
	}
}

func TestHabitProgressBar_VariousWidths(t *testing.T) {
	for _, w := range []int{5, 10, 20, 50} {
		bar := habitProgressBar(50, w)
		if bar == "" {
			t.Errorf("progress bar with width %d should not be empty", w)
		}
	}
}

// ── Completion Rate Statistics ───────────────────────────────────

func TestCompletionRate_NoHabits(t *testing.T) {
	ht := NewHabitTracker()
	rate := ht.completionRate(7)
	if rate != 0 {
		t.Fatalf("expected 0%% rate with no habits, got %f", rate)
	}
}

func TestCompletionRate_AllCompletedWeek(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()
	todayTime, _ := time.Parse("2006-01-02", today)

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01"},
	}

	// Complete Exercise every day for last 7 days
	for i := 0; i < 7; i++ {
		d := todayTime.AddDate(0, 0, -i)
		ht.logs = append(ht.logs, habitLog{
			Date:      d.Format("2006-01-02"),
			Completed: []string{"Exercise"},
		})
	}

	rate := ht.completionRate(7)
	if rate != 100.0 {
		t.Fatalf("expected 100%% completion rate, got %f", rate)
	}
}

func TestCompletionRate_HalfCompleted(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()
	todayTime, _ := time.Parse("2006-01-02", today)

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01"},
		{Name: "Read", Created: "2026-03-01"},
	}

	// Only Exercise completed for all 7 days, Read never
	for i := 0; i < 7; i++ {
		d := todayTime.AddDate(0, 0, -i)
		ht.logs = append(ht.logs, habitLog{
			Date:      d.Format("2006-01-02"),
			Completed: []string{"Exercise"},
		})
	}

	rate := ht.completionRate(7)
	// 7 completions out of 14 possible (2 habits * 7 days) = 50%
	if rate != 50.0 {
		t.Fatalf("expected 50%% completion rate, got %f", rate)
	}
}

func TestCompletionRate_30Days(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()
	todayTime, _ := time.Parse("2006-01-02", today)

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01"},
	}

	// Complete only 15 out of 30 days
	for i := 0; i < 30; i += 2 {
		d := todayTime.AddDate(0, 0, -i)
		ht.logs = append(ht.logs, habitLog{
			Date:      d.Format("2006-01-02"),
			Completed: []string{"Exercise"},
		})
	}

	rate := ht.completionRate(30)
	// 15 completions out of 30 total = 50%
	if rate != 50.0 {
		t.Fatalf("expected 50%% completion rate, got %f", rate)
	}
}

// ── Best Day ─────────────────────────────────────────────────────

func TestBestDay_NoLogs(t *testing.T) {
	ht := NewHabitTracker()
	date, count := ht.bestDay()
	if date != "" {
		t.Fatalf("expected empty date, got %q", date)
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
}

func TestBestDay_MultipleDays(t *testing.T) {
	ht := NewHabitTracker()
	ht.logs = []habitLog{
		{Date: "2026-03-01", Completed: []string{"Exercise"}},
		{Date: "2026-03-02", Completed: []string{"Exercise", "Read", "Meditate"}},
		{Date: "2026-03-03", Completed: []string{"Exercise", "Read"}},
	}

	date, count := ht.bestDay()
	if date != "2026-03-02" {
		t.Fatalf("expected best day '2026-03-02', got %q", date)
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

// ── Longest Streak ───────────────────────────────────────────────

func TestLongestStreak_NoCompletions(t *testing.T) {
	ht := NewHabitTracker()
	streak := ht.longestStreak("Exercise")
	if streak != 0 {
		t.Fatalf("expected 0, got %d", streak)
	}
}

func TestLongestStreak_SingleDay(t *testing.T) {
	ht := NewHabitTracker()
	ht.logs = []habitLog{
		{Date: "2026-03-05", Completed: []string{"Exercise"}},
	}

	streak := ht.longestStreak("Exercise")
	if streak != 1 {
		t.Fatalf("expected 1, got %d", streak)
	}
}

func TestLongestStreak_ConsecutiveDays(t *testing.T) {
	ht := NewHabitTracker()
	ht.logs = []habitLog{
		{Date: "2026-03-01", Completed: []string{"Exercise"}},
		{Date: "2026-03-02", Completed: []string{"Exercise"}},
		{Date: "2026-03-03", Completed: []string{"Exercise"}},
		{Date: "2026-03-04", Completed: []string{"Exercise"}},
		{Date: "2026-03-05", Completed: []string{"Exercise"}},
	}

	streak := ht.longestStreak("Exercise")
	if streak != 5 {
		t.Fatalf("expected 5, got %d", streak)
	}
}

func TestLongestStreak_WithGap(t *testing.T) {
	ht := NewHabitTracker()
	ht.logs = []habitLog{
		{Date: "2026-03-01", Completed: []string{"Exercise"}},
		{Date: "2026-03-02", Completed: []string{"Exercise"}},
		{Date: "2026-03-03", Completed: []string{"Exercise"}},
		// Gap on 2026-03-04
		{Date: "2026-03-05", Completed: []string{"Exercise"}},
		{Date: "2026-03-06", Completed: []string{"Exercise"}},
	}

	streak := ht.longestStreak("Exercise")
	if streak != 3 {
		t.Fatalf("expected 3 (first run before gap), got %d", streak)
	}
}

func TestLongestStreak_OnlyOtherHabits(t *testing.T) {
	ht := NewHabitTracker()
	ht.logs = []habitLog{
		{Date: "2026-03-01", Completed: []string{"Read"}},
		{Date: "2026-03-02", Completed: []string{"Read"}},
	}

	streak := ht.longestStreak("Exercise")
	if streak != 0 {
		t.Fatalf("expected 0 for non-existent habit, got %d", streak)
	}
}

// ── Active Goals ─────────────────────────────────────────────────

func TestActiveGoals_NoGoals(t *testing.T) {
	ht := NewHabitTracker()
	active := ht.activeGoals()
	if len(active) != 0 {
		t.Fatalf("expected 0 active goals, got %d", len(active))
	}
}

func TestActiveGoals_MixedArchived(t *testing.T) {
	ht := NewHabitTracker()
	ht.goals = []goalEntry{
		{Title: "Goal A", Archived: false},
		{Title: "Goal B", Archived: true},
		{Title: "Goal C", Archived: false},
		{Title: "Goal D", Archived: true},
	}

	active := ht.activeGoals()
	if len(active) != 2 {
		t.Fatalf("expected 2 active goals, got %d", len(active))
	}
	if active[0] != 0 || active[1] != 2 {
		t.Fatalf("expected indices [0, 2], got %v", active)
	}
}

func TestActiveGoals_AllArchived(t *testing.T) {
	ht := NewHabitTracker()
	ht.goals = []goalEntry{
		{Title: "Goal A", Archived: true},
		{Title: "Goal B", Archived: true},
	}

	active := ht.activeGoals()
	if len(active) != 0 {
		t.Fatalf("expected 0 active goals, got %d", len(active))
	}
}

// ── Completed Goal Count ─────────────────────────────────────────

func TestCompletedGoalCount(t *testing.T) {
	ht := NewHabitTracker()
	ht.goals = []goalEntry{
		{Title: "A", Archived: true},
		{Title: "B", Archived: false},
		{Title: "C", Archived: true},
		{Title: "D", Archived: true},
	}

	if ht.completedGoalCount() != 3 {
		t.Fatalf("expected 3 completed goals, got %d", ht.completedGoalCount())
	}
}

func TestCompletedGoalCount_Empty(t *testing.T) {
	ht := NewHabitTracker()
	if ht.completedGoalCount() != 0 {
		t.Fatalf("expected 0, got %d", ht.completedGoalCount())
	}
}

// ── Last 14 Days Chart ───────────────────────────────────────────

func TestLast14DaysChart_Empty(t *testing.T) {
	ht := NewHabitTracker()
	counts := ht.last14DaysChart()
	if len(counts) != 14 {
		t.Fatalf("expected 14 entries, got %d", len(counts))
	}
	for i, c := range counts {
		if c != 0 {
			t.Errorf("day %d: expected 0, got %d", i, c)
		}
	}
}

func TestLast14DaysChart_WithData(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()
	todayTime, _ := time.Parse("2006-01-02", today)

	// Add completions for today and 3 days ago
	ht.logs = []habitLog{
		{Date: todayTime.Format("2006-01-02"), Completed: []string{"Exercise", "Read"}},
		{Date: todayTime.AddDate(0, 0, -3).Format("2006-01-02"), Completed: []string{"Exercise"}},
	}

	counts := ht.last14DaysChart()
	if len(counts) != 14 {
		t.Fatalf("expected 14 entries, got %d", len(counts))
	}
	// Last entry (index 13) = today
	if counts[13] != 2 {
		t.Errorf("today (index 13): expected 2, got %d", counts[13])
	}
	// Index 10 = 3 days ago
	if counts[10] != 1 {
		t.Errorf("3 days ago (index 10): expected 1, got %d", counts[10])
	}
}

// ── Max Cursor ───────────────────────────────────────────────────

func TestMaxCursor_HabitsTab(t *testing.T) {
	ht := NewHabitTracker()
	ht.tab = 0
	ht.habits = []habitEntry{
		{Name: "A"}, {Name: "B"}, {Name: "C"},
	}
	if ht.maxCursor() != 3 {
		t.Fatalf("expected maxCursor 3, got %d", ht.maxCursor())
	}
}

func TestMaxCursor_GoalsTab(t *testing.T) {
	ht := NewHabitTracker()
	ht.tab = 1
	ht.goals = []goalEntry{
		{Title: "A", Archived: false},
		{Title: "B", Archived: true},
		{Title: "C", Archived: false},
	}
	if ht.maxCursor() != 2 {
		t.Fatalf("expected maxCursor 2 (only active goals), got %d", ht.maxCursor())
	}
}

func TestMaxCursor_StatsTab(t *testing.T) {
	ht := NewHabitTracker()
	ht.tab = 2
	if ht.maxCursor() != 0 {
		t.Fatalf("expected maxCursor 0 for stats tab, got %d", ht.maxCursor())
	}
}

// ── Edge Cases ───────────────────────────────────────────────────

func TestEdgeCase_NoHabits_CompletionRate(t *testing.T) {
	ht := NewHabitTracker()
	rate7 := ht.completionRate(7)
	rate30 := ht.completionRate(30)
	if rate7 != 0 {
		t.Fatalf("expected 0 rate with no habits, got %f", rate7)
	}
	if rate30 != 0 {
		t.Fatalf("expected 0 rate with no habits, got %f", rate30)
	}
}

func TestEdgeCase_NoHabits_BestDay(t *testing.T) {
	ht := NewHabitTracker()
	date, count := ht.bestDay()
	if date != "" || count != 0 {
		t.Fatalf("expected empty best day with no data, got date=%q count=%d", date, count)
	}
}

func TestEdgeCase_AllCompletedToday(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01"},
		{Name: "Read", Created: "2026-03-01"},
		{Name: "Meditate", Created: "2026-03-01"},
	}
	ht.logs = []habitLog{
		{Date: today, Completed: []string{"Exercise", "Read", "Meditate"}},
	}

	// All should be completed
	for _, h := range ht.habits {
		if !ht.isTodayCompleted(h.Name) {
			t.Errorf("habit %q should be completed today", h.Name)
		}
	}

	// Completion rate for 1 day (today) should be 100%
	rate := ht.completionRate(1)
	if rate != 100.0 {
		t.Fatalf("expected 100%% rate for today, got %f", rate)
	}
}

func TestEdgeCase_StreakBroken_NotDoneToday(t *testing.T) {
	ht := NewHabitTracker()
	todayTime, _ := time.Parse("2006-01-02", todayStr())

	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 0},
	}

	// Completed the last 10 days but NOT today
	for i := 1; i <= 10; i++ {
		d := todayTime.AddDate(0, 0, -i)
		ht.logs = append(ht.logs, habitLog{
			Date:      d.Format("2006-01-02"),
			Completed: []string{"Exercise"},
		})
	}

	ht.recalcStreaks()

	// Streak starts from today, so if today is missing, streak = 0
	if ht.habits[0].Streak != 0 {
		t.Fatalf("expected streak 0 when today is not completed, got %d", ht.habits[0].Streak)
	}

	// But longest streak should still be 10
	longest := ht.longestStreak("Exercise")
	if longest != 10 {
		t.Fatalf("expected longest streak 10, got %d", longest)
	}
}

func TestEdgeCase_IsTodayCompleted_NoLogs(t *testing.T) {
	ht := NewHabitTracker()
	if ht.isTodayCompleted("AnyHabit") {
		t.Fatal("isTodayCompleted should return false with no logs")
	}
}

func TestEdgeCase_IsTodayCompleted_LogExistsButHabitNot(t *testing.T) {
	ht := NewHabitTracker()
	today := todayStr()
	ht.logs = []habitLog{
		{Date: today, Completed: []string{"OtherHabit"}},
	}

	if ht.isTodayCompleted("Exercise") {
		t.Fatal("isTodayCompleted should return false when habit not in today's log")
	}
}

// ── habitPadRight ────────────────────────────────────────────────

func TestHabitPadRight(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"hello", 10, "hello     "},
		{"hello", 5, "hello"},
		{"hello", 3, "hello"}, // no truncation, just returns as-is
		{"", 5, "     "},
		{"x", 1, "x"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := habitPadRight(tc.input, tc.width)
			if got != tc.expected {
				t.Errorf("habitPadRight(%q, %d) = %q, want %q", tc.input, tc.width, got, tc.expected)
			}
		})
	}
}

// ── Perform Delete ───────────────────────────────────────────────

func TestPerformDelete_Habit(t *testing.T) {
	ht := NewHabitTracker()
	ht.vaultRoot = t.TempDir()
	ht.tab = 0
	ht.habits = []habitEntry{
		{Name: "A", Created: "2026-03-01"},
		{Name: "B", Created: "2026-03-02"},
		{Name: "C", Created: "2026-03-03"},
	}
	ht.cursor = 1

	ht.performDelete()

	if len(ht.habits) != 2 {
		t.Fatalf("expected 2 habits after delete, got %d", len(ht.habits))
	}
	if ht.habits[0].Name != "A" || ht.habits[1].Name != "C" {
		t.Fatalf("expected A,C remaining, got %q,%q", ht.habits[0].Name, ht.habits[1].Name)
	}
}

func TestPerformDelete_LastHabit(t *testing.T) {
	ht := NewHabitTracker()
	ht.vaultRoot = t.TempDir()
	ht.tab = 0
	ht.habits = []habitEntry{
		{Name: "A", Created: "2026-03-01"},
		{Name: "B", Created: "2026-03-02"},
	}
	ht.cursor = 1 // pointing to last item

	ht.performDelete()

	if len(ht.habits) != 1 {
		t.Fatalf("expected 1 habit after delete, got %d", len(ht.habits))
	}
	// Cursor should adjust downward
	if ht.cursor != 0 {
		t.Fatalf("cursor should adjust to 0 after deleting last item, got %d", ht.cursor)
	}
}

func TestPerformDelete_ArchiveGoal(t *testing.T) {
	ht := NewHabitTracker()
	ht.vaultRoot = t.TempDir()
	ht.tab = 1
	ht.goals = []goalEntry{
		{Title: "Goal A", Archived: false},
		{Title: "Goal B", Archived: false},
	}
	ht.goalExpanded = 0

	ht.performDelete()

	if !ht.goals[0].Archived {
		t.Fatal("Goal A should be archived after delete")
	}
	if ht.goalExpanded != -1 {
		t.Fatalf("goalExpanded should be -1 after archive, got %d", ht.goalExpanded)
	}
}

// ── File-based Load/Save round-trip ──────────────────────────────

func TestSaveAndLoadHabits_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	ht := NewHabitTracker()
	ht.vaultRoot = tmpDir
	ht.habits = []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 5},
		{Name: "Read", Created: "2026-03-02", Streak: 3},
	}
	ht.logs = []habitLog{
		{Date: "2026-03-08", Completed: []string{"Exercise", "Read"}},
		{Date: "2026-03-07", Completed: []string{"Exercise"}},
	}

	ht.saveHabits()

	// Load into a fresh tracker
	ht2 := NewHabitTracker()
	ht2.vaultRoot = tmpDir
	ht2.loadHabits()

	if len(ht2.habits) != 2 {
		t.Fatalf("expected 2 habits after reload, got %d", len(ht2.habits))
	}
	if ht2.habits[0].Name != "Exercise" {
		t.Fatalf("expected first habit 'Exercise', got %q", ht2.habits[0].Name)
	}
	if ht2.habits[1].Name != "Read" {
		t.Fatalf("expected second habit 'Read', got %q", ht2.habits[1].Name)
	}

	if len(ht2.logs) != 2 {
		t.Fatalf("expected 2 log entries after reload, got %d", len(ht2.logs))
	}
	if ht2.logs[0].Date != "2026-03-08" {
		t.Fatalf("expected first log date '2026-03-08', got %q", ht2.logs[0].Date)
	}
	if len(ht2.logs[0].Completed) != 2 {
		t.Fatalf("expected 2 completions in first log, got %d", len(ht2.logs[0].Completed))
	}
}

func TestSaveAndLoadGoals_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()

	ht := NewHabitTracker()
	ht.vaultRoot = tmpDir
	ht.goals = []goalEntry{
		{
			Title:      "Learn Go",
			TargetDate: "2026-12-31",
			Milestones: []milestone{
				{Text: "Read tutorial", Done: true},
				{Text: "Build project", Done: false},
			},
			Archived: false,
		},
		{
			Title:      "Old Goal",
			TargetDate: "2025-06-01",
			Milestones: []milestone{
				{Text: "Done task", Done: true},
			},
			Archived: true,
		},
	}

	ht.saveGoals()

	// Load into a fresh tracker
	ht2 := NewHabitTracker()
	ht2.vaultRoot = tmpDir
	ht2.loadGoals()

	if len(ht2.goals) != 2 {
		t.Fatalf("expected 2 goals after reload, got %d", len(ht2.goals))
	}

	g1 := ht2.goals[0]
	if g1.Title != "Learn Go" {
		t.Fatalf("expected first goal 'Learn Go', got %q", g1.Title)
	}
	if g1.Archived {
		t.Fatal("first goal should not be archived")
	}
	if len(g1.Milestones) != 2 {
		t.Fatalf("expected 2 milestones, got %d", len(g1.Milestones))
	}
	if !g1.Milestones[0].Done {
		t.Fatal("first milestone should be done")
	}
	if g1.Milestones[1].Done {
		t.Fatal("second milestone should not be done")
	}

	g2 := ht2.goals[1]
	if g2.Title != "Old Goal" {
		t.Fatalf("expected second goal 'Old Goal', got %q", g2.Title)
	}
	if !g2.Archived {
		t.Fatal("second goal should be archived")
	}
}

func TestLoadHabits_NoFile(t *testing.T) {
	ht := NewHabitTracker()
	ht.vaultRoot = t.TempDir()
	ht.loadHabits()

	if ht.habits != nil {
		t.Fatal("habits should be nil when file does not exist")
	}
	if ht.logs != nil {
		t.Fatal("logs should be nil when file does not exist")
	}
}

func TestLoadGoals_NoFile(t *testing.T) {
	ht := NewHabitTracker()
	ht.vaultRoot = t.TempDir()
	ht.loadGoals()

	if ht.goals != nil {
		t.Fatal("goals should be nil when file does not exist")
	}
}
