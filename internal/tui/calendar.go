package tui

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
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

// PlannerBlock represents a scheduled block from the daily planner.
type PlannerBlock struct {
	Date      string // YYYY-MM-DD
	StartTime string // HH:MM
	EndTime   string // HH:MM
	Text      string
	BlockType string // task, event, break, focus
	Done      bool
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
	weekGridCursorHour int // 0-16 (row, hours 6-22)

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

	// Full event creation/editing
	eventEditMode  int    // 0=none, 1=title, 2=time, 3=duration, 4=location, 5=recurrence, 6=color, 7=description
	eventEditID    string // "" for new event, "E001" for editing
	eventEditTitle string
	eventEditTime  string // HH:MM
	eventEditDur   int    // minutes
	eventEditLoc   string
	eventEditRecur string // daily/weekly/monthly/yearly/""
	eventEditColor string
	eventEditDesc  string
	eventEditBuf   string // current input buffer

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

// writePlannerBlock appends a scheduled block to the planner file for the given date.
func writePlannerBlock(vaultRoot, date string, block PlannerBlock) {
	if vaultRoot == "" {
		return
	}
	dir := filepath.Join(vaultRoot, "Planner")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	path := filepath.Join(dir, date+".md")

	line := fmt.Sprintf("- %s-%s | %s | %s\n", block.StartTime, block.EndTime, block.Text, block.BlockType)

	content, err := os.ReadFile(path)
	if err != nil {
		// Create new planner file with the block
		content = []byte("---\ndate: " + date + "\n---\n\n## Schedule\n" + line)
		_ = os.WriteFile(path, content, 0644)
		return
	}

	// Insert block after "## Schedule" line
	scheduleHeader := "## Schedule\n"
	idx := bytes.Index(content, []byte(scheduleHeader))
	if idx < 0 {
		content = append(content, []byte("\n"+scheduleHeader+line)...)
	} else {
		insertAt := idx + len(scheduleHeader)
		content = append(content[:insertAt], append([]byte(line), content[insertAt:]...)...)
	}
	_ = os.WriteFile(path, content, 0644)
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
					c.eventInput = c.eventInput[:len(c.eventInput)-1]
				}
			default:
				if len(msg.String()) == 1 || msg.String() == " " {
					c.eventInput += msg.String()
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
					block := PlannerBlock{
						StartTime: fmt.Sprintf("%02d:00", c.timeBlockHour),
						EndTime:   fmt.Sprintf("%02d:%02d", endHour, endMin),
						Text:      task.Text,
						BlockType: "task",
					}
					// Add to in-memory planner blocks
					c.plannerBlocks[c.timeBlockDate] = append(c.plannerBlocks[c.timeBlockDate], block)
					// Write to planner file
					writePlannerBlock(c.vaultRoot, c.timeBlockDate, block)
					c.timeBlockMode = false
				}
			}
			return c, nil
		}

		// Full event creation/editing wizard
		if c.eventEditMode > 0 {
			switch msg.String() {
			case "esc":
				c.eventEditMode = 0
			case "enter":
				switch c.eventEditMode {
				case 1: // Title → Time
					c.eventEditTitle = strings.TrimSpace(c.eventEditBuf)
					if c.eventEditTitle == "" {
						c.eventEditMode = 0
						break
					}
					c.eventEditBuf = ""
					c.eventEditMode = 2
				case 2: // Time → Duration
					c.eventEditTime = strings.TrimSpace(c.eventEditBuf)
					c.eventEditBuf = ""
					c.eventEditMode = 3
				case 3: // Duration → Location
					if d := strings.TrimSpace(c.eventEditBuf); d != "" {
						dur := 60
						_, _ = fmt.Sscanf(d, "%d", &dur)
						if dur > 0 {
							c.eventEditDur = dur
						}
					}
					c.eventEditBuf = ""
					c.eventEditMode = 4
				case 4: // Location → Recurrence
					c.eventEditLoc = strings.TrimSpace(c.eventEditBuf)
					c.eventEditBuf = ""
					c.eventEditMode = 5
				case 5: // Recurrence → Color
					c.eventEditMode = 6
					c.eventEditBuf = ""
				case 6: // Color (skip) → Description
					c.eventEditMode = 7
					c.eventEditBuf = ""
				case 7: // Description → save
					c.eventEditDesc = strings.TrimSpace(c.eventEditBuf)
					c.eventEditMode = 0
					c.saveEditedEvent()
				}
			case "1":
				if c.eventEditMode == 5 {
					c.eventEditRecur = "daily"
					c.eventEditMode = 6
					c.eventEditBuf = ""
				} else if c.eventEditMode == 6 {
					c.eventEditColor = "red"
					c.eventEditMode = 7
					c.eventEditBuf = ""
				} else if c.eventEditMode == 3 {
					c.eventEditDur = 30
					c.eventEditBuf = ""
					c.eventEditMode = 4
				} else {
					c.eventEditBuf += msg.String()
				}
			case "2":
				if c.eventEditMode == 5 {
					c.eventEditRecur = "weekly"
					c.eventEditMode = 6
					c.eventEditBuf = ""
				} else if c.eventEditMode == 6 {
					c.eventEditColor = "green"
					c.eventEditMode = 7
					c.eventEditBuf = ""
				} else if c.eventEditMode == 3 {
					c.eventEditDur = 60
					c.eventEditBuf = ""
					c.eventEditMode = 4
				} else {
					c.eventEditBuf += msg.String()
				}
			case "3":
				if c.eventEditMode == 5 {
					c.eventEditRecur = "monthly"
					c.eventEditMode = 6
					c.eventEditBuf = ""
				} else if c.eventEditMode == 6 {
					c.eventEditColor = "yellow"
					c.eventEditMode = 7
					c.eventEditBuf = ""
				} else if c.eventEditMode == 3 {
					c.eventEditDur = 90
					c.eventEditBuf = ""
					c.eventEditMode = 4
				} else {
					c.eventEditBuf += msg.String()
				}
			case "4":
				if c.eventEditMode == 5 {
					c.eventEditRecur = "yearly"
					c.eventEditMode = 6
					c.eventEditBuf = ""
				} else if c.eventEditMode == 6 {
					c.eventEditColor = "mauve"
					c.eventEditMode = 7
					c.eventEditBuf = ""
				} else if c.eventEditMode == 3 {
					c.eventEditDur = 120
					c.eventEditBuf = ""
					c.eventEditMode = 4
				} else {
					c.eventEditBuf += msg.String()
				}
			case "5":
				if c.eventEditMode == 6 {
					c.eventEditColor = "teal"
					c.eventEditMode = 7
					c.eventEditBuf = ""
				} else {
					c.eventEditBuf += msg.String()
				}
			case "6":
				if c.eventEditMode == 6 {
					c.eventEditColor = "peach"
					c.eventEditMode = 7
					c.eventEditBuf = ""
				} else {
					c.eventEditBuf += msg.String()
				}
			case "0":
				if c.eventEditMode == 5 {
					c.eventEditRecur = ""
					c.eventEditMode = 6
					c.eventEditBuf = ""
				} else if c.eventEditMode == 6 {
					c.eventEditColor = ""
					c.eventEditMode = 7
					c.eventEditBuf = ""
				} else {
					c.eventEditBuf += msg.String()
				}
			case "backspace":
				if len(c.eventEditBuf) > 0 {
					c.eventEditBuf = c.eventEditBuf[:len(c.eventEditBuf)-1]
				}
			default:
				if len(msg.String()) == 1 || msg.String() == " " {
					c.eventEditBuf += msg.String()
				}
			}
			return c, nil
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
				maxHour := c.height - 15
				if maxHour > 17 {
					maxHour = 17
				}
				if c.weekGridCursorHour < maxHour {
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

		case "e":
			c.showEvents = !c.showEvents

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
			// Start full event creation wizard
			c.eventEditMode = 1
			c.eventEditID = ""
			c.eventEditTitle = ""
			c.eventEditTime = ""
			c.eventEditDur = 60
			c.eventEditLoc = ""
			c.eventEditRecur = ""
			c.eventEditColor = ""
			c.eventEditDesc = ""
			c.eventEditBuf = ""

		case "b":
			if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				c.timeBlockMode = true
				c.timeBlockCursor = 0
				weekStart := c.cursor.AddDate(0, 0, -int(c.cursor.Weekday()))
				c.timeBlockDate = c.cursor.Format("2006-01-02")
				c.timeBlockHour = c.weekGridCursorHour + 5 // startHour is 5
				// Collect candidate tasks: incomplete, due this week or overdue
				c.timeBlockTasks = nil
				weekEnd := weekStart.AddDate(0, 0, 7).Format("2006-01-02")
				for _, t := range c.allTasks {
					if !t.Done && (t.Date <= weekEnd || t.Date == "") {
						c.timeBlockTasks = append(c.timeBlockTasks, t)
					}
				}
			}

		case "d":
			// Delete event (in agenda view)
			if c.view == calViewAgenda && c.agendaCursor >= 0 && c.agendaCursor < len(c.agendaItems) {
				item := c.agendaItems[c.agendaCursor]
				if item.eventID != "" {
					c.confirmDelete = true
					c.pendingDeleteID = item.eventID
				}
			}
		}
	}

	return c, nil
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

func (c Calendar) viewMonth() string {
	width := c.width * 2 / 3
	if width < 42 {
		width = 42
	}
	if width > 60 {
		width = 60
	}

	var b strings.Builder

	// Title
	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	viewLabel := DimStyle.Render(" [Month] (press 'v' to change view)")
	b.WriteString("  " + titleIcon + titleText + viewLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")

	// Month/Year header
	monthYear := c.viewing.Format("January 2006")
	navLeft := lipgloss.NewStyle().Foreground(overlay1).Render("< ")
	navRight := lipgloss.NewStyle().Foreground(overlay1).Render(" >")
	header := navLeft + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(monthYear) + navRight
	headerPad := (32 - lipgloss.Width(header)) / 2
	if headerPad < 0 {
		headerPad = 0
	}
	b.WriteString("  " + strings.Repeat(" ", headerPad) + header)
	b.WriteString("\n\n")

	// Day-of-week header with week number column
	wkStyle := lipgloss.NewStyle().Foreground(surface1)
	dayHeaderStyle := lipgloss.NewStyle().Foreground(subtext0).Bold(true)
	weekendHeaderStyle := lipgloss.NewStyle().Foreground(overlay0).Bold(true)
	dayNames := []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	var dayRow strings.Builder
	dayRow.WriteString("  " + wkStyle.Render("Wk") + " ")
	for i, d := range dayNames {
		if i == 0 || i == 6 { // weekend
			dayRow.WriteString(weekendHeaderStyle.Render(fmt.Sprintf("%4s", d)))
		} else {
			dayRow.WriteString(dayHeaderStyle.Render(fmt.Sprintf("%4s", d)))
		}
	}
	b.WriteString(dayRow.String())
	b.WriteString("\n")

	// Calendar grid
	year, month, _ := c.viewing.Date()
	firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	startWeekday := int(firstOfMonth.Weekday())
	daysInMonth := daysIn(month, year)

	prevMonth := firstOfMonth.AddDate(0, 0, -1)
	prevDays := daysIn(prevMonth.Month(), prevMonth.Year())

	// Compute the date of the first cell (may be in previous month)
	firstCellDate := firstOfMonth.AddDate(0, 0, -startWeekday)
	_, weekNum := firstCellDate.ISOWeek()
	row := "  " + wkStyle.Render(fmt.Sprintf("%2d", weekNum)) + " "
	col := 0

	for i := 0; i < startWeekday; i++ {
		day := prevDays - startWeekday + 1 + i
		isWeekend := i == 0 || i == 6
		cell := c.renderDayCell(day, false, false, false, false, 0, 0, false, true, isWeekend)
		row += cell
		col++
	}

	for d := 1; d <= daysInMonth; d++ {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, int(month), d)
		dt := time.Date(year, month, d, 0, 0, 0, 0, time.Local)

		isToday := dt.Equal(c.today)
		isCursor := dt.Equal(c.cursor)
		hasNote := c.dailyNoteDates[dateStr]
		hasEvent := c.dateHasEvent(dt)
		tasksDone, tasksTotal := c.taskStats(dateStr)
		isWeekend := dt.Weekday() == time.Sunday || dt.Weekday() == time.Saturday

		cell := c.renderDayCell(d, isToday, isCursor, hasNote, hasEvent, tasksDone, tasksTotal, true, false, isWeekend)
		row += cell
		col++

		if col == 7 {
			b.WriteString(row)
			b.WriteString("\n")
			col = 0
			if d < daysInMonth {
				nextDate := dt.AddDate(0, 0, 1)
				_, wn := nextDate.ISOWeek()
				row = "  " + wkStyle.Render(fmt.Sprintf("%2d", wn)) + " "
			} else {
				row = ""
			}
		}
	}

	if col > 0 {
		nextDay := 1
		for col < 7 {
			isWeekend := col == 0 || col == 6
			cell := c.renderDayCell(nextDay, false, false, false, false, 0, 0, false, true, isWeekend)
			row += cell
			col++
			nextDay++
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	// Event creation wizard
	if c.eventEditMode > 0 {
		c.renderEventWizard(&b, width)
	} else if c.addingEvent {
		c.renderQuickAdd(&b, width)
	}

	// Confirm delete
	if c.confirmDelete {
		c.renderConfirmDelete(&b)
	}

	// Cursor date info
	c.renderDateInfo(&b, width)

	// Event creation wizard
	c.renderEventWizard(&b, width)

	// Footer
	c.renderFooter(&b, width)

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Week View (with mini calendar sidebar)
// ---------------------------------------------------------------------------

func (c Calendar) viewWeek() string {
	width := c.width - 4
	if width < 70 {
		width = 70
	}

	var b strings.Builder

	// Compact header: week range left, view label right
	weekStart := c.cursor.AddDate(0, 0, -int(c.cursor.Weekday()))
	weekEnd := weekStart.AddDate(0, 0, 6)
	_, weekNum := c.cursor.ISOWeek()
	weekLabel := fmt.Sprintf("W%d  %s – %s", weekNum, weekStart.Format("Jan 2"), weekEnd.Format("Jan 2, 2006"))
	headerLeft := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  " + weekLabel)
	headerRight := DimStyle.Render("week ")
	headerGap := width - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight) - 4
	if headerGap < 1 {
		headerGap = 1
	}
	b.WriteString(headerLeft + strings.Repeat(" ", headerGap) + headerRight)
	b.WriteString("\n")

	// Time grid layout: time column + 7 day columns
	timeColW := 6
	dayColW := (width - timeColW - 8) / 7
	if dayColW < 10 {
		dayColW = 10
	}

	// Day header row
	sepChar := lipgloss.NewStyle().Foreground(surface0).Render("│")
	headerRow := strings.Repeat(" ", timeColW)
	dayNamesShort := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	dayNamesFull := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	for i := 0; i < 7; i++ {
		day := weekStart.AddDate(0, 0, i)
		dayName := dayNamesShort[i]
		if dayColW >= 14 {
			dayName = dayNamesFull[i]
		}
		label := dayName + " " + day.Format("2")
		style := lipgloss.NewStyle().Foreground(surface2)
		if sameDay(day, c.today) {
			style = lipgloss.NewStyle().Foreground(green).Bold(true)
			label = "● " + label
		}
		if sameDay(day, c.cursor) {
			style = lipgloss.NewStyle().Background(surface0).Foreground(mauve).Bold(true)
		}
		cell := style.Render(TruncateDisplay(label, dayColW-2))
		headerRow += sepChar + PadRight(cell, dayColW-1)
	}
	b.WriteString("  " + headerRow + "\n")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("─", width-8)) + "\n")

	// Active goals strip
	if len(c.activeGoals) > 0 {
		goalsLine := "  " + DimStyle.Render("Goals: ")
		for i, g := range c.activeGoals {
			if i >= 4 {
				break
			}
			prog := g.Progress()
			color := yellow
			if prog >= 75 {
				color = green
			} else if prog < 25 {
				color = red
			}
			badge := lipgloss.NewStyle().Foreground(color).Render(
				fmt.Sprintf("[%s %d%%]", TruncateDisplay(g.Title, 12), prog))
			goalsLine += badge + " "
		}
		b.WriteString(goalsLine + "\n")
	}

	// All-day events row
	hasAllDay := false
	for di := 0; di < 7; di++ {
		day := weekStart.AddDate(0, 0, di)
		for _, ev := range c.eventsForDate(day) {
			if ev.AllDay {
				hasAllDay = true
				break
			}
		}
		if hasAllDay {
			break
		}
	}
	if hasAllDay {
		allDayLabel := DimStyle.Render("all day ")
		allDayCells := ""
		for di := 0; di < 7; di++ {
			day := weekStart.AddDate(0, 0, di)
			cellText := ""
			for _, ev := range c.eventsForDate(day) {
				if ev.AllDay {
					cellText = ev.Title
					break
				}
			}
			if cellText != "" {
				allDayCells += lipgloss.NewStyle().Foreground(calEventColor(CalendarEvent{Color: "yellow"})).
					Render(TruncateDisplay(cellText, dayColW-1))
			}
			allDayCells += strings.Repeat(" ", maxInt(0, dayColW-lipgloss.Width(TruncateDisplay(cellText, dayColW-1))))
		}
		b.WriteString("  " + allDayLabel + allDayCells + "\n")
	}

	b.WriteString("  " + DimStyle.Render(strings.Repeat("─", width-8)) + "\n")

	// Render time rows — use as much vertical space as available
	maxRows := c.height - 10
	if maxRows < 8 {
		maxRows = 8
	}
	startHour := 5
	endHour := startHour + maxRows
	if endHour > 23 {
		endHour = 23
		maxRows = endHour - startHour
	}

	for row := 0; row < maxRows; row++ {
		hour := row + startHour
		if hour > 23 {
			break
		}
		// Time label (24h format)
		now := time.Now()
		isCurrentHour := now.Hour() == hour
		timeSt := DimStyle.Render(fmt.Sprintf("%02d:00 ", hour))
		if isCurrentHour {
			timeSt = lipgloss.NewStyle().Foreground(green).Bold(true).
				Render(fmt.Sprintf("▸%02d:%02d", now.Hour(), now.Minute()))
		}

		// Build cells for each day
		cells := ""
		for di := 0; di < 7; di++ {
			day := weekStart.AddDate(0, 0, di)
			dateStr := day.Format("2006-01-02")
			cellText := ""
			cellColor := overlay0

			// Check planner blocks for this hour
			for _, pb := range c.plannerBlocks[dateStr] {
				pbHour := 0
				if len(pb.StartTime) >= 2 {
					_, _ = fmt.Sscanf(pb.StartTime, "%d", &pbHour)
				}
				if pbHour == hour {
					cellText = pb.Text
					cellColor = lavender
					if pb.Done {
						cellColor = green
					}
					break
				}
			}

			// Check events: show events that START at this hour, or SPAN into it
			if cellText == "" {
				evCount := 0
				for _, ev := range c.eventsForDate(day) {
					if ev.AllDay {
						continue
					}
					startHour := ev.Date.Hour()
					endHour := startHour + 1
					if !ev.EndDate.IsZero() {
						endHour = ev.EndDate.Hour()
						if ev.EndDate.Minute() > 0 {
							endHour++
						}
					}
					if startHour <= hour && hour < endHour {
						evCount++
						if cellText == "" {
							if startHour == hour {
								// Show just minutes if not on the hour, else just title
								if ev.Date.Minute() > 0 {
									cellText = ev.Date.Format(":04") + " " + ev.Title
								} else {
									cellText = ev.Title
								}
							} else {
								// Continuation block
								cellText = "  " + ev.Title
							}
							cellColor = calEventColor(ev)
						}
					}
				}
				if evCount > 1 && cellText != "" {
					cellText = TruncateDisplay(cellText, dayColW-6) + fmt.Sprintf(" +%d", evCount-1)
				}
			}

			// Show tasks with due dates in the first empty morning slot (09:00)
			if cellText == "" && hour == 9 {
				if dateTasks, ok := c.tasks[dateStr]; ok {
					pending := 0
					for _, t := range dateTasks {
						if !t.Done {
							pending++
						}
					}
					if pending > 0 {
						cellText = fmt.Sprintf("%d task", pending)
						if pending > 1 {
							cellText += "s"
						}
						cellColor = yellow
					}
				}
			}

			// Render cell
			isCursorCell := sameDay(day, c.cursor) && row == c.weekGridCursorHour
			isToday := sameDay(day, c.today)
			cellContent := ""
			if cellText != "" {
				// Event block: subtle background to make it stand out
				blockStyle := lipgloss.NewStyle().Foreground(cellColor).Background(surface0)
				if isToday {
					blockStyle = blockStyle.Background(surface1)
				}
				cellContent = blockStyle.Render(PadRight(TruncateDisplay(cellText, dayColW-3), dayColW-2))
			} else if isToday {
				// Empty cell in today's column gets a subtle tint
				cellContent = lipgloss.NewStyle().Background(surface0).
					Render(strings.Repeat(" ", dayColW-2))
			}
			if isCursorCell {
				cursorStyle := lipgloss.NewStyle().Background(surface1).Foreground(mauve).Bold(true)
				if cellContent == "" {
					cellContent = cursorStyle.Render(PadRight("▎", dayColW-2))
				} else {
					cellContent = cursorStyle.Render(PadRight(TruncateDisplay(cellText, dayColW-3), dayColW-2))
				}
			}
			cells += sepChar + PadRight(cellContent, dayColW-1)
		}

		b.WriteString("  " + timeSt + cells + "\n")
	}

	// Quick add input
	if c.addingEvent {
		c.renderQuickAdd(&b, width)
	}

	// Event creation wizard
	c.renderEventWizard(&b, width)

	// Task time-blocking picker
	if c.timeBlockMode && len(c.timeBlockTasks) > 0 {
		b.WriteString("\n")
		sepStyle := lipgloss.NewStyle().Foreground(surface0)
		b.WriteString("  " + sepStyle.Render(strings.Repeat("─", width-8)) + "\n")
		titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		b.WriteString(titleStyle.Render(fmt.Sprintf("  Block task at %02d:00 on %s", c.timeBlockHour, c.timeBlockDate)) + "\n")

		maxShow := 8
		start := 0
		if c.timeBlockCursor >= maxShow {
			start = c.timeBlockCursor - maxShow + 1
		}
		end := start + maxShow
		if end > len(c.timeBlockTasks) {
			end = len(c.timeBlockTasks)
		}

		for i := start; i < end; i++ {
			t := c.timeBlockTasks[i]
			prefix := "  "
			style := DimStyle
			if i == c.timeBlockCursor {
				prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▸ ")
				style = lipgloss.NewStyle().Foreground(text)
			}
			dur := ""
			if t.EstimatedMinutes > 0 {
				dur = fmt.Sprintf(" (%dm)", t.EstimatedMinutes)
			}
			b.WriteString("  " + prefix + style.Render(TruncateDisplay(t.Text, width-20)) + DimStyle.Render(dur) + "\n")
		}
		b.WriteString("  " + DimStyle.Render("Enter:block  Esc:cancel") + "\n")
	}

	c.renderFooter(&b, width)

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		MaxHeight(c.height - 2).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Agenda View (enhanced 14-day lookahead)
// ---------------------------------------------------------------------------

// rebuildAgendaItems builds the flat interactive-item list for the agenda view
// and clamps cursor/scroll so they remain within valid bounds. This must be
// called from pointer-receiver lifecycle methods (Update, SetNoteContents, …)
// because viewAgenda() uses a value receiver and cannot persist state changes.
func (c *Calendar) rebuildAgendaItems() {
	lookAhead := 14
	var items []agendaItem
	sectionCount := 0

	for d := 0; d < lookAhead; d++ {
		day := c.today.AddDate(0, 0, d)
		dateStr := day.Format("2006-01-02")

		// Add events for this date
		for _, ev := range c.eventsForDate(day) {
			items = append(items, agendaItem{
				itemType: "event",
				dateStr:  dateStr,
				eventID:  ev.ID,
			})
		}

		// Add planner blocks for this date
		for pi := range c.plannerBlocks[dateStr] {
			items = append(items, agendaItem{
				itemType: "planner",
				dateStr:  dateStr,
				index:    pi,
			})
		}

		// Add tasks for this date
		for ti := range c.tasks[dateStr] {
			items = append(items, agendaItem{
				itemType: "task",
				dateStr:  dateStr,
				index:    ti,
			})
		}
		sectionCount++
	}

	c.agendaItems = items

	// Clamp cursor
	if c.agendaCursor >= len(items) {
		c.agendaCursor = len(items) - 1
	}
	if c.agendaCursor < 0 {
		c.agendaCursor = 0
	}

	// Clamp scroll
	if c.agendaScroll >= sectionCount {
		c.agendaScroll = sectionCount - 1
	}
	if c.agendaScroll < 0 {
		c.agendaScroll = 0
	}
}

func (c Calendar) view1Day() string {
	width := c.width * 2 / 3
	if width < 50 {
		width = 50
	}
	if width > 90 {
		width = 90
	}

	var b strings.Builder

	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	viewLabel := DimStyle.Render(" [day]")
	b.WriteString("  " + titleIcon + titleText + viewLabel)
	b.WriteString("\n")

	// Date header
	dayLabel := c.cursor.Format("Monday, January 2, 2006")
	isToday := c.cursor.Equal(c.today)
	dayStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	if isToday {
		dayStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	}
	b.WriteString("  " + dayStyle.Render(dayLabel))
	if isToday {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("(today)"))
	}
	b.WriteString("\n")
	b.WriteString("  " + DimStyle.Render(strings.Repeat("─", width-8)) + "\n")

	dateStr := c.cursor.Format("2006-01-02")
	now := time.Now()
	contentW := width - 14

	// Time slots 06:00 - 22:00
	for hour := 6; hour <= 22; hour++ {
		timeLabel := fmt.Sprintf("%02d:00", hour)

		// Current time indicator
		isNow := now.Hour() == hour && isToday
		if isNow {
			timeSt := lipgloss.NewStyle().Foreground(green).Bold(true).Render(timeLabel + " ")
			b.WriteString("  " + timeSt)
		} else {
			b.WriteString("  " + DimStyle.Render(timeLabel+" "))
		}

		// Find event/planner block at this hour
		cellText := ""
		cellColor := overlay0

		for _, pb := range c.plannerBlocks[dateStr] {
			pbHour := 0
			if len(pb.StartTime) >= 2 {
				_, _ = fmt.Sscanf(pb.StartTime, "%d", &pbHour)
			}
			if pbHour == hour {
				cellText = pb.Text
				cellColor = lavender
				if pb.Done {
					cellColor = green
				}
				break
			}
		}

		if cellText == "" {
			evCount := 0
			for _, ev := range c.eventsForDate(c.cursor) {
				if ev.AllDay {
					continue
				}
				startHour := ev.Date.Hour()
				endHour := startHour + 1
				if !ev.EndDate.IsZero() {
					endHour = ev.EndDate.Hour()
					if ev.EndDate.Minute() > 0 {
						endHour++
					}
				}
				if startHour <= hour && hour < endHour {
					evCount++
					if cellText == "" {
						if startHour == hour {
							timeRange := ev.Date.Format("15:04")
							if !ev.EndDate.IsZero() {
								timeRange += "-" + ev.EndDate.Format("15:04")
							}
							cellText = timeRange + " " + ev.Title
							if ev.Location != "" {
								cellText += " @ " + ev.Location
							}
						} else {
							cellText = "│ " + ev.Title
						}
						cellColor = calEventColor(ev)
					}
				}
			}
			if evCount > 1 && cellText != "" {
				cellText += fmt.Sprintf(" +%d", evCount-1)
			}
		}

		if cellText != "" {
			b.WriteString(lipgloss.NewStyle().Foreground(cellColor).Render(TruncateDisplay(cellText, contentW)))
		} else if isNow {
			b.WriteString(lipgloss.NewStyle().Foreground(red).Render(strings.Repeat("─", contentW)))
		} else {
			b.WriteString(DimStyle.Render("·"))
		}
		b.WriteString("\n")
	}

	// All-day events
	allDayEvents := false
	for _, ev := range c.eventsForDate(c.cursor) {
		if ev.AllDay {
			if !allDayEvents {
				b.WriteString("\n  " + lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("All Day") + "\n")
				allDayEvents = true
			}
			b.WriteString("  " + lipgloss.NewStyle().Foreground(calEventColor(ev)).Render("  "+ev.Title) + "\n")
		}
	}

	// Tasks due this day
	if tasks, ok := c.tasks[dateStr]; ok && len(tasks) > 0 {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("Tasks") + "\n")
		for _, t := range tasks {
			check := DimStyle.Render("[ ] ")
			if t.Done {
				check = lipgloss.NewStyle().Foreground(green).Render("[x] ")
			}
			b.WriteString("  " + check + t.Text + "\n")
		}
	}

	// Event creation wizard
	if c.eventEditMode > 0 {
		c.renderEventWizard(&b, width)
	}

	// Confirm delete
	if c.confirmDelete {
		c.renderConfirmDelete(&b)
	}

	// Help
	b.WriteString("\n")
	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"h/l", "prev/next day"}, {"w", "switch view"}, {"a", "add event"},
		{"d", "delete"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (c Calendar) viewAgenda() string {
	width := c.width * 2 / 3
	if width < 50 {
		width = 50
	}
	if width > 70 {
		width = 70
	}

	var b strings.Builder

	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	viewLabel := DimStyle.Render(" [agenda]")
	b.WriteString("  " + titleIcon + titleText + viewLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")

	// Show 14-day lookahead from today
	lookAhead := 14
	subTitle := fmt.Sprintf("Next %d Days", lookAhead)
	b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(subTitle))
	b.WriteString("\n\n")

	// Build all agenda sections and interactive item list
	type agendaSection struct {
		header string
		lines  []string
		items  []int // indices into c.agendaItems for each line (-1 if not interactive)
	}
	var sections []agendaSection
	var items []agendaItem

	for d := 0; d < lookAhead; d++ {
		day := c.today.AddDate(0, 0, d)
		dateStr := day.Format("2006-01-02")
		dayTasks := c.tasks[dateStr]
		dayEvents := c.eventsForDate(day)
		dayPlannerBlocks := c.plannerBlocks[dateStr]
		hasNote := c.dailyNoteDates[dateStr]

		// Day header
		dayLabel := day.Format("Mon Jan 2")
		switch d {
		case 0:
			dayLabel += " (today)"
		case 1:
			dayLabel += " (tomorrow)"
		}

		isToday := d == 0
		dayStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
		if isToday {
			dayStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
		}
		isWeekend := day.Weekday() == time.Sunday || day.Weekday() == time.Saturday
		if isWeekend && !isToday {
			dayStyle = lipgloss.NewStyle().Foreground(overlay0).Bold(true)
		}

		section := agendaSection{
			header: "  " + dayStyle.Render(dayLabel),
		}

		// Daily note indicator
		if hasNote {
			section.lines = append(section.lines,
				"    "+lipgloss.NewStyle().Foreground(green).Render(IconDailyChar)+" "+
					lipgloss.NewStyle().Foreground(green).Render("Daily note"))
			section.items = append(section.items, -1)
		}

		// Events (interactive — can be selected and deleted)
		for _, ev := range dayEvents {
			timeStr := "all day"
			if !ev.AllDay {
				timeStr = ev.Date.Format("15:04")
				if !ev.EndDate.IsZero() {
					dur := ev.EndDate.Sub(ev.Date)
					timeStr += "-" + ev.EndDate.Format("15:04")
					if dur.Hours() >= 1 {
						timeStr += fmt.Sprintf(" (%dh", int(dur.Hours()))
						if int(dur.Minutes())%60 > 0 {
							timeStr += fmt.Sprintf("%dm", int(dur.Minutes())%60)
						}
						timeStr += ")"
					} else if dur.Minutes() > 0 {
						timeStr += fmt.Sprintf(" (%dm)", int(dur.Minutes()))
					}
				}
			}
			evColor := calEventColor(ev)
			itemIdx := len(items)
			items = append(items, agendaItem{
				itemType: "event",
				dateStr:  dateStr,
				eventID:  ev.ID,
			})
			evLine := "    " + lipgloss.NewStyle().Foreground(evColor).Render(IconCalendarChar+" ") +
				DimStyle.Render(timeStr+" ") +
				lipgloss.NewStyle().Foreground(text).Render(ev.Title)
			if ev.Location != "" {
				evLine += DimStyle.Render(" @ " + ev.Location)
			}
			if ev.Recurrence != "" {
				evLine += lipgloss.NewStyle().Foreground(overlay1).Render(" ⟲" + ev.Recurrence)
			}
			section.lines = append(section.lines, evLine)
			section.items = append(section.items, itemIdx)
		}

		// Planner blocks
		for _, pb := range dayPlannerBlocks {
			timeRange := pb.StartTime + "-" + pb.EndTime
			tag := "[P]"
			switch pb.BlockType {
			case "break":
				tag = "[B]"
			case "focus":
				tag = "[F]"
			case "event":
				tag = "[E]"
			}
			pbText := TruncateDisplay(pb.Text, width-22)
			pbStyle := lipgloss.NewStyle().Foreground(lavender)
			doneMarker := ""
			if pb.Done {
				pbStyle = lipgloss.NewStyle().Foreground(green).Strikethrough(true)
				doneMarker = lipgloss.NewStyle().Foreground(green).Render(" ✓")
			}
			section.lines = append(section.lines,
				"    "+pbStyle.Render(timeRange+" "+tag+" "+pbText)+doneMarker)
			section.items = append(section.items, -1)
		}

		// Tasks with priority coloring (interactive)
		for ti, task := range dayTasks {
			itemIdx := len(items)
			items = append(items, agendaItem{
				itemType: "task",
				dateStr:  dateStr,
				index:    ti,
			})
			checkIcon := lipgloss.NewStyle().Foreground(yellow).Render("○")
			if task.Done {
				checkIcon = lipgloss.NewStyle().Foreground(green).Render("●")
			}
			taskText := TruncateDisplay(task.Text, width-12)
			textColor := text
			if task.Priority > 0 {
				textColor = priorityColor(task.Priority)
			}
			section.lines = append(section.lines,
				"    "+checkIcon+" "+lipgloss.NewStyle().Foreground(textColor).Render(taskText))
			section.items = append(section.items, itemIdx)
		}

		// If nothing for this day, show dim "No events"
		if len(dayEvents) == 0 && len(dayTasks) == 0 && len(dayPlannerBlocks) == 0 && !hasNote {
			section.lines = append(section.lines, DimStyle.Render("    No events or tasks. Press 'a' to add one."))
			section.items = append(section.items, -1)
		}

		sections = append(sections, section)
	}

	// NOTE: agendaItems, agendaCursor, and agendaScroll are maintained by
	// rebuildAgendaItems() which runs in pointer-receiver methods (Update,
	// SetNoteContents, etc.). We must NOT assign to receiver fields here
	// because viewAgenda uses a value receiver and changes would be lost.

	// Apply scroll and render visible sections
	maxLines := c.height - 14
	if maxLines < 8 {
		maxLines = 8
	}

	lineCount := 0
	for i := c.agendaScroll; i < len(sections) && lineCount < maxLines; i++ {
		sec := sections[i]
		b.WriteString(sec.header)
		b.WriteString("\n")
		lineCount++

		for li, line := range sec.lines {
			if lineCount >= maxLines {
				break
			}
			// Highlight the line if it corresponds to the agenda cursor
			if sec.items[li] >= 0 && sec.items[li] == c.agendaCursor {
				marker := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▸ ")
				line = "  " + marker + strings.TrimLeft(line, " ")
			}
			b.WriteString(line)
			b.WriteString("\n")
			lineCount++
		}
		b.WriteString("\n")
		lineCount++
	}

	// Scroll indicator
	if c.agendaScroll > 0 || c.agendaScroll+maxLines/3 < len(sections) {
		scrollInfo := fmt.Sprintf("  Showing from day %d/%d", c.agendaScroll+1, lookAhead)
		b.WriteString(DimStyle.Render(scrollInfo))
		b.WriteString("\n")
	}

	// Task summary
	totalTasks := len(c.allTasks)
	doneTasks := 0
	for _, t := range c.allTasks {
		if t.Done {
			doneTasks++
		}
	}
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("Total tasks: %d  Done: ", totalTasks)) +
		lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("%d", doneTasks)) +
		lipgloss.NewStyle().Foreground(text).Render("  Pending: ") +
		lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf("%d", totalTasks-doneTasks)))

	// Quick add input
	if c.addingEvent {
		b.WriteString("\n")
		c.renderQuickAdd(&b, width)
	}

	c.renderFooter(&b, width)

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Year View (compact 4x3 grid of mini months)
// ---------------------------------------------------------------------------

func (c Calendar) viewYear() string {
	width := c.width * 3 / 4
	if width < 68 {
		width = 68
	}
	if width > 88 {
		width = 88
	}

	var b strings.Builder

	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	viewLabel := DimStyle.Render(" [year]")
	b.WriteString("  " + titleIcon + titleText + viewLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")

	yearStr := fmt.Sprintf("%d", c.cursor.Year())
	b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(yearStr))
	b.WriteString("\n\n")

	// 4 rows x 3 columns of mini months
	year := c.cursor.Year()
	monthBlocks := make([]string, 12)

	for m := 0; m < 12; m++ {
		monthBlocks[m] = c.renderYearMiniMonth(year, time.Month(m+1))
	}

	for row := 0; row < 4; row++ {
		// Each mini month is 3 lines: name, day header, dots
		// Split each block into lines
		blockLines := make([][]string, 3)
		for col := 0; col < 3; col++ {
			idx := row*3 + col
			blockLines[col] = strings.Split(monthBlocks[idx], "\n")
		}

		// Find max lines among the 3 blocks
		maxL := 0
		for col := 0; col < 3; col++ {
			if len(blockLines[col]) > maxL {
				maxL = len(blockLines[col])
			}
		}

		colWidth := (width - 8) / 3
		if colWidth < 20 {
			colWidth = 20
		}

		for line := 0; line < maxL; line++ {
			rowStr := "  "
			for col := 0; col < 3; col++ {
				cellContent := ""
				if line < len(blockLines[col]) {
					cellContent = blockLines[col][line]
				}
				cellWidth := lipgloss.Width(cellContent)
				pad := colWidth - cellWidth
				if pad < 0 {
					pad = 0
				}
				rowStr += cellContent + strings.Repeat(" ", pad)
			}
			b.WriteString(rowStr)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Quick add input
	if c.addingEvent {
		c.renderQuickAdd(&b, width)
	}

	c.renderFooter(&b, width)

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// renderYearMiniMonth renders a compact 3-line mini month for the year view.
// Line 1: month name
// Line 2: day-of-week abbreviations
// Line 3+: activity dots per day
func (c Calendar) renderYearMiniMonth(year int, month time.Month) string {
	var b strings.Builder

	// Month name, highlight current month
	monthName := month.String()[:3]
	isCursorMonth := c.cursor.Year() == year && c.cursor.Month() == month
	isTodayMonth := c.today.Year() == year && c.today.Month() == month

	nameStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
	if isCursorMonth {
		nameStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
	}
	if isTodayMonth {
		nameStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	}
	b.WriteString(nameStyle.Render(monthName))
	b.WriteString("\n")

	// Short day-of-week header
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("S M T W T F S"))
	b.WriteString("\n")

	// Dots for each day
	firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	startWeekday := int(firstOfMonth.Weekday())
	daysInMo := daysIn(month, year)

	dotGreen := lipgloss.NewStyle().Foreground(green).Render("*")
	dotEvent := lipgloss.NewStyle().Foreground(blue).Render("*")
	dotTask := lipgloss.NewStyle().Foreground(yellow).Render("*")
	dotToday := lipgloss.NewStyle().Background(green).Foreground(crust).Bold(true).Render("*")
	dotCursor := lipgloss.NewStyle().Foreground(peach).Bold(true).Render("*")
	dotEmpty := lipgloss.NewStyle().Foreground(surface0).Render(".")
	dotPad := " "

	row := ""
	col := 0
	for i := 0; i < startWeekday; i++ {
		row += dotPad + " "
		col++
	}

	for d := 1; d <= daysInMo; d++ {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, int(month), d)
		dt := time.Date(year, month, d, 0, 0, 0, 0, time.Local)
		hasNote := c.dailyNoteDates[dateStr]
		hasEvent := c.dateHasEvent(dt)
		_, tasksTotal := c.taskStats(dateStr)
		isToday := dt.Equal(c.today)
		isCur := dt.Equal(c.cursor)

		dot := dotEmpty
		switch {
		case isToday:
			dot = dotToday
		case isCur:
			dot = dotCursor
		case hasNote:
			dot = dotGreen
		case hasEvent:
			dot = dotEvent
		case tasksTotal > 0:
			dot = dotTask
		}

		row += dot + " "
		col++

		if col == 7 {
			b.WriteString(row)
			b.WriteString("\n")
			row = ""
			col = 0
		}
	}

	if col > 0 {
		b.WriteString(row)
		b.WriteString("\n")
	}

	return b.String()
}

func (c Calendar) renderEventWizard(b *strings.Builder, width int) {
	ps := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	is := lipgloss.NewStyle().Foreground(text)
	ds := DimStyle
	ss := lipgloss.NewStyle().Foreground(lavender).Bold(true)

	dateLabel := c.cursor.Format("Mon, Jan 2 2006")
	b.WriteString("  " + ps.Render("New Event") + "  " + ds.Render(dateLabel) + "\n\n")

	switch c.eventEditMode {
	case 1: // Title
		b.WriteString("  " + ps.Render("Title: ") + is.Render(c.eventEditBuf+"\u2588") + "\n")
		b.WriteString("  " + ds.Render("Enter to continue, Esc to cancel"))
	case 2: // Time
		b.WriteString("  " + ds.Render("Title: "+c.eventEditTitle) + "\n")
		b.WriteString("  " + ps.Render("Start time (HH:MM, empty for all-day): ") + is.Render(c.eventEditBuf+"\u2588") + "\n")
	case 3: // Duration
		b.WriteString("  " + ds.Render("Title: "+c.eventEditTitle) + "\n")
		if c.eventEditTime != "" {
			b.WriteString("  " + ds.Render("Time: "+c.eventEditTime) + "\n")
		}
		b.WriteString("  " + ps.Render("Duration:") + "\n")
		b.WriteString("  " + ss.Render("1") + ds.Render(" 30min  ") + ss.Render("2") + ds.Render(" 1hr  ") +
			ss.Render("3") + ds.Render(" 1.5hr  ") + ss.Render("4") + ds.Render(" 2hr") + "\n")
		b.WriteString("  " + ds.Render("or type minutes: ") + is.Render(c.eventEditBuf+"\u2588") + "\n")
	case 4: // Location
		b.WriteString("  " + ds.Render("Title: "+c.eventEditTitle) + "\n")
		b.WriteString("  " + ps.Render("Location (optional): ") + is.Render(c.eventEditBuf+"\u2588") + "\n")
	case 5: // Recurrence
		b.WriteString("  " + ds.Render("Title: "+c.eventEditTitle) + "\n")
		b.WriteString("  " + ps.Render("Recurrence:") + "\n")
		b.WriteString("  " + ss.Render("1") + ds.Render(" daily  ") + ss.Render("2") + ds.Render(" weekly  ") +
			ss.Render("3") + ds.Render(" monthly  ") + ss.Render("4") + ds.Render(" yearly") + "\n")
		b.WriteString("  " + ss.Render("0") + ds.Render(" none (one-time event)") + "\n")
	case 6: // Color
		b.WriteString("  " + ds.Render("Title: "+c.eventEditTitle) + "\n")
		b.WriteString("  " + ps.Render("Color:") + "\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(red).Render(ss.Render("1")+" red") + "  ")
		b.WriteString(lipgloss.NewStyle().Foreground(green).Render(ss.Render("2")+" green") + "  ")
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render(ss.Render("3")+" yellow") + "  ")
		b.WriteString(lipgloss.NewStyle().Foreground(mauve).Render(ss.Render("4")+" purple") + "\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(teal).Render(ss.Render("5")+" teal") + "  ")
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Render(ss.Render("6")+" orange") + "  ")
		b.WriteString(ss.Render("0") + ds.Render(" default (blue)") + "\n")
	case 7: // Description
		b.WriteString("  " + ds.Render("Title: "+c.eventEditTitle) + "\n")
		if c.eventEditColor != "" {
			b.WriteString("  " + ds.Render("Color: "+c.eventEditColor) + "\n")
		}
		b.WriteString("  " + ps.Render("Description (optional): ") + is.Render(c.eventEditBuf+"\u2588") + "\n")
		b.WriteString("  " + ds.Render("Enter to save, Esc to cancel"))
	}
	b.WriteString("\n")
}

func (c Calendar) renderConfirmDelete(b *strings.Builder) {
	b.WriteString("\n  " + lipgloss.NewStyle().Foreground(red).Bold(true).Render("Delete this event? (y/n)"))
	b.WriteString("\n")
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

func (c Calendar) renderQuickAdd(b *strings.Builder, width int) {
	b.WriteString("\n")
	promptStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	dateLabel := c.cursor.Format("Jan 2")
	b.WriteString("  " + promptStyle.Render("Add task for "+dateLabel+": "))

	inputStyle := lipgloss.NewStyle().Foreground(text).Background(surface0)
	inputWidth := width - 26
	if inputWidth < 15 {
		inputWidth = 15
	}
	displayInput := c.eventInput
	if len(displayInput) > inputWidth {
		displayInput = displayInput[len(displayInput)-inputWidth:]
	}
	// Show cursor
	cursorChar := lipgloss.NewStyle().Foreground(peach).Background(surface0).Render("_")
	b.WriteString(inputStyle.Render(displayInput) + cursorChar)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter to save, Esc to cancel"))
	b.WriteString("\n")
}

func (c Calendar) renderDateInfo(b *strings.Builder, width int) {
	if c.showEvents {
		b.WriteString("\n")
		evtHeader := lipgloss.NewStyle().Foreground(blue).Bold(true).
			Render("  Events for " + c.cursor.Format("Jan 2"))
		b.WriteString(evtHeader)
		b.WriteString("\n")

		dayEvents := c.eventsForDate(c.cursor)
		if len(dayEvents) == 0 {
			b.WriteString(DimStyle.Render("    No events"))
			b.WriteString("\n")
		} else {
			for _, ev := range dayEvents {
				evCol := calEventColor(ev)
				bullet := lipgloss.NewStyle().Foreground(evCol).Render("  " + IconCalendarChar + " ")
				title := lipgloss.NewStyle().Foreground(text).Bold(true).Render(ev.Title)
				timeStr := ""
				if !ev.AllDay {
					timeStr = ev.Date.Format("15:04")
					if !ev.EndDate.IsZero() {
						timeStr += "-" + ev.EndDate.Format("15:04")
					}
					timeStr = " (" + timeStr + ")"
				} else {
					timeStr = " (all day)"
				}
				timePart := DimStyle.Render(timeStr)
				b.WriteString(bullet + title + timePart)
				b.WriteString("\n")
				if ev.Location != "" {
					b.WriteString(DimStyle.Render("      @ "+ev.Location) + "\n")
				}
				if ev.Recurrence != "" {
					b.WriteString(lipgloss.NewStyle().Foreground(overlay1).Render("      ⟲ "+ev.Recurrence) + "\n")
				}
				if ev.Description != "" {
					desc := ev.Description
					if len(desc) > 60 {
						desc = desc[:57] + "..."
					}
					b.WriteString(DimStyle.Render("      "+desc) + "\n")
				}
			}
		}
	} else {
		dateStr := c.cursor.Format("2006-01-02")
		dayEvents := c.eventsForDate(c.cursor)
		hasNote := c.dailyNoteDates[dateStr]
		tasksDone, tasksTotal := c.taskStats(dateStr)
		dayPlannerBlocks := c.plannerBlocks[dateStr]

		if len(dayEvents) > 0 || hasNote || tasksTotal > 0 || len(dayPlannerBlocks) > 0 {
			b.WriteString("\n")
			dateLabel := lipgloss.NewStyle().Foreground(mauve).Bold(true).
				Render("  " + c.cursor.Format("Mon Jan 2"))
			b.WriteString(dateLabel)
			b.WriteString("\n")
			if hasNote {
				dot := lipgloss.NewStyle().Foreground(green).Render("  " + IconDailyChar + " ")
				b.WriteString(dot + lipgloss.NewStyle().Foreground(text).Render("Daily note"))
				b.WriteString("\n")
			}
			// Show event titles inline (Google Calendar style)
			for i, ev := range dayEvents {
				if i >= 4 {
					more := DimStyle.Render(fmt.Sprintf("    +%d more events", len(dayEvents)-4))
					b.WriteString(more + "\n")
					break
				}
				bullet := lipgloss.NewStyle().Foreground(blue).Render("  " + IconCalendarChar + " ")
				timeStr := ""
				if !ev.AllDay {
					timeStr = ev.Date.Format("15:04") + " "
				}
				timePart := lipgloss.NewStyle().Foreground(overlay1).Render(timeStr)
				title := lipgloss.NewStyle().Foreground(text).Render(ev.Title)
				b.WriteString(bullet + timePart + title)
				if ev.Location != "" {
					b.WriteString(DimStyle.Render(" @ " + ev.Location))
				}
				b.WriteString("\n")
			}
			if len(dayPlannerBlocks) > 0 {
				for i, pb := range dayPlannerBlocks {
					if i >= 3 {
						more := DimStyle.Render(fmt.Sprintf("    +%d more blocks", len(dayPlannerBlocks)-3))
						b.WriteString(more + "\n")
						break
					}
					dot := lipgloss.NewStyle().Foreground(lavender).Render("  ▪ ")
					timeStr := lipgloss.NewStyle().Foreground(overlay1).Render(pb.StartTime + "-" + pb.EndTime + " ")
					b.WriteString(dot + timeStr + lipgloss.NewStyle().Foreground(text).Render(pb.Text) + "\n")
				}
			}
			if tasksTotal > 0 {
				taskColor := yellow
				if tasksDone == tasksTotal {
					taskColor = green
				}
				dot := lipgloss.NewStyle().Foreground(taskColor).Render("  ○ ")
				b.WriteString(dot + lipgloss.NewStyle().Foreground(text).Render(
					fmt.Sprintf("%d/%d tasks done", tasksDone, tasksTotal)))
				b.WriteString("\n")
			}
		}
	}
}

func (c Calendar) renderFooter(b *strings.Builder, width int) {
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")

	var pairs []struct{ Key, Desc string }
	switch c.view {
	case calViewAgenda:
		pairs = []struct{ Key, Desc string }{
			{"j/k", "move"}, {"[]", "month"}, {"w", "view"}, {"t", "today"},
			{"Space", "toggle"}, {"a", "add"}, {"d", "delete"},
			{"Enter", "open"}, {"Esc", "close"},
		}
	case calViewWeek, calView3Day, calView1Day:
		pairs = []struct{ Key, Desc string }{
			{"←→", "day"}, {"↑↓", "hour"}, {"[]", "month"}, {"w", "view"},
			{"t", "today"}, {"a", "add"}, {"b", "block"}, {"e", "events"}, {"Esc", "close"},
		}
	default:
		pairs = []struct{ Key, Desc string }{
			{"hjkl", "nav"}, {"[]", "month"}, {"w", "view"}, {"t", "today"},
			{"a", "add event"}, {"Enter", "open"}, {"e", "events"},
			{"Esc", "close"},
		}
	}
	b.WriteString(RenderHelpBar(pairs))
}

func (c Calendar) renderDayCell(day int, isToday, isCursor, hasNote, hasEvent bool, tasksDone, tasksTotal int, currentMonth, dim, isWeekend bool) string {
	numStr := fmt.Sprintf("%2d", day)
	marker := " "

	// Task count badge for month view
	badge := ""
	if currentMonth && tasksTotal > 0 {
		badgeColor := yellow
		if tasksDone == tasksTotal {
			badgeColor = green
		}
		badge = lipgloss.NewStyle().Foreground(badgeColor).Render(fmt.Sprintf("[%d]", tasksTotal))
	}

	if currentMonth {
		if hasNote && hasEvent {
			marker = lipgloss.NewStyle().Foreground(green).Render("*")
		} else if hasNote {
			marker = lipgloss.NewStyle().Foreground(green).Render("*")
		} else if hasEvent {
			marker = lipgloss.NewStyle().Foreground(blue).Render("*")
		} else if tasksTotal > 0 {
			if tasksDone == tasksTotal {
				marker = lipgloss.NewStyle().Foreground(green).Render("*")
			} else {
				marker = lipgloss.NewStyle().Foreground(yellow).Render("*")
			}
		}
	}

	var styled string
	switch {
	case isCursor && isToday:
		styled = lipgloss.NewStyle().
			Background(green).
			Foreground(crust).
			Bold(true).
			Render(numStr)
	case isToday:
		styled = lipgloss.NewStyle().
			Background(green).
			Foreground(crust).
			Bold(true).
			Render(numStr)
	case isCursor:
		styled = lipgloss.NewStyle().
			Background(peach).
			Foreground(crust).
			Bold(true).
			Render(numStr)
	case !currentMonth || dim:
		styled = DimStyle.Render(numStr)
	case isWeekend:
		if hasNote && hasEvent {
			styled = lipgloss.NewStyle().Foreground(green).Render(numStr)
		} else if hasNote {
			styled = lipgloss.NewStyle().Foreground(green).Render(numStr)
		} else if hasEvent {
			styled = lipgloss.NewStyle().Foreground(blue).Render(numStr)
		} else {
			styled = lipgloss.NewStyle().Foreground(overlay0).Render(numStr)
		}
	case hasNote && hasEvent:
		styled = lipgloss.NewStyle().Foreground(green).Render(numStr)
	case hasNote:
		styled = lipgloss.NewStyle().Foreground(green).Render(numStr)
	case hasEvent:
		styled = lipgloss.NewStyle().Foreground(blue).Render(numStr)
	default:
		styled = lipgloss.NewStyle().Foreground(text).Render(numStr)
	}

	if dim || !currentMonth {
		marker = " "
		badge = ""
	}

	if badge != "" {
		return " " + styled + badge
	}
	return " " + styled + marker
}

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

// ---------------------------------------------------------------------------
// ICS Parsing
// ---------------------------------------------------------------------------

// icsEvent is an intermediate struct with recurrence info.
type icsEvent struct {
	CalendarEvent
	rrule string // raw RRULE value
}

func ParseICSFile(path string) ([]CalendarEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ics file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var parsed []icsEvent
	var current *icsEvent
	inEvent := false

	// Read all lines and unfold (ICS continuation lines start with space/tab)
	scanner := bufio.NewScanner(f)
	var rawLines []string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(rawLines) > 0 {
			rawLines[len(rawLines)-1] += line[1:] // unfold
		} else {
			rawLines = append(rawLines, line)
		}
	}

	for _, line := range rawLines {

		if line == "BEGIN:VEVENT" {
			inEvent = true
			current = &icsEvent{}
			continue
		}

		if line == "END:VEVENT" {
			if inEvent && current != nil {
				parsed = append(parsed, *current)
			}
			inEvent = false
			current = nil
			continue
		}

		if !inEvent || current == nil {
			continue
		}

		key, value := icsKeyValue(line)
		baseProp := icsBaseProp(key)

		switch baseProp {
		case "SUMMARY":
			current.Title = value
		case "LOCATION":
			current.Location = value
		case "DTSTART":
			t, allDay, err := parseICSTime(value)
			if err != nil {
				continue
			}
			current.Date = t
			current.AllDay = allDay
		case "DTEND":
			t, _, err := parseICSTime(value)
			if err != nil {
				continue
			}
			current.EndDate = t
		case "RRULE":
			current.rrule = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read ics file: %w", err)
	}

	// Expand recurring events for the next 90 days
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	horizon := today.AddDate(0, 3, 0)

	var events []CalendarEvent
	for _, ie := range parsed {
		if ie.rrule == "" {
			events = append(events, ie.CalendarEvent)
			continue
		}
		// Parse RRULE
		freq, interval, count, until := parseICSRRule(ie.rrule)
		if freq == "" {
			events = append(events, ie.CalendarEvent)
			continue
		}
		// Generate occurrences
		dur := ie.EndDate.Sub(ie.Date)
		d := ie.Date
		occurrences := 0
		for !d.After(horizon) {
			if count > 0 && occurrences >= count {
				break
			}
			if !until.IsZero() && d.After(until) {
				break
			}
			if !d.Before(today.AddDate(0, -1, 0)) {
				ev := ie.CalendarEvent
				ev.Date = time.Date(d.Year(), d.Month(), d.Day(),
					ie.Date.Hour(), ie.Date.Minute(), 0, 0, time.Local)
				ev.EndDate = ev.Date.Add(dur)
				events = append(events, ev)
			}
			occurrences++
			switch freq {
			case "DAILY":
				d = d.AddDate(0, 0, interval)
			case "WEEKLY":
				d = d.AddDate(0, 0, 7*interval)
			case "MONTHLY":
				d = d.AddDate(0, interval, 0)
			case "YEARLY":
				d = d.AddDate(interval, 0, 0)
			default:
				d = horizon.AddDate(0, 0, 1) // break
			}
		}
	}

	return events, nil
}

func parseICSRRule(rrule string) (freq string, interval int, count int, until time.Time) {
	interval = 1 // default interval
	for _, part := range strings.Split(rrule, ";") {
		switch {
		case strings.HasPrefix(part, "FREQ="):
			freq = strings.TrimPrefix(part, "FREQ=")
		case strings.HasPrefix(part, "INTERVAL="):
			if n, err := strconv.Atoi(strings.TrimPrefix(part, "INTERVAL=")); err == nil && n > 0 {
				interval = n
			}
		case strings.HasPrefix(part, "COUNT="):
			if n, err := strconv.Atoi(strings.TrimPrefix(part, "COUNT=")); err == nil && n > 0 {
				count = n
			}
		case strings.HasPrefix(part, "UNTIL="):
			val := strings.TrimPrefix(part, "UNTIL=")
			if t, _, err := parseICSTime(val); err == nil {
				until = t
			}
		}
	}
	return
}

func icsKeyValue(line string) (string, string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return line, ""
	}
	return line[:idx], line[idx+1:]
}

func icsBaseProp(key string) string {
	if idx := strings.Index(key, ";"); idx >= 0 {
		return key[:idx]
	}
	return key
}

func parseICSTime(value string) (time.Time, bool, error) {
	value = strings.TrimSpace(value)
	// UTC format: 20060102T150405Z
	if t, err := time.Parse("20060102T150405Z", value); err == nil {
		return t.Local(), false, nil
	}
	// Local format: 20060102T150405
	if t, err := time.Parse("20060102T150405", value); err == nil {
		return t, false, nil
	}
	// Date only: 20060102
	if t, err := time.Parse("20060102", value); err == nil {
		return t, true, nil
	}
	// ISO format: 2006-01-02T15:04:05Z
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Local(), false, nil
	}
	// ISO without timezone: 2006-01-02T15:04:05
	if t, err := time.Parse("2006-01-02T15:04:05", value); err == nil {
		return t, false, nil
	}
	// ISO date: 2006-01-02
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, true, nil
	}
	return time.Time{}, false, fmt.Errorf("unrecognized ICS time format: %q", value)
}
// UI configuration updated.
// Grid/Timeline UI implemented
