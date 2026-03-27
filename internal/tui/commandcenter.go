package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

// timeSlot represents a single scheduled entry in the day's schedule.
type timeSlot struct {
	Time     string // "09:00-09:30"
	Task     string
	Type     string // "task", "event", "break", "habit"
	Done     bool
	Priority int
}

// projectSummary is a compact project view for the command center.
type projectSummary struct {
	Name       string
	Status     string
	Progress   float64 // 0.0-1.0
	NextAction string  // most important next task
	TasksDone  int
	TasksTotal int
	Color      string
}

// habitStatus tracks a single habit's completion for today.
type habitStatus struct {
	Name   string
	Done   bool
	Streak int
}

// ---------------------------------------------------------------------------
// CommandCenter overlay
// ---------------------------------------------------------------------------

// CommandCenter is the "What do I do RIGHT NOW?" dashboard that pulls from
// all productivity systems: tasks, projects, habits, and calendar events.
type CommandCenter struct {
	active bool
	width  int
	height int

	// Data sections
	nowTask   *Task            // the single most important task RIGHT NOW
	nextTasks []Task           // next 3 tasks after nowTask
	schedule  []timeSlot       // today's time blocks
	projects  []projectSummary // active projects with progress
	habits    []habitStatus    // today's habits

	// UI state
	section int // 0=now, 1=schedule, 2=projects, 3=habits
	scroll  int // scroll within current section

	// Consumed-once outputs
	startPomodoro   bool
	completedTask   *Task
	selectedProject string
	toggledHabit    string
}

// NewCommandCenter creates a new inactive CommandCenter overlay.
func NewCommandCenter() CommandCenter {
	return CommandCenter{}
}

// IsActive reports whether the command center overlay is currently displayed.
func (cc CommandCenter) IsActive() bool {
	return cc.active
}

// Open activates the overlay.
func (cc *CommandCenter) Open() {
	cc.active = true
	cc.section = 0
	cc.scroll = 0
	cc.startPomodoro = false
	cc.completedTask = nil
	cc.selectedProject = ""
	cc.toggledHabit = ""
}

// Close deactivates the overlay.
func (cc *CommandCenter) Close() {
	cc.active = false
}

// SetSize updates the available terminal dimensions.
func (cc *CommandCenter) SetSize(w, h int) {
	cc.width = w
	cc.height = h
}

// ---------------------------------------------------------------------------
// Consumed-once getters
// ---------------------------------------------------------------------------

// ShouldStartPomodoro returns true once if the user requested a pomodoro.
func (cc *CommandCenter) ShouldStartPomodoro() bool {
	if cc.startPomodoro {
		cc.startPomodoro = false
		return true
	}
	return false
}

// CompletedTask returns the task marked done (consumed once).
func (cc *CommandCenter) CompletedTask() *Task {
	if cc.completedTask != nil {
		t := cc.completedTask
		cc.completedTask = nil
		return t
	}
	return nil
}

// SelectedProject returns the chosen project name (consumed once).
func (cc *CommandCenter) SelectedProject() string {
	if cc.selectedProject != "" {
		p := cc.selectedProject
		cc.selectedProject = ""
		return p
	}
	return ""
}

// ToggledHabit returns the habit name to toggle (consumed once).
func (cc *CommandCenter) ToggledHabit() string {
	if cc.toggledHabit != "" {
		h := cc.toggledHabit
		cc.toggledHabit = ""
		return h
	}
	return ""
}

// ---------------------------------------------------------------------------
// Data loading
// ---------------------------------------------------------------------------

// LoadData populates the command center with data from all productivity systems.
func (cc *CommandCenter) LoadData(tasks []Task, projects []Project, habits []habitEntry, habitLogs []habitLog, events []PlannerEvent) {
	today := time.Now().Format("2006-01-02")

	// ── Sort and select tasks ──
	// Filter to incomplete tasks only.
	var pending []Task
	for _, t := range tasks {
		if !t.Done {
			pending = append(pending, t)
		}
	}

	// Sort by priority score descending.
	sort.Slice(pending, func(i, j int) bool {
		return cc.priorityScore(pending[i]) > cc.priorityScore(pending[j])
	})

	if len(pending) > 0 {
		first := pending[0]
		cc.nowTask = &first
		end := len(pending)
		if end > 4 {
			end = 4
		}
		cc.nextTasks = make([]Task, 0, 3)
		for i := 1; i < end; i++ {
			cc.nextTasks = append(cc.nextTasks, pending[i])
		}
	} else {
		cc.nowTask = nil
		cc.nextTasks = nil
	}

	// ── Build schedule from tasks with ScheduledTime + events ──
	cc.schedule = nil

	// Add tasks that have a scheduled time.
	for _, t := range pending {
		if t.ScheduledTime != "" {
			cc.schedule = append(cc.schedule, timeSlot{
				Time:     t.ScheduledTime,
				Task:     t.Text,
				Type:     "task",
				Done:     t.Done,
				Priority: t.Priority,
			})
		}
	}

	// Add calendar events.
	for _, ev := range events {
		timeStr := ev.Time
		if timeStr == "" {
			timeStr = "all day"
		} else {
			dur := ev.Duration
			if dur == 0 {
				dur = 60
			}
			t, err := time.Parse("15:04", ev.Time)
			if err == nil {
				end := t.Add(time.Duration(dur) * time.Minute)
				timeStr = t.Format("15:04") + "-" + end.Format("15:04")
			}
		}
		cc.schedule = append(cc.schedule, timeSlot{
			Time: timeStr,
			Task: ev.Title,
			Type: "event",
		})
	}

	// Sort schedule by time string.
	sort.Slice(cc.schedule, func(i, j int) bool {
		return cc.schedule[i].Time < cc.schedule[j].Time
	})

	// ── Build project summaries ──
	cc.projects = nil
	for _, p := range projects {
		if p.Status != "active" {
			continue
		}
		// Count tasks for this project.
		var done, total int
		nextAction := ""
		for _, t := range tasks {
			// Match tasks by project folder or tag.
			matchFolder := p.Folder != "" && strings.HasPrefix(t.NotePath, p.Folder)
			matchTag := false
			if p.TaskFilter != "" {
				matchTag = strings.Contains(t.Text, "#"+p.TaskFilter) ||
					strings.Contains(strings.ToLower(t.Text), strings.ToLower(p.TaskFilter))
			} else if len(p.Tags) > 0 {
				matchTag = strings.Contains(t.Text, "#"+p.Tags[0]) ||
					strings.Contains(strings.ToLower(t.Text), strings.ToLower(p.Tags[0]))
			}
			if !matchFolder && !matchTag {
				continue
			}
			total++
			if t.Done {
				done++
			} else if nextAction == "" {
				nextAction = t.Text
				if r := []rune(nextAction); len(r) > 30 {
					nextAction = string(r[:27]) + "..."
				}
			}
		}
		progress := 0.0
		if total > 0 {
			progress = float64(done) / float64(total)
		}
		cc.projects = append(cc.projects, projectSummary{
			Name:       p.Name,
			Status:     p.Status,
			Progress:   progress,
			NextAction: nextAction,
			TasksDone:  done,
			TasksTotal: total,
			Color:      p.Color,
		})
	}

	// ── Build habits status from today's log ──
	cc.habits = nil
	for _, h := range habits {
		done := false
		for _, log := range habitLogs {
			if log.Date == today {
				for _, c := range log.Completed {
					if c == h.Name {
						done = true
						break
					}
				}
				break
			}
		}
		cc.habits = append(cc.habits, habitStatus{
			Name:   h.Name,
			Done:   done,
			Streak: h.Streak,
		})
	}
}

// priorityScore computes an urgency score for a task used for sorting.
func (cc *CommandCenter) priorityScore(task Task) int {
	score := task.Priority * 25 // highest=100, high=75, medium=50, low=25

	if task.DueDate != "" {
		today := time.Now().Format("2006-01-02")
		tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

		if task.DueDate < today {
			score += 50 // overdue
		} else if task.DueDate == today {
			score += 30 // due today
		} else if task.DueDate == tomorrow {
			score += 10 // due tomorrow
		}
	}

	if task.ScheduledTime != "" {
		// Check if scheduled time is now or soon (within the next hour).
		now := time.Now()
		parts := strings.SplitN(task.ScheduledTime, "-", 2)
		if len(parts) >= 1 {
			if t, err := time.Parse("15:04", parts[0]); err == nil {
				scheduled := time.Date(now.Year(), now.Month(), now.Day(),
					t.Hour(), t.Minute(), 0, 0, now.Location())
				diff := scheduled.Sub(now)
				if diff >= -30*time.Minute && diff <= 60*time.Minute {
					score += 20
				}
			}
		}
	}

	return score
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles keyboard input for the command center overlay.
func (cc CommandCenter) Update(msg tea.Msg) (CommandCenter, tea.Cmd) {
	if !cc.active {
		return cc, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "esc", "q":
			cc.active = false
			return cc, nil

		case "tab":
			cc.section = (cc.section + 1) % 4
			cc.scroll = 0

		case "shift+tab":
			cc.section--
			if cc.section < 0 {
				cc.section = 3
			}
			cc.scroll = 0

		case "up", "k":
			if cc.scroll > 0 {
				cc.scroll--
			}

		case "down", "j":
			cc.scroll++
			maxScroll := cc.maxScroll()
			if cc.scroll > maxScroll {
				cc.scroll = maxScroll
			}

		case " ":
			// Space on NOW task: start pomodoro
			if cc.section == 0 && cc.nowTask != nil {
				cc.startPomodoro = true
			}

		case "enter":
			switch cc.section {
			case 0:
				// Mark NOW task done, advance next task to NOW.
				if cc.nowTask != nil {
					completed := *cc.nowTask
					cc.completedTask = &completed
					// Advance
					if len(cc.nextTasks) > 0 {
						first := cc.nextTasks[0]
						cc.nowTask = &first
						cc.nextTasks = cc.nextTasks[1:]
					} else {
						cc.nowTask = nil
					}
				}
			case 2:
				// Select project
				if len(cc.projects) > 0 {
					idx := cc.scroll
					if idx >= len(cc.projects) {
						idx = len(cc.projects) - 1
					}
					cc.selectedProject = cc.projects[idx].Name
				}
			case 3:
				// Toggle habit
				if len(cc.habits) > 0 {
					idx := cc.scroll
					if idx >= len(cc.habits) {
						idx = len(cc.habits) - 1
					}
					cc.toggledHabit = cc.habits[idx].Name
					cc.habits[idx].Done = !cc.habits[idx].Done
				}
			}

		case "r":
			// Refresh data — caller will reload data
			// We just signal that the user wants a refresh by closing and reopening
			// handled by the caller
		}
	}

	return cc, nil
}

// maxScroll returns the maximum scroll value for the current section.
func (cc *CommandCenter) maxScroll() int {
	switch cc.section {
	case 0:
		return 0
	case 1:
		n := len(cc.schedule)
		if n < 1 {
			return 0
		}
		return n - 1
	case 2:
		n := len(cc.projects)
		if n < 1 {
			return 0
		}
		return n - 1
	case 3:
		n := len(cc.habits)
		if n < 1 {
			return 0
		}
		return n - 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the command center overlay.
func (cc CommandCenter) View() string {
	width := cc.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 6

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconCalendarChar + " COMMAND CENTER")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n\n")

	// ── NOW section ──
	b.WriteString(cc.viewNow(innerW))
	b.WriteString("\n")

	// ── NEXT UP section ──
	b.WriteString(cc.viewNextUp(innerW))
	b.WriteString("\n")

	// ── SCHEDULE section ──
	b.WriteString(cc.viewSchedule(innerW))
	b.WriteString("\n")

	// ── PROJECTS section ──
	b.WriteString(cc.viewProjects(innerW))
	b.WriteString("\n")

	// ── HABITS section ──
	b.WriteString(cc.viewHabitsSection(innerW))

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerW)))
	b.WriteString("\n")
	b.WriteString(cc.renderHelp())

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// viewNow renders the NOW section — the single most important task.
func (cc CommandCenter) viewNow(innerW int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	active := cc.section == 0

	header := sectionStyle.Render("  \u25b6 NOW")
	if active {
		header = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  \u25b6 NOW")
	}
	b.WriteString(header)
	b.WriteString("\n")

	if cc.nowTask == nil {
		b.WriteString(DimStyle.Render("  No pending tasks"))
		b.WriteString("\n")
		return b.String()
	}

	task := cc.nowTask

	// Build the NOW task card with border.
	icon := cc.priorityIcon(task.Priority)
	taskName := task.Text
	maxNameW := innerW - 12
	if r := []rune(taskName); len(r) > maxNameW {
		taskName = string(r[:maxNameW-3]) + "..."
	}

	cardStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(0, 1).
		Width(innerW - 4)

	var card strings.Builder
	card.WriteString(icon + " " + lipgloss.NewStyle().Foreground(text).Bold(true).Render(taskName))
	card.WriteString("\n")

	// Details line
	var details []string
	if task.DueDate != "" {
		dueLabel := cc.formatDueDate(task.DueDate)
		details = append(details, DimStyle.Render("Due: ")+
			lipgloss.NewStyle().Foreground(cc.dueDateColor(task.DueDate)).Render(dueLabel))
	}
	if task.Project != "" {
		details = append(details, lipgloss.NewStyle().Foreground(lavender).Render(task.Project))
	}
	if task.NotePath != "" {
		noteName := strings.TrimSuffix(task.NotePath, ".md")
		if r := []rune(noteName); len(r) > 20 {
			noteName = string(r[:17]) + "..."
		}
		details = append(details, DimStyle.Render("Note: "+noteName))
	}
	if len(details) > 0 {
		card.WriteString("   " + strings.Join(details, " \u00b7 "))
		card.WriteString("\n")
	}

	// Action hints
	if active {
		card.WriteString("   " + DimStyle.Render("[Space] Start Focus \u00b7 [Enter] Done"))
	}

	b.WriteString("  " + cardStyle.Render(card.String()))
	b.WriteString("\n")

	return b.String()
}

// viewNextUp renders the NEXT UP section.
func (cc CommandCenter) viewNextUp(innerW int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	b.WriteString(sectionStyle.Render("  NEXT UP"))
	b.WriteString("\n")

	if len(cc.nextTasks) == 0 {
		b.WriteString(DimStyle.Render("  No upcoming tasks"))
		b.WriteString("\n")
		return b.String()
	}

	for i, task := range cc.nextTasks {
		icon := cc.priorityIcon(task.Priority)
		taskName := task.Text
		maxW := innerW - 30
		if maxW < 20 {
			maxW = 20
		}
		if r := []rune(taskName); len(r) > maxW {
			taskName = string(r[:maxW-3]) + "..."
		}

		num := lipgloss.NewStyle().Foreground(overlay0).Render(fmt.Sprintf("  %d. ", i+1))
		name := lipgloss.NewStyle().Foreground(text).Render(taskName)

		dueStr := ""
		if task.DueDate != "" {
			dueLabel := cc.formatDueDate(task.DueDate)
			dueStr = "  " + lipgloss.NewStyle().Foreground(cc.dueDateColor(task.DueDate)).Render(dueLabel)
		}

		projStr := ""
		if task.Project != "" {
			projStr = "  " + DimStyle.Render("["+task.Project+"]")
		}

		b.WriteString(num + icon + " " + name + dueStr + projStr)
		b.WriteString("\n")
	}

	return b.String()
}

// viewSchedule renders the TODAY'S SCHEDULE section.
func (cc CommandCenter) viewSchedule(innerW int) string {
	var b strings.Builder

	active := cc.section == 1
	sectionStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	if active {
		sectionStyle = lipgloss.NewStyle().Foreground(mauve).Bold(true)
	}

	b.WriteString(sectionStyle.Render("  \u2500\u2500\u2500 TODAY'S SCHEDULE \u2500\u2500\u2500"))
	b.WriteString("\n")

	if len(cc.schedule) == 0 {
		b.WriteString(DimStyle.Render("  No scheduled items"))
		b.WriteString("\n")
		return b.String()
	}

	// Show visible items.
	visH := 6
	start := 0
	if active {
		start = cc.scroll
		if start >= len(cc.schedule) {
			start = len(cc.schedule) - 1
		}
		if start < 0 {
			start = 0
		}
	}
	end := start + visH
	if end > len(cc.schedule) {
		end = len(cc.schedule)
	}

	for i := start; i < end; i++ {
		slot := cc.schedule[i]
		timeStyle := lipgloss.NewStyle().Foreground(overlay0).Width(16)
		var icon string
		switch slot.Type {
		case "task":
			icon = cc.priorityIcon(slot.Priority)
		case "event":
			icon = lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
		case "break":
			icon = DimStyle.Render("\u2615")
		default:
			icon = DimStyle.Render("\u00b7")
		}

		taskName := slot.Task
		maxW := innerW - 22
		if maxW < 15 {
			maxW = 15
		}
		if r := []rune(taskName); len(r) > maxW {
			taskName = string(r[:maxW-3]) + "..."
		}

		nameStyle := lipgloss.NewStyle().Foreground(text)
		if slot.Done {
			nameStyle = nameStyle.Strikethrough(true).Foreground(overlay0)
		}

		isSelected := active && i == cc.scroll
		line := "  " + timeStyle.Render(slot.Time) + " " + icon + " " + nameStyle.Render(taskName)
		if isSelected {
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(innerW).
				Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// viewProjects renders the PROJECTS section.
func (cc CommandCenter) viewProjects(innerW int) string {
	var b strings.Builder

	active := cc.section == 2
	sectionStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	if active {
		sectionStyle = lipgloss.NewStyle().Foreground(mauve).Bold(true)
	}

	b.WriteString(sectionStyle.Render("  \u2500\u2500\u2500 PROJECTS \u2500\u2500\u2500"))
	b.WriteString("\n")

	if len(cc.projects) == 0 {
		b.WriteString(DimStyle.Render("  No active projects"))
		b.WriteString("\n")
		return b.String()
	}

	// Show visible projects.
	visH := 5
	start := 0
	if active {
		start = cc.scroll
		if start >= len(cc.projects) {
			start = len(cc.projects) - 1
		}
		if start < 0 {
			start = 0
		}
	}
	end := start + visH
	if end > len(cc.projects) {
		end = len(cc.projects)
	}

	for i := start; i < end; i++ {
		proj := cc.projects[i]

		// Project name
		nameColor := projectAccentColor(proj.Color)
		nameStyle := lipgloss.NewStyle().Foreground(nameColor).Bold(true)
		name := proj.Name
		if r := []rune(name); len(r) > 16 {
			name = string(r[:13]) + "..."
		}

		// Progress bar
		barWidth := 10
		pct := int(proj.Progress * 100)
		filled := barWidth * pct / 100
		if filled > barWidth {
			filled = barWidth
		}
		empty := barWidth - filled
		bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("\u2588", filled)) +
			lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2591", empty))

		pctStr := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(fmt.Sprintf("%d%%", pct))

		// Next action
		nextAct := ""
		if proj.NextAction != "" {
			act := proj.NextAction
			maxActW := innerW - 40
			if maxActW < 10 {
				maxActW = 10
			}
			if r := []rune(act); len(r) > maxActW {
				act = string(r[:maxActW-3]) + "..."
			}
			nextAct = DimStyle.Render(" \u2192 " + act)
		}

		// Task count indicator
		taskCountStr := ""
		if proj.TasksTotal > 0 {
			taskCountStr = DimStyle.Render(fmt.Sprintf(" [%d/%d]", proj.TasksDone, proj.TasksTotal))
		}

		isSelected := active && i == cc.scroll
		line := fmt.Sprintf("  %-16s %s %s%s%s",
			nameStyle.Render(ccPadRight(name, 16)),
			bar,
			pctStr,
			taskCountStr,
			nextAct)

		if isSelected {
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(innerW).
				Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// viewHabitsSection renders the HABITS section.
func (cc CommandCenter) viewHabitsSection(innerW int) string {
	var b strings.Builder

	active := cc.section == 3
	sectionStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	if active {
		sectionStyle = lipgloss.NewStyle().Foreground(mauve).Bold(true)
	}

	b.WriteString(sectionStyle.Render("  \u2500\u2500\u2500 HABITS \u2500\u2500\u2500"))
	b.WriteString("\n")

	if len(cc.habits) == 0 {
		b.WriteString(DimStyle.Render("  No habits tracked"))
		b.WriteString("\n")
		return b.String()
	}

	for i, h := range cc.habits {
		var checkbox string
		if h.Done {
			checkbox = lipgloss.NewStyle().Foreground(green).Render("[x]")
		} else {
			checkbox = lipgloss.NewStyle().Foreground(overlay0).Render("[ ]")
		}

		name := h.Name
		if r := []rune(name); len(r) > 20 {
			name = string(r[:17]) + "..."
		}
		nameStyle := lipgloss.NewStyle().Foreground(text)
		if h.Done {
			nameStyle = nameStyle.Foreground(overlay0)
		}

		streakStr := ""
		if h.Streak > 0 {
			streakStr = lipgloss.NewStyle().Foreground(peach).Bold(true).
				Render(fmt.Sprintf(" \U0001f525 %d days", h.Streak))
		}

		isSelected := active && i == cc.scroll
		line := "  " + checkbox + " " + nameStyle.Render(ccPadRight(name, 20)) + streakStr

		if isSelected {
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Width(innerW).
				Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderHelp renders the footer help bar.
func (cc CommandCenter) renderHelp() string {
	return RenderHelpBar([]struct{ Key, Desc string }{
		{"Tab", "section"}, {"j/k", "scroll"}, {"Space", "focus"},
		{"Enter", "action"}, {"Esc", "close"},
	})
}

// ---------------------------------------------------------------------------
// View helpers
// ---------------------------------------------------------------------------

// priorityIcon returns a colored priority indicator.
func (cc CommandCenter) priorityIcon(priority int) string {
	switch priority {
	case 4:
		return lipgloss.NewStyle().Foreground(red).Render("\u25cf")
	case 3:
		return lipgloss.NewStyle().Foreground(peach).Render("\u25cf")
	case 2:
		return lipgloss.NewStyle().Foreground(yellow).Render("\u25cf")
	case 1:
		return lipgloss.NewStyle().Foreground(blue).Render("\u25cf")
	default:
		return lipgloss.NewStyle().Foreground(overlay0).Render("\u25cb")
	}
}

// formatDueDate returns a human-readable due date label.
func (cc CommandCenter) formatDueDate(dueDate string) string {
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	switch {
	case dueDate < today:
		return "overdue"
	case dueDate == today:
		return "today"
	case dueDate == tomorrow:
		return "tomorrow"
	default:
		// Parse and format as "Mar 15"
		if t, err := time.Parse("2006-01-02", dueDate); err == nil {
			return t.Format("Jan 2")
		}
		return dueDate
	}
}

// dueDateColor returns a color for the due date urgency.
func (cc CommandCenter) dueDateColor(dueDate string) lipgloss.Color {
	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	switch {
	case dueDate < today:
		return red
	case dueDate == today:
		return peach
	case dueDate == tomorrow:
		return yellow
	default:
		return subtext0
	}
}

// ccPadRight pads a string to the given width.
func ccPadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
