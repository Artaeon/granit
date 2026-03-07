package tui

import (
	"fmt"
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
	Text     string
	Done     bool
	DueDate  string // "2006-01-02" or ""
	Priority int    // 0=none, 1=low, 2=medium, 3=high
	Tags     []string
	NotePath string // source note relative path
	LineNum  int    // 1-based line number in source note
}

// taskView identifies which tab is active.
type taskView int

const (
	taskViewToday     taskView = iota // 0
	taskViewUpcoming                  // 1
	taskViewAll                       // 2
	taskViewCompleted                 // 3
	taskViewCalendar                  // 4
)

// inputMode tracks text-input sub-modes.
type tmInputMode int

const (
	tmInputNone   tmInputMode = iota
	tmInputAdd                // adding a new task
	tmInputDate               // editing due date
	tmInputSearch             // filtering
)

// ---------------------------------------------------------------------------
// Regex patterns
// ---------------------------------------------------------------------------

var (
	tmTaskRe    = regexp.MustCompile(`^(\s*- \[)([ xX])(\] .+)`)
	tmDueDateRe = regexp.MustCompile(`\x{1F4C5}\s*(\d{4}-\d{2}-\d{2})`)
	tmPrioHighRe   = regexp.MustCompile(`\x{23EB}`)  // ⏫
	tmPrioMedRe    = regexp.MustCompile(`\x{1F53C}`)  // 🔼
	tmPrioLowRe    = regexp.MustCompile(`\x{1F53D}`)  // 🔽
	tmTagRe     = regexp.MustCompile(`#([A-Za-z0-9_/-]+)`)
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
	allTasks  []Task
	filtered  []Task // currently displayed tasks
	vaultRoot string

	// Navigation
	view   taskView
	cursor int
	scroll int

	// Input
	inputMode  tmInputMode
	inputBuf   string
	inputNote  string // note path for new task target

	// Calendar sub-view state
	calYear  int
	calMonth time.Month
	calDay   int // selected day (1-based), 0 = none

	// Consumed-once results
	togglePath string
	toggleLine int
	toggleDone bool
	toggleOK   bool

	jumpPath string
	jumpLine int
	jumpOK   bool

	newTaskPath string
	newTaskText string
	newTaskOK   bool

	dateUpdatePath string
	dateUpdateLine int
	dateUpdateDate string
	dateUpdateOK   bool

	prioUpdatePath string
	prioUpdateLine int
	prioUpdatePrio int
	prioUpdateOK   bool

	activeNotePath string // fallback note for adding tasks
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
func (tm *TaskManager) Open(vaultRoot string, notes map[string]*vault.Note) {
	tm.active = true
	tm.vaultRoot = vaultRoot
	tm.allTasks = ParseAllTasks(notes)
	tm.view = taskViewToday
	tm.cursor = 0
	tm.scroll = 0
	tm.inputMode = tmInputNone
	tm.inputBuf = ""
	tm.inputNote = ""
	tm.clearResults()
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

// GetToggleResult returns a consumed-once toggle result.
func (tm *TaskManager) GetToggleResult() (notePath string, lineNum int, newDone bool, ok bool) {
	if !tm.toggleOK {
		return "", 0, false, false
	}
	p, l, d := tm.togglePath, tm.toggleLine, tm.toggleDone
	tm.toggleOK = false
	return p, l, d, true
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

// GetNewTask returns a consumed-once new task result.
func (tm *TaskManager) GetNewTask() (notePath string, taskText string, ok bool) {
	if !tm.newTaskOK {
		return "", "", false
	}
	p, t := tm.newTaskPath, tm.newTaskText
	tm.newTaskOK = false
	return p, t, true
}

func (tm *TaskManager) clearResults() {
	tm.toggleOK = false
	tm.jumpOK = false
	tm.newTaskOK = false
	tm.dateUpdateOK = false
	tm.prioUpdateOK = false
}

// GetDateUpdateResult returns a consumed-once date change result.
func (tm *TaskManager) GetDateUpdateResult() (notePath string, lineNum int, newDate string, ok bool) {
	if !tm.dateUpdateOK {
		return "", 0, "", false
	}
	p, l, d := tm.dateUpdatePath, tm.dateUpdateLine, tm.dateUpdateDate
	tm.dateUpdateOK = false
	return p, l, d, true
}

// GetPrioUpdateResult returns a consumed-once priority change result.
func (tm *TaskManager) GetPrioUpdateResult() (notePath string, lineNum int, newPrio int, ok bool) {
	if !tm.prioUpdateOK {
		return "", 0, 0, false
	}
	p, l, pr := tm.prioUpdatePath, tm.prioUpdateLine, tm.prioUpdatePrio
	tm.prioUpdateOK = false
	return p, l, pr, true
}

// SetActiveNote sets the fallback note for adding tasks when no task is selected.
func (tm *TaskManager) SetActiveNote(path string) {
	tm.activeNotePath = path
}

// Refresh re-parses all tasks from the vault without closing the overlay.
func (tm *TaskManager) Refresh(notes map[string]*vault.Note) {
	savedView := tm.view
	savedScroll := tm.scroll
	savedCursor := tm.cursor
	tm.allTasks = ParseAllTasks(notes)
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

			// Priority
			if tmPrioHighRe.MatchString(taskText) {
				t.Priority = 3
			} else if tmPrioMedRe.MatchString(taskText) {
				t.Priority = 2
			} else if tmPrioLowRe.MatchString(taskText) {
				t.Priority = 1
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

func tmIsThisWeek(dateStr string) bool {
	if dateStr == "" {
		return false
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return false
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	weekEnd := today.AddDate(0, 0, 7)
	return !t.Before(today) && t.Before(weekEnd)
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
		// Tasks with due dates first
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
	// Sort by note path then line number (stable ordering)
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
// Priority helpers
// ---------------------------------------------------------------------------

func tmPriorityIcon(p int) string {
	switch p {
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
		// Handle input modes first
		if tm.inputMode != tmInputNone {
			return tm.updateInput(msg)
		}
		return tm.updateNormal(msg)
	}
	return tm, nil
}

func (tm TaskManager) updateInput(msg tea.KeyMsg) (TaskManager, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		if tm.inputMode == tmInputSearch {
			tm.inputMode = tmInputNone
			tm.inputBuf = ""
			tm.rebuildFiltered()
		} else {
			tm.inputMode = tmInputNone
			tm.inputBuf = ""
		}
		return tm, nil

	case "enter":
		switch tm.inputMode {
		case tmInputAdd:
			if strings.TrimSpace(tm.inputBuf) != "" {
				taskLine := "- [ ] " + strings.TrimSpace(tm.inputBuf)
				// If in calendar view, append the selected date
				if tm.view == taskViewCalendar && tm.calDay > 0 {
					dateStr := fmt.Sprintf("%04d-%02d-%02d", tm.calYear, int(tm.calMonth), tm.calDay)
					if !strings.Contains(taskLine, "\U0001F4C5") {
						taskLine += " \U0001F4C5 " + dateStr
					}
				}
				// Determine target note
				target := tm.inputNote
				if target == "" && len(tm.filtered) > 0 && tm.cursor < len(tm.filtered) {
					target = tm.filtered[tm.cursor].NotePath
				}
				if target == "" {
					target = tm.activeNotePath
				}
				if target != "" {
					tm.newTaskPath = target
					tm.newTaskText = taskLine
					tm.newTaskOK = true
				}
			}
			tm.inputMode = tmInputNone
			tm.inputBuf = ""
			return tm, nil

		case tmInputDate:
			if tm.cursor < len(tm.filtered) {
				dateStr := strings.TrimSpace(tm.inputBuf)
				if _, err := time.Parse("2006-01-02", dateStr); err == nil {
					task := tm.filtered[tm.cursor]
					tm.dateUpdatePath = task.NotePath
					tm.dateUpdateLine = task.LineNum
					tm.dateUpdateDate = dateStr
					tm.dateUpdateOK = true
					// Update local state
					for i := range tm.allTasks {
						if tm.allTasks[i].NotePath == task.NotePath && tm.allTasks[i].LineNum == task.LineNum {
							tm.allTasks[i].DueDate = dateStr
							break
						}
					}
					tm.rebuildFiltered()
				}
			}
			tm.inputMode = tmInputNone
			tm.inputBuf = ""
			return tm, nil

		case tmInputSearch:
			// Keep search active, just confirm
			return tm, nil
		}

	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = tm.inputBuf[:len(tm.inputBuf)-1]
			if tm.inputMode == tmInputSearch {
				tm.rebuildFiltered()
			}
		}
		return tm, nil

	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
			if tm.inputMode == tmInputSearch {
				tm.rebuildFiltered()
			}
		}
		return tm, nil
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

	case "tab":
		tm.view = (tm.view + 1) % 5
		tm.rebuildFiltered()

	// Navigation
	case "up", "k":
		if tm.view == taskViewCalendar && len(tm.filtered) == 0 {
			// Navigate calendar days
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
			tm.togglePath = task.NotePath
			tm.toggleLine = task.LineNum
			tm.toggleDone = !task.Done
			tm.toggleOK = true
			// Update local state so the view refreshes immediately
			for i := range tm.allTasks {
				if tm.allTasks[i].NotePath == task.NotePath && tm.allTasks[i].LineNum == task.LineNum {
					tm.allTasks[i].Done = !task.Done
					break
				}
			}
			tm.rebuildFiltered()
		} else if tm.view == taskViewCalendar && len(tm.filtered) == 0 {
			// In calendar with no tasks shown, select the day to view tasks
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
		if tm.cursor < len(tm.filtered) {
			tm.inputNote = tm.filtered[tm.cursor].NotePath
		} else if tm.activeNotePath != "" {
			tm.inputNote = tm.activeNotePath
		}

	// Set due date
	case "d":
		if tm.cursor < len(tm.filtered) {
			tm.inputMode = tmInputDate
			task := tm.filtered[tm.cursor]
			if task.DueDate != "" {
				tm.inputBuf = task.DueDate
			} else {
				tm.inputBuf = tmToday()
			}
		}

	// Cycle priority
	case "p":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			newPrio := (task.Priority + 1) % 4
			tm.prioUpdatePath = task.NotePath
			tm.prioUpdateLine = task.LineNum
			tm.prioUpdatePrio = newPrio
			tm.prioUpdateOK = true
			for i := range tm.allTasks {
				if tm.allTasks[i].NotePath == task.NotePath && tm.allTasks[i].LineNum == task.LineNum {
					tm.allTasks[i].Priority = newPrio
					break
				}
			}
			tm.rebuildFiltered()
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

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the task manager overlay.
func (tm TaskManager) View() string {
	width := tm.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
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
	default:
		tm.renderTaskList(&b, innerW)
	}

	// Input bar (if active)
	tm.renderInput(&b, innerW)

	// Help bar
	tm.renderHelp(&b, innerW)

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
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
	}
	counts := []int{
		len(tm.filterToday()),
		len(tm.filterUpcoming()),
		len(tm.filterAll()),
		len(tm.filterCompleted()),
		-1, // calendar doesn't show count
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
		switch tm.view {
		case taskViewToday:
			emptyMsg = "No tasks due today"
		case taskViewUpcoming:
			emptyMsg = "No upcoming tasks this week"
		case taskViewCompleted:
			emptyMsg = "No completed tasks"
		}
		b.WriteString(DimStyle.Render("  " + emptyMsg))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Press 'a' to add a task"))
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
	displayText = tmPrioHighRe.ReplaceAllString(displayText, "")
	displayText = tmPrioMedRe.ReplaceAllString(displayText, "")
	displayText = tmPrioLowRe.ReplaceAllString(displayText, "")
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

	// Source note name
	noteName := strings.TrimSuffix(filepath.Base(task.NotePath), ".md")
	noteLabel := lipgloss.NewStyle().Foreground(overlay0).Render(noteName)

	// Truncate text if necessary
	maxTextW := w - 20
	if maxTextW < 10 {
		maxTextW = 10
	}
	runes := []rune(displayText)
	if len(runes) > maxTextW {
		displayText = string(runes[:maxTextW-1]) + "\u2026"
	}

	// Build the line
	prefix := "  "
	if isSelected {
		prefix = lipgloss.NewStyle().Foreground(mauve).Render("  " + ThemeAccentBar + " ")
	}

	line := prefix + checkbox + " " + prioStyled + " " + textStyle.Render(displayText)
	if dueBadge != "" {
		line += " " + dueBadge
	}
	line += " " + noteLabel

	if isSelected {
		// Highlight background for selected row
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
		b.WriteString("  " + promptStyle.Render("Due date (YYYY-MM-DD): ") + inputStyle.Render(tm.inputBuf+"\u2588"))
	case tmInputSearch:
		b.WriteString("  " + promptStyle.Render("Filter: ") + inputStyle.Render(tm.inputBuf+"\u2588"))
	}
	b.WriteString("\n")
}

func (tm *TaskManager) renderHelp(b *strings.Builder, w int) {
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", w-4)))
	b.WriteString("\n")

	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(overlay0)

	var pairs []string
	if tm.view == taskViewCalendar {
		pairs = []string{
			keyStyle.Render("h/l") + descStyle.Render(":month "),
			keyStyle.Render("j/k") + descStyle.Render(":day "),
			keyStyle.Render("x") + descStyle.Render(":toggle "),
			keyStyle.Render("g") + descStyle.Render(":go "),
			keyStyle.Render("a") + descStyle.Render(":add "),
			keyStyle.Render("Esc") + descStyle.Render(":close"),
		}
	} else {
		pairs = []string{
			keyStyle.Render("j/k") + descStyle.Render(":nav "),
			keyStyle.Render("x") + descStyle.Render(":toggle "),
			keyStyle.Render("g") + descStyle.Render(":go "),
			keyStyle.Render("a") + descStyle.Render(":add "),
			keyStyle.Render("d") + descStyle.Render(":date "),
			keyStyle.Render("p") + descStyle.Render(":prio "),
			keyStyle.Render("/") + descStyle.Render(":search "),
			keyStyle.Render("Tab") + descStyle.Render(":view "),
			keyStyle.Render("Esc") + descStyle.Render(":close"),
		}
	}

	b.WriteString("  " + strings.Join(pairs, " "))
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
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", maxInt(w-4, 10))))
	b.WriteString("\n")

	if len(tm.filtered) == 0 {
		b.WriteString(DimStyle.Render("  No tasks for this day"))
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
