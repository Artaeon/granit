package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// RenderFootnoteMarker returns a styled superscript-like rendering for a
// footnote marker. Used by the editor's inline highlighter to colour
// [^id] references.
func RenderFootnoteMarker(id string) string {
	style := lipgloss.NewStyle().
		Foreground(peach).
		Bold(true)
	return style.Render("[^" + id + "]")
}
