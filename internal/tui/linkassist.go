package tui

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LinkAssistItem represents a single unlinked mention found in the note.
type LinkAssistItem struct {
	NoteName  string // the note title being linked
	MatchText string // the exact text found in content
	LineNum   int    // line number where found
	ColStart  int    // column where match starts
	ColEnd    int    // column where match ends
	Accepted  bool   // user toggled this on
}

// LinkAssist is an overlay that suggests wikilinks to insert while editing.
// It analyzes the current note's text and finds mentions of other note titles
// that aren't already linked.
type LinkAssist struct {
	active      bool
	suggestions []LinkAssistItem
	cursor      int
	scroll      int
	width       int
	height      int
	applyResult []LinkAssistItem
	hasResult   bool
}

// NewLinkAssist creates a new LinkAssist overlay.
func NewLinkAssist() LinkAssist {
	return LinkAssist{}
}

// IsActive returns whether the link assist overlay is currently visible.
func (la *LinkAssist) IsActive() bool {
	return la.active
}

// SetSize updates the available dimensions for the overlay.
func (la *LinkAssist) SetSize(width, height int) {
	la.width = width
	la.height = height
}

// Open analyzes the content for unlinked mentions of other notes and populates
// the suggestion list.
func (la *LinkAssist) Open(content string, notePaths []string, currentNote string) {
	la.active = true
	la.cursor = 0
	la.scroll = 0
	la.suggestions = nil
	la.applyResult = nil
	la.hasResult = false

	// Extract current note basename (without .md) for exclusion.
	currentName := strings.TrimSuffix(filepath.Base(currentNote), ".md")

	// Build list of candidate note names from paths.
	type candidate struct {
		name string
	}
	var candidates []candidate
	seen := make(map[string]bool)
	for _, p := range notePaths {
		name := strings.TrimSuffix(filepath.Base(p), ".md")
		// Skip short names (< 3 chars) to avoid false positives.
		if len(name) < 3 {
			continue
		}
		// Skip the current note's own name.
		if strings.EqualFold(name, currentName) {
			continue
		}
		lower := strings.ToLower(name)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		candidates = append(candidates, candidate{name: name})
	}

	lines := strings.Split(content, "\n")

	// Build a set of lines that are inside code blocks.
	inCodeBlock := make([]bool, len(lines))
	fenced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			fenced = !fenced
			inCodeBlock[i] = true
			continue
		}
		inCodeBlock[i] = fenced
	}

	// Precompile a regex to detect if a match position is inside [[...]].
	// We'll check per-match below.
	wikilinkRe := regexp.MustCompile(`\[\[[^\]]*\]\]`)

	for _, c := range candidates {
		nameLower := strings.ToLower(c.name)

		for lineIdx, line := range lines {
			// Skip lines inside code blocks.
			if inCodeBlock[lineIdx] {
				continue
			}

			lineLower := strings.ToLower(line)
			searchFrom := 0

			for searchFrom < len(lineLower) {
				idx := strings.Index(lineLower[searchFrom:], nameLower)
				if idx < 0 {
					break
				}

				colStart := searchFrom + idx
				colEnd := colStart + len(c.name)

				// Check word boundaries: the match should not be part of a
				// larger word. Allow match at start/end of line or when
				// adjacent to non-alphanumeric characters.
				validStart := colStart == 0 || !isAlphanumeric(line[colStart-1])
				validEnd := colEnd >= len(line) || !isAlphanumeric(line[colEnd])

				if validStart && validEnd {
					// Check if this match is already inside [[ ... ]].
					insideLink := false
					for _, loc := range wikilinkRe.FindAllStringIndex(line, -1) {
						if colStart >= loc[0] && colEnd <= loc[1] {
							insideLink = true
							break
						}
					}

					if !insideLink {
						matchText := line[colStart:colEnd]
						la.suggestions = append(la.suggestions, LinkAssistItem{
							NoteName:  c.name,
							MatchText: matchText,
							LineNum:   lineIdx,
							ColStart:  colStart,
							ColEnd:    colEnd,
							Accepted:  false,
						})
					}
				}

				searchFrom = colEnd
			}
		}
	}

	// Sort suggestions by line number, then column.
	sort.Slice(la.suggestions, func(i, j int) bool {
		if la.suggestions[i].LineNum != la.suggestions[j].LineNum {
			return la.suggestions[i].LineNum < la.suggestions[j].LineNum
		}
		return la.suggestions[i].ColStart < la.suggestions[j].ColStart
	})
}

// Close hides the link assist overlay without applying.
func (la *LinkAssist) Close() {
	la.active = false
}

// GetApplyResult returns the accepted suggestions (consumed-once pattern).
// Returns the list and true if the user confirmed, or nil and false otherwise.
func (la *LinkAssist) GetApplyResult() ([]LinkAssistItem, bool) {
	if la.hasResult {
		result := la.applyResult
		la.applyResult = nil
		la.hasResult = false
		return result, true
	}
	return nil, false
}

// Update handles keyboard input for the link assist overlay.
func (la LinkAssist) Update(msg tea.Msg) (LinkAssist, tea.Cmd) {
	if !la.active {
		return la, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			la.active = false

		case "up", "k":
			if la.cursor > 0 {
				la.cursor--
				if la.cursor < la.scroll {
					la.scroll = la.cursor
				}
			}

		case "down", "j":
			if la.cursor < len(la.suggestions)-1 {
				la.cursor++
				visH := la.visibleHeight()
				if la.cursor >= la.scroll+visH {
					la.scroll = la.cursor - visH + 1
				}
			}

		case " ", "enter":
			if len(la.suggestions) > 0 && la.cursor < len(la.suggestions) {
				la.suggestions[la.cursor].Accepted = !la.suggestions[la.cursor].Accepted
			}

		case "a":
			// Apply all accepted suggestions.
			var accepted []LinkAssistItem
			for _, s := range la.suggestions {
				if s.Accepted {
					accepted = append(accepted, s)
				}
			}
			if len(accepted) > 0 {
				la.applyResult = accepted
				la.hasResult = true
			}
			la.active = false
		}
	}
	return la, nil
}

// visibleHeight returns how many suggestion rows fit in the overlay.
func (la *LinkAssist) visibleHeight() int {
	h := la.height - 10
	if h < 1 {
		h = 1
	}
	return h
}

// View renders the link assist overlay.
func (la LinkAssist) View() string {
	width := la.width / 2
	if width < 55 {
		width = 55
	}
	if width > 80 {
		width = 80
	}

	var b strings.Builder

	// Title with suggestion count.
	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  Link Assistant")
	count := lipgloss.NewStyle().
		Foreground(overlay0).
		Render(fmt.Sprintf(" (%d suggestions)", len(la.suggestions)))
	b.WriteString(title + count)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n\n")

	if len(la.suggestions) == 0 {
		b.WriteString(DimStyle.Render("  No unlinked mentions found"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  All note references are already linked"))
	} else {
		visH := la.visibleHeight()
		if visH < 5 {
			visH = 5
		}
		end := la.scroll + visH
		if end > len(la.suggestions) {
			end = len(la.suggestions)
		}

		noteNameStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
		lineNumStyle := lipgloss.NewStyle().Foreground(overlay0)
		arrowStyle := lipgloss.NewStyle().Foreground(green)
		linkStyle := lipgloss.NewStyle().Foreground(lavender)
		checkOnStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		checkOffStyle := lipgloss.NewStyle().Foreground(surface1)

		for i := la.scroll; i < end; i++ {
			s := la.suggestions[i]

			// Checkbox.
			var checkbox string
			if s.Accepted {
				checkbox = checkOnStyle.Render("[x]")
			} else {
				checkbox = checkOffStyle.Render("[ ]")
			}

			// Note name and link preview.
			nameStr := noteNameStyle.Render(s.NoteName)
			lineInfo := lineNumStyle.Render(fmt.Sprintf("L%d", s.LineNum+1))
			arrow := arrowStyle.Render(" → ")
			linkPreview := linkStyle.Render("[[" + s.NoteName + "]]")

			row := fmt.Sprintf("  %s %s %s %s%s%s", checkbox, lineInfo, nameStr, arrow, linkPreview, "")

			// Clamp display width.
			if i == la.cursor {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(row))
			} else {
				b.WriteString(row)
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}

		// Show accepted count.
		acceptedCount := 0
		for _, s := range la.suggestions {
			if s.Accepted {
				acceptedCount++
			}
		}
		if acceptedCount > 0 {
			b.WriteString("\n")
			b.WriteString(lipgloss.NewStyle().
				Foreground(green).
				Render(fmt.Sprintf("  %d selected", acceptedCount)))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"Space/Enter", "toggle"}, {"a", "apply"}, {"Esc", "cancel"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// applyLinkSuggestions takes note content and a list of accepted suggestions,
// and wraps each matched mention with [[ ]]. Suggestions are applied from last
// to first (by position) so that earlier column offsets remain valid.
func applyLinkSuggestions(content string, suggestions []LinkAssistItem) string {
	if len(suggestions) == 0 {
		return content
	}

	// Sort in reverse order (last line first, last column first) so that
	// insertions don't shift positions of earlier suggestions.
	sorted := make([]LinkAssistItem, len(suggestions))
	copy(sorted, suggestions)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].LineNum != sorted[j].LineNum {
			return sorted[i].LineNum > sorted[j].LineNum
		}
		return sorted[i].ColStart > sorted[j].ColStart
	})

	lines := strings.Split(content, "\n")
	for _, s := range sorted {
		if s.LineNum >= len(lines) {
			continue
		}
		line := lines[s.LineNum]
		if s.ColEnd > len(line) {
			continue
		}
		lines[s.LineNum] = line[:s.ColStart] + "[[" + line[s.ColStart:s.ColEnd] + "]]" + line[s.ColEnd:]
	}
	return strings.Join(lines, "\n")
}

// isAlphanumeric returns true if a byte is a letter or digit.
func isAlphanumeric(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}
