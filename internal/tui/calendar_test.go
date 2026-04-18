package tui

import (
	"strings"
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

	// Press 'a' to enter event wizard
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if c.eventEditMode != 1 {
		t.Fatal("expected eventEditMode=1 after pressing 'a'")
	}

	// Type "Buy groceries" as title
	for _, r := range "Buy groceries" {
		c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press enter → time step
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Press enter → skip time (all day)
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Press enter → skip duration
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Press enter → skip location
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Press 0 → no recurrence → color step
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	// Press 0 → default color → description step
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("0")})
	// Press enter → skip description → saves
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})

	ne := c.PendingNativeEvent()
	if ne == nil {
		t.Fatal("expected pending native event")
	}
	date := ne.Date
	text := ne.Title
	ok := true
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

	// Enter event wizard
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})

	// Press enter with empty title → cancels
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if c.eventEditMode != 0 {
		t.Error("expected wizard to cancel on empty title")
	}
}

func TestCalendar_AddEventEscapeCancels(t *testing.T) {
	c := NewCalendar()
	c.Open()

	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if c.eventEditMode != 1 {
		t.Fatal("expected eventEditMode=1")
	}

	// Type something
	for _, r := range "test" {
		c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Escape cancels
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if c.eventEditMode != 0 {
		t.Error("expected eventEditMode=0 after Esc")
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

	// 'w' cycles week -> 3day
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if c.view != calView3Day {
		t.Errorf("expected calView3Day after second 'w', got %d", c.view)
	}

	// 'w' cycles 3day -> 1day
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if c.view != calView1Day {
		t.Errorf("expected calView1Day after third 'w', got %d", c.view)
	}

	// 'w' cycles 1day -> agenda
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if c.view != calViewAgenda {
		t.Errorf("expected calViewAgenda after fourth 'w', got %d", c.view)
	}

	// 'w' cycles agenda -> year
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if c.view != calViewYear {
		t.Errorf("expected calViewYear after fifth 'w', got %d", c.view)
	}

	// 'w' cycles year -> month
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	if c.view != calViewMonth {
		t.Errorf("expected calViewMonth after sixth 'w', got %d", c.view)
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

	// Type "abc" into the Title field (focused by default when form opens).
	for _, r := range "abc" {
		c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if c.eventEditTitle != "abc" {
		t.Fatalf("expected eventEditTitle='abc', got %q", c.eventEditTitle)
	}

	// Backspace
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if c.eventEditTitle != "ab" {
		t.Errorf("expected eventEditTitle='ab' after backspace, got %q", c.eventEditTitle)
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

// ---------------------------------------------------------------------------
// 3-Day View — renders non-empty output
// ---------------------------------------------------------------------------

func TestCalendar_3DayViewRenders(t *testing.T) {
	c := NewCalendar()
	c.Open()
	c.SetSize(120, 40)

	// Switch to 3-day view: month -> week -> 3day (press 'w' twice)
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	c, _ = c.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})

	if c.view != calView3Day {
		t.Fatalf("expected calView3Day, got %d", c.view)
	}

	output := c.View()
	if output == "" {
		t.Fatal("expected non-empty View() output in 3-day view")
	}
}

// ---------------------------------------------------------------------------
// SetHabitData / habitStats — correct counts
// ---------------------------------------------------------------------------

func TestCalendar_SetHabitData(t *testing.T) {
	c := NewCalendar()

	entries := []habitEntry{
		{Name: "Exercise", Created: "2026-03-01", Streak: 5},
		{Name: "Read", Created: "2026-03-01", Streak: 3},
		{Name: "Meditate", Created: "2026-03-01", Streak: 1},
	}
	logs := []habitLog{
		{Date: "2026-03-25", Completed: []string{"Exercise", "Read"}},
		{Date: "2026-03-24", Completed: []string{"Exercise"}},
	}

	c.SetHabitData(entries, logs)

	// 2026-03-25: 2 done out of 3 total
	done, total := c.habitStats("2026-03-25")
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}
	if done != 2 {
		t.Fatalf("expected done=2 for 2026-03-25, got %d", done)
	}

	// 2026-03-24: 1 done out of 3 total
	done, total = c.habitStats("2026-03-24")
	if total != 3 {
		t.Fatalf("expected total=3, got %d", total)
	}
	if done != 1 {
		t.Fatalf("expected done=1 for 2026-03-24, got %d", done)
	}

	// Date with no logs: 0 done out of 3 total
	done, total = c.habitStats("2026-03-20")
	if total != 3 {
		t.Fatalf("expected total=3 for date with no logs, got %d", total)
	}
	if done != 0 {
		t.Fatalf("expected done=0 for date with no logs, got %d", done)
	}

	// No entries at all: 0, 0
	c2 := NewCalendar()
	done, total = c2.habitStats("2026-03-25")
	if total != 0 || done != 0 {
		t.Fatalf("expected (0,0) with no habit entries, got (%d,%d)", done, total)
	}
}

// ---------------------------------------------------------------------------
// CalendarPanel — initial state, rendering, size
// ---------------------------------------------------------------------------

func TestCalendarPanel_NewCalendarPanel(t *testing.T) {
	cp := NewCalendarPanel()

	if cp.width != 0 {
		t.Errorf("expected initial width=0, got %d", cp.width)
	}
	if cp.height != 0 {
		t.Errorf("expected initial height=0, got %d", cp.height)
	}
	if cp.daysWithEvents == nil {
		t.Error("expected daysWithEvents map to be initialised")
	}
	if len(cp.plannerBlocks) != 0 {
		t.Errorf("expected empty plannerBlocks, got %d", len(cp.plannerBlocks))
	}
	if len(cp.upcomingTasks) != 0 {
		t.Errorf("expected empty upcomingTasks, got %d", len(cp.upcomingTasks))
	}
	if cp.now.IsZero() {
		t.Error("expected non-zero now time")
	}
}

func TestCalendarPanel_View_Empty(t *testing.T) {
	cp := NewCalendarPanel()
	cp.SetSize(40, 30)

	output := cp.View()
	if output == "" {
		t.Error("expected non-empty View() output even with no data")
	}
}

func TestCalendarPanel_SetSize(t *testing.T) {
	cp := NewCalendarPanel()
	cp.SetSize(80, 50)

	if cp.width != 80 {
		t.Errorf("expected width=80, got %d", cp.width)
	}
	if cp.height != 50 {
		t.Errorf("expected height=50, got %d", cp.height)
	}
}

// ---------------------------------------------------------------------------
// CalendarPanel — Refresh with planner blocks and tasks
// ---------------------------------------------------------------------------

func TestCalendarPanel_Refresh(t *testing.T) {
	cp := NewCalendarPanel()
	cp.SetSize(40, 30)

	todayStr := time.Now().Format("2006-01-02")

	plannerBlocks := map[string][]PlannerBlock{
		todayStr: {
			{Date: todayStr, StartTime: "09:00", EndTime: "10:00", Text: "Morning standup", BlockType: "event"},
			{Date: todayStr, StartTime: "14:00", EndTime: "15:00", Text: "Deep work", BlockType: "focus"},
		},
	}

	noteContents := map[string]string{
		"projects/alpha/tasks.md": "- [ ] Buy milk \U0001f4c5 " + todayStr + "\n- [x] Done task \U0001f4c5 " + todayStr,
	}

	cp.Refresh(plannerBlocks, noteContents)

	if len(cp.plannerBlocks) != 2 {
		t.Errorf("expected 2 planner blocks, got %d", len(cp.plannerBlocks))
	}

	// Blocks should be sorted by start time
	if len(cp.plannerBlocks) >= 2 && cp.plannerBlocks[0].StartTime > cp.plannerBlocks[1].StartTime {
		t.Error("expected planner blocks sorted by start time")
	}

	if len(cp.upcomingTasks) == 0 {
		t.Error("expected at least one upcoming task")
	}
}

func TestCalendarPanel_ViewWithSchedule(t *testing.T) {
	cp := NewCalendarPanel()
	cp.SetSize(60, 30)
	cp.plannerBlocks = []PlannerBlock{
		{StartTime: "09:00", Text: "Morning meeting", BlockType: "event"},
		{StartTime: "14:00", Text: "Code review", BlockType: "task"},
	}

	output := cp.View()
	if output == "" {
		t.Fatal("expected non-empty View() output")
	}
	plain := stripAnsiCodes(output)
	if !strings.Contains(plain, "09:00") {
		t.Error("expected schedule block time '09:00' in output")
	}
	if !strings.Contains(plain, "Morning meeting") {
		t.Error("expected schedule block text in output")
	}
	// "No scheduled blocks" should NOT appear
	if strings.Contains(plain, "No scheduled blocks") {
		t.Error("should not show 'No scheduled blocks' when blocks exist")
	}
}

func TestCalendarPanel_ViewWithTasks(t *testing.T) {
	cp := NewCalendarPanel()
	cp.SetSize(60, 30)

	todayStr := time.Now().Format("2006-01-02")
	cp.upcomingTasks = []calendarPanelTask{
		{Text: "Review PR", DueDate: todayStr, Priority: 3, Project: "granit"},
		{Text: "Write docs", DueDate: todayStr, Priority: 1},
	}

	output := cp.View()
	plain := stripAnsiCodes(output)

	if !strings.Contains(plain, "Review PR") {
		t.Error("expected task text 'Review PR' in output")
	}
	if !strings.Contains(plain, "granit") {
		t.Error("expected project name 'granit' in output")
	}
	if strings.Contains(plain, "No tasks due") {
		t.Error("should not show 'No tasks due' when tasks exist")
	}
}

// ---------------------------------------------------------------------------
// unscheduleBlockAtCursor — 'D' key in day/week views removes a planner block
// and propagates cleanup to the source task when a SourceRef is present.
// ---------------------------------------------------------------------------

func TestCalendar_UnscheduleBlockAtCursor_ClearsTaskAndPlanner(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Tasks.md": "# Tasks\n\n- [ ] Write report ⏰ 09:00-10:00\n",
	})
	today := time.Now().Format("2006-01-02")
	ref := ScheduleRef{NotePath: "Tasks.md", LineNum: 3, Text: "Write report"}
	// Seed a planner block so the Calendar has something at the cursor.
	if err := UpsertPlannerBlock(root, today, ref, PlannerBlock{
		Date: today, StartTime: "09:00", EndTime: "10:00",
		Text: "Write report", BlockType: "task", SourceRef: ref,
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	c := NewCalendar()
	c.vaultRoot = root
	c.view = calView1Day
	c.cursor = time.Now()
	c.plannerBlocks = map[string][]PlannerBlock{
		today: {
			{
				Date: today, StartTime: "09:00", EndTime: "10:00",
				Text: "Write report", BlockType: "task", SourceRef: ref,
			},
		},
	}
	// Put the grid cursor somewhere inside 09:00-10:00. weekGridStartHourFor
	// returns 6 by default, so half-hour row 6 corresponds to 09:00.
	c.weekGridCursorHour = 6

	c.unscheduleBlockAtCursor()

	if got := len(c.plannerBlocks[today]); got != 0 {
		t.Errorf("expected in-memory blocks cleared, got %d", got)
	}
	plan := readFile(t, root+"/Planner/"+today+".md")
	if strings.Contains(plan, "Write report") {
		t.Errorf("planner block not removed from disk:\n%s", plan)
	}
	src := readFile(t, root+"/Tasks.md")
	if strings.Contains(src, "⏰") {
		t.Errorf("source ⏰ marker not cleared:\n%s", src)
	}
}

func TestCalendar_ShiftBlockAtCursor_MovesBothSurfaces(t *testing.T) {
	root := newTestVault(t, map[string]string{
		"Tasks.md": "# Tasks\n\n- [ ] Write ⏰ 09:00-10:00\n",
	})
	today := time.Now().Format("2006-01-02")
	ref := ScheduleRef{NotePath: "Tasks.md", LineNum: 3, Text: "Write"}
	_ = UpsertPlannerBlock(root, today, ref, PlannerBlock{
		Date: today, StartTime: "09:00", EndTime: "10:00",
		Text: "Write", BlockType: "task", SourceRef: ref,
	})

	c := NewCalendar()
	c.vaultRoot = root
	c.view = calView1Day
	c.cursor = time.Now()
	c.plannerBlocks = map[string][]PlannerBlock{
		today: {{
			Date: today, StartTime: "09:00", EndTime: "10:00",
			Text: "Write", BlockType: "task", SourceRef: ref,
		}},
	}
	c.weekGridCursorHour = 6 // 09:00 when grid starts at 06:00

	c.shiftBlockAtCursor(15) // push 15 minutes later

	if got := c.plannerBlocks[today][0].StartTime; got != "09:15" {
		t.Errorf("in-memory start not shifted: got %q want 09:15", got)
	}
	plan := readFile(t, root+"/Planner/"+today+".md")
	if !strings.Contains(plan, "09:15-10:15 | Write | task") {
		t.Errorf("planner block not shifted on disk:\n%s", plan)
	}
	src := readFile(t, root+"/Tasks.md")
	if !strings.Contains(src, "⏰ 09:15-10:15") {
		t.Errorf("source ⏰ marker not shifted:\n%s", src)
	}
}

func TestCalendar_ShiftBlockAtCursor_ClampsBeforeMidnight(t *testing.T) {
	root := t.TempDir()
	today := time.Now().Format("2006-01-02")
	c := NewCalendar()
	c.vaultRoot = root
	c.view = calView1Day
	c.cursor = time.Now()
	c.plannerBlocks = map[string][]PlannerBlock{
		today: {{
			Date: today, StartTime: "00:00", EndTime: "00:30",
			Text: "Early", BlockType: "task",
		}},
	}
	c.weekGridCursorHour = 0 // cursor at start

	// Attempt to shift before midnight — must be a no-op.
	c.shiftBlockAtCursor(-15)
	if got := c.plannerBlocks[today][0].StartTime; got != "00:00" {
		t.Errorf("expected clamp no-op, got %q", got)
	}
}

func TestCalendar_BlockAtCursor_PrefersNonPomodoro(t *testing.T) {
	today := time.Now().Format("2006-01-02")
	c := NewCalendar()
	c.view = calView1Day
	c.cursor = time.Now()
	// Two overlapping blocks at the same time — a planned task and a
	// completed pomodoro session. The cursor action (D/,/.) should target
	// the plan, not the audit trail.
	c.plannerBlocks = map[string][]PlannerBlock{
		today: {
			{StartTime: "09:00", EndTime: "09:25", BlockType: "pomodoro", Text: "Session", Done: true},
			{StartTime: "09:00", EndTime: "10:00", BlockType: "task", Text: "Planned"},
		},
	}
	c.weekGridCursorHour = 6 // 09:00 (grid starts at 06:00 by default)

	_, pb := c.blockAtCursor(today)
	if pb == nil {
		t.Fatal("expected a block at cursor")
	}
	if pb.BlockType != "task" {
		t.Errorf("expected task block, got %q (pomodoro should not win)", pb.BlockType)
	}
}

func TestCalendar_ShiftBlockAtCursor_ClampsCursorToVisibleGrid(t *testing.T) {
	root := t.TempDir()
	today := time.Now().Format("2006-01-02")
	c := NewCalendar()
	c.vaultRoot = root
	c.view = calView1Day
	c.cursor = time.Now()
	// Smallest possible grid (height too small means maxGridSlots clamps
	// to 16). Place a block at the very end of the day.
	c.height = 20
	c.plannerBlocks = map[string][]PlannerBlock{
		today: {{StartTime: "13:00", EndTime: "14:00", BlockType: "task", Text: "Late"}},
	}
	c.weekGridCursorHour = 14 // ≈ 13:00 if grid starts at 06:00

	c.shiftBlockAtCursor(60 * 9) // push to 22:00 — past visible window

	if got, max := c.weekGridCursorHour, c.maxGridSlots()-1; got > max {
		t.Errorf("cursor not clamped: got %d, max %d", got, max)
	}
}

func TestCalendar_ShiftBlockAtCursor_CursorFollowsBlock(t *testing.T) {
	root := t.TempDir()
	today := time.Now().Format("2006-01-02")
	c := NewCalendar()
	c.vaultRoot = root
	c.view = calView1Day
	c.cursor = time.Now()
	c.plannerBlocks = map[string][]PlannerBlock{
		today: {{StartTime: "09:00", EndTime: "10:00", BlockType: "task", Text: "Move me"}},
	}
	c.weekGridCursorHour = 6 // 09:00 when grid starts at 06:00

	c.shiftBlockAtCursor(30) // push to 09:30

	if got := c.plannerBlocks[today][0].StartTime; got != "09:30" {
		t.Fatalf("block start not shifted: %q", got)
	}
	// Cursor must track the block so a second shift still hits it.
	if got := c.cursorSlotMinutes(); got != 9*60+30 {
		t.Errorf("cursor did not follow block: at %d mins, want %d", got, 9*60+30)
	}
}

func TestCalendar_UnscheduleBlockAtCursor_NoOpWhenNothingSelected(t *testing.T) {
	root := t.TempDir()
	c := NewCalendar()
	c.vaultRoot = root
	c.view = calView1Day
	c.cursor = time.Now()
	c.plannerBlocks = map[string][]PlannerBlock{}
	c.weekGridCursorHour = 6
	// Must not panic or error.
	c.unscheduleBlockAtCursor()
}
