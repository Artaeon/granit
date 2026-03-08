package tui

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewAIScheduler defaults
// ---------------------------------------------------------------------------

func TestAISchedulerNewDefaults(t *testing.T) {
	as := NewAIScheduler()

	t.Run("not active by default", func(t *testing.T) {
		if as.IsActive() {
			t.Error("new scheduler should not be active")
		}
	})

	t.Run("default work start", func(t *testing.T) {
		if as.prefs.WorkStart != 8 {
			t.Errorf("expected WorkStart=8, got %d", as.prefs.WorkStart)
		}
	})

	t.Run("default work end", func(t *testing.T) {
		if as.prefs.WorkEnd != 18 {
			t.Errorf("expected WorkEnd=18, got %d", as.prefs.WorkEnd)
		}
	})

	t.Run("default lunch start", func(t *testing.T) {
		if as.prefs.LunchStart != 12 {
			t.Errorf("expected LunchStart=12, got %d", as.prefs.LunchStart)
		}
	})

	t.Run("default lunch duration", func(t *testing.T) {
		if as.prefs.LunchDuration != 60 {
			t.Errorf("expected LunchDuration=60, got %d", as.prefs.LunchDuration)
		}
	})

	t.Run("default focus block min", func(t *testing.T) {
		if as.prefs.FocusBlockMin != 25 {
			t.Errorf("expected FocusBlockMin=25, got %d", as.prefs.FocusBlockMin)
		}
	})

	t.Run("default break every", func(t *testing.T) {
		if as.prefs.BreakEvery != 90 {
			t.Errorf("expected BreakEvery=90, got %d", as.prefs.BreakEvery)
		}
	})

	t.Run("no schedule initially", func(t *testing.T) {
		if as.schedule != nil {
			t.Error("expected nil schedule on init")
		}
	})

	t.Run("phase is zero", func(t *testing.T) {
		if as.phase != 0 {
			t.Errorf("expected phase=0, got %d", as.phase)
		}
	})
}

// ---------------------------------------------------------------------------
// Open and Close
// ---------------------------------------------------------------------------

func TestAISchedulerOpenClose(t *testing.T) {
	as := NewAIScheduler()

	tasks := []SchedulerTask{
		{Text: "Write tests", Priority: 3, Estimated: 60, Done: false},
		{Text: "Already done", Priority: 1, Estimated: 30, Done: true},
		{Text: "Review PR", Priority: 2, Estimated: 45, Done: false},
	}
	events := []SchedulerEvent{
		{Title: "Standup", Time: "09:00", Duration: 15},
	}

	as.Open("/vault", tasks, events, "local", "", "", "", "")

	t.Run("active after open", func(t *testing.T) {
		if !as.IsActive() {
			t.Error("expected scheduler to be active after Open")
		}
	})

	t.Run("filters done tasks", func(t *testing.T) {
		if len(as.tasks) != 2 {
			t.Errorf("expected 2 incomplete tasks, got %d", len(as.tasks))
		}
		for _, task := range as.tasks {
			if task.Done {
				t.Error("done task should have been filtered out")
			}
		}
	})

	t.Run("events copied", func(t *testing.T) {
		if len(as.events) != 1 {
			t.Errorf("expected 1 event, got %d", len(as.events))
		}
	})

	t.Run("default provider is local", func(t *testing.T) {
		if as.aiProvider != "local" {
			t.Errorf("expected provider 'local', got %q", as.aiProvider)
		}
	})

	t.Run("default ollama URL", func(t *testing.T) {
		if as.ollamaURL != "http://localhost:11434" {
			t.Errorf("unexpected ollamaURL %q", as.ollamaURL)
		}
	})

	t.Run("default ollama model", func(t *testing.T) {
		if as.ollamaModel != "qwen2.5:0.5b" {
			t.Errorf("unexpected ollamaModel %q", as.ollamaModel)
		}
	})

	t.Run("default openai model", func(t *testing.T) {
		if as.openaiModel != "gpt-4o-mini" {
			t.Errorf("unexpected openaiModel %q", as.openaiModel)
		}
	})

	t.Run("close deactivates", func(t *testing.T) {
		as.Close()
		if as.IsActive() {
			t.Error("expected inactive after Close")
		}
	})
}

func TestAISchedulerOpenAllDoneTasks(t *testing.T) {
	as := NewAIScheduler()
	tasks := []SchedulerTask{
		{Text: "Done 1", Priority: 1, Done: true},
		{Text: "Done 2", Priority: 2, Done: true},
	}
	as.Open("/vault", tasks, nil, "", "", "", "", "")

	if len(as.tasks) != 0 {
		t.Errorf("expected 0 tasks when all are done, got %d", len(as.tasks))
	}
}

// ---------------------------------------------------------------------------
// GetSchedule
// ---------------------------------------------------------------------------

func TestAISchedulerGetSchedule(t *testing.T) {
	as := NewAIScheduler()

	t.Run("no schedule before apply", func(t *testing.T) {
		slots, ok := as.GetSchedule()
		if ok {
			t.Error("expected false before apply")
		}
		if slots != nil {
			t.Error("expected nil slots before apply")
		}
	})

	t.Run("returns schedule after apply", func(t *testing.T) {
		as.applied = true
		as.schedule = []schedulerSlot{
			{StartHour: 8, StartMin: 0, EndHour: 9, EndMin: 0, Task: "Test", Type: "task"},
		}

		slots, ok := as.GetSchedule()
		if !ok {
			t.Error("expected true after apply")
		}
		if len(slots) != 1 {
			t.Errorf("expected 1 slot, got %d", len(slots))
		}
	})

	t.Run("applied resets after get", func(t *testing.T) {
		_, ok := as.GetSchedule()
		if ok {
			t.Error("expected false on second GetSchedule call")
		}
	})
}

// ---------------------------------------------------------------------------
// slotMinutes
// ---------------------------------------------------------------------------

func TestAISchedulerSlotMinutes(t *testing.T) {
	tests := []struct {
		name     string
		slot     schedulerSlot
		expected int
	}{
		{"full hour", schedulerSlot{StartHour: 8, StartMin: 0, EndHour: 9, EndMin: 0}, 60},
		{"half hour", schedulerSlot{StartHour: 9, StartMin: 0, EndHour: 9, EndMin: 30}, 30},
		{"90 minutes", schedulerSlot{StartHour: 10, StartMin: 0, EndHour: 11, EndMin: 30}, 90},
		{"15 minutes", schedulerSlot{StartHour: 12, StartMin: 0, EndHour: 12, EndMin: 15}, 15},
		{"zero duration", schedulerSlot{StartHour: 8, StartMin: 0, EndHour: 8, EndMin: 0}, 0},
		{"cross-hour boundary", schedulerSlot{StartHour: 8, StartMin: 45, EndHour: 9, EndMin: 15}, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.slot.slotMinutes()
			if got != tt.expected {
				t.Errorf("expected %d minutes, got %d", tt.expected, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Local schedule generation — task ordering by priority
// ---------------------------------------------------------------------------

func TestAISchedulerTaskOrderingByPriority(t *testing.T) {
	as := NewAIScheduler()
	as.tasks = []SchedulerTask{
		{Text: "Low priority", Priority: 1, Estimated: 30},
		{Text: "Highest priority", Priority: 4, Estimated: 30},
		{Text: "Medium priority", Priority: 2, Estimated: 30},
		{Text: "High priority", Priority: 3, Estimated: 30},
	}
	as.events = nil

	as.generateLocalSchedule()

	// Extract task slots only (skip lunch, breaks, events)
	var taskSlots []schedulerSlot
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskSlots = append(taskSlots, s)
		}
	}

	if len(taskSlots) != 4 {
		t.Fatalf("expected 4 task slots, got %d", len(taskSlots))
	}

	// First task scheduled should be highest priority (earliest time slot)
	if taskSlots[0].Task != "Highest priority" {
		t.Errorf("expected first task to be 'Highest priority', got %q", taskSlots[0].Task)
	}
	if taskSlots[1].Task != "High priority" {
		t.Errorf("expected second task to be 'High priority', got %q", taskSlots[1].Task)
	}
	if taskSlots[2].Task != "Medium priority" {
		t.Errorf("expected third task to be 'Medium priority', got %q", taskSlots[2].Task)
	}
	if taskSlots[3].Task != "Low priority" {
		t.Errorf("expected fourth task to be 'Low priority', got %q", taskSlots[3].Task)
	}
}

func TestAISchedulerTaskOrderingByDueDate(t *testing.T) {
	as := NewAIScheduler()
	as.tasks = []SchedulerTask{
		{Text: "No due date", Priority: 2, Estimated: 30, DueDate: ""},
		{Text: "Due tomorrow", Priority: 2, Estimated: 30, DueDate: "2026-03-09"},
		{Text: "Due today", Priority: 2, Estimated: 30, DueDate: "2026-03-08"},
	}
	as.events = nil

	as.generateLocalSchedule()

	var taskSlots []schedulerSlot
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskSlots = append(taskSlots, s)
		}
	}

	if len(taskSlots) != 3 {
		t.Fatalf("expected 3 task slots, got %d", len(taskSlots))
	}

	// Same priority: earlier due date should come first
	if taskSlots[0].Task != "Due today" {
		t.Errorf("expected first task to be 'Due today', got %q", taskSlots[0].Task)
	}
	if taskSlots[1].Task != "Due tomorrow" {
		t.Errorf("expected second task to be 'Due tomorrow', got %q", taskSlots[1].Task)
	}
	if taskSlots[2].Task != "No due date" {
		t.Errorf("expected third task to be 'No due date', got %q", taskSlots[2].Task)
	}
}

// ---------------------------------------------------------------------------
// Work hours configuration
// ---------------------------------------------------------------------------

func TestAISchedulerWorkHoursConfig(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     9,
		WorkEnd:       17,
		LunchStart:    12,
		LunchDuration: 30,
		FocusBlockMin: 25,
		BreakEvery:    120,
	}
	as.tasks = []SchedulerTask{
		{Text: "Morning task", Priority: 3, Estimated: 60},
		{Text: "Afternoon task", Priority: 2, Estimated: 60},
	}
	as.events = nil

	as.generateLocalSchedule()

	for _, s := range as.schedule {
		startMins := s.StartHour*60 + s.StartMin
		endMins := s.EndHour*60 + s.EndMin

		if startMins < as.prefs.WorkStart*60 {
			t.Errorf("slot %q starts at %02d:%02d, before work start %02d:00",
				s.Task, s.StartHour, s.StartMin, as.prefs.WorkStart)
		}
		if endMins > as.prefs.WorkEnd*60 {
			t.Errorf("slot %q ends at %02d:%02d, after work end %02d:00",
				s.Task, s.EndHour, s.EndMin, as.prefs.WorkEnd)
		}
	}
}

func TestAISchedulerCustomWorkHours(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     6,
		WorkEnd:       14,
		LunchStart:    11,
		LunchDuration: 30,
		FocusBlockMin: 15,
		BreakEvery:    200,
	}
	as.tasks = []SchedulerTask{
		{Text: "Early task", Priority: 3, Estimated: 30},
	}
	as.events = nil

	as.generateLocalSchedule()

	var taskSlots []schedulerSlot
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskSlots = append(taskSlots, s)
		}
	}

	if len(taskSlots) == 0 {
		t.Fatal("expected at least one task slot")
	}

	// First task should start at or after 06:00
	if taskSlots[0].StartHour < 6 {
		t.Errorf("first task starts at %02d:%02d, expected >= 06:00",
			taskSlots[0].StartHour, taskSlots[0].StartMin)
	}
}

// ---------------------------------------------------------------------------
// Lunch break slot blocking
// ---------------------------------------------------------------------------

func TestAISchedulerLunchBreakBlocking(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    200, // high value to avoid breaks complicating the test
	}
	as.tasks = []SchedulerTask{
		{Text: "Big task", Priority: 4, Estimated: 120},
		{Text: "Another task", Priority: 3, Estimated: 120},
	}
	as.events = nil

	as.generateLocalSchedule()

	// Check that a lunch slot exists
	hasLunch := false
	lunchStart := -1
	lunchEnd := -1
	for _, s := range as.schedule {
		if s.Type == "lunch" {
			hasLunch = true
			lunchStart = s.StartHour*60 + s.StartMin
			lunchEnd = s.EndHour*60 + s.EndMin
		}
	}

	if !hasLunch {
		t.Fatal("expected a lunch slot in the schedule")
	}

	if lunchStart != 12*60 {
		t.Errorf("expected lunch to start at 12:00 (720 min), got %d min", lunchStart)
	}
	if lunchEnd != 13*60 {
		t.Errorf("expected lunch to end at 13:00 (780 min), got %d min", lunchEnd)
	}

	// Check that no task overlaps with lunch
	for _, s := range as.schedule {
		if s.Type == "task" {
			sStart := s.StartHour*60 + s.StartMin
			sEnd := s.EndHour*60 + s.EndMin
			if sStart < lunchEnd && sEnd > lunchStart {
				t.Errorf("task %q (%02d:%02d-%02d:%02d) overlaps with lunch (%02d:%02d-%02d:%02d)",
					s.Task, s.StartHour, s.StartMin, s.EndHour, s.EndMin,
					lunchStart/60, lunchStart%60, lunchEnd/60, lunchEnd%60)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Focus block minimum duration handling
// ---------------------------------------------------------------------------

func TestAISchedulerFocusBlockMinDuration(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 45, // 45 min minimum focus
		BreakEvery:    200,
	}
	// Task estimated at only 10 minutes — should be expanded to 45
	as.tasks = []SchedulerTask{
		{Text: "Quick task", Priority: 2, Estimated: 10},
	}
	as.events = nil

	as.generateLocalSchedule()

	var taskSlots []schedulerSlot
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskSlots = append(taskSlots, s)
		}
	}

	if len(taskSlots) == 0 {
		t.Fatal("expected at least one task slot")
	}

	dur := taskSlots[0].slotMinutes()
	if dur < 45 {
		t.Errorf("task duration %d minutes is less than focus block minimum 45", dur)
	}
}

func TestAISchedulerEstimatedAboveFocusBlock(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    200,
	}
	// Task estimated at 90 minutes, above the 25 min focus block
	as.tasks = []SchedulerTask{
		{Text: "Long task", Priority: 3, Estimated: 90},
	}
	as.events = nil

	as.generateLocalSchedule()

	var taskSlots []schedulerSlot
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskSlots = append(taskSlots, s)
		}
	}

	if len(taskSlots) == 0 {
		t.Fatal("expected at least one task slot")
	}

	dur := taskSlots[0].slotMinutes()
	if dur != 90 {
		t.Errorf("expected 90 minute task slot, got %d", dur)
	}
}

// ---------------------------------------------------------------------------
// Break interval scheduling
// ---------------------------------------------------------------------------

func TestAISchedulerBreakIntervalScheduling(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    60, // break every 60 minutes
	}
	// Three tasks that total > 60 min of work, should trigger a break
	as.tasks = []SchedulerTask{
		{Text: "Task A", Priority: 4, Estimated: 50},
		{Text: "Task B", Priority: 3, Estimated: 50},
		{Text: "Task C", Priority: 2, Estimated: 50},
	}
	as.events = nil

	as.generateLocalSchedule()

	breakCount := 0
	for _, s := range as.schedule {
		if s.Type == "break" {
			breakCount++
			dur := s.slotMinutes()
			if dur != 10 {
				t.Errorf("expected 10 minute break, got %d", dur)
			}
		}
	}

	// After 50 min Task A, workMinsSinceBreak = 50 (< 60, no break)
	// After 50 min Task B, workMinsSinceBreak = 100 (>= 60, break scheduled before Task C)
	if breakCount < 1 {
		t.Errorf("expected at least 1 break with breakEvery=60 and >60min of tasks, got %d", breakCount)
	}
}

func TestAISchedulerNoBreakForShortWork(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    90,
	}
	// Single short task should not trigger a break
	as.tasks = []SchedulerTask{
		{Text: "Quick task", Priority: 3, Estimated: 30},
	}
	as.events = nil

	as.generateLocalSchedule()

	for _, s := range as.schedule {
		if s.Type == "break" {
			t.Error("expected no break for a single 30-min task with breakEvery=90")
		}
	}
}

// ---------------------------------------------------------------------------
// Estimated time calculation (default when 0)
// ---------------------------------------------------------------------------

func TestAISchedulerDefaultEstimatedTime(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    200,
	}
	// Task with no estimated time — should default to 60 minutes
	as.tasks = []SchedulerTask{
		{Text: "No estimate", Priority: 3, Estimated: 0},
	}
	as.events = nil

	as.generateLocalSchedule()

	var taskSlots []schedulerSlot
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskSlots = append(taskSlots, s)
		}
	}

	if len(taskSlots) == 0 {
		t.Fatal("expected at least one task slot")
	}

	dur := taskSlots[0].slotMinutes()
	if dur != 60 {
		t.Errorf("expected default 60 minute slot for task with no estimate, got %d", dur)
	}
}

func TestAISchedulerNegativeEstimatedTime(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    200,
	}
	// Negative estimated time should also default to 60
	as.tasks = []SchedulerTask{
		{Text: "Negative estimate", Priority: 2, Estimated: -10},
	}
	as.events = nil

	as.generateLocalSchedule()

	var taskSlots []schedulerSlot
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskSlots = append(taskSlots, s)
		}
	}

	if len(taskSlots) == 0 {
		t.Fatal("expected at least one task slot")
	}

	dur := taskSlots[0].slotMinutes()
	if dur != 60 {
		t.Errorf("expected default 60 minute slot for negative estimate, got %d", dur)
	}
}

// ---------------------------------------------------------------------------
// Edge case: no tasks
// ---------------------------------------------------------------------------

func TestAISchedulerNoTasks(t *testing.T) {
	as := NewAIScheduler()
	as.tasks = nil
	as.events = nil

	as.generateLocalSchedule()

	// Should only have a lunch slot
	if len(as.schedule) != 1 {
		t.Errorf("expected exactly 1 slot (lunch) with no tasks, got %d", len(as.schedule))
	}
	if as.schedule[0].Type != "lunch" {
		t.Errorf("expected lunch slot, got type %q", as.schedule[0].Type)
	}
}

func TestAISchedulerStartGenerationNoTasks(t *testing.T) {
	as := NewAIScheduler()
	as.active = true
	as.tasks = nil

	result, cmd := as.startGeneration()
	if cmd != nil {
		t.Error("expected nil cmd with no tasks")
	}
	if result.statusMsg != "No tasks to schedule" {
		t.Errorf("expected 'No tasks to schedule', got %q", result.statusMsg)
	}
	// Phase should remain 0
	if result.phase != 0 {
		t.Errorf("expected phase=0, got %d", result.phase)
	}
}

// ---------------------------------------------------------------------------
// Edge case: all tasks completed (filtered by Open)
// ---------------------------------------------------------------------------

func TestAISchedulerAllTasksCompleted(t *testing.T) {
	as := NewAIScheduler()
	tasks := []SchedulerTask{
		{Text: "Done 1", Priority: 4, Estimated: 60, Done: true},
		{Text: "Done 2", Priority: 3, Estimated: 45, Done: true},
	}
	as.Open("/vault", tasks, nil, "local", "", "", "", "")

	if len(as.tasks) != 0 {
		t.Errorf("expected 0 tasks after filtering done, got %d", len(as.tasks))
	}

	// Attempting to start generation should show no tasks message
	result, cmd := as.startGeneration()
	if cmd != nil {
		t.Error("expected nil cmd with no tasks")
	}
	if result.statusMsg != "No tasks to schedule" {
		t.Errorf("expected 'No tasks to schedule', got %q", result.statusMsg)
	}
}

// ---------------------------------------------------------------------------
// Fixed events placement
// ---------------------------------------------------------------------------

func TestAISchedulerFixedEventsPlacement(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    200,
	}
	as.tasks = []SchedulerTask{
		{Text: "Task A", Priority: 3, Estimated: 50},
	}
	as.events = []SchedulerEvent{
		{Title: "Standup", Time: "09:00", Duration: 15},
		{Title: "Planning", Time: "14:00", Duration: 60},
	}

	as.generateLocalSchedule()

	// Check events are in the schedule
	eventCount := 0
	for _, s := range as.schedule {
		if s.Type == "event" {
			eventCount++
		}
	}
	if eventCount != 2 {
		t.Errorf("expected 2 events in schedule, got %d", eventCount)
	}

	// No task should overlap with events
	for _, s := range as.schedule {
		if s.Type == "task" {
			sStart := s.StartHour*60 + s.StartMin
			sEnd := s.EndHour*60 + s.EndMin
			for _, e := range as.schedule {
				if e.Type == "event" {
					eStart := e.StartHour*60 + e.StartMin
					eEnd := e.EndHour*60 + e.EndMin
					if sStart < eEnd && sEnd > eStart {
						t.Errorf("task %q (%02d:%02d-%02d:%02d) overlaps with event %q (%02d:%02d-%02d:%02d)",
							s.Task, s.StartHour, s.StartMin, s.EndHour, s.EndMin,
							e.Task, e.StartHour, e.StartMin, e.EndHour, e.EndMin)
					}
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Schedule is sorted by start time
// ---------------------------------------------------------------------------

func TestAISchedulerSortedByStartTime(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    200,
	}
	as.tasks = []SchedulerTask{
		{Text: "Task 1", Priority: 4, Estimated: 60},
		{Text: "Task 2", Priority: 3, Estimated: 60},
		{Text: "Task 3", Priority: 2, Estimated: 60},
	}
	as.events = []SchedulerEvent{
		{Title: "Meeting", Time: "10:00", Duration: 30},
	}

	as.generateLocalSchedule()

	for i := 1; i < len(as.schedule); i++ {
		prevStart := as.schedule[i-1].StartHour*60 + as.schedule[i-1].StartMin
		currStart := as.schedule[i].StartHour*60 + as.schedule[i].StartMin
		if currStart < prevStart {
			t.Errorf("schedule not sorted: slot %d starts at %d, but slot %d starts at %d",
				i-1, prevStart, i, currStart)
		}
	}
}

// ---------------------------------------------------------------------------
// No overlapping slots in generated schedule
// ---------------------------------------------------------------------------

func TestAISchedulerNoOverlappingSlots(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    60,
	}
	as.tasks = []SchedulerTask{
		{Text: "Task A", Priority: 4, Estimated: 60},
		{Text: "Task B", Priority: 3, Estimated: 45},
		{Text: "Task C", Priority: 2, Estimated: 30},
		{Text: "Task D", Priority: 1, Estimated: 50},
	}
	as.events = []SchedulerEvent{
		{Title: "Meeting", Time: "10:00", Duration: 30},
	}

	as.generateLocalSchedule()

	for i := 0; i < len(as.schedule); i++ {
		for j := i + 1; j < len(as.schedule); j++ {
			iStart := as.schedule[i].StartHour*60 + as.schedule[i].StartMin
			iEnd := as.schedule[i].EndHour*60 + as.schedule[i].EndMin
			jStart := as.schedule[j].StartHour*60 + as.schedule[j].StartMin
			jEnd := as.schedule[j].EndHour*60 + as.schedule[j].EndMin

			if iStart < jEnd && jStart < iEnd {
				t.Errorf("overlap between %q (%02d:%02d-%02d:%02d) and %q (%02d:%02d-%02d:%02d)",
					as.schedule[i].Task, as.schedule[i].StartHour, as.schedule[i].StartMin,
					as.schedule[i].EndHour, as.schedule[i].EndMin,
					as.schedule[j].Task, as.schedule[j].StartHour, as.schedule[j].StartMin,
					as.schedule[j].EndHour, as.schedule[j].EndMin)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Tasks that exceed work hours are skipped
// ---------------------------------------------------------------------------

func TestAISchedulerTasksExceedingWorkHours(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       10, // only 2 hours of work
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    200,
	}
	// lunch at 12 won't fit either, but will be placed
	// Three 60-minute tasks but only ~120 min available (minus lunch which is outside range)
	as.tasks = []SchedulerTask{
		{Text: "Task A", Priority: 4, Estimated: 55},
		{Text: "Task B", Priority: 3, Estimated: 55},
		{Text: "Task C", Priority: 2, Estimated: 55},
	}
	as.events = nil

	as.generateLocalSchedule()

	taskCount := 0
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskCount++
		}
	}

	// With transition buffers (5 min each), each task takes 60 min.
	// 120 min work window can fit at most 2 tasks.
	if taskCount > 2 {
		t.Errorf("expected at most 2 tasks in 2-hour window, got %d", taskCount)
	}
}

// ---------------------------------------------------------------------------
// parseAIResponse
// ---------------------------------------------------------------------------

func TestAISchedulerParseAIResponse(t *testing.T) {
	as := NewAIScheduler()
	as.tasks = []SchedulerTask{
		{Text: "Fix auth bug", Priority: 4, Estimated: 90},
		{Text: "Write docs", Priority: 2, Estimated: 60},
	}

	t.Run("valid response", func(t *testing.T) {
		response := `Here's your schedule:
08:00-09:30 | Fix auth bug | task
09:30-09:40 | Break | break
09:40-10:40 | Write docs | task
12:00-13:00 | Lunch | lunch`

		as.parseAIResponse(response)

		if len(as.schedule) != 4 {
			t.Fatalf("expected 4 slots, got %d", len(as.schedule))
		}

		// Check first slot
		if as.schedule[0].Task != "Fix auth bug" {
			t.Errorf("expected first task 'Fix auth bug', got %q", as.schedule[0].Task)
		}
		if as.schedule[0].StartHour != 8 || as.schedule[0].StartMin != 0 {
			t.Errorf("expected start 08:00, got %02d:%02d", as.schedule[0].StartHour, as.schedule[0].StartMin)
		}
		if as.schedule[0].EndHour != 9 || as.schedule[0].EndMin != 30 {
			t.Errorf("expected end 09:30, got %02d:%02d", as.schedule[0].EndHour, as.schedule[0].EndMin)
		}

		// Priority should be matched from original tasks
		if as.schedule[0].Priority != 4 {
			t.Errorf("expected priority 4 for 'Fix auth bug', got %d", as.schedule[0].Priority)
		}
	})

	t.Run("invalid response falls back to local", func(t *testing.T) {
		as.tasks = []SchedulerTask{
			{Text: "Task A", Priority: 3, Estimated: 30},
		}

		as.parseAIResponse("This is gibberish that doesn't match the schedule format.")

		// Should have fallen back to local algorithm which generates at least lunch + task
		if len(as.schedule) == 0 {
			t.Error("expected fallback schedule, got empty")
		}

		hasTask := false
		for _, s := range as.schedule {
			if s.Type == "task" {
				hasTask = true
			}
		}
		if !hasTask {
			t.Error("expected at least one task slot from local fallback")
		}
	})

	t.Run("empty response falls back to local", func(t *testing.T) {
		as.tasks = []SchedulerTask{
			{Text: "Fallback task", Priority: 2, Estimated: 30},
		}

		as.parseAIResponse("")

		if len(as.schedule) == 0 {
			t.Error("expected fallback schedule on empty response")
		}
	})
}

// ---------------------------------------------------------------------------
// buildSchedulerPrompt
// ---------------------------------------------------------------------------

func TestAISchedulerBuildPrompt(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     9,
		WorkEnd:       17,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 30,
		BreakEvery:    90,
	}
	as.tasks = []SchedulerTask{
		{Text: "Write code", Priority: 3, Estimated: 120, DueDate: "2026-03-10"},
		{Text: "Review PR", Priority: 2, Estimated: 0},
	}
	as.events = []SchedulerEvent{
		{Title: "Standup", Time: "09:00", Duration: 15},
	}

	prompt := as.buildSchedulerPrompt()

	t.Run("contains task text", func(t *testing.T) {
		if !strings.Contains(prompt, "Write code") {
			t.Error("prompt should contain task text 'Write code'")
		}
		if !strings.Contains(prompt, "Review PR") {
			t.Error("prompt should contain task text 'Review PR'")
		}
	})

	t.Run("contains due date", func(t *testing.T) {
		if !strings.Contains(prompt, "2026-03-10") {
			t.Error("prompt should contain due date")
		}
	})

	t.Run("default estimate for zero", func(t *testing.T) {
		// Task with 0 estimate should show 60min in prompt
		if !strings.Contains(prompt, "est: 60min") {
			t.Error("prompt should use default 60min estimate for task with 0 estimate")
		}
	})

	t.Run("contains work hours", func(t *testing.T) {
		if !strings.Contains(prompt, "09:00-17:00") {
			t.Error("prompt should contain work hours")
		}
	})

	t.Run("contains event", func(t *testing.T) {
		if !strings.Contains(prompt, "Standup") {
			t.Error("prompt should contain event title")
		}
	})

	t.Run("contains break interval", func(t *testing.T) {
		if !strings.Contains(prompt, "90min") {
			t.Error("prompt should contain break interval")
		}
	})

	t.Run("contains focus block", func(t *testing.T) {
		if !strings.Contains(prompt, "30 minutes") {
			t.Error("prompt should contain focus block minimum")
		}
	})
}

// ---------------------------------------------------------------------------
// countScheduledTasks
// ---------------------------------------------------------------------------

func TestAISchedulerCountScheduledTasks(t *testing.T) {
	as := NewAIScheduler()
	as.schedule = []schedulerSlot{
		{Task: "Task A", Type: "task"},
		{Task: "Lunch", Type: "lunch"},
		{Task: "Task B", Type: "task"},
		{Task: "Break", Type: "break"},
		{Task: "Meeting", Type: "event"},
		{Task: "Task C", Type: "task"},
	}

	count := as.countScheduledTasks()
	if count != 3 {
		t.Errorf("expected 3 tasks, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// startGeneration with local provider
// ---------------------------------------------------------------------------

func TestAISchedulerStartGenerationLocal(t *testing.T) {
	as := NewAIScheduler()
	as.active = true
	as.aiProvider = "local"
	as.tasks = []SchedulerTask{
		{Text: "Task A", Priority: 3, Estimated: 60},
		{Text: "Task B", Priority: 2, Estimated: 45},
	}
	as.events = nil

	result, cmd := as.startGeneration()

	if cmd != nil {
		t.Error("local provider should not return a command")
	}
	if result.phase != 2 {
		t.Errorf("expected phase=2 after local generation, got %d", result.phase)
	}
	if len(result.schedule) == 0 {
		t.Error("expected non-empty schedule after local generation")
	}
	if result.statusMsg != "Generated with local scheduler" {
		t.Errorf("unexpected status message: %q", result.statusMsg)
	}
}

// ---------------------------------------------------------------------------
// adjustField — preference boundary checks
// ---------------------------------------------------------------------------

func TestAISchedulerAdjustField(t *testing.T) {
	t.Run("workStart clamps to 0", func(t *testing.T) {
		as := NewAIScheduler()
		as.prefs.WorkStart = 0
		as.prefs.WorkEnd = 18
		as.setupField = 0
		as.adjustField(-1)
		if as.prefs.WorkStart < 0 {
			t.Errorf("workStart should not go below 0, got %d", as.prefs.WorkStart)
		}
	})

	t.Run("workStart clamps to 23", func(t *testing.T) {
		as := NewAIScheduler()
		as.prefs.WorkStart = 23
		as.prefs.WorkEnd = 24
		as.setupField = 0
		as.adjustField(1)
		if as.prefs.WorkStart > 23 {
			t.Errorf("workStart should not exceed 23, got %d", as.prefs.WorkStart)
		}
	})

	t.Run("workEnd cannot go below 1", func(t *testing.T) {
		as := NewAIScheduler()
		as.prefs.WorkEnd = 1
		as.prefs.WorkStart = 0
		as.setupField = 1
		as.adjustField(-1)
		if as.prefs.WorkEnd < 1 {
			t.Errorf("workEnd should not go below 1, got %d", as.prefs.WorkEnd)
		}
	})

	t.Run("workEnd cannot exceed 24", func(t *testing.T) {
		as := NewAIScheduler()
		as.prefs.WorkEnd = 24
		as.setupField = 1
		as.adjustField(1)
		if as.prefs.WorkEnd > 24 {
			t.Errorf("workEnd should not exceed 24, got %d", as.prefs.WorkEnd)
		}
	})

	t.Run("workStart stays below workEnd", func(t *testing.T) {
		as := NewAIScheduler()
		as.prefs.WorkStart = 10
		as.prefs.WorkEnd = 11
		as.setupField = 0
		as.adjustField(1)
		if as.prefs.WorkStart >= as.prefs.WorkEnd {
			t.Errorf("workStart (%d) should stay below workEnd (%d)", as.prefs.WorkStart, as.prefs.WorkEnd)
		}
	})

	t.Run("focusBlockMin clamps range 5-120", func(t *testing.T) {
		as := NewAIScheduler()
		as.prefs.FocusBlockMin = 5
		as.setupField = 3
		as.adjustField(-1)
		if as.prefs.FocusBlockMin < 5 {
			t.Errorf("focusBlockMin should not go below 5, got %d", as.prefs.FocusBlockMin)
		}

		as.prefs.FocusBlockMin = 120
		as.adjustField(1)
		if as.prefs.FocusBlockMin > 120 {
			t.Errorf("focusBlockMin should not exceed 120, got %d", as.prefs.FocusBlockMin)
		}
	})

	t.Run("breakEvery clamps range 20-180", func(t *testing.T) {
		as := NewAIScheduler()
		as.prefs.BreakEvery = 20
		as.setupField = 4
		as.adjustField(-1)
		if as.prefs.BreakEvery < 20 {
			t.Errorf("breakEvery should not go below 20, got %d", as.prefs.BreakEvery)
		}

		as.prefs.BreakEvery = 180
		as.adjustField(1)
		if as.prefs.BreakEvery > 180 {
			t.Errorf("breakEvery should not exceed 180, got %d", as.prefs.BreakEvery)
		}
	})
}

// ---------------------------------------------------------------------------
// combinedCursor / setCombinedCursor
// ---------------------------------------------------------------------------

func TestAISchedulerCombinedCursor(t *testing.T) {
	as := NewAIScheduler()
	as.tasks = []SchedulerTask{
		{Text: "Task A"},
		{Text: "Task B"},
		{Text: "Task C"},
	}

	t.Run("pref fields 0-4", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			as.setupField = i
			as.taskCursor = -1
			if as.combinedCursor() != i {
				t.Errorf("expected combinedCursor=%d for setupField=%d, got %d", i, i, as.combinedCursor())
			}
		}
	})

	t.Run("task fields start at 5", func(t *testing.T) {
		as.setupField = 5
		as.taskCursor = 0
		if as.combinedCursor() != 5 {
			t.Errorf("expected combinedCursor=5, got %d", as.combinedCursor())
		}
		as.taskCursor = 2
		if as.combinedCursor() != 7 {
			t.Errorf("expected combinedCursor=7, got %d", as.combinedCursor())
		}
	})

	t.Run("setCombinedCursor for prefs", func(t *testing.T) {
		as.setCombinedCursor(2)
		if as.setupField != 2 {
			t.Errorf("expected setupField=2, got %d", as.setupField)
		}
		if as.taskCursor != -1 {
			t.Errorf("expected taskCursor=-1 for pref field, got %d", as.taskCursor)
		}
	})

	t.Run("setCombinedCursor for tasks", func(t *testing.T) {
		as.setCombinedCursor(6)
		if as.setupField != 5 {
			t.Errorf("expected setupField=5 for task cursor, got %d", as.setupField)
		}
		if as.taskCursor != 1 {
			t.Errorf("expected taskCursor=1, got %d", as.taskCursor)
		}
	})

	t.Run("setCombinedCursor clamps to last task", func(t *testing.T) {
		as.setCombinedCursor(100)
		if as.taskCursor != 2 {
			t.Errorf("expected taskCursor clamped to 2, got %d", as.taskCursor)
		}
	})
}

// ---------------------------------------------------------------------------
// breakEvery=0 defaults to 90
// ---------------------------------------------------------------------------

func TestAISchedulerBreakEveryZeroDefaults(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    0, // should default to 90 inside generateLocalSchedule
	}
	as.tasks = []SchedulerTask{
		{Text: "Task A", Priority: 3, Estimated: 60},
	}
	as.events = nil

	// Should not panic
	as.generateLocalSchedule()

	taskCount := 0
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskCount++
		}
	}
	if taskCount != 1 {
		t.Errorf("expected 1 task, got %d", taskCount)
	}
}

// ---------------------------------------------------------------------------
// Events with invalid time format
// ---------------------------------------------------------------------------

func TestAISchedulerEventInvalidTimeFormat(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    200,
	}
	as.tasks = []SchedulerTask{
		{Text: "Task A", Priority: 3, Estimated: 30},
	}
	as.events = []SchedulerEvent{
		{Title: "Bad event", Time: "invalid", Duration: 30},
	}

	// Should not panic; invalid event is just skipped
	as.generateLocalSchedule()

	eventCount := 0
	for _, s := range as.schedule {
		if s.Type == "event" {
			eventCount++
		}
	}
	if eventCount != 0 {
		t.Errorf("expected 0 events for invalid time format, got %d", eventCount)
	}
}

// ---------------------------------------------------------------------------
// scheduleLinePattern regex
// ---------------------------------------------------------------------------

func TestAISchedulerScheduleLinePattern(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		matches bool
	}{
		{"standard format", "08:00-09:30 | Fix auth bug | task", true},
		{"with spaces", "08:00 - 09:30 | Fix auth bug | task", true},
		{"single digit hour", "8:00-9:30 | Fix auth bug | task", true},
		{"no match text only", "Just some text", false},
		{"empty line", "", false},
		{"partial format", "08:00-09:30 | Fix auth bug", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := scheduleLinePattern.FindStringSubmatch(tt.line)
			if tt.matches && m == nil {
				t.Errorf("expected match for %q", tt.line)
			}
			if !tt.matches && m != nil {
				t.Errorf("expected no match for %q, got %v", tt.line, m)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Priority matching in parseAIResponse
// ---------------------------------------------------------------------------

func TestAISchedulerPriorityMatching(t *testing.T) {
	as := NewAIScheduler()
	as.tasks = []SchedulerTask{
		{Text: "Fix auth bug", Priority: 4, Estimated: 90},
		{Text: "Write docs", Priority: 1, Estimated: 60},
	}

	response := "08:00-09:30 | Fix auth bug | task\n09:30-10:30 | Write docs | task\n10:30-11:00 | Unknown task | task"
	as.parseAIResponse(response)

	if len(as.schedule) != 3 {
		t.Fatalf("expected 3 slots, got %d", len(as.schedule))
	}

	// Known task should have priority matched
	if as.schedule[0].Priority != 4 {
		t.Errorf("expected priority 4 for 'Fix auth bug', got %d", as.schedule[0].Priority)
	}
	if as.schedule[1].Priority != 1 {
		t.Errorf("expected priority 1 for 'Write docs', got %d", as.schedule[1].Priority)
	}

	// Unknown task gets priority 0
	if as.schedule[2].Priority != 0 {
		t.Errorf("expected priority 0 for unknown task, got %d", as.schedule[2].Priority)
	}
}

// ---------------------------------------------------------------------------
// Transition buffer (5 min between tasks)
// ---------------------------------------------------------------------------

func TestAISchedulerTransitionBuffer(t *testing.T) {
	as := NewAIScheduler()
	as.prefs = SchedulerPrefs{
		WorkStart:     8,
		WorkEnd:       18,
		LunchStart:    12,
		LunchDuration: 60,
		FocusBlockMin: 25,
		BreakEvery:    200,
	}
	as.tasks = []SchedulerTask{
		{Text: "Task A", Priority: 4, Estimated: 30},
		{Text: "Task B", Priority: 3, Estimated: 30},
	}
	as.events = nil

	as.generateLocalSchedule()

	var taskSlots []schedulerSlot
	for _, s := range as.schedule {
		if s.Type == "task" {
			taskSlots = append(taskSlots, s)
		}
	}

	if len(taskSlots) < 2 {
		t.Fatal("expected at least 2 task slots")
	}

	// End of first task + 5 min transition <= start of second task
	firstEnd := taskSlots[0].EndHour*60 + taskSlots[0].EndMin
	secondStart := taskSlots[1].StartHour*60 + taskSlots[1].StartMin

	gap := secondStart - firstEnd
	if gap < 5 {
		t.Errorf("expected at least 5-minute transition buffer between tasks, got %d minutes gap", gap)
	}
}
