package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// CalendarEvent represents a single event parsed from an .ics file.
type CalendarEvent struct {
	Title    string
	Date     time.Time
	EndDate  time.Time
	Location string
	AllDay   bool
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
	calView3Day
	calViewAgenda
	calViewYear
)

// TaskItem represents a task extracted from notes.
type TaskItem struct {
	Text     string
	Done     bool
	NotePath string
	Date     string // YYYY-MM-DD if associated with a daily note
	Priority int    // 0=none, 1=low, 2=medium, 3=high, 4=highest
	LineNum  int    // 1-based line number in source note
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
	selected string   // date the user confirmed with Enter ("2006-01-02"), empty otherwise

	dailyNoteDates map[string]bool // set of "2006-01-02" strings that have daily notes
	events         []CalendarEvent
	showEvents     bool // whether the event sub-panel is expanded
	view           calendarView

	// Task data
	tasks    map[string][]TaskItem // date -> tasks
	allTasks []TaskItem            // all tasks across notes

	// Planner block data (keyed by date "YYYY-MM-DD")
	plannerBlocks map[string][]PlannerBlock

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

	// Pending event to be saved by app.go
	pendingEventDate string
	pendingEventText string

	// Task toggles pending consumption by app.go
	taskToggles []TaskToggle
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

func (c *Calendar) Close()        { c.active = false }
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
			if dateStr != "" {
				c.tasks[dateStr] = append(c.tasks[dateStr], task)
			}
		}
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
	itemType string // "task", "planner"
	dateStr  string
	index    int // index within the day's tasks or planner blocks
}

// SetPlannerBlocks stores planner schedule data keyed by date.
func (c *Calendar) SetPlannerBlocks(blocks map[string][]PlannerBlock) {
	c.plannerBlocks = blocks
}

// SetHabitData stores habit entries and logs for display in calendar views.
func (c *Calendar) SetHabitData(entries []habitEntry, logs []habitLog) {
	c.habitEntries = entries
	c.habitLogs = logs
}

// habitStats returns (completed, total) habit counts for a given date string.
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
			} else if c.view == calView3Day {
				// No vertical navigation in 3-day view
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
			} else if c.view == calView3Day {
				// No vertical navigation in 3-day view
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

		case "w":
			// Cycle through month -> week -> 3day -> agenda (skip year, use y for that)
			switch c.view {
			case calViewMonth:
				c.view = calViewWeek
			case calViewWeek:
				c.view = calView3Day
			case calView3Day:
				c.view = calViewAgenda
				c.agendaScroll = 0
				c.agendaCursor = 0
				c.agendaItems = nil
			case calViewAgenda:
				c.view = calViewMonth
			case calViewYear:
				c.view = calViewMonth
			}

		case "y":
			if c.view == calViewYear {
				c.view = calViewMonth
			} else {
				c.view = calViewYear
			}

		case "a":
			c.addingEvent = true
			c.eventInput = ""
		}
	}

	return c, nil
}

func (c *Calendar) syncViewing() {
	c.viewing = time.Date(c.cursor.Year(), c.cursor.Month(), 1, 0, 0, 0, 0, time.Local)
}

func (c Calendar) View() string {
	switch c.view {
	case calViewWeek:
		return c.viewWeek()
	case calView3Day:
		return c.view3Day()
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
	viewLabel := DimStyle.Render(" [month]")
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

	// Quick add input
	if c.addingEvent {
		c.renderQuickAdd(&b, width)
	}

	// Cursor date info
	c.renderDateInfo(&b, width)

	// Footer
	c.renderFooter(&b, width)

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Week View (time grid with 7 day columns)
// ---------------------------------------------------------------------------

func (c Calendar) viewWeek() string {
	width := c.width * 3 / 4
	if width < 80 {
		width = 80
	}
	if width > 120 {
		width = 120
	}

	var b strings.Builder

	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	viewLabel := DimStyle.Render(" [week]")
	b.WriteString("  " + titleIcon + titleText + viewLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")

	// Find the Sunday of the cursor's week
	weekStart := c.cursor.AddDate(0, 0, -int(c.cursor.Weekday()))

	weekEnd := weekStart.AddDate(0, 0, 6)
	_, weekNum := c.cursor.ISOWeek()
	weekLabel := fmt.Sprintf("Week %d: ", weekNum) + weekStart.Format("Jan 2") + " \u2013 " + weekEnd.Format("Jan 2, 2006")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(weekLabel))
	b.WriteString("\n")

	// Column widths: time gutter + 7 day columns
	timeGutterW := 7
	dayColW := (width - timeGutterW - 4) / 7
	if dayColW < 8 {
		dayColW = 8
	}

	nowHour := time.Now().Hour()
	nowMin := time.Now().Minute()
	todayStr := time.Now().Format("2006-01-02")

	// Day headers with task/habit counts
	dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	headerLine := strings.Repeat(" ", timeGutterW) + "\u2503"
	for i := 0; i < 7; i++ {
		day := weekStart.AddDate(0, 0, i)
		dateStr := day.Format("2006-01-02")
		isToday := day.Equal(c.today)
		isCursor := day.Equal(c.cursor)
		_, tasksTotal := c.taskStats(dateStr)
		habitsDone, habitsTotal := c.habitStats(dateStr)

		label := dayNames[i] + " " + day.Format("2")
		headerStyle := lipgloss.NewStyle().Foreground(text)
		if isToday {
			headerStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
		} else if isCursor {
			headerStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
		} else if i == 0 || i == 6 {
			headerStyle = lipgloss.NewStyle().Foreground(overlay0)
		}

		cellContent := headerStyle.Render(label)
		if tasksTotal > 0 {
			cellContent += lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf(" T:%d", tasksTotal))
		}
		if habitsTotal > 0 {
			cellContent += lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf(" H:%d/%d", habitsDone, habitsTotal))
		}

		cellWidth := lipgloss.Width(cellContent)
		pad := dayColW - cellWidth
		if pad < 0 {
			pad = 0
		}
		headerLine += " " + cellContent + strings.Repeat(" ", pad) + "\u2503"
	}
	b.WriteString(headerLine + "\n")

	// Separator
	sepLine := strings.Repeat(" ", timeGutterW) + "\u2523"
	for i := 0; i < 7; i++ {
		sepLine += strings.Repeat("\u2501", dayColW+1) + "\u254B"
	}
	b.WriteString(sepLine + "\n")

	// Build grid data: for each day, map hour -> content
	type gridCell struct {
		text  string
		color lipgloss.Color
	}
	dayGrids := make([]map[int]gridCell, 7)
	for i := 0; i < 7; i++ {
		dayGrids[i] = make(map[int]gridCell)
		day := weekStart.AddDate(0, 0, i)
		dateStr := day.Format("2006-01-02")

		// Place planner blocks
		for _, pb := range c.plannerBlocks[dateStr] {
			startH, startM := parseTimeHM(pb.StartTime)
			endH, endM := parseTimeHM(pb.EndTime)
			if startH < 6 {
				startH = 6
			}
			if endH > 22 || (endH == 22 && endM > 0) {
				endH = 22
				endM = 0
			}
			_ = startM
			_ = endM
			blockColor := c.blockTypeColor(pb.BlockType)
			for h := startH; h < endH; h++ {
				label := pb.Text
				if h == startH {
					// Show text on first hour
					maxLen := dayColW - 3
					if maxLen < 4 {
						maxLen = 4
					}
					if len(label) > maxLen {
						label = label[:maxLen-1] + "\u2026"
					}
				} else {
					label = "\u2502" // continuation bar
				}
				dayGrids[i][h] = gridCell{text: label, color: blockColor}
			}
		}

		// Place events
		for _, ev := range c.eventsForDate(day) {
			if ev.AllDay {
				continue
			}
			h := ev.Date.Hour()
			if h < 6 || h >= 22 {
				continue
			}
			if _, exists := dayGrids[i][h]; !exists {
				label := ev.Title
				maxLen := dayColW - 3
				if maxLen < 4 {
					maxLen = 4
				}
				if len(label) > maxLen {
					label = label[:maxLen-1] + "\u2026"
				}
				dayGrids[i][h] = gridCell{text: label, color: lavender}
			}
		}
	}

	// Time rows 06:00 - 21:00
	for hour := 6; hour < 22; hour++ {
		timeLabel := fmt.Sprintf(" %02d:00 ", hour)
		timeStyle := DimStyle
		isCurrentHour := todayStr >= weekStart.Format("2006-01-02") &&
			todayStr <= weekEnd.Format("2006-01-02") && hour == nowHour
		if isCurrentHour {
			timeStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
		}

		rowLine := timeStyle.Render(timeLabel) + "\u2503"
		for i := 0; i < 7; i++ {
			day := weekStart.AddDate(0, 0, i)
			dateStr := day.Format("2006-01-02")
			cell, hasBlock := dayGrids[i][hour]

			// Current time indicator
			isNowSlot := dateStr == todayStr && hour == nowHour
			cellStr := ""
			if hasBlock {
				blockStyle := lipgloss.NewStyle().Foreground(cell.color)
				if cell.text == "\u2502" {
					cellStr = blockStyle.Render(" \u2588 ") + strings.Repeat(" ", dayColW-3)
				} else {
					cellStr = blockStyle.Render(" \u2588 "+cell.text)
					cellWidth := lipgloss.Width(cellStr)
					pad := dayColW + 1 - cellWidth
					if pad < 0 {
						pad = 0
					}
					cellStr += strings.Repeat(" ", pad)
				}
			} else {
				cellStr = strings.Repeat(" ", dayColW+1)
			}

			if isNowSlot {
				// Draw current time marker
				markerPos := nowMin * dayColW / 60
				if markerPos < 1 {
					markerPos = 1
				}
				if !hasBlock {
					marker := strings.Repeat("\u2500", dayColW)
					cellStr = lipgloss.NewStyle().Foreground(red).Render(" "+marker)
					pad := dayColW + 1 - lipgloss.Width(cellStr)
					if pad < 0 {
						pad = 0
					}
					cellStr += strings.Repeat(" ", pad)
				}
			}

			rowLine += cellStr + "\u2503"
		}
		b.WriteString(rowLine + "\n")
	}

	// Quick add input
	if c.addingEvent {
		c.renderQuickAdd(&b, width)
	}

	c.renderFooter(&b, width)

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// 3-Day View (horizontal 3-column time grid)
// ---------------------------------------------------------------------------

func (c Calendar) view3Day() string {
	width := c.width * 3 / 4
	if width < 72 {
		width = 72
	}
	if width > 110 {
		width = 110
	}

	var b strings.Builder

	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	viewLabel := DimStyle.Render(" [3-day]")
	b.WriteString("  " + titleIcon + titleText + viewLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")

	// 3 columns: cursor day, cursor+1, cursor+2
	timeGutterW := 7
	colW := (width - timeGutterW - 4) / 3
	if colW < 16 {
		colW = 16
	}

	nowHour := time.Now().Hour()
	nowMin := time.Now().Minute()
	todayStr := time.Now().Format("2006-01-02")

	// Day headers
	headerLine := strings.Repeat(" ", timeGutterW) + "\u2503"
	for i := 0; i < 3; i++ {
		day := c.cursor.AddDate(0, 0, i)
		dateStr := day.Format("2006-01-02")
		isToday := day.Equal(c.today)

		label := day.Format("Mon 2 Jan")
		headerStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
		if isToday {
			headerStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
			label += " (today)"
		}
		tasksDone, tasksTotal := c.taskStats(dateStr)
		habitsDone, habitsTotal := c.habitStats(dateStr)

		cellContent := headerStyle.Render(label)
		if tasksTotal > 0 {
			cellContent += lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf(" T:%d/%d", tasksDone, tasksTotal))
		}
		if habitsTotal > 0 {
			cellContent += lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf(" H:%d/%d", habitsDone, habitsTotal))
		}

		cellWidth := lipgloss.Width(cellContent)
		pad := colW - cellWidth
		if pad < 0 {
			pad = 0
		}
		headerLine += " " + cellContent + strings.Repeat(" ", pad) + "\u2503"
	}
	b.WriteString(headerLine + "\n")

	// Separator
	sepLine := strings.Repeat(" ", timeGutterW) + "\u2523"
	for i := 0; i < 3; i++ {
		sepLine += strings.Repeat("\u2501", colW+1) + "\u254B"
	}
	b.WriteString(sepLine + "\n")

	// Build grid data for each of the 3 days
	type gridCell struct {
		text    string
		color   lipgloss.Color
		project string
	}
	dayGrids := make([]map[int]gridCell, 3)
	for i := 0; i < 3; i++ {
		dayGrids[i] = make(map[int]gridCell)
		day := c.cursor.AddDate(0, 0, i)
		dateStr := day.Format("2006-01-02")

		// Place planner blocks
		for _, pb := range c.plannerBlocks[dateStr] {
			startH, _ := parseTimeHM(pb.StartTime)
			endH, _ := parseTimeHM(pb.EndTime)
			if startH < 6 {
				startH = 6
			}
			if endH > 22 {
				endH = 22
			}
			blockColor := c.blockTypeColor(pb.BlockType)
			for h := startH; h < endH; h++ {
				label := pb.Text
				if h == startH {
					maxLen := colW - 4
					if maxLen < 4 {
						maxLen = 4
					}
					if len(label) > maxLen {
						label = label[:maxLen-1] + "\u2026"
					}
				} else {
					label = "\u2502"
				}
				dayGrids[i][h] = gridCell{text: label, color: blockColor}
			}
		}

		// Place events
		for _, ev := range c.eventsForDate(day) {
			if ev.AllDay {
				continue
			}
			h := ev.Date.Hour()
			if h < 6 || h >= 22 {
				continue
			}
			if _, exists := dayGrids[i][h]; !exists {
				label := ev.Title
				maxLen := colW - 4
				if maxLen < 4 {
					maxLen = 4
				}
				if len(label) > maxLen {
					label = label[:maxLen-1] + "\u2026"
				}
				dayGrids[i][h] = gridCell{text: label, color: lavender}
			}
		}
	}

	// Time rows 06:00 - 21:00
	for hour := 6; hour < 22; hour++ {
		timeLabel := fmt.Sprintf(" %02d:00 ", hour)
		timeStyle := DimStyle
		if hour == nowHour {
			timeStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
		}

		rowLine := timeStyle.Render(timeLabel) + "\u2503"
		for i := 0; i < 3; i++ {
			day := c.cursor.AddDate(0, 0, i)
			dateStr := day.Format("2006-01-02")
			cell, hasBlock := dayGrids[i][hour]

			isNowSlot := dateStr == todayStr && hour == nowHour
			cellStr := ""
			if hasBlock {
				blockStyle := lipgloss.NewStyle().Foreground(cell.color)
				if cell.text == "\u2502" {
					cellStr = blockStyle.Render(" \u2588 ") + strings.Repeat(" ", colW-3)
				} else {
					rendered := blockStyle.Render(" \u2588 " + cell.text)
					cellWidth := lipgloss.Width(rendered)
					pad := colW + 1 - cellWidth
					if pad < 0 {
						pad = 0
					}
					cellStr = rendered + strings.Repeat(" ", pad)
				}
			} else {
				cellStr = strings.Repeat(" ", colW+1)
			}

			if isNowSlot && !hasBlock {
				_ = nowMin
				marker := strings.Repeat("\u2500", colW)
				cellStr = lipgloss.NewStyle().Foreground(red).Render(" "+marker)
				pad := colW + 1 - lipgloss.Width(cellStr)
				if pad < 0 {
					pad = 0
				}
				cellStr += strings.Repeat(" ", pad)
			}

			rowLine += cellStr + "\u2503"
		}
		b.WriteString(rowLine + "\n")
	}

	// Bottom summary row: tasks and habits per day
	summSep := strings.Repeat(" ", timeGutterW) + "\u2523"
	for i := 0; i < 3; i++ {
		summSep += strings.Repeat("\u2501", colW+1) + "\u254B"
	}
	b.WriteString(summSep + "\n")

	// Tasks pending row
	taskLine := strings.Repeat(" ", timeGutterW) + "\u2503"
	for i := 0; i < 3; i++ {
		day := c.cursor.AddDate(0, 0, i)
		dateStr := day.Format("2006-01-02")
		tasksDone, tasksTotal := c.taskStats(dateStr)
		pending := tasksTotal - tasksDone
		label := fmt.Sprintf(" Tasks: %d pending", pending)
		taskStyle := lipgloss.NewStyle().Foreground(yellow)
		if pending == 0 && tasksTotal > 0 {
			taskStyle = lipgloss.NewStyle().Foreground(green)
			label = " Tasks: all done"
		} else if tasksTotal == 0 {
			taskStyle = DimStyle
			label = " No tasks"
		}
		rendered := taskStyle.Render(label)
		cellWidth := lipgloss.Width(rendered)
		pad := colW + 1 - cellWidth
		if pad < 0 {
			pad = 0
		}
		taskLine += rendered + strings.Repeat(" ", pad) + "\u2503"
	}
	b.WriteString(taskLine + "\n")

	// Habits row
	habitLine := strings.Repeat(" ", timeGutterW) + "\u2503"
	for i := 0; i < 3; i++ {
		day := c.cursor.AddDate(0, 0, i)
		dateStr := day.Format("2006-01-02")
		habitsDone, habitsTotal := c.habitStats(dateStr)
		label := ""
		habitStyle := DimStyle
		if habitsTotal > 0 {
			label = fmt.Sprintf(" Habits: %d/%d done", habitsDone, habitsTotal)
			habitStyle = lipgloss.NewStyle().Foreground(green)
			if habitsDone < habitsTotal {
				habitStyle = lipgloss.NewStyle().Foreground(peach)
			}
		}
		rendered := habitStyle.Render(label)
		cellWidth := lipgloss.Width(rendered)
		pad := colW + 1 - cellWidth
		if pad < 0 {
			pad = 0
		}
		habitLine += rendered + strings.Repeat(" ", pad) + "\u2503"
	}
	b.WriteString(habitLine + "\n")

	// Quick add input
	if c.addingEvent {
		c.renderQuickAdd(&b, width)
	}

	c.renderFooter(&b, width)

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// blockTypeColor returns the color for a planner block type.
func (c Calendar) blockTypeColor(blockType string) lipgloss.Color {
	switch blockType {
	case "task":
		return blue
	case "event":
		return lavender
	case "break":
		return overlay0
	case "focus":
		return green
	default:
		return text
	}
}

// parseTimeHM parses "HH:MM" into hour and minute integers.
func parseTimeHM(t string) (int, int) {
	parts := strings.SplitN(t, ":", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	h, m := 0, 0
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)
	return h, m
}

// renderMiniCalendar builds a compact month calendar for use in the week view sidebar.
func (c Calendar) renderMiniCalendar() string {
	var b strings.Builder

	year, month, _ := c.viewing.Date()
	monthYear := c.viewing.Format("Jan 2006")

	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(headerStyle.Render(fmt.Sprintf("  %-12s", monthYear)))
	b.WriteString("\n")

	dayHeaderStyle := lipgloss.NewStyle().Foreground(subtext0)
	b.WriteString(dayHeaderStyle.Render("  Su Mo Tu We Th Fr Sa"))
	b.WriteString("\n")

	firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	startWeekday := int(firstOfMonth.Weekday())
	daysInMo := daysIn(month, year)

	row := "  "
	col := 0
	for i := 0; i < startWeekday; i++ {
		row += "   "
		col++
	}

	for d := 1; d <= daysInMo; d++ {
		dt := time.Date(year, month, d, 0, 0, 0, 0, time.Local)
		isToday := dt.Equal(c.today)
		isCursor := dt.Equal(c.cursor)

		numStr := fmt.Sprintf("%2d", d)
		switch {
		case isCursor && isToday:
			numStr = lipgloss.NewStyle().Background(green).Foreground(crust).Bold(true).Render(numStr)
		case isToday:
			numStr = lipgloss.NewStyle().Foreground(green).Bold(true).Render(numStr)
		case isCursor:
			numStr = lipgloss.NewStyle().Foreground(peach).Bold(true).Render(numStr)
		default:
			numStr = lipgloss.NewStyle().Foreground(overlay0).Render(numStr)
		}
		row += numStr + " "
		col++

		if col == 7 {
			b.WriteString(row)
			b.WriteString("\n")
			row = "  "
			col = 0
		}
	}

	if col > 0 {
		b.WriteString(row)
		b.WriteString("\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Agenda View (enhanced 14-day lookahead)
// ---------------------------------------------------------------------------

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

		// Events
		for _, ev := range dayEvents {
			timeStr := "all day"
			if !ev.AllDay {
				timeStr = ev.Date.Format("15:04")
			}
			section.lines = append(section.lines,
				"    "+lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar+" ")+
					DimStyle.Render(timeStr+" ")+
					lipgloss.NewStyle().Foreground(text).Render(ev.Title))
			section.items = append(section.items, -1)
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
			pbText := pb.Text
			if len(pbText) > width-22 {
				pbText = pbText[:width-25] + "..."
			}
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
			taskText := task.Text
			if len(taskText) > width-12 {
				taskText = taskText[:width-15] + "..."
			}
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
			section.lines = append(section.lines, DimStyle.Render("    No events"))
			section.items = append(section.items, -1)
		}

		sections = append(sections, section)
	}

	// Store agenda items for interaction
	c.agendaItems = items

	// Clamp agenda cursor
	if c.agendaCursor >= len(items) {
		c.agendaCursor = len(items) - 1
	}
	if c.agendaCursor < 0 {
		c.agendaCursor = 0
	}

	// Apply scroll and render visible sections
	maxLines := c.height - 14
	if maxLines < 8 {
		maxLines = 8
	}

	// Clamp scroll
	if c.agendaScroll >= len(sections) {
		c.agendaScroll = len(sections) - 1
	}
	if c.agendaScroll < 0 {
		c.agendaScroll = 0
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
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
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
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
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
				bullet := lipgloss.NewStyle().Foreground(blue).Render("  " + IconCalendarChar + " ")
				title := lipgloss.NewStyle().Foreground(text).Render(ev.Title)
				timeStr := ""
				if !ev.AllDay {
					timeStr = " (" + ev.Date.Format("15:04") + ")"
				} else {
					timeStr = " (all day)"
				}
				timePart := DimStyle.Render(timeStr)
				b.WriteString(bullet + title + timePart)
				if ev.Location != "" {
					loc := DimStyle.Render("      @ " + ev.Location)
					b.WriteString("\n" + loc)
				}
				b.WriteString("\n")
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
			if hasNote {
				dot := lipgloss.NewStyle().Foreground(green).Render("  " + IconDailyChar + " ")
				b.WriteString(dot + lipgloss.NewStyle().Foreground(text).Render("Daily note exists"))
				b.WriteString("\n")
			}
			if len(dayEvents) > 0 {
				dot := lipgloss.NewStyle().Foreground(blue).Render("  " + IconCalendarChar + " ")
				count := fmt.Sprintf("%d event", len(dayEvents))
				if len(dayEvents) > 1 {
					count += "s"
				}
				b.WriteString(dot + lipgloss.NewStyle().Foreground(text).Render(count))
				b.WriteString("\n")
			}
			if len(dayPlannerBlocks) > 0 {
				dot := lipgloss.NewStyle().Foreground(lavender).Render("  ▪ ")
				count := fmt.Sprintf("%d planner block", len(dayPlannerBlocks))
				if len(dayPlannerBlocks) > 1 {
					count += "s"
				}
				b.WriteString(dot + lipgloss.NewStyle().Foreground(text).Render(count))
				b.WriteString("\n")
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

	keyStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	descStyle := lipgloss.NewStyle().Foreground(overlay0)
	sep := lipgloss.NewStyle().Foreground(surface1).Render(" | ")

	b.WriteString("  " +
		keyStyle.Render("hjkl") + descStyle.Render(" nav") + sep +
		keyStyle.Render("[]") + descStyle.Render(" month") + sep +
		keyStyle.Render("w") + descStyle.Render(" view") + sep +
		keyStyle.Render("t") + descStyle.Render(" today"))
	b.WriteString("\n")
	b.WriteString("  " +
		keyStyle.Render("Enter") + descStyle.Render(" open note") + sep +
		keyStyle.Render("a") + descStyle.Render(" add task") + sep +
		keyStyle.Render("Space") + descStyle.Render(" toggle") + sep +
		keyStyle.Render("y") + descStyle.Render(" year") + sep +
		keyStyle.Render("e") + descStyle.Render(" events") + sep +
		keyStyle.Render("Esc") + descStyle.Render(" close"))
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

// ---------------------------------------------------------------------------
// ICS Parsing
// ---------------------------------------------------------------------------

func ParseICSFile(path string) ([]CalendarEvent, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ics file: %w", err)
	}
	defer func() { _ = f.Close() }()

	var events []CalendarEvent
	var current *CalendarEvent
	inEvent := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r\n")

		if line == "BEGIN:VEVENT" {
			inEvent = true
			current = &CalendarEvent{}
			continue
		}

		if line == "END:VEVENT" {
			if inEvent && current != nil {
				events = append(events, *current)
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
			t, allDay := parseICSTime(value)
			current.Date = t
			current.AllDay = allDay
		case "DTEND":
			t, _ := parseICSTime(value)
			current.EndDate = t
		}
	}

	if err := scanner.Err(); err != nil {
		return events, fmt.Errorf("read ics file: %w", err)
	}

	return events, nil
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

func parseICSTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if t, err := time.Parse("20060102T150405Z", value); err == nil {
		return t.Local(), false
	}
	if t, err := time.Parse("20060102T150405", value); err == nil {
		return t, false
	}
	if t, err := time.Parse("20060102", value); err == nil {
		return t, true
	}
	return time.Time{}, false
}
