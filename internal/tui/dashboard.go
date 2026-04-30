package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/objects"
)

// dashTask represents a single task entry for the dashboard.
type dashTask struct {
	Text string
	Done bool
}

// dashTypeCount is one row in the typed-objects panel: type icon +
// human label + object count. Sorted by count desc when rendered.
type dashTypeCount struct {
	Icon  string
	Name  string
	ID    string
	Count int
}

// dashRecentObject is one row in the "Recently Captured" sub-panel —
// most-recently-modified typed objects across all types. Lets the user
// see capture velocity at a glance.
type dashRecentObject struct {
	Title   string
	TypeID  string
	Icon    string
	Path    string
	TimeAgo string
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
	OverlayBase
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
	todayHabits      []dashHabit
	habitFileContent string // cached during vault walk to avoid re-reading

	// Writing activity (last 7 days)
	weeklyWords   [7]int
	weekDays      [7]string
	writingStreak int

	// Projects & Goals summary
	activeProjects int
	totalProjects  int
	activeGoals    int
	totalGoals     int
	projectNames   []string // top active project names
	goalNames      []string // top active goal names

	// Daily scripture (cached on open)
	dailyScripture Scripture

	// Typed-objects panel (Phase 9): per-type counts, top-N recently
	// created objects, and inline results from the primary saved view.
	// All populated via SetTypedObjects before Open. Empty when the
	// vault has no typed objects — the renderer just hides the panel.
	objCountsByType []dashTypeCount     // sorted by count desc
	objTotal        int                 // sum across all types
	objRecent       []dashRecentObject  // top 6 by mtime
	primaryView     *objects.View       // optional starred view
	primaryViewObjs []*objects.Object   // resolved objects for the primary view (top 5)

	// Business/Revenue metrics
	bizTasksDone  int // completed tasks tagged #revenue/#client/#business this week
	bizTasksTotal int // total such tasks this week
	bizHoursWeek  float64 // hours tracked on business tasks this week

	// Productivity Pulse (aggregated from FocusSessions + TimeTracker)
	focusMinToday  int    // total focus minutes today (from FocusSessions file)
	focusSessions  int    // number of focus/pomodoro sessions today
	nextDeadline   string // soonest upcoming task due date text
	nextDeadlineDays int  // days until next deadline (-1 = none)

	// Today's edit count (files modified today)
	todayEditCount int

	// Quick actions result
	action CommandAction

	scroll int
}

// SetTypedObjects feeds the dashboard the typed-objects context. Call
// this BEFORE Open() so the panel data is ready by first render. The
// primaryViewID is optional — when non-empty and present in the
// catalog, the dashboard surfaces its top results inline. Pass an
// empty string to skip the saved-view section.
//
// All inputs are tolerated as nil — the renderer hides the panel
// entirely when there's nothing to show. This way callers don't have
// to special-case "vault hasn't been scanned yet."
func (d *Dashboard) SetTypedObjects(reg *objects.Registry, idx *objects.Index,
	cat *objects.ViewCatalog, primaryViewID string, vaultRoot string) {

	d.objCountsByType = nil
	d.objTotal = 0
	d.objRecent = nil
	d.primaryView = nil
	d.primaryViewObjs = nil

	if idx == nil || reg == nil {
		return
	}
	d.objTotal = idx.Total()

	// Per-type counts. Iterate registry.All() to keep registry order
	// (not arbitrary map order); skip empty types so the panel stays
	// dense — the Object Browser is the right place to discover empty
	// types, the dashboard surfaces utilisation.
	counts := idx.CountByType()
	for _, t := range reg.All() {
		c := counts[t.ID]
		if c == 0 {
			continue
		}
		d.objCountsByType = append(d.objCountsByType, dashTypeCount{
			Icon: t.Icon, Name: t.Name, ID: t.ID, Count: c,
		})
	}
	sort.Slice(d.objCountsByType, func(i, j int) bool {
		return d.objCountsByType[i].Count > d.objCountsByType[j].Count
	})

	// Recently-created objects: read mtime for each indexed object,
	// sort desc, keep the top 6. Bounded scan: only iterates over
	// indexed (typed) objects, not the whole vault.
	type pathTime struct {
		path  string
		mtime time.Time
	}
	all := []pathTime{}
	for _, t := range reg.All() {
		for _, obj := range idx.ByType(t.ID) {
			abs := filepath.Join(vaultRoot, obj.NotePath)
			if info, err := os.Stat(abs); err == nil {
				all = append(all, pathTime{path: obj.NotePath, mtime: info.ModTime()})
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].mtime.After(all[j].mtime) })
	if len(all) > 6 {
		all = all[:6]
	}
	for _, pt := range all {
		obj := idx.ByPath(pt.path)
		if obj == nil {
			continue
		}
		t, _ := reg.ByID(obj.TypeID)
		d.objRecent = append(d.objRecent, dashRecentObject{
			Title: obj.Title, TypeID: obj.TypeID, Icon: t.Icon,
			Path: obj.NotePath, TimeAgo: relativeTime(pt.mtime),
		})
	}

	// Primary saved view (optional inline section).
	if cat != nil && strings.TrimSpace(primaryViewID) != "" {
		if v, ok := cat.ByID(primaryViewID); ok {
			d.primaryView = &v
			res := objects.Evaluate(idx, v)
			if len(res) > 5 {
				res = res[:5]
			}
			d.primaryViewObjs = res
		}
	}
}

// relativeTime formats a "moments ago / 5m / 2h / 3d" string for
// dashboard rows. Identical pattern to recentNotes' TimeAgo.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// Open activates the dashboard and scans the vault for data.
func (d *Dashboard) Open(vaultRoot string, projects []Project, goals []Goal) {
	d.Activate()
	d.vaultRoot = vaultRoot
	d.scroll = 0
	d.action = CmdNone

	// Summarise projects
	d.totalProjects = len(projects)
	d.activeProjects = 0
	d.projectNames = nil
	for _, p := range projects {
		if p.Status == "" || p.Status == "active" {
			d.activeProjects++
			if len(d.projectNames) < 5 {
				d.projectNames = append(d.projectNames, p.Name)
			}
		}
	}

	// Summarise goals
	d.totalGoals = len(goals)
	d.activeGoals = 0
	d.goalNames = nil
	for _, g := range goals {
		if g.Status == GoalStatusActive {
			d.activeGoals++
			if len(d.goalNames) < 5 {
				d.goalNames = append(d.goalNames, g.Title)
			}
		}
	}

	d.dailyScripture = DailyScripture(vaultRoot)
	d.scan()
	d.scanProductivity()
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

// scan walks the vault directory once to collect all dashboard data: stats,
// tasks, business metrics, today's edit count, recent notes, and weekly
// writing activity.
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
	d.bizTasksDone = 0
	d.bizTasksTotal = 0
	d.bizHoursWeek = 0
	d.todayEditCount = 0
	d.focusMinToday = 0
	d.focusSessions = 0
	d.nextDeadline = ""
	d.nextDeadlineDays = -1
	for i := range d.weeklyWords {
		d.weeklyWords[i] = 0
	}

	if d.vaultRoot == "" {
		return
	}

	now := time.Now()
	todayStr := now.Format("2006-01-02")
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := todayStart.AddDate(0, 0, -int(now.Weekday()))
	tagSet := make(map[string]struct{})

	// Regexes for task parsing.
	taskRe := regexp.MustCompile(`^\s*- \[([ xX])\] (.+)`)
	dueDateRe := regexp.MustCompile(`\x{1F4C5}\s*(\d{4}-\d{2}-\d{2})`)

	// Business tags to match.
	bizTags := []string{"#revenue", "#client", "#business", "#sales", "#invoice"}

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
		modTime := info.ModTime()

		// Today's edit count.
		if !modTime.Before(todayStart) {
			d.todayEditCount++
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)
		words := len(strings.Fields(content))
		d.totalWords += words

		// Cache habits file content to avoid a second read in parseHabits.
		rel, _ := filepath.Rel(d.vaultRoot, path)
		if rel == filepath.Join("Habits", "habits.md") {
			d.habitFileContent = content
		}

		// Infer due date from daily note filename (YYYY-MM-DD.md).
		noteDateStr := ""
		base := strings.TrimSuffix(name, ".md")
		if _, parseErr := time.Parse("2006-01-02", base); parseErr == nil {
			noteDateStr = base
		}

		// Whether this file was modified this week (for business metrics).
		modifiedThisWeek := !modTime.Before(weekStart)

		// Process each line: tags, tasks, and business metrics in one pass.
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)

			// --- Tag collection ---
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

			// --- Task parsing (today + overdue) ---
			if m := taskRe.FindStringSubmatch(line); m != nil {
				done := m[1] == "x" || m[1] == "X"
				taskText := m[2]

				// Find due date: explicit emoji takes priority, then daily note filename.
				dueDate := ""
				if dm := dueDateRe.FindStringSubmatch(taskText); dm != nil {
					dueDate = dm[1]
				} else if noteDateStr != "" {
					dueDate = noteDateStr
				}

				if dueDate == todayStr {
					d.todayTasks = append(d.todayTasks, dashTask{Text: taskText, Done: done})
					d.tasksDue++
					if done {
						d.tasksDone++
					}
				} else if dueDate != "" && dueDate < todayStr && !done {
					d.overdueTasks = append(d.overdueTasks, dashTask{Text: taskText, Done: false})
					d.overdueCount++
				}
				// Track soonest upcoming deadline.
				if dueDate > todayStr && !done {
					if dt, err := time.Parse("2006-01-02", dueDate); err == nil {
						days := int(dt.Sub(todayStart).Hours() / 24)
						if d.nextDeadlineDays < 0 || days < d.nextDeadlineDays {
							d.nextDeadlineDays = days
							// Clean the task text for display (strip date emoji).
							label := dueDateRe.ReplaceAllString(taskText, "")
							d.nextDeadline = strings.TrimSpace(label)
						}
					}
				}
			}

			// --- Business metrics (tasks with biz tags, modified this week) ---
			if modifiedThisWeek && strings.HasPrefix(trimmed, "- [") {
				lower := strings.ToLower(trimmed)
				isBiz := false
				for _, tag := range bizTags {
					if strings.Contains(lower, tag) {
						isBiz = true
						break
					}
				}
				if isBiz {
					d.bizTasksTotal++
					if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
						d.bizTasksDone++
					}
				}
			}
		}

		// File info for recency / weekly stats.
		files = append(files, fileEntry{path: path, modTime: modTime, words: words})

		// Weekly word counts — bucket by day offset.
		for i := 0; i < 7; i++ {
			dayStart := todayStart.AddDate(0, 0, -(6 - i))
			dayEnd := dayStart.AddDate(0, 0, 1)
			if !modTime.Before(dayStart) && modTime.Before(dayEnd) {
				d.weeklyWords[i] += words
			}
		}

		return nil
	})

	d.totalTags = len(tagSet)

	// Parse today's habit status (reads a single specific file, not a vault walk).
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

// scanProductivity reads today's FocusSessions file and timetracker data
// to populate the Productivity Pulse section.
func (d *Dashboard) scanProductivity() {
	if d.vaultRoot == "" {
		return
	}
	todayStr := time.Now().Format("2006-01-02")
	fp := filepath.Join(d.vaultRoot, "FocusSessions", todayStr+".md")
	data, err := os.ReadFile(fp)
	if err != nil {
		return
	}
	durationRe := regexp.MustCompile(`Duration:\s*(\d+)\s*min`)
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## Session") {
			d.focusSessions++
		}
		if m := durationRe.FindStringSubmatch(trimmed); m != nil {
			var v int
			if _, err := fmt.Sscanf(m[1], "%d", &v); err == nil {
				d.focusMinToday += v
			}
		}
	}
}

// parseHabits extracts today's habit status from the cached habits file content.
func (d *Dashboard) parseHabits(todayStr string) {
	content := d.habitFileContent
	if content == "" {
		// Fallback: file wasn't encountered during vault walk
		data, err := os.ReadFile(filepath.Join(d.vaultRoot, "Habits", "habits.md"))
		if err != nil {
			return
		}
		content = string(data)
	}
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
		// Skip separator rows like |---|---|---|
		if strings.Contains(trimmed, "---") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) < 4 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		// Skip the header row by exact-matching the first cell
		// (using Contains here would drop habits whose name contains "Habit").
		if strings.EqualFold(name, "habit") {
			continue
		}
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
		if strings.Contains(trimmed, "---") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) < 3 {
			continue
		}
		date := strings.TrimSpace(parts[1])
		// Skip the header row by exact-matching the first cell.
		if strings.EqualFold(date, "date") {
			continue
		}
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
		case "p":
			d.action = CmdProjectMode
			d.active = false
		case "g":
			d.action = CmdGoalsMode
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

	// --- Daily Scripture ---
	verseStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)
	refStyle := lipgloss.NewStyle().Foreground(overlay0)
	verseTxt := TruncateDisplay(d.dailyScripture.Text, innerW-6)
	lines = append(lines, verseStyle.Render("  "+verseTxt))
	lines = append(lines, refStyle.Render("  "+d.dailyScripture.Source))
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
		if shown > len(d.overdueTasks) {
			shown = len(d.overdueTasks)
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

	// --- Typed Objects panel (Phase 9): two columns when populated.
	// Hidden entirely when the vault has no typed objects so brand-new
	// users don't see an empty section confusingly.
	if d.objTotal > 0 {
		// Left column: per-type counts (top 6).
		var typeLines []string
		typeLines = append(typeLines, sectionTitle.Render("📦 Typed Objects"))
		typeLines = append(typeLines, dimSt.Render(strings.Repeat("─", halfW-4)))
		shown := d.objCountsByType
		if len(shown) > 6 {
			shown = shown[:6]
		}
		for _, c := range shown {
			icon := c.Icon
			if strings.TrimSpace(icon) == "" {
				icon = "•"
			}
			label := TruncateDisplay(c.Name, halfW-12)
			line := fmt.Sprintf("  %s %s",
				icon,
				labelStyle.Render(PadRight(label, halfW-10)))
			line += numStyle.Render(fmt.Sprintf("%d", c.Count))
			typeLines = append(typeLines, line)
		}
		typeLines = append(typeLines, "")
		typeLines = append(typeLines, dimSt.Render(fmt.Sprintf("  %d total · Alt+O to browse · Alt+V for views",
			d.objTotal)))
		typesPanel := lipgloss.NewStyle().Width(halfW).
			Render(strings.Join(typeLines, "\n"))

		// Right column: recently captured + (optional) primary saved view.
		var rightLines []string
		rightTitle := "Recently Captured"
		rightLines = append(rightLines, sectionTitle.Render("🕐 "+rightTitle))
		rightLines = append(rightLines, dimSt.Render(strings.Repeat("─", halfW-4)))
		if len(d.objRecent) == 0 {
			rightLines = append(rightLines, dimSt.Render("  No typed objects yet"))
		} else {
			for _, r := range d.objRecent {
				icon := r.Icon
				if strings.TrimSpace(icon) == "" {
					icon = "•"
				}
				titleW := halfW - 14
				if titleW < 8 {
					titleW = 8
				}
				title := TruncateDisplay(r.Title, titleW)
				rightLines = append(rightLines,
					"  "+icon+" "+labelStyle.Render(PadRight(title, titleW))+
						"  "+dimSt.Render(r.TimeAgo))
			}
		}
		// Optional saved-view inline.
		if d.primaryView != nil {
			rightLines = append(rightLines, "")
			rightLines = append(rightLines, sectionTitle.Render("☆ "+d.primaryView.Name))
			rightLines = append(rightLines, dimSt.Render(strings.Repeat("─", halfW-4)))
			if len(d.primaryViewObjs) == 0 {
				rightLines = append(rightLines, dimSt.Render("  (no matches)"))
			} else {
				for _, obj := range d.primaryViewObjs {
					title := TruncateDisplay(obj.Title, halfW-6)
					rightLines = append(rightLines, "  • "+labelStyle.Render(title))
				}
			}
		}
		recentObjsPanel := lipgloss.NewStyle().Width(halfW).
			Render(strings.Join(rightLines, "\n"))

		row1b := lipgloss.JoinHorizontal(lipgloss.Top, typesPanel, "  ", recentObjsPanel)
		lines = append(lines, row1b)
		lines = append(lines, "")
	}

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

	// Activity score (notes modified today — computed in scan()).
	streakLines = append(streakLines, fmt.Sprintf("  Today:   %s notes edited",
		numStyle.Render(smallNum(d.todayEditCount))))

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

	// --- Projects & Goals ---
	if d.totalProjects > 0 || d.totalGoals > 0 {
		var projLines []string
		projLines = append(projLines, sectionTitle.Render(IconFolderChar+" Projects"))
		projLines = append(projLines, dimSt.Render(strings.Repeat("\u2500", halfW-4)))
		projLines = append(projLines, fmt.Sprintf("  %s active / %s total",
			numStyle.Render(smallNum(d.activeProjects)),
			dimSt.Render(smallNum(d.totalProjects))))
		for _, name := range d.projectNames {
			projLines = append(projLines, "  "+labelStyle.Render(IconFolderChar)+" "+labelStyle.Render(TruncateDisplay(name, halfW-6)))
		}
		projPanel := lipgloss.NewStyle().Width(halfW).Render(strings.Join(projLines, "\n"))

		var goalLines []string
		goalLines = append(goalLines, sectionTitle.Render(IconGraphChar+" Goals"))
		goalLines = append(goalLines, dimSt.Render(strings.Repeat("\u2500", halfW-4)))
		goalLines = append(goalLines, fmt.Sprintf("  %s active / %s total",
			numStyle.Render(smallNum(d.activeGoals)),
			dimSt.Render(smallNum(d.totalGoals))))
		for _, name := range d.goalNames {
			goalLines = append(goalLines, "  "+labelStyle.Render(IconGraphChar)+" "+labelStyle.Render(TruncateDisplay(name, halfW-6)))
		}
		goalPanel := lipgloss.NewStyle().Width(halfW).Render(strings.Join(goalLines, "\n"))

		row3 := lipgloss.JoinHorizontal(lipgloss.Top, projPanel, "  ", goalPanel)
		lines = append(lines, row3)
		lines = append(lines, "")
	}

	// --- Business Pulse ---
	if d.bizTasksTotal > 0 {
		bizStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
		lines = append(lines, bizStyle.Render("  "+IconGraphChar+" Business Pulse (this week)"))
		lines = append(lines, dimSt.Render("  "+strings.Repeat("\u2500", innerW-6)))
		lines = append(lines, fmt.Sprintf("  %s/%s tasks completed",
			numStyle.Render(smallNum(d.bizTasksDone)),
			dimSt.Render(smallNum(d.bizTasksTotal))))
		pct := 0
		if d.bizTasksTotal > 0 {
			pct = d.bizTasksDone * 100 / d.bizTasksTotal
			if pct > 100 {
				pct = 100
			}
		}
		barW := 20
		filled := barW * pct / 100
		bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("\u2588", filled)) +
			lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2591", barW-filled))
		lines = append(lines, "  "+bar+" "+numStyle.Render(fmt.Sprintf("%d%%", pct)))
		lines = append(lines, dimSt.Render("  Tag tasks with #revenue #client #business"))
		lines = append(lines, "")
	}

	// --- Productivity Pulse ---
	if d.focusSessions > 0 || d.nextDeadlineDays >= 0 {
		lines = append(lines, sectionTitle.Render("\u26A1 Productivity Pulse"))
		lines = append(lines, dimSt.Render(strings.Repeat("\u2500", innerW-4)))

		if d.focusSessions > 0 {
			hours := d.focusMinToday / 60
			mins := d.focusMinToday % 60
			var timeStr string
			if hours > 0 {
				timeStr = fmt.Sprintf("%dh %dm", hours, mins)
			} else {
				timeStr = fmt.Sprintf("%d min", mins)
			}
			lines = append(lines, fmt.Sprintf("  %s focus time today (%s sessions)",
				numStyle.Render(timeStr),
				numStyle.Render(smallNum(d.focusSessions))))
		}

		if d.nextDeadlineDays >= 0 && d.nextDeadline != "" {
			deadlineLabel := TruncateDisplay(d.nextDeadline, innerW-30)
			var urgency string
			switch {
			case d.nextDeadlineDays <= 1:
				urgency = lipgloss.NewStyle().Foreground(red).Bold(true).Render("tomorrow")
			case d.nextDeadlineDays <= 3:
				urgency = lipgloss.NewStyle().Foreground(peach).Render(
					fmt.Sprintf("%d days", d.nextDeadlineDays))
			default:
				urgency = dimSt.Render(fmt.Sprintf("%d days", d.nextDeadlineDays))
			}
			lines = append(lines, fmt.Sprintf("  %s %s in %s",
				lipgloss.NewStyle().Foreground(yellow).Render("\u23F3"),
				labelStyle.Render(deadlineLabel), urgency))
		}
		lines = append(lines, "")
	}

	// --- Footer ---
	lines = append(lines, dimSt.Render(strings.Repeat("\u2500", innerW-4)))
	lines = append(lines, RenderHelpBar([]struct{ Key, Desc string }{
		{"n", "new note"}, {"t", "tasks"}, {"p", "projects"},
		{"g", "goals"}, {"d", "daily"}, {"f", "focus"}, {"Esc", "close"},
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
