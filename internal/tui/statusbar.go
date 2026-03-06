package tui

import (
	"fmt"
	"strings"
)

type StatusBar struct {
	vaultPath  string
	activeNote string
	noteCount  int
	mode       string
	width      int
	message    string
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

func (sb StatusBar) View() string {
	left := fmt.Sprintf(" GRANIT | %s | %s", sb.mode, sb.activeNote)
	right := fmt.Sprintf("%d notes | %s ", sb.noteCount, sb.vaultPath)

	gap := sb.width - len(left) - len(right)
	if gap < 0 {
		gap = 1
	}

	bar := left + strings.Repeat(" ", gap) + right

	if sb.message != "" {
		msgLine := "\n " + sb.message
		return StatusBarStyle.Width(sb.width).Render(bar) + DimStyle.Render(msgLine)
	}

	return StatusBarStyle.Width(sb.width).Render(bar)
}
