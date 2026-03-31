package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// dashTask represents a single task entry for the dashboard.
type dashTask struct {
	Text string
	Done bool
}

// dashHabit represents a habit with today's status for the dashboard.
type dashHabit struct {
	Name      string
	Completed bool
	Streak    int
}

// dashNote represents a recently modified note.
type dashNote struct {
	Name    string
	Path    string
	TimeAgo string
}

// Dashboard is a full-screen overlay showing a vault overview at a glance.
type Dashboard struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	// Scanned data
	totalNotes   int
	totalWords   int
	totalTags    int
	totalFolders int

	// Today's tasks
	todayTasks   []dashTask
	tasksDue     int
	tasksDone    int
	overdueTasks []dashTask
	overdueCount int

	// Recent notes (top 6 by modification time)
	recentNotes []dashNote

	// Habits
	todayHabits []dashHabit

	// Writing activity (last 7 days)
	weeklyWords   [7]int
	weekDays      [7]string
	writingStreak int

	// Quick actions result
	action CommandAction

	scroll int
}

// IsActive returns whether the dashboard overlay is visible.
func (d Dashboard) IsActive() bool {
	return d.active
}

// SetSize updates the available terminal dimensions.
func (d *Dashboard) SetSize(w, h int) {
	d.width = w
	d.height = h
}

// Open activates the dashboard and scans the vault for data.
func (d *Dashboard) Open(vaultRoot string) {
	d.active = true
	d.vaultRoot = vaultRoot
	d.scroll = 0
	d.action = CmdNone
	d.scan()
}

// Close deactivates the dashboard overlay.
func (d *Dashboard) Close() {
	d.active = false
}

// GetAction returns a pending action and clears it.
func (d *Dashboard) GetAction() (CommandAction, bool) {
	if d.action != CmdNone {
		a := d.action
		d.action = CmdNone
		return a, true
	}
	return CmdNone, false
}

// scan walks the vault directory to collect stats, tasks, recent notes,
// and weekly writing activity.
func (d *Dashboard) scan() {
	d.totalNotes = 0
	d.totalWords = 0
	d.totalTags = 0
	d.totalFolders = 0
	d.todayTasks = nil
	d.tasksDue = 0
	d.tasksDone = 0
	d.overdueTasks = nil
	d.overdueCount = 0
	d.todayHabits = nil
	d.recentNotes = nil
	d.writingStreak = 0
	for i := range d.weeklyWords {
		d.weeklyWords[i] = 0
	}

	if d.vaultRoot == "" {
		return
	}

	now := time.Now()
	todayStr := now.Format("2006-01-02")
	tagSet := make(map[string]struct{})

	type fileEntry struct {
		path    string
		modTime time.Time
		words   int
	}
	var files []fileEntry

	// Populate weekDays labels for the last 7 days (oldest first).
	for i := 6; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		d.weekDays[6-i] = day.Format("Mon")
	}

	_ = filepath.Walk(d.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip hidden directories and the trash folder.
		name := info.Name()
		if info.IsDir() {
			if strings.HasPrefix(name, ".") || name == ".granit-trash" {
				return filepath.SkipDir
			}
			// Count folders (exclude the vault root itself).
			if path != d.vaultRoot {
				d.totalFolders++
			}
			return nil
		}

		if !strings.HasSuffix(name, ".md") {
			return nil
		}

		d.totalNotes++

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)
		words := len(strings.Fields(content))
		d.totalWords += words

		// Collect tags (#tag patterns).
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			// Scan for inline #tags
			for i := 0; i < len(trimmed); i++ {
				if trimmed[i] == '#' {
					// Must be at start of string or preceded by whitespace.
					if i > 0 && trimmed[i-1] != ' ' && trimmed[i-1] != '\t' {
						continue
					}
					// Not a markdown heading (## ...).
					if i+1 < len(trimmed) && trimmed[i+1] == '#' {
						continue
					}
					// Extract tag word.
					j := i + 1
					for j < len(trimmed) && trimmed[j] != ' ' && trimmed[j] != '\t' &&
						trimmed[j] != ',' && trimmed[j] != '\n' && trimmed[j] != '#' {
						j++
					}
					if j > i+1 {
						tag := trimmed[i+1 : j]
						// Skip pure numbers.
						allDigit := true
						for _, c := range tag {
							if c < '0' || c > '9' {
								allDigit = false
								break
							}
						}
						if !allDigit {
							tagSet[tag] = struct{}{}
						}
					}
				}
			}
		}

		// File info for recency / weekly stats.
		modTime := info.ModTime()
		files = append(files, fileEntry{path: path, modTime: modTime, words: words})

		// Weekly word counts — bucket by day offset.
		for i := 0; i < 7; i++ {
			dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(6 - i))
			dayEnd := dayStart.AddDate(0, 0, 1)
			if !modTime.Before(dayStart) && modTime.Before(dayEnd) {
				d.weeklyWords[i] += words
			}
		}

		return nil
	})

	d.totalTags = len(tagSet)

	// Parse today's tasks from Tasks.md (or tasks.md).
	d.parseTasks(todayStr)

	// Parse today's habit status.
	d.parseHabits(todayStr)

	// Recent notes: sort by mod time descending, take top 6.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})
	limit := 6
	if len(files) < limit {
		limit = len(files)
	}
	for i := 0; i < limit; i++ {
		rel, relErr := filepath.Rel(d.vaultRoot, files[i].path)
		if relErr != nil {
			rel = filepath.Base(files[i].path)
		}
		d.recentNotes = append(d.recentNotes, dashNote{
			Name:    rel,
			Path:    files[i].path,
			TimeAgo: dashTimeAgo(now, files[i].modTime),
		})
	}

	// Writing streak: count consecutive days (ending today) that have
	// at least one modified file.
	d.writingStreak = 0
	for i := 6; i >= 0; i-- {
		if d.weeklyWords[i] > 0 {
			// Only count from the most recent end.
			if i == 6 || d.writingStreak > 0 {
				d.writingStreak++
			}
		} else if d.writingStreak > 0 {
			break
		}
	}
}

// parseTasks reads Tasks.md and extracts items for today.
func (d *Dashboard) parseTasks(todayStr string) {
	candidates := []string{
		filepath.Join(d.vaultRoot, "Tasks.md"),
		filepath.Join(d.vaultRoot, "tasks.md"),
		filepath.Join(d.vaultRoot, "TODO.md"),
		filepath.Join(d.vaultRoot, "todo.md"),
	}

	var content string
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err == nil {
			content = string(data)
			break
		}
	}
	if content == "" {
		return
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [") {
			continue
		}

		done := false
		var taskText string
		if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			done = true
			taskText = strings.TrimSpace(trimmed[5:])
		} else if strings.HasPrefix(trimmed, "- [ ]") {
			taskText = strings.TrimSpace(trimmed[5:])
		} else {
			continue
		}

		// Include tasks that mention today's date or have no date at all.
		// Also detect overdue tasks (date before today, not done).
		hasDate := false
		for i := 0; i < len(taskText)-9; i++ {
			if taskText[i] >= '0' && taskText[i] <= '9' {
				chunk := taskText[i:]
				if len(chunk) >= 10 && chunk[4] == '-' && chunk[7] == '-' {
					dateStr := chunk[:10]
					hasDate = true
					if dateStr == todayStr {
						d.todayTasks = append(d.todayTasks, dashTask{Text: taskText, Done: done})
						d.tasksDue++
						if done {
							d.tasksDone++
						}
					} else if dateStr < todayStr && !done {
						d.overdueTasks = append(d.overdueTasks, dashTask{Text: taskText, Done: false})
						d.overdueCount++
					}
					break
				}
			}
		}
		if !hasDate {
			d.todayTasks = append(d.todayTasks, dashTask{Text: taskText, Done: done})
			d.tasksDue++
			if done {
				d.tasksDone++
			}
		}
	}
}

// parseHabits reads the Habits/habits.md file and extracts today's status.
func (d *Dashboard) parseHabits(todayStr string) {
	habitsPath := filepath.Join(d.vaultRoot, "Habits", "habits.md")
	data, err := os.ReadFile(habitsPath)
	if err != nil {
		return
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	// Parse habits from the table in ## Habits section
	type habitInfo struct {
		name   string
		streak int
	}
	var habits []habitInfo
	inHabits := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Habits" {
			inHabits = true
			continue
		}
		if strings.HasPrefix(trimmed, "## ") && inHabits {
			break
		}
		if !inHabits || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		// Skip header/separator rows
		if strings.Contains(trimmed, "---") || strings.Contains(trimmed, "Habit") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) < 4 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		streakStr := strings.TrimSpace(parts[3])
		streak := 0
		_, _ = fmt.Sscanf(streakStr, "%d", &streak)
		if name != "" {
			habits = append(habits, habitInfo{name: name, streak: streak})
		}
	}

	// Parse today's log to find completed habits
	completed := make(map[string]bool)
	inLog := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Log" {
			inLog = true
			continue
		}
		if strings.HasPrefix(trimmed, "## ") && inLog {
			break
		}
		if !inLog || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		if strings.Contains(trimmed, "---") || strings.Contains(trimmed, "Date") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) < 3 {
			continue
		}
		date := strings.TrimSpace(parts[1])
		if date == todayStr {
			names := strings.Split(parts[2], ",")
			for _, n := range names {
				completed[strings.TrimSpace(n)] = true
			}
		}
	}

	// Build dashboard habit list
	for _, h := range habits {
		d.todayHabits = append(d.todayHabits, dashHabit{
			Name:      h.name,
			Completed: completed[h.name],
			Streak:    h.streak,
		})
	}
}

// Update handles key input for the dashboard overlay.
func (d Dashboard) Update(msg tea.Msg) (Dashboard, tea.Cmd) {
	if !d.active {
		return d, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			d.active = false
		case "n":
			d.action = CmdNewNote
			d.active = false
		case "t":
			d.action = CmdTaskManager
			d.active = false
		case "c":
			d.action = CmdShowCalendar
			d.active = false
		case "s":
			d.action = CmdStandupGenerator
			d.active = false
		case "d":
			d.action = CmdDailyNote
			d.active = false
		case "f":
			d.action = CmdFocusSession
			d.active = false
		case "j", "down":
			d.scroll++
		case "k", "up":
			if d.scroll > 0 {
				d.scroll--
			}
		}
	}
	return d, nil
}

// View renders the dashboard overlay.
func (d Dashboard) View() string {
	panelWidth := d.width * 9 / 10
	if panelWidth > 120 {
		panelWidth = 120
	}
	if panelWidth < 60 {
		panelWidth = 60
	}
	innerW := panelWidth - 6 // account for border + padding

	// Style definitions.
	sectionTitle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	dimSt := lipgloss.NewStyle().Foreground(overlay0)
	greetStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	dateStyle := lipgloss.NewStyle().Foreground(subtext0)
	doneStyle := lipgloss.NewStyle().Foreground(green)
	todoStyle := lipgloss.NewStyle().Foreground(yellow)
	noteIconStyle := lipgloss.NewStyle().Foreground(blue)
	streakFilled := lipgloss.NewStyle().Foreground(green).Render("\u2588")
	streakEmpty := lipgloss.NewStyle().Foreground(surface1).Render("\u2591")
	actFilled := lipgloss.NewStyle().Foreground(mauve).Render("\u2588")
	actEmpty := lipgloss.NewStyle().Foreground(surface0).Render("\u25cb")

	var lines []string

	// --- Greeting ---
	now := time.Now()
	greeting := greetingForHour(now.Hour())
	user := os.Getenv("USER")
	if user == "" {
		user = "friend"
	}
	dateStr := now.Format("Mon, Jan 2 2006")
	greetLine := greetStyle.Render(fmt.Sprintf("  %s, %s!", greeting, user))
	datePart := dateStyle.Render(dateStr)
	gap := innerW - lipgloss.Width(greetLine) - lipgloss.Width(datePart)
	if gap < 2 {
		gap = 2
	}
	lines = append(lines, greetLine+strings.Repeat(" ", gap)+datePart)
	lines = append(lines, dimSt.Render("  "+strings.Repeat("\u2500", innerW-4)))
	lines = append(lines, "")

	// --- Overdue warning ---
	if d.overdueCount > 0 {
		warnStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		lines = append(lines, warnStyle.Render(fmt.Sprintf("  ⚠ %d OVERDUE TASK", d.overdueCount)+func() string {
			if d.overdueCount > 1 {
				return "S"
			}
			return ""
		}()))
		shown := d.overdueCount
		if shown > 3 {
			shown = 3
		}
		for i := 0; i < shown; i++ {
			taskText := TruncateDisplay(d.overdueTasks[i].Text, innerW-8)
			lines = append(lines, lipgloss.NewStyle().Foreground(red).Render("    • "+taskText))
		}
		if d.overdueCount > 3 {
			lines = append(lines, dimSt.Render(fmt.Sprintf("    +%d more", d.overdueCount-3)))
		}
		lines = append(lines, "")
	}

	// --- Two-column: Today's Tasks | Recent Notes ---
	halfW := (innerW - 4) / 2
	if halfW < 24 {
		halfW = 24
	}

	// Today's Tasks sub-panel.
	var taskLines []string
	taskLines = append(taskLines, sectionTitle.Render(IconCalendarChar+" Today's Tasks"))
	taskLines = append(taskLines, dimSt.Render(strings.Repeat("\u2500", halfW-4)))
	if len(d.todayTasks) == 0 {
		taskLines = append(taskLines, dimSt.Render("  No tasks for today"))
		taskLines = append(taskLines, dimSt.Render("  Press 't' to open task manager"))
	} else {
		shown := d.todayTasks
		if len(shown) > 5 {
			shown = shown[:5]
		}
		for _, t := range shown {
			mark := todoStyle.Render("\u25a1")
			if t.Done {
				mark = doneStyle.Render("\u25a0")
			}
			txt := TruncateDisplay(t.Text, halfW-6)
			taskLines = append(taskLines, "  "+mark+" "+labelStyle.Render(txt))
		}
		summary := fmt.Sprintf("  %d tasks (%d done)", len(d.todayTasks), d.tasksDone)
		taskLines = append(taskLines, dimSt.Render(summary))
	}
	taskPanel := lipgloss.NewStyle().
		Width(halfW).
		Render(strings.Join(taskLines, "\n"))

	// Recent Notes sub-panel.
	var recentLines []string
	recentLines = append(recentLines, sectionTitle.Render(IconFileChar+" Recent Notes"))
	recentLines = append(recentLines, dimSt.Render(strings.Repeat("\u2500", halfW-4)))
	if len(d.recentNotes) == 0 {
		recentLines = append(recentLines, dimSt.Render("  No recent notes"))
		recentLines = append(recentLines, dimSt.Render("  Press 'n' to create one"))
	} else {
		for _, n := range d.recentNotes {
			maxNameW := halfW - lipgloss.Width(n.TimeAgo) - 8
			if maxNameW < 10 {
				maxNameW = 10
			}
			name := TruncateDisplay(n.Name, maxNameW)
			gap := halfW - 6 - lipgloss.Width(name) - lipgloss.Width(n.TimeAgo)
			if gap < 1 {
				gap = 1
			}
			recentLines = append(recentLines,
				"  "+noteIconStyle.Render(IconFileChar)+" "+labelStyle.Render(name)+
					strings.Repeat(" ", gap)+dimSt.Render(n.TimeAgo))
		}
	}
	recentPanel := lipgloss.NewStyle().
		Width(halfW).
		Render(strings.Join(recentLines, "\n"))

	row1 := lipgloss.JoinHorizontal(lipgloss.Top, taskPanel, "  ", recentPanel)
	lines = append(lines, row1)
	lines = append(lines, "")

	// --- Two-column: Quick Stats | Streaks ---
	var statsLines []string
	statsLines = append(statsLines, sectionTitle.Render(IconGraphChar+" Quick Stats"))
	statsLines = append(statsLines, dimSt.Render(strings.Repeat("\u2500", halfW-4)))
	statsLines = append(statsLines, fmt.Sprintf("  %s notes   %s words",
		numStyle.Render(formatNum(d.totalNotes)),
		numStyle.Render(formatNum(d.totalWords))))
	statsLines = append(statsLines, fmt.Sprintf("  %s tasks   %s due",
		numStyle.Render(formatNum(d.tasksDue)),
		numStyle.Render(formatNum(d.tasksDue-d.tasksDone))))
	statsLines = append(statsLines, fmt.Sprintf("  %s tags    %s folders",
		numStyle.Render(formatNum(d.totalTags)),
		numStyle.Render(formatNum(d.totalFolders))))
	statsPanel := lipgloss.NewStyle().
		Width(halfW).
		Render(strings.Join(statsLines, "\n"))

	var streakLines []string
	streakLines = append(streakLines, sectionTitle.Render(IconDailyChar+" Streaks"))
	streakLines = append(streakLines, dimSt.Render(strings.Repeat("\u2500", halfW-4)))

	// Writing streak bar.
	streakBar := "  Writing: " + numStyle.Render(fmt.Sprintf("%d days ", d.writingStreak))
	for i := 0; i < 7; i++ {
		if i < d.writingStreak {
			streakBar += streakFilled
		} else {
			streakBar += streakEmpty
		}
	}
	streakLines = append(streakLines, streakBar)

	// Activity score (notes modified today).
	todayCount := 0
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	_ = filepath.Walk(d.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(info.Name(), ".md") {
			return nil
		}
		if !info.ModTime().Before(todayStart) {
			todayCount++
		}
		return nil
	})
	streakLines = append(streakLines, fmt.Sprintf("  Today:   %s notes edited",
		numStyle.Render(smallNum(todayCount))))

	streakPanel := lipgloss.NewStyle().
		Width(halfW).
		Render(strings.Join(streakLines, "\n"))

	row2 := lipgloss.JoinHorizontal(lipgloss.Top, statsPanel, "  ", streakPanel)
	lines = append(lines, row2)
	lines = append(lines, "")

	// --- This Week activity bar ---
	var weekLines []string
	weekLines = append(weekLines, sectionTitle.Render(IconOutlineChar+" This Week"))
	weekLines = append(weekLines, dimSt.Render(strings.Repeat("\u2500", innerW-4)))

	// Day labels.
	dayRow := "  "
	for i := 0; i < 7; i++ {
		dayRow += fmt.Sprintf("%-6s", d.weekDays[i])
	}
	weekLines = append(weekLines, dimSt.Render(dayRow))

	// Activity dots.
	dotRow := "  "
	for i := 0; i < 7; i++ {
		if d.weeklyWords[i] > 0 {
			dotRow += fmt.Sprintf("%-6s", actFilled)
		} else {
			dotRow += fmt.Sprintf("%-6s", actEmpty)
		}
	}
	weekLines = append(weekLines, dotRow)

	// Word counts.
	wordRow := "  "
	for i := 0; i < 7; i++ {
		val := "-"
		if d.weeklyWords[i] > 0 {
			val = formatNum(d.weeklyWords[i])
		}
		wordRow += fmt.Sprintf("%-6s", val)
	}
	weekLines = append(weekLines, labelStyle.Render(wordRow)+" "+dimSt.Render("words"))

	lines = append(lines, strings.Join(weekLines, "\n"))
	lines = append(lines, "")

	// --- Today's Habits ---
	if len(d.todayHabits) > 0 {
		lines = append(lines, sectionTitle.Render("  "+IconCalendarChar+" Today's Habits"))
		for _, h := range d.todayHabits {
			var icon string
			if h.Completed {
				icon = doneStyle.Render("[x]")
			} else {
				icon = todoStyle.Render("[ ]")
			}
			streakText := ""
			if h.Streak > 0 {
				streakText = dimSt.Render(fmt.Sprintf(" %dd", h.Streak))
			}
			lines = append(lines, "    "+icon+" "+labelStyle.Render(h.Name)+streakText)
		}
		lines = append(lines, "")
	}

	// --- Footer ---
	lines = append(lines, dimSt.Render(strings.Repeat("\u2500", innerW-4)))
	lines = append(lines, RenderHelpBar([]struct{ Key, Desc string }{
		{"n", "new note"}, {"t", "tasks"}, {"c", "calendar"},
		{"s", "standup"}, {"d", "daily"}, {"f", "focus"}, {"Esc", "close"},
	}))

	// Assemble with scrolling.
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  "+IconOutlineChar+" Dashboard")
	b.WriteString(title)
	b.WriteString("\n")

	visH := d.height - 8
	if visH < 10 {
		visH = 10
	}

	// Flatten all lines (some entries contain newlines from JoinHorizontal).
	var flat []string
	for _, l := range lines {
		flat = append(flat, strings.Split(l, "\n")...)
	}

	maxScroll := len(flat) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if d.scroll > maxScroll {
		d.scroll = maxScroll
	}
	end := d.scroll + visH
	if end > len(flat) {
		end = len(flat)
	}

	if d.scroll > 0 {
		b.WriteString(DimStyle.Render("  "+ScrollIndicator(d.scroll, len(flat), visH)) + "\n")
	}
	for i := d.scroll; i < end; i++ {
		b.WriteString(flat[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	if end < len(flat) {
		b.WriteString("\n" + DimStyle.Render("  "+ScrollIndicator(d.scroll, len(flat), visH)))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return border.Render(b.String())
}

// greetingForHour returns a time-appropriate greeting.
func greetingForHour(hour int) string {
	switch {
	case hour >= 5 && hour < 12:
		return "Good morning"
	case hour >= 12 && hour < 17:
		return "Good afternoon"
	case hour >= 17 && hour < 21:
		return "Good evening"
	default:
		return "Good night"
	}
}

// dashTimeAgo formats a duration between now and t as a compact string.
func dashTimeAgo(now, t time.Time) string {
	diff := now.Sub(t)
	switch {
	case diff < time.Minute:
		return "now"
	case diff < time.Hour:
		m := int(diff.Minutes())
		return fmt.Sprintf("%dm", m)
	case diff < 24*time.Hour:
		h := int(diff.Hours())
		return fmt.Sprintf("%dh", h)
	case diff < 7*24*time.Hour:
		d := int(diff.Hours() / 24)
		return fmt.Sprintf("%dd", d)
	default:
		w := int(diff.Hours() / 24 / 7)
		return fmt.Sprintf("%dw", w)
	}
}
