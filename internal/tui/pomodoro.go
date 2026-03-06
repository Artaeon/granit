package tui

import (
	"fmt"
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

// Pomodoro implements an overlay Pomodoro timer with writing stats tracking.
type Pomodoro struct {
	active bool
	width  int
	height int

	state     PomodoroState
	remaining time.Duration
	total     time.Duration

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
}

type pomodoroSession struct {
	Start    time.Time
	Duration time.Duration
	Words    int
	Notes    int
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

// Start begins a new work session.
func (p *Pomodoro) Start() {
	p.state = PomodoroWork
	p.remaining = p.workDuration
	p.total = p.workDuration
	p.startTime = time.Now()
	p.wordsWritten = 0
}

// Pause toggles the timer between running and paused. When paused the state
// is preserved but ticks stop being consumed.
func (p *Pomodoro) Pause() {
	if p.state == PomodoroIdle {
		return
	}
	// We signal "paused" by negating remaining. A negative remaining means
	// the timer is paused; ticks will not decrement it.
	p.remaining = -p.remaining
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
		return fmt.Sprintf("\U0001f345 %d:%02d", mins, secs) // tomato emoji
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
		}
	}

	return p, nil
}

// View renders the Pomodoro overlay.
func (p Pomodoro) View() string {
	width := p.width / 2
	if width < 44 {
		width = 44
	}
	if width > 60 {
		width = 60
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

		filledStyle := lipgloss.NewStyle().Foreground(mauve)
		emptyStyle := lipgloss.NewStyle().Foreground(surface1)

		bar := "  " + filledStyle.Render(strings.Repeat("\u2588", filled)) +
			emptyStyle.Render(strings.Repeat("\u2591", empty))
		b.WriteString(bar)
		b.WriteString("\n\n")
	}

	// Stats section
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")

	statLabel := lipgloss.NewStyle().Foreground(subtext0)
	statValue := lipgloss.NewStyle().Foreground(text)

	b.WriteString("  " + statLabel.Render("Sessions today: ") +
		statValue.Render(fmt.Sprintf("%d", p.sessionsToday)))
	b.WriteString("\n")

	b.WriteString("  " + statLabel.Render("Words this session: ") +
		statValue.Render(fmt.Sprintf("%d", p.wordsWritten)))
	b.WriteString("\n")

	totalWords := 0
	for _, s := range p.sessionLog {
		totalWords += s.Words
	}
	totalWords += p.wordsWritten
	b.WriteString("  " + statLabel.Render("Total words today: ") +
		statValue.Render(fmt.Sprintf("%d", totalWords)))
	b.WriteString("\n")

	b.WriteString("  " + statLabel.Render("Notes edited: ") +
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

			b.WriteString("  " +
				lipgloss.NewStyle().Foreground(overlay0).Render(timeStr) + "  " +
				lipgloss.NewStyle().Foreground(text).Render(durStr) + "  " +
				lipgloss.NewStyle().Foreground(green).Render(wordStr) + "  " +
				lipgloss.NewStyle().Foreground(blue).Render(noteStr))
			b.WriteString("\n")
		}
	}

	// Controls
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")
	controlStyle := lipgloss.NewStyle().Foreground(overlay1)
	b.WriteString(controlStyle.Render("  Space: start/pause  s: skip  r: reset  Esc: minimize"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
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

// finishWorkSession records the completed work session in the log.
func (p *Pomodoro) finishWorkSession() {
	p.sessionsToday++
	p.totalSessions++
	session := pomodoroSession{
		Start:    p.startTime,
		Duration: p.workDuration,
		Words:    p.wordsWritten,
		Notes:    len(p.notesEdited),
	}
	p.sessionLog = append(p.sessionLog, session)
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
