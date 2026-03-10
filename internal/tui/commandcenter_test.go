package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Initialization
// ---------------------------------------------------------------------------

func TestCommandCenterNew(t *testing.T) {
	cc := NewCommandCenter()
	if cc.active {
		t.Error("expected new command center to be inactive")
	}
	if cc.section != 0 {
		t.Errorf("expected section 0, got %d", cc.section)
	}
	if cc.scroll != 0 {
		t.Errorf("expected scroll 0, got %d", cc.scroll)
	}
	if cc.nowTask != nil {
		t.Error("expected nil nowTask on init")
	}
	if len(cc.nextTasks) != 0 {
		t.Errorf("expected empty nextTasks, got %d", len(cc.nextTasks))
	}
	if len(cc.schedule) != 0 {
		t.Errorf("expected empty schedule, got %d", len(cc.schedule))
	}
	if len(cc.projects) != 0 {
		t.Errorf("expected empty projects, got %d", len(cc.projects))
	}
	if len(cc.habits) != 0 {
		t.Errorf("expected empty habits, got %d", len(cc.habits))
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive
// ---------------------------------------------------------------------------

func TestCommandCenterOpenCloseIsActive(t *testing.T) {
	cc := NewCommandCenter()
	if cc.IsActive() {
		t.Error("expected IsActive false before Open")
	}

	cc.Open()
	if !cc.IsActive() {
		t.Error("expected IsActive true after Open")
	}
	if cc.section != 0 {
		t.Errorf("expected section reset to 0, got %d", cc.section)
	}
	if cc.scroll != 0 {
		t.Errorf("expected scroll reset to 0, got %d", cc.scroll)
	}

	cc.Close()
	if cc.IsActive() {
		t.Error("expected IsActive false after Close")
	}
}

func TestCommandCenterOpenResetsState(t *testing.T) {
	cc := NewCommandCenter()
	cc.startPomodoro = true
	cc.completedTask = &Task{Text: "old"}
	cc.selectedProject = "proj"
	cc.toggledHabit = "habit"
	cc.section = 2
	cc.scroll = 5

	cc.Open()

	if cc.startPomodoro {
		t.Error("Open should reset startPomodoro")
	}
	if cc.completedTask != nil {
		t.Error("Open should reset completedTask")
	}
	if cc.selectedProject != "" {
		t.Error("Open should reset selectedProject")
	}
	if cc.toggledHabit != "" {
		t.Error("Open should reset toggledHabit")
	}
	if cc.section != 0 {
		t.Errorf("Open should reset section to 0, got %d", cc.section)
	}
	if cc.scroll != 0 {
		t.Errorf("Open should reset scroll to 0, got %d", cc.scroll)
	}
}

// ---------------------------------------------------------------------------
// SetSize
// ---------------------------------------------------------------------------

func TestCommandCenterSetSize(t *testing.T) {
	cc := NewCommandCenter()
	cc.SetSize(120, 40)
	if cc.width != 120 {
		t.Errorf("expected width 120, got %d", cc.width)
	}
	if cc.height != 40 {
		t.Errorf("expected height 40, got %d", cc.height)
	}
}

// ---------------------------------------------------------------------------
// LoadData — empty inputs
// ---------------------------------------------------------------------------

func TestCommandCenterLoadDataEmpty(t *testing.T) {
	cc := NewCommandCenter()
	cc.LoadData(nil, nil, nil, nil, nil)

	if cc.nowTask != nil {
		t.Error("expected nil nowTask with empty tasks")
	}
	if cc.nextTasks != nil {
		t.Error("expected nil nextTasks with empty tasks")
	}
	if len(cc.schedule) != 0 {
		t.Errorf("expected empty schedule, got %d", len(cc.schedule))
	}
	if len(cc.projects) != 0 {
		t.Errorf("expected empty projects, got %d", len(cc.projects))
	}
	if len(cc.habits) != 0 {
		t.Errorf("expected empty habits, got %d", len(cc.habits))
	}
}

// ---------------------------------------------------------------------------
// LoadData — single task
// ---------------------------------------------------------------------------

func TestCommandCenterLoadDataSingleTask(t *testing.T) {
	cc := NewCommandCenter()
	tasks := []Task{
		{Text: "write tests", Done: false, Priority: 3},
	}
	cc.LoadData(tasks, nil, nil, nil, nil)

	if cc.nowTask == nil {
		t.Fatal("expected nowTask to be set")
	}
	if cc.nowTask.Text != "write tests" {
		t.Errorf("expected nowTask text 'write tests', got %q", cc.nowTask.Text)
	}
	if len(cc.nextTasks) != 0 {
		t.Errorf("expected empty nextTasks with single task, got %d", len(cc.nextTasks))
	}
}

// ---------------------------------------------------------------------------
// LoadData — multiple tasks with priorities
// ---------------------------------------------------------------------------

func TestCommandCenterLoadDataMultiplePriorities(t *testing.T) {
	cc := NewCommandCenter()
	tasks := []Task{
		{Text: "low prio", Done: false, Priority: 1},
		{Text: "highest prio", Done: false, Priority: 4},
		{Text: "medium prio", Done: false, Priority: 2},
		{Text: "high prio", Done: false, Priority: 3},
		{Text: "also high", Done: false, Priority: 3},
	}
	cc.LoadData(tasks, nil, nil, nil, nil)

	if cc.nowTask == nil {
		t.Fatal("expected nowTask to be set")
	}
	if cc.nowTask.Text != "highest prio" {
		t.Errorf("expected nowTask to be highest priority, got %q", cc.nowTask.Text)
	}
	// nextTasks should be the next 3 by priority score
	if len(cc.nextTasks) != 3 {
		t.Fatalf("expected 3 nextTasks, got %d", len(cc.nextTasks))
	}
}

// ---------------------------------------------------------------------------
// LoadData — done tasks are excluded
// ---------------------------------------------------------------------------

func TestCommandCenterLoadDataExcludesDone(t *testing.T) {
	cc := NewCommandCenter()
	tasks := []Task{
		{Text: "done task", Done: true, Priority: 4},
		{Text: "pending task", Done: false, Priority: 1},
	}
	cc.LoadData(tasks, nil, nil, nil, nil)

	if cc.nowTask == nil {
		t.Fatal("expected nowTask to be set")
	}
	if cc.nowTask.Text != "pending task" {
		t.Errorf("expected nowTask to be pending task, got %q", cc.nowTask.Text)
	}
}

// ---------------------------------------------------------------------------
// LoadData — tasks with due dates
// ---------------------------------------------------------------------------

func TestCommandCenterLoadDataDueDates(t *testing.T) {
	cc := NewCommandCenter()
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	nextWeek := time.Now().AddDate(0, 0, 7).Format("2006-01-02")

	tasks := []Task{
		{Text: "future task", Done: false, Priority: 2, DueDate: nextWeek},
		{Text: "overdue task", Done: false, Priority: 2, DueDate: yesterday},
		{Text: "today task", Done: false, Priority: 2, DueDate: today},
	}
	cc.LoadData(tasks, nil, nil, nil, nil)

	if cc.nowTask == nil {
		t.Fatal("expected nowTask to be set")
	}
	// Overdue gets +50, today gets +30, future gets 0 — all have priority 2 (score 50 base)
	if cc.nowTask.Text != "overdue task" {
		t.Errorf("expected overdue task as nowTask, got %q", cc.nowTask.Text)
	}
}

// ---------------------------------------------------------------------------
// LoadData — projects with task counts
// ---------------------------------------------------------------------------

func TestCommandCenterLoadDataProjects(t *testing.T) {
	cc := NewCommandCenter()

	projects := []Project{
		{
			Name:       "Alpha",
			Status:     "active",
			Folder:     "projects/alpha/",
			TaskFilter: "",
		},
		{
			Name:   "Inactive",
			Status: "archived",
		},
	}
	tasks := []Task{
		{Text: "task 1", Done: false, NotePath: "projects/alpha/notes.md"},
		{Text: "task 2", Done: true, NotePath: "projects/alpha/notes.md"},
		{Text: "task 3", Done: false, NotePath: "projects/beta/notes.md"},
	}
	cc.LoadData(tasks, projects, nil, nil, nil)

	if len(cc.projects) != 1 {
		t.Fatalf("expected 1 active project, got %d", len(cc.projects))
	}
	proj := cc.projects[0]
	if proj.Name != "Alpha" {
		t.Errorf("expected project 'Alpha', got %q", proj.Name)
	}
	if proj.TasksTotal != 2 {
		t.Errorf("expected 2 total tasks for Alpha, got %d", proj.TasksTotal)
	}
	if proj.TasksDone != 1 {
		t.Errorf("expected 1 done task for Alpha, got %d", proj.TasksDone)
	}
	expectedProgress := 0.5
	if proj.Progress != expectedProgress {
		t.Errorf("expected progress %f, got %f", expectedProgress, proj.Progress)
	}
}

func TestCommandCenterLoadDataProjectTagMatching(t *testing.T) {
	cc := NewCommandCenter()

	projects := []Project{
		{
			Name:   "Tagged",
			Status: "active",
			Tags:   []string{"work"},
		},
	}
	tasks := []Task{
		{Text: "task with #work tag", Done: false},
		{Text: "unrelated task", Done: false},
	}
	cc.LoadData(tasks, projects, nil, nil, nil)

	if len(cc.projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(cc.projects))
	}
	if cc.projects[0].TasksTotal != 1 {
		t.Errorf("expected 1 task matching tag, got %d", cc.projects[0].TasksTotal)
	}
}

// ---------------------------------------------------------------------------
// LoadData — habits with completion
// ---------------------------------------------------------------------------

func TestCommandCenterLoadDataHabits(t *testing.T) {
	cc := NewCommandCenter()
	today := time.Now().Format("2006-01-02")

	habits := []habitEntry{
		{Name: "Meditate", Streak: 5},
		{Name: "Exercise", Streak: 2},
	}
	logs := []habitLog{
		{Date: today, Completed: []string{"Meditate"}},
	}
	cc.LoadData(nil, nil, habits, logs, nil)

	if len(cc.habits) != 2 {
		t.Fatalf("expected 2 habits, got %d", len(cc.habits))
	}
	// Meditate should be marked done, Exercise should not
	var meditateStatus, exerciseStatus *habitStatus
	for i := range cc.habits {
		switch cc.habits[i].Name {
		case "Meditate":
			meditateStatus = &cc.habits[i]
		case "Exercise":
			exerciseStatus = &cc.habits[i]
		}
	}
	if meditateStatus == nil || !meditateStatus.Done {
		t.Error("expected Meditate to be marked done")
	}
	if exerciseStatus == nil || exerciseStatus.Done {
		t.Error("expected Exercise to be not done")
	}
	if meditateStatus != nil && meditateStatus.Streak != 5 {
		t.Errorf("expected Meditate streak 5, got %d", meditateStatus.Streak)
	}
}

// ---------------------------------------------------------------------------
// LoadData — schedule from events and scheduled tasks
// ---------------------------------------------------------------------------

func TestCommandCenterLoadDataSchedule(t *testing.T) {
	cc := NewCommandCenter()

	tasks := []Task{
		{Text: "scheduled task", Done: false, Priority: 2, ScheduledTime: "09:00-10:00"},
	}
	events := []PlannerEvent{
		{Title: "Team meeting", Time: "14:00", Duration: 60},
	}
	cc.LoadData(tasks, nil, nil, nil, events)

	if len(cc.schedule) != 2 {
		t.Fatalf("expected 2 schedule items, got %d", len(cc.schedule))
	}
	// Schedule should be sorted by time
	if cc.schedule[0].Time != "09:00-10:00" {
		t.Errorf("expected first slot at 09:00-10:00, got %q", cc.schedule[0].Time)
	}
	if cc.schedule[1].Time != "14:00-15:00" {
		t.Errorf("expected second slot at 14:00-15:00, got %q", cc.schedule[1].Time)
	}
	if cc.schedule[0].Type != "task" {
		t.Errorf("expected first slot type 'task', got %q", cc.schedule[0].Type)
	}
	if cc.schedule[1].Type != "event" {
		t.Errorf("expected second slot type 'event', got %q", cc.schedule[1].Type)
	}
}

func TestCommandCenterLoadDataEventAllDay(t *testing.T) {
	cc := NewCommandCenter()

	events := []PlannerEvent{
		{Title: "Holiday", Time: "", Duration: 0},
	}
	cc.LoadData(nil, nil, nil, nil, events)

	if len(cc.schedule) != 1 {
		t.Fatalf("expected 1 schedule item, got %d", len(cc.schedule))
	}
	if cc.schedule[0].Time != "all day" {
		t.Errorf("expected 'all day', got %q", cc.schedule[0].Time)
	}
}

// ---------------------------------------------------------------------------
// priorityScore
// ---------------------------------------------------------------------------

func TestCommandCenterPriorityScore(t *testing.T) {
	cc := NewCommandCenter()
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	nextWeek := time.Now().AddDate(0, 0, 7).Format("2006-01-02")

	tests := []struct {
		name     string
		task     Task
		minScore int
		maxScore int
	}{
		{
			name:     "priority 4 no due",
			task:     Task{Priority: 4},
			minScore: 100,
			maxScore: 100,
		},
		{
			name:     "priority 3 no due",
			task:     Task{Priority: 3},
			minScore: 75,
			maxScore: 75,
		},
		{
			name:     "priority 2 no due",
			task:     Task{Priority: 2},
			minScore: 50,
			maxScore: 50,
		},
		{
			name:     "priority 1 no due",
			task:     Task{Priority: 1},
			minScore: 25,
			maxScore: 25,
		},
		{
			name:     "priority 0 no due",
			task:     Task{Priority: 0},
			minScore: 0,
			maxScore: 0,
		},
		{
			name:     "overdue adds 50",
			task:     Task{Priority: 2, DueDate: yesterday},
			minScore: 100,
			maxScore: 100,
		},
		{
			name:     "due today adds 30",
			task:     Task{Priority: 2, DueDate: today},
			minScore: 80,
			maxScore: 80,
		},
		{
			name:     "due tomorrow adds 10",
			task:     Task{Priority: 2, DueDate: tomorrow},
			minScore: 60,
			maxScore: 60,
		},
		{
			name:     "due next week adds 0",
			task:     Task{Priority: 2, DueDate: nextWeek},
			minScore: 50,
			maxScore: 50,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			score := cc.priorityScore(tc.task)
			if score < tc.minScore || score > tc.maxScore {
				t.Errorf("expected score in [%d, %d], got %d", tc.minScore, tc.maxScore, score)
			}
		})
	}
}

func TestCommandCenterPriorityScoreOverdueHigherThanToday(t *testing.T) {
	cc := NewCommandCenter()
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	overdue := cc.priorityScore(Task{Priority: 2, DueDate: yesterday})
	dueToday := cc.priorityScore(Task{Priority: 2, DueDate: today})

	if overdue <= dueToday {
		t.Errorf("expected overdue score (%d) > due today score (%d)", overdue, dueToday)
	}
}

func TestCommandCenterPriorityScoreTodayHigherThanTomorrow(t *testing.T) {
	cc := NewCommandCenter()
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	dueToday := cc.priorityScore(Task{Priority: 2, DueDate: today})
	dueTomorrow := cc.priorityScore(Task{Priority: 2, DueDate: tomorrow})

	if dueToday <= dueTomorrow {
		t.Errorf("expected today score (%d) > tomorrow score (%d)", dueToday, dueTomorrow)
	}
}

// ---------------------------------------------------------------------------
// Section navigation — Tab / Shift+Tab
// ---------------------------------------------------------------------------

func TestCommandCenterSectionTabCycle(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()

	expected := []int{1, 2, 3, 0, 1}
	for i, want := range expected {
		cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyTab})
		if cc.section != want {
			t.Errorf("step %d: expected section %d after Tab, got %d", i, want, cc.section)
		}
	}
}

func TestCommandCenterSectionShiftTabCycle(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()

	expected := []int{3, 2, 1, 0, 3}
	for i, want := range expected {
		cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
		if cc.section != want {
			t.Errorf("step %d: expected section %d after ShiftTab, got %d", i, want, cc.section)
		}
	}
}

func TestCommandCenterTabResetsScroll(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.scroll = 5
	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyTab})
	if cc.scroll != 0 {
		t.Errorf("expected scroll reset to 0 on Tab, got %d", cc.scroll)
	}
}

// ---------------------------------------------------------------------------
// Scroll within sections
// ---------------------------------------------------------------------------

func TestCommandCenterScrollUpDown(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()

	// Section 0 (now) — maxScroll is 0, can't scroll
	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cc.scroll != 0 {
		t.Errorf("section 0 should not scroll down, got %d", cc.scroll)
	}

	// Switch to schedule section (1) with items
	cc.section = 1
	cc.schedule = []timeSlot{
		{Time: "09:00", Task: "A"},
		{Time: "10:00", Task: "B"},
		{Time: "11:00", Task: "C"},
	}
	cc.scroll = 0

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cc.scroll != 1 {
		t.Errorf("expected scroll 1, got %d", cc.scroll)
	}

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cc.scroll != 2 {
		t.Errorf("expected scroll 2, got %d", cc.scroll)
	}

	// Can't scroll past maxScroll (n-1=2)
	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyDown})
	if cc.scroll != 2 {
		t.Errorf("expected scroll clamped at 2, got %d", cc.scroll)
	}

	// Scroll up
	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cc.scroll != 1 {
		t.Errorf("expected scroll 1 after up, got %d", cc.scroll)
	}

	// Can't scroll above 0
	cc.scroll = 0
	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyUp})
	if cc.scroll != 0 {
		t.Errorf("expected scroll clamped at 0, got %d", cc.scroll)
	}
}

func TestCommandCenterScrollKJ(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.section = 1
	cc.schedule = []timeSlot{
		{Time: "09:00", Task: "A"},
		{Time: "10:00", Task: "B"},
	}
	cc.scroll = 0

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if cc.scroll != 1 {
		t.Errorf("expected scroll 1 after j, got %d", cc.scroll)
	}

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if cc.scroll != 0 {
		t.Errorf("expected scroll 0 after k, got %d", cc.scroll)
	}
}

// ---------------------------------------------------------------------------
// Consumed-once getters
// ---------------------------------------------------------------------------

func TestCommandCenterShouldStartPomodoro(t *testing.T) {
	cc := NewCommandCenter()
	// Not set — should return false
	if cc.ShouldStartPomodoro() {
		t.Error("expected false when not set")
	}
	cc.startPomodoro = true
	if !cc.ShouldStartPomodoro() {
		t.Error("expected true on first call")
	}
	if cc.ShouldStartPomodoro() {
		t.Error("expected false on second call (consumed)")
	}
}

func TestCommandCenterCompletedTask(t *testing.T) {
	cc := NewCommandCenter()
	if cc.CompletedTask() != nil {
		t.Error("expected nil when not set")
	}
	task := &Task{Text: "test task"}
	cc.completedTask = task
	result := cc.CompletedTask()
	if result == nil || result.Text != "test task" {
		t.Error("expected task on first call")
	}
	if cc.CompletedTask() != nil {
		t.Error("expected nil on second call (consumed)")
	}
}

func TestCommandCenterSelectedProject(t *testing.T) {
	cc := NewCommandCenter()
	if cc.SelectedProject() != "" {
		t.Error("expected empty when not set")
	}
	cc.selectedProject = "Alpha"
	result := cc.SelectedProject()
	if result != "Alpha" {
		t.Errorf("expected 'Alpha', got %q", result)
	}
	if cc.SelectedProject() != "" {
		t.Error("expected empty on second call (consumed)")
	}
}

func TestCommandCenterToggledHabit(t *testing.T) {
	cc := NewCommandCenter()
	if cc.ToggledHabit() != "" {
		t.Error("expected empty when not set")
	}
	cc.toggledHabit = "Meditate"
	result := cc.ToggledHabit()
	if result != "Meditate" {
		t.Errorf("expected 'Meditate', got %q", result)
	}
	if cc.ToggledHabit() != "" {
		t.Error("expected empty on second call (consumed)")
	}
}

// ---------------------------------------------------------------------------
// Enter on NOW task — completes task and advances
// ---------------------------------------------------------------------------

func TestCommandCenterEnterCompleteNowTask(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.nowTask = &Task{Text: "current task", Priority: 4}
	cc.nextTasks = []Task{
		{Text: "next 1", Priority: 3},
		{Text: "next 2", Priority: 2},
	}

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// completedTask should be set
	completed := cc.CompletedTask()
	if completed == nil {
		t.Fatal("expected completedTask to be set")
	}
	if completed.Text != "current task" {
		t.Errorf("expected completed 'current task', got %q", completed.Text)
	}

	// nowTask should advance to next1
	if cc.nowTask == nil {
		t.Fatal("expected nowTask to advance")
	}
	if cc.nowTask.Text != "next 1" {
		t.Errorf("expected nowTask 'next 1', got %q", cc.nowTask.Text)
	}

	// nextTasks should have 1 remaining
	if len(cc.nextTasks) != 1 {
		t.Fatalf("expected 1 remaining nextTask, got %d", len(cc.nextTasks))
	}
	if cc.nextTasks[0].Text != "next 2" {
		t.Errorf("expected nextTask 'next 2', got %q", cc.nextTasks[0].Text)
	}
}

func TestCommandCenterEnterCompleteLastTask(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.nowTask = &Task{Text: "only task"}
	cc.nextTasks = nil

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyEnter})

	completed := cc.CompletedTask()
	if completed == nil || completed.Text != "only task" {
		t.Error("expected completed 'only task'")
	}
	if cc.nowTask != nil {
		t.Error("expected nil nowTask when no next tasks")
	}
}

func TestCommandCenterEnterNoTaskNoPanic(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.nowTask = nil

	// Should not panic
	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cc.CompletedTask() != nil {
		t.Error("expected no completedTask when nowTask was nil")
	}
}

// ---------------------------------------------------------------------------
// Space on NOW task — sets pomodoro
// ---------------------------------------------------------------------------

func TestCommandCenterSpaceStartPomodoro(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.nowTask = &Task{Text: "focus task"}

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeySpace})

	if !cc.ShouldStartPomodoro() {
		t.Error("expected startPomodoro flag to be set")
	}
}

func TestCommandCenterSpaceNoTaskNoPomodoro(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.nowTask = nil

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeySpace})

	if cc.ShouldStartPomodoro() {
		t.Error("expected no pomodoro when nowTask is nil")
	}
}

func TestCommandCenterSpaceNonZeroSectionNoPomodoro(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.nowTask = &Task{Text: "some task"}
	cc.section = 2

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeySpace})

	if cc.ShouldStartPomodoro() {
		t.Error("expected no pomodoro when section != 0")
	}
}

// ---------------------------------------------------------------------------
// Enter on projects section — select project
// ---------------------------------------------------------------------------

func TestCommandCenterEnterSelectProject(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.section = 2
	cc.projects = []projectSummary{
		{Name: "Alpha"},
		{Name: "Beta"},
	}
	cc.scroll = 1

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyEnter})

	proj := cc.SelectedProject()
	if proj != "Beta" {
		t.Errorf("expected selected project 'Beta', got %q", proj)
	}
}

func TestCommandCenterEnterSelectProjectClampsScroll(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.section = 2
	cc.projects = []projectSummary{
		{Name: "Alpha"},
	}
	cc.scroll = 5 // Beyond bounds

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyEnter})

	proj := cc.SelectedProject()
	if proj != "Alpha" {
		t.Errorf("expected selected project 'Alpha' (clamped), got %q", proj)
	}
}

// ---------------------------------------------------------------------------
// Enter on habits section — toggle habit
// ---------------------------------------------------------------------------

func TestCommandCenterEnterToggleHabit(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.section = 3
	cc.habits = []habitStatus{
		{Name: "Meditate", Done: false},
		{Name: "Exercise", Done: false},
	}
	cc.scroll = 0

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyEnter})

	habit := cc.ToggledHabit()
	if habit != "Meditate" {
		t.Errorf("expected toggled habit 'Meditate', got %q", habit)
	}
	if !cc.habits[0].Done {
		t.Error("expected habit Done to be toggled to true")
	}
}

func TestCommandCenterEnterToggleHabitDoneToUndone(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.section = 3
	cc.habits = []habitStatus{
		{Name: "Meditate", Done: true},
	}
	cc.scroll = 0

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if cc.habits[0].Done {
		t.Error("expected habit Done toggled from true to false")
	}
}

// ---------------------------------------------------------------------------
// Esc closes overlay
// ---------------------------------------------------------------------------

func TestCommandCenterEscCloses(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if cc.IsActive() {
		t.Error("expected Esc to close overlay")
	}
}

func TestCommandCenterQCloses(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()

	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	if cc.IsActive() {
		t.Error("expected 'q' to close overlay")
	}
}

// ---------------------------------------------------------------------------
// Update when inactive does nothing
// ---------------------------------------------------------------------------

func TestCommandCenterUpdateInactiveNoop(t *testing.T) {
	cc := NewCommandCenter()
	// Not opened — should be no-op
	cc, _ = cc.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cc.IsActive() {
		t.Error("expected inactive command center to stay inactive")
	}
}

// ---------------------------------------------------------------------------
// maxScroll
// ---------------------------------------------------------------------------

func TestCommandCenterMaxScroll(t *testing.T) {
	cc := NewCommandCenter()

	// Section 0 always 0
	cc.section = 0
	if cc.maxScroll() != 0 {
		t.Errorf("section 0 maxScroll expected 0, got %d", cc.maxScroll())
	}

	// Section 1 — empty
	cc.section = 1
	cc.schedule = nil
	if cc.maxScroll() != 0 {
		t.Errorf("empty schedule maxScroll expected 0, got %d", cc.maxScroll())
	}

	// Section 1 — 3 items
	cc.schedule = []timeSlot{{}, {}, {}}
	if cc.maxScroll() != 2 {
		t.Errorf("schedule maxScroll expected 2, got %d", cc.maxScroll())
	}

	// Section 2 — empty
	cc.section = 2
	cc.projects = nil
	if cc.maxScroll() != 0 {
		t.Errorf("empty projects maxScroll expected 0, got %d", cc.maxScroll())
	}

	// Section 2 — 2 items
	cc.projects = []projectSummary{{}, {}}
	if cc.maxScroll() != 1 {
		t.Errorf("projects maxScroll expected 1, got %d", cc.maxScroll())
	}

	// Section 3 — empty
	cc.section = 3
	cc.habits = nil
	if cc.maxScroll() != 0 {
		t.Errorf("empty habits maxScroll expected 0, got %d", cc.maxScroll())
	}

	// Section 3 — 4 items
	cc.habits = []habitStatus{{}, {}, {}, {}}
	if cc.maxScroll() != 3 {
		t.Errorf("habits maxScroll expected 3, got %d", cc.maxScroll())
	}
}

// ---------------------------------------------------------------------------
// View smoke test — should not panic
// ---------------------------------------------------------------------------

func TestCommandCenterViewSmoke(t *testing.T) {
	cc := NewCommandCenter()
	cc.Open()
	cc.SetSize(120, 40)

	tasks := []Task{
		{Text: "important task", Priority: 4, DueDate: time.Now().Format("2006-01-02")},
		{Text: "another task", Priority: 2},
	}
	cc.LoadData(tasks, nil, nil, nil, nil)

	// Should not panic for any section
	for s := 0; s < 4; s++ {
		cc.section = s
		output := cc.View()
		if output == "" {
			t.Errorf("expected non-empty view for section %d", s)
		}
	}
}

// ---------------------------------------------------------------------------
// ccPadRight
// ---------------------------------------------------------------------------

func TestCcPadRight(t *testing.T) {
	if got := ccPadRight("hi", 5); got != "hi   " {
		t.Errorf("expected 'hi   ', got %q", got)
	}
	if got := ccPadRight("hello", 5); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
	if got := ccPadRight("toolong", 3); got != "toolong" {
		t.Errorf("expected 'toolong' unchanged, got %q", got)
	}
}
