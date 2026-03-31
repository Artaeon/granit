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

// weeklyReviewStep identifies the current step of the review workflow.
type weeklyReviewStep int

const (
	wrStepTasks   weeklyReviewStep = iota // 0 — completed vs incomplete tasks
	wrStepWins                             // 1 — what went well
	wrStepLessons                          // 2 — what did I learn
	wrStepNext                             // 3 — top 3 priorities
	wrStepSummary                          // 4 — compiled review
)

// WeeklyReview is a guided weekly review workflow overlay.
type WeeklyReview struct {
	active bool
	width  int
	height int

	vaultRoot string
	step      weeklyReviewStep

	// Data from vault
	completedTasks []Task
	incompleteTasks []Task
	weekYear       int
	weekNum        int

	// User inputs
	wins     string
	lessons  string
	nextWeek string

	// Text editing state
	inputBuf string

	// Save state
	saved bool
}

// NewWeeklyReview returns a WeeklyReview in its default (inactive) state.
func NewWeeklyReview() WeeklyReview {
	return WeeklyReview{}
}

// IsActive reports whether the weekly review overlay is visible.
func (wr WeeklyReview) IsActive() bool {
	return wr.active
}

// SetSize updates available terminal dimensions.
func (wr *WeeklyReview) SetSize(w, h int) {
	wr.width = w
	wr.height = h
}

// Open activates the weekly review overlay, gathering data.
func (wr *WeeklyReview) Open(vaultRoot string, v *vault.Vault) {
	wr.vaultRoot = vaultRoot
	wr.active = true
	wr.step = wrStepTasks
	wr.saved = false
	wr.inputBuf = ""

	now := time.Now()
	wr.weekYear, wr.weekNum = now.ISOWeek()

	// Load existing review if any
	existing := wr.loadExisting()
	if existing != "" {
		wr.parseExisting(existing)
	}

	// Gather tasks for this week
	wr.gatherWeekTasks(v)
}

// Close saves and deactivates.
func (wr *WeeklyReview) Close() {
	if !wr.saved && (wr.wins != "" || wr.lessons != "" || wr.nextWeek != "") {
		wr.saveReview()
	}
	wr.active = false
}

// reviewPath returns the file path for this week's review.
func (wr *WeeklyReview) reviewPath() string {
	name := fmt.Sprintf("%d-W%02d.md", wr.weekYear, wr.weekNum)
	return filepath.Join(wr.vaultRoot, "Reviews", name)
}

// loadExisting reads any existing review file.
func (wr *WeeklyReview) loadExisting() string {
	raw, err := os.ReadFile(wr.reviewPath())
	if err != nil {
		return ""
	}
	return string(raw)
}

// parseExisting populates fields from an existing review markdown.
func (wr *WeeklyReview) parseExisting(content string) {
	sections := map[string]*string{
		"## Wins":              &wr.wins,
		"## Lessons":           &wr.lessons,
		"## Next Week":         &wr.nextWeek,
	}

	lines := strings.Split(content, "\n")
	var currentTarget *string
	for _, line := range lines {
		if target, ok := sections[strings.TrimSpace(line)]; ok {
			currentTarget = target
			continue
		}
		if strings.HasPrefix(line, "## ") {
			currentTarget = nil
			continue
		}
		if currentTarget != nil {
			if *currentTarget != "" {
				*currentTarget += "\n"
			}
			*currentTarget += line
		}
	}

	// Trim leading/trailing whitespace
	wr.wins = strings.TrimSpace(wr.wins)
	wr.lessons = strings.TrimSpace(wr.lessons)
	wr.nextWeek = strings.TrimSpace(wr.nextWeek)
}

// gatherWeekTasks finds tasks completed/incomplete this week.
func (wr *WeeklyReview) gatherWeekTasks(v *vault.Vault) {
	wr.completedTasks = nil
	wr.incompleteTasks = nil

	allTasks := ParseAllTasks(v.Notes)

	now := time.Now()
	year, week := now.ISOWeek()

	// Calculate week boundaries (Monday-Sunday)
	// Find Monday of current ISO week
	offset := int(time.Monday - now.Weekday())
	if offset > 0 {
		offset -= 7
	}
	weekStart := time.Date(now.Year(), now.Month(), now.Day()+offset, 0, 0, 0, 0, time.Local)
	weekEnd := weekStart.AddDate(0, 0, 7)

	_ = year
	_ = week

	for _, task := range allTasks {
		if task.DueDate != "" {
			due, err := time.Parse("2006-01-02", task.DueDate)
			if err == nil {
				if (due.Equal(weekStart) || due.After(weekStart)) && due.Before(weekEnd) {
					if task.Done {
						wr.completedTasks = append(wr.completedTasks, task)
					} else {
						wr.incompleteTasks = append(wr.incompleteTasks, task)
					}
					continue
				}
			}
		}
		// TODO: Also include tasks completed this week (no due date but done).
		// We cannot reliably determine when it was completed without git history,
		// but include recent done tasks for the user to review.
	}
}

// saveReview writes the review to a markdown file.
func (wr *WeeklyReview) saveReview() {
	dir := filepath.Join(wr.vaultRoot, "Reviews")
	_ = os.MkdirAll(dir, 0o755)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Weekly Review — %d-W%02d\n\n", wr.weekYear, wr.weekNum))

	// Tasks summary
	b.WriteString("## Tasks Summary\n\n")
	b.WriteString(fmt.Sprintf("- Completed: %d\n", len(wr.completedTasks)))
	b.WriteString(fmt.Sprintf("- Incomplete: %d\n\n", len(wr.incompleteTasks)))

	if len(wr.completedTasks) > 0 {
		b.WriteString("### Completed\n\n")
		for _, t := range wr.completedTasks {
			b.WriteString("- [x] " + t.Text + "\n")
		}
		b.WriteString("\n")
	}

	if len(wr.incompleteTasks) > 0 {
		b.WriteString("### Incomplete\n\n")
		for _, t := range wr.incompleteTasks {
			b.WriteString("- [ ] " + t.Text + "\n")
		}
		b.WriteString("\n")
	}

	// Wins
	b.WriteString("## Wins\n\n")
	if wr.wins != "" {
		b.WriteString(wr.wins + "\n\n")
	}

	// Lessons
	b.WriteString("## Lessons\n\n")
	if wr.lessons != "" {
		b.WriteString(wr.lessons + "\n\n")
	}

	// Next week
	b.WriteString("## Next Week\n\n")
	if wr.nextWeek != "" {
		b.WriteString(wr.nextWeek + "\n\n")
	}

	// Timestamp
	b.WriteString(fmt.Sprintf("---\n*Generated: %s*\n", time.Now().Format("2006-01-02 15:04")))

	_ = os.WriteFile(wr.reviewPath(), []byte(b.String()), 0o644)
	wr.saved = true
}

// Update handles key events.
func (wr WeeklyReview) Update(msg tea.Msg) (WeeklyReview, tea.Cmd) {
	if !wr.active {
		return wr, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// Text input steps
		if wr.step == wrStepWins || wr.step == wrStepLessons || wr.step == wrStepNext {
			return wr.updateTextInput(msg)
		}

		switch key {
		case "esc":
			wr.Close()
			return wr, nil
		case "tab", "enter":
			if wr.step == wrStepSummary {
				wr.saveReview()
				wr.Close()
				return wr, nil
			}
			wr.step++
			wr.prepareStep()
			return wr, nil
		case "shift+tab":
			if wr.step > 0 {
				wr.step--
				wr.prepareStep()
			}
			return wr, nil

		// Task review: toggle tasks done
		case "x", " ":
			// TODO: implement task toggle in review step
			// if wr.step == wrStepTasks && len(wr.incompleteTasks) > 0 {
			//     parse inputBuf as selected index and toggle done
			// }
		case "up", "k":
			// Scroll in tasks view (reuse inputBuf as cursor index)
		case "down", "j":
			// Scroll in tasks view
		}
	}
	return wr, nil
}

// updateTextInput handles key events for text input steps.
func (wr WeeklyReview) updateTextInput(msg tea.KeyMsg) (WeeklyReview, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		wr.Close()
		return wr, nil
	case "tab":
		// Save current input and advance
		wr.commitInput()
		wr.step++
		wr.prepareStep()
		return wr, nil
	case "shift+tab":
		wr.commitInput()
		if wr.step > 0 {
			wr.step--
			wr.prepareStep()
		}
		return wr, nil
	case "enter":
		wr.inputBuf += "\n"
		return wr, nil
	case "backspace":
		if len(wr.inputBuf) > 0 {
			wr.inputBuf = wr.inputBuf[:len(wr.inputBuf)-1]
		}
		return wr, nil
	default:
		if len(key) == 1 && key[0] >= 32 {
			wr.inputBuf += key
		}
		return wr, nil
	}
}

// commitInput saves the current input buffer to the appropriate field.
func (wr *WeeklyReview) commitInput() {
	text := strings.TrimSpace(wr.inputBuf)
	switch wr.step {
	case wrStepWins:
		wr.wins = text
	case wrStepLessons:
		wr.lessons = text
	case wrStepNext:
		wr.nextWeek = text
	}
}

// prepareStep initializes state for the current step.
func (wr *WeeklyReview) prepareStep() {
	switch wr.step {
	case wrStepWins:
		wr.inputBuf = wr.wins
	case wrStepLessons:
		wr.inputBuf = wr.lessons
	case wrStepNext:
		wr.inputBuf = wr.nextWeek
	case wrStepSummary:
		wr.commitInput()
	default:
		wr.inputBuf = ""
	}
}

// View renders the weekly review overlay.
func (wr WeeklyReview) View() string {
	width := wr.width * 3 / 5
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render(fmt.Sprintf("Weekly Review — %d-W%02d", wr.weekYear, wr.weekNum)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	// Progress indicator
	steps := []string{"Tasks", "Wins", "Lessons", "Next Week", "Summary"}
	var stepParts []string
	for i, s := range steps {
		if i == int(wr.step) {
			stepParts = append(stepParts, lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(s))
		} else if i < int(wr.step) {
			stepParts = append(stepParts, lipgloss.NewStyle().Foreground(green).Render(s))
		} else {
			stepParts = append(stepParts, lipgloss.NewStyle().Foreground(overlay0).Render(s))
		}
	}
	b.WriteString(strings.Join(stepParts, lipgloss.NewStyle().Foreground(surface1).Render(" > ")))
	b.WriteString("\n\n")

	// Step content
	switch wr.step {
	case wrStepTasks:
		b.WriteString(wr.viewTasks(innerW))
	case wrStepWins:
		b.WriteString(wr.viewTextInput("What went well this week?", innerW))
	case wrStepLessons:
		b.WriteString(wr.viewTextInput("What did I learn?", innerW))
	case wrStepNext:
		b.WriteString(wr.viewTextInput("Top 3 priorities for next week?", innerW))
	case wrStepSummary:
		b.WriteString(wr.viewSummary(innerW))
	}

	// Footer
	b.WriteString("\n")
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	var keys []string
	if wr.step < wrStepSummary {
		keys = append(keys, keyStyle.Render("Tab")+dimStyle.Render(" next"))
	} else {
		keys = append(keys, keyStyle.Render("Tab")+dimStyle.Render(" save & close"))
	}
	if wr.step > 0 {
		keys = append(keys, keyStyle.Render("Shift+Tab")+dimStyle.Render(" back"))
	}
	keys = append(keys, keyStyle.Render("Esc")+dimStyle.Render(" close"))
	b.WriteString(strings.Join(keys, dimStyle.Render("  ")))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// viewTasks renders the task review step.
func (wr WeeklyReview) viewTasks(w int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	textStyle := lipgloss.NewStyle().Foreground(text)

	b.WriteString(sectionStyle.Render("Tasks This Week"))
	b.WriteString("\n\n")

	// Completed
	b.WriteString(textStyle.Render(fmt.Sprintf("Completed: %d", len(wr.completedTasks))))
	b.WriteString("\n")
	maxShow := 8
	for i, t := range wr.completedTasks {
		if i >= maxShow {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  ... and %d more", len(wr.completedTasks)-maxShow)))
			b.WriteString("\n")
			break
		}
		b.WriteString(lipgloss.NewStyle().Foreground(green).Render("  [x] "))
		display := t.Text
		if len(display) > w-10 {
			display = display[:w-13] + "..."
		}
		b.WriteString(dimStyle.Render(display))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Incomplete
	b.WriteString(textStyle.Render(fmt.Sprintf("Incomplete: %d", len(wr.incompleteTasks))))
	b.WriteString("\n")
	for i, t := range wr.incompleteTasks {
		if i >= maxShow {
			b.WriteString(dimStyle.Render(fmt.Sprintf("  ... and %d more", len(wr.incompleteTasks)-maxShow)))
			b.WriteString("\n")
			break
		}
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Render("  [ ] "))
		display := t.Text
		if len(display) > w-10 {
			display = display[:w-13] + "..."
		}
		b.WriteString(textStyle.Render(display))
		b.WriteString("\n")
	}

	if len(wr.completedTasks) == 0 && len(wr.incompleteTasks) == 0 {
		b.WriteString(dimStyle.Render("  No tasks with due dates found this week."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Press Tab to continue to Wins..."))
	b.WriteString("\n")

	return b.String()
}

// viewTextInput renders a text input step.
func (wr WeeklyReview) viewTextInput(prompt string, w int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	b.WriteString(sectionStyle.Render(prompt))
	b.WriteString("\n\n")

	// Text area
	inputStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Width(w - 4).
		Padding(1, 1)

	cursor := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("│")
	content := wr.inputBuf + cursor

	// Show line count
	lines := strings.Split(wr.inputBuf, "\n")
	b.WriteString(inputStyle.Render(content))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(
		fmt.Sprintf("  %d line(s)  •  Enter for new line  •  Tab to continue", len(lines))))
	b.WriteString("\n")

	return b.String()
}

// viewSummary renders the compiled review.
func (wr WeeklyReview) viewSummary(w int) string {
	var b strings.Builder

	sectionStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	textStyle := lipgloss.NewStyle().Foreground(text)

	b.WriteString(sectionStyle.Render("Review Summary"))
	b.WriteString("\n\n")

	b.WriteString(textStyle.Render(fmt.Sprintf("Tasks: %d completed, %d remaining",
		len(wr.completedTasks), len(wr.incompleteTasks))))
	b.WriteString("\n\n")

	if wr.wins != "" {
		b.WriteString(sectionStyle.Render("Wins:"))
		b.WriteString("\n")
		b.WriteString(textStyle.Render(wr.wins))
		b.WriteString("\n\n")
	}

	if wr.lessons != "" {
		b.WriteString(sectionStyle.Render("Lessons:"))
		b.WriteString("\n")
		b.WriteString(textStyle.Render(wr.lessons))
		b.WriteString("\n\n")
	}

	if wr.nextWeek != "" {
		b.WriteString(sectionStyle.Render("Next Week:"))
		b.WriteString("\n")
		b.WriteString(textStyle.Render(wr.nextWeek))
		b.WriteString("\n\n")
	}

	savePath := wr.reviewPath()
	relPath := strings.TrimPrefix(savePath, wr.vaultRoot+"/")
	if wr.saved {
		b.WriteString(lipgloss.NewStyle().Foreground(green).Bold(true).Render("Saved to " + relPath))
	} else {
		b.WriteString(dimStyle.Render("Will save to " + relPath))
	}
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("Press Tab to save and close."))
	b.WriteString("\n")

	return b.String()
}
