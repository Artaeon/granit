package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// LinkCompleter – enhanced wikilink autocomplete popup
// ---------------------------------------------------------------------------

const (
	lcMaxVisible = 8
	lcMaxWidth   = 40
)

// LinkCompleter provides an inline autocomplete popup for wikilink insertion.
// When the user types [[ in the editor, the editor activates the completer.
// As the user types more characters the list is fuzzy-filtered.  Tab / Enter
// confirms the selection; Escape cancels.
type LinkCompleter struct {
	active bool

	// Note data
	allNotes     []string
	noteSnippets map[string]string // path -> first 80 chars of content

	// State
	query    string
	filtered []string
	cursor   int

	// Position in editor (for rendering)
	anchorLine int
	anchorCol  int

	// Result
	completed string // the completed link text (without [[ ]])
}

// NewLinkCompleter returns an initialised LinkCompleter ready for use.
func NewLinkCompleter() *LinkCompleter {
	return &LinkCompleter{
		noteSnippets: make(map[string]string),
	}
}

// SetNotes sets the full list of note paths and their preview snippets.
func (lc *LinkCompleter) SetNotes(paths []string, snippets map[string]string) {
	lc.allNotes = paths
	if snippets != nil {
		lc.noteSnippets = snippets
	} else {
		lc.noteSnippets = make(map[string]string)
	}
}

// IsActive reports whether the link completer popup is currently open.
func (lc *LinkCompleter) IsActive() bool {
	return lc.active
}

// Activate opens the popup at the given editor position.
func (lc *LinkCompleter) Activate(line, col int) {
	lc.active = true
	lc.anchorLine = line
	lc.anchorCol = col
	lc.query = ""
	lc.cursor = 0
	lc.completed = ""
	lc.refilter()
}

// Deactivate closes the popup and resets transient state.
func (lc *LinkCompleter) Deactivate() {
	lc.active = false
	lc.filtered = nil
	lc.cursor = 0
	lc.query = ""
	lc.completed = ""
}

// AddChar appends a character to the query and refilters the list.
func (lc *LinkCompleter) AddChar(ch string) {
	lc.query += ch
	lc.refilter()
}

// RemoveChar removes the last character from the query (backspace).
func (lc *LinkCompleter) RemoveChar() {
	if len(lc.query) > 0 {
		lc.query = lc.query[:len(lc.query)-1]
		lc.refilter()
	}
}

// MoveUp moves the selection cursor up by one entry.
func (lc *LinkCompleter) MoveUp() {
	if lc.cursor > 0 {
		lc.cursor--
	}
}

// MoveDown moves the selection cursor down by one entry.
func (lc *LinkCompleter) MoveDown() {
	if lc.cursor < len(lc.filtered)-1 {
		lc.cursor++
	}
}

// Confirm returns the selected note name and deactivates the completer.
// The caller should insert the returned string followed by ]] at the cursor.
func (lc *LinkCompleter) Confirm() string {
	if len(lc.filtered) == 0 {
		lc.Deactivate()
		return ""
	}
	result := lc.filtered[lc.cursor]
	lc.completed = result
	lc.Deactivate()
	return result
}

// GetQuery returns the current search query typed by the user.
func (lc *LinkCompleter) GetQuery() string {
	return lc.query
}

// ---------------------------------------------------------------------------
// Fuzzy matching (unique name to avoid collision with fuzzyMatch, cmdFuzzyMatch,
// twFuzzyMatch already defined in this package)
// ---------------------------------------------------------------------------

func linkFuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

// ---------------------------------------------------------------------------
// Filtering
// ---------------------------------------------------------------------------

// refilter rebuilds lc.filtered from allNotes using the current query.
// Results are sorted: exact prefix matches first, then fuzzy matches.
// The list is capped at lcMaxVisible entries.
func (lc *LinkCompleter) refilter() {
	lc.filtered = nil
	q := strings.ToLower(lc.query)

	var prefixMatches []string
	var fuzzyMatches []string

	for _, path := range lc.allNotes {
		baseName := strings.TrimSuffix(filepath.Base(path), ".md")
		lowerBase := strings.ToLower(baseName)
		lowerPath := strings.ToLower(path)

		if q == "" {
			// Empty query: everything is a prefix match.
			prefixMatches = append(prefixMatches, baseName)
			continue
		}

		isPrefix := strings.HasPrefix(lowerBase, q)
		isFuzzyBase := linkFuzzyMatch(lowerBase, q)
		isFuzzyPath := linkFuzzyMatch(lowerPath, q)

		if isPrefix {
			prefixMatches = append(prefixMatches, baseName)
		} else if isFuzzyBase || isFuzzyPath {
			fuzzyMatches = append(fuzzyMatches, baseName)
		}
	}

	lc.filtered = append(lc.filtered, prefixMatches...)
	lc.filtered = append(lc.filtered, fuzzyMatches...)

	if len(lc.filtered) > lcMaxVisible {
		lc.filtered = lc.filtered[:lcMaxVisible]
	}

	if lc.cursor >= len(lc.filtered) {
		lc.cursor = maxInt(0, len(lc.filtered)-1)
	}
}

// snippetForName returns the preview snippet for a note by matching its
// basename against the snippets map (which is keyed by full path).
func (lc *LinkCompleter) snippetForName(name string) string {
	// Try to find the path whose Base (minus .md) matches name.
	for _, p := range lc.allNotes {
		baseName := strings.TrimSuffix(filepath.Base(p), ".md")
		if baseName == name {
			if s, ok := lc.noteSnippets[p]; ok {
				return s
			}
			return ""
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Render
// ---------------------------------------------------------------------------

// Render returns the popup as a self-contained styled string.  The caller may
// overlay it near the cursor position in the editor.  maxWidth and maxHeight
// allow the caller to constrain the popup to the available viewport.
func (lc *LinkCompleter) Render(maxWidth, maxHeight int) string {
	if !lc.active || len(lc.filtered) == 0 {
		return ""
	}

	// Determine how many items we can show.
	visibleItems := len(lc.filtered)
	if visibleItems > lcMaxVisible {
		visibleItems = lcMaxVisible
	}
	// Each item takes 2 rows (name + snippet), plus header (1) + query (1) +
	// divider (1) = 3 fixed rows, plus 2 for top/bottom border.
	maxItems := (maxHeight - 5) / 2
	if maxItems < 1 {
		maxItems = 1
	}
	if visibleItems > maxItems {
		visibleItems = maxItems
	}

	// Scroll window around cursor.
	start := 0
	if lc.cursor >= visibleItems {
		start = lc.cursor - visibleItems + 1
	}
	end := start + visibleItems
	if end > len(lc.filtered) {
		end = len(lc.filtered)
		start = end - visibleItems
		if start < 0 {
			start = 0
		}
	}

	// Compute inner width.
	innerWidth := 20
	for i := start; i < end; i++ {
		nameLen := len(lc.filtered[i]) + 4 // bullet + space + name + padding
		if nameLen > innerWidth {
			innerWidth = nameLen
		}
	}
	capWidth := lcMaxWidth - 4 // border (2) + padding (2)
	if maxWidth > 0 && maxWidth-4 < capWidth {
		capWidth = maxWidth - 4
	}
	if innerWidth > capWidth {
		innerWidth = capWidth
	}
	if innerWidth < 20 {
		innerWidth = 20
	}

	// Styles.
	titleStyle := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true)

	queryStyle := lipgloss.NewStyle().
		Foreground(text)

	dividerStyle := lipgloss.NewStyle().
		Foreground(overlay0)

	selNameStyle := lipgloss.NewStyle().
		Background(surface0).
		Foreground(peach).
		Bold(true)

	selSnippetStyle := lipgloss.NewStyle().
		Background(surface0).
		Foreground(overlay0)

	normNameStyle := lipgloss.NewStyle().
		Foreground(text)

	normSnippetStyle := lipgloss.NewStyle().
		Foreground(overlay0)

	var rows []string

	// Title row.
	title := titleStyle.Render("Link to...")
	titlePad := innerWidth - lipgloss.Width(title)
	if titlePad > 0 {
		title += strings.Repeat("─", titlePad)
	}
	rows = append(rows, title)

	// Query row with cursor indicator.
	queryText := TruncateDisplay(" > "+lc.query+"_", innerWidth)
	rows = append(rows, queryStyle.Render(lcPadRight(queryText, innerWidth)))

	// Divider.
	rows = append(rows, dividerStyle.Render(strings.Repeat("─", innerWidth)))

	// Result items.
	for i := start; i < end; i++ {
		name := lc.filtered[i]
		snippet := lc.snippetForName(name)

		// Truncate name if needed.
		displayName := TruncateDisplay(name, innerWidth-4)

		// Truncate snippet.
		snippet = TruncateDisplay(snippet, innerWidth-2)
		if snippet == "" {
			snippet = " "
		}

		if i == lc.cursor {
			marker := lipgloss.NewStyle().Foreground(peach).Background(surface0).Render("\u25cf") // ●
			nameRow := marker + " " + selNameStyle.Render(lcPadRight(displayName, innerWidth-2))
			snippetRow := selSnippetStyle.Render(lcPadRight("  "+snippet, innerWidth))
			rows = append(rows, nameRow, snippetRow)
		} else {
			marker := lipgloss.NewStyle().Foreground(surface1).Render("\u2022") // •
			nameRow := marker + " " + normNameStyle.Render(lcPadRight(displayName, innerWidth-2))
			snippetRow := normSnippetStyle.Render(lcPadRight("  "+snippet, innerWidth))
			rows = append(rows, nameRow, snippetRow)
		}
	}

	content := strings.Join(rows, "\n")

	popup := lipgloss.NewStyle().
		Border(PanelBorder).
		BorderForeground(surface1).
		Background(mantle).
		Padding(0, 1).
		Render(content)

	return popup
}

// lcPadRight pads s with spaces to reach the desired width, or truncates if
// longer.  Named uniquely to avoid collision with padRight in stats.go.
func lcPadRight(s string, width int) string {
	n := lipgloss.Width(s)
	if n >= width {
		// Truncate using rune-aware approach.
		runes := []rune(s)
		for lipgloss.Width(string(runes)) > width && len(runes) > 0 {
			runes = runes[:len(runes)-1]
		}
		return string(runes)
	}
	return s + strings.Repeat(" ", width-n)
}
