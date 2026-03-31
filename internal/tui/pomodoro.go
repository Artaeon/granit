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

// PomodoroState represents the current phase of the Pomodoro timer.
type PomodoroState int

const (
	PomodoroIdle       PomodoroState = iota
	PomodoroWork
	PomodoroShortBreak
	PomodoroBreakonLong
)

// QueueTask represents a single task in the focus queue.
type QueueTask struct {
	Text       string
	Priority   int
	Project    string // project name (for tracking)
	Estimated  int    // estimated minutes
	Elapsed    int    // actual minutes spent
	Done       bool
	SourcePath string // relative path to source note (for bidirectional sync)
	SourceLine int    // 0-based line number in source note
}

// Pomodoro implements an overlay Pomodoro timer with writing stats tracking.
type Pomodoro struct {
	active bool
	width  int
	height int

	state     PomodoroState
	remaining time.Duration
	total     time.Duration
	paused    bool

	// Config
	workDuration   time.Duration // default 25min
	shortBreak     time.Duration // default 5min
	longBreak      time.Duration // default 15min
	longBreakAfter int           // default 4 sessions

	// Stats
	sessionsToday int
	totalSessions int
	wordsWritten  int
	wordsAtStart  int
	notesEdited   map[string]bool

	// History
	startTime  time.Time
	sessionLog []pomodoroSession

	// Focus queue
	queue       []QueueTask    // ordered task queue for the day
	queueCursor int            // current position in queue
	showQueue   bool           // show queue panel in overlay
	queueScroll int            // scroll offset for long queue lists

	// Time tracking
	taskTimeLog map[string]int // task text -> minutes spent
	currentTask string         // task being worked on now

	// Queue add-task input mode
	addingTask   bool
	addTaskInput string

	// Vault root for session logging
	vaultRoot string

	// Completed tasks to sync back to source files (consumed-once by app)
	completedTasks []TaskCompletion

	// Daily session goal
	pomodoroGoal int // target sessions per day, 0 means use config default
}

type pomodoroSession struct {
	Start    time.Time
	Duration time.Duration
	Words    int
	Notes    int
	Task     string // task worked on during this session
}

// pomodoroTickMsg is sent every second while the timer is running.
type pomodoroTickMsg struct{}

// NewPomodoro creates a new Pomodoro timer with sensible defaults.
func NewPomodoro() Pomodoro {
	return Pomodoro{
		state:          PomodoroIdle,
		workDuration:   25 * time.Minute,
		shortBreak:     5 * time.Minute,
		longBreak:      15 * time.Minute,
		longBreakAfter: 4,
		notesEdited:    make(map[string]bool),
		taskTimeLog:    make(map[string]int),
		showQueue:      true,
	}
}

// IsActive reports whether the Pomodoro overlay is open.
func (p *Pomodoro) IsActive() bool {
	return p.active
}

// Open displays the Pomodoro overlay.
func (p *Pomodoro) Open() {
	p.active = true
}

// Close hides the Pomodoro overlay (timer keeps running in background).
func (p *Pomodoro) Close() {
	p.active = false
}

// SetSize updates the available terminal dimensions.
func (p *Pomodoro) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// SetVaultRoot stores the vault root path for session logging.
func (p *Pomodoro) SetVaultRoot(root string) {
	p.vaultRoot = root
}

// SetGoal sets the daily session goal.
func (p *Pomodoro) SetGoal(goal int) {
	p.pomodoroGoal = goal
}

// GetCompletedTasks returns and clears the list of task completions that
// should be synced back to their source files. Consumed-once pattern.
func (p *Pomodoro) GetCompletedTasks() []TaskCompletion {
	if len(p.completedTasks) == 0 {
		return nil
	}
	result := p.completedTasks
	p.completedTasks = nil
	return result
}

// Start begins a new work session.
func (p *Pomodoro) Start() {
	p.state = PomodoroWork
	p.remaining = p.workDuration
	p.total = p.workDuration
	p.paused = false
	p.startTime = time.Now()
	p.wordsWritten = 0

	// Set current task from queue if available
	if qt := p.CurrentQueueTask(); qt != nil {
		p.currentTask = qt.Text
	}
}

// Pause toggles the timer between running and paused. When paused the state
// is preserved but ticks stop being consumed.
func (p *Pomodoro) Pause() {
	if p.state == PomodoroIdle {
		return
	}
	// Toggle pause: negative remaining = paused, positive = running.
	// Only negate if the sign change is meaningful (prevents double-toggle).
	if p.paused {
		if p.remaining < 0 {
			p.remaining = -p.remaining
		}
		p.paused = false
	} else {
		if p.remaining > 0 {
			p.remaining = -p.remaining
		}
		p.paused = true
	}
}

// Skip advances to the next phase: work -> break, break -> idle (ready for
// next work session).
func (p *Pomodoro) Skip() {
	switch p.state {
	case PomodoroWork:
		p.finishWorkSession()
		p.startBreak()
	case PomodoroShortBreak, PomodoroBreakonLong:
		p.state = PomodoroIdle
		p.remaining = 0
		p.total = 0
	}
}

// UpdateWordCount records the current total word count so that words written
// during this session can be computed.
func (p *Pomodoro) UpdateWordCount(count int) {
	if p.state == PomodoroWork {
		if p.wordsAtStart == 0 {
			p.wordsAtStart = count
		}
		p.wordsWritten = count - p.wordsAtStart
		if p.wordsWritten < 0 {
			p.wordsWritten = 0
		}
	}
}

// NoteEdited records that a note was worked on during this session.
func (p *Pomodoro) NoteEdited(path string) {
	if p.notesEdited == nil {
		p.notesEdited = make(map[string]bool)
	}
	p.notesEdited[path] = true
}

// IsRunning reports whether the timer is actively counting down.
func (p *Pomodoro) IsRunning() bool {
	return p.state != PomodoroIdle && p.remaining > 0
}

// ---------------------------------------------------------------------------
// Focus Queue methods
// ---------------------------------------------------------------------------

// SetQueue replaces the focus queue with the provided tasks.
func (p *Pomodoro) SetQueue(tasks []QueueTask) {
	p.queue = tasks
	p.queueCursor = 0
	p.queueScroll = 0
	// Advance to first undone task
	for i, t := range p.queue {
		if !t.Done {
			p.queueCursor = i
			break
		}
	}
	if len(p.queue) > 0 {
		p.showQueue = true
		qt := p.CurrentQueueTask()
		if qt != nil {
			p.currentTask = qt.Text
		}
	}
}

// AddToQueue appends a task to the focus queue.
func (p *Pomodoro) AddToQueue(task QueueTask) {
	p.queue = append(p.queue, task)
	if len(p.queue) == 1 {
		p.queueCursor = 0
		p.currentTask = task.Text
	}
}

// CurrentQueueTask returns the current (non-done) task in the queue, or nil.
func (p *Pomodoro) CurrentQueueTask() *QueueTask {
	if len(p.queue) == 0 {
		return nil
	}
	for i := p.queueCursor; i < len(p.queue); i++ {
		if !p.queue[i].Done {
			p.queueCursor = i
			return &p.queue[i]
		}
	}
	return nil
}

// AdvanceQueue marks the current task done, records elapsed time, and moves
// to the next undone task. Returns true if there is a next task.
func (p *Pomodoro) AdvanceQueue() bool {
	qt := p.CurrentQueueTask()
	if qt == nil {
		return false
	}

	// Track any remaining in-progress time first
	p.trackElapsedOnCurrentTask()
	// Now mark done and log
	qt.Done = true
	p.logSessionEntry(qt.Text, qt.Project, qt.Elapsed, true)

	// Record completion for sync back to source file
	if qt.SourcePath != "" {
		p.completedTasks = append(p.completedTasks, TaskCompletion{
			NotePath: qt.SourcePath,
			LineNum:  qt.SourceLine,
			Text:     qt.Text,
			Done:     true,
		})
	}

	// Move to next undone task
	for i := p.queueCursor + 1; i < len(p.queue); i++ {
		if !p.queue[i].Done {
			p.queueCursor = i
			p.currentTask = p.queue[i].Text
			// Auto-start a new pomodoro for the next task
			p.state = PomodoroWork
			p.remaining = p.workDuration
			p.total = p.workDuration
			p.startTime = time.Now()
			p.wordsWritten = 0
			p.wordsAtStart = 0
			p.notesEdited = make(map[string]bool)
			return true
		}
	}

	// All tasks done
	p.currentTask = ""
	return false
}

// SkipTask skips the current queue task without marking done, moves to next.
func (p *Pomodoro) SkipTask() bool {
	if len(p.queue) == 0 {
		return false
	}
	if p.queueCursor < 0 {
		p.queueCursor = 0
	}
	for i := p.queueCursor + 1; i < len(p.queue); i++ {
		if !p.queue[i].Done {
			p.queueCursor = i
			p.currentTask = p.queue[i].Text
			return true
		}
	}
	return false
}

// GetTimeLog returns the accumulated time log mapping task text to minutes.
func (p *Pomodoro) GetTimeLog() map[string]int {
	return p.taskTimeLog
}

// queueComplete returns true if all queue tasks are done.
func (p *Pomodoro) queueComplete() bool {
	for _, t := range p.queue {
		if !t.Done {
			return false
		}
	}
	return len(p.queue) > 0
}

// StatusString returns a short status string suitable for the status bar.
// Returns an empty string when idle.
func (p *Pomodoro) StatusString() string {
	if p.state == PomodoroIdle {
		return ""
	}
	rem := p.remaining
	if rem < 0 {
		rem = -rem // paused: show absolute value
	}
	mins := int(rem.Minutes())
	secs := int(rem.Seconds()) % 60
	switch p.state {
	case PomodoroWork:
		base := fmt.Sprintf("\U0001f345 %d:%02d", mins, secs) // tomato emoji
		if p.currentTask != "" {
			taskLabel := TruncateDisplay(p.currentTask, 25)
			base += " \u00b7 " + taskLabel
		}
		return base
	case PomodoroShortBreak, PomodoroBreakonLong:
		return fmt.Sprintf("\u2615 %d:%02d", mins, secs) // coffee emoji
	}
	return ""
}

// Update handles messages for the Pomodoro overlay (value receiver per overlay
// pattern).
func (p Pomodoro) Update(msg tea.Msg) (Pomodoro, tea.Cmd) {
	switch msg := msg.(type) {
	case pomodoroTickMsg:
		if p.state == PomodoroIdle {
			return p, nil
		}
		// If paused (negative remaining), just re-schedule tick without decrementing.
		if p.remaining < 0 {
			return p, p.tick()
		}
		p.remaining -= time.Second
		if p.remaining <= 0 {
			p.remaining = 0
			switch p.state {
			case PomodoroWork:
				// Track elapsed time on current queue task
				p.trackElapsedOnCurrentTask()
				p.finishWorkSession()
				p.startBreak()
			case PomodoroShortBreak, PomodoroBreakonLong:
				p.state = PomodoroIdle
			}
			if p.state != PomodoroIdle {
				return p, p.tick()
			}
			return p, nil
		}
		return p, p.tick()

	case tea.KeyMsg:
		if !p.active {
			return p, nil
		}

		// Handle add-task input mode
		if p.addingTask {
			return p.updateAddTask(msg)
		}

		switch msg.String() {
		case "esc":
			p.active = false
			return p, nil
		case " ":
			if p.state == PomodoroIdle {
				p.Start()
				return p, p.tick()
			}
			p.Pause()
			return p, nil
		case "s":
			p.Skip()
			if p.IsRunning() {
				return p, p.tick()
			}
			return p, nil
		case "r":
			p.state = PomodoroIdle
			p.remaining = 0
			p.total = 0
			return p, nil
		case "enter":
			// Complete current queue task and advance
			if len(p.queue) > 0 && p.CurrentQueueTask() != nil {
				hasNext := p.AdvanceQueue()
				if hasNext {
					return p, p.tick()
				}
				// All done — stop timer
				p.state = PomodoroIdle
				p.remaining = 0
				p.total = 0
				return p, nil
			}
			return p, nil
		case "n":
			// Skip to next task in queue
			if len(p.queue) > 0 {
				p.SkipTask()
			}
			return p, nil
		case "a":
			// Enter add-task mode
			p.addingTask = true
			p.addTaskInput = ""
			return p, nil
		case "q":
			// Toggle queue panel visibility
			p.showQueue = !p.showQueue
			return p, nil
		case "j":
			// Scroll queue down
			if p.showQueue && len(p.queue) > 0 {
				maxScroll := len(p.queue) - 1
				if p.queueScroll < maxScroll {
					p.queueScroll++
				}
			}
			return p, nil
		case "k":
			// Scroll queue up
			if p.showQueue && p.queueScroll > 0 {
				p.queueScroll--
			}
			return p, nil
		}
	}

	return p, nil
}

// updateAddTask handles key events while in the add-task mini input mode.
func (p Pomodoro) updateAddTask(msg tea.KeyMsg) (Pomodoro, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.addingTask = false
		p.addTaskInput = ""
		return p, nil
	case "enter":
		text := strings.TrimSpace(p.addTaskInput)
		if text != "" {
			p.AddToQueue(QueueTask{
				Text:      text,
				Estimated: int(p.workDuration.Minutes()),
			})
		}
		p.addingTask = false
		p.addTaskInput = ""
		return p, nil
	case "backspace":
		if len(p.addTaskInput) > 0 {
			p.addTaskInput = p.addTaskInput[:len(p.addTaskInput)-1]
		}
		return p, nil
	default:
		for _, r := range msg.Runes {
			p.addTaskInput += string(r)
		}
		return p, nil
	}
}

// View renders the Pomodoro overlay.
func (p Pomodoro) View() string {
	width := p.width / 2
	if width < 52 {
		width = 52
	}
	if width > 65 {
		width = 65
	}

	var b strings.Builder

	// Title
	titleIcon := lipgloss.NewStyle().Foreground(peach).Render("\U0001f345")
	titleText := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" Pomodoro Timer")
	b.WriteString("  " + titleIcon + titleText)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n\n")

	// State indicator
	stateLabel := ""
	switch p.state {
	case PomodoroIdle:
		stateLabel = lipgloss.NewStyle().Foreground(overlay0).Bold(true).Render("READY")
	case PomodoroWork:
		stateLabel = lipgloss.NewStyle().Foreground(green).Bold(true).Render("FOCUS")
	case PomodoroShortBreak:
		stateLabel = lipgloss.NewStyle().Foreground(blue).Bold(true).Render("SHORT BREAK")
	case PomodoroBreakonLong:
		stateLabel = lipgloss.NewStyle().Foreground(peach).Bold(true).Render("LONG BREAK")
	}
	statePad := (width - 4 - lipgloss.Width(stateLabel)) / 2
	if statePad < 0 {
		statePad = 0
	}
	b.WriteString(strings.Repeat(" ", statePad) + stateLabel)
	b.WriteString("\n\n")

	// Big timer display (MM:SS)
	rem := p.remaining
	paused := false
	if rem < 0 {
		rem = -rem
		paused = true
	}
	mins := int(rem.Minutes())
	secs := int(rem.Seconds()) % 60
	timerStr := fmt.Sprintf("%02d:%02d", mins, secs)

	timerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	if paused {
		timerStyle = lipgloss.NewStyle().Foreground(overlay0).Bold(true)
	}
	timerRendered := timerStyle.Render(timerStr)
	timerPad := (width - 4 - lipgloss.Width(timerRendered)) / 2
	if timerPad < 0 {
		timerPad = 0
	}
	b.WriteString(strings.Repeat(" ", timerPad) + timerRendered)
	if paused {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(yellow).Render("(paused)"))
	}
	b.WriteString("\n\n")

	// Progress bar
	if p.state != PomodoroIdle && p.total > 0 {
		barWidth := width - 8
		if barWidth < 10 {
			barWidth = 10
		}
		elapsed := p.total - rem
		filled := int(float64(barWidth) * float64(elapsed) / float64(p.total))
		if filled > barWidth {
			filled = barWidth
		}
		if filled < 0 {
			filled = 0
		}
		empty := barWidth - filled
		pct := 0
		if p.total > 0 {
			pct = int(100 * float64(elapsed) / float64(p.total))
			if pct > 100 {
				pct = 100
			}
		}

		filledStyle := lipgloss.NewStyle().Foreground(mauve)
		emptyStyle := lipgloss.NewStyle().Foreground(surface1)

		bar := "  " + filledStyle.Render(strings.Repeat("\u2588", filled)) +
			emptyStyle.Render(strings.Repeat("\u2591", empty)) +
			lipgloss.NewStyle().Foreground(subtext0).Render(fmt.Sprintf(" %d%%", pct))
		b.WriteString(bar)
		b.WriteString("\n\n")
	}

	// Current task display (when queue is active)
	if p.currentTask != "" && len(p.queue) > 0 {
		nowLabel := lipgloss.NewStyle().Foreground(green).Bold(true).Render("\u25b6 NOW: ")
		taskText := TruncateDisplay(p.currentTask, width-18)
		b.WriteString("  " + nowLabel + lipgloss.NewStyle().Foreground(text).Bold(true).Render(taskText))
		b.WriteString("\n")

		if qt := p.CurrentQueueTask(); qt != nil {
			details := "  "
			if qt.Project != "" {
				details += lipgloss.NewStyle().Foreground(blue).Render("Project: "+qt.Project)
				if qt.Estimated > 0 {
					details += lipgloss.NewStyle().Foreground(overlay0).Render(" \u00b7 ")
				}
			}
			if qt.Estimated > 0 {
				details += lipgloss.NewStyle().Foreground(subtext0).Render(
					fmt.Sprintf("Est: %dmin", qt.Estimated))
			}
			b.WriteString("  " + details)
			b.WriteString("\n")
			b.WriteString("  " + lipgloss.NewStyle().Foreground(subtext0).Render(
				fmt.Sprintf("  Time spent: %dmin", qt.Elapsed)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Focus Queue panel
	if p.showQueue && len(p.queue) > 0 {
		b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
		b.WriteString("\n")
		queueTitle := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("FOCUS QUEUE")
		b.WriteString("  " + queueTitle)
		b.WriteString("\n")

		maxVisible := 6
		start := p.queueScroll
		end := start + maxVisible
		if end > len(p.queue) {
			end = len(p.queue)
		}

		for i := start; i < end; i++ {
			qt := p.queue[i]
			num := fmt.Sprintf("%d. ", i+1)
			icon := "\u25cb" // ○ not done
			iconStyle := lipgloss.NewStyle().Foreground(overlay0)
			textStyle := lipgloss.NewStyle().Foreground(text)

			if qt.Done {
				icon = "\u2713" // ✓
				iconStyle = lipgloss.NewStyle().Foreground(green)
				textStyle = lipgloss.NewStyle().Foreground(overlay0).Strikethrough(true)
			} else if i == p.queueCursor {
				icon = "\u25b6" // ▶
				iconStyle = lipgloss.NewStyle().Foreground(green).Bold(true)
				textStyle = lipgloss.NewStyle().Foreground(text).Bold(true)
			}

			taskLabel := qt.Text
			timeInfo := ""
			if qt.Estimated > 0 {
				timeInfo = fmt.Sprintf("%dmin / %dmin", qt.Elapsed, qt.Estimated)
			} else {
				timeInfo = fmt.Sprintf("%dmin", qt.Elapsed)
			}

			// Calculate available width for task text
			timeWidth := len(timeInfo) + 2
			numWidth := len(num) + 2 // icon + space
			maxTaskWidth := width - 8 - numWidth - timeWidth
			if maxTaskWidth < 10 {
				maxTaskWidth = 10
			}
			taskLabel = TruncateDisplay(taskLabel, maxTaskWidth)

			// Pad to right-align time info
			leftPart := num + iconStyle.Render(icon) + " " + textStyle.Render(taskLabel)
			leftWidth := lipgloss.Width(leftPart)
			padding := width - 8 - leftWidth - len(timeInfo)
			if padding < 1 {
				padding = 1
			}

			timeStyle := lipgloss.NewStyle().Foreground(subtext0)
			if qt.Done {
				timeStyle = lipgloss.NewStyle().Foreground(green)
			}

			b.WriteString("  " + leftPart + strings.Repeat(" ", padding) + timeStyle.Render(timeInfo))
			b.WriteString("\n")
		}

		if len(p.queue) > maxVisible {
			remaining := len(p.queue) - end
			if remaining > 0 {
				b.WriteString("  " + DimStyle.Render(fmt.Sprintf("  ... %d more", remaining)))
				b.WriteString("\n")
			}
		}

		// Queue completion message
		if p.queueComplete() {
			b.WriteString("\n")
			doneMsg := lipgloss.NewStyle().Foreground(green).Bold(true).Render(
				"  All tasks complete!")
			b.WriteString("  " + doneMsg)
			b.WriteString("\n")
		}
	}

	// Stats section
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")

	statLabel := lipgloss.NewStyle().Foreground(subtext0)
	statValue := lipgloss.NewStyle().Foreground(text)

	// Daily goal progress
	goal := p.pomodoroGoal
	if goal <= 0 {
		goal = 8
	}
	sessionLabel := fmt.Sprintf("%d/%d today", p.sessionsToday, goal)
	if p.sessionsToday >= goal {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Bold(true).Render(
			"Sessions: "+sessionLabel+" \u2714 Goal reached!"))
	} else {
		b.WriteString("  " + statLabel.Render("Sessions: ") + statValue.Render(sessionLabel))
	}

	// Mini progress bar for daily goal
	goalBarWidth := 20
	goalFilled := int(float64(goalBarWidth) * float64(p.sessionsToday) / float64(goal))
	if goalFilled > goalBarWidth {
		goalFilled = goalBarWidth
	}
	goalEmpty := goalBarWidth - goalFilled
	goalBarColor := mauve
	if p.sessionsToday >= goal {
		goalBarColor = green
	}
	b.WriteString("  " +
		lipgloss.NewStyle().Foreground(goalBarColor).Render(strings.Repeat("\u2588", goalFilled)) +
		lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("\u2591", goalEmpty)))
	b.WriteString("\n")

	// Compact stats line
	totalWords := 0
	for _, s := range p.sessionLog {
		totalWords += s.Words
	}
	totalWords += p.wordsWritten
	b.WriteString("  " +
		statLabel.Render("Words: ") +
		statValue.Render(fmt.Sprintf("%d", totalWords)) +
		statLabel.Render(" \u00b7 Notes: ") +
		statValue.Render(fmt.Sprintf("%d", len(p.notesEdited))))
	b.WriteString("\n")

	// Session history (last 5)
	if len(p.sessionLog) > 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("Recent Sessions"))
		b.WriteString("\n")

		start := 0
		if len(p.sessionLog) > 5 {
			start = len(p.sessionLog) - 5
		}
		for _, s := range p.sessionLog[start:] {
			timeStr := s.Start.Format("15:04")
			durStr := fmt.Sprintf("%dm", int(s.Duration.Minutes()))
			wordStr := fmt.Sprintf("%dw", s.Words)
			noteStr := fmt.Sprintf("%dn", s.Notes)

			line := "  " +
				lipgloss.NewStyle().Foreground(overlay0).Render(timeStr) + "  " +
				lipgloss.NewStyle().Foreground(text).Render(durStr) + "  " +
				lipgloss.NewStyle().Foreground(green).Render(wordStr) + "  " +
				lipgloss.NewStyle().Foreground(blue).Render(noteStr)
			if s.Task != "" {
				taskLabel := s.Task
				taskLabel = TruncateDisplay(taskLabel, 20)
				line += "  " + lipgloss.NewStyle().Foreground(subtext0).Render(taskLabel)
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	// Add-task input mode
	if p.addingTask {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
		b.WriteString("\n")
		b.WriteString("  " + lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("Add task: "))
		inputDisplay := p.addTaskInput +
			lipgloss.NewStyle().Background(text).Foreground(mantle).Render(" ")
		inputBox := lipgloss.NewStyle().
			Foreground(text).
			Background(surface0).
			Padding(0, 1).
			Width(width - 18)
		b.WriteString(inputBox.Render(inputDisplay))
		b.WriteString("\n")
	}

	// Controls
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")
	if p.addingTask {
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "confirm"}, {"Esc", "cancel"},
		}))
	} else if len(p.queue) > 0 {
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Space", "start/pause"}, {"Enter", "complete"}, {"n", "skip"},
			{"a", "add task"}, {"q", "queue"}, {"Esc", "close"},
		}))
	} else {
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Space", "start/pause"}, {"s", "skip"}, {"r", "reset"},
			{"a", "add task"}, {"Esc", "close"},
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

// tick returns a command that sends a pomodoroTickMsg after 1 second.
func (p Pomodoro) tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return pomodoroTickMsg{}
	})
}

// trackElapsedOnCurrentTask adds the incremental elapsed pomodoro minutes
// (since last tracking) to the current queue task and the time log.
func (p *Pomodoro) trackElapsedOnCurrentTask() {
	if p.currentTask == "" || len(p.queue) == 0 {
		return
	}
	maxMins := int(p.workDuration.Minutes())
	rem := p.remaining
	if rem < 0 {
		rem = -rem
	}
	totalElapsed := int((p.workDuration - rem).Minutes())
	if totalElapsed < 0 {
		totalElapsed = 0
	}
	if totalElapsed > maxMins {
		totalElapsed = maxMins
	}

	// Compute incremental delta since last tracking to avoid double-counting
	qt := p.CurrentQueueTask()
	alreadyTracked := 0
	if qt != nil {
		alreadyTracked = qt.Elapsed
	}
	delta := totalElapsed - alreadyTracked
	if delta <= 0 {
		return
	}

	// Update queue task elapsed time
	if qt != nil {
		qt.Elapsed += delta
	}

	// Update time log
	if p.taskTimeLog == nil {
		p.taskTimeLog = make(map[string]int)
	}
	p.taskTimeLog[p.currentTask] += delta
}

// finishWorkSession records the completed work session in the log.
func (p *Pomodoro) finishWorkSession() {
	p.sessionsToday++
	p.totalSessions++
	session := pomodoroSession{
		Start:    p.startTime,
		Duration: p.workDuration,
		Words:    p.wordsWritten,
		Notes:    len(p.notesEdited),
		Task:     p.currentTask,
	}
	p.sessionLog = append(p.sessionLog, session)

	// Log non-queue session to file
	if p.currentTask != "" {
		project := ""
		if qt := p.CurrentQueueTask(); qt != nil {
			project = qt.Project
		}
		p.logSessionEntry(p.currentTask, project, int(p.workDuration.Minutes()), false)
	}

	// Reset per-session counters
	p.wordsWritten = 0
	p.wordsAtStart = 0
	p.notesEdited = make(map[string]bool)
}

// startBreak begins the appropriate break phase based on session count.
func (p *Pomodoro) startBreak() {
	if p.sessionsToday%p.longBreakAfter == 0 {
		p.state = PomodoroBreakonLong
		p.remaining = p.longBreak
		p.total = p.longBreak
	} else {
		p.state = PomodoroShortBreak
		p.remaining = p.shortBreak
		p.total = p.shortBreak
	}
	p.startTime = time.Now()
}

// logSessionEntry writes a session entry to FocusSessions/YYYY-MM-DD.md.
func (p *Pomodoro) logSessionEntry(task, project string, durationMin int, completed bool) {
	if p.vaultRoot == "" {
		return
	}
	dir := filepath.Join(p.vaultRoot, "FocusSessions")
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

	// Write header if file is new
	info, _ := f.Stat()
	if info != nil && info.Size() == 0 {
		header := fmt.Sprintf("# Focus Sessions - %s\n\n", now.Format("2006-01-02"))
		if _, err := f.WriteString(header); err != nil {
			return
		}
	}

	pomodoroCount := durationMin / int(p.workDuration.Minutes())
	if pomodoroCount < 1 {
		pomodoroCount = 1
	}

	status := "In progress"
	if completed {
		status = "Completed \u2713"
	}

	var entry strings.Builder
	entry.WriteString(fmt.Sprintf("## Session %s\n", now.Format("15:04")))
	entry.WriteString(fmt.Sprintf("- Task: %s\n", task))
	if project != "" {
		entry.WriteString(fmt.Sprintf("- Project: %s\n", project))
	}
	entry.WriteString(fmt.Sprintf("- Duration: %d min (%d pomodoros)\n", durationMin, pomodoroCount))
	entry.WriteString(fmt.Sprintf("- Status: %s\n", status))
	entry.WriteString("\n")

	if _, err := f.WriteString(entry.String()); err != nil {
		return
	}
}
