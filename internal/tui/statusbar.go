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
	pomodoroStatus     string // e.g. "🍅 12:34"
	focusSessionStatus string // e.g. "◉ 12:34"
	clockInStatus      string // e.g. "⏱ 1:23:45 · Project"
	researchStatus string // e.g. "Researching: AI trends"
	dueTodayCount  int
	overdueCount   int
	inboxCount     int
	readingProgress int  // 0-100 percentage
	viewMode        bool // whether currently in view mode
	dayPlanned      bool // true once morning routine or plan my day has been run
	gitStatus       string // e.g. "✓ synced", "● 3 changed", "⚠ no git"
	gitInitialized  bool
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

func (sb *StatusBar) SetFocusSessionStatus(status string) {
	sb.focusSessionStatus = status
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

func (sb *StatusBar) SetDayPlanned(planned bool) {
	sb.dayPlanned = planned
}

func (sb *StatusBar) SetGitStatus(status string) {
	sb.gitStatus = status
}

func (sb *StatusBar) SetGitInitialized(init bool) {
	sb.gitInitialized = init
}

func (sb StatusBar) View() string {
	// ── Mode badge ───────────────────────────────────────────────────
	modeColor := mauve
	switch sb.mode {
	case "FILES":
		modeColor = green
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
	if strings.HasPrefix(sb.mode, "VIM:") && modeColor == mauve {
		modeColor = pink
	}

	mode := lipgloss.NewStyle().
		Background(modeColor).Foreground(crust).Bold(true).Padding(0, 1).
		Render(sb.mode)

	// ── File section ─────────────────────────────────────────────────
	fileIcon := IconMd
	if strings.Contains(sb.activeNote, "/") {
		fileIcon = IconFolder
	}
	if isDaily(sb.activeNote) {
		fileIcon = IconDaily
	}
	fileSection := StatusFileStyle.Render(fileIcon + " " + sb.activeNote)

	// ── Cursor position (edit mode only) ─────────────────────────────
	cursorPos := ""
	if sb.mode == "EDIT" || strings.HasPrefix(sb.mode, "VIM:") {
		cursorPos = lipgloss.NewStyle().
			Background(mantle).Foreground(overlay0).Padding(0, 1).
			Render(fmt.Sprintf("%d:%d", sb.lineNum+1, sb.colNum+1))
	}

	// ── Word count ───────────────────────────────────────────────────
	wordInfo := ""
	if sb.wordCount > 0 {
		wordInfo = lipgloss.NewStyle().
			Background(mantle).Foreground(overlay0).Padding(0, 1).
			Render(fmt.Sprintf("%dw", sb.wordCount))
	}

	// ── Reading progress (view mode) ─────────────────────────────────
	readingBar := ""
	if sb.viewMode {
		barWidth := 8
		filled := barWidth * sb.readingProgress / 100
		empty := barWidth - filled
		barStr := strings.Repeat("━", filled) + strings.Repeat("─", empty)
		readingBar = lipgloss.NewStyle().Background(mantle).Padding(0, 1).Render(
			lipgloss.NewStyle().Foreground(mauve).Render(barStr) +
				lipgloss.NewStyle().Foreground(overlay0).Render(fmt.Sprintf(" %d%%", sb.readingProgress)),
		)
	}

	// ── Right-side indicators (subtle style for non-urgent) ──────────
	dimBadge := func(icon string, label string, fg lipgloss.Color) string {
		return lipgloss.NewStyle().
			Background(mantle).Foreground(fg).Padding(0, 1).
			Render(icon + " " + label)
	}
	alertBadge := func(label string, bg lipgloss.Color) string {
		return lipgloss.NewStyle().
			Background(bg).Foreground(crust).Bold(true).Padding(0, 1).
			Render(label)
	}

	// Active indicators (high priority — colored badges)
	overdueIndicator := ""
	if sb.overdueCount > 0 {
		overdueIndicator = alertBadge(fmt.Sprintf("%d overdue", sb.overdueCount), red)
	}

	taskIndicator := ""
	if sb.dueTodayCount > 0 {
		taskIndicator = alertBadge(fmt.Sprintf("%d due", sb.dueTodayCount), yellow)
	}

	// Pomodoro, focus session & clock-in (medium priority — colored badges)
	pomoIndicator := ""
	if sb.pomodoroStatus != "" {
		pomoIndicator = alertBadge(sb.pomodoroStatus, peach)
	}

	focusIndicator := ""
	if sb.focusSessionStatus != "" {
		focusIndicator = alertBadge(sb.focusSessionStatus, mauve)
	}

	clockIndicator := ""
	if sb.clockInStatus != "" {
		clockIndicator = alertBadge(sb.clockInStatus, teal)
	}

	// Research indicator
	researchIndicator := ""
	if sb.researchStatus != "" {
		researchIndicator = alertBadge(sb.researchStatus, lavender)
	}

	inboxIndicator := ""
	if sb.inboxCount > 0 {
		inboxIndicator = dimBadge("◆", fmt.Sprintf("%d inbox", sb.inboxCount), sapphire)
	}

	// Plan-your-day nudge (subtle, not a loud badge)
	planIndicator := ""
	if !sb.dayPlanned {
		planIndicator = dimBadge("◇", "plan day", mauve)
	}

	// Git (subtle)
	gitIndicator := ""
	if !sb.gitInitialized {
		gitIndicator = dimBadge("⚠", "no git", yellow)
	} else if sb.gitStatus != "" {
		icon := "✓"
		color := green
		if sb.gitStatus != "synced" {
			icon = "●"
			color = peach
		}
		gitIndicator = dimBadge(icon, sb.gitStatus, color)
	}

	// AI (subtle)
	aiIndicator := ""
	if sb.ai.Provider == "ollama" || sb.ai.Provider == "openai" {
		color := green
		if sb.ai.Provider == "openai" {
			color = blue
		}
		aiIndicator = dimBadge(IconBotChar, sb.ai.Model, color)
	}

	// ── Vault info ───────────────────────────────────────────────────
	vaultLabel := sb.vaultPath
	if sb.noteCount > 0 {
		vaultLabel = fmt.Sprintf("%d notes  %s", sb.noteCount, sb.vaultPath)
	}
	rightInfo := lipgloss.NewStyle().
		Background(mantle).Foreground(surface2).Padding(0, 1).
		Render(vaultLabel)

	// ── Overflow handling ────────────────────────────────────────────
	totalUsed := func() int {
		return lipgloss.Width(mode) + lipgloss.Width(fileSection) + lipgloss.Width(cursorPos) +
			lipgloss.Width(wordInfo) + lipgloss.Width(readingBar) +
			lipgloss.Width(researchIndicator) + lipgloss.Width(planIndicator) +
			lipgloss.Width(inboxIndicator) + lipgloss.Width(overdueIndicator) +
			lipgloss.Width(taskIndicator) + lipgloss.Width(clockIndicator) +
			lipgloss.Width(pomoIndicator) + lipgloss.Width(focusIndicator) +
			lipgloss.Width(gitIndicator) +
			lipgloss.Width(aiIndicator) + lipgloss.Width(rightInfo)
	}

	if totalUsed() > sb.width { planIndicator = "" }
	if totalUsed() > sb.width { readingBar = "" }
	if totalUsed() > sb.width { wordInfo = "" }
	if totalUsed() > sb.width { aiIndicator = "" }
	if totalUsed() > sb.width { gitIndicator = "" }
	if totalUsed() > sb.width { rightInfo = "" }

	if totalUsed() > sb.width {
		overhead := totalUsed() - sb.width
		plainFile := fileIcon + " " + sb.activeNote
		runes := []rune(plainFile)
		cut := len(runes)
		for cut > 0 && lipgloss.Width(string(runes[:cut])) > lipgloss.Width(plainFile)-overhead-3 {
			cut--
		}
		if cut > 0 {
			plainFile = string(runes[:cut]) + "…"
		} else {
			plainFile = "…"
		}
		fileSection = StatusFileStyle.Render(plainFile)
	}

	// ── Layout: left | gap | right ───────────────────────────────────
	leftLen := lipgloss.Width(mode) + lipgloss.Width(fileSection) + lipgloss.Width(cursorPos) + lipgloss.Width(wordInfo) + lipgloss.Width(readingBar)
	rightLen := lipgloss.Width(researchIndicator) + lipgloss.Width(planIndicator) +
		lipgloss.Width(inboxIndicator) + lipgloss.Width(overdueIndicator) +
		lipgloss.Width(taskIndicator) + lipgloss.Width(clockIndicator) +
		lipgloss.Width(pomoIndicator) + lipgloss.Width(focusIndicator) +
		lipgloss.Width(gitIndicator) +
		lipgloss.Width(aiIndicator) + lipgloss.Width(rightInfo)
	gap := sb.width - leftLen - rightLen
	if gap < 0 {
		gap = 0
	}
	gapStr := StatusBarBg.Width(gap).Render(strings.Repeat(" ", gap))

	// Separator line above status bar for visual distinction
	sepLine := lipgloss.NewStyle().Foreground(surface0).Width(sb.width).
		Render(strings.Repeat("─", sb.width))

	bar := sepLine + "\n" + mode + fileSection + cursorPos + wordInfo + readingBar + gapStr +
		researchIndicator + planIndicator + inboxIndicator +
		overdueIndicator + taskIndicator + clockIndicator + pomoIndicator + focusIndicator +
		gitIndicator + aiIndicator + rightInfo

	// ── Help bar ─────────────────────────────────────────────────────
	var helpItems []struct{ key, desc string }
	switch {
	case sb.mode == "FILES":
		helpItems = []struct{ key, desc string }{
			{"Enter", "open"}, {"n", "new"}, {"d", "delete"}, {"r", "rename"},
			{"/", "search"}, {"z/Z", "fold"}, {"Ctrl+P", "quick open"},
			{"Tab", "editor"}, {"Ctrl+Q", "quit"},
		}
	case sb.mode == "EDIT":
		helpItems = []struct{ key, desc string }{
			{"Ctrl+S", "save"}, {"Ctrl+W", "close tab"}, {"Ctrl+E", "view"},
			{"Ctrl+P", "quick open"}, {"Ctrl+K", "tasks"}, {"Ctrl+R", "AI bots"},
			{"Ctrl+X", "cmds"}, {"F5", "help"}, {"Ctrl+Q", "quit"},
		}
	case sb.mode == "VIEW":
		helpItems = []struct{ key, desc string }{
			{"j/k", "scroll"}, {"space", "page"}, {"Ctrl+E", "edit"},
			{"Ctrl+W", "close tab"}, {"Ctrl+P", "quick open"}, {"Ctrl+X", "cmds"},
			{"F5", "help"}, {"Ctrl+Q", "quit"},
		}
	case sb.mode == "LINKS":
		helpItems = []struct{ key, desc string }{
			{"Enter", "open"}, {"j/k", "nav"}, {"Tab", "editor"}, {"Ctrl+Q", "quit"},
		}
	case strings.HasPrefix(sb.mode, "VIM:"):
		helpItems = []struct{ key, desc string }{
			{":w", "save"}, {":q", "quit"}, {"Ctrl+E", "view"},
			{"Ctrl+W", "close tab"}, {"Ctrl+P", "quick open"}, {"Ctrl+K", "tasks"},
			{"Ctrl+R", "AI bots"}, {"Ctrl+X", "cmds"}, {"F5", "help"},
		}
	default:
		helpItems = []struct{ key, desc string }{
			{"Tab", "panel"}, {"Ctrl+P", "quick open"}, {"Ctrl+W", "close tab"},
			{"Ctrl+N", "new"}, {"Ctrl+S", "save"}, {"Ctrl+K", "tasks"},
			{"Ctrl+X", "cmds"}, {"F5", "help"}, {"Ctrl+Q", "quit"},
		}
	}

	var helpParts []string
	for _, item := range helpItems {
		helpParts = append(helpParts,
			HelpKeyStyle.Render(item.key)+HelpDescStyle.Render(" "+item.desc))
	}
	helpBar := HelpBarStyle.Width(sb.width).Render(strings.Join(helpParts, "   "))

	if toast, ok := sb.topStatusToast(); ok {
		var msgStyle lipgloss.Style
		switch toast.priority {
		case ToastError:
			msgStyle = lipgloss.NewStyle().
				Background(red).Foreground(crust).Bold(true).
				Padding(0, 1).Width(sb.width)
		case ToastWarning:
			msgStyle = lipgloss.NewStyle().
				Background(yellow).Foreground(crust).
				Padding(0, 1).Width(sb.width)
		default:
			msgStyle = lipgloss.NewStyle().
				Background(surface0).Foreground(text).
				Padding(0, 1).Width(sb.width)
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
