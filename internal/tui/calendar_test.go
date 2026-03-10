package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// NewCalendar — initial state
// ---------------------------------------------------------------------------

func TestCalendar_NewCalendar(t *testing.T) {
	c := NewCalendar()

	if c.active {
		t.Error("expected calendar to be inactive on creation")
	}
	if c.view != calViewMonth {
		t.Errorf("expected default view calViewMonth (0), got %d", c.view)
	}
	if c.dailyNoteDates == nil {
		t.Error("expected dailyNoteDates map to be initialised")
	}
	if c.tasks == nil {
		t.Error("expected tasks map to be initialised")
	}
	if c.plannerBlocks == nil {
		t.Error("expected plannerBlocks map to be initialised")
	}
	// cursor and today should be the same date
	if !c.cursor.Equal(c.today) {
		t.Errorf("expected cursor == today, got cursor=%v today=%v", c.cursor, c.today)
	}
}

// ---------------------------------------------------------------------------
// Open / Close / IsActive — state transitions
// ---------------------------------------------------------------------------

func TestCalendar_OpenCloseIsActive(t *testing.T) {
	c := NewCalendar()

	if c.IsActive() {
		t.Error("expected IsActive false before Open")
	}

	c.Open()
	if !c.IsActive() {
		t.Error("expected IsActive true after Open")
	}

	c.Close()
	if c.IsActive() {
		t.Error("expected IsActive false after Close")
	}
}

func TestCalendar_OpenResetsState(t *testing.T) {
	c := NewCalendar()
	c.Open()

	// Mutate some internal state
	c.selected = "2025-01-15"
	c.showEvents = true
	c.addingEvent = true
	c.eventInput = "hello"
	c.agendaCursor = 5
	c.agendaScroll = 3
	c.taskToggles = append(c.taskToggles, TaskToggle{NotePath: "x.md"})

	// Re-open should reset everything
	c.Open()
	if c.selected != "" {
		t.Error("expected selected to be empty after Open")
	}
	if c.showEvents {
		t.Error("expected showEvents to be false after Open")
	}
	if c.addingEvent {
		t.Error("expected addingEvent to be false after Open")
	}
	if c.eventInput != "" {
		t.Error("expected eventInput to be empty after Open")
	}
	if c.agendaCursor != 0 {
		t.Error("expected agendaCursor to be 0 after Open")
	}
	if c.agendaScroll != 0 {
		t.Error("expected agendaScroll to be 0 after Open")
	}
	if c.taskToggles != nil {
		t.Error("expected taskToggles to be nil after Open")
	}
}

// ---------------------------------------------------------------------------
// SetSize
// ---------------------------------------------------------------------------

func TestCalendar_SetSize(t *testing.T) {
	c := NewCalendar()
	c.SetSize(120, 40)

	if c.width != 120 {
		t.Errorf("expected width=120, got %d", c.width)
	}
	if c.height != 40 {
		t.Errorf("expected height=40, got %d", c.height)
	}
}

// ---------------------------------------------------------------------------
// Navigation — left/right/up/down in month view
// ---------------------------------------------------------------------------

func TestCalendar_NavigateRight(t *testing.T) {
	c := NewCalendar()
	c.Open()
	start := c.cursor

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})

	expected := start.AddDate(0, 0, 1)
	if !c.cursor.Equal(expected) {
		t.Errorf("expected cursor %v after right, got %v", expected, c.cursor)
	}
}

func TestCalendar_NavigateLeft(t *testing.T) {
	c := NewCalendar()
	c.Open()
	start := c.cursor

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})

	expected := start.AddDate(0, 0, -1)
	if !c.cursor.Equal(expected) {
		t.Errorf("expected cursor %v after left, got %v", expected, c.cursor)
	}
}

func TestCalendar_NavigateUp(t *testing.T) {
	c := NewCalendar()
	c.Open()
	start := c.cursor

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})

	expected := start.AddDate(0, 0, -7)
	if !c.cursor.Equal(expected) {
		t.Errorf("expected cursor %v after up, got %v", expected, c.cursor)
	}
}

func TestCalendar_NavigateDown(t *testing.T) {
	c := NewCalendar()
	c.Open()
	start := c.cursor

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	expected := start.AddDate(0, 0, 7)
	if !c.cursor.Equal(expected) {
		t.Errorf("expected cursor %v after down, got %v", expected, c.cursor)
	}
}

func TestCalendar_NavigateArrowKeys(t *testing.T) {
	c := NewCalendar()
	c.Open()
	start := c.cursor

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRight})
	if !c.cursor.Equal(start.AddDate(0, 0, 1)) {
		t.Error("right arrow should move cursor +1 day")
	}

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if !c.cursor.Equal(start) {
		t.Error("left arrow should move cursor -1 day")
	}

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyDown})
	if !c.cursor.Equal(start.AddDate(0, 0, 7)) {
		t.Error("down arrow should move cursor +7 days")
	}

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyUp})
	if !c.cursor.Equal(start) {
		t.Error("up arrow should move cursor -7 days")
	}
}

// ---------------------------------------------------------------------------
// Month boundaries — navigating past month end wraps correctly
// ---------------------------------------------------------------------------

func TestCalendar_MonthBoundary(t *testing.T) {
	c := NewCalendar()
	c.Open()
	// Set cursor to Jan 31, 2025
	c.cursor = time.Date(2025, time.January, 31, 0, 0, 0, 0, time.Local)
	c.syncViewing()

	// Navigate right -> should go to Feb 1
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})

	expected := time.Date(2025, time.February, 1, 0, 0, 0, 0, time.Local)
	if !c.cursor.Equal(expected) {
		t.Errorf("expected cursor %v after crossing month boundary, got %v", expected, c.cursor)
	}
	// Viewing month should have updated
	if c.viewing.Month() != time.February {
		t.Errorf("expected viewing month February, got %v", c.viewing.Month())
	}
}

func TestCalendar_MonthBoundaryBackward(t *testing.T) {
	c := NewCalendar()
	c.Open()
	// Set cursor to March 1, 2025
	c.cursor = time.Date(2025, time.March, 1, 0, 0, 0, 0, time.Local)
	c.syncViewing()

	// Navigate left -> should go to Feb 28
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})

	expected := time.Date(2025, time.February, 28, 0, 0, 0, 0, time.Local)
	if !c.cursor.Equal(expected) {
		t.Errorf("expected cursor %v after crossing month boundary backward, got %v", expected, c.cursor)
	}
}

func TestCalendar_MonthJumpBrackets(t *testing.T) {
	c := NewCalendar()
	c.Open()
	c.cursor = time.Date(2025, time.January, 15, 0, 0, 0, 0, time.Local)
	c.syncViewing()

	// ] jumps forward one month
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("]")})
	if c.cursor.Month() != time.February || c.cursor.Day() != 15 {
		t.Errorf("expected Feb 15 after ], got %v", c.cursor)
	}

	// [ jumps backward one month
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("[")})
	if c.cursor.Month() != time.January || c.cursor.Day() != 15 {
		t.Errorf("expected Jan 15 after [, got %v", c.cursor)
	}
}

// ---------------------------------------------------------------------------
// SetDailyNotes — marks dates with daily notes
// ---------------------------------------------------------------------------

func TestCalendar_SetDailyNotes(t *testing.T) {
	c := NewCalendar()

	notes := []string{
		"daily/2025-01-15.md",
		"daily/2025-02-20.md",
		"not-a-date.md",
		"daily/2025-03-10.md",
	}
	c.SetDailyNotes(notes)

	if !c.dailyNoteDates["2025-01-15"] {
		t.Error("expected 2025-01-15 to be marked")
	}
	if !c.dailyNoteDates["2025-02-20"] {
		t.Error("expected 2025-02-20 to be marked")
	}
	if !c.dailyNoteDates["2025-03-10"] {
		t.Error("expected 2025-03-10 to be marked")
	}
	if c.dailyNoteDates["not-a-date"] {
		t.Error("expected non-date file to not be marked")
	}
	if len(c.dailyNoteDates) != 3 {
		t.Errorf("expected 3 daily note dates, got %d", len(c.dailyNoteDates))
	}
}

func TestCalendar_SetDailyNotesEmpty(t *testing.T) {
	c := NewCalendar()
	c.SetDailyNotes(nil)

	if len(c.dailyNoteDates) != 0 {
		t.Errorf("expected 0 daily note dates for nil input, got %d", len(c.dailyNoteDates))
	}
}

// ---------------------------------------------------------------------------
// SelectedDate — consumed-once pattern
// ---------------------------------------------------------------------------

func TestCalendar_SelectedDateConsumedOnce(t *testing.T) {
	c := NewCalendar()
	c.Open()

	// Press enter to select the current date
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})

	date := c.SelectedDate()
	if date == "" {
		t.Error("expected a selected date after Enter")
	}

	// Second call should return empty (consumed)
	date2 := c.SelectedDate()
	if date2 != "" {
		t.Errorf("expected empty string on second SelectedDate() call, got %q", date2)
	}
}

func TestCalendar_SelectedDateFormat(t *testing.T) {
	c := NewCalendar()
	c.Open()
	c.cursor = time.Date(2025, time.June, 15, 0, 0, 0, 0, time.Local)

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})

	date := c.SelectedDate()
	if date != "2025-06-15" {
		t.Errorf("expected 2025-06-15, got %q", date)
	}
}

func TestCalendar_EnterClosesCalendar(t *testing.T) {
	c := NewCalendar()
	c.Open()

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if c.IsActive() {
		t.Error("expected calendar to be closed after Enter")
	}
}

// ---------------------------------------------------------------------------
// PendingEvent — consumed-once pattern
// ---------------------------------------------------------------------------

func TestCalendar_PendingEventConsumedOnce(t *testing.T) {
	c := NewCalendar()
	c.Open()
	c.SetSize(120, 40)
	c.cursor = time.Date(2025, time.March, 10, 0, 0, 0, 0, time.Local)

	// Press 'a' to enter add mode
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !c.addingEvent {
		t.Fatal("expected addingEvent to be true after pressing 'a'")
	}

	// Type "Buy groceries"
	for _, r := range "Buy groceries" {
		c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press enter to save
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})

	date, text, ok := c.PendingEvent()
	if !ok {
		t.Fatal("expected PendingEvent to return ok=true")
	}
	if date != "2025-03-10" {
		t.Errorf("expected date 2025-03-10, got %q", date)
	}
	if text != "Buy groceries" {
		t.Errorf("expected text 'Buy groceries', got %q", text)
	}

	// Second call should return nothing (consumed)
	_, _, ok2 := c.PendingEvent()
	if ok2 {
		t.Error("expected PendingEvent to return ok=false on second call")
	}
}

func TestCalendar_PendingEventEmptyText(t *testing.T) {
	c := NewCalendar()
	c.Open()

	// Enter add mode
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	// Press enter with empty text
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})

	_, _, ok := c.PendingEvent()
	if ok {
		t.Error("expected PendingEvent to return ok=false for empty event text")
	}
}

func TestCalendar_AddEventEscapeCancels(t *testing.T) {
	c := NewCalendar()
	c.Open()

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !c.addingEvent {
		t.Fatal("expected addingEvent mode")
	}

	// Type something
	for _, r := range "test" {
		c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Escape cancels
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.addingEvent {
		t.Error("expected addingEvent to be false after Esc")
	}
	if c.eventInput != "" {
		t.Error("expected eventInput to be empty after Esc")
	}

	// No pending event
	_, _, ok := c.PendingEvent()
	if ok {
		t.Error("expected no pending event after Esc")
	}
}

// ---------------------------------------------------------------------------
// Mode switching — month/week/agenda views
// ---------------------------------------------------------------------------

func TestCalendar_ViewCycling(t *testing.T) {
	c := NewCalendar()
	c.Open()

	if c.view != calViewMonth {
		t.Error("expected initial view to be month")
	}

	// 'w' cycles month -> week
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if c.view != calViewWeek {
		t.Errorf("expected calViewWeek after first 'w', got %d", c.view)
	}

	// 'w' cycles week -> agenda
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if c.view != calViewAgenda {
		t.Errorf("expected calViewAgenda after second 'w', got %d", c.view)
	}

	// 'w' cycles agenda -> month
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if c.view != calViewMonth {
		t.Errorf("expected calViewMonth after third 'w', got %d", c.view)
	}
}

func TestCalendar_YearViewToggle(t *testing.T) {
	c := NewCalendar()
	c.Open()

	// 'y' toggles year view on
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if c.view != calViewYear {
		t.Errorf("expected calViewYear after 'y', got %d", c.view)
	}

	// 'y' toggles year view off -> back to month
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if c.view != calViewMonth {
		t.Errorf("expected calViewMonth after second 'y', got %d", c.view)
	}
}

// ---------------------------------------------------------------------------
// SetNoteContents — task extraction from notes
// ---------------------------------------------------------------------------

func TestCalendar_SetNoteContents(t *testing.T) {
	c := NewCalendar()

	notes := map[string]string{
		"daily/2025-01-15.md": "# Daily\n- [x] Done task\n- [ ] Pending task\nSome text\n- [ ] Another task",
		"project.md":          "# Project\n- [ ] Not a daily task\n- [X] Complete",
	}
	c.SetNoteContents(notes)

	// Should have 2 tasks for 2025-01-15
	if len(c.tasks["2025-01-15"]) != 3 {
		t.Errorf("expected 3 tasks for 2025-01-15, got %d", len(c.tasks["2025-01-15"]))
	}

	// project.md tasks are NOT filed under a date
	if len(c.tasks[""]) != 0 {
		t.Errorf("expected 0 tasks under empty date key, got %d", len(c.tasks[""]))
	}

	// All tasks: 3 from daily + 2 from project
	if len(c.allTasks) != 5 {
		t.Errorf("expected 5 total tasks, got %d", len(c.allTasks))
	}

	// Verify done/pending detection
	dailyTasks := c.tasks["2025-01-15"]
	doneCount := 0
	for _, task := range dailyTasks {
		if task.Done {
			doneCount++
		}
	}
	if doneCount != 1 {
		t.Errorf("expected 1 done task in daily note, got %d", doneCount)
	}
}

func TestCalendar_SetNoteContentsLineNumbers(t *testing.T) {
	c := NewCalendar()

	notes := map[string]string{
		"daily/2025-01-15.md": "# Title\n\n- [ ] First task\n- [ ] Second task",
	}
	c.SetNoteContents(notes)

	tasks := c.tasks["2025-01-15"]
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
	// Line numbers should be 1-based
	if tasks[0].LineNum != 3 {
		t.Errorf("expected first task at line 3, got %d", tasks[0].LineNum)
	}
	if tasks[1].LineNum != 4 {
		t.Errorf("expected second task at line 4, got %d", tasks[1].LineNum)
	}
}

func TestCalendar_SetNoteContentsEmpty(t *testing.T) {
	c := NewCalendar()
	c.SetNoteContents(map[string]string{})

	if len(c.allTasks) != 0 {
		t.Errorf("expected 0 tasks for empty input, got %d", len(c.allTasks))
	}
}

// ---------------------------------------------------------------------------
// GetTaskToggles — consumed-once pattern
// ---------------------------------------------------------------------------

func TestCalendar_GetTaskTogglesConsumedOnce(t *testing.T) {
	c := NewCalendar()
	c.taskToggles = []TaskToggle{
		{NotePath: "test.md", LineNum: 3, Text: "A task", Done: true},
	}

	toggles := c.GetTaskToggles()
	if len(toggles) != 1 {
		t.Fatalf("expected 1 toggle, got %d", len(toggles))
	}
	if toggles[0].NotePath != "test.md" {
		t.Errorf("expected NotePath 'test.md', got %q", toggles[0].NotePath)
	}

	// Second call should return nil
	toggles2 := c.GetTaskToggles()
	if toggles2 != nil {
		t.Error("expected nil on second GetTaskToggles call")
	}
}

// ---------------------------------------------------------------------------
// Today shortcut
// ---------------------------------------------------------------------------

func TestCalendar_TodayShortcut(t *testing.T) {
	c := NewCalendar()
	c.Open()

	// Move cursor away from today
	for i := 0; i < 10; i++ {
		c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	}
	if c.cursor.Equal(c.today) {
		t.Fatal("cursor should have moved away from today")
	}

	// Press 't' to jump back to today
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if !c.cursor.Equal(c.today) {
		t.Errorf("expected cursor to be today after 't', got %v", c.cursor)
	}
}

// ---------------------------------------------------------------------------
// Escape closes calendar (or events sub-panel)
// ---------------------------------------------------------------------------

func TestCalendar_EscapeClosesCalendar(t *testing.T) {
	c := NewCalendar()
	c.Open()

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.IsActive() {
		t.Error("expected calendar to close on Esc")
	}
}

func TestCalendar_EscapeClosesEventsPanelFirst(t *testing.T) {
	c := NewCalendar()
	c.Open()

	// Toggle events panel on
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	if !c.showEvents {
		t.Fatal("expected showEvents to be true after 'e'")
	}

	// Esc should close events panel, not the calendar
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.showEvents {
		t.Error("expected showEvents to be false after Esc")
	}
	if !c.IsActive() {
		t.Error("expected calendar to remain active (Esc closes events panel first)")
	}
}

// ---------------------------------------------------------------------------
// Inactive calendar ignores updates
// ---------------------------------------------------------------------------

func TestCalendar_InactiveIgnoresInput(t *testing.T) {
	c := NewCalendar()
	start := c.cursor

	// Calendar is not open — sending keys should have no effect
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	if !c.cursor.Equal(start) {
		t.Error("inactive calendar should not respond to key messages")
	}
}

// ---------------------------------------------------------------------------
// Task priority
// ---------------------------------------------------------------------------

func TestCalendar_TaskPriority(t *testing.T) {
	tests := []struct {
		text string
		want int
	}{
		{"Normal task", 0},
		{"Low priority \U0001f535", 1},
		{"Medium priority \U0001f7e1", 2},
		{"High priority \U0001f7e0", 3},
		{"Highest priority \U0001f534", 4},
	}

	for _, tc := range tests {
		got := taskPriority(tc.text)
		if got != tc.want {
			t.Errorf("taskPriority(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// syncViewing
// ---------------------------------------------------------------------------

func TestCalendar_SyncViewing(t *testing.T) {
	c := NewCalendar()
	c.Open()
	c.cursor = time.Date(2025, time.November, 20, 0, 0, 0, 0, time.Local)
	c.syncViewing()

	if c.viewing.Month() != time.November {
		t.Errorf("expected viewing month November, got %v", c.viewing.Month())
	}
	if c.viewing.Day() != 1 {
		t.Errorf("expected viewing day 1 (first of month), got %d", c.viewing.Day())
	}
}

// ---------------------------------------------------------------------------
// Backspace in event input
// ---------------------------------------------------------------------------

func TestCalendar_AddEventBackspace(t *testing.T) {
	c := NewCalendar()
	c.Open()

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	// Type "abc"
	for _, r := range "abc" {
		c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if c.eventInput != "abc" {
		t.Fatalf("expected eventInput='abc', got %q", c.eventInput)
	}

	// Backspace
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if c.eventInput != "ab" {
		t.Errorf("expected eventInput='ab' after backspace, got %q", c.eventInput)
	}
}

// ---------------------------------------------------------------------------
// daysIn helper
// ---------------------------------------------------------------------------

func TestCalendar_DaysIn(t *testing.T) {
	tests := []struct {
		month time.Month
		year  int
		want  int
	}{
		{time.January, 2025, 31},
		{time.February, 2025, 28},
		{time.February, 2024, 29}, // leap year
		{time.April, 2025, 30},
		{time.December, 2025, 31},
	}

	for _, tc := range tests {
		got := daysIn(tc.month, tc.year)
		if got != tc.want {
			t.Errorf("daysIn(%v, %d) = %d, want %d", tc.month, tc.year, got, tc.want)
		}
	}
}
