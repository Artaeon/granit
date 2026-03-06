package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	bpMaxLines  = 15
	bpMaxWidth  = 50
	bpMaxHeight = 18
)

// BacklinkPreview shows a floating popup with the content of a wikilinked
// note when the editor cursor rests on a [[wikilink]].
type BacklinkPreview struct {
	active       bool
	width        int
	height       int

	// Current preview
	linkTarget   string // the note name inside [[ ]]
	preview      string // rendered preview content (first ~15 lines)
	previewLines int    // number of lines in preview

	// Position hints
	cursorLine   int // editor line where the link is
	cursorCol    int // editor column

	// Scroll within preview
	scroll       int
}

// NewBacklinkPreview returns a zero-value BacklinkPreview ready for use.
func NewBacklinkPreview() BacklinkPreview {
	return BacklinkPreview{}
}

// IsActive reports whether the preview popup is currently visible.
func (bp *BacklinkPreview) IsActive() bool {
	return bp.active
}

// SetSize updates the available terminal dimensions for layout calculations.
func (bp *BacklinkPreview) SetSize(w, h int) {
	bp.width = w
	bp.height = h
}

// Update checks whether the cursor is on a wikilink and, if so, loads a
// preview of the target note. It should be called whenever the cursor moves.
//
// content is the current editor buffer (one entry per line).
// getNoteContent returns the raw file content for a note name, or "" if
// the note does not exist.
func (bp *BacklinkPreview) Update(content []string, cursorLine, cursorCol int, getNoteContent func(name string) string) {
	bp.cursorLine = cursorLine
	bp.cursorCol = cursorCol

	if cursorLine < 0 || cursorLine >= len(content) {
		bp.deactivate()
		return
	}

	line := content[cursorLine]
	target := extractWikilink(line, cursorCol)
	if target == "" {
		bp.deactivate()
		return
	}

	// Only reload when the target changes.
	if target == bp.linkTarget && bp.active {
		return
	}

	raw := getNoteContent(target)
	if raw == "" {
		bp.deactivate()
		return
	}

	bp.linkTarget = target
	bp.active = true
	bp.scroll = 0
	bp.buildPreview(raw)
}

// ScrollDown scrolls the preview content down by one line.
func (bp *BacklinkPreview) ScrollDown() {
	maxScroll := bp.previewLines - bpMaxLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if bp.scroll < maxScroll {
		bp.scroll++
	}
}

// ScrollUp scrolls the preview content up by one line.
func (bp *BacklinkPreview) ScrollUp() {
	if bp.scroll > 0 {
		bp.scroll--
	}
}

// Dismiss manually hides the preview popup.
func (bp *BacklinkPreview) Dismiss() {
	bp.deactivate()
}

func (bp *BacklinkPreview) deactivate() {
	bp.active = false
	bp.linkTarget = ""
	bp.preview = ""
	bp.previewLines = 0
	bp.scroll = 0
}

// buildPreview processes raw note content into rendered preview lines.
func (bp *BacklinkPreview) buildPreview(raw string) {
	lines := strings.Split(raw, "\n")

	// Strip YAML frontmatter.
	lines = stripFrontmatter(lines)

	bp.previewLines = len(lines)
	if bp.previewLines == 0 {
		bp.preview = DimStyle.Render("(empty note)")
		bp.previewLines = 1
		return
	}

	popupWidth := bp.popupWidth() - 4 // account for border + padding
	if popupWidth < 10 {
		popupWidth = 10
	}

	var rendered []string
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			styled := DimStyle.Render(truncateLine(line, popupWidth))
			rendered = append(rendered, styled)
			continue
		}

		if inCodeBlock {
			styled := DimStyle.Render(truncateLine(line, popupWidth))
			rendered = append(rendered, styled)
			continue
		}

		styled := bp.renderPreviewLine(trimmed, popupWidth)
		rendered = append(rendered, styled)
	}

	bp.previewLines = len(rendered)
	bp.preview = strings.Join(rendered, "\n")
}

// renderPreviewLine applies basic markdown highlighting to a single line.
func (bp *BacklinkPreview) renderPreviewLine(line string, maxWidth int) string {
	if line == "" {
		return ""
	}

	// Headers
	if strings.HasPrefix(line, "#") {
		headerStyle := lipgloss.NewStyle().
			Foreground(mauve).
			Bold(true)
		return headerStyle.Render(truncateLine(line, maxWidth))
	}

	// Bold text: render **word** with bold styling inline
	if strings.Contains(line, "**") {
		return bp.renderBoldLine(line, maxWidth)
	}

	// Regular text
	textStyle := lipgloss.NewStyle().Foreground(text)
	return textStyle.Render(truncateLine(line, maxWidth))
}

// renderBoldLine handles inline **bold** segments within a line.
func (bp *BacklinkPreview) renderBoldLine(line string, maxWidth int) string {
	line = truncateLine(line, maxWidth)
	normalStyle := lipgloss.NewStyle().Foreground(text)
	boldStyle := lipgloss.NewStyle().Foreground(text).Bold(true)

	var b strings.Builder
	remaining := line
	for {
		idx := strings.Index(remaining, "**")
		if idx == -1 {
			b.WriteString(normalStyle.Render(remaining))
			break
		}
		// Text before the bold marker
		if idx > 0 {
			b.WriteString(normalStyle.Render(remaining[:idx]))
		}
		remaining = remaining[idx+2:]
		// Find closing **
		closeIdx := strings.Index(remaining, "**")
		if closeIdx == -1 {
			// No closing marker, render rest normally
			b.WriteString(normalStyle.Render("**" + remaining))
			break
		}
		b.WriteString(boldStyle.Render(remaining[:closeIdx]))
		remaining = remaining[closeIdx+2:]
	}
	return b.String()
}

// View renders the preview popup as a bordered floating box.
func (bp BacklinkPreview) View() string {
	if !bp.active || bp.preview == "" {
		return ""
	}

	popupWidth := bp.popupWidth()

	// Title bar: link icon + note name
	titleStyle := lipgloss.NewStyle().
		Foreground(lavender).
		Bold(true)
	title := IconLinkChar + " " + bp.linkTarget
	innerWidth := popupWidth - 4 // border + padding
	if innerWidth < 10 {
		innerWidth = 10
	}
	if lipgloss.Width(title) > innerWidth {
		title = truncateLine(title, innerWidth)
	}
	titleRendered := titleStyle.Render(title)

	divider := lipgloss.NewStyle().
		Foreground(overlay0).
		Render(strings.Repeat("─", innerWidth))

	// Visible content window
	allLines := strings.Split(bp.preview, "\n")
	start := bp.scroll
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + bpMaxLines
	if end > len(allLines) {
		end = len(allLines)
	}
	visibleLines := allLines[start:end]

	var rows []string
	rows = append(rows, titleRendered)
	rows = append(rows, divider)
	rows = append(rows, visibleLines...)

	// Scroll / continuation indicator
	if end < len(allLines) {
		moreStyle := lipgloss.NewStyle().Foreground(overlay0)
		rows = append(rows, moreStyle.Render("..."))
	} else if start > 0 {
		topStyle := lipgloss.NewStyle().Foreground(overlay0)
		rows = append(rows, topStyle.Render("~ top"))
	}

	content := strings.Join(rows, "\n")

	popup := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lavender).
		Background(surface0).
		Padding(0, 1).
		MaxWidth(popupWidth).
		MaxHeight(bpMaxHeight).
		Render(content)

	return popup
}

// popupWidth returns the computed width for the preview popup,
// capped at bpMaxWidth or one-third of the available width.
func (bp BacklinkPreview) popupWidth() int {
	w := bp.width / 3
	if w > bpMaxWidth {
		w = bpMaxWidth
	}
	if w < 20 {
		w = 20
	}
	return w
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractWikilink returns the wikilink target if the column position col falls
// within a [[...]] span on the given line. For [[note|alias]] it returns just
// "note". Returns "" if the cursor is not on a wikilink.
func extractWikilink(line string, col int) string {
	if col < 0 || col > len(line) {
		return ""
	}

	// Search backwards from col for the nearest [[
	openIdx := -1
	for i := col; i >= 1; i-- {
		if line[i-1] == '[' && i >= 2 && line[i-2] == '[' {
			openIdx = i - 2
			break
		}
	}
	if openIdx == -1 {
		// Also check if col is right on the opening brackets
		if col+1 < len(line) && line[col] == '[' && line[col+1] == '[' {
			openIdx = col
		}
		if openIdx == -1 {
			return ""
		}
	}

	// Find the closing ]]
	closeIdx := strings.Index(line[openIdx+2:], "]]")
	if closeIdx == -1 {
		return ""
	}
	closeIdx += openIdx + 2 // absolute position of first ]

	// Check that col falls within the [[...]] span (inclusive of brackets)
	if col < openIdx || col > closeIdx+1 {
		return ""
	}

	inner := line[openIdx+2 : closeIdx]
	if inner == "" {
		return ""
	}

	// Handle [[note|alias]] — extract just the note part
	if pipeIdx := strings.Index(inner, "|"); pipeIdx >= 0 {
		inner = inner[:pipeIdx]
	}

	return strings.TrimSpace(inner)
}

// stripFrontmatter removes YAML frontmatter delimited by --- at the
// start of the document.
func stripFrontmatter(lines []string) []string {
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return lines
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			// Return everything after the closing ---
			rest := lines[i+1:]
			// Skip leading empty lines after frontmatter
			for len(rest) > 0 && strings.TrimSpace(rest[0]) == "" {
				rest = rest[1:]
			}
			return rest
		}
	}
	// No closing ---, return as-is (malformed frontmatter)
	return lines
}

// truncateLine shortens s to maxWidth runes, appending "..." if truncated.
func truncateLine(s string, maxWidth int) string {
	if maxWidth <= 3 {
		return "..."
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-3]) + "..."
}
