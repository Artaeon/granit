package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// NotePreview is a floating overlay that shows a read-only preview of a note.
// The user can scroll through the first few lines of content and optionally
// press Enter to open the note in the editor.
type NotePreview struct {
	active       bool
	width        int
	height       int
	title        string
	content      string
	notePath     string
	scroll       int
	lines        []string
	selectedNote string // consumed-once: set when user presses Enter
}

const (
	notePreviewMaxLines  = 30
	notePreviewMaxWidth  = 60
	notePreviewMaxHeight = 20
)

// NewNotePreview returns a zero-value NotePreview ready for use.
func NewNotePreview() NotePreview {
	return NotePreview{}
}

// SetSize updates the available screen dimensions so the overlay can
// adapt its panel size.
func (np *NotePreview) SetSize(w, h int) {
	np.width = w
	np.height = h
}

// Open populates the preview with a note's path, title, and content.
// Only the first notePreviewMaxLines lines of content are kept.
func (np *NotePreview) Open(notePath, title, content string) {
	np.active = true
	np.notePath = notePath
	np.title = title
	np.content = content
	np.scroll = 0
	np.selectedNote = ""

	allLines := strings.Split(content, "\n")
	if len(allLines) > notePreviewMaxLines {
		allLines = allLines[:notePreviewMaxLines]
	}
	np.lines = allLines
}

// Close hides the preview overlay.
func (np *NotePreview) Close() {
	np.active = false
}

// IsActive reports whether the preview overlay is currently visible.
func (np NotePreview) IsActive() bool {
	return np.active
}

// GetSelectedNote returns the note path the user chose to open (by
// pressing Enter) and clears it so it is only consumed once. The bool
// indicates whether a selection was made.
func (np *NotePreview) GetSelectedNote() (string, bool) {
	if np.selectedNote == "" {
		return "", false
	}
	path := np.selectedNote
	np.selectedNote = ""
	return path, true
}

// Update handles keyboard input while the preview is active.
func (np NotePreview) Update(msg tea.Msg) (NotePreview, tea.Cmd) {
	if !np.active {
		return np, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			np.active = false
		case "up", "k":
			if np.scroll > 0 {
				np.scroll--
			}
		case "down", "j":
			maxScroll := np.maxScroll()
			if np.scroll < maxScroll {
				np.scroll++
			}
		case "enter":
			if np.notePath != "" {
				np.selectedNote = np.notePath
				np.active = false
			}
		}
	}

	return np, nil
}

// View renders the preview overlay as a bordered floating panel.
func (np NotePreview) View() string {
	panelWidth := np.panelWidth()
	innerWidth := panelWidth - 6 // border (2) + padding (2*2)

	var b strings.Builder

	// ── Title ──────────────────────────────────────────────
	displayTitle := np.title
	if displayTitle == "" {
		displayTitle = "Untitled"
	}
	displayTitle = TruncateDisplay(displayTitle, innerWidth-2)
	titleRendered := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + displayTitle)
	b.WriteString(titleRendered)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	// ── Content lines ─────────────────────────────────────
	visH := np.visibleHeight()

	if len(np.lines) == 0 {
		b.WriteString(DimStyle.Render("  (empty note)"))
		b.WriteString("\n")
	} else {
		end := np.scroll + visH
		if end > len(np.lines) {
			end = len(np.lines)
		}

		for i := np.scroll; i < end; i++ {
			rendered := np.renderLine(np.lines[i], innerWidth)
			b.WriteString(rendered)
			if i < end-1 {
				b.WriteString("\n")
			}
		}

		// Scroll indicator when content is truncated.
		total := len(np.lines)
		if total > visH {
			b.WriteString("\n")
			indicator := fmt.Sprintf("  lines %d-%d of %d", np.scroll+1, end, total)
			b.WriteString(DimStyle.Render(indicator))
		}
	}

	// ── Footer ────────────────────────────────────────────
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")
	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"j/k", "scroll"}, {"Enter", "open"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return border.Render(b.String())
}

// ── internal helpers ──────────────────────────────────────────────────

// panelWidth returns the overlay width clamped to sensible bounds.
func (np NotePreview) panelWidth() int {
	half := np.width / 2
	w := notePreviewMaxWidth
	if half < w {
		w = half
	}
	if w < 40 {
		w = 40
	}
	return w
}

// visibleHeight returns how many content lines fit in the overlay.
func (np NotePreview) visibleHeight() int {
	half := np.height / 2
	h := notePreviewMaxHeight
	if half < h {
		h = half
	}
	if h < 5 {
		h = 5
	}
	return h
}

// maxScroll returns the maximum scroll offset.
func (np NotePreview) maxScroll() int {
	max := len(np.lines) - np.visibleHeight()
	if max < 0 {
		return 0
	}
	return max
}

// renderLine applies basic formatting to a single line of markdown for
// the preview display. Headers are shown in bold+mauve, bullet points
// are preserved, and long lines are truncated.
func (np NotePreview) renderLine(line string, maxWidth int) string {
	trimmed := strings.TrimSpace(line)

	// Detect heading level.
	if level := headingLevel(trimmed); level > 0 && len(trimmed) > level && trimmed[level] == ' ' {
		text := strings.TrimSpace(trimmed[level+1:])
		prefix := strings.Repeat("#", level) + " "
		styled := lipgloss.NewStyle().
			Foreground(mauve).
			Bold(true).
			Render(prefix + truncateLine(text, maxWidth-len(prefix)-4))
		return "  " + styled
	}

	// Bullet / list items — preserve marker.
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
		strings.HasPrefix(trimmed, "+ ") {
		marker := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(string(trimmed[0]))
		rest := lipgloss.NewStyle().Foreground(text).Render(
			truncateLine(trimmed[1:], maxWidth-4))
		return "  " + marker + rest
	}

	// Numbered list items.
	if len(trimmed) > 0 && trimmed[0] >= '0' && trimmed[0] <= '9' {
		dotIdx := strings.Index(trimmed, ". ")
		if dotIdx > 0 && dotIdx <= 3 {
			num := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(trimmed[:dotIdx+1])
			rest := lipgloss.NewStyle().Foreground(text).Render(
				truncateLine(trimmed[dotIdx+1:], maxWidth-4))
			return "  " + num + rest
		}
	}

	// Checkbox items.
	if strings.HasPrefix(trimmed, "- [ ] ") {
		marker := lipgloss.NewStyle().Foreground(yellow).Render("- [ ]")
		rest := lipgloss.NewStyle().Foreground(text).Render(
			truncateLine(trimmed[5:], maxWidth-8))
		return "  " + marker + rest
	}
	if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
		marker := lipgloss.NewStyle().Foreground(green).Render("- [x]")
		rest := lipgloss.NewStyle().Foreground(text).Render(
			truncateLine(trimmed[5:], maxWidth-8))
		return "  " + marker + rest
	}

	// Blockquote.
	if strings.HasPrefix(trimmed, "> ") {
		styled := lipgloss.NewStyle().
			Foreground(overlay0).
			Italic(true).
			Render(truncateLine(trimmed, maxWidth-4))
		return "  " + styled
	}

	// Plain text — preserve leading whitespace for indentation.
	indent := leadingSpaces(line)
	if indent > 6 {
		indent = 6
	}
	display := strings.Repeat(" ", indent) + trimmed
	return "  " + lipgloss.NewStyle().Foreground(text).Render(
		truncateLine(display, maxWidth-2))
}

// leadingSpaces counts the number of leading space characters in s.
func leadingSpaces(s string) int {
	n := 0
	for _, ch := range s {
		if ch == ' ' {
			n++
		} else if ch == '\t' {
			n += 4
		} else {
			break
		}
	}
	return n
}
