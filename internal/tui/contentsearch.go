package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

// ContentSearchResult represents a single search hit within a note.
type ContentSearchResult struct {
	FilePath string
	Line     int    // 0-based line number
	Col      int    // column of match start
	Context  string // the matching line text
}

// ContentSearch is an overlay that performs full-text search across all notes
// in the vault, displaying results grouped by file with matching lines.
type ContentSearch struct {
	active   bool
	width    int
	height   int
	query    string
	results  []ContentSearchResult
	cursor   int
	scroll   int
	selected *ContentSearchResult // set when user presses Enter

	// Vault data
	noteContents map[string]string        // relPath -> content (fallback)
	searchIndex  *vault.SearchIndex       // inverted index for fast search
}

// NewContentSearch returns a zero-value ContentSearch ready for use.
func NewContentSearch() ContentSearch {
	return ContentSearch{}
}

// IsActive reports whether the overlay is visible.
func (cs *ContentSearch) IsActive() bool {
	return cs.active
}

// Open activates the overlay with the given vault contents and optional search index.
func (cs *ContentSearch) Open(noteContents map[string]string, si *vault.SearchIndex) {
	cs.active = true
	cs.query = ""
	cs.results = nil
	cs.cursor = 0
	cs.scroll = 0
	cs.selected = nil
	cs.noteContents = noteContents
	cs.searchIndex = si
}

// Close deactivates the overlay.
func (cs *ContentSearch) Close() {
	cs.active = false
	cs.query = ""
	cs.results = nil
	cs.selected = nil
	cs.noteContents = nil
	cs.searchIndex = nil
}

// SetSize updates the available dimensions for the overlay.
func (cs *ContentSearch) SetSize(w, h int) {
	cs.width = w
	cs.height = h
}

// SelectedResult returns the result the user chose (via Enter) and clears it.
// Returns nil if no selection was made.
func (cs *ContentSearch) SelectedResult() *ContentSearchResult {
	r := cs.selected
	cs.selected = nil
	return r
}

// Update handles key events for the overlay.
func (cs ContentSearch) Update(msg tea.Msg) (ContentSearch, tea.Cmd) {
	if !cs.active {
		return cs, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			cs.active = false
			return cs, nil

		case "enter":
			if len(cs.results) > 0 && cs.cursor < len(cs.results) {
				r := cs.results[cs.cursor]
				cs.selected = &r
				cs.active = false
			}
			return cs, nil

		case "up", "k":
			if cs.cursor > 0 {
				cs.cursor--
				if cs.cursor < cs.scroll {
					cs.scroll = cs.cursor
				}
			}
			return cs, nil

		case "down", "j":
			if cs.cursor < len(cs.results)-1 {
				cs.cursor++
				visH := cs.visibleHeight()
				if cs.cursor >= cs.scroll+visH {
					cs.scroll = cs.cursor - visH + 1
				}
			}
			return cs, nil

		case "backspace":
			if len(cs.query) > 0 {
				cs.query = cs.query[:len(cs.query)-1]
				cs.search()
			}
			return cs, nil

		default:
			ch := msg.String()
			if len(ch) == 1 && ch[0] >= 32 {
				cs.query += ch
				cs.search()
			}
			return cs, nil
		}
	}
	return cs, nil
}

// View renders the overlay panel.
func (cs ContentSearch) View() string {
	width := cs.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	innerWidth := width - 6

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSearchChar + " Search Vault Contents")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")

	// Search input
	prompt := SearchPromptStyle.Render("  > ")
	input := cs.query + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")

	// Results area
	if cs.query == "" {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Type to search across all notes..."))
		b.WriteString("\n")
	} else if len(cs.results) == 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  No matches found"))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		visH := cs.visibleHeight()
		end := cs.scroll + visH
		if end > len(cs.results) {
			end = len(cs.results)
		}

		prevFile := ""
		for i := cs.scroll; i < end; i++ {
			r := cs.results[i]

			// Show file header when the file changes
			if r.FilePath != prevFile {
				if prevFile != "" {
					b.WriteString("\n")
				}
				fileStyle := lipgloss.NewStyle().
					Foreground(blue).
					Bold(true)
				b.WriteString("  " + fileStyle.Render(r.FilePath))
				b.WriteString("\n")
				prevFile = r.FilePath
			}

			// Format the matching line with line number
			lineNum := DimStyle.Render(fmt.Sprintf("  %4d: ", r.Line+1))
			contextLine := cs.highlightMatch(r.Context, innerWidth-10)

			if i == cs.cursor {
				line := lineNum + contextLine
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(innerWidth).
					Render(line))
			} else {
				b.WriteString(lineNum + contextLine)
			}

			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")
	footer := "  Enter: jump to match  Esc: close"
	if len(cs.results) > 0 {
		footer += "  (" + smallNum(len(cs.results)) + " results)"
	}
	b.WriteString(DimStyle.Render(footer))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(yellow).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// search performs case-insensitive substring search across all note contents.
// When a search index is available and ready, it uses the inverted index for
// faster results with TF-IDF ranking. Otherwise, it falls back to linear scan.
func (cs *ContentSearch) search() {
	cs.results = nil
	cs.cursor = 0
	cs.scroll = 0

	if cs.query == "" {
		return
	}

	// Fast path: use the inverted search index if available
	if cs.searchIndex != nil && cs.searchIndex.IsReady() {
		cs.searchIndexed()
		return
	}

	// Fallback: linear scan
	cs.searchLinear()
}

// searchIndexed uses the vault's inverted index for fast full-text search.
func (cs *ContentSearch) searchIndexed() {
	indexed := cs.searchIndex.Search(cs.query)

	limit := 100
	if len(indexed) > limit {
		indexed = indexed[:limit]
	}

	cs.results = make([]ContentSearchResult, len(indexed))
	for i, r := range indexed {
		cs.results[i] = ContentSearchResult{
			FilePath: r.Path,
			Line:     r.Line,
			Col:      r.Column,
			Context:  r.MatchLine,
		}
	}
}

// searchLinear performs the original linear scan across all note contents.
func (cs *ContentSearch) searchLinear() {
	lowerQuery := strings.ToLower(cs.query)

	// Collect all file paths and sort them for deterministic ordering.
	paths := make([]string, 0, len(cs.noteContents))
	for p := range cs.noteContents {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// Two passes: first collect exact-case matches, then case-insensitive-only.
	var exact []ContentSearchResult
	var fuzzy []ContentSearchResult

	for _, path := range paths {
		content := cs.noteContents[path]
		lines := strings.Split(content, "\n")

		for lineIdx, line := range lines {
			lowerLine := strings.ToLower(line)
			col := strings.Index(lowerLine, lowerQuery)
			if col == -1 {
				continue
			}

			result := ContentSearchResult{
				FilePath: path,
				Line:     lineIdx,
				Col:      col,
				Context:  line,
			}

			// Check if there's an exact-case match too.
			if strings.Contains(line, cs.query) {
				exact = append(exact, result)
			} else {
				fuzzy = append(fuzzy, result)
			}

			// Enforce global limit across both buckets.
			if len(exact)+len(fuzzy) >= 100 {
				break
			}
		}
		if len(exact)+len(fuzzy) >= 100 {
			break
		}
	}

	cs.results = append(exact, fuzzy...)
}

// highlightMatch returns the context line with the query highlighted in yellow+bold.
func (cs ContentSearch) highlightMatch(line string, maxWidth int) string {
	if maxWidth < 10 {
		maxWidth = 10
	}

	// Truncate long lines before highlighting.
	display := line
	if len(display) > maxWidth {
		display = display[:maxWidth-3] + "..."
	}

	if cs.query == "" {
		return NormalItemStyle.Render(display)
	}

	lowerDisplay := strings.ToLower(display)
	lowerQuery := strings.ToLower(cs.query)

	idx := strings.Index(lowerDisplay, lowerQuery)
	if idx < 0 {
		return NormalItemStyle.Render(display)
	}

	before := display[:idx]
	match := display[idx : idx+len(cs.query)]
	after := display[idx+len(cs.query):]

	highlighted := MatchHighlightStyle.Render(match)

	return NormalItemStyle.Render(before) + highlighted + NormalItemStyle.Render(after)
}

// visibleHeight returns the number of result lines visible in the content area.
func (cs ContentSearch) visibleHeight() int {
	h := cs.height - 14
	if h < 5 {
		h = 5
	}
	return h
}
