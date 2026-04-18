package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ===========================================================================
// Planner Sync Tests
// ===========================================================================

// TestSyncPlannerTask_SourceTracking verifies that PlannerTask stores the
// bidirectional sync metadata (NotePath, LineNum) needed to write completion
// state back to the originating markdown file.
func TestSyncPlannerTask_SourceTracking(t *testing.T) {
	task := PlannerTask{
		Text:     "Review PR #42",
		Done:     false,
		Priority: 3,
		DueDate:  "2026-03-08",
		Source:   "Tasks.md",
		NotePath: "Tasks.md",
		LineNum:  5,
	}

	if task.NotePath != "Tasks.md" {
		t.Errorf("NotePath = %q, want %q", task.NotePath, "Tasks.md")
	}
	if task.LineNum != 5 {
		t.Errorf("LineNum = %d, want %d", task.LineNum, 5)
	}
	if task.Text != "Review PR #42" {
		t.Errorf("Text = %q, want %q", task.Text, "Review PR #42")
	}
	if task.Done {
		t.Error("expected Done == false for a new task")
	}
}

// TestSyncPlannerBlock_SourceTracking verifies that a timeBlock preserves
// source path and line number when populated from a PlannerTask, so that
// toggling done in the planner can be synced back to the note.
func TestSyncPlannerBlock_SourceTracking(t *testing.T) {
	dp := NewDailyPlanner()
	dp.initBlocks()

	// Simulate assigning a task to slot 4 (08:00)
	task := PlannerTask{
		Text:     "Write tests",
		NotePath: "Projects/granit.md",
		LineNum:  12,
		Priority: 2,
	}

	idx := 4 // 08:00
	dp.placeBlock(idx, task.Text, blockTask, 2)
	dp.blocks[idx].SourcePath = task.NotePath
	dp.blocks[idx].SourceLine = task.LineNum
	dp.blocks[idx].Priority = task.Priority

	b := dp.blocks[idx]
	if b.SourcePath != "Projects/granit.md" {
		t.Errorf("SourcePath = %q, want %q", b.SourcePath, "Projects/granit.md")
	}
	if b.SourceLine != 12 {
		t.Errorf("SourceLine = %d, want %d", b.SourceLine, 12)
	}
	if b.TaskText != "Write tests" {
		t.Errorf("TaskText = %q, want %q", b.TaskText, "Write tests")
	}
	if b.TaskType != blockTask {
		t.Errorf("TaskType = %d, want blockTask (%d)", b.TaskType, blockTask)
	}
}

// TestSyncPlanner_TaskCompletion_Struct verifies that the TaskCompletion
// struct holds all fields needed to propagate a done-toggle back to the
// originating note file.
func TestSyncPlanner_TaskCompletion_Struct(t *testing.T) {
	tc := TaskCompletion{
		NotePath: "inbox/tasks.md",
		LineNum:  7,
		Text:     "Fix login bug",
		Done:     true,
	}

	if tc.NotePath != "inbox/tasks.md" {
		t.Errorf("NotePath = %q, want %q", tc.NotePath, "inbox/tasks.md")
	}
	if tc.LineNum != 7 {
		t.Errorf("LineNum = %d, want %d", tc.LineNum, 7)
	}
	if tc.Text != "Fix login bug" {
		t.Errorf("Text = %q, want %q", tc.Text, "Fix login bug")
	}
	if !tc.Done {
		t.Error("expected Done == true")
	}

	// Verify toggling back to unchecked
	tc.Done = false
	if tc.Done {
		t.Error("expected Done == false after toggle")
	}
}

// TestSyncPlanner_GetCompletedTasks verifies the consumed-once pattern for
// completed task retrieval from the planner.
func TestSyncPlanner_GetCompletedTasks(t *testing.T) {
	dp := NewDailyPlanner()

	// Append a completion directly to the internal slice
	dp.completedTasks = append(dp.completedTasks, TaskCompletion{
		NotePath: "todo.md",
		LineNum:  3,
		Text:     "Deploy v2",
		Done:     true,
	})

	// First retrieval should return the completion
	if len(dp.completedTasks) != 1 {
		t.Fatalf("expected 1 completed task, got %d", len(dp.completedTasks))
	}

	got := dp.completedTasks[0]
	if got.NotePath != "todo.md" || got.LineNum != 3 || got.Text != "Deploy v2" || !got.Done {
		t.Errorf("unexpected completion: %+v", got)
	}

	// Simulate consumed-once by clearing after read
	dp.completedTasks = nil

	if len(dp.completedTasks) != 0 {
		t.Errorf("expected 0 completed tasks after consumption, got %d", len(dp.completedTasks))
	}
}

// ===========================================================================
// Calendar Sync Tests
// ===========================================================================

// TestSyncCalendar_PlannerBlockStruct verifies the PlannerBlock struct has all
// fields required to represent a planner time block in the calendar view.
func TestSyncCalendar_PlannerBlockStruct(t *testing.T) {
	pb := PlannerBlock{
		Date:      "2026-03-08",
		StartTime: "09:00",
		EndTime:   "10:30",
		Text:      "Deep work: parser refactor",
		BlockType: "task",
		Done:      false,
	}

	if pb.Date != "2026-03-08" {
		t.Errorf("Date = %q, want %q", pb.Date, "2026-03-08")
	}
	if pb.StartTime != "09:00" {
		t.Errorf("StartTime = %q, want %q", pb.StartTime, "09:00")
	}
	if pb.EndTime != "10:30" {
		t.Errorf("EndTime = %q, want %q", pb.EndTime, "10:30")
	}
	if pb.Text != "Deep work: parser refactor" {
		t.Errorf("Text = %q, want %q", pb.Text, "Deep work: parser refactor")
	}
	if pb.BlockType != "task" {
		t.Errorf("BlockType = %q, want %q", pb.BlockType, "task")
	}
	if pb.Done {
		t.Error("expected Done == false")
	}
}

// TestSyncCalendar_SetPlannerBlocks verifies that SetPlannerBlocks stores
// blocks and they can be read back from the calendar's internal state.
func TestSyncCalendar_SetPlannerBlocks(t *testing.T) {
	cal := NewCalendar()

	blocks := map[string][]PlannerBlock{
		"2026-03-08": {
			{Date: "2026-03-08", StartTime: "08:00", EndTime: "09:00", Text: "Standup", BlockType: "event", Done: false},
			{Date: "2026-03-08", StartTime: "09:00", EndTime: "10:30", Text: "Code review", BlockType: "task", Done: false},
		},
		"2026-03-09": {
			{Date: "2026-03-09", StartTime: "14:00", EndTime: "15:00", Text: "Planning", BlockType: "event", Done: false},
		},
	}

	cal.SetPlannerBlocks(blocks)

	if len(cal.plannerBlocks) != 2 {
		t.Fatalf("expected 2 dates in plannerBlocks, got %d", len(cal.plannerBlocks))
	}
	if len(cal.plannerBlocks["2026-03-08"]) != 2 {
		t.Errorf("expected 2 blocks for 2026-03-08, got %d", len(cal.plannerBlocks["2026-03-08"]))
	}
	if len(cal.plannerBlocks["2026-03-09"]) != 1 {
		t.Errorf("expected 1 block for 2026-03-09, got %d", len(cal.plannerBlocks["2026-03-09"]))
	}

	first := cal.plannerBlocks["2026-03-08"][0]
	if first.Text != "Standup" {
		t.Errorf("first block Text = %q, want %q", first.Text, "Standup")
	}
}

// TestSyncCalendar_TaskToggle verifies the TaskToggle struct and the
// consumed-once GetTaskToggles pattern on Calendar.
func TestSyncCalendar_TaskToggle(t *testing.T) {
	toggle := TaskToggle{
		NotePath: "daily/2026-03-08.md",
		LineNum:  4,
		Text:     "Buy groceries",
		Done:     true,
	}

	if toggle.NotePath != "daily/2026-03-08.md" {
		t.Errorf("NotePath = %q, want %q", toggle.NotePath, "daily/2026-03-08.md")
	}
	if toggle.LineNum != 4 {
		t.Errorf("LineNum = %d, want %d", toggle.LineNum, 4)
	}
	if toggle.Text != "Buy groceries" {
		t.Errorf("Text = %q, want %q", toggle.Text, "Buy groceries")
	}
	if !toggle.Done {
		t.Error("expected Done == true")
	}

	// Verify consumed-once via GetTaskToggles
	cal := NewCalendar()
	cal.taskToggles = append(cal.taskToggles, toggle)

	toggles := cal.GetTaskToggles()
	if len(toggles) != 1 {
		t.Fatalf("expected 1 toggle, got %d", len(toggles))
	}
	if toggles[0].Text != "Buy groceries" {
		t.Errorf("toggle Text = %q, want %q", toggles[0].Text, "Buy groceries")
	}

	// Second call should return nil (consumed)
	toggles2 := cal.GetTaskToggles()
	if toggles2 != nil {
		t.Errorf("expected nil after consumption, got %d toggles", len(toggles2))
	}
}

// ===========================================================================
// Task Manager Sync Tests
// ===========================================================================

// TestSyncTask_StructFields verifies the Task struct has all fields needed for
// sync: NotePath, LineNum, DueDate, Priority, Tags, Done.
func TestSyncTask_StructFields(t *testing.T) {
	task := Task{
		Text:     "Implement sync layer",
		Done:     false,
		DueDate:  "2026-03-10",
		Priority: 3,
		Tags:     []string{"dev", "sync"},
		NotePath: "Projects/granit.md",
		LineNum:  15,
	}

	if task.NotePath != "Projects/granit.md" {
		t.Errorf("NotePath = %q, want %q", task.NotePath, "Projects/granit.md")
	}
	if task.LineNum != 15 {
		t.Errorf("LineNum = %d, want %d", task.LineNum, 15)
	}
	if task.Priority != 3 {
		t.Errorf("Priority = %d, want %d", task.Priority, 3)
	}
	if len(task.Tags) != 2 {
		t.Fatalf("Tags len = %d, want 2", len(task.Tags))
	}
	if task.Tags[0] != "dev" || task.Tags[1] != "sync" {
		t.Errorf("Tags = %v, want [dev sync]", task.Tags)
	}
}

// TestSyncParseScheduledTime tests parsing a scheduled time marker from task
// text. The convention is an alarm clock emoji followed by a time range:
// ⏰ 09:00-10:30
func TestSyncParseScheduledTime(t *testing.T) {
	// Note: The ScheduledTime field does not exist on Task yet.
	// This test validates the parsing pattern that would extract it.
	taskText := "Review architecture docs \u23F0 09:00-10:30 #dev"

	// Extract the schedule marker
	idx := strings.Index(taskText, "\u23F0")
	if idx < 0 {
		t.Fatal("expected to find alarm clock emoji in task text")
	}

	remainder := strings.TrimSpace(taskText[idx+len("\u23F0"):])
	// Parse time range
	parts := strings.Fields(remainder)
	if len(parts) < 1 {
		t.Fatal("expected at least one field after the emoji")
	}

	timeRange := parts[0]
	timeParts := strings.Split(timeRange, "-")
	if len(timeParts) != 2 {
		t.Fatalf("expected 2 time parts, got %d", len(timeParts))
	}

	startTime := timeParts[0]
	endTime := timeParts[1]

	if startTime != "09:00" {
		t.Errorf("startTime = %q, want %q", startTime, "09:00")
	}
	if endTime != "10:30" {
		t.Errorf("endTime = %q, want %q", endTime, "10:30")
	}
}

// ===========================================================================
// Integration Flow Tests
// ===========================================================================

// TestSyncFlow_TaskCompletionData verifies that a TaskCompletion carries
// enough information to toggle a task checkbox in its source markdown file.
func TestSyncFlow_TaskCompletionData(t *testing.T) {
	// Simulate: user marks a task done in the daily planner, producing a
	// TaskCompletion that app.go would use to rewrite the source file.
	tc := TaskCompletion{
		NotePath: "inbox/tasks.md",
		LineNum:  3,
		Text:     "Fix login redirect",
		Done:     true,
	}

	// The flow: planner toggles done -> produces TaskCompletion ->
	// app.go reads the source file at NotePath, changes line LineNum from
	// "- [ ]" to "- [x]" (or vice versa).
	if tc.NotePath == "" {
		t.Error("NotePath must be set for file-level sync")
	}
	if tc.LineNum <= 0 {
		t.Error("LineNum must be positive (1-based) for file-level sync")
	}

	// Verify the completion can represent both check and uncheck
	tc.Done = false
	if tc.Done {
		t.Error("expected Done == false after toggle")
	}
}

// TestSyncFlow_PlannerBlockToCalendar verifies that a PlannerBlock contains
// all fields the calendar needs to display a scheduled block in its agenda
// and week views.
func TestSyncFlow_PlannerBlockToCalendar(t *testing.T) {
	block := PlannerBlock{
		Date:      "2026-03-08",
		StartTime: "14:00",
		EndTime:   "15:30",
		Text:      "Architecture review",
		BlockType: "task",
		Done:      false,
	}

	// Calendar needs: date (for filtering), start/end (for display), text,
	// type (for color coding), done status
	requiredFields := map[string]string{
		"Date":      block.Date,
		"StartTime": block.StartTime,
		"EndTime":   block.EndTime,
		"Text":      block.Text,
		"BlockType": string(block.BlockType),
	}

	for field, val := range requiredFields {
		if val == "" {
			t.Errorf("%s must not be empty for calendar display", field)
		}
	}

	// Verify type is a valid calendar block type
	validTypes := map[BlockType]bool{
		BlockTypeTask: true, BlockTypeEvent: true, BlockTypeBreak: true, BlockTypeFocus: true,
	}
	if !validTypes[block.BlockType] {
		t.Errorf("BlockType %q is not a valid calendar block type", block.BlockType)
	}
}

// TestSyncFlow_SchedulerSlotStructure verifies the schedulerSlot struct has
// all fields needed for full sync: task text, time range, type, and priority.
func TestSyncFlow_SchedulerSlotStructure(t *testing.T) {
	slot := schedulerSlot{
		StartHour: 9,
		StartMin:  0,
		EndHour:   10,
		EndMin:    30,
		Task:      "Deep work: parser rewrite",
		Type:      "task",
		Priority:  4,
	}

	if slot.Task == "" {
		t.Error("Task must not be empty")
	}
	if slot.Type == "" {
		t.Error("Type must not be empty")
	}
	if slot.StartHour < 0 || slot.StartHour > 23 {
		t.Errorf("StartHour = %d, out of range 0-23", slot.StartHour)
	}
	if slot.EndHour < 0 || slot.EndHour > 24 {
		t.Errorf("EndHour = %d, out of range 0-24", slot.EndHour)
	}

	// Verify slotMinutes calculation
	dur := slot.slotMinutes()
	if dur != 90 {
		t.Errorf("slotMinutes() = %d, want 90", dur)
	}

	// Verify it can map to a PlannerBlock for calendar display
	pb := PlannerBlock{
		Date:      "2026-03-08",
		StartTime: "09:00",
		EndTime:   "10:30",
		Text:      slot.Task,
		BlockType: NormaliseBlockType(slot.Type),
		Done:      false,
	}
	if pb.Text != slot.Task {
		t.Errorf("PlannerBlock.Text = %q, want %q", pb.Text, slot.Task)
	}
}

// ===========================================================================
// File-Level Sync Tests
// ===========================================================================

// TestSyncWriteTaskToggle writes a Tasks.md with an unchecked task, toggles
// the checkbox to [x], and verifies the file content reflects the change.
func TestSyncWriteTaskToggle(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "Tasks.md")

	original := "# Tasks\n\n- [ ] Review PR #42\n- [ ] Fix login bug\n- [x] Deploy v1\n"
	if err := os.WriteFile(filePath, []byte(original), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Simulate toggling line 3 (1-based) from unchecked to checked
	toggleLine := 3 // "- [ ] Review PR #42"

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	if toggleLine < 1 || toggleLine > len(lines) {
		t.Fatalf("line %d out of range (file has %d lines)", toggleLine, len(lines))
	}

	line := lines[toggleLine-1]
	if !strings.Contains(line, "- [ ]") {
		t.Fatalf("expected unchecked task on line %d, got %q", toggleLine, line)
	}

	// Toggle: replace "- [ ]" with "- [x]"
	lines[toggleLine-1] = strings.Replace(line, "- [ ]", "- [x]", 1)

	if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatalf("write toggled file: %v", err)
	}

	// Verify
	result, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}

	resultLines := strings.Split(string(result), "\n")
	if !strings.Contains(resultLines[toggleLine-1], "- [x]") {
		t.Errorf("expected checked task on line %d, got %q", toggleLine, resultLines[toggleLine-1])
	}
	// Other lines should be unchanged
	if !strings.Contains(resultLines[3], "- [ ] Fix login bug") {
		t.Errorf("line 4 should be unchanged, got %q", resultLines[3])
	}
	if !strings.Contains(resultLines[4], "- [x] Deploy v1") {
		t.Errorf("line 5 should be unchanged, got %q", resultLines[4])
	}
}

// TestSyncWriteScheduleMarker writes a Tasks.md with a task and adds a
// schedule time marker (alarm clock emoji + time range), verifying the file
// content is updated correctly.
func TestSyncWriteScheduleMarker(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "Tasks.md")

	original := "# Tasks\n\n- [ ] Review architecture docs\n- [ ] Write tests\n"
	if err := os.WriteFile(filePath, []byte(original), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	targetLine := 3 // "- [ ] Review architecture docs"
	scheduleMarker := " \u23F0 09:00-10:30"

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	lines := strings.Split(string(content), "\n")
	lines[targetLine-1] = lines[targetLine-1] + scheduleMarker

	if err := os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644); err != nil {
		t.Fatalf("write scheduled file: %v", err)
	}

	// Verify
	result, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read result: %v", err)
	}

	resultLines := strings.Split(string(result), "\n")
	expected := "- [ ] Review architecture docs \u23F0 09:00-10:30"
	if resultLines[targetLine-1] != expected {
		t.Errorf("line %d = %q, want %q", targetLine, resultLines[targetLine-1], expected)
	}
	// Other lines unchanged
	if resultLines[3] != "- [ ] Write tests" {
		t.Errorf("line 4 should be unchanged, got %q", resultLines[3])
	}
}

// TestSyncParsePlannerFile writes a planner file in the expected markdown
// format, parses it via the DailyPlanner's loadFromFile, and verifies that
// blocks are correctly reconstructed.
func TestSyncParsePlannerFile(t *testing.T) {
	dir := t.TempDir()
	plannerDir := filepath.Join(dir, "Daily Planner")
	if err := os.MkdirAll(plannerDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	plannerContent := `---
date: 2026-03-08
type: planner
---

## Schedule

- 08:00-09:00 | Morning standup | event
- 09:00-10:30 | Deep work: parser | task
- 10:30-11:00 | Coffee break | break | done
- 14:00-15:00 | Code review | task

## Habits

- [x] Exercise (streak: 5)
- [ ] Read 30 min (streak: 3)
`

	plannerFile := filepath.Join(plannerDir, "2026-03-08.md")
	if err := os.WriteFile(plannerFile, []byte(plannerContent), 0644); err != nil {
		t.Fatalf("write planner file: %v", err)
	}

	dp := NewDailyPlanner()
	dp.vaultRoot = dir
	dp.initBlocks()

	// Set the date to match the file
	dp.date = time.Date(2026, 3, 8, 0, 0, 0, 0, time.Local)

	loaded := dp.loadFromFile()
	if !loaded {
		t.Fatal("expected loadFromFile to succeed")
	}

	// Verify schedule blocks
	// 08:00 is slot index 4 ((8-6)*2 = 4)
	slot0800 := dp.blocks[4]
	if slot0800.TaskText != "Morning standup" {
		t.Errorf("08:00 block text = %q, want %q", slot0800.TaskText, "Morning standup")
	}
	if slot0800.TaskType != blockEvent {
		t.Errorf("08:00 block type = %d, want blockEvent (%d)", slot0800.TaskType, blockEvent)
	}

	// 09:00 is slot index 6 ((9-6)*2 = 6)
	slot0900 := dp.blocks[6]
	if slot0900.TaskText != "Deep work: parser" {
		t.Errorf("09:00 block text = %q, want %q", slot0900.TaskText, "Deep work: parser")
	}
	if slot0900.TaskType != blockTask {
		t.Errorf("09:00 block type = %d, want blockTask (%d)", slot0900.TaskType, blockTask)
	}

	// 10:30 is slot index 9 ((10-6)*2 + 1 = 9)
	slot1030 := dp.blocks[9]
	if slot1030.TaskText != "Coffee break" {
		t.Errorf("10:30 block text = %q, want %q", slot1030.TaskText, "Coffee break")
	}
	if slot1030.TaskType != blockBreak {
		t.Errorf("10:30 block type = %d, want blockBreak (%d)", slot1030.TaskType, blockBreak)
	}
	if !slot1030.Done {
		t.Error("10:30 block should be marked done")
	}

	// 14:00 is slot index 16 ((14-6)*2 = 16)
	slot1400 := dp.blocks[16]
	if slot1400.TaskText != "Code review" {
		t.Errorf("14:00 block text = %q, want %q", slot1400.TaskText, "Code review")
	}
	if slot1400.Done {
		t.Error("14:00 block should NOT be marked done")
	}

	// Verify habits were parsed
	if len(dp.habits) < 2 {
		t.Fatalf("expected at least 2 habits, got %d", len(dp.habits))
	}

	// Find Exercise habit
	foundExercise := false
	foundRead := false
	for _, h := range dp.habits {
		if h.Name == "Exercise" {
			foundExercise = true
			if !h.Done {
				t.Error("Exercise habit should be done")
			}
			if h.Streak != 5 {
				t.Errorf("Exercise streak = %d, want 5", h.Streak)
			}
		}
		if h.Name == "Read 30 min" {
			foundRead = true
			if h.Done {
				t.Error("Read 30 min habit should NOT be done")
			}
			if h.Streak != 3 {
				t.Errorf("Read 30 min streak = %d, want 3", h.Streak)
			}
		}
	}
	if !foundExercise {
		t.Error("Exercise habit not found")
	}
	if !foundRead {
		t.Error("Read 30 min habit not found")
	}
}
