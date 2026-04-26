package tui

import (
	"encoding/json"
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
	OverlayBase

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

	// Quick search across events. '/' opens an input bar that
	// types a fuzzy filter; the query persists across view
	// switches and renders as a chip in the title bar. Empty
	// query means no filter (all events shown).
	searching   bool
	searchQuery string

	// Jump-to-date prompt. 'J' opens; accepts YYYY-MM-DD,
	// "today"/"tomorrow"/weekday names ("monday", "next monday"),
	// or relative offsets ("+7d", "-3w", "+2m", "+1y"). On Enter
	// the cursor + viewing both move to the parsed date.
	jumpingDate bool
	jumpDateBuf string

	// Find-next-free-slot picker. 'n' opens a duration prompt
	// (step 0); pressing 1-8 picks a duration and computes the
	// next free slots in the user's working hours over the next
	// 14 days (step 1). Enter on a slot moves the cursor and
	// opens the new-event form pre-filled with that time.
	findingSlot      bool
	findSlotStep     int        // 0=duration prompt, 1=results list
	findSlotDuration int        // chosen duration in minutes
	findSlotResults  []freeSlot // top matches in next 14 days
	findSlotCursor   int        // result-list cursor

	// Help overlay. '?' toggles a full cheat sheet of all
	// calendar bindings — without it the 30+ keys are
	// undiscoverable. Any key dismisses.
	showingHelp bool

	// Working-hours config — drives findFreeSlots and the
	// non-working-hours background tint in the week grid.
	// Persisted to .granit/calendar-config.json so power users
	// don't have to retune every launch. Sensible defaults
	// (09:00–18:00) apply when zero/unset.
	workStartHour int
	workEndHour   int

	// Layer toggles — let the user focus on just events, just
	// tasks, or just planner blocks at a glance. Default to
	// all-on. Persisted alongside working hours so the chosen
	// view survives restarts. NOTE: showEvents above is the
	// pre-existing event sub-panel toggle; these layer toggles
	// are independent and gate which sources contribute to the
	// week grid + agenda views.
	showEventsLayer   bool
	showTasksLayer    bool
	showPlannerLayer  bool

	// Working-hours / layer-toggle prompts. 'H' opens a small
	// input bar to set hours ("9-18" or "08:00-19:00"); 'L'
	// opens a layer-toggle prompt where E/T/B flip each layer.
	editingWorkHours bool
	workHoursBuf     string
	layerPrompt      bool

	// Event templates — power users have recurring meeting
	// types ("1:1 weekly", "standup", "deep work block"). 'X'
	// saves the event under the cursor as a template; 'T'
	// opens the picker; selecting one seeds the new-event form
	// the next time the user presses 'a'. Persisted to
	// .granit/calendar-templates.json.
	eventTemplates       []eventTemplate
	templatePicker       bool
	templatePickerCursor int
	templateSavePrompt   bool   // 'X' opened: prompt for template name
	templateNameBuf      string // template name being typed during save
	templateSaveSource   *eventTemplate
	pendingTemplateSeed  *eventTemplate // consumed on next 'a'
}

// eventTemplate is a reusable event blueprint. Only the Name +
// Title are required; the rest are optional pre-fills for the
// new-event form. Persisted to .granit/calendar-templates.json.
type eventTemplate struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	DurationMin int    `json:"duration_min,omitempty"`
	Location    string `json:"location,omitempty"`
	Color       string `json:"color,omitempty"`
	Recurrence  string `json:"recurrence,omitempty"`
	Description string `json:"description,omitempty"`
}

// calendarConfig is the on-disk schema for working-hours +
// layer toggles. Kept as a flat struct so future fields can
// land via JSON additive merges.
type calendarConfig struct {
	WorkStartHour    int  `json:"work_start_hour"`
	WorkEndHour      int  `json:"work_end_hour"`
	ShowEventsLayer  bool `json:"show_events_layer"`
	ShowTasksLayer   bool `json:"show_tasks_layer"`
	ShowPlannerLayer bool `json:"show_planner_layer"`
}

// freeSlot is a contiguous block of time with no events or
// planner blocks scheduled. Returned by findFreeSlots and
// rendered in the find-slot picker.
type freeSlot struct {
	Start time.Time
	End   time.Time
}

// NewCalendar creates a new Calendar overlay.
func NewCalendar() Calendar {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	return Calendar{
		cursor:           today,
		viewing:          today,
		today:            today,
		dailyNoteDates:   make(map[string]bool),
		tasks:            make(map[string][]TaskItem),
		plannerBlocks:    make(map[string][]PlannerBlock),
		// Layers default ON so behavior matches pre-toggle world
		// for fresh installs; loadCalendarConfig overrides if
		// the user persisted explicit values last session.
		showEventsLayer:  true,
		showTasksLayer:   true,
		showPlannerLayer: true,
	}
}

func (c *Calendar) SetActiveGoals(goals []Goal) { c.activeGoals = goals }
func (c *Calendar) SetVaultRoot(root string) {
	c.vaultRoot = root
	c.loadCalendarConfig()
	c.loadEventTemplates()
}

// loadCalendarConfig restores working-hours + layer toggles from
// .granit/calendar-config.json. Missing file silently falls back
// to defaults (09:00–18:00 work, all layers on).
func (c *Calendar) loadCalendarConfig() {
	if c.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(c.vaultRoot, ".granit", "calendar-config.json"))
	if err != nil {
		return
	}
	var cfg calendarConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}
	c.workStartHour = cfg.WorkStartHour
	c.workEndHour = cfg.WorkEndHour
	c.showEventsLayer = cfg.ShowEventsLayer
	c.showTasksLayer = cfg.ShowTasksLayer
	c.showPlannerLayer = cfg.ShowPlannerLayer
}

// saveCalendarConfig persists working-hours + layer toggles. The
// sidecar lives in .granit/ (not the vault) so it doesn't pollute
// markdown listings or sync conflicts.
func (c *Calendar) saveCalendarConfig() {
	if c.vaultRoot == "" {
		return
	}
	dir := filepath.Join(c.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0o755)
	cfg := calendarConfig{
		WorkStartHour:    c.workStartHour,
		WorkEndHour:      c.workEndHour,
		ShowEventsLayer:  c.showEventsLayer,
		ShowTasksLayer:   c.showTasksLayer,
		ShowPlannerLayer: c.showPlannerLayer,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteNote(filepath.Join(dir, "calendar-config.json"), string(data))
}

// loadEventTemplates restores the user's named event templates.
// Missing file leaves the slice nil — picker shows an empty state.
func (c *Calendar) loadEventTemplates() {
	if c.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(c.vaultRoot, ".granit", "calendar-templates.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &c.eventTemplates)
}

// saveEventTemplates persists the templates list.
func (c *Calendar) saveEventTemplates() {
	if c.vaultRoot == "" {
		return
	}
	dir := filepath.Join(c.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(c.eventTemplates, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteNote(filepath.Join(dir, "calendar-templates.json"), string(data))
}

// effectiveWorkHours returns the working-hour range in use,
// falling back to 09–18 when no explicit config has been set.
// Centralised so renderers and findFreeSlots agree.
func (c Calendar) effectiveWorkHours() (int, int) {
	start, end := c.workStartHour, c.workEndHour
	if start <= 0 || end <= 0 || end <= start {
		start, end = 9, 18
	}
	return start, end
}

// parseWorkHoursInput parses "9-18" / "08:00-19:00" / "8-19" and
// returns (startHour, endHour, ok). Hours-only or HH:MM (minutes
// dropped) accepted — minutes don't make sense for grid tinting,
// the granularity is one hour.
func parseWorkHoursInput(s string) (int, int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, 0, false
	}
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	parseHour := func(p string) (int, bool) {
		p = strings.TrimSpace(p)
		if i := strings.Index(p, ":"); i >= 0 {
			p = p[:i]
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 || n > 23 {
			return 0, false
		}
		return n, true
	}
	a, ok1 := parseHour(parts[0])
	b, ok2 := parseHour(parts[1])
	if !ok1 || !ok2 || b <= a {
		return 0, 0, false
	}
	return a, b, true
}

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
	c.Activate()
	c.selected = ""
	c.showEvents = false
	c.addingEvent = false
	c.eventInput = ""
	c.agendaScroll = 0
	c.agendaCursor = 0
	c.agendaItems = nil
	c.taskToggles = nil
	// Reset transient prompts so reopening lands in a clean
	// state — a stuck templatePicker from a prior session
	// would steal the first keystroke otherwise.
	c.editingWorkHours = false
	c.workHoursBuf = ""
	c.layerPrompt = false
	c.templatePicker = false
	c.templateSavePrompt = false
	c.templateNameBuf = ""
	c.templateSaveSource = nil
	c.pendingTemplateSeed = nil
	now := time.Now()
	c.today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	c.cursor = c.today
	c.viewing = c.today
}

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
		// Help overlay — any key dismisses, mirroring TaskManager's
		// pattern. Must run before other input modes so '?' from
		// inside an active help screen toggles cleanly.
		if c.showingHelp {
			c.showingHelp = false
			return c, nil
		}

		// Quick-search input mode — '/' enters this; the buffer
		// live-filters which events render. Esc cancels and
		// clears; Enter commits and exits the input bar (the
		// query stays applied; 'c' or '/'+empty clears it).
		if c.searching {
			switch msg.String() {
			case "esc":
				c.searching = false
				c.searchQuery = ""
			case "enter":
				c.searching = false
			case "backspace":
				if len(c.searchQuery) > 0 {
					c.searchQuery = TrimLastRune(c.searchQuery)
				}
			default:
				if len(msg.String()) == 1 || msg.String() == " " {
					c.searchQuery += msg.String()
				}
			}
			// Rebuild agenda so the filtered list reflects the
			// new query on the very next render — without this,
			// the search bar updates but the visible items lag
			// behind until the user moves the cursor.
			c.rebuildAgendaItems()
			return c, nil
		}

		// Jump-to-date input mode — 'J' enters this; Enter
		// parses the buffer and moves the cursor.
		if c.jumpingDate {
			switch msg.String() {
			case "esc":
				c.jumpingDate = false
				c.jumpDateBuf = ""
			case "enter":
				if t, ok := parseJumpDate(c.jumpDateBuf, c.today); ok {
					c.cursor = t
					c.viewing = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
				}
				c.jumpingDate = false
				c.jumpDateBuf = ""
			case "backspace":
				if len(c.jumpDateBuf) > 0 {
					c.jumpDateBuf = TrimLastRune(c.jumpDateBuf)
				}
			default:
				if len(msg.String()) == 1 {
					c.jumpDateBuf += msg.String()
				}
			}
			return c, nil
		}

		// Find-next-free-slot picker — 'n' enters; step 0 is
		// the duration prompt, step 1 is the results list.
		if c.findingSlot {
			switch c.findSlotStep {
			case 0:
				durMap := map[string]int{
					"1": 15, "2": 30, "3": 45,
					"4": 60, "5": 90, "6": 120,
					"7": 180, "8": 240,
				}
				if msg.String() == "esc" {
					c.findingSlot = false
					return c, nil
				}
				if dur, ok := durMap[msg.String()]; ok {
					c.findSlotDuration = dur
					sh, eh := c.effectiveWorkHours()
					c.findSlotResults = c.findFreeSlots(dur, 14, sh, eh, 5)
					c.findSlotCursor = 0
					// Stay in the picker even with zero results so
					// the user sees the "no slots" message instead
					// of a silent dismiss that's easy to miss.
					c.findSlotStep = 1
				}
			case 1:
				switch msg.String() {
				case "esc":
					c.findingSlot = false
				case "up", "k":
					if c.findSlotCursor > 0 {
						c.findSlotCursor--
					}
				case "down", "j":
					if c.findSlotCursor < len(c.findSlotResults)-1 {
						c.findSlotCursor++
					}
				case "enter":
					if c.findSlotCursor < len(c.findSlotResults) {
						slot := c.findSlotResults[c.findSlotCursor]
						c.cursor = time.Date(slot.Start.Year(), slot.Start.Month(), slot.Start.Day(), 0, 0, 0, 0, time.Local)
						c.viewing = time.Date(slot.Start.Year(), slot.Start.Month(), 1, 0, 0, 0, 0, time.Local)
						// Pre-fill the new-event form with this slot.
						c.eventEditMode = 1
						c.eventEditField = efTitle
						c.eventEditID = ""
						c.eventEditTitle = ""
						c.eventEditTime = slot.Start.Format("15:04")
						c.eventEditDur = c.findSlotDuration
						c.eventEditDurBuf = strconv.Itoa(c.findSlotDuration)
						c.eventEditLoc = ""
						c.eventEditRecur = ""
						c.eventEditColor = ""
						c.eventEditDesc = ""
					}
					c.findingSlot = false
				}
			}
			return c, nil
		}

		// Working-hours input mode — 'H' enters; accepts
		// "9-18" or "08:00-19:00". Enter commits + persists,
		// Esc cancels. Bad input leaves prior values intact.
		if c.editingWorkHours {
			switch msg.String() {
			case "esc":
				c.editingWorkHours = false
				c.workHoursBuf = ""
			case "enter":
				if a, b, ok := parseWorkHoursInput(c.workHoursBuf); ok {
					c.workStartHour = a
					c.workEndHour = b
					c.saveCalendarConfig()
				}
				c.editingWorkHours = false
				c.workHoursBuf = ""
			case "backspace":
				if len(c.workHoursBuf) > 0 {
					c.workHoursBuf = TrimLastRune(c.workHoursBuf)
				}
			default:
				if len(msg.String()) == 1 {
					c.workHoursBuf += msg.String()
				}
			}
			return c, nil
		}

		// Layer-toggle prompt — 'L' opens; E/T/B flip events,
		// tasks, planner blocks. Esc / any other key dismisses.
		// Persisted on every flip so the toggle sticks.
		if c.layerPrompt {
			switch msg.String() {
			case "e", "E":
				c.showEventsLayer = !c.showEventsLayer
				c.saveCalendarConfig()
				if c.view == calViewAgenda {
					c.rebuildAgendaItems()
				}
			case "t", "T":
				c.showTasksLayer = !c.showTasksLayer
				c.saveCalendarConfig()
				if c.view == calViewAgenda {
					c.rebuildAgendaItems()
				}
			case "b", "B":
				c.showPlannerLayer = !c.showPlannerLayer
				c.saveCalendarConfig()
				if c.view == calViewAgenda {
					c.rebuildAgendaItems()
				}
			default:
				// Any other key dismisses the prompt — the
				// flip-keys are the only "stay open" path.
				c.layerPrompt = false
			}
			return c, nil
		}

		// Template picker — 'T' opens; up/down move, Enter
		// seeds the new-event form via pendingTemplateSeed.
		if c.templatePicker {
			switch msg.String() {
			case "esc":
				c.templatePicker = false
			case "up", "k":
				if c.templatePickerCursor > 0 {
					c.templatePickerCursor--
				}
			case "down", "j":
				if c.templatePickerCursor < len(c.eventTemplates)-1 {
					c.templatePickerCursor++
				}
			case "enter":
				if c.templatePickerCursor < len(c.eventTemplates) {
					tmpl := c.eventTemplates[c.templatePickerCursor]
					c.openEventFormFromTemplate(&tmpl)
				}
				c.templatePicker = false
			case "d", "D":
				// Quick-delete from picker so users can prune
				// stale templates without hand-editing JSON.
				if c.templatePickerCursor < len(c.eventTemplates) {
					c.eventTemplates = append(
						c.eventTemplates[:c.templatePickerCursor],
						c.eventTemplates[c.templatePickerCursor+1:]...)
					c.saveEventTemplates()
					if c.templatePickerCursor >= len(c.eventTemplates) && c.templatePickerCursor > 0 {
						c.templatePickerCursor--
					}
				}
			}
			return c, nil
		}

		// Template save prompt — 'X' opens with the cursor's
		// event preloaded; user types a name and presses Enter
		// to add to templates list.
		if c.templateSavePrompt {
			switch msg.String() {
			case "esc":
				c.templateSavePrompt = false
				c.templateNameBuf = ""
				c.templateSaveSource = nil
			case "enter":
				name := strings.TrimSpace(c.templateNameBuf)
				if name != "" && c.templateSaveSource != nil {
					tmpl := *c.templateSaveSource
					tmpl.Name = name
					c.eventTemplates = append(c.eventTemplates, tmpl)
					c.saveEventTemplates()
				}
				c.templateSavePrompt = false
				c.templateNameBuf = ""
				c.templateSaveSource = nil
			case "backspace":
				if len(c.templateNameBuf) > 0 {
					c.templateNameBuf = TrimLastRune(c.templateNameBuf)
				}
			default:
				if len(msg.String()) == 1 || msg.String() == " " {
					c.templateNameBuf += msg.String()
				}
			}
			return c, nil
		}

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
			// If a template was just picked, seed the form from it instead
			// of the empty defaults.
			c.openNewEventForm()

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

		// 'H' opens the working-hours prompt. Lowercase 'h' is
		// already left-arrow nav, and 'W' (the natural mnemonic
		// for "working hours") is taken by view-cycle-back, so
		// 'H' wins on availability + mnemonic ("Hours").
		case "H":
			c.editingWorkHours = true
			start, end := c.effectiveWorkHours()
			c.workHoursBuf = fmt.Sprintf("%d-%d", start, end)

		// 'L' opens the layer-toggle prompt. E/T/B inside the
		// prompt flip events / tasks / planner blocks. Persists
		// on every flip so the next launch remembers the focus.
		case "L":
			c.layerPrompt = true

		// 'T' opens the template picker. Capital so it doesn't
		// conflict with 't' = jump to today. Picking a template
		// seeds the new-event form for the next 'a' press.
		case "T":
			c.templatePicker = true
			c.templatePickerCursor = 0

		// 'X' saves the event under the cursor as a template.
		// Only meaningful in time-grid views with the cursor on
		// an actual event — silent no-op otherwise.
		case "X":
			if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
				c.openTemplateSavePrompt()
			}

		// Quick search: '/' opens an input bar; types live-filter
		// events. The query persists across view switches.
		case "/":
			c.searching = true
			// Don't reset the buffer — re-pressing '/' lets the
			// user refine the existing query.

		// Clear active search query (mirror of TaskManager 'c').
		case "c":
			if c.searchQuery != "" {
				c.searchQuery = ""
				c.rebuildAgendaItems()
			}

		// Jump to date prompt: J → "2026-05-15" / "next monday" / "+7d"
		case "J":
			c.jumpingDate = true
			c.jumpDateBuf = ""

		// Find next free slot: opens duration prompt, then results.
		case "n":
			c.findingSlot = true
			c.findSlotStep = 0
			c.findSlotDuration = 0
			c.findSlotResults = nil
			c.findSlotCursor = 0

		// Help overlay — full cheat sheet of bindings.
		case "?":
			c.showingHelp = true

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
	// Day view renders a fixed 06:00-22:30 half-hour grid.
	if c.view == calView1Day {
		return 34
	}
	startHour, endHour := c.weekGridHourRangeFor()
	slots := (endHour - startHour) * 2
	if slots < 16 {
		slots = 16
	}
	return slots
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

// openNewEventForm opens the event creation wizard. When a
// template is queued via pendingTemplateSeed (set by the picker),
// the form fields are seeded from it and the seed is consumed.
// Otherwise the form opens with empty/default values, with the
// time pre-filled from the grid cursor in time-grid views.
func (c *Calendar) openNewEventForm() {
	c.eventEditMode = 1
	c.eventEditField = efTitle
	c.eventEditID = ""

	if c.pendingTemplateSeed != nil {
		t := c.pendingTemplateSeed
		c.eventEditTitle = t.Title
		c.eventEditDur = t.DurationMin
		if c.eventEditDur <= 0 {
			c.eventEditDur = 60
		}
		c.eventEditDurBuf = strconv.Itoa(c.eventEditDur)
		c.eventEditLoc = t.Location
		c.eventEditRecur = t.Recurrence
		c.eventEditColor = t.Color
		c.eventEditDesc = t.Description
		c.eventEditTime = ""
		c.pendingTemplateSeed = nil
	} else {
		c.eventEditTitle = ""
		c.eventEditTime = ""
		c.eventEditDur = 60
		c.eventEditDurBuf = "60"
		c.eventEditLoc = ""
		c.eventEditRecur = ""
		c.eventEditColor = ""
		c.eventEditDesc = ""
	}

	// Time-grid views: use the cursor row as the start time
	// when the template didn't pin one. The previous all-day
	// default forced retyping the time on every quick-add.
	if c.view == calViewWeek || c.view == calView3Day || c.view == calView1Day {
		sH := c.weekGridStartHourFor()
		cursorMin := (sH+c.weekGridCursorHour/2)*60 + (c.weekGridCursorHour%2)*30
		c.eventEditTime = fmt.Sprintf("%02d:%02d", cursorMin/60, cursorMin%60)
	}
}

// openTemplateSavePrompt scans the event under the grid cursor
// and, if found, opens a name-input prompt to add it to the
// templates list. Silent no-op when no event is present.
func (c *Calendar) openTemplateSavePrompt() {
	cursorDay := c.cursor
	sH := c.weekGridStartHourFor()
	cursorSlotMin := (sH+c.weekGridCursorHour/2)*60 + (c.weekGridCursorHour%2)*30
	for _, ev := range c.eventsForDate(cursorDay) {
		if ev.AllDay {
			continue
		}
		evStart := ev.Date.Hour()*60 + ev.Date.Minute()
		evEnd := evStart + 60
		if !ev.EndDate.IsZero() {
			evEnd = ev.EndDate.Hour()*60 + ev.EndDate.Minute()
		}
		if evStart < cursorSlotMin+30 && evEnd > cursorSlotMin {
			dur := 60
			if !ev.EndDate.IsZero() {
				dur = int(ev.EndDate.Sub(ev.Date).Minutes())
			}
			c.templateSaveSource = &eventTemplate{
				Title:       ev.Title,
				DurationMin: dur,
				Location:    ev.Location,
				Color:       ev.Color,
				Recurrence:  ev.Recurrence,
				Description: ev.Description,
			}
			c.templateSavePrompt = true
			c.templateNameBuf = ev.Title // pre-fill with title as a sane default name
			return
		}
	}
}

// openEventFormFromTemplate stages a template for the next 'a'
// press and fires it immediately so the user lands in the form
// with template fields already populated.
func (c *Calendar) openEventFormFromTemplate(t *eventTemplate) {
	c.pendingTemplateSeed = t
	c.openNewEventForm()
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
	// Help overlay takes the full pane — no point rendering
	// the calendar body underneath since the user is in
	// "show me the keys" mode.
	if c.showingHelp {
		return c.renderHelpOverlay()
	}
	var body string
	switch c.view {
	case calViewWeek:
		body = c.viewWeek()
	case calView3Day:
		body = c.viewWeek() // reuse week grid for 3-day view
	case calView1Day:
		body = c.view1Day()
	case calViewAgenda:
		body = c.viewAgenda()
	case calViewYear:
		body = c.viewYear()
	default:
		body = c.viewMonth()
	}
	// Active prompts: search bar / jump-date / find-slot picker
	// land at the bottom of the view so they don't bump the
	// existing layout. Mutually exclusive — only one at a time.
	if c.findingSlot {
		return body + "\n" + c.renderFindSlotPrompt()
	}
	if c.jumpingDate {
		return body + "\n" + c.renderJumpDatePrompt()
	}
	if c.searching {
		return body + "\n" + c.renderSearchPrompt()
	}
	if c.editingWorkHours {
		return body + "\n" + c.renderWorkHoursPrompt()
	}
	if c.layerPrompt {
		return body + "\n" + c.renderLayerPrompt()
	}
	if c.templatePicker {
		return body + "\n" + c.renderTemplatePicker()
	}
	if c.templateSavePrompt {
		return body + "\n" + c.renderTemplateSavePrompt()
	}
	// Idle state: if a search query is active (committed but not
	// open), show a compact reminder + match count so the user
	// always knows the filter is on AND how many it caught.
	if c.searchQuery != "" {
		matches := c.countSearchMatches()
		hint := fmt.Sprintf("/ search: %s  %d match", c.searchQuery, matches)
		if matches != 1 {
			hint += "es"
		}
		hint += "  (c to clear)"
		return body + "\n  " + DimStyle.Render(hint)
	}
	return body
}

// countSearchMatches returns the number of events + planner
// blocks + tasks across the next 14 days whose title matches
// the active search query. Used for the idle-state hint.
func (c Calendar) countSearchMatches() int {
	if c.searchQuery == "" {
		return 0
	}
	count := 0
	for d := 0; d < 14; d++ {
		day := c.today.AddDate(0, 0, d)
		dateStr := day.Format("2006-01-02")
		for _, ev := range c.eventsForDate(day) {
			if c.matchesSearch(ev.Title, ev.Location, ev.Description) {
				count++
			}
		}
		for _, pb := range c.plannerBlocks[dateStr] {
			if c.matchesSearch(pb.Text) {
				count++
			}
		}
		for _, t := range c.tasks[dateStr] {
			if c.matchesSearch(t.Text) {
				count++
			}
		}
	}
	return count
}

// renderSearchPrompt draws the live-typing bar for '/'.
func (c Calendar) renderSearchPrompt() string {
	cursor := lipgloss.NewStyle().Foreground(mauve).Render("▏")
	label := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("/ search ")
	hint := DimStyle.Render("  (Enter commit, Esc cancel)")
	return "  " + label + c.searchQuery + cursor + hint
}

// renderJumpDatePrompt draws the live-typing bar for 'J'.
func (c Calendar) renderJumpDatePrompt() string {
	cursor := lipgloss.NewStyle().Foreground(mauve).Render("▏")
	label := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("J jump → ")
	hint := DimStyle.Render("  (YYYY-MM-DD / today / monday / next monday / +7d / -3w)")
	return "  " + label + c.jumpDateBuf + cursor + hint
}

// renderWorkHoursPrompt draws the input bar for 'H'. Shows the
// current effective range as a hint so the user can confirm
// their persisted setting before retyping.
func (c Calendar) renderWorkHoursPrompt() string {
	cursor := lipgloss.NewStyle().Foreground(mauve).Render("▏")
	label := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("H working hours → ")
	sh, eh := c.effectiveWorkHours()
	hint := DimStyle.Render(fmt.Sprintf("  (current %02d:00-%02d:00; format \"9-18\" or \"08:00-19:00\"; Esc cancel)", sh, eh))
	return "  " + label + c.workHoursBuf + cursor + hint
}

// renderLayerPrompt draws the layer-toggle prompt. Shows the
// current state of each layer with a [x] / [ ] marker so the
// user always knows which keys flip what.
func (c Calendar) renderLayerPrompt() string {
	mark := func(on bool) string {
		if on {
			return lipgloss.NewStyle().Foreground(green).Bold(true).Render("[x]")
		}
		return lipgloss.NewStyle().Foreground(surface2).Render("[ ]")
	}
	label := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("L layers")
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	parts := []string{
		keyStyle.Render("E") + ":" + mark(c.showEventsLayer) + " events",
		keyStyle.Render("T") + ":" + mark(c.showTasksLayer) + " tasks",
		keyStyle.Render("B") + ":" + mark(c.showPlannerLayer) + " planner blocks",
	}
	hint := DimStyle.Render("  (any other key closes)")
	return "  " + label + "  " + strings.Join(parts, "  ") + hint
}

// renderTemplatePicker draws the template-list popup. Shows
// up to 10 templates with the cursor highlighted; cursored row
// also previews duration/location/recurrence so the user can
// pick by detail without committing first.
func (c Calendar) renderTemplatePicker() string {
	var b strings.Builder
	header := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("T event templates — Enter:use  d:delete  Esc:cancel")
	b.WriteString("  " + header + "\n")
	if len(c.eventTemplates) == 0 {
		b.WriteString("  " + DimStyle.Render("No templates yet. Press X on an event in week/day view to save one."))
		return b.String()
	}
	maxShow := 10
	start := 0
	if c.templatePickerCursor >= maxShow {
		start = c.templatePickerCursor - maxShow + 1
	}
	end := start + maxShow
	if end > len(c.eventTemplates) {
		end = len(c.eventTemplates)
	}
	for i := start; i < end; i++ {
		t := c.eventTemplates[i]
		marker := "  "
		nameStyle := lipgloss.NewStyle().Foreground(text)
		if i == c.templatePickerCursor {
			marker = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▸ ")
			nameStyle = lipgloss.NewStyle().Foreground(text).Bold(true)
		}
		details := ""
		if t.DurationMin > 0 {
			details += fmt.Sprintf(" %dm", t.DurationMin)
		}
		if t.Location != "" {
			details += " @" + t.Location
		}
		if t.Recurrence != "" {
			details += " ⟲" + t.Recurrence
		}
		b.WriteString("  " + marker + nameStyle.Render(t.Name) +
			DimStyle.Render(" — "+t.Title) + DimStyle.Render(details) + "\n")
	}
	return b.String()
}

// renderTemplateSavePrompt draws the name-input bar for 'X'.
func (c Calendar) renderTemplateSavePrompt() string {
	cursor := lipgloss.NewStyle().Foreground(mauve).Render("▏")
	label := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("X save template → ")
	srcTitle := ""
	if c.templateSaveSource != nil {
		srcTitle = c.templateSaveSource.Title
	}
	hint := DimStyle.Render(fmt.Sprintf("  (from \"%s\"; Enter save, Esc cancel)", srcTitle))
	return "  " + label + c.templateNameBuf + cursor + hint
}

// renderHelpOverlay returns a full-pane cheat sheet of every
// calendar binding, grouped by purpose. Mirrors TaskManager's
// help renderer so the two surfaces feel uniform.
func (c Calendar) renderHelpOverlay() string {
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true).Width(14)
	descStyle := lipgloss.NewStyle().Foreground(text)
	sectionStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)

	var b strings.Builder
	b.WriteString("\n  " + titleStyle.Render("📅 Calendar Keyboard Shortcuts") + "\n\n")

	sections := []struct {
		title string
		keys  [][2]string
	}{
		{"Navigation", [][2]string{
			{"h / l", "Previous / next day"},
			{"j / k", "Down / up (week & agenda); ±week (month)"},
			{"[ / ]", "Previous / next month"},
			{"{ / }", "Previous / next year"},
			{"t", "Jump to today"},
			{"J", "Jump-to-date prompt (YYYY-MM-DD / today / monday / next monday / +7d / 15)"},
		}},
		{"Views", [][2]string{
			{"w / v", "Cycle view forward (Month → Week → 3d → Day → Agenda → Year)"},
			{"W / V", "Cycle view backward"},
			{"y", "Toggle year view"},
		}},
		{"Events", [][2]string{
			{"a", "Add event (full form: title, time, duration, location, recur, color)"},
			{"e", "Edit event under cursor (time-grid)"},
			{"d", "Delete event (with confirm)"},
			{"D", "Unschedule planner block under cursor"},
			{", / .", "Shift planner block −15min / +15min"},
			{"b", "Time-block: pick task to schedule into cursor hour"},
			{"X", "Save event under cursor as a reusable template"},
			{"T", "Open template picker — pick one then 'a' creates from it"},
			{"g", "Add weekly milestone to a goal"},
		}},
		{"Search · Jump · Find slot", [][2]string{
			{"/", "Quick search (live-filter agenda; dim non-matches in week/month)"},
			{"c", "Clear active search query"},
			{"n", "Find next free slot (1-8 → 15m..4h, then Enter to schedule)"},
			{"?", "This help"},
		}},
		{"Layout · Layers · Hours", [][2]string{
			{"H", "Set working hours (format \"9-18\" or \"08:00-19:00\")"},
			{"L", "Toggle layers prompt — E events / T tasks / B planner blocks"},
		}},
		{"Conflict awareness", [][2]string{
			{"⚠+N badge", "Red tint marks overlap; +N counts the other entries in that slot"},
		}},
	}

	for _, sec := range sections {
		b.WriteString("  " + sectionStyle.Render(sec.title) + "\n")
		for _, kv := range sec.keys {
			b.WriteString("    " + keyStyle.Render(kv[0]) + descStyle.Render(kv[1]) + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("  " + DimStyle.Render("Press any key to close"))
	return b.String()
}

// renderFindSlotPrompt draws either the duration menu (step 0)
// or the results list (step 1) for the find-free-slot picker.
func (c Calendar) renderFindSlotPrompt() string {
	if c.findSlotStep == 0 {
		label := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("n find next free slot")
		menu := "  1=15m  2=30m  3=45m  4=1h  5=90m  6=2h  7=3h  8=4h"
		hint := DimStyle.Render("  (Esc cancel)")
		return "  " + label + "\n  " + menu + hint
	}
	var b strings.Builder
	header := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render(fmt.Sprintf("n free slots (%dm) — Enter to schedule, Esc cancel", c.findSlotDuration))
	b.WriteString("  " + header + "\n")
	if len(c.findSlotResults) == 0 {
		// Honest empty state — tell the user the search window
		// is exhausted so they don't think the picker broke.
		sh, eh := c.effectiveWorkHours()
		b.WriteString("  " + lipgloss.NewStyle().Foreground(red).
			Render(fmt.Sprintf("No free slots in the next 14 days (%02d:00–%02d:00). Try a shorter duration, change H, or clear blocks.", sh, eh)))
		return b.String()
	}
	for i, slot := range c.findSlotResults {
		marker := "  "
		line := fmt.Sprintf("%s — %s",
			slot.Start.Format("Mon Jan 2"),
			slot.Start.Format("15:04")+"-"+slot.End.Format("15:04"))
		if i == c.findSlotCursor {
			marker = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▸ ")
			line = lipgloss.NewStyle().Foreground(text).Bold(true).Render(line)
		} else {
			line = lipgloss.NewStyle().Foreground(text).Render(line)
		}
		b.WriteString("  " + marker + line + "\n")
	}
	return b.String()
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

// matchesSearch reports whether the haystack passes the active
// search filter. Empty query matches everything. Match is a
// case-insensitive substring; the variadic form lets callers
// pass title + location + description so "/zoom" finds all
// online meetings even when the URL is in description.
func (c Calendar) matchesSearch(haystacks ...string) bool {
	if c.searchQuery == "" {
		return true
	}
	q := strings.ToLower(c.searchQuery)
	for _, h := range haystacks {
		if h != "" && strings.Contains(strings.ToLower(h), q) {
			return true
		}
	}
	return false
}

// parseJumpDate converts a free-form date expression into an
// absolute time.Time. Supported formats (anchored at `today`):
//
//	2026-05-15        → that date
//	today / tomorrow / yesterday
//	monday … sunday   → next occurrence (today if it's that day)
//	next monday …     → strictly the following week
//	+7d / -3w / +2m / +1y → relative offset
//
// Returns ok=false on unparseable input so the caller can leave
// the cursor untouched and surface a hint.
func parseJumpDate(s string, today time.Time) (time.Time, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return time.Time{}, false
	}
	// Absolute YYYY-MM-DD.
	if t, err := time.ParseInLocation("2006-01-02", s, time.Local); err == nil {
		return t, true
	}
	// Relative offset: +Nd / -Nw / +Nm / +Ny.
	if (strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-")) && len(s) >= 3 {
		sign := 1
		if s[0] == '-' {
			sign = -1
		}
		unit := s[len(s)-1]
		n, err := strconv.Atoi(s[1 : len(s)-1])
		if err == nil {
			n *= sign
			switch unit {
			case 'd':
				return today.AddDate(0, 0, n), true
			case 'w':
				return today.AddDate(0, 0, 7*n), true
			case 'm':
				return today.AddDate(0, n, 0), true
			case 'y':
				return today.AddDate(n, 0, 0), true
			}
		}
	}
	switch s {
	case "today":
		return today, true
	case "tomorrow":
		return today.AddDate(0, 0, 1), true
	case "yesterday":
		return today.AddDate(0, 0, -1), true
	}
	// Bare day number ("15") jumps to that day in the current
	// month. Caps at the actual last day to avoid the
	// "April 31 → May 1" overflow surprise.
	if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 31 {
		y, mo, _ := today.Date()
		// Last day of month: subtract one day from the first
		// of next month.
		lastDay := time.Date(y, mo+1, 0, 0, 0, 0, 0, time.Local).Day()
		if n > lastDay {
			n = lastDay
		}
		return time.Date(y, mo, n, 0, 0, 0, 0, time.Local), true
	}
	weekdays := map[string]time.Weekday{
		"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday,
		"wednesday": time.Wednesday, "thursday": time.Thursday,
		"friday": time.Friday, "saturday": time.Saturday,
	}
	bare := s
	skipWeek := false
	if strings.HasPrefix(s, "next ") {
		bare = strings.TrimPrefix(s, "next ")
		skipWeek = true
	}
	if wd, ok := weekdays[bare]; ok {
		diff := (int(wd) - int(today.Weekday()) + 7) % 7
		if diff == 0 && skipWeek {
			diff = 7
		}
		if diff > 0 || !skipWeek {
			return today.AddDate(0, 0, diff), true
		}
		return today, true
	}
	return time.Time{}, false
}

// findFreeSlots scans the next `days` days inside [startHour,
// endHour) and returns up to `limit` contiguous slots of
// `durationMin` minutes that don't overlap any event or
// scheduled planner block. Half-hour grid resolution.
func (c Calendar) findFreeSlots(durationMin, days, startHour, endHour, limit int) []freeSlot {
	if durationMin <= 0 || days <= 0 || endHour <= startHour {
		return nil
	}
	durationStep := durationMin / 30
	if durationMin%30 != 0 {
		durationStep++
	}
	if durationStep < 1 {
		durationStep = 1
	}
	var out []freeSlot
	dayStart := time.Date(c.today.Year(), c.today.Month(), c.today.Day(), 0, 0, 0, 0, time.Local)
	for d := 0; d < days && len(out) < limit; d++ {
		day := dayStart.AddDate(0, 0, d)
		// Build a busy-set of half-hour indexes for this day.
		busy := make(map[int]bool)
		mark := func(startMin, endMin int) {
			s := startMin / 30
			if startMin%30 != 0 {
				// fall through — start lands inside the slot
			}
			e := endMin / 30
			if endMin%30 != 0 {
				e++
			}
			for i := s; i < e; i++ {
				busy[i] = true
			}
		}
		for _, ev := range c.eventsForDate(day) {
			if ev.AllDay {
				// Whole-day event blocks every slot.
				for i := startHour * 2; i < endHour*2; i++ {
					busy[i] = true
				}
				continue
			}
			startM := ev.Date.Hour()*60 + ev.Date.Minute()
			endM := startM + 60
			if !ev.EndDate.IsZero() {
				endM = ev.EndDate.Hour()*60 + ev.EndDate.Minute()
			}
			mark(startM, endM)
		}
		dateStr := day.Format("2006-01-02")
		for _, blk := range c.plannerBlocks[dateStr] {
			if blk.StartTime == "" || blk.EndTime == "" {
				continue
			}
			sh, sm := parseHHMM(blk.StartTime)
			eh, em := parseHHMM(blk.EndTime)
			mark(sh*60+sm, eh*60+em)
		}
		// Skip past hours on today so we don't surface "10:00"
		// when it's already 14:30 — useless.
		earliest := startHour * 2
		if d == 0 {
			now := time.Now()
			cur := now.Hour()*2
			if now.Minute() >= 30 {
				cur++
			}
			if cur > earliest {
				earliest = cur
			}
		}
		for i := earliest; i+durationStep <= endHour*2 && len(out) < limit; i++ {
			free := true
			for j := 0; j < durationStep; j++ {
				if busy[i+j] {
					free = false
					break
				}
			}
			if !free {
				continue
			}
			startM := i * 30
			start := time.Date(day.Year(), day.Month(), day.Day(), startM/60, startM%60, 0, 0, time.Local)
			out = append(out, freeSlot{Start: start, End: start.Add(time.Duration(durationMin) * time.Minute)})
			// Skip ahead past this slot so we don't emit
			// overlapping suggestions ("Tue 10:00", "Tue 10:30"
			// for the same hour); jump to slot end.
			i += durationStep - 1
		}
	}
	return out
}

// weekGridStartHourFor computes the effective start hour for the week grid,
// mirroring the logic in viewWeek(). Default 6, shifts earlier if any event
// starts before 6 (down to 4). Used by key handlers (e/d/b) to map cursor
// position to absolute time.
func (c Calendar) weekGridStartHourFor() int {
	if c.view == calView1Day {
		return 6
	}
	startHour, _ := c.weekGridHourRangeFor()
	return startHour
}

// weekGridHourRangeFor returns the hour window used by the week/3-day grid.
// Keep this in sync with viewWeek so cursor math and render stay aligned.
func (c Calendar) weekGridHourRangeFor() (int, int) {
	workStart, workEnd := c.effectiveWorkHours()
	startHour := maxInt(5, workStart-2)
	endHour := minInt(23, workEnd+4)

	// Monday-first week boundaries, matching week renderer.
	mondayOffset := (int(c.cursor.Weekday()) + 6) % 7
	weekStart := c.cursor.AddDate(0, 0, -mondayOffset)

	for di := 0; di < 7; di++ {
		day := weekStart.AddDate(0, 0, di)
		dateStr := day.Format("2006-01-02")

		if c.showEventsLayer {
			for _, ev := range c.eventsForDate(day) {
				if ev.AllDay {
					continue
				}
				s := ev.Date.Hour()
				e := ev.Date.Hour() + 1
				if !ev.EndDate.IsZero() {
					e = (ev.EndDate.Hour()*60 + ev.EndDate.Minute() + 59) / 60
				}
				if s < startHour {
					startHour = maxInt(4, s)
				}
				if e > endHour {
					endHour = minInt(23, e)
				}
			}
		}

		if c.showPlannerLayer {
			for _, pb := range c.plannerBlocks[dateStr] {
				sH, _ := parseHHMM(pb.StartTime)
				eH, eM := parseHHMM(pb.EndTime)
				s := sH
				e := (eH*60 + eM + 59) / 60
				if e <= s {
					e = s + 1
				}
				if s < startHour {
					startHour = maxInt(4, s)
				}
				if e > endHour {
					endHour = minInt(23, e)
				}
			}
		}
	}

	if endHour-startHour < 10 {
		endHour = minInt(23, startHour+10)
	}
	if endHour <= startHour {
		endHour = startHour + 1
	}
	return startHour, endHour
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

