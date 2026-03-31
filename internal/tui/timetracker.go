package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// timeTrackerTickMsg is sent every second while a timer is running.
type timeTrackerTickMsg time.Time

// timeEntry represents a single recorded time tracking session.
type timeEntry struct {
	NotePath  string        `json:"note_path"`
	TaskText  string        `json:"task_text"`
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Pomodoros int           `json:"pomodoros"`
	Date      string        `json:"date"` // YYYY-MM-DD
}

// dayTotal holds aggregated time for a single calendar day.
type dayTotal struct {
	Date     string
	Duration time.Duration
	Pomos    int
}

// noteAggregate holds aggregated stats for a single note.
type noteAggregate struct {
	NotePath   string
	TotalTime  time.Duration
	TotalPomos int
	Sessions   int
}

// TimeTracker is an overlay that tracks time spent on notes and integrates
// with pomodoro counting. It persists data to .granit/timetracker.json.
type TimeTracker struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	entries []timeEntry

	// Navigation
	phase  int // 0=overview, 1=daily, 2=weekly, 3=note-detail
	cursor int
	scroll int

	// Active timer
	activeTimer  *timeEntry
	timerRunning bool
	timerElapsed time.Duration
	timerStart   time.Time
	currentNote  string
	currentTask  string

	// View state
	viewDate     time.Time
	todayEntries []timeEntry
	weekEntries  []timeEntry
	noteEntries  []timeEntry

	// Aggregated stats
	totalToday    time.Duration
	totalWeek     time.Duration
	pomodorosToday int
	pomodorosWeek  int

	// For note-detail phase
	detailNotePath string
}

// NewTimeTracker returns a TimeTracker in its default (inactive) state.
func NewTimeTracker() TimeTracker {
	return TimeTracker{
		viewDate: time.Now(),
	}
}

// SetSize updates the available terminal dimensions.
func (tt *TimeTracker) SetSize(w, h int) {
	tt.width = w
	tt.height = h
}

// Open activates the time tracker overlay and loads persisted data.
func (tt *TimeTracker) Open(vaultRoot string) {
	tt.active = true
	tt.vaultRoot = vaultRoot
	tt.phase = 0
	tt.cursor = 0
	tt.scroll = 0
	tt.viewDate = time.Now()
	tt.loadEntries()
	tt.todaySummary()
	tt.weekSummary()
}

// Close hides the time tracker overlay. The timer continues in the background.
func (tt *TimeTracker) Close() {
	tt.active = false
}

// TaskTimeMap returns cumulative duration per task text across all entries.
func (tt *TimeTracker) TaskTimeMap() map[string]int {
	result := make(map[string]int)
	for _, e := range tt.entries {
		if e.TaskText != "" {
			result[e.TaskText] += int(e.Duration.Minutes())
		}
	}
	return result
}

// IsActive reports whether the time tracker overlay is currently visible.
func (tt TimeTracker) IsActive() bool {
	return tt.active
}

// ----- Storage -----

// storagePath returns the full path to the timetracker.json file.
func (tt *TimeTracker) storagePath() string {
	return filepath.Join(tt.vaultRoot, ".granit", "timetracker.json")
}

// loadEntries reads persisted time entries from disk.
func (tt *TimeTracker) loadEntries() {
	tt.entries = nil
	data, err := os.ReadFile(tt.storagePath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &tt.entries)
}

// saveEntries writes all time entries to disk.
func (tt *TimeTracker) saveEntries() {
	dir := filepath.Dir(tt.storagePath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(tt.entries, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(tt.storagePath(), data, 0644)
}

// ----- Timer Control -----

// StartTimer begins tracking time for the given note and task.
func (tt *TimeTracker) StartTimer(notePath, taskText string) {
	now := time.Now()
	tt.activeTimer = &timeEntry{
		NotePath:  notePath,
		TaskText:  taskText,
		StartTime: now,
		Date:      now.Format("2006-01-02"),
	}
	tt.timerRunning = true
	tt.timerStart = now
	tt.timerElapsed = 0
	tt.currentNote = notePath
	tt.currentTask = taskText
}

// StopTimer stops the current timer and saves the completed entry.
func (tt *TimeTracker) StopTimer() {
	if !tt.timerRunning || tt.activeTimer == nil {
		return
	}
	now := time.Now()
	tt.activeTimer.EndTime = now
	tt.activeTimer.Duration = now.Sub(tt.activeTimer.StartTime)
	tt.entries = append(tt.entries, *tt.activeTimer)
	tt.saveEntries()

	tt.activeTimer = nil
	tt.timerRunning = false
	tt.timerElapsed = 0
	tt.currentNote = ""
	tt.currentTask = ""

	// Refresh aggregated views
	tt.todaySummary()
	tt.weekSummary()
}

// RecordPomodoro is called externally when a pomodoro completes. It increments
// the pomodoro count for today's entry matching the given note/task, or creates
// a new entry if none exists.
func (tt *TimeTracker) RecordPomodoro(notePath, taskText string) {
	today := time.Now().Format("2006-01-02")
	for i := len(tt.entries) - 1; i >= 0; i-- {
		e := &tt.entries[i]
		if e.Date == today && e.NotePath == notePath && e.TaskText == taskText {
			e.Pomodoros++
			tt.saveEntries()
			tt.todaySummary()
			tt.weekSummary()
			return
		}
	}
	// No matching entry today — create a stub entry for the pomodoro.
	now := time.Now()
	entry := timeEntry{
		NotePath:  notePath,
		TaskText:  taskText,
		StartTime: now,
		EndTime:   now,
		Duration:  0,
		Pomodoros: 1,
		Date:      today,
	}
	tt.entries = append(tt.entries, entry)
	tt.saveEntries()
	tt.todaySummary()
	tt.weekSummary()
}

// IsTimerRunning reports whether a timer is currently active.
func (tt TimeTracker) IsTimerRunning() bool {
	return tt.timerRunning
}

// GetTimerStatus returns the current note name and elapsed duration for
// status bar display.
func (tt TimeTracker) GetTimerStatus() (string, time.Duration) {
	if !tt.timerRunning {
		return "", 0
	}
	name := filepath.Base(tt.currentNote)
	name = strings.TrimSuffix(name, ".md")
	return name, tt.timerElapsed
}

// ----- Data Aggregation -----

// todaySummary filters entries for today and computes totals.
func (tt *TimeTracker) todaySummary() {
	today := time.Now().Format("2006-01-02")
	tt.todayEntries = nil
	tt.totalToday = 0
	tt.pomodorosToday = 0

	for _, e := range tt.entries {
		if e.Date == today {
			tt.todayEntries = append(tt.todayEntries, e)
			tt.totalToday += e.Duration
			tt.pomodorosToday += e.Pomodoros
		}
	}
}

// weekSummary filters entries for the current week (Mon-Sun) and computes totals.
func (tt *TimeTracker) weekSummary() {
	now := time.Now()
	weekday := now.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	mondayOffset := int(weekday) - int(time.Monday)
	monday := now.AddDate(0, 0, -mondayOffset)
	mondayStr := monday.Format("2006-01-02")
	sunday := monday.AddDate(0, 0, 6)
	sundayStr := sunday.Format("2006-01-02")

	tt.weekEntries = nil
	tt.totalWeek = 0
	tt.pomodorosWeek = 0

	for _, e := range tt.entries {
		if e.Date >= mondayStr && e.Date <= sundayStr {
			tt.weekEntries = append(tt.weekEntries, e)
			tt.totalWeek += e.Duration
			tt.pomodorosWeek += e.Pomodoros
		}
	}
}

// noteHistory returns all entries for a specific note path.
func (tt *TimeTracker) noteHistory(notePath string) []timeEntry {
	var result []timeEntry
	for _, e := range tt.entries {
		if e.NotePath == notePath {
			result = append(result, e)
		}
	}
	return result
}

// dailyTotals returns aggregated time per day for the last N days, ordered
// from oldest to newest.
func (tt *TimeTracker) dailyTotals(days int) []dayTotal {
	now := time.Now()
	totals := make(map[string]*dayTotal)
	var keys []string

	for i := days - 1; i >= 0; i-- {
		d := now.AddDate(0, 0, -i)
		key := d.Format("2006-01-02")
		totals[key] = &dayTotal{Date: key}
		keys = append(keys, key)
	}

	for _, e := range tt.entries {
		if dt, ok := totals[e.Date]; ok {
			dt.Duration += e.Duration
			dt.Pomos += e.Pomodoros
		}
	}

	result := make([]dayTotal, 0, len(keys))
	for _, k := range keys {
		result = append(result, *totals[k])
	}
	return result
}

// topNotesByTime returns the top N notes ranked by total time for the given
// slice of entries.
func topNotesByTime(entries []timeEntry, n int) []noteAggregate {
	agg := make(map[string]*noteAggregate)
	for _, e := range entries {
		na, ok := agg[e.NotePath]
		if !ok {
			na = &noteAggregate{NotePath: e.NotePath}
			agg[e.NotePath] = na
		}
		na.TotalTime += e.Duration
		na.TotalPomos += e.Pomodoros
		na.Sessions++
	}

	var list []noteAggregate
	for _, na := range agg {
		list = append(list, *na)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].TotalTime > list[j].TotalTime
	})
	if len(list) > n {
		list = list[:n]
	}
	return list
}

// ----- Update -----

// Update handles keyboard input and tick messages for the time tracker overlay.
func (tt TimeTracker) Update(msg tea.Msg) (TimeTracker, tea.Cmd) {
	if !tt.active && !tt.timerRunning {
		return tt, nil
	}

	switch msg := msg.(type) {
	case timeTrackerTickMsg:
		if tt.timerRunning {
			tt.timerElapsed = time.Since(tt.timerStart)
			return tt, tea.Tick(time.Second, func(t time.Time) tea.Msg {
				return timeTrackerTickMsg(t)
			})
		}
		return tt, nil

	case tea.KeyMsg:
		if !tt.active {
			return tt, nil
		}
		return tt.handleKey(msg)
	}

	return tt, nil
}

// handleKey dispatches key events based on the current phase.
func (tt TimeTracker) handleKey(msg tea.KeyMsg) (TimeTracker, tea.Cmd) {
	key := msg.String()

	switch tt.phase {
	case 0: // Overview
		return tt.handleOverviewKey(key)
	case 1: // Daily detail
		return tt.handleDailyKey(key)
	case 2: // Weekly detail
		return tt.handleWeeklyKey(key)
	case 3: // Note detail
		return tt.handleNoteDetailKey(key)
	}

	return tt, nil
}

// handleOverviewKey processes keys for the overview phase.
func (tt TimeTracker) handleOverviewKey(key string) (TimeTracker, tea.Cmd) {
	topNotes := topNotesByTime(tt.todayEntries, 5)

	switch key {
	case "q", "esc":
		tt.active = false
	case "s":
		if tt.timerRunning {
			tt.StopTimer()
		} else {
			if tt.currentNote != "" {
				tt.StartTimer(tt.currentNote, tt.currentTask)
				return tt, tea.Tick(time.Second, func(t time.Time) tea.Msg {
					return timeTrackerTickMsg(t)
				})
			}
		}
	case "1":
		tt.phase = 1
		tt.cursor = 0
		tt.scroll = 0
		tt.viewDate = time.Now()
		tt.refreshDailyView()
	case "2":
		tt.phase = 2
		tt.cursor = 0
		tt.scroll = 0
		tt.refreshWeeklyView()
	case "j", "down":
		if tt.cursor < len(topNotes)-1 {
			tt.cursor++
		}
	case "k", "up":
		if tt.cursor > 0 {
			tt.cursor--
		}
	case "enter":
		if len(topNotes) > 0 && tt.cursor < len(topNotes) {
			tt.detailNotePath = topNotes[tt.cursor].NotePath
			tt.noteEntries = tt.noteHistory(tt.detailNotePath)
			tt.phase = 3
			tt.cursor = 0
			tt.scroll = 0
		}
	}
	return tt, nil
}

// handleDailyKey processes keys for the daily detail phase.
func (tt TimeTracker) handleDailyKey(key string) (TimeTracker, tea.Cmd) {
	switch key {
	case "esc":
		tt.phase = 0
		tt.cursor = 0
		tt.scroll = 0
	case "h":
		tt.viewDate = tt.viewDate.AddDate(0, 0, -1)
		tt.cursor = 0
		tt.scroll = 0
		tt.refreshDailyView()
	case "l":
		tt.viewDate = tt.viewDate.AddDate(0, 0, 1)
		tt.cursor = 0
		tt.scroll = 0
		tt.refreshDailyView()
	case "j", "down":
		if tt.cursor < len(tt.todayEntries)-1 {
			tt.cursor++
			tt.adjustScroll()
		}
	case "k", "up":
		if tt.cursor > 0 {
			tt.cursor--
			if tt.cursor < tt.scroll {
				tt.scroll = tt.cursor
			}
		}
	case "enter":
		if len(tt.todayEntries) > 0 && tt.cursor < len(tt.todayEntries) {
			tt.detailNotePath = tt.todayEntries[tt.cursor].NotePath
			tt.noteEntries = tt.noteHistory(tt.detailNotePath)
			tt.phase = 3
			tt.cursor = 0
			tt.scroll = 0
		}
	}
	return tt, nil
}

// handleWeeklyKey processes keys for the weekly detail phase.
func (tt TimeTracker) handleWeeklyKey(key string) (TimeTracker, tea.Cmd) {
	weekNotes := topNotesByTime(tt.weekEntries, 50)

	switch key {
	case "esc":
		tt.phase = 0
		tt.cursor = 0
		tt.scroll = 0
	case "j", "down":
		if tt.cursor < len(weekNotes)-1 {
			tt.cursor++
			tt.adjustScroll()
		}
	case "k", "up":
		if tt.cursor > 0 {
			tt.cursor--
			if tt.cursor < tt.scroll {
				tt.scroll = tt.cursor
			}
		}
	case "enter":
		if len(weekNotes) > 0 && tt.cursor < len(weekNotes) {
			tt.detailNotePath = weekNotes[tt.cursor].NotePath
			tt.noteEntries = tt.noteHistory(tt.detailNotePath)
			tt.phase = 3
			tt.cursor = 0
			tt.scroll = 0
		}
	}
	return tt, nil
}

// handleNoteDetailKey processes keys for the note detail phase.
func (tt TimeTracker) handleNoteDetailKey(key string) (TimeTracker, tea.Cmd) {
	switch key {
	case "esc":
		tt.phase = 0
		tt.cursor = 0
		tt.scroll = 0
	case "j", "down":
		tt.scroll++
	case "k", "up":
		if tt.scroll > 0 {
			tt.scroll--
		}
	}
	return tt, nil
}

// adjustScroll ensures the cursor stays visible within the scrollable area.
func (tt *TimeTracker) adjustScroll() {
	visH := tt.height - 12
	if visH < 1 {
		visH = 1
	}
	if tt.cursor >= tt.scroll+visH {
		tt.scroll = tt.cursor - visH + 1
	}
}

// refreshDailyView updates todayEntries for the currently viewed date.
func (tt *TimeTracker) refreshDailyView() {
	dateStr := tt.viewDate.Format("2006-01-02")
	tt.todayEntries = nil
	tt.totalToday = 0
	tt.pomodorosToday = 0
	for _, e := range tt.entries {
		if e.Date == dateStr {
			tt.todayEntries = append(tt.todayEntries, e)
			tt.totalToday += e.Duration
			tt.pomodorosToday += e.Pomodoros
		}
	}
}

// refreshWeeklyView updates weekEntries for the week containing viewDate.
func (tt *TimeTracker) refreshWeeklyView() {
	ref := tt.viewDate
	weekday := ref.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	mondayOffset := int(weekday) - int(time.Monday)
	monday := ref.AddDate(0, 0, -mondayOffset)
	mondayStr := monday.Format("2006-01-02")
	sunday := monday.AddDate(0, 0, 6)
	sundayStr := sunday.Format("2006-01-02")

	tt.weekEntries = nil
	tt.totalWeek = 0
	tt.pomodorosWeek = 0
	for _, e := range tt.entries {
		if e.Date >= mondayStr && e.Date <= sundayStr {
			tt.weekEntries = append(tt.weekEntries, e)
			tt.totalWeek += e.Duration
			tt.pomodorosWeek += e.Pomodoros
		}
	}
}

// ----- View -----

// View renders the time tracker overlay.
func (tt TimeTracker) View() string {
	width := tt.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}

	var content string
	switch tt.phase {
	case 0:
		content = tt.viewOverview(width)
	case 1:
		content = tt.viewDaily(width)
	case 2:
		content = tt.viewWeekly(width)
	case 3:
		content = tt.viewNoteDetail(width)
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(content)
}

// viewOverview renders the overview phase (phase 0).
func (tt TimeTracker) viewOverview(width int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	barStyle := lipgloss.NewStyle().Foreground(mauve)
	dimBarStyle := lipgloss.NewStyle().Foreground(surface1)

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render(IconCalendarChar + "  Time Tracker")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	// Active timer status
	if tt.timerRunning {
		timerStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		noteName := filepath.Base(tt.currentNote)
		noteName = strings.TrimSuffix(noteName, ".md")
		elapsed := ttFormatDuration(tt.timerElapsed)
		b.WriteString("  " + timerStyle.Render("RECORDING") + "  ")
		b.WriteString(lipgloss.NewStyle().Foreground(lavender).Render(noteName))
		b.WriteString("  " + numStyle.Render(elapsed))
		if tt.currentTask != "" {
			b.WriteString("\n  " + DimStyle.Render("Task: "+tt.currentTask))
		}
		b.WriteString("\n\n")
	} else {
		b.WriteString("  " + DimStyle.Render("No active timer"))
		b.WriteString("\n\n")
	}

	// Today's summary
	b.WriteString(sectionStyle.Render("  Today"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", 30)))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Total Time:   ") + numStyle.Render(ttFormatDuration(tt.totalToday)))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Pomodoros:    ") + numStyle.Render(fmt.Sprintf("%d", tt.pomodorosToday)) + " " + renderPomoIcons(tt.pomodorosToday))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Sessions:    ") + numStyle.Render(fmt.Sprintf("%d", len(tt.todayEntries))))
	b.WriteString("\n\n")

	// Top 5 notes today
	topNotes := topNotesByTime(tt.todayEntries, 5)
	if len(topNotes) > 0 {
		b.WriteString(sectionStyle.Render("  Top Notes Today"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", 30)))
		b.WriteString("\n")

		maxTime := topNotes[0].TotalTime
		for i, na := range topNotes {
			name := filepath.Base(na.NotePath)
			name = strings.TrimSuffix(name, ".md")
			name = TruncateDisplay(name, 22)

			barLen := 0
			if maxTime > 0 {
				barLen = int(na.TotalTime) * 18 / int(maxTime)
			}
			if barLen < 1 && na.TotalTime > 0 {
				barLen = 1
			}
			emptyLen := 18 - barLen

			bar := barStyle.Render(strings.Repeat("\u2588", barLen)) + dimBarStyle.Render(strings.Repeat("\u2591", emptyLen))
			dur := formatDurationShort(na.TotalTime)

			prefix := "  "
			if i == tt.cursor {
				prefix = lipgloss.NewStyle().Foreground(green).Bold(true).Render("> ")
			}
			line := prefix + ttPadRight(name, 24) + bar + " " + numStyle.Render(dur)
			if na.TotalPomos > 0 {
				line += " " + DimStyle.Render(fmt.Sprintf("(%dp)", na.TotalPomos))
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Week summary
	b.WriteString(sectionStyle.Render("  This Week"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", 30)))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Total Time:   ") + numStyle.Render(ttFormatDuration(tt.totalWeek)))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Pomodoros:    ") + numStyle.Render(fmt.Sprintf("%d", tt.pomodorosWeek)))
	b.WriteString("\n\n")

	// Last 7 days bar chart
	dailyData := tt.dailyTotals(7)
	b.WriteString(sectionStyle.Render("  Last 7 Days"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", 30)))
	b.WriteString("\n")
	b.WriteString(tt.renderDailyChart(dailyData, width-10))
	b.WriteString("\n")

	// Help
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(tt.helpLine(0))

	return b.String()
}

// viewDaily renders the daily detail phase (phase 1).
func (tt TimeTracker) viewDaily(width int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)

	dateStr := tt.viewDate.Format("2006-01-02")
	dayName := tt.viewDate.Format("Monday")
	isToday := dateStr == time.Now().Format("2006-01-02")

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render(IconDailyChar + "  Daily Detail")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	// Date header
	dateLabel := sectionStyle.Render("  " + dayName + ", " + dateStr)
	if isToday {
		dateLabel += lipgloss.NewStyle().Foreground(green).Bold(true).Render("  (today)")
	}
	b.WriteString(dateLabel)
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Total: ") + numStyle.Render(ttFormatDuration(tt.totalToday)))
	b.WriteString("  " + labelStyle.Render("Pomodoros: ") + numStyle.Render(fmt.Sprintf("%d", tt.pomodorosToday)))
	b.WriteString("\n\n")

	if len(tt.todayEntries) == 0 {
		b.WriteString("  " + DimStyle.Render("No entries for this date"))
		b.WriteString("\n")
	} else {
		visH := tt.height - 14
		if visH < 5 {
			visH = 5
		}
		end := tt.scroll + visH
		if end > len(tt.todayEntries) {
			end = len(tt.todayEntries)
		}

		headerLine := DimStyle.Render(fmt.Sprintf("  %-24s %-20s %8s  %s", "Note", "Task", "Duration", "Pomos"))
		b.WriteString(headerLine)
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-10)))
		b.WriteString("\n")

		for i := tt.scroll; i < end; i++ {
			e := tt.todayEntries[i]
			name := filepath.Base(e.NotePath)
			name = strings.TrimSuffix(name, ".md")
			name = TruncateDisplay(name, 22)

			task := e.TaskText
			task = TruncateDisplay(task, 18)
			if task == "" {
				task = DimStyle.Render("-")
			}

			dur := formatDurationShort(e.Duration)
			pomoStr := ""
			if e.Pomodoros > 0 {
				pomoStr = fmt.Sprintf("%d", e.Pomodoros)
			}

			if i == tt.cursor {
				line := fmt.Sprintf("  %-24s %-20s %8s  %s", name, task, dur, pomoStr)
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString(fmt.Sprintf("  %-24s %-20s %8s  %s",
					lipgloss.NewStyle().Foreground(lavender).Render(name),
					DimStyle.Render(task),
					lipgloss.NewStyle().Foreground(text).Render(dur),
					lipgloss.NewStyle().Foreground(yellow).Render(pomoStr),
				))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(tt.helpLine(1))

	return b.String()
}

// viewWeekly renders the weekly detail phase (phase 2).
func (tt TimeTracker) viewWeekly(width int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	barStyle := lipgloss.NewStyle().Foreground(teal)
	dimBarStyle := lipgloss.NewStyle().Foreground(surface1)

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render(IconGraphChar + "  Weekly Summary")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	// Week stats
	b.WriteString(labelStyle.Render("  Total Time:   ") + numStyle.Render(ttFormatDuration(tt.totalWeek)))
	b.WriteString("  " + labelStyle.Render("Pomodoros: ") + numStyle.Render(fmt.Sprintf("%d", tt.pomodorosWeek)))
	b.WriteString("  " + labelStyle.Render("Sessions: ") + numStyle.Render(fmt.Sprintf("%d", len(tt.weekEntries))))
	b.WriteString("\n\n")

	// Daily chart for the week
	dailyData := tt.dailyTotals(7)
	b.WriteString(sectionStyle.Render("  Hours per Day"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", 30)))
	b.WriteString("\n")
	b.WriteString(tt.renderDailyChart(dailyData, width-10))
	b.WriteString("\n")

	// Notes by time
	weekNotes := topNotesByTime(tt.weekEntries, 50)
	if len(weekNotes) > 0 {
		b.WriteString(sectionStyle.Render("  Notes by Time"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", 30)))
		b.WriteString("\n")

		maxTime := weekNotes[0].TotalTime
		visH := tt.height - 22
		if visH < 5 {
			visH = 5
		}
		end := tt.scroll + visH
		if end > len(weekNotes) {
			end = len(weekNotes)
		}

		for i := tt.scroll; i < end; i++ {
			na := weekNotes[i]
			name := filepath.Base(na.NotePath)
			name = strings.TrimSuffix(name, ".md")
			name = TruncateDisplay(name, 22)

			barLen := 0
			if maxTime > 0 {
				barLen = int(na.TotalTime) * 18 / int(maxTime)
			}
			if barLen < 1 && na.TotalTime > 0 {
				barLen = 1
			}
			emptyLen := 18 - barLen

			bar := barStyle.Render(strings.Repeat("\u2588", barLen)) + dimBarStyle.Render(strings.Repeat("\u2591", emptyLen))
			dur := formatDurationShort(na.TotalTime)

			prefix := "  "
			if i == tt.cursor {
				prefix = lipgloss.NewStyle().Foreground(green).Bold(true).Render("> ")
			}
			line := prefix + ttPadRight(name, 24) + bar + " " + numStyle.Render(dur)
			if na.TotalPomos > 0 {
				line += " " + lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf("%dp", na.TotalPomos))
			}
			line += " " + DimStyle.Render(fmt.Sprintf("(%d sess)", na.Sessions))
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(tt.helpLine(2))

	return b.String()
}

// viewNoteDetail renders the note detail phase (phase 3).
func (tt TimeTracker) viewNoteDetail(width int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)

	noteName := filepath.Base(tt.detailNotePath)
	noteName = strings.TrimSuffix(noteName, ".md")

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render(IconDailyChar + "  Note: " + noteName)
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	// Compute note-level aggregates
	var totalTime time.Duration
	var totalPomos int
	var sessionCount int
	dates := make(map[string]bool)

	for _, e := range tt.noteEntries {
		totalTime += e.Duration
		totalPomos += e.Pomodoros
		sessionCount++
		dates[e.Date] = true
	}

	avgSession := time.Duration(0)
	if sessionCount > 0 {
		avgSession = totalTime / time.Duration(sessionCount)
	}

	// Stats
	b.WriteString(sectionStyle.Render("  Summary"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", 30)))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Total Time:       ") + numStyle.Render(ttFormatDuration(totalTime)))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Total Pomodoros:  ") + numStyle.Render(fmt.Sprintf("%d", totalPomos)) + " " + renderPomoIcons(totalPomos))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Total Sessions:   ") + numStyle.Render(fmt.Sprintf("%d", sessionCount)))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Active Days:      ") + numStyle.Render(fmt.Sprintf("%d", len(dates))))
	b.WriteString("\n")
	b.WriteString(labelStyle.Render("  Avg Session:      ") + numStyle.Render(ttFormatDuration(avgSession)))
	b.WriteString("\n\n")

	// Session list
	b.WriteString(sectionStyle.Render("  Sessions"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", 30)))
	b.WriteString("\n")

	if len(tt.noteEntries) == 0 {
		b.WriteString("  " + DimStyle.Render("No sessions recorded"))
		b.WriteString("\n")
	} else {
		// Sort sessions by date descending
		sorted := make([]timeEntry, len(tt.noteEntries))
		copy(sorted, tt.noteEntries)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].StartTime.After(sorted[j].StartTime)
		})

		var lines []string
		for _, e := range sorted {
			timeStr := e.StartTime.Format("15:04")
			endStr := ""
			if !e.EndTime.IsZero() {
				endStr = e.EndTime.Format("15:04")
			}
			dur := formatDurationShort(e.Duration)
			task := e.TaskText
			task = TruncateDisplay(task, 20)

			line := fmt.Sprintf("  %s  %s-%s  %8s",
				lipgloss.NewStyle().Foreground(sapphire).Render(e.Date),
				lipgloss.NewStyle().Foreground(text).Render(timeStr),
				lipgloss.NewStyle().Foreground(text).Render(endStr),
				numStyle.Render(dur),
			)
			if task != "" {
				line += "  " + DimStyle.Render(task)
			}
			if e.Pomodoros > 0 {
				line += "  " + lipgloss.NewStyle().Foreground(yellow).Render(fmt.Sprintf("%dp", e.Pomodoros))
			}
			lines = append(lines, line)
		}

		visH := tt.height - 20
		if visH < 5 {
			visH = 5
		}
		maxScroll := len(lines) - visH
		if maxScroll < 0 {
			maxScroll = 0
		}
		if tt.scroll > maxScroll {
			tt.scroll = maxScroll
		}
		end := tt.scroll + visH
		if end > len(lines) {
			end = len(lines)
		}

		for i := tt.scroll; i < end; i++ {
			b.WriteString(lines[i])
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(tt.helpLine(3))

	return b.String()
}

// ----- Bar Chart Rendering -----

// renderDailyChart renders a horizontal bar chart of hours per day.
func (tt TimeTracker) renderDailyChart(data []dayTotal, availWidth int) string {
	if len(data) == 0 {
		return "  " + DimStyle.Render("No data")
	}

	barStyle := lipgloss.NewStyle().Foreground(sapphire)
	dimBarStyle := lipgloss.NewStyle().Foreground(surface1)
	numStyle := lipgloss.NewStyle().Foreground(peach)
	labelWidth := 6 // "Mon  " or day abbreviation
	valueWidth := 8 // "  1h 30m"
	barMaxWidth := availWidth - labelWidth - valueWidth - 4
	if barMaxWidth < 10 {
		barMaxWidth = 10
	}

	// Find max duration for scaling
	var maxDur time.Duration
	for _, d := range data {
		if d.Duration > maxDur {
			maxDur = d.Duration
		}
	}

	var b strings.Builder
	for _, d := range data {
		// Parse date to get day abbreviation
		t, err := time.Parse("2006-01-02", d.Date)
		dayLabel := d.Date[5:]
		if err == nil {
			dayLabel = t.Format("Mon")
		}

		isToday := d.Date == time.Now().Format("2006-01-02")
		dayStyle := lipgloss.NewStyle().Foreground(overlay0)
		if isToday {
			dayStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
		}

		barLen := 0
		if maxDur > 0 {
			barLen = int(d.Duration) * barMaxWidth / int(maxDur)
		}
		if barLen < 1 && d.Duration > 0 {
			barLen = 1
		}
		emptyLen := barMaxWidth - barLen
		if emptyLen < 0 {
			emptyLen = 0
		}

		bar := barStyle.Render(strings.Repeat("\u2588", barLen)) + dimBarStyle.Render(strings.Repeat("\u2591", emptyLen))

		durLabel := ""
		if d.Duration > 0 {
			durLabel = formatDurationShort(d.Duration)
		}

		b.WriteString("  " + dayStyle.Render(ttPadRight(dayLabel, labelWidth)) + bar + " " + numStyle.Render(durLabel))
		b.WriteString("\n")
	}

	return b.String()
}

// ----- Help Lines -----

// helpLine returns the contextual help text for the given phase.
func (tt TimeTracker) helpLine(phase int) string {
	switch phase {
	case 0: // Overview
		return RenderHelpBar([]struct{ Key, Desc string }{
			{"s", "start/stop"}, {"1", "today"}, {"2", "weekly"},
			{"j/k", "navigate"}, {"Enter", "detail"}, {"q/Esc", "close"},
		})
	case 1: // Daily
		return RenderHelpBar([]struct{ Key, Desc string }{
			{"h/l", "prev/next day"}, {"j/k", "navigate"},
			{"Enter", "note detail"}, {"Esc", "back"},
		})
	case 2: // Weekly
		return RenderHelpBar([]struct{ Key, Desc string }{
			{"j/k", "navigate"}, {"Enter", "note detail"}, {"Esc", "back"},
		})
	case 3: // Note detail
		return RenderHelpBar([]struct{ Key, Desc string }{
			{"j/k", "scroll"}, {"Esc", "back"},
		})
	}
	return ""
}

// ----- Formatting Helpers -----

// ttFormatDuration renders a duration as "Xh Ym Zs" with appropriate precision.
func ttFormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// formatDurationShort renders a compact duration string.
func formatDurationShort(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60

	if h > 0 {
		if m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%ds", s)
}

// renderPomoIcons renders small tomato-like icons for pomodoro count.
func renderPomoIcons(count int) string {
	if count <= 0 {
		return ""
	}
	pomoStyle := lipgloss.NewStyle().Foreground(red)
	dimPomoStyle := lipgloss.NewStyle().Foreground(surface1)

	shown := count
	if shown > 12 {
		shown = 12
	}

	full := pomoStyle.Render(strings.Repeat("\u25cf", shown))
	if count > 12 {
		full += dimPomoStyle.Render(fmt.Sprintf(" +%d", count-12))
	}
	return full
}

// ttPadRight pads a string to the given width with spaces. Uses its own name
// to avoid collisions with the existing padRight in stats.go.
func ttPadRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
