package tui

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MisspelledWord represents a word that the spell checker flagged, along with
// its location in the original content and up to 5 replacement suggestions.
type MisspelledWord struct {
	Word    string
	Line    int
	Col     int
	Suggest []string // up to 5 suggestions
}

// spellCheckDoneMsg is sent when an async spell check completes.
type spellCheckDoneMsg struct {
	words []MisspelledWord
}

// spellCheckTickMsg fires after a debounce period to trigger inline checking.
type spellCheckTickMsg struct {
	editTime time.Time
}

// SpellChecker provides spell-checking integration using aspell, hunspell, or
// a built-in dictionary fallback. It presents an overlay for reviewing and
// applying corrections, and supports inline highlighting of misspelled words.
type SpellChecker struct {
	active      bool
	width       int
	height      int
	words       []MisspelledWord
	cursor      int
	scroll      int
	selected    *MisspelledWord // currently highlighted misspelling
	replacement string          // the chosen replacement
	applied     bool

	// Spell engine (shared between overlay and inline checking)
	engine *spellEngine

	// Inline spell check state
	inlineEnabled bool              // config-driven toggle
	inlineWords   []MisspelledWord  // last inline check results
	inlinePos     map[int]map[int]bool // line -> col -> true for rendering

}

// NewSpellChecker probes for aspell, hunspell, or a built-in dictionary and
// returns a ready-to-use SpellChecker.
func NewSpellChecker() SpellChecker {
	return SpellChecker{
		engine: newSpellEngine(),
	}
}

// IsAvailable reports whether a spell-checking backend was found on the system.
func (sc *SpellChecker) IsAvailable() bool {
	return sc.engine.isAvailable()
}

// BackendName returns the name of the active spell-checking backend.
func (sc *SpellChecker) BackendName() string {
	return sc.engine.backendName()
}

// SetInlineEnabled enables or disables inline spell check highlighting.
func (sc *SpellChecker) SetInlineEnabled(enabled bool) {
	sc.inlineEnabled = enabled
	if !enabled {
		sc.inlineWords = nil
		sc.inlinePos = nil
	}
}

// InlineEnabled reports whether inline spell checking is currently on.
func (sc *SpellChecker) InlineEnabled() bool {
	return sc.inlineEnabled && sc.engine.isAvailable()
}

// stripMarkdown removes markdown formatting so the spell checker does not
// flag markup syntax as misspelled words.
func stripMarkdownForSpellCheck(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inCodeBlock := false
	inFrontmatter := false
	pastFirstLine := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Toggle code blocks — replace the entire line with blanks so that
		// line numbers stay aligned with the original content.
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			result = append(result, "")
			continue
		}
		if inCodeBlock {
			result = append(result, "")
			continue
		}

		// Track frontmatter region
		if trimmed == "---" {
			if !pastFirstLine {
				inFrontmatter = true
				pastFirstLine = true
				result = append(result, "")
				continue
			}
			if inFrontmatter {
				inFrontmatter = false
				result = append(result, "")
				continue
			}
		}
		pastFirstLine = true
		if inFrontmatter {
			result = append(result, "")
			continue
		}

		cleaned := line

		// Strip heading markers
		cleaned = regexp.MustCompile(`^#{1,6}\s`).ReplaceAllString(cleaned, "")

		// Strip inline code
		cleaned = regexp.MustCompile("`[^`]*`").ReplaceAllString(cleaned, "")

		// Strip wiki-links [[target|display]] -> display, [[target]] -> target
		cleaned = regexp.MustCompile(`\[\[([^|\]]*\|)?([^\]]*)\]\]`).ReplaceAllString(cleaned, "$2")

		// Strip markdown links [text](url) -> text
		cleaned = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`).ReplaceAllString(cleaned, "$1")

		// Strip images ![alt](url)
		cleaned = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`).ReplaceAllString(cleaned, "$1")

		// Strip URLs
		cleaned = regexp.MustCompile(`https?://\S+`).ReplaceAllString(cleaned, "")

		// Strip bold/italic markers
		cleaned = strings.ReplaceAll(cleaned, "***", "")
		cleaned = strings.ReplaceAll(cleaned, "**", "")
		cleaned = strings.ReplaceAll(cleaned, "*", "")
		cleaned = strings.ReplaceAll(cleaned, "___", "")
		cleaned = strings.ReplaceAll(cleaned, "__", "")
		cleaned = strings.ReplaceAll(cleaned, "_", " ")

		// Strip HTML tags
		cleaned = regexp.MustCompile(`<[^>]+>`).ReplaceAllString(cleaned, "")

		result = append(result, cleaned)
	}

	return strings.Join(result, "\n")
}

// Check runs the spell checker on content and returns all misspelled words
// with their positions in the original text.
func (sc *SpellChecker) Check(content string) []MisspelledWord {
	return sc.engine.check(content)
}

// parseAmpersandLine parses an aspell "&" line:
//
//	& word count offset: suggestion1, suggestion2, ...
func parseAmpersandLine(line string) (word string, offset int, suggestions []string) {
	// Remove the leading "& "
	line = strings.TrimPrefix(line, "& ")

	// Split at colon to separate metadata from suggestions
	colonIdx := strings.Index(line, ": ")
	if colonIdx < 0 {
		return "", 0, nil
	}

	meta := line[:colonIdx]
	sugStr := line[colonIdx+2:]

	parts := strings.Fields(meta)
	if len(parts) < 3 {
		return "", 0, nil
	}

	word = parts[0]
	offset, _ = strconv.Atoi(parts[2])

	for _, s := range strings.Split(sugStr, ", ") {
		s = strings.TrimSpace(s)
		if s != "" {
			suggestions = append(suggestions, s)
		}
	}

	return word, offset, suggestions
}

// parseHashLine parses an aspell "#" line:
//
//	# word offset
func parseHashLine(line string) (word string, offset int) {
	line = strings.TrimPrefix(line, "# ")
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", 0
	}
	word = parts[0]
	offset, _ = strconv.Atoi(parts[1])
	return word, offset
}

// findWordCol maps the aspell offset (which is relative to the cleaned line)
// back to the column position in the original content line. It searches for
// the misspelled word in the original line to get an accurate column.
func findWordCol(originalLines []string, lineIdx int, word string, aspellOffset int) int {
	if lineIdx < 0 || lineIdx >= len(originalLines) {
		// Fallback: use aspell offset (1-based) converted to 0-based.
		if aspellOffset > 0 {
			return aspellOffset - 1
		}
		return 0
	}

	origLine := originalLines[lineIdx]

	// Try to find the word in the original line starting near the aspell offset.
	// aspell offsets are 1-based.
	searchStart := 0
	if aspellOffset > 1 && aspellOffset-1 < len(origLine) {
		searchStart = aspellOffset - 1
	}

	idx := strings.Index(origLine[searchStart:], word)
	if idx >= 0 {
		return searchStart + idx
	}

	// Fallback: search from the beginning
	idx = strings.Index(origLine, word)
	if idx >= 0 {
		return idx
	}

	// Last resort
	if aspellOffset > 0 {
		return aspellOffset - 1
	}
	return 0
}

// IsActive reports whether the spell check overlay is currently open.
func (sc *SpellChecker) IsActive() bool {
	return sc.active
}

// Open runs a spell check on content and opens the overlay to display results.
func (sc *SpellChecker) Open(content string) {
	sc.active = true
	sc.cursor = 0
	sc.scroll = 0
	sc.replacement = ""
	sc.applied = false
	sc.selected = nil
	sc.words = sc.Check(content)

	if len(sc.words) > 0 {
		sc.selected = &sc.words[0]
	}
}

// Close dismisses the spell check overlay.
func (sc *SpellChecker) Close() {
	sc.active = false
}

// SetSize updates the available dimensions for rendering the overlay.
func (sc *SpellChecker) SetSize(w, h int) {
	sc.width = w
	sc.height = h
}

// GetCorrection returns the most recently applied correction. After reading
// the result the applied flag is cleared so each correction is consumed once.
func (sc *SpellChecker) GetCorrection() (word string, line, col int, replacement string, ok bool) {
	if !sc.applied || sc.selected == nil {
		return "", 0, 0, "", false
	}
	sc.applied = false
	return sc.selected.Word, sc.selected.Line, sc.selected.Col, sc.replacement, true
}

// AddToPersonalDict adds the selected word to the personal dictionary.
func (sc *SpellChecker) AddToPersonalDict(word string) {
	sc.engine.addToPersonal(word)
}

// IgnoreAllOccurrences marks a word as ignored for this session and removes
// all occurrences from the current results list.
func (sc *SpellChecker) IgnoreAllOccurrences(word string) {
	sc.engine.addSessionIgnore(word)
	lw := strings.ToLower(word)

	// Remove all occurrences from the word list
	filtered := sc.words[:0]
	for _, w := range sc.words {
		if strings.ToLower(w.Word) != lw {
			filtered = append(filtered, w)
		}
	}
	sc.words = filtered
}

// removeCurrentWord removes the word at the cursor from the list and adjusts.
func (sc *SpellChecker) removeCurrentWord() {
	if len(sc.words) == 0 || sc.cursor >= len(sc.words) {
		return
	}
	sc.words = append(sc.words[:sc.cursor], sc.words[sc.cursor+1:]...)
	if sc.cursor >= len(sc.words) && sc.cursor > 0 {
		sc.cursor--
	}
	if len(sc.words) > 0 {
		sc.selected = &sc.words[sc.cursor]
	} else {
		sc.selected = nil
	}
	sc.applied = false
	sc.replacement = ""
}

// Update handles keyboard input for the spell check overlay.
func (sc SpellChecker) Update(msg tea.Msg) (SpellChecker, tea.Cmd) {
	if !sc.active {
		return sc, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			sc.active = false
			return sc, nil

		case "up", "k":
			if sc.cursor > 0 {
				sc.cursor--
				if sc.cursor < sc.scroll {
					sc.scroll = sc.cursor
				}
				if len(sc.words) > 0 && sc.cursor < len(sc.words) {
					sc.selected = &sc.words[sc.cursor]
				} else {
					sc.selected = nil
				}
				sc.applied = false
				sc.replacement = ""
			}
			return sc, nil

		case "down", "j":
			if sc.cursor < len(sc.words)-1 {
				sc.cursor++
				visH := sc.visibleHeight()
				if sc.cursor >= sc.scroll+visH {
					sc.scroll = sc.cursor - visH + 1
				}
				if len(sc.words) > 0 && sc.cursor < len(sc.words) {
					sc.selected = &sc.words[sc.cursor]
				} else {
					sc.selected = nil
				}
				sc.applied = false
				sc.replacement = ""
			}
			return sc, nil

		case "enter":
			// Apply first suggestion
			if len(sc.words) > 0 && sc.cursor < len(sc.words) {
				w := &sc.words[sc.cursor]
				if len(w.Suggest) > 0 {
					sc.selected = w
					sc.replacement = w.Suggest[0]
					sc.applied = true
				}
			}
			return sc, nil

		case "1", "2", "3", "4", "5":
			idx := int(msg.String()[0]-'0') - 1
			if len(sc.words) > 0 && sc.cursor < len(sc.words) {
				w := &sc.words[sc.cursor]
				if idx < len(w.Suggest) {
					sc.selected = w
					sc.replacement = w.Suggest[idx]
					sc.applied = true
				}
			}
			return sc, nil

		case "i":
			// Ignore this occurrence — remove from list
			sc.removeCurrentWord()
			return sc, nil

		case "d":
			// Add to personal dictionary and remove all occurrences
			if len(sc.words) > 0 && sc.cursor < len(sc.words) {
				w := sc.words[sc.cursor]
				sc.AddToPersonalDict(w.Word)
				sc.IgnoreAllOccurrences(w.Word)
				// Adjust cursor
				if sc.cursor >= len(sc.words) && sc.cursor > 0 {
					sc.cursor--
				}
				if len(sc.words) > 0 {
					sc.selected = &sc.words[sc.cursor]
				} else {
					sc.selected = nil
				}
			}
			return sc, nil

		case "a":
			// Ignore all occurrences of this word (session only)
			if len(sc.words) > 0 && sc.cursor < len(sc.words) {
				w := sc.words[sc.cursor]
				sc.IgnoreAllOccurrences(w.Word)
				// Adjust cursor
				if sc.cursor >= len(sc.words) && sc.cursor > 0 {
					sc.cursor--
				}
				if len(sc.words) > 0 {
					sc.selected = &sc.words[sc.cursor]
				} else {
					sc.selected = nil
				}
			}
			return sc, nil
		}
	}

	return sc, nil
}

// visibleHeight returns the number of word entries that fit in the overlay.
func (sc *SpellChecker) visibleHeight() int {
	// Each entry takes ~3 lines (word line, suggestion line, blank).
	// Reserve space for title, separator, summary, bottom hints.
	h := (sc.height - 10) / 3
	if h < 3 {
		h = 3
	}
	return h
}

// View renders the spell check overlay.
func (sc SpellChecker) View() string {
	width := sc.width / 2
	if width < 50 {
		width = 50
	}
	if width > 80 {
		width = 80
	}

	var b strings.Builder

	// Title
	toolName := sc.engine.backendName()
	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  Spell Check (" + toolName + ")")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n\n")

	// Summary
	count := len(sc.words)
	if count == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(green).Render("  No misspelled words found"))
		b.WriteString("\n")
	} else {
		summary := "  " + smallNum(count) + " misspelled word"
		if count != 1 {
			summary += "s"
		}
		summary += " found"
		b.WriteString(DimStyle.Render(summary))
		b.WriteString("\n\n")

		visH := sc.visibleHeight()
		end := sc.scroll + visH
		if end > len(sc.words) {
			end = len(sc.words)
		}

		misspelledStyle := lipgloss.NewStyle().
			Foreground(red).
			Bold(true)

		firstSuggestStyle := lipgloss.NewStyle().
			Foreground(green)

		selectedRowStyle := lipgloss.NewStyle().
			Background(surface0).
			Foreground(peach)

		for i := sc.scroll; i < end; i++ {
			w := sc.words[i]
			isSelected := i == sc.cursor

			// Word line: > "word" (line X, col Y)
			pointer := "  "
			if isSelected {
				pointer = "> "
			}

			wordDisplay := misspelledStyle.Render("\"" + w.Word + "\"")
			position := DimStyle.Render(" (line " + smallNum(w.Line+1) + ", col " + smallNum(w.Col+1) + ")")

			wordLine := "  " + pointer + wordDisplay + position

			if isSelected {
				b.WriteString(selectedRowStyle.
					Width(width - 6).
					Render(wordLine))
			} else {
				b.WriteString(wordLine)
			}
			b.WriteString("\n")

			// Suggestions line with numbered hints
			if len(w.Suggest) > 0 {
				sugLine := "      "
				for si, s := range w.Suggest {
					numStr := lipgloss.NewStyle().Foreground(yellow).Render(smallNum(si+1) + ":")
					if si == 0 {
						sugLine += numStr + firstSuggestStyle.Render(s)
					} else {
						sugLine += NormalItemStyle.Render("  ") + numStr + NormalItemStyle.Render(s)
					}
				}
				b.WriteString(sugLine)
			} else {
				b.WriteString(DimStyle.Render("      No suggestions"))
			}
			b.WriteString("\n")

			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	// Bottom hints
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-8)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter/1-5: fix  i: ignore  a: ignore all"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  d: add to dictionary  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(green).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// --- Inline spell check support ---

// RunInlineCheck runs the spell checker asynchronously and returns a command
// that sends the results when done.
func (sc *SpellChecker) RunInlineCheck(content string) tea.Cmd {
	engine := sc.engine
	return func() tea.Msg {
		words := engine.check(content)
		return spellCheckDoneMsg{words: words}
	}
}

// HandleInlineResult processes the async spell check result for inline display.
func (sc *SpellChecker) HandleInlineResult(words []MisspelledWord) {
	sc.inlineWords = words
	sc.rebuildInlinePositions()
}

// rebuildInlinePositions constructs the line->col->bool map for fast lookup
// during editor rendering.
func (sc *SpellChecker) rebuildInlinePositions() {
	sc.inlinePos = make(map[int]map[int]bool)
	for _, w := range sc.inlineWords {
		if sc.inlinePos[w.Line] == nil {
			sc.inlinePos[w.Line] = make(map[int]bool)
		}
		for c := w.Col; c < w.Col+utf8.RuneCountInString(w.Word); c++ {
			sc.inlinePos[w.Line][c] = true
		}
	}
}

// InlinePositions returns the position map for inline misspelling highlights.
// Returns nil if inline checking is disabled or no results are available.
func (sc *SpellChecker) InlinePositions() map[int]map[int]bool {
	if !sc.inlineEnabled {
		return nil
	}
	return sc.inlinePos
}

// ScheduleInlineCheck returns a tea.Cmd that fires after a debounce delay.
// The caller should record editTime and only process the tick if it matches.
func ScheduleInlineCheck(editTime time.Time) tea.Cmd {
	return tea.Tick(1*time.Second, func(_ time.Time) tea.Msg {
		return spellCheckTickMsg{editTime: editTime}
	})
}

// GetMisspelledPositions builds a lookup map of line -> col -> true for all
// misspelled words, so the editor can highlight them while rendering.
func GetMisspelledPositions(content string, words []MisspelledWord) map[int]map[int]bool {
	positions := make(map[int]map[int]bool)
	lines := strings.Split(content, "\n")

	for _, w := range words {
		if w.Line < 0 || w.Line >= len(lines) {
			continue
		}
		if positions[w.Line] == nil {
			positions[w.Line] = make(map[int]bool)
		}
		// Mark every column that the misspelled word occupies.
		for c := w.Col; c < w.Col+utf8.RuneCountInString(w.Word); c++ {
			positions[w.Line][c] = true
		}
	}

	return positions
}
