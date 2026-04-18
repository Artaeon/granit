package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Calendar rendering — all view methods extracted from calendar.go
// ---------------------------------------------------------------------------

// plannerBlockTag returns the 3-char "[X]" prefix used in the agenda view
// to distinguish block kinds at a glance.
func plannerBlockTag(blockType string) string {
	switch strings.ToLower(blockType) {
	case "task", "deep-work", "deep_work":
		return "[T]"
	case "focus":
		return "[F]"
	case "break", "lunch":
		return "[B]"
	case "meeting", "event":
		return "[E]"
	case "admin":
		return "[A]"
	case "habit":
		return "[H]"
	case "review":
		return "[R]"
	case "pomodoro":
		return "[🍅]"
	}
	return "[P]"
}

// plannerBlockColor picks the background color for a planner block by its
// type tag. Covers every kind emitted by the schedule generators
// (taskmanager, PlanMyDay, AI scheduler, MorningRoutine) so no slot ever
// falls through to the grey default unless it really is uncategorised.
// Done blocks render in surface2 regardless of type.
func plannerBlockColor(blockType string, done bool) lipgloss.Color {
	if done {
		return surface2
	}
	switch strings.ToLower(blockType) {
	case "task", "deep-work", "deep_work":
		return blue
	case "focus":
		return peach
	case "break", "lunch":
		return green
	case "meeting", "event":
		return lavender
	case "admin":
		return overlay1
	case "habit":
		return teal
	case "review":
		return mauve
	case "pomodoro":
		return red
	}
	return lavender
}

func (c Calendar) viewMonth() string {
	width := c.width * 2 / 3
	if width < 86 {
		width = 86
	}
	if width > 100 {
		width = 100
	}

	var b strings.Builder
	now := time.Now()

	// Title bar — consistent with week and day views
	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	viewLabel := DimStyle.Render(" [month]")
	clockLabel := lipgloss.NewStyle().Foreground(green).Bold(true).Render(now.Format("15:04"))
	headerLeft := "  " + titleIcon + titleText + viewLabel
	headerGap := width - lipgloss.Width(headerLeft) - lipgloss.Width(clockLabel) - 6
	if headerGap < 1 {
		headerGap = 1
	}
	b.WriteString(headerLeft + strings.Repeat(" ", headerGap) + clockLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", width-8)))
	b.WriteString("\n")

	// Month/Year header
	monthYear := c.viewing.Format("January 2006")
	navLeft := lipgloss.NewStyle().Foreground(overlay1).Render("< ")
	navRight := lipgloss.NewStyle().Foreground(overlay1).Render(" >")
	header := navLeft + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(monthYear) + navRight
	gridWidth := 3 + 7*monthCellWidth // wk col + 7 day cells
	headerPad := (gridWidth - lipgloss.Width(header)) / 2
	if headerPad < 0 {
		headerPad = 0
	}
	b.WriteString("  " + strings.Repeat(" ", headerPad) + header)
	b.WriteString("\n")

	// Month summary: count events and tasks for this month
	year, month, _ := c.viewing.Date()
	monthEventCount := 0
	monthTaskPending := 0
	monthTaskDone := 0
	for d := 1; d <= daysIn(month, year); d++ {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, int(month), d)
		dt := time.Date(year, month, d, 0, 0, 0, 0, time.Local)
		monthEventCount += len(c.eventsForDate(dt))
		for _, t := range c.tasks[dateStr] {
			if t.Done {
				monthTaskDone++
			} else {
				monthTaskPending++
			}
		}
	}
	var monthParts []string
	if monthEventCount > 0 {
		monthParts = append(monthParts, lipgloss.NewStyle().Foreground(blue).Render(fmt.Sprintf("%d events", monthEventCount)))
	}
	if monthTaskPending > 0 {
		monthParts = append(monthParts, lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf("%d tasks", monthTaskPending)))
	}
	if monthTaskDone > 0 {
		monthParts = append(monthParts, lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("✓%d done", monthTaskDone)))
	}
	if len(monthParts) > 0 {
		summaryStr := strings.Join(monthParts, DimStyle.Render(" · "))
		summaryPad := (gridWidth - lipgloss.Width(summaryStr)) / 2
		if summaryPad < 2 {
			summaryPad = 2
		}
		b.WriteString(strings.Repeat(" ", summaryPad) + summaryStr)
	}
	b.WriteString("\n")

	// Day-of-week header with week number column
	wkStyle := lipgloss.NewStyle().Foreground(surface1)
	dayHeaderStyle := lipgloss.NewStyle().Foreground(subtext0).Bold(true)
	weekendHeaderStyle := lipgloss.NewStyle().Foreground(overlay0).Bold(true)
	dayNames := []string{"Mo", "Tu", "We", "Th", "Fr", "Sa", "Su"}
	var dayRow strings.Builder
	dayRow.WriteString("  " + wkStyle.Render("Wk") + " ")
	for i, d := range dayNames {
		// Center the 2-char day name within monthCellWidth
		padLeft := (monthCellWidth - 2) / 2
		padRight := monthCellWidth - 2 - padLeft
		styled := dayHeaderStyle.Render(d)
		if i == 5 || i == 6 {
			styled = weekendHeaderStyle.Render(d)
		}
		dayRow.WriteString(strings.Repeat(" ", padLeft) + styled + strings.Repeat(" ", padRight))
	}
	b.WriteString(dayRow.String())
	b.WriteString("\n")

	// Calendar grid (Monday-first, matching sidebar calendar panel)
	firstOfMonth := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	// Convert Go's Sunday=0 weekday to Monday=0: Sun→6, Mon→0, Tue→1, ...
	startWeekday := (int(firstOfMonth.Weekday()) + 6) % 7
	daysInMonth := daysIn(month, year)

	prevMonth := firstOfMonth.AddDate(0, 0, -1)
	prevDays := daysIn(prevMonth.Month(), prevMonth.Year())

	// Each "week" emits 3 visual lines: day-numbers, pill row 1, pill row 2.
	// We collect 7 cells per week then assemble.
	type weekCell struct{ num, p1, p2 string }
	weekIndent := "  "
	emptyWkCol := strings.Repeat(" ", 3)

	emitWeek := func(cells []weekCell, weekNum int) {
		b.WriteString(weekIndent + wkStyle.Render(fmt.Sprintf("%2d", weekNum)) + " ")
		for _, c := range cells {
			b.WriteString(c.num)
		}
		b.WriteString("\n")
		b.WriteString(weekIndent + emptyWkCol)
		for _, c := range cells {
			b.WriteString(c.p1)
		}
		b.WriteString("\n")
		b.WriteString(weekIndent + emptyWkCol)
		for _, c := range cells {
			b.WriteString(c.p2)
		}
		b.WriteString("\n")
	}

	cells := make([]weekCell, 0, 7)
	col := 0
	curWeekNum := 0
	if startWeekday > 0 {
		_, curWeekNum = firstOfMonth.AddDate(0, 0, -startWeekday).ISOWeek()
	} else {
		_, curWeekNum = firstOfMonth.ISOWeek()
	}

	// Leading days from previous month (dim)
	for i := 0; i < startWeekday; i++ {
		day := prevDays - startWeekday + 1 + i
		isWeekend := i == 5 || i == 6
		num, p1, p2 := c.renderMonthCell(day, time.Time{}, false, false, false, 0, 0, false, true, isWeekend, nil)
		cells = append(cells, weekCell{num, p1, p2})
		col++
	}

	for d := 1; d <= daysInMonth; d++ {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, int(month), d)
		dt := time.Date(year, month, d, 0, 0, 0, 0, time.Local)

		isToday := dt.Equal(c.today)
		isCursor := dt.Equal(c.cursor)
		hasNote := c.dailyNoteDates[dateStr]
		evs := c.eventsForDate(dt)
		tasksDone, tasksTotal := c.taskStats(dateStr)
		isWeekend := dt.Weekday() == time.Sunday || dt.Weekday() == time.Saturday

		num, p1, p2 := c.renderMonthCell(d, dt, isToday, isCursor, hasNote, tasksDone, tasksTotal, true, false, isWeekend, evs)
		cells = append(cells, weekCell{num, p1, p2})
		col++

		if col == 7 {
			emitWeek(cells, curWeekNum)
			cells = cells[:0]
			col = 0
			if d < daysInMonth {
				nextDate := dt.AddDate(0, 0, 1)
				_, curWeekNum = nextDate.ISOWeek()
			}
		}
	}

	if col > 0 {
		nextDay := 1
		for col < 7 {
			isWeekend := col == 5 || col == 6
			num, p1, p2 := c.renderMonthCell(nextDay, time.Time{}, false, false, false, 0, 0, false, true, isWeekend, nil)
			cells = append(cells, weekCell{num, p1, p2})
			col++
			nextDay++
		}
		emitWeek(cells, curWeekNum)
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
	// Use full terminal width for the weekly calendar (like Google Calendar)
	width := c.width - 2
	if width < 90 {
		width = 90
	}

	var b strings.Builder
	now := time.Now()

	// Week boundaries
	weekStart := c.cursor.AddDate(0, 0, -int(c.cursor.Weekday()))
	weekEnd := weekStart.AddDate(0, 0, 6)
	_, weekNum := c.cursor.ISOWeek()

	// ── Title bar ──────────────────────────────────────────────────────────
	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	viewLabel := DimStyle.Render(" [week]")
	weekLabel := lipgloss.NewStyle().Foreground(text).Render(
		fmt.Sprintf("  W%d – %s – %s", weekNum,
			weekStart.Format("Apr 2"), weekEnd.Format("Apr 2, 2006")))
	// Show cursor date below title
	cursorDateLabel := lipgloss.NewStyle().Foreground(overlay1).Render(
		"  " + c.cursor.Format("Monday 2"))
	b.WriteString("  " + titleIcon + titleText + viewLabel + weekLabel + "\n")
	b.WriteString("  " + cursorDateLabel + "\n")

	// ── Layout math ────────────────────────────────────────────────────────
	timeColW := 8
	dayColW := (width - timeColW - 9) / 7
	if dayColW < 14 {
		dayColW = 14
	}
	gridW := timeColW + dayColW*7 + 7

	// ── Separator helpers ──────────────────────────────────────────────────
	sepChar := lipgloss.NewStyle().Foreground(surface1).Render("│")
	todaySepChar := lipgloss.NewStyle().Foreground(green).Bold(true).Render("┃")
	sepFor := func(colIdx int) string {
		day := weekStart.AddDate(0, 0, colIdx)
		if sameDay(day, c.today) {
			return todaySepChar
		}
		// Also highlight the right edge of today's column
		if colIdx > 0 {
			prevDay := weekStart.AddDate(0, 0, colIdx-1)
			if sameDay(prevDay, c.today) {
				return todaySepChar
			}
		}
		return sepChar
	}

	// ── Day headers — boxed with per-day colors, today highlighted ─────────
	dayNamesShort := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	dayNamesFull := []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

	// Per-day accent colors for visual rhythm
	dayHeaderColors := []lipgloss.Color{
		peach,    // Sun — warm
		blue,     // Mon — fresh start
		sapphire, // Tue
		lavender, // Wed — midweek
		teal,     // Thu
		green,    // Fri — almost weekend
		peach,    // Sat — warm
	}

	// Top border
	topBorder := strings.Repeat(" ", timeColW)
	for i := 0; i < 7; i++ {
		day := weekStart.AddDate(0, 0, i)
		col := dayHeaderColors[i]
		if sameDay(day, c.today) {
			col = green
		}
		borderStyle := lipgloss.NewStyle().Foreground(col)
		topBorder += borderStyle.Render("┌" + strings.Repeat("─", dayColW-2) + "┐")
	}
	b.WriteString("  " + topBorder + "\n")

	// Day name + date row
	headerRow := strings.Repeat(" ", timeColW)
	for i := 0; i < 7; i++ {
		day := weekStart.AddDate(0, 0, i)
		dayName := dayNamesShort[i]
		if dayColW >= 18 {
			dayName = dayNamesFull[i]
		}
		dateNum := day.Format("2")
		label := dayName + " " + dateNum

		col := dayHeaderColors[i]
		style := lipgloss.NewStyle().Foreground(col).Bold(true)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			style = lipgloss.NewStyle().Foreground(col)
		}
		isHeaderToday := sameDay(day, c.today)
		if isHeaderToday {
			col = green
			// Today: inverted green background for maximum prominence
			style = lipgloss.NewStyle().Foreground(crust).Background(green).Bold(true)
		}
		if sameDay(day, c.cursor) && !isHeaderToday {
			style = lipgloss.NewStyle().Foreground(mauve).Bold(true)
		}
		truncLabel := TruncateDisplay(label, dayColW-4)
		cell := style.Render(truncLabel)
		padLen := dayColW - 4 - lipgloss.Width(cell)
		if padLen < 0 {
			padLen = 0
		}
		left := lipgloss.NewStyle().Foreground(col).Render("│")
		right := lipgloss.NewStyle().Foreground(col).Render("│")
		padStr := strings.Repeat(" ", padLen)
		if isHeaderToday {
			// Fill the padding with the green background too
			padStr = lipgloss.NewStyle().Background(green).Render(padStr)
		}
		headerRow += left + " " + cell + padStr + " " + right
	}
	b.WriteString("  " + headerRow + "\n")

	// Bottom border
	botBorder := strings.Repeat(" ", timeColW)
	for i := 0; i < 7; i++ {
		day := weekStart.AddDate(0, 0, i)
		col := dayHeaderColors[i]
		if sameDay(day, c.today) {
			col = green
		}
		botBorder += lipgloss.NewStyle().Foreground(col).Render("└" + strings.Repeat("─", dayColW-2) + "┘")
	}
	b.WriteString("  " + botBorder + "\n")

	// ── All-day events row ─────────────────────────────────────────────────
	hasAllDay := false
	for di := 0; di < 7; di++ {
		if hasAllDay {
			break
		}
		day := weekStart.AddDate(0, 0, di)
		for _, ev := range c.eventsForDate(day) {
			if ev.AllDay {
				hasAllDay = true
				break
			}
		}
	}
	if hasAllDay {
		allDayCells := ""
		for di := 0; di < 7; di++ {
			day := weekStart.AddDate(0, 0, di)
			var allDayEvs []CalendarEvent
			for _, ev := range c.eventsForDate(day) {
				if ev.AllDay {
					allDayEvs = append(allDayEvs, ev)
				}
			}
			cellStr := ""
			if len(allDayEvs) > 0 {
				inner := dayColW - 2
				if inner < 2 {
					inner = 2
				}
				title := allDayEvs[0].Title
				if len(allDayEvs) > 1 {
					title += fmt.Sprintf(" +%d", len(allDayEvs)-1)
				}
				title = TruncateDisplay(title, inner)
				padded := title + strings.Repeat(" ", maxInt(0, inner-lipgloss.Width(title)))
				cellStr = lipgloss.NewStyle().Foreground(crust).Background(calEventColor(allDayEvs[0])).Bold(true).Render(" " + padded + " ")
			} else {
				cellStr = strings.Repeat(" ", dayColW)
			}
			pad := dayColW - lipgloss.Width(cellStr)
			if pad > 0 {
				cellStr += strings.Repeat(" ", pad)
			}
			allDayCells += cellStr
		}
		b.WriteString("  " + strings.Repeat(" ", timeColW) + allDayCells + "\n")
	}

	// ── Pre-compute event positions for each day ───────────────────────────
	type weekEntry struct {
		kind     string // "event", "planner"
		title    string
		color    lipgloss.Color
		startMin int
		endMin   int
		location string
	}
	type dayEntries struct {
		events []weekEntry
	}
	allDayEntries := make([]dayEntries, 7)
	for di := 0; di < 7; di++ {
		day := weekStart.AddDate(0, 0, di)
		dateStr := day.Format("2006-01-02")
		var entries []weekEntry

		for _, ev := range c.eventsForDate(day) {
			if ev.AllDay {
				continue
			}
			startMin := ev.Date.Hour()*60 + ev.Date.Minute()
			endMin := startMin + 60
			if !ev.EndDate.IsZero() {
				endMin = ev.EndDate.Hour()*60 + ev.EndDate.Minute()
				if endMin <= startMin {
					endMin = startMin + 60
				}
			}
			entries = append(entries, weekEntry{
				kind: "event", title: ev.Title, color: calEventColor(ev),
				startMin: startMin, endMin: endMin, location: ev.Location,
			})
		}

		for _, pb := range c.plannerBlocks[dateStr] {
			sH, sM := parseHHMM(pb.StartTime)
			eH, eM := parseHHMM(pb.EndTime)
			startMin := sH*60 + sM
			endMin := eH*60 + eM
			if endMin <= startMin {
				endMin = startMin + 60
			}
			entries = append(entries, weekEntry{
				kind: "planner", title: pb.Text, color: plannerBlockColor(pb.BlockType, pb.Done),
				startMin: startMin, endMin: endMin,
			})
		}
		allDayEntries[di] = dayEntries{events: entries}
	}

	// ── Determine visible hour range ───────────────────────────────────────
	maxHalfHours := (c.height - 12) * 1
	if maxHalfHours < 16 {
		maxHalfHours = 16
	}
	startHour := 6
	for di := 0; di < 7; di++ {
		for _, e := range allDayEntries[di].events {
			if e.startMin/60 < startHour && e.startMin/60 >= 4 {
				startHour = e.startMin / 60
			}
		}
	}
	endHour := startHour + maxHalfHours/2
	if endHour > 23 {
		endHour = 23
	}

	// ── Render time grid ───────────────────────────────────────────────────
	todayInWeek := !c.today.Before(weekStart) && c.today.Before(weekStart.AddDate(0, 0, 7))
	todayCol := -1
	if todayInWeek {
		todayCol = int(c.today.Sub(weekStart).Hours() / 24)
	}

	nowMins := now.Hour()*60 + now.Minute()
	nowLineDrawn := false

	cursorSlotMin := (startHour + c.weekGridCursorHour/2) * 60
	if c.weekGridCursorHour%2 == 1 {
		cursorSlotMin += 30
	}

	// ── Time-block colors ──────────────────────────────────────────────────
	// 4 time blocks with distinct, VISIBLE background tints and event colors.
	// The bg colors are lighter than base so empty cells are NOT black.
	type timeBlock struct {
		startH   int
		endH     int
		label    string
		bg       lipgloss.Color // empty cell background (lighter than base)
		fg       lipgloss.Color // label/separator color
		eventBg  lipgloss.Color // event block background in this time period
		eventFg  lipgloss.Color // event text color
	}
	timeBlocks := []timeBlock{
		{5, 10, "MORNING",
			base, lavender,                            // bg: theme base (clean)
			lipgloss.Color("#7B6BA6"), crust},         // events: medium purple
		{10, 15, "MIDDAY",
			base, sapphire,                            // bg: theme base
			lipgloss.Color("#5B7EA8"), crust},         // events: medium blue
		{15, 20, "AFTERNOON",
			base, peach,                               // bg: theme base
			lipgloss.Color("#B8875A"), crust},         // events: warm amber
		{20, 24, "EVENING",
			base, teal,                                // bg: theme base
			lipgloss.Color("#5A9E9E"), crust},         // events: medium teal
	}

	getTimeBlock := func(hour int) *timeBlock {
		for i := range timeBlocks {
			if hour >= timeBlocks[i].startH && hour < timeBlocks[i].endH {
				return &timeBlocks[i]
			}
		}
		return &timeBlocks[0]
	}

	// eventColorForSlot returns the event background color based on the time
	// block it falls in. Events with explicit colors (from user/ICS) keep theirs;
	// planner blocks and default-colored events get the time-block event color.
	eventColorForSlot := func(e weekEntry, hour int) lipgloss.Color {
		// If the event has a user-set color (not the default blue), keep it
		if e.color != blue {
			return e.color
		}
		// Otherwise, tint by time block
		tb := getTimeBlock(hour)
		return tb.eventBg
	}

	lastTimeBlockLabel := ""

	for hour := startHour; hour < endHour; hour++ {
		for half := 0; half < 2; half++ {
			slotMin := hour*60 + half*30
			isTopHalf := half == 0
			isCurrentSlot := todayInWeek && now.Hour() == hour && ((half == 0 && now.Minute() < 30) || (half == 1 && now.Minute() >= 30))

			// ── Time-block transition separator ────────────────────────
			if isTopHalf {
				for _, tb := range timeBlocks {
					if hour == tb.startH && tb.label != lastTimeBlockLabel {
						lastTimeBlockLabel = tb.label
						// Draw a colored separator with block label
						blockLabel := lipgloss.NewStyle().Foreground(tb.fg).Bold(true).
							Render(" " + tb.label + " ")
						sepLine := lipgloss.NewStyle().Foreground(tb.fg).
							Render(strings.Repeat("─", maxInt(1, gridW-lipgloss.Width(blockLabel)-2)))
						b.WriteString("  " + blockLabel + sepLine + "\n")
						break
					}
				}
			}

			// Time label — color-coded by time block
			tb := getTimeBlock(hour)
			var timeSt string
			if isCurrentSlot {
				timeSt = lipgloss.NewStyle().Foreground(green).Bold(true).
					Render(fmt.Sprintf(" ▸%02d:%02d ", now.Hour(), now.Minute()))
			} else if isTopHalf {
				timeSt = lipgloss.NewStyle().Foreground(tb.fg).
					Render(fmt.Sprintf("  %02d:00 ", hour))
			} else {
				timeSt = lipgloss.NewStyle().Foreground(surface2).
					Render("    :30 ")
			}

			// Build cells for each day
			cells := ""

			for di := 0; di < 7; di++ {
				day := weekStart.AddDate(0, 0, di)
				isToday := sameDay(day, c.today)
				isCursorCell := sameDay(day, c.cursor) && slotMin == cursorSlotMin

				// Find entries overlapping this slot
				var active *weekEntry
				overlapCount := 0
				for i := range allDayEntries[di].events {
					e := &allDayEntries[di].events[i]
					if e.startMin < slotMin+30 && e.endMin > slotMin {
						overlapCount++
						if active == nil {
							active = e
						}
					}
				}

				inner := dayColW - 2
				if inner < 4 {
					inner = 4
				}

				// Choose the cell background: today gets a subtle green tint,
				// others use the theme base color (clean, not black).
				var cellBg lipgloss.Color
				if isToday {
					cellBg = surface0 // slightly lighter than base for today
				} else {
					cellBg = tb.bg // theme base
				}

				// Build cell content — ALL cells get a background so no black gaps
				cellContent := ""
				if isCursorCell && active != nil {
					// Cursor on event: mauve highlight
					cursorStyle := lipgloss.NewStyle().Background(mauve).Foreground(crust).Bold(true).Width(inner + 1)
					isEntryStart := active.startMin >= slotMin && active.startMin < slotMin+30
					var curLabel string
					if isEntryStart {
						startH := active.startMin / 60
						startM := active.startMin % 60
						curLabel = fmt.Sprintf("%02d:%02d %s", startH, startM, active.title)
					} else {
						curLabel = "▎ " + active.title
					}
					cellContent = cursorStyle.Render(" " + TruncateDisplay(curLabel, inner-1))
				} else if active != nil {
					isEntryStart := active.startMin >= slotMin && active.startMin < slotMin+30

					var label string
					if isEntryStart {
						startH := active.startMin / 60
						startM := active.startMin % 60
						timeStr := fmt.Sprintf("%02d:%02d", startH, startM)
						label = timeStr + " " + active.title
						if active.location != "" && inner > 24 {
							label += " @" + active.location
						}
						if overlapCount > 1 {
							label = TruncateDisplay(label, inner-4) + fmt.Sprintf(" +%d", overlapCount-1)
						}
					} else {
						label = "  " + active.title
					}

					// Event color: time-block aware
					evColor := eventColorForSlot(*active, hour)
					evFg := active.color
					if evFg == blue {
						evFg = tb.eventFg
					} else {
						evFg = crust
					}
					blockStyle := lipgloss.NewStyle().Foreground(evFg).Background(evColor).Width(inner + 1)
					if isEntryStart {
						blockStyle = blockStyle.Bold(true)
					}
					cellContent = blockStyle.Render(" " + TruncateDisplay(label, inner-1))
				} else if isCursorCell {
					cursorStyle := lipgloss.NewStyle().Background(mauve).Foreground(crust).Bold(true).Width(inner + 1)
					cellContent = cursorStyle.Render(" ▎")
				} else {
					// Empty cell — filled with background color (NO black)
					cellContent = lipgloss.NewStyle().Background(cellBg).Width(inner + 1).Render("")
				}

				// Separator also gets background so no black line between columns
				sep := sepFor(di)
				cells += sep + cellContent
			}

			b.WriteString("  " + timeSt + cells + "\n")

			// ── Current-time red line ───────────────────────────────────
			if !nowLineDrawn && todayInWeek && nowMins >= slotMin && nowMins < slotMin+30 {
				nowLineDrawn = true
				timeLabel := lipgloss.NewStyle().Foreground(crust).Background(red).Bold(true).
					Render(fmt.Sprintf("▸%02d:%02d  ", now.Hour(), now.Minute()))
				var rowCells string
				for di := 0; di < 7; di++ {
					inner := dayColW - 2
					if inner < 1 {
						inner = 1
					}
					var cell string
					if di == todayCol {
						// Bold red background line for today
						cell = lipgloss.NewStyle().Foreground(crust).Background(red).Bold(true).
							Width(inner + 1).Render(" NOW")
					} else {
						cell = lipgloss.NewStyle().Foreground(red).
							Render(strings.Repeat("╌", inner))
					}
					rowCells += sepFor(di) + PadRight(cell, dayColW-1)
				}
				b.WriteString("  " + timeLabel + rowCells + "\n")
			}
		}
	}

	// ── Event Details panel ────────────────────────────────────────────────
	// Persistent detail panel at the bottom showing full info for cursor event
	b.WriteString("  " + lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", gridW)) + "\n")
	cursorDay := int(c.cursor.Weekday())
	foundDetail := false
	if cursorDay >= 0 && cursorDay < 7 {
		for _, e := range allDayEntries[cursorDay].events {
			if e.startMin < cursorSlotMin+30 && e.endMin > cursorSlotMin {
				foundDetail = true
				// Header
				detailTitle := lipgloss.NewStyle().Foreground(overlay1).Bold(true).Render("  Event Details")
				b.WriteString(detailTitle + "\n")
				// Colored left bar + event name
				colorBar := lipgloss.NewStyle().Background(e.color).Render("  ")
				nameStyled := lipgloss.NewStyle().Foreground(text).Bold(true).Render("  " + e.title)
				b.WriteString(colorBar + nameStyled + "\n")
				// Time range + duration
				startH, startM := e.startMin/60, e.startMin%60
				endH, endM := e.endMin/60, e.endMin%60
				dur := e.endMin - e.startMin
				timeRange := fmt.Sprintf("%02d:%02d – %02d:%02d", startH, startM, endH, endM)
				durLabel := FormatMinutes(dur)
				b.WriteString("  " + lipgloss.NewStyle().Foreground(teal).Render("  Time:     ") +
					lipgloss.NewStyle().Foreground(text).Render(timeRange) +
					DimStyle.Render(" ["+durLabel+"]") + "\n")
				// Location
				if e.location != "" {
					b.WriteString("  " + lipgloss.NewStyle().Foreground(teal).Render("  Location: ") +
						lipgloss.NewStyle().Foreground(text).Render(e.location) + "\n")
				}
				// Type
				typeLabel := e.kind
				if e.kind == "planner" {
					typeLabel = "scheduled block"
				}
				b.WriteString("  " + lipgloss.NewStyle().Foreground(teal).Render("  Type:     ") +
					DimStyle.Render(typeLabel) + "\n")
				break
			}
		}
	}
	if !foundDetail {
		b.WriteString("  " + DimStyle.Render("  Navigate to an event to see details") + "\n")
	}

	// ── Modals ─────────────────────────────────────────────────────────────
	if c.addingEvent {
		c.renderQuickAdd(&b, width)
	}
	c.renderEventWizard(&b, width)

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

	if c.weekMilestoneMode && len(c.activeGoals) > 0 {
		b.WriteString("\n")
		sepStyle := lipgloss.NewStyle().Foreground(surface0)
		b.WriteString("  " + sepStyle.Render(strings.Repeat("─", width-8)) + "\n")
		titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		if c.weekMilestoneStep == 0 {
			b.WriteString(titleStyle.Render("  Add milestone — select goal:") + "\n")
			maxShow := 8
			start := 0
			if c.weekMilestoneCursor >= maxShow {
				start = c.weekMilestoneCursor - maxShow + 1
			}
			end := start + maxShow
			if end > len(c.activeGoals) {
				end = len(c.activeGoals)
			}
			for i := start; i < end; i++ {
				g := c.activeGoals[i]
				prefix := "  "
				style := DimStyle
				if i == c.weekMilestoneCursor {
					prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▸ ")
					style = lipgloss.NewStyle().Foreground(text)
				}
				prog := fmt.Sprintf(" (%d%%)", g.Progress())
				b.WriteString("  " + prefix + style.Render(TruncateDisplay(g.Title, width-20)) + DimStyle.Render(prog) + "\n")
			}
			b.WriteString("  " + DimStyle.Render("Enter:select  Esc:cancel") + "\n")
		} else {
			goalTitle := ""
			for _, g := range c.activeGoals {
				if g.ID == c.weekMilestoneGoalID {
					goalTitle = g.Title
					break
				}
			}
			b.WriteString(titleStyle.Render("  Milestone for: "+TruncateDisplay(goalTitle, width-24)) + "\n")
			inputStyle := lipgloss.NewStyle().Foreground(text).Background(surface0).Padding(0, 1)
			cursor := lipgloss.NewStyle().Foreground(mauve).Render("█")
			b.WriteString("  " + inputStyle.Render(c.weekMilestoneBuf+cursor) + "\n")
			b.WriteString("  " + DimStyle.Render("Enter:save  Esc:cancel") + "\n")
		}
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
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	var b strings.Builder
	now := time.Now()
	isToday := c.cursor.Equal(c.today)
	dateStr := c.cursor.Format("2006-01-02")

	// ── Title bar ──────────────────────────────────────────────────────────
	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Calendar")
	viewLabel := DimStyle.Render(" [day]")
	b.WriteString("  " + titleIcon + titleText + viewLabel)
	b.WriteString("\n")

	// Date header with weekday
	dayLabel := c.cursor.Format("Monday, January 2, 2006")
	dayStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	if isToday {
		dayStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
	}
	b.WriteString("  " + dayStyle.Render(dayLabel))
	if isToday {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("(today)"))
	}
	b.WriteString("\n")

	// ── Summary strip: event count + task count + hours busy ───────────────
	dayEvents := c.eventsForDate(c.cursor)
	timedEvCount := 0
	totalBusyMin := 0
	for _, ev := range dayEvents {
		if !ev.AllDay {
			timedEvCount++
			dur := 60
			if !ev.EndDate.IsZero() {
				dur = int(ev.EndDate.Sub(ev.Date).Minutes())
			}
			totalBusyMin += dur
		}
	}
	pending, doneCount := 0, 0
	if tasks, ok := c.tasks[dateStr]; ok {
		for _, t := range tasks {
			if t.Done {
				doneCount++
			} else {
				pending++
			}
		}
	}
	var summaryParts []string
	if timedEvCount > 0 {
		busyLabel := fmt.Sprintf("%d events", timedEvCount)
		if totalBusyMin >= 60 {
			busyLabel += fmt.Sprintf(" %dh%02dm", totalBusyMin/60, totalBusyMin%60)
		}
		summaryParts = append(summaryParts, makePill(blue, busyLabel))
	}
	if pending > 0 {
		summaryParts = append(summaryParts, makePill(yellow, fmt.Sprintf("%d tasks", pending)))
	}
	if doneCount > 0 {
		summaryParts = append(summaryParts, makePill(green, fmt.Sprintf("✓%d done", doneCount)))
	}
	if len(summaryParts) > 0 {
		b.WriteString("  " + strings.Join(summaryParts, " ") + "\n")
	}

	// ── Daily focus banner ─────────────────────────────────────────────────
	if focus, ok := c.dailyGoals[dateStr]; ok && focus.TopGoal != "" {
		focusPill := makePill(peach, "FOCUS")
		focusText := lipgloss.NewStyle().Foreground(text).Italic(true).Render(" " + focus.TopGoal)
		b.WriteString("  " + focusPill + focusText + "\n")
	}

	// ── All-day events ─────────────────────────────────────────────────────
	hasAllDay := false
	for _, ev := range dayEvents {
		if ev.AllDay {
			if !hasAllDay {
				hasAllDay = true
				b.WriteString("  " + lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("All Day") + "  ")
			}
			pill := lipgloss.NewStyle().Foreground(crust).Background(calEventColor(ev)).Bold(true).Padding(0, 1).Render(ev.Title)
			b.WriteString(pill + " ")
		}
	}
	if hasAllDay {
		b.WriteString("\n")
	}

	// ── "Up next" strip ────────────────────────────────────────────────────
	if isToday {
		var upcoming []CalendarEvent
		for _, ev := range dayEvents {
			if ev.AllDay || !ev.Date.After(now) {
				continue
			}
			upcoming = append(upcoming, ev)
		}
		sort.Slice(upcoming, func(i, j int) bool {
			return upcoming[i].Date.Before(upcoming[j].Date)
		})
		if len(upcoming) > 0 {
			var parts []string
			for i, ev := range upcoming {
				if i >= 3 {
					parts = append(parts, DimStyle.Render(fmt.Sprintf("+%d more", len(upcoming)-3)))
					break
				}
				when := ev.Date.Format("15:04")
				untilMin := int(ev.Date.Sub(now).Minutes())
				untilStr := ""
				if untilMin > 0 {
					if untilMin >= 60 {
						untilStr = fmt.Sprintf(" in %dh%dm", untilMin/60, untilMin%60)
					} else {
						untilStr = fmt.Sprintf(" in %dm", untilMin)
					}
				}
				pill := lipgloss.NewStyle().Foreground(crust).Background(calEventColor(ev)).Bold(true).Padding(0, 1).
					Render(when + " " + TruncateDisplay(ev.Title, 20))
				parts = append(parts, pill+lipgloss.NewStyle().Foreground(teal).Render(untilStr))
			}
			header := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("  Up next  ")
			b.WriteString(header + strings.Join(parts, " ") + "\n")
		}
	}

	b.WriteString("  " + lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("─", width-8)) + "\n")

	// ── Collect entries ────────────────────────────────────────────────────
	type activeEntry struct {
		kind     string
		title    string
		color    lipgloss.Color
		startMin int
		endMin   int
		location string
	}
	var entries []activeEntry
	for _, ev := range dayEvents {
		if ev.AllDay {
			continue
		}
		startMin := ev.Date.Hour()*60 + ev.Date.Minute()
		endMin := startMin + 60
		if !ev.EndDate.IsZero() {
			endMin = ev.EndDate.Hour()*60 + ev.EndDate.Minute()
			if endMin <= startMin {
				endMin = startMin + 60
			}
		}
		entries = append(entries, activeEntry{
			kind: "event", title: ev.Title, color: calEventColor(ev),
			startMin: startMin, endMin: endMin, location: ev.Location,
		})
	}
	for _, pb := range c.plannerBlocks[dateStr] {
		var sH, sM, eH, eM int
		_, _ = fmt.Sscanf(pb.StartTime, "%d:%d", &sH, &sM)
		_, _ = fmt.Sscanf(pb.EndTime, "%d:%d", &eH, &eM)
		startMin := sH*60 + sM
		endMin := eH*60 + eM
		if endMin <= startMin {
			endMin = startMin + 60
		}
		entries = append(entries, activeEntry{
			kind: "planner", title: pb.Text, color: plannerBlockColor(pb.BlockType, pb.Done),
			startMin: startMin, endMin: endMin,
		})
	}

	contentW := width - 14
	if contentW < 20 {
		contentW = 20
	}
	nowMins := now.Hour()*60 + now.Minute()
	nowLineDrawn := false

	// ── Half-hour time grid 06:00–22:00 ────────────────────────────────────
	for hour := 6; hour <= 22; hour++ {
		for half := 0; half < 2; half++ {
			slotMin := hour*60 + half*30
			isTopHalf := half == 0
			isCurrentSlot := isToday && nowMins >= slotMin && nowMins < slotMin+30

			// Time label — green ▸ only on the slot that contains now
			var timeSt string
			if isTopHalf {
				if isCurrentSlot {
					timeSt = lipgloss.NewStyle().Foreground(green).Bold(true).
						Render(fmt.Sprintf("  ▸%02d:%02d ", now.Hour(), now.Minute()))
				} else {
					timeSt = DimStyle.Render(fmt.Sprintf("  %02d:00  ", hour))
				}
			} else {
				if isCurrentSlot {
					timeSt = lipgloss.NewStyle().Foreground(green).Bold(true).
						Render(fmt.Sprintf("  ▸%02d:%02d ", now.Hour(), now.Minute()))
				} else {
					timeSt = DimStyle.Render("     :30  ")
				}
			}

			// Find active entries for this slot
			var active *activeEntry
			activeCount := 0
			for i := range entries {
				e := &entries[i]
				if e.startMin < slotMin+30 && e.endMin > slotMin {
					activeCount++
					if active == nil {
						active = e
					}
				}
			}

			if active != nil {
				isEntryStart := active.startMin >= slotMin && active.startMin < slotMin+30
				inner := contentW - 2
				if inner < 4 {
					inner = 4
				}

				var label string
				if isEntryStart {
					startH, startM := active.startMin/60, active.startMin%60
					endH, endM := active.endMin/60, active.endMin%60
					timeRange := fmt.Sprintf("%02d:%02d–%02d:%02d", startH, startM, endH, endM)
					label = timeRange + "  " + active.title
					if active.location != "" {
						label += "  @ " + active.location
					}
					if activeCount > 1 {
						label = TruncateDisplay(label, inner-4) + fmt.Sprintf(" +%d", activeCount-1)
					}
				} else {
					// Continuation bar
					label = "▏"
				}
				label = TruncateDisplay(label, inner)
				padLen := inner - lipgloss.Width(label)
				if padLen < 0 {
					padLen = 0
				}
				label += strings.Repeat(" ", padLen)
				blockStyle := lipgloss.NewStyle().Foreground(crust).Background(active.color)
				if isEntryStart {
					blockStyle = blockStyle.Bold(true)
				}
				b.WriteString(timeSt + blockStyle.Render(" "+label+" "))
			} else if isTopHalf {
				b.WriteString(timeSt + DimStyle.Render("┊"))
			} else {
				b.WriteString(timeSt + DimStyle.Render("·"))
			}
			b.WriteString("\n")

			// Current-time indicator
			if !nowLineDrawn && isToday && nowMins >= slotMin && nowMins < slotMin+30 {
				nowLineDrawn = true
				nowLabel := lipgloss.NewStyle().Foreground(red).Bold(true).
					Render(fmt.Sprintf("  %02d:%02d  ", now.Hour(), now.Minute()))
				nowLine := lipgloss.NewStyle().Foreground(red).Bold(true).
					Render(strings.Repeat("━", contentW))
				b.WriteString(nowLabel + nowLine + "\n")
			}
		}
	}

	// ── Tasks due this day ─────────────────────────────────────────────────
	if tasks, ok := c.tasks[dateStr]; ok && len(tasks) > 0 {
		b.WriteString("\n")
		taskHeader := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Tasks")
		// Progress indicator
		taskProg := ""
		if doneCount+pending > 0 {
			progBarW := 6
			progFilled := progBarW * doneCount / (doneCount + pending)
			taskProg = " " + lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", progFilled)) +
				lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", progBarW-progFilled)) +
				DimStyle.Render(fmt.Sprintf(" %d/%d", doneCount, doneCount+pending))
		}
		b.WriteString(taskHeader + taskProg + "\n")
		for _, t := range tasks {
			check := DimStyle.Render("  [ ] ")
			textStyle := lipgloss.NewStyle().Foreground(text)
			if t.Done {
				check = lipgloss.NewStyle().Foreground(green).Render("  [x] ")
				textStyle = lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true)
			}
			prioIcon := ""
			switch t.Priority {
			case 4:
				prioIcon = lipgloss.NewStyle().Foreground(red).Render("!! ")
			case 3:
				prioIcon = lipgloss.NewStyle().Foreground(peach).Render("!  ")
			}
			b.WriteString(check + prioIcon + textStyle.Render(TruncateDisplay(t.Text, width-16)) + "\n")
		}
	}

	// ── Habit progress ─────────────────────────────────────────────────────
	if hDone, hTotal := c.habitStats(dateStr); hTotal > 0 {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("─", width-12)) + "\n")
		habitHeader := lipgloss.NewStyle().Foreground(green).Bold(true).Render("  Habits")
		habitCount := fmt.Sprintf(" %d/%d", hDone, hTotal)
		b.WriteString(habitHeader + DimStyle.Render(habitCount))
		// Mini progress bar
		barW := 8
		filled := barW * hDone / hTotal
		bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled)) +
			lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", barW-filled))
		b.WriteString("  " + bar + "\n")
	}

	// ── Modals ─────────────────────────────────────────────────────────────
	if c.eventEditMode > 0 {
		c.renderEventWizard(&b, width)
	}
	if c.confirmDelete {
		c.renderConfirmDelete(&b)
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
			tag := plannerBlockTag(pb.BlockType)
			pbText := TruncateDisplay(pb.Text, width-22)
			pbStyle := lipgloss.NewStyle().Foreground(plannerBlockColor(pb.BlockType, false))
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

// renderEventWizard draws the full event form: every field visible at once,
// with the focused row marked by a ▸ arrow and the rest shown dim.
// No-op when the form is not open (some callers invoke this unconditionally).
func (c Calendar) renderEventWizard(b *strings.Builder, width int) {
	if c.eventEditMode == 0 {
		return
	}
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	dimStyle := DimStyle

	dateLabel := c.cursor.Format("Mon, Jan 2 2006")
	heading := titleStyle.Render("New Event")
	b.WriteString("\n  " + heading + "  " + dimStyle.Render(dateLabel) + "\n")
	b.WriteString("  " + dimStyle.Render(strings.Repeat("─", 40)) + "\n")

	for field := 0; field < eventFormCount; field++ {
		c.renderEventFormField(b, field)
	}

	b.WriteString("\n  " + dimStyle.Render(
		"Tab/↓ next · Shift+Tab/↑ prev · Enter save · Esc cancel"))
	b.WriteString("\n")
}

// renderEventFormField renders one field row (label + value + inline choice hints).
// The focused field shows an input cursor on text fields or highlights the
// selected option on choice fields.
func (c Calendar) renderEventFormField(b *strings.Builder, field int) {
	focused := field == c.eventEditField

	// Row prefix: ▸ on focus, space otherwise.
	prefix := "   "
	if focused {
		prefix = lipgloss.NewStyle().Foreground(peach).Bold(true).Render(" ▸ ")
	}

	labelStyle := lipgloss.NewStyle().Foreground(subtext0)
	if focused {
		labelStyle = lipgloss.NewStyle().Foreground(mauve).Bold(true)
	}
	label := labelStyle.Render(PadRight(eventFormFieldNames[field]+":", 13))

	var value string
	switch field {
	case efTitle:
		value = c.renderTextFieldValue(c.eventEditTitle, focused, "(required)")
	case efTime:
		value = c.renderTextFieldValue(c.eventEditTime, focused, "(empty = all-day)")
	case efDuration:
		value = c.renderTextFieldValue(c.eventEditDurBuf, focused, fmt.Sprintf("%d min (default)", c.eventEditDur))
		if focused {
			value += "  " + DimStyle.Render("e.g. 30 / 60 / 90 / 120")
		}
	case efLocation:
		value = c.renderTextFieldValue(c.eventEditLoc, focused, "(optional)")
	case efDescription:
		value = c.renderTextFieldValue(c.eventEditDesc, focused, "(optional)")
	case efRecurrence:
		value = c.renderChoiceFieldValue(c.eventEditRecur, recurrenceOptions, focused)
	case efColor:
		value = c.renderColorChoice(c.eventEditColor, focused)
	}

	b.WriteString(prefix + label + value + "\n")
}

// renderTextFieldValue renders a text field's current value. When focused,
// a cursor block is appended. Empty values show a dim placeholder.
func (c Calendar) renderTextFieldValue(value string, focused bool, placeholder string) string {
	if focused {
		cursor := lipgloss.NewStyle().Foreground(text).Background(surface1).Render(" ")
		if value == "" {
			return DimStyle.Render(placeholder) + cursor
		}
		return lipgloss.NewStyle().Foreground(text).Render(value) + cursor
	}
	if value == "" {
		return DimStyle.Italic(true).Render(placeholder)
	}
	return lipgloss.NewStyle().Foreground(text).Render(value)
}

// renderChoiceFieldValue renders a single-select field as inline pills.
// The selected option is highlighted; digit-key shortcuts brighten when focused.
func (c Calendar) renderChoiceFieldValue(current string, opts []struct{ key, value, label string }, focused bool) string {
	keyStyle := DimStyle
	if focused {
		keyStyle = lipgloss.NewStyle().Foreground(lavender).Bold(true)
	}
	var parts []string
	for _, opt := range opts {
		keyHint := keyStyle.Render(opt.key)
		var label string
		if opt.value == current {
			label = lipgloss.NewStyle().Foreground(crust).Background(peach).Bold(true).Padding(0, 1).Render(opt.label)
		} else if focused {
			label = lipgloss.NewStyle().Foreground(subtext0).Render(opt.label)
		} else {
			label = DimStyle.Render(opt.label)
		}
		parts = append(parts, keyHint+":"+label)
	}
	return strings.Join(parts, "  ")
}

// renderColorChoice is like renderChoiceFieldValue but each option pill uses
// its own background color so the visual picker shows what the user will get.
func (c Calendar) renderColorChoice(current string, focused bool) string {
	colorFor := func(name string) lipgloss.Color {
		switch name {
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

	keyStyle := DimStyle
	if focused {
		keyStyle = lipgloss.NewStyle().Foreground(lavender).Bold(true)
	}
	var parts []string
	for _, opt := range colorOptions {
		keyHint := keyStyle.Render(opt.key)
		bg := colorFor(opt.value)
		body := " " + opt.label + " "
		pill := lipgloss.NewStyle().Foreground(crust).Background(bg).Render(body)
		if opt.value == current {
			pill = lipgloss.NewStyle().Foreground(crust).Background(bg).Bold(true).Underline(true).Render(body)
		}
		parts = append(parts, keyHint+":"+pill)
	}
	return strings.Join(parts, " ")
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
			// Show event titles inline with colored bullets
			for i, ev := range dayEvents {
				if i >= 4 {
					more := DimStyle.Render(fmt.Sprintf("    +%d more events", len(dayEvents)-4))
					b.WriteString(more + "\n")
					break
				}
				evCol := calEventColor(ev)
				bullet := lipgloss.NewStyle().Foreground(evCol).Render("  ● ")
				timeStr := ""
				if !ev.AllDay {
					timeStr = ev.Date.Format("15:04")
					if !ev.EndDate.IsZero() {
						timeStr += "–" + ev.EndDate.Format("15:04")
					}
					timeStr += " "
				} else {
					timeStr = "all-day "
				}
				timePart := lipgloss.NewStyle().Foreground(teal).Render(timeStr)
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
			{"←→", "day"}, {"↑↓", "½hr"}, {"[]", "month"}, {"w", "view"},
			{"t", "today"}, {"a", "add"}, {"e", "edit"}, {"d", "del-evt"},
			{"b", "block"}, {",.", "shift ±15m"}, {"D", "unsched"}, {"Esc", "close"},
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

// monthCellWidth is the visual width of a single day cell in the month grid.
// At 11 chars, event pills can show ~8-char titles (e.g., "·Planning ").
// Grid width: Wk(3) + 7×11 = 80 — fits standard terminals.
const monthCellWidth = 11

// padToCell pads styled content to exactly monthCellWidth visual columns.
func padToCell(s string) string {
	w := lipgloss.Width(s)
	if w >= monthCellWidth {
		return s
	}
	return s + strings.Repeat(" ", monthCellWidth-w)
}

// renderEventPill renders an event title with a colored background, fitting the cell.
// Leaves 1 leading space before the pill so adjacent cells don't smear together.
func renderEventPill(title string, color lipgloss.Color) string {
	bullet := "·"
	innerCells := monthCellWidth - 1 - lipgloss.Width(bullet) // cells available for the title
	title = TruncateDisplay(title, innerCells)
	pad := innerCells - lipgloss.Width(title)
	if pad < 0 {
		pad = 0
	}
	body := bullet + title + strings.Repeat(" ", pad)
	pill := lipgloss.NewStyle().Foreground(crust).Background(color).Render(body)
	return " " + pill
}

// renderMorePill renders a "+N more" label, dim-styled, no background.
func renderMorePill(n int) string {
	text := fmt.Sprintf("+%d more", n)
	return padToCell(" " + lipgloss.NewStyle().Foreground(overlay1).Italic(true).Render(text))
}

// renderMonthCell returns the three visual lines for one month-grid cell:
// (day-number line, event-pill row 1, event-pill row 2). Each is exactly
// monthCellWidth visual columns wide.
func (c Calendar) renderMonthCell(
	day int,
	dt time.Time,
	isToday, isCursor, hasNote bool,
	tasksDone, tasksTotal int,
	currentMonth, dim, isWeekend bool,
	evs []CalendarEvent,
) (string, string, string) {
	numStr := fmt.Sprintf("%2d", day)

	var styled string
	switch {
	case isToday:
		styled = lipgloss.NewStyle().Background(green).Foreground(crust).Bold(true).Render(numStr)
	case isCursor:
		styled = lipgloss.NewStyle().Background(peach).Foreground(crust).Bold(true).Render(numStr)
	case !currentMonth || dim:
		styled = DimStyle.Render(numStr)
	case isWeekend:
		styled = lipgloss.NewStyle().Foreground(overlay0).Render(numStr)
	case hasNote:
		styled = lipgloss.NewStyle().Foreground(green).Render(numStr)
	default:
		styled = lipgloss.NewStyle().Foreground(text).Render(numStr)
	}

	badge := ""
	if currentMonth && !dim && tasksTotal > 0 {
		pending := tasksTotal - tasksDone
		if pending == 0 {
			// All done — green checkmark
			badge = " " + lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("✓%d", tasksDone))
		} else if tasksDone > 0 {
			// Partial — green done / yellow pending
			badge = " " + lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("%d", tasksDone)) +
				lipgloss.NewStyle().Foreground(surface2).Render("/") +
				lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf("%d", tasksTotal))
		} else {
			// None done — yellow count
			badge = " " + lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf("%d", pending))
		}
	}
	numLine := padToCell(" " + styled + badge)

	// Pill rows: only render for current-month, non-dim cells.
	pill1 := strings.Repeat(" ", monthCellWidth)
	pill2 := strings.Repeat(" ", monthCellWidth)
	if currentMonth && !dim && len(evs) > 0 {
		// Sort by time-of-day so morning events appear first.
		sorted := append([]CalendarEvent(nil), evs...)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].Date.Before(sorted[j].Date)
		})
		pill1 = renderEventPill(sorted[0].Title, calEventColor(sorted[0]))
		switch {
		case len(sorted) == 2:
			pill2 = renderEventPill(sorted[1].Title, calEventColor(sorted[1]))
		case len(sorted) > 2:
			pill2 = renderMorePill(len(sorted) - 1)
		}
	}
	return numLine, pill1, pill2
}
