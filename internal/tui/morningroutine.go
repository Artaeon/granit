package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Phases — interactive, question-driven daily planning
// ---------------------------------------------------------------------------

type morningPhase int

const (
	morningScripture  morningPhase = iota // Step 1: Bible quote
	morningGoal                           // Step 2: Set today's goal
	morningTasks                          // Step 3: Select/create tasks
	morningHabits                         // Step 4: Select habits
	morningThoughts                       // Step 5: Reflection note
	morningSummary                        // Step 6: Overview + save
	morningComplete                       // Done
)

// morningPlanSavedMsg is sent after the daily note has been written.
type morningPlanSavedMsg struct{}

// ---------------------------------------------------------------------------
// Overlay
// ---------------------------------------------------------------------------

// MorningRoutine is a guided interactive daily planning workflow.
// No AI calls — just scripture, questions, selections, and a summary
// that gets saved to the daily note.
type MorningRoutine struct {
	active bool
	width  int
	height int
	phase  morningPhase
	scroll int

	// Step 1: Scripture
	scripture Scripture

	// Step 2: Goal
	todayGoal string

	// Step 3: Tasks — select from existing + create new
	allTasks      []Task           // all open tasks from vault
	taskSelected  []bool           // toggle state per task
	taskCursor    int              // cursor position in task list
	newTaskInput  string           // text for creating a new task
	taskAddMode   bool             // currently typing a new task
	createdTasks  []string         // tasks created during this session

	// Step 4: Habits
	allHabits     []habitEntry
	habitSelected []bool
	habitCursor   int

	// Step 5: Thoughts
	thoughts string

	// Vault data
	vaultRoot string
	goals     []Goal
	events    []PlannerEvent

	// Config
	dailyNotesFolder string
}

// NewMorningRoutine creates a new MorningRoutine overlay.
func NewMorningRoutine() MorningRoutine {
	return MorningRoutine{}
}

// IsActive reports whether the morning routine overlay is visible.
func (mr MorningRoutine) IsActive() bool { return mr.active }

// SetSize updates the available terminal dimensions.
func (mr *MorningRoutine) SetSize(w, h int) {
	mr.width = w
	mr.height = h
}

// Open initialises the morning routine with gathered vault data.
func (mr *MorningRoutine) Open(
	vaultRoot string,
	goals []Goal,
	tasks []Task,
	events []PlannerEvent,
	habits []habitEntry,
) tea.Cmd {
	mr.active = true
	mr.vaultRoot = vaultRoot
	mr.goals = goals
	mr.events = events

	mr.phase = morningScripture
	mr.scroll = 0
	mr.scripture = DailyScripture(vaultRoot)

	// Goal
	mr.todayGoal = ""

	// Tasks — collect open tasks due today or overdue or high priority
	mr.allTasks = nil
	today := time.Now().Format("2006-01-02")
	for _, t := range tasks {
		if t.Done {
			continue
		}
		isRelevant := t.DueDate == today ||
			(t.DueDate != "" && t.DueDate < today) ||
			t.Priority >= 3
		if isRelevant {
			mr.allTasks = append(mr.allTasks, t)
		}
	}
	// Also add tasks due in next 2 days
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	dayAfter := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
	for _, t := range tasks {
		if t.Done {
			continue
		}
		if t.DueDate == tomorrow || t.DueDate == dayAfter {
			// Check not already added
			alreadyAdded := false
			for _, existing := range mr.allTasks {
				if existing.Text == t.Text && existing.NotePath == t.NotePath {
					alreadyAdded = true
					break
				}
			}
			if !alreadyAdded {
				mr.allTasks = append(mr.allTasks, t)
			}
		}
	}
	mr.taskSelected = make([]bool, len(mr.allTasks))
	// Pre-select overdue and due-today tasks
	for i, t := range mr.allTasks {
		if t.DueDate != "" && t.DueDate <= today {
			mr.taskSelected[i] = true
		}
	}
	mr.taskCursor = 0
	mr.newTaskInput = ""
	mr.taskAddMode = false
	mr.createdTasks = nil

	// Habits
	mr.allHabits = habits
	mr.habitSelected = make([]bool, len(habits))
	mr.habitCursor = 0

	// Thoughts
	mr.thoughts = ""

	return nil
}

// Update handles messages.
func (mr MorningRoutine) Update(msg tea.Msg) (MorningRoutine, tea.Cmd) {
	if !mr.active {
		return mr, nil
	}

	switch msg := msg.(type) {
	case morningPlanSavedMsg:
		return mr, nil

	case tea.KeyMsg:
		switch mr.phase {
		case morningScripture:
			return mr.updateScripture(msg)
		case morningGoal:
			return mr.updateGoal(msg)
		case morningTasks:
			return mr.updateTasks(msg)
		case morningHabits:
			return mr.updateHabits(msg)
		case morningThoughts:
			return mr.updateThoughts(msg)
		case morningSummary:
			return mr.updateSummary(msg)
		case morningComplete:
			if msg.String() == "enter" || msg.String() == "esc" || msg.String() == "q" {
				mr.active = false
			}
			return mr, nil
		}
	}
	return mr, nil
}

// ---------------------------------------------------------------------------
// Phase updates
// ---------------------------------------------------------------------------

func (mr MorningRoutine) updateScripture(msg tea.KeyMsg) (MorningRoutine, tea.Cmd) {
	switch msg.String() {
	case "enter":
		mr.phase = morningGoal
		mr.scroll = 0
	case "esc", "q":
		mr.active = false
	}
	return mr, nil
}

func (mr MorningRoutine) updateGoal(msg tea.KeyMsg) (MorningRoutine, tea.Cmd) {
	switch msg.String() {
	case "enter":
		mr.phase = morningTasks
		mr.scroll = 0
	case "esc":
		mr.phase = morningTasks
		mr.scroll = 0
	case "backspace":
		if len(mr.todayGoal) > 0 {
			runes := []rune(mr.todayGoal)
			mr.todayGoal = string(runes[:len(runes)-1])
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= 32 {
			mr.todayGoal += ch
		} else if ch == "space" {
			mr.todayGoal += " "
		}
	}
	return mr, nil
}

func (mr MorningRoutine) updateTasks(msg tea.KeyMsg) (MorningRoutine, tea.Cmd) {
	if mr.taskAddMode {
		return mr.updateTaskAdd(msg)
	}

	totalItems := len(mr.allTasks) + len(mr.createdTasks)
	switch msg.String() {
	case "enter":
		mr.phase = morningHabits
		mr.scroll = 0
	case "esc":
		mr.phase = morningHabits
		mr.scroll = 0
	case "j", "down":
		if mr.taskCursor < totalItems-1 {
			mr.taskCursor++
		}
	case "k", "up":
		if mr.taskCursor > 0 {
			mr.taskCursor--
		}
	case " ":
		// Toggle task selection
		if mr.taskCursor < len(mr.allTasks) {
			mr.taskSelected[mr.taskCursor] = !mr.taskSelected[mr.taskCursor]
		}
		// Created tasks are always selected, no toggle
	case "a":
		mr.taskAddMode = true
		mr.newTaskInput = ""
	}
	return mr, nil
}

func (mr MorningRoutine) updateTaskAdd(msg tea.KeyMsg) (MorningRoutine, tea.Cmd) {
	switch msg.String() {
	case "enter":
		if mr.newTaskInput != "" {
			mr.createdTasks = append(mr.createdTasks, mr.newTaskInput)
			mr.newTaskInput = ""
		}
		mr.taskAddMode = false
	case "esc":
		mr.taskAddMode = false
		mr.newTaskInput = ""
	case "backspace":
		if len(mr.newTaskInput) > 0 {
			runes := []rune(mr.newTaskInput)
			mr.newTaskInput = string(runes[:len(runes)-1])
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= 32 {
			mr.newTaskInput += ch
		} else if ch == "space" {
			mr.newTaskInput += " "
		}
	}
	return mr, nil
}

func (mr MorningRoutine) updateHabits(msg tea.KeyMsg) (MorningRoutine, tea.Cmd) {
	switch msg.String() {
	case "enter":
		mr.phase = morningThoughts
		mr.scroll = 0
	case "esc":
		mr.phase = morningThoughts
		mr.scroll = 0
	case "j", "down":
		if mr.habitCursor < len(mr.allHabits)-1 {
			mr.habitCursor++
		}
	case "k", "up":
		if mr.habitCursor > 0 {
			mr.habitCursor--
		}
	case " ":
		if mr.habitCursor < len(mr.habitSelected) {
			mr.habitSelected[mr.habitCursor] = !mr.habitSelected[mr.habitCursor]
		}
	}
	return mr, nil
}

func (mr MorningRoutine) updateThoughts(msg tea.KeyMsg) (MorningRoutine, tea.Cmd) {
	switch msg.String() {
	case "enter":
		mr.phase = morningSummary
		mr.scroll = 0
	case "esc":
		mr.phase = morningSummary
		mr.scroll = 0
	case "backspace":
		if len(mr.thoughts) > 0 {
			runes := []rune(mr.thoughts)
			mr.thoughts = string(runes[:len(runes)-1])
		}
	default:
		ch := msg.String()
		if len(ch) == 1 && ch[0] >= 32 {
			mr.thoughts += ch
		} else if ch == "space" {
			mr.thoughts += " "
		}
	}
	return mr, nil
}

func (mr MorningRoutine) updateSummary(msg tea.KeyMsg) (MorningRoutine, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Save to daily note and close
		mr.phase = morningComplete
		return mr, mr.saveToDailyNote()
	case "esc", "q":
		mr.active = false
	case "j", "down":
		mr.scroll++
	case "k", "up":
		if mr.scroll > 0 {
			mr.scroll--
		}
	}
	return mr, nil
}

// ---------------------------------------------------------------------------
// Save to daily note
// ---------------------------------------------------------------------------

func (mr *MorningRoutine) saveToDailyNote() tea.Cmd {
	vaultRoot := mr.vaultRoot
	folder := mr.dailyNotesFolder
	content := mr.buildDailyPlanMarkdown()

	return func() tea.Msg {
		today := time.Now().Format("2006-01-02")
		dailyName := today + ".md"
		if folder != "" {
			dailyName = filepath.Join(folder, dailyName)
		}
		dailyPath := filepath.Join(vaultRoot, dailyName)

		existing, err := os.ReadFile(dailyPath)
		if err != nil {
			// Create new daily note — ensure directory exists
			if mkErr := os.MkdirAll(filepath.Dir(dailyPath), 0755); mkErr != nil {
				return morningPlanSavedMsg{}
			}
			weekday := time.Now().Weekday().String()
			header := fmt.Sprintf("---\ndate: %s\ntype: daily\ntags: [daily]\n---\n\n# %s — %s\n\n",
				today, today, weekday)
			if writeErr := os.WriteFile(dailyPath, []byte(header+content), 0644); writeErr != nil {
				return morningPlanSavedMsg{}
			}
		} else {
			// Replace existing "## Daily Plan" section or append if not present
			newContent := replaceDailySection(string(existing), content, "## Daily Plan")
			if writeErr := os.WriteFile(dailyPath, []byte(newContent), 0644); writeErr != nil {
				return morningPlanSavedMsg{}
			}
		}
		return morningPlanSavedMsg{}
	}
}

func (mr *MorningRoutine) buildDailyPlanMarkdown() string {
	var b strings.Builder
	today := time.Now().Format("Monday, January 2, 2006")

	b.WriteString(fmt.Sprintf("## Daily Plan — %s\n\n", today))

	// Scripture
	b.WriteString(fmt.Sprintf("> *\"%s\"* — %s\n\n", mr.scripture.Text, mr.scripture.Source))

	// Goal
	if mr.todayGoal != "" {
		b.WriteString(fmt.Sprintf("### Today's Goal\n\n**%s**\n\n", mr.todayGoal))
	}

	// Tasks
	selectedTasks := mr.getSelectedTasks()
	if len(selectedTasks) > 0 {
		b.WriteString("### Tasks\n\n")
		for _, t := range selectedTasks {
			b.WriteString(fmt.Sprintf("- [ ] %s\n", t))
		}
		b.WriteString("\n")
	}

	// Habits
	selectedHabits := mr.getSelectedHabits()
	if len(selectedHabits) > 0 {
		b.WriteString("### Habits\n\n")
		for _, h := range selectedHabits {
			b.WriteString(fmt.Sprintf("- [ ] %s\n", h))
		}
		b.WriteString("\n")
	}

	// Thoughts
	if mr.thoughts != "" {
		b.WriteString(fmt.Sprintf("### Thoughts\n\n%s\n\n", mr.thoughts))
	}

	return b.String()
}

func (mr *MorningRoutine) getSelectedTasks() []string {
	var result []string
	for i, t := range mr.allTasks {
		if mr.taskSelected[i] {
			label := t.Text
			if t.DueDate != "" {
				dueSuffix := " (due: " + t.DueDate + ")"
				if !strings.HasSuffix(label, dueSuffix) {
					label += dueSuffix
				}
			}
			result = append(result, label)
		}
	}
	result = append(result, mr.createdTasks...)
	return result
}

func (mr *MorningRoutine) getSelectedHabits() []string {
	var result []string
	for i, h := range mr.allHabits {
		if mr.habitSelected[i] {
			result = append(result, h.Name)
		}
	}
	return result
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (mr MorningRoutine) View() string {
	if !mr.active {
		return ""
	}

	width := mr.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 8

	var b strings.Builder

	// Header with progress
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	stepNames := []string{"Scripture", "Goal", "Tasks", "Habits", "Thoughts", "Summary", "Done"}
	stepNum := int(mr.phase) + 1
	totalSteps := 6
	if stepNum > totalSteps {
		stepNum = totalSteps
	}
	progress := fmt.Sprintf("Step %d/%d", stepNum, totalSteps)
	b.WriteString("  " + headerStyle.Render("Plan My Day") + "  " +
		DimStyle.Render(progress+" — "+stepNames[mr.phase]) + "\n")

	// Progress bar
	filled := int(mr.phase) * innerW / totalSteps
	if filled > innerW {
		filled = innerW
	}
	bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("=", filled)) +
		lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("-", innerW-filled))
	b.WriteString("  " + bar + "\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)) + "\n\n")

	switch mr.phase {
	case morningScripture:
		mr.viewScripture(&b, innerW)
	case morningGoal:
		mr.viewGoal(&b, innerW)
	case morningTasks:
		mr.viewTasks(&b, innerW)
	case morningHabits:
		mr.viewHabits(&b, innerW)
	case morningThoughts:
		mr.viewThoughts(&b, innerW)
	case morningSummary:
		mr.viewSummary(&b, innerW)
	case morningComplete:
		mr.viewComplete(&b, innerW)
	}

	// Help bar
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)) + "\n")
	b.WriteString(mr.helpBar())

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (mr MorningRoutine) helpBar() string {
	switch mr.phase {
	case morningScripture:
		return DimStyle.Render("  Enter next  Esc close")
	case morningGoal:
		return DimStyle.Render("  Type your goal  Enter next  Esc skip")
	case morningTasks:
		if mr.taskAddMode {
			return DimStyle.Render("  Type task name  Enter add  Esc cancel")
		}
		return DimStyle.Render("  j/k move  Space toggle  a add task  Enter next  Esc skip")
	case morningHabits:
		return DimStyle.Render("  j/k move  Space toggle  Enter next  Esc skip")
	case morningThoughts:
		return DimStyle.Render("  Type your thoughts  Enter next  Esc skip")
	case morningSummary:
		return DimStyle.Render("  Enter save to daily note  j/k scroll  Esc close")
	case morningComplete:
		return DimStyle.Render("  Enter close")
	}
	return ""
}

// ---------------------------------------------------------------------------
// Phase views
// ---------------------------------------------------------------------------

func (mr MorningRoutine) viewScripture(b *strings.Builder, w int) {
	verseStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)
	refStyle := lipgloss.NewStyle().Foreground(overlay1)

	b.WriteString("  " + lipgloss.NewStyle().Foreground(blue).Bold(true).Render("Today's Scripture") + "\n\n")

	// Word-wrap the verse text
	wrappedLines := morningWordWrap(mr.scripture.Text, w-6)
	for _, wl := range wrappedLines {
		b.WriteString("  " + verseStyle.Render(wl) + "\n")
	}
	b.WriteString("\n")
	b.WriteString("  " + refStyle.Render("— "+mr.scripture.Source) + "\n\n")
	b.WriteString("  " + DimStyle.Render("Take a moment to reflect, then press Enter.") + "\n")
}

func (mr MorningRoutine) viewGoal(b *strings.Builder, _ int) {
	questionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text)

	b.WriteString("  " + questionStyle.Render("What is your main goal for today?") + "\n\n")

	// Show active goals as context
	hasGoals := false
	for _, g := range mr.goals {
		if g.Status == GoalStatusActive {
			if !hasGoals {
				b.WriteString("  " + DimStyle.Render("Active goals:") + "\n")
				hasGoals = true
			}
			b.WriteString("  " + DimStyle.Render(fmt.Sprintf("  %s (%d%%)", g.Title, g.Progress())) + "\n")
		}
	}
	if hasGoals {
		b.WriteString("\n")
	}

	b.WriteString("  " + inputStyle.Render(mr.todayGoal) + DimStyle.Render("_") + "\n")
}

func (mr MorningRoutine) viewTasks(b *strings.Builder, w int) {
	questionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	unselectedStyle := lipgloss.NewStyle().Foreground(overlay0)
	cursorStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	dueStyle := lipgloss.NewStyle().Foreground(peach)

	b.WriteString("  " + questionStyle.Render("Select the tasks you want to accomplish today") + "\n\n")

	if len(mr.allTasks) == 0 && len(mr.createdTasks) == 0 {
		b.WriteString("  " + DimStyle.Render("No pending tasks found. Press 'a' to add one.") + "\n")
	}

	today := time.Now().Format("2006-01-02")
	for i, t := range mr.allTasks {
		prefix := "  "
		if i == mr.taskCursor {
			prefix = cursorStyle.Render("> ")
		}

		checkbox := unselectedStyle.Render("[ ] ")
		textStyle := unselectedStyle
		if mr.taskSelected[i] {
			checkbox = selectedStyle.Render("[x] ")
			textStyle = lipgloss.NewStyle().Foreground(text)
		}

		label := TruncateDisplay(t.Text, w-14)
		suffix := ""
		if t.DueDate != "" && t.DueDate < today {
			suffix = dueStyle.Render(" overdue")
		} else if t.DueDate == today {
			suffix = dueStyle.Render(" today")
		} else if t.DueDate != "" {
			suffix = DimStyle.Render(" " + t.DueDate)
		}
		if t.Priority >= 3 {
			suffix += lipgloss.NewStyle().Foreground(red).Render(" !")
		}

		b.WriteString(prefix + checkbox + textStyle.Render(label) + suffix + "\n")
	}

	// Created tasks
	for i, t := range mr.createdTasks {
		idx := len(mr.allTasks) + i
		prefix := "  "
		if idx == mr.taskCursor {
			prefix = cursorStyle.Render("> ")
		}
		checkbox := selectedStyle.Render("[+] ")
		b.WriteString(prefix + checkbox + lipgloss.NewStyle().Foreground(green).Render(TruncateDisplay(t, w-14)) + "\n")
	}

	// Add mode
	if mr.taskAddMode {
		b.WriteString("\n")
		promptStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		b.WriteString("  " + promptStyle.Render("New task: ") + mr.newTaskInput + DimStyle.Render("_") + "\n")
	}
}

func (mr MorningRoutine) viewHabits(b *strings.Builder, _ int) {
	questionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	unselectedStyle := lipgloss.NewStyle().Foreground(overlay0)
	cursorStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	streakStyle := lipgloss.NewStyle().Foreground(peach)

	b.WriteString("  " + questionStyle.Render("Which habits will you do today?") + "\n\n")

	if len(mr.allHabits) == 0 {
		b.WriteString("  " + DimStyle.Render("No habits tracked yet.") + "\n")
		b.WriteString("  " + DimStyle.Render("Use the Habit Tracker to add habits.") + "\n")
		return
	}

	for i, h := range mr.allHabits {
		prefix := "  "
		if i == mr.habitCursor {
			prefix = cursorStyle.Render("> ")
		}

		checkbox := unselectedStyle.Render("[ ] ")
		textStyle := unselectedStyle
		if mr.habitSelected[i] {
			checkbox = selectedStyle.Render("[x] ")
			textStyle = lipgloss.NewStyle().Foreground(text)
		}

		streak := ""
		if h.Streak > 0 {
			streak = streakStyle.Render(fmt.Sprintf(" %dd streak", h.Streak))
		}

		b.WriteString(prefix + checkbox + textStyle.Render(h.Name) + streak + "\n")
	}
}

func (mr MorningRoutine) viewThoughts(b *strings.Builder, _ int) {
	questionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text)

	b.WriteString("  " + questionStyle.Render("Any thoughts or intentions for today?") + "\n\n")
	b.WriteString("  " + DimStyle.Render("(optional — a note to your future self)") + "\n\n")
	b.WriteString("  " + inputStyle.Render(mr.thoughts) + DimStyle.Render("_") + "\n")
}

func (mr MorningRoutine) viewSummary(b *strings.Builder, w int) {
	titleStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	itemStyle := lipgloss.NewStyle().Foreground(text)
	verseStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)

	b.WriteString("  " + titleStyle.Render("Your Day at a Glance") + "\n\n")

	// Scripture
	truncVerse := TruncateDisplay(mr.scripture.Text, w-10)
	b.WriteString("  " + verseStyle.Render(truncVerse) + "\n")
	b.WriteString("  " + DimStyle.Render("— "+mr.scripture.Source) + "\n\n")

	// Goal
	if mr.todayGoal != "" {
		b.WriteString("  " + labelStyle.Render("Goal: ") + itemStyle.Render(mr.todayGoal) + "\n\n")
	}

	// Tasks
	selectedTasks := mr.getSelectedTasks()
	if len(selectedTasks) > 0 {
		b.WriteString("  " + labelStyle.Render(fmt.Sprintf("Tasks (%d)", len(selectedTasks))) + "\n")
		for _, t := range selectedTasks {
			b.WriteString("  " + itemStyle.Render("  - "+TruncateDisplay(t, w-10)) + "\n")
		}
		b.WriteString("\n")
	}

	// Habits
	selectedHabits := mr.getSelectedHabits()
	if len(selectedHabits) > 0 {
		b.WriteString("  " + labelStyle.Render(fmt.Sprintf("Habits (%d)", len(selectedHabits))) + "\n")
		for _, h := range selectedHabits {
			b.WriteString("  " + itemStyle.Render("  - "+h) + "\n")
		}
		b.WriteString("\n")
	}

	// Calendar events
	if len(mr.events) > 0 {
		b.WriteString("  " + labelStyle.Render(fmt.Sprintf("Events (%d)", len(mr.events))) + "\n")
		for _, e := range mr.events {
			b.WriteString("  " + DimStyle.Render(fmt.Sprintf("  - %s (%s, %dmin)", e.Title, e.Time, e.Duration)) + "\n")
		}
		b.WriteString("\n")
	}

	// Thoughts
	if mr.thoughts != "" {
		b.WriteString("  " + labelStyle.Render("Thoughts") + "\n")
		b.WriteString("  " + DimStyle.Render("  "+mr.thoughts) + "\n\n")
	}

	b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("Press Enter to save this plan to your daily note.") + "\n")
}

func (mr MorningRoutine) viewComplete(b *strings.Builder, _ int) {
	successStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString("  " + successStyle.Render("Day planned!") + "\n\n")

	selected := len(mr.getSelectedTasks())
	habits := len(mr.getSelectedHabits())
	b.WriteString("  " + DimStyle.Render(fmt.Sprintf("Saved to daily note: %d tasks, %d habits", selected, habits)) + "\n")
	if mr.todayGoal != "" {
		b.WriteString("  " + DimStyle.Render("Goal: "+mr.todayGoal) + "\n")
	}
	b.WriteString("\n  " + DimStyle.Render("Go make today count.") + "\n")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// morningWordWrap splits text into lines that fit within maxWidth characters.
func morningWordWrap(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) > maxWidth {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	lines = append(lines, current)
	return lines
}
