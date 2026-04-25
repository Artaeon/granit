package tui

import (
	"encoding/json"
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
	habitInputSearch       // '/' fuzzy filter on habit name
	habitInputCategory     // 'g' set / type new category for cursor habit
	habitInputNote         // 'N' add a note for cursor habit on activeDate
)

// habitSortMode controls habit list ordering.
type habitSortMode int

const (
	habitSortName    habitSortMode = iota // alphabetical
	habitSortCurrent                      // current streak desc
	habitSortLongest                      // longest streak desc
	habitSortCreated                      // oldest first
)

func (m habitSortMode) String() string {
	switch m {
	case habitSortCurrent:
		return "current"
	case habitSortLongest:
		return "longest"
	case habitSortCreated:
		return "created"
	}
	return "name"
}

// HabitTracker is an overlay for tracking daily habits and goals.
type HabitTracker struct {
	OverlayBase

	vaultRoot       string
	dailyNotesFolder string // from config, e.g. "Jots"
	tab             int    // 0=habits, 1=goals, 2=stats
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

	// lastSaveErr stores the most recent persistence error so the host
	// Model can surface it via reportError. Consumed once.
	lastSaveErr error

	// Power-user features. activeDate is the date the cursor's
	// space/enter toggles for — defaults to today, '<'/'>' walk
	// it, 't' returns to today. Lets users log retroactively
	// without faking timestamps. searchQuery + sortMode +
	// archived map narrow + reorder the visible list. Archived
	// habits stay in habits.md but render only when showArchived
	// is true; persistence is in .granit/habits-archived.json so
	// we don't need to alter the markdown table format.
	activeDate    string
	sortMode      habitSortMode
	searchQuery   string
	archived      map[string]bool // habit name → archived
	showArchived  bool
	statusMsg     string

	// Categories: habit name → category string. Persisted as
	// JSON sidecar so we don't have to extend the markdown
	// table schema. When categories exist, the list view
	// renders per-category section headers; uncategorised
	// habits land under "Other".
	categories map[string]string

	// Per-check-in notes: keyed by "habit|date" so a habit can
	// carry a different note for each day. ✎ dot in the row
	// signals "this habit has a note for the active date" so
	// power users can scan and see which logs have context.
	notes map[string]string
}

// ConsumeSaveError returns and clears the last persistence error, if any.
// The host Model calls this after each Update and routes non-nil errors
// through reportError so the user sees that habit/goal data didn't save.
func (ht *HabitTracker) ConsumeSaveError() error {
	if ht == nil {
		return nil
	}
	err := ht.lastSaveErr
	ht.lastSaveErr = nil
	return err
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
			systemPrompt = DeepCovenSystem("habit coach",
				"Analyze the user's habit tracking data. Look for:\n"+
					"1. Consistency patterns — which habits stick, which don't\n"+
					"2. Day-of-week trends — when do they fall off?\n"+
					"3. Streak health — are streaks growing or resetting?\n"+
					"4. Missing habits — any obvious gaps in their routine?\n"+
					"5. Quick wins to build momentum\n\n"+
					"Be brutally honest. No filler. Format as:\n"+
					"## Habit Health Report\n"+
					"### Strong Habits\n- {habit}: {why it's working}\n"+
					"### Struggling Habits\n- {habit}: {what's wrong and what to do}\n"+
					"### Patterns\n{1-2 observations about when/how they complete habits}\n"+
					"### Coach's Note\n{2-3 sentences of honest, actionable advice}")
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

// Open activates the overlay, loading data from the vault.
func (ht *HabitTracker) Open(vaultRoot string) {
	ht.Activate()
	ht.vaultRoot = vaultRoot
	ht.tab = 0
	ht.cursor = 0
	ht.scroll = 0
	ht.inputMode = habitInputNone
	ht.inputValue = ""
	ht.goalExpanded = -1
	ht.milestoneCur = 0
	ht.confirmDelete = false
	ht.activeDate = todayStr()
	ht.searchQuery = ""
	ht.statusMsg = ""
	ht.loadHabits()
	ht.loadGoals()
	ht.loadArchived()
	ht.loadCategories()
	ht.loadNotes()
}

// loadCategories restores the habit-name → category map from
// .granit/habits-categories.json. Silent on missing file.
func (ht *HabitTracker) loadCategories() {
	ht.categories = make(map[string]string)
	if ht.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(ht.vaultRoot, ".granit", "habits-categories.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ht.categories)
	if ht.categories == nil {
		ht.categories = make(map[string]string)
	}
}

// saveCategories persists the habit-name → category map.
func (ht *HabitTracker) saveCategories() {
	if ht.vaultRoot == "" {
		return
	}
	dir := filepath.Join(ht.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(ht.categories, "", "  ")
	if err != nil {
		return
	}
	if err := atomicWriteNote(filepath.Join(dir, "habits-categories.json"), string(data)); err != nil {
		ht.lastSaveErr = err
	}
}

// loadNotes restores the per-check-in note map from disk.
// Keys are "habit|YYYY-MM-DD" so notes survive even if the
// habit gets renamed (the note for the old name is orphaned —
// acceptable trade-off vs. a more complex schema).
func (ht *HabitTracker) loadNotes() {
	ht.notes = make(map[string]string)
	if ht.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(ht.vaultRoot, ".granit", "habits-notes.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ht.notes)
	if ht.notes == nil {
		ht.notes = make(map[string]string)
	}
}

// saveNotes persists the per-check-in note map.
func (ht *HabitTracker) saveNotes() {
	if ht.vaultRoot == "" {
		return
	}
	dir := filepath.Join(ht.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(ht.notes, "", "  ")
	if err != nil {
		return
	}
	if err := atomicWriteNote(filepath.Join(dir, "habits-notes.json"), string(data)); err != nil {
		ht.lastSaveErr = err
	}
}

// noteKey is the stable composite key for the per-check-in note
// map: "<habit name>|<YYYY-MM-DD>". Single helper so the format
// only lives in one place.
func noteKey(habit, date string) string { return habit + "|" + date }

// uniqueCategories returns the sorted unique list of categories
// currently assigned to any habit. Used by the 'g' cycle and
// the section-header render.
func (ht HabitTracker) uniqueCategories() []string {
	seen := make(map[string]bool)
	for _, c := range ht.categories {
		if c != "" {
			seen[c] = true
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// loadArchived restores the archived-name set from disk. Silent
// on missing/malformed file — fresh users see no archived habits.
func (ht *HabitTracker) loadArchived() {
	ht.archived = make(map[string]bool)
	if ht.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(ht.vaultRoot, ".granit", "habits-archived.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ht.archived)
	if ht.archived == nil {
		ht.archived = make(map[string]bool)
	}
}

// saveArchived persists the archived-name set.
func (ht *HabitTracker) saveArchived() {
	if ht.vaultRoot == "" {
		return
	}
	dir := filepath.Join(ht.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(ht.archived, "", "  ")
	if err != nil {
		return
	}
	if err := atomicWriteNote(filepath.Join(dir, "habits-archived.json"), string(data)); err != nil {
		ht.lastSaveErr = err
	}
}

// isCompletedOn checks whether a habit was logged on a given date.
func (ht HabitTracker) isCompletedOn(habitName, date string) bool {
	for _, log := range ht.logs {
		if log.Date == date {
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

// toggleOnDate is the date-parameterised version of toggleToday.
// Powers '<' / '>' retroactive logging — same semantics, just
// targets activeDate instead of always-today.
func (ht *HabitTracker) toggleOnDate(habitName, date string) {
	for i, log := range ht.logs {
		if log.Date == date {
			for j, c := range log.Completed {
				if c == habitName {
					ht.logs[i].Completed = append(log.Completed[:j], log.Completed[j+1:]...)
					if len(ht.logs[i].Completed) == 0 {
						ht.logs = append(ht.logs[:i], ht.logs[i+1:]...)
					}
					ht.recalcStreaks()
					ht.saveHabits()
					return
				}
			}
			ht.logs[i].Completed = append(ht.logs[i].Completed, habitName)
			ht.recalcStreaks()
			ht.saveHabits()
			return
		}
	}
	ht.logs = append([]habitLog{{Date: date, Completed: []string{habitName}}}, ht.logs...)
	ht.recalcStreaks()
	ht.saveHabits()
}

// visibleHabits returns the slice of habits to render after
// applying archive visibility, search query, and sort mode.
// The view loop iterates this — not ht.habits — so cursor /
// selection stays consistent with what's drawn.
func (ht HabitTracker) visibleHabits() []habitEntry {
	q := strings.ToLower(ht.searchQuery)
	out := make([]habitEntry, 0, len(ht.habits))
	for _, h := range ht.habits {
		if !ht.showArchived && ht.archived[h.Name] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(h.Name), q) {
			continue
		}
		out = append(out, h)
	}
	switch ht.sortMode {
	case habitSortName:
		sort.SliceStable(out, func(i, j int) bool {
			return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
		})
	case habitSortCurrent:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Streak > out[j].Streak
		})
	case habitSortLongest:
		sort.SliceStable(out, func(i, j int) bool {
			return ht.longestStreak(out[i].Name) > ht.longestStreak(out[j].Name)
		})
	case habitSortCreated:
		sort.SliceStable(out, func(i, j int) bool {
			return out[i].Created < out[j].Created
		})
	}
	return out
}

// habitsDir returns the path to the Habits folder.
func (ht HabitTracker) habitsDir() string {
	return filepath.Join(ht.vaultRoot, "Habits")
}

// ensureDir creates the Habits directory if needed.
func (ht HabitTracker) ensureDir() {
	if ht.vaultRoot == "" {
		return
	}
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

	if err := atomicWriteNote(filepath.Join(ht.habitsDir(), "habits.md"), b.String()); err != nil {
		ht.lastSaveErr = err
	}
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

	if err := atomicWriteNote(filepath.Join(ht.habitsDir(), "goals.md"), b.String()); err != nil {
		ht.lastSaveErr = err
	}
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
	todayPatterns := ht.dailyNotePatterns(today)

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
		if err := atomicWriteNote(note.Path, newContent); err == nil {
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
	todayPatterns := ht.dailyNotePatterns(today)

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

		if err := atomicWriteNote(note.Path, newContent); err == nil {
			note.Content = newContent
		}
		return
	}
}

// dailyNotePatterns returns paths to search for today's daily note, prioritizing
// the configured daily notes folder if set.
func (ht HabitTracker) dailyNotePatterns(today string) []string {
	var patterns []string
	if ht.dailyNotesFolder != "" {
		patterns = append(patterns, filepath.Join(ht.dailyNotesFolder, today+".md"))
	}
	patterns = append(patterns,
		today+".md",
		"Daily/"+today+".md",
		"Journal/"+today+".md",
		"daily/"+today+".md",
		"journal/"+today+".md",
		"Jots/"+today+".md",
		"jots/"+today+".md",
	)
	return patterns
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
		// Use calendar day difference: consecutive days have exactly 1 day between them.
		if curr.Equal(prev.AddDate(0, 0, 1)) {
			current++
			if current > longest {
				longest = current
			}
		} else if !curr.Equal(prev) {
			// Different day but not consecutive — streak breaks.
			current = 1
		}
		// Same day (duplicate log entries) — no change to streak.
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
		if ht.tab == 1 && ht.goalExpanded >= 0 && ht.goalExpanded < len(ht.goals) {
			if ht.milestoneCur > 0 {
				ht.milestoneCur--
			}
		} else {
			if ht.cursor > 0 {
				ht.cursor--
			}
		}

	case "down", "j":
		if ht.tab == 1 && ht.goalExpanded >= 0 && ht.goalExpanded < len(ht.goals) {
			g := ht.goals[ht.goalExpanded]
			if len(g.Milestones) > 0 && ht.milestoneCur < len(g.Milestones)-1 {
				ht.milestoneCur++
			}
		} else {
			max := ht.maxCursor()
			if ht.cursor < max {
				ht.cursor++
			}
		}

	case " ", "enter":
		switch ht.tab {
		case 0: // Toggle habit on the activeDate (defaults to today)
			vis := ht.visibleHabits()
			if ht.cursor < len(vis) {
				name := vis[ht.cursor].Name
				if ht.activeDate == "" {
					ht.activeDate = todayStr()
				}
				wasCompleted := ht.isCompletedOn(name, ht.activeDate)
				ht.toggleOnDate(name, ht.activeDate)
				// Only sync today's toggles to the daily-note
				// task line — retroactive logs don't touch the
				// past daily note (it'd be confusing edits).
				if ht.activeDate == todayStr() {
					if !wasCompleted {
						ht.SyncHabitToTasks(name, ht.vault)
					} else {
						ht.UnsyncHabitFromTasks(name, ht.vault)
					}
				}
			}
		case 1: // Toggle milestone or expand goal
			if ht.goalExpanded >= 0 && ht.goalExpanded < len(ht.goals) {
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
		if ht.tab == 1 && ht.goalExpanded >= 0 && ht.goalExpanded < len(ht.goals) {
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
			if ht.goalExpanded >= 0 && ht.goalExpanded < len(ht.goals) {
				ht.confirmDelete = true
			} else {
				active := ht.activeGoals()
				if ht.cursor < len(active) {
					ht.goalExpanded = active[ht.cursor]
					ht.confirmDelete = true
				}
			}
		}

	// Power-user keys (habits tab only — explicit guard so they
	// don't hijack the goals/stats tabs).
	case "<":
		// Walk activeDate back one day for retroactive logging.
		if ht.tab == 0 {
			if ht.activeDate == "" {
				ht.activeDate = todayStr()
			}
			if t, err := time.Parse("2006-01-02", ht.activeDate); err == nil {
				ht.activeDate = t.AddDate(0, 0, -1).Format("2006-01-02")
				ht.statusMsg = "Active date: " + ht.activeDate
			}
		}
	case ">":
		// Walk activeDate forward one day, capped at today so we
		// never let users log a future "completion" by accident.
		if ht.tab == 0 {
			if ht.activeDate == "" {
				ht.activeDate = todayStr()
			}
			if t, err := time.Parse("2006-01-02", ht.activeDate); err == nil {
				next := t.AddDate(0, 0, 1).Format("2006-01-02")
				if next > todayStr() {
					next = todayStr()
				}
				ht.activeDate = next
				ht.statusMsg = "Active date: " + ht.activeDate
			}
		}
	case "t":
		// Snap activeDate back to today.
		if ht.tab == 0 {
			ht.activeDate = todayStr()
			ht.statusMsg = "Active date: today"
		}
	case "s":
		// Cycle sort mode: name → current → longest → created.
		if ht.tab == 0 {
			ht.sortMode = (ht.sortMode + 1) % 4
			ht.cursor = 0
			ht.statusMsg = "Sort: " + ht.sortMode.String()
		}
	case "/":
		if ht.tab == 0 {
			ht.inputMode = habitInputSearch
		}
	case "c":
		// Clear search + reset showArchived to default off.
		if ht.tab == 0 && (ht.searchQuery != "" || ht.showArchived) {
			ht.searchQuery = ""
			ht.showArchived = false
			ht.cursor = 0
			ht.statusMsg = "Filters cleared"
		}
	case "A":
		// Archive / unarchive cursor habit. Preserves history;
		// unlike 'd' which permanently deletes.
		if ht.tab == 0 {
			vis := ht.visibleHabits()
			if ht.cursor < len(vis) {
				name := vis[ht.cursor].Name
				if ht.archived[name] {
					delete(ht.archived, name)
					ht.statusMsg = "Unarchived: " + name
				} else {
					ht.archived[name] = true
					ht.statusMsg = "Archived: " + name
				}
				ht.saveArchived()
				if ht.cursor >= len(ht.visibleHabits()) {
					ht.cursor = max(0, len(ht.visibleHabits())-1)
				}
			}
		}
	case "H":
		// Toggle archived visibility — power users sometimes
		// want to revive an old habit.
		if ht.tab == 0 {
			ht.showArchived = !ht.showArchived
			ht.cursor = 0
			if ht.showArchived {
				ht.statusMsg = "Showing archived"
			} else {
				ht.statusMsg = "Hiding archived"
			}
		}
	case "g":
		// Set / cycle the cursor habit's category. If the
		// habit has no category, prompt to type one. If it
		// already has one, cycle through existing categories
		// → "" (clear). Power users with 15+ habits can group
		// by Health / Work / Learning and the list view
		// renders section headers per category.
		if ht.tab == 0 {
			vis := ht.visibleHabits()
			if ht.cursor < len(vis) {
				name := vis[ht.cursor].Name
				cats := ht.uniqueCategories()
				cur := ht.categories[name]
				if cur == "" || len(cats) == 0 {
					// No category yet — prompt for one. Lets
					// users coin new category names.
					ht.inputMode = habitInputCategory
					ht.inputValue = ""
					return ht, nil
				}
				// Cycle through existing categories then "".
				next := ""
				for i, c := range cats {
					if c == cur {
						if i+1 < len(cats) {
							next = cats[i+1]
						}
						break
					}
				}
				if next == "" {
					delete(ht.categories, name)
					ht.statusMsg = "Category cleared: " + name
				} else {
					ht.categories[name] = next
					ht.statusMsg = "Category: " + next
				}
				ht.saveCategories()
			}
		}
	case "N":
		// Add / edit a per-check-in note for the cursor habit
		// on the active date. Stored separately from the
		// completion log so we can capture context ("felt
		// great", "30min run") without bloating the streak data.
		if ht.tab == 0 {
			vis := ht.visibleHabits()
			if ht.cursor < len(vis) {
				ht.inputMode = habitInputNote
				ht.inputValue = ht.notes[noteKey(vis[ht.cursor].Name, ht.activeDate)]
			}
		}
	}

	return ht, nil
}

func (ht HabitTracker) updateInput(msg tea.KeyMsg) (HabitTracker, tea.Cmd) {
	key := msg.String()

	// Category input — typed name gets assigned to cursor habit.
	if ht.inputMode == habitInputCategory {
		switch key {
		case "esc":
			ht.inputMode = habitInputNone
			ht.inputValue = ""
		case "enter":
			cat := strings.TrimSpace(ht.inputValue)
			vis := ht.visibleHabits()
			if ht.cursor < len(vis) {
				name := vis[ht.cursor].Name
				if cat == "" {
					delete(ht.categories, name)
					ht.statusMsg = "Category cleared: " + name
				} else {
					ht.categories[name] = cat
					ht.statusMsg = "Category set: " + cat
				}
				ht.saveCategories()
			}
			ht.inputMode = habitInputNone
			ht.inputValue = ""
		case "backspace":
			if len(ht.inputValue) > 0 {
				ht.inputValue = TrimLastRune(ht.inputValue)
			}
		default:
			if len(key) == 1 {
				ht.inputValue += key
			} else if key == "space" {
				ht.inputValue += " "
			}
		}
		return ht, nil
	}

	// Note input — typed text becomes the note for the cursor
	// habit on the active date. Empty commit deletes the note.
	if ht.inputMode == habitInputNote {
		switch key {
		case "esc":
			ht.inputMode = habitInputNone
			ht.inputValue = ""
		case "enter":
			text := strings.TrimSpace(ht.inputValue)
			vis := ht.visibleHabits()
			if ht.cursor < len(vis) {
				k := noteKey(vis[ht.cursor].Name, ht.activeDate)
				if text == "" {
					delete(ht.notes, k)
					ht.statusMsg = "Note removed"
				} else {
					ht.notes[k] = text
					ht.statusMsg = "Note saved"
				}
				ht.saveNotes()
			}
			ht.inputMode = habitInputNone
			ht.inputValue = ""
		case "backspace":
			if len(ht.inputValue) > 0 {
				ht.inputValue = TrimLastRune(ht.inputValue)
			}
		default:
			if len(key) == 1 {
				ht.inputValue += key
			} else if key == "space" {
				ht.inputValue += " "
			}
		}
		return ht, nil
	}

	// Search input is its own mode — live-typing narrows the
	// habit list as the user types. Esc clears, Enter commits.
	if ht.inputMode == habitInputSearch {
		switch key {
		case "esc":
			ht.searchQuery = ""
			ht.inputMode = habitInputNone
		case "enter":
			ht.inputMode = habitInputNone
		case "backspace":
			if len(ht.searchQuery) > 0 {
				ht.searchQuery = TrimLastRune(ht.searchQuery)
			}
		default:
			if len(key) == 1 || key == "space" {
				if key == "space" {
					ht.searchQuery += " "
				} else {
					ht.searchQuery += key
				}
			}
		}
		ht.cursor = 0
		return ht, nil
	}

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
			// Strip pipe characters to prevent markdown table corruption.
			val = strings.ReplaceAll(val, "|", "-")
			ht.habits = append(ht.habits, habitEntry{
				Name:    val,
				Created: todayStr(),
				Streak:  0,
			})
			ht.saveHabits()
			// Clear filter so the new habit is immediately
			// visible — without this the user would type a name
			// that doesn't match the active filter and wonder
			// where their habit went.
			ht.searchQuery = ""
			ht.cursor = len(ht.visibleHabits()) - 1
			if ht.cursor < 0 {
				ht.cursor = 0
			}
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
			ht.inputValue = TrimLastRune(ht.inputValue)
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
	case 0: // Delete habit — translate visible-cursor → underlying index
		vis := ht.visibleHabits()
		if ht.cursor < len(vis) {
			target := vis[ht.cursor].Name
			for i, h := range ht.habits {
				if h.Name == target {
					ht.habits = append(ht.habits[:i], ht.habits[i+1:]...)
					break
				}
			}
			delete(ht.archived, target)
			ht.saveHabits()
			ht.saveArchived()
			if ht.cursor >= len(ht.visibleHabits()) && ht.cursor > 0 {
				ht.cursor--
			}
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
		// Cursor walks the visible (filtered+sorted+archive-aware)
		// list now, not the raw ht.habits slice.
		return len(ht.visibleHabits())
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
	longestStyle := lipgloss.NewStyle().Foreground(lavender)
	chipStyle := lipgloss.NewStyle().Foreground(crust).Background(sapphire).Padding(0, 1)

	// Header line: "Today: 2026-04-25" or "VIEWING: 2026-04-23 (←/→ t)"
	// when the user has walked off today via < or >.
	dateLabel := "Today: " + todayStr()
	if ht.activeDate != "" && ht.activeDate != todayStr() {
		dateLabel = "VIEWING " + ht.activeDate + "  (t = today)"
	}
	header := sectionStyle.Render("  " + IconCalendarChar + " " + dateLabel)
	// Chips: search + sort + showing archived. Always render
	// sort so power users can see the active mode at a glance.
	header += "  " + chipStyle.Render("sort:" + ht.sortMode.String())
	if ht.searchQuery != "" {
		header += "  " + chipStyle.Render("/" + ht.searchQuery)
	}
	if ht.showArchived {
		header += "  " + chipStyle.Render("+archived")
	}
	lines = append(lines, header)
	lines = append(lines, "")

	// Live search bar — render before the habit list so the user
	// sees the narrowing as they type.
	if ht.inputMode == habitInputSearch {
		promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		inputStyle := lipgloss.NewStyle().Foreground(text)
		lines = append(lines,
			"  "+promptStyle.Render("/ Search: ")+inputStyle.Render(ht.searchQuery+"█"))
		lines = append(lines, "  "+DimStyle.Render("Live filter on habit name — Enter to commit, Esc to cancel"))
		lines = append(lines, "")
	}

	visible := ht.visibleHabits()
	if len(visible) == 0 {
		if len(ht.habits) == 0 {
			lines = append(lines, DimStyle.Render("  No habits tracked yet. Press 'n' to add one."))
		} else {
			lines = append(lines, DimStyle.Render("  No habits match. Press 'c' to clear filters."))
		}
	}

	// Group rendering by category. visibleHabits is already
	// sort-mode ordered; we re-bucket by category so per-category
	// section headers appear above each group. Uncategorised
	// habits land in "Other" — only shown when there's at least
	// one categorised habit (otherwise the header would be noise).
	categorised := false
	for _, h := range visible {
		if ht.categories[h.Name] != "" {
			categorised = true
			break
		}
	}
	noteStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	categoryHeaderStyle := lipgloss.NewStyle().Foreground(sapphire).Bold(true)
	lastCat := ""

	for i, h := range visible {
		// Category section header (only when at least one
		// habit is categorised in the visible set).
		if categorised {
			cat := ht.categories[h.Name]
			if cat == "" {
				cat = "Other"
			}
			if cat != lastCat {
				if lastCat != "" {
					lines = append(lines, "")
				}
				lines = append(lines, "  "+categoryHeaderStyle.Render("· "+cat))
				lastCat = cat
			}
		}
		checked := ht.isCompletedOn(h.Name, ht.activeDate)
		checkbox := lipgloss.NewStyle().Foreground(yellow).Render("[ ]")
		if checked {
			checkbox = lipgloss.NewStyle().Foreground(green).Bold(true).Render("[✓]")
		}

		nameW := innerW - 50
		if nameW < 10 {
			nameW = 10
		}
		name := TruncateDisplay(h.Name, nameW)
		// Archived prefix so the user can tell when showArchived
		// is on — without it, an archived habit looks identical
		// to an active one.
		if ht.archived[h.Name] {
			name = "(arch) " + name
		}

		cursor := "  "
		nameStyle := labelStyle
		if i == ht.cursor {
			cursor = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▶ ")
			nameStyle = lipgloss.NewStyle().Foreground(peach).Bold(true).Underline(true)
		}
		// Note marker: ✎ when this habit has a note for the
		// active date so the user can see at a glance which
		// rows have context to read.
		noteMarker := "  "
		if ht.notes[noteKey(h.Name, ht.activeDate)] != "" {
			noteMarker = noteStyle.Render("✎ ")
		}

		streak := ht.streakBlocks(h.Name)
		curNum := streakStyle.Render(fmt.Sprintf(" %d🔥", h.Streak))
		longNum := longestStyle.Render(fmt.Sprintf(" max %d", ht.longestStreak(h.Name)))
		line := cursor + checkbox + " " + noteMarker + nameStyle.Render(PadRight(name, nameW)) + " " + streak + curNum + longNum
		lines = append(lines, line)
	}

	// Active note bar — when the cursor habit has a note for
	// the active date, render its full text below the list so
	// power users see the context without opening anything.
	if ht.cursor < len(visible) {
		k := noteKey(visible[ht.cursor].Name, ht.activeDate)
		if note := ht.notes[k]; note != "" {
			lines = append(lines, "")
			lines = append(lines, "  "+lipgloss.NewStyle().Foreground(lavender).Italic(true).Render("✎ "+note))
		}
	}

	// Category / note input bars — render below the list so
	// the user sees the live-typing prompt with the list above.
	if ht.inputMode == habitInputCategory {
		promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		inputStyle := lipgloss.NewStyle().Foreground(text)
		lines = append(lines, "")
		lines = append(lines, "  "+promptStyle.Render("Category: ")+inputStyle.Render(ht.inputValue+"█"))
		lines = append(lines, "  "+DimStyle.Render("Type a category name — Enter to save, empty to clear, Esc to cancel"))
	}
	if ht.inputMode == habitInputNote {
		promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		inputStyle := lipgloss.NewStyle().Foreground(text)
		lines = append(lines, "")
		dateLbl := ht.activeDate
		if dateLbl == todayStr() {
			dateLbl = "today"
		}
		lines = append(lines, "  "+promptStyle.Render("Note ("+dateLbl+"): ")+inputStyle.Render(ht.inputValue+"█"))
		lines = append(lines, "  "+DimStyle.Render("Enter to save, empty to remove, Esc to cancel"))
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
				if ms.Done {
					check = lipgloss.NewStyle().Foreground(green).Bold(true).Render("●")
				}
				treePrefix := lipgloss.NewStyle().Foreground(surface2).Render("    ├─")
				if mi == len(g.Milestones)-1 {
					treePrefix = lipgloss.NewStyle().Foreground(surface2).Render("    └─")
				}
				msCursor := "  "
				msStyle := labelStyle
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
			{"n", "new"}, {"d", "delete"}, {"A", "archive"}, {"H", "show arch"},
			{"g", "category"}, {"N", "note"},
			{"<", "prev day"}, {">", "next day"}, {"t", "today"},
			{"s", "sort"}, {"/", "filter"}, {"c", "clear"}, {"Esc", "close"},
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

