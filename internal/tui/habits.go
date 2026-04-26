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
	habitInputBulk         // '+' bulk-add: comma- or newline-separated names
	habitInputHelp         // '?' full keyboard reference overlay
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

	// Frequency target per habit (cadence the streak math
	// honours). Values: "daily", "weekdays", "weekends",
	// "3x-week". Stored in .granit/habits-frequency.json keyed
	// by habit name. Habits with no entry default to "daily"
	// (current behaviour). 'f' cycles the cursor habit
	// through the presets.
	frequencies map[string]string

	// Time-of-day reminder per habit. Values: "morning",
	// "midday", "afternoon", "evening", or HH:MM. Stored in
	// .granit/habits-times.json. Display: chip in the row;
	// habits whose time matches the current period get a
	// "DUE NOW" badge so the dashboard can surface them.
	times map[string]string

	// Filter by category (analogous to TaskManager's #tag).
	// 'T' cycles through every category in use; empty means
	// no filter active.
	categoryFilter string

	// Per-habit AI insight result, keyed by habit name. Set
	// when the user presses 'i' on a habit; cleared on next
	// render after they navigate away or open the coach
	// (which uses showCoach for the global view).
	insightHabit string
	insightText  string
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

// habitAIInsightMsg carries a per-habit AI insight (the user
// asked specifically about one habit via the 'i' key).
type habitAIInsightMsg struct {
	habit    string
	insight  string
	err      error
}

// aiHabitInsight asks the LLM about a single habit's last-30-day
// log. Scoping the prompt to one habit makes the response
// actionable ("you skipped on Mondays for 3 weeks") instead of
// the holistic coach's broader observations.
func (ht *HabitTracker) aiHabitInsight(habit string) tea.Cmd {
	ai := ht.ai
	logs := make([]habitLog, 0, len(ht.logs))
	cutoff := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	for _, l := range ht.logs {
		if l.Date >= cutoff {
			completed := false
			for _, c := range l.Completed {
				if c == habit {
					completed = true
					break
				}
			}
			logs = append(logs, habitLog{Date: l.Date, Completed: []string{
				func() string {
					if completed {
						return habit
					}
					return ""
				}(),
			}})
		}
	}
	freq := ht.frequencies[habit]
	if freq == "" {
		freq = "daily"
	}
	cat := ht.categories[habit]
	t := ht.times[habit]
	curStreak := ht.streakFor(habit, todayStr())
	maxStreak := ht.longestStreak(habit)

	return func() tea.Msg {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Habit: %s\n", habit))
		sb.WriteString(fmt.Sprintf("Frequency: %s\n", freq))
		if cat != "" {
			sb.WriteString(fmt.Sprintf("Category: %s\n", cat))
		}
		if t != "" {
			sb.WriteString(fmt.Sprintf("Time of day: %s\n", t))
		}
		sb.WriteString(fmt.Sprintf("Current streak: %d, longest: %d\n\n", curStreak, maxStreak))
		sb.WriteString("Last 30 days (✓=done, ·=skip):\n")
		for _, l := range logs {
			mark := "·"
			if len(l.Completed) > 0 && l.Completed[0] != "" {
				mark = "✓"
			}
			sb.WriteString(fmt.Sprintf("  %s %s\n", l.Date, mark))
		}
		systemPrompt := "You are a habit coach. The user is asking about ONE specific habit. " +
			"Look at the day-of-week patterns (when do they skip?), the streak trajectory " +
			"(growing or resetting?), and propose ONE concrete change. Be brief — 4-6 lines max. " +
			"No bullet lists; write as you'd speak."
		resp, err := ai.Chat(systemPrompt, sb.String())
		return habitAIInsightMsg{habit: habit, insight: strings.TrimSpace(resp), err: err}
	}
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
	ht.categoryFilter = ""
	ht.insightHabit = ""
	ht.insightText = ""
	ht.loadHabits()
	ht.loadGoals()
	ht.loadArchived()
	ht.loadCategories()
	ht.loadNotes()
	ht.loadFrequencies()
	ht.loadTimes()
}

// loadFrequencies restores per-habit cadence presets from disk.
func (ht *HabitTracker) loadFrequencies() {
	ht.frequencies = make(map[string]string)
	if ht.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(ht.vaultRoot, ".granit", "habits-frequency.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ht.frequencies)
	if ht.frequencies == nil {
		ht.frequencies = make(map[string]string)
	}
}

// saveFrequencies persists the per-habit cadence map.
func (ht *HabitTracker) saveFrequencies() {
	if ht.vaultRoot == "" {
		return
	}
	dir := filepath.Join(ht.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(ht.frequencies, "", "  ")
	if err != nil {
		return
	}
	if err := atomicWriteNote(filepath.Join(dir, "habits-frequency.json"), string(data)); err != nil {
		ht.lastSaveErr = err
	}
}

// loadTimes restores per-habit time-of-day reminders.
func (ht *HabitTracker) loadTimes() {
	ht.times = make(map[string]string)
	if ht.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(ht.vaultRoot, ".granit", "habits-times.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ht.times)
	if ht.times == nil {
		ht.times = make(map[string]string)
	}
}

// saveTimes persists the per-habit time map.
func (ht *HabitTracker) saveTimes() {
	if ht.vaultRoot == "" {
		return
	}
	dir := filepath.Join(ht.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0o755)
	data, err := json.MarshalIndent(ht.times, "", "  ")
	if err != nil {
		return
	}
	if err := atomicWriteNote(filepath.Join(dir, "habits-times.json"), string(data)); err != nil {
		ht.lastSaveErr = err
	}
}

// freqIsRequired reports whether a habit's frequency demands
// a check-in on the given date. Used by the streak walker so
// "weekdays" habits don't break their streak on Saturday.
// "daily" / unset always returns true.
// "3x-week" returns true on every day (the streak math sums
// per week instead of per day — see streakFor).
func (ht HabitTracker) freqIsRequired(habit, date string) bool {
	freq := ht.frequencies[habit]
	if freq == "" || freq == "daily" {
		return true
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return true
	}
	wd := t.Weekday()
	switch freq {
	case "weekdays":
		return wd >= time.Monday && wd <= time.Friday
	case "weekends":
		return wd == time.Saturday || wd == time.Sunday
	case "3x-week":
		// Daily check-in not required; the streak counts in
		// week-buckets. Treat every day as not-required so the
		// daily-walk streak helper falls through; the actual
		// streak number gets recomputed in recalcStreaks via
		// the weekly path.
		return false
	}
	return true
}

// currentPeriod maps the wall-clock hour to one of the four
// time buckets used by the smart-reminder display so a habit
// with time="morning" can light up between roughly 06:00 and
// 11:00.
func currentPeriod(now time.Time) string {
	switch h := now.Hour(); {
	case h < 11:
		return "morning"
	case h < 14:
		return "midday"
	case h < 18:
		return "afternoon"
	default:
		return "evening"
	}
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
		// Category filter (T-cycle): when set, only habits in
		// that category survive. Empty filter = no narrowing.
		if ht.categoryFilter != "" && ht.categories[h.Name] != ht.categoryFilter {
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
		ht.habits[i].Streak = ht.streakFor(ht.habits[i].Name, today)
	}
}

// streakFor walks backward from `today` honouring the habit's
// frequency. Daily habits count consecutive days completed.
// Weekdays / weekends only require check-ins on those days
// (skip days don't break). 3x-week sums per ISO week and
// counts consecutive weeks meeting the target.
func (ht HabitTracker) streakFor(habit, today string) int {
	freq := ht.frequencies[habit]
	if freq == "3x-week" {
		return ht.streakWeekly(habit, today, 3)
	}
	d, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0
	}
	streak := 0
	// Allow up to ~2 years of walk-back so a high streak doesn't
	// run forever on stale data.
	for guard := 0; guard < 730; guard++ {
		ds := d.Format("2006-01-02")
		// If this date isn't a required day for the cadence,
		// skip it without breaking the streak.
		if !ht.freqIsRequired(habit, ds) {
			d = d.AddDate(0, 0, -1)
			continue
		}
		done := false
		for _, log := range ht.logs {
			if log.Date == ds {
				for _, c := range log.Completed {
					if c == habit {
						done = true
						break
					}
				}
				break
			}
		}
		if !done {
			break
		}
		streak++
		d = d.AddDate(0, 0, -1)
	}
	return streak
}

// streakWeekly counts consecutive ISO weeks ending in the
// week containing `today` where the habit was completed at
// least `target` times. Used for the "3x-week" frequency.
func (ht HabitTracker) streakWeekly(habit, today string, target int) int {
	t, err := time.Parse("2006-01-02", today)
	if err != nil {
		return 0
	}
	// Walk back week-by-week. Stop after 104 weeks (2 years).
	streak := 0
	cursor := t
	for guard := 0; guard < 104; guard++ {
		count := 0
		// Count completions in the 7 days ending at cursor.
		for offset := 0; offset < 7; offset++ {
			d := cursor.AddDate(0, 0, -offset).Format("2006-01-02")
			for _, log := range ht.logs {
				if log.Date == d {
					for _, c := range log.Completed {
						if c == habit {
							count++
							break
						}
					}
					break
				}
			}
		}
		if count < target {
			break
		}
		streak++
		cursor = cursor.AddDate(0, 0, -7)
	}
	return streak
}

// weeklySparkline returns the colorised 12-block sparkline for
// the last 12 weeks of a habit. Each cell uses an intensity
// glyph (▁▂▃▄▅▆▇█) sized by completion count, color graded
// from grey (0) → green (target). Compact (12 cells) so it
// fits in the row alongside the existing 7-day blocks.
func (ht HabitTracker) weeklySparkline(habit string) string {
	weeks := ht.weeklyCompletions(habit)
	glyphs := []string{"·", "▂", "▃", "▄", "▅", "▆", "▇", "█"}
	var b strings.Builder
	b.WriteString(" ")
	for _, count := range weeks {
		idx := count
		if idx >= len(glyphs) {
			idx = len(glyphs) - 1
		}
		var col lipgloss.Color
		switch {
		case count == 0:
			col = surface1
		case count <= 2:
			col = peach
		case count <= 4:
			col = yellow
		default:
			col = green
		}
		b.WriteString(lipgloss.NewStyle().Foreground(col).Render(glyphs[idx]))
	}
	return b.String()
}

// weeklyCompletions returns the per-week completion count for
// the last 12 weeks (oldest to newest). Used by the per-habit
// sparkline rendered in the row.
func (ht HabitTracker) weeklyCompletions(habit string) []int {
	now := time.Now()
	weeks := make([]int, 12)
	// idx 11 is the current week, 0 is 11 weeks ago.
	for w := 0; w < 12; w++ {
		weekEnd := now.AddDate(0, 0, -7*(11-w))
		count := 0
		for offset := 0; offset < 7; offset++ {
			d := weekEnd.AddDate(0, 0, -offset).Format("2006-01-02")
			for _, log := range ht.logs {
				if log.Date == d {
					for _, c := range log.Completed {
						if c == habit {
							count++
							break
						}
					}
					break
				}
			}
		}
		weeks[w] = count
	}
	return weeks
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
	case habitAIInsightMsg:
		ht.aiPending = false
		ht.insightHabit = msg.habit
		if msg.err != nil {
			ht.insightText = "AI error: " + msg.err.Error()
		} else {
			ht.insightText = msg.insight
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
		ht.tab = (ht.tab + 1) % 2
		ht.cursor = 0
		ht.scroll = 0

	case "1":
		ht.tab = 0
		ht.cursor = 0
		ht.scroll = 0
	case "2":
		ht.tab = 1
		ht.cursor = 0
		ht.scroll = 0

	case "up", "k":
		if ht.cursor > 0 {
			ht.cursor--
		}

	case "down", "j":
		max := ht.maxCursor()
		if ht.cursor < max {
			ht.cursor++
		}

	case " ", "enter":
		// Habits-only now (Stats has no interactive rows).
		if ht.tab == 0 {
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
		}

	case "n":
		// New habit only on Habits tab.
		if ht.tab == 0 {
			ht.inputMode = habitInputNewHabit
			ht.inputValue = ""
		}

	case "d":
		// Delete habit only on Habits tab.
		if ht.tab == 0 && ht.cursor < len(ht.habits) {
			ht.confirmDelete = true
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

	// Cycle frequency target: daily → weekdays → weekends →
	// 3x-week → daily. Recalc streaks afterwards because the
	// new cadence changes the math.
	case "f":
		if ht.tab == 0 {
			vis := ht.visibleHabits()
			if ht.cursor < len(vis) {
				name := vis[ht.cursor].Name
				cycle := []string{"daily", "weekdays", "weekends", "3x-week"}
				cur := ht.frequencies[name]
				if cur == "" {
					cur = "daily"
				}
				next := "daily"
				for i, c := range cycle {
					if c == cur {
						next = cycle[(i+1)%len(cycle)]
						break
					}
				}
				if next == "daily" {
					delete(ht.frequencies, name)
				} else {
					ht.frequencies[name] = next
				}
				ht.saveFrequencies()
				ht.recalcStreaks()
				ht.statusMsg = "Frequency: " + next
			}
		}

	// Cycle reminder time-of-day: morning → midday →
	// afternoon → evening → cleared. Powers the "DUE NOW"
	// chip + dashboard widget surfacing of due habits.
	case "r":
		if ht.tab == 0 {
			vis := ht.visibleHabits()
			if ht.cursor < len(vis) {
				name := vis[ht.cursor].Name
				cycle := []string{"morning", "midday", "afternoon", "evening"}
				cur := ht.times[name]
				next := "morning"
				if cur != "" {
					for i, c := range cycle {
						if c == cur {
							if i+1 < len(cycle) {
								next = cycle[i+1]
							} else {
								next = ""
							}
							break
						}
					}
				}
				if next == "" {
					delete(ht.times, name)
					ht.statusMsg = "Reminder cleared"
				} else {
					ht.times[name] = next
					ht.statusMsg = "Reminder: " + next
				}
				ht.saveTimes()
			}
		}

	// Cycle category filter through every category in use.
	// Composes with searchQuery so power users can drill down
	// "Health + 'run'" in two keystrokes.
	case "T":
		if ht.tab == 0 {
			cats := ht.uniqueCategories()
			if len(cats) == 0 {
				ht.statusMsg = "No categorised habits"
				break
			}
			next := ""
			if ht.categoryFilter == "" {
				next = cats[0]
			} else {
				for i, c := range cats {
					if c == ht.categoryFilter {
						if i+1 < len(cats) {
							next = cats[i+1]
						}
						break
					}
				}
			}
			ht.categoryFilter = next
			if next == "" {
				ht.statusMsg = "Category filter cleared"
			} else {
				ht.statusMsg = "Filter: " + next
			}
			ht.cursor = 0
		}

	// AI insight on the cursor habit — last 30 days only.
	// Uses the existing per-habit AI coach plumbing but
	// scopes the prompt to one habit so the response is
	// actionable ("you skipped X on Mondays" beats "your
	// completion rate is 73%").
	case "i":
		if ht.tab == 0 && !ht.aiPending && ht.ai.Provider != "local" && ht.ai.Provider != "" {
			vis := ht.visibleHabits()
			if ht.cursor < len(vis) {
				name := vis[ht.cursor].Name
				ht.insightHabit = name
				ht.insightText = "Asking AI about " + name + "…"
				ht.aiPending = true
				return ht, ht.aiHabitInsight(name)
			}
		}

	// '+' bulk-add habits — comma-separated names land as
	// individual habits in one keystroke. Power-user pattern
	// for setting up a starter pack: "meditate, journal, gym, read"
	// → 4 habits in one Enter. Habits tab only.
	case "+":
		if ht.tab == 0 {
			ht.inputMode = habitInputBulk
			ht.inputValue = ""
		}

	// '?' opens the comprehensive keyboard reference. Mirror
	// of TaskManager's ? for cross-surface consistency.
	case "?":
		ht.inputMode = habitInputHelp
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

	// Help overlay — any key dismisses (matches TaskManager).
	if ht.inputMode == habitInputHelp {
		ht.inputMode = habitInputNone
		return ht, nil
	}

	// Bulk-add input: comma-separated names → N habits.
	// Newline shouldn't be typeable in this single-line input,
	// so commas are the only delimiter. Whitespace trimmed.
	if ht.inputMode == habitInputBulk {
		switch key {
		case "esc":
			ht.inputMode = habitInputNone
			ht.inputValue = ""
		case "enter":
			created := 0
			for _, name := range strings.Split(ht.inputValue, ",") {
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				name = strings.ReplaceAll(name, "|", "-")
				// Skip duplicates so re-entering a starter pack
				// doesn't double-stamp the same habits.
				dup := false
				for _, h := range ht.habits {
					if h.Name == name {
						dup = true
						break
					}
				}
				if dup {
					continue
				}
				ht.habits = append(ht.habits, habitEntry{
					Name:    name,
					Created: todayStr(),
					Streak:  0,
				})
				created++
			}
			if created > 0 {
				ht.saveHabits()
				ht.statusMsg = fmt.Sprintf("Added %d habits", created)
				// Clear filter so the new habits are visible.
				ht.searchQuery = ""
				ht.categoryFilter = ""
			} else {
				ht.statusMsg = "Nothing to add (duplicates or empty input)"
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
	if ht.tab != 0 {
		return
	}
	// Delete habit — translate visible-cursor → underlying index.
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
}

func (ht HabitTracker) maxCursor() int {
	switch ht.tab {
	case 0:
		// Cursor walks the visible (filtered+sorted+archive-aware)
		// list now, not the raw ht.habits slice.
		return len(ht.visibleHabits())
	case 1:
		// Stats tab — read-only, no cursor target.
		return 0
	}
	return 0
}

// ── View ─────────────────────────────────────────────────────────

// View renders the habit tracker overlay.
func (ht HabitTracker) View() string {
	// Tab mode fills the editor pane; overlay mode keeps the
	// historical 60–100 char clamp so the centered popup stays
	// readable on wide terminals. Without the tab branch, habits
	// in tab mode rendered as a small 100-char body in a wide
	// pane with a sea of empty cells around it.
	var width int
	if ht.IsTabMode() {
		width = ht.width - 2
		if width < 60 {
			width = 60
		}
	} else {
		width = ht.width * 2 / 3
		if width < 60 {
			width = 60
		}
		if width > 100 {
			width = 100
		}
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

	// Content based on active tab — unless help overlay is up,
	// in which case it takes the full content area.
	if ht.inputMode == habitInputHelp {
		b.WriteString(ht.renderHelpOverlay(innerW))
	} else if ht.inputMode == habitInputBulk {
		// Bulk-add prompt takes the content area too so the
		// user sees what they're typing in big letters.
		promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		inputStyle := lipgloss.NewStyle().Foreground(text)
		b.WriteString("  " + promptStyle.Render("Bulk add: ") + inputStyle.Render(ht.inputValue+"█"))
		b.WriteString("\n  ")
		b.WriteString(DimStyle.Render("Comma-separated names → one habit each. Enter to commit, Esc to cancel."))
		b.WriteString("\n  ")
		b.WriteString(DimStyle.Render("Example: meditate, journal, gym, read 30min"))
	} else if ht.showCoach {
		ht.renderCoach(&b, innerW)
	} else {
		switch ht.tab {
		case 0:
			b.WriteString(ht.viewHabits(innerW))
		case 1:
			b.WriteString(ht.viewStats(innerW))
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")
	b.WriteString(ht.renderHelp())

	if ht.IsTabMode() {
		return b.String()
	}
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (ht HabitTracker) renderTabBar() string {
	// Goals tab removed — Goals lives in its own dedicated module
	// now (CmdGoalsMode / Alt+G). Tab indices: 0=Habits, 1=Stats.
	tabs := []string{"Habits", "Stats"}
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

		// Frequency chip — only show when non-default ("daily"
		// implied means every day, no chip needed).
		freqChip := ""
		if f := ht.frequencies[h.Name]; f != "" && f != "daily" {
			freqChip = " " + lipgloss.NewStyle().Foreground(crust).Background(sapphire).Padding(0, 1).Render(f)
		}

		// Time chip — current period gets a green "DUE NOW"
		// flair when the habit is set for that period AND
		// hasn't been completed today.
		timeChip := ""
		if t := ht.times[h.Name]; t != "" {
			if t == currentPeriod(time.Now()) && !ht.isCompletedOn(h.Name, todayStr()) {
				timeChip = " " + lipgloss.NewStyle().Foreground(crust).Background(green).Bold(true).Padding(0, 1).
					Render("DUE " + t)
			} else {
				timeChip = " " + lipgloss.NewStyle().Foreground(overlay1).Render("@" + t)
			}
		}

		// 12-week sparkline — last 12 weeks of completion
		// counts, each cell colored by intensity. Power users
		// can spot a falling-off-cliff trend in one glance.
		spark := ht.weeklySparkline(h.Name)

		line := cursor + checkbox + " " + noteMarker + nameStyle.Render(PadRight(name, nameW)) +
			" " + streak + curNum + longNum + spark + freqChip + timeChip
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

	// Per-habit AI insight panel — shown below the active note
	// when 'i' was pressed on a habit. Stays visible across
	// renders until the user navigates away or asks for a new
	// insight on a different habit.
	if ht.insightHabit != "" && ht.insightText != "" {
		lines = append(lines, "")
		header := lipgloss.NewStyle().Foreground(mauve).Bold(true).
			Render("  💡 Insight: " + ht.insightHabit)
		lines = append(lines, header)
		body := lipgloss.NewStyle().Foreground(lavender).Italic(true)
		for _, line := range strings.Split(ht.insightText, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			lines = append(lines, "  "+body.Render(TruncateDisplay(line, innerW-4)))
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
		lines = append(lines, "")
	}

	// 90-day heatmap (GitHub-style 7×13 grid). Each cell is a
	// day; intensity = total habits completed that day. Power
	// users spot patterns ("I do nothing on Saturdays") without
	// drilling into individual rows. The grid reads top-to-
	// bottom = Sun..Sat, left-to-right = oldest to newest.
	if len(ht.habits) > 0 {
		lines = append(lines, sectionStyle.Render("  Last 90 Days (heatmap)"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		lines = append(lines, ht.heatmap90())
	}

	return strings.Join(lines, "\n")
}

// heatmap90 renders the 7×13-cell GitHub-style heatmap. Each
// cell represents one day in the last 90 days, color graded
// by total completions. Returns the joined-lines block ready
// for direct WriteString.
func (ht HabitTracker) heatmap90() string {
	now := time.Now()
	days := 90
	// Build a date → completion-count map for fast lookup.
	counts := make(map[string]int)
	cutoff := now.AddDate(0, 0, -days).Format("2006-01-02")
	for _, log := range ht.logs {
		if log.Date >= cutoff {
			counts[log.Date] = len(log.Completed)
		}
	}
	// Find max for color grading.
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	if maxCount == 0 {
		return DimStyle.Render("  No completions in the last 90 days")
	}
	// 7 rows = days of the week (Sun row 0, Sat row 6).
	// Cols = enough to cover 90 days, ~13.
	cols := 13
	rows := 7
	cellGlyph := "■"
	style := func(count int) lipgloss.Color {
		if count == 0 {
			return surface1
		}
		ratio := float64(count) / float64(maxCount)
		switch {
		case ratio >= 0.75:
			return green
		case ratio >= 0.5:
			return yellow
		case ratio >= 0.25:
			return peach
		default:
			return overlay1
		}
	}
	// Today is the rightmost cell. Walk backward filling the
	// grid right-to-left so today lands at (todayDOW, cols-1).
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	gridDate := make([][]string, rows)
	for r := 0; r < rows; r++ {
		gridDate[r] = make([]string, cols)
	}
	d := today
	for col := cols - 1; col >= 0; col-- {
		for row := 6; row >= 0; row-- {
			if d.Before(today.AddDate(0, 0, -days)) {
				continue
			}
			gridDate[row][col] = d.Format("2006-01-02")
			d = d.AddDate(0, 0, -1)
		}
	}
	// Render row by row.
	dayLabels := []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"}
	var b strings.Builder
	for r := 0; r < rows; r++ {
		b.WriteString("  " + DimStyle.Render(dayLabels[r]) + " ")
		for c := 0; c < cols; c++ {
			date := gridDate[r][c]
			if date == "" {
				b.WriteString("  ")
				continue
			}
			b.WriteString(lipgloss.NewStyle().Foreground(style(counts[date])).Render(cellGlyph))
			b.WriteString(" ")
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderHelpOverlay returns the comprehensive keyboard
// reference shown when the user presses '?'. Mirrors
// TaskManager's overlay layout so the two surfaces feel
// uniform — sections (Showcase / Navigation / Track / Manage
// / Filter / Power-user) with key + description rows.
func (ht HabitTracker) renderHelpOverlay(w int) string {
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true).Width(12)
	descStyle := lipgloss.NewStyle().Foreground(text)
	sectionStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)

	var b strings.Builder
	b.WriteString("\n  " + titleStyle.Render("📖 Habit Tracker — Keyboard Reference") + "\n\n")

	sections := []struct {
		title string
		keys  [][2]string
	}{
		{"Showcase — common workflows", [][2]string{
			{"+ then meditate, journal, gym, read", "Bulk-add 4 habits in one keystroke"},
			{"<<<<< then space", "Walk back 5 days, then toggle yesterday's habit"},
			{"f", "Cycle frequency: daily → weekdays → weekends → 3x-week"},
			{"r", "Cycle reminder: morning → midday → afternoon → evening"},
			{"g + Health then T", "Tag habit as Health, then filter list to Health"},
			{"i", "AI insight on cursor habit (last 30 days)"},
			{"I", "AI coach on ALL habits (holistic patterns)"},
			{"Tab → Stats", "See heatmap, streaks, completion %"},
		}},
		{"Navigation", [][2]string{
			{"Tab / 1-2", "Switch tabs (Habits / Stats)"},
			{"j/k or ↓/↑", "Move cursor"},
			{"Esc", "Close tracker / dismiss overlay"},
		}},
		{"Track — daily check-ins", [][2]string{
			{"Space / Enter", "Toggle habit done for the active date"},
			{"<", "Walk active date back one day (retroactive logging)"},
			{">", "Walk active date forward one day (capped at today)"},
			{"t", "Snap active date back to today"},
			{"N", "Add / edit a per-check-in note for active date"},
		}},
		{"Manage habits", [][2]string{
			{"n", "New single habit"},
			{"+", "Bulk-add: comma-separated names → N habits"},
			{"d", "Delete habit (permanent)"},
			{"A", "Archive habit (preserves history)"},
			{"H", "Toggle archived visibility"},
			{"g", "Set / cycle category for cursor habit"},
			{"f", "Cycle frequency target (daily / weekdays / weekends / 3x-week)"},
			{"r", "Cycle reminder time-of-day (morning / midday / afternoon / evening)"},
		}},
		{"Filter & sort (sticky)", [][2]string{
			{"/", "Live search by habit name"},
			{"T", "Cycle category filter"},
			{"s", "Cycle sort (name / current streak / longest / created)"},
			{"c", "Clear all filters + show-archived"},
		}},
		{"AI", [][2]string{
			{"i", "Per-habit insight (asks about ONE habit's last 30 days)"},
			{"I", "Holistic coach (analyses every habit's patterns)"},
		}},
		{"Visual cues", [][2]string{
			{"5🔥 max 14", "Current streak (fire) + longest streak"},
			{"▂▃▄▅▆▇█", "12-week sparkline — intensity = completions per week"},
			{"DUE morning", "Green badge when current period matches habit's reminder"},
			{"@morning", "Dim chip showing reminder period (when not 'due now')"},
			{"weekdays / 3x-week", "Frequency chip (suppressed for default 'daily')"},
			{"✎", "Note marker — habit has a note for the active date"},
			{"(arch)", "Habit is archived (only shown when H toggle is on)"},
		}},
		{"Persistence (where data lives)", [][2]string{
			{"Habits/habits.md", "Habit names + log table (markdown, version-controllable)"},
			{".granit/habits-archived.json", "Archive flags"},
			{".granit/habits-categories.json", "Category assignments"},
			{".granit/habits-notes.json", "Per-check-in notes (key: habit|date)"},
			{".granit/habits-frequency.json", "Frequency targets"},
			{".granit/habits-times.json", "Time-of-day reminders"},
		}},
	}

	for _, sec := range sections {
		b.WriteString("  " + sectionStyle.Render(sec.title) + "\n")
		for _, kv := range sec.keys {
			b.WriteString("    " + keyStyle.Render(kv[0]) + descStyle.Render(kv[1]) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("  " + DimStyle.Render("Press any key to close"))
	return b.String()
}

func (ht HabitTracker) renderHelp() string {
	var pairs []struct{ Key, Desc string }
	switch ht.tab {
	case 0:
		pairs = []struct{ Key, Desc string }{
			{"?", "help"},
			{"Tab/1-2", "switch"}, {"j/k", "move"}, {"Space", "toggle"},
			{"n", "new"}, {"+", "bulk-add"}, {"d", "del"}, {"A", "archive"}, {"H", "show arch"},
			{"g", "category"}, {"T", "filter"}, {"N", "note"},
			{"f", "freq"}, {"r", "reminder"}, {"i", "AI"},
			{"<", "prev"}, {">", "next"}, {"t", "today"},
			{"s", "sort"}, {"/", "search"}, {"c", "clear"}, {"Esc", "close"},
		}
	case 1:
		pairs = []struct{ Key, Desc string }{
			{"Tab/1-2", "switch"}, {"Esc", "close"},
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

