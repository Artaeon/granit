package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// NewPomodoro — initial state
// ---------------------------------------------------------------------------

func TestNewPomodoro_InitialState(t *testing.T) {
	p := NewPomodoro()

	if p.state != PomodoroIdle {
		t.Errorf("expected state=PomodoroIdle, got %d", p.state)
	}
	if len(p.queue) != 0 {
		t.Errorf("expected empty queue, got %d items", len(p.queue))
	}
	if p.taskTimeLog == nil {
		t.Error("expected non-nil taskTimeLog map")
	}
	if len(p.taskTimeLog) != 0 {
		t.Errorf("expected empty taskTimeLog, got %d entries", len(p.taskTimeLog))
	}
	if p.workDuration != 25*time.Minute {
		t.Errorf("expected workDuration=25m, got %v", p.workDuration)
	}
	if p.shortBreak != 5*time.Minute {
		t.Errorf("expected shortBreak=5m, got %v", p.shortBreak)
	}
	if p.longBreak != 15*time.Minute {
		t.Errorf("expected longBreak=15m, got %v", p.longBreak)
	}
	if p.longBreakAfter != 4 {
		t.Errorf("expected longBreakAfter=4, got %d", p.longBreakAfter)
	}
	if !p.showQueue {
		t.Error("expected showQueue=true by default")
	}
	if p.active {
		t.Error("expected active=false initially")
	}
	if p.currentTask != "" {
		t.Errorf("expected empty currentTask, got %q", p.currentTask)
	}
	if p.queueCursor != 0 {
		t.Errorf("expected queueCursor=0, got %d", p.queueCursor)
	}
	if p.sessionsToday != 0 {
		t.Errorf("expected sessionsToday=0, got %d", p.sessionsToday)
	}
}

// ---------------------------------------------------------------------------
// SetQueue
// ---------------------------------------------------------------------------

func TestPomodoro_SetQueue_AdvancesToFirstUndone(t *testing.T) {
	p := NewPomodoro()

	tasks := []QueueTask{
		{Text: "done task", Done: true},
		{Text: "done task 2", Done: true},
		{Text: "first undone", Done: false},
		{Text: "second undone", Done: false},
	}
	p.SetQueue(tasks)

	if p.queueCursor != 2 {
		t.Errorf("expected queueCursor=2 (first undone), got %d", p.queueCursor)
	}
	if p.currentTask != "first undone" {
		t.Errorf("expected currentTask='first undone', got %q", p.currentTask)
	}
	if !p.showQueue {
		t.Error("expected showQueue=true after SetQueue with items")
	}
}

func TestPomodoro_SetQueue_AllDone(t *testing.T) {
	p := NewPomodoro()

	tasks := []QueueTask{
		{Text: "done1", Done: true},
		{Text: "done2", Done: true},
	}
	p.SetQueue(tasks)

	// When all tasks are done, the cursor stays at 0 (the for-loop doesn't break
	// on an undone task), and CurrentQueueTask returns nil.
	qt := p.CurrentQueueTask()
	if qt != nil {
		t.Errorf("expected nil CurrentQueueTask when all done, got %v", qt)
	}
	// currentTask should still be empty since no undone task was found.
	// (The SetQueue sets currentTask only if CurrentQueueTask returns non-nil.)
	if p.currentTask != "" {
		t.Errorf("expected empty currentTask when all done, got %q", p.currentTask)
	}
}

func TestPomodoro_SetQueue_EmptySlice(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{})

	if len(p.queue) != 0 {
		t.Errorf("expected empty queue, got %d items", len(p.queue))
	}
	// Should not panic.
	qt := p.CurrentQueueTask()
	if qt != nil {
		t.Error("expected nil CurrentQueueTask for empty queue")
	}
}

func TestPomodoro_SetQueue_NilSlice(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue(nil)

	if p.queue != nil {
		t.Errorf("expected nil queue, got %v", p.queue)
	}
	qt := p.CurrentQueueTask()
	if qt != nil {
		t.Error("expected nil CurrentQueueTask for nil queue")
	}
}

func TestPomodoro_SetQueue_ResetsScroll(t *testing.T) {
	p := NewPomodoro()
	p.queueScroll = 5

	p.SetQueue([]QueueTask{{Text: "task1"}})

	if p.queueScroll != 0 {
		t.Errorf("expected queueScroll reset to 0, got %d", p.queueScroll)
	}
}

// ---------------------------------------------------------------------------
// AddToQueue
// ---------------------------------------------------------------------------

func TestPomodoro_AddToQueue_Appends(t *testing.T) {
	p := NewPomodoro()

	p.AddToQueue(QueueTask{Text: "first"})
	if len(p.queue) != 1 {
		t.Fatalf("expected 1 item, got %d", len(p.queue))
	}
	if p.queue[0].Text != "first" {
		t.Errorf("expected queue[0].Text='first', got %q", p.queue[0].Text)
	}
	// First item should set cursor and currentTask.
	if p.queueCursor != 0 {
		t.Errorf("expected queueCursor=0, got %d", p.queueCursor)
	}
	if p.currentTask != "first" {
		t.Errorf("expected currentTask='first', got %q", p.currentTask)
	}

	p.AddToQueue(QueueTask{Text: "second"})
	if len(p.queue) != 2 {
		t.Fatalf("expected 2 items, got %d", len(p.queue))
	}
	if p.queue[1].Text != "second" {
		t.Errorf("expected queue[1].Text='second', got %q", p.queue[1].Text)
	}
	// currentTask should still be "first" since it wasn't the first add.
	if p.currentTask != "first" {
		t.Errorf("expected currentTask still 'first', got %q", p.currentTask)
	}
}

func TestPomodoro_AddToQueue_FirstSetsCurrent(t *testing.T) {
	p := NewPomodoro()
	p.AddToQueue(QueueTask{Text: "only task"})

	if p.currentTask != "only task" {
		t.Errorf("expected currentTask='only task', got %q", p.currentTask)
	}
}

// ---------------------------------------------------------------------------
// CurrentQueueTask
// ---------------------------------------------------------------------------

func TestPomodoro_CurrentQueueTask_ReturnsUndone(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{
		{Text: "task A", Done: false},
		{Text: "task B", Done: false},
	})

	qt := p.CurrentQueueTask()
	if qt == nil {
		t.Fatal("expected non-nil CurrentQueueTask")
	}
	if qt.Text != "task A" {
		t.Errorf("expected 'task A', got %q", qt.Text)
	}
}

func TestPomodoro_CurrentQueueTask_SkipsDone(t *testing.T) {
	p := NewPomodoro()
	p.queue = []QueueTask{
		{Text: "done1", Done: true},
		{Text: "done2", Done: true},
		{Text: "undone", Done: false},
	}
	p.queueCursor = 0

	qt := p.CurrentQueueTask()
	if qt == nil {
		t.Fatal("expected non-nil CurrentQueueTask")
	}
	if qt.Text != "undone" {
		t.Errorf("expected 'undone', got %q", qt.Text)
	}
	// Cursor should have advanced.
	if p.queueCursor != 2 {
		t.Errorf("expected queueCursor=2, got %d", p.queueCursor)
	}
}

func TestPomodoro_CurrentQueueTask_AllDoneReturnsNil(t *testing.T) {
	p := NewPomodoro()
	p.queue = []QueueTask{
		{Text: "done1", Done: true},
		{Text: "done2", Done: true},
	}
	p.queueCursor = 0

	qt := p.CurrentQueueTask()
	if qt != nil {
		t.Errorf("expected nil when all tasks done, got %v", qt)
	}
}

func TestPomodoro_CurrentQueueTask_EmptyQueue(t *testing.T) {
	p := NewPomodoro()

	qt := p.CurrentQueueTask()
	if qt != nil {
		t.Errorf("expected nil for empty queue, got %v", qt)
	}
}

// ---------------------------------------------------------------------------
// AdvanceQueue
// ---------------------------------------------------------------------------

func TestPomodoro_AdvanceQueue_MarksDoneAndMoves(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{
		{Text: "task1", Done: false},
		{Text: "task2", Done: false},
		{Text: "task3", Done: false},
	})

	ok := p.AdvanceQueue()
	if !ok {
		t.Error("expected AdvanceQueue to return true when next task exists")
	}
	if !p.queue[0].Done {
		t.Error("expected task1 to be marked done")
	}
	if p.currentTask != "task2" {
		t.Errorf("expected currentTask='task2', got %q", p.currentTask)
	}
	if p.queueCursor != 1 {
		t.Errorf("expected queueCursor=1, got %d", p.queueCursor)
	}
	// State should be reset to Work.
	if p.state != PomodoroWork {
		t.Errorf("expected state=PomodoroWork, got %d", p.state)
	}
}

func TestPomodoro_AdvanceQueue_EmptyQueue(t *testing.T) {
	p := NewPomodoro()

	ok := p.AdvanceQueue()
	if ok {
		t.Error("expected AdvanceQueue to return false for empty queue")
	}
}

func TestPomodoro_AdvanceQueue_AtEnd(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{
		{Text: "only task", Done: false},
	})

	ok := p.AdvanceQueue()
	if ok {
		t.Error("expected AdvanceQueue to return false when no more tasks")
	}
	if !p.queue[0].Done {
		t.Error("expected the only task to be marked done")
	}
	if p.currentTask != "" {
		t.Errorf("expected empty currentTask after all done, got %q", p.currentTask)
	}
}

func TestPomodoro_AdvanceQueue_SkipsAlreadyDone(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{
		{Text: "task1", Done: false},
		{Text: "task2", Done: true}, // already done
		{Text: "task3", Done: false},
	})

	ok := p.AdvanceQueue()
	if !ok {
		t.Error("expected AdvanceQueue to return true")
	}
	// Should have skipped task2 (already done) and moved to task3.
	if p.currentTask != "task3" {
		t.Errorf("expected currentTask='task3', got %q", p.currentTask)
	}
	if p.queueCursor != 2 {
		t.Errorf("expected queueCursor=2, got %d", p.queueCursor)
	}
}

func TestPomodoro_AdvanceQueue_AllRemainingDone(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{
		{Text: "task1", Done: false},
		{Text: "task2", Done: true},
	})

	ok := p.AdvanceQueue()
	if ok {
		t.Error("expected false when all remaining tasks are done")
	}
	if p.currentTask != "" {
		t.Errorf("expected empty currentTask, got %q", p.currentTask)
	}
}

// ---------------------------------------------------------------------------
// SkipTask
// ---------------------------------------------------------------------------

func TestPomodoro_SkipTask_MovesToNext(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{
		{Text: "task1", Done: false},
		{Text: "task2", Done: false},
		{Text: "task3", Done: false},
	})

	ok := p.SkipTask()
	if !ok {
		t.Error("expected SkipTask to return true")
	}
	// task1 should NOT be marked done.
	if p.queue[0].Done {
		t.Error("expected task1 to remain not-done after skip")
	}
	if p.currentTask != "task2" {
		t.Errorf("expected currentTask='task2', got %q", p.currentTask)
	}
	if p.queueCursor != 1 {
		t.Errorf("expected queueCursor=1, got %d", p.queueCursor)
	}
}

func TestPomodoro_SkipTask_EmptyQueue(t *testing.T) {
	p := NewPomodoro()

	ok := p.SkipTask()
	if ok {
		t.Error("expected SkipTask to return false for empty queue")
	}
}

func TestPomodoro_SkipTask_AtEnd(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{
		{Text: "only task", Done: false},
	})

	ok := p.SkipTask()
	if ok {
		t.Error("expected SkipTask to return false when at end")
	}
}

func TestPomodoro_SkipTask_SkipsDone(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{
		{Text: "task1", Done: false},
		{Text: "task2", Done: true},
		{Text: "task3", Done: false},
	})

	ok := p.SkipTask()
	if !ok {
		t.Error("expected SkipTask to return true")
	}
	if p.currentTask != "task3" {
		t.Errorf("expected currentTask='task3', got %q", p.currentTask)
	}
}

// ---------------------------------------------------------------------------
// GetTimeLog
// ---------------------------------------------------------------------------

func TestPomodoro_GetTimeLog_ReturnsMap(t *testing.T) {
	p := NewPomodoro()

	log := p.GetTimeLog()
	if log == nil {
		t.Error("expected non-nil time log")
	}
	if len(log) != 0 {
		t.Errorf("expected empty time log, got %d entries", len(log))
	}

	// Manually add an entry and verify it's reflected.
	p.taskTimeLog["test task"] = 10
	log = p.GetTimeLog()
	if log["test task"] != 10 {
		t.Errorf("expected 10 minutes for 'test task', got %d", log["test task"])
	}
}

// ---------------------------------------------------------------------------
// Queue navigation (j/k scroll, q toggle)
// ---------------------------------------------------------------------------

func TestPomodoro_QueueScroll_JK(t *testing.T) {
	p := NewPomodoro()
	p.active = true
	p.showQueue = true
	p.SetQueue([]QueueTask{
		{Text: "task1"},
		{Text: "task2"},
		{Text: "task3"},
	})

	// Scroll down with 'j'.
	keyJ := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}}
	p, _ = p.Update(keyJ)
	if p.queueScroll != 1 {
		t.Errorf("expected queueScroll=1 after j, got %d", p.queueScroll)
	}

	p, _ = p.Update(keyJ)
	if p.queueScroll != 2 {
		t.Errorf("expected queueScroll=2 after second j, got %d", p.queueScroll)
	}

	// Should clamp at max.
	p, _ = p.Update(keyJ)
	if p.queueScroll != 2 {
		t.Errorf("expected queueScroll to stay at 2 (max), got %d", p.queueScroll)
	}

	// Scroll up with 'k'.
	keyK := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}}
	p, _ = p.Update(keyK)
	if p.queueScroll != 1 {
		t.Errorf("expected queueScroll=1 after k, got %d", p.queueScroll)
	}

	p, _ = p.Update(keyK)
	if p.queueScroll != 0 {
		t.Errorf("expected queueScroll=0 after second k, got %d", p.queueScroll)
	}

	// Should clamp at 0.
	p, _ = p.Update(keyK)
	if p.queueScroll != 0 {
		t.Errorf("expected queueScroll to stay at 0 (min), got %d", p.queueScroll)
	}
}

func TestPomodoro_ToggleQueueVisibility(t *testing.T) {
	p := NewPomodoro()
	p.active = true

	if !p.showQueue {
		t.Error("expected showQueue=true initially")
	}

	keyQ := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}
	p, _ = p.Update(keyQ)
	if p.showQueue {
		t.Error("expected showQueue=false after q toggle")
	}

	p, _ = p.Update(keyQ)
	if !p.showQueue {
		t.Error("expected showQueue=true after second q toggle")
	}
}

// ---------------------------------------------------------------------------
// StatusString
// ---------------------------------------------------------------------------

func TestPomodoro_StatusString_Idle(t *testing.T) {
	p := NewPomodoro()
	s := p.StatusString()
	if s != "" {
		t.Errorf("expected empty StatusString when idle, got %q", s)
	}
}

func TestPomodoro_StatusString_Work_WithTask(t *testing.T) {
	p := NewPomodoro()
	p.state = PomodoroWork
	p.remaining = 15 * time.Minute
	p.currentTask = "Write tests"

	s := p.StatusString()
	if s == "" {
		t.Error("expected non-empty StatusString during work")
	}
	// Should contain the task name.
	if !containsSubstring(s, "Write tests") {
		t.Errorf("StatusString should contain task name, got %q", s)
	}
}

func TestPomodoro_StatusString_Work_EmptyTask(t *testing.T) {
	p := NewPomodoro()
	p.state = PomodoroWork
	p.remaining = 10 * time.Minute
	p.currentTask = ""

	s := p.StatusString()
	if s == "" {
		t.Error("expected non-empty StatusString during work even without task")
	}
}

func TestPomodoro_StatusString_Work_LongTaskTruncated(t *testing.T) {
	p := NewPomodoro()
	p.state = PomodoroWork
	p.remaining = 5 * time.Minute
	p.currentTask = "This is a very long task name that should be truncated"

	s := p.StatusString()
	if !containsSubstring(s, "…") {
		t.Errorf("expected truncated task with '…' in StatusString, got %q", s)
	}
}

func TestPomodoro_StatusString_Break(t *testing.T) {
	p := NewPomodoro()
	p.state = PomodoroShortBreak
	p.remaining = 3 * time.Minute

	s := p.StatusString()
	if s == "" {
		t.Error("expected non-empty StatusString during break")
	}
}

func TestPomodoro_StatusString_LongBreak(t *testing.T) {
	p := NewPomodoro()
	p.state = PomodoroBreakonLong
	p.remaining = 10 * time.Minute

	s := p.StatusString()
	if s == "" {
		t.Error("expected non-empty StatusString during long break")
	}
}

func TestPomodoro_StatusString_Paused(t *testing.T) {
	p := NewPomodoro()
	p.state = PomodoroWork
	p.remaining = -10 * time.Minute // negative means paused

	s := p.StatusString()
	if s == "" {
		t.Error("expected non-empty StatusString when paused")
	}
}

// ---------------------------------------------------------------------------
// QueueComplete
// ---------------------------------------------------------------------------

func TestPomodoro_QueueComplete_AllDone(t *testing.T) {
	p := NewPomodoro()
	p.queue = []QueueTask{
		{Text: "t1", Done: true},
		{Text: "t2", Done: true},
	}
	if !p.queueComplete() {
		t.Error("expected queueComplete=true when all tasks are done")
	}
}

func TestPomodoro_QueueComplete_SomeUndone(t *testing.T) {
	p := NewPomodoro()
	p.queue = []QueueTask{
		{Text: "t1", Done: true},
		{Text: "t2", Done: false},
	}
	if p.queueComplete() {
		t.Error("expected queueComplete=false when some tasks are not done")
	}
}

func TestPomodoro_QueueComplete_EmptyQueue(t *testing.T) {
	p := NewPomodoro()
	if p.queueComplete() {
		t.Error("expected queueComplete=false for empty queue")
	}
}

// ---------------------------------------------------------------------------
// Start sets currentTask from queue
// ---------------------------------------------------------------------------

func TestPomodoro_Start_SetsTaskFromQueue(t *testing.T) {
	p := NewPomodoro()
	p.SetQueue([]QueueTask{
		{Text: "my work", Done: false},
	})

	p.Start()

	if p.state != PomodoroWork {
		t.Errorf("expected PomodoroWork state, got %d", p.state)
	}
	if p.currentTask != "my work" {
		t.Errorf("expected currentTask='my work', got %q", p.currentTask)
	}
}

func TestPomodoro_Start_EmptyQueueNoTask(t *testing.T) {
	p := NewPomodoro()
	p.Start()

	if p.state != PomodoroWork {
		t.Errorf("expected PomodoroWork state, got %d", p.state)
	}
	if p.currentTask != "" {
		t.Errorf("expected empty currentTask, got %q", p.currentTask)
	}
}

// ---------------------------------------------------------------------------
// Open/Close/IsActive
// ---------------------------------------------------------------------------

func TestPomodoro_OpenCloseIsActive(t *testing.T) {
	p := NewPomodoro()

	if p.IsActive() {
		t.Error("should not be active initially")
	}

	p.Open()
	if !p.IsActive() {
		t.Error("should be active after Open")
	}

	p.Close()
	if p.IsActive() {
		t.Error("should not be active after Close")
	}
}

// ---------------------------------------------------------------------------
// IsRunning
// ---------------------------------------------------------------------------

func TestPomodoro_IsRunning(t *testing.T) {
	p := NewPomodoro()
	if p.IsRunning() {
		t.Error("should not be running when idle")
	}

	p.state = PomodoroWork
	p.remaining = 10 * time.Minute
	if !p.IsRunning() {
		t.Error("should be running during work with positive remaining")
	}

	p.remaining = 0
	if p.IsRunning() {
		t.Error("should not be running with zero remaining")
	}
}

// ---------------------------------------------------------------------------
// Pause
// ---------------------------------------------------------------------------

func TestPomodoro_Pause(t *testing.T) {
	p := NewPomodoro()
	p.state = PomodoroWork
	p.remaining = 10 * time.Minute

	p.Pause()
	if p.remaining >= 0 {
		t.Error("expected negative remaining after pause")
	}

	p.Pause()
	if p.remaining < 0 {
		t.Error("expected positive remaining after unpause")
	}
}

func TestPomodoro_Pause_Idle(t *testing.T) {
	p := NewPomodoro()
	p.Pause() // Should be a no-op
	if p.remaining != 0 {
		t.Errorf("expected 0 remaining when idle, got %v", p.remaining)
	}
}

// ---------------------------------------------------------------------------
// View renders without panic
// ---------------------------------------------------------------------------

func TestPomodoro_View_NoPanic(t *testing.T) {
	p := NewPomodoro()
	p.active = true
	p.width = 100
	p.height = 40

	// Idle state.
	output := p.View()
	if output == "" {
		t.Error("expected non-empty View output")
	}

	// With queue.
	p.SetQueue([]QueueTask{
		{Text: "task1", Project: "proj", Estimated: 25},
		{Text: "task2", Done: true},
	})
	p.state = PomodoroWork
	p.remaining = 15 * time.Minute
	p.total = 25 * time.Minute

	output = p.View()
	if output == "" {
		t.Error("expected non-empty View output with queue")
	}
}

func TestPomodoro_View_AddingTaskMode(t *testing.T) {
	p := NewPomodoro()
	p.active = true
	p.width = 100
	p.height = 40
	p.addingTask = true
	p.addTaskInput = "new task"

	output := p.View()
	if output == "" {
		t.Error("expected non-empty View output in add-task mode")
	}
}

// ---------------------------------------------------------------------------
// Update: add task via "a" key then type + enter
// ---------------------------------------------------------------------------

func TestPomodoro_Update_AddTaskFlow(t *testing.T) {
	p := NewPomodoro()
	p.active = true

	// Press 'a' to enter add-task mode.
	keyA := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	p, _ = p.Update(keyA)
	if !p.addingTask {
		t.Error("expected addingTask=true after 'a'")
	}

	// Type some characters.
	keyH := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}}
	keyI := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}
	p, _ = p.Update(keyH)
	p, _ = p.Update(keyI)
	if p.addTaskInput != "Hi" {
		t.Errorf("expected addTaskInput='Hi', got %q", p.addTaskInput)
	}

	// Press Enter to commit.
	keyEnter := tea.KeyMsg{Type: tea.KeyEnter}
	p, _ = p.Update(keyEnter)
	if p.addingTask {
		t.Error("expected addingTask=false after Enter")
	}
	if len(p.queue) != 1 {
		t.Fatalf("expected 1 item in queue, got %d", len(p.queue))
	}
	if p.queue[0].Text != "Hi" {
		t.Errorf("expected queue[0].Text='Hi', got %q", p.queue[0].Text)
	}
}

func TestPomodoro_Update_AddTaskCancel(t *testing.T) {
	p := NewPomodoro()
	p.active = true

	keyA := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}}
	p, _ = p.Update(keyA)

	keyX := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
	p, _ = p.Update(keyX)

	keyEsc := tea.KeyMsg{Type: tea.KeyEsc}
	p, _ = p.Update(keyEsc)

	if p.addingTask {
		t.Error("expected addingTask=false after Esc")
	}
	if len(p.queue) != 0 {
		t.Errorf("expected empty queue after cancel, got %d", len(p.queue))
	}
}

func TestPomodoro_Update_AddTaskBackspace(t *testing.T) {
	p := NewPomodoro()
	p.active = true
	p.addingTask = true
	p.addTaskInput = "abc"

	keyBs := tea.KeyMsg{Type: tea.KeyBackspace}
	p, _ = p.Update(keyBs)
	if p.addTaskInput != "ab" {
		t.Errorf("expected 'ab' after backspace, got %q", p.addTaskInput)
	}

	// Backspace on empty.
	p.addTaskInput = ""
	p, _ = p.Update(keyBs)
	if p.addTaskInput != "" {
		t.Errorf("expected empty after backspace on empty, got %q", p.addTaskInput)
	}
}

func TestPomodoro_Update_AddTaskEmptySubmit(t *testing.T) {
	p := NewPomodoro()
	p.active = true
	p.addingTask = true
	p.addTaskInput = "   " // whitespace only

	keyEnter := tea.KeyMsg{Type: tea.KeyEnter}
	p, _ = p.Update(keyEnter)

	if len(p.queue) != 0 {
		t.Errorf("expected empty queue for whitespace-only task, got %d", len(p.queue))
	}
}

// ---------------------------------------------------------------------------
// SetGoal
// ---------------------------------------------------------------------------

func TestPomodoro_SetGoal(t *testing.T) {
	p := NewPomodoro()

	if p.pomodoroGoal != 0 {
		t.Errorf("expected initial pomodoroGoal=0, got %d", p.pomodoroGoal)
	}

	p.SetGoal(10)
	if p.pomodoroGoal != 10 {
		t.Errorf("expected pomodoroGoal=10, got %d", p.pomodoroGoal)
	}

	p.SetGoal(0)
	if p.pomodoroGoal != 0 {
		t.Errorf("expected pomodoroGoal=0 after reset, got %d", p.pomodoroGoal)
	}
}

// ---------------------------------------------------------------------------
// GetCompletedTasks — consumed-once pattern
// ---------------------------------------------------------------------------

func TestPomodoro_GetCompletedTasks(t *testing.T) {
	p := NewPomodoro()

	// Initially empty.
	result := p.GetCompletedTasks()
	if result != nil {
		t.Errorf("expected nil for empty completedTasks, got %v", result)
	}

	// Add some completed tasks.
	p.completedTasks = []TaskCompletion{
		{NotePath: "work/sprint.md", LineNum: 5, Text: "fix bug", Done: true},
		{NotePath: "daily/today.md", LineNum: 2, Text: "review PR", Done: true},
	}

	// First call returns the tasks.
	result = p.GetCompletedTasks()
	if len(result) != 2 {
		t.Fatalf("expected 2 completed tasks, got %d", len(result))
	}
	if result[0].NotePath != "work/sprint.md" {
		t.Errorf("expected first task NotePath='work/sprint.md', got %q", result[0].NotePath)
	}
	if result[1].Text != "review PR" {
		t.Errorf("expected second task Text='review PR', got %q", result[1].Text)
	}

	// Second call returns nil (consumed).
	result2 := p.GetCompletedTasks()
	if result2 != nil {
		t.Errorf("expected nil on second GetCompletedTasks call, got %v", result2)
	}
}

// containsSubstring is defined in focusmode_test.go (same package)
