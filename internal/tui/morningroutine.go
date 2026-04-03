package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Phases
// ---------------------------------------------------------------------------

type morningPhase int

const (
	morningDevotional morningPhase = iota // Step 1: Scripture + reflection
	morningBriefing                       // Step 2: DEEPCOVEN briefing
	morningPlan                           // Step 3: Plan My Day
	morningPriorities                     // Step 4: Confirm top 3 priorities
	morningComplete                       // Done
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type morningDevotionalMsg struct {
	reflection string
	err        error
}

type morningBriefingMsg struct {
	content string
	err     error
}

type morningPlanMsg struct {
	response string
	err      error
}

type morningTickMsg struct{}

func morningTickCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return morningTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// Overlay
// ---------------------------------------------------------------------------

// MorningRoutine is a guided 4-step morning ritual that chains scripture
// devotional, DEEPCOVEN briefing, plan-my-day, and priority confirmation.
type MorningRoutine struct {
	active bool
	width  int
	height int
	phase  morningPhase
	scroll int

	// Loading state
	loading     bool
	loadingTick int

	// Step results
	scripture  Scripture
	reflection string // devotional AI reflection
	briefing   string // DEEPCOVEN morning briefing
	planResult string // raw AI plan response
	priorities []string

	// Data gathered at open
	vaultRoot     string
	ai            AIConfig
	goals         []Goal
	noteContents  map[string]string
	notePaths     []string
	tasks         []Task
	events        []PlannerEvent
	habits        []habitEntry
	projects      []Project
	yesterdayTasks []string

	// Per-session "already done" tracking
	dayPlanned bool // passed from statusbar at open time
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

// Open gathers all data and starts the first AI step.
func (mr *MorningRoutine) Open(
	vaultRoot string,
	ai AIConfig,
	goals []Goal,
	noteContents map[string]string,
	notePaths []string,
	tasks []Task,
	events []PlannerEvent,
	habits []habitEntry,
	projects []Project,
	yesterdayTasks []string,
	dayPlanned bool,
) tea.Cmd {
	mr.active = true
	mr.vaultRoot = vaultRoot
	mr.ai = ai
	mr.goals = goals
	mr.noteContents = noteContents
	mr.notePaths = notePaths
	mr.tasks = tasks
	mr.events = events
	mr.habits = habits
	mr.projects = projects
	mr.yesterdayTasks = yesterdayTasks
	mr.dayPlanned = dayPlanned

	mr.phase = morningDevotional
	mr.scroll = 0
	mr.reflection = ""
	mr.briefing = ""
	mr.planResult = ""
	mr.priorities = nil
	mr.loading = true
	mr.loadingTick = 0
	mr.scripture = DailyScripture(vaultRoot)

	return tea.Batch(mr.startDevotional(), morningTickCmd())
}

// Update handles messages.
func (mr MorningRoutine) Update(msg tea.Msg) (MorningRoutine, tea.Cmd) {
	if !mr.active {
		return mr, nil
	}

	switch msg := msg.(type) {
	case morningDevotionalMsg:
		mr.loading = false
		if msg.err != nil {
			mr.reflection = "AI unavailable: " + msg.err.Error()
		} else {
			mr.reflection = msg.reflection
		}
		return mr, nil

	case morningBriefingMsg:
		mr.loading = false
		if msg.err != nil {
			mr.briefing = "AI unavailable: " + msg.err.Error()
		} else {
			mr.briefing = msg.content
		}
		return mr, nil

	case morningPlanMsg:
		mr.loading = false
		if msg.err != nil {
			mr.planResult = "AI unavailable: " + msg.err.Error()
		} else {
			mr.planResult = msg.response
			mr.extractPriorities()
		}
		return mr, nil

	case morningTickMsg:
		if mr.loading {
			mr.loadingTick++
			return mr, morningTickCmd()
		}
		return mr, nil

	case tea.KeyMsg:
		key := msg.String()
		switch key {
		case "q":
			mr.active = false
			return mr, nil
		case "esc":
			// Skip current step
			return mr.advancePhase()
		case "enter":
			if mr.loading {
				return mr, nil // can't advance while loading
			}
			return mr.advancePhase()
		case "j", "down":
			mr.scroll++
		case "k", "up":
			if mr.scroll > 0 {
				mr.scroll--
			}
		}
	}
	return mr, nil
}

// advancePhase moves to the next step, starting its AI call.
func (mr MorningRoutine) advancePhase() (MorningRoutine, tea.Cmd) {
	mr.scroll = 0
	mr.loading = true
	mr.loadingTick = 0

	switch mr.phase {
	case morningDevotional:
		mr.phase = morningBriefing
		return mr, tea.Batch(mr.startBriefing(), morningTickCmd())
	case morningBriefing:
		if mr.dayPlanned {
			// Plan already done, skip to priorities
			mr.phase = morningPriorities
			mr.loading = false
			if len(mr.priorities) == 0 {
				mr.priorities = []string{"(set your top priorities)"}
			}
			return mr, nil
		}
		mr.phase = morningPlan
		return mr, tea.Batch(mr.startPlan(), morningTickCmd())
	case morningPlan:
		mr.phase = morningPriorities
		mr.loading = false
		if len(mr.priorities) == 0 {
			mr.priorities = []string{"(set your top priorities)"}
		}
		return mr, nil
	case morningPriorities:
		mr.phase = morningComplete
		mr.loading = false
		return mr, nil
	case morningComplete:
		mr.active = false
		return mr, nil
	}
	return mr, nil
}

// ---------------------------------------------------------------------------
// AI steps
// ---------------------------------------------------------------------------

func (mr *MorningRoutine) startDevotional() tea.Cmd {
	ai := mr.ai
	scripture := mr.scripture
	goals := make([]Goal, len(mr.goals))
	copy(goals, mr.goals)

	return func() tea.Msg {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("TODAY: %s\n", time.Now().Format("Monday, January 2, 2006")))
		sb.WriteString(fmt.Sprintf("SCRIPTURE: \"%s\" -- %s\n", scripture.Text, scripture.Source))
		if len(goals) > 0 {
			sb.WriteString("\nACTIVE GOALS:\n")
			for _, g := range goals {
				if g.Status == GoalStatusActive {
					sb.WriteString(fmt.Sprintf("- %s (%d%%)\n", g.Title, g.Progress()))
				}
			}
		}
		systemPrompt := "You are DEEPCOVEN, a faith-informed personal advisor. " +
			"Connect this verse to the user's goals. " +
			"Give: verse insight (2 sentences), today's application, prayer focus, and one action. Under 10 lines."
		resp, err := ai.Chat(systemPrompt, sb.String())
		return morningDevotionalMsg{reflection: strings.TrimSpace(resp), err: err}
	}
}

func (mr *MorningRoutine) startBriefing() tea.Cmd {
	ai := mr.ai
	noteContents := mr.noteContents
	notePaths := mr.notePaths

	return func() tea.Msg {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Today: %s\n\n", time.Now().Format("2006-01-02")))

		// Recent notes (up to 10)
		count := 0
		sb.WriteString("RECENT NOTES:\n")
		for _, p := range notePaths {
			if count >= 10 {
				break
			}
			content := noteContents[p]
			if content == "" {
				continue
			}
			preview := content
			if len([]rune(preview)) > 300 {
				preview = string([]rune(preview)[:300])
			}
			sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", p, preview))
			count++
		}

		// Open tasks
		sb.WriteString("OPEN TASKS:\n")
		taskCount := 0
		for _, p := range notePaths {
			for _, line := range strings.Split(noteContents[p], "\n") {
				if strings.Contains(line, "- [ ]") && taskCount < 20 {
					sb.WriteString(strings.TrimSpace(line) + "\n")
					taskCount++
				}
			}
		}

		systemPrompt := deepCovenPrompt
		resp, err := ai.Chat(systemPrompt, sb.String())
		return morningBriefingMsg{content: strings.TrimSpace(resp), err: err}
	}
}

func (mr *MorningRoutine) startPlan() tea.Cmd {
	ai := mr.ai
	tasks := make([]Task, len(mr.tasks))
	copy(tasks, mr.tasks)
	events := make([]PlannerEvent, len(mr.events))
	copy(events, mr.events)
	habits := make([]habitEntry, len(mr.habits))
	copy(habits, mr.habits)
	goals := make([]Goal, len(mr.goals))
	copy(goals, mr.goals)

	return func() tea.Msg {
		now := time.Now()
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Current time: %s %s\n", now.Format("15:04"), now.Weekday()))
		sb.WriteString("Plan my remaining day. Create a time-blocked schedule.\n\n")

		sb.WriteString("TASKS:\n")
		for i, t := range tasks {
			if i >= 20 || t.Done {
				continue
			}
			due := ""
			if t.DueDate != "" {
				due = " (due: " + t.DueDate + ")"
			}
			sb.WriteString(fmt.Sprintf("- [P%d] %s%s\n", t.Priority, t.Text, due))
		}

		if len(events) > 0 {
			sb.WriteString("\nEVENTS:\n")
			for _, e := range events {
				sb.WriteString(fmt.Sprintf("- %s (%s, %dmin)\n", e.Title, e.Time, e.Duration))
			}
		}

		if len(goals) > 0 {
			sb.WriteString("\nACTIVE GOALS:\n")
			for _, g := range goals {
				if g.Status == GoalStatusActive {
					sb.WriteString(fmt.Sprintf("- %s (%d%%)\n", g.Title, g.Progress()))
				}
			}
		}

		systemPrompt := "You are a productivity coach. Create an optimized daily schedule.\n\n" +
			"Format:\nTOP_GOAL: {one sentence}\n\nSCHEDULE:\nHH:MM-HH:MM | Task | type\n\n" +
			"FOCUS_ORDER:\n1. Task\n2. Task\n3. Task\n\nADVICE: {2-3 sentences}"

		resp, err := ai.Chat(systemPrompt, sb.String())
		return morningPlanMsg{response: strings.TrimSpace(resp), err: err}
	}
}

func (mr *MorningRoutine) extractPriorities() {
	mr.priorities = nil
	inFocus := false
	for _, line := range strings.Split(mr.planResult, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "FOCUS_ORDER") {
			inFocus = true
			continue
		}
		if inFocus {
			if trimmed == "" || strings.HasPrefix(trimmed, "ADVICE") || strings.HasPrefix(trimmed, "SCHEDULE") {
				break
			}
			// Strip "1. " prefix
			cleaned := trimmed
			if len(cleaned) > 2 && cleaned[1] == '.' {
				cleaned = strings.TrimSpace(cleaned[2:])
			}
			if cleaned != "" {
				mr.priorities = append(mr.priorities, cleaned)
			}
		}
	}
	if len(mr.priorities) > 5 {
		mr.priorities = mr.priorities[:5]
	}
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
	stepNames := []string{"Devotional", "Briefing", "Plan", "Priorities", "Complete"}
	stepNum := int(mr.phase) + 1
	if stepNum > 4 {
		stepNum = 4
	}
	progress := fmt.Sprintf("Step %d/4", stepNum)
	b.WriteString("  " + headerStyle.Render(IconBotChar+" Morning Warrior") + "  " +
		DimStyle.Render(progress+" -- "+stepNames[mr.phase]) + "\n")

	// Progress bar
	filled := int(mr.phase) * innerW / 4
	if filled > innerW {
		filled = innerW
	}
	bar := lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("=", filled)) +
		lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("-", innerW-filled))
	b.WriteString("  " + bar + "\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)) + "\n\n")

	switch mr.phase {
	case morningDevotional:
		mr.viewDevotional(&b, innerW)
	case morningBriefing:
		mr.viewBriefing(&b, innerW)
	case morningPlan:
		mr.viewPlan(&b, innerW)
	case morningPriorities:
		mr.viewPriorities(&b, innerW)
	case morningComplete:
		mr.viewComplete(&b, innerW)
	}

	// Help bar
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", innerW-4)) + "\n")
	if mr.phase == morningComplete {
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "close"},
		}))
	} else {
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "next"}, {"Esc", "skip"}, {"q", "close"}, {"j/k", "scroll"},
		}))
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (mr MorningRoutine) viewDevotional(b *strings.Builder, w int) {
	verseStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)
	refStyle := lipgloss.NewStyle().Foreground(overlay1)
	headStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(text)

	b.WriteString("  " + verseStyle.Render(TruncateDisplay(mr.scripture.Text, w-6)) + "\n")
	b.WriteString("  " + refStyle.Render("-- "+mr.scripture.Source) + "\n\n")

	if mr.loading {
		spinChars := []string{"\u25CB", "\u25D4", "\u25D1", "\u25D5", "\u25CF"}
		spin := spinChars[mr.loadingTick%len(spinChars)]
		b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Render(spin+" Reflecting on this verse...") + "\n")
		return
	}

	for _, line := range strings.Split(mr.reflection, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			b.WriteString("\n")
		} else if strings.HasPrefix(trimmed, "##") {
			b.WriteString("  " + headStyle.Render(strings.TrimLeft(trimmed, "# ")) + "\n")
		} else {
			b.WriteString("  " + bodyStyle.Render(TruncateDisplay(trimmed, w-6)) + "\n")
		}
	}
}

func (mr MorningRoutine) viewBriefing(b *strings.Builder, w int) {
	headStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(text)

	if mr.loading {
		spinChars := []string{"\u25CB", "\u25D4", "\u25D1", "\u25D5", "\u25CF"}
		spin := spinChars[mr.loadingTick%len(spinChars)]
		b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Render(spin+" DEEPCOVEN is analyzing your vault...") + "\n")
		return
	}

	for _, line := range strings.Split(mr.briefing, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			b.WriteString("\n")
		} else if strings.HasPrefix(trimmed, "##") {
			b.WriteString("  " + headStyle.Render(strings.TrimLeft(trimmed, "# ")) + "\n")
		} else {
			b.WriteString("  " + bodyStyle.Render(TruncateDisplay(trimmed, w-6)) + "\n")
		}
	}
}

func (mr MorningRoutine) viewPlan(b *strings.Builder, w int) {
	headStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(text)

	if mr.loading {
		spinChars := []string{"\u25CB", "\u25D4", "\u25D1", "\u25D5", "\u25CF"}
		spin := spinChars[mr.loadingTick%len(spinChars)]
		b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Render(spin+" Planning your day...") + "\n")
		return
	}

	for _, line := range strings.Split(mr.planResult, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			b.WriteString("\n")
		} else if strings.HasPrefix(trimmed, "##") || strings.HasPrefix(trimmed, "SCHEDULE") ||
			strings.HasPrefix(trimmed, "TOP_GOAL") || strings.HasPrefix(trimmed, "FOCUS_ORDER") ||
			strings.HasPrefix(trimmed, "ADVICE") {
			b.WriteString("  " + headStyle.Render(trimmed) + "\n")
		} else {
			b.WriteString("  " + bodyStyle.Render(TruncateDisplay(trimmed, w-6)) + "\n")
		}
	}
}

func (mr MorningRoutine) viewPriorities(b *strings.Builder, w int) {
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	itemStyle := lipgloss.NewStyle().Foreground(green).Bold(true)

	b.WriteString("  " + headerStyle.Render("Today's Top Priorities") + "\n\n")

	for i, p := range mr.priorities {
		b.WriteString(fmt.Sprintf("  %s %s\n",
			itemStyle.Render(fmt.Sprintf("%d.", i+1)),
			lipgloss.NewStyle().Foreground(text).Render(p)))
	}
	if len(mr.priorities) == 0 {
		b.WriteString("  " + DimStyle.Render("No priorities extracted from plan") + "\n")
	}
	b.WriteString("\n  " + DimStyle.Render("Press Enter to finish your morning routine"))
}

func (mr MorningRoutine) viewComplete(b *strings.Builder, _ int) {
	successStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString("  " + successStyle.Render("Morning routine complete!") + "\n\n")
	b.WriteString("  " + DimStyle.Render("You've reviewed your devotional, briefing, and plan.") + "\n")
	b.WriteString("  " + DimStyle.Render("Go make today count.") + "\n")
}
