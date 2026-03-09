package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type StatusBar struct {
	vaultPath  string
	activeNote string
	noteCount  int
	mode       string
	width      int
	message    string
	lineNum    int
	colNum     int
	wordCount  int
	aiProvider     string // "local", "ollama", "openai"
	aiModel        string
	pomodoroStatus string // e.g. "🍅 12:34"
	researchStatus string // e.g. "Researching: AI trends"
	dueTodayCount  int
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
}

func (sb *StatusBar) SetCursor(line, col int) {
	sb.lineNum = line
	sb.colNum = col
}

func (sb *StatusBar) SetWordCount(count int) {
	sb.wordCount = count
}

func (sb *StatusBar) SetAIStatus(provider, model string) {
	sb.aiProvider = provider
	sb.aiModel = model
}

func (sb *StatusBar) SetPomodoroStatus(status string) {
	sb.pomodoroStatus = status
}

func (sb *StatusBar) SetResearchStatus(status string) {
	sb.researchStatus = status
}

func (sb *StatusBar) SetDueTodayCount(count int) {
	sb.dueTodayCount = count
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
	modeColors := map[string]lipgloss.Color{
		"FILES":   green,
		"EDIT":    mauve,
		"VIEW":    green,
		"LINKS":   blue,
		"SEARCH":  yellow,
		"COMMAND": peach,
	}
	modeColor, ok := modeColors[sb.mode]
	if !ok {
		modeColor = mauve
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
	switch sb.aiProvider {
	case "ollama":
		aiIndicator = lipgloss.NewStyle().
			Background(green).Foreground(crust).Bold(true).Padding(0, 1).
			Render(IconBotChar + " " + sb.aiModel)
	case "openai":
		aiIndicator = lipgloss.NewStyle().
			Background(blue).Foreground(crust).Bold(true).Padding(0, 1).
			Render(IconBotChar + " " + sb.aiModel)
	}

	// Pomodoro indicator
	pomoIndicator := ""
	if sb.pomodoroStatus != "" {
		pomoIndicator = lipgloss.NewStyle().
			Background(peach).Foreground(crust).Bold(true).Padding(0, 1).
			Render(sb.pomodoroStatus)
	}

	// Research indicator
	researchIndicator := ""
	if sb.researchStatus != "" {
		researchIndicator = lipgloss.NewStyle().
			Background(lavender).Foreground(crust).Bold(true).Padding(0, 1).
			Render(sb.researchStatus)
	}

	// Task counter
	taskIndicator := ""
	if sb.dueTodayCount > 0 {
		taskIndicator = lipgloss.NewStyle().
			Background(yellow).Foreground(crust).Bold(true).Padding(0, 1).
			Render(fmt.Sprintf("%d due", sb.dueTodayCount))
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
			lipgloss.Width(taskIndicator) + lipgloss.Width(pomoIndicator) +
			lipgloss.Width(aiIndicator) + lipgloss.Width(rightInfo)
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
		if len(plainFile) > overhead+4 {
			plainFile = plainFile[:len(plainFile)-overhead-3] + "..."
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
	rightLen := lipgloss.Width(researchIndicator) + lipgloss.Width(taskIndicator) + lipgloss.Width(pomoIndicator) + lipgloss.Width(aiIndicator) + lipgloss.Width(rightInfo)
	gap := sb.width - leftLen - rightLen
	if gap < 0 {
		gap = 0
	}
	gapStr := ""
	if gap > 0 {
		gapStr = StatusBarBg.Width(gap).Render(strings.Repeat(" ", gap))
	}

	bar := mode + fileSection + cursorPos + readingBar + gapStr + researchIndicator + taskIndicator + pomoIndicator + aiIndicator + rightInfo

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

	if sb.message != "" {
		msgStyle := lipgloss.NewStyle().
			Background(surface0).
			Foreground(yellow).
			Padding(0, 1).
			Width(sb.width)
		return bar + "\n" + msgStyle.Render(" " + sb.message) + "\n" + helpBar
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
