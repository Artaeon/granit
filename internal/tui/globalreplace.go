package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

// GlobalReplaceMatch represents a single match across the vault.
type GlobalReplaceMatch struct {
	FilePath string
	Line     int
	Col      int
	Context  string
}

// GlobalReplace is an overlay for searching and replacing text across all
// vault files. It performs live search as you type, groups results by file,
// and supports replacing one match, all in a file, or all across the vault.
type GlobalReplace struct {
	active      bool
	width       int
	height      int
	findQuery   string
	replaceText string
	focusField  int // 0=find, 1=replace
	matches     []GlobalReplaceMatch
	cursor      int
	scroll      int

	// Regex search mode
	regexMode bool
	regexErr  string // non-empty when the regex pattern is invalid

	vault     *vault.Vault
	modified  map[string]bool // files changed during this session
	jumpFile  string
	jumpLine  int
	jumpReady bool
}

func NewGlobalReplace() GlobalReplace {
	return GlobalReplace{}
}

func (gr *GlobalReplace) IsActive() bool {
	return gr.active
}

func (gr *GlobalReplace) Open(v *vault.Vault) {
	gr.active = true
	gr.findQuery = ""
	gr.replaceText = ""
	gr.focusField = 0
	gr.matches = nil
	gr.cursor = 0
	gr.scroll = 0
	gr.vault = v
	gr.modified = make(map[string]bool)
	gr.jumpFile = ""
	gr.jumpLine = 0
	gr.jumpReady = false
	gr.regexErr = ""
}

func (gr *GlobalReplace) Close() {
	gr.active = false
	gr.vault = nil
}

func (gr *GlobalReplace) SetSize(w, h int) {
	gr.width = w
	gr.height = h
}

// IsRegexMode reports whether regex search is enabled.
func (gr *GlobalReplace) IsRegexMode() bool {
	return gr.regexMode
}

// ToggleRegex flips the regex mode flag and re-runs the search.
func (gr *GlobalReplace) ToggleRegex() {
	gr.regexMode = !gr.regexMode
	gr.regexErr = ""
	gr.search()
}

// GetJumpResult returns the file/line to navigate to and clears the flag.
func (gr *GlobalReplace) GetJumpResult() (string, int, bool) {
	if gr.jumpReady {
		gr.jumpReady = false
		return gr.jumpFile, gr.jumpLine, true
	}
	return "", 0, false
}

// ModifiedFiles returns the set of relPaths that were changed.
func (gr *GlobalReplace) ModifiedFiles() map[string]bool {
	return gr.modified
}

func (gr GlobalReplace) Update(msg tea.Msg) (GlobalReplace, tea.Cmd) {
	if !gr.active {
		return gr, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			gr.active = false
			return gr, nil

		case "alt+r":
			gr.regexMode = !gr.regexMode
			gr.regexErr = ""
			gr.search()
			return gr, nil

		case "tab":
			gr.focusField = (gr.focusField + 1) % 2
			return gr, nil

		case "enter":
			// Jump to the match in the editor
			if len(gr.matches) > 0 && gr.cursor < len(gr.matches) {
				m := gr.matches[gr.cursor]
				gr.jumpFile = m.FilePath
				gr.jumpLine = m.Line
				gr.jumpReady = true
				gr.active = false
			}
			return gr, nil

		case "up":
			if gr.cursor > 0 {
				gr.cursor--
				if gr.cursor < gr.scroll {
					gr.scroll = gr.cursor
				}
			}
			return gr, nil

		case "down":
			if gr.cursor < len(gr.matches)-1 {
				gr.cursor++
				visH := gr.visibleHeight()
				if gr.cursor >= gr.scroll+visH {
					gr.scroll = gr.cursor - visH + 1
				}
			}
			return gr, nil

		case "ctrl+r":
			// Replace current match
			if len(gr.matches) > 0 && gr.cursor < len(gr.matches) && gr.findQuery != "" && gr.replaceText != gr.findQuery {
				gr.replaceMatch(gr.cursor)
			}
			return gr, nil

		case "ctrl+a":
			// Replace all matches
			if len(gr.matches) > 0 && gr.findQuery != "" && gr.replaceText != gr.findQuery {
				gr.replaceAll()
			}
			return gr, nil

		case "ctrl+f":
			// Replace all in current file
			if len(gr.matches) > 0 && gr.cursor < len(gr.matches) && gr.findQuery != "" && gr.replaceText != gr.findQuery {
				gr.replaceAllInFile(gr.matches[gr.cursor].FilePath)
			}
			return gr, nil

		case "backspace":
			if gr.focusField == 0 && len(gr.findQuery) > 0 {
				gr.findQuery = gr.findQuery[:len(gr.findQuery)-1]
				gr.search()
			} else if gr.focusField == 1 && len(gr.replaceText) > 0 {
				gr.replaceText = gr.replaceText[:len(gr.replaceText)-1]
			}
			return gr, nil

		default:
			ch := msg.String()
			if len(ch) == 1 && ch[0] >= 32 {
				if gr.focusField == 0 {
					gr.findQuery += ch
					gr.search()
				} else {
					gr.replaceText += ch
				}
			}
			return gr, nil
		}
	}
	return gr, nil
}

func (gr GlobalReplace) View() string {
	width := gr.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 110 {
		width = 110
	}

	innerWidth := width - 6

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSearchChar + " Global Search & Replace")
	b.WriteString(title)

	// Regex mode indicator
	modeIndicator := DimStyle.Render(" [Aa]")
	if gr.regexMode {
		modeIndicator = lipgloss.NewStyle().Foreground(yellow).Bold(true).Render(" [.*]")
	}
	b.WriteString(modeIndicator)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")

	// Find field
	var findLabel string
	if gr.focusField == 0 {
		findLabel = SearchPromptStyle.Render("  Find:    ")
	} else {
		findLabel = DimStyle.Render("  Find:    ")
	}
	findInput := gr.findQuery
	if gr.focusField == 0 {
		findInput += DimStyle.Render("_")
	}
	b.WriteString(findLabel + findInput)

	// Match count or error
	if gr.findQuery != "" {
		if gr.regexErr != "" {
			errStyle := lipgloss.NewStyle().Foreground(red).Bold(true)
			b.WriteString("  " + errStyle.Render("Invalid regex"))
		} else {
			fileCount := gr.countFiles()
			b.WriteString(DimStyle.Render("  " + smallNum(len(gr.matches)) + " matches in " + smallNum(fileCount) + " files"))
		}
	}
	b.WriteString("\n")

	// Regex error detail
	if gr.regexErr != "" {
		errStyle := lipgloss.NewStyle().Foreground(red)
		b.WriteString(errStyle.Render("  " + gr.regexErr))
		b.WriteString("\n")
	}

	// Replace field
	var replaceLabel string
	if gr.focusField == 1 {
		replaceLabel = lipgloss.NewStyle().Foreground(green).Bold(true).Render("  Replace: ")
	} else {
		replaceLabel = DimStyle.Render("  Replace: ")
	}
	replaceInput := gr.replaceText
	if gr.focusField == 1 {
		replaceInput += DimStyle.Render("_")
	}
	b.WriteString(replaceLabel + replaceInput)
	if gr.regexMode {
		b.WriteString(DimStyle.Render("  ($1, $2 for groups)"))
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")

	// Results area
	if gr.findQuery == "" {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Type to search across all vault notes..."))
		b.WriteString("\n")
	} else if len(gr.matches) == 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  No matches found"))
		b.WriteString("\n")
	} else {
		visH := gr.visibleHeight()
		end := gr.scroll + visH
		if end > len(gr.matches) {
			end = len(gr.matches)
		}

		prevFile := ""
		for i := gr.scroll; i < end; i++ {
			m := gr.matches[i]

			// File header when file changes
			if m.FilePath != prevFile {
				if prevFile != "" {
					b.WriteString("\n")
				}
				fileStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
				b.WriteString("  " + fileStyle.Render(m.FilePath))
				b.WriteString("\n")
				prevFile = m.FilePath
			}

			// Match line
			lineNum := DimStyle.Render(fmt.Sprintf("  %4d: ", m.Line+1))
			contextLine := gr.highlightMatch(m.Context, innerWidth-10)

			// Show replacement preview
			preview := ""
			if gr.replaceText != "" && gr.findQuery != "" {
				replaced := gr.previewReplace(m.Context, innerWidth-10)
				if replaced != contextLine {
					preview = "\n" + DimStyle.Render("      → ") + lipgloss.NewStyle().Foreground(green).Render(replaced)
				}
			}

			if i == gr.cursor {
				line := lineNum + contextLine
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(innerWidth).
					Render(line))
				if preview != "" {
					b.WriteString(preview)
				}
			} else {
				b.WriteString(lineNum + contextLine)
			}
			b.WriteString("\n")
		}
	}

	// Status of replacements
	if len(gr.modified) > 0 {
		b.WriteString("\n")
		modStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString(modStyle.Render(fmt.Sprintf("  %d file(s) modified", len(gr.modified))))
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", innerWidth)))
	b.WriteString("\n")
	footer := "  Tab: switch field  Enter: jump  Alt+R: regex  Ctrl+R: replace one"
	b.WriteString(DimStyle.Render(footer))
	b.WriteString("\n")
	footer2 := "  Ctrl+F: replace in file  Ctrl+A: replace all  Esc: close"
	b.WriteString(DimStyle.Render(footer2))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(green).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// search performs case-insensitive search across all vault notes.
// When regexMode is enabled, it uses regex matching instead.
func (gr *GlobalReplace) search() {
	gr.matches = nil
	gr.cursor = 0
	gr.scroll = 0
	gr.regexErr = ""

	if gr.findQuery == "" || gr.vault == nil {
		return
	}

	paths := gr.vault.SortedPaths()
	sort.Strings(paths)

	if gr.regexMode {
		gr.searchRegex(paths)
	} else {
		gr.searchPlain(paths)
	}
}

// searchPlain performs case-insensitive plain-text search.
func (gr *GlobalReplace) searchPlain(paths []string) {
	lowerQuery := strings.ToLower(gr.findQuery)

	for _, path := range paths {
		note := gr.vault.GetNote(path)
		if note == nil {
			continue
		}

		lines := strings.Split(note.Content, "\n")
		for lineIdx, line := range lines {
			lowerLine := strings.ToLower(line)
			col := strings.Index(lowerLine, lowerQuery)
			if col == -1 {
				continue
			}

			gr.matches = append(gr.matches, GlobalReplaceMatch{
				FilePath: path,
				Line:     lineIdx,
				Col:      col,
				Context:  line,
			})

			if len(gr.matches) >= 200 {
				return
			}
		}
	}
}

// searchRegex performs regex-based search across all vault notes.
func (gr *GlobalReplace) searchRegex(paths []string) {
	re, err := regexp.Compile("(?i)" + gr.findQuery)
	if err != nil {
		gr.regexErr = err.Error()
		return
	}

	for _, path := range paths {
		note := gr.vault.GetNote(path)
		if note == nil {
			continue
		}

		lines := strings.Split(note.Content, "\n")
		for lineIdx, line := range lines {
			loc := re.FindStringIndex(line)
			if loc == nil {
				continue
			}

			gr.matches = append(gr.matches, GlobalReplaceMatch{
				FilePath: path,
				Line:     lineIdx,
				Col:      loc[0],
				Context:  line,
			})

			if len(gr.matches) >= 200 {
				return
			}
		}
	}
}

// replaceMatch replaces a single match and writes the file.
func (gr *GlobalReplace) replaceMatch(idx int) {
	if idx < 0 || idx >= len(gr.matches) {
		return
	}

	m := gr.matches[idx]
	note := gr.vault.GetNote(m.FilePath)
	if note == nil {
		return
	}

	lines := strings.Split(note.Content, "\n")
	if m.Line >= len(lines) {
		return
	}

	line := lines[m.Line]
	var newLine string
	if gr.regexMode {
		re, err := regexp.Compile("(?i)" + gr.findQuery)
		if err != nil {
			return
		}
		// Replace only the first match on this line
		loc := re.FindStringIndex(line)
		if loc == nil {
			return
		}
		newLine = line[:loc[0]] + re.ReplaceAllString(line[loc[0]:loc[1]], gr.replaceText) + line[loc[1]:]
	} else {
		newLine = caseInsensitiveReplaceFirst(line, gr.findQuery, gr.replaceText)
	}
	if newLine == line {
		return
	}
	lines[m.Line] = newLine

	newContent := strings.Join(lines, "\n")
	gr.writeFile(m.FilePath, newContent)
	gr.search() // refresh results
}

// replaceAllInFile replaces all matches in a specific file.
func (gr *GlobalReplace) replaceAllInFile(filePath string) {
	note := gr.vault.GetNote(filePath)
	if note == nil {
		return
	}

	var newContent string
	if gr.regexMode {
		re, err := regexp.Compile("(?i)" + gr.findQuery)
		if err != nil {
			return
		}
		newContent = re.ReplaceAllString(note.Content, gr.replaceText)
	} else {
		newContent = caseInsensitiveReplaceAll(note.Content, gr.findQuery, gr.replaceText)
	}
	if newContent == note.Content {
		return
	}

	gr.writeFile(filePath, newContent)
	gr.search()
}

// replaceAll replaces all matches across all files.
func (gr *GlobalReplace) replaceAll() {
	// Collect unique files
	files := make(map[string]bool)
	for _, m := range gr.matches {
		files[m.FilePath] = true
	}

	var re *regexp.Regexp
	if gr.regexMode {
		var err error
		re, err = regexp.Compile("(?i)" + gr.findQuery)
		if err != nil {
			return
		}
	}

	for filePath := range files {
		note := gr.vault.GetNote(filePath)
		if note == nil {
			continue
		}

		var newContent string
		if gr.regexMode {
			newContent = re.ReplaceAllString(note.Content, gr.replaceText)
		} else {
			newContent = caseInsensitiveReplaceAll(note.Content, gr.findQuery, gr.replaceText)
		}
		if newContent != note.Content {
			gr.writeFile(filePath, newContent)
		}
	}

	gr.search()
}

// writeFile persists changes to disk and updates the vault note.
func (gr *GlobalReplace) writeFile(relPath, content string) {
	if gr.vault == nil {
		return
	}
	absPath := filepath.Join(gr.vault.Root, relPath)
	if err := os.WriteFile(absPath, []byte(content), 0644); err != nil {
		return
	}

	// Update in-memory note
	if note, ok := gr.vault.Notes[relPath]; ok {
		note.Content = content
	}

	gr.modified[relPath] = true
}

func (gr *GlobalReplace) highlightMatch(line string, maxWidth int) string {
	if maxWidth < 10 {
		maxWidth = 10
	}

	display := line
	if len(display) > maxWidth {
		display = display[:maxWidth-3] + "..."
	}

	if gr.findQuery == "" {
		return NormalItemStyle.Render(display)
	}

	if gr.regexMode {
		return gr.highlightMatchRegex(display)
	}

	lowerDisplay := strings.ToLower(display)
	lowerQuery := strings.ToLower(gr.findQuery)

	idx := strings.Index(lowerDisplay, lowerQuery)
	if idx < 0 {
		return NormalItemStyle.Render(display)
	}

	before := display[:idx]
	match := display[idx : idx+len(gr.findQuery)]
	after := display[idx+len(gr.findQuery):]

	highlighted := MatchHighlightStyle.Render(match)
	return NormalItemStyle.Render(before) + highlighted + NormalItemStyle.Render(after)
}

// highlightMatchRegex highlights regex matches in the display line.
func (gr *GlobalReplace) highlightMatchRegex(display string) string {
	re, err := regexp.Compile("(?i)" + gr.findQuery)
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

func (gr *GlobalReplace) previewReplace(line string, maxWidth int) string {
	if maxWidth < 10 {
		maxWidth = 10
	}

	var replaced string
	if gr.regexMode {
		re, err := regexp.Compile("(?i)" + gr.findQuery)
		if err != nil {
			return line
		}
		// Preview: replace first match only
		loc := re.FindStringIndex(line)
		if loc == nil {
			return line
		}
		replaced = line[:loc[0]] + re.ReplaceAllString(line[loc[0]:loc[1]], gr.replaceText) + line[loc[1]:]
	} else {
		replaced = caseInsensitiveReplaceFirst(line, gr.findQuery, gr.replaceText)
	}
	if len(replaced) > maxWidth {
		replaced = replaced[:maxWidth-3] + "..."
	}
	return replaced
}

func (gr *GlobalReplace) countFiles() int {
	files := make(map[string]bool)
	for _, m := range gr.matches {
		files[m.FilePath] = true
	}
	return len(files)
}

func (gr GlobalReplace) visibleHeight() int {
	h := gr.height - 18
	if h < 5 {
		h = 5
	}
	return h
}

// caseInsensitiveReplaceFirst replaces the first case-insensitive occurrence.
func caseInsensitiveReplaceFirst(s, old, new string) string {
	if old == "" {
		return s
	}
	lowerS := strings.ToLower(s)
	lowerOld := strings.ToLower(old)
	idx := strings.Index(lowerS, lowerOld)
	if idx < 0 {
		return s
	}
	return s[:idx] + new + s[idx+len(old):]
}

// caseInsensitiveReplaceAll replaces all case-insensitive occurrences.
func caseInsensitiveReplaceAll(s, old, new string) string {
	if old == "" {
		return s
	}
	lowerOld := strings.ToLower(old)
	var result strings.Builder
	remaining := s

	for {
		lowerRemaining := strings.ToLower(remaining)
		idx := strings.Index(lowerRemaining, lowerOld)
		if idx < 0 {
			result.WriteString(remaining)
			break
		}
		result.WriteString(remaining[:idx])
		result.WriteString(new)
		remaining = remaining[idx+len(old):]
	}

	return result.String()
}
