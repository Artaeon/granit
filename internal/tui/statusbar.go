package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// statusToast represents a single queued status bar notification.
type statusToast struct {
	text     string
	priority ToastLevel
}

type StatusBar struct {
	vaultPath  string
	activeNote string
	noteCount  int
	mode       string
	width      int
	message    string // kept for backward compat (legacy single message)
	messages   []statusToast
	lineNum    int
	colNum     int
	wordCount  int
	ai AIConfig
	pomodoroStatus string // e.g. "🍅 12:34"
	clockInStatus  string // e.g. "⏱ 1:23:45 · Project"
	researchStatus string // e.g. "Researching: AI trends"
	dueTodayCount  int
	overdueCount   int
	inboxCount     int
	readingProgress int  // 0-100 percentage
	viewMode        bool // whether currently in view mode
}

func NewStatusBar() StatusBar {
	return StatusBar{
		mode: "NORMAL",
	}
}

func (sb *StatusBar) SetWidth(width int) {
	sb.width = width
}

func (sb *StatusBar) SetVaultPath(path string) {
	sb.vaultPath = path
}

func (sb *StatusBar) SetActiveNote(note string) {
	sb.activeNote = note
}

func (sb *StatusBar) SetNoteCount(count int) {
	sb.noteCount = count
}

func (sb *StatusBar) SetMode(mode string) {
	sb.mode = mode
}

func (sb *StatusBar) SetMessage(msg string) {
	sb.message = msg
	if msg == "" {
		// Clear all messages (backward compat with clearMessageMsg).
		sb.messages = nil
		return
	}
	sb.pushStatusToast(msg, ToastInfo)
}

func (sb *StatusBar) SetWarning(msg string) {
	sb.message = msg
	sb.pushStatusToast(msg, ToastWarning)
}

func (sb *StatusBar) SetError(msg string) {
	sb.message = msg
	sb.pushStatusToast(msg, ToastError)
}

// pushStatusToast adds a message to the queue. Duplicates are ignored.
func (sb *StatusBar) pushStatusToast(text string, priority ToastLevel) {
	for _, t := range sb.messages {
		if t.text == text && t.priority == priority {
			return
		}
	}
	sb.messages = append(sb.messages, statusToast{text: text, priority: priority})
}

// topStatusToast returns the highest-priority message in the queue.
// If multiple share the same priority, the most recent one wins.
func (sb *StatusBar) topStatusToast() (statusToast, bool) {
	if len(sb.messages) == 0 {
		// Fall back to legacy single message field.
		if sb.message != "" {
			return statusToast{text: sb.message, priority: ToastInfo}, true
		}
		return statusToast{}, false
	}
	best := sb.messages[len(sb.messages)-1]
	for _, t := range sb.messages {
		if t.priority > best.priority {
			best = t
		}
	}
	return best, true
}

func (sb *StatusBar) SetCursor(line, col int) {
	sb.lineNum = line
	sb.colNum = col
}

func (sb *StatusBar) SetWordCount(count int) {
	sb.wordCount = count
}

func (sb *StatusBar) SetAIStatus(provider, model string) {
	sb.ai.Provider = provider
	sb.ai.Model = model
}

func (sb *StatusBar) SetPomodoroStatus(status string) {
	sb.pomodoroStatus = status
}

func (sb *StatusBar) SetClockInStatus(status string) {
	sb.clockInStatus = status
}

func (sb *StatusBar) SetResearchStatus(status string) {
	sb.researchStatus = status
}

func (sb *StatusBar) SetDueTodayCount(count int) {
	sb.dueTodayCount = count
}

func (sb *StatusBar) SetOverdueCount(count int) {
	sb.overdueCount = count
}

func (sb *StatusBar) SetInboxCount(count int) {
	sb.inboxCount = count
}

func (sb *StatusBar) SetReadingProgress(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	sb.readingProgress = percent
}

func (sb *StatusBar) SetViewMode(active bool) {
	sb.viewMode = active
}

func (sb StatusBar) View() string {
	// Mode badge
	modeColor := mauve
	switch sb.mode {
	case "FILES":
		modeColor = green
	case "EDIT":
		modeColor = mauve
	case "VIEW":
		modeColor = green
	case "LINKS":
		modeColor = blue
	case "SEARCH":
		modeColor = yellow
	case "COMMAND":
		modeColor = peach
	case "VIM:NORMAL":
		modeColor = blue
	case "VIM:INSERT":
		modeColor = green
	case "VIM:VISUAL":
		modeColor = peach
	case "VIM:COMMAND":
		modeColor = yellow
	}

	if strings.HasPrefix(sb.mode, "VIM:") {
		// e.g. VIM:REPLACE or something unhandled
		if modeColor == mauve {
			modeColor = pink
		}
	}

	modeStyle := lipgloss.NewStyle().
		Background(modeColor).
		Foreground(crust).
		Bold(true).
		Padding(0, 1)

	mode := modeStyle.Render(" " + sb.mode + " ")

	// File section
	fileIcon := IconMd
	if strings.Contains(sb.activeNote, "/") {
		fileIcon = IconFolder
	}
	if isDaily(sb.activeNote) {
		fileIcon = IconDaily
	}
	fileSection := StatusFileStyle.Render(fileIcon + " " + sb.activeNote)

	// Cursor position
	cursorPos := ""
	if sb.mode == "EDIT" {
		cursorPos = StatusInfoStyle.Render(fmt.Sprintf("Ln %d, Col %d", sb.lineNum+1, sb.colNum+1))
	}

	// Reading progress bar (view mode only)
	readingBar := ""
	if sb.viewMode {
		barWidth := 10
		filled := barWidth * sb.readingProgress / 100
		empty := barWidth - filled
		barStr := strings.Repeat("\u2588", filled) + strings.Repeat("\u2591", empty)
		progressLabel := fmt.Sprintf(" %d%%", sb.readingProgress)

		barStyle := lipgloss.NewStyle().Foreground(mauve)
		labelStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		bgStyle := lipgloss.NewStyle().Background(surface0).Padding(0, 1)
		readingBar = bgStyle.Render(
			labelStyle.Render("Reading: ") + barStyle.Render(barStr) + labelStyle.Render(progressLabel),
		)
	}

	// AI indicator
	aiIndicator := ""
	switch sb.ai.Provider {
	case "ollama":
		aiIndicator = lipgloss.NewStyle().
			Background(green).Foreground(crust).Bold(true).Padding(0, 1).
			Render(IconBotChar + " " + sb.ai.Model)
	case "openai":
		aiIndicator = lipgloss.NewStyle().
			Background(blue).Foreground(crust).Bold(true).Padding(0, 1).
			Render(IconBotChar + " " + sb.ai.Model)
	}

	// Pomodoro indicator
	pomoIndicator := ""
	if sb.pomodoroStatus != "" {
		pomoIndicator = lipgloss.NewStyle().
			Background(peach).Foreground(crust).Bold(true).Padding(0, 1).
			Render(sb.pomodoroStatus)
	}

	// Clock-in indicator
	clockIndicator := ""
	if sb.clockInStatus != "" {
		clockIndicator = lipgloss.NewStyle().
			Background(teal).Foreground(crust).Bold(true).Padding(0, 1).
			Render(sb.clockInStatus)
	}

	// Research indicator
	researchIndicator := ""
	if sb.researchStatus != "" {
		researchIndicator = lipgloss.NewStyle().
			Background(lavender).Foreground(crust).Bold(true).Padding(0, 1).
			Render(sb.researchStatus)
	}

	// Overdue counter
	overdueIndicator := ""
	if sb.overdueCount > 0 {
		overdueIndicator = lipgloss.NewStyle().
			Background(red).Foreground(crust).Bold(true).Padding(0, 1).
			Render(fmt.Sprintf("%d overdue", sb.overdueCount))
	}

	// Task counter
	taskIndicator := ""
	if sb.dueTodayCount > 0 {
		taskIndicator = lipgloss.NewStyle().
			Background(yellow).Foreground(crust).Bold(true).Padding(0, 1).
			Render(fmt.Sprintf("%d due", sb.dueTodayCount))
	}

	inboxIndicator := ""
	if sb.inboxCount > 0 {
		inboxIndicator = lipgloss.NewStyle().
			Background(sapphire).Foreground(crust).Bold(true).Padding(0, 1).
			Render(fmt.Sprintf("%d inbox", sb.inboxCount))
	}

	// Right side info
	wordInfo := ""
	if sb.wordCount > 0 {
		wordInfo = fmt.Sprintf("%d words  ", sb.wordCount)
	}
	rightInfo := StatusInfoStyle.Render(fmt.Sprintf("%s%d notes  %s", wordInfo, sb.noteCount, sb.vaultPath))

	// Truncate to prevent overflow on narrow terminals
	totalUsed := func() int {
		return lipgloss.Width(mode) + lipgloss.Width(fileSection) + lipgloss.Width(cursorPos) +
			lipgloss.Width(readingBar) + lipgloss.Width(researchIndicator) +
			lipgloss.Width(taskIndicator) + lipgloss.Width(clockIndicator) +
			lipgloss.Width(pomoIndicator) + lipgloss.Width(aiIndicator) + lipgloss.Width(rightInfo)
	}

	// Step 1: If too wide, hide least important indicators (reading progress, AI badge)
	if totalUsed() > sb.width {
		readingBar = ""
	}
	if totalUsed() > sb.width {
		aiIndicator = ""
	}

	// Step 2: If still too wide, truncate the file section with "..."
	if totalUsed() > sb.width {
		overhead := totalUsed() - sb.width
		plainFile := fileIcon + " " + sb.activeNote
		if lipgloss.Width(plainFile) > overhead+4 {
			// Truncate by runes to avoid cutting multi-byte characters
			runes := []rune(plainFile)
			cut := len(runes)
			for lipgloss.Width(string(runes[:cut])) > lipgloss.Width(plainFile)-overhead-3 && cut > 0 {
				cut--
			}
			plainFile = string(runes[:cut]) + "..."
		} else {
			plainFile = "..."
		}
		fileSection = StatusFileStyle.Render(plainFile)
	}

	// Step 3: If still too wide, drop the right info section
	if totalUsed() > sb.width {
		rightInfo = ""
	}

	// Calculate gap
	leftLen := lipgloss.Width(mode) + lipgloss.Width(fileSection) + lipgloss.Width(cursorPos) + lipgloss.Width(readingBar)
	rightLen := lipgloss.Width(researchIndicator) + lipgloss.Width(inboxIndicator) + lipgloss.Width(overdueIndicator) + lipgloss.Width(taskIndicator) + lipgloss.Width(clockIndicator) + lipgloss.Width(pomoIndicator) + lipgloss.Width(aiIndicator) + lipgloss.Width(rightInfo)
	gap := sb.width - leftLen - rightLen
	if gap < 0 {
		gap = 0
	}
	gapStr := ""
	if gap > 0 {
		gapStr = StatusBarBg.Width(gap).Render(strings.Repeat(" ", gap))
	}

	bar := mode + fileSection + cursorPos + readingBar + gapStr + researchIndicator + inboxIndicator + overdueIndicator + taskIndicator + clockIndicator + pomoIndicator + aiIndicator + rightInfo

	// Help bar
	helpItems := []struct{ key, desc string }{
		{"Tab", "panel"},
		{"Ctrl+P", "search"},
		{"Ctrl+N", "new"},
		{"Ctrl+S", "save"},
		{"Ctrl+K", "tasks"},
		{"Ctrl+X", "cmds"},
		{"Ctrl+Q", "quit"},
	}

	var helpParts []string
	for _, item := range helpItems {
		helpParts = append(helpParts,
			HelpKeyStyle.Render(item.key)+" "+HelpDescStyle.Render(item.desc))
	}
	helpBar := HelpBarStyle.Width(sb.width).Render(strings.Join(helpParts, "  "))

	if toast, ok := sb.topStatusToast(); ok {
		var msgStyle lipgloss.Style
		switch toast.priority {
		case ToastError:
			msgStyle = lipgloss.NewStyle().
				Background(red).
				Foreground(crust).
				Bold(true).
				Padding(0, 1).
				Width(sb.width)
		case ToastWarning:
			msgStyle = lipgloss.NewStyle().
				Background(yellow).
				Foreground(crust).
				Padding(0, 1).
				Width(sb.width)
		default:
			msgStyle = lipgloss.NewStyle().
				Background(surface0).
				Foreground(yellow).
				Padding(0, 1).
				Width(sb.width)
		}
		return bar + "\n" + msgStyle.Render(" "+toast.text) + "\n" + helpBar
	}

	return bar + "\n" + helpBar
}

func isDaily(name string) bool {
	// Simple check: YYYY-MM-DD pattern
	if len(name) < 10 {
		return false
	}
	base := name
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		base = name[idx+1:]
	}
	if len(base) >= 10 &&
		base[4] == '-' && base[7] == '-' &&
		base[0] >= '0' && base[0] <= '9' {
		return true
	}
	return false
}
