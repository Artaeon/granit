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

// calendarView controls which view mode is active.
type calendarView int

const (
	calViewMonth calendarView = iota
	calViewWeek
	calViewAgenda
)

// TaskItem represents a task extracted from notes.
type TaskItem struct {
	Text     string
	Done     bool
	NotePath string
	Date     string // YYYY-MM-DD if associated with a daily note
}

var taskPattern = regexp.MustCompile(`^- \[([ xX])\] (.+)`)

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
	tasks     map[string][]TaskItem // date -> tasks
	allTasks  []TaskItem            // all tasks across notes
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
	now := time.Now()
	c.today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	c.cursor = c.today
	c.viewing = c.today
}

func (c *Calendar) Close()       { c.active = false }
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

		for _, line := range strings.Split(content, "\n") {
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

func (c Calendar) Update(msg tea.Msg) (Calendar, tea.Cmd) {
	if !c.active {
		return c, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
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
			c.cursor = c.cursor.AddDate(0, 0, -7)
			c.syncViewing()
		case "down", "j":
			c.cursor = c.cursor.AddDate(0, 0, 7)
			c.syncViewing()

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
			c.selected = c.cursor.Format("2006-01-02")
			c.active = false
			return c, nil

		case "e":
			c.showEvents = !c.showEvents

		case "t":
			c.cursor = c.today
			c.syncViewing()

		case "w":
			// Toggle between month, week, agenda views
			c.view = (c.view + 1) % 3
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
	case calViewAgenda:
		return c.viewAgenda()
	default:
		return c.viewMonth()
	}
}

// ---------------------------------------------------------------------------
// Month View
// ---------------------------------------------------------------------------

func (c Calendar) viewMonth() string {
	width := c.width * 2 / 3
	if width < 38 {
		width = 38
	}
	if width > 56 {
		width = 56
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
	headerPad := (28 - lipgloss.Width(header)) / 2
	if headerPad < 0 {
		headerPad = 0
	}
	b.WriteString("  " + strings.Repeat(" ", headerPad) + header)
	b.WriteString("\n\n")

	// Day-of-week header
	dayNames := []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	dayHeaderStyle := lipgloss.NewStyle().Foreground(subtext0).Bold(true)
	var dayRow strings.Builder
	dayRow.WriteString("  ")
	for _, d := range dayNames {
		dayRow.WriteString(dayHeaderStyle.Render(fmt.Sprintf("%4s", d)))
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

	row := "  "
	col := 0

	for i := 0; i < startWeekday; i++ {
		day := prevDays - startWeekday + 1 + i
		cell := c.renderDayCell(day, false, false, false, false, 0, 0, false, true)
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

		cell := c.renderDayCell(d, isToday, isCursor, hasNote, hasEvent, tasksDone, tasksTotal, true, false)
		row += cell
		col++

		if col == 7 {
			b.WriteString(row)
			b.WriteString("\n")
			row = "  "
			col = 0
		}
	}

	if col > 0 {
		nextDay := 1
		for col < 7 {
			cell := c.renderDayCell(nextDay, false, false, false, false, 0, 0, false, true)
			row += cell
			col++
			nextDay++
		}
		b.WriteString(row)
		b.WriteString("\n")
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
// Week View
// ---------------------------------------------------------------------------

func (c Calendar) viewWeek() string {
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
	viewLabel := DimStyle.Render(" [week]")
	b.WriteString("  " + titleIcon + titleText + viewLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")

	// Find the Sunday of the cursor's week
	weekStart := c.cursor.AddDate(0, 0, -int(c.cursor.Weekday()))

	// Week header
	weekEnd := weekStart.AddDate(0, 0, 6)
	weekLabel := weekStart.Format("Jan 2") + " - " + weekEnd.Format("Jan 2, 2006")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(weekLabel))
	b.WriteString("\n\n")

	dayNames := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

	for i := 0; i < 7; i++ {
		day := weekStart.AddDate(0, 0, i)
		dateStr := day.Format("2006-01-02")
		isToday := day.Equal(c.today)
		isCursor := day.Equal(c.cursor)
		hasNote := c.dailyNoteDates[dateStr]
		hasEvent := c.dateHasEvent(day)
		tasksDone, tasksTotal := c.taskStats(dateStr)

		// Day header
		dayLabel := dayNames[i] + " " + day.Format("Jan 2")
		dayStyle := lipgloss.NewStyle().Foreground(text)
		if isToday {
			dayStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
		}
		if isCursor {
			dayStyle = dayStyle.Underline(true)
		}

		prefix := "  "
		if isCursor {
			prefix = lipgloss.NewStyle().Foreground(mauve).Render("▸ ")
		}

		line := prefix + dayStyle.Render(dayLabel)

		// Indicators
		indicators := ""
		if hasNote {
			indicators += " " + lipgloss.NewStyle().Foreground(green).Render(IconDailyChar)
		}
		if hasEvent {
			indicators += " " + lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
		}
		if tasksTotal > 0 {
			taskColor := yellow
			if tasksDone == tasksTotal {
				taskColor = green
			}
			indicators += " " + lipgloss.NewStyle().Foreground(taskColor).Render(
				fmt.Sprintf("[%d/%d]", tasksDone, tasksTotal))
		}

		b.WriteString(line + indicators)
		b.WriteString("\n")

		// Show events and tasks for cursor day
		if isCursor {
			dayEvents := c.eventsForDate(day)
			for _, ev := range dayEvents {
				timeStr := ""
				if !ev.AllDay {
					timeStr = ev.Date.Format("15:04") + " "
				}
				b.WriteString("    " + lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar+" ") +
					DimStyle.Render(timeStr) +
					lipgloss.NewStyle().Foreground(text).Render(ev.Title))
				b.WriteString("\n")
			}
			dayTasks := c.tasks[dateStr]
			for _, task := range dayTasks {
				checkIcon := lipgloss.NewStyle().Foreground(yellow).Render("○")
				if task.Done {
					checkIcon = lipgloss.NewStyle().Foreground(green).Render("●")
				}
				taskText := task.Text
				if len(taskText) > width-12 {
					taskText = taskText[:width-15] + "..."
				}
				b.WriteString("    " + checkIcon + " " + lipgloss.NewStyle().Foreground(text).Render(taskText))
				b.WriteString("\n")
			}
		}
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
// Agenda View
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

	// Show upcoming tasks (next 14 days from today)
	b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("Upcoming Tasks"))
	b.WriteString("\n\n")

	maxLines := c.height - 12
	if maxLines < 5 {
		maxLines = 5
	}
	lineCount := 0

	for d := 0; d < 14 && lineCount < maxLines; d++ {
		day := c.today.AddDate(0, 0, d)
		dateStr := day.Format("2006-01-02")
		dayTasks := c.tasks[dateStr]
		dayEvents := c.eventsForDate(day)

		if len(dayTasks) == 0 && len(dayEvents) == 0 {
			continue
		}

		// Day header
		dayLabel := day.Format("Mon Jan 2")
		if d == 0 {
			dayLabel += " (today)"
		} else if d == 1 {
			dayLabel += " (tomorrow)"
		}

		isToday := d == 0
		dayStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
		if isToday {
			dayStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
		}

		b.WriteString("  " + dayStyle.Render(dayLabel))
		b.WriteString("\n")
		lineCount++

		for _, ev := range dayEvents {
			if lineCount >= maxLines {
				break
			}
			timeStr := "all day"
			if !ev.AllDay {
				timeStr = ev.Date.Format("15:04")
			}
			b.WriteString("    " + lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar+" ") +
				DimStyle.Render(timeStr+" ") +
				lipgloss.NewStyle().Foreground(text).Render(ev.Title))
			b.WriteString("\n")
			lineCount++
		}

		for _, task := range dayTasks {
			if lineCount >= maxLines {
				break
			}
			checkIcon := lipgloss.NewStyle().Foreground(yellow).Render("○")
			if task.Done {
				checkIcon = lipgloss.NewStyle().Foreground(green).Render("●")
			}
			taskText := task.Text
			if len(taskText) > width-12 {
				taskText = taskText[:width-15] + "..."
			}
			b.WriteString("    " + checkIcon + " " + lipgloss.NewStyle().Foreground(text).Render(taskText))
			b.WriteString("\n")
			lineCount++
		}
		b.WriteString("\n")
		lineCount++
	}

	if lineCount == 0 {
		b.WriteString(DimStyle.Render("  No upcoming tasks or events"))
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
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("Total tasks: %d  Done: ", totalTasks)) +
		lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("%d", doneTasks)) +
		lipgloss.NewStyle().Foreground(text).Render(fmt.Sprintf("  Pending: ")) +
		lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf("%d", totalTasks-doneTasks)))

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
// Shared helpers
// ---------------------------------------------------------------------------

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

		if len(dayEvents) > 0 || hasNote || tasksTotal > 0 {
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
		keyStyle.Render("e") + descStyle.Render(" events") + sep +
		keyStyle.Render("Esc") + descStyle.Render(" close"))
}

func (c Calendar) renderDayCell(day int, isToday, isCursor, hasNote, hasEvent bool, tasksDone, tasksTotal int, currentMonth, dim bool) string {
	numStr := fmt.Sprintf("%2d", day)
	marker := " "

	if currentMonth {
		if hasNote && hasEvent {
			marker = lipgloss.NewStyle().Foreground(green).Render("·")
		} else if hasNote {
			marker = lipgloss.NewStyle().Foreground(green).Render("·")
		} else if hasEvent {
			marker = lipgloss.NewStyle().Foreground(blue).Render("·")
		} else if tasksTotal > 0 {
			if tasksDone == tasksTotal {
				marker = lipgloss.NewStyle().Foreground(green).Render("·")
			} else {
				marker = lipgloss.NewStyle().Foreground(yellow).Render("·")
			}
		}
	}

	var styled string
	switch {
	case isCursor && isToday:
		styled = lipgloss.NewStyle().
			Background(peach).
			Foreground(crust).
			Bold(true).
			Underline(true).
			Render(numStr)
	case isToday:
		styled = lipgloss.NewStyle().
			Background(peach).
			Foreground(crust).
			Bold(true).
			Render(numStr)
	case isCursor:
		styled = lipgloss.NewStyle().
			Foreground(mauve).
			Underline(true).
			Bold(true).
			Render(numStr)
	case !currentMonth || dim:
		styled = DimStyle.Render(numStr)
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
	defer f.Close()

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
