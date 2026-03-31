package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	acMaxVisible = 6
	acMaxWidth   = 40
)

// Autocomplete provides wikilink suggestions when the user types [[ in the editor.
type Autocomplete struct {
	active      bool
	suggestions []string
	cursor      int
	query       string
	allNotes    []string // all note paths in vault
	x, y        int      // screen position for popup
}

// NewAutocomplete returns a zero-value Autocomplete ready for use.
func NewAutocomplete() Autocomplete {
	return Autocomplete{}
}

// SetNotes sets the full list of available note paths used for suggestions.
func (a *Autocomplete) SetNotes(paths []string) {
	a.allNotes = paths
}

// IsActive reports whether the autocomplete popup is currently open.
func (a *Autocomplete) IsActive() bool {
	return a.active
}

// Open activates the autocomplete popup with the given initial query and
// screen position.
func (a *Autocomplete) Open(query string, x, y int) {
	a.active = true
	a.x = x
	a.y = y
	a.cursor = 0
	a.UpdateQuery(query)
}

// Close deactivates the autocomplete popup and resets its state.
func (a *Autocomplete) Close() {
	a.active = false
	a.suggestions = nil
	a.cursor = 0
	a.query = ""
}

// UpdateQuery filters the note list by fuzzy-matching query against note
// names (without .md extension). It reuses the same fuzzy matching logic as
// the sidebar.
func (a *Autocomplete) UpdateQuery(query string) {
	a.query = query
	a.suggestions = nil
	q := strings.ToLower(query)
	for _, path := range a.allNotes {
		name := strings.TrimSuffix(filepath.Base(path), ".md")
		if q == "" || fuzzyMatch(strings.ToLower(name), q) {
			a.suggestions = append(a.suggestions, name)
		}
	}
	if a.cursor >= len(a.suggestions) {
		a.cursor = maxInt(0, len(a.suggestions)-1)
	}
}

// MoveUp moves the selection cursor up by one entry.
func (a *Autocomplete) MoveUp() {
	if a.cursor > 0 {
		a.cursor--
	}
}

// MoveDown moves the selection cursor down by one entry.
func (a *Autocomplete) MoveDown() {
	if a.cursor < len(a.suggestions)-1 {
		a.cursor++
	}
}

// Selected returns the currently highlighted note name (without .md), or an
// empty string if there are no suggestions.
func (a *Autocomplete) Selected() string {
	if len(a.suggestions) == 0 {
		return ""
	}
	return a.suggestions[a.cursor]
}

// View renders the autocomplete popup as a compact bordered box.
func (a Autocomplete) View() string {
	if !a.active || len(a.suggestions) == 0 {
		return ""
	}

	// Determine visible window around the cursor.
	visible := len(a.suggestions)
	if visible > acMaxVisible {
		visible = acMaxVisible
	}
	start := 0
	if a.cursor >= visible {
		start = a.cursor - visible + 1
	}
	end := start + visible
	if end > len(a.suggestions) {
		end = len(a.suggestions)
		start = end - visible
		if start < 0 {
			start = 0
		}
	}

	// Compute the inner width: longest visible name, capped at acMaxWidth - 2
	// (accounting for 1-char padding on each side of border).
	innerWidth := 10
	for i := start; i < end; i++ {
		if len(a.suggestions[i]) > innerWidth {
			innerWidth = len(a.suggestions[i])
		}
	}
	maxInner := acMaxWidth - 2
	if innerWidth > maxInner {
		innerWidth = maxInner
	}

	selectedStyle := lipgloss.NewStyle().
		Background(surface0).
		Foreground(peach).
		Bold(true).
		Width(innerWidth)

	normalStyle := lipgloss.NewStyle().
		Foreground(text).
		Width(innerWidth)

	var rows []string

	// Header showing query context.
	headerStyle := lipgloss.NewStyle().
		Foreground(blue).
		Bold(true).
		Width(innerWidth)
	header := "Link to..."
	if a.query != "" {
		header = " " + a.query
	}
	if len(header) > innerWidth {
		header = header[:innerWidth]
	}
	rows = append(rows, headerStyle.Render(header))

	divider := lipgloss.NewStyle().
		Foreground(overlay0).
		Render(strings.Repeat("─", innerWidth))
	rows = append(rows, divider)

	for i := start; i < end; i++ {
		name := TruncateDisplay(a.suggestions[i], innerWidth)
		if i == a.cursor {
			rows = append(rows, selectedStyle.Render(name))
		} else {
			rows = append(rows, normalStyle.Render(name))
		}
	}

	// Show scroll hint when there are more entries than visible.
	if len(a.suggestions) > acMaxVisible {
		hint := lipgloss.NewStyle().
			Foreground(overlay0).
			Width(innerWidth).
			Render(strings.Repeat(" ", innerWidth-6) + "↕ more")
		rows = append(rows, hint)
	}

	content := strings.Join(rows, "\n")

	popup := lipgloss.NewStyle().
		Border(PanelBorder).
		BorderForeground(overlay0).
		Background(mantle).
		Padding(0, 1).
		Render(content)

	return popup
}
