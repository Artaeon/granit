package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

// ---------------------------------------------------------------------------
// Task data
// ---------------------------------------------------------------------------

// Task represents a single task extracted from a markdown note.
type Task struct {
	Text          string   `json:"text"`
	Done          bool     `json:"done"`
	DueDate       string   `json:"due_date"`           // "2006-01-02" or ""
	Priority      int      `json:"priority"`            // 0=none, 1=low, 2=medium, 3=high, 4=highest
	ScheduledTime string   `json:"scheduled_time,omitempty"` // "HH:MM-HH:MM" or "" — set by AI scheduler
	Tags          []string `json:"tags,omitempty"`
	NotePath      string   `json:"note_path"`           // source note relative path
	LineNum       int      `json:"line_num"`             // 1-based line number in source note
}

// taskView identifies which tab is active.
type taskView int

const (
	taskViewToday     taskView = iota // 0
	taskViewUpcoming                  // 1
	taskViewAll                       // 2
	taskViewCompleted                 // 3
	taskViewCalendar                  // 4
	taskViewKanban                    // 5
)

// inputMode tracks sub-modes.
type tmInputMode int

const (
	tmInputNone   tmInputMode = iota
	tmInputAdd                // adding a new task
	tmInputDate               // date picker
	tmInputSearch             // filtering
)

// ---------------------------------------------------------------------------
// Regex patterns
// ---------------------------------------------------------------------------

var (
	tmTaskRe       = regexp.MustCompile(`^(\s*- \[)([ xX])(\] .+)`)
	tmDueDateRe    = regexp.MustCompile(`\x{1F4C5}\s*(\d{4}-\d{2}-\d{2})`)
	tmPrioHighestRe = regexp.MustCompile(`\x{1F53A}`)  // 🔺
	tmPrioHighRe   = regexp.MustCompile(`\x{23EB}`)    // ⏫
	tmPrioMedRe    = regexp.MustCompile(`\x{1F53C}`)   // 🔼
	tmPrioLowRe    = regexp.MustCompile(`\x{1F53D}`)   // 🔽
	tmTagRe        = regexp.MustCompile(`#([A-Za-z0-9_/-]+)`)
	tmScheduleRe   = regexp.MustCompile(`⏰\s*(\d{2}:\d{2}-\d{2}:\d{2})`)
)

// ---------------------------------------------------------------------------
// TaskManager overlay
// ---------------------------------------------------------------------------

// TaskManager is the comprehensive task management overlay.
type TaskManager struct {
	active bool
	width  int
	height int

	// Data
	vault     *vault.Vault
	allTasks  []Task
	filtered  []Task // currently displayed tasks

	// Navigation
	view   taskView
	cursor int
	scroll int

	// Input
	inputMode tmInputMode
	inputBuf  string

	// Date picker state
	datePickerDate time.Time // currently selected date in picker

	// Calendar sub-view state
	calYear  int
	calMonth time.Month
	calDay   int // selected day (1-based), 0 = none

	// Kanban state
	kanbanCol    int      // 0-3: which column cursor is in
	kanbanCursor [4]int   // cursor position within each column
	kanbanScroll [4]int   // scroll position within each column

	// Status message
	statusMsg string

	// Consumed-once: jump result
	jumpPath string
	jumpLine int
	jumpOK   bool

	// File change tracking
	fileChanged    bool
	lastChangedNote string // path of most recently modified note
}

// NewTaskManager creates a new TaskManager overlay.
func NewTaskManager() TaskManager {
	now := time.Now()
	return TaskManager{
		calYear:  now.Year(),
		calMonth: now.Month(),
		calDay:   now.Day(),
	}
}

// IsActive returns whether the task manager overlay is visible.
func (tm *TaskManager) IsActive() bool {
	return tm.active
}

// SetSize updates the available dimensions.
func (tm *TaskManager) SetSize(width, height int) {
	tm.width = width
	tm.height = height
}

// Open parses all tasks from the vault and activates the overlay.
func (tm *TaskManager) Open(v *vault.Vault) {
	tm.active = true
	tm.vault = v
	tm.allTasks = ParseAllTasks(v.Notes)
	tm.view = taskViewToday
	tm.cursor = 0
	tm.scroll = 0
	tm.inputMode = tmInputNone
	tm.inputBuf = ""
	tm.statusMsg = ""
	tm.jumpOK = false
	tm.fileChanged = false
	tm.lastChangedNote = ""
	tm.kanbanCol = 0
	tm.kanbanCursor = [4]int{}
	tm.kanbanScroll = [4]int{}
	now := time.Now()
	tm.calYear = now.Year()
	tm.calMonth = now.Month()
	tm.calDay = now.Day()
	tm.rebuildFiltered()
}

// Close hides the overlay.
func (tm *TaskManager) Close() {
	tm.active = false
}

// Refresh re-parses all tasks from the vault without resetting UI state.
// This is used when another component modifies vault files while the task
// manager is still open, so the displayed task list stays current.
func (tm *TaskManager) Refresh(v *vault.Vault) {
	tm.vault = v
	tm.allTasks = ParseAllTasks(v.Notes)
	tm.rebuildFiltered()
	if tm.cursor >= len(tm.filtered) {
		tm.cursor = maxInt(0, len(tm.filtered)-1)
	}
}

// GetJumpResult returns a consumed-once jump result.
func (tm *TaskManager) GetJumpResult() (notePath string, lineNum int, ok bool) {
	if !tm.jumpOK {
		return "", 0, false
	}
	p, l := tm.jumpPath, tm.jumpLine
	tm.jumpOK = false
	return p, l, true
}

// WasFileChanged returns true (consumed-once) if any file was modified.
func (tm *TaskManager) WasFileChanged() bool {
	if tm.fileChanged {
		tm.fileChanged = false
		return true
	}
	return false
}

// ActiveNotePath returns the path of the note that was most recently modified.
func (tm *TaskManager) ActiveNotePath() string {
	return tm.lastChangedNote
}

// CountTasksDueToday returns the number of incomplete tasks due today or overdue.
func CountTasksDueToday(notes map[string]*vault.Note) int {
	count := 0
	for _, note := range notes {
		if note.Content == "" {
			continue
		}
		for _, line := range strings.Split(note.Content, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- [ ]") {
				continue
			}
			if dm := tmDueDateRe.FindStringSubmatch(line); dm != nil {
				if tmIsToday(dm[1]) || tmIsOverdue(dm[1]) {
					count++
				}
			}
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

// ParseAllTasks scans all note content for task lines.
func ParseAllTasks(notes map[string]*vault.Note) []Task {
	var tasks []Task
	for _, note := range notes {
		if note.Content == "" {
			continue
		}
		lines := strings.Split(note.Content, "\n")
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- [") {
				continue
			}
			m := tmTaskRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			done := m[2] == "x" || m[2] == "X"
			taskText := m[3][2:] // strip "] " prefix

			t := Task{
				Text:     taskText,
				Done:     done,
				NotePath: note.RelPath,
				LineNum:  i + 1,
			}

			// Due date
			if dm := tmDueDateRe.FindStringSubmatch(taskText); dm != nil {
				t.DueDate = dm[1]
			}

			// Priority (check highest first)
			if tmPrioHighestRe.MatchString(taskText) {
				t.Priority = 4
			} else if tmPrioHighRe.MatchString(taskText) {
				t.Priority = 3
			} else if tmPrioMedRe.MatchString(taskText) {
				t.Priority = 2
			} else if tmPrioLowRe.MatchString(taskText) {
				t.Priority = 1
			}

			// Scheduled time (set by AI scheduler)
			if sm := tmScheduleRe.FindStringSubmatch(taskText); sm != nil {
				t.ScheduledTime = sm[1]
			}

			// Tags
			for _, tm := range tmTagRe.FindAllStringSubmatch(taskText, -1) {
				t.Tags = append(t.Tags, tm[1])
			}

			tasks = append(tasks, t)
		}
	}
	return tasks
}

// ---------------------------------------------------------------------------
// Date helpers
// ---------------------------------------------------------------------------

func tmToday() string {
	return time.Now().Format("2006-01-02")
}

func tmIsToday(dateStr string) bool {
	return dateStr == tmToday()
}

func tmIsOverdue(dateStr string) bool {
	if dateStr == "" {
		return false
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return false
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return t.Before(today)
}


func tmDaysUntil(dateStr string) int {
	if dateStr == "" {
		return 9999
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return 9999
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return int(t.Sub(today).Hours() / 24)
}

func tmFormatDue(dateStr string) string {
	if dateStr == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	days := tmDaysUntil(dateStr)
	switch {
	case days < 0:
		return fmt.Sprintf("%dd overdue", -days)
	case days == 0:
		return "today"
	case days == 1:
		return "tomorrow"
	case days <= 7:
		return t.Format("Mon")
	default:
		return t.Format("Jan 2")
	}
}

func tmDaysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// tmNextMonday returns the next Monday from today.
func tmNextMonday() time.Time {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	daysUntilMon := (8 - int(today.Weekday())) % 7
	if daysUntilMon == 0 {
		daysUntilMon = 7
	}
	return today.AddDate(0, 0, daysUntilMon)
}

// tmFormatDateLong formats a date for the date picker preview.
func tmFormatDateLong(t time.Time) string {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	diff := int(d.Sub(today).Hours() / 24)
	label := t.Format("Monday, Jan 2")
	switch {
	case diff < 0:
		return fmt.Sprintf("%s (%dd ago)", label, -diff)
	case diff == 0:
		return label + " (today)"
	case diff == 1:
		return label + " (tomorrow)"
	default:
		return fmt.Sprintf("%s (+%dd)", label, diff)
	}
}

// ---------------------------------------------------------------------------
// Direct file I/O helpers
// ---------------------------------------------------------------------------

// writeLineChange modifies a specific line in a note file and writes it back.
func (tm *TaskManager) writeLineChange(notePath string, lineNum int, transform func(string) string) bool {
	if tm.vault == nil {
		return false
	}
	note := tm.vault.GetNote(notePath)
	if note == nil {
		return false
	}
	lines := strings.Split(note.Content, "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return false
	}
	lines[lineNum-1] = transform(lines[lineNum-1])
	newContent := strings.Join(lines, "\n")
	note.Content = newContent
	absPath := filepath.Join(tm.vault.Root, notePath)
	if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
		return false
	}
	tm.fileChanged = true
	tm.lastChangedNote = notePath
	return true
}

// reparse re-parses all tasks from the vault and rebuilds the filtered list.
func (tm *TaskManager) reparse() {
	savedView := tm.view
	savedCursor := tm.cursor
	savedScroll := tm.scroll
	tm.allTasks = ParseAllTasks(tm.vault.Notes)
	tm.view = savedView
	tm.rebuildFiltered()
	tm.scroll = savedScroll
	if savedCursor >= len(tm.filtered) {
		savedCursor = len(tm.filtered) - 1
	}
	if savedCursor < 0 {
		savedCursor = 0
	}
	tm.cursor = savedCursor
}

// doToggle toggles the done state of the task at the current cursor.
func (tm *TaskManager) doToggle(task Task) {
	newDone := !task.Done
	ok := tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		if newDone {
			return strings.Replace(line, "[ ]", "[x]", 1)
		}
		line = strings.Replace(line, "[x]", "[ ]", 1)
		line = strings.Replace(line, "[X]", "[ ]", 1)
		return line
	})
	if ok {
		if newDone {
			tm.statusMsg = "Task completed"
		} else {
			tm.statusMsg = "Task reopened"
		}
		tm.reparse()
	}
}

// doSetDate sets the due date on the task at the current cursor.
func (tm *TaskManager) doSetDate(task Task, newDate string) {
	ok := tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		line = tmDueDateRe.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " ")
		line += " \U0001F4C5 " + newDate
		return line
	})
	if ok {
		dt, _ := time.Parse("2006-01-02", newDate)
		tm.statusMsg = "Due date set to " + dt.Format("Jan 2")
		tm.reparse()
	}
}

// doCyclePriority cycles the priority of the task at the current cursor.
func (tm *TaskManager) doCyclePriority(task Task) {
	newPrio := (task.Priority + 1) % 5 // 0→1→2→3→4→0
	ok := tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		// Remove all existing priority markers
		line = tmPrioHighestRe.ReplaceAllString(line, "")
		line = tmPrioHighRe.ReplaceAllString(line, "")
		line = tmPrioMedRe.ReplaceAllString(line, "")
		line = tmPrioLowRe.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " ")
		if newPrio > 0 {
			line += " " + tmPriorityIcon(newPrio)
		}
		return line
	})
	if ok {
		tm.reparse()
	}
}

// doAddTask adds a new task to Tasks.md in vault root.
func (tm *TaskManager) doAddTask(taskText string) {
	if tm.vault == nil {
		return
	}
	taskLine := "- [ ] " + strings.TrimSpace(taskText)
	// If in calendar view, append the selected date
	if tm.view == taskViewCalendar && tm.calDay > 0 {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", tm.calYear, int(tm.calMonth), tm.calDay)
		if !strings.Contains(taskLine, "\U0001F4C5") {
			taskLine += " \U0001F4C5 " + dateStr
		}
	}

	targetPath := "Tasks.md"
	absPath := filepath.Join(tm.vault.Root, targetPath)

	note := tm.vault.GetNote(targetPath)
	if note == nil {
		// Create Tasks.md with header
		header := "# Tasks\n\n"
		if err := os.WriteFile(absPath, []byte(header), 0644); err != nil {
			return
		}
		// Re-scan to pick up the new file
		_ = tm.vault.Scan()
		note = tm.vault.GetNote(targetPath)
		if note == nil {
			return
		}
	}

	newContent := note.Content
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}
	newContent += taskLine + "\n"
	note.Content = newContent
	if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
		return
	}

	tm.fileChanged = true
	tm.lastChangedNote = targetPath
	tm.statusMsg = "Task added to Tasks.md"

	// Switch to All view so the task is visible
	tm.view = taskViewAll
	tm.reparse()
}

// doKanbanMove moves a task between kanban columns by adding/removing tags.
func (tm *TaskManager) doKanbanMove(task Task, direction int) {
	cols := tm.kanbanColumns()
	// Find which column the task is currently in
	currentCol := -1
	for c := 0; c < 4; c++ {
		for _, t := range cols[c] {
			if t.NotePath == task.NotePath && t.LineNum == task.LineNum {
				currentCol = c
				break
			}
		}
		if currentCol >= 0 {
			break
		}
	}
	if currentCol < 0 {
		return
	}

	targetCol := currentCol + direction
	if targetCol < 0 || targetCol > 3 {
		return
	}

	tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		// Remove #doing and #inprogress tags
		doingRe := regexp.MustCompile(`\s*#doing\b`)
		inprogRe := regexp.MustCompile(`\s*#inprogress\b`)
		line = doingRe.ReplaceAllString(line, "")
		line = inprogRe.ReplaceAllString(line, "")

		switch targetCol {
		case 0: // Backlog: remove date, uncheck
			line = tmDueDateRe.ReplaceAllString(line, "")
			line = strings.Replace(line, "[x]", "[ ]", 1)
			line = strings.Replace(line, "[X]", "[ ]", 1)
		case 1: // Todo: uncheck (keep date)
			line = strings.Replace(line, "[x]", "[ ]", 1)
			line = strings.Replace(line, "[X]", "[ ]", 1)
		case 2: // In Progress: add #doing, uncheck
			line = strings.Replace(line, "[x]", "[ ]", 1)
			line = strings.Replace(line, "[X]", "[ ]", 1)
			line = strings.TrimRight(line, " ")
			line += " #doing"
		case 3: // Done: check
			line = strings.Replace(line, "[ ]", "[x]", 1)
		}
		line = strings.TrimRight(line, " ")
		return line
	})
	tm.reparse()
	// Move cursor to target column
	tm.kanbanCol = targetCol
	// Rebuild columns and clamp cursor
	newCols := tm.kanbanColumns()
	if tm.kanbanCursor[targetCol] >= len(newCols[targetCol]) {
		tm.kanbanCursor[targetCol] = len(newCols[targetCol]) - 1
	}
	if tm.kanbanCursor[targetCol] < 0 {
		tm.kanbanCursor[targetCol] = 0
	}
}

// ---------------------------------------------------------------------------
// Filtering / sorting
// ---------------------------------------------------------------------------

func (tm *TaskManager) rebuildFiltered() {
	tm.cursor = 0
	tm.scroll = 0
	switch tm.view {
	case taskViewToday:
		tm.filtered = tm.filterToday()
	case taskViewUpcoming:
		tm.filtered = tm.filterUpcoming()
	case taskViewAll:
		tm.filtered = tm.filterAll()
	case taskViewCompleted:
		tm.filtered = tm.filterCompleted()
	case taskViewCalendar:
		tm.filtered = tm.filterCalendarDay()
	case taskViewKanban:
		tm.filtered = nil // kanban uses columns, not flat list
	}
	if tm.inputMode == tmInputSearch && tm.inputBuf != "" {
		tm.filtered = tm.applySearch(tm.filtered)
	}
}

func (tm *TaskManager) filterToday() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if t.Done {
			continue
		}
		if tmIsToday(t.DueDate) || tmIsOverdue(t.DueDate) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].DueDate < out[j].DueDate
	})
	return out
}

func (tm *TaskManager) filterUpcoming() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if t.Done {
			continue
		}
		if t.DueDate == "" {
			continue
		}
		days := tmDaysUntil(t.DueDate)
		if days >= 0 && days <= 7 {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].DueDate != out[j].DueDate {
			return out[i].DueDate < out[j].DueDate
		}
		return out[i].Priority > out[j].Priority
	})
	return out
}

func (tm *TaskManager) filterAll() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if t.Done {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].DueDate == "") != (out[j].DueDate == "") {
			return out[i].DueDate != ""
		}
		if out[i].DueDate != out[j].DueDate {
			return out[i].DueDate < out[j].DueDate
		}
		return out[i].Priority > out[j].Priority
	})
	return out
}

func (tm *TaskManager) filterCompleted() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if t.Done {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].NotePath != out[j].NotePath {
			return out[i].NotePath < out[j].NotePath
		}
		return out[i].LineNum < out[j].LineNum
	})
	return out
}

func (tm *TaskManager) filterCalendarDay() []Task {
	if tm.calDay == 0 {
		return nil
	}
	dateStr := fmt.Sprintf("%04d-%02d-%02d", tm.calYear, int(tm.calMonth), tm.calDay)
	var out []Task
	for _, t := range tm.allTasks {
		if t.DueDate == dateStr {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Priority > out[j].Priority
	})
	return out
}

func (tm *TaskManager) applySearch(tasks []Task) []Task {
	query := strings.ToLower(tm.inputBuf)
	var out []Task
	for _, t := range tasks {
		if strings.Contains(strings.ToLower(t.Text), query) ||
			strings.Contains(strings.ToLower(t.NotePath), query) {
			out = append(out, t)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Kanban columns
// ---------------------------------------------------------------------------

// kanbanColumns returns 4 columns: Backlog, Todo, In Progress, Done.
func (tm *TaskManager) kanbanColumns() [4][]Task {
	var cols [4][]Task

	for _, t := range tm.allTasks {
		hasDoingTag := false
		for _, tag := range t.Tags {
			if tag == "doing" || tag == "inprogress" {
				hasDoingTag = true
				break
			}
		}

		switch {
		case t.Done:
			cols[3] = append(cols[3], t)
		case hasDoingTag:
			cols[2] = append(cols[2], t)
		case t.DueDate != "":
			cols[1] = append(cols[1], t)
		default:
			cols[0] = append(cols[0], t)
		}
	}

	// Sort each column by priority descending
	for c := 0; c < 4; c++ {
		sort.Slice(cols[c], func(i, j int) bool {
			return cols[c][i].Priority > cols[c][j].Priority
		})
	}

	return cols
}

// ---------------------------------------------------------------------------
// Priority helpers
// ---------------------------------------------------------------------------

func tmPriorityIcon(p int) string {
	switch p {
	case 4:
		return "\U0001F53A" // 🔺
	case 3:
		return "\u23EB" // ⏫
	case 2:
		return "\U0001F53C" // 🔼
	case 1:
		return "\U0001F53D" // 🔽
	default:
		return " "
	}
}

func tmPriorityColor(p int) lipgloss.Color {
	switch p {
	case 4:
		return red
	case 3:
		return red
	case 2:
		return yellow
	case 1:
		return blue
	default:
		return overlay0
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles keyboard input for the task manager.
func (tm TaskManager) Update(msg tea.Msg) (TaskManager, tea.Cmd) {
	if !tm.active {
		return tm, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Clear status message on any keypress
		tm.statusMsg = ""

		// Handle input modes first
		if tm.inputMode != tmInputNone {
			return tm.updateInput(msg)
		}
		if tm.view == taskViewKanban {
			return tm.updateKanban(msg)
		}
		return tm.updateNormal(msg)
	}
	return tm, nil
}

func (tm TaskManager) updateInput(msg tea.KeyMsg) (TaskManager, tea.Cmd) {
	key := msg.String()

	switch tm.inputMode {
	case tmInputDate:
		return tm.updateDatePicker(key)
	case tmInputAdd:
		return tm.updateAddInput(key)
	case tmInputSearch:
		return tm.updateSearchInput(key)
	}

	return tm, nil
}

func (tm TaskManager) updateAddInput(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil

	case "enter":
		if strings.TrimSpace(tm.inputBuf) != "" {
			tm.doAddTask(tm.inputBuf)
		}
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil

	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = tm.inputBuf[:len(tm.inputBuf)-1]
		}
		return tm, nil

	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
		}
		return tm, nil
	}
}

func (tm TaskManager) updateSearchInput(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		tm.rebuildFiltered()
		return tm, nil

	case "enter":
		// Keep search active, just confirm
		return tm, nil

	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = tm.inputBuf[:len(tm.inputBuf)-1]
			tm.rebuildFiltered()
		}
		return tm, nil

	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
			tm.rebuildFiltered()
		}
		return tm, nil
	}
}

func (tm TaskManager) updateDatePicker(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		return tm, nil

	case "enter":
		// Confirm date
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			newDate := tm.datePickerDate.Format("2006-01-02")
			tm.doSetDate(task, newDate)
		}
		tm.inputMode = tmInputNone
		return tm, nil

	case "t":
		// Today
		now := time.Now()
		tm.datePickerDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		return tm, nil

	case "m":
		// Tomorrow
		now := time.Now()
		tm.datePickerDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, 1)
		return tm, nil

	case "w":
		// Next Monday
		tm.datePickerDate = tmNextMonday()
		return tm, nil

	case "+", "right":
		tm.datePickerDate = tm.datePickerDate.AddDate(0, 0, 1)
		return tm, nil

	case "-", "left":
		tm.datePickerDate = tm.datePickerDate.AddDate(0, 0, -1)
		return tm, nil

	default:
		return tm, nil
	}
}

func (tm TaskManager) updateKanban(msg tea.KeyMsg) (TaskManager, tea.Cmd) {
	key := msg.String()
	cols := tm.kanbanColumns()

	switch key {
	case "esc", "q":
		tm.active = false
		return tm, nil

	case "1":
		tm.view = taskViewToday
		tm.rebuildFiltered()
	case "2":
		tm.view = taskViewUpcoming
		tm.rebuildFiltered()
	case "3":
		tm.view = taskViewAll
		tm.rebuildFiltered()
	case "4":
		tm.view = taskViewCompleted
		tm.rebuildFiltered()
	case "5":
		tm.view = taskViewCalendar
		tm.rebuildFiltered()
	case "6":
		// Already on kanban
	case "tab":
		tm.view = (tm.view + 1) % 6
		tm.rebuildFiltered()

	case "h", "left":
		if tm.kanbanCol > 0 {
			tm.kanbanCol--
		}
	case "l", "right":
		if tm.kanbanCol < 3 {
			tm.kanbanCol++
		}

	case "j", "down":
		col := cols[tm.kanbanCol]
		if tm.kanbanCursor[tm.kanbanCol] < len(col)-1 {
			tm.kanbanCursor[tm.kanbanCol]++
			visH := tm.kanbanVisibleHeight()
			if tm.kanbanCursor[tm.kanbanCol] >= tm.kanbanScroll[tm.kanbanCol]+visH {
				tm.kanbanScroll[tm.kanbanCol] = tm.kanbanCursor[tm.kanbanCol] - visH + 1
			}
		}
	case "k", "up":
		if tm.kanbanCursor[tm.kanbanCol] > 0 {
			tm.kanbanCursor[tm.kanbanCol]--
			if tm.kanbanCursor[tm.kanbanCol] < tm.kanbanScroll[tm.kanbanCol] {
				tm.kanbanScroll[tm.kanbanCol] = tm.kanbanCursor[tm.kanbanCol]
			}
		}

	case "x", "enter":
		col := cols[tm.kanbanCol]
		if tm.kanbanCursor[tm.kanbanCol] < len(col) {
			task := col[tm.kanbanCursor[tm.kanbanCol]]
			tm.doToggle(task)
		}

	case ">":
		col := cols[tm.kanbanCol]
		if tm.kanbanCol < 3 && tm.kanbanCursor[tm.kanbanCol] < len(col) {
			task := col[tm.kanbanCursor[tm.kanbanCol]]
			tm.doKanbanMove(task, 1)
		}

	case "<":
		col := cols[tm.kanbanCol]
		if tm.kanbanCol > 0 && tm.kanbanCursor[tm.kanbanCol] < len(col) {
			task := col[tm.kanbanCursor[tm.kanbanCol]]
			tm.doKanbanMove(task, -1)
		}

	case "g":
		col := cols[tm.kanbanCol]
		if tm.kanbanCursor[tm.kanbanCol] < len(col) {
			task := col[tm.kanbanCursor[tm.kanbanCol]]
			tm.jumpPath = task.NotePath
			tm.jumpLine = task.LineNum
			tm.jumpOK = true
			tm.active = false
		}

	case "a":
		tm.inputMode = tmInputAdd
		tm.inputBuf = ""

	case "/":
		tm.inputMode = tmInputSearch
		tm.inputBuf = ""
	}

	return tm, nil
}

func (tm TaskManager) updateNormal(msg tea.KeyMsg) (TaskManager, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc", "q":
		tm.active = false
		return tm, nil

	// Tab switching with number keys
	case "1":
		tm.view = taskViewToday
		tm.rebuildFiltered()
	case "2":
		tm.view = taskViewUpcoming
		tm.rebuildFiltered()
	case "3":
		tm.view = taskViewAll
		tm.rebuildFiltered()
	case "4":
		tm.view = taskViewCompleted
		tm.rebuildFiltered()
	case "5":
		tm.view = taskViewCalendar
		tm.rebuildFiltered()
	case "6":
		tm.view = taskViewKanban
		tm.rebuildFiltered()

	case "tab":
		tm.view = (tm.view + 1) % 6
		tm.rebuildFiltered()

	// Navigation
	case "up", "k":
		if tm.view == taskViewCalendar && len(tm.filtered) == 0 {
			tm.calDay -= 7
			if tm.calDay < 1 {
				tm.calMonth--
				if tm.calMonth < 1 {
					tm.calMonth = 12
					tm.calYear--
				}
				tm.calDay += tmDaysInMonth(tm.calYear, tm.calMonth)
			}
			tm.rebuildFiltered()
		} else if tm.cursor > 0 {
			tm.cursor--
			if tm.cursor < tm.scroll {
				tm.scroll = tm.cursor
			}
		}

	case "down", "j":
		if tm.view == taskViewCalendar && len(tm.filtered) == 0 {
			maxDay := tmDaysInMonth(tm.calYear, tm.calMonth)
			tm.calDay += 7
			if tm.calDay > maxDay {
				tm.calDay -= maxDay
				tm.calMonth++
				if tm.calMonth > 12 {
					tm.calMonth = 1
					tm.calYear++
				}
			}
			tm.rebuildFiltered()
		} else if tm.cursor < len(tm.filtered)-1 {
			tm.cursor++
			visH := tm.visibleHeight()
			if tm.cursor >= tm.scroll+visH {
				tm.scroll = tm.cursor - visH + 1
			}
		}

	case "h", "left":
		if tm.view == taskViewCalendar {
			tm.calMonth--
			if tm.calMonth < 1 {
				tm.calMonth = 12
				tm.calYear--
			}
			maxDay := tmDaysInMonth(tm.calYear, tm.calMonth)
			if tm.calDay > maxDay {
				tm.calDay = maxDay
			}
			tm.rebuildFiltered()
		}

	case "l", "right":
		if tm.view == taskViewCalendar {
			tm.calMonth++
			if tm.calMonth > 12 {
				tm.calMonth = 1
				tm.calYear++
			}
			maxDay := tmDaysInMonth(tm.calYear, tm.calMonth)
			if tm.calDay > maxDay {
				tm.calDay = maxDay
			}
			tm.rebuildFiltered()
		}

	// Toggle completion
	case "enter", "x":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			tm.doToggle(task)
		} else if tm.view == taskViewCalendar && len(tm.filtered) == 0 {
			tm.rebuildFiltered()
		}

	// Jump to source
	case "g":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			tm.jumpPath = task.NotePath
			tm.jumpLine = task.LineNum
			tm.jumpOK = true
			tm.active = false
		}

	// Add new task
	case "a":
		tm.inputMode = tmInputAdd
		tm.inputBuf = ""

	// Set due date (date picker)
	case "d":
		if tm.cursor < len(tm.filtered) {
			tm.inputMode = tmInputDate
			task := tm.filtered[tm.cursor]
			if task.DueDate != "" {
				if t, err := time.Parse("2006-01-02", task.DueDate); err == nil {
					tm.datePickerDate = t
				} else {
					now := time.Now()
					tm.datePickerDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
				}
			} else {
				now := time.Now()
				tm.datePickerDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
			}
		}

	// Cycle priority
	case "p":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			tm.doCyclePriority(task)
		}

	// Search
	case "/":
		tm.inputMode = tmInputSearch
		tm.inputBuf = ""
	}

	return tm, nil
}

func (tm *TaskManager) visibleHeight() int {
	h := tm.height - 14
	if h < 3 {
		h = 3
	}
	return h
}

func (tm *TaskManager) kanbanVisibleHeight() int {
	h := tm.height - 12
	if h < 3 {
		h = 3
	}
	return h
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the task manager overlay.
func (tm TaskManager) View() string {
	width := tm.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	// Kanban needs more width
	if tm.view == taskViewKanban {
		width = tm.width * 4 / 5
		if width < 80 {
			width = 80
		}
		if width > 140 {
			width = 140
		}
	}

	innerW := width - 8 // account for border + padding

	var b strings.Builder

	// Title bar
	tm.renderTitle(&b, innerW)

	// Tab bar
	tm.renderTabs(&b, innerW)

	// View content
	switch tm.view {
	case taskViewCalendar:
		tm.renderCalendarView(&b, innerW)
	case taskViewKanban:
		tm.renderKanbanView(&b, innerW)
	default:
		tm.renderTaskList(&b, innerW)
	}

	// Input bar (if active)
	tm.renderInput(&b, innerW)

	// Status message
	if tm.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render(tm.statusMsg))
	}

	// Help bar
	tm.renderHelp(&b, innerW)

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (tm *TaskManager) renderTitle(b *strings.Builder, w int) {
	icon := lipgloss.NewStyle().Foreground(blue).Render(IconOutlineChar)
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Tasks")

	total := 0
	done := 0
	for _, t := range tm.allTasks {
		total++
		if t.Done {
			done++
		}
	}
	stats := lipgloss.NewStyle().Foreground(overlay0).
		Render(fmt.Sprintf("  %d/%d done", done, total))

	b.WriteString("  " + icon + title + stats)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", w-4)))
	b.WriteString("\n")
}

func (tm *TaskManager) renderTabs(b *strings.Builder, w int) {
	activeStyle := lipgloss.NewStyle().
		Foreground(crust).
		Background(mauve).
		Bold(true).
		Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(overlay0).
		Background(surface0).
		Padding(0, 1)

	names := []string{
		"Today",
		"Upcoming",
		"All",
		"Done",
		"Calendar",
		"Kanban",
	}
	counts := []int{
		len(tm.filterToday()),
		len(tm.filterUpcoming()),
		len(tm.filterAll()),
		len(tm.filterCompleted()),
		-1, // calendar doesn't show count
		-1, // kanban doesn't show count
	}

	var tabs []string
	for i, name := range names {
		label := fmt.Sprintf("%d:%s", i+1, name)
		if counts[i] >= 0 {
			label += fmt.Sprintf(" %d", counts[i])
		}
		if taskView(i) == tm.view {
			tabs = append(tabs, activeStyle.Render(label))
		} else {
			tabs = append(tabs, inactiveStyle.Render(label))
		}
	}

	b.WriteString("  " + strings.Join(tabs, " "))
	b.WriteString("\n\n")
}

func (tm *TaskManager) renderTaskList(b *strings.Builder, w int) {
	if len(tm.filtered) == 0 {
		emptyMsg := "No tasks"
		hint := "Press 'a' to add a task"
		switch tm.view {
		case taskViewToday:
			emptyMsg = "No tasks due today"
		case taskViewUpcoming:
			emptyMsg = "No upcoming tasks this week"
		case taskViewCompleted:
			emptyMsg = "No completed tasks"
			hint = "Complete a task with 'x' to see it here"
		case taskViewAll:
			emptyMsg = "No tasks in vault"
		}
		b.WriteString(DimStyle.Render("  " + emptyMsg))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + hint))
		b.WriteString("\n")
		return
	}

	visH := tm.visibleHeight()
	end := tm.scroll + visH
	if end > len(tm.filtered) {
		end = len(tm.filtered)
	}

	// Group header tracking for upcoming view
	lastGroup := ""

	for i := tm.scroll; i < end; i++ {
		task := tm.filtered[i]

		// Upcoming view: group by day
		if tm.view == taskViewUpcoming && task.DueDate != "" {
			group := task.DueDate
			if group != lastGroup {
				if lastGroup != "" {
					b.WriteString("\n")
				}
				dt, _ := time.Parse("2006-01-02", group)
				dayLabel := dt.Format("Monday, Jan 2")
				if tmIsToday(group) {
					dayLabel += " (today)"
				}
				b.WriteString("  " + lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(dayLabel))
				b.WriteString("\n")
				lastGroup = group
			}
		}

		tm.renderTaskRow(b, i, task, w)
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")

	// Scroll indicator
	if len(tm.filtered) > visH {
		pos := ""
		if tm.scroll > 0 {
			pos += "\u25B2 " // ▲
		}
		pos += fmt.Sprintf("%d/%d", tm.cursor+1, len(tm.filtered))
		if end < len(tm.filtered) {
			pos += " \u25BC" // ▼
		}
		b.WriteString(DimStyle.Render("  " + pos))
		b.WriteString("\n")
	}
}

func (tm *TaskManager) renderTaskRow(b *strings.Builder, idx int, task Task, w int) {
	isSelected := idx == tm.cursor

	// Checkbox
	var checkbox string
	if task.Done {
		checkbox = lipgloss.NewStyle().Foreground(green).Render("[x]")
	} else {
		checkbox = lipgloss.NewStyle().Foreground(overlay0).Render("[ ]")
	}

	// Priority indicator
	prioIcon := tmPriorityIcon(task.Priority)
	prioStyled := lipgloss.NewStyle().Foreground(tmPriorityColor(task.Priority)).Render(prioIcon)

	// Task text (strip emoji markers for cleaner display)
	displayText := task.Text
	displayText = tmDueDateRe.ReplaceAllString(displayText, "")
	displayText = tmPrioHighestRe.ReplaceAllString(displayText, "")
	displayText = tmPrioHighRe.ReplaceAllString(displayText, "")
	displayText = tmPrioMedRe.ReplaceAllString(displayText, "")
	displayText = tmPrioLowRe.ReplaceAllString(displayText, "")
	displayText = tmScheduleRe.ReplaceAllString(displayText, "")
	displayText = strings.TrimSpace(displayText)

	textStyle := lipgloss.NewStyle().Foreground(text)
	if task.Done {
		textStyle = lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true)
	}

	// Due date badge
	var dueBadge string
	if task.DueDate != "" {
		dueLabel := tmFormatDue(task.DueDate)
		if tmIsOverdue(task.DueDate) {
			dueBadge = lipgloss.NewStyle().Foreground(crust).Background(red).Padding(0, 1).Render(dueLabel)
		} else if tmIsToday(task.DueDate) {
			dueBadge = lipgloss.NewStyle().Foreground(crust).Background(yellow).Padding(0, 1).Render(dueLabel)
		} else {
			dueBadge = lipgloss.NewStyle().Foreground(crust).Background(surface1).Padding(0, 1).Render(dueLabel)
		}
	}

	// Scheduled time badge (from AI scheduler)
	var scheduleBadge string
	if task.ScheduledTime != "" {
		scheduleBadge = lipgloss.NewStyle().
			Foreground(crust).
			Background(teal).
			Padding(0, 1).
			Render(task.ScheduledTime)
	}

	// Source note badge
	noteName := strings.TrimSuffix(filepath.Base(task.NotePath), ".md")
	noteLabel := lipgloss.NewStyle().
		Foreground(text).
		Background(surface1).
		Padding(0, 1).
		Render(noteName)

	// Truncate text if necessary
	maxTextW := w - 30
	if maxTextW < 10 {
		maxTextW = 10
	}
	displayText = TruncateDisplay(displayText, maxTextW)

	// Build the line
	prefix := "  "
	if isSelected {
		prefix = lipgloss.NewStyle().Foreground(mauve).Render("  " + ThemeAccentBar + " ")
	}

	line := prefix + checkbox + " " + prioStyled + " " + textStyle.Render(displayText)
	if scheduleBadge != "" {
		line += " " + scheduleBadge
	}
	if dueBadge != "" {
		line += " " + dueBadge
	}
	line += " " + noteLabel

	if isSelected {
		b.WriteString(lipgloss.NewStyle().
			Background(surface0).
			Width(w).
			Render(line))
	} else {
		b.WriteString(line)
	}
}

func (tm *TaskManager) renderInput(b *strings.Builder, w int) {
	if tm.inputMode == tmInputNone {
		return
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", w-4)))
	b.WriteString("\n")

	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text).Background(surface0).Padding(0, 1)

	switch tm.inputMode {
	case tmInputAdd:
		b.WriteString("  " + promptStyle.Render("New task: ") + inputStyle.Render(tm.inputBuf+"\u2588"))
	case tmInputDate:
		// Date picker display
		preview := tmFormatDateLong(tm.datePickerDate)
		dateStr := tm.datePickerDate.Format("2006-01-02")
		b.WriteString("  " + promptStyle.Render("Due: ") +
			lipgloss.NewStyle().Foreground(text).Bold(true).Render(preview))
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(overlay0).Render(dateStr))
		b.WriteString("\n")
		shortcutStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		descStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString("  " +
			shortcutStyle.Render("t") + descStyle.Render(":today ") +
			shortcutStyle.Render("m") + descStyle.Render(":tomorrow ") +
			shortcutStyle.Render("w") + descStyle.Render(":monday ") +
			shortcutStyle.Render("+/-") + descStyle.Render(":adjust ") +
			shortcutStyle.Render("Enter") + descStyle.Render(":set ") +
			shortcutStyle.Render("Esc") + descStyle.Render(":cancel"))
	case tmInputSearch:
		b.WriteString("  " + promptStyle.Render("Filter: ") + inputStyle.Render(tm.inputBuf+"\u2588"))
	}
	b.WriteString("\n")
}

func (tm *TaskManager) renderHelp(b *strings.Builder, w int) {
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", w-4)))
	b.WriteString("\n")

	var pairs []struct{ Key, Desc string }
	switch tm.view {
	case taskViewCalendar:
		pairs = []struct{ Key, Desc string }{
			{"h/l", "month"}, {"j/k", "day"}, {"x", "toggle"},
			{"g", "go"}, {"a", "add"}, {"Tab", "view"}, {"Esc", "close"},
		}
	case taskViewKanban:
		pairs = []struct{ Key, Desc string }{
			{"h/l", "col"}, {"j/k", "nav"}, {"x", "toggle"}, {">/<", "move"},
			{"g", "go"}, {"a", "add"}, {"Tab", "view"}, {"Esc", "close"},
		}
	default:
		pairs = []struct{ Key, Desc string }{
			{"j/k", "nav"}, {"x", "toggle"}, {"g", "go"}, {"a", "add"},
			{"d", "date"}, {"p", "prio"}, {"/", "search"}, {"Tab", "view"}, {"Esc", "close"},
		}
	}

	b.WriteString(RenderHelpBar(pairs))
}

// ---------------------------------------------------------------------------
// Calendar sub-view
// ---------------------------------------------------------------------------

func (tm *TaskManager) renderCalendarView(b *strings.Builder, w int) {
	// Month header
	monthName := time.Month(tm.calMonth).String()
	monthYear := fmt.Sprintf("%s %d", monthName, tm.calYear)
	navLeft := lipgloss.NewStyle().Foreground(overlay1).Render("\u25C0 ")
	navRight := lipgloss.NewStyle().Foreground(overlay1).Render(" \u25B6")
	header := navLeft + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(monthYear) + navRight
	headerPad := (28 - lipgloss.Width(header)) / 2
	if headerPad < 0 {
		headerPad = 0
	}
	b.WriteString("  " + strings.Repeat(" ", headerPad) + header)
	b.WriteString("\n\n")

	// Day-of-week header
	dayNames := []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	dayHeaderStyle := lipgloss.NewStyle().Foreground(subtext0).Bold(true)
	b.WriteString("  ")
	for _, d := range dayNames {
		b.WriteString(dayHeaderStyle.Render(fmt.Sprintf("%4s", d)))
	}
	b.WriteString("\n")

	// Calendar grid
	firstOfMonth := time.Date(tm.calYear, tm.calMonth, 1, 0, 0, 0, 0, time.Local)
	startWeekday := int(firstOfMonth.Weekday())
	totalDays := tmDaysInMonth(tm.calYear, tm.calMonth)
	todayStr := tmToday()

	row := "  "
	col := 0

	// Leading blanks
	for i := 0; i < startWeekday; i++ {
		row += "    "
		col++
	}

	for d := 1; d <= totalDays; d++ {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", tm.calYear, int(tm.calMonth), d)

		// Count tasks for this day
		count := 0
		for _, t := range tm.allTasks {
			if t.DueDate == dateStr {
				count++
			}
		}

		dayStr := fmt.Sprintf("%2d", d)
		isCursor := d == tm.calDay
		isToday := dateStr == todayStr

		var cell string
		switch {
		case isCursor && isToday:
			cell = lipgloss.NewStyle().Foreground(crust).Background(peach).Bold(true).Render(dayStr)
		case isCursor:
			cell = lipgloss.NewStyle().Foreground(crust).Background(mauve).Bold(true).Render(dayStr)
		case isToday:
			cell = lipgloss.NewStyle().Foreground(peach).Bold(true).Render(dayStr)
		case count > 0:
			cell = lipgloss.NewStyle().Foreground(blue).Render(dayStr)
		default:
			cell = lipgloss.NewStyle().Foreground(text).Render(dayStr)
		}

		// Task count badge
		badge := " "
		if count > 0 {
			badge = lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("%d", count))
		}

		row += " " + cell + badge
		col++

		if col == 7 {
			b.WriteString(row + "\n")
			row = "  "
			col = 0
		}
	}

	if col > 0 {
		b.WriteString(row + "\n")
	}
	b.WriteString("\n")

	// Selected day's tasks
	dateLabel := fmt.Sprintf("%04d-%02d-%02d", tm.calYear, int(tm.calMonth), tm.calDay)
	dt, _ := time.Parse("2006-01-02", dateLabel)
	dayTitle := dt.Format("Monday, Jan 2")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(dayTitle))
	b.WriteString("\n")
	calW := w - 4
	if calW < 10 {
		calW = 10
	}
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", calW)))
	b.WriteString("\n")

	if len(tm.filtered) == 0 {
		b.WriteString(DimStyle.Render("  No tasks for this day"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Press 'a' to add a task"))
		b.WriteString("\n")
	} else {
		visH := tm.visibleHeight() - 12 // reserve space for calendar grid
		if visH < 3 {
			visH = 3
		}
		end := tm.scroll + visH
		if end > len(tm.filtered) {
			end = len(tm.filtered)
		}
		for i := tm.scroll; i < end; i++ {
			tm.renderTaskRow(b, i, tm.filtered[i], w)
			if i < end-1 {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
}

// ---------------------------------------------------------------------------
// Kanban view
// ---------------------------------------------------------------------------

func (tm *TaskManager) renderKanbanView(b *strings.Builder, w int) {
	cols := tm.kanbanColumns()
	colNames := []string{"Backlog", "Todo", "In Progress", "Done"}
	colColors := []lipgloss.Color{overlay1, blue, peach, green}

	colWidth := w / 4
	if colWidth < 15 {
		colWidth = 15
	}

	visH := tm.kanbanVisibleHeight()

	// Render each column
	var colViews []string
	for c := 0; c < 4; c++ {
		var cb strings.Builder
		isActiveCol := c == tm.kanbanCol

		// Column header
		headerStyle := lipgloss.NewStyle().Foreground(colColors[c]).Bold(true)
		countStr := fmt.Sprintf(" %d", len(cols[c]))
		countStyle := lipgloss.NewStyle().Foreground(overlay0)
		header := headerStyle.Render(colNames[c]) + countStyle.Render(countStr)
		cb.WriteString(header)
		cb.WriteString("\n")

		// Separator
		sepColor := overlay0
		if isActiveCol {
			sepColor = colColors[c]
		}
		cb.WriteString(lipgloss.NewStyle().Foreground(sepColor).Render(strings.Repeat("\u2500", colWidth-2)))
		cb.WriteString("\n")

		// Tasks
		if len(cols[c]) == 0 {
			cb.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("(empty)"))
			cb.WriteString("\n")
		} else {
			start := tm.kanbanScroll[c]
			end := start + visH
			if end > len(cols[c]) {
				end = len(cols[c])
			}
			for i := start; i < end; i++ {
				task := cols[c][i]
				isSelected := isActiveCol && i == tm.kanbanCursor[c]
				tm.renderKanbanCard(&cb, task, isSelected, colWidth-2)
				cb.WriteString("\n")
			}
			// Scroll indicator
			if len(cols[c]) > visH {
				pos := fmt.Sprintf("%d/%d", tm.kanbanCursor[c]+1, len(cols[c]))
				cb.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(pos))
				cb.WriteString("\n")
			}
		}

		colView := lipgloss.NewStyle().
			Width(colWidth).
			Render(cb.String())
		colViews = append(colViews, colView)
	}

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, colViews...))
	b.WriteString("\n")
}

func (tm *TaskManager) renderKanbanCard(b *strings.Builder, task Task, isSelected bool, w int) {
	// Checkbox
	var checkbox string
	if task.Done {
		checkbox = lipgloss.NewStyle().Foreground(green).Render("[x]")
	} else {
		checkbox = lipgloss.NewStyle().Foreground(overlay0).Render("[ ]")
	}

	// Priority
	prioIcon := tmPriorityIcon(task.Priority)
	prioStyled := lipgloss.NewStyle().Foreground(tmPriorityColor(task.Priority)).Render(prioIcon)

	// Task text
	displayText := task.Text
	displayText = tmDueDateRe.ReplaceAllString(displayText, "")
	displayText = tmPrioHighestRe.ReplaceAllString(displayText, "")
	displayText = tmPrioHighRe.ReplaceAllString(displayText, "")
	displayText = tmPrioMedRe.ReplaceAllString(displayText, "")
	displayText = tmPrioLowRe.ReplaceAllString(displayText, "")
	displayText = strings.TrimSpace(displayText)

	maxTextW := w - 8
	if maxTextW < 5 {
		maxTextW = 5
	}
	runes := []rune(displayText)
	if len(runes) > maxTextW {
		displayText = string(runes[:maxTextW-1]) + "\u2026"
	}

	textStyle := lipgloss.NewStyle().Foreground(text)
	if task.Done {
		textStyle = lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true)
	}

	line := checkbox + " " + prioStyled + " " + textStyle.Render(displayText)

	// Due date badge (compact)
	if task.DueDate != "" {
		dueLabel := tmFormatDue(task.DueDate)
		var dueBadge string
		if tmIsOverdue(task.DueDate) {
			dueBadge = lipgloss.NewStyle().Foreground(red).Render(dueLabel)
		} else if tmIsToday(task.DueDate) {
			dueBadge = lipgloss.NewStyle().Foreground(yellow).Render(dueLabel)
		} else {
			dueBadge = lipgloss.NewStyle().Foreground(overlay0).Render(dueLabel)
		}
		line += " " + dueBadge
	}

	if isSelected {
		prefix := lipgloss.NewStyle().Foreground(mauve).Render(ThemeAccentBar + " ")
		b.WriteString(lipgloss.NewStyle().
			Background(surface0).
			Width(w).
			Render(prefix + line))
	} else {
		b.WriteString("  " + line)
	}
}
