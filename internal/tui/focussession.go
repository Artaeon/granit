package tui

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// focusSessionTickMsg is sent every second while the session timer is running.
type focusSessionTickMsg time.Time

// Focus session phases.
const (
	fsPhaseSetup  = 0
	fsPhaseActive = 1
	fsPhaseBreak  = 2
	fsPhaseReview = 3
)

// FocusSession is a guided work session overlay combining a timer, task
// selection, and session notes (scratchpad).
type FocusSession struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	phase int // 0=setup, 1=active, 2=break, 3=review

	// Setup
	durations  []int // selectable work durations in minutes
	durIdx     int
	goalInput  string
	setupField int // 0=duration, 1=goal
	tasks      []string // tasks loaded from Tasks.md
	taskIdx    int      // -1 = no task selected
	taskScroll int

	// Active session
	duration  time.Duration
	startTime time.Time
	elapsed   time.Duration
	scratchpad string
	paused    bool

	// Break
	breakDurs    []int // selectable break durations in minutes
	breakIdx     int
	breakStart   time.Time
	breakElapsed time.Duration
	breakActive  bool

	// Review
	sessionGoal  string
	sessionNotes string
	totalElapsed time.Duration
	sessionTask  string
}

// NewFocusSession creates a FocusSession in its default (inactive) state.
func NewFocusSession() FocusSession {
	return FocusSession{
		durations: []int{25, 45, 60, 90},
		breakDurs: []int{5, 10, 15},
		taskIdx:   -1,
	}
}

// IsActive reports whether the focus session overlay is open.
func (fs FocusSession) IsActive() bool {
	return fs.active
}

// Open activates the focus session overlay and resets to the setup phase.
func (fs *FocusSession) Open(vaultRoot string) {
	fs.active = true
	fs.vaultRoot = vaultRoot
	fs.phase = fsPhaseSetup
	fs.durIdx = 0
	// Ensure durations are initialized (handles zero-value struct)
	if len(fs.durations) == 0 {
		fs.durations = []int{25, 45, 60, 90}
	}
	if len(fs.breakDurs) == 0 {
		fs.breakDurs = []int{5, 10, 15}
	}
	fs.goalInput = ""
	fs.setupField = 0
	fs.scratchpad = ""
	fs.paused = false
	fs.elapsed = 0
	fs.breakIdx = 0
	fs.breakElapsed = 0
	fs.breakActive = false
	fs.sessionGoal = ""
	fs.sessionNotes = ""
	fs.totalElapsed = 0
	fs.sessionTask = ""
	fs.taskIdx = -1
	fs.taskScroll = 0
	fs.loadTasks()
}

// OpenWithTask opens the focus session pre-loaded with a specific task,
// skipping the task selection step in setup.
func (fs *FocusSession) OpenWithTask(vaultRoot, taskText string) {
	fs.Open(vaultRoot)
	fs.sessionTask = taskText
	fs.goalInput = taskText
	fs.setupField = 1 // skip task selection, go to duration
}

// SetSize updates the available terminal dimensions.
func (fs *FocusSession) SetSize(w, h int) {
	fs.width = w
	fs.height = h
}

// loadTasks reads incomplete tasks from Tasks.md in the vault root.
func (fs *FocusSession) loadTasks() {
	fs.tasks = nil
	taskFile := filepath.Join(fs.vaultRoot, "Tasks.md")
	f, err := os.Open(taskFile)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ] ") {
			taskText := strings.TrimPrefix(trimmed, "- [ ] ")
			taskText = strings.TrimSpace(taskText)
			if taskText != "" {
				fs.tasks = append(fs.tasks, taskText)
			}
		}
	}
}

// Update handles messages for the FocusSession overlay.
func (fs FocusSession) Update(msg tea.Msg) (FocusSession, tea.Cmd) {
	if !fs.active {
		return fs, nil
	}

	switch msg := msg.(type) {
	case focusSessionTickMsg:
		return fs.handleTick()

	case tea.KeyMsg:
		switch fs.phase {
		case fsPhaseSetup:
			return fs.updateSetup(msg)
		case fsPhaseActive:
			return fs.updateActive(msg)
		case fsPhaseBreak:
			return fs.updateBreak(msg)
		case fsPhaseReview:
			return fs.updateReview(msg)
		}
	}

	return fs, nil
}

// handleTick processes a timer tick during active and break phases.
func (fs FocusSession) handleTick() (FocusSession, tea.Cmd) {
	now := time.Now()

	switch fs.phase {
	case fsPhaseActive:
		if fs.paused {
			return fs, fs.tick()
		}
		fs.elapsed = now.Sub(fs.startTime)
		if fs.elapsed >= fs.duration {
			fs.elapsed = fs.duration
			fs.totalElapsed = fs.elapsed
			fs.sessionGoal = fs.goalInput
			fs.sessionNotes = fs.scratchpad
			fs.phase = fsPhaseBreak
			fs.breakActive = false
			return fs, nil
		}
		return fs, fs.tick()

	case fsPhaseBreak:
		if fs.breakActive {
			fs.breakElapsed = now.Sub(fs.breakStart)
			breakMin := 5 // fallback
			if fs.breakIdx >= 0 && fs.breakIdx < len(fs.breakDurs) {
				breakMin = fs.breakDurs[fs.breakIdx]
			}
			breakDur := time.Duration(breakMin) * time.Minute
			if fs.breakElapsed >= breakDur {
				fs.breakElapsed = breakDur
				fs.phase = fsPhaseReview
				return fs, nil
			}
			return fs, fs.tick()
		}
	}

	return fs, nil
}

// updateSetup handles key events during the setup phase.
func (fs FocusSession) updateSetup(msg tea.KeyMsg) (FocusSession, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		fs.active = false
		return fs, nil

	case "tab":
		fs.setupField = (fs.setupField + 1) % 2
		return fs, nil

	case "left", "h":
		if fs.setupField == 0 {
			if fs.durIdx > 0 {
				fs.durIdx--
			}
		}
		return fs, nil

	case "right", "l":
		if fs.setupField == 0 {
			if fs.durIdx < len(fs.durations)-1 {
				fs.durIdx++
			}
		}
		return fs, nil

	case "up", "k":
		if fs.setupField == 0 && len(fs.tasks) > 0 {
			if fs.taskIdx > -1 {
				fs.taskIdx--
			}
		}
		return fs, nil

	case "down", "j":
		if fs.setupField == 0 && len(fs.tasks) > 0 {
			if fs.taskIdx < len(fs.tasks)-1 {
				fs.taskIdx++
			}
		}
		return fs, nil

	case "enter":
		if fs.setupField == 0 {
			// Move to goal field
			fs.setupField = 1
			return fs, nil
		}
		// Start the session
		return fs.startSession()

	case "backspace":
		if fs.setupField == 1 && len(fs.goalInput) > 0 {
			r := []rune(fs.goalInput)
			fs.goalInput = string(r[:len(r)-1])
		}
		return fs, nil

	default:
		if fs.setupField == 1 {
			// Accept printable characters for goal input
			for _, r := range msg.Runes {
				fs.goalInput += string(r)
			}
		}
		return fs, nil
	}
}

// startSession transitions from setup to active phase.
func (fs FocusSession) startSession() (FocusSession, tea.Cmd) {
	fs.phase = fsPhaseActive
	dur := 25 // default fallback
	if fs.durIdx >= 0 && fs.durIdx < len(fs.durations) {
		dur = fs.durations[fs.durIdx]
	}
	fs.duration = time.Duration(dur) * time.Minute
	fs.startTime = time.Now()
	fs.elapsed = 0
	fs.scratchpad = ""
	fs.paused = false
	fs.sessionGoal = fs.goalInput
	if fs.taskIdx >= 0 && fs.taskIdx < len(fs.tasks) {
		fs.sessionTask = fs.tasks[fs.taskIdx]
	} else {
		fs.sessionTask = ""
	}
	return fs, fs.tick()
}

// updateActive handles key events during the active session phase.
func (fs FocusSession) updateActive(msg tea.KeyMsg) (FocusSession, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		// End session early, go to review
		fs.totalElapsed = fs.elapsed
		fs.sessionNotes = fs.scratchpad
		fs.phase = fsPhaseReview
		return fs, nil

	case "ctrl+p":
		fs.paused = !fs.paused
		if fs.paused {
			// Record how much time elapsed before pause
			fs.elapsed = time.Since(fs.startTime)
		} else {
			// Adjust start time to account for pause
			fs.startTime = time.Now().Add(-fs.elapsed)
		}
		return fs, nil

	case "backspace":
		if len(fs.scratchpad) > 0 {
			r := []rune(fs.scratchpad)
			fs.scratchpad = string(r[:len(r)-1])
		}
		return fs, nil

	case "enter":
		fs.scratchpad += "\n"
		return fs, nil

	default:
		for _, r := range msg.Runes {
			fs.scratchpad += string(r)
		}
		return fs, nil
	}
}

// updateBreak handles key events during the break phase.
func (fs FocusSession) updateBreak(msg tea.KeyMsg) (FocusSession, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		fs.phase = fsPhaseReview
		return fs, nil

	case "left", "h":
		if !fs.breakActive && fs.breakIdx > 0 {
			fs.breakIdx--
		}
		return fs, nil

	case "right", "l":
		if !fs.breakActive && fs.breakIdx < len(fs.breakDurs)-1 {
			fs.breakIdx++
		}
		return fs, nil

	case "enter":
		if !fs.breakActive {
			fs.breakActive = true
			fs.breakStart = time.Now()
			fs.breakElapsed = 0
			return fs, fs.tick()
		}
		return fs, nil

	case "s":
		// Skip break, go to review
		fs.phase = fsPhaseReview
		return fs, nil
	}

	return fs, nil
}

// updateReview handles key events during the review phase.
func (fs FocusSession) updateReview(msg tea.KeyMsg) (FocusSession, tea.Cmd) {
	key := msg.String()

	switch key {
	case "esc":
		fs.active = false
		return fs, nil

	case "enter":
		fs.saveSession()
		fs.active = false
		return fs, nil
	}

	return fs, nil
}

// tick returns a command that sends a focusSessionTickMsg after 1 second.
func (fs FocusSession) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return focusSessionTickMsg(t)
	})
}

// saveSession writes the session log entry to FocusSessions/YYYY-MM-DD.md.
func (fs *FocusSession) saveSession() {
	dir := filepath.Join(fs.vaultRoot, "FocusSessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}

	now := time.Now()
	filename := now.Format("2006-01-02") + ".md"
	filePath := filepath.Join(dir, filename)

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	// Check if file is new (empty) to write a header
	info, _ := f.Stat()
	if info != nil && info.Size() == 0 {
		header := fmt.Sprintf("# Focus Sessions - %s\n\n", now.Format("2006-01-02"))
		_, _ = f.WriteString(header)
	}

	endTime := fs.startTime.Add(fs.totalElapsed)
	durMinutes := int(fs.totalElapsed.Minutes())

	var entry strings.Builder
	entry.WriteString(fmt.Sprintf("## Session %s - %s\n",
		fs.startTime.Format("15:04"), endTime.Format("15:04")))

	if fs.sessionGoal != "" {
		entry.WriteString(fmt.Sprintf("- Goal: %s\n", fs.sessionGoal))
	}
	if fs.sessionTask != "" {
		entry.WriteString(fmt.Sprintf("- Task: %s\n", fs.sessionTask))
	}
	entry.WriteString(fmt.Sprintf("- Duration: %d min\n", durMinutes))

	scratchWords := len(strings.Fields(fs.sessionNotes))
	entry.WriteString(fmt.Sprintf("- Words written: %d\n", scratchWords))

	if fs.sessionNotes != "" {
		entry.WriteString("- Notes:\n")
		for _, line := range strings.Split(fs.sessionNotes, "\n") {
			entry.WriteString(fmt.Sprintf("  %s\n", line))
		}
	}
	entry.WriteString("\n")

	_, _ = f.WriteString(entry.String())
}

// View renders the FocusSession overlay.
func (fs FocusSession) View() string {
	width := fs.width / 2
	if width < 50 {
		width = 50
	}
	if width > 70 {
		width = 70
	}

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString("  " + titleStyle.Render(IconOutlineChar+" Focus Session"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n\n")

	switch fs.phase {
	case fsPhaseSetup:
		fs.viewSetup(&b, width)
	case fsPhaseActive:
		fs.viewActive(&b, width)
	case fsPhaseBreak:
		fs.viewBreak(&b, width)
	case fsPhaseReview:
		fs.viewReview(&b, width)
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// viewSetup renders the setup phase: duration picker, optional task, goal input.
func (fs FocusSession) viewSetup(b *strings.Builder, width int) {
	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	activeStyle := lipgloss.NewStyle().Foreground(crust).Background(mauve).Bold(true).Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().Foreground(overlay0).Padding(0, 1)
	highlightStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)

	// Duration selector
	fieldIndicator := " "
	if fs.setupField == 0 {
		fieldIndicator = highlightStyle.Render(">")
	}
	b.WriteString(fieldIndicator + " " + sectionStyle.Render("Duration"))
	b.WriteString("\n  ")
	for i, d := range fs.durations {
		label := fmt.Sprintf("%d min", d)
		if i == fs.durIdx {
			b.WriteString(activeStyle.Render(label))
		} else {
			b.WriteString(inactiveStyle.Render(label))
		}
		if i < len(fs.durations)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n\n")

	// Task selector
	if len(fs.tasks) == 0 {
		b.WriteString("  " + DimStyle.Render("No tasks in Tasks.md — add with Ctrl+K or Ctrl+T"))
		b.WriteString("\n\n")
	}
	if len(fs.tasks) > 0 {
		b.WriteString("  " + sectionStyle.Render("Task (optional)"))
		b.WriteString("\n")

		maxVisible := 5
		if len(fs.tasks) < maxVisible {
			maxVisible = len(fs.tasks)
		}

		// "No task" option
		if fs.taskIdx == -1 && fs.setupField == 0 {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Bold(true).Render("> (none)"))
		} else if fs.taskIdx == -1 {
			b.WriteString("  " + labelStyle.Render("  (none)"))
		} else {
			b.WriteString("  " + DimStyle.Render("  (none)"))
		}
		b.WriteString("\n")

		start := fs.taskScroll
		end := start + maxVisible
		if end > len(fs.tasks) {
			end = len(fs.tasks)
		}

		for i := start; i < end; i++ {
			taskText := TruncateDisplay(fs.tasks[i], width-12)
			if i == fs.taskIdx && fs.setupField == 0 {
				b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Bold(true).Render("> "+taskText))
			} else {
				b.WriteString("  " + DimStyle.Render("  "+taskText))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Goal input
	fieldIndicator = " "
	if fs.setupField == 1 {
		fieldIndicator = highlightStyle.Render(">")
	}
	b.WriteString(fieldIndicator + " " + sectionStyle.Render("Session Goal"))
	b.WriteString("\n")

	goalDisplay := fs.goalInput
	if fs.setupField == 1 {
		goalDisplay += lipgloss.NewStyle().Background(text).Foreground(mantle).Render(" ")
	}
	if goalDisplay == "" && fs.setupField != 1 {
		goalDisplay = DimStyle.Render("(type your goal here)")
	}
	inputBox := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1).
		Width(width - 10)
	b.WriteString("  " + inputBox.Render(goalDisplay))
	b.WriteString("\n\n")

	// Controls
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")
	ctrlStyle := lipgloss.NewStyle().Foreground(overlay1)
	b.WriteString(ctrlStyle.Render("  Tab: switch field  "))
	b.WriteString(ctrlStyle.Render("\u2190/\u2192: duration  "))
	b.WriteString(ctrlStyle.Render("Enter: start  Esc: close"))
}

// viewActive renders the active session phase: timer, progress, scratchpad.
func (fs FocusSession) viewActive(b *strings.Builder, width int) {
	// Phase label
	if fs.paused {
		pauseLabel := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("PAUSED")
		pad := (width - 4 - lipgloss.Width(pauseLabel)) / 2
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad) + pauseLabel)
	} else {
		focusLabel := lipgloss.NewStyle().Foreground(green).Bold(true).Render("FOCUSING")
		pad := (width - 4 - lipgloss.Width(focusLabel)) / 2
		if pad < 0 {
			pad = 0
		}
		b.WriteString(strings.Repeat(" ", pad) + focusLabel)
	}
	b.WriteString("\n\n")

	// Big countdown timer (remaining = duration - elapsed)
	remaining := fs.duration - fs.elapsed
	if remaining < 0 {
		remaining = 0
	}
	mins := int(remaining.Minutes())
	secs := int(remaining.Seconds()) % 60
	timerStr := fmt.Sprintf("%02d:%02d", mins, secs)

	timerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	if fs.paused {
		timerStyle = lipgloss.NewStyle().Foreground(overlay0).Bold(true)
	}
	timerRendered := timerStyle.Render(timerStr)
	timerPad := (width - 4 - lipgloss.Width(timerRendered)) / 2
	if timerPad < 0 {
		timerPad = 0
	}
	b.WriteString(strings.Repeat(" ", timerPad) + timerRendered)
	b.WriteString("\n\n")

	// Progress bar
	barWidth := width - 8
	if barWidth < 10 {
		barWidth = 10
	}
	filled := 0
	if fs.duration > 0 {
		filled = int(float64(barWidth) * float64(fs.elapsed) / float64(fs.duration))
		if filled > barWidth {
			filled = barWidth
		}
		if filled < 0 {
			filled = 0
		}
	}
	empty := barWidth - filled

	filledStyle := lipgloss.NewStyle().Foreground(mauve)
	emptyStyle := lipgloss.NewStyle().Foreground(surface1)
	bar := "  " + filledStyle.Render(strings.Repeat("\u2588", filled)) +
		emptyStyle.Render(strings.Repeat("\u2591", empty))
	b.WriteString(bar)

	// Percentage
	pct := 0
	if fs.duration > 0 {
		pct = int(100 * float64(fs.elapsed) / float64(fs.duration))
		if pct > 100 {
			pct = 100
		}
	}
	pctStr := lipgloss.NewStyle().Foreground(subtext0).Render(fmt.Sprintf(" %d%%", pct))
	b.WriteString(pctStr)
	b.WriteString("\n\n")

	// Goal display
	if fs.sessionGoal != "" {
		goalLabel := lipgloss.NewStyle().Foreground(blue).Bold(true)
		b.WriteString("  " + goalLabel.Render("Goal: "))
		b.WriteString(lipgloss.NewStyle().Foreground(text).Render(fs.sessionGoal))
		b.WriteString("\n")
	}

	// Task display
	if fs.sessionTask != "" {
		taskLabel := lipgloss.NewStyle().Foreground(teal).Bold(true)
		b.WriteString("  " + taskLabel.Render("Task: "))
		taskText := TruncateDisplay(fs.sessionTask, width-14)
		b.WriteString(lipgloss.NewStyle().Foreground(text).Render(taskText))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Scratchpad
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")
	scratchLabel := lipgloss.NewStyle().Foreground(peach).Bold(true)
	b.WriteString("  " + scratchLabel.Render("Scratchpad"))
	b.WriteString("\n")

	// Show scratchpad content with cursor
	scratchHeight := fs.height/4 - 2
	if scratchHeight < 3 {
		scratchHeight = 3
	}
	if scratchHeight > 10 {
		scratchHeight = 10
	}

	scratchContent := fs.scratchpad
	scratchContent += lipgloss.NewStyle().Background(text).Foreground(mantle).Render(" ")

	scratchLines := strings.Split(scratchContent, "\n")
	// Show only the last scratchHeight lines (auto-scroll)
	if len(scratchLines) > scratchHeight {
		scratchLines = scratchLines[len(scratchLines)-scratchHeight:]
	}

	scratchBox := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Width(width - 10).
		Height(scratchHeight).
		Padding(0, 1)

	boxContent := strings.Join(scratchLines, "\n")
	b.WriteString("  " + scratchBox.Render(boxContent))
	b.WriteString("\n\n")

	// Controls
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")
	ctrlStyle := lipgloss.NewStyle().Foreground(overlay1)
	b.WriteString(ctrlStyle.Render("  Ctrl+P: pause  Esc: end session  (type to add notes)"))
}

// viewBreak renders the break phase: congratulations + break timer.
func (fs FocusSession) viewBreak(b *strings.Builder, width int) {
	// Celebration message
	doneLabel := lipgloss.NewStyle().Foreground(green).Bold(true).Render("Session Complete!")
	donePad := (width - 4 - lipgloss.Width(doneLabel)) / 2
	if donePad < 0 {
		donePad = 0
	}
	b.WriteString(strings.Repeat(" ", donePad) + doneLabel)
	b.WriteString("\n\n")

	breakMsg := lipgloss.NewStyle().Foreground(blue).Bold(true).Render("Take a break!")
	breakPad := (width - 4 - lipgloss.Width(breakMsg)) / 2
	if breakPad < 0 {
		breakPad = 0
	}
	b.WriteString(strings.Repeat(" ", breakPad) + breakMsg)
	b.WriteString("\n\n")

	// Session summary
	statLabel := lipgloss.NewStyle().Foreground(subtext0)
	statValue := lipgloss.NewStyle().Foreground(text)
	durMins := int(fs.totalElapsed.Minutes())
	b.WriteString("  " + statLabel.Render("Duration: ") + statValue.Render(fmt.Sprintf("%d min", durMins)))
	b.WriteString("\n")

	if fs.sessionGoal != "" {
		b.WriteString("  " + statLabel.Render("Goal: ") + statValue.Render(fs.sessionGoal))
		b.WriteString("\n")
	}

	scratchWords := len(strings.Fields(fs.sessionNotes))
	b.WriteString("  " + statLabel.Render("Words in scratchpad: ") + statValue.Render(fmt.Sprintf("%d", scratchWords)))
	b.WriteString("\n\n")

	// Break duration selector
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")
	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	b.WriteString("  " + sectionStyle.Render("Break Duration"))
	b.WriteString("\n  ")

	activeStyle := lipgloss.NewStyle().Foreground(crust).Background(mauve).Bold(true).Padding(0, 1)
	inactiveStyle := lipgloss.NewStyle().Foreground(overlay0).Padding(0, 1)

	if fs.breakActive {
		// Show break countdown
		breakMin := 5
		if fs.breakIdx >= 0 && fs.breakIdx < len(fs.breakDurs) {
			breakMin = fs.breakDurs[fs.breakIdx]
		}
		breakDur := time.Duration(breakMin) * time.Minute
		breakRemaining := breakDur - fs.breakElapsed
		if breakRemaining < 0 {
			breakRemaining = 0
		}
		mins := int(breakRemaining.Minutes())
		secs := int(breakRemaining.Seconds()) % 60
		timerStr := fmt.Sprintf("%02d:%02d", mins, secs)
		timerStyle := lipgloss.NewStyle().Foreground(teal).Bold(true)
		timerRendered := timerStyle.Render(timerStr)
		timerPad := (width - 4 - lipgloss.Width(timerRendered)) / 2
		if timerPad < 0 {
			timerPad = 0
		}
		b.WriteString("\n")
		b.WriteString(strings.Repeat(" ", timerPad) + timerRendered)

		// Break progress bar
		barWidth := width - 8
		if barWidth < 10 {
			barWidth = 10
		}
		bFilled := 0
		if breakDur > 0 {
			bFilled = int(float64(barWidth) * float64(fs.breakElapsed) / float64(breakDur))
			if bFilled > barWidth {
				bFilled = barWidth
			}
		}
		bEmpty := barWidth - bFilled
		filledStyle := lipgloss.NewStyle().Foreground(teal)
		emptyStyle := lipgloss.NewStyle().Foreground(surface1)
		b.WriteString("\n  " + filledStyle.Render(strings.Repeat("\u2588", bFilled)) +
			emptyStyle.Render(strings.Repeat("\u2591", bEmpty)))
	} else {
		for i, d := range fs.breakDurs {
			label := fmt.Sprintf("%d min", d)
			if i == fs.breakIdx {
				b.WriteString(activeStyle.Render(label))
			} else {
				b.WriteString(inactiveStyle.Render(label))
			}
			if i < len(fs.breakDurs)-1 {
				b.WriteString("  ")
			}
		}
	}
	b.WriteString("\n\n")

	// Controls
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")
	ctrlStyle := lipgloss.NewStyle().Foreground(overlay1)
	if fs.breakActive {
		b.WriteString(ctrlStyle.Render("  Esc: skip to review"))
	} else {
		b.WriteString(ctrlStyle.Render("  Enter: start break  s: skip  \u2190/\u2192: duration  Esc: review"))
	}
}

// viewReview renders the review phase: session stats + save option.
func (fs FocusSession) viewReview(b *strings.Builder, width int) {
	// Title
	reviewLabel := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Session Review")
	reviewPad := (width - 4 - lipgloss.Width(reviewLabel)) / 2
	if reviewPad < 0 {
		reviewPad = 0
	}
	b.WriteString(strings.Repeat(" ", reviewPad) + reviewLabel)
	b.WriteString("\n\n")

	statLabel := lipgloss.NewStyle().Foreground(subtext0)
	statValue := lipgloss.NewStyle().Foreground(text).Bold(true)

	// Duration
	durMins := int(fs.totalElapsed.Minutes())
	durSecs := int(fs.totalElapsed.Seconds()) % 60
	b.WriteString("  " + statLabel.Render("Duration:    ") +
		statValue.Render(fmt.Sprintf("%d min %d sec", durMins, durSecs)))
	b.WriteString("\n")

	// Time range
	endTime := fs.startTime.Add(fs.totalElapsed)
	b.WriteString("  " + statLabel.Render("Time:        ") +
		statValue.Render(fmt.Sprintf("%s - %s", fs.startTime.Format("15:04"), endTime.Format("15:04"))))
	b.WriteString("\n")

	// Goal
	if fs.sessionGoal != "" {
		b.WriteString("  " + statLabel.Render("Goal:        ") +
			lipgloss.NewStyle().Foreground(text).Render(fs.sessionGoal))
		b.WriteString("\n")
	}

	// Task
	if fs.sessionTask != "" {
		b.WriteString("  " + statLabel.Render("Task:        ") +
			lipgloss.NewStyle().Foreground(text).Render(fs.sessionTask))
		b.WriteString("\n")
	}

	// Words written
	scratchWords := len(strings.Fields(fs.sessionNotes))
	wordColor := green
	if scratchWords == 0 {
		wordColor = overlay0
	}
	b.WriteString("  " + statLabel.Render("Words:       ") +
		lipgloss.NewStyle().Foreground(wordColor).Bold(true).Render(fmt.Sprintf("%d", scratchWords)))
	b.WriteString("\n")

	// Notes preview
	if fs.sessionNotes != "" {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
		b.WriteString("\n")
		notesLabel := lipgloss.NewStyle().Foreground(peach).Bold(true)
		b.WriteString("  " + notesLabel.Render("Session Notes"))
		b.WriteString("\n")

		previewLines := strings.Split(fs.sessionNotes, "\n")
		maxPreview := 6
		if len(previewLines) > maxPreview {
			previewLines = previewLines[:maxPreview]
			previewLines = append(previewLines, DimStyle.Render("..."))
		}
		for _, line := range previewLines {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(text).Render(line))
			b.WriteString("\n")
		}
	}

	// Save path info
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")
	savePath := fmt.Sprintf("FocusSessions/%s.md", time.Now().Format("2006-01-02"))
	b.WriteString("  " + DimStyle.Render("Will save to: "+savePath))
	b.WriteString("\n\n")

	// Controls
	ctrlStyle := lipgloss.NewStyle().Foreground(overlay1)
	b.WriteString(ctrlStyle.Render("  Enter: save & close  Esc: close without saving"))
}
