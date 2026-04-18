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

// ---------------------------------------------------------------------------
// Phases — interactive, question-driven daily planning
// ---------------------------------------------------------------------------

type morningPhase int

const (
	morningOverview   morningPhase = iota // Step 1: Calendar + yesterday + goals overview
	morningScripture                      // Step 2: Bible quote
	morningGoal                           // Step 3: Set today's goal
	morningTasks                          // Step 4: Select/schedule tasks
	morningHabits                         // Step 5: Select habits
	morningThoughts                       // Step 6: Reflection note
	morningSummary                        // Step 7: Timeline + save
	morningComplete                       // Done
)

// morningPlanSavedMsg is sent after the daily note has been written.
// Err carries the first write failure encountered — the host surfaces
// it via reportError on receipt. A partially-saved plan (daily note
// ok, one planner block fail) sets Err to the first failure; further
// writes still run so best-effort persistence is preserved.
type morningPlanSavedMsg struct{ Err error }

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

	// Step 1: Overview data
	yesterdayTasks []string // carry-forward from yesterday

	// Step 2: Scripture
	scripture Scripture

	// Step 3: Goal
	todayGoal string

	// Step 4: Tasks — ALL open tasks, select which to schedule
	allTasks     []Task   // all open tasks from vault
	taskSelected []bool   // toggle state per task
	taskCursor   int      // cursor position in task list
	newTaskInput string   // text for creating a new task
	taskAddMode  bool     // currently typing a new task
	createdTasks []string // tasks created during this session
	taskScroll   int      // scroll offset for long task lists

	// Step 5: Habits
	allHabits     []habitEntry
	habitSelected []bool
	habitCursor   int

	// Step 6: Thoughts
	thoughts string

	// Vault data
	vaultRoot      string
	goals          []Goal
	events         []PlannerEvent
	projects       []Project       // active projects
	existingBlocks []PlannerBlock   // already-scheduled blocks for today
	existingFocus  DailyFocus       // focus set by a previous planning session

	// Generated schedule (built when entering summary phase)
	schedule []daySlot

	// Config
	dailyNotesFolder string

	// aiRefineRequested is set when the user presses 'p' on the complete
	// screen, asking to hand off to Plan My Day for AI refinement. The
	// host (app_update.go) consumes it via ConsumeAIRefineRequest() after
	// the overlay closes. This keeps the overlay stateless wrt. other
	// overlays and avoids chaining them directly.
	aiRefineRequested bool
}

// ConsumeAIRefineRequest returns true (consumed-once) if the user asked to
// continue into Plan My Day after the morning routine ended.
func (mr *MorningRoutine) ConsumeAIRefineRequest() bool {
	if mr.aiRefineRequested {
		mr.aiRefineRequested = false
		return true
	}
	return false
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
	projects []Project,
	yesterdayTasks []string,
	existingBlocks []PlannerBlock,
	existingFocus DailyFocus,
) tea.Cmd {
	mr.active = true
	mr.vaultRoot = vaultRoot
	mr.goals = goals
	mr.events = events
	mr.projects = projects
	mr.yesterdayTasks = yesterdayTasks
	mr.existingBlocks = existingBlocks
	mr.existingFocus = existingFocus

	mr.phase = morningOverview
	mr.scroll = 0
	mr.scripture = DailyScripture(vaultRoot)

	// Pre-populate goal from existing daily focus (set by Plan My Day or earlier session)
	mr.todayGoal = existingFocus.TopGoal

	// Tasks — include ALL incomplete tasks, sorted by relevance
	mr.allTasks = nil
	today := time.Now().Format("2006-01-02")
	for _, t := range tasks {
		if t.Done {
			continue
		}
		mr.allTasks = append(mr.allTasks, t)
	}
	// Sort: overdue first, then due today, then high priority, then by date, then rest
	sort.SliceStable(mr.allTasks, func(i, j int) bool {
		a, b := mr.allTasks[i], mr.allTasks[j]
		aScore := mrTaskScore(a, today)
		bScore := mrTaskScore(b, today)
		if aScore != bScore {
			return aScore > bScore
		}
		return a.Priority > b.Priority
	})
	mr.taskSelected = make([]bool, len(mr.allTasks))
	// Pre-select overdue, due-today, and high-priority tasks
	for i, t := range mr.allTasks {
		isOverdue := t.DueDate != "" && t.DueDate < today
		isDueToday := t.DueDate == today
		isHighPri := t.Priority >= 3
		if isOverdue || isDueToday || isHighPri {
			mr.taskSelected[i] = true
		}
	}
	mr.taskCursor = 0
	mr.taskScroll = 0
	mr.newTaskInput = ""
	mr.taskAddMode = false
	mr.createdTasks = nil

	// Habits — pre-select all
	mr.allHabits = habits
	mr.habitSelected = make([]bool, len(habits))
	for i := range mr.habitSelected {
		mr.habitSelected[i] = true
	}
	mr.habitCursor = 0

	// Thoughts
	mr.thoughts = ""
	mr.schedule = nil

	return nil
}

// mrTaskScore assigns a sorting score to a task for the morning planning view.
func mrTaskScore(t Task, today string) int {
	score := 0
	if t.DueDate != "" && t.DueDate < today {
		score += 1000 // overdue
	} else if t.DueDate == today {
		score += 800 // due today
	} else if t.DueDate != "" {
		// Due in future — closer = higher score
		dt, err := time.Parse("2006-01-02", t.DueDate)
		if err == nil {
			daysAway := int(dt.Sub(time.Now()).Hours() / 24)
			if daysAway <= 2 {
				score += 600
			} else if daysAway <= 7 {
				score += 400
			} else {
				score += 100
			}
		}
	}
	score += t.Priority * 50
	return score
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
		case morningOverview:
			return mr.updateOverview(msg)
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
			switch msg.String() {
			case "p", "P":
				// Hand off to Plan My Day: close this overlay and flag so
				// the host opens the AI planner with today's context.
				mr.aiRefineRequested = true
				mr.active = false
			case "enter", "esc", "q":
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

func (mr MorningRoutine) updateOverview(msg tea.KeyMsg) (MorningRoutine, tea.Cmd) {
	switch msg.String() {
	case "enter":
		mr.phase = morningScripture
		mr.scroll = 0
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

func (mr MorningRoutine) updateScripture(msg tea.KeyMsg) (MorningRoutine, tea.Cmd) {
	switch msg.String() {
	case "enter":
		mr.phase = morningGoal
		mr.scroll = 0
	case "shift+tab":
		mr.phase = morningOverview
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
	case "shift+tab":
		mr.phase = morningScripture
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
	visHeight := mr.height - 20
	if visHeight < 8 {
		visHeight = 8
	}
	switch msg.String() {
	case "enter":
		mr.phase = morningHabits
		mr.scroll = 0
	case "shift+tab":
		mr.phase = morningGoal
		mr.scroll = 0
	case "esc":
		mr.phase = morningHabits
		mr.scroll = 0
	case "j", "down":
		if mr.taskCursor < totalItems-1 {
			mr.taskCursor++
			// Auto-scroll
			if mr.taskCursor >= mr.taskScroll+visHeight {
				mr.taskScroll = mr.taskCursor - visHeight + 1
			}
		}
	case "k", "up":
		if mr.taskCursor > 0 {
			mr.taskCursor--
			if mr.taskCursor < mr.taskScroll {
				mr.taskScroll = mr.taskCursor
			}
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
	case "shift+tab":
		mr.phase = morningTasks
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
		mr.generateSchedule()
		mr.phase = morningSummary
		mr.scroll = 0
	case "shift+tab":
		mr.phase = morningHabits
		mr.scroll = 0
	case "esc":
		mr.generateSchedule()
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
// Schedule generation
// ---------------------------------------------------------------------------

// generateSchedule builds a time-blocked daily schedule using the shared
// schedule generator. Collects selected tasks, events, and habits and
// delegates to GenerateLocalSchedule (schedule.go).
func (mr *MorningRoutine) generateSchedule() {
	var tasks []ScheduleTask
	for i, t := range mr.allTasks {
		if !mr.taskSelected[i] {
			continue
		}
		tasks = append(tasks, ScheduleTask{
			Text: t.Text, Priority: t.Priority, Estimate: t.EstimatedMinutes,
		})
	}
	for _, ct := range mr.createdTasks {
		tasks = append(tasks, ScheduleTask{Text: ct, Priority: 2, Estimate: 45})
	}

	mr.schedule = GenerateLocalSchedule(ScheduleInput{
		Tasks:          tasks,
		Events:         mr.events,
		Habits:         mr.getSelectedHabits(),
		Projects:       mr.projects,
		ExistingBlocks: mr.existingBlocks,
		WorkEnd:        22 * 60,
	})
}

// ---------------------------------------------------------------------------
// Save to daily note
// ---------------------------------------------------------------------------

func (mr *MorningRoutine) saveToDailyNote() tea.Cmd {
	vaultRoot := mr.vaultRoot
	folder := mr.dailyNotesFolder
	content := mr.buildDailyPlanMarkdown()
	schedule := make([]daySlot, len(mr.schedule))
	copy(schedule, mr.schedule)
	todayGoal := mr.todayGoal
	selectedTasks := mr.getSelectedTasks()
	// Snapshot the task list so the goroutine can resolve slot→source refs
	// without racing against a concurrent vault rescan.
	allTasks := make([]Task, len(mr.allTasks))
	copy(allTasks, mr.allTasks)
	// Capture created tasks to persist them to Tasks.md
	createdTasks := make([]string, len(mr.createdTasks))
	copy(createdTasks, mr.createdTasks)

	return func() tea.Msg {
		today := time.Now().Format("2006-01-02")
		dailyName := today + ".md"
		if folder != "" {
			dailyName = filepath.Join(folder, dailyName)
		}
		dailyPath := filepath.Join(vaultRoot, dailyName)

		// firstErr captures the first failure encountered. Best-effort
		// semantics — subsequent writes keep running so the user gets
		// as much of their plan persisted as possible.
		var firstErr error
		recordErr := func(ctx string, err error) {
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", ctx, err)
			}
		}

		existing, err := os.ReadFile(dailyPath)
		if err != nil {
			// Create new daily note — ensure directory exists
			if mkErr := os.MkdirAll(filepath.Dir(dailyPath), 0o755); mkErr != nil {
				return morningPlanSavedMsg{Err: fmt.Errorf("create daily note folder: %w", mkErr)}
			}
			weekday := time.Now().Weekday().String()
			header := fmt.Sprintf("---\ndate: %s\ntype: daily\ntags: [daily]\n---\n\n# %s — %s\n\n",
				today, today, weekday)
			recordErr("write daily note", atomicWriteNote(dailyPath, header+content))
		} else {
			newContent := replaceDailySection(string(existing), content, "## Daily Plan")
			recordErr("update daily note", atomicWriteNote(dailyPath, newContent))
		}

		// Route each slot through the unified schedule layer so task-slots
		// also get a ⏰ marker on their source line. Non-task kinds (break,
		// lunch, meeting, habit, review) only land in the planner file.
		for _, slot := range schedule {
			ref := scheduleRefForSlotText(slot.Task, allTasks)
			if isTaskSlot(slot.Type) && ref.hasLocation() {
				recordErr("schedule slot", SetTaskSchedule(vaultRoot, today, ref, slot.Start, slot.End, slot.Type))
			} else {
				recordErr("upsert planner block", UpsertPlannerBlock(vaultRoot, today, ScheduleRef{Text: slot.Task}, PlannerBlock{
					Date: today, StartTime: slot.Start, EndTime: slot.End,
					Text: slot.Task, BlockType: slot.Type, SourceRef: ref,
				}))
			}
		}

		// Write daily focus so calendar shows the goal
		writePlannerFocus(vaultRoot, today, todayGoal, selectedTasks)

		// Persist newly created tasks to Tasks.md so they appear in the task manager
		for _, ct := range createdTasks {
			recordErr("append created task", appendTaskLine(vaultRoot, "- [ ] "+ct))
		}

		return morningPlanSavedMsg{Err: firstErr}
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

	// Schedule (synced with tasks, events, habits)
	if len(mr.schedule) > 0 {
		b.WriteString("### Schedule\n\n")
		b.WriteString("| Time | Task | Type |\n")
		b.WriteString("|------|------|------|\n")
		for _, slot := range mr.schedule {
			b.WriteString(fmt.Sprintf("| %s-%s | %s | %s |\n", slot.Start, slot.End, slot.Task, slot.Type))
		}
		b.WriteString("\n")
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
	stepNames := []string{"Overview", "Scripture", "Goal", "Tasks", "Habits", "Thoughts", "Summary", "Done"}
	stepNum := int(mr.phase) + 1
	totalSteps := 7
	if stepNum > totalSteps {
		stepNum = totalSteps
	}
	now := time.Now()
	clockLabel := lipgloss.NewStyle().Foreground(green).Bold(true).Render(now.Format("15:04"))
	titleIcon := lipgloss.NewStyle().Foreground(blue).Render(IconCalendarChar)
	stepLabel := DimStyle.Render(fmt.Sprintf(" [%d/%d %s]", stepNum, totalSteps, stepNames[mr.phase]))
	headerLeft := "  " + titleIcon + headerStyle.Render(" Plan My Day") + stepLabel
	headerGap := width - 8 - lipgloss.Width(headerLeft) - lipgloss.Width(clockLabel)
	if headerGap < 1 {
		headerGap = 1
	}
	b.WriteString(headerLeft + strings.Repeat(" ", headerGap) + clockLabel + "\n")

	// Progress bar — uses block chars for clearer visual
	barW := innerW - 4
	filled := int(mr.phase) * barW / totalSteps
	if filled > barW {
		filled = barW
	}
	bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", barW-filled))
	b.WriteString("  " + bar + "\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", innerW-4)) + "\n\n")

	switch mr.phase {
	case morningOverview:
		mr.viewOverview(&b, innerW)
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
	back := "Shift+Tab back  "
	switch mr.phase {
	case morningOverview:
		return DimStyle.Render("  Enter next  j/k scroll  Esc close")
	case morningScripture:
		return DimStyle.Render("  Enter next  " + back + "Esc close")
	case morningGoal:
		return DimStyle.Render("  Type goal  Enter next  " + back + "Esc skip")
	case morningTasks:
		if mr.taskAddMode {
			return DimStyle.Render("  Type task  Enter add  Esc cancel")
		}
		selected := 0
		for _, s := range mr.taskSelected {
			if s {
				selected++
			}
		}
		return DimStyle.Render(fmt.Sprintf("  j/k move  Space toggle (%d)  a add  Enter next  %sEsc skip", selected, back))
	case morningHabits:
		return DimStyle.Render("  j/k move  Space toggle  Enter next  " + back + "Esc skip")
	case morningThoughts:
		return DimStyle.Render("  Type thoughts  Enter next  " + back + "Esc skip")
	case morningSummary:
		return DimStyle.Render("  Enter save  j/k scroll  Esc close")
	case morningComplete:
		return DimStyle.Render("  Enter close")
	}
	return ""
}

// ---------------------------------------------------------------------------
// Phase views
// ---------------------------------------------------------------------------

func (mr MorningRoutine) viewOverview(b *strings.Builder, w int) {
	titleStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	today := time.Now()

	b.WriteString("  " + titleStyle.Render("Good morning! Here's your day.") + "\n")
	b.WriteString("  " + DimStyle.Render(today.Format("Monday, January 2, 2006")) + "\n\n")

	// ── Calendar events ────────────────────────────────────────────────────
	if len(mr.events) > 0 {
		// Sort by time
		sortedEvents := make([]PlannerEvent, len(mr.events))
		copy(sortedEvents, mr.events)
		sort.Slice(sortedEvents, func(i, j int) bool {
			return sortedEvents[i].Time < sortedEvents[j].Time
		})

		// Compute total busy time
		busyMins := 0
		for _, e := range sortedEvents {
			if e.Time != "" && e.Duration > 0 {
				busyMins += e.Duration
			}
		}
		busySuffix := ""
		if busyMins > 0 {
			if busyMins >= 60 {
				busySuffix = fmt.Sprintf("  %dh%02dm busy", busyMins/60, busyMins%60)
			} else {
				busySuffix = fmt.Sprintf("  %dm busy", busyMins)
			}
		}
		b.WriteString("  " + labelStyle.Render(fmt.Sprintf("Today's Calendar (%d)", len(sortedEvents))) +
			DimStyle.Render(busySuffix) + "\n")

		for ei, e := range sortedEvents {
			if ei >= 6 {
				b.WriteString("    " + DimStyle.Render(fmt.Sprintf("+%d more events", len(sortedEvents)-6)) + "\n")
				break
			}
			timeStr := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("all-day")
			if e.Time != "" {
				h, m := 0, 0
				_, _ = fmt.Sscanf(e.Time, "%d:%d", &h, &m)
				endMins := h*60 + m + e.Duration
				timeRange := fmt.Sprintf("%s–%02d:%02d", e.Time, endMins/60, endMins%60)
				durStr := ""
				if e.Duration > 0 {
					durStr = fmt.Sprintf(" (%dm)", e.Duration)
				}
				timeStr = lipgloss.NewStyle().Foreground(teal).Render(timeRange) + DimStyle.Render(durStr)
			}
			b.WriteString("    " + timeStr + "  " +
				lipgloss.NewStyle().Foreground(text).Render(TruncateDisplay(e.Title, w-30)) + "\n")
		}
		b.WriteString("\n")
	} else {
		b.WriteString("  " + DimStyle.Render("No calendar events today") + "\n\n")
	}

	// ── Yesterday's carry-forward ──────────────────────────────────────────
	if len(mr.yesterdayTasks) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Bold(true).
			Render(fmt.Sprintf("Yesterday's Unfinished (%d)", len(mr.yesterdayTasks))) + "\n")
		for i, t := range mr.yesterdayTasks {
			if i >= 5 {
				b.WriteString("    " + DimStyle.Render(fmt.Sprintf("+%d more", len(mr.yesterdayTasks)-5)) + "\n")
				break
			}
			b.WriteString("    " + lipgloss.NewStyle().Foreground(peach).Render("↪ ") +
				DimStyle.Render(TruncateDisplay(t, w-10)) + "\n")
		}
		b.WriteString("\n")
	}

	// ── Active goals ───────────────────────────────────────────────────────
	activeGoals := 0
	for _, g := range mr.goals {
		if g.Status == GoalStatusActive {
			activeGoals++
		}
	}
	if activeGoals > 0 {
		b.WriteString("  " + labelStyle.Render(fmt.Sprintf("Goals (%d)", activeGoals)) + "\n")
		shown := 0
		for _, g := range mr.goals {
			if g.Status != GoalStatusActive {
				continue
			}
			if shown >= 5 {
				b.WriteString("    " + DimStyle.Render(fmt.Sprintf("+%d more", activeGoals-5)) + "\n")
				break
			}
			prog := g.Progress()
			barW := 6
			filled := barW * prog / 100
			bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled)) +
				lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", barW-filled))
			overdue := ""
			if g.IsOverdue() {
				overdue = lipgloss.NewStyle().Foreground(red).Render(" overdue")
			}
			b.WriteString("    " + bar + " " + DimStyle.Render(fmt.Sprintf("%3d%%", prog)) + " " +
				lipgloss.NewStyle().Foreground(text).Render(TruncateDisplay(g.Title, w-24)) + overdue + "\n")
			shown++
		}
		b.WriteString("\n")
	}

	// ── Active projects ────────────────────────────────────────────────────
	if len(mr.projects) > 0 {
		b.WriteString("  " + labelStyle.Render(fmt.Sprintf("Projects (%d)", len(mr.projects))) + "\n")
		for i, p := range mr.projects {
			if i >= 5 {
				b.WriteString("    " + DimStyle.Render(fmt.Sprintf("+%d more", len(mr.projects)-5)) + "\n")
				break
			}
			prog := p.Progress()
			pct := int(prog * 100)
			barW := 6
			filled := barW * pct / 100
			bar := lipgloss.NewStyle().Foreground(sapphire).Render(strings.Repeat("█", filled)) +
				lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", barW-filled))
			nextAction := ""
			if p.NextAction != "" {
				nextAction = DimStyle.Render("  → " + TruncateDisplay(p.NextAction, w-40))
			}
			b.WriteString("    " + bar + " " +
				lipgloss.NewStyle().Foreground(text).Render(TruncateDisplay(p.Name, 20)) + nextAction + "\n")
		}
		b.WriteString("\n")
	}

	// ── Existing schedule (from Plan My Day or previous session) ───────────
	if len(mr.existingBlocks) > 0 {
		b.WriteString("  " + labelStyle.Render(fmt.Sprintf("Already Scheduled (%d blocks)", len(mr.existingBlocks))) + "\n")
		for i, pb := range mr.existingBlocks {
			if i >= 5 {
				b.WriteString("    " + DimStyle.Render(fmt.Sprintf("+%d more", len(mr.existingBlocks)-5)) + "\n")
				break
			}
			timeStr := lipgloss.NewStyle().Foreground(teal).Render(pb.StartTime + "–" + pb.EndTime)
			typeCol := lavender
			switch pb.BlockType {
			case "task":
				typeCol = blue
			case "break":
				typeCol = green
			case "focus":
				typeCol = peach
			}
			typeTag := lipgloss.NewStyle().Foreground(typeCol).Render(" [" + pb.BlockType + "]")
			doneTag := ""
			if pb.Done {
				doneTag = lipgloss.NewStyle().Foreground(green).Render(" ✓")
			}
			b.WriteString("    " + timeStr + "  " +
				lipgloss.NewStyle().Foreground(text).Render(TruncateDisplay(pb.Text, w-30)) +
				typeTag + doneTag + "\n")
		}
		b.WriteString("\n")
	}

	// ── Existing focus ─────────────────────────────────────────────────────
	if mr.existingFocus.TopGoal != "" {
		focusPill := makePill(peach, "EXISTING GOAL")
		b.WriteString("  " + focusPill + " " +
			lipgloss.NewStyle().Foreground(text).Italic(true).Render(mr.existingFocus.TopGoal) + "\n\n")
	}

	// ── Task summary ───────────────────────────────────────────────────────
	todayStr := today.Format("2006-01-02")
	overdue, dueToday, upcoming, total := 0, 0, 0, 0
	for _, t := range mr.allTasks {
		total++
		if t.DueDate != "" && t.DueDate < todayStr {
			overdue++
		} else if t.DueDate == todayStr {
			dueToday++
		} else if t.DueDate != "" {
			upcoming++
		}
	}
	if total > 0 {
		b.WriteString("  " + labelStyle.Render(fmt.Sprintf("Tasks (%d open)", total)) + "  ")
		var taskParts []string
		if overdue > 0 {
			taskParts = append(taskParts, lipgloss.NewStyle().Foreground(red).Bold(true).Render(fmt.Sprintf("%d overdue", overdue)))
		}
		if dueToday > 0 {
			taskParts = append(taskParts, lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("%d today", dueToday)))
		}
		if upcoming > 0 {
			taskParts = append(taskParts, lipgloss.NewStyle().Foreground(lavender).Render(fmt.Sprintf("%d upcoming", upcoming)))
		}
		b.WriteString(strings.Join(taskParts, DimStyle.Render(" · ")) + "\n\n")
	}

	b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("Press Enter to start planning your day.") + "\n")
}

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

func (mr MorningRoutine) viewGoal(b *strings.Builder, w int) {
	questionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text)

	b.WriteString("  " + questionStyle.Render("What is your #1 goal for today?") + "\n\n")

	// Show active goals with progress as context
	hasGoals := false
	for _, g := range mr.goals {
		if g.Status == GoalStatusActive {
			if !hasGoals {
				b.WriteString("  " + labelStyle.Render("Your active goals:") + "\n")
				hasGoals = true
			}
			prog := g.Progress()
			barW := 6
			filled := barW * prog / 100
			bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled)) +
				lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", barW-filled))
			b.WriteString("    " + bar + " " + DimStyle.Render(fmt.Sprintf("%3d%%", prog)) + " " +
				lipgloss.NewStyle().Foreground(text).Render(TruncateDisplay(g.Title, w-20)) + "\n")
		}
	}
	if hasGoals {
		b.WriteString("\n")
	}

	// Show active projects
	if len(mr.projects) > 0 {
		b.WriteString("  " + labelStyle.Render("Active projects:") + "\n")
		for i, p := range mr.projects {
			if i >= 4 {
				b.WriteString("    " + DimStyle.Render(fmt.Sprintf("+%d more", len(mr.projects)-4)) + "\n")
				break
			}
			nextStr := ""
			if p.NextAction != "" {
				nextStr = DimStyle.Render("  → " + TruncateDisplay(p.NextAction, w-30))
			}
			b.WriteString("    " + lipgloss.NewStyle().Foreground(sapphire).Render("● ") +
				lipgloss.NewStyle().Foreground(text).Render(p.Name) + nextStr + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("  " + inputStyle.Render(mr.todayGoal) + lipgloss.NewStyle().Foreground(mauve).Render("█") + "\n")
}

func (mr MorningRoutine) viewTasks(b *strings.Builder, w int) {
	questionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	unselectedStyle := lipgloss.NewStyle().Foreground(overlay0)
	cursorStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)

	// Count selected
	selected := 0
	totalEst := 0
	for i, s := range mr.taskSelected {
		if s {
			selected++
			totalEst += mr.allTasks[i].EstimatedMinutes
		}
	}
	estStr := ""
	if totalEst > 0 {
		if totalEst >= 60 {
			estStr = fmt.Sprintf(" ~%dh%02dm", totalEst/60, totalEst%60)
		} else {
			estStr = fmt.Sprintf(" ~%dm", totalEst)
		}
	}
	b.WriteString("  " + questionStyle.Render("Select tasks to schedule today") +
		DimStyle.Render(fmt.Sprintf("  %d/%d selected%s", selected, len(mr.allTasks)+len(mr.createdTasks), estStr)) + "\n\n")

	if len(mr.allTasks) == 0 && len(mr.createdTasks) == 0 {
		b.WriteString("  " + DimStyle.Render("No pending tasks found. Press 'a' to add one.") + "\n")
	}

	today := time.Now().Format("2006-01-02")

	// Visible window for scrolling
	totalItems := len(mr.allTasks) + len(mr.createdTasks)
	visHeight := mr.height - 20
	if visHeight < 8 {
		visHeight = 8
	}
	scrollEnd := mr.taskScroll + visHeight
	if scrollEnd > totalItems {
		scrollEnd = totalItems
	}

	// Group headers
	lastGroup := ""
	for i := mr.taskScroll; i < scrollEnd; i++ {
		if i < len(mr.allTasks) {
			t := mr.allTasks[i]
			// Determine group
			group := ""
			switch {
			case t.DueDate != "" && t.DueDate < today:
				group = "OVERDUE"
			case t.DueDate == today:
				group = "TODAY"
			case t.DueDate != "" && tmDaysUntil(t.DueDate) <= 2:
				group = "NEXT 2 DAYS"
			case t.Priority >= 3:
				group = "HIGH PRIORITY"
			default:
				group = "OTHER"
			}
			if group != lastGroup {
				groupColors := map[string]lipgloss.Color{
					"OVERDUE": red, "TODAY": green, "NEXT 2 DAYS": lavender,
					"HIGH PRIORITY": peach, "OTHER": overlay1,
				}
				col := groupColors[group]
				pill := makePill(col, group)
				b.WriteString("  " + pill + "\n")
				lastGroup = group
			}

			prefix := "  "
			if i == mr.taskCursor {
				prefix = cursorStyle.Render("▸ ")
			}

			checkbox := unselectedStyle.Render("[ ] ")
			textStyle := unselectedStyle
			if mr.taskSelected[i] {
				checkbox = selectedStyle.Render("[x] ")
				textStyle = lipgloss.NewStyle().Foreground(text)
			}

			// Clean text — strip emoji markers for display
			displayText := TruncateDisplay(t.Text, w-20)

			// Right-side badges
			var badges []string
			if t.EstimatedMinutes > 0 {
				badges = append(badges, lipgloss.NewStyle().Foreground(sky).Render(fmt.Sprintf("~%dm", t.EstimatedMinutes)))
			}
			if t.Project != "" {
				badges = append(badges, lipgloss.NewStyle().Foreground(sapphire).Render(t.Project))
			}
			if t.DueDate != "" && t.DueDate != today && group != "OVERDUE" {
				badges = append(badges, DimStyle.Render(t.DueDate))
			}
			badgeStr := ""
			if len(badges) > 0 {
				badgeStr = " " + strings.Join(badges, DimStyle.Render("·"))
			}

			b.WriteString(prefix + checkbox + textStyle.Render(displayText) + badgeStr + "\n")
		} else {
			// Created tasks
			ci := i - len(mr.allTasks)
			if ci < len(mr.createdTasks) {
				ct := mr.createdTasks[ci]
				prefix := "  "
				if i == mr.taskCursor {
					prefix = cursorStyle.Render("▸ ")
				}
				checkbox := selectedStyle.Render("[+] ")
				b.WriteString(prefix + checkbox + lipgloss.NewStyle().Foreground(green).Render(TruncateDisplay(ct, w-14)) + "\n")
			}
		}
	}

	// Scroll indicator
	if totalItems > visHeight {
		pos := fmt.Sprintf("%d/%d", mr.taskCursor+1, totalItems)
		if mr.taskScroll > 0 {
			pos = "▲ " + pos
		}
		if scrollEnd < totalItems {
			pos += " ▼"
		}
		b.WriteString("  " + DimStyle.Render(pos) + "\n")
	}

	// Add mode
	if mr.taskAddMode {
		b.WriteString("\n")
		promptStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		b.WriteString("  " + promptStyle.Render("New task: ") + mr.newTaskInput + lipgloss.NewStyle().Foreground(mauve).Render("█") + "\n")
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
	b.WriteString("  " + inputStyle.Render(mr.thoughts) + lipgloss.NewStyle().Foreground(mauve).Render("█") + "\n")
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
		goalPill := makePill(peach, "GOAL")
		b.WriteString("  " + goalPill + " " + itemStyle.Bold(true).Render(mr.todayGoal) + "\n\n")
	}

	// Summary stats
	selectedTasks := mr.getSelectedTasks()
	selectedHabits := mr.getSelectedHabits()
	var statParts []string
	if len(selectedTasks) > 0 {
		statParts = append(statParts, lipgloss.NewStyle().Foreground(blue).Render(fmt.Sprintf("%d tasks", len(selectedTasks))))
	}
	if len(mr.events) > 0 {
		statParts = append(statParts, lipgloss.NewStyle().Foreground(lavender).Render(fmt.Sprintf("%d events", len(mr.events))))
	}
	if len(selectedHabits) > 0 {
		statParts = append(statParts, lipgloss.NewStyle().Foreground(green).Render(fmt.Sprintf("%d habits", len(selectedHabits))))
	}
	if len(statParts) > 0 {
		b.WriteString("  " + strings.Join(statParts, DimStyle.Render(" · ")) + "\n\n")
	}

	// ── DAILY TIMELINE ─────────────────────────────────────────────────────
	if len(mr.schedule) > 0 {
		timelineHeader := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  DAILY TIMELINE")
		b.WriteString(timelineHeader + "\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("─", w-6)) + "\n")

		now := time.Now()
		nowMins := now.Hour()*60 + now.Minute()
		contentW := w - 14
		if contentW < 20 {
			contentW = 20
		}

		// Determine timeline range
		timelineStart := 8 * 60
		if len(mr.schedule) > 0 {
			firstMin := slotToMinutes(mr.schedule[0].Start)
			if firstMin < timelineStart {
				timelineStart = (firstMin / 60) * 60
			}
		}
		timelineEnd := 22 * 60

		nowLineDrawn := false
		for mins := timelineStart; mins < timelineEnd; mins += 30 {
			hour := mins / 60
			half := mins % 60
			isTopHalf := half == 0
			isNowSlot := nowMins >= mins && nowMins < mins+30

			var timeSt string
			if isNowSlot {
				timeSt = lipgloss.NewStyle().Foreground(green).Bold(true).
					Render(fmt.Sprintf("  ▸%02d:%02d ", now.Hour(), now.Minute()))
			} else if isTopHalf {
				timeSt = DimStyle.Render(fmt.Sprintf("  %02d:00  ", hour))
			} else {
				timeSt = DimStyle.Render("     :30  ")
			}

			// Find active schedule slot
			var activeSlot *daySlot
			for i := range mr.schedule {
				s := &mr.schedule[i]
				sMin := slotToMinutes(s.Start)
				eMin := slotToMinutes(s.End)
				if sMin < mins+30 && eMin > mins {
					activeSlot = s
					break
				}
			}

			if activeSlot != nil {
				sMin := slotToMinutes(activeSlot.Start)
				isStart := sMin >= mins && sMin < mins+30

				var blockColor lipgloss.Color
				switch activeSlot.Type {
				case "deep-work":
					blockColor = blue
				case "admin":
					blockColor = sapphire
				case "meeting":
					blockColor = lavender
				case "break":
					blockColor = green
				case "habit":
					blockColor = teal
				case "review":
					blockColor = peach
				default:
					blockColor = blue
				}

				inner := contentW - 2
				if inner < 4 {
					inner = 4
				}
				var label string
				if isStart {
					label = activeSlot.Start + "–" + activeSlot.End + "  " + activeSlot.Task
					if activeSlot.Project != "" {
						label += "  [" + activeSlot.Project + "]"
					}
				} else {
					label = "▏" + activeSlot.Task
				}
				label = TruncateDisplay(label, inner)
				padLen := inner - lipgloss.Width(label)
				if padLen < 0 {
					padLen = 0
				}
				label += strings.Repeat(" ", padLen)
				blockStyle := lipgloss.NewStyle().Foreground(crust).Background(blockColor)
				if isStart {
					blockStyle = blockStyle.Bold(true)
				}
				b.WriteString(timeSt + blockStyle.Render(" "+label+" ") + "\n")
			} else if isTopHalf {
				b.WriteString(timeSt + DimStyle.Render("┊") + "\n")
			} else {
				b.WriteString(timeSt + DimStyle.Render("·") + "\n")
			}

			if !nowLineDrawn && isNowSlot {
				nowLineDrawn = true
				nowLabel := lipgloss.NewStyle().Foreground(red).Bold(true).
					Render(fmt.Sprintf("  %02d:%02d  ", now.Hour(), now.Minute()))
				nowLine := lipgloss.NewStyle().Foreground(red).Bold(true).
					Render(strings.Repeat("━", contentW))
				b.WriteString(nowLabel + nowLine + "\n")
			}
		}
		// Timeline legend
		legend := "  " +
			lipgloss.NewStyle().Foreground(blue).Render("█") + " work " +
			lipgloss.NewStyle().Foreground(lavender).Render("█") + " meeting " +
			lipgloss.NewStyle().Foreground(green).Render("█") + " break " +
			lipgloss.NewStyle().Foreground(teal).Render("█") + " habit " +
			lipgloss.NewStyle().Foreground(peach).Render("█") + " review"
		b.WriteString(legend + "\n\n")
	}

	// ── Goals progress ─────────────────────────────────────────────────────
	if len(mr.goals) > 0 {
		b.WriteString(DimStyle.Render("  " + strings.Repeat("─", w-6)) + "\n")
		b.WriteString("  " + labelStyle.Render("Goals") + "\n")
		for i, g := range mr.goals {
			if i >= 5 {
				b.WriteString(DimStyle.Render(fmt.Sprintf("  +%d more", len(mr.goals)-5)) + "\n")
				break
			}
			prog := g.Progress()
			barW := 6
			filled := barW * prog / 100
			bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("█", filled)) +
				lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("░", barW-filled))
			b.WriteString("  " + bar + " " + DimStyle.Render(fmt.Sprintf("%3d%%", prog)) + " " +
				itemStyle.Render(TruncateDisplay(g.Title, w-20)) + "\n")
		}
		b.WriteString("\n")
	}

	// Thoughts
	if mr.thoughts != "" {
		b.WriteString(DimStyle.Render("  " + strings.Repeat("─", w-6)) + "\n")
		b.WriteString("  " + labelStyle.Render("Thoughts") + "\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(text).Italic(true).Render("  "+mr.thoughts) + "\n\n")
	}

	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", w-6)) + "\n")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Bold(true).Render("Press Enter to save this plan to your daily note.") + "\n")
}

func (mr MorningRoutine) viewComplete(b *strings.Builder, _ int) {
	successPill := makePill(green, "SAVED")
	b.WriteString("  " + successPill + lipgloss.NewStyle().Foreground(green).Bold(true).Render(" Day plan saved to daily note!") + "\n\n")

	selected := len(mr.getSelectedTasks())
	habits := len(mr.getSelectedHabits())
	var parts []string
	if selected > 0 {
		parts = append(parts, fmt.Sprintf("%d tasks", selected))
	}
	if len(mr.events) > 0 {
		parts = append(parts, fmt.Sprintf("%d events", len(mr.events)))
	}
	if habits > 0 {
		parts = append(parts, fmt.Sprintf("%d habits", habits))
	}
	if len(mr.schedule) > 0 {
		parts = append(parts, fmt.Sprintf("%d time blocks", len(mr.schedule)))
	}
	if len(parts) > 0 {
		b.WriteString("  " + DimStyle.Render(strings.Join(parts, " · ")) + "\n")
	}
	if len(mr.schedule) > 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(teal).Render(
			mr.schedule[0].Start+" → "+mr.schedule[len(mr.schedule)-1].End) + "\n")
	}
	if mr.todayGoal != "" {
		goalPill := makePill(peach, "GOAL")
		b.WriteString("  " + goalPill + " " + lipgloss.NewStyle().Foreground(text).Render(mr.todayGoal) + "\n")
	}
	b.WriteString("\n  " + DimStyle.Render("Go make today count.") + "\n")
	b.WriteString("\n  " + lipgloss.NewStyle().Foreground(mauve).Render(
		"P") + DimStyle.Render(" refine with AI  ") +
		lipgloss.NewStyle().Foreground(mauve).Render("Enter") + DimStyle.Render(" close") + "\n")
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
