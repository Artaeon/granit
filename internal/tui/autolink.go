package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// MentionStatus tracks whether a mention has been reviewed and what
// decision the user made.
type MentionStatus int

const (
	MentionPending  MentionStatus = iota // not yet reviewed
	MentionAccepted                      // user accepted → convert to wikilink
	MentionRejected                      // user skipped / rejected
)

// Replacement stores an accepted mention replacement that the caller
// (app.go) can apply to the editor content.
type Replacement struct {
	Line     int    // line number in the editor
	Col      int    // column position
	Length   int    // length of original text
	NoteName string // display name to use inside [[...]]
}

// AutoLinker detects potential wikilink targets in text and suggests them.
// It maintains a set of known note names and checks editor content for
// unlinked references. It also provides an interactive overlay where the
// user can review each mention and accept or reject it.
type AutoLinker struct {
	noteNames []string // lowercase note names (without .md)
	notePaths []string // original paths

	// Overlay state
	active   bool
	mentions []LinkSuggestion
	statuses []MentionStatus // per-mention decision
	cursor   int
	scroll   int
	width    int
	height   int
	lines    []string // snapshot of editor lines for context display

	// Collected accepted replacements, ready for the caller to consume.
	replacements []Replacement
}

// NewAutoLinker creates an AutoLinker from the vault's note paths.
func NewAutoLinker() *AutoLinker {
	return &AutoLinker{}
}

// SetNotes updates the known note list.
func (al *AutoLinker) SetNotes(paths []string) {
	al.notePaths = paths
	al.noteNames = make([]string, len(paths))
	for i, p := range paths {
		name := p
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		name = strings.TrimSuffix(name, ".md")
		al.noteNames[i] = strings.ToLower(name)
	}
}

// SetSize updates the available dimensions for the overlay.
func (al *AutoLinker) SetSize(width, height int) {
	al.width = width
	al.height = height
}

// IsActive returns whether the unlinked mentions overlay is visible.
func (al *AutoLinker) IsActive() bool {
	return al.active
}

// Open scans the given content for unlinked mentions and opens the
// interactive overlay. If no mentions are found the overlay is not
// activated.
func (al *AutoLinker) Open(content string, currentNote string) {
	al.mentions = al.FindUnlinkedMentions(content, currentNote)
	if len(al.mentions) == 0 {
		// Nothing to review.
		al.active = false
		return
	}
	al.statuses = make([]MentionStatus, len(al.mentions))
	al.cursor = 0
	al.scroll = 0
	al.replacements = nil
	al.lines = strings.Split(content, "\n")
	al.active = true
}

// Close hides the overlay and collects all accepted mentions into the
// replacements slice. Replacements are sorted from last line/col to first
// so the caller can apply them without shifting indices.
func (al *AutoLinker) Close() {
	al.active = false
	al.collectReplacements()
}

// collectReplacements gathers accepted mentions in reverse order (bottom
// of the document first) so the caller can apply them sequentially without
// column/line offset drift.
func (al *AutoLinker) collectReplacements() {
	al.replacements = nil
	// Walk in reverse so later positions come first.
	for i := len(al.mentions) - 1; i >= 0; i-- {
		if al.statuses[i] == MentionAccepted {
			m := al.mentions[i]
			al.replacements = append(al.replacements, Replacement{
				Line:     m.Line,
				Col:      m.Col,
				Length:   m.Length,
				NoteName: m.NoteName,
			})
		}
	}
}

// GetReplacements returns the list of accepted replacements that the
// caller should apply to the editor content. Each call drains the list.
func (al *AutoLinker) GetReplacements() []Replacement {
	r := al.replacements
	al.replacements = nil
	return r
}

// reviewedCount returns how many mentions have been accepted or rejected.
func (al *AutoLinker) reviewedCount() int {
	n := 0
	for _, s := range al.statuses {
		if s != MentionPending {
			n++
		}
	}
	return n
}

// acceptedCount returns how many mentions were accepted.
func (al *AutoLinker) acceptedCount() int {
	n := 0
	for _, s := range al.statuses {
		if s == MentionAccepted {
			n++
		}
	}
	return n
}

// advanceToNextPending moves the cursor to the next unreviewed mention,
// wrapping around if necessary. Returns false if all are reviewed.
func (al *AutoLinker) advanceToNextPending() bool {
	if len(al.mentions) == 0 {
		return false
	}
	start := al.cursor
	for i := 0; i < len(al.mentions); i++ {
		idx := (start + 1 + i) % len(al.mentions)
		if al.statuses[idx] == MentionPending {
			al.cursor = idx
			return true
		}
	}
	return false
}

// LinkSuggestion represents an unlinked mention that could be a wikilink.
type LinkSuggestion struct {
	NotePath string // the note it should link to
	NoteName string // display name
	Line     int    // line number in editor
	Col      int    // column position
	Length   int    // length of the matched text
}

// FindUnlinkedMentions scans content for note names that appear in the text
// but are not already wrapped in [[...]].
func (al *AutoLinker) FindUnlinkedMentions(content string, currentNote string) []LinkSuggestion {
	if len(al.noteNames) == 0 {
		return nil
	}

	lines := strings.Split(content, "\n")
	var suggestions []LinkSuggestion
	seen := make(map[string]bool)

	currentBase := strings.TrimSuffix(currentNote, ".md")
	if idx := strings.LastIndex(currentBase, "/"); idx >= 0 {
		currentBase = currentBase[idx+1:]
	}
	currentBaseLower := strings.ToLower(currentBase)

	for lineNum, line := range lines {
		lineLower := strings.ToLower(line)

		for i, nameLower := range al.noteNames {
			// Skip self-references and very short names (< 3 chars)
			if len(nameLower) < 3 || nameLower == currentBaseLower {
				continue
			}
			// Skip if already suggested this note
			if seen[nameLower] {
				continue
			}

			// Find the name in this line
			searchFrom := 0
			for {
				idx := strings.Index(lineLower[searchFrom:], nameLower)
				if idx < 0 {
					break
				}
				col := searchFrom + idx

				// Check it's a whole word (not part of a larger word)
				if !isWordBoundary(lineLower, col, len(nameLower)) {
					searchFrom = col + 1
					continue
				}

				// Check it's not already inside [[...]]
				if isInsideWikilink(line, col) {
					searchFrom = col + 1
					continue
				}

				// Check it's not in a code block or frontmatter
				if isInCodeSpan(line, col) {
					searchFrom = col + 1
					continue
				}

				// Extract the original case version
				originalName := al.notePaths[i]
				baseName := originalName
				if idx := strings.LastIndex(baseName, "/"); idx >= 0 {
					baseName = baseName[idx+1:]
				}
				baseName = strings.TrimSuffix(baseName, ".md")

				suggestions = append(suggestions, LinkSuggestion{
					NotePath: originalName,
					NoteName: baseName,
					Line:     lineNum,
					Col:      col,
					Length:   len(nameLower),
				})
				seen[nameLower] = true
				break
			}
		}
	}

	return suggestions
}

// ApplyLink wraps the text at the given position with [[ ]].
func ApplyLink(content []string, line, col, length int) {
	if line >= len(content) {
		return
	}
	l := content[line]
	if col+length > len(l) {
		return
	}
	content[line] = l[:col] + "[[" + l[col:col+length] + "]]" + l[col+length:]
}

// Update handles keyboard input for the unlinked mentions overlay.
func (al AutoLinker) Update(msg tea.Msg) (AutoLinker, tea.Cmd) {
	if !al.active {
		return al, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			al.Close()

		case "up", "k":
			if al.cursor > 0 {
				al.cursor--
				if al.cursor < al.scroll {
					al.scroll = al.cursor
				}
			}

		case "down", "j":
			if al.cursor < len(al.mentions)-1 {
				al.cursor++
				visH := al.visibleHeight()
				if al.cursor >= al.scroll+visH {
					al.scroll = al.cursor - visH + 1
				}
			}

		case "enter", "a":
			// Accept: mark current mention as accepted
			if len(al.mentions) > 0 && al.cursor < len(al.mentions) {
				al.statuses[al.cursor] = MentionAccepted
				if !al.advanceToNextPending() {
					// All reviewed — auto-close
					al.Close()
				} else {
					al.ensureVisible()
				}
			}

		case "x":
			// Reject / skip: mark current mention as rejected
			if len(al.mentions) > 0 && al.cursor < len(al.mentions) {
				al.statuses[al.cursor] = MentionRejected
				if !al.advanceToNextPending() {
					al.Close()
				} else {
					al.ensureVisible()
				}
			}

		case "A":
			// Accept all remaining pending mentions
			for i := range al.statuses {
				if al.statuses[i] == MentionPending {
					al.statuses[i] = MentionAccepted
				}
			}
			al.Close()

		case "X":
			// Reject all remaining pending mentions
			for i := range al.statuses {
				if al.statuses[i] == MentionPending {
					al.statuses[i] = MentionRejected
				}
			}
			al.Close()
		}
	}
	return al, nil
}

// visibleHeight returns the number of mention rows that fit in the
// overlay at the current terminal height.
func (al *AutoLinker) visibleHeight() int {
	// Each mention takes ~4 lines in rendering: 2 content lines + separator + gap.
	// Header/footer uses roughly 12 lines. Use 4-line estimate per mention.
	h := (al.height - 12) / 4
	if h < 2 {
		h = 2
	}
	return h
}

// ensureVisible adjusts scroll so cursor is in the visible window.
func (al *AutoLinker) ensureVisible() {
	visH := al.visibleHeight()
	if al.cursor < al.scroll {
		al.scroll = al.cursor
	}
	if al.cursor >= al.scroll+visH {
		al.scroll = al.cursor - visH + 1
	}
}

// contextSnippet extracts a short snippet of the line around the mention,
// highlighting the matched text.
func (al *AutoLinker) contextSnippet(m LinkSuggestion, maxWidth int) string {
	if m.Line >= len(al.lines) {
		return ""
	}
	line := al.lines[m.Line]

	// Determine window around the match
	matchEnd := m.Col + m.Length
	if matchEnd > len(line) {
		matchEnd = len(line)
	}

	// We want to show some context before and after the match
	contextBudget := maxWidth - m.Length - 6 // 6 for highlight markers
	if contextBudget < 0 {
		contextBudget = 0
	}
	beforeBudget := contextBudget / 2
	afterBudget := contextBudget - beforeBudget

	start := m.Col - beforeBudget
	end := matchEnd + afterBudget

	prefix := ""
	suffix := ""
	if start < 0 {
		start = 0
	} else if start > 0 {
		prefix = "..."
	}
	if end > len(line) {
		end = len(line)
	} else if end < len(line) {
		suffix = "..."
	}

	before := line[start:m.Col]
	matched := line[m.Col:matchEnd]
	after := ""
	if matchEnd < end {
		after = line[matchEnd:end]
	}

	dimBefore := DimStyle.Render(prefix + before)
	hlMatch := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(matched)
	dimAfter := DimStyle.Render(after + suffix)

	return dimBefore + hlMatch + dimAfter
}

// View renders the unlinked mentions overlay.
func (al AutoLinker) View() string {
	width := al.width / 2
	if width < 55 {
		width = 55
	}
	if width > 80 {
		width = 80
	}
	innerWidth := width - 6

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(sapphire).
		Bold(true).
		Render("  Unlinked Mentions")
	total := len(al.mentions)
	reviewed := al.reviewedCount()
	accepted := al.acceptedCount()
	counter := lipgloss.NewStyle().
		Foreground(overlay0).
		Render(fmt.Sprintf("  %d/%d reviewed", reviewed, total))
	b.WriteString(title + counter)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")

	// Accepted summary
	if accepted > 0 {
		acceptBadge := lipgloss.NewStyle().Foreground(green).
			Render(fmt.Sprintf("  %d accepted", accepted))
		b.WriteString(acceptBadge)
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if total == 0 {
		b.WriteString(DimStyle.Render("  No unlinked mentions found"))
		b.WriteString("\n")
	} else {
		visH := al.visibleHeight()
		end := al.scroll + visH
		if end > total {
			end = total
		}

		statusIcons := map[MentionStatus]string{
			MentionPending:  lipgloss.NewStyle().Foreground(overlay0).Render("\u25cb"), // open circle
			MentionAccepted: lipgloss.NewStyle().Foreground(green).Render("\u2713"),    // check
			MentionRejected: lipgloss.NewStyle().Foreground(red).Render("\u2717"),      // cross
		}

		noteStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
		lineNumStyle := lipgloss.NewStyle().Foreground(overlay0)

		for i := al.scroll; i < end; i++ {
			m := al.mentions[i]
			status := al.statuses[i]
			icon := statusIcons[status]

			// First line: status icon + note name + proposed wikilink
			proposedLink := lipgloss.NewStyle().Foreground(mauve).
				Render("[[" + m.NoteName + "]]")
			noteLabel := noteStyle.Render(m.NoteName)
			lineLabel := lineNumStyle.Render(fmt.Sprintf("  L%d", m.Line+1))

			firstLine := fmt.Sprintf("  %s  %s %s %s", icon, noteLabel, lineLabel, proposedLink)

			// Second line: context snippet
			snippet := al.contextSnippet(m, innerWidth-6)
			secondLine := "     " + snippet

			if i == al.cursor {
				// Highlighted row
				row := lipgloss.NewStyle().
					Background(surface0).
					Width(innerWidth).
					Render(firstLine)
				b.WriteString(row)
				b.WriteString("\n")
				row2 := lipgloss.NewStyle().
					Background(surface0).
					Width(innerWidth).
					Render(secondLine)
				b.WriteString(row2)
			} else {
				b.WriteString(firstLine)
				b.WriteString("\n")
				b.WriteString(secondLine)
			}
			if i < end-1 {
				b.WriteString("\n")
				b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2508", innerWidth-4)))
				b.WriteString("\n")
			}
		}

		// Scroll indicator
		if total > visH {
			b.WriteString("\n")
			scrollInfo := DimStyle.Render(
				fmt.Sprintf("  %d-%d of %d", al.scroll+1, end, total))
			b.WriteString(scrollInfo)
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")

	// Key hints
	enterKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Enter/a")
	enterDesc := DimStyle.Render(": accept  ")
	skipKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("x")
	skipDesc := DimStyle.Render(": skip  ")
	allAcceptKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("A")
	allAcceptDesc := DimStyle.Render(": accept all  ")
	allRejectKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("X")
	allRejectDesc := DimStyle.Render(": skip all")
	b.WriteString("  " + enterKey + enterDesc + skipKey + skipDesc)
	b.WriteString("\n")
	b.WriteString("  " + allAcceptKey + allAcceptDesc + allRejectKey + allRejectDesc)
	b.WriteString("\n")
	navKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("j/k")
	navDesc := DimStyle.Render(": navigate  ")
	escKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Esc/q")
	escDesc := DimStyle.Render(": close")
	b.WriteString("  " + navKey + navDesc + escKey + escDesc)

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(sapphire).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// isWordBoundary checks if the match at position pos with given length
// is surrounded by non-alphanumeric characters.
func isWordBoundary(line string, pos, length int) bool {
	if pos > 0 {
		ch := line[pos-1]
		if isAlphaNum(ch) {
			return false
		}
	}
	end := pos + length
	if end < len(line) {
		ch := line[end]
		if isAlphaNum(ch) {
			return false
		}
	}
	return true
}

func isAlphaNum(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '-'
}

// isInsideWikilink checks if position col is inside a [[...]] wikilink.
func isInsideWikilink(line string, col int) bool {
	// Look backwards for [[ without ]]
	before := line[:col]
	lastOpen := strings.LastIndex(before, "[[")
	if lastOpen < 0 {
		return false
	}
	lastClose := strings.LastIndex(before, "]]")
	return lastOpen > lastClose
}

// isInCodeSpan checks if position col is inside a `...` code span.
func isInCodeSpan(line string, col int) bool {
	inCode := false
	for i := 0; i < col && i < len(line); i++ {
		if line[i] == '`' {
			inCode = !inCode
		}
	}
	return inCode
}
