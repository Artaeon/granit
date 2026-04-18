package tui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CalendarEvent represents a single event parsed from an .ics file or native store.
type CalendarEvent struct {
	Title       string
	Date        time.Time
	EndDate     time.Time
	Location    string
	Description string
	AllDay      bool
	ID          string // native event ID (empty for ICS events)
	Color       string // "red", "blue", "green", "yellow", "mauve", "teal", "peach"
	Recurrence  string // "daily", "weekly", "monthly", "yearly"
}

// DailyFocus holds the top goal and focus items for a given day.
type DailyFocus struct {
	TopGoal    string
	FocusItems []string
}

// PlannerBlock represents a scheduled block from the daily planner.
// SourceRef, when set, identifies the task line that produced this block so
// upsert/remove can match precisely even when multiple tasks share text.
type PlannerBlock struct {
	Date      string // YYYY-MM-DD
	StartTime string // HH:MM
	EndTime   string // HH:MM
	Text      string
	BlockType BlockType // canonical kind; see blocktype.go for constants
	Done      bool
	SourceRef ScheduleRef // optional — empty for hand-written / event blocks
}

// TaskToggle represents a task whose completion state was toggled in the calendar.
type TaskToggle struct {
	NotePath string
	LineNum  int
	Text     string
	Done     bool
}

// calendarView controls which view mode is active.
type calendarView int

const (
	calViewMonth calendarView = iota
	calViewWeek
	calViewAgenda
	calViewYear
	calView3Day
	calView1Day
)

// TaskItem represents a task extracted from notes.
type TaskItem struct {
	Text             string
	Done             bool
	NotePath         string
	Date             string // YYYY-MM-DD if associated with a daily note
	Priority         int    // 0=none, 1=low, 2=medium, 3=high, 4=highest
	LineNum          int    // 1-based line number in source note
	EstimatedMinutes int    // time estimate in minutes (0 = unknown)
}

var taskPattern = regexp.MustCompile(`^- \[([ xX])\] (.+)`)

// taskPriority extracts priority from task text based on emoji markers.
func taskPriority(text string) int {
	if strings.Contains(text, "\U0001f534") { // red circle = highest
		return 4
	}
	if strings.Contains(text, "\U0001f7e0") { // orange circle = high
		return 3
	}
	if strings.Contains(text, "\U0001f7e1") { // yellow circle = medium
		return 2
	}
	if strings.Contains(text, "\U0001f535") { // blue circle = low
		return 1
	}
	return 0
}

// priorityColor returns the color for a given priority level.
func priorityColor(priority int) lipgloss.Color {
	switch priority {
	case 4:
		return red
	case 3:
		return peach
	case 2:
		return yellow
	case 1:
		return blue
	default:
		return text
	}
}

// Calendar is an overlay component that displays a month-view calendar grid.
type Calendar struct {
	active bool
	width  int
	height int

	cursor   time.Time // the currently highlighted date
	viewing  time.Time // the month being displayed (year + month)
	today    time.Time // today's date (year, month, day only)
	selected string    // date the user confirmed with Enter ("2006-01-02"), empty otherwise

	dailyNoteDates map[string]bool // set of "2006-01-02" strings that have daily notes
	events         []CalendarEvent
	showEvents     bool // whether the event sub-panel is expanded
	view           calendarView

	// Task data
	tasks    map[string][]TaskItem // date -> tasks
	allTasks []TaskItem            // all tasks across notes

	// Planner block data (keyed by date "YYYY-MM-DD")
	plannerBlocks map[string][]PlannerBlock

	// Week grid cursor for time-grid navigation
	weekGridCursorHour int // half-hour row index (0 = first visible slot)

	// Habit data for enriched views
	habitEntries []habitEntry
	habitLogs    []habitLog

	// Agenda scroll offset and cursor for task toggling
	agendaScroll int
	agendaCursor int
	agendaItems  []agendaItem // flat list of interactive items in agenda view

	// Quick add event
	addingEvent bool
	eventInput  string

	// Full event creation/editing — single-form modal with all fields visible.
	eventEditMode   int    // 0=closed, 1=open
	eventEditField  int    // focused field index, see ef* constants
	eventEditID     string // "" for new event, "E001" for editing
	eventEditTitle  string
	eventEditTime   string // HH:MM
	eventEditDur    int    // minutes (parsed from eventEditDurBuf on save)
	eventEditDurBuf string // text buffer for duration field
	eventEditLoc    string
	eventEditRecur  string // daily/weekly/monthly/yearly/""
	eventEditColor  string
	eventEditDesc   string

	// Pending event to be saved by app.go
	pendingEventDate string
	pendingEventText string

	// Pending native event to save
	pendingNativeEvent *NativeEvent

	// Pending event deletion
	pendingDeleteID string
	confirmDelete   bool

	// Task toggles pending consumption by app.go
	taskToggles []TaskToggle

	// Goals integration
	activeGoals []Goal
	vaultRoot   string

	// Task time-blocking
	timeBlockMode   bool
	timeBlockTasks  []TaskItem
	timeBlockCursor int
	timeBlockDate   string
	timeBlockHour   int

	// Daily focus data (keyed by date "YYYY-MM-DD")
	dailyGoals map[string]DailyFocus

	// Weekly milestone creation
	weekMilestoneMode   bool
	weekMilestoneCursor int
	weekMilestoneStep   int    // 0=pick goal, 1=enter milestone text
	weekMilestoneBuf    string
	weekMilestoneGoalID string
}

// NewCalendar creates a new Calendar overlay.
func NewCalendar() Calendar {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return Calendar{
		cursor:         today,
		viewing:        today,
		today:          today,
		dailyNoteDates: make(map[string]bool),
		tasks:          make(map[string][]TaskItem),
		plannerBlocks:  make(map[string][]PlannerBlock),
	}
}

func (c *Calendar) SetSize(width, height int) {
	c.width = width
	c.height = height
}

func (c *Calendar) SetActiveGoals(goals []Goal) { c.activeGoals = goals }
func (c *Calendar) SetVaultRoot(root string)     { c.vaultRoot = root }

// SetDailyFocus stores focus data for a given date.
func (c *Calendar) SetDailyFocus(date string, focus DailyFocus) {
	if c.dailyGoals == nil {
		c.dailyGoals = make(map[string]DailyFocus)
	}
	c.dailyGoals[date] = focus
}

// SetAllDailyFocus replaces all daily focus data.
func (c *Calendar) SetAllDailyFocus(all map[string]DailyFocus) {
	c.dailyGoals = all
}

func (c *Calendar) Open() {
	c.active = true
	c.selected = ""
	c.showEvents = false
	c.addingEvent = false
	c.eventInput = ""
	c.agendaScroll = 0
	c.agendaCursor = 0
	c.agendaItems = nil
	c.taskToggles = nil
	now := time.Now()
	c.today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	c.cursor = c.today
	c.viewing = c.today
}

func (c *Calendar) Close()         { c.active = false }
func (c *Calendar) IsActive() bool { return c.active }

func (c *Calendar) SetDailyNotes(notes []string) {
	c.dailyNoteDates = make(map[string]bool, len(notes))
	for _, p := range notes {
		base := filepath.Base(p)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		if _, err := time.Parse("2006-01-02", base); err == nil {
			c.dailyNoteDates[base] = true
		}
	}
}

func (c *Calendar) SetEvents(events []CalendarEvent) {
	c.events = events
	if c.view == calViewAgenda {
		c.rebuildAgendaItems()
	}
}

// GetEvents returns the currently loaded calendar events.
func (c Calendar) GetEvents() []CalendarEvent {
	return c.events
}

// SetHabitData stores habit entries and logs for enriched calendar views.
func (c *Calendar) SetHabitData(entries []habitEntry, logs []habitLog) {
	c.habitEntries = entries
	c.habitLogs = logs
}

// habitStats returns (completed, total) habit counts for a given date.
func (c Calendar) habitStats(dateStr string) (int, int) {
	total := len(c.habitEntries)
	if total == 0 {
		return 0, 0
	}
	done := 0
	for _, log := range c.habitLogs {
		if log.Date == dateStr {
			done = len(log.Completed)
			break
		}
	}
	return done, total
}

// SetNoteContents parses tasks from note content for the calendar.
func (c *Calendar) SetNoteContents(notes map[string]string) {
	c.tasks = make(map[string][]TaskItem)
	c.allTasks = nil

	for path, content := range notes {
		// Check if this is a daily note (filename is date)
		base := filepath.Base(path)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		dateStr := ""
		if _, err := time.Parse("2006-01-02", base); err == nil {
			dateStr = base
		}

		for lineIdx, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			matches := taskPattern.FindStringSubmatch(line)
			if matches == nil {
				continue
			}
			done := matches[1] == "x" || matches[1] == "X"
			task := TaskItem{
				Text:     matches[2],
				Done:     done,
				NotePath: path,
				Date:     dateStr,
				Priority: taskPriority(matches[2]),
				LineNum:  lineIdx + 1, // 1-based
			}
			c.allTasks = append(c.allTasks, task)

			// Place task in date bucket — from daily note filename or due date marker
			taskDate := dateStr
			if taskDate == "" {
				if dm := tmDueDateRe.FindStringSubmatch(line); dm != nil {
					taskDate = dm[1]
					task.Date = taskDate
				}
			}
			if taskDate != "" {
				c.tasks[taskDate] = append(c.tasks[taskDate], task)
			}
		}
	}
	// Keep agenda items in sync when task data changes.
	if c.view == calViewAgenda {
		c.rebuildAgendaItems()
	}
}

func (c *Calendar) SelectedDate() string {
	s := c.selected
	c.selected = ""
	return s
}

// PendingEvent returns any event the user quick-added, then clears it.
// Returns (date "2006-01-02", taskText, ok).
func (c *Calendar) PendingEvent() (string, string, bool) {
	if c.pendingEventDate == "" {
		return "", "", false
	}
	date := c.pendingEventDate
	text := c.pendingEventText
	c.pendingEventDate = ""
	c.pendingEventText = ""
	return date, text, true
}

// agendaItem is an interactive item in the agenda view that can be toggled.
type agendaItem struct {
	itemType string // "task", "planner", "event"
	dateStr  string
	index    int    // index within the day's tasks or planner blocks
	eventID  string // native event ID (for editing/deletion)
}

// SetPlannerBlocks stores planner schedule data keyed by date.
func (c *Calendar) SetPlannerBlocks(blocks map[string][]PlannerBlock) {
	c.plannerBlocks = blocks
	if c.view == calViewAgenda {
		c.rebuildAgendaItems()
	}
}

// GetTaskToggles returns pending task toggles and clears them (consumed-once).
func (c *Calendar) GetTaskToggles() []TaskToggle {
	if len(c.taskToggles) == 0 {
		return nil
	}
	toggles := c.taskToggles
	c.taskToggles = nil
	return toggles
}

func (c Calendar) Update(msg tea.Msg) (Calendar, tea.Cmd) {
	if !c.active {
		return c, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Quick-add event input mode
		if c.addingEvent {
			switch msg.String() {
			case "esc":
				c.addingEvent = false
				c.eventInput = ""
			case "enter":
				if strings.TrimSpace(c.eventInput) != "" {
					c.pendingEventDate = c.cursor.Format("2006-01-02")
					c.pendingEventText = strings.TrimSpace(c.eventInput)
				}
				c.addingEvent = false
				c.eventInput = ""
			case "backspace":
				if len(c.eventInput) > 0 {
					c.eventInput = TrimLastRune(c.eventInput)
				}
			default:
				if len(msg.String()) == 1 || msg.String() == " " {
					c.eventInput += msg.String()
				}
			}
			return c, nil
		}

		// Weekly milestone creation mode
		if c.weekMilestoneMode {
			switch msg.String() {
			case "esc":
				c.weekMilestoneMode = false
			case "up", "k":
				if c.weekMilestoneStep == 0 && c.weekMilestoneCursor > 0 {
					c.weekMilestoneCursor--
				}
			case "down", "j":
				if c.weekMilestoneStep == 0 && c.weekMilestoneCursor < len(c.activeGoals)-1 {
					c.weekMilestoneCursor++
				}
			case "enter":
				if c.weekMilestoneStep == 0 && c.weekMilestoneCursor < len(c.activeGoals) {
					// Selected a goal, now enter milestone text
					c.weekMilestoneGoalID = c.activeGoals[c.weekMilestoneCursor].ID
					c.weekMilestoneStep = 1
					c.weekMilestoneBuf = ""
				} else if c.weekMilestoneStep == 1 && c.weekMilestoneBuf != "" {
					// Save milestone — due date is end of the cursor's week
					endOfWeek := c.cursor.AddDate(0, 0, 6-int(c.cursor.Weekday()))
					addMilestoneToGoal(c.vaultRoot, c.weekMilestoneGoalID, c.weekMilestoneBuf,
						endOfWeek.Format("2006-01-02"))
					c.activeGoals = loadActiveGoals(c.vaultRoot) // reload
					c.weekMilestoneMode = false
				}
			case "backspace":
				if c.weekMilestoneStep == 1 && len(c.weekMilestoneBuf) > 0 {
					c.weekMilestoneBuf = TrimLastRune(c.weekMilestoneBuf)
				}
			default:
				if c.weekMilestoneStep == 1 {
					ch := msg.String()
					if len(ch) == 1 {
						c.weekMilestoneBuf += ch
					}
				}
			}
			return c, nil
		}

		// Task time-blocking picker
		if c.timeBlockMode {
			switch msg.String() {
			case "esc":
				c.timeBlockMode = false
			case "up", "k":
				if c.timeBlockCursor > 0 {
					c.timeBlockCursor--
				}
			case "down", "j":
				if c.timeBlockCursor < len(c.timeBlockTasks)-1 {
					c.timeBlockCursor++
				}
			case "enter":
				if c.timeBlockCursor < len(c.timeBlockTasks) {
					task := c.timeBlockTasks[c.timeBlockCursor]
					dur := task.EstimatedMinutes
					if dur <= 0 {
						dur = 60
					}
					endHour := c.timeBlockHour + dur/60
					endMin := dur % 60
					if endHour > 23 {
						endHour = 23
						endMin = 59
					}
					start := fmt.Sprintf("%02d:00", c.timeBlockHour)
					end := fmt.Sprintf("%02d:%02d", endHour, endMin)
					ref := ScheduleRef{
						NotePath: task.NotePath,
						LineNum:  task.LineNum,
						Text:     task.Text,
					}
					// Route through the unified schedule layer so the ⏰ marker
					// also lands on the source task line (previously only the
					// planner block was written — TaskManager never saw it).
					_ = SetTaskSchedule(c.vaultRoot, c.timeBlockDate, ref, start, end, "task")
					c.plannerBlocks[c.timeBlockDate] = append(c.plannerBlocks[c.timeBlockDate], PlannerBlock{
						Date: c.timeBlockDate, StartTime: start, EndTime: end,
						Text: task.Text, BlockType: "task", SourceRef: ref,
					})
					c.timeBlockMode = false
				}
			}
			return c, nil
		}

		// Full event creation/editing form (all fields visible at once).
		if c.eventEditMode > 0 {
			return c.updateEventForm(msg), nil
		}

		// Handle delete confirmation
		if c.confirmDelete {
			if msg.String() == "y" || msg.String() == "Y" {
				c.confirmDelete = false
			} else {
				c.confirmDelete = false
				c.pendingDeleteID = ""
			}
			return c, nil
		}

		switch msg.String() {
		case "esc", "q":
			if c.showEvents {
				c.showEvents = false
			} else {
				c.active = false
			}
			return c, nil

		case "left", "h":
			c.cursor = c.cursor.AddDate(0, 0, -1)
			c.syncViewing()
		case "right", "l":
			c.cursor = c.cursor.AddDate(0, 0, 1)
			c.syncViewing()
		case "up", "k":
			if c.view == calViewAgenda {
				if c.agendaCursor > 0 {
					c.agendaCursor--
				} else if c.agendaScroll > 0 {
					c.agendaScroll--
				}
			} else if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				if c.weekGridCursorHour > 0 {
					c.weekGridCursorHour--
				}
			} else {
				c.cursor = c.cursor.AddDate(0, 0, -7)
				c.syncViewing()
			}
		case "down", "j":
			if c.view == calViewAgenda {
				if c.agendaCursor < len(c.agendaItems)-1 {
					c.agendaCursor++
				} else {
					c.agendaScroll++
				}
			} else if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				if c.weekGridCursorHour < c.maxGridSlots()-1 {
					c.weekGridCursorHour++
				}
			} else {
				c.cursor = c.cursor.AddDate(0, 0, 7)
				c.syncViewing()
			}

		case " ":
			// Toggle task completion in agenda view
			if c.view == calViewAgenda && c.agendaCursor >= 0 && c.agendaCursor < len(c.agendaItems) {
				item := c.agendaItems[c.agendaCursor]
				if item.itemType == "task" {
					tasks := c.tasks[item.dateStr]
					if item.index >= 0 && item.index < len(tasks) {
						tasks[item.index].Done = !tasks[item.index].Done
						c.tasks[item.dateStr] = tasks
						// Also update allTasks
						for i := range c.allTasks {
							if c.allTasks[i].NotePath == tasks[item.index].NotePath &&
								c.allTasks[i].LineNum == tasks[item.index].LineNum {
								c.allTasks[i].Done = tasks[item.index].Done
								break
							}
						}
						c.taskToggles = append(c.taskToggles, TaskToggle{
							NotePath: tasks[item.index].NotePath,
							LineNum:  tasks[item.index].LineNum,
							Text:     tasks[item.index].Text,
							Done:     tasks[item.index].Done,
						})
					}
				}
			}

		case "[":
			c.cursor = c.cursor.AddDate(0, -1, 0)
			c.syncViewing()
		case "]":
			c.cursor = c.cursor.AddDate(0, 1, 0)
			c.syncViewing()
		case "{":
			c.cursor = c.cursor.AddDate(-1, 0, 0)
			c.syncViewing()
		case "}":
			c.cursor = c.cursor.AddDate(1, 0, 0)
			c.syncViewing()

		case "enter":
			if c.view == calViewAgenda {
				// In agenda view, toggle task on enter too
				if c.agendaCursor >= 0 && c.agendaCursor < len(c.agendaItems) {
					item := c.agendaItems[c.agendaCursor]
					if item.itemType == "task" {
						tasks := c.tasks[item.dateStr]
						if item.index >= 0 && item.index < len(tasks) {
							tasks[item.index].Done = !tasks[item.index].Done
							c.tasks[item.dateStr] = tasks
							for i := range c.allTasks {
								if c.allTasks[i].NotePath == tasks[item.index].NotePath &&
									c.allTasks[i].LineNum == tasks[item.index].LineNum {
									c.allTasks[i].Done = tasks[item.index].Done
									break
								}
							}
							c.taskToggles = append(c.taskToggles, TaskToggle{
								NotePath: tasks[item.index].NotePath,
								LineNum:  tasks[item.index].LineNum,
								Text:     tasks[item.index].Text,
								Done:     tasks[item.index].Done,
							})
						}
						return c, nil
					}
				}
			}
			c.selected = c.cursor.Format("2006-01-02")
			c.active = false
			return c, nil

		case "t":
			c.cursor = c.today
			c.syncViewing()

		case "w", "v":
			// Cycle through month -> week -> 3day -> 1day -> agenda -> year -> month
			switch c.view {
			case calViewMonth:
				c.view = calViewWeek
			case calViewWeek:
				c.view = calView3Day
			case calView3Day:
				c.view = calView1Day
			case calView1Day:
				c.view = calViewAgenda
				c.agendaScroll = 0
				c.agendaCursor = 0
				c.rebuildAgendaItems()
			case calViewAgenda:
				c.view = calViewYear
			case calViewYear:
				c.view = calViewMonth
			}

		case "W", "V":
			// Cycle backwards
			switch c.view {
			case calViewMonth:
				c.view = calViewYear
			case calViewYear:
				c.view = calViewAgenda
				c.agendaScroll = 0
				c.agendaCursor = 0
				c.rebuildAgendaItems()
			case calViewAgenda:
				c.view = calView1Day
			case calView1Day:
				c.view = calView3Day
			case calView3Day:
				c.view = calViewWeek
			case calViewWeek:
				c.view = calViewMonth
			}

		case "y":
			if c.view == calViewYear {
				c.view = calViewMonth
			} else {
				c.view = calViewYear
			}

		case "a":
			// Open the full event creation form focused on Title. In time-grid
			// views, pre-fill Time from the grid cursor so "press a on a slot"
			// creates an event at that hour (most users expect this — the
			// previous all-day default meant re-typing the time every time).
			c.eventEditMode = 1
			c.eventEditField = efTitle
			c.eventEditID = ""
			c.eventEditTitle = ""
			c.eventEditTime = ""
			if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				sH := c.weekGridStartHourFor()
				cursorMin := (sH+c.weekGridCursorHour/2)*60 + (c.weekGridCursorHour%2)*30
				c.eventEditTime = fmt.Sprintf("%02d:%02d", cursorMin/60, cursorMin%60)
			}
			c.eventEditDur = 60
			c.eventEditDurBuf = "60"
			c.eventEditLoc = ""
			c.eventEditRecur = ""
			c.eventEditColor = ""
			c.eventEditDesc = ""

		case "b":
			if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				c.timeBlockMode = true
				c.timeBlockCursor = 0
				weekStart := c.cursor.AddDate(0, 0, -int(c.cursor.Weekday()))
				c.timeBlockDate = c.cursor.Format("2006-01-02")
				c.timeBlockHour = c.weekGridStartHourFor() + c.weekGridCursorHour/2
				// Collect candidate tasks: incomplete, due this week or overdue
				c.timeBlockTasks = nil
				weekEnd := weekStart.AddDate(0, 0, 7).Format("2006-01-02")
				for _, t := range c.allTasks {
					if !t.Done && (t.Date <= weekEnd || t.Date == "") {
						c.timeBlockTasks = append(c.timeBlockTasks, t)
					}
				}
			}

		case "g":
			if len(c.activeGoals) > 0 {
				c.weekMilestoneMode = true
				c.weekMilestoneCursor = 0
				c.weekMilestoneStep = 0
				c.weekMilestoneBuf = ""
				c.weekMilestoneGoalID = ""
			}

		case "e":
			if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				// Edit event under cursor: find the first event in the cursor's time slot
				cursorDay := c.cursor
				sH := c.weekGridStartHourFor()
				cursorSlotMin := (sH+c.weekGridCursorHour/2)*60 + (c.weekGridCursorHour%2)*30
				for _, ev := range c.eventsForDate(cursorDay) {
					if ev.AllDay || ev.ID == "" {
						continue
					}
					evStart := ev.Date.Hour()*60 + ev.Date.Minute()
					evEnd := evStart + 60
					if !ev.EndDate.IsZero() {
						evEnd = ev.EndDate.Hour()*60 + ev.EndDate.Minute()
					}
					if evStart < cursorSlotMin+30 && evEnd > cursorSlotMin {
						// Pre-populate the event form
						c.eventEditMode = 1
						c.eventEditField = efTitle
						c.eventEditID = ev.ID
						c.eventEditTitle = ev.Title
						c.eventEditTime = ev.Date.Format("15:04")
						dur := 60
						if !ev.EndDate.IsZero() {
							dur = int(ev.EndDate.Sub(ev.Date).Minutes())
						}
						c.eventEditDur = dur
						c.eventEditDurBuf = strconv.Itoa(dur)
						c.eventEditLoc = ev.Location
						c.eventEditRecur = ev.Recurrence
						c.eventEditColor = ev.Color
						c.eventEditDesc = ev.Description
						break
					}
				}
			} else {
				c.showEvents = !c.showEvents
			}

		case "d":
			// Delete event (in agenda view or week/day view)
			if c.view == calViewAgenda && c.agendaCursor >= 0 && c.agendaCursor < len(c.agendaItems) {
				item := c.agendaItems[c.agendaCursor]
				if item.eventID != "" {
					c.confirmDelete = true
					c.pendingDeleteID = item.eventID
				}
			} else if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				// Delete event under cursor in time-grid views
				cursorDay := c.cursor
				sH := c.weekGridStartHourFor()
				cursorSlotMin := (sH+c.weekGridCursorHour/2)*60 + (c.weekGridCursorHour%2)*30
				for _, ev := range c.eventsForDate(cursorDay) {
					if ev.AllDay || ev.ID == "" {
						continue
					}
					evStart := ev.Date.Hour()*60 + ev.Date.Minute()
					evEnd := evStart + 60
					if !ev.EndDate.IsZero() {
						evEnd = ev.EndDate.Hour()*60 + ev.EndDate.Minute()
					}
					if evStart < cursorSlotMin+30 && evEnd > cursorSlotMin {
						c.confirmDelete = true
						c.pendingDeleteID = ev.ID
						break
					}
				}
			}

		case "D":
			// Unschedule the planner block under the cursor. If the block
			// was produced from a task (SourceRef set), route through the
			// schedule layer so the task's ⏰ marker is cleared too.
			if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				c.unscheduleBlockAtCursor()
			}

		case ",":
			if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				c.shiftBlockAtCursor(-15)
			}
		case ".":
			if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				c.shiftBlockAtCursor(15)
			}
		}
	}

	return c, nil
}

// cursorSlotMinutes returns the minute-of-day the grid cursor points at,
// for week/3-day/1-day views. Only meaningful in those views.
func (c *Calendar) cursorSlotMinutes() int {
	sH := c.weekGridStartHourFor()
	return (sH+c.weekGridCursorHour/2)*60 + (c.weekGridCursorHour%2)*30
}

// maxGridSlots mirrors the bound used by the j/k navigation handler so
// programmatic cursor moves (e.g. shiftBlockAtCursor) can't drive the
// cursor past the visible grid. Half-hour rows; 17 hours max.
func (c *Calendar) maxGridSlots() int {
	maxSlots := c.height - 14
	if maxSlots < 16 {
		maxSlots = 16
	}
	if maxSlots > 34 {
		maxSlots = 34
	}
	return maxSlots
}

// blockAtCursor returns the index and pointer to the planner block the
// grid cursor is on, or (-1, nil) if no block overlaps. When multiple
// blocks overlap the cursor slot, the "primary" planned block is preferred
// over "pomodoro" (actual-work) entries — a user pressing D or ',' on a
// slot where both a plan and a session exist almost always means the plan.
func (c *Calendar) blockAtCursor(dateStr string) (int, *PlannerBlock) {
	cursorSlotMin := c.cursorSlotMinutes()
	blocks := c.plannerBlocks[dateStr]
	var firstMatch = -1
	for i := range blocks {
		pb := &blocks[i]
		startMin := slotToMinutes(pb.StartTime)
		endMin := slotToMinutes(pb.EndTime)
		if endMin <= startMin {
			endMin = startMin + 60
		}
		if !(startMin < cursorSlotMin+30 && endMin > cursorSlotMin) {
			continue
		}
		if firstMatch == -1 {
			firstMatch = i
		}
		// Prefer any non-pomodoro overlap; pomodoro blocks are a read-only
		// audit trail and editing them isn't what the user asked for.
		if pb.BlockType != BlockTypePomodoro {
			return i, pb
		}
	}
	if firstMatch >= 0 {
		return firstMatch, &blocks[firstMatch]
	}
	return -1, nil
}

// shiftBlockAtCursor moves the planner block under the grid cursor by
// delta minutes (positive = later, negative = earlier), clamped to
// [00:00, 23:59). Duration is preserved. Routes through the unified
// schedule layer so the source task's ⏰ marker moves with the block.
// The grid cursor tracks the moved block so repeated shifts stay anchored.
// No-op when no block is under the cursor.
func (c *Calendar) shiftBlockAtCursor(deltaMin int) {
	dateStr := c.cursor.Format("2006-01-02")
	i, pb := c.blockAtCursor(dateStr)
	if pb == nil {
		return
	}
	startMin := slotToMinutes(pb.StartTime)
	endMin := slotToMinutes(pb.EndTime)
	if endMin <= startMin {
		endMin = startMin + 60
	}
	newStart := startMin + deltaMin
	newEnd := endMin + deltaMin
	if newStart < 0 || newEnd > 24*60-1 {
		return // clamped out of range — refuse rather than wrap
	}
	startStr := fmtTimeSlot(newStart)
	endStr := fmtTimeSlot(newEnd)

	ref := pb.SourceRef
	if ref.Text == "" {
		ref.Text = pb.Text
	}
	updated := *pb
	updated.StartTime = startStr
	updated.EndTime = endStr

	if ref.hasLocation() {
		_ = SetTaskSchedule(c.vaultRoot, dateStr, ref, startStr, endStr, pb.BlockType)
	} else {
		_ = UpsertPlannerBlock(c.vaultRoot, dateStr, ref, updated)
	}
	c.plannerBlocks[dateStr][i] = updated

	// Keep the grid cursor on the block so the next ',' / '.' still targets
	// it. Half-hour grid index = (newStart / 30) - (gridStartHour * 2).
	// Clamp to [0, maxGridSlots-1] so a shift past the visible window
	// (e.g. block dragged to 22:00 on a 06:00-start grid) leaves the
	// cursor on the last visible row instead of off-screen.
	gridStartMin := c.weekGridStartHourFor() * 60
	if newStart >= gridStartMin {
		idx := (newStart - gridStartMin) / 30
		if max := c.maxGridSlots() - 1; idx > max {
			idx = max
		}
		c.weekGridCursorHour = idx
	}
}

// unscheduleBlockAtCursor removes the planner block under the grid cursor.
// If the block's SourceRef points to a real task, the task's ⏰ marker is
// cleared too (via ClearTaskSchedule); otherwise only the planner-side
// entry is removed. Mutates both the in-memory plannerBlocks cache and
// disk.
func (c *Calendar) unscheduleBlockAtCursor() {
	dateStr := c.cursor.Format("2006-01-02")
	i, pb := c.blockAtCursor(dateStr)
	if pb == nil {
		return
	}
	ref := pb.SourceRef
	if ref.Text == "" {
		ref.Text = pb.Text
	}
	if ref.hasLocation() {
		_ = ClearTaskSchedule(c.vaultRoot, dateStr, ref)
	} else {
		_ = RemovePlannerBlock(c.vaultRoot, dateStr, ref)
	}
	blocks := c.plannerBlocks[dateStr]
	c.plannerBlocks[dateStr] = append(blocks[:i], blocks[i+1:]...)
}

// Event form fields, indexed 0..eventFormCount-1.
const (
	efTitle = iota
	efTime
	efDuration
	efLocation
	efRecurrence
	efColor
	efDescription
	eventFormCount
)

var eventFormFieldNames = [eventFormCount]string{
	"Title",
	"Time",
	"Duration",
	"Location",
	"Recurrence",
	"Color",
	"Description",
}

// isTextField reports whether the given field accepts free-form text input.
// Choice fields (Recurrence, Color) use number keys to pick values.
func isTextField(field int) bool {
	switch field {
	case efRecurrence, efColor:
		return false
	default:
		return true
	}
}

// recurrenceOptions maps digit keys to recurrence values.
var recurrenceOptions = []struct {
	key, value, label string
}{
	{"0", "", "none"},
	{"1", "daily", "daily"},
	{"2", "weekly", "weekly"},
	{"3", "monthly", "monthly"},
	{"4", "yearly", "yearly"},
}

// colorOptions maps digit keys to color names. The zero-key entry uses "" so
// the event falls back to the default blue styling.
var colorOptions = []struct {
	key, value, label string
}{
	{"0", "", "blue"},
	{"1", "red", "red"},
	{"2", "green", "green"},
	{"3", "yellow", "yellow"},
	{"4", "mauve", "mauve"},
	{"5", "teal", "teal"},
	{"6", "peach", "peach"},
}

// updateEventForm handles a key event for the single-form event wizard.
// Returns the updated calendar. When the form is saved (Enter) it populates
// pendingNativeEvent and closes the modal.
func (c Calendar) updateEventForm(msg tea.KeyMsg) Calendar {
	key := msg.String()
	switch key {
	case "esc":
		c.eventEditMode = 0
		return c

	case "enter":
		if strings.TrimSpace(c.eventEditTitle) == "" {
			// Matches legacy UX: pressing Enter on an empty form cancels.
			c.eventEditMode = 0
			return c
		}
		c.commitDurationBuf()
		c.eventEditMode = 0
		c.saveEditedEvent()
		return c

	case "tab", "down":
		c.eventEditField = (c.eventEditField + 1) % eventFormCount
		return c

	case "shift+tab", "up":
		c.eventEditField = (c.eventEditField + eventFormCount - 1) % eventFormCount
		return c

	case "backspace":
		if isTextField(c.eventEditField) {
			c.trimLastRuneOfFocusedField()
		} else {
			// Backspace on a choice field clears the selection.
			switch c.eventEditField {
			case efRecurrence:
				c.eventEditRecur = ""
			case efColor:
				c.eventEditColor = ""
			}
		}
		return c
	}

	// Digit keys drive choice fields.
	if !isTextField(c.eventEditField) {
		switch c.eventEditField {
		case efRecurrence:
			for _, opt := range recurrenceOptions {
				if opt.key == key {
					c.eventEditRecur = opt.value
					return c
				}
			}
		case efColor:
			for _, opt := range colorOptions {
				if opt.key == key {
					c.eventEditColor = opt.value
					return c
				}
			}
		}
		return c
	}

	// Text-field input: append printable single-char keys (incl. space).
	if len(key) == 1 || key == " " {
		c.appendToFocusedField(key)
	}
	return c
}

// appendToFocusedField writes s onto the end of the currently-focused text field.
func (c *Calendar) appendToFocusedField(s string) {
	switch c.eventEditField {
	case efTitle:
		c.eventEditTitle += s
	case efTime:
		c.eventEditTime += s
	case efDuration:
		c.eventEditDurBuf += s
	case efLocation:
		c.eventEditLoc += s
	case efDescription:
		c.eventEditDesc += s
	}
}

// trimLastRuneOfFocusedField removes one trailing rune from the focused field.
func (c *Calendar) trimLastRuneOfFocusedField() {
	switch c.eventEditField {
	case efTitle:
		c.eventEditTitle = TrimLastRune(c.eventEditTitle)
	case efTime:
		c.eventEditTime = TrimLastRune(c.eventEditTime)
	case efDuration:
		c.eventEditDurBuf = TrimLastRune(c.eventEditDurBuf)
	case efLocation:
		c.eventEditLoc = TrimLastRune(c.eventEditLoc)
	case efDescription:
		c.eventEditDesc = TrimLastRune(c.eventEditDesc)
	}
}

// commitDurationBuf parses eventEditDurBuf into the int eventEditDur.
// Leaves the default value when the buffer is empty or unparseable.
func (c *Calendar) commitDurationBuf() {
	if strings.TrimSpace(c.eventEditDurBuf) == "" {
		return
	}
	dur := c.eventEditDur
	if _, err := fmt.Sscanf(c.eventEditDurBuf, "%d", &dur); err == nil && dur > 0 {
		c.eventEditDur = dur
	}
}

func (c *Calendar) saveEditedEvent() {
	dateStr := c.cursor.Format("2006-01-02")
	endTime := ""
	if c.eventEditTime != "" {
		h, m := 0, 0
		_, _ = fmt.Sscanf(c.eventEditTime, "%d:%d", &h, &m)
		endMin := h*60 + m + c.eventEditDur
		endTime = fmt.Sprintf("%02d:%02d", endMin/60, endMin%60)
	}
	allDay := c.eventEditTime == ""
	c.pendingNativeEvent = &NativeEvent{
		Title:       c.eventEditTitle,
		Description: c.eventEditDesc,
		Date:        dateStr,
		StartTime:   c.eventEditTime,
		EndTime:     endTime,
		Location:    c.eventEditLoc,
		Color:       c.eventEditColor,
		Recurrence:  c.eventEditRecur,
		AllDay:      allDay,
	}
}

// PendingNativeEvent returns any event created via the wizard, then clears it.
func (c *Calendar) PendingNativeEvent() *NativeEvent {
	e := c.pendingNativeEvent
	c.pendingNativeEvent = nil
	return e
}

// PendingDeleteID returns the event ID to delete, then clears it.
func (c *Calendar) PendingDeleteID() string {
	id := c.pendingDeleteID
	c.pendingDeleteID = ""
	return id
}

func (c *Calendar) syncViewing() {
	c.viewing = time.Date(c.cursor.Year(), c.cursor.Month(), 1, 0, 0, 0, 0, time.Local)
}

func (c Calendar) View() string {
	switch c.view {
	case calViewWeek:
		return c.viewWeek()
	case calView3Day:
		return c.viewWeek() // reuse week grid for 3-day view
	case calView1Day:
		return c.view1Day()
	case calViewAgenda:
		return c.viewAgenda()
	case calViewYear:
		return c.viewYear()
	default:
		return c.viewMonth()
	}
}

// ---------------------------------------------------------------------------
// Month View
// ---------------------------------------------------------------------------


func (c Calendar) taskStats(dateStr string) (done, total int) {
	tasks := c.tasks[dateStr]
	total = len(tasks)
	for _, t := range tasks {
		if t.Done {
			done++
		}
	}
	return
}

func (c Calendar) dateHasEvent(dt time.Time) bool {
	y, m, d := dt.Date()
	for _, ev := range c.events {
		ey, em, ed := ev.Date.Date()
		if ey == y && em == m && ed == d {
			return true
		}
		if !ev.EndDate.IsZero() {
			start := time.Date(ey, em, ed, 0, 0, 0, 0, time.Local)
			endY, endM, endD := ev.EndDate.Date()
			end := time.Date(endY, endM, endD, 0, 0, 0, 0, time.Local)
			check := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
			if !check.Before(start) && !check.After(end) {
				return true
			}
		}
	}
	return false
}

func (c Calendar) eventsForDate(dt time.Time) []CalendarEvent {
	y, m, d := dt.Date()
	var result []CalendarEvent
	for _, ev := range c.events {
		ey, em, ed := ev.Date.Date()
		match := false
		if ey == y && em == m && ed == d {
			match = true
		}
		if !match && !ev.EndDate.IsZero() {
			start := time.Date(ey, em, ed, 0, 0, 0, 0, time.Local)
			endY, endM, endD := ev.EndDate.Date()
			end := time.Date(endY, endM, endD, 0, 0, 0, 0, time.Local)
			check := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
			if !check.Before(start) && !check.After(end) {
				match = true
			}
		}
		if match {
			result = append(result, ev)
		}
	}
	return result
}

// weekGridStartHourFor computes the effective start hour for the week grid,
// mirroring the logic in viewWeek(). Default 6, shifts earlier if any event
// starts before 6 (down to 4). Used by key handlers (e/d/b) to map cursor
// position to absolute time.
func (c Calendar) weekGridStartHourFor() int {
	weekStart := c.cursor.AddDate(0, 0, -int(c.cursor.Weekday()))
	startHour := 6
	for di := 0; di < 7; di++ {
		day := weekStart.AddDate(0, 0, di)
		for _, ev := range c.eventsForDate(day) {
			if ev.AllDay {
				continue
			}
			h := ev.Date.Hour()
			if h < startHour && h >= 4 {
				startHour = h
			}
		}
	}
	return startHour
}

func daysIn(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// calEventColor returns a lipgloss color based on a CalendarEvent's color name.
func calEventColor(ev CalendarEvent) lipgloss.Color {
	switch ev.Color {
	case "red":
		return red
	case "green":
		return green
	case "yellow":
		return yellow
	case "mauve":
		return mauve
	case "teal":
		return teal
	case "peach":
		return peach
	default:
		return blue
	}
}

// sameDay returns true if two times are on the same calendar date.
func sameDay(a, b time.Time) bool {
	y1, m1, d1 := a.Date()
	y2, m2, d2 := b.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

