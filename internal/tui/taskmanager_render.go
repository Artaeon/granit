package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// TaskManager rendering — all View/render methods extracted from taskmanager.go
// ---------------------------------------------------------------------------

func (tm *TaskManager) visibleHeight() int {
	// Reserve: border(2) + padding(2) + title(2) + tabs(2) + input/status(2) + help bar(2) + gaps(4)
	// Plus 1 line for cursor detail strip, plus 1 line for workload banner when visible.
	h := tm.height - 19
	if tm.view == taskViewToday || tm.view == taskViewUpcoming || tm.view == taskViewAll {
		h--
	}
	if h < 3 {
		h = 3
	}
	return h
}

func (tm *TaskManager) kanbanVisibleHeight() int {
	// Reserve: header(1) + rule-hint(1) + gap(1) = 3 lines per column before cards.
	h := tm.height - 17
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
	// Use wider layout — match calendar's full-width feel
	width := tm.width - 2
	if width < 80 {
		width = 80
	}
	if width > 160 {
		width = 160
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
	default:
		tm.renderTaskList(&b, innerW)
		// Mini timeline for Plan view — shows today's scheduled tasks at a glance
		if tm.view == taskViewToday {
			tm.renderMiniTimeline(&b, innerW)
		}
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

func (tm *TaskManager) renderHelpOverlay(b *strings.Builder, w int) {
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true).Width(12)
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
			{"Tab", "Cycle views (Plan / Upcoming / All / Done / Calendar / Kanban)"},
			{"1-6", "Jump directly to a view"},
			{"g", "Jump to the source note of the cursor task"},
			{"Esc", "Close task manager"},
		}},
		{"Create & edit", [][2]string{
			{"a", "Add a new task"},
			{"x / Enter", "Toggle done / not done"},
			{".", "Quick-edit: p:high d:tomorrow ~30m in one input"},
			{"d", "Set / change due date"},
			{"r", "Reschedule (tomorrow / Monday / +1wk / +1mo / custom)"},
			{"p", "Cycle priority (none → low → med → high → highest)"},
			{"E", "Set time estimate (15m / 30m / 45m / 1h / 1.5h / 2h)"},
			{"e", "Expand / collapse subtasks"},
			{"b", "Add task dependency"},
			{"u", "Undo last edit (10-deep stack)"},
		}},
		{"Filter & search", [][2]string{
			{"F", "Unified filter prompt: #tag p:high sort:due free text"},
			{"/", "Plain search (fuzzy — \"bygr\" matches \"buy groceries\")"},
			{"#", "Cycle single-tag filter"},
			{"P", "Cycle single-priority filter"},
			{"s", "Cycle sort mode (priority / date / A-Z / source / tag)"},
			{"c", "Clear all active filters"},
		}},
		{"Planning & focus", [][2]string{
			{"n", "Add / edit task note"},
			{"z", "Snooze task (1h / 4h / tomorrow 9am)"},
			{"W", "Pin / unpin (pinned sort to top)"},
			{"A", "Auto-suggest priority (heuristic)"},
			{"R", "Batch reschedule all overdue (Plan view)"},
			{"f", "Start focus session on cursor task"},
		}},
		{"Bulk", [][2]string{
			{"v", "Enter / exit select mode"},
			{"Space", "Toggle selection (in select mode)"},
			{"T", "Save cursor task as template"},
			{"t", "Create task from template"},
			{"X", "Archive completed tasks >30 days (with confirm)"},
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

	// Progress bar: ████░░░░ 42%
	pct := 0
	if total > 0 {
		pct = done * 100 / total
	}
	barW := 8
	filled := barW * pct / 100
	bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", barW-filled))
	stats := "  " + bar + lipgloss.NewStyle().Foreground(overlay0).
		Render(fmt.Sprintf(" %d/%d", done, total))

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
	if tm.searchTerm != "" {
		preview := tm.searchTerm
		if len(preview) > 20 {
			preview = preview[:17] + "…"
		}
		filters += " " + filterStyle.Render("\"" + preview + "\"")
	}
	if tm.selectMode && len(tm.selected) > 0 {
		filters += " " + lipgloss.NewStyle().Foreground(crust).Background(mauve).Padding(0, 1).
			Render(fmt.Sprintf("%d selected", len(tm.selected)))
	}

	now := time.Now()
	clockLabel := lipgloss.NewStyle().Foreground(green).Bold(true).Render(now.Format("15:04"))
	headerLeft := "  " + icon + title + stats + filters
	headerGap := w - lipgloss.Width(headerLeft) - lipgloss.Width(clockLabel) - 2
	if headerGap < 2 {
		// At narrow widths, drop filters to make room for the clock
		headerLeft = "  " + icon + title + stats
		headerGap = w - lipgloss.Width(headerLeft) - lipgloss.Width(clockLabel) - 2
		if headerGap < 2 {
			headerGap = 2
		}
	}
	b.WriteString(headerLeft + strings.Repeat(" ", headerGap) + clockLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", w-4)))
	b.WriteString("\n")

	// Workload banner — section counts + total estimated time, visible under the title
	// on the views where it matters (Plan, Upcoming, All).
	if tm.view == taskViewToday || tm.view == taskViewUpcoming || tm.view == taskViewAll {
		tm.renderWorkloadBanner(b)
	}
}

// renderWorkloadBanner emits a single-line "at a glance" strip showing section
// counts (overdue/today/tomorrow) and total estimated minutes for the current view.
// Each pill carries its own workload estimate so you can see, e.g.,
// "3 overdue ~45m · 5 today ~2h · 2 tomorrow ~30m" and budget your day.
// Returns early if nothing is worth showing.
func (tm *TaskManager) renderWorkloadBanner(b *strings.Builder) {
	var overdue, today, tomorrow, other sectionStats
	for _, t := range tm.filtered {
		if t.Done {
			continue
		}
		switch {
		case tmIsOverdue(t.DueDate):
			overdue.add(t)
		case tmIsToday(t.DueDate):
			today.add(t)
		case t.DueDate != "" && tmDaysUntil(t.DueDate) == 1:
			tomorrow.add(t)
		default:
			other.add(t)
		}
	}
	if overdue.count == 0 && today.count == 0 && tomorrow.count == 0 && other.count == 0 {
		return
	}

	pill := func(bg lipgloss.Color, label string) string {
		return makePill(bg, label)
	}

	var parts []string
	if overdue.count > 0 {
		parts = append(parts, pill(red, overdue.label("overdue")))
	}
	if today.count > 0 {
		parts = append(parts, pill(green, today.label("today")))
	}
	if tomorrow.count > 0 {
		parts = append(parts, pill(lavender, tomorrow.label("tomorrow")))
	}
	// "other" (no due date or >1 day out) only shown on All / Upcoming views
	// where it's meaningful — on Plan it'd just duplicate the section breakdown.
	if tm.view != taskViewToday && other.count > 0 {
		parts = append(parts, pill(overlay1, other.label("other")))
	}

	sep := DimStyle.Render(" · ")
	b.WriteString("  " + strings.Join(parts, sep))
	b.WriteString("\n")
}

// sectionStats accumulates task count and total estimated minutes for one
// bucket (overdue/today/tomorrow/other) in the workload banner.
type sectionStats struct {
	count     int
	totalMins int
}

func (s *sectionStats) add(t Task) {
	s.count++
	s.totalMins += t.EstimatedMinutes
}

// label formats the section as "5 today ~2h" — workload suffix is omitted
// when no tasks in the bucket have estimates.
func (s sectionStats) label(name string) string {
	out := fmt.Sprintf("%d %s", s.count, name)
	if s.totalMins > 0 {
		h, m := s.totalMins/60, s.totalMins%60
		switch {
		case h > 0 && m > 0:
			out += fmt.Sprintf(" ~%dh%dm", h, m)
		case h > 0:
			out += fmt.Sprintf(" ~%dh", h)
		default:
			out += fmt.Sprintf(" ~%dm", m)
		}
	}
	return out
}

func (tm *TaskManager) renderTabs(b *strings.Builder, w int) {
	activeStyle := lipgloss.NewStyle().
		Foreground(crust).
		Background(mauve).
		Bold(true).
		Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().
		Foreground(subtext0).
		Background(surface0).
		Padding(0, 1)

	names := []string{
		"Plan",
		"Upcoming",
		"All",
		"Done",
		"Calendar",
		"Kanban",
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
		icon := "📋"
		switch tm.view {
		case taskViewToday:
			emptyMsg = "Nothing on the plan"
			hint = "No overdue, today, or tomorrow tasks — press 'a' to add one"
			icon = "✨"
		case taskViewUpcoming:
			emptyMsg = "No upcoming tasks this week"
			hint = "Tasks with due dates in the next 14 days appear here"
			icon = "📅"
		case taskViewCompleted:
			emptyMsg = "No completed tasks"
			hint = "Complete a task with 'x' to see it here"
			icon = "✅"
		case taskViewAll:
			emptyMsg = "No tasks in vault"
			icon = "📝"
		}
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(overlay0).Render(icon+" "+emptyMsg))
		b.WriteString("\n")
		b.WriteString("  " + DimStyle.Render(hint))
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

		// Today view: group headers by time block / overdue / today / tomorrow.
		if tm.view == taskViewToday {
			group := timeBlockGroup(task)
			if group != lastGroup {
				if lastGroup != "" {
					b.WriteString("\n")
				}
				// Count tasks and estimate in this group
				groupCount := 0
				groupEst := 0
				for _, ft := range tm.filtered {
					if ft.Done {
						continue
					}
					if timeBlockGroup(ft) == group {
						groupCount++
						groupEst += ft.EstimatedMinutes
					}
				}
				estStr := ""
				if groupEst > 0 {
					estStr = " " + FormatMinutes(groupEst)
				}
				countStr := fmt.Sprintf("  %d tasks%s", groupCount, estStr)

				// Colored separator line with label (matching calendar time blocks)
				var sectionColor lipgloss.Color
				var sectionLabel string
				switch group {
				case "overdue":
					sectionColor = red
					sectionLabel = "OVERDUE"
				case "morning":
					sectionColor = lavender
					sectionLabel = "MORNING (06–10)"
				case "midday":
					sectionColor = sapphire
					sectionLabel = "MIDDAY (10–14)"
				case "afternoon":
					sectionColor = peach
					sectionLabel = "AFTERNOON (14–18)"
				case "evening":
					sectionColor = teal
					sectionLabel = "EVENING (18–22)"
				case "today":
					sectionColor = green
					sectionLabel = "TODAY (unscheduled)"
				case "tomorrow":
					sectionColor = lavender
					sectionLabel = "TOMORROW"
				}
				label := lipgloss.NewStyle().Foreground(sectionColor).Bold(true).Render(" " + sectionLabel + " ")
				count := DimStyle.Render(countStr + " ")
				sepLen := w - lipgloss.Width(label) - lipgloss.Width(count) - 4
				if sepLen < 2 {
					sepLen = 2
				}
				sep := lipgloss.NewStyle().Foreground(sectionColor).Render(strings.Repeat("─", sepLen))
				b.WriteString("  " + label + count + sep + "\n")
				lastGroup = group
			}
		}

		// All view: group by Note Path
		if tm.view == taskViewAll || tm.view == taskViewCompleted {
			group := task.NotePath
			if group != lastGroup {
				if lastGroup != "" {
					b.WriteString("\n")
				}
				fileName := filepath.Base(group)
				label := lipgloss.NewStyle().Foreground(sapphire).Bold(true).
					Render(" " + fileName + " ")
				sepLen := w - lipgloss.Width(label) - 4
				if sepLen < 2 {
					sepLen = 2
				}
				sep := lipgloss.NewStyle().Foreground(sapphire).Render(strings.Repeat("─", sepLen))
				b.WriteString("  " + label + sep + "\n")
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
				daysAway := tmDaysUntil(group)
				// Every switch arm assigns sectionColor; no fall-through
				// default needed beyond the explicit overlay1 below.
				var sectionColor lipgloss.Color
				suffix := ""
				switch {
				case tmIsOverdue(group):
					sectionColor = red
					suffix = " (overdue)"
				case tmIsToday(group):
					sectionColor = green
					suffix = " (today)"
				case daysAway == 1:
					sectionColor = lavender
					suffix = " (tomorrow)"
				case daysAway <= 3:
					sectionColor = sapphire
					suffix = fmt.Sprintf(" (in %d days)", daysAway)
				case daysAway <= 7:
					sectionColor = blue
					suffix = fmt.Sprintf(" (in %d days)", daysAway)
				default:
					sectionColor = overlay1
				}
				label := lipgloss.NewStyle().Foreground(sectionColor).Bold(true).
					Render(" " + dayLabel + suffix + " ")
				sepLen := w - lipgloss.Width(label) - 4
				if sepLen < 2 {
					sepLen = 2
				}
				sep := lipgloss.NewStyle().Foreground(sectionColor).Render(strings.Repeat("─", sepLen))
				b.WriteString("  " + label + sep + "\n")
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

	// Snoozed-tasks indicator — snoozed tasks are filtered out of every view,
	// so surface a reminder that they exist to prevent forgotten work.
	if snoozed := tm.snoozedCount(); snoozed > 0 {
		word := "tasks"
		if snoozed == 1 {
			word = "task"
		}
		b.WriteString(DimStyle.Render(fmt.Sprintf("  ↪ %d snoozed %s hidden (press 'z' on a task to snooze/wake)", snoozed, word)))
		b.WriteString("\n")
	}
}

// snoozedCount returns the number of tasks currently snoozed (excluded from
// all views). Excludes done tasks.
func (tm *TaskManager) snoozedCount() int {
	n := 0
	for _, t := range tm.allTasks {
		if !t.Done && tmIsSnoozed(t) {
			n++
		}
	}
	return n
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
		switch {
		case tmIsOverdue(task.DueDate):
			dueBadge = lipgloss.NewStyle().Foreground(crust).Background(red).Padding(0, 1).Render(dueLabel)
		case tmIsToday(task.DueDate):
			dueBadge = lipgloss.NewStyle().Foreground(crust).Background(yellow).Padding(0, 1).Render(dueLabel)
		default:
			dueBadge = lipgloss.NewStyle().Foreground(crust).Background(surface1).Padding(0, 1).Render(dueLabel)
		}
	}

	// Estimate badge — inline on row for quick scanning
	var estBadge string
	if task.EstimatedMinutes > 0 && !task.Done {
		estBadge = lipgloss.NewStyle().Foreground(sky).Render(" ~" + tmFormatMinutes(task.EstimatedMinutes))
	}

	// Selection indicator (in select mode)
	selectStr := ""
	prefixExtra := 0
	if tm.selectMode {
		if tm.selected[taskKey(task)] {
			selectStr = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("[*] ")
		} else {
			selectStr = DimStyle.Render("[ ] ")
		}
		prefixExtra += 4
	}

	// Blocked indicator — structural, kept on row.
	blockedStr := ""
	if tm.blockedCache[taskKey(task)] {
		blockedStr = lipgloss.NewStyle().Foreground(red).Render("🔒 ")
		textStyle = lipgloss.NewStyle().Foreground(overlay0)
		prefixExtra += 3
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

	// Hint dots: tiny indicators that *something exists* without taking horizontal room.
	// These tell you at a glance the task has metadata worth peeking at.
	hintDots := ""
	hintWidth := 0
	if task.EstimatedMinutes > 0 || task.ScheduledTime != "" {
		hintDots += lipgloss.NewStyle().Foreground(sky).Render("·")
		hintWidth++
	}
	if len(task.Tags) > 0 {
		hintDots += lipgloss.NewStyle().Foreground(sapphire).Render("·")
		hintWidth++
	}
	if tm.pinnedTasks[taskKey(task)] || tm.taskNotes[taskKey(task)] != "" || task.GoalID != "" {
		hintDots += lipgloss.NewStyle().Foreground(yellow).Render("·")
		hintWidth++
	}
	if hintDots != "" {
		hintDots = " " + hintDots
		hintWidth++
	}

	// Truncate text — budget for right-side badges.
	dueWidth := 0
	if dueBadge != "" {
		dueWidth = lipgloss.Width(dueBadge) + 1
	}
	estWidth := 0
	if estBadge != "" {
		estWidth = lipgloss.Width(estBadge)
	}
	maxTextW := w - 12 - prefixExtra - dueWidth - estWidth - hintWidth
	if maxTextW < 10 {
		maxTextW = 10
	}
	displayText = TruncateDisplay(displayText, maxTextW)

	// Build the line with colored priority bar on the left edge
	prioBarColor := tmPriorityColor(task.Priority)
	if task.Done {
		prioBarColor = surface1
	}
	prioBar := lipgloss.NewStyle().Foreground(prioBarColor).Render("▎")

	prefix := " "
	if isSelected {
		prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▸")
	}

	// Small scheduled time indicator before task text
	schedBadge := ""
	if task.ScheduledTime != "" && !task.Done {
		parts := strings.SplitN(task.ScheduledTime, "-", 2)
		if len(parts) >= 1 {
			schedBadge = lipgloss.NewStyle().Foreground(teal).Render(parts[0] + " ")
		}
	}

	leftSide := strings.Join([]string{
		"  " + prioBar + prefix + selectStr + indentStr + collapseIndicator + blockedStr + checkbox,
		prioStyled,
		schedBadge + textStyle.Render(displayText) + estBadge + hintDots,
	}, " ")

	rightSide := dueBadge

	leftW := lipgloss.Width(leftSide)
	rightW := lipgloss.Width(rightSide)
	spacing := w - leftW - rightW - 2
	if spacing < 1 {
		spacing = 1
	}

	line := leftSide
	if rightSide != "" {
		line += strings.Repeat(" ", spacing) + rightSide
	}

	if isSelected {
		b.WriteString(lipgloss.NewStyle().
			Background(surface0).
			Width(w).
			Render(line))
	} else {
		b.WriteString(line)
	}

	// Detail strip — only for the cursor row, only if there's something to show.
	if isSelected {
		detail := tm.renderTaskDetailLine(task)
		if detail != "" {
			b.WriteString("\n")
			detailStyle := lipgloss.NewStyle().Background(surface0).Padding(0, 1)
			b.WriteString("   " + detailStyle.Render(detail))
		}
	}
}

// renderTaskDetailLine builds the dim metadata strip shown under the cursor row.
// Returns "" if the task has no extra metadata worth surfacing.
func (tm *TaskManager) renderTaskDetailLine(task Task) string {
	var parts []string

	if tm.pinnedTasks[taskKey(task)] {
		parts = append(parts, lipgloss.NewStyle().Foreground(yellow).Render("\U0001F4CC pinned"))
	}
	if tm.taskNotes[taskKey(task)] != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(yellow).Render("\U0001F4DD note"))
	}
	if task.GoalID != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(sapphire).Render("\u2691 "+task.GoalID))
	}
	if task.ScheduledTime != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(teal).Render("⏰ "+task.ScheduledTime))
	}
	if task.EstimatedMinutes > 0 {
		label := tmFormatMinutes(task.EstimatedMinutes)
		parts = append(parts, lipgloss.NewStyle().Foreground(sky).Render("~"+label))
	}
	if task.ActualMinutes > 0 {
		label := tmFormatMinutes(task.ActualMinutes)
		col := green
		if task.EstimatedMinutes > 0 && task.ActualMinutes > task.EstimatedMinutes {
			col = red
		}
		parts = append(parts, lipgloss.NewStyle().Foreground(col).Render("◷ "+label))
	}
	if len(task.Tags) > 0 {
		tagColors := []lipgloss.Color{sapphire, teal, lavender, pink, peach, sky}
		var tagBits []string
		for i, tag := range task.Tags {
			c := tagColors[i%len(tagColors)]
			tagBits = append(tagBits, lipgloss.NewStyle().Foreground(c).Render("#"+tag))
		}
		parts = append(parts, strings.Join(tagBits, " "))
	}
	if task.NotePath != "" {
		noteName := strings.TrimSuffix(filepath.Base(task.NotePath), ".md")
		parts = append(parts, DimStyle.Render("📁 "+noteName))
	}

	if len(parts) == 0 {
		return ""
	}
	sep := DimStyle.Render(" · ")
	return strings.Join(parts, sep)
}

// renderMiniTimeline draws a compact timeline strip for the Plan view, showing
// today's scheduled tasks, events, and breaks as colored blocks across a
// 08:00–22:00 span. This gives a visual "day at a glance" below the task list.
func (tm *TaskManager) renderMiniTimeline(b *strings.Builder, w int) {
	// Collect tasks that have a scheduled time for today
	type timeBlock struct {
		startMin int
		endMin   int
		label    string
		color    lipgloss.Color
	}
	var blocks []timeBlock
	today := time.Now().Format("2006-01-02")

	for _, t := range tm.filtered {
		if t.Done || t.ScheduledTime == "" {
			continue
		}
		if t.DueDate != "" && t.DueDate != today {
			continue
		}
		// Parse "HH:MM-HH:MM"
		parts := strings.SplitN(t.ScheduledTime, "-", 2)
		if len(parts) != 2 {
			continue
		}
		var sH, sM, eH, eM int
		_, _ = fmt.Sscanf(parts[0], "%d:%d", &sH, &sM)
		_, _ = fmt.Sscanf(parts[1], "%d:%d", &eH, &eM)
		startMin := sH*60 + sM
		endMin := eH*60 + eM
		if endMin <= startMin {
			continue
		}
		// Color by time block (matching calendar)
		var col lipgloss.Color
		startHour := sH
		switch {
		case startHour < 10:
			col = lipgloss.Color("#7B6BA6") // morning purple
		case startHour < 15:
			col = lipgloss.Color("#5B7EA8") // midday blue
		case startHour < 20:
			col = lipgloss.Color("#B8875A") // afternoon amber
		default:
			col = lipgloss.Color("#5A9E9E") // evening teal
		}
		// High priority gets accent color override
		if t.Priority >= 3 {
			col = peach
		}
		// Clean display text
		cleanText := tmDueDateRe.ReplaceAllString(t.Text, "")
		cleanText = tmScheduleRe.ReplaceAllString(cleanText, "")
		cleanText = tmEstimateRe.ReplaceAllString(cleanText, "")
		cleanText = tmTagRe.ReplaceAllString(cleanText, "")
		cleanText = strings.TrimSpace(cleanText)
		blocks = append(blocks, timeBlock{startMin: startMin, endMin: endMin, label: cleanText, color: col})
	}

	if len(blocks) == 0 {
		return
	}

	b.WriteString("\n")
	// Timeline header with colored separator (matching calendar style)
	label := lipgloss.NewStyle().Foreground(teal).Bold(true).Render(" TODAY'S SCHEDULE ")
	sepLen := w - lipgloss.Width(label) - 6
	if sepLen < 2 {
		sepLen = 2
	}
	b.WriteString("  " + label + lipgloss.NewStyle().Foreground(teal).Render(strings.Repeat("─", sepLen)) + "\n")

	// Render compact timeline: each hour is a fixed width
	rangeStart := 8 * 60  // 08:00
	rangeEnd := 22 * 60   // 22:00
	totalMins := rangeEnd - rangeStart
	barW := w - 12
	if barW < 10 {
		barW = 10
	}
	if barW > w-4 {
		barW = w - 4
	}

	// Draw hour markers — position them by visual column, not byte length
	var markerBuf strings.Builder
	markerBuf.WriteString("  ")
	lastPos := 0
	for h := 8; h <= 22; h += 2 {
		pos := (h*60 - rangeStart) * barW / totalMins
		if pos < 0 {
			pos = 0
		}
		if pos >= barW-1 {
			break
		}
		gap := pos - lastPos
		if gap > 0 {
			markerBuf.WriteString(strings.Repeat(" ", gap))
		}
		markerBuf.WriteString(DimStyle.Render(fmt.Sprintf("%02d", h)))
		lastPos = pos + 2
	}
	b.WriteString(markerBuf.String() + "\n")

	// Draw the bar: fill with blocks
	bar := make([]rune, barW)
	barColors := make([]lipgloss.Color, barW)
	for i := range bar {
		bar[i] = '░'
		barColors[i] = surface0
	}

	// Now indicator
	now := time.Now()
	nowMin := now.Hour()*60 + now.Minute()

	// Fill blocks
	for _, blk := range blocks {
		startPos := (blk.startMin - rangeStart) * barW / totalMins
		endPos := (blk.endMin - rangeStart) * barW / totalMins
		if startPos < 0 {
			startPos = 0
		}
		if endPos > barW {
			endPos = barW
		}
		for p := startPos; p < endPos; p++ {
			bar[p] = '█'
			barColors[p] = blk.color
		}
	}

	// Render the bar with colors
	barStr := "  "
	lastColor := lipgloss.Color("")
	segment := ""
	for i := 0; i < barW; i++ {
		c := barColors[i]
		ch := string(bar[i])
		if c != lastColor {
			if segment != "" {
				barStr += lipgloss.NewStyle().Foreground(lastColor).Render(segment)
			}
			segment = ch
			lastColor = c
		} else {
			segment += ch
		}
	}
	if segment != "" {
		barStr += lipgloss.NewStyle().Foreground(lastColor).Render(segment)
	}
	b.WriteString(barStr + "\n")

	// Now marker
	if nowMin >= rangeStart && nowMin < rangeEnd {
		nowPos := (nowMin - rangeStart) * barW / totalMins + 2
		b.WriteString(strings.Repeat(" ", nowPos) +
			lipgloss.NewStyle().Foreground(red).Bold(true).Render("▲") +
			lipgloss.NewStyle().Foreground(red).Render(" now") + "\n")
	}

	// Legend: show scheduled blocks (cap at 6 with overflow)
	for i, blk := range blocks {
		if i >= 6 {
			b.WriteString("  " + DimStyle.Render(fmt.Sprintf("+%d more", len(blocks)-6)) + "\n")
			break
		}
		startH, startM := blk.startMin/60, blk.startMin%60
		endH, endM := blk.endMin/60, blk.endMin%60
		timeStr := fmt.Sprintf("%02d:%02d-%02d:%02d", startH, startM, endH, endM)
		pill := lipgloss.NewStyle().Foreground(blk.color).Render("█") + " " +
			DimStyle.Render(timeStr) + " " +
			lipgloss.NewStyle().Foreground(text).Render(TruncateDisplay(blk.label, 20))
		b.WriteString("  " + pill + "\n")
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
		b.WriteString("  " + promptStyle.Render("Search: ") + inputStyle.Render(tm.inputBuf+"\u2588"))
		b.WriteString("\n  " + DimStyle.Render("Enter to commit  ·  Esc to clear"))
	case tmInputFilter:
		b.WriteString("  " + promptStyle.Render("Filter: ") + inputStyle.Render(tm.inputBuf+"\u2588"))
		b.WriteString("\n  " + DimStyle.Render("Tokens: #tag · p:low|med|high|highest · sort:priority|due|az|source|tag · free text = search"))
	case tmInputQuickEdit:
		label := "Quick-edit:"
		if tm.cursor < len(tm.filtered) {
			t := tm.filtered[tm.cursor]
			preview := TruncateDisplay(tmCleanText(t.Text), 40)
			label = "Quick-edit “" + preview + "”:"
		}
		b.WriteString("  " + promptStyle.Render(label+" ") + inputStyle.Render(tm.inputBuf+"\u2588"))
		b.WriteString("\n  " + DimStyle.Render("Tokens: p:high · d:tomorrow · d:2026-05-01 · d:+3d · ~30m · ~1h30m"))
	case tmInputTimeBlock:
		b.WriteString("  " + promptStyle.Render("Schedule to time block:"))
		b.WriteString("\n")

		// Compute load per block for context
		type blkInfo struct {
			key      string
			label    string
			hours    string
			color    lipgloss.Color
			startMin int
			endMin   int
		}
		blkDefs := []blkInfo{
			{"1", "Morning", "06–10", lavender, 360, 600},
			{"2", "Midday", "10–14", sapphire, 600, 840},
			{"3", "Afternoon", "14–18", peach, 840, 1080},
			{"4", "Evening", "18–22", teal, 1080, 1320},
		}
		ss := lipgloss.NewStyle().Bold(true)
		ds := lipgloss.NewStyle().Foreground(overlay0)
		for _, bi := range blkDefs {
			count := 0
			totalEst := 0
			for _, t := range tm.allTasks {
				if t.Done || t.ScheduledTime == "" {
					continue
				}
				parts := strings.SplitN(t.ScheduledTime, "-", 2)
				if len(parts) < 1 {
					continue
				}
				sh, sm := parseHHMM(parts[0])
				sMins := sh*60 + sm
				if sMins >= bi.startMin && sMins < bi.endMin {
					count++
					est := t.EstimatedMinutes
					if est <= 0 {
						est = 60
					}
					totalEst += est
				}
			}
			load := ""
			if count > 0 {
				load = fmt.Sprintf(" %dt %s", count, FormatMinutes(totalEst))
			}
			b.WriteString("  " +
				ss.Foreground(bi.color).Render(bi.key) +
				ds.Render(":"+bi.label+" ("+bi.hours+")") +
				lipgloss.NewStyle().Foreground(bi.color).Render(load) + "\n")
		}
		b.WriteString("  " +
			ss.Foreground(overlay0).Render("0") + ds.Render(":Clear  ") +
			ds.Render("Esc:cancel"))
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
			{"j/k", "nav"}, {"x", "done"}, {"a", "add"}, {"d", "date"}, {"p", "prio"},
			{"B", "time block"}, {"r", "reschedule"}, {"E", "estimate"},
			{"/", "search"}, {"?", "help"}, {"Tab", "view"}, {"Esc", "close"},
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
	if colWidth < 20 {
		colWidth = 20
	}
	visH := tm.kanbanVisibleHeight()

	var colViews []string
	for c := 0; c < 4; c++ {
		var cb strings.Builder
		isActiveCol := c == tm.kanbanCol

		/* Header — active column gets thick colored bar + highlighted count */
		countStr := fmt.Sprintf(" %d ", len(cols[c]))
		countStyle := lipgloss.NewStyle().Foreground(overlay0).Background(surface0).Padding(0, 1).Bold(true)
		headerStyle := lipgloss.NewStyle().Foreground(colColors[c]).Bold(true)

		activeBar := "  "
		if isActiveCol {
			activeBar = lipgloss.NewStyle().Foreground(colColors[c]).Bold(true).Render("▎ ")
			countStyle = countStyle.Background(colColors[c]).Foreground(crust)
		}

		cb.WriteString(activeBar + headerStyle.Render(colNames[c]) + " " + countStyle.Render(countStr) + "\n")
		// Thin separator under header
		cb.WriteString("  " + lipgloss.NewStyle().Foreground(colColors[c]).Render(strings.Repeat("─", colWidth-6)) + "\n")

		/* Content */
		if len(cols[c]) == 0 {
			cb.WriteString("  " + DimStyle.Render("No tasks") + "\n")
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
