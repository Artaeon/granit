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
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type triageResultMsg struct {
	recommendations []triageRecommendation
	err             error
}

type triageTickMsg struct{}

func triageTickCmd() tea.Cmd {
	return tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
		return triageTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

type triageRecommendation struct {
	TaskText string
	Reason   string // why this task should be done today
	Priority int    // AI-suggested priority (1-5)
	TimeEst  string // estimated time
}

type staleTaskInfo struct {
	TaskText   string
	DaysOpen   int
	Suggestion string // "finish", "delegate", "delete", "break down"
}

// ---------------------------------------------------------------------------
// TaskTriage overlay
// ---------------------------------------------------------------------------

// TaskTriage is an AI-powered daily task prioritization overlay.
// It analyzes open tasks, calendar events, and active goals to recommend
// the top tasks to focus on today, and flags stale tasks that need attention.
type TaskTriage struct {
	active bool
	width  int
	height int

	ai        AIConfig
	vaultRoot string

	// Input data
	allTasks    []Task
	todayEvents []string // today's calendar events (simple strings)
	activeGoals []Goal   // active goals for context

	// AI result
	recommendations []triageRecommendation
	staleTasks      []staleTaskInfo

	// UI state
	phase  int // 0=loading, 1=results
	cursor int
	scroll int
	tab    int // 0=today's focus, 1=stale tasks

	// Loading
	loading     bool
	loadingTick int

	errMsg string
}

// NewTaskTriage creates a new TaskTriage overlay.
func NewTaskTriage() TaskTriage {
	return TaskTriage{}
}

// IsActive reports whether the overlay is currently displayed.
func (tt TaskTriage) IsActive() bool { return tt.active }

// SetSize updates the available terminal dimensions.
func (tt *TaskTriage) SetSize(w, h int) {
	tt.width = w
	tt.height = h
}

// Open activates the triage overlay and starts analysis.
func (tt *TaskTriage) Open(vaultRoot string, tasks []Task, goals []Goal, cfg AIConfig) tea.Cmd {
	tt.active = true
	tt.vaultRoot = vaultRoot
	tt.ai = cfg
	tt.phase = 0
	tt.cursor = 0
	tt.scroll = 0
	tt.tab = 0
	tt.loading = true
	tt.loadingTick = 0
	tt.errMsg = ""
	tt.recommendations = nil
	tt.staleTasks = nil
	tt.todayEvents = nil

	// Filter to only open tasks
	tt.allTasks = nil
	for _, t := range tasks {
		if !t.Done {
			tt.allTasks = append(tt.allTasks, t)
		}
	}

	// Filter to active goals
	tt.activeGoals = nil
	for _, g := range goals {
		if g.Status == GoalStatusActive {
			tt.activeGoals = append(tt.activeGoals, g)
		}
	}

	// Compute stale tasks locally (no AI needed)
	tt.computeStaleTasks()

	// Start AI triage
	return tea.Batch(tt.triageCmd(), triageTickCmd())
}

// Close deactivates the overlay.
func (tt *TaskTriage) Close() { tt.active = false }

// ---------------------------------------------------------------------------
// Stale task detection (local, no AI)
// ---------------------------------------------------------------------------

func (tt *TaskTriage) computeStaleTasks() {
	tt.staleTasks = nil
	now := time.Now()

	for _, t := range tt.allTasks {
		if t.Done {
			continue
		}

		// Tasks with due date passed > 7 days ago
		if t.DueDate != "" {
			due, err := time.Parse("2006-01-02", t.DueDate)
			if err == nil && due.Before(now) {
				daysOverdue := int(now.Sub(due).Hours() / 24)
				if daysOverdue > 7 {
					suggestion := "finish"
					if daysOverdue > 30 {
						suggestion = "delete"
					} else if daysOverdue > 14 {
						suggestion = "break down"
					}
					tt.staleTasks = append(tt.staleTasks, staleTaskInfo{
						TaskText:   t.Text,
						DaysOpen:   daysOverdue,
						Suggestion: suggestion,
					})
				}
			}
			continue
		}

		// Tasks with no due date — check source file modification time
		// as a heuristic for how long the task has existed
		if t.NotePath != "" && tt.vaultRoot != "" {
			fullPath := filepath.Join(tt.vaultRoot, t.NotePath)
			info, err := os.Stat(fullPath)
			if err == nil {
				daysOld := int(now.Sub(info.ModTime()).Hours() / 24)
				if daysOld > 14 {
					var suggestion string
					switch {
					case daysOld > 60:
						suggestion = "delete"
					case daysOld > 30:
						suggestion = "delegate"
					default:
						suggestion = "break down"
					}
					tt.staleTasks = append(tt.staleTasks, staleTaskInfo{
						TaskText:   t.Text,
						DaysOpen:   daysOld,
						Suggestion: suggestion,
					})
				}
			}
		}

	}
}

// ---------------------------------------------------------------------------
// Prompt builder
// ---------------------------------------------------------------------------

func (tt *TaskTriage) buildPrompt() (string, string) {
	now := time.Now()

	systemPrompt := "You are a productivity coach. Analyze the user's open tasks and suggest the top 5 tasks they should focus on today. Be specific about WHY each task matters today."

	var b strings.Builder

	b.WriteString(fmt.Sprintf("TODAY: %s, %s\n\n", now.Format("2006-01-02"), now.Weekday().String()))

	b.WriteString("OPEN TASKS:\n")
	for i, t := range tt.allTasks {
		if i >= 50 {
			b.WriteString(fmt.Sprintf("... and %d more tasks\n", len(tt.allTasks)-50))
			break
		}
		line := fmt.Sprintf("%d. %s", i+1, t.Text)
		if t.Priority > 0 {
			pNames := []string{"", "low", "medium", "high", "highest"}
			if t.Priority < len(pNames) {
				line += fmt.Sprintf(" [priority: %s]", pNames[t.Priority])
			}
		}
		if t.DueDate != "" {
			line += fmt.Sprintf(" [due: %s]", t.DueDate)
		}
		if len(t.Tags) > 0 {
			line += fmt.Sprintf(" [tags: %s]", strings.Join(t.Tags, ", "))
		}
		if t.EstimatedMinutes > 0 {
			line += fmt.Sprintf(" [est: %dmin]", t.EstimatedMinutes)
		}
		b.WriteString(line + "\n")
	}

	if len(tt.activeGoals) > 0 {
		b.WriteString("\nACTIVE GOALS:\n")
		for _, g := range tt.activeGoals {
			progress := 0
			if len(g.Milestones) > 0 {
				done := 0
				for _, m := range g.Milestones {
					if m.Done {
						done++
					}
				}
				progress = done * 100 / len(g.Milestones)
			}
			b.WriteString(fmt.Sprintf("- %s (%d%% complete)\n", g.Title, progress))
		}
	}

	b.WriteString("\nTODAY'S SCHEDULE:\n")
	if len(tt.todayEvents) > 0 {
		for _, ev := range tt.todayEvents {
			b.WriteString("- " + ev + "\n")
		}
	} else {
		b.WriteString("No events scheduled\n")
	}

	b.WriteString(`
Pick the 5 most important tasks for today. Consider:
- Overdue tasks (highest urgency)
- Tasks due today or tomorrow
- Tasks that align with active goals
- Quick wins that can build momentum
- Context: what meetings/events affect the day

Format each recommendation EXACTLY like this:
TASK: {exact task text}
REASON: {1-line why this matters today}
PRIORITY: {1-5, where 5 is most critical}
TIME: {estimated minutes}
---
`)

	return systemPrompt, b.String()
}

// ---------------------------------------------------------------------------
// AI dispatch
// ---------------------------------------------------------------------------

func (tt *TaskTriage) triageCmd() tea.Cmd {
	if len(tt.allTasks) == 0 {
		return func() tea.Msg {
			return triageResultMsg{recommendations: nil}
		}
	}

	ai := tt.ai
	systemPrompt, userPrompt := tt.buildPrompt()

	return func() tea.Msg {
		resp, err := ai.Chat(systemPrompt, userPrompt)
		if err != nil {
			return triageResultMsg{err: err}
		}
		recs := parseTriageResponse(resp)
		return triageResultMsg{recommendations: recs}
	}
}

// ---------------------------------------------------------------------------
// Response parser
// ---------------------------------------------------------------------------

func parseTriageResponse(response string) []triageRecommendation {
	var recs []triageRecommendation
	blocks := strings.Split(response, "---")

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		var rec triageRecommendation
		lines := strings.Split(block, "\n")
		hasTask := false

		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "TASK:") {
				rec.TaskText = strings.TrimSpace(strings.TrimPrefix(line, "TASK:"))
				hasTask = true
			} else if strings.HasPrefix(line, "REASON:") {
				rec.Reason = strings.TrimSpace(strings.TrimPrefix(line, "REASON:"))
			} else if strings.HasPrefix(line, "PRIORITY:") {
				val := strings.TrimSpace(strings.TrimPrefix(line, "PRIORITY:"))
				if p, err := strconv.Atoi(val); err == nil {
					rec.Priority = p
				}
			} else if strings.HasPrefix(line, "TIME:") {
				rec.TimeEst = strings.TrimSpace(strings.TrimPrefix(line, "TIME:"))
			}
		}

		if hasTask && rec.TaskText != "" {
			if rec.Priority < 1 {
				rec.Priority = 3
			}
			if rec.Priority > 5 {
				rec.Priority = 5
			}
			if rec.TimeEst == "" {
				rec.TimeEst = "30min"
			}
			recs = append(recs, rec)
		}
	}

	// Limit to 5 recommendations
	if len(recs) > 5 {
		recs = recs[:5]
	}

	return recs
}

// ---------------------------------------------------------------------------
// Local fallback (no AI)
// ---------------------------------------------------------------------------

func (tt *TaskTriage) localFallback() []triageRecommendation {
	now := time.Now()
	today := now.Format("2006-01-02")
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")

	type scored struct {
		task  Task
		score int
	}

	var items []scored
	for _, t := range tt.allTasks {
		s := 0

		// Overdue tasks get highest score
		if t.DueDate != "" {
			due, err := time.Parse("2006-01-02", t.DueDate)
			if err == nil {
				if due.Before(now) {
					daysOverdue := int(now.Sub(due).Hours() / 24)
					s += 100 + daysOverdue*10 // heavily prioritize overdue
				} else if t.DueDate == today {
					s += 80
				} else if t.DueDate == tomorrow {
					s += 60
				} else {
					// Tasks due within a week
					daysUntil := int(due.Sub(now).Hours() / 24)
					if daysUntil <= 7 {
						s += 40 - daysUntil*3
					}
				}
			}
		}

		// Priority boost
		s += t.Priority * 15

		// Goal-aligned tasks get a boost
		if t.GoalID != "" {
			s += 20
		}

		// Quick wins (< 30 min) get a small boost
		if t.EstimatedMinutes > 0 && t.EstimatedMinutes <= 30 {
			s += 10
		}

		items = append(items, scored{task: t, score: s})
	}

	// Sort by score descending
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})

	// Take top 5
	limit := 5
	if len(items) < limit {
		limit = len(items)
	}

	var recs []triageRecommendation
	for _, it := range items[:limit] {
		t := it.task
		reason := "Open task"

		if t.DueDate != "" {
			due, err := time.Parse("2006-01-02", t.DueDate)
			if err == nil {
				if due.Before(now) {
					daysOverdue := int(now.Sub(due).Hours() / 24)
					reason = fmt.Sprintf("Overdue by %d days", daysOverdue)
				} else if t.DueDate == today {
					reason = "Due today"
				} else if t.DueDate == tomorrow {
					reason = "Due tomorrow"
				} else {
					daysUntil := int(due.Sub(now).Hours() / 24)
					reason = fmt.Sprintf("Due in %d days", daysUntil)
				}
			}
		} else if t.Priority >= 3 {
			reason = "High priority task"
		} else if t.GoalID != "" {
			reason = "Aligns with active goal"
		} else if t.EstimatedMinutes > 0 && t.EstimatedMinutes <= 30 {
			reason = "Quick win — build momentum"
		}

		pri := t.Priority
		if pri == 0 {
			pri = 3
		}
		if pri > 5 {
			pri = 5
		}

		est := "30min"
		if t.EstimatedMinutes > 0 {
			est = fmt.Sprintf("%dmin", t.EstimatedMinutes)
		}

		recs = append(recs, triageRecommendation{
			TaskText: t.Text,
			Reason:   reason,
			Priority: pri,
			TimeEst:  est,
		})
	}

	return recs
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles messages for the triage overlay.
func (tt TaskTriage) Update(msg tea.Msg) (TaskTriage, tea.Cmd) {
	if !tt.active {
		return tt, nil
	}

	switch msg := msg.(type) {
	case triageTickMsg:
		if tt.phase == 0 && tt.loading {
			tt.loadingTick++
			return tt, triageTickCmd()
		}

	case triageResultMsg:
		tt.loading = false
		if msg.err != nil {
			// AI failed, fall back to local
			tt.errMsg = msg.err.Error()
			tt.recommendations = tt.localFallback()
		} else {
			tt.recommendations = msg.recommendations
			if len(tt.recommendations) == 0 {
				tt.recommendations = tt.localFallback()
			}
		}
		tt.phase = 1
		tt.cursor = 0
		tt.scroll = 0
		return tt, nil

	case tea.KeyMsg:
		switch tt.phase {
		case 0:
			// Loading phase — only esc
			if msg.String() == "esc" {
				tt.active = false
			}
			return tt, nil
		case 1:
			return tt.updateResults(msg)
		}
	}
	return tt, nil
}

// updateResults handles key events in the results phase.
func (tt TaskTriage) updateResults(msg tea.KeyMsg) (TaskTriage, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		tt.active = false

	case "tab":
		tt.tab = (tt.tab + 1) % 2
		tt.cursor = 0
		tt.scroll = 0

	case "j", "down":
		maxItems := tt.currentListLen()
		if tt.cursor < maxItems-1 {
			tt.cursor++
		}
		tt.adjustScroll()

	case "k", "up":
		if tt.cursor > 0 {
			tt.cursor--
		}
		tt.adjustScroll()
	}

	return tt, nil
}

// currentListLen returns the number of items in the current tab.
func (tt TaskTriage) currentListLen() int {
	if tt.tab == 0 {
		return len(tt.recommendations)
	}
	return len(tt.staleTasks)
}

// adjustScroll keeps the cursor visible in the viewport.
func (tt *TaskTriage) adjustScroll() {
	maxVisible := tt.maxVisible()
	if tt.cursor < tt.scroll {
		tt.scroll = tt.cursor
	}
	if tt.cursor >= tt.scroll+maxVisible {
		tt.scroll = tt.cursor - maxVisible + 1
	}
}

func (tt TaskTriage) maxVisible() int {
	v := tt.height/3 - 4
	if v < 3 {
		v = 3
	}
	return v
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the triage overlay.
func (tt TaskTriage) View() string {
	if !tt.active {
		return ""
	}

	width := tt.overlayWidth()

	switch tt.phase {
	case 0:
		return tt.viewLoading(width)
	case 1:
		return tt.viewResults(width)
	}
	return ""
}

func (tt TaskTriage) overlayWidth() int {
	w := tt.width * 2 / 3
	if w < 54 {
		w = 54
	}
	if w > 90 {
		w = 90
	}
	return w
}

func (tt TaskTriage) renderBorder(content string, width int) string {
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)
	return border.Render(content)
}

// ---------------------------------------------------------------------------
// View: loading phase
// ---------------------------------------------------------------------------

func (tt TaskTriage) viewLoading(width int) string {
	var buf strings.Builder

	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconBotChar + " Smart Task Triage")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)))
	buf.WriteString("\n\n")

	spinFrames := []string{"|", "/", "-", "\\"}
	frame := spinFrames[tt.loadingTick%len(spinFrames)]

	providerLabel := "local algorithm"
	switch tt.ai.Provider {
	case "ollama":
		providerLabel = "Ollama (" + tt.ai.ModelOrDefault("qwen2.5:0.5b") + ")"
	case "openai":
		providerLabel = "OpenAI (" + tt.ai.ModelOrDefault("gpt-4o-mini") + ")"
	case "nous":
		providerLabel = "Nous"
	case "nerve":
		providerLabel = "Nerve"
	case "claude":
		providerLabel = "Claude Code"
	}

	buf.WriteString(lipgloss.NewStyle().Foreground(yellow).Bold(true).
		Render(fmt.Sprintf("  %s Analyzing your tasks with %s...", frame, providerLabel)))
	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render(fmt.Sprintf("  %d open tasks to triage", len(tt.allTasks))))
	if len(tt.activeGoals) > 0 {
		buf.WriteString(DimStyle.Render(fmt.Sprintf(", %d active goals", len(tt.activeGoals))))
	}
	buf.WriteString("\n\n")
	buf.WriteString(DimStyle.Render("  Esc: cancel"))

	return tt.renderBorder(buf.String(), width)
}

// ---------------------------------------------------------------------------
// View: results phase
// ---------------------------------------------------------------------------

func (tt TaskTriage) viewResults(width int) string {
	var buf strings.Builder

	now := time.Now()

	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconBotChar + " Smart Task Triage")
	buf.WriteString(title)
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)))
	buf.WriteString("\n\n")

	// Error message if AI failed
	if tt.errMsg != "" {
		buf.WriteString(lipgloss.NewStyle().Foreground(yellow).Italic(true).
			Render("  AI unavailable, using local fallback: "+tt.errMsg) + "\n\n")
	}

	// Tab bar
	tab0Style := DimStyle
	tab1Style := DimStyle
	if tt.tab == 0 {
		tab0Style = lipgloss.NewStyle().Foreground(mauve).Bold(true)
	} else {
		tab1Style = lipgloss.NewStyle().Foreground(mauve).Bold(true)
	}

	staleCount := len(tt.staleTasks)
	tab0Label := fmt.Sprintf("  Today's Focus (%d)", len(tt.recommendations))
	tab1Label := fmt.Sprintf("  Stale Tasks (%d)", staleCount)
	buf.WriteString(tab0Style.Render(tab0Label))
	buf.WriteString("  ")
	buf.WriteString(tab1Style.Render(tab1Label))
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  " + strings.Repeat(string(ThemeSeparator), width-10)))
	buf.WriteString("\n\n")

	if tt.tab == 0 {
		buf.WriteString(tt.viewFocusTab(width, now))
	} else {
		buf.WriteString(tt.viewStaleTab(width))
	}

	// Footer
	buf.WriteString("\n")
	buf.WriteString(DimStyle.Render("  Tab: switch tabs  j/k: navigate  Esc: close"))

	return tt.renderBorder(buf.String(), width)
}

// viewFocusTab renders the "Today's Focus" tab content.
func (tt TaskTriage) viewFocusTab(width int, now time.Time) string {
	var buf strings.Builder

	dateStr := now.Format("Mon, Jan 2 2006")
	buf.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).
		Render(fmt.Sprintf("  Today's Focus — %s", dateStr)) + "\n\n")

	if len(tt.recommendations) == 0 {
		buf.WriteString(DimStyle.Render("  No task recommendations available.") + "\n")
		return buf.String()
	}

	maxVisible := tt.maxVisible()
	startIdx := tt.scroll
	endIdx := startIdx + maxVisible
	if endIdx > len(tt.recommendations) {
		endIdx = len(tt.recommendations)
	}

	for i := startIdx; i < endIdx; i++ {
		rec := tt.recommendations[i]
		selected := i == tt.cursor

		// Priority color
		priColor := tt.priorityColor(rec.Priority)
		priStr := lipgloss.NewStyle().Foreground(priColor).Bold(true).
			Render(fmt.Sprintf(" %d ", rec.Priority))

		// Task text
		taskStyle := lipgloss.NewStyle().Foreground(text)
		if selected {
			taskStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
		}

		// Pointer
		pointer := "  "
		if selected {
			pointer = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
		}

		// Time estimate on the right
		timeStr := lipgloss.NewStyle().Foreground(teal).Render(rec.TimeEst)

		// Task line: measure available width
		taskTextWidth := width - 16 // account for pointer, priority, time, padding
		displayText := rec.TaskText
		if len(displayText) > taskTextWidth && taskTextWidth > 3 {
			displayText = displayText[:taskTextWidth-3] + "..."
		}

		buf.WriteString(pointer + priStr + " " + taskStyle.Render(displayText))
		// Pad to push time estimate to the right
		taskRenderedLen := len(pointer) + 4 + 1 + len(displayText) // approximate
		padding := width - taskRenderedLen - len(rec.TimeEst) - 6
		if padding < 1 {
			padding = 1
		}
		buf.WriteString(strings.Repeat(" ", padding) + timeStr)
		buf.WriteString("\n")

		// Reason below, indented
		if rec.Reason != "" {
			reason := DimStyle.Render("      " + rec.Reason)
			buf.WriteString(reason + "\n")
		}

		// Spacing between items
		if i < endIdx-1 {
			buf.WriteString("\n")
		}
	}

	// Scroll indicator
	if len(tt.recommendations) > maxVisible {
		buf.WriteString(DimStyle.Render(fmt.Sprintf("\n  %d of %d tasks shown", endIdx-startIdx, len(tt.recommendations))))
	}

	return buf.String()
}

// viewStaleTab renders the "Stale Tasks" tab content.
func (tt TaskTriage) viewStaleTab(width int) string {
	var buf strings.Builder

	buf.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).
		Render("  Stale Tasks — needs attention") + "\n\n")

	if len(tt.staleTasks) == 0 {
		buf.WriteString(lipgloss.NewStyle().Foreground(green).Render("  No stale tasks! Your backlog is clean.") + "\n")
		return buf.String()
	}

	maxVisible := tt.maxVisible()
	startIdx := tt.scroll
	endIdx := startIdx + maxVisible
	if endIdx > len(tt.staleTasks) {
		endIdx = len(tt.staleTasks)
	}

	for i := startIdx; i < endIdx; i++ {
		st := tt.staleTasks[i]
		selected := i == tt.cursor

		// Pointer
		pointer := "  "
		if selected {
			pointer = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("> ")
		}

		// Task text
		taskStyle := lipgloss.NewStyle().Foreground(text)
		if selected {
			taskStyle = lipgloss.NewStyle().Foreground(peach).Bold(true)
		}

		// Days indicator
		daysColor := yellow
		if st.DaysOpen > 30 {
			daysColor = red
		}
		daysStr := lipgloss.NewStyle().Foreground(daysColor).Bold(true).
			Render(fmt.Sprintf("%dd", st.DaysOpen))

		displayText := st.TaskText
		taskTextWidth := width - 20
		if len(displayText) > taskTextWidth && taskTextWidth > 3 {
			displayText = displayText[:taskTextWidth-3] + "..."
		}

		buf.WriteString(pointer + daysStr + " " + taskStyle.Render(displayText) + "\n")

		// Suggestion below
		suggColor := tt.suggestionColor(st.Suggestion)
		suggLabel := lipgloss.NewStyle().Foreground(suggColor).Italic(true).
			Render("      Suggestion: " + st.Suggestion)
		buf.WriteString(suggLabel + "\n")

		if i < endIdx-1 {
			buf.WriteString("\n")
		}
	}

	// Scroll indicator
	if len(tt.staleTasks) > maxVisible {
		buf.WriteString(DimStyle.Render(fmt.Sprintf("\n  %d of %d stale tasks shown", endIdx-startIdx, len(tt.staleTasks))))
	}

	return buf.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (tt TaskTriage) priorityColor(pri int) lipgloss.Color {
	switch pri {
	case 5:
		return red
	case 4:
		return peach
	case 3:
		return yellow
	case 2:
		return blue
	default:
		return subtext0
	}
}

func (tt TaskTriage) suggestionColor(s string) lipgloss.Color {
	switch s {
	case "finish":
		return green
	case "delegate":
		return blue
	case "delete":
		return red
	case "break down":
		return yellow
	default:
		return subtext0
	}
}
