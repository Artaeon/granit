package tui

import (
	"fmt"
	"regexp"
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

	// Regex search mode
	regexMode bool
	regexErr  string // non-empty when the regex pattern is invalid

	// Search history (Up/Down to recall previous queries)
	history    []string
	historyIdx int    // -1 means new query mode; 0..len-1 indexes from most recent
	savedQuery string // stash current typed query when browsing history

	// Vault data
	noteContents map[string]string  // relPath -> content (fallback)
	searchIndex  *vault.SearchIndex // inverted index for fast search
	vaultRoot    string
}

// NewContentSearch returns a zero-value ContentSearch ready for use.
func NewContentSearch() ContentSearch {
	return ContentSearch{historyIdx: -1}
}

// IsActive reports whether the overlay is visible.
func (cs *ContentSearch) IsActive() bool {
	return cs.active
}

// IsRegexMode reports whether regex search is enabled.
func (cs *ContentSearch) IsRegexMode() bool {
	return cs.regexMode
}

// ToggleRegex flips the regex mode and re-runs the search.
func (cs *ContentSearch) ToggleRegex() {
	cs.regexMode = !cs.regexMode
	cs.regexErr = ""
	cs.search()
}

// Open activates the overlay with the given vault contents and optional search index.
func (cs *ContentSearch) Open(noteContents map[string]string, si *vault.SearchIndex, vaultRoot string) {
	cs.active = true
	cs.query = ""
	cs.results = nil
	cs.cursor = 0
	cs.scroll = 0
	cs.selected = nil
	cs.noteContents = noteContents
	cs.searchIndex = si
	cs.vaultRoot = vaultRoot
	cs.historyIdx = -1
	cs.savedQuery = ""
	// Load history from disk
	h := loadSearchHistory(vaultRoot)
	cs.history = h.ContentSearch
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

		case "alt+r":
			cs.regexMode = !cs.regexMode
			cs.regexErr = ""
			cs.search()
			return cs, nil

		case "enter":
			if len(cs.results) > 0 && cs.cursor < len(cs.results) {
				r := cs.results[cs.cursor]
				cs.selected = &r
				cs.active = false
				// Save query to history
				if cs.query != "" {
					cs.history = appendToHistory(cs.history, cs.query)
					h := loadSearchHistory(cs.vaultRoot)
					h.ContentSearch = cs.history
					saveSearchHistory(cs.vaultRoot, h)
				}
			}
			return cs, nil

		case "up", "k":
			if len(cs.results) > 0 {
				// Navigate results
				if cs.cursor > 0 {
					cs.cursor--
					if cs.cursor < cs.scroll {
						cs.scroll = cs.cursor
					}
				}
			} else if len(cs.history) > 0 {
				// Browse history (most recent first)
				if cs.historyIdx == -1 {
					cs.savedQuery = cs.query
					cs.historyIdx = len(cs.history) - 1
				} else if cs.historyIdx > 0 {
					cs.historyIdx--
				}
				cs.query = cs.history[cs.historyIdx]
				cs.search()
			}
			return cs, nil

		case "down", "j":
			if len(cs.results) > 0 {
				// Navigate results
				if cs.cursor < len(cs.results)-1 {
					cs.cursor++
					visH := cs.visibleHeight()
					if cs.cursor >= cs.scroll+visH {
						cs.scroll = cs.cursor - visH + 1
					}
				}
			} else if cs.historyIdx >= 0 {
				// Browse history forward
				if cs.historyIdx < len(cs.history)-1 {
					cs.historyIdx++
					cs.query = cs.history[cs.historyIdx]
				} else {
					// Past the end — restore typed query
					cs.historyIdx = -1
					cs.query = cs.savedQuery
				}
				cs.search()
			}
			return cs, nil

		case "backspace":
			if len(cs.query) > 0 {
				cs.query = cs.query[:len(cs.query)-1]
				cs.historyIdx = -1
				cs.search()
			}
			return cs, nil

		default:
			ch := msg.String()
			if len(ch) == 1 && ch[0] >= 32 {
				cs.query += ch
				cs.historyIdx = -1
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

	// Search input with regex mode indicator
	prompt := SearchPromptStyle.Render("  > ")
	input := cs.query + DimStyle.Render("_")
	modeIndicator := DimStyle.Render(" [Aa]")
	if cs.regexMode {
		modeIndicator = lipgloss.NewStyle().Foreground(yellow).Bold(true).Render(" [.*]")
	}
	b.WriteString(prompt + input + modeIndicator)
	b.WriteString("\n")
	if cs.regexErr != "" {
		errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
		b.WriteString(errStyle.Render("  Invalid regex: " + cs.regexErr))
		b.WriteString("\n")
	}
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
	footer := "  Enter: jump  ↑↓: history  Alt+R: regex  Esc: close"
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
// When regexMode is enabled it compiles the query as a regex pattern instead.
func (cs *ContentSearch) search() {
	cs.results = nil
	cs.cursor = 0
	cs.scroll = 0
	cs.regexErr = ""

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
	// Collect all file paths and sort them for deterministic ordering.
	paths := make([]string, 0, len(cs.noteContents))
	for p := range cs.noteContents {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	if cs.regexMode {
		cs.searchRegex(paths)
	} else {
		cs.searchPlain(paths)
	}
}

// searchPlain performs case-insensitive plain-text search.
func (cs *ContentSearch) searchPlain(paths []string) {
	lowerQuery := strings.ToLower(cs.query)

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

// searchRegex performs regex-based search across all note contents.
func (cs *ContentSearch) searchRegex(paths []string) {
	re, err := regexp.Compile("(?i)" + cs.query)
	if err != nil {
		cs.regexErr = err.Error()
		return
	}

	for _, path := range paths {
		content := cs.noteContents[path]
		lines := strings.Split(content, "\n")

		for lineIdx, line := range lines {
			loc := re.FindStringIndex(line)
			if loc == nil {
				continue
			}

			cs.results = append(cs.results, ContentSearchResult{
				FilePath: path,
				Line:     lineIdx,
				Col:      loc[0],
				Context:  line,
			})

			if len(cs.results) >= 100 {
				return
			}
		}
	}
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

	if cs.regexMode {
		return cs.highlightMatchRegex(display)
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

// highlightMatchRegex highlights regex matches in the display line.
func (cs ContentSearch) highlightMatchRegex(display string) string {
	re, err := regexp.Compile("(?i)" + cs.query)
	if err != nil {
		return NormalItemStyle.Render(display)
	}

	locs := re.FindAllStringIndex(display, -1)
	if len(locs) == 0 {
		return NormalItemStyle.Render(display)
	}

	var result strings.Builder
	prev := 0
	for _, loc := range locs {
		if loc[0] > prev {
			result.WriteString(NormalItemStyle.Render(display[prev:loc[0]]))
		}
		result.WriteString(MatchHighlightStyle.Render(display[loc[0]:loc[1]]))
		prev = loc[1]
	}
	if prev < len(display) {
		result.WriteString(NormalItemStyle.Render(display[prev:]))
	}
	return result.String()
}

// visibleHeight returns the number of result lines visible in the content area.
func (cs ContentSearch) visibleHeight() int {
	h := cs.height - 14
	if h < 5 {
		h = 5
	}
	return h
}
