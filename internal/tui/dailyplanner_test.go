package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestPlanner returns a DailyPlanner with blocks initialised and a temp
// vault root, suitable for unit tests.
func newTestPlanner(t *testing.T) *DailyPlanner {
	t.Helper()
	dp := NewDailyPlanner()
	dp.vaultRoot = t.TempDir()
	dp.date = time.Date(2026, 3, 8, 10, 0, 0, 0, time.Local)
	dp.initBlocks()
	return &dp
}

// openTestPlanner creates an active planner via the Open path, using a fresh
// temp dir so that loadFromFile finds nothing.
func openTestPlanner(t *testing.T, tasks []PlannerTask, events []PlannerEvent, habits []PlannerHabit) *DailyPlanner {
	t.Helper()
	dp := NewDailyPlanner()
	dp.Open(t.TempDir(), tasks, events, habits)
	return &dp
}

// ---------------------------------------------------------------------------
// 1. NewDailyPlanner() initialisation
// ---------------------------------------------------------------------------

func TestNewDailyPlanner(t *testing.T) {
	dp := NewDailyPlanner()

	if dp.active {
		t.Error("new planner should not be active")
	}
	if dp.IsActive() {
		t.Error("IsActive should return false for a new planner")
	}
	if dp.blocks != nil {
		t.Error("blocks should be nil before Open")
	}
	if dp.cursor != 0 {
		t.Error("cursor should be 0")
	}
	if dp.focus != panelSchedule {
		t.Error("focus should default to panelSchedule")
	}
	if dp.moving {
		t.Error("moving should default to false")
	}
	if dp.adding {
		t.Error("adding should default to false")
	}
	if dp.hasFocusResult {
		t.Error("hasFocusResult should default to false")
	}
	if dp.modified {
		t.Error("modified should default to false")
	}
}

// ---------------------------------------------------------------------------
// 2. PlannerTask struct population
// ---------------------------------------------------------------------------

func TestPlannerTaskPopulation(t *testing.T) {
	task := PlannerTask{
		Text:     "Write report",
		Done:     false,
		Priority: 3,
		DueDate:  "2026-03-08",
		Source:   "Tasks.md",
	}

	if task.Text != "Write report" {
		t.Errorf("expected Text 'Write report', got %q", task.Text)
	}
	if task.Done {
		t.Error("expected Done=false")
	}
	if task.Priority != 3 {
		t.Errorf("expected Priority 3, got %d", task.Priority)
	}
	if task.DueDate != "2026-03-08" {
		t.Errorf("expected DueDate '2026-03-08', got %q", task.DueDate)
	}
	if task.Source != "Tasks.md" {
		t.Errorf("expected Source 'Tasks.md', got %q", task.Source)
	}
}

func TestPlannerTaskDoneState(t *testing.T) {
	task := PlannerTask{Text: "Finished task", Done: true, Priority: 0}
	if !task.Done {
		t.Error("expected Done=true")
	}
}

func TestPlannerTaskPriorityRange(t *testing.T) {
	for _, pri := range []int{0, 1, 2, 3, 4} {
		task := PlannerTask{Text: "Test", Priority: pri}
		if task.Priority != pri {
			t.Errorf("expected Priority %d, got %d", pri, task.Priority)
		}
	}
}

func TestPlannerEventDefaults(t *testing.T) {
	ev := PlannerEvent{Title: "Standup"}
	if ev.Time != "" {
		t.Error("Time should default to empty string")
	}
	if ev.Duration != 0 {
		t.Error("Duration should default to 0")
	}
}

func TestPlannerHabitFields(t *testing.T) {
	habit := PlannerHabit{Name: "Exercise", Done: true, Streak: 7}
	if habit.Name != "Exercise" {
		t.Error("Name mismatch")
	}
	if !habit.Done {
		t.Error("Done mismatch")
	}
	if habit.Streak != 7 {
		t.Error("Streak mismatch")
	}
}

// ---------------------------------------------------------------------------
// 3. timeBlock creation and management
// ---------------------------------------------------------------------------

func TestInitBlocks_Count(t *testing.T) {
	dp := newTestPlanner(t)
	// 6:00..21:30 = 16 hours * 2 = 32 slots
	if len(dp.blocks) != 32 {
		t.Fatalf("expected 32 blocks, got %d", len(dp.blocks))
	}
}

func TestInitBlocks_FirstAndLast(t *testing.T) {
	dp := newTestPlanner(t)

	first := dp.blocks[0]
	if first.Hour != 6 || first.HalfHour {
		t.Errorf("first block should be 06:00, got %d:%v", first.Hour, first.HalfHour)
	}

	last := dp.blocks[31]
	if last.Hour != 21 || !last.HalfHour {
		t.Errorf("last block should be 21:30, got %d:%v", last.Hour, last.HalfHour)
	}
}

func TestInitBlocks_AllEmpty(t *testing.T) {
	dp := newTestPlanner(t)
	for i, b := range dp.blocks {
		if b.TaskType != blockEmpty {
			t.Errorf("block %d should be empty, got type %d", i, b.TaskType)
		}
		if b.TaskText != "" {
			t.Errorf("block %d should have empty text, got %q", i, b.TaskText)
		}
	}
}

func TestInitBlocks_HalfHourAlternation(t *testing.T) {
	dp := newTestPlanner(t)
	for i, b := range dp.blocks {
		expectHalf := (i % 2) == 1
		if b.HalfHour != expectHalf {
			t.Errorf("block %d: expected HalfHour=%v, got %v", i, expectHalf, b.HalfHour)
		}
	}
}

func TestInitBlocks_HourProgression(t *testing.T) {
	dp := newTestPlanner(t)
	for i := 0; i < len(dp.blocks); i += 2 {
		expectedHour := 6 + i/2
		if dp.blocks[i].Hour != expectedHour {
			t.Errorf("block %d: expected hour %d, got %d", i, expectedHour, dp.blocks[i].Hour)
		}
		if dp.blocks[i+1].Hour != expectedHour {
			t.Errorf("block %d: expected hour %d, got %d", i+1, expectedHour, dp.blocks[i+1].Hour)
		}
	}
}

func TestInitBlocks_ReinitClears(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(5, "Something", blockTask, 1)

	dp.initBlocks()

	if dp.blocks[5].TaskType != blockEmpty {
		t.Error("reinit should clear all blocks")
	}
}

// ---------------------------------------------------------------------------
// 4. Task import from vault (Open with tasks, events, habits)
// ---------------------------------------------------------------------------

func TestOpen_SetsActiveAndDate(t *testing.T) {
	dp := openTestPlanner(t, nil, nil, nil)
	if !dp.active {
		t.Error("planner should be active after Open")
	}
	if dp.date.IsZero() {
		t.Error("date should be set after Open")
	}
}

func TestOpen_ResetsState(t *testing.T) {
	dp := openTestPlanner(t, nil, nil, nil)

	if dp.cursor != 0 {
		t.Error("cursor should be 0")
	}
	if dp.scroll != 0 {
		t.Error("scroll should be 0")
	}
	if dp.focus != panelSchedule {
		t.Error("focus should be panelSchedule")
	}
	if dp.adding {
		t.Error("adding should be false")
	}
	if dp.addBuf != "" {
		t.Error("addBuf should be empty")
	}
	if dp.moving {
		t.Error("moving should be false")
	}
	if dp.moveFrom != -1 {
		t.Error("moveFrom should be -1")
	}
	if dp.hasFocusResult {
		t.Error("hasFocusResult should be false")
	}
	if dp.modified {
		t.Error("modified should be false")
	}
}

func TestOpen_ImportsTasksDueToday(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	tasks := []PlannerTask{
		{Text: "Due today", DueDate: today, Priority: 2},
		{Text: "Due tomorrow", DueDate: "2099-12-31", Priority: 1},
	}
	dp := openTestPlanner(t, tasks, nil, nil)

	if len(dp.unscheduled) != 1 {
		t.Fatalf("expected 1 unscheduled task, got %d", len(dp.unscheduled))
	}
	if dp.unscheduled[0].Text != "Due today" {
		t.Errorf("expected 'Due today', got %q", dp.unscheduled[0].Text)
	}
}

func TestOpen_SortsUnscheduledByPriorityDesc(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	tasks := []PlannerTask{
		{Text: "Low", DueDate: today, Priority: 1},
		{Text: "High", DueDate: today, Priority: 4},
		{Text: "Med", DueDate: today, Priority: 2},
	}
	dp := openTestPlanner(t, tasks, nil, nil)

	if len(dp.unscheduled) != 3 {
		t.Fatalf("expected 3 unscheduled tasks, got %d", len(dp.unscheduled))
	}
	if dp.unscheduled[0].Priority != 4 {
		t.Errorf("first should have priority 4, got %d", dp.unscheduled[0].Priority)
	}
	if dp.unscheduled[1].Priority != 2 {
		t.Errorf("second should have priority 2, got %d", dp.unscheduled[1].Priority)
	}
	if dp.unscheduled[2].Priority != 1 {
		t.Errorf("third should have priority 1, got %d", dp.unscheduled[2].Priority)
	}
}

func TestOpen_PlacesTimedEvents(t *testing.T) {
	events := []PlannerEvent{
		{Title: "Standup", Time: "09:00", Duration: 30},
	}
	dp := openTestPlanner(t, nil, events, nil)

	// 09:00 maps to slot (9-6)*2 = 6
	if dp.blocks[6].TaskType != blockEvent {
		t.Error("slot 6 (09:00) should be an event")
	}
	if dp.blocks[6].TaskText != "Standup" {
		t.Errorf("expected 'Standup', got %q", dp.blocks[6].TaskText)
	}
}

func TestOpen_PlacesMultiSlotEvent(t *testing.T) {
	events := []PlannerEvent{
		{Title: "Workshop", Time: "10:00", Duration: 90},
	}
	dp := openTestPlanner(t, nil, events, nil)

	// 10:00 = slot 8, 90 min = 3 slots
	for _, idx := range []int{8, 9, 10} {
		if dp.blocks[idx].TaskText != "Workshop" {
			t.Errorf("slot %d should have 'Workshop', got %q", idx, dp.blocks[idx].TaskText)
		}
		if dp.blocks[idx].TaskType != blockEvent {
			t.Errorf("slot %d should be blockEvent", idx)
		}
	}
}

func TestOpen_CopiesHabits(t *testing.T) {
	habits := []PlannerHabit{
		{Name: "Exercise", Done: false, Streak: 3},
		{Name: "Read", Done: true, Streak: 10},
	}
	dp := openTestPlanner(t, nil, nil, habits)

	if len(dp.habits) != 2 {
		t.Fatalf("expected 2 habits, got %d", len(dp.habits))
	}
	if dp.habits[0].Name != "Exercise" || dp.habits[0].Streak != 3 {
		t.Error("habit 0 mismatch")
	}
	if dp.habits[1].Name != "Read" || !dp.habits[1].Done {
		t.Error("habit 1 mismatch")
	}
}

func TestOpen_DefaultDurationEvent(t *testing.T) {
	// Duration 0 should default to 60 minutes = 2 slots
	events := []PlannerEvent{
		{Title: "Meeting", Time: "14:00", Duration: 0},
	}
	dp := openTestPlanner(t, nil, events, nil)

	// 14:00 = slot (14-6)*2 = 16
	if dp.blocks[16].TaskText != "Meeting" {
		t.Errorf("slot 16 should be 'Meeting', got %q", dp.blocks[16].TaskText)
	}
	if dp.blocks[17].TaskText != "Meeting" {
		t.Errorf("slot 17 should be 'Meeting' (default 60min = 2 slots), got %q", dp.blocks[17].TaskText)
	}
}

func TestOpen_SkipsEventsWithNoTime(t *testing.T) {
	events := []PlannerEvent{
		{Title: "All day event", Time: "", Duration: 60},
	}
	dp := openTestPlanner(t, nil, events, nil)

	for i, b := range dp.blocks {
		if b.TaskText == "All day event" {
			t.Errorf("event with no time should not be placed in slot %d", i)
		}
	}
}

func TestOpen_HabitsCopiedIndependently(t *testing.T) {
	habits := []PlannerHabit{
		{Name: "Exercise", Done: false, Streak: 3},
	}
	dp := openTestPlanner(t, nil, nil, habits)

	// Mutate the original slice
	habits[0].Done = true

	// Planner's copy should be unaffected
	if dp.habits[0].Done {
		t.Error("modifying original habits should not affect planner copy")
	}
}

// ---------------------------------------------------------------------------
// 5. GetFocusResult() consumed-once pattern
// ---------------------------------------------------------------------------

func TestGetFocusResult_ConsumedOnce(t *testing.T) {
	dp := newTestPlanner(t)

	dp.focusTask = "Deep work"
	dp.focusDuration = 60
	dp.hasFocusResult = true

	task, dur, ok := dp.GetFocusResult()
	if !ok {
		t.Error("expected ok=true")
	}
	if task != "Deep work" {
		t.Errorf("expected 'Deep work', got %q", task)
	}
	if dur != 60 {
		t.Errorf("expected 60, got %d", dur)
	}

	// Second call should return not-ok
	_, _, ok2 := dp.GetFocusResult()
	if ok2 {
		t.Error("expected ok=false after consumption")
	}
}

func TestGetFocusResult_NotSet(t *testing.T) {
	dp := newTestPlanner(t)

	_, _, ok := dp.GetFocusResult()
	if ok {
		t.Error("expected ok=false when no focus result is set")
	}
}

func TestGetFocusResult_ClearsState(t *testing.T) {
	dp := newTestPlanner(t)
	dp.focusTask = "Task"
	dp.focusDuration = 30
	dp.hasFocusResult = true

	dp.GetFocusResult()

	if dp.focusTask != "" {
		t.Error("focusTask should be cleared")
	}
	if dp.focusDuration != 0 {
		t.Error("focusDuration should be cleared")
	}
	if dp.hasFocusResult {
		t.Error("hasFocusResult should be false")
	}
}

// ---------------------------------------------------------------------------
// 6. ToggleDone behaviour (toggle across span)
// ---------------------------------------------------------------------------

func TestToggleDone_SingleSlot(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(5, "Quick task", blockTask, 1)

	dp.toggleDone(5)
	if !dp.blocks[5].Done {
		t.Error("slot 5 should be done")
	}

	dp.toggleDone(5)
	if dp.blocks[5].Done {
		t.Error("slot 5 should be undone after second toggle")
	}
}

func TestToggleDone_MultiSlotSpan(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(10, "Long task", blockTask, 3)

	// Toggle from the middle
	dp.toggleDone(11)

	for i := 10; i <= 12; i++ {
		if !dp.blocks[i].Done {
			t.Errorf("slot %d should be done", i)
		}
	}
}

func TestToggleDone_EmptySlotNoOp(t *testing.T) {
	dp := newTestPlanner(t)
	dp.toggleDone(0) // empty slot

	if dp.blocks[0].Done {
		t.Error("empty slot should not become done")
	}
}

func TestToggleDone_OutOfBounds(t *testing.T) {
	dp := newTestPlanner(t)
	// Should not panic
	dp.toggleDone(-1)
	dp.toggleDone(100)
}

// ---------------------------------------------------------------------------
// 7. Time slot allocation (30-min blocks from 6:00-22:00)
// ---------------------------------------------------------------------------

func TestTimeToSlotIndex(t *testing.T) {
	dp := newTestPlanner(t)

	tests := []struct {
		time string
		want int
	}{
		{"06:00", 0},
		{"06:30", 1},
		{"07:00", 2},
		{"12:00", 12},
		{"21:00", 30},
		{"21:30", 31},
	}

	for _, tc := range tests {
		got := dp.timeToSlotIndex(tc.time)
		if got != tc.want {
			t.Errorf("timeToSlotIndex(%q) = %d, want %d", tc.time, got, tc.want)
		}
	}
}

func TestTimeToSlotIndex_OutOfRange(t *testing.T) {
	dp := newTestPlanner(t)

	outOfRange := []string{"05:00", "05:59", "22:00", "23:00", "00:00"}
	for _, ts := range outOfRange {
		got := dp.timeToSlotIndex(ts)
		if got != -1 {
			t.Errorf("timeToSlotIndex(%q) = %d, want -1", ts, got)
		}
	}
}

func TestTimeToSlotIndex_InvalidFormat(t *testing.T) {
	dp := newTestPlanner(t)

	invalid := []string{"", "abc", "12", "25:00"}
	for _, ts := range invalid {
		got := dp.timeToSlotIndex(ts)
		if got != -1 {
			t.Errorf("timeToSlotIndex(%q) = %d, want -1", ts, got)
		}
	}
}

func TestSlotTime(t *testing.T) {
	dp := newTestPlanner(t)

	tests := []struct {
		idx  int
		want string
	}{
		{0, "06:00"},
		{1, "06:30"},
		{6, "09:00"},
		{31, "21:30"},
	}

	for _, tc := range tests {
		got := dp.slotTime(tc.idx)
		if got != tc.want {
			t.Errorf("slotTime(%d) = %q, want %q", tc.idx, got, tc.want)
		}
	}
}

func TestSlotTime_OutOfBounds(t *testing.T) {
	dp := newTestPlanner(t)

	if got := dp.slotTime(-1); got != "??:??" {
		t.Errorf("slotTime(-1) = %q, want '??:??'", got)
	}
	if got := dp.slotTime(100); got != "??:??" {
		t.Errorf("slotTime(100) = %q, want '??:??'", got)
	}
}

func TestSlotTimeEnd(t *testing.T) {
	dp := newTestPlanner(t)

	tests := []struct {
		idx  int
		want string
	}{
		{0, "06:30"},  // 06:00 + 30 min
		{1, "07:00"},  // 06:30 + 30 min
		{31, "22:00"}, // 21:30 + 30 min
	}

	for _, tc := range tests {
		got := dp.slotTimeEnd(tc.idx)
		if got != tc.want {
			t.Errorf("slotTimeEnd(%d) = %q, want %q", tc.idx, got, tc.want)
		}
	}
}

func TestSlotTimeEnd_OutOfBounds(t *testing.T) {
	dp := newTestPlanner(t)

	if got := dp.slotTimeEnd(-1); got != "??:??" {
		t.Errorf("slotTimeEnd(-1) = %q, want '??:??'", got)
	}
	if got := dp.slotTimeEnd(100); got != "??:??" {
		t.Errorf("slotTimeEnd(100) = %q, want '??:??'", got)
	}
}

func TestPlaceBlock_SingleSlot(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(0, "Quick task", blockTask, 1)

	if dp.blocks[0].TaskText != "Quick task" {
		t.Errorf("expected 'Quick task', got %q", dp.blocks[0].TaskText)
	}
	if dp.blocks[0].TaskType != blockTask {
		t.Errorf("expected blockTask, got %d", dp.blocks[0].TaskType)
	}
	// Slot 1 should remain empty
	if dp.blocks[1].TaskType != blockEmpty {
		t.Error("adjacent slot should remain empty")
	}
}

func TestPlaceBlock_MultiSlot(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(4, "Long meeting", blockEvent, 3)

	for i := 4; i <= 6; i++ {
		if dp.blocks[i].TaskText != "Long meeting" {
			t.Errorf("slot %d should have 'Long meeting', got %q", i, dp.blocks[i].TaskText)
		}
		if dp.blocks[i].TaskType != blockEvent {
			t.Errorf("slot %d should be blockEvent", i)
		}
	}
	// Slot 7 should remain empty
	if dp.blocks[7].TaskType != blockEmpty {
		t.Error("slot 7 should remain empty")
	}
}

func TestPlaceBlock_ClampAtEnd(t *testing.T) {
	dp := newTestPlanner(t)
	// Place a 4-slot block starting at slot 30 -- should clamp at 31
	dp.placeBlock(30, "Late work", blockTask, 4)

	if dp.blocks[30].TaskText != "Late work" {
		t.Error("slot 30 should have the task")
	}
	if dp.blocks[31].TaskText != "Late work" {
		t.Error("slot 31 should have the task")
	}
	// Only 2 slots should be filled (30, 31), no out-of-bounds
}

func TestPlaceBlock_NegativeIndex(t *testing.T) {
	dp := newTestPlanner(t)
	// Should not panic
	dp.placeBlock(-1, "Nowhere", blockTask, 1)
	// Verify no block was modified
	for i, b := range dp.blocks {
		if b.TaskType != blockEmpty {
			t.Errorf("block %d should be empty after negative index place", i)
		}
	}
}

func TestPlaceBlock_OutOfBoundsIndex(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(100, "Nowhere", blockTask, 1)
	for i, b := range dp.blocks {
		if b.TaskType != blockEmpty {
			t.Errorf("block %d should be empty after out-of-bounds place", i)
		}
	}
}

func TestPlaceBlock_ZeroSlots(t *testing.T) {
	dp := newTestPlanner(t)
	// slots < 1 should be treated as 1
	dp.placeBlock(5, "One slot", blockTask, 0)
	if dp.blocks[5].TaskText != "One slot" {
		t.Error("block 5 should be placed even when slots=0 (clamped to 1)")
	}
	if dp.blocks[6].TaskType != blockEmpty {
		t.Error("only 1 slot should be filled")
	}
}

func TestPlaceBlock_SetsColor(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(0, "Event", blockEvent, 1)
	if dp.blocks[0].Color != lavender {
		t.Errorf("event block should have lavender color, got %v", dp.blocks[0].Color)
	}
}

func TestPlaceBlock_SetsDoneFalse(t *testing.T) {
	dp := newTestPlanner(t)
	// Pre-set Done to true
	dp.blocks[0].Done = true
	dp.placeBlock(0, "Override", blockTask, 1)
	if dp.blocks[0].Done {
		t.Error("placeBlock should set Done=false")
	}
}

// ---------------------------------------------------------------------------
// 8. Day navigation
// ---------------------------------------------------------------------------

func TestDayNavigation_Forward(t *testing.T) {
	dp := newTestPlanner(t)
	dp.active = true
	original := dp.date

	dp.date = dp.date.AddDate(0, 0, 1)
	dp.reloadDay()

	expected := original.AddDate(0, 0, 1)
	if dp.date != expected {
		t.Errorf("expected date %v, got %v", expected, dp.date)
	}
	if len(dp.blocks) != 32 {
		t.Errorf("blocks should be reinitialised, got %d", len(dp.blocks))
	}
}

func TestDayNavigation_Backward(t *testing.T) {
	dp := newTestPlanner(t)
	dp.active = true
	original := dp.date

	dp.date = dp.date.AddDate(0, 0, -1)
	dp.reloadDay()

	expected := original.AddDate(0, 0, -1)
	if dp.date != expected {
		t.Errorf("expected date %v, got %v", expected, dp.date)
	}
}

func TestReloadDay_ResetsState(t *testing.T) {
	dp := newTestPlanner(t)
	dp.cursor = 15
	dp.scroll = 5
	dp.moving = true
	dp.moveFrom = 10
	dp.adding = true
	dp.addBuf = "half typed"
	dp.modified = true
	dp.unscheduled = []PlannerTask{{Text: "old"}}

	dp.reloadDay()

	if dp.cursor != 0 {
		t.Error("cursor should reset to 0")
	}
	if dp.scroll != 0 {
		t.Error("scroll should reset to 0")
	}
	if dp.moving {
		t.Error("moving should be false")
	}
	if dp.moveFrom != -1 {
		t.Error("moveFrom should be -1")
	}
	if dp.adding {
		t.Error("adding should be false")
	}
	if dp.addBuf != "" {
		t.Error("addBuf should be empty")
	}
	if dp.modified {
		t.Error("modified should be false")
	}
	if dp.unscheduled != nil {
		t.Error("unscheduled should be nil")
	}
}

func TestReloadDay_ReinitialiseBlocks(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(0, "Old task", blockTask, 3)

	dp.reloadDay()

	for i, b := range dp.blocks {
		if b.TaskType != blockEmpty {
			t.Errorf("block %d should be empty after reloadDay, got type %d", i, b.TaskType)
		}
	}
}

// ---------------------------------------------------------------------------
// 9. Block movement and deletion
// ---------------------------------------------------------------------------

func TestDeleteBlock_SingleSlot(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(5, "Delete me", blockTask, 1)
	dp.deleteBlock(5)

	if dp.blocks[5].TaskType != blockEmpty {
		t.Error("slot 5 should be empty after delete")
	}
	if dp.blocks[5].TaskText != "" {
		t.Error("text should be cleared")
	}
}

func TestDeleteBlock_MultiSlotSpan(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(10, "Big block", blockTask, 3)

	// Delete from the middle of the span
	dp.deleteBlock(11)

	for i := 10; i <= 12; i++ {
		if dp.blocks[i].TaskType != blockEmpty {
			t.Errorf("slot %d should be empty after deleting span", i)
		}
		if dp.blocks[i].TaskText != "" {
			t.Errorf("slot %d text should be empty, got %q", i, dp.blocks[i].TaskText)
		}
	}
}

func TestDeleteBlock_ClearsAllFields(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(5, "Full block", blockTask, 1)
	dp.blocks[5].Priority = 3
	dp.blocks[5].Done = true

	dp.deleteBlock(5)

	b := dp.blocks[5]
	if b.TaskType != blockEmpty {
		t.Error("TaskType should be blockEmpty")
	}
	if b.TaskText != "" {
		t.Error("TaskText should be empty")
	}
	if b.Done {
		t.Error("Done should be false")
	}
	if b.Color != "" {
		t.Error("Color should be empty")
	}
	if b.Priority != 0 {
		t.Error("Priority should be 0")
	}
}

func TestDeleteBlock_EmptySlotNoOp(t *testing.T) {
	dp := newTestPlanner(t)
	// Should not panic on empty slot
	dp.deleteBlock(0)
	if dp.blocks[0].TaskType != blockEmpty {
		t.Error("should remain empty")
	}
}

func TestDeleteBlock_OutOfBounds(t *testing.T) {
	dp := newTestPlanner(t)
	// Should not panic
	dp.deleteBlock(-1)
	dp.deleteBlock(100)
}

func TestDeleteBlock_DoesNotAffectAdjacentDifferentBlock(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(5, "Block A", blockTask, 2)
	dp.placeBlock(7, "Block B", blockTask, 2)

	dp.deleteBlock(5) // should only clear slots 5-6

	if dp.blocks[7].TaskText != "Block B" {
		t.Error("adjacent different block should not be affected")
	}
	if dp.blocks[8].TaskText != "Block B" {
		t.Error("adjacent different block slot 8 should not be affected")
	}
}

func TestMoveBlock_Basic(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(2, "Move me", blockTask, 1)

	dp.moveBlock(2, 10)

	if dp.blocks[2].TaskType != blockEmpty {
		t.Error("source slot should be cleared")
	}
	if dp.blocks[10].TaskText != "Move me" {
		t.Errorf("destination should have 'Move me', got %q", dp.blocks[10].TaskText)
	}
	if dp.blocks[10].TaskType != blockTask {
		t.Error("destination should be blockTask")
	}
}

func TestMoveBlock_MultiSlotSpan(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(0, "Span", blockEvent, 3)

	dp.moveBlock(1, 20) // move from middle of span

	// Source slots cleared
	for i := 0; i <= 2; i++ {
		if dp.blocks[i].TaskType != blockEmpty {
			t.Errorf("source slot %d should be cleared", i)
		}
	}
	// Destination slots filled
	for i := 20; i <= 22; i++ {
		if dp.blocks[i].TaskText != "Span" {
			t.Errorf("dest slot %d should have 'Span', got %q", i, dp.blocks[i].TaskText)
		}
	}
}

func TestMoveBlock_SamePosition_NoOp(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(5, "Stay", blockTask, 1)

	dp.moveBlock(5, 5)

	if dp.blocks[5].TaskText != "Stay" {
		t.Error("block should remain in place")
	}
}

func TestMoveBlock_EmptySource_NoOp(t *testing.T) {
	dp := newTestPlanner(t)
	dp.moveBlock(0, 10) // source is empty
	if dp.blocks[10].TaskType != blockEmpty {
		t.Error("destination should remain empty when moving empty source")
	}
}

func TestMoveBlock_OutOfBounds(t *testing.T) {
	dp := newTestPlanner(t)
	// Should not panic
	dp.moveBlock(-1, 5)
	dp.moveBlock(5, -1)
	dp.moveBlock(100, 5)
	dp.moveBlock(5, 100)
}

func TestMoveBlock_PreservesProperties(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(3, "Tracked", blockTask, 1)
	dp.blocks[3].Priority = 4
	dp.blocks[3].Done = true

	dp.moveBlock(3, 20)

	b := dp.blocks[20]
	if b.Priority != 4 {
		t.Errorf("expected priority 4, got %d", b.Priority)
	}
	if !b.Done {
		t.Error("expected Done=true")
	}
	if b.TaskType != blockTask {
		t.Error("expected blockTask type preserved")
	}
}

func TestMoveBlock_ClampAtEnd(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(0, "Big span", blockTask, 3)

	// Move to near the end - span should be clamped
	dp.moveBlock(0, 30)

	// Slots 30-31 should be filled (clamped from 3 to 2)
	if dp.blocks[30].TaskText != "Big span" {
		t.Error("slot 30 should have block")
	}
	if dp.blocks[31].TaskText != "Big span" {
		t.Error("slot 31 should have block")
	}
	// Source should be clear
	for i := 0; i <= 2; i++ {
		if dp.blocks[i].TaskType != blockEmpty {
			t.Errorf("source slot %d should be cleared", i)
		}
	}
}

// ---------------------------------------------------------------------------
// 10. Edge cases
// ---------------------------------------------------------------------------

func TestEdgeCase_EmptyDay(t *testing.T) {
	dp := openTestPlanner(t, nil, nil, nil)

	if len(dp.blocks) != 32 {
		t.Fatalf("expected 32 blocks, got %d", len(dp.blocks))
	}
	if dp.doneCount != 0 {
		t.Errorf("expected doneCount 0, got %d", dp.doneCount)
	}
	if dp.totalCount != 0 {
		t.Errorf("expected totalCount 0, got %d", dp.totalCount)
	}
	if len(dp.unscheduled) != 0 {
		t.Errorf("expected 0 unscheduled, got %d", len(dp.unscheduled))
	}
}

func TestEdgeCase_AllSlotsFilled(t *testing.T) {
	dp := newTestPlanner(t)

	// Fill every slot with unique text to avoid span grouping
	for i := 0; i < len(dp.blocks); i++ {
		dp.blocks[i].TaskText = "Busy"
		dp.blocks[i].TaskType = blockTask
	}

	// isTextScheduled should return true for "Busy"
	if !dp.isTextScheduled("Busy") {
		t.Error("'Busy' should be scheduled")
	}
	if dp.isTextScheduled("Not here") {
		t.Error("'Not here' should not be scheduled")
	}
}

func TestEdgeCase_ToggleDone_FullSpan(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(0, "All day", blockTask, 32)
	dp.toggleDone(15) // toggle from the middle

	for i := 0; i < 32; i++ {
		if !dp.blocks[i].Done {
			t.Errorf("slot %d should be done", i)
		}
	}
}

func TestEdgeCase_ToggleDone_UndoToggle(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(5, "Toggle twice", blockTask, 2)

	dp.toggleDone(5)
	if !dp.blocks[5].Done || !dp.blocks[6].Done {
		t.Error("both slots should be done after first toggle")
	}

	dp.toggleDone(6) // toggle again from second slot
	if dp.blocks[5].Done || dp.blocks[6].Done {
		t.Error("both slots should be un-done after second toggle")
	}
}

func TestEdgeCase_SpanDuration(t *testing.T) {
	dp := newTestPlanner(t)

	// Empty slot returns 30
	if got := dp.spanDuration(0); got != 30 {
		t.Errorf("empty slot span = %d, want 30", got)
	}

	dp.placeBlock(10, "Hour block", blockTask, 2)
	if got := dp.spanDuration(10); got != 60 {
		t.Errorf("2-slot span = %d, want 60", got)
	}
	if got := dp.spanDuration(11); got != 60 {
		t.Errorf("2-slot span from idx 11 = %d, want 60", got)
	}

	// Out of bounds
	if got := dp.spanDuration(-1); got != 30 {
		t.Errorf("out-of-bounds span = %d, want 30", got)
	}
	if got := dp.spanDuration(100); got != 30 {
		t.Errorf("out-of-bounds span (100) = %d, want 30", got)
	}
}

func TestEdgeCase_RecountProgress(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(0, "Task A", blockTask, 2)
	dp.placeBlock(4, "Task B", blockEvent, 1)
	dp.toggleDone(0) // marks Task A (2 slots) as done

	dp.recountProgress()

	// Task A occupies 2 slots with different (hour, halfhour) keys -> 2 unique entries
	// Task B occupies 1 slot -> 1 unique entry
	// Total = 3, done = 2 (both Task A slots are done)
	if dp.totalCount != 3 {
		t.Errorf("expected totalCount=3, got %d", dp.totalCount)
	}
	if dp.doneCount != 2 {
		t.Errorf("expected doneCount=2, got %d", dp.doneCount)
	}
}

func TestEdgeCase_RecountProgress_EmptySchedule(t *testing.T) {
	dp := newTestPlanner(t)
	dp.recountProgress()

	if dp.totalCount != 0 || dp.doneCount != 0 {
		t.Errorf("empty schedule should have 0/0, got %d/%d", dp.doneCount, dp.totalCount)
	}
}

func TestEdgeCase_PastDate(t *testing.T) {
	dp := newTestPlanner(t)
	dp.date = time.Date(2020, 1, 1, 0, 0, 0, 0, time.Local)

	// Blocks should still initialise correctly regardless of date
	if len(dp.blocks) != 32 {
		t.Errorf("expected 32 blocks for past date, got %d", len(dp.blocks))
	}

	// Can still place and manipulate blocks
	dp.placeBlock(0, "Past task", blockTask, 1)
	if dp.blocks[0].TaskText != "Past task" {
		t.Error("should be able to place blocks on past dates")
	}
}

func TestEdgeCase_SetSize(t *testing.T) {
	dp := newTestPlanner(t)
	dp.SetSize(120, 40)

	if dp.width != 120 {
		t.Errorf("expected width=120, got %d", dp.width)
	}
	if dp.height != 40 {
		t.Errorf("expected height=40, got %d", dp.height)
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestBlockColor(t *testing.T) {
	if got := blockColor(blockEvent, 0); got != lavender {
		t.Errorf("blockEvent color = %v, want lavender", got)
	}
	if got := blockColor(blockBreak, 0); got != overlay0 {
		t.Errorf("blockBreak color = %v, want overlay0", got)
	}
	if got := blockColor(blockFocus, 0); got != green {
		t.Errorf("blockFocus color = %v, want green", got)
	}
	if got := blockColor(blockEmpty, 0); got != text {
		t.Errorf("blockEmpty color = %v, want text", got)
	}
}

func TestTaskPriorityColor(t *testing.T) {
	if got := taskPriorityColor(4); got != red {
		t.Errorf("priority 4 = %v, want red", got)
	}
	if got := taskPriorityColor(3); got != peach {
		t.Errorf("priority 3 = %v, want peach", got)
	}
	if got := taskPriorityColor(2); got != yellow {
		t.Errorf("priority 2 = %v, want yellow", got)
	}
	if got := taskPriorityColor(1); got != blue {
		t.Errorf("priority 1 = %v, want blue", got)
	}
	if got := taskPriorityColor(0); got != text {
		t.Errorf("priority 0 = %v, want text", got)
	}
}

func TestBlockTypeTag(t *testing.T) {
	tests := []struct {
		bt   blockType
		want string
	}{
		{blockTask, "[T]"},
		{blockEvent, "[E]"},
		{blockBreak, "[B]"},
		{blockFocus, "[F]"},
		{blockEmpty, ""},
	}
	for _, tc := range tests {
		if got := blockTypeTag(tc.bt); got != tc.want {
			t.Errorf("blockTypeTag(%d) = %q, want %q", tc.bt, got, tc.want)
		}
	}
}

func TestBlockTypeName(t *testing.T) {
	tests := []struct {
		bt   blockType
		want string
	}{
		{blockTask, "task"},
		{blockEvent, "event"},
		{blockBreak, "break"},
		{blockFocus, "focus"},
		{blockEmpty, ""},
	}
	for _, tc := range tests {
		if got := blockTypeName(tc.bt); got != tc.want {
			t.Errorf("blockTypeName(%d) = %q, want %q", tc.bt, got, tc.want)
		}
	}
}

func TestParseBlockType(t *testing.T) {
	tests := []struct {
		input string
		want  blockType
	}{
		{"task", blockTask},
		{"Task", blockTask},
		{"  TASK  ", blockTask},
		{"event", blockEvent},
		{"break", blockBreak},
		{"focus", blockFocus},
		{"unknown", blockTask}, // default
		{"", blockTask},        // default
	}
	for _, tc := range tests {
		if got := parseBlockType(tc.input); got != tc.want {
			t.Errorf("parseBlockType(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// File I/O round-trip
// ---------------------------------------------------------------------------

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(0, "Morning email", blockTask, 2)
	dp.placeBlock(6, "Standup", blockEvent, 1)
	dp.placeBlock(10, "Lunch", blockBreak, 2)
	dp.toggleDone(0) // mark morning email as done

	dp.habits = []PlannerHabit{
		{Name: "Exercise", Done: true, Streak: 5},
		{Name: "Meditate", Done: false, Streak: 0},
	}

	dp.saveToFile()

	// Verify file exists
	fp := dp.plannerFilePath()
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		t.Fatalf("planner file should exist at %s", fp)
	}

	// Create a new planner for the same date and load
	dp2 := newTestPlanner(t)
	dp2.vaultRoot = dp.vaultRoot
	dp2.date = dp.date
	dp2.habits = []PlannerHabit{
		{Name: "Exercise"},
		{Name: "Meditate"},
	}
	dp2.initBlocks()
	loaded := dp2.loadFromFile()
	if !loaded {
		t.Fatal("loadFromFile should return true")
	}

	// Check schedule blocks
	if dp2.blocks[0].TaskText != "Morning email" {
		t.Errorf("slot 0 text = %q, want 'Morning email'", dp2.blocks[0].TaskText)
	}
	if !dp2.blocks[0].Done {
		t.Error("slot 0 should be done")
	}
	if dp2.blocks[6].TaskText != "Standup" {
		t.Errorf("slot 6 text = %q, want 'Standup'", dp2.blocks[6].TaskText)
	}
	if dp2.blocks[6].TaskType != blockEvent {
		t.Error("slot 6 should be blockEvent")
	}
	if dp2.blocks[10].TaskText != "Lunch" {
		t.Errorf("slot 10 text = %q, want 'Lunch'", dp2.blocks[10].TaskText)
	}
	if dp2.blocks[10].TaskType != blockBreak {
		t.Error("slot 10 should be blockBreak")
	}

	// Check habits round-trip
	if len(dp2.habits) != 2 {
		t.Fatalf("expected 2 habits, got %d", len(dp2.habits))
	}
	if !dp2.habits[0].Done {
		t.Error("Exercise habit should be done after reload")
	}
	if dp2.habits[0].Streak != 5 {
		t.Errorf("Exercise streak = %d, want 5", dp2.habits[0].Streak)
	}
}

func TestLoadFromFile_NoFile(t *testing.T) {
	dp := newTestPlanner(t)
	loaded := dp.loadFromFile()
	if loaded {
		t.Error("loadFromFile should return false when no file exists")
	}
}

func TestPlannerFilePath(t *testing.T) {
	dp := newTestPlanner(t)
	fp := dp.plannerFilePath()
	expected := filepath.Join(dp.vaultRoot, "Daily Planner", "2026-03-08.md")
	if fp != expected {
		t.Errorf("plannerFilePath = %q, want %q", fp, expected)
	}
}

func TestPlannerDir(t *testing.T) {
	dp := newTestPlanner(t)
	dir := dp.plannerDir()
	expected := filepath.Join(dp.vaultRoot, "Daily Planner")
	if dir != expected {
		t.Errorf("plannerDir = %q, want %q", dir, expected)
	}
}

func TestSaveIfModified_SavesWhenModified(t *testing.T) {
	dp := newTestPlanner(t)
	dp.placeBlock(0, "Save test", blockTask, 1)
	dp.modified = true

	dp.saveIfModified()

	if dp.modified {
		t.Error("modified should be false after saveIfModified")
	}
	fp := dp.plannerFilePath()
	if _, err := os.Stat(fp); os.IsNotExist(err) {
		t.Error("file should be created")
	}
}

func TestSaveIfModified_SkipsWhenNotModified(t *testing.T) {
	dp := newTestPlanner(t)
	dp.modified = false

	dp.saveIfModified()

	fp := dp.plannerFilePath()
	if _, err := os.Stat(fp); !os.IsNotExist(err) {
		t.Error("file should not be created when not modified")
	}
}

// ---------------------------------------------------------------------------
// isTextScheduled
// ---------------------------------------------------------------------------

func TestIsTextScheduled(t *testing.T) {
	dp := newTestPlanner(t)

	if dp.isTextScheduled("anything") {
		t.Error("empty schedule should not have any text scheduled")
	}

	dp.placeBlock(5, "Scheduled item", blockTask, 1)

	if !dp.isTextScheduled("Scheduled item") {
		t.Error("should find scheduled text")
	}
	if dp.isTextScheduled("Other item") {
		t.Error("should not find unscheduled text")
	}
}

// ---------------------------------------------------------------------------
// VisibleSlots
// ---------------------------------------------------------------------------

func TestVisibleSlots(t *testing.T) {
	dp := newTestPlanner(t)

	dp.height = 50
	vis := dp.visibleSlots()
	if vis != 50-14 {
		t.Errorf("visibleSlots() = %d, want %d", vis, 50-14)
	}

	// Small height clamped to 8
	dp.height = 10
	vis = dp.visibleSlots()
	if vis != 8 {
		t.Errorf("visibleSlots() for small height = %d, want 8", vis)
	}

	dp.height = 22 // 22 - 14 = 8 exactly
	vis = dp.visibleSlots()
	if vis != 8 {
		t.Errorf("visibleSlots() at boundary = %d, want 8", vis)
	}
}

// ---------------------------------------------------------------------------
// adjustScroll
// ---------------------------------------------------------------------------

func TestAdjustScroll_CursorBelowView(t *testing.T) {
	dp := newTestPlanner(t)
	dp.height = 22 // visibleSlots = 8
	dp.cursor = 20
	dp.scroll = 0

	dp.adjustScroll()

	// cursor should be within [scroll, scroll+8)
	if dp.cursor < dp.scroll || dp.cursor >= dp.scroll+dp.visibleSlots() {
		t.Errorf("cursor %d should be in view [%d, %d)", dp.cursor, dp.scroll, dp.scroll+dp.visibleSlots())
	}
}

func TestAdjustScroll_CursorAboveView(t *testing.T) {
	dp := newTestPlanner(t)
	dp.height = 22 // visibleSlots = 8
	dp.cursor = 2
	dp.scroll = 10

	dp.adjustScroll()

	if dp.scroll != 2 {
		t.Errorf("scroll should be %d, got %d", 2, dp.scroll)
	}
}

// ---------------------------------------------------------------------------
// parseScheduleLine / parseHabitLine (internal parsing)
// ---------------------------------------------------------------------------

func TestParseScheduleLine_ValidLine(t *testing.T) {
	dp := newTestPlanner(t)
	dp.parseScheduleLine("- 09:00-10:00 | Fix auth bug | task | done")

	// 09:00 = slot 6, 10:00 = slot 8
	for i := 6; i < 8; i++ {
		if dp.blocks[i].TaskText != "Fix auth bug" {
			t.Errorf("slot %d text = %q, want 'Fix auth bug'", i, dp.blocks[i].TaskText)
		}
		if dp.blocks[i].TaskType != blockTask {
			t.Errorf("slot %d type = %d, want blockTask", i, dp.blocks[i].TaskType)
		}
		if !dp.blocks[i].Done {
			t.Errorf("slot %d should be done", i)
		}
	}
}

func TestParseScheduleLine_NotDone(t *testing.T) {
	dp := newTestPlanner(t)
	dp.parseScheduleLine("- 14:00-15:00 | Review PR | task")

	// 14:00 = slot 16
	if dp.blocks[16].Done {
		t.Error("block should not be done")
	}
	if dp.blocks[16].TaskText != "Review PR" {
		t.Errorf("text = %q, want 'Review PR'", dp.blocks[16].TaskText)
	}
}

func TestParseScheduleLine_IgnoresNonListLines(t *testing.T) {
	dp := newTestPlanner(t)
	dp.parseScheduleLine("some random text")
	dp.parseScheduleLine("## Schedule")
	dp.parseScheduleLine("")

	for i, b := range dp.blocks {
		if b.TaskType != blockEmpty {
			t.Errorf("slot %d should be empty", i)
		}
	}
}

func TestParseScheduleLine_InvalidTimeRange(t *testing.T) {
	dp := newTestPlanner(t)
	dp.parseScheduleLine("- 25:00-26:00 | Bad time | task")

	for i, b := range dp.blocks {
		if b.TaskType != blockEmpty {
			t.Errorf("slot %d should be empty for invalid time", i)
		}
	}
}

func TestParseHabitLine_Done(t *testing.T) {
	dp := newTestPlanner(t)
	dp.habits = []PlannerHabit{{Name: "Exercise"}}

	dp.parseHabitLine("- [x] Exercise (streak: 5)")

	if !dp.habits[0].Done {
		t.Error("Exercise should be done")
	}
	if dp.habits[0].Streak != 5 {
		t.Errorf("streak = %d, want 5", dp.habits[0].Streak)
	}
}

func TestParseHabitLine_NotDone(t *testing.T) {
	dp := newTestPlanner(t)
	dp.habits = []PlannerHabit{{Name: "Read"}}

	dp.parseHabitLine("- [ ] Read (streak: 2)")

	if dp.habits[0].Done {
		t.Error("Read should not be done")
	}
	if dp.habits[0].Streak != 2 {
		t.Errorf("streak = %d, want 2", dp.habits[0].Streak)
	}
}

func TestParseHabitLine_NewHabit(t *testing.T) {
	dp := newTestPlanner(t)
	dp.habits = nil

	dp.parseHabitLine("- [x] Meditate (streak: 3)")

	if len(dp.habits) != 1 {
		t.Fatalf("expected 1 habit, got %d", len(dp.habits))
	}
	if dp.habits[0].Name != "Meditate" {
		t.Errorf("name = %q, want 'Meditate'", dp.habits[0].Name)
	}
	if !dp.habits[0].Done {
		t.Error("should be done")
	}
	if dp.habits[0].Streak != 3 {
		t.Errorf("streak = %d, want 3", dp.habits[0].Streak)
	}
}

func TestParseHabitLine_IgnoresNonHabitLines(t *testing.T) {
	dp := newTestPlanner(t)
	dp.habits = nil

	dp.parseHabitLine("some text")
	dp.parseHabitLine("")
	dp.parseHabitLine("- regular list item")

	if len(dp.habits) != 0 {
		t.Errorf("expected 0 habits, got %d", len(dp.habits))
	}
}
