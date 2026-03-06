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
	aiProvider    string // "local", "ollama", "openai"
	aiModel       string
	pomodoroStatus string // e.g. "🍅 12:34"
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

	// Right side info
	wordInfo := ""
	if sb.wordCount > 0 {
		wordInfo = fmt.Sprintf("%d words  ", sb.wordCount)
	}
	rightInfo := StatusInfoStyle.Render(fmt.Sprintf("%s%d notes  %s", wordInfo, sb.noteCount, sb.vaultPath))

	// Calculate gap
	leftLen := lipgloss.Width(mode) + lipgloss.Width(fileSection) + lipgloss.Width(cursorPos)
	rightLen := lipgloss.Width(pomoIndicator) + lipgloss.Width(aiIndicator) + lipgloss.Width(rightInfo)
	gap := sb.width - leftLen - rightLen
	if gap < 0 {
		gap = 1
	}
	gapStr := StatusBarBg.Width(gap).Render(strings.Repeat(" ", gap))

	bar := mode + fileSection + cursorPos + gapStr + pomoIndicator + aiIndicator + rightInfo

	// Help bar
	helpItems := []struct{ key, desc string }{
		{"Tab", "panel"},
		{"Ctrl+P", "search"},
		{"Ctrl+N", "new"},
		{"Ctrl+S", "save"},
		{"Ctrl+F", "find"},
		{"Ctrl+X", "cmds"},
		{"F5", "help"},
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
