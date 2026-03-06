package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
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
	}
}

// SetSize updates the available dimensions for rendering.
func (c *Calendar) SetSize(width, height int) {
	c.width = width
	c.height = height
}

// Open activates the calendar overlay.
func (c *Calendar) Open() {
	c.active = true
	c.selected = ""
	c.showEvents = false
	now := time.Now()
	c.today = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	c.cursor = c.today
	c.viewing = c.today
}

// Close deactivates the calendar overlay.
func (c *Calendar) Close() {
	c.active = false
}

// IsActive returns whether the calendar overlay is currently shown.
func (c *Calendar) IsActive() bool {
	return c.active
}

// SetDailyNotes accepts a list of note file paths and extracts dates from
// filenames that match the "2006-01-02" pattern so dots can be rendered on
// those calendar days.
func (c *Calendar) SetDailyNotes(notes []string) {
	c.dailyNoteDates = make(map[string]bool, len(notes))
	for _, p := range notes {
		base := filepath.Base(p)
		base = strings.TrimSuffix(base, filepath.Ext(base))
		// Try parsing the filename as a date.
		if _, err := time.Parse("2006-01-02", base); err == nil {
			c.dailyNoteDates[base] = true
		}
	}
}

// SetEvents sets the list of calendar events to display.
func (c *Calendar) SetEvents(events []CalendarEvent) {
	c.events = events
}

// SelectedDate returns the date string ("2006-01-02") the user confirmed with
// Enter, then clears it. Returns empty string if no selection was made.
func (c *Calendar) SelectedDate() string {
	s := c.selected
	c.selected = ""
	return s
}

// Update handles key messages for the calendar overlay.
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
			// Jump back to today.
			c.cursor = c.today
			c.syncViewing()
		}
	}

	return c, nil
}

// syncViewing ensures the viewing month matches the cursor month.
func (c *Calendar) syncViewing() {
	c.viewing = time.Date(c.cursor.Year(), c.cursor.Month(), 1, 0, 0, 0, 0, time.Local)
}

// View renders the calendar overlay.
func (c Calendar) View() string {
	width := c.width * 2 / 3
	if width < 34 {
		width = 34
	}
	if width > 50 {
		width = 50
	}

	var b strings.Builder

	// ── Title ──
	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	b.WriteString("  " + titleIcon + titleText)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")

	// ── Month / Year header ──
	monthYear := c.viewing.Format("January 2006")
	navLeft := lipgloss.NewStyle().Foreground(overlay1).Render("< ")
	navRight := lipgloss.NewStyle().Foreground(overlay1).Render(" >")
	header := navLeft + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(monthYear) + navRight
	// Center the header within the grid width (7 cells * 4 chars = 28).
	headerPad := (28 - lipgloss.Width(header)) / 2
	if headerPad < 0 {
		headerPad = 0
	}
	b.WriteString("  " + strings.Repeat(" ", headerPad) + header)
	b.WriteString("\n\n")

	// ── Day-of-week header ──
	dayNames := []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	dayHeaderStyle := lipgloss.NewStyle().Foreground(subtext0).Bold(true)
	var dayRow strings.Builder
	dayRow.WriteString("  ")
	for _, d := range dayNames {
		dayRow.WriteString(dayHeaderStyle.Render(fmt.Sprintf("%4s", d)))
	}
	b.WriteString(dayRow.String())
	b.WriteString("\n")

	// ── Calendar grid ──
	year, month, _ := c.viewing.Date()
	firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	startWeekday := int(firstOfMonth.Weekday()) // 0=Sun
	daysInMonth := daysIn(month, year)

	// Previous month filler days.
	prevMonth := firstOfMonth.AddDate(0, 0, -1)
	prevDays := daysIn(prevMonth.Month(), prevMonth.Year())

	row := "  "
	col := 0

	// Leading blanks from previous month (rendered dim).
	for i := 0; i < startWeekday; i++ {
		day := prevDays - startWeekday + 1 + i
		cell := c.renderDayCell(day, false, false, false, false, false, true)
		row += cell
		col++
	}

	// Current month days.
	for d := 1; d <= daysInMonth; d++ {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, int(month), d)
		dt := time.Date(year, month, d, 0, 0, 0, 0, time.Local)

		isToday := dt.Equal(c.today)
		isCursor := dt.Equal(c.cursor)
		hasNote := c.dailyNoteDates[dateStr]
		hasEvent := c.dateHasEvent(dt)

		cell := c.renderDayCell(d, isToday, isCursor, hasNote, hasEvent, true, false)
		row += cell
		col++

		if col == 7 {
			b.WriteString(row)
			b.WriteString("\n")
			row = "  "
			col = 0
		}
	}

	// Trailing blanks from next month (rendered dim).
	if col > 0 {
		nextDay := 1
		for col < 7 {
			cell := c.renderDayCell(nextDay, false, false, false, false, false, true)
			row += cell
			col++
			nextDay++
		}
		b.WriteString(row)
		b.WriteString("\n")
	}

	// ── Events for selected date ──
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
				bullet := lipgloss.NewStyle().Foreground(blue).Render("  \u2022 ")
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
	}

	// ── Indicators for cursor date ──
	if !c.showEvents {
		dayEvents := c.eventsForDate(c.cursor)
		noteKey := c.cursor.Format("2006-01-02")
		hasNote := c.dailyNoteDates[noteKey]

		if len(dayEvents) > 0 || hasNote {
			b.WriteString("\n")
			if hasNote {
				dot := lipgloss.NewStyle().Foreground(green).Render("  \u25cf ")
				b.WriteString(dot + lipgloss.NewStyle().Foreground(text).Render("Daily note exists"))
				b.WriteString("\n")
			}
			if len(dayEvents) > 0 {
				dot := lipgloss.NewStyle().Foreground(blue).Render("  \u25cf ")
				count := fmt.Sprintf("%d event", len(dayEvents))
				if len(dayEvents) > 1 {
					count += "s"
				}
				b.WriteString(dot + lipgloss.NewStyle().Foreground(text).Render(count))
				b.WriteString("\n")
			}
		}
	}

	// ── Footer ──
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")
	footerLine1 := lipgloss.NewStyle().Foreground(overlay1).
		Render("  \u2190\u2192: day  \u2191\u2193: week  []: month")
	b.WriteString(footerLine1)
	b.WriteString("\n")
	footerLine2 := lipgloss.NewStyle().Foreground(overlay1).
		Render("  Enter: open note  e: events  t: today")
	b.WriteString(footerLine2)

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// renderDayCell renders a single day cell (4 chars wide).
func (c Calendar) renderDayCell(day int, isToday, isCursor, hasNote, hasEvent, currentMonth, dim bool) string {
	numStr := fmt.Sprintf("%2d", day)
	marker := " "

	if currentMonth {
		if hasNote && hasEvent {
			marker = lipgloss.NewStyle().Foreground(green).Render("\u00b7")
		} else if hasNote {
			marker = lipgloss.NewStyle().Foreground(green).Render("\u00b7")
		} else if hasEvent {
			marker = lipgloss.NewStyle().Foreground(blue).Render("\u00b7")
		}
	}

	var styled string
	switch {
	case isCursor && isToday:
		// Selected + today: peach background, bold, underline.
		styled = lipgloss.NewStyle().
			Background(peach).
			Foreground(crust).
			Bold(true).
			Underline(true).
			Render(numStr)
	case isToday:
		// Today: peach background, bold.
		styled = lipgloss.NewStyle().
			Background(peach).
			Foreground(crust).
			Bold(true).
			Render(numStr)
	case isCursor:
		// Cursor (selected day): mauve underline.
		styled = lipgloss.NewStyle().
			Foreground(mauve).
			Underline(true).
			Bold(true).
			Render(numStr)
	case !currentMonth || dim:
		styled = DimStyle.Render(numStr)
	case hasNote && hasEvent:
		// Both note and event: green foreground (note takes priority).
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

	// Each cell is 4 chars: " XX" or " XX" + marker occupying the trailing space.
	return " " + styled + marker
}

// dateHasEvent checks whether any event falls on the given date.
func (c Calendar) dateHasEvent(dt time.Time) bool {
	y, m, d := dt.Date()
	for _, ev := range c.events {
		ey, em, ed := ev.Date.Date()
		if ey == y && em == m && ed == d {
			return true
		}
		// For multi-day / all-day events, check if dt falls within the range.
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

// eventsForDate returns all events that fall on the given date.
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

// daysIn returns the number of days in the given month and year.
func daysIn(m time.Month, year int) int {
	return time.Date(year, m+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// ---------------------------------------------------------------------------
// ICS Parsing
// ---------------------------------------------------------------------------

// ParseICSFile reads a .ics file at the given path and returns a slice of
// CalendarEvent values extracted from VEVENT blocks.
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

		// Parse property:value.  Handle properties with parameters like
		// DTSTART;VALUE=DATE:20260306
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

// icsKeyValue splits a line like "DTSTART;VALUE=DATE:20260306" into
// key="DTSTART;VALUE=DATE" and value="20260306".
func icsKeyValue(line string) (string, string) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return line, ""
	}
	return line[:idx], line[idx+1:]
}

// icsBaseProp extracts the base property name, stripping any parameters.
// e.g. "DTSTART;VALUE=DATE" -> "DTSTART"
func icsBaseProp(key string) string {
	if idx := strings.Index(key, ";"); idx >= 0 {
		return key[:idx]
	}
	return key
}

// parseICSTime attempts to parse a DTSTART / DTEND value.
// Supported formats:
//   - "20060102T150405Z" (UTC datetime)
//   - "20060102T150405"  (local datetime)
//   - "20060102"         (date only, marks as all-day)
//
// Returns the parsed time and whether it is an all-day event.
func parseICSTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)

	// Full datetime with Z suffix (UTC).
	if t, err := time.Parse("20060102T150405Z", value); err == nil {
		return t.Local(), false
	}
	// Full datetime without Z (treat as local).
	if t, err := time.Parse("20060102T150405", value); err == nil {
		return t, false
	}
	// Date only.
	if t, err := time.Parse("20060102", value); err == nil {
		return t, true
	}

	return time.Time{}, false
}
