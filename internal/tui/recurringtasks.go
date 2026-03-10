package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// RecurringTask defines a single recurring task that is automatically created
// in the vault's Tasks.md on a daily, weekly, or monthly schedule.
type RecurringTask struct {
	Text        string `json:"text"`
	Frequency   string `json:"frequency"`    // "daily", "weekly", "monthly"
	DayOfWeek   int    `json:"day_of_week"`  // 0-6 (Sun-Sat), used for weekly
	DayOfMonth  int    `json:"day_of_month"` // 1-31, used for monthly
	LastCreated string `json:"last_created"` // YYYY-MM-DD
	Enabled     bool   `json:"enabled"`
}

type recurringView int

const (
	rvList recurringView = iota
	rvAdd
	rvEdit
)

type recurringField int

const (
	rfText recurringField = iota
	rfFrequency
	rfDay
)

// RecurringTasks is the overlay component for managing recurring task rules.
type RecurringTasks struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	tasks  []RecurringTask
	cursor int
	scroll int

	view      recurringView
	editIdx   int // index being edited (rvEdit only)
	field     recurringField
	inputText string
	inputFreq int // 0=daily, 1=weekly, 2=monthly
	inputDay  int // day-of-week (0-6) or day-of-month (1-31)

	// consumed-once: how many tasks were auto-created on Open
	createdCount int
	createdReady bool
}

var freqLabels = []string{"daily", "weekly", "monthly"}

// NewRecurringTasks creates a new RecurringTasks component.
func NewRecurringTasks() RecurringTasks { return RecurringTasks{} }

// IsActive returns whether the recurring tasks overlay is visible.
func (rt RecurringTasks) IsActive() bool { return rt.active }

// SetSize updates the available dimensions for the overlay.
func (rt *RecurringTasks) SetSize(w, h int) { rt.width = w; rt.height = h }

// Open activates the overlay, loads tasks from disk, and runs CheckAndCreate.
func (rt *RecurringTasks) Open(vaultRoot string) {
	rt.active = true
	rt.vaultRoot = vaultRoot
	rt.cursor = 0
	rt.scroll = 0
	rt.view = rvList
	rt.createdReady = false
	rt.createdCount = 0
	rt.load()
	if created := rt.checkAndCreate(); created > 0 {
		rt.createdCount = created
		rt.createdReady = true
		rt.save()
	}
}

// GetCreatedCount returns how many tasks were auto-created when the overlay
// was opened. Uses a consumed-once pattern: the second call returns (0, false).
func (rt *RecurringTasks) GetCreatedCount() (int, bool) {
	if rt.createdReady {
		rt.createdReady = false
		return rt.createdCount, true
	}
	return 0, false
}

// ----- persistence -----

func (rt *RecurringTasks) configPath() string {
	return filepath.Join(rt.vaultRoot, ".granit", "recurring.json")
}

func (rt *RecurringTasks) load() {
	rt.tasks = nil
	data, err := os.ReadFile(rt.configPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &rt.tasks)
}

func (rt *RecurringTasks) save() {
	dir := filepath.Join(rt.vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(rt.tasks, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(rt.configPath(), data, 0644)
}

// ----- auto-create logic -----

// checkAndCreate iterates over all enabled tasks and creates any that are due.
func (rt *RecurringTasks) checkAndCreate() int {
	today := time.Now()
	todayStr := today.Format("2006-01-02")
	created := 0
	for i := range rt.tasks {
		task := &rt.tasks[i]
		if !task.Enabled || !rt.isDue(task, today) {
			continue
		}
		rt.appendToTasksFile(task.Text, todayStr)
		task.LastCreated = todayStr
		created++
	}
	return created
}

func (rt *RecurringTasks) isDue(task *RecurringTask, today time.Time) bool {
	todayStr := today.Format("2006-01-02")
	if task.LastCreated == todayStr {
		return false
	}
	switch task.Frequency {
	case "daily":
		return true
	case "weekly":
		return int(today.Weekday()) == task.DayOfWeek
	case "monthly":
		return today.Day() == task.DayOfMonth
	}
	return false
}

func (rt *RecurringTasks) appendToTasksFile(text, dateStr string) {
	tasksPath := filepath.Join(rt.vaultRoot, "Tasks.md")
	line := fmt.Sprintf("- [ ] %s 📅 %s\n", text, dateStr)
	if _, err := os.Stat(tasksPath); os.IsNotExist(err) {
		_ = os.WriteFile(tasksPath, []byte("# Tasks\n\n"+line), 0644)
		return
	}
	f, err := os.OpenFile(tasksPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line)
}

// ----- add / edit helpers -----

func (rt *RecurringTasks) startAdd() {
	rt.view = rvAdd
	rt.field = rfText
	rt.inputText = ""
	rt.inputFreq = 0
	rt.inputDay = 1
}

func (rt *RecurringTasks) startEdit() {
	if len(rt.tasks) == 0 || rt.cursor >= len(rt.tasks) {
		return
	}
	task := rt.tasks[rt.cursor]
	rt.view = rvEdit
	rt.editIdx = rt.cursor
	rt.field = rfText
	rt.inputText = task.Text
	rt.inputFreq = freqIndex(task.Frequency)
	switch task.Frequency {
	case "weekly":
		rt.inputDay = task.DayOfWeek
	case "monthly":
		rt.inputDay = task.DayOfMonth
	default:
		rt.inputDay = 0
	}
}

func freqIndex(freq string) int {
	for i, f := range freqLabels {
		if f == freq {
			return i
		}
	}
	return 0
}

func (rt *RecurringTasks) saveForm() {
	text := strings.TrimSpace(rt.inputText)
	if text == "" {
		return
	}
	freq := freqLabels[rt.inputFreq]
	day := rt.inputDay
	// Clamp day values.
	switch freq {
	case "weekly":
		if day < 0 {
			day = 0
		} else if day > 6 {
			day = 6
		}
	case "monthly":
		if day < 1 {
			day = 1
		} else if day > 31 {
			day = 31
		}
	}
	if rt.view == rvAdd {
		rt.tasks = append(rt.tasks, RecurringTask{
			Text: text, Frequency: freq,
			DayOfWeek: day, DayOfMonth: day, Enabled: true,
		})
		rt.cursor = len(rt.tasks) - 1
	} else if rt.view == rvEdit && rt.editIdx < len(rt.tasks) {
		rt.tasks[rt.editIdx].Text = text
		rt.tasks[rt.editIdx].Frequency = freq
		if freq == "weekly" {
			rt.tasks[rt.editIdx].DayOfWeek = day
		} else if freq == "monthly" {
			rt.tasks[rt.editIdx].DayOfMonth = day
		}
	}
	rt.save()
	rt.view = rvList
}

func (rt *RecurringTasks) deleteSelected() {
	if len(rt.tasks) == 0 || rt.cursor >= len(rt.tasks) {
		return
	}
	rt.tasks = append(rt.tasks[:rt.cursor], rt.tasks[rt.cursor+1:]...)
	if rt.cursor >= len(rt.tasks) && rt.cursor > 0 {
		rt.cursor--
	}
	rt.save()
}

func (rt *RecurringTasks) toggleSelected() {
	if len(rt.tasks) == 0 || rt.cursor >= len(rt.tasks) {
		return
	}
	rt.tasks[rt.cursor].Enabled = !rt.tasks[rt.cursor].Enabled
	rt.save()
}

// ----- Update -----

// Update handles keyboard input for the recurring tasks overlay.
func (rt RecurringTasks) Update(msg tea.Msg) (RecurringTasks, tea.Cmd) {
	if !rt.active {
		return rt, nil
	}
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch rt.view {
		case rvList:
			rt = rt.updateList(msg)
		case rvAdd, rvEdit:
			rt = rt.updateForm(msg)
		}
	}
	return rt, nil
}

func (rt RecurringTasks) updateList(msg tea.KeyMsg) RecurringTasks {
	switch msg.String() {
	case "esc":
		rt.active = false
	case "j", "down":
		if rt.cursor < len(rt.tasks)-1 {
			rt.cursor++
			if visH := rt.visibleHeight(); rt.cursor >= rt.scroll+visH {
				rt.scroll = rt.cursor - visH + 1
			}
		}
	case "k", "up":
		if rt.cursor > 0 {
			rt.cursor--
			if rt.cursor < rt.scroll {
				rt.scroll = rt.cursor
			}
		}
	case " ":
		rt.toggleSelected()
	case "a":
		rt.startAdd()
	case "e":
		rt.startEdit()
	case "d":
		rt.deleteSelected()
	}
	return rt
}

func (rt RecurringTasks) updateForm(msg tea.KeyMsg) RecurringTasks {
	key := msg.String()

	switch key {
	case "esc":
		rt.view = rvList
		return rt
	case "enter":
		if rt.field == rfDay || (rt.field == rfFrequency && freqLabels[rt.inputFreq] == "daily") {
			rt.saveForm()
		} else {
			rt.field++
			if rt.field == rfDay && freqLabels[rt.inputFreq] == "daily" {
				rt.saveForm()
			}
		}
		return rt
	case "tab":
		rt.field++
		if freqLabels[rt.inputFreq] == "daily" && rt.field == rfDay {
			rt.field = rfText
		} else if rt.field > rfDay {
			rt.field = rfText
		}
		return rt
	case "shift+tab":
		if rt.field == rfText {
			if freqLabels[rt.inputFreq] == "daily" {
				rt.field = rfFrequency
			} else {
				rt.field = rfDay
			}
		} else {
			rt.field--
		}
		return rt
	}

	switch rt.field {
	case rfText:
		switch key {
		case "backspace":
			if len(rt.inputText) > 0 {
				rt.inputText = rt.inputText[:len(rt.inputText)-1]
			}
		case "left", "right":
			// ignore cursor movement in text field
		default:
			if len(key) == 1 {
				rt.inputText += key
			} else if key == "space" {
				rt.inputText += " "
			}
		}
	case rfFrequency:
		if key == "left" || key == "h" {
			if rt.inputFreq > 0 {
				rt.inputFreq--
			}
			rt.resetDayForFreq()
		} else if key == "right" || key == "l" {
			if rt.inputFreq < len(freqLabels)-1 {
				rt.inputFreq++
			}
			rt.resetDayForFreq()
		}
	case rfDay:
		if key == "left" || key == "h" {
			if rt.inputDay > rt.minDay() {
				rt.inputDay--
			}
		} else if key == "right" || key == "l" {
			if rt.inputDay < rt.maxDay() {
				rt.inputDay++
			}
		}
	}
	return rt
}

func (rt *RecurringTasks) resetDayForFreq() {
	switch freqLabels[rt.inputFreq] {
	case "weekly":
		rt.inputDay = 0
	case "monthly":
		rt.inputDay = 1
	}
}

func (rt RecurringTasks) minDay() int {
	if freqLabels[rt.inputFreq] == "monthly" {
		return 1
	}
	return 0
}

func (rt RecurringTasks) maxDay() int {
	if freqLabels[rt.inputFreq] == "monthly" {
		return 31
	}
	return 6
}

func (rt RecurringTasks) visibleHeight() int {
	h := rt.height - 12
	if h < 3 {
		h = 3
	}
	return h
}

// ----- View -----

// View renders the recurring tasks overlay.
func (rt RecurringTasks) View() string {
	width := rt.width / 2
	if width < 56 {
		width = 56
	}
	if width > 76 {
		width = 76
	}
	var b strings.Builder
	switch rt.view {
	case rvList:
		rt.viewList(&b, width)
	case rvAdd:
		rt.viewForm(&b, width, "Add Recurring Task")
	case rvEdit:
		rt.viewForm(&b, width, "Edit Recurring Task")
	}
	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)
	return border.Render(b.String())
}

func (rt RecurringTasks) viewList(b *strings.Builder, width int) {
	icon := lipgloss.NewStyle().Foreground(mauve).Render(IconCalendarChar)
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Recurring Tasks")
	count := lipgloss.NewStyle().Foreground(overlay0).Render(fmt.Sprintf(" (%d)", len(rt.tasks)))
	b.WriteString(icon + title + count + "\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)) + "\n\n")

	if len(rt.tasks) == 0 {
		b.WriteString(DimStyle.Render("  No recurring tasks defined") + "\n")
		b.WriteString(DimStyle.Render("  Press ") +
			lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("a") +
			DimStyle.Render(" to add one"))
	} else {
		visH := rt.visibleHeight()
		end := rt.scroll + visH
		if end > len(rt.tasks) {
			end = len(rt.tasks)
		}
		freqStyle := lipgloss.NewStyle().Foreground(sapphire)
		enabledStyle := lipgloss.NewStyle().Foreground(green)
		disabledStyle := lipgloss.NewStyle().Foreground(red)
		lastStyle := lipgloss.NewStyle().Foreground(overlay0)

		for i := rt.scroll; i < end; i++ {
			task := rt.tasks[i]
			status := disabledStyle.Render("○")
			if task.Enabled {
				status = enabledStyle.Render("●")
			}
			freq := freqStyle.Render(rt.freqBadge(task))
			var last string
			if task.LastCreated != "" {
				last = lastStyle.Render("  last: " + task.LastCreated)
			}
			maxTextW := width - 30
			if maxTextW < 10 {
				maxTextW = 10
			}
			taskText := task.Text
			if r := []rune(taskText); len(r) > maxTextW {
				taskText = string(r[:maxTextW-1]) + "…"
			}
			if i == rt.cursor {
				line := fmt.Sprintf("  %s %s  %s%s", status, taskText, freq, last)
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).Foreground(peach).
					Bold(true).Width(width - 6).Render(line))
			} else {
				b.WriteString(fmt.Sprintf("  %s %s  %s%s", status,
					lipgloss.NewStyle().Foreground(text).Render(taskText), freq, last))
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
		if len(rt.tasks) > visH {
			b.WriteString("\n")
			b.WriteString(DimStyle.Render(fmt.Sprintf("  (%d/%d)", rt.cursor+1, len(rt.tasks))))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)) + "\n")
	keys := []struct{ key, desc string }{
		{"a", "add"}, {"e", "edit"}, {"d", "delete"}, {"space", "toggle"}, {"Esc", "close"},
	}
	var footer strings.Builder
	footer.WriteString("  ")
	for i, k := range keys {
		footer.WriteString(lipgloss.NewStyle().Foreground(lavender).Bold(true).Render(k.key))
		footer.WriteString(DimStyle.Render(": " + k.desc))
		if i < len(keys)-1 {
			footer.WriteString("  ")
		}
	}
	b.WriteString(footer.String())
}

func (rt RecurringTasks) viewForm(b *strings.Builder, width int, title string) {
	icon := lipgloss.NewStyle().Foreground(mauve).Render(IconCalendarChar)
	titleStr := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  " + title)
	b.WriteString(icon + titleStr + "\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)) + "\n\n")

	activeLabel := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inactiveLabel := lipgloss.NewStyle().Foreground(subtext0)
	inputActive := lipgloss.NewStyle().Foreground(text).Background(surface0).Padding(0, 1).Width(width - 16)
	inputInactive := lipgloss.NewStyle().Foreground(subtext0).Background(surface1).Padding(0, 1).Width(width - 16)

	// Text field
	if rt.field == rfText {
		b.WriteString("  " + activeLabel.Render("Task:  ") + inputActive.Render(rt.inputText+"█"))
	} else {
		t := rt.inputText
		if t == "" {
			t = "(empty)"
		}
		b.WriteString("  " + inactiveLabel.Render("Task:  ") + inputInactive.Render(t))
	}
	b.WriteString("\n\n")

	// Frequency selector
	if rt.field == rfFrequency {
		b.WriteString("  " + activeLabel.Render("Freq:  "))
	} else {
		b.WriteString("  " + inactiveLabel.Render("Freq:  "))
	}
	for i, label := range freqLabels {
		if i == rt.inputFreq {
			b.WriteString(lipgloss.NewStyle().
				Foreground(mantle).Background(mauve).Bold(true).Padding(0, 1).Render(label))
		} else {
			b.WriteString(lipgloss.NewStyle().
				Foreground(subtext0).Background(surface1).Padding(0, 1).Render(label))
		}
		if i < len(freqLabels)-1 {
			b.WriteString(" ")
		}
	}
	if rt.field == rfFrequency {
		b.WriteString(DimStyle.Render("  ◀ ▶"))
	}
	b.WriteString("\n\n")

	// Day selector (only for weekly/monthly)
	freq := freqLabels[rt.inputFreq]
	if freq != "daily" {
		if rt.field == rfDay {
			b.WriteString("  " + activeLabel.Render("Day:   "))
		} else {
			b.WriteString("  " + inactiveLabel.Render("Day:   "))
		}
		dayStr := rt.dayLabel(freq, rt.inputDay)
		if rt.field == rfDay {
			b.WriteString(lipgloss.NewStyle().
				Foreground(mantle).Background(sapphire).Bold(true).Padding(0, 1).Render(dayStr))
			b.WriteString(DimStyle.Render("  ◀ ▶"))
		} else {
			b.WriteString(lipgloss.NewStyle().
				Foreground(subtext0).Background(surface1).Padding(0, 1).Render(dayStr))
		}
		b.WriteString("\n\n")
	}

	// Footer
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)) + "\n")
	tabKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Tab")
	enterKey := lipgloss.NewStyle().Foreground(green).Bold(true).Render("Enter")
	escKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Esc")
	b.WriteString("  " + tabKey + DimStyle.Render(": next field  ") +
		enterKey + DimStyle.Render(": save  ") + escKey + DimStyle.Render(": cancel"))
}

// ----- formatting helpers -----

func (rt RecurringTasks) freqBadge(task RecurringTask) string {
	switch task.Frequency {
	case "daily":
		return "[daily]"
	case "weekly":
		return "[" + rt.weekdayShort(task.DayOfWeek) + "]"
	case "monthly":
		return fmt.Sprintf("[%s of month]", rt.ordinal(task.DayOfMonth))
	}
	return "[" + task.Frequency + "]"
}

func (rt RecurringTasks) dayLabel(freq string, day int) string {
	if freq == "weekly" {
		return rt.weekdayName(day)
	}
	return fmt.Sprintf("%s of month", rt.ordinal(day))
}

func (rt RecurringTasks) weekdayName(d int) string {
	if d < 0 || d > 6 {
		d = 0
	}
	return time.Weekday(d).String()
}

func (rt RecurringTasks) weekdayShort(d int) string {
	name := rt.weekdayName(d)
	if len(name) >= 3 {
		return name[:3]
	}
	return name
}

func (rt RecurringTasks) ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
		// 11th, 12th, 13th
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
