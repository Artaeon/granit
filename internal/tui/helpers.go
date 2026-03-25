package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// OverlayBorderColor is the default border color for overlay panels.
var OverlayBorderColor = lipgloss.Color("#6C7086") // overlay0

// TruncateDisplay truncates a string to maxWidth, adding ellipsis if needed.
func TruncateDisplay(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w <= maxWidth {
		return s
	}
	if maxWidth <= 1 {
		return "…"
	}
	// Trim rune-by-rune until it fits
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))> maxWidth-1 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

// RenderHelpBar renders a compact help bar from key/desc pairs.
func RenderHelpBar(bindings []struct{ Key, Desc string }) string {
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	keyStyle := lipgloss.NewStyle().Foreground(subtext0).Bold(true)

	var parts []string
	for _, b := range bindings {
		parts = append(parts, keyStyle.Render(b.Key)+" "+dimStyle.Render(b.Desc))
	}
	return dimStyle.Render(strings.Join(parts, "  "))
}
