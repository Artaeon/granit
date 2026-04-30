package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/vault"
)

// ---------------------------------------------------------------------------
// Task data
// ---------------------------------------------------------------------------

// Task represents a single task extracted from a markdown note.
// Task is the canonical task representation. Aliased to
// tasks.Task so the new sidecar-tracked fields (ID, Triage,
// ScheduledStart, etc.) are accessible from tui code without
// per-call conversion. The struct definition lives in
// internal/tasks/task.go.
type Task = tasks.Task

// taskView identifies which tab is active.
type taskView int

const (
	taskViewToday     taskView = iota // 0
	taskViewUpcoming                  // 1
	taskViewAll                       // 2
	taskViewCompleted                 // 3
	taskViewCalendar                  // 4
	taskViewKanban                    // 5
	taskViewInbox                     // 6 — untriaged tasks (Triage == TriageInbox)
	taskViewStale                     // 7 — tasks not updated in 7+ days
	taskViewByProject                 // 8 — grouped by Project field
	taskViewQuickWins                 // 9 — high-priority + small estimate (≤30min)
	taskViewByTag                     // 10 — grouped by first tag
	taskViewReview                    // 11 — completed in the last 7 days
)

const taskViewCount = 12

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
	tmInputFilter                      // unified filter input — parses #tag, p:level, sort:mode, search text
	tmInputInlineEdit                  // inline edit of the cursor task's full line (no source-note context switch)
	tmInputQuickEdit                   // single-line quick edit on cursor task: p:N, d:YYYY-MM-DD, ~30m
	tmInputTimeBlock                   // assign task to a time block: 1=morning 2=midday 3=afternoon 4=evening
	tmInputTriageSet                   // direct-set triage submode entered via 'm' (mark): i/t/s/n/d/x set state then exit
	tmInputSavedFilter                 // '*' save / recall named filter combo
)

// tmSavedFilter persists a named filter combo so power users can
// recall complex multi-axis narrowings ("My Sprint" = #urgent +
// p:high + sort:due) with one keystroke. Mirrors the filter
// fields on TaskManager so apply is just a struct copy.
type tmSavedFilter struct {
	Tag      string `json:"tag"`
	Priority int    `json:"priority"`
	Triage   string `json:"triage"`
	Search   string `json:"search"`
	Sort     int    `json:"sort"`
}

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
	OverlayBase

	// Data
	vault    *vault.Vault
	config   config.Config
	// taskStore is set when cfg.UseTaskStore is on. When non-nil,
	// reads source from store.All() (after a fresh Reload) and the
	// "add task" path routes through store.Create. The 15+
	// writeLineChange callers stay on the legacy path for now —
	// migrating those to store.UpdateLine is a separate effort
	// since each caller has bespoke transform logic.
	taskStore *tasks.TaskStore
	allTasks  []Task
	filtered  []Task // currently displayed tasks

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
	filterTag      string                // "" = no tag filter, "tagname" = filter to this tag
	filterPriority int                   // -1 = no priority filter, 0-4 = filter to this level
	filterTriage   tasks.TriageState     // "" = no triage filter, "inbox"/"triaged"/etc. = filter
	filterProject  string                // "" = no project filter, "name" = filter to this project name
	searchTerm     string                // "" = no search, otherwise substring or "#tag" query

	// Saved filter views — power users persist a filter combo
	// ("My Sprint" = #urgent + p:high + sort:due) and recall
	// it with one keystroke instead of re-typing the unified
	// query. Persisted to .granit/tm-saved-filters.json. '*'
	// opens the picker / save prompt.
	savedFilters map[string]tmSavedFilter

	// Time-tracking handoff — set when user presses 'y' on a
	// task. The model reads it via GetTimerToggleRequest after
	// each Update and starts/stops the actual timer (TaskManager
	// doesn't own TimeTracker; consumed-once flag pattern).
	timerToggleReq  bool
	timerToggleTask Task

	// Snapshot of the model's TimeTracker state, refreshed
	// before each render via SetActiveTimer. Lets the row
	// renderer show a "▸ Nm" badge on the task being tracked
	// without TM holding a reference to the timer overlay.
	activeTimerTask string
	activeTimerSecs int

	// Compact mode — power-user density toggle. When on, the
	// per-cursor detail strip and section dividers are
	// suppressed so the user sees ~60% more tasks per screen.
	// Hint dots already convey "metadata exists" without taking
	// horizontal room. Toggled with D in normal mode.
	compact bool

	// Cached tab counts (updated by rebuildFiltered)
	tabCounts [taskViewCount]int

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

	// AI config
	ai          AIConfig
	aiPending bool // waiting for AI response

	// lastSaveErr stores the most recent error from saving
	// task-templates.json, pinned-tasks.json, or task-notes.json so the
	// host Model can surface it via reportError. Consumed once.
	lastSaveErr error
}

// ConsumeSaveError returns and clears the last persistence error, if any.
// The host Model calls this after each Update and routes non-nil errors
// through reportError so the user sees that pinned/template/note state
// didn't save.
func (tm *TaskManager) ConsumeSaveError() error {
	if tm == nil {
		return nil
	}
	err := tm.lastSaveErr
	tm.lastSaveErr = nil
	return err
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

// SetTaskStore wires the TaskStore so reads come from the
// canonical task layer (with stable IDs and triage state) and
// adds route through store.Create. Nil-safe — falls back to the
// legacy ParseAllTasks / appendTaskLine paths when nil.
func (tm *TaskManager) SetTaskStore(s *tasks.TaskStore) { tm.taskStore = s }

// parseAllTasks returns the canonical task list. Store-backed
// when wired (a Reload+All snapshot so external edits show up),
// or a fresh ParseAllTasks scan otherwise.
func (tm *TaskManager) parseAllTasks() []Task {
	if tm.taskStore != nil {
		_ = tm.taskStore.Reload()
		return tm.taskStore.All()
	}
	return ParseAllTasks(tm.vault.Notes)
}

// Open parses all tasks from the vault and activates the overlay.
func (tm *TaskManager) Open(v *vault.Vault) {
	tm.Activate()
	tm.vault = v
	tm.allTasks = FilterTasks(tm.parseAllTasks(), tm.config)
	tm.view = taskViewToday
	tm.cursor = 0
	tm.scroll = 0
	tm.inputMode = tmInputNone
	tm.inputBuf = ""
	tm.statusMsg = ""
	// Filters are sticky across sessions — load before resetting
	// the in-memory defaults so a fresh-open session that has no
	// saved file falls through to the empty-filter default below.
	tm.filterTag = ""
	tm.filterPriority = -1
	tm.filterTriage = ""
	tm.searchTerm = ""
	tm.loadFilters()
	tm.loadSavedFilters()
	tm.timerToggleReq = false
	tm.activeTimerTask = ""
	tm.activeTimerSecs = 0
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


// Refresh re-parses all tasks from the vault without resetting UI state.
// This is used when another component modifies vault files while the task
// manager is still open, so the displayed task list stays current.
func (tm *TaskManager) Refresh(v *vault.Vault) {
	tm.vault = v
	tm.allTasks = FilterTasks(tm.parseAllTasks(), tm.config)
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

// CountTasksDueToday returns the number of incomplete tasks due today or overdue.
func CountTasksDueToday(notes map[string]*vault.Note) int {
	return CountTasksDueTodayFromList(ParseAllTasks(notes))
}

// CountTasksDueTodayFromList counts from a pre-parsed task list. Excludes
// snoozed and dropped tasks so the status-bar badge matches what the user
// actually sees in the active TaskManager views — otherwise the user sees
// "5 due today" in the status bar but only 3 rows in Plan because 2 have
// been snoozed/dropped, and they have to dig to figure out the discrepancy.
func CountTasksDueTodayFromList(tasksList []Task) int {
	count := 0
	for _, t := range tasksList {
		if t.Done || tmIsSnoozed(t) || t.Triage == tasks.TriageDropped {
			continue
		}
		if t.DueDate != "" && (tmIsToday(t.DueDate) || tmIsOverdue(t.DueDate)) {
			count++
		}
	}
	return count
}

// CountOverdueTasks counts how many unchecked tasks are past their due date.
func CountOverdueTasks(notes map[string]*vault.Note) int {
	return CountOverdueTasksFromList(ParseAllTasks(notes))
}

// CountOverdueTasksFromList counts from a pre-parsed task list. Same
// exclusion logic as CountTasksDueTodayFromList — snoozed/dropped tasks
// are not part of the user's actionable workload, so counting them in
// the overdue badge inflates the number above what the views actually
// surface and makes the badge a lie.
func CountOverdueTasksFromList(tasksList []Task) int {
	count := 0
	for _, t := range tasksList {
		if t.Done || tmIsSnoozed(t) || t.Triage == tasks.TriageDropped {
			continue
		}
		if t.DueDate != "" && tmIsOverdue(t.DueDate) {
			count++
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

// ParseAllTasks scans all note content for task lines. Thin wrapper
// over tasks.ParseNotes — the actual parsing logic lives in the
// tasks package now so the upcoming TaskStore can call it directly
// without depending on tui.
func ParseAllTasks(notes map[string]*vault.Note) []Task {
	items := make([]tasks.NoteContent, 0, len(notes))
	for _, n := range notes {
		items = append(items, tasks.NoteContent{Path: n.RelPath, Content: n.Content})
	}
	return tasks.ParseNotes(items)
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
	if err := atomicWriteNote(absPath, newContent); err != nil {
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
	tm.allTasks = FilterTasks(tm.parseAllTasks(), tm.config)
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

// doDelete removes the task line entirely from disk. Prefers the unified
// TaskStore (stable IDs, sidecar tombstone) when wired so the deletion
// participates in the planning loop; falls back to in-place line removal
// via the vault note when the store isn't available. Returns true on
// success so the caller can refresh state and surface a status message.
func (tm *TaskManager) doDelete(task Task) bool {
	if tm.taskStore != nil && task.ID != "" {
		if err := tm.taskStore.Delete(task.ID); err == nil {
			tm.fileChanged = true
			tm.lastChangedNote = task.NotePath
			return true
		}
		// Fall through to legacy path on store error so the user
		// still gets the line removed even if the sidecar is in
		// a weird state.
	}
	if tm.vault == nil {
		return false
	}
	note := tm.vault.GetNote(task.NotePath)
	if note == nil {
		return false
	}
	lines := strings.Split(note.Content, "\n")
	if task.LineNum < 1 || task.LineNum > len(lines) {
		return false
	}
	tm.undoStack = append(tm.undoStack, taskAction{
		NotePath: task.NotePath,
		LineNum:  task.LineNum,
		OldLine:  lines[task.LineNum-1],
	})
	if len(tm.undoStack) > 10 {
		tm.undoStack = tm.undoStack[len(tm.undoStack)-10:]
	}
	lines = append(lines[:task.LineNum-1], lines[task.LineNum:]...)
	newContent := strings.Join(lines, "\n")
	note.Content = newContent
	absPath := filepath.Join(tm.vault.Root, task.NotePath)
	if err := atomicWriteNote(absPath, newContent); err != nil {
		return false
	}
	tm.fileChanged = true
	tm.lastChangedNote = task.NotePath
	return true
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

	// Dedup guard: if the user toggles a recurring task done →
	// undone → done in quick succession (or any other reason
	// doToggle fires twice on the same instance), createNextRecurrence
	// would otherwise append a fresh copy each time, leaving the
	// vault with N identical "next instance" lines. Scan the source
	// file for an existing incomplete task with the same normalised
	// text AND the same target due date — if found, skip the append
	// and just update the status message so the user knows the
	// recurrence is already in flight.
	note := tm.vault.GetNote(task.NotePath)
	if note == nil {
		return
	}
	wantedNorm := tasks.NormalizeTaskText(newLine)
	for _, line := range strings.Split(note.Content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [ ]") {
			continue
		}
		if !strings.Contains(trimmed, dateStr) {
			continue
		}
		if tasks.NormalizeTaskText(trimmed) == wantedNorm {
			tm.statusMsg = fmt.Sprintf("Task completed — next instance for %s already exists", dateStr)
			return
		}
	}

	// Append to the same file
	note.Content += "\n" + newLine + "\n"
	absPath := filepath.Join(tm.vault.Root, task.NotePath)
	if err := atomicWriteNote(absPath, note.Content); err != nil {
		tm.statusMsg = "recurrence re-create failed: " + err.Error()
		return
	}
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
	data, err := readTasksFile(tm.vault.Root)
	if err != nil || len(data) == 0 {
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

	// Write the archive file FIRST. If this fails we must NOT truncate
	// Tasks.md — losing the archive write while keeping the truncation
	// silently destroys completed-task history. Surface the error and
	// return 0 so the caller doesn't claim work was archived.
	archiveDir := filepath.Join(tm.vault.Root, "Archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		tm.statusMsg = "archive failed: " + err.Error()
		return 0
	}
	archiveFile := filepath.Join(archiveDir, "tasks-"+now.Format("2006-01")+".md")
	archiveContent := ""
	if existing, err := os.ReadFile(archiveFile); err == nil {
		archiveContent = string(existing)
	}
	archiveContent += "\n" + strings.Join(archived, "\n") + "\n"
	if err := atomicWriteNote(archiveFile, archiveContent); err != nil {
		tm.statusMsg = "archive write failed: " + err.Error()
		return 0
	}

	// Archive is durable; safe to truncate the source. A failure here
	// means the archive has the rows AND the source still does — at most
	// duplication on next archive pass, never loss.
	if err := writeTasksFile(tm.vault.Root, []byte(strings.Join(kept, "\n")+"\n")); err != nil {
		tm.statusMsg = "tasks rewrite failed (archive saved): " + err.Error()
		return 0
	}

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
	if err := atomicWriteNote(filepath.Join(dir, "task-templates.json"), string(data)); err != nil {
		tm.lastSaveErr = err
	}
}

// tmFilterState is the on-disk form of the active filter set.
// Saved as .granit/tm-filters.json so reopening TaskManager
// (within the same vault) restores the user's last filter view —
// power users who pinned themselves to "p:high #urgent" don't
// have to re-type that combo every session.
type tmFilterState struct {
	Tag      string `json:"tag"`
	Priority int    `json:"priority"`
	Triage   string `json:"triage"`
	Search   string `json:"search"`
	Sort     int    `json:"sort"`
}

// loadFilters restores filterTag/Priority/Triage/searchTerm/
// sortMode from .granit/tm-filters.json. Silent on missing or
// malformed file — fresh users get the default empty filters.
func (tm *TaskManager) loadFilters() {
	if tm.vault == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(tm.vault.Root, ".granit", "tm-filters.json"))
	if err != nil {
		return
	}
	var st tmFilterState
	if err := json.Unmarshal(data, &st); err != nil {
		return
	}
	tm.filterTag = st.Tag
	tm.filterPriority = st.Priority
	tm.filterTriage = tasks.TriageState(st.Triage)
	tm.searchTerm = st.Search
	if st.Sort >= 0 && st.Sort < 5 {
		tm.sortMode = tmSortMode(st.Sort)
	}
}

// saveFilters writes the current filter set to disk. Called from
// each filter-mutating handler so the on-disk state matches what
// the user just did. Errors land on lastSaveErr (surfaced via
// the same toast pipeline as other save failures).
func (tm *TaskManager) saveFilters() {
	if tm.vault == nil {
		return
	}
	dir := filepath.Join(tm.vault.Root, ".granit")
	_ = os.MkdirAll(dir, 0755)
	st := tmFilterState{
		Tag:      tm.filterTag,
		Priority: tm.filterPriority,
		Triage:   string(tm.filterTriage),
		Search:   tm.searchTerm,
		Sort:     int(tm.sortMode),
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return
	}
	if err := atomicWriteNote(filepath.Join(dir, "tm-filters.json"), string(data)); err != nil {
		tm.lastSaveErr = err
	}
}

// loadSavedFilters restores the named filter combos from
// .granit/tm-saved-filters.json. Silent on missing/malformed.
func (tm *TaskManager) loadSavedFilters() {
	tm.savedFilters = make(map[string]tmSavedFilter)
	if tm.vault == nil {
		return
	}
	data, err := os.ReadFile(filepath.Join(tm.vault.Root, ".granit", "tm-saved-filters.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &tm.savedFilters)
	if tm.savedFilters == nil {
		tm.savedFilters = make(map[string]tmSavedFilter)
	}
}

// saveSavedFilters persists the named filter combo map.
func (tm *TaskManager) saveSavedFilters() {
	if tm.vault == nil {
		return
	}
	dir := filepath.Join(tm.vault.Root, ".granit")
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(tm.savedFilters, "", "  ")
	if err != nil {
		return
	}
	if err := atomicWriteNote(filepath.Join(dir, "tm-saved-filters.json"), string(data)); err != nil {
		tm.lastSaveErr = err
	}
}

// applySavedFilter copies a stored combo onto the live filter
// fields and rebuilds. Used by the * picker on Enter.
func (tm *TaskManager) applySavedFilter(name string) {
	sf, ok := tm.savedFilters[name]
	if !ok {
		return
	}
	tm.filterTag = sf.Tag
	tm.filterPriority = sf.Priority
	tm.filterTriage = tasks.TriageState(sf.Triage)
	tm.searchTerm = sf.Search
	if sf.Sort >= 0 && sf.Sort < 5 {
		tm.sortMode = tmSortMode(sf.Sort)
	}
	tm.rebuildFiltered()
	tm.saveFilters()
	tm.statusMsg = "Applied: " + name
}

// captureCurrentFilter snapshots the live filter fields into
// a tmSavedFilter for the * save action.
func (tm *TaskManager) captureCurrentFilter() tmSavedFilter {
	return tmSavedFilter{
		Tag:      tm.filterTag,
		Priority: tm.filterPriority,
		Triage:   string(tm.filterTriage),
		Search:   tm.searchTerm,
		Sort:     int(tm.sortMode),
	}
}

// SetActiveTimer updates TaskManager's render-only snapshot of
// the model's TimeTracker state. Called from the model right
// before View() so the row renderer can show a "▸ Nm" badge on
// the task currently being tracked. Empty taskText means no
// timer running.
func (tm *TaskManager) SetActiveTimer(taskText string, elapsedSecs int) {
	tm.activeTimerTask = taskText
	tm.activeTimerSecs = elapsedSecs
}

// GetTimerToggleRequest returns the task the user pressed 'y'
// on (and clears the request). Model checks this after each
// Update to start/stop the actual TimeTracker.
func (tm *TaskManager) GetTimerToggleRequest() (Task, bool) {
	if !tm.timerToggleReq {
		return Task{}, false
	}
	tm.timerToggleReq = false
	return tm.timerToggleTask, true
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
	if err := atomicWriteNote(filepath.Join(dir, "pinned-tasks.json"), string(data)); err != nil {
		tm.lastSaveErr = err
	}
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
	if err := atomicWriteNote(filepath.Join(dir, "task-notes.json"), string(data)); err != nil {
		tm.lastSaveErr = err
	}
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

// aiBreakdownTask sends the task to an LLM to generate subtasks.
func (tm *TaskManager) aiBreakdownTask(task Task) tea.Cmd {
	prompt := fmt.Sprintf(
		"Break down this task into 3-7 concrete, actionable subtasks.\n"+
			"Task: %s\n"+
			"Context: project=%s, priority=%d, due=%s\n\n"+
			"Respond with ONLY a list of subtasks, one per line, each starting with '- [ ] '. "+
			"Keep each subtask short and specific. No explanations, no numbering, just the checklist.",
		task.Text, task.Project, task.Priority, task.DueDate,
	)

	ai := tm.ai
	return func() tea.Msg {
		resp, err := ai.Chat("You are a task planning assistant. Be concise.", prompt)
		if err != nil {
			return tmAIResultMsg{err: err}
		}
		return tmAIResultMsg{
			subtasks: parseSubtasks(resp),
			taskPath: task.NotePath,
			taskLine: task.LineNum,
		}
	}
}

type tmAIResultMsg struct {
	subtasks []string
	taskPath string
	taskLine int
	err      error
}

// parseSubtasks extracts "- [ ] ..." lines from AI response.
func parseSubtasks(response string) []string {
	var subtasks []string
	for _, line := range strings.Split(response, "\n") {
		trimmed := strings.TrimSpace(line)
		// Accept lines that look like task items
		if strings.HasPrefix(trimmed, "- [ ] ") {
			subtasks = append(subtasks, strings.TrimSpace(trimmed[6:]))
		} else if strings.HasPrefix(trimmed, "- ") && !strings.HasPrefix(trimmed, "- [") {
			// Also accept "- text" without checkbox
			subtasks = append(subtasks, strings.TrimSpace(trimmed[2:]))
		}
	}
	return subtasks
}

// insertSubtasks writes subtasks as indented items below the parent task line.
func (tm *TaskManager) insertSubtasks(notePath string, parentLine int, subtasks []string) {
	if tm.vault == nil || len(subtasks) == 0 {
		return
	}
	absPath := filepath.Join(tm.vault.Root, notePath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	idx := parentLine - 1
	if idx < 0 || idx >= len(lines) {
		return
	}

	// Determine parent indent
	parentIndent := 0
	for _, ch := range lines[idx] {
		if ch == ' ' {
			parentIndent++
		} else {
			break
		}
	}
	childIndent := strings.Repeat(" ", parentIndent+2)

	// Build subtask lines
	var newLines []string
	for _, st := range subtasks {
		newLines = append(newLines, childIndent+"- [ ] "+st)
	}

	// Insert after parent line
	result := make([]string, 0, len(lines)+len(newLines))
	result = append(result, lines[:idx+1]...)
	result = append(result, newLines...)
	result = append(result, lines[idx+1:]...)

	if err := atomicWriteNote(absPath, strings.Join(result, "\n")); err != nil {
		tm.statusMsg = "subtask insert failed: " + err.Error()
		return
	}
	tm.fileChanged = true
	tm.lastChangedNote = notePath
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
	body := strings.TrimSpace(taskText)
	// View-aware due-date inference. The intent of pressing
	// 'a' on Today is "add a task FOR today" — without this
	// hint, the new task lands without a date, doesn't match
	// the Today filter, and silently disappears from view.
	// Same for Upcoming (defaults to tomorrow). Calendar uses
	// the cursor date. The user can always strip / change
	// the date inline.
	switch tm.view {
	case taskViewToday:
		if !strings.Contains(body, "\U0001F4C5") {
			body += " \U0001F4C5 " + time.Now().Format("2006-01-02")
		}
	case taskViewUpcoming:
		if !strings.Contains(body, "\U0001F4C5") {
			body += " \U0001F4C5 " + time.Now().AddDate(0, 0, 1).Format("2006-01-02")
		}
	case taskViewCalendar:
		if tm.calDay > 0 && !strings.Contains(body, "\U0001F4C5") {
			dateStr := fmt.Sprintf("%04d-%02d-%02d", tm.calYear, int(tm.calMonth), tm.calDay)
			body += " \U0001F4C5 " + dateStr
		}
	}

	targetPath := "Tasks.md"
	if tm.taskStore != nil {
		if _, err := tm.taskStore.Create(body, tasks.CreateOpts{Origin: tasks.OriginManual}); err != nil {
			return
		}
	} else if err := appendTaskLine(tm.vault.Root, "- [ ] "+body); err != nil {
		return
	}
	// Sync the in-memory vault note with what we just wrote so the
	// re-parse below sees the new task.
	note := tm.vault.GetNote(targetPath)
	if note == nil {
		_ = tm.vault.Scan()
		note = tm.vault.GetNote(targetPath)
	}
	if note != nil {
		if data, err := readTasksFile(tm.vault.Root); err == nil {
			note.Content = string(data)
		}
	}

	tm.fileChanged = true
	tm.lastChangedNote = targetPath
	tm.statusMsg = "Task added"

	// Stay on the user's current view — the view-aware due
	// date above guarantees the new task lands where they're
	// looking. Forcing a switch to All view dropped users out
	// of their planning context (the symptom users hit:
	// "I added a task on Today and now I'm on All — where
	// did my Today list go?").
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

// switchView rebuilds the filtered list for a new view. Filters
// are intentionally NOT cleared here — power users rely on
// "filter once, browse multiple views" workflows. The active
// filter chips render in the title bar so the user always sees
// what's narrowing their list.
func (tm *TaskManager) switchView() {
	tm.inputMode = tmInputNone
	tm.inputBuf = ""
	tm.rebuildFiltered()
}

func (tm *TaskManager) rebuildFiltered() {
	// Sticky cursor: remember which task ID the cursor was on
	// before the rebuild so we can restore it after re-filtering.
	// Without this, every refresh (file watcher, save, toggle)
	// resets the cursor to 0 and the user loses their place.
	stickyID := ""
	stickyKey := ""
	if tm.cursor < len(tm.filtered) {
		t := tm.filtered[tm.cursor]
		stickyID = t.ID
		stickyKey = taskKey(t)
	}
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
	case taskViewInbox:
		tm.filtered = tm.filterInbox()
	case taskViewStale:
		tm.filtered = tm.filterStale()
	case taskViewByProject:
		tm.filtered = tm.filterByProject()
	case taskViewQuickWins:
		tm.filtered = tm.filterQuickWins()
	case taskViewByTag:
		tm.filtered = tm.filterByTag()
	case taskViewReview:
		tm.filtered = tm.filterReview()
	}
	if tm.view != taskViewKanban {
		// Apply active tag filter.
		if tm.filterTag != "" {
			tm.filtered = tm.applyTagFilter(tm.filtered)
		}
		// Apply active priority filter.
		if tm.filterPriority >= 0 {
			tm.filtered = tm.applyPriorityFilter(tm.filtered)
		}
		// Apply active triage-state filter (Phase 4 — surfaces
		// the Phase 2 sidecar field "triage" so power users can
		// see only inbox / scheduled / etc.).
		if tm.filterTriage != "" {
			tm.filtered = tm.applyTriageFilter(tm.filtered)
		}
		// Apply persistent text search (set by either search mode or filter mode).
		if tm.searchTerm != "" {
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
	tm.tabCounts = [taskViewCount]int{
		len(tm.applyActiveFilters(tm.filterToday())),
		len(tm.applyActiveFilters(tm.filterUpcoming())),
		len(tm.applyActiveFilters(tm.filterAll())),
		len(tm.applyActiveFilters(tm.filterCompleted())),
		-1, // calendar doesn't show count
		-1, // kanban doesn't show count
		len(tm.applyActiveFilters(tm.filterInbox())),
		len(tm.applyActiveFilters(tm.filterStale())),
		len(tm.applyActiveFilters(tm.filterByProject())),
		len(tm.applyActiveFilters(tm.filterQuickWins())),
		len(tm.applyActiveFilters(tm.filterByTag())),
		len(tm.applyActiveFilters(tm.filterReview())),
	}

	// Sticky cursor restore — match by stable ID first, fall
	// back to NotePath:LineNum key. If the task left the
	// current view (e.g. completed and we're on Today),
	// cursor stays at 0.
	if stickyID != "" || stickyKey != "" {
		for i, t := range tm.filtered {
			if (stickyID != "" && t.ID == stickyID) || (stickyKey != "" && taskKey(t) == stickyKey) {
				tm.cursor = i
				visH := tm.visibleHeight()
				if visH > 0 && tm.cursor >= visH {
					tm.scroll = tm.cursor - visH/2
				}
				break
			}
		}
	}
}

func (tm *TaskManager) filterToday() []Task {
	var overdue []Task
	var morning, midday, afternoon, evening []Task // scheduled by time block
	var unscheduledToday, tomorrow []Task

	for _, t := range tm.allTasks {
		if tm.shouldHideFromActive(t) {
			continue
		}
		switch timeBlockGroup(t) {
		case "morning":
			morning = append(morning, t)
		case "midday":
			midday = append(midday, t)
		case "afternoon":
			afternoon = append(afternoon, t)
		case "evening":
			evening = append(evening, t)
		case "overdue":
			overdue = append(overdue, t)
		case "today":
			unscheduledToday = append(unscheduledToday, t)
		case "tomorrow":
			tomorrow = append(tomorrow, t)
		}
	}

	byPriority := func(s []Task) {
		sort.Slice(s, func(i, j int) bool { return s[i].Priority > s[j].Priority })
	}
	// Sort by schedule time within blocks
	bySchedule := func(s []Task) {
		sort.Slice(s, func(i, j int) bool { return s[i].ScheduledTime < s[j].ScheduledTime })
	}
	byPriority(overdue)
	bySchedule(morning)
	bySchedule(midday)
	bySchedule(afternoon)
	bySchedule(evening)
	byPriority(unscheduledToday)
	byPriority(tomorrow)

	// Order: overdue → morning → midday → afternoon → evening → unscheduled today → tomorrow
	var out []Task
	out = append(out, overdue...)
	out = append(out, morning...)
	out = append(out, midday...)
	out = append(out, afternoon...)
	out = append(out, evening...)
	out = append(out, unscheduledToday...)
	out = append(out, tomorrow...)
	return out
}

func (tm *TaskManager) filterUpcoming() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if tm.shouldHideFromActive(t) {
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
		if tm.shouldHideFromActive(t) {
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

// filterInbox returns untriaged tasks (Triage == TriageInbox or
// empty). The inbox is the front-door queue for the
// Capture-Triage-Schedule loop — power users live here when
// processing fresh captures.
func (tm *TaskManager) filterInbox() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if tm.shouldHideFromActive(t) {
			continue
		}
		if t.Triage == "" || t.Triage == tasks.TriageInbox {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// Newest captures first — fresh inbox items deserve
		// the user's attention before older ones go stale.
		if !out[i].CreatedAt.IsZero() && !out[j].CreatedAt.IsZero() {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].Priority > out[j].Priority
	})
	return out
}

// filterStale returns active tasks that haven't been touched in
// 7+ days (no completion, no triage update, no creation in
// the last week). Surfaces forgotten work without forcing the
// user to scroll All view.
func (tm *TaskManager) filterStale() []Task {
	cutoff := time.Now().AddDate(0, 0, -7)
	var out []Task
	for _, t := range tm.allTasks {
		if tm.shouldHideFromActive(t) {
			continue
		}
		// "Stale" criterion: created > 7d ago AND not
		// recently triaged. Tasks with no CreatedAt fall
		// through (legacy data) — show them too so the user
		// can prune.
		fresh := false
		if !t.CreatedAt.IsZero() && t.CreatedAt.After(cutoff) {
			fresh = true
		}
		if t.LastTriagedAt != nil && t.LastTriagedAt.After(cutoff) {
			fresh = true
		}
		if !fresh {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		// Oldest first — those need the most attention.
		if !out[i].CreatedAt.IsZero() && !out[j].CreatedAt.IsZero() {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].NotePath < out[j].NotePath
	})
	return out
}

// filterQuickWins returns active tasks with high priority
// (≥ medium = 2) and a small time estimate (≤30min OR no
// estimate at all). Power-user "I have 30 minutes, what can
// I crush?" view. Sort: priority desc, then estimate asc.
func (tm *TaskManager) filterQuickWins() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if tm.shouldHideFromActive(t) {
			continue
		}
		if t.Priority < 2 {
			continue
		}
		if t.EstimatedMinutes > 30 {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		// Smaller estimate first within priority — fastest win.
		ai, aj := out[i].EstimatedMinutes, out[j].EstimatedMinutes
		if ai == 0 {
			ai = 999
		}
		if aj == 0 {
			aj = 999
		}
		return ai < aj
	})
	return out
}

// filterByTag groups active tasks by their first tag.
// Tasks without tags land under "(no tag)" at the end.
// Order: tag-name asc, priority desc within.
func (tm *TaskManager) filterByTag() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if tm.shouldHideFromActive(t) {
			continue
		}
		out = append(out, t)
	}
	firstTag := func(t Task) string {
		if len(t.Tags) > 0 {
			return strings.ToLower(t.Tags[0])
		}
		return "~~no-tag" // sorts last (~ > ascii letters)
	}
	sort.Slice(out, func(i, j int) bool {
		ti, tj := firstTag(out[i]), firstTag(out[j])
		if ti != tj {
			return ti < tj
		}
		return out[i].Priority > out[j].Priority
	})
	return out
}

// filterReview returns tasks completed in the last 7 days.
// Sorted by completion date (newest first) so the most
// recent wins lead the list — useful for weekly retros.
func (tm *TaskManager) filterReview() []Task {
	cutoff := time.Now().AddDate(0, 0, -7)
	var out []Task
	for _, t := range tm.allTasks {
		if !t.Done {
			continue
		}
		if t.CompletedAt != nil && t.CompletedAt.After(cutoff) {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CompletedAt != nil && out[j].CompletedAt != nil {
			return out[i].CompletedAt.After(*out[j].CompletedAt)
		}
		return out[i].Priority > out[j].Priority
	})
	return out
}

// filterByProject groups active tasks by Project field.
// Tasks without a project fall under "(no project)" at the
// end. Order: project-name asc, then priority desc within.
func (tm *TaskManager) filterByProject() []Task {
	var out []Task
	for _, t := range tm.allTasks {
		if tm.shouldHideFromActive(t) {
			continue
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		// Empty project sorts last so it doesn't lead the
		// group view — projects are the headline.
		if (out[i].Project == "") != (out[j].Project == "") {
			return out[i].Project != ""
		}
		if out[i].Project != out[j].Project {
			return strings.ToLower(out[i].Project) < strings.ToLower(out[j].Project)
		}
		return out[i].Priority > out[j].Priority
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
		if tm.shouldHideFromActive(t) {
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
	query := strings.ToLower(strings.TrimSpace(tm.searchTerm))
	if query == "" {
		return tasks
	}

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
				if fuzzyMatch(strings.ToLower(tag), tagQuery) {
					out = append(out, t)
					break
				}
			}
		}
		return out
	}

	var out []Task
	for _, t := range tasks {
		hay := strings.ToLower(t.Text + " " + t.NotePath)
		if fuzzyMatch(hay, query) {
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

// tmIsHiddenFromActive returns true when a task should NOT appear in the
// active views (Plan, Upcoming, All, Inbox, Stale, Quick Wins, By Tag,
// By Project, Calendar, Kanban). Three states qualify: completed (Done),
// snoozed (SnoozedUntil in the future), or dropped (TriageDropped).
//
// Centralising this check fixes a long-standing leak where 'm d' /
// ';'-cycle to Dropped did not actually remove the task from the user's
// daily view — every filter function checked Done+Snoozed but missed
// Dropped, so triaging away a task did nothing visible. The Done view
// (filterCompleted) and Review view (filterReview) intentionally do
// NOT use this helper because they specifically want completed tasks.
//
// Free function form retained for callers that don't have the TaskManager
// in scope; methods that DO should prefer (*TaskManager).shouldHideFromActive
// because it also respects an active triage filter (so the user can use
// F → triage:dropped to see and recover dropped tasks).
func tmIsHiddenFromActive(task Task) bool {
	return task.Done || tmIsSnoozed(task) || task.Triage == tasks.TriageDropped
}

// shouldHideFromActive is the filter-function entry point for "should this
// task be excluded from the current view?". Same semantics as
// tmIsHiddenFromActive *except* when the user has set an explicit triage
// filter — in that case the base view must NOT pre-hide the very state
// the user asked to see, otherwise applyTriageFilter runs against an
// empty pool and the user can't reach their dropped/snoozed tasks. Without
// this opt-out, dropped tasks were unrecoverable through the UI: hidden
// from every active view AND filtered out before the triage filter could
// match them.
func (tm *TaskManager) shouldHideFromActive(task Task) bool {
	switch tm.filterTriage {
	case tasks.TriageDropped:
		return task.Done || tmIsSnoozed(task) // show dropped, hide other normally-hidden states
	case tasks.TriageSnoozed:
		return task.Done || task.Triage == tasks.TriageDropped // show snoozed
	}
	return tmIsHiddenFromActive(task)
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
	if tm.filterProject != "" {
		tasks = tm.applyProjectFilter(tasks)
	}
	return tasks
}

// applyProjectFilter narrows the list to tasks whose Project field
// matches tm.filterProject (case-insensitive). Powers the 'P' shortcut
// — "show only tasks for the project the cursor is on."
func (tm *TaskManager) applyProjectFilter(taskList []Task) []Task {
	target := strings.ToLower(strings.TrimSpace(tm.filterProject))
	var out []Task
	for _, t := range taskList {
		if strings.EqualFold(strings.TrimSpace(t.Project), target) {
			out = append(out, t)
		}
	}
	return out
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

// applyTriageFilter filters tasks by Triage state. Tasks with no
// explicit triage state (empty string) are treated as TriageInbox
// — that's the migration default for tasks predating Phase 2.
func (tm *TaskManager) applyTriageFilter(taskList []Task) []Task {
	var out []Task
	want := tm.filterTriage
	for _, t := range taskList {
		state := t.Triage
		if state == "" {
			state = tasks.TriageInbox
		}
		if state == want {
			out = append(out, t)
		}
	}
	return out
}

// inlineEditSeed returns the initial buffer contents for inline
// edit mode — the task's stored Text field, which is everything
// after the "] " prefix as captured by the parser. Includes
// tags + emoji markers so the user can edit them in place.
func inlineEditSeed(t Task) string {
	return strings.TrimSpace(t.Text)
}

// setTriageState writes a new triage state for the given task,
// persists it via the store (when wired), and patches the
// in-memory caches so the next render shows the new chip
// immediately. Used by both cycleTriageState (';' cycle) and
// the direct-set submode ('m' leader). Updates statusMsg as a
// power-user feedback signal.
func (tm *TaskManager) setTriageState(task Task, next tasks.TriageState) {
	if tm.taskStore != nil && task.ID != "" {
		if err := tm.taskStore.Triage(task.ID, next); err != nil {
			tm.statusMsg = "Triage failed: " + err.Error()
			return
		}
	}
	for i := range tm.allTasks {
		if tm.allTasks[i].ID == task.ID || (tm.allTasks[i].NotePath == task.NotePath && tm.allTasks[i].LineNum == task.LineNum) {
			tm.allTasks[i].Triage = next
		}
	}
	for i := range tm.filtered {
		if tm.filtered[i].ID == task.ID || (tm.filtered[i].NotePath == task.NotePath && tm.filtered[i].LineNum == task.LineNum) {
			tm.filtered[i].Triage = next
		}
	}
	tm.statusMsg = "Triage → " + string(next)
}

// cycleTriageState rotates the cursor task through the
// (inbox → triaged → scheduled → snoozed → dropped → inbox)
// cycle. Persists via the task store when wired; falls back to
// in-memory mutation when the store isn't available so the
// off-flag path doesn't crash.
func (tm *TaskManager) cycleTriageState(task Task) {
	cycle := []tasks.TriageState{
		tasks.TriageInbox,
		tasks.TriageTriaged,
		tasks.TriageScheduled,
		tasks.TriageSnoozed,
		tasks.TriageDropped,
	}
	current := task.Triage
	if current == "" {
		current = tasks.TriageInbox
	}
	next := tasks.TriageInbox
	for i, s := range cycle {
		if s == current {
			next = cycle[(i+1)%len(cycle)]
			break
		}
	}
	tm.setTriageState(task, next)
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
		// Hide snoozed and dropped from the kanban; Done still appears
		// in the rightmost "Done" column because that column is the
		// completion bucket. Dropped tasks have no column — they should
		// not visually clutter the board. Honors the active triage
		// filter the same way active views do, so 'F triage:dropped'
		// still shows dropped tasks (they cluster into Backlog since
		// dropped + no due date = Backlog by the switch below).
		if tm.shouldHideFromActive(t) && !t.Done {
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
	case tmAIResultMsg:
		tm.aiPending = false
		if msg.err != nil {
			tm.statusMsg = "AI error: " + msg.err.Error()
		} else if len(msg.subtasks) > 0 {
			tm.insertSubtasks(msg.taskPath, msg.taskLine, msg.subtasks)
			tm.reparse()
			tm.statusMsg = fmt.Sprintf("Added %d subtasks", len(msg.subtasks))
		} else {
			tm.statusMsg = "AI returned no subtasks"
		}
		return tm, nil

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
	case tmInputInlineEdit:
		return tm.updateInlineEdit(key)
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
	case tmInputFilter:
		return tm.updateFilterInput(key)
	case tmInputQuickEdit:
		return tm.updateQuickEditInput(key)
	case tmInputTimeBlock:
		return tm.updateTimeBlock(key)
	case tmInputTriageSet:
		return tm.updateTriageSet(key)
	case tmInputSavedFilter:
		return tm.updateSavedFilter(key)
	}

	return tm, nil
}

// updateSavedFilter handles the '*' input mode where the user
// either applies an existing saved filter or saves the current
// combo under a new name. Behavior:
//
//	Enter on a name that exists  → apply that saved combo
//	Enter on a new name          → save current combo with that name
//	Backspace                    → edit
//	Esc                          → cancel
//	Ctrl+D                       → delete the typed name (if exists)
//
// The render layer shows the list of saved names so the user
// can scan instead of remembering — matches palette ergonomics.
func (tm TaskManager) updateSavedFilter(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil
	case "enter":
		name := strings.TrimSpace(tm.inputBuf)
		if name == "" {
			tm.inputMode = tmInputNone
			return tm, nil
		}
		if _, exists := tm.savedFilters[name]; exists {
			tm.applySavedFilter(name)
		} else {
			if tm.savedFilters == nil {
				tm.savedFilters = make(map[string]tmSavedFilter)
			}
			tm.savedFilters[name] = tm.captureCurrentFilter()
			tm.saveSavedFilters()
			tm.statusMsg = "Saved filter: " + name
		}
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil
	case "ctrl+d":
		// Delete the typed name (if it exists) — power-user
		// "* foo Ctrl+D" workflow for cleanup.
		name := strings.TrimSpace(tm.inputBuf)
		if _, exists := tm.savedFilters[name]; exists {
			delete(tm.savedFilters, name)
			tm.saveSavedFilters()
			tm.statusMsg = "Deleted filter: " + name
		}
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil
	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = TrimLastRune(tm.inputBuf)
		}
		return tm, nil
	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
		}
		return tm, nil
	}
}

// updateTriageSet handles the direct-set triage submode entered
// via 'm' (mark) in normal mode. One keystroke maps to one
// triage state — no cycling, no picker overlay. The submode
// exits after a single key (success or unknown) so the user is
// back in normal mode immediately. Esc cancels without writing.
//
//	i  → inbox
//	t  → triaged
//	s  → scheduled
//	n  → snoozed
//	d  → dropped
//	x  → done
func (tm TaskManager) updateTriageSet(key string) (TaskManager, tea.Cmd) {
	if key == "esc" {
		tm.inputMode = tmInputNone
		tm.statusMsg = ""
		return tm, nil
	}
	if tm.cursor >= len(tm.filtered) {
		tm.inputMode = tmInputNone
		return tm, nil
	}
	task := tm.filtered[tm.cursor]
	var next tasks.TriageState
	switch key {
	case "i":
		next = tasks.TriageInbox
	case "t":
		next = tasks.TriageTriaged
	case "s":
		next = tasks.TriageScheduled
	case "n":
		next = tasks.TriageSnoozed
	case "d":
		next = tasks.TriageDropped
	case "x":
		next = tasks.TriageDone
	default:
		// Unknown key — exit submode without action so the user
		// isn't stuck if they hit 'm' by accident.
		tm.inputMode = tmInputNone
		tm.statusMsg = "Triage cancelled"
		return tm, nil
	}
	tm.setTriageState(task, next)
	tm.inputMode = tmInputNone
	return tm, nil
}

// updateTimeBlock handles the time-block assignment picker.
// 1=Morning(06:00-10:00), 2=Midday(10:00-14:00), 3=Afternoon(14:00-18:00), 4=Evening(18:00-22:00)
func (tm TaskManager) updateTimeBlock(key string) (TaskManager, tea.Cmd) {
	type blockDef struct {
		start    string
		end      string
		label    string
		startMin int
		endMin   int
	}
	blocks := map[string]blockDef{
		"1": {"06:00", "10:00", "Morning", 360, 600},
		"2": {"10:00", "14:00", "Midday", 600, 840},
		"3": {"14:00", "18:00", "Afternoon", 840, 1080},
		"4": {"18:00", "22:00", "Evening", 1080, 1320},
		"0": {"", "", "Clear", 0, 0},
	}

	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		return tm, nil
	case "1", "2", "3", "4", "0":
		if tm.inputTask == nil {
			tm.inputMode = tmInputNone
			return tm, nil
		}
		blk := blocks[key]
		task := *tm.inputTask
		// Done tasks can't be scheduled — their block would orphan in the
		// Plan view (filtered out) but still appear on the calendar.
		if blk.start != "" && task.Done {
			tm.inputMode = tmInputNone
			tm.statusMsg = "Task already done — can't schedule a completed task"
			return tm, nil
		}
		if blk.start == "" {
			// Clear the schedule
			tm.removeScheduleMarker(task)
			// Update in-memory task immediately
			for i := range tm.allTasks {
				if tm.allTasks[i].NotePath == task.NotePath && tm.allTasks[i].LineNum == task.LineNum {
					tm.allTasks[i].ScheduledTime = ""
					break
				}
			}
			tm.statusMsg = "Time block cleared"
		} else {
			est := task.EstimatedMinutes
			if est <= 0 {
				est = 60 // default 1h
			}
			// Find the next free slot within this block by scanning existing tasks
			slotStart := tm.findFreeSlot(blk.startMin, blk.endMin, est, task)
			slotEnd := slotStart + est
			if slotEnd > blk.endMin {
				slotEnd = blk.endMin
			}
			startStr := fmtTimeSlot(slotStart)
			endStr := fmtTimeSlot(slotEnd)
			schedStr := startStr + "-" + endStr
			tm.assignSchedule(task, startStr, endStr)
			// Update in-memory task immediately so UI reflects the change
			for i := range tm.allTasks {
				if tm.allTasks[i].NotePath == task.NotePath && tm.allTasks[i].LineNum == task.LineNum {
					tm.allTasks[i].ScheduledTime = schedStr
					break
				}
			}
			tm.statusMsg = fmt.Sprintf("Scheduled for %s (%s–%s)", blk.label, startStr, endStr)
		}
		tm.inputMode = tmInputNone
		tm.rebuildFiltered() // rebuild with updated in-memory data
		return tm, nil
	}
	return tm, nil
}

// findFreeSlot scans existing scheduled tasks to find the earliest free
// slot of the given duration within [blockStart, blockEnd) minutes.
// Excludes the task being scheduled (identified by NotePath+LineNum).
func (tm *TaskManager) findFreeSlot(blockStart, blockEnd, duration int, exclude Task) int {
	// Collect occupied intervals in this block
	type interval struct{ start, end int }
	var occupied []interval
	for _, t := range tm.allTasks {
		if t.Done || t.ScheduledTime == "" {
			continue
		}
		if t.NotePath == exclude.NotePath && t.LineNum == exclude.LineNum {
			continue
		}
		parts := strings.SplitN(t.ScheduledTime, "-", 2)
		if len(parts) != 2 {
			continue
		}
		sh, sm := parseHHMM(parts[0])
		eh, em := parseHHMM(parts[1])
		s := sh*60 + sm
		e := eh*60 + em
		// Only consider intervals overlapping this block
		if e > blockStart && s < blockEnd {
			occupied = append(occupied, interval{s, e})
		}
	}
	// Sort by start time
	sort.Slice(occupied, func(i, j int) bool { return occupied[i].start < occupied[j].start })

	// Walk forward from blockStart looking for a gap of at least `duration`
	cursor := blockStart
	for _, iv := range occupied {
		if cursor+duration <= iv.start {
			return cursor // found a gap before this interval
		}
		if iv.end > cursor {
			cursor = iv.end
		}
	}
	// Check if remaining space after all intervals fits
	if cursor+duration <= blockEnd {
		return cursor
	}
	// Block is full — place at blockStart anyway (overlap, but user chose it)
	return blockStart
}

// assignSchedule sets the ⏰ marker on the task's source line AND mirrors
// the time block into today's Planner/{date}.md so the Calendar view sees
// it. The source-side write goes through writeLineChange to keep the
// vault cache and undo stack consistent; the planner-side write goes
// through UpsertPlannerBlock (idempotent — re-scheduling replaces,
// doesn't duplicate).
//
// Surfaces failures via statusMsg. Previously both writes swallowed
// errors silently — if the planner mirror failed (disk full, permission
// denied), the source line got the ⏰ marker but the calendar view
// never showed the block, leaving the schedule visibly inconsistent
// without explanation.
func (tm *TaskManager) assignSchedule(task Task, startTime, endTime string) {
	marker := " ⏰ " + startTime + "-" + endTime
	if !tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		cleaned := tmScheduleRe.ReplaceAllString(line, "")
		return strings.TrimRight(cleaned, " ") + marker
	}) {
		tm.statusMsg = "Schedule: failed to update source line"
		return
	}
	ref := ScheduleRef{NotePath: task.NotePath, LineNum: task.LineNum, Text: task.Text}
	today := time.Now().Format("2006-01-02")
	if err := UpsertPlannerBlock(tm.vault.Root, today, ref, PlannerBlock{
		Date: today, StartTime: startTime, EndTime: endTime,
		Text: task.Text, BlockType: "task", SourceRef: ref,
	}); err != nil {
		tm.statusMsg = "Schedule: source updated, planner write failed: " + err.Error()
	}
}

// removeScheduleMarker clears the ⏰ marker from the task's source line AND
// removes the mirrored planner block from today's Planner file. Errors on
// either side are surfaced via statusMsg so a half-applied unschedule
// doesn't go unnoticed.
func (tm *TaskManager) removeScheduleMarker(task Task) {
	if !tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		return tmScheduleRe.ReplaceAllString(line, "")
	}) {
		tm.statusMsg = "Unschedule: failed to update source line"
		return
	}
	ref := ScheduleRef{NotePath: task.NotePath, LineNum: task.LineNum, Text: task.Text}
	today := time.Now().Format("2006-01-02")
	if err := RemovePlannerBlock(tm.vault.Root, today, ref); err != nil {
		tm.statusMsg = "Unschedule: source updated, planner write failed: " + err.Error()
	}
}

// updateInlineEdit handles the i-key inline edit mode. The user
// edits the full task line (sans "- [ ] " prefix) in a single-
// line buffer; Enter commits the change to the source markdown
// preserving the prefix and leading indent. Esc cancels.
//
// Edits to the full line (not just text body) so power users can
// adjust dates/priorities/tags inline without three separate
// modes. Re-parses after commit so the row updates.
func (tm TaskManager) updateInlineEdit(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil
	case "enter":
		tm.applyInlineEdit()
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil
	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = TrimLastRune(tm.inputBuf)
		}
		return tm, nil
	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
		}
		return tm, nil
	}
}

// applyInlineEdit writes the edited line back to the source
// note. Preserves leading whitespace + bullet + checkbox; the
// rest of the line is replaced with the user's input. Skips
// silently when the buffer is empty (treat as cancel — better
// than wiping the task accidentally).
//
// Defensive: strips a leading "- [ ] " (or [x] / [X]) from the
// buffer before appending so a user who pastes-or-retypes a
// full task line doesn't get double-prefixed
// ("- [ ] - [ ] foo"). Newlines are also rejected to keep the
// single-line invariant — pasting a multiline buffer would
// otherwise corrupt adjacent rows.
func (tm *TaskManager) applyInlineEdit() {
	if tm.cursor < 0 || tm.cursor >= len(tm.filtered) {
		return
	}
	body := strings.TrimSpace(tm.inputBuf)
	if body == "" {
		tm.statusMsg = "Edit cancelled (empty)"
		return
	}
	if strings.ContainsAny(body, "\n\r") {
		tm.statusMsg = "Edit rejected — line breaks not allowed"
		return
	}
	body = stripCheckboxPrefix(body)
	if body == "" {
		tm.statusMsg = "Edit cancelled (only checkbox)"
		return
	}
	task := tm.filtered[tm.cursor]
	ok := tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		// Preserve everything up to and including the "] " that
		// closes the checkbox (so leading indent + bullet +
		// checkbox state survive); replace the tail with the
		// user's body.
		idx := strings.Index(line, "] ")
		if idx < 0 {
			// Line stopped looking like a task between the user
			// pressing i and Enter. Bail out — preserves the
			// original line untouched.
			return line
		}
		return line[:idx+2] + body
	})
	if ok {
		tm.statusMsg = "Edited"
		tm.reparse()
	}
}

// stripCheckboxPrefix removes a leading "- [ ] " (or "- [x] " /
// "- [X] ") plus any leading whitespace from a task body. Used
// by inline edit to defang accidental prefix double-up — a user
// who types or pastes the full markdown line should still end
// up with just the body part.
func stripCheckboxPrefix(s string) string {
	trimmed := strings.TrimLeft(s, " \t")
	if len(trimmed) >= 6 &&
		(trimmed[:6] == "- [ ] " || trimmed[:6] == "- [x] " || trimmed[:6] == "- [X] ") {
		return strings.TrimSpace(trimmed[6:])
	}
	return s
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
			tm.inputBuf = TrimLastRune(tm.inputBuf)
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
		// Esc cancels the search and clears it.
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		tm.searchTerm = ""
		tm.rebuildFiltered()
		tm.saveFilters()
		return tm, nil

	case "enter":
		// Enter commits the search and exits the input — search stays visible
		// as a badge in the title bar and can be cleared with `c`.
		tm.inputMode = tmInputNone
		tm.saveFilters()
		return tm, nil

	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = TrimLastRune(tm.inputBuf)
			tm.searchTerm = tm.inputBuf
			tm.rebuildFiltered()
		}
		return tm, nil

	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
			tm.searchTerm = tm.inputBuf
			tm.rebuildFiltered()
		}
		return tm, nil
	}
}

// updateFilterInput collects a unified filter query. On Enter it parses the
// buffer into tag / priority / sort / search filters and applies them.
// Query grammar (whitespace-separated tokens, any order):
//
//	#tag          → set tag filter
//	p:<level>     → set priority filter (none|low|med|high|highest or 0-4)
//	sort:<mode>   → set sort (priority|due|az|source|tag)
//	-             → anything else becomes search text (joined with spaces)
func (tm TaskManager) updateFilterInput(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil

	case "enter":
		tm.applyFilterQuery(tm.inputBuf)
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil

	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = TrimLastRune(tm.inputBuf)
		}
		return tm, nil

	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
		}
		return tm, nil
	}
}

// applyFilterQuery parses a unified filter string and sets the resulting
// filters on tm. An empty query clears all filters.
func (tm *TaskManager) applyFilterQuery(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		tm.filterTag = ""
		tm.filterPriority = -1
		tm.sortMode = tmSortPriority
		tm.statusMsg = "Filters cleared"
		tm.rebuildFiltered()
		return
	}

	tm.filterTag = ""
	tm.filterPriority = -1
	tm.sortMode = tmSortPriority
	var searchTerms []string
	var applied []string

	priorityLevels := map[string]int{
		"none": 0, "low": 1, "med": 2, "medium": 2, "high": 3, "highest": 4,
		"0": 0, "1": 1, "2": 2, "3": 3, "4": 4,
	}
	sortModes := map[string]tmSortMode{
		"priority": tmSortPriority,
		"due":      tmSortDueDate,
		"duedate":  tmSortDueDate,
		"az":       tmSortAlpha,
		"alpha":    tmSortAlpha,
		"source":   tmSortSource,
		"tag":      tmSortTag,
	}

	triageStates := map[string]tasks.TriageState{
		"inbox":     tasks.TriageInbox,
		"triaged":   tasks.TriageTriaged,
		"scheduled": tasks.TriageScheduled,
		"done":      tasks.TriageDone,
		"dropped":   tasks.TriageDropped,
		"snoozed":   tasks.TriageSnoozed,
	}

	tm.filterTriage = ""
	for _, tok := range strings.Fields(query) {
		switch {
		case strings.HasPrefix(tok, "#") && len(tok) > 1:
			tm.filterTag = tok[1:]
			applied = append(applied, "tag="+tm.filterTag)
		case strings.HasPrefix(tok, "p:"):
			if lvl, ok := priorityLevels[strings.ToLower(tok[2:])]; ok {
				tm.filterPriority = lvl
				applied = append(applied, "p="+tok[2:])
			}
		case strings.HasPrefix(tok, "triage:"):
			if state, ok := triageStates[strings.ToLower(tok[7:])]; ok {
				tm.filterTriage = state
				applied = append(applied, "triage="+tok[7:])
			}
		case strings.HasPrefix(tok, "sort:"):
			if mode, ok := sortModes[strings.ToLower(tok[5:])]; ok {
				tm.sortMode = mode
				applied = append(applied, "sort="+tok[5:])
			}
		default:
			searchTerms = append(searchTerms, tok)
		}
	}

	if len(searchTerms) > 0 {
		tm.searchTerm = strings.Join(searchTerms, " ")
		applied = append(applied, "search="+tm.searchTerm)
	} else {
		tm.searchTerm = ""
	}

	tm.rebuildFiltered()
	tm.saveFilters()
	if len(applied) > 0 {
		tm.statusMsg = "Applied: " + strings.Join(applied, " ")
	} else {
		tm.statusMsg = "No filters applied (bad query)"
	}
}

// updateQuickEditInput collects a compact one-line edit query and applies it
// to the cursor task on Enter. See applyQuickEdit for the grammar.
func (tm TaskManager) updateQuickEditInput(key string) (TaskManager, tea.Cmd) {
	switch key {
	case "esc":
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil

	case "enter":
		tm.applyQuickEdit(tm.inputBuf)
		tm.inputMode = tmInputNone
		tm.inputBuf = ""
		return tm, nil

	case "backspace":
		if len(tm.inputBuf) > 0 {
			tm.inputBuf = TrimLastRune(tm.inputBuf)
		}
		return tm, nil

	default:
		if len(key) == 1 || key == " " {
			tm.inputBuf += key
		}
		return tm, nil
	}
}

// applyQuickEdit parses a compact edit expression and applies it to the cursor
// task in-place. Grammar (whitespace-separated tokens):
//
//	p:<N|name>    priority 0-4 or none/low/med/high/highest
//	d:<date>      due date: YYYY-MM-DD, today, tomorrow, +Nd
//	~<dur>        time estimate: 30m, 2h, 1h30m
//
// Unknown tokens are ignored. Sets statusMsg so the user sees what landed.
func (tm *TaskManager) applyQuickEdit(query string) {
	query = strings.TrimSpace(query)
	if query == "" || tm.cursor >= len(tm.filtered) {
		return
	}
	task := tm.filtered[tm.cursor]

	priorityLevels := map[string]int{
		"none": 0, "low": 1, "med": 2, "medium": 2, "high": 3, "highest": 4,
		"0": 0, "1": 1, "2": 2, "3": 3, "4": 4,
	}

	var applied []string
	var targetPriority = -1
	var targetDueDate string
	var targetEstimateMins int

	for _, tok := range strings.Fields(query) {
		switch {
		case strings.HasPrefix(tok, "p:"):
			if lvl, ok := priorityLevels[strings.ToLower(tok[2:])]; ok {
				targetPriority = lvl
				applied = append(applied, "p="+tok[2:])
			}
		case strings.HasPrefix(tok, "d:"):
			if d := parseQuickEditDate(tok[2:]); d != "" {
				targetDueDate = d
				applied = append(applied, "d="+d)
			}
		case strings.HasPrefix(tok, "~"):
			if m := parseEstimateSpec(tok[1:]); m > 0 {
				targetEstimateMins = m
				applied = append(applied, fmt.Sprintf("~%dm", m))
			}
		}
	}

	if len(applied) == 0 {
		tm.statusMsg = "Nothing to apply (try: p:high d:tomorrow ~30m)"
		return
	}

	ok := tm.writeLineChange(task.NotePath, task.LineNum, func(line string) string {
		if targetPriority >= 0 {
			line = tmPrioHighestRe.ReplaceAllString(line, "")
			line = tmPrioHighRe.ReplaceAllString(line, "")
			line = tmPrioMedRe.ReplaceAllString(line, "")
			line = tmPrioLowRe.ReplaceAllString(line, "")
			line = strings.TrimRight(line, " ")
			if targetPriority > 0 {
				line += " " + tmPriorityIcon(targetPriority)
			}
		}
		if targetDueDate != "" {
			line = tmDueDateRe.ReplaceAllString(line, "")
			line = strings.TrimRight(line, " ")
			line += " \U0001F4C5 " + targetDueDate
		}
		if targetEstimateMins > 0 {
			line = tmEstimateRe.ReplaceAllString(line, "")
			line = strings.TrimRight(line, " ")
			var label string
			switch {
			case targetEstimateMins < 60:
				label = fmt.Sprintf("~%dm", targetEstimateMins)
			case targetEstimateMins%60 == 0:
				label = fmt.Sprintf("~%dh", targetEstimateMins/60)
			default:
				label = fmt.Sprintf("~%dh%dm", targetEstimateMins/60, targetEstimateMins%60)
			}
			line += " " + label
		}
		return line
	})
	if ok {
		tm.reparse()
		tm.statusMsg = "Updated: " + strings.Join(applied, " ")
	} else {
		tm.statusMsg = "Failed to write task"
	}
}

// parseQuickEditDate accepts shorthand date specs and returns a YYYY-MM-DD
// string, or "" when the input doesn't parse.
func parseQuickEditDate(spec string) string {
	spec = strings.TrimSpace(strings.ToLower(spec))
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	switch spec {
	case "today":
		return today.Format("2006-01-02")
	case "tomorrow":
		return today.AddDate(0, 0, 1).Format("2006-01-02")
	}
	// Absolute YYYY-MM-DD
	if t, err := time.Parse("2006-01-02", spec); err == nil {
		return t.Format("2006-01-02")
	}
	// +Nd shorthand
	if strings.HasPrefix(spec, "+") && strings.HasSuffix(spec, "d") {
		n, err := strconv.Atoi(spec[1 : len(spec)-1])
		if err == nil && n > 0 {
			return today.AddDate(0, 0, n).Format("2006-01-02")
		}
	}
	return ""
}

// parseEstimateSpec parses "30m", "2h", "1h30m" → total minutes. Returns 0
// when the input doesn't match.
var quickEditEstRe = regexp.MustCompile(`^(?:(\d+)h)?(?:(\d+)m)?$`)

func parseEstimateSpec(spec string) int {
	m := quickEditEstRe.FindStringSubmatch(strings.TrimSpace(spec))
	if m == nil {
		return 0
	}
	var h, mins int
	if m[1] != "" {
		h, _ = strconv.Atoi(m[1])
	}
	if m[2] != "" {
		mins, _ = strconv.Atoi(m[2])
	}
	return h*60 + mins
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
			tm.inputBuf = TrimLastRune(tm.inputBuf)
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
			tm.inputBuf = TrimLastRune(tm.inputBuf)
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
			tm.inputBuf = TrimLastRune(tm.inputBuf)
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
		tm.view = taskViewKanban
		tm.switchView()
	case "7":
		tm.view = taskViewInbox
		tm.switchView()
	case "8":
		tm.view = taskViewStale
		tm.switchView()
	case "9":
		tm.view = taskViewByProject
		tm.switchView()
	case "0":
		// 10th view — fits into the digit row by overflowing
		// to "0". Quick Wins is the most asked-for "I have
		// 30 min" view so it earns the digit slot.
		tm.view = taskViewQuickWins
		tm.switchView()
	case "tab":
		tm.view = (tm.view + 1) % taskViewCount
		tm.switchView()

	case "h", "left":
		if tm.kanbanCol > 0 {
			tm.kanbanCol--
			// Clamp cursor to new column's size.
			if n := len(cols[tm.kanbanCol]); n > 0 && tm.kanbanCursor[tm.kanbanCol] >= n {
				tm.kanbanCursor[tm.kanbanCol] = n - 1
			}
		}
	case "l", "right":
		if tm.kanbanCol < 3 {
			tm.kanbanCol++
			if n := len(cols[tm.kanbanCol]); n > 0 && tm.kanbanCursor[tm.kanbanCol] >= n {
				tm.kanbanCursor[tm.kanbanCol] = n - 1
			}
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

	// Handle pending confirmation. Only y/Y fires the action; any other
	// key dismisses the prompt with a "Cancelled" status so the user
	// gets visible feedback that their non-yes keystroke was registered
	// (otherwise an accidental Enter looks identical to having taken
	// no action and the prompt just vanishes).
	if tm.confirmMsg != "" {
		switch key {
		case "y", "Y":
			if tm.confirmAction != nil {
				tm.confirmAction()
			}
		default:
			tm.statusMsg = "Cancelled"
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

	case "tab":
		tm.view = (tm.view + 1) % taskViewCount
		tm.switchView()
	case "shift+tab", "backtab":
		tm.view = (tm.view - 1 + taskViewCount) % taskViewCount
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

	// Delete cursor task (with confirmation). The line is removed
	// from disk via TaskStore.Delete (or in-place line removal as a
	// fallback). Irreversible — undo is line-replace and a deleted
	// line has no slot to restore into, so we gate on a y/n prompt
	// rather than risk a quick-keypress mishap deleting the wrong
	// task. The preview shows the truncated task text so the user
	// confirms what they're actually about to remove.
	//
	// Three keys bound: `Ctrl+D` (universal convention), `Delete`
	// (the keyboard key), and `!` (legacy alias kept for muscle
	// memory). The previous binding was only `!` — undiscoverable,
	// users couldn't find how to delete tasks at all.
	case "!", "ctrl+d", "delete":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			preview := tmCleanText(task.Text)
			if len(preview) > 60 {
				preview = preview[:57] + "..."
			}
			tm.confirmMsg = fmt.Sprintf("Delete task: %q? (y/n)", preview)
			taskCopy := task
			tm.confirmAction = func() {
				if tm.doDelete(taskCopy) {
					tm.statusMsg = "Task deleted"
					tm.reparse()
				} else {
					tm.statusMsg = "Delete failed"
				}
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

	// AI breakdown — split task into subtasks
	case "S":
		if tm.cursor < len(tm.filtered) && !tm.aiPending && tm.ai.Provider != "local" && tm.ai.Provider != "" {
			task := tm.filtered[tm.cursor]
			if !task.Done {
				tm.aiPending = true
				tm.statusMsg = "AI generating subtasks..."
				return tm, tm.aiBreakdownTask(task)
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

	// Time-block assignment (B = Block schedule)
	case "B":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			tm.inputTask = &task
			tm.inputMode = tmInputTimeBlock
		}

	// Focus session on current task
	case "f":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			tm.focusTask = tmCleanText(task.Text)
			tm.hasFocusReq = true
			tm.active = false
		}

	// Toggle time-tracking timer for the cursor task. Model
	// reads this via GetTimerToggleRequest after Update and
	// drives the actual TimeTracker (StartTimer / StopTimer).
	// Showing in the row: "▸ Nm" badge appears next to the
	// task currently being tracked.
	case "y":
		if tm.cursor < len(tm.filtered) {
			tm.timerToggleTask = tm.filtered[tm.cursor]
			tm.timerToggleReq = true
		}

	// Saved filter views — '*' opens the picker / save prompt.
	// Empty input shows the list; typing a name either applies
	// (if exists) or prompts to save the current combo.
	case "*":
		tm.inputMode = tmInputSavedFilter
		tm.inputBuf = ""

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
		tm.saveFilters()

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
		tm.saveFilters()

	// Project filter: focus the cursor task's project. Press again
	// (or use the unified clear) to drop the filter. Mnemonic: '=' as
	// in "show me what equals this project". When the cursor task has
	// no Project field set, the keystroke is a no-op + status hint
	// rather than a silent miss.
	case "=":
		if tm.cursor >= len(tm.filtered) {
			break
		}
		task := tm.filtered[tm.cursor]
		// Toggle: if this project is already the active filter, clear it.
		if strings.EqualFold(strings.TrimSpace(tm.filterProject), strings.TrimSpace(task.Project)) && tm.filterProject != "" {
			tm.filterProject = ""
			tm.statusMsg = "Project filter cleared"
		} else if strings.TrimSpace(task.Project) == "" {
			tm.statusMsg = "No project on this task — set one via frontmatter or @project:Name"
		} else {
			tm.filterProject = task.Project
			tm.statusMsg = "Filter: project = " + task.Project + " (= to clear)"
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
		tm.saveFilters()

	// Clear all active filters
	case "c":
		if tm.filterTag != "" || tm.filterPriority >= 0 || tm.filterTriage != "" || tm.searchTerm != "" || tm.sortMode != tmSortPriority {
			tm.filterTag = ""
			tm.filterPriority = -1
			tm.filterTriage = ""
			tm.searchTerm = ""
			tm.sortMode = tmSortPriority
			tm.statusMsg = "Filters cleared"
			tm.rebuildFiltered()
			tm.saveFilters()
		}

	// Unified filter prompt — set multiple filters in one input.
	case "F":
		tm.inputMode = tmInputFilter
		tm.inputBuf = ""

	// Quick edit — single-line popover to adjust priority/due/estimate at once.
	case ".":
		if tm.cursor < len(tm.filtered) {
			tm.inputMode = tmInputQuickEdit
			tm.inputBuf = ""
		}

	// Cycle triage state on the cursor task. inbox → triaged →
	// scheduled → snoozed → dropped → inbox. Power-user shortcut
	// for inbox-zero workflows that don't want to leave the
	// TaskManager tab to use the dedicated Triage queue.
	case ";":
		if tm.cursor < len(tm.filtered) {
			tm.cycleTriageState(tm.filtered[tm.cursor])
		}

	// Direct-set triage leader. Press 'm' (mark) then a single
	// letter to set the cursor task's triage state without
	// cycling: i=inbox t=triaged s=scheduled n=snoozed d=dropped
	// x=done. One keystroke faster than ';' when you know exactly
	// where the task should go.
	case "m":
		if tm.cursor < len(tm.filtered) {
			tm.inputMode = tmInputTriageSet
			tm.statusMsg = "Triage: i=inbox t=triaged s=scheduled n=snoozed d=dropped x=done (Esc cancel)"
		}

	// Inline edit the cursor task's full line — no source-note
	// context switch. Esc cancels, Enter commits via
	// writeLineChange. Power users edit text + tags + dates +
	// priorities all in one input instead of three separate
	// modes.
	case "i":
		if tm.cursor < len(tm.filtered) {
			task := tm.filtered[tm.cursor]
			// Pre-populate with the line content after the
			// "- [ ] " prefix so the user can edit-in-place
			// (including tags and emoji markers — power users
			// know what they're doing).
			tm.inputMode = tmInputInlineEdit
			tm.inputBuf = inlineEditSeed(task)
		}

	// Compact density toggle — hides the per-cursor detail
	// strip and section dividers so power users see ~60% more
	// rows on screen. Hint dots still convey "this task has
	// metadata" without taking horizontal room.
	case "D":
		tm.compact = !tm.compact
		if tm.compact {
			tm.statusMsg = "Compact mode on (D to toggle)"
		} else {
			tm.statusMsg = "Compact mode off"
		}
	}

	return tm, nil
}

