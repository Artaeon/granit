package tui

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"

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

// SpellChecker provides spell-checking integration by piping text through
// aspell or hunspell, and presents an overlay for reviewing and applying
// corrections.
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
	available   bool // whether aspell/hunspell is installed
	tool        string // "aspell" or "hunspell"
}

// NewSpellChecker probes for aspell or hunspell and returns a ready-to-use
// SpellChecker. If neither tool is found, IsAvailable() will return false.
func NewSpellChecker() SpellChecker {
	sc := SpellChecker{}

	if path, err := exec.LookPath("aspell"); err == nil && path != "" {
		sc.available = true
		sc.tool = "aspell"
	} else if path, err := exec.LookPath("hunspell"); err == nil && path != "" {
		sc.available = true
		sc.tool = "hunspell"
	}

	return sc
}

// IsAvailable reports whether a spell-checking tool was found on the system.
func (sc *SpellChecker) IsAvailable() bool {
	return sc.available
}

// stripMarkdown removes markdown formatting so the spell checker does not
// flag markup syntax as misspelled words.
func stripMarkdownForSpellCheck(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	inCodeBlock := false

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

		cleaned := line

		// Strip frontmatter delimiters
		if trimmed == "---" {
			result = append(result, "")
			continue
		}

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
	if !sc.available {
		return nil
	}

	cleaned := stripMarkdownForSpellCheck(content)
	originalLines := strings.Split(content, "\n")

	var args []string
	if sc.tool == "aspell" {
		args = []string{"pipe"}
	} else {
		args = []string{"-a"}
	}

	cmd := exec.Command(sc.tool, args...)
	cmd.Stdin = strings.NewReader(cleaned)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}

	var words []MisspelledWord
	outputLines := strings.Split(string(out), "\n")

	// aspell/hunspell pipe mode: the first line is a version banner.
	// Then for each input line, output lines appear followed by a blank line
	// separating the next input line.
	inputLine := 0
	firstLine := true

	for _, ol := range outputLines {
		if firstLine {
			// Skip version/banner line
			firstLine = false
			continue
		}

		if ol == "" {
			// Blank line = separator between input lines
			inputLine++
			continue
		}

		if strings.HasPrefix(ol, "&") {
			// & word count offset: suggestion1, suggestion2, ...
			word, offset, suggestions := parseAmpersandLine(ol)
			if word == "" {
				continue
			}

			col := findWordCol(originalLines, inputLine, word, offset)

			if len(suggestions) > 5 {
				suggestions = suggestions[:5]
			}

			words = append(words, MisspelledWord{
				Word:    word,
				Line:    inputLine,
				Col:     col,
				Suggest: suggestions,
			})
		} else if strings.HasPrefix(ol, "#") {
			// # word offset — no suggestions
			word, offset := parseHashLine(ol)
			if word == "" {
				continue
			}

			col := findWordCol(originalLines, inputLine, word, offset)

			words = append(words, MisspelledWord{
				Word:    word,
				Line:    inputLine,
				Col:     col,
				Suggest: nil,
			})
		}
		// Lines starting with * or @ are correct/ignored — skip them.
	}

	return words
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
				sc.selected = &sc.words[sc.cursor]
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
				sc.selected = &sc.words[sc.cursor]
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
			// Ignore — remove word from list
			if len(sc.words) > 0 && sc.cursor < len(sc.words) {
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
	titleIcon := "Spell Check"
	toolName := sc.tool
	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  " + titleIcon + " (" + toolName + ")")
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

			// Suggestions line
			if len(w.Suggest) > 0 {
				sugLine := "      Suggestions: "
				for si, s := range w.Suggest {
					if si == 0 {
						sugLine += firstSuggestStyle.Render(s)
					} else {
						sugLine += NormalItemStyle.Render(", " + s)
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
	b.WriteString(DimStyle.Render("  Enter/1-5: fix  i: ignore  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(green).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
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
		for c := w.Col; c < w.Col+len(w.Word); c++ {
			positions[w.Line][c] = true
		}
	}

	return positions
}
