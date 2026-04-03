package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

// reviewPhase tracks the current step of the daily review.
type reviewPhase int

const (
	reviewCompleted  reviewPhase = iota // show completed tasks
	reviewOverdue                       // reschedule overdue tasks
	reviewPlan                          // plan tomorrow
	reviewReflect                       // free-text reflection
	reviewAISummary                     // AI-generated day summary
	reviewSaved                         // confirmation
)

// dailyReviewAIMsg carries the AI-generated daily review summary.
type dailyReviewAIMsg struct {
	summary string
	err     error
}

// DailyReview is a guided end-of-day review overlay.
type DailyReview struct {
	active    bool
	width     int
	height    int
	vaultRoot string
	phase     reviewPhase

	// Phase 1: completed tasks today
	completed []Task

	// Phase 2: overdue tasks with reschedule choices
	overdue       []Task
	overdueCursor int
	rescheduleMap map[int]string // index -> chosen action

	// Phase 3: plan tomorrow
	tomorrow   []Task
	planCursor int

	// Phase 4: reflection
	reflectBuf string

	// AI integration
	ai        AIConfig
	aiSummary string

	// For writing changes back
	vault       *vault.Vault
	fileChanged bool

	scroll    int
	statusMsg string
}

func (dr DailyReview) IsActive() bool { return dr.active }

func (dr *DailyReview) SetSize(w, h int) {
	dr.width = w
	dr.height = h
}

// Open initializes the daily review with vault data.
func (dr *DailyReview) Open(vaultRoot string, v *vault.Vault) {
	dr.active = true
	dr.vaultRoot = vaultRoot
	dr.vault = v
	dr.phase = reviewCompleted
	dr.scroll = 0
	dr.overdueCursor = 0
	dr.planCursor = 0
	dr.reflectBuf = ""
	dr.statusMsg = ""
	dr.fileChanged = false
	dr.rescheduleMap = make(map[int]string)

	allTasks := ParseAllTasks(v.Notes)

	dr.completed = nil
	dr.overdue = nil
	dr.tomorrow = nil

	now := time.Now()
	tomorrowStr := now.AddDate(0, 0, 1).Format("2006-01-02")

	for _, t := range allTasks {
		if t.Done && tmIsToday(t.DueDate) {
			dr.completed = append(dr.completed, t)
		} else if !t.Done && tmIsOverdue(t.DueDate) {
			dr.overdue = append(dr.overdue, t)
		} else if !t.Done && t.DueDate == tomorrowStr {
			dr.tomorrow = append(dr.tomorrow, t)
		}
	}
}

// WasFileChanged returns true (consumed-once) if files were modified.
func (dr *DailyReview) WasFileChanged() bool {
	if dr.fileChanged {
		dr.fileChanged = false
		return true
	}
	return false
}

func (dr DailyReview) Update(msg tea.Msg) (DailyReview, tea.Cmd) {
	if !dr.active {
		return dr, nil
	}
	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		switch dr.phase {
		case reviewCompleted:
			switch key {
			case "esc", "q":
				dr.active = false
			case "enter", "n":
				if len(dr.overdue) > 0 {
					dr.phase = reviewOverdue
				} else {
					dr.phase = reviewPlan
				}
				dr.scroll = 0
			}

		case reviewOverdue:
			switch key {
			case "esc":
				dr.active = false
			case "j", "down":
				if dr.overdueCursor < len(dr.overdue)-1 {
					dr.overdueCursor++
				}
			case "k", "up":
				if dr.overdueCursor > 0 {
					dr.overdueCursor--
				}
			case "1": // reschedule to tomorrow
				dr.rescheduleMap[dr.overdueCursor] = "tomorrow"
				dr.statusMsg = "→ tomorrow"
				if dr.overdueCursor < len(dr.overdue)-1 {
					dr.overdueCursor++
				}
			case "2": // reschedule to next week
				dr.rescheduleMap[dr.overdueCursor] = "nextweek"
				dr.statusMsg = "→ next week"
				if dr.overdueCursor < len(dr.overdue)-1 {
					dr.overdueCursor++
				}
			case "s": // skip
				dr.rescheduleMap[dr.overdueCursor] = "skip"
				dr.statusMsg = "skipped"
				if dr.overdueCursor < len(dr.overdue)-1 {
					dr.overdueCursor++
				}
			case "enter", "n":
				dr.applyReschedules()
				dr.phase = reviewPlan
				dr.scroll = 0
			}

		case reviewPlan:
			switch key {
			case "esc":
				dr.active = false
			case "enter", "n":
				dr.phase = reviewReflect
				dr.scroll = 0
			}

		case reviewReflect:
			switch key {
			case "esc":
				dr.phase = reviewPlan
			case "enter":
				dr.saveReview()
				if dr.ai.Provider != "" && dr.ai.Provider != "local" {
					dr.phase = reviewAISummary
					dr.aiSummary = ""
					return dr, dr.aiDailySummaryCmd()
				}
				dr.phase = reviewSaved
			case "backspace":
				if len(dr.reflectBuf) > 0 {
					dr.reflectBuf = dr.reflectBuf[:len(dr.reflectBuf)-1]
				}
			default:
				if len(key) == 1 || key == " " {
					dr.reflectBuf += key
				}
			}

		case reviewAISummary:
			switch key {
			case "esc", "enter", "q":
				dr.phase = reviewSaved
			case "j", "down":
				dr.scroll++
			case "k", "up":
				if dr.scroll > 0 {
					dr.scroll--
				}
			}

		case reviewSaved:
			dr.active = false
		}

	case dailyReviewAIMsg:
		if msg.err != nil {
			dr.aiSummary = "AI unavailable: " + msg.err.Error()
		} else {
			dr.aiSummary = msg.summary
		}
		dr.scroll = 0
	}
	return dr, nil
}

func (dr *DailyReview) applyReschedules() {
	if dr.vault == nil {
		return
	}
	now := time.Now()
	tomorrow := now.AddDate(0, 0, 1).Format("2006-01-02")
	nextWeek := now.AddDate(0, 0, 7).Format("2006-01-02")

	for idx, action := range dr.rescheduleMap {
		if idx >= len(dr.overdue) || action == "skip" {
			continue
		}
		task := dr.overdue[idx]
		newDate := ""
		switch action {
		case "tomorrow":
			newDate = tomorrow
		case "nextweek":
			newDate = nextWeek
		}
		if newDate == "" {
			continue
		}
		note := dr.vault.GetNote(task.NotePath)
		if note == nil {
			continue
		}
		lines := strings.Split(note.Content, "\n")
		if task.LineNum < 1 || task.LineNum > len(lines) {
			continue
		}
		line := lines[task.LineNum-1]
		line = tmDueDateRe.ReplaceAllString(line, "")
		line = strings.TrimRight(line, " ")
		line += " \U0001F4C5 " + newDate
		lines[task.LineNum-1] = line
		absPath := filepath.Join(dr.vaultRoot, task.NotePath)
		if err := os.WriteFile(absPath, []byte(strings.Join(lines, "\n")), 0644); err == nil {
			dr.fileChanged = true
		}
	}
}

// aiDailySummaryCmd sends the day's data to the LLM for a summary.
func (dr *DailyReview) aiDailySummaryCmd() tea.Cmd {
	ai := dr.ai
	completed := make([]Task, len(dr.completed))
	copy(completed, dr.completed)
	overdue := make([]Task, len(dr.overdue))
	copy(overdue, dr.overdue)
	reflection := dr.reflectBuf
	rescheduleMap := make(map[int]string)
	for k, v := range dr.rescheduleMap {
		rescheduleMap[k] = v
	}

	return func() tea.Msg {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Date: %s\n\n", time.Now().Format("2006-01-02")))

		sb.WriteString("COMPLETED TODAY:\n")
		for _, t := range completed {
			sb.WriteString(fmt.Sprintf("- %s\n", t.Text))
		}
		if len(completed) == 0 {
			sb.WriteString("- (none)\n")
		}

		sb.WriteString("\nOVERDUE/RESCHEDULED:\n")
		for i, t := range overdue {
			action := rescheduleMap[i]
			if action == "" {
				action = "no action"
			}
			sb.WriteString(fmt.Sprintf("- %s [%s]\n", t.Text, action))
		}
		if len(overdue) == 0 {
			sb.WriteString("- (none)\n")
		}

		if reflection != "" {
			sb.WriteString(fmt.Sprintf("\nUSER REFLECTION:\n%s\n", reflection))
		}

		systemPrompt := "You are DEEPCOVEN, a direct and honest end-of-day coach.\n\n" +
			"Based on what was completed, rescheduled, and the user's reflection:\n" +
			"1. WIN OF THE DAY: The single most impactful thing accomplished\n" +
			"2. PATTERN: Any recurring theme (momentum, procrastination, overcommitment)\n" +
			"3. TOMORROW'S PRIORITY: The #1 thing to tackle first tomorrow\n" +
			"4. HONEST NOTE: 1 sentence of encouragement or reality check\n\n" +
			"Keep it under 8 lines total. Be direct."

		resp, err := ai.Chat(systemPrompt, sb.String())
		return dailyReviewAIMsg{summary: strings.TrimSpace(resp), err: err}
	}
}

func (dr *DailyReview) saveReview() {
	now := time.Now()
	dateStr := now.Format("2006-01-02")

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("date: " + dateStr + "\n")
	b.WriteString("type: daily-review\n")
	b.WriteString("---\n\n")
	b.WriteString("# Daily Review - " + now.Format("Monday, January 2, 2006") + "\n\n")

	b.WriteString("## Completed\n")
	if len(dr.completed) == 0 {
		b.WriteString("- No tasks completed today\n")
	} else {
		for _, t := range dr.completed {
			b.WriteString("- [x] " + t.Text + "\n")
		}
	}
	b.WriteString("\n")

	b.WriteString("## Rescheduled\n")
	rescheduled := 0
	for idx, action := range dr.rescheduleMap {
		if idx < len(dr.overdue) && action != "skip" {
			b.WriteString("- " + dr.overdue[idx].Text + " → " + action + "\n")
			rescheduled++
		}
	}
	if rescheduled == 0 {
		b.WriteString("- No tasks rescheduled\n")
	}
	b.WriteString("\n")

	if dr.reflectBuf != "" {
		b.WriteString("## Reflection\n")
		b.WriteString(dr.reflectBuf + "\n\n")
	}

	// Save to Reviews directory
	reviewDir := filepath.Join(dr.vaultRoot, "Reviews")
	_ = os.MkdirAll(reviewDir, 0755)
	filename := filepath.Join(reviewDir, "daily-"+dateStr+".md")
	_ = os.WriteFile(filename, []byte(b.String()), 0644)
}

func (dr DailyReview) View() string {
	width := dr.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}
	innerW := width - 8

	var b strings.Builder

	// Title
	icon := lipgloss.NewStyle().Foreground(blue).Render(IconOutlineChar)
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Daily Review")
	phaseNames := []string{"Completed", "Overdue", "Plan", "Reflect", "AI Summary", "Saved"}
	phaseLabel := DimStyle.Render("  [" + phaseNames[dr.phase] + "]")
	b.WriteString("  " + icon + title + phaseLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", innerW-4)))
	b.WriteString("\n\n")

	switch dr.phase {
	case reviewCompleted:
		dr.viewCompleted(&b, innerW)
	case reviewOverdue:
		dr.viewOverdue(&b, innerW)
	case reviewPlan:
		dr.viewPlan(&b, innerW)
	case reviewReflect:
		dr.viewReflect(&b, innerW)
	case reviewAISummary:
		dr.viewAISummary(&b, innerW)
	case reviewSaved:
		dr.viewSaved(&b, innerW)
	}

	// Help bar
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("─", innerW-4)))
	b.WriteString("\n")
	switch dr.phase {
	case reviewCompleted:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "next"}, {"Esc", "close"},
		}))
	case reviewOverdue:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"1", "tomorrow"}, {"2", "+1 week"}, {"s", "skip"},
			{"j/k", "nav"}, {"Enter", "apply & next"}, {"Esc", "close"},
		}))
	case reviewPlan:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "next"}, {"Esc", "close"},
		}))
	case reviewReflect:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "save"}, {"Esc", "back"},
		}))
	case reviewAISummary:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "done"}, {"j/k", "scroll"},
		}))
	case reviewSaved:
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"any key", "close"},
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

func (dr DailyReview) viewCompleted(b *strings.Builder, w int) {
	header := lipgloss.NewStyle().Foreground(green).Bold(true).Render("  Tasks Completed Today")
	b.WriteString(header)
	b.WriteString("\n\n")

	if len(dr.completed) == 0 {
		b.WriteString(DimStyle.Render("  No tasks completed today. Tomorrow is a new day!"))
		b.WriteString("\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).
			Render(fmt.Sprintf("  %d tasks done!", len(dr.completed))))
		b.WriteString("\n\n")
		for _, t := range dr.completed {
			taskLabel := TruncateDisplay(tmCleanText(t.Text), w-8)
			b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("✓") + " " +
				lipgloss.NewStyle().Foreground(text).Render(taskLabel))
			b.WriteString("\n")
		}
	}
}

func (dr DailyReview) viewOverdue(b *strings.Builder, w int) {
	header := lipgloss.NewStyle().Foreground(red).Bold(true).
		Render(fmt.Sprintf("  %d Overdue Tasks", len(dr.overdue)))
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Reschedule each task or skip"))
	b.WriteString("\n\n")

	for i, t := range dr.overdue {
		prefix := "  "
		if i == dr.overdueCursor {
			prefix = lipgloss.NewStyle().Foreground(mauve).Render("▸ ")
		}
		taskText := TruncateDisplay(tmCleanText(t.Text), w-20)
		choice := ""
		if c, ok := dr.rescheduleMap[i]; ok {
			switch c {
			case "tomorrow":
				choice = lipgloss.NewStyle().Foreground(green).Render(" → tomorrow")
			case "nextweek":
				choice = lipgloss.NewStyle().Foreground(blue).Render(" → next week")
			case "skip":
				choice = DimStyle.Render(" (skipped)")
			}
		}
		b.WriteString(prefix + lipgloss.NewStyle().Foreground(text).Render(taskText) + choice)
		b.WriteString("\n")
	}

	if dr.statusMsg != "" {
		b.WriteString("\n  " + lipgloss.NewStyle().Foreground(green).Render(dr.statusMsg))
	}
}

func (dr DailyReview) viewPlan(b *strings.Builder, w int) {
	header := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Tomorrow's Tasks")
	b.WriteString(header)
	b.WriteString("\n\n")

	if len(dr.tomorrow) == 0 {
		b.WriteString(DimStyle.Render("  No tasks scheduled for tomorrow yet."))
		b.WriteString("\n")
	} else {
		for _, t := range dr.tomorrow {
			taskText := TruncateDisplay(tmCleanText(t.Text), w-8)
			prioIcon := tmPriorityIcon(t.Priority)
			b.WriteString("  " + lipgloss.NewStyle().Foreground(tmPriorityColor(t.Priority)).Render(prioIcon) +
				" " + lipgloss.NewStyle().Foreground(text).Render(taskText))
			b.WriteString("\n")
		}
	}

	// Show rescheduled tasks that will appear tomorrow
	rescheduledTomorrow := 0
	for idx, action := range dr.rescheduleMap {
		if idx < len(dr.overdue) && action == "tomorrow" {
			if rescheduledTomorrow == 0 {
				b.WriteString("\n" + DimStyle.Render("  Rescheduled to tomorrow:"))
				b.WriteString("\n")
			}
			taskText := TruncateDisplay(tmCleanText(dr.overdue[idx].Text), w-8)
			b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Render("→") +
				" " + lipgloss.NewStyle().Foreground(text).Render(taskText))
			b.WriteString("\n")
			rescheduledTomorrow++
		}
	}
}

func (dr DailyReview) viewReflect(b *strings.Builder, w int) {
	header := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("  Reflection")
	b.WriteString(header)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  What went well? What could improve?"))
	b.WriteString("\n\n")

	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text).Background(surface0).Padding(0, 1)
	b.WriteString("  " + promptStyle.Render("> ") + inputStyle.Render(dr.reflectBuf+"\u2588"))
	b.WriteString("\n")
}

func (dr DailyReview) viewAISummary(b *strings.Builder, w int) {
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	bodyStyle := lipgloss.NewStyle().Foreground(lavender).Italic(true)
	subhStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)

	b.WriteString("  " + headerStyle.Render(IconBotChar+" DEEPCOVEN — Day Summary") + "\n\n")

	if dr.aiSummary == "" {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Render("Analyzing your day...") + "\n")
		return
	}

	for _, line := range strings.Split(dr.aiSummary, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			b.WriteString("\n")
		} else if strings.HasPrefix(trimmed, "##") {
			b.WriteString("  " + subhStyle.Render(strings.TrimLeft(trimmed, "# ")) + "\n")
		} else {
			b.WriteString("  " + bodyStyle.Render(TruncateDisplay(trimmed, w-6)) + "\n")
		}
	}
}

func (dr DailyReview) viewSaved(b *strings.Builder, _ int) {
	b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).Render("  ✓ Review saved!"))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Saved to Reviews/daily-" + time.Now().Format("2006-01-02") + ".md"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Press any key to close."))
	b.WriteString("\n")
}
