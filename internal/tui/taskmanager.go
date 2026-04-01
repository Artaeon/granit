package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/vault"
)

// ---------------------------------------------------------------------------
// Task data
// ---------------------------------------------------------------------------

// Task represents a single task extracted from a markdown note.
type Task struct {
	Text             string   `json:"text"`
	Done             bool     `json:"done"`
	DueDate          string   `json:"due_date"`                 // "2006-01-02" or ""
	Priority         int      `json:"priority"`                 // 0=none, 1=low, 2=medium, 3=high, 4=highest
	ScheduledTime    string   `json:"scheduled_time,omitempty"` // "HH:MM-HH:MM" or "" — set by AI scheduler
	Tags             []string `json:"tags,omitempty"`
	NotePath         string   `json:"note_path"`                   // source note relative path
	LineNum          int      `json:"line_num"`                    // 1-based line number in source note
	Indent           int      `json:"indent,omitempty"`            // 0=top-level, 1=subtask, 2=sub-subtask
	ParentLine       int      `json:"parent_line,omitempty"`       // LineNum of parent task, 0 if top-level
	DependsOn        []string `json:"depends_on,omitempty"`        // dependency references (task text snippets)
	EstimatedMinutes int      `json:"estimated_minutes,omitempty"` // time estimate in minutes
	Recurrence       string   `json:"recurrence,omitempty"`        // "daily", "weekly", "monthly", etc.
	Project          string   `json:"project,omitempty"`           // matched project name
	SnoozedUntil     string   `json:"snoozed_until,omitempty"`     // ISO "2006-01-02T15:04"
	GoalID           string   `json:"goal_id,omitempty"`           // linked goal e.g. "G001"
	ActualMinutes    int      `json:"-"`                           // computed from time tracker, not serialized
}

// taskView identifies which tab is active.
type taskView int

const (
	taskViewToday      taskView = iota // 0
	taskViewUpcoming                   // 1
	taskViewAll                        // 2
	taskViewCompleted                  // 3
	taskViewCalendar                   // 4
	taskViewKanban                     // 5
	taskViewEisenhower                 // 6
)

// tmSortMode controls how tasks are sorted within a view.
type tmSortMode int

const (
	tmSortPriority tmSortMode = iota // default: priority desc, then date
	tmSortDueDate                    // due date asc, empty last
	tmSortAlpha                      // alphabetical by cleaned text
	tmSortSource                     // by source note path, then line
	tmSortTag                        // by first tag alphabetically
)

var tmSortNames = []string{"priority", "due date", "A-Z", "source", "tag"}

// inputMode tracks sub-modes.
type tmInputMode int

const (
	tmInputNone            tmInputMode = iota
	tmInputAdd                         // adding a new task
	tmInputDate                        // date picker
	tmInputSearch                      // filtering
	tmInputReschedule                  // quick reschedule picker
	tmInputDependency                  // adding a dependency
	tmInputEstimate                    // setting time estimate
	tmInputNote                        // editing task note
	tmInputSnooze                      // snooze picker
	tmInputBatchReschedule             // batch reschedule overdue tasks
	tmInputHelp                        // help overlay
	tmInputTemplate                    // template picker
	tmInputTemplateName                // naming a new template
)

// ---------------------------------------------------------------------------
// Regex patterns
// ---------------------------------------------------------------------------

var (
	tmTaskRe        = regexp.MustCompile(`^(\s*- \[)([ xX])(\] .+)`)
	tmDueDateRe     = regexp.MustCompile(`\x{1F4C5}\s*(\d{4}-\d{2}-\d{2})`)
	tmPrioHighestRe = regexp.MustCompile(`\x{1F53A}`) // 🔺
	tmPrioHighRe    = regexp.MustCompile(`\x{23EB}`)  // ⏫
	tmPrioMedRe     = regexp.MustCompile(`\x{1F53C}`) // 🔼
	tmPrioLowRe     = regexp.MustCompile(`\x{1F53D}`) // 🔽
	tmTagRe         = regexp.MustCompile(`#([A-Za-z0-9_/-]+)`)
	tmScheduleRe    = regexp.MustCompile(`⏰\s*(\d{2}:\d{2}-\d{2}:\d{2})`)
	tmDependsRe     = regexp.MustCompile(`depends:"([^"]+)"|depends:([^\s]+)`)
	tmEstimateRe    = regexp.MustCompile(`~(\d+)(m|h)`)
	tmRecurEmojiRe  = regexp.MustCompile(`\x{1F501}\s*(daily|weekly|monthly|3x-week)`)
	tmRecurTagRe    = regexp.MustCompile(`#(daily|weekly|monthly|3x-week)\b`)
	tmSnoozeRe      = regexp.MustCompile(`snooze:(\d{4}-\d{2}-\d{2}T\d{2}:\d{2})`)
	tmGoalIDRe      = regexp.MustCompile(`goal:(G\d{3,})`)
)

// taskTemplate stores a reusable task definition.
type taskTemplate struct {
	Name string `json:"name"`
	Text string `json:"text"` // full task text with markers (tags, priority, estimate)
}

// taskAction stores a single task line change for undo.
type taskAction struct {
	NotePath string
	LineNum  int
	OldLine  string
}

// ---------------------------------------------------------------------------
// TaskManager overlay
// ---------------------------------------------------------------------------

// TaskManager is the comprehensive task management overlay.
type TaskManager struct {
	active bool
	width  int
	height int

	// Data
	vault    *vault.Vault
	config   config.Config
	allTasks []Task
	filtered []Task // currently displayed tasks

	// Navigation
	view   taskView
	cursor int
	scroll int

	// Input
	inputMode tmInputMode
	inputBuf  string

	// Sort mode
	sortMode tmSortMode

	// Date picker state
	datePickerDate time.Time // currently selected date in picker

	// Calendar sub-view state
	calYear  int
	calMonth time.Month
	calDay   int // selected day (1-based), 0 = none

	// Kanban state
	kanbanCol    int    // 0-3: which column cursor is in
	kanbanCursor [4]int // cursor position within each column
	kanbanScroll [4]int // scroll position within each column

	// Active filters (applied on top of view-specific filters)
	filterTag      string // "" = no tag filter, "tagname" = filter to this tag
	filterPriority int    // -1 = no priority filter, 0-4 = filter to this level

	// Cached tab counts (updated by rebuildFiltered)
	tabCounts [7]int

	// Cached blocked status (computed once in rebuildFiltered, not per-render)
	blockedCache map[string]bool

	// Confirmation prompt for destructive actions
	confirmMsg    string
	confirmAction func()

	// Snapshot of task for input modes (prevents cursor drift bugs)
	inputTask *Task

	// Status message
	statusMsg string

	// Subtask collapse state (key: "notePath:lineNum" of parent)
	collapsed map[string]bool

	// Bulk selection mode
	selectMode bool
	selected   map[string]bool // key: "notePath:lineNum"

	// Batch reschedule state
	batchReschedule []Task
	batchReschIdx   int

	// Pinned tasks (persisted to .granit/pinned-tasks.json)
	pinnedTasks map[string]bool

	// Task templates (persisted to .granit/task-templates.json)
	taskTemplates []taskTemplate

	// Task notes (persisted to .granit/task-notes.json)
	taskNotes map[string]string

	// Undo stack (up to 10 actions)
	undoStack []taskAction

	// Consumed-once: jump result
	jumpPath string
	jumpLine int
	jumpOK   bool

	// Consumed-once: focus session launch
	focusTask   string
	hasFocusReq bool

	// File change tracking
	fileChanged     bool
	lastChangedNote string // path of most recently modified note
}

// NewTaskManager creates a new TaskManager overlay.
func NewTaskManager() TaskManager {
	now := time.Now()
	return TaskManager{
		calYear:        now.Year(),
		calMonth:       now.Month(),
		calDay:         now.Day(),
		filterPriority: -1,
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
	tm.allTasks = FilterTasks(ParseAllTasks(v.Notes), tm.config)
	tm.view = taskViewToday
	tm.cursor = 0
	tm.scroll = 0
	tm.inputMode = tmInputNone
	tm.inputBuf = ""
	tm.statusMsg = ""
	tm.filterTag = ""
	tm.filterPriority = -1
	tm.collapsed = make(map[string]bool)
	tm.selectMode = false
	tm.selected = make(map[string]bool)
	tm.undoStack = nil
	tm.loadPinnedTasks()
	tm.loadTaskNotes()
	tm.loadTaskTemplates()
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
	tm.allTasks = FilterTasks(ParseAllTasks(v.Notes), tm.config)
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

// GetFocusRequest returns a task to start a focus session on (consumed-once).
func (tm *TaskManager) GetFocusRequest() (task string, ok bool) {
	if !tm.hasFocusReq {
		return "", false
	}
	t := tm.focusTask
	tm.hasFocusReq = false
	return t, true
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

// taskDueDate returns the due date for a task line, checking both the 📅 emoji
// and the daily note filename (e.g. 2026-03-31.md). Matches ParseAllTasks logic.
func taskDueDate(line string, notePath string) string {
	if dm := tmDueDateRe.FindStringSubmatch(line); dm != nil {
		return dm[1]
	}
	base := filepath.Base(notePath)
	base = strings.TrimSuffix(base, ".md")
	if _, err := time.Parse("2006-01-02", base); err == nil {
		return base
	}
	return ""
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
			if d := taskDueDate(line, note.RelPath); d != "" {
				if tmIsToday(d) || tmIsOverdue(d) {
					count++
				}
			}
		}
	}
	return count
}

// CountOverdueTasks counts how many unchecked tasks are past their due date.
func CountOverdueTasks(notes map[string]*vault.Note) int {
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
			if d := taskDueDate(line, note.RelPath); d != "" {
				if tmIsOverdue(d) {
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
		// Track recent tasks per indent level for parent linkage.
		// Key: indent level, value: LineNum of the most recent task at that level.
		parentStack := make(map[int]int) // indent -> LineNum

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

			// Determine indentation level from leading whitespace.
			indent := 0
			for _, ch := range line {
				if ch == ' ' {
					indent++
				} else if ch == '\t' {
					indent += 2
				} else {
					break
				}
			}
			indentLevel := indent / 2 // 2 spaces per level

			// Find parent: the most recent task at a lower indent level.
			parentLine := 0
			for lvl := indentLevel - 1; lvl >= 0; lvl-- {
				if pl, ok := parentStack[lvl]; ok {
					parentLine = pl
					break
				}
			}
			parentStack[indentLevel] = i + 1

			t := Task{
				Text:       taskText,
				Done:       done,
				NotePath:   note.RelPath,
				LineNum:    i + 1,
				Indent:     indentLevel,
				ParentLine: parentLine,
			}

			// Due date
			// Infer due date from daily note filename
			if t.DueDate == "" {
				base := filepath.Base(note.RelPath)
				base = strings.TrimSuffix(base, ".md")
				if _, err := time.Parse("2006-01-02", base); err == nil {
					t.DueDate = base
				}
			}
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

			// Dependencies (supports depends:"multi word" and depends:single)
			for _, dm := range tmDependsRe.FindAllStringSubmatch(taskText, -1) {
				dep := dm[1] // quoted form
				if dep == "" {
					dep = dm[2] // unquoted form
				}
				if dep != "" {
					t.DependsOn = append(t.DependsOn, dep)
				}
			}

			// Recurrence
			if rm := tmRecurEmojiRe.FindStringSubmatch(taskText); rm != nil {
				t.Recurrence = rm[1]
			} else if rm := tmRecurTagRe.FindStringSubmatch(taskText); rm != nil {
				t.Recurrence = rm[1]
			}

			// Snooze
			if sm := tmSnoozeRe.FindStringSubmatch(taskText); sm != nil {
				t.SnoozedUntil = sm[1]
			}

			// Goal link
			if gm := tmGoalIDRe.FindStringSubmatch(taskText); gm != nil {
				t.GoalID = gm[1]
			}

			// Time estimate (~30m, ~2h)
			if em := tmEstimateRe.FindStringSubmatch(taskText); em != nil {
				val := 0
				_, _ = fmt.Sscanf(em[1], "%d", &val)
				if em[2] == "h" {
					val *= 60
				}
				t.EstimatedMinutes = val
			}

			tasks = append(tasks, t)
		}
	}
	return tasks
}

// FilterTasks applies config-based task filtering (exclude folders, require
// tags, hide completed).
func FilterTasks(tasks []Task, cfg config.Config) []Task {
	if cfg.TaskFilterMode == "all" && len(cfg.TaskExcludeFolders) == 0 && !cfg.TaskExcludeDone {
		return tasks
	}
	var filtered []Task
	for _, t := range tasks {
		excluded := false
		for _, folder := range cfg.TaskExcludeFolders {
			if strings.HasPrefix(t.NotePath, folder) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		if cfg.TaskFilterMode == "tagged" {
			reqTags := cfg.TaskRequiredTags
			if len(reqTags) == 0 {
				reqTags = []string{"task", "todo"}
			}
			hasTag := false
			for _, reqTag := range reqTags {
				for _, taskTag := range t.Tags {
					if strings.EqualFold(taskTag, reqTag) {
						hasTag = true
						break
					}
				}
				if hasTag {
					break
				}
			}
			if !hasTag {
				continue
			}
		}
		if cfg.TaskExcludeDone && t.Done {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}

// MatchTasksToProjects assigns project names to tasks based on folder,
// task filter, or tag matching.
func MatchTasksToProjects(tasks []Task, projects []Project) {
	for i := range tasks {
		matchTaskToProject(&tasks[i], projects)
	}
}

func matchTaskToProject(task *Task, projects []Project) {
	for _, proj := range projects {
		if proj.Folder != "" && strings.HasPrefix(task.NotePath, proj.Folder) {
			task.Project = proj.Name
			return
		}
		if proj.TaskFilter != "" {
			for _, tag := range task.Tags {
				if strings.EqualFold(tag, proj.TaskFilter) {
					task.Project = proj.Name
					return
				}
			}
		}
		for _, ptag := range proj.Tags {
			for _, ttag := range task.Tags {
				if strings.EqualFold(ttag, ptag) {
					task.Project = proj.Name
					return
				}
			}
		}
	}
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
	// Push old line onto undo stack (max 10)
	tm.undoStack = append(tm.undoStack, taskAction{
		NotePath: notePath,
		LineNum:  lineNum,
		OldLine:  lines[lineNum-1],
	})
	if len(tm.undoStack) > 10 {
		tm.undoStack = tm.undoStack[len(tm.undoStack)-10:]
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
	tm.allTasks = FilterTasks(ParseAllTasks(tm.vault.Notes), tm.config)
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
			// Auto-create next instance for recurring tasks
			if task.Recurrence != "" {
				tm.createNextRecurrence(task)
			}
		} else {
			tm.statusMsg = "Task reopened"
		}
		tm.reparse()
	}
}

// createNextRecurrence creates the next instance of a recurring task.
func (tm *TaskManager) createNextRecurrence(task Task) {
	if tm.vault == nil {
		return
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	var nextDate time.Time
	switch task.Recurrence {
	case "daily":
		nextDate = today.AddDate(0, 0, 1)
	case "weekly":
		nextDate = today.AddDate(0, 0, 7)
	case "monthly":
		nextDate = today.AddDate(0, 1, 0)
	case "3x-week":
		// Next occurrence: skip 2 days (Mon→Wed→Fri→Mon)
		nextDate = today.AddDate(0, 0, 2)
		if nextDate.Weekday() == time.Saturday {
			nextDate = nextDate.AddDate(0, 0, 2)
		} else if nextDate.Weekday() == time.Sunday {
			nextDate = nextDate.AddDate(0, 0, 1)
		}
	default:
		return
	}

	// Build new task line: unchecked version with new date
	cleanText := tmCleanText(task.Text)
	dateStr := nextDate.Format("2006-01-02")
	newLine := fmt.Sprintf("- [ ] %s \U0001F4C5 %s", cleanText, dateStr)

	// Preserve recurrence marker
	if tmRecurEmojiRe.MatchString(task.Text) {
		newLine += " \U0001F501 " + task.Recurrence
	} else {
		newLine += " #" + task.Recurrence
	}

	// Append to the same file
	note := tm.vault.GetNote(task.NotePath)
	if note == nil {
		return
	}
	note.Content += "\n" + newLine + "\n"
	absPath := filepath.Join(tm.vault.Root, task.NotePath)
	_ = os.WriteFile(absPath, []byte(note.Content), 0644)
	tm.statusMsg = fmt.Sprintf("Task completed — next: %s", dateStr)
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

// doBulkToggle toggles all selected tasks.
func (tm *TaskManager) doBulkToggle() {
	count := 0
	for _, t := range tm.allTasks {
		if tm.selected[taskKey(t)] {
			tm.writeLineChange(t.NotePath, t.LineNum, func(line string) string {
				if strings.Contains(line, "[ ]") {
					return strings.Replace(line, "[ ]", "[x]", 1)
				}
				line = strings.Replace(line, "[x]", "[ ]", 1)
				return strings.Replace(line, "[X]", "[ ]", 1)
			})
			count++
		}
	}
	if count > 0 {
		tm.statusMsg = fmt.Sprintf("Toggled %d tasks", count)
		tm.selectMode = false
		tm.selected = make(map[string]bool)
		tm.reparse()
	}
}

// doBulkSetDate sets a date on all selected tasks.
func (tm *TaskManager) doBulkSetDate(dateStr string) {
	count := 0
	for _, t := range tm.allTasks {
		if tm.selected[taskKey(t)] {
			tm.writeLineChange(t.NotePath, t.LineNum, func(line string) string {
				line = tmDueDateRe.ReplaceAllString(line, "")
				line = strings.TrimRight(line, " ")
				return line + " \U0001F4C5 " + dateStr
			})
			count++
		}
	}
	if count > 0 {
		tm.statusMsg = fmt.Sprintf("Set date on %d tasks", count)
		tm.reparse()
	}
}

// doArchive moves completed tasks older than 30 days from Tasks.md to Archive/tasks-YYYY-MM.md.
func (tm *TaskManager) doArchive() int {
	if tm.vault == nil {
		return 0
	}
	tasksPath := filepath.Join(tm.vault.Root, "Tasks.md")
	data, err := os.ReadFile(tasksPath)
	if err != nil {
		return 0
	}
	lines := strings.Split(string(data), "\n")
	var kept, archived []string
	now := time.Now()
	cutoff := now.AddDate(0, 0, -30)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isCompleted := strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]")
		shouldArchive := false

		if isCompleted {
			if dm := tmDueDateRe.FindStringSubmatch(line); dm != nil {
				if dueDate, err := time.Parse("2006-01-02", dm[1]); err == nil {
					if dueDate.Before(cutoff) {
						shouldArchive = true
					}
				}
			} else {
				// Completed with no date: archive if it has been 30+ days
				// (we can't know exactly, so archive all undated completed tasks)
				shouldArchive = true
			}
		}

		if shouldArchive {
			archived = append(archived, line)
		} else {
			kept = append(kept, line)
		}
	}

	if len(archived) == 0 {
		return 0
	}

	// Write archive file
	archiveDir := filepath.Join(tm.vault.Root, "Archive")
	_ = os.MkdirAll(archiveDir, 0755)
	archiveFile := filepath.Join(archiveDir, "tasks-"+now.Format("2006-01")+".md")
	archiveContent := ""
	if existing, err := os.ReadFile(archiveFile); err == nil {
		archiveContent = string(existing)
	}
	archiveContent += "\n" + strings.Join(archived, "\n") + "\n"
	_ = os.WriteFile(archiveFile, []byte(archiveContent), 0644)

	// Write updated Tasks.md
	_ = os.WriteFile(tasksPath, []byte(strings.Join(kept, "\n")+"\n"), 0644)

	// Re-scan vault
	if note := tm.vault.GetNote("Tasks.md"); note != nil {
		note.Content = strings.Join(kept, "\n")
	}

	return len(archived)
}

func (tm *TaskManager) loadTaskTemplates() {
	tm.taskTemplates = nil
	if tm.vault == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(tm.vault.Root, ".granit", "task-templates.json"))
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &tm.taskTemplates); err != nil {
		tm.taskTemplates = nil
	}
}

func (tm *TaskManager) saveTaskTemplates() {
	if tm.vault == nil {
		return
	}
	dir := filepath.Join(tm.vault.Root, ".granit")
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(tm.taskTemplates, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "task-templates.json"), data, 0644)
}

// loadPinnedTasks reads pinned task keys from .granit/pinned-tasks.json.
func (tm *TaskManager) loadPinnedTasks() {
	tm.pinnedTasks = make(map[string]bool)
	if tm.vault == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(tm.vault.Root, ".granit", "pinned-tasks.json"))
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &tm.pinnedTasks); err != nil {
		tm.pinnedTasks = make(map[string]bool)
	}
}

func (tm *TaskManager) savePinnedTasks() {
	if tm.vault == nil {
		return
	}
	dir := filepath.Join(tm.vault.Root, ".granit")
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(tm.pinnedTasks, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "pinned-tasks.json"), data, 0644)
}

// loadTaskNotes reads task notes from .granit/task-notes.json.
func (tm *TaskManager) loadTaskNotes() {
	tm.taskNotes = make(map[string]string)
	if tm.vault == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(tm.vault.Root, ".granit", "task-notes.json"))
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &tm.taskNotes); err != nil {
		tm.taskNotes = make(map[string]string)
	}
}

func (tm *TaskManager) saveTaskNotes() {
	if tm.vault == nil {
		return
	}
	dir := filepath.Join(tm.vault.Root, ".granit")
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(tm.taskNotes, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "task-notes.json"), data, 0644)
}

// suggestPriority uses heuristics to recommend a priority for a task.
func suggestPriority(task Task, allTasks []Task) int {
	score := 0
	if tmIsOverdue(task.DueDate) {
		score += 2
	}
	if tmIsToday(task.DueDate) {
		score++
	}
	if task.DueDate != "" && tmDaysUntil(task.DueDate) <= 2 {
		score++
	}
	// Check if this task blocks others
	cleaned := strings.ToLower(tmCleanText(task.Text))
	for _, t := range allTasks {
		for _, dep := range t.DependsOn {
			if strings.HasPrefix(cleaned, strings.ToLower(dep)) {
				score++
				break
			}
		}
	}
	if task.Project != "" {
		score++
	}
	if task.DueDate == "" {
		score--
	}
	switch {
	case score >= 4:
		return 4
	case score >= 3:
		return 3
	case score >= 1:
		return 2
	default:
		return 1
	}
}

// doSetPriority sets a specific priority on a task (not cycling).
func (tm *TaskManager) doSetPriority(task Task, newPrio int) {
	ok := tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
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

// doUndo restores the last modified task line to its previous state.
func (tm *TaskManager) doUndo() {
	if len(tm.undoStack) == 0 {
		tm.statusMsg = "Nothing to undo"
		return
	}
	// Pop from stack
	act := tm.undoStack[len(tm.undoStack)-1]
	tm.undoStack = tm.undoStack[:len(tm.undoStack)-1]
	ok := tm.writeLineChange(act.NotePath, act.LineNum, func(_ string) string {
		return act.OldLine
	})
	if ok {
		// Remove the entry that writeLineChange just pushed (the undo itself)
		if len(tm.undoStack) > 0 {
			tm.undoStack = tm.undoStack[:len(tm.undoStack)-1]
		}
		remaining := len(tm.undoStack)
		if remaining > 0 {
			tm.statusMsg = fmt.Sprintf("Undone (%d more)", remaining)
		} else {
			tm.statusMsg = "Undone"
		}
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

// switchView clears any active search and rebuilds the filtered list for a
// new view. Use this when changing tabs so stale search text doesn't silently
// hide tasks in the destination view.
func (tm *TaskManager) switchView() {
	tm.inputMode = tmInputNone
	tm.inputBuf = ""
	tm.filterTag = ""
	tm.filterPriority = -1
	tm.rebuildFiltered()
}

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
	case taskViewEisenhower:
		tm.filtered = nil // uses quadrants, not flat list
	}
	if tm.view != taskViewKanban && tm.view != taskViewEisenhower {
		// Apply active tag filter.
		if tm.filterTag != "" {
			tm.filtered = tm.applyTagFilter(tm.filtered)
		}
		// Apply active priority filter.
		if tm.filterPriority >= 0 {
			tm.filtered = tm.applyPriorityFilter(tm.filtered)
		}
		// Apply text search.
		if tm.inputMode == tmInputSearch && tm.inputBuf != "" {
			tm.filtered = tm.applySearch(tm.filtered)
		}
		// Apply custom sort (only when not default priority sort,
		// since view filters already sort by priority).
		if tm.sortMode != tmSortPriority {
			tm.filtered = tm.applySortMode(tm.filtered)
		}
		// Sort pinned tasks to the top.
		if len(tm.pinnedTasks) > 0 {
			sort.SliceStable(tm.filtered, func(i, j int) bool {
				pi := tm.pinnedTasks[taskKey(tm.filtered[i])]
				pj := tm.pinnedTasks[taskKey(tm.filtered[j])]
				if pi != pj {
					return pi
				}
				return false
			})
		}
		// Hide children of collapsed parent tasks.
		tm.filtered = tm.applyCollapseFilter(tm.filtered)
	}
	// Pre-compute blocked status for all filtered tasks so renderTaskRow
	// doesn't run O(n*m) dependency checks on every frame.
	tm.blockedCache = make(map[string]bool)
	for _, t := range tm.filtered {
		if len(t.DependsOn) > 0 {
			tm.blockedCache[taskKey(t)] = tm.isBlocked(t)
		}
	}

	// Cache tab counts so renderTabs doesn't re-filter on every frame.
	// Apply active tag/priority filters to counts so they reflect what
	// the user would actually see in each tab.
	tm.tabCounts = [7]int{
		len(tm.applyActiveFilters(tm.filterToday())),
		len(tm.applyActiveFilters(tm.filterUpcoming())),
		len(tm.applyActiveFilters(tm.filterAll())),
		len(tm.applyActiveFilters(tm.filterCompleted())),
		-1, // calendar doesn't show count
		-1, // kanban doesn't show count
		-1, // eisenhower doesn't show count
	}
}

func (tm *TaskManager) filterToday() []Task {
	var overdue, today []Task
	for _, t := range tm.allTasks {
		if t.Done || tmIsSnoozed(t) {
			continue
		}
		if tmIsOverdue(t.DueDate) {
			overdue = append(overdue, t)
		} else if tmIsToday(t.DueDate) {
			today = append(today, t)
		}
	}
	// Sort each group by priority descending
	sort.Slice(overdue, func(i, j int) bool {
		return overdue[i].Priority > overdue[j].Priority
	})
	sort.Slice(today, func(i, j int) bool {
		return today[i].Priority > today[j].Priority
	})
	// Overdue first, then today
	return append(overdue, today...)
}

func (tm *TaskManager) filterUpcoming() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if t.Done || tmIsSnoozed(t) {
			continue
		}
		if t.DueDate == "" {
			continue
		}
		days := tmDaysUntil(t.DueDate)
		// Exclude today (days == 0) and overdue — those belong to the Today tab.
		if days >= 1 && days <= 7 {
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
		if t.Done || tmIsSnoozed(t) {
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
		if t.Done {
			continue
		}
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

	// Support #tag queries: "#doing" matches tasks tagged with "doing".
	if strings.HasPrefix(query, "#") {
		if len(query) == 1 {
			// Bare "#" with no tag name — return all tasks unfiltered.
			return tasks
		}
		tagQuery := query[1:]
		var out []Task
		for _, t := range tasks {
			for _, tag := range t.Tags {
				if strings.Contains(strings.ToLower(tag), tagQuery) {
					out = append(out, t)
					break
				}
			}
		}
		return out
	}

	var out []Task
	for _, t := range tasks {
		if strings.Contains(strings.ToLower(t.Text), query) ||
			strings.Contains(strings.ToLower(t.NotePath), query) {
			out = append(out, t)
		}
	}
	return out
}

// applySortMode re-sorts a task list according to the current sort mode.
func (tm *TaskManager) applySortMode(tasks []Task) []Task {
	out := make([]Task, len(tasks))
	copy(out, tasks)
	switch tm.sortMode {
	case tmSortDueDate:
		sort.Slice(out, func(i, j int) bool {
			di, dj := out[i].DueDate, out[j].DueDate
			if di == "" && dj == "" {
				return out[i].Priority > out[j].Priority
			}
			if di == "" {
				return false
			}
			if dj == "" {
				return true
			}
			if di != dj {
				return di < dj
			}
			return out[i].Priority > out[j].Priority
		})
	case tmSortAlpha:
		sort.Slice(out, func(i, j int) bool {
			ti := strings.ToLower(tmCleanText(out[i].Text))
			tj := strings.ToLower(tmCleanText(out[j].Text))
			return ti < tj
		})
	case tmSortSource:
		sort.Slice(out, func(i, j int) bool {
			if out[i].NotePath != out[j].NotePath {
				return out[i].NotePath < out[j].NotePath
			}
			return out[i].LineNum < out[j].LineNum
		})
	case tmSortTag:
		sort.Slice(out, func(i, j int) bool {
			ti, tj := "", ""
			if len(out[i].Tags) > 0 {
				ti = out[i].Tags[0]
			}
			if len(out[j].Tags) > 0 {
				tj = out[j].Tags[0]
			}
			if ti != tj {
				return ti < tj
			}
			return out[i].Priority > out[j].Priority
		})
	}
	return out
}

// tmCleanText strips emoji markers and tags for sort comparison.
func tmIsSnoozed(task Task) bool {
	if task.SnoozedUntil == "" {
		return false
	}
	t, err := time.Parse("2006-01-02T15:04", task.SnoozedUntil)
	if err != nil {
		return false
	}
	return time.Now().Before(t)
}

func tmFormatMinutes(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	h, m := minutes/60, minutes%60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh%dm", h, m)
}

func tmCleanText(s string) string {
	s = tmDueDateRe.ReplaceAllString(s, "")
	s = tmPrioHighestRe.ReplaceAllString(s, "")
	s = tmPrioHighRe.ReplaceAllString(s, "")
	s = tmPrioMedRe.ReplaceAllString(s, "")
	s = tmPrioLowRe.ReplaceAllString(s, "")
	s = tmScheduleRe.ReplaceAllString(s, "")
	s = tmTagRe.ReplaceAllString(s, "")
	s = tmDependsRe.ReplaceAllString(s, "")
	s = tmEstimateRe.ReplaceAllString(s, "")
	s = tmSnoozeRe.ReplaceAllString(s, "")
	s = tmGoalIDRe.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// taskKey returns a unique identifier for a task.
func taskKey(t Task) string {
	return fmt.Sprintf("%s:%d", t.NotePath, t.LineNum)
}

// isBlocked returns true if any of the task's dependencies are not yet done.
// Matching uses cleaned task text with prefix comparison to avoid false
// positives from short substring matches.
func (tm *TaskManager) isBlocked(task Task) bool {
	if len(task.DependsOn) == 0 {
		return false
	}
	for _, dep := range task.DependsOn {
		depLower := strings.ToLower(dep)
		for _, t := range tm.allTasks {
			if t.Done {
				continue
			}
			cleaned := strings.ToLower(tmCleanText(t.Text))
			// Require the dependency to match the start of the task text
			// or be an exact match, to avoid false positives.
			if strings.HasPrefix(cleaned, depLower) || cleaned == depLower {
				return true
			}
		}
	}
	return false
}

// applyCollapseFilter removes children of collapsed parent tasks,
// walking up the ancestor chain so grandchildren are also hidden.
func (tm *TaskManager) applyCollapseFilter(tasks []Task) []Task {
	if len(tm.collapsed) == 0 {
		return tasks
	}
	// Build a lookup for parent lines within each note.
	type noteLineKey struct {
		path string
		line int
	}
	parentOf := make(map[noteLineKey]int)
	for _, t := range tm.allTasks {
		if t.ParentLine > 0 {
			parentOf[noteLineKey{t.NotePath, t.LineNum}] = t.ParentLine
		}
	}

	var out []Task
	for _, t := range tasks {
		hidden := false
		checkLine := t.ParentLine
		for checkLine > 0 {
			key := fmt.Sprintf("%s:%d", t.NotePath, checkLine)
			if tm.collapsed[key] {
				hidden = true
				break
			}
			checkLine = parentOf[noteLineKey{t.NotePath, checkLine}]
		}
		if !hidden {
			out = append(out, t)
		}
	}
	return out
}

// taskHasChildren returns true if any task in allTasks is a child of this task.
func (tm *TaskManager) taskHasChildren(task Task) bool {
	for _, t := range tm.allTasks {
		if t.NotePath == task.NotePath && t.ParentLine == task.LineNum {
			return true
		}
	}
	return false
}

// applyActiveFilters applies the current tag and priority filters to a task
// list. Used both for the displayed list and for computing tab counts.
func (tm *TaskManager) applyActiveFilters(tasks []Task) []Task {
	if tm.filterTag != "" {
		tasks = tm.applyTagFilter(tasks)
	}
	if tm.filterPriority >= 0 {
		tasks = tm.applyPriorityFilter(tasks)
	}
	return tasks
}

func (tm *TaskManager) applyTagFilter(tasks []Task) []Task {
	var out []Task
	for _, t := range tasks {
		for _, tag := range t.Tags {
			if tag == tm.filterTag {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

func (tm *TaskManager) applyPriorityFilter(tasks []Task) []Task {
	var out []Task
	for _, t := range tasks {
		if t.Priority == tm.filterPriority {
			out = append(out, t)
		}
	}
	return out
}

// collectTags returns all unique tags across allTasks with their counts,
// sorted by frequency descending.
func (tm *TaskManager) collectTags() []struct {
	Tag   string
	Count int
} {
	counts := make(map[string]int)
	for _, t := range tm.allTasks {
		if t.Done {
			continue
		}
		for _, tag := range t.Tags {
			counts[tag]++
		}
	}
	type tagCount struct {
		Tag   string
		Count int
	}
	result := make([]tagCount, 0, len(counts))
	for tag, count := range counts {
		result = append(result, tagCount{Tag: tag, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count != result[j].Count {
			return result[i].Count > result[j].Count
		}
		return result[i].Tag < result[j].Tag
	})
	out := make([]struct {
		Tag   string
		Count int
	}, len(result))
	for i, tc := range result {
		out[i] = struct {
			Tag   string
			Count int
		}{Tag: tc.Tag, Count: tc.Count}
	}
	return out
}

// uniqueProjects returns a sorted deduplicated list of project names
// assigned to tasks.
func (tm *TaskManager) uniqueProjects() []string {
	seen := make(map[string]bool)
	var names []string
	for _, t := range tm.allTasks {
		if t.Project != "" && !seen[t.Project] {
			seen[t.Project] = true
			names = append(names, t.Project)
		}
	}
	sort.Strings(names)
	return names
}

// ---------------------------------------------------------------------------
// Kanban columns
// ---------------------------------------------------------------------------

// kanbanColumns returns 4 columns: Backlog, Todo, In Progress, Done.
func (tm *TaskManager) kanbanColumns() [4][]Task {
	var cols [4][]Task

	for _, t := range tm.allTasks {
		if tmIsSnoozed(t) {
			continue
		}
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
	case tmInputReschedule:
		return tm.updateReschedule(key)
	case tmInputDependency:
		return tm.updateDependency(key)
	case tmInputEstimate:
		return tm.updateEstimate(key)
	case tmInputNote:
		return tm.updateNote(key)
	case tmInputSnooze:
		return tm.updateSnooze(key)
	case tmInputBatchReschedule:
		return tm.updateBatchReschedule(key)
	case tmInputHelp:
		tm.inputMode = tmInputNone // any key closes help
		return tm, nil
	case tmInputTemplate:
		return tm.updateTemplate(key)
	case tmInputTemplateName:
		return tm.updateTemplateName(key)
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

func (tm TaskManager) updateReschedule(key string) (TaskManager, tea.Cmd) {
	if tm.cursor >= len(tm.filtered) {
		tm.inputMode = tmInputNone
		return tm, nil
	}
	task := tm.filtered[tm.cursor]
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	switch key {
	case "1": // tomorrow
		tm.inputMode = tmInputNone
		tm.doSetDate(task, today.AddDate(0, 0, 1).Format("2006-01-02"))
	case "2": // next Monday
		tm.inputMode = tmInputNone
		tm.doSetDate(task, tmNextMonday().Format("2006-01-02"))
	case "3": // +1 week
		tm.inputMode = tmInputNone
		tm.doSetDate(task, today.AddDate(0, 0, 7).Format("2006-01-02"))
	case "4": // +1 month
		tm.inputMode = tmInputNone
		tm.doSetDate(task, today.AddDate(0, 1, 0).Format("2006-01-02"))
	case "5", "c": // custom → switch to date picker
		tm.inputMode = tmInputDate
		tm.datePickerDate = today.AddDate(0, 0, 1)
	case "esc":
		tm.inputMode = tmInputNone
	}
	return tm, nil
}

func (tm TaskManager) updateDependency(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
	case "enter":
		dep := strings.TrimSpace(tm.inputBuf)
		if dep != "" && tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			depStr := dep
			if strings.Contains(dep, " ") {
				depStr = `"` + dep + `"` // quote multi-word dependencies
			}
			ok := tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
				return strings.TrimRight(line, " ") + " depends:" + depStr
			})
			if ok {
				tm.statusMsg = "Dependency added: " + dep
				tm.reparse()
			}
		}
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = tm.inputBuf[:len(tm.inputBuf)-1]
		}
	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
		}
	}
	return tm, nil
}

func (tm TaskManager) updateTemplate(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
	default:
		// Number key selects template
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			idx := int(key[0] - '1')
			if idx < len(tm.taskTemplates) {
				tmpl := tm.taskTemplates[idx]
				tm.doAddTask(tmpl.Text)
				tm.statusMsg = "Created from template: " + tmpl.Name
				tm.inputMode = tmInputNone
			}
		}
	}
	return tm, nil
}

func (tm TaskManager) updateTemplateName(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
	case "enter":
		name := strings.TrimSpace(tm.inputBuf)
		if name != "" && tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			// Store the full task text with all markers
			text := tmTaskRe.FindStringSubmatch(strings.TrimSpace(task.Text))
			taskLine := task.Text
			if text != nil {
				taskLine = text[3][2:] // strip "] " prefix
			}
			tm.taskTemplates = append(tm.taskTemplates, taskTemplate{
				Name: name,
				Text: taskLine,
			})
			tm.saveTaskTemplates()
			tm.statusMsg = "Template saved: " + name
		}
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = tm.inputBuf[:len(tm.inputBuf)-1]
		}
	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
		}
	}
	return tm, nil
}

func (tm TaskManager) updateBatchReschedule(key string) (TaskManager, tea.Cmd) {
	if tm.batchReschIdx >= len(tm.batchReschedule) {
		tm.inputMode = tmInputNone
		tm.statusMsg = "Batch reschedule complete"
		tm.reparse()
		return tm, nil
	}
	task := tm.batchReschedule[tm.batchReschIdx]
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	switch key {
	case "1": // tomorrow
		tm.doSetDate(task, today.AddDate(0, 0, 1).Format("2006-01-02"))
		tm.batchReschIdx++
	case "2": // next week
		tm.doSetDate(task, today.AddDate(0, 0, 7).Format("2006-01-02"))
		tm.batchReschIdx++
	case "s": // skip
		tm.batchReschIdx++
	case "esc":
		tm.inputMode = tmInputNone
		tm.reparse()
		return tm, nil
	}

	if tm.batchReschIdx >= len(tm.batchReschedule) {
		tm.inputMode = tmInputNone
		tm.statusMsg = fmt.Sprintf("Rescheduled %d tasks", len(tm.batchReschedule))
		tm.reparse()
	}
	return tm, nil
}

func (tm TaskManager) updateSnooze(key string) (TaskManager, tea.Cmd) {
	if tm.inputTask == nil {
		tm.inputMode = tmInputNone
		return tm, nil
	}
	task := *tm.inputTask
	now := time.Now()
	var snoozeTime time.Time

	switch key {
	case "1": // 1 hour
		snoozeTime = now.Add(1 * time.Hour)
	case "2": // 4 hours
		snoozeTime = now.Add(4 * time.Hour)
	case "3": // tomorrow 9am
		tomorrow := time.Date(now.Year(), now.Month(), now.Day()+1, 9, 0, 0, 0, time.Local)
		snoozeTime = tomorrow
	case "esc":
		tm.inputMode = tmInputNone
		return tm, nil
	default:
		return tm, nil
	}

	snoozeStr := snoozeTime.Format("2006-01-02T15:04")
	ok := tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		line = tmSnoozeRe.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " ")
		return line + " snooze:" + snoozeStr
	})
	if ok {
		tm.statusMsg = "Snoozed until " + snoozeTime.Format("Jan 2 15:04")
		tm.reparse()
	}
	tm.inputMode = tmInputNone
	return tm, nil
}

func (tm TaskManager) updateNote(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
	case "enter":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			k := taskKey(task)
			note := strings.TrimSpace(tm.inputBuf)
			if note == "" {
				delete(tm.taskNotes, k)
			} else {
				tm.taskNotes[k] = note
			}
			tm.saveTaskNotes()
			tm.statusMsg = "Note saved"
		}
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = tm.inputBuf[:len(tm.inputBuf)-1]
		}
	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
		}
	}
	return tm, nil
}

func (tm TaskManager) updateEstimate(key string) (TaskManager, tea.Cmd) {
	if tm.cursor >= len(tm.filtered) {
		tm.inputMode = tmInputNone
		return tm, nil
	}
	task := tm.filtered[tm.cursor]
	var minutes int
	switch key {
	case "1":
		minutes = 15
	case "2":
		minutes = 30
	case "3":
		minutes = 45
	case "4":
		minutes = 60
	case "5":
		minutes = 90
	case "6":
		minutes = 120
	case "esc":
		tm.inputMode = tmInputNone
		return tm, nil
	default:
		return tm, nil
	}

	label := fmt.Sprintf("~%dm", minutes)
	if minutes >= 60 && minutes%60 == 0 {
		label = fmt.Sprintf("~%dh", minutes/60)
	}

	ok := tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		line = tmEstimateRe.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " ")
		return line + " " + label
	})
	if ok {
		tm.statusMsg = "Estimate: " + label
		tm.reparse()
	}
	tm.inputMode = tmInputNone
	return tm, nil
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
		tm.switchView()
	case "2":
		tm.view = taskViewUpcoming
		tm.switchView()
	case "3":
		tm.view = taskViewAll
		tm.switchView()
	case "4":
		tm.view = taskViewCompleted
		tm.switchView()
	case "5":
		tm.view = taskViewCalendar
		tm.switchView()
	case "6":
		// Already on kanban
	case "tab":
		tm.view = (tm.view + 1) % 7
		tm.switchView()

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

	// Handle pending confirmation
	if tm.confirmMsg != "" {
		switch key {
		case "y", "Y":
			if tm.confirmAction != nil {
				tm.confirmAction()
			}
		}
		tm.confirmMsg = ""
		tm.confirmAction = nil
		return tm, nil
	}

	// Handle select mode
	if tm.selectMode {
		switch key {
		case "esc", "q":
			tm.selectMode = false
			tm.selected = make(map[string]bool)
			tm.statusMsg = "Selection cleared"
		case "v":
			tm.selectMode = false
			tm.selected = make(map[string]bool)
		case " ":
			if tm.cursor < len(tm.filtered) {
				k := taskKey(tm.filtered[tm.cursor])
				tm.selected[k] = !tm.selected[k]
				if !tm.selected[k] {
					delete(tm.selected, k)
				}
				// Auto-advance cursor
				if tm.cursor < len(tm.filtered)-1 {
					tm.cursor++
				}
			}
		case "j", "down":
			if tm.cursor < len(tm.filtered)-1 {
				tm.cursor++
				visH := tm.visibleHeight()
				if tm.cursor >= tm.scroll+visH {
					tm.scroll = tm.cursor - visH + 1
				}
			}
		case "k", "up":
			if tm.cursor > 0 {
				tm.cursor--
				if tm.cursor < tm.scroll {
					tm.scroll = tm.cursor
				}
			}
		case "x":
			if len(tm.selected) > 0 {
				tm.doBulkToggle()
			}
		case "d":
			if len(tm.selected) > 0 {
				now := time.Now()
				tm.doBulkSetDate(now.AddDate(0, 0, 1).Format("2006-01-02"))
				tm.selectMode = false
				tm.selected = make(map[string]bool)
			}
		case "c":
			tm.selected = make(map[string]bool)
			tm.statusMsg = "Selection cleared"
		}
		return tm, nil
	}

	switch key {
	case "esc", "q":
		tm.active = false
		return tm, nil

	// Tab switching with number keys
	case "1":
		tm.view = taskViewToday
		tm.switchView()
	case "2":
		tm.view = taskViewUpcoming
		tm.switchView()
	case "3":
		tm.view = taskViewAll
		tm.switchView()
	case "4":
		tm.view = taskViewCompleted
		tm.switchView()
	case "5":
		tm.view = taskViewCalendar
		tm.switchView()
	case "6":
		tm.view = taskViewKanban
		tm.switchView()
	case "7":
		tm.view = taskViewEisenhower
		tm.switchView()

	case "tab":
		tm.view = (tm.view + 1) % 7
		tm.switchView()
	case "shift+tab", "backtab":
		tm.view = (tm.view - 1 + 7) % 7
		tm.switchView()

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

	// Archive old completed tasks (with confirmation)
	case "X":
		tm.confirmMsg = "Archive all completed tasks >30 days? (y/n)"
		tm.confirmAction = func() {
			count := tm.doArchive()
			if count > 0 {
				tm.statusMsg = fmt.Sprintf("Archived %d completed tasks", count)
				tm.reparse()
			} else {
				tm.statusMsg = "No tasks to archive"
			}
		}

	// Save task as template
	case "T":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			tm.inputMode = tmInputTemplateName
			tm.inputBuf = tmCleanText(task.Text)
		}

	// Create task from template
	case "t":
		if len(tm.taskTemplates) > 0 {
			tm.inputMode = tmInputTemplate
		} else {
			tm.statusMsg = "No templates (T to save one)"
		}

	// Help overlay
	case "?":
		tm.inputMode = tmInputHelp

	// Batch reschedule overdue (Today view)
	case "R":
		if tm.view == taskViewToday {
			var overdue []Task
			for _, t := range tm.filtered {
				if tmIsOverdue(t.DueDate) {
					overdue = append(overdue, t)
				}
			}
			if len(overdue) > 0 {
				tm.batchReschedule = overdue
				tm.batchReschIdx = 0
				tm.inputMode = tmInputBatchReschedule
			} else {
				tm.statusMsg = "No overdue tasks"
			}
		}

	// Snooze task
	case "z":
		if tm.cursor < len(tm.filtered) {
			t := tm.filtered[tm.cursor]
			tm.inputTask = &t
			tm.inputMode = tmInputSnooze
		}

	// Task note
	case "n":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			tm.inputMode = tmInputNote
			tm.inputBuf = tm.taskNotes[taskKey(task)]
		}

	// Pin/unpin task
	case "W":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			key := taskKey(task)
			if tm.pinnedTasks[key] {
				delete(tm.pinnedTasks, key)
				tm.statusMsg = "Unpinned"
			} else {
				tm.pinnedTasks[key] = true
				tm.statusMsg = "Pinned"
			}
			tm.savePinnedTasks()
			tm.rebuildFiltered()
		}

	// Auto-priority (heuristic)
	case "A":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			suggested := suggestPriority(task, tm.allTasks)
			prioNames := []string{"none", "low", "medium", "high", "highest"}
			if suggested != task.Priority {
				tm.doSetPriority(task, suggested)
				tm.statusMsg = "Auto-priority: " + prioNames[suggested]
			} else {
				tm.statusMsg = "Priority already optimal"
			}
		}

	// Undo last action
	case "u":
		tm.doUndo()

	// Set time estimate
	case "E":
		if tm.cursor < len(tm.filtered) {
			tm.inputMode = tmInputEstimate
		}

	// Focus session on current task
	case "f":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			tm.focusTask = tmCleanText(task.Text)
			tm.hasFocusReq = true
			tm.active = false
		}

	// Select mode (bulk operations)
	case "v":
		tm.selectMode = true
		tm.selected = make(map[string]bool)
		if tm.cursor < len(tm.filtered) {
			tm.selected[taskKey(tm.filtered[tm.cursor])] = true
		}
		tm.statusMsg = "SELECT MODE: space=select x=toggle d=date Esc=exit"

	// Add dependency
	case "b":
		if tm.cursor < len(tm.filtered) {
			tm.inputMode = tmInputDependency
			tm.inputBuf = ""
		}

	// Expand/collapse subtasks
	case "e":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			if tm.taskHasChildren(task) {
				key := fmt.Sprintf("%s:%d", task.NotePath, task.LineNum)
				tm.collapsed[key] = !tm.collapsed[key]
				tm.rebuildFiltered()
			}
		}

	// Sort mode
	case "s":
		tm.sortMode = (tm.sortMode + 1) % 5
		tm.statusMsg = "Sort: " + tmSortNames[tm.sortMode]
		tm.rebuildFiltered()

	// Reschedule task
	case "r":
		if tm.cursor < len(tm.filtered) {
			tm.inputMode = tmInputReschedule
		}

	// Search
	case "/":
		tm.inputMode = tmInputSearch
		tm.inputBuf = ""

	// Tag filter: cycle through available tags
	case "#":
		tags := tm.collectTags()
		if len(tags) == 0 {
			tm.statusMsg = "No tags found"
			break
		}
		if tm.filterTag == "" {
			tm.filterTag = tags[0].Tag
		} else {
			// Find current tag and advance to next, or wrap to clear
			found := false
			for i, tc := range tags {
				if tc.Tag == tm.filterTag {
					if i+1 < len(tags) {
						tm.filterTag = tags[i+1].Tag
					} else {
						tm.filterTag = "" // wrap around clears filter
					}
					found = true
					break
				}
			}
			if !found {
				tm.filterTag = tags[0].Tag
			}
		}
		if tm.filterTag != "" {
			tm.statusMsg = "Tag: #" + tm.filterTag
		} else {
			tm.statusMsg = "Tag filter cleared"
		}
		tm.rebuildFiltered()

	// Priority filter: cycle none -> highest -> high -> medium -> low -> none
	case "P":
		switch tm.filterPriority {
		case -1:
			tm.filterPriority = 4
		case 4:
			tm.filterPriority = 3
		case 3:
			tm.filterPriority = 2
		case 2:
			tm.filterPriority = 1
		default:
			tm.filterPriority = -1
		}
		prioNames := map[int]string{1: "low", 2: "medium", 3: "high", 4: "highest"}
		if tm.filterPriority >= 0 {
			tm.statusMsg = "Priority: " + prioNames[tm.filterPriority]
		} else {
			tm.statusMsg = "Priority filter cleared"
		}
		tm.rebuildFiltered()

	// Clear all active filters
	case "c":
		if tm.filterTag != "" || tm.filterPriority >= 0 {
			tm.filterTag = ""
			tm.filterPriority = -1
			tm.statusMsg = "Filters cleared"
			tm.rebuildFiltered()
		}
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
	case taskViewEisenhower:
		tm.renderEisenhowerView(&b, innerW)
	default:
		tm.renderTaskList(&b, innerW)
	}
	// Help overlay replaces content
	if tm.inputMode == tmInputHelp {
		b.Reset()
		tm.renderTitle(&b, innerW)
		tm.renderTabs(&b, innerW)
		tm.renderHelpOverlay(&b, innerW)
	}

	// Input bar (if active)
	tm.renderInput(&b, innerW)

	// Confirmation prompt
	if tm.confirmMsg != "" {
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(red).Bold(true).Render(tm.confirmMsg))
	}

	// Status message
	if tm.statusMsg != "" {
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render(tm.statusMsg))
	}

	// Help bar
	tm.renderHelp(&b, innerW)

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// eisenhowerQuadrants returns tasks grouped into 4 quadrants.
func (tm *TaskManager) eisenhowerQuadrants() [4][]Task {
	var q [4][]Task
	for _, t := range tm.allTasks {
		if t.Done || tmIsSnoozed(t) {
			continue
		}
		urgent := tmIsOverdue(t.DueDate) || tmIsToday(t.DueDate) || (t.DueDate != "" && tmDaysUntil(t.DueDate) <= 2)
		important := t.Priority >= 3
		switch {
		case important && urgent:
			q[0] = append(q[0], t)
		case important && !urgent:
			q[1] = append(q[1], t)
		case !important && urgent:
			q[2] = append(q[2], t)
		default:
			q[3] = append(q[3], t)
		}
	}
	return q
}

func (tm *TaskManager) renderEisenhowerView(b *strings.Builder, w int) {
	q := tm.eisenhowerQuadrants()
	quadLabels := [4]string{" DO (urgent+important) ", " SCHEDULE (important) ", " DELEGATE (urgent) ", " ELIMINATE (neither) "}
	quadColors := [4]lipgloss.Color{red, blue, yellow, overlay0}
	
	colW := (w - 4) / 2
	if colW < 20 { colW = 20 }
	
	visH := tm.visibleHeight()
	maxItems := (visH - 8) / 2
	if maxItems < 2 { maxItems = 2 }

	var rows []string
	for r := 0; r < 2; r++ {
		var top, bottom strings.Builder
		for c := 0; c < 2; c++ {
			idx := r*2 + c
			var build strings.Builder
			
			// Label
			headerStyle := lipgloss.NewStyle().
				Foreground(crust).
				Background(quadColors[idx]).
				Bold(true).
				MarginBottom(1)
			build.WriteString(headerStyle.Render(quadLabels[idx]) + "\n")

			for i := 0; i < maxItems; i++ {
				if i < len(q[idx]) {
					t := q[idx][i]
					taskStr := lipgloss.NewStyle().Foreground(quadColors[idx]).Render(tmPriorityIcon(t.Priority)) + " " + TruncateDisplay(tmCleanText(t.Text), colW-6)
					build.WriteString(taskStr + "\n")
				} else {
					build.WriteString("\n")
				}
			}

			if len(q[idx]) > maxItems {
				build.WriteString(DimStyle.Render(fmt.Sprintf("+%d more", len(q[idx])-maxItems)) + "\n")
			} else {
				build.WriteString("\n")
			}

			boxStyle := lipgloss.NewStyle().
				Width(colW).
				Border(PanelBorder).
				BorderForeground(quadColors[idx]).
				Padding(0, 1)

			if c == 0 {
				top.WriteString(boxStyle.Render(build.String()))
			} else {
				bottom.WriteString(boxStyle.Render(build.String()))
			}
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, top.String(), bottom.String()))
	}
	
	b.WriteString(lipgloss.JoinVertical(lipgloss.Left, rows...))
	b.WriteString("\n")
}

func (tm *TaskManager) renderHelpOverlay(b *strings.Builder, w int) {
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true).Width(8)
	descStyle := lipgloss.NewStyle().Foreground(text)
	sectionStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)

	b.WriteString("\n")
	b.WriteString("  " + titleStyle.Render("📋 Task Manager Keyboard Shortcuts") + "\n\n")

	sections := []struct {
		title string
		keys  [][2]string
	}{
		{"Navigation", [][2]string{
			{"j/k", "Move cursor up/down"},
			{"Tab", "Cycle views (Today/Upcoming/All/Done/Calendar/Kanban/Matrix)"},
			{"1-7", "Jump to specific view"},
			{"g", "Jump to task source note"},
			{"/", "Search tasks (#tag syntax supported)"},
			{"Esc", "Close task manager"},
		}},
		{"Actions", [][2]string{
			{"x", "Toggle task done/undone"},
			{"a", "Add new task"},
			{"d", "Set/change due date"},
			{"r", "Reschedule (tomorrow/Monday/+1wk/+1mo/custom)"},
			{"p", "Cycle priority (none/low/med/high/highest)"},
			{"E", "Set time estimate (15m/30m/45m/1h/1.5h/2h)"},
			{"e", "Expand/collapse subtasks"},
			{"b", "Add task dependency"},
			{"f", "Start focus session on task"},
		}},
		{"Power Features", [][2]string{
			{"u", "Undo last action (10-deep stack)"},
			{"n", "Add/edit task note"},
			{"z", "Snooze task (1h/4h/tomorrow 9am)"},
			{"W", "Pin/unpin task (pinned sort to top)"},
			{"A", "Auto-suggest priority (heuristic)"},
			{"R", "Batch reschedule all overdue (Today view)"},
		}},
		{"Filters & Sort", [][2]string{
			{"#", "Cycle tag filter"},
			{"P", "Cycle priority filter"},
			{"s", "Cycle sort mode (priority/date/A-Z/source/tag)"},
			{"c", "Clear all active filters"},
		}},
		{"Bulk Operations", [][2]string{
			{"v", "Enter/exit select mode"},
			{"Space", "Toggle selection (in select mode)"},
		}},
	}

	for _, sec := range sections {
		b.WriteString("  " + sectionStyle.Render(sec.title) + "\n")
		for _, kv := range sec.keys {
			b.WriteString("    " + keyStyle.Render(kv[0]) + descStyle.Render(kv[1]) + "\n")
		}
		b.WriteString("\n")
	}

	// Task syntax reference
	syntaxKeyStyle := lipgloss.NewStyle().Foreground(teal).Width(20)
	b.WriteString("  " + sectionStyle.Render("Task Syntax") + "\n")
	syntaxItems := [][2]string{
		{"- [ ] / - [x]", "Task checkbox (incomplete / complete)"},
		{"📅 2026-04-01", "Due date"},
		{"🔺 ⏫ 🔼 🔽", "Priority (highest / high / medium / low)"},
		{"#tag", "Tag (use multiple, e.g. #work #urgent)"},
		{"~30m / ~2h", "Time estimate"},
		{"⏰ 09:00-10:30", "Scheduled time block"},
		{"🔁 daily", "Recurrence (daily/weekly/monthly/3x-week)"},
		{"depends:\"task\"", "Task dependency (blocks until done)"},
		{"goal:G001", "Link to goal"},
		{"snooze:...T09:00", "Snooze until date+time"},
	}
	for _, kv := range syntaxItems {
		b.WriteString("    " + syntaxKeyStyle.Render(kv[0]) + descStyle.Render(kv[1]) + "\n")
	}
	b.WriteString("\n")
	b.WriteString("  " + sectionStyle.Render("Subtasks") + "\n")
	b.WriteString("    " + descStyle.Render("Indent with 2 spaces to create subtasks:") + "\n")
	b.WriteString("    " + lipgloss.NewStyle().Foreground(text).Render("  - [ ] Parent task") + "\n")
	b.WriteString("    " + lipgloss.NewStyle().Foreground(text).Render("    - [ ] Subtask (2 spaces deeper)") + "\n\n")
	b.WriteString("  " + sectionStyle.Render("Example") + "\n")
	b.WriteString("    " + lipgloss.NewStyle().Foreground(text).Render("- [ ] Ship v2.0 📅 2026-04-01 🔺 #release ~2h goal:G001") + "\n\n")

	b.WriteString("  " + DimStyle.Render("Press any key to close"))
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

	// Workload estimate for Today and Upcoming views
	if tm.view == taskViewToday || tm.view == taskViewUpcoming || tm.view == taskViewAll {
		totalMin := 0
		for _, t := range tm.filtered {
			if !t.Done {
				totalMin += t.EstimatedMinutes
			}
		}
		if totalMin > 0 {
			var workload string
			h, m := totalMin/60, totalMin%60
			if h > 0 && m > 0 {
				workload = fmt.Sprintf("~%dh%dm", h, m)
			} else if h > 0 {
				workload = fmt.Sprintf("~%dh", h)
			} else {
				workload = fmt.Sprintf("~%dm", m)
			}
			stats += lipgloss.NewStyle().Foreground(teal).Render("  " + workload)
		}
	}

	// Active filter indicators
	var filters string
	filterStyle := lipgloss.NewStyle().Foreground(crust).Background(sapphire).Padding(0, 1)
	if tm.filterTag != "" {
		filters += " " + filterStyle.Render("#"+tm.filterTag)
	}
	if tm.filterPriority >= 0 {
		prioNames := []string{"none", "low", "med", "high", "highest"}
		filters += " " + filterStyle.Render("P:"+prioNames[tm.filterPriority])
	}
	if tm.sortMode != tmSortPriority {
		filters += " " + filterStyle.Render("Sort:"+tmSortNames[tm.sortMode])
	}
	if tm.selectMode && len(tm.selected) > 0 {
		filters += " " + lipgloss.NewStyle().Foreground(crust).Background(mauve).Padding(0, 1).
			Render(fmt.Sprintf("%d selected", len(tm.selected)))
	}

	b.WriteString("  " + icon + title + stats + filters)
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
		"Matrix",
	}
	counts := tm.tabCounts[:]

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

		// Today view: group overdue vs today
		if tm.view == taskViewToday {
			var group string
			if tmIsOverdue(task.DueDate) {
				group = "overdue"
			} else {
				group = "today"
			}
			if group != lastGroup {
				if lastGroup != "" {
					b.WriteString("\n")
				}
				if group == "overdue" {
					b.WriteString("  " + lipgloss.NewStyle().Foreground(crust).Background(red).Bold(true).Padding(0, 1).Render("OVERDUE"))
				} else {
					b.WriteString("  " + lipgloss.NewStyle().Foreground(crust).Background(green).Bold(true).Padding(0, 1).Render("TODAY"))
				}
				b.WriteString("\n")
				lastGroup = group
			}
		}

		// Upcoming view: group by day
			// All view: group by Note Path
			if tm.view == taskViewAll || tm.view == taskViewCompleted {
				group := task.NotePath
				if group != lastGroup {
					if lastGroup != "" {
						b.WriteString("\n")
					}
					b.WriteString("  " + lipgloss.NewStyle().Foreground(crust).Background(sapphire).Bold(true).Padding(0, 1).Render(filepath.Base(group)))
					b.WriteString("\n")
					lastGroup = group
				}
			}

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
				b.WriteString("  " + lipgloss.NewStyle().Foreground(crust).Background(lavender).Bold(true).Padding(0, 1).Render(dayLabel))
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

	// Task text (strip emoji markers and tags for cleaner display)
	displayText := task.Text
	displayText = tmDueDateRe.ReplaceAllString(displayText, "")
	displayText = tmPrioHighestRe.ReplaceAllString(displayText, "")
	displayText = tmPrioHighRe.ReplaceAllString(displayText, "")
	displayText = tmPrioMedRe.ReplaceAllString(displayText, "")
	displayText = tmPrioLowRe.ReplaceAllString(displayText, "")
	displayText = tmScheduleRe.ReplaceAllString(displayText, "")
	displayText = tmTagRe.ReplaceAllString(displayText, "")
	displayText = tmDependsRe.ReplaceAllString(displayText, "")
	displayText = tmEstimateRe.ReplaceAllString(displayText, "")
	displayText = tmSnoozeRe.ReplaceAllString(displayText, "")
	displayText = tmGoalIDRe.ReplaceAllString(displayText, "")
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

	// Estimate badge
	var estimateBadge string
	estimateWidth := 0
	if task.EstimatedMinutes > 0 {
		label := fmt.Sprintf("~%dm", task.EstimatedMinutes)
		if task.EstimatedMinutes >= 60 && task.EstimatedMinutes%60 == 0 {
			label = fmt.Sprintf("~%dh", task.EstimatedMinutes/60)
		} else if task.EstimatedMinutes > 60 {
			label = fmt.Sprintf("~%dh%dm", task.EstimatedMinutes/60, task.EstimatedMinutes%60)
		}
		estimateBadge = lipgloss.NewStyle().
			Foreground(crust).Background(sky).Padding(0, 1).
			Render(label)
		estimateWidth = len(label) + 3
	}

	// Actual time badge (from time tracker)
	var actualBadge string
	actualWidth := 0
	if task.ActualMinutes > 0 {
		label := tmFormatMinutes(task.ActualMinutes)
		badgeColor := green
		if task.EstimatedMinutes > 0 && task.ActualMinutes > task.EstimatedMinutes {
			badgeColor = red
		}
		actualBadge = lipgloss.NewStyle().
			Foreground(crust).Background(badgeColor).Padding(0, 1).
			Render(label)
		actualWidth = len(label) + 3
	}

	// Source note badge
	noteName := strings.TrimSuffix(filepath.Base(task.NotePath), ".md")
	noteLabel := lipgloss.NewStyle().
		Foreground(text).
		Background(surface1).
		Padding(0, 1).
		Render(noteName)

	// Pin indicator
	pinStr := ""
	prefixExtra := 0
	if tm.pinnedTasks[taskKey(task)] {
		pinStr = lipgloss.NewStyle().Foreground(yellow).Render("\U0001F4CC ")
		prefixExtra += 3
	}

	// Note indicator
	noteStr := ""
	if tm.taskNotes[taskKey(task)] != "" {
		noteStr = lipgloss.NewStyle().Foreground(yellow).Render("\U0001F4DD ")
		prefixExtra += 3
	}

	// Goal link indicator
	goalStr := ""
	if task.GoalID != "" {
		goalStr = lipgloss.NewStyle().Foreground(sapphire).Render("\u2691 ")
		prefixExtra += 2
	}

	// Selection indicator (in select mode)
	selectStr := ""
	if tm.selectMode {
		if tm.selected[taskKey(task)] {
			selectStr = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("[*] ")
		} else {
			selectStr = DimStyle.Render("[ ] ")
		}
		prefixExtra += 4
	}

	// Blocked indicator (uses pre-computed cache from rebuildFiltered)
	blockedStr := ""
	if tm.blockedCache[taskKey(task)] {
		blockedStr = lipgloss.NewStyle().Foreground(red).Render("🔒")
		textStyle = lipgloss.NewStyle().Foreground(overlay0) // dim blocked tasks
		prefixExtra += 2
	}

	// Subtask indentation and collapse indicator
	indentStr := ""
	if task.Indent > 0 {
		indentStr = strings.Repeat("  ", task.Indent) + DimStyle.Render("└ ")
		prefixExtra += task.Indent*2 + 2
	}
	collapseIndicator := ""
	if tm.taskHasChildren(task) {
		key := fmt.Sprintf("%s:%d", task.NotePath, task.LineNum)
		if tm.collapsed[key] {
			collapseIndicator = lipgloss.NewStyle().Foreground(overlay1).Render("[+] ")
			prefixExtra += 4
		}
	}

	// Truncate text (account for all variable-width prefixes and badges)
	tagWidth := 0
	for _, tag := range task.Tags {
		tagWidth += len(tag) + 2 // "#" + tag + space
	}
	maxTextW := w - 30 - tagWidth - prefixExtra - estimateWidth - actualWidth
	if maxTextW < 10 {
		maxTextW = 10
	}
	displayText = TruncateDisplay(displayText, maxTextW)

	// Build the line
	prefix := "  "
	if isSelected {
		prefix = lipgloss.NewStyle().Foreground(mauve).Render("  " + ThemeAccentBar + " ")
	}

	// Tag badges
	tagColors := []lipgloss.Color{sapphire, teal, lavender, pink, peach, sky}
	var tagBadges string
	for i, tag := range task.Tags {
		c := tagColors[i%len(tagColors)]
		tagBadges += " " + lipgloss.NewStyle().Foreground(c).Render("#"+tag)
	}

leftSideTokens := []string{prefix + pinStr + noteStr + goalStr + selectStr + indentStr + collapseIndicator + blockedStr + checkbox, prioStyled, textStyle.Render(displayText)}
        if tagBadges != "" {
                leftSideTokens = append(leftSideTokens, tagBadges)
        }
        leftSide := strings.Join(leftSideTokens, " ")

        rightSideTokens := []string{}
        if estimateBadge != "" {
                rightSideTokens = append(rightSideTokens, estimateBadge)
        }
        if actualBadge != "" {
                rightSideTokens = append(rightSideTokens, actualBadge)
        }
        if scheduleBadge != "" {
                rightSideTokens = append(rightSideTokens, scheduleBadge)
        }
        if dueBadge != "" {
                rightSideTokens = append(rightSideTokens, dueBadge)
        }
        rightSideTokens = append(rightSideTokens, noteLabel)
        rightSide := strings.Join(rightSideTokens, " ")

        leftW := lipgloss.Width(leftSide)
        rightW := lipgloss.Width(rightSide)
        
        spacing := w - leftW - rightW - 2
        if spacing < 1 {
                spacing = 1
        }

        line := leftSide + strings.Repeat(" ", spacing) + rightSide

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
	case tmInputDependency:
		b.WriteString("  " + promptStyle.Render("Depends on: ") + inputStyle.Render(tm.inputBuf+"\u2588"))
	case tmInputNote:
		b.WriteString("  " + promptStyle.Render("Note: ") + inputStyle.Render(tm.inputBuf+"\u2588"))
	case tmInputBatchReschedule:
		if tm.batchReschIdx < len(tm.batchReschedule) {
			task := tm.batchReschedule[tm.batchReschIdx]
			progress := fmt.Sprintf("[%d/%d] ", tm.batchReschIdx+1, len(tm.batchReschedule))
			taskText := TruncateDisplay(tmCleanText(task.Text), w-20)
			b.WriteString("  " + promptStyle.Render("Reschedule: ") + DimStyle.Render(progress) + lipgloss.NewStyle().Foreground(text).Render(taskText))
			b.WriteString("\n")
			ss := lipgloss.NewStyle().Foreground(lavender).Bold(true)
			ds := lipgloss.NewStyle().Foreground(overlay0)
			b.WriteString("  " +
				ss.Render("1") + ds.Render(":tomorrow ") +
				ss.Render("2") + ds.Render(":+1week ") +
				ss.Render("s") + ds.Render(":skip ") +
				ss.Render("Esc") + ds.Render(":cancel"))
		}
	case tmInputTemplate:
		b.WriteString("  " + promptStyle.Render("Create from template:") + "\n")
		for i, tmpl := range tm.taskTemplates {
			if i >= 9 {
				break
			}
			numStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
			b.WriteString("    " + numStyle.Render(fmt.Sprintf("%d", i+1)) + " " + tmpl.Name + " — " + DimStyle.Render(TruncateDisplay(tmpl.Text, w-20)) + "\n")
		}
		b.WriteString("  " + DimStyle.Render("Esc:cancel"))
	case tmInputTemplateName:
		b.WriteString("  " + promptStyle.Render("Template name: ") + inputStyle.Render(tm.inputBuf+"\u2588"))
	case tmInputSnooze:
		b.WriteString("  " + promptStyle.Render("Snooze: "))
		b.WriteString("\n")
		ss := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		ds := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString("  " +
			ss.Render("1") + ds.Render(":1h ") +
			ss.Render("2") + ds.Render(":4h ") +
			ss.Render("3") + ds.Render(":tomorrow 9am ") +
			ss.Render("Esc") + ds.Render(":cancel"))
	case tmInputEstimate:
		b.WriteString("  " + promptStyle.Render("Estimate: "))
		b.WriteString("\n")
		shortcutStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		descStyle2 := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString("  " +
			shortcutStyle.Render("1") + descStyle2.Render(":15m ") +
			shortcutStyle.Render("2") + descStyle2.Render(":30m ") +
			shortcutStyle.Render("3") + descStyle2.Render(":45m ") +
			shortcutStyle.Render("4") + descStyle2.Render(":1h ") +
			shortcutStyle.Render("5") + descStyle2.Render(":1.5h ") +
			shortcutStyle.Render("6") + descStyle2.Render(":2h ") +
			shortcutStyle.Render("Esc") + descStyle2.Render(":cancel"))
	case tmInputReschedule:
		b.WriteString("  " + promptStyle.Render("Reschedule: "))
		b.WriteString("\n")
		shortcutStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		descStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString("  " +
			shortcutStyle.Render("1") + descStyle.Render(":tomorrow ") +
			shortcutStyle.Render("2") + descStyle.Render(":monday ") +
			shortcutStyle.Render("3") + descStyle.Render(":+1week ") +
			shortcutStyle.Render("4") + descStyle.Render(":+1month ") +
			shortcutStyle.Render("5") + descStyle.Render(":custom ") +
			shortcutStyle.Render("Esc") + descStyle.Render(":cancel"))
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
			{"j/k", "nav"}, {"x", "toggle"}, {"u", "undo"}, {"n", "note"}, {"z", "snooze"},
			{"e", "expand"}, {"f", "focus"}, {"g", "go"}, {"a", "add"}, {"d", "date"},
			{"E", "estimate"}, {"r", "reschedule"}, {"s", "sort"}, {"W", "pin"}, {"A", "auto-prio"},
			{"p", "prio"}, {"#", "tag"}, {"P", "filter prio"}, {"c", "clear"}, {"/", "search"},
			{"Tab", "view"}, {"Esc", "close"},
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
	if colWidth < 20 { colWidth = 20 }
	visH := tm.kanbanVisibleHeight()

	var colViews []string
	for c := 0; c < 4; c++ {
		var cb strings.Builder
		isActiveCol := c == tm.kanbanCol

		/* Header */
		countStr := fmt.Sprintf(" %d ", len(cols[c]))
		countStyle := lipgloss.NewStyle().Foreground(surface0).Background(base).Padding(0, 1).MarginLeft(1).Bold(true)
		headerStyle := lipgloss.NewStyle().Foreground(colColors[c]).Bold(true).PaddingBottom(1)
		
		if isActiveCol {
			countStyle = countStyle.Background(colColors[c]).Foreground(base)
		}

		cb.WriteString(headerStyle.Render(colNames[c]) + countStyle.Render(countStr) + "\n")

		/* Content */
		if len(cols[c]) == 0 {
			emptyBox := lipgloss.NewStyle().
			    Width(colWidth - 4).
			    Height(3).
			    BorderStyle(PanelBorder).
			    BorderForeground(surface0).
			    Align(lipgloss.Center).
			    Render("Empty")
			cb.WriteString(emptyBox + "\n")
		} else {
			start := tm.kanbanScroll[c]
			end := start + visH
			if end > len(cols[c]) { end = len(cols[c]) }
			for i := start; i < end; i++ {
				isSelected := isActiveCol && i == tm.kanbanCursor[c]
				tm.renderKanbanCard(&cb, cols[c][i], isSelected, colWidth-4)
				cb.WriteString("\n")
			}
			if end < len(cols[c]) {
				cb.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(fmt.Sprintf("\u2193 %d more", len(cols[c])-end)) + "\n")
			}
		}

		colStyle := lipgloss.NewStyle().Width(colWidth - 2).Padding(0, 1)
		if isActiveCol {
			colStyle = colStyle.Border(lipgloss.NormalBorder(), false, true, false, true).BorderForeground(surface1).Padding(0, 0)
		} else {
		    colStyle = colStyle.Border(lipgloss.NormalBorder(), false, true, false, false).BorderForeground(surface0)
		}

		colViews = append(colViews, colStyle.Render(cb.String()))
	}

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, colViews...))
	b.WriteString("\n")
}

func (tm *TaskManager) renderKanbanCard(b *strings.Builder, task Task, isSelected bool, w int) {
	borderColor := surface0
	if isSelected { borderColor = lavender }
	
	boxW := w
	if boxW < 10 { boxW = 10 }

	boxStyle := lipgloss.NewStyle().
	    Border(PanelBorder).
	    BorderForeground(borderColor).
	    Width(boxW).
	    Padding(0, 1)

	if isSelected { 
	    boxStyle = boxStyle.Background(surface0).BorderForeground(lavender) 
	}

	var contentBuilder strings.Builder

	/* Priority & Due Date */
	prioIcon := tmPriorityIcon(task.Priority)
	prioColor := tmPriorityColor(task.Priority)
	if prioIcon != "" {
		contentBuilder.WriteString(lipgloss.NewStyle().Foreground(prioColor).Render(prioIcon) + " ")
	}

	if task.DueDate != "" {
		dueLabel := tmFormatDue(task.DueDate)
		dueColor := overlay0
		if tmIsOverdue(task.DueDate) { dueColor = red } else if tmIsToday(task.DueDate) { dueColor = yellow }
		contentBuilder.WriteString(lipgloss.NewStyle().Foreground(dueColor).Render("⏰ " + dueLabel))
	} else {
	    contentBuilder.WriteString(lipgloss.NewStyle().Foreground(surface0).Render("⏰ No Date"))
	}
	contentBuilder.WriteString("\n")

	/* Title */
	displayText := tmCleanText(task.Text)
	runes := []rune(displayText)
	maxTextW := boxW - 2 
	if maxTextW < 1 { maxTextW = 1 }
	if len(runes) > maxTextW*2 { displayText = string(runes[:maxTextW*2-3]) + "..." }
	
	textStyle := lipgloss.NewStyle().Foreground(text).Width(maxTextW)
	if task.Done { textStyle = textStyle.Foreground(overlay0).Strikethrough(true) }
	
	contentBuilder.WriteString(textStyle.Render(displayText) + "\n")
	
	/* Status & Badge */
	checkbox := lipgloss.NewStyle().Foreground(overlay0).Render("[ ]")
	if task.Done { checkbox = lipgloss.NewStyle().Foreground(green).Render("[✓]") }
	
	projectBadge := ""
	if task.Project != "" { projectBadge = lipgloss.NewStyle().Foreground(blue).Render("#" + task.Project) }

    bottomLine := checkbox
    if projectBadge != "" {
        padLen := maxTextW - lipgloss.Width(checkbox) - lipgloss.Width(projectBadge)
        if padLen > 0 {
            bottomLine += strings.Repeat(" ", padLen) + projectBadge
        } else {
            bottomLine += " " + projectBadge
        }
    }
	contentBuilder.WriteString(bottomLine)

	b.WriteString(boxStyle.Render(contentBuilder.String()))
}

// UI configuration updated.
// Enhanced UI for Tasks
