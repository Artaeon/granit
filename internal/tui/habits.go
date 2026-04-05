package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

// habitEntry represents a single tracked habit.
type habitEntry struct {
	Name    string
	Created string // YYYY-MM-DD
	Streak  int
}

// goalEntry represents a goal with milestones.
type goalEntry struct {
	Title      string
	TargetDate string // YYYY-MM-DD
	Milestones []milestone
	Archived   bool
}

// milestone is a sub-task of a goal.
type milestone struct {
	Text string
	Done bool
}

// goalProgress returns the percentage of completed milestones.
func (g goalEntry) goalProgress() int {
	if len(g.Milestones) == 0 {
		return 0
	}
	done := 0
	for _, m := range g.Milestones {
		if m.Done {
			done++
		}
	}
	return done * 100 / len(g.Milestones)
}

// habitLog represents a day's completed habits.
type habitLog struct {
	Date      string // YYYY-MM-DD
	Completed []string
}

// habitInputMode tracks which input field is active.
type habitInputMode int

const (
	habitInputNone habitInputMode = iota
	habitInputNewHabit
	habitInputNewGoalTitle
	habitInputNewGoalDate
	habitInputNewMilestone
)

// HabitTracker is an overlay for tracking daily habits and goals.
type HabitTracker struct {
	active bool
	width  int
	height int

	vaultRoot string
	tab       int // 0=habits, 1=goals, 2=stats
	cursor    int
	scroll    int

	habits []habitEntry
	logs   []habitLog
	goals  []goalEntry

	// Input state
	inputMode  habitInputMode
	inputValue string

	// Goal sub-cursor for milestones
	goalExpanded int // index of expanded goal, -1 = none
	milestoneCur int // cursor within milestones

	// Delete confirmation
	confirmDelete bool

	// AI Coach
	ai        AIConfig
	aiPending bool
	coachText string
	showCoach bool

	// Vault reference for syncing habit completions to tasks
	vault *vault.Vault
}

// habitAICoachMsg carries a holistic AI analysis of habit patterns.
type habitAICoachMsg struct {
	analysis string
	err      error
}

// aiHabitCoach sends habit data to the LLM for pattern analysis.
func (ht *HabitTracker) aiHabitCoach() tea.Cmd {
	ai := ht.ai
	habits := make([]habitEntry, len(ht.habits))
	copy(habits, ht.habits)
	logs := make([]habitLog, len(ht.logs))
	for i, l := range ht.logs {
		logs[i] = habitLog{Date: l.Date, Completed: make([]string, len(l.Completed))}
		copy(logs[i].Completed, l.Completed)
	}

	return func() tea.Msg {
		now := time.Now()
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Today: %s (%s)\n\n", now.Format("2006-01-02"), now.Weekday()))

		sb.WriteString("HABITS:\n")
		for _, h := range habits {
			sb.WriteString(fmt.Sprintf("- %s (streak: %d, created: %s)\n", h.Name, h.Streak, h.Created))
		}

		sb.WriteString("\nRECENT LOG (last 14 days):\n")
		cutoff := now.AddDate(0, 0, -14).Format("2006-01-02")
		for _, log := range logs {
			if log.Date >= cutoff {
				completed := "(none)"
				if len(log.Completed) > 0 {
					completed = strings.Join(log.Completed, ", ")
				}
				sb.WriteString(fmt.Sprintf("  %s: %s\n", log.Date, completed))
			}
		}

		var systemPrompt string
		if ai.IsSmallModel() {
			systemPrompt = "Analyze the user's habit data. Format as:\n" +
				"## Habit Health Report\n" +
				"### Strong Habits\n- habit: why it works\n" +
				"### Struggling Habits\n- habit: what's wrong\n" +
				"### Patterns\nbrief observation\n" +
				"### Coach's Note\nshort honest advice"
		} else {
			systemPrompt = "You are DEEPCOVEN, a direct and honest habit coach.\n\n" +
				"Analyze the user's habit tracking data. Look for:\n" +
				"1. Consistency patterns — which habits stick, which don't\n" +
				"2. Day-of-week trends — when do they fall off?\n" +
				"3. Streak health — are streaks growing or resetting?\n" +
				"4. Missing habits — any obvious gaps in their routine?\n" +
				"5. Quick wins to build momentum\n\n" +
				"Be brutally honest. No filler. Format as:\n" +
				"## Habit Health Report\n" +
				"### Strong Habits\n- {habit}: {why it's working}\n" +
				"### Struggling Habits\n- {habit}: {what's wrong and what to do}\n" +
				"### Patterns\n{1-2 observations about when/how they complete habits}\n" +
				"### Coach's Note\n{2-3 sentences of honest, actionable advice}"
		}

		resp, err := ai.Chat(systemPrompt, sb.String())
		return habitAICoachMsg{analysis: strings.TrimSpace(resp), err: err}
	}
}

// NewHabitTracker creates a new HabitTracker overlay.
func NewHabitTracker() HabitTracker {
	return HabitTracker{
		goalExpanded: -1,
	}
}

// IsActive returns whether the habit tracker overlay is visible.
func (ht HabitTracker) IsActive() bool {
	return ht.active
}

// Open activates the overlay, loading data from the vault.
func (ht *HabitTracker) Open(vaultRoot string) {
	ht.active = true
	ht.vaultRoot = vaultRoot
	ht.tab = 0
	ht.cursor = 0
	ht.scroll = 0
	ht.inputMode = habitInputNone
	ht.inputValue = ""
	ht.goalExpanded = -1
	ht.milestoneCur = 0
	ht.confirmDelete = false
	ht.loadHabits()
	ht.loadGoals()
}

// Close hides the overlay.
func (ht *HabitTracker) Close() {
	ht.active = false
}

// SetSize updates the available dimensions.
func (ht *HabitTracker) SetSize(w, h int) {
	ht.width = w
	ht.height = h
}

// habitsDir returns the path to the Habits folder.
func (ht HabitTracker) habitsDir() string {
	return filepath.Join(ht.vaultRoot, "Habits")
}

// ensureDir creates the Habits directory if needed.
func (ht HabitTracker) ensureDir() {
	dir := ht.habitsDir()
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0o755)
	}
}

// todayStr returns today's date as YYYY-MM-DD.
func todayStr() string {
	return time.Now().Format("2006-01-02")
}

// ── Loading ──────────────────────────────────────────────────────

func (ht *HabitTracker) loadHabits() {
	ht.habits = nil
	ht.logs = nil

	data, err := os.ReadFile(filepath.Join(ht.habitsDir(), "habits.md"))
	if err != nil {
		return
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	section := "" // "habits" or "log"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Habits" {
			section = "habits"
			continue
		}
		if trimmed == "## Log" {
			section = "log"
			continue
		}
		if strings.HasPrefix(trimmed, "|") && !strings.Contains(trimmed, "---") {
			parts := strings.Split(trimmed, "|")
			// Clean parts
			var cells []string
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					cells = append(cells, p)
				}
			}
			if section == "habits" && len(cells) >= 3 {
				// Skip header row
				if cells[0] == "Habit" {
					continue
				}
				streak, _ := strconv.Atoi(strings.TrimSpace(cells[2]))
				ht.habits = append(ht.habits, habitEntry{
					Name:    strings.TrimSpace(cells[0]),
					Created: strings.TrimSpace(cells[1]),
					Streak:  streak,
				})
			}
			if section == "log" && len(cells) >= 2 {
				if cells[0] == "Date" {
					continue
				}
				var completed []string
				for _, c := range strings.Split(cells[1], ",") {
					c = strings.TrimSpace(c)
					if c != "" {
						completed = append(completed, c)
					}
				}
				ht.logs = append(ht.logs, habitLog{
					Date:      strings.TrimSpace(cells[0]),
					Completed: completed,
				})
			}
		}
	}

	// Recalculate streaks from logs
	ht.recalcStreaks()
}

func (ht *HabitTracker) recalcStreaks() {
	today := todayStr()
	for i := range ht.habits {
		streak := 0
		d, err := time.Parse("2006-01-02", today)
		if err != nil {
			continue
		}
		for {
			ds := d.Format("2006-01-02")
			found := false
			for _, log := range ht.logs {
				if log.Date == ds {
					for _, c := range log.Completed {
						if c == ht.habits[i].Name {
							found = true
							break
						}
					}
					break
				}
			}
			if !found {
				break
			}
			streak++
			d = d.AddDate(0, 0, -1)
		}
		ht.habits[i].Streak = streak
	}
}

func (ht *HabitTracker) saveHabits() {
	ht.ensureDir()

	var b strings.Builder
	b.WriteString("---\ntype: habits\n---\n# Daily Habits\n\n")
	b.WriteString("## Habits\n")
	b.WriteString("| Habit | Created | Streak |\n")
	b.WriteString("|-------|---------|--------|\n")
	for _, h := range ht.habits {
		b.WriteString(fmt.Sprintf("| %s | %s | %d |\n", h.Name, h.Created, h.Streak))
	}

	b.WriteString("\n## Log\n")
	b.WriteString("| Date | Completed |\n")
	b.WriteString("|------|-----------|\n")
	for _, log := range ht.logs {
		b.WriteString(fmt.Sprintf("| %s | %s |\n", log.Date, strings.Join(log.Completed, ", ")))
	}

	_ = os.WriteFile(filepath.Join(ht.habitsDir(), "habits.md"), []byte(b.String()), 0o644)
}

func (ht *HabitTracker) loadGoals() {
	ht.goals = nil

	data, err := os.ReadFile(filepath.Join(ht.habitsDir(), "goals.md"))
	if err != nil {
		return
	}

	content := string(data)
	lines := strings.Split(content, "\n")

	section := "" // "active" or "archived"
	var currentGoal *goalEntry

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "## Active Goals" {
			section = "active"
			continue
		}
		if trimmed == "## Archived Goals" {
			// Save current goal
			if currentGoal != nil {
				ht.goals = append(ht.goals, *currentGoal)
				currentGoal = nil
			}
			section = "archived"
			continue
		}

		if strings.HasPrefix(trimmed, "### ") {
			// Save previous goal
			if currentGoal != nil {
				ht.goals = append(ht.goals, *currentGoal)
			}
			title := strings.TrimPrefix(trimmed, "### ")
			currentGoal = &goalEntry{
				Title:    title,
				Archived: section == "archived",
			}
			continue
		}

		if currentGoal != nil {
			if strings.HasPrefix(trimmed, "- **Target:**") {
				currentGoal.TargetDate = strings.TrimSpace(strings.TrimPrefix(trimmed, "- **Target:**"))
				continue
			}
			if strings.HasPrefix(trimmed, "- **Progress:**") {
				// Computed from milestones, skip
				continue
			}
			if strings.HasPrefix(trimmed, "- [x] ") {
				currentGoal.Milestones = append(currentGoal.Milestones, milestone{
					Text: strings.TrimPrefix(trimmed, "- [x] "),
					Done: true,
				})
			} else if strings.HasPrefix(trimmed, "- [ ] ") {
				currentGoal.Milestones = append(currentGoal.Milestones, milestone{
					Text: strings.TrimPrefix(trimmed, "- [ ] "),
					Done: false,
				})
			}
		}
	}
	if currentGoal != nil {
		ht.goals = append(ht.goals, *currentGoal)
	}
}

func (ht *HabitTracker) saveGoals() {
	ht.ensureDir()

	var b strings.Builder
	b.WriteString("---\ntype: goals\n---\n# Goals\n\n")
	b.WriteString("## Active Goals\n\n")

	for _, g := range ht.goals {
		if g.Archived {
			continue
		}
		b.WriteString(fmt.Sprintf("### %s\n", g.Title))
		b.WriteString(fmt.Sprintf("- **Target:** %s\n", g.TargetDate))
		b.WriteString(fmt.Sprintf("- **Progress:** %d%%\n", g.goalProgress()))
		for _, ms := range g.Milestones {
			if ms.Done {
				b.WriteString(fmt.Sprintf("- [x] %s\n", ms.Text))
			} else {
				b.WriteString(fmt.Sprintf("- [ ] %s\n", ms.Text))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## Archived Goals\n\n")
	for _, g := range ht.goals {
		if !g.Archived {
			continue
		}
		b.WriteString(fmt.Sprintf("### %s\n", g.Title))
		b.WriteString(fmt.Sprintf("- **Target:** %s\n", g.TargetDate))
		b.WriteString(fmt.Sprintf("- **Progress:** %d%%\n", g.goalProgress()))
		for _, ms := range g.Milestones {
			if ms.Done {
				b.WriteString(fmt.Sprintf("- [x] %s\n", ms.Text))
			} else {
				b.WriteString(fmt.Sprintf("- [ ] %s\n", ms.Text))
			}
		}
		b.WriteString("\n")
	}

	_ = os.WriteFile(filepath.Join(ht.habitsDir(), "goals.md"), []byte(b.String()), 0o644)
}

// ── Helpers ──────────────────────────────────────────────────────

// isTodayCompleted checks if a habit is done for today.
func (ht HabitTracker) isTodayCompleted(habitName string) bool {
	today := todayStr()
	for _, log := range ht.logs {
		if log.Date == today {
			for _, c := range log.Completed {
				if c == habitName {
					return true
				}
			}
			return false
		}
	}
	return false
}

// toggleToday toggles a habit's completion for today.
func (ht *HabitTracker) toggleToday(habitName string) {
	today := todayStr()
	for i, log := range ht.logs {
		if log.Date == today {
			// Check if already completed
			for j, c := range log.Completed {
				if c == habitName {
					// Remove
					ht.logs[i].Completed = append(log.Completed[:j], log.Completed[j+1:]...)
					if len(ht.logs[i].Completed) == 0 {
						ht.logs = append(ht.logs[:i], ht.logs[i+1:]...)
					}
					ht.recalcStreaks()
					ht.saveHabits()
					return
				}
			}
			// Add
			ht.logs[i].Completed = append(ht.logs[i].Completed, habitName)
			ht.recalcStreaks()
			ht.saveHabits()
			return
		}
	}
	// No entry for today — create one
	ht.logs = append([]habitLog{{Date: today, Completed: []string{habitName}}}, ht.logs...)
	ht.recalcStreaks()
	ht.saveHabits()
}

// SyncHabitToTasks finds a matching "- [ ] habitName" task in today's daily
// note or jot and toggles it to "- [x] habitName". This bridges the habit
// tracker with the task system so completing a habit marks the corresponding
// task done automatically.
func (ht *HabitTracker) SyncHabitToTasks(habitName string, v *vault.Vault) {
	if v == nil {
		return
	}

	today := time.Now().Format("2006-01-02")
	// Patterns for daily notes / jots that might contain today's tasks.
	todayPatterns := []string{
		today + ".md",                           // YYYY-MM-DD.md
		"Daily/" + today + ".md",                // Daily/YYYY-MM-DD.md
		"Journal/" + today + ".md",              // Journal/YYYY-MM-DD.md
		"daily/" + today + ".md",                // daily/YYYY-MM-DD.md
		"journal/" + today + ".md",              // journal/YYYY-MM-DD.md
		"Jots/" + today + ".md",                 // Jots/YYYY-MM-DD.md
		"jots/" + today + ".md",                 // jots/YYYY-MM-DD.md
	}

	unchecked := "- [ ] " + habitName
	checked := "- [x] " + habitName

	for _, pattern := range todayPatterns {
		note, ok := v.Notes[pattern]
		if !ok || note.Content == "" {
			continue
		}

		if !strings.Contains(note.Content, unchecked) {
			continue
		}

		// Replace the first occurrence of unchecked with checked.
		newContent := strings.Replace(note.Content, unchecked, checked, 1)
		if newContent == note.Content {
			continue
		}

		// Write back to disk and update the in-memory note.
		if err := os.WriteFile(note.Path, []byte(newContent), 0o644); err == nil {
			note.Content = newContent
		}
		return // Only update the first matching note.
	}
}

// UnsyncHabitFromTasks reverts a "- [x] habitName" task back to "- [ ] habitName"
// in today's daily note when a habit completion is toggled off.
func (ht *HabitTracker) UnsyncHabitFromTasks(habitName string, v *vault.Vault) {
	if v == nil {
		return
	}

	today := time.Now().Format("2006-01-02")
	todayPatterns := []string{
		today + ".md",
		"Daily/" + today + ".md",
		"Journal/" + today + ".md",
		"daily/" + today + ".md",
		"journal/" + today + ".md",
		"Jots/" + today + ".md",
		"jots/" + today + ".md",
	}

	checked := "- [x] " + habitName
	unchecked := "- [ ] " + habitName

	for _, pattern := range todayPatterns {
		note, ok := v.Notes[pattern]
		if !ok || note.Content == "" {
			continue
		}

		if !strings.Contains(note.Content, checked) {
			continue
		}

		newContent := strings.Replace(note.Content, checked, unchecked, 1)
		if newContent == note.Content {
			continue
		}

		if err := os.WriteFile(note.Path, []byte(newContent), 0o644); err == nil {
			note.Content = newContent
		}
		return
	}
}

// streakBlocks returns a colored 7-day visual for a habit.
func (ht HabitTracker) streakBlocks(habitName string) string {
	today, _ := time.Parse("2006-01-02", todayStr())
	var blocks []string
	for i := 6; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		ds := d.Format("2006-01-02")
		done := false
		for _, log := range ht.logs {
			if log.Date == ds {
				for _, c := range log.Completed {
					if c == habitName {
						done = true
						break
					}
				}
				break
			}
		}
		if done {
			blocks = append(blocks, lipgloss.NewStyle().Foreground(green).Render("█"))
		} else {
			blocks = append(blocks, lipgloss.NewStyle().Foreground(surface1).Render("░"))
		}
	}
	return strings.Join(blocks, "")
}

// activeGoals returns non-archived goals.
func (ht HabitTracker) activeGoals() []int {
	var indices []int
	for i, g := range ht.goals {
		if !g.Archived {
			indices = append(indices, i)
		}
	}
	return indices
}

// completionRate returns the rate for a period (7 or 30 days).
func (ht HabitTracker) completionRate(days int) float64 {
	if len(ht.habits) == 0 {
		return 0
	}
	today, _ := time.Parse("2006-01-02", todayStr())
	total := 0
	done := 0
	for i := 0; i < days; i++ {
		d := today.AddDate(0, 0, -i)
		ds := d.Format("2006-01-02")
		total += len(ht.habits)
		for _, log := range ht.logs {
			if log.Date == ds {
				done += len(log.Completed)
				break
			}
		}
	}
	if total == 0 {
		return 0
	}
	return float64(done) * 100.0 / float64(total)
}

// bestDay returns the date and count of the most habits completed in a single day.
func (ht HabitTracker) bestDay() (string, int) {
	bestDate := ""
	bestCount := 0
	for _, log := range ht.logs {
		if len(log.Completed) > bestCount {
			bestCount = len(log.Completed)
			bestDate = log.Date
		}
	}
	return bestDate, bestCount
}

// longestStreak returns the longest streak for a habit.
func (ht HabitTracker) longestStreak(habitName string) int {
	// Collect all dates where this habit was completed
	var dates []string
	for _, log := range ht.logs {
		for _, c := range log.Completed {
			if c == habitName {
				dates = append(dates, log.Date)
				break
			}
		}
	}
	if len(dates) == 0 {
		return 0
	}
	sort.Strings(dates)

	longest := 1
	current := 1
	for i := 1; i < len(dates); i++ {
		prev, _ := time.Parse("2006-01-02", dates[i-1])
		curr, _ := time.Parse("2006-01-02", dates[i])
		if curr.Sub(prev).Hours() <= 24 {
			current++
			if current > longest {
				longest = current
			}
		} else {
			current = 1
		}
	}
	return longest
}

// last14DaysChart returns per-day completion counts for the last 14 days.
func (ht HabitTracker) last14DaysChart() []int {
	today, _ := time.Parse("2006-01-02", todayStr())
	counts := make([]int, 14)
	for i := 13; i >= 0; i-- {
		d := today.AddDate(0, 0, -i)
		ds := d.Format("2006-01-02")
		for _, log := range ht.logs {
			if log.Date == ds {
				counts[13-i] = len(log.Completed)
				break
			}
		}
	}
	return counts
}

// completedGoalCount returns the number of archived goals.
func (ht HabitTracker) completedGoalCount() int {
	count := 0
	for _, g := range ht.goals {
		if g.Archived {
			count++
		}
	}
	return count
}

// ── Update ───────────────────────────────────────────────────────

// Update handles messages for the habit tracker overlay.
func (ht HabitTracker) Update(msg tea.Msg) (HabitTracker, tea.Cmd) {
	if !ht.active {
		return ht, nil
	}

	switch msg := msg.(type) {
	case habitAICoachMsg:
		ht.aiPending = false
		if msg.err != nil {
			ht.coachText = "AI error: " + msg.err.Error()
			ht.showCoach = true
		} else {
			ht.coachText = msg.analysis
			ht.showCoach = true
		}
		return ht, nil
	case tea.KeyMsg:
		return ht.updateKeys(msg)
	}
	return ht, nil
}

func (ht HabitTracker) updateKeys(msg tea.KeyMsg) (HabitTracker, tea.Cmd) {
	key := msg.String()

	// Handle input mode first
	if ht.inputMode != habitInputNone {
		return ht.updateInput(msg)
	}

	// Handle delete confirmation
	if ht.confirmDelete {
		switch key {
		case "y":
			ht.confirmDelete = false
			ht.performDelete()
		case "n", "esc":
			ht.confirmDelete = false
		}
		return ht, nil
	}

	switch key {
	case "esc":
		if ht.showCoach {
			ht.showCoach = false
			ht.coachText = ""
		} else if ht.goalExpanded >= 0 && ht.tab == 1 {
			ht.goalExpanded = -1
			ht.milestoneCur = 0
		} else {
			ht.active = false
		}
		return ht, nil

	case "I":
		if !ht.aiPending && ht.ai.Provider != "local" && ht.ai.Provider != "" && len(ht.habits) > 0 {
			ht.aiPending = true
			ht.showCoach = false
			return ht, ht.aiHabitCoach()
		}

	case "tab":
		ht.tab = (ht.tab + 1) % 3
		ht.cursor = 0
		ht.scroll = 0
		ht.goalExpanded = -1
		ht.milestoneCur = 0

	case "1":
		ht.tab = 0
		ht.cursor = 0
		ht.scroll = 0
	case "2":
		ht.tab = 1
		ht.cursor = 0
		ht.scroll = 0
		ht.goalExpanded = -1
	case "3":
		ht.tab = 2
		ht.cursor = 0
		ht.scroll = 0

	case "up", "k":
		if ht.tab == 1 && ht.goalExpanded >= 0 {
			if ht.milestoneCur > 0 {
				ht.milestoneCur--
			}
		} else {
			if ht.cursor > 0 {
				ht.cursor--
			}
		}

	case "down", "j":
		if ht.tab == 1 && ht.goalExpanded >= 0 {
			g := ht.goals[ht.goalExpanded]
			if ht.milestoneCur < len(g.Milestones)-1 {
				ht.milestoneCur++
			}
		} else {
			max := ht.maxCursor()
			if ht.cursor < max-1 {
				ht.cursor++
			}
		}

	case " ", "enter":
		switch ht.tab {
		case 0: // Toggle habit
			if ht.cursor < len(ht.habits) {
				name := ht.habits[ht.cursor].Name
				wasCompleted := ht.isTodayCompleted(name)
				ht.toggleToday(name)
				// If we just completed (not uncompleted), sync to tasks
				if !wasCompleted {
					ht.SyncHabitToTasks(name, ht.vault)
				} else {
					ht.UnsyncHabitFromTasks(name, ht.vault)
				}
			}
		case 1: // Toggle milestone or expand goal
			if ht.goalExpanded >= 0 {
				g := &ht.goals[ht.goalExpanded]
				if ht.milestoneCur < len(g.Milestones) {
					g.Milestones[ht.milestoneCur].Done = !g.Milestones[ht.milestoneCur].Done
					ht.saveGoals()
				}
			} else {
				active := ht.activeGoals()
				if ht.cursor < len(active) {
					ht.goalExpanded = active[ht.cursor]
					ht.milestoneCur = 0
				}
			}
		}

	case "n":
		switch ht.tab {
		case 0:
			ht.inputMode = habitInputNewHabit
			ht.inputValue = ""
		case 1:
			ht.inputMode = habitInputNewGoalTitle
			ht.inputValue = ""
		}

	case "m":
		if ht.tab == 1 && ht.goalExpanded >= 0 {
			ht.inputMode = habitInputNewMilestone
			ht.inputValue = ""
		}

	case "d":
		switch ht.tab {
		case 0:
			if ht.cursor < len(ht.habits) {
				ht.confirmDelete = true
			}
		case 1:
			if ht.goalExpanded >= 0 {
				ht.confirmDelete = true
			} else {
				active := ht.activeGoals()
				if ht.cursor < len(active) {
					ht.goalExpanded = active[ht.cursor]
					ht.confirmDelete = true
				}
			}
		}
	}

	return ht, nil
}

func (ht HabitTracker) updateInput(msg tea.KeyMsg) (HabitTracker, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		ht.inputMode = habitInputNone
		ht.inputValue = ""
		return ht, nil

	case "enter":
		val := strings.TrimSpace(ht.inputValue)
		if val == "" {
			ht.inputMode = habitInputNone
			return ht, nil
		}

		switch ht.inputMode {
		case habitInputNewHabit:
			ht.habits = append(ht.habits, habitEntry{
				Name:    val,
				Created: todayStr(),
				Streak:  0,
			})
			ht.saveHabits()
			ht.inputMode = habitInputNone
			ht.inputValue = ""

		case habitInputNewGoalTitle:
			// Move to date input
			ht.inputMode = habitInputNewGoalDate
			// Store title temporarily in inputValue with a prefix
			ht.inputValue = val + "|"
			return ht, nil

		case habitInputNewGoalDate:
			// inputValue has "title|date_being_typed"
			parts := strings.SplitN(ht.inputValue, "|", 2)
			if len(parts) < 2 {
				ht.inputMode = habitInputNone
				ht.inputValue = ""
				return ht, nil
			}
			title := parts[0]
			// The user pressed enter, val is the full inputValue; extract date part
			dateStr := strings.TrimSpace(strings.TrimPrefix(ht.inputValue, title+"|"))
			if dateStr == "" {
				dateStr = todayStr()
			}
			ht.goals = append(ht.goals, goalEntry{
				Title:      title,
				TargetDate: dateStr,
			})
			ht.saveGoals()
			ht.inputMode = habitInputNone
			ht.inputValue = ""

		case habitInputNewMilestone:
			if ht.goalExpanded >= 0 && ht.goalExpanded < len(ht.goals) {
				ht.goals[ht.goalExpanded].Milestones = append(ht.goals[ht.goalExpanded].Milestones, milestone{
					Text: val,
					Done: false,
				})
				ht.saveGoals()
			}
			ht.inputMode = habitInputNone
			ht.inputValue = ""
		}

		return ht, nil

	case "backspace":
		if ht.inputMode == habitInputNewGoalDate {
			// Only delete from after the pipe
			parts := strings.SplitN(ht.inputValue, "|", 2)
			if len(parts) == 2 && len(parts[1]) > 0 {
				ht.inputValue = parts[0] + "|" + parts[1][:len(parts[1])-1]
			}
		} else if len(ht.inputValue) > 0 {
			ht.inputValue = ht.inputValue[:len(ht.inputValue)-1]
		}
		return ht, nil

	default:
		if len(key) == 1 {
			if ht.inputMode == habitInputNewGoalDate {
				ht.inputValue += key
			} else {
				ht.inputValue += key
			}
		} else if key == "space" {
			if ht.inputMode == habitInputNewGoalDate {
				ht.inputValue += " "
			} else {
				ht.inputValue += " "
			}
		}
		return ht, nil
	}
}

func (ht *HabitTracker) performDelete() {
	switch ht.tab {
	case 0: // Delete habit
		if ht.cursor < len(ht.habits) {
			ht.habits = append(ht.habits[:ht.cursor], ht.habits[ht.cursor+1:]...)
			if ht.cursor >= len(ht.habits) && ht.cursor > 0 {
				ht.cursor--
			}
			ht.saveHabits()
		}
	case 1: // Archive goal
		if ht.goalExpanded >= 0 && ht.goalExpanded < len(ht.goals) {
			ht.goals[ht.goalExpanded].Archived = true
			ht.goalExpanded = -1
			ht.milestoneCur = 0
			ht.saveGoals()
		}
	}
}

func (ht HabitTracker) maxCursor() int {
	switch ht.tab {
	case 0:
		return len(ht.habits)
	case 1:
		return len(ht.activeGoals())
	case 2:
		return 0
	}
	return 0
}

// ── View ─────────────────────────────────────────────────────────

// View renders the habit tracker overlay.
func (ht HabitTracker) View() string {
	width := ht.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 6

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Habit & Goal Tracker")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	// Tab bar
	b.WriteString(ht.renderTabBar())
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	// Content based on active tab
	if ht.showCoach {
		ht.renderCoach(&b, innerW)
	} else {
		switch ht.tab {
		case 0:
			b.WriteString(ht.viewHabits(innerW))
		case 1:
			b.WriteString(ht.viewGoals(innerW))
		case 2:
			b.WriteString(ht.viewStats(innerW))
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(ht.renderHelp())

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (ht HabitTracker) renderTabBar() string {
	tabs := []string{"Habits", "Goals", "Stats"}
	var rendered []string

	for i, t := range tabs {
		label := fmt.Sprintf(" %s ", t)
		if i == ht.tab {
			style := lipgloss.NewStyle().
				Foreground(crust).
				Background(green).
				Bold(true)
			rendered = append(rendered, style.Render(label))
		} else {
			style := lipgloss.NewStyle().
				Foreground(text).
				Background(surface0)
			rendered = append(rendered, style.Render(label))
		}
	}

	return "  " + strings.Join(rendered, DimStyle.Render(" "))
}

func (ht HabitTracker) viewHabits(innerW int) string {
	var lines []string

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	streakStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	lines = append(lines, sectionStyle.Render("  "+IconCalendarChar+" Today: "+todayStr()))
	lines = append(lines, "")

	if len(ht.habits) == 0 {
		lines = append(lines, DimStyle.Render("  No habits tracked yet. Press 'n' to add one."))
	}

	for i, h := range ht.habits {
		checked := ht.isTodayCompleted(h.Name)
		checkbox := lipgloss.NewStyle().Foreground(yellow).Render("[ ]")
		if checked {
                        checkbox = lipgloss.NewStyle().Foreground(green).Bold(true).Render("[✓]")
                }

                nameW := innerW - 40
                if nameW < 10 {
                        nameW = 10
                }
                name := TruncateDisplay(h.Name, nameW)

                cursor := "  "
                nameStyle := labelStyle
                if i == ht.cursor {
                        cursor = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▶ ")
                        nameStyle = lipgloss.NewStyle().Foreground(peach).Bold(true).Underline(true)
                }

                streak := ht.streakBlocks(h.Name)
                streakNum := streakStyle.Render(fmt.Sprintf(" %d🔥", h.Streak))
		line := cursor + checkbox + " " + nameStyle.Render(PadRight(name, nameW)) + " " + streak + streakNum
		lines = append(lines, line)
	}

	// Delete confirmation
	if ht.confirmDelete && ht.tab == 0 {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(red).Bold(true).Render("  Delete this habit? (y/n)"))
	}

	// Input mode
	if ht.inputMode == habitInputNewHabit {
		lines = append(lines, "")
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  New habit: ")
		lines = append(lines, prompt+lipgloss.NewStyle().Foreground(text).Render(ht.inputValue+"_"))
	}

	return strings.Join(lines, "\n")
}

func (ht HabitTracker) viewGoals(innerW int) string {
	var lines []string

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	dateStyle := lipgloss.NewStyle().Foreground(teal)

	active := ht.activeGoals()

	if len(active) == 0 {
		lines = append(lines, DimStyle.Render("  No active goals. Press 'n' to create one."))
	}

	for ci, gi := range active {
		g := ht.goals[gi]
		cursor := "  "
		nameStyle := sectionStyle
		if ci == ht.cursor && ht.goalExpanded < 0 {
			cursor = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▶ ")
			nameStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
		}

		prog := g.goalProgress()
		barW := innerW / 4
		if barW < 10 {
			barW = 10
		}
		bar := habitProgressBar(prog, barW)

		lines = append(lines, cursor+nameStyle.Render(g.Title))
		lines = append(lines, fmt.Sprintf("    %s  %s %s",
			dateStyle.Render("Target: "+g.TargetDate),
			bar,
			lipgloss.NewStyle().Foreground(peach).Bold(true).Render(fmt.Sprintf("%d%%", prog))))
		// Show milestones if expanded
		if ht.goalExpanded == gi {
			for mi, ms := range g.Milestones {
				check := lipgloss.NewStyle().Foreground(overlay1).Render("○")
				msStyle := labelStyle
				if ms.Done {
					check = lipgloss.NewStyle().Foreground(green).Bold(true).Render("●")
					msStyle = lipgloss.NewStyle().Foreground(surface1).Strikethrough(true)
				}
				treePrefix := lipgloss.NewStyle().Foreground(surface2).Render("    ├─")
				if mi == len(g.Milestones)-1 {
					treePrefix = lipgloss.NewStyle().Foreground(surface2).Render("    └─")
				}
				msCursor := "  "
				msStyle = labelStyle
				if mi == ht.milestoneCur {
					msCursor = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▶ ")
					if ms.Done {
						msStyle = lipgloss.NewStyle().Foreground(peach).Strikethrough(true)
					} else {
						msStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
					}
				}
				lines = append(lines, treePrefix+" "+msCursor+check+" "+msStyle.Render(ms.Text))
			}
		}
		lines = append(lines, "")
	}

	// Delete confirmation
	if ht.confirmDelete && ht.tab == 1 {
		lines = append(lines, lipgloss.NewStyle().Foreground(red).Bold(true).Render("  Archive this goal? (y/n)"))
	}

	// Input modes
	if ht.inputMode == habitInputNewGoalTitle {
		lines = append(lines, "")
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Goal title: ")
		lines = append(lines, prompt+lipgloss.NewStyle().Foreground(text).Render(ht.inputValue+"_"))
	}
	if ht.inputMode == habitInputNewGoalDate {
		parts := strings.SplitN(ht.inputValue, "|", 2)
		dateVal := ""
		if len(parts) == 2 {
			dateVal = parts[1]
		}
		lines = append(lines, "")
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Target date (YYYY-MM-DD): ")
		lines = append(lines, prompt+lipgloss.NewStyle().Foreground(text).Render(dateVal+"_"))
	}
	if ht.inputMode == habitInputNewMilestone {
		lines = append(lines, "")
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  New milestone: ")
		lines = append(lines, prompt+lipgloss.NewStyle().Foreground(text).Render(ht.inputValue+"_"))
	}

	return strings.Join(lines, "\n")
}

func (ht HabitTracker) viewStats(innerW int) string {
	var lines []string

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	barStyle := lipgloss.NewStyle().Foreground(mauve)

	// Overview
	lines = append(lines, sectionStyle.Render("  Overview"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, labelStyle.Render("  Total Habits:      ")+numStyle.Render(strconv.Itoa(len(ht.habits))))
	lines = append(lines, labelStyle.Render("  Total Goals:       ")+numStyle.Render(strconv.Itoa(len(ht.goals))))
	lines = append(lines, labelStyle.Render("  Completed Goals:   ")+numStyle.Render(strconv.Itoa(ht.completedGoalCount())))

	weekRate := ht.completionRate(7)
	monthRate := ht.completionRate(30)
	lines = append(lines, labelStyle.Render("  This Week:         ")+numStyle.Render(fmt.Sprintf("%.0f%%", weekRate)))
	lines = append(lines, labelStyle.Render("  This Month:        ")+numStyle.Render(fmt.Sprintf("%.0f%%", monthRate)))

	bestDate, bestCount := ht.bestDay()
	if bestDate != "" {
		lines = append(lines, labelStyle.Render("  Best Day:          ")+numStyle.Render(bestDate)+" "+DimStyle.Render(fmt.Sprintf("(%d habits)", bestCount)))
	}

	lines = append(lines, "")

	// Streak details
	if len(ht.habits) > 0 {
		lines = append(lines, sectionStyle.Render("  Streaks"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		for _, h := range ht.habits {
			current := h.Streak
			longest := ht.longestStreak(h.Name)
			name := TruncateDisplay(h.Name, 20)
			lines = append(lines, "  "+labelStyle.Render(PadRight(name, 22))+
				lipgloss.NewStyle().Foreground(green).Bold(true).Render(fmt.Sprintf("current: %d", current))+"  "+
				DimStyle.Render(fmt.Sprintf("longest: %d", longest)))
		}
		lines = append(lines, "")
	}

	// Goals progress
	active := ht.activeGoals()
	if len(active) > 0 {
		lines = append(lines, sectionStyle.Render("  Goals Progress"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		for _, gi := range active {
			g := ht.goals[gi]
			prog := g.goalProgress()
			barW := innerW / 4
			if barW < 10 {
				barW = 10
			}
			bar := habitProgressBar(prog, barW)
			name := TruncateDisplay(g.Title, 20)
			lines = append(lines, "  "+labelStyle.Render(PadRight(name, 22))+bar+" "+numStyle.Render(fmt.Sprintf("%d%%", prog)))
		}
		lines = append(lines, "")
	}

	// 14-day bar chart
	counts := ht.last14DaysChart()
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	if maxCount > 0 {
		lines = append(lines, sectionStyle.Render("  Last 14 Days"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))

		chartH := 6
		for row := chartH; row >= 1; row-- {
			line := "  "
			for _, c := range counts {
				threshold := c * chartH / maxCount
				if threshold >= row {
					line += barStyle.Render("██") + " "
				} else {
					line += "   "
				}
			}
			lines = append(lines, line)
		}
		// Date labels (just day numbers)
		today, _ := time.Parse("2006-01-02", todayStr())
		dateLine := "  "
		for i := 13; i >= 0; i-- {
			d := today.AddDate(0, 0, -i)
			dateLine += DimStyle.Render(fmt.Sprintf("%2d", d.Day())) + " "
		}
		lines = append(lines, dateLine)
	}

	return strings.Join(lines, "\n")
}

func (ht HabitTracker) renderHelp() string {
	var pairs []struct{ Key, Desc string }
	switch ht.tab {
	case 0:
		pairs = []struct{ Key, Desc string }{
			{"Tab/1-3", "switch"}, {"j/k", "move"}, {"Space", "toggle"},
			{"n", "new"}, {"d", "delete"}, {"Esc", "close"},
		}
	case 1:
		if ht.goalExpanded >= 0 {
			pairs = []struct{ Key, Desc string }{
				{"Space", "toggle"}, {"m", "milestone"}, {"d", "archive"}, {"Esc", "back"},
			}
		} else {
			pairs = []struct{ Key, Desc string }{
				{"Tab/1-3", "switch"}, {"j/k", "move"}, {"Enter", "expand"},
				{"n", "new"}, {"d", "archive"}, {"Esc", "close"},
			}
		}
	case 2:
		pairs = []struct{ Key, Desc string }{
			{"Tab/1-3", "switch"}, {"Esc", "close"},
		}
	}
	pairs = append(pairs, struct{ Key, Desc string }{"I", "AI coach"})
	return RenderHelpBar(pairs)
}

func (ht HabitTracker) renderCoach(b *strings.Builder, w int) {
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)
	headingStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)

	b.WriteString(headerStyle.Render("  "+IconBotChar+" AI Habit Coach") + "\n\n")

	for _, line := range strings.Split(ht.coachText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			b.WriteString("\n")
			continue
		}
		if strings.HasPrefix(trimmed, "##") {
			heading := strings.TrimLeft(trimmed, "# ")
			b.WriteString("  " + headingStyle.Render(heading) + "\n")
		} else {
			b.WriteString("  " + bodyStyle.Render(TruncateDisplay(trimmed, w-6)) + "\n")
		}
	}
	b.WriteString("\n")
	b.WriteString("  " + DimStyle.Render("Esc to dismiss"))
}

// habitProgressBar renders a progress bar with block characters.
func habitProgressBar(pct int, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := pct * width / 100
	empty := width - filled
	bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("░", empty))
	return bar
}

