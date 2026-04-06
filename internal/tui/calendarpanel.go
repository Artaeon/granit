package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// CalendarPanel renders a compact calendar + today's schedule in the right
// sidebar, replacing the backlinks panel when the "calendar" layout is active
// or when the user toggles it with a keybinding.
type CalendarPanel struct {
	width  int
	height int

	// Data
	now            time.Time
	plannerBlocks  []PlannerBlock      // today's planner blocks
	upcomingTasks  []calendarPanelTask // tasks due today/tomorrow
	todayEvents    []CalendarEvent     // ICS calendar events for today
	daysWithEvents map[int]bool        // day-of-month -> has events
	vaultRoot      string
}

// calendarPanelTask is a lightweight task entry for the upcoming list.
type calendarPanelTask struct {
	Text     string
	DueDate  string // YYYY-MM-DD
	Priority int    // 0-4
	Project  string // project/folder name
	Done     bool
}

// NewCalendarPanel creates a new CalendarPanel.
func NewCalendarPanel() CalendarPanel {
	return CalendarPanel{
		now:            time.Now(),
		daysWithEvents: make(map[int]bool),
	}
}

// SetSize updates the panel dimensions.
func (cp *CalendarPanel) SetSize(width, height int) {
	cp.width = width
	cp.height = height
}

// SetVaultRoot stores the vault root path for loading data.
func (cp *CalendarPanel) SetVaultRoot(root string) {
	cp.vaultRoot = root
}

// SetEvents provides calendar events (from ICS files and native store).
func (cp *CalendarPanel) SetEvents(events []CalendarEvent) {
	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := todayStart.Add(24 * time.Hour)
	year, month, _ := now.Date()

	cp.todayEvents = nil
	for _, ev := range events {
		// Mark days with events in the mini calendar
		if ev.Date.Year() == year && ev.Date.Month() == month {
			cp.daysWithEvents[ev.Date.Day()] = true
		}

		// Collect today's events
		if !ev.Date.Before(todayStart) && ev.Date.Before(tomorrowStart) {
			cp.todayEvents = append(cp.todayEvents, ev)
		} else if ev.AllDay && ev.Date.Year() == year && ev.Date.Month() == month && ev.Date.Day() == now.Day() {
			cp.todayEvents = append(cp.todayEvents, ev)
		}
	}

	// Sort today's events by time
	sort.Slice(cp.todayEvents, func(i, j int) bool {
		return cp.todayEvents[i].Date.Before(cp.todayEvents[j].Date)
	})
}

// Refresh reloads planner blocks and tasks from the vault.
func (cp *CalendarPanel) Refresh(allPlannerBlocks map[string][]PlannerBlock, noteContents map[string]string) {
	cp.now = time.Now()
	todayStr := cp.now.Format("2006-01-02")
	tomorrowStr := cp.now.Add(24 * time.Hour).Format("2006-01-02")

	// Today's planner blocks
	cp.plannerBlocks = allPlannerBlocks[todayStr]

	// Sort blocks by start time
	sort.Slice(cp.plannerBlocks, func(i, j int) bool {
		return cp.plannerBlocks[i].StartTime < cp.plannerBlocks[j].StartTime
	})

	// Mark days that have events this month
	cp.daysWithEvents = make(map[int]bool)
	year, month, _ := cp.now.Date()
	monthPrefix := fmt.Sprintf("%04d-%02d-", year, int(month))
	for dateStr, blocks := range allPlannerBlocks {
		if strings.HasPrefix(dateStr, monthPrefix) && len(blocks) > 0 {
			if t, err := time.Parse("2006-01-02", dateStr); err == nil {
				cp.daysWithEvents[t.Day()] = true
			}
		}
	}

	// Scan notes for tasks due today or tomorrow
	cp.upcomingTasks = nil
	for notePath, content := range noteContents {
		project := ""
		dir := filepath.Dir(notePath)
		if dir != "." && dir != "" {
			project = filepath.Base(dir)
		}
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, "- [ ]") && !strings.HasPrefix(trimmed, "- [x]") {
				continue
			}
			done := strings.HasPrefix(trimmed, "- [x]")
			// Look for 📅 YYYY-MM-DD date marker
			idx := strings.Index(trimmed, "\U0001f4c5 ")
			if idx < 0 {
				continue
			}
			dateStr := trimmed[idx+len("\U0001f4c5 "):]
			if len(dateStr) < 10 {
				continue
			}
			dueDate := dateStr[:10]
			if dueDate != todayStr && dueDate != tomorrowStr {
				continue
			}

			taskText := strings.TrimSpace(trimmed[5:])
			// Strip the date suffix
			if eIdx := strings.Index(taskText, " \U0001f4c5"); eIdx >= 0 {
				taskText = taskText[:eIdx]
			}

			priority := 0
			if strings.Contains(trimmed, "🔴") || strings.Contains(trimmed, "⏫") {
				priority = 4
			} else if strings.Contains(trimmed, "🟠") || strings.Contains(trimmed, "🔼") {
				priority = 3
			} else if strings.Contains(trimmed, "🟡") {
				priority = 2
			} else if strings.Contains(trimmed, "🔽") {
				priority = 1
			}

			cp.upcomingTasks = append(cp.upcomingTasks, calendarPanelTask{
				Text:     taskText,
				DueDate:  dueDate,
				Priority: priority,
				Project:  project,
				Done:     done,
			})
		}
	}

	// Sort by priority desc, then date asc
	sort.Slice(cp.upcomingTasks, func(i, j int) bool {
		if cp.upcomingTasks[i].Priority != cp.upcomingTasks[j].Priority {
			return cp.upcomingTasks[i].Priority > cp.upcomingTasks[j].Priority
		}
		return cp.upcomingTasks[i].DueDate < cp.upcomingTasks[j].DueDate
	})

	// Limit to 5 tasks
	if len(cp.upcomingTasks) > 5 {
		cp.upcomingTasks = cp.upcomingTasks[:5]
	}

	// Also check for daily note existence for days with events
	if cp.vaultRoot != "" {
		dailyDir := filepath.Join(cp.vaultRoot, "Planner")
		entries, err := os.ReadDir(dailyDir)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
					continue
				}
				name := strings.TrimSuffix(e.Name(), ".md")
				if strings.HasPrefix(name, monthPrefix) {
					if t, err := time.Parse("2006-01-02", name); err == nil {
						cp.daysWithEvents[t.Day()] = true
					}
				}
			}
		}
	}
}

// View renders the calendar panel.
func (cp CalendarPanel) View() string {
	contentWidth := cp.width - 2
	if contentWidth < 10 {
		contentWidth = 10
	}

	var b strings.Builder
	sepStyle := lipgloss.NewStyle().Foreground(surface0)
	sep := sepStyle.Render(strings.Repeat("─", contentWidth))
	sectionTitle := func(icon, title string, color lipgloss.Color) {
		b.WriteString(sep + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(color).Bold(true).Render("  " + icon + " " + title))
		b.WriteString("\n")
	}

	// Mini month calendar
	b.WriteString(cp.renderMiniCalendar(contentWidth))

	// Today's events (ICS)
	sectionTitle("◆", "Events", peach)
	if len(cp.todayEvents) == 0 {
		b.WriteString("  " + DimStyle.Render("No events today") + "\n")
	} else {
		evTimeStyle := lipgloss.NewStyle().Foreground(overlay0)
		evNameStyle := lipgloss.NewStyle().Foreground(text)
		for _, ev := range cp.todayEvents {
			timeStr := ev.Date.Format("15:04")
			if ev.AllDay {
				timeStr = "     "
			}
			title := TruncateDisplay(ev.Title, contentWidth-12)
			b.WriteString("  " + evTimeStyle.Render(timeStr) + " " + evNameStyle.Render(title) + "\n")
		}
	}

	// Today's schedule (planner blocks)
	if len(cp.plannerBlocks) > 0 {
		sectionTitle("▸", "Schedule", blue)
		for _, block := range cp.plannerBlocks {
			b.WriteString(cp.renderScheduleBlock(block, contentWidth) + "\n")
		}
	}

	// Upcoming tasks
	sectionTitle("○", "Tasks", green)
	if len(cp.upcomingTasks) == 0 {
		b.WriteString("  " + DimStyle.Render("No tasks due") + "\n")
	} else {
		todayStr := cp.now.Format("2006-01-02")
		for _, task := range cp.upcomingTasks {
			b.WriteString(cp.renderTask(task, todayStr, contentWidth) + "\n")
		}
	}

	return b.String()
}

// renderMiniCalendar renders a compact month grid.
func (cp CalendarPanel) renderMiniCalendar(width int) string {
	var b strings.Builder

	year, month, today := cp.now.Date()

	// Month + year header with today's date
	monthHeader := fmt.Sprintf("%s %d", month.String()[:3], year)
	todayLabel := fmt.Sprintf("%s %d", cp.now.Weekday().String()[:3], today)
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	todayStyle := lipgloss.NewStyle().Foreground(overlay0)
	gap := width - len(monthHeader) - len(todayLabel) - 4
	if gap < 1 {
		gap = 1
	}
	b.WriteString("  " + headerStyle.Render(monthHeader) + strings.Repeat(" ", gap) + todayStyle.Render(todayLabel))
	b.WriteString("\n")

	// Day-of-week header
	dayHeaders := "Mo Tu We Th Fr Sa Su"
	dayHeaderStyle := lipgloss.NewStyle().Foreground(surface2)
	b.WriteString("  " + dayHeaderStyle.Render(dayHeaders))
	b.WriteString("\n")

	// First day of month
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, cp.now.Location())
	// Monday = 0, Sunday = 6
	weekday := int(firstDay.Weekday())
	if weekday == 0 {
		weekday = 6 // Sunday
	} else {
		weekday-- // Monday=0
	}

	daysInMonth := time.Date(year, month+1, 0, 0, 0, 0, 0, cp.now.Location()).Day()

	todayStyle = lipgloss.NewStyle().Foreground(base).Background(mauve).Bold(true)
	normalStyle := lipgloss.NewStyle().Foreground(text)
	eventDotStyle := lipgloss.NewStyle().Foreground(blue)

	var line strings.Builder
	line.WriteString("  ")
	for i := 0; i < weekday; i++ {
		line.WriteString("   ")
	}

	for day := 1; day <= daysInMonth; day++ {
		dayStr := fmt.Sprintf("%2d", day)

		hasEvent := cp.daysWithEvents[day]

		if day == today {
			line.WriteString(todayStyle.Render(dayStr))
		} else if hasEvent {
			line.WriteString(eventDotStyle.Render(dayStr))
		} else {
			line.WriteString(normalStyle.Render(dayStr))
		}

		// Add dot indicator or space after the day number
		if hasEvent && day != today {
			line.WriteString(eventDotStyle.Render("\u00b7"))
		} else {
			line.WriteString(" ")
		}

		weekday++
		if weekday == 7 {
			b.WriteString(line.String())
			b.WriteString("\n")
			line.Reset()
			line.WriteString("  ")
			weekday = 0
		}
	}
	// Flush remaining days
	if weekday > 0 {
		b.WriteString(line.String())
		b.WriteString("\n")
	}

	return b.String()
}

// renderScheduleBlock renders a single time block entry.
func (cp CalendarPanel) renderScheduleBlock(block PlannerBlock, width int) string {
	timeStr := block.StartTime

	// Block indicator color by type
	var blockColor lipgloss.Color
	switch block.BlockType {
	case "task":
		blockColor = blue
	case "event":
		blockColor = mauve
	case "break":
		blockColor = green
	case "focus":
		blockColor = peach
	default:
		blockColor = text
	}

	indicator := lipgloss.NewStyle().Foreground(blockColor).Render("\u2588")
	timeStyle := lipgloss.NewStyle().Foreground(overlay0)

	taskText := block.Text
	maxTextLen := width - 10 // time (5) + space + indicator + space + padding
	if maxTextLen < 5 {
		maxTextLen = 5
	}
	if len(taskText) > maxTextLen {
		taskText = taskText[:maxTextLen-3] + "..."
	}

	textStyle := lipgloss.NewStyle().Foreground(text)
	if block.Done {
		textStyle = lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true)
	}

	return "  " + timeStyle.Render(timeStr) + " " + indicator + " " + textStyle.Render(taskText)
}

// renderTask renders a single upcoming task entry.
func (cp CalendarPanel) renderTask(task calendarPanelTask, todayStr string, width int) string {
	// Priority indicator
	var priIndicator string
	switch task.Priority {
	case 4:
		priIndicator = lipgloss.NewStyle().Foreground(red).Render("\u25b2")
	case 3:
		priIndicator = lipgloss.NewStyle().Foreground(peach).Render("\u25b2")
	case 2:
		priIndicator = lipgloss.NewStyle().Foreground(yellow).Render("\u25cf")
	case 1:
		priIndicator = lipgloss.NewStyle().Foreground(blue).Render("\u25bc")
	default:
		priIndicator = lipgloss.NewStyle().Foreground(overlay0).Render("\u25cb")
	}

	// Due label
	dueLabel := ""
	if task.DueDate == todayStr {
		dueLabel = lipgloss.NewStyle().Foreground(yellow).Render("today")
	} else {
		dueLabel = lipgloss.NewStyle().Foreground(teal).Render("tmrw")
	}

	taskText := task.Text
	maxLen := width - 16 // priority + space + text + space + due
	if task.Project != "" {
		maxLen -= len(task.Project) + 3
	}
	if maxLen < 5 {
		maxLen = 5
	}
	if len(taskText) > maxLen {
		taskText = taskText[:maxLen-3] + "..."
	}

	textStyle := lipgloss.NewStyle().Foreground(text)
	if task.Done {
		textStyle = lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true)
	}

	result := "  " + priIndicator + " " + textStyle.Render(taskText)
	if task.Project != "" {
		result += " " + DimStyle.Render("["+task.Project+"]")
	}
	result += " " + dueLabel

	return result
}
