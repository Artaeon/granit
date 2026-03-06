package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Editor struct {
	content   []string
	cursor    int
	col       int
	scroll    int
	focused   bool
	height    int
	width     int
	filePath  string
	modified  bool
	wordCount int
}

func NewEditor() Editor {
	return Editor{
		content: []string{""},
	}
}

func (e *Editor) SetSize(width, height int) {
	e.width = width
	e.height = height
}

func (e *Editor) LoadContent(content string, filePath string) {
	e.content = strings.Split(content, "\n")
	if len(e.content) == 0 {
		e.content = []string{""}
	}
	e.filePath = filePath
	e.cursor = 0
	e.col = 0
	e.scroll = 0
	e.modified = false
	e.countWords()
}

func (e *Editor) GetContent() string {
	return strings.Join(e.content, "\n")
}

func (e *Editor) IsModified() bool {
	return e.modified
}

func (e *Editor) GetCursor() (int, int) {
	return e.cursor, e.col
}

func (e *Editor) GetWordCount() int {
	return e.wordCount
}

func (e *Editor) countWords() {
	total := 0
	for _, line := range e.content {
		words := strings.Fields(line)
		total += len(words)
	}
	e.wordCount = total
}

func (e Editor) Update(msg tea.Msg) (Editor, tea.Cmd) {
	if !e.focused {
		return e, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up":
			if e.cursor > 0 {
				e.cursor--
				if e.col > len(e.content[e.cursor]) {
					e.col = len(e.content[e.cursor])
				}
				if e.cursor < e.scroll {
					e.scroll = e.cursor
				}
			}
		case "down":
			if e.cursor < len(e.content)-1 {
				e.cursor++
				if e.col > len(e.content[e.cursor]) {
					e.col = len(e.content[e.cursor])
				}
				visibleHeight := e.height - 4
				if e.cursor >= e.scroll+visibleHeight {
					e.scroll = e.cursor - visibleHeight + 1
				}
			}
		case "left":
			if e.col > 0 {
				e.col--
			} else if e.cursor > 0 {
				e.cursor--
				e.col = len(e.content[e.cursor])
			}
		case "right":
			if e.col < len(e.content[e.cursor]) {
				e.col++
			} else if e.cursor < len(e.content)-1 {
				e.cursor++
				e.col = 0
			}
		case "home", "ctrl+a":
			e.col = 0
		case "end", "ctrl+e":
			e.col = len(e.content[e.cursor])
		case "pgup":
			visH := e.height - 4
			if visH < 1 {
				visH = 1
			}
			e.cursor -= visH
			if e.cursor < 0 {
				e.cursor = 0
			}
			e.scroll -= visH
			if e.scroll < 0 {
				e.scroll = 0
			}
			if e.col > len(e.content[e.cursor]) {
				e.col = len(e.content[e.cursor])
			}
		case "pgdown":
			visH := e.height - 4
			if visH < 1 {
				visH = 1
			}
			e.cursor += visH
			if e.cursor >= len(e.content) {
				e.cursor = len(e.content) - 1
			}
			e.scroll += visH
			maxScroll := len(e.content) - visH
			if maxScroll < 0 {
				maxScroll = 0
			}
			if e.scroll > maxScroll {
				e.scroll = maxScroll
			}
			if e.col > len(e.content[e.cursor]) {
				e.col = len(e.content[e.cursor])
			}
		case "enter":
			line := e.content[e.cursor]
			before := line[:e.col]
			after := line[e.col:]
			e.content[e.cursor] = before
			newContent := make([]string, 0, len(e.content)+1)
			newContent = append(newContent, e.content[:e.cursor+1]...)
			newContent = append(newContent, after)
			newContent = append(newContent, e.content[e.cursor+1:]...)
			e.content = newContent
			e.cursor++
			e.col = 0
			e.modified = true
			e.countWords()
		case "backspace":
			if e.col > 0 {
				line := e.content[e.cursor]
				e.content[e.cursor] = line[:e.col-1] + line[e.col:]
				e.col--
				e.modified = true
				e.countWords()
			} else if e.cursor > 0 {
				prevLen := len(e.content[e.cursor-1])
				e.content[e.cursor-1] += e.content[e.cursor]
				e.content = append(e.content[:e.cursor], e.content[e.cursor+1:]...)
				e.cursor--
				e.col = prevLen
				e.modified = true
				e.countWords()
			}
		case "delete", "ctrl+d":
			line := e.content[e.cursor]
			if e.col < len(line) {
				e.content[e.cursor] = line[:e.col] + line[e.col+1:]
				e.modified = true
				e.countWords()
			} else if e.cursor < len(e.content)-1 {
				e.content[e.cursor] += e.content[e.cursor+1]
				e.content = append(e.content[:e.cursor+1], e.content[e.cursor+2:]...)
				e.modified = true
				e.countWords()
			}
		case "ctrl+k":
			// Kill to end of line
			line := e.content[e.cursor]
			if e.col < len(line) {
				e.content[e.cursor] = line[:e.col]
				e.modified = true
				e.countWords()
			}
		case "tab":
			// Insert tab as spaces
			line := e.content[e.cursor]
			e.content[e.cursor] = line[:e.col] + "    " + line[e.col:]
			e.col += 4
			e.modified = true
		default:
			char := msg.String()
			if len(char) == 1 && char[0] >= 32 {
				line := e.content[e.cursor]
				e.content[e.cursor] = line[:e.col] + char + line[e.col:]
				e.col++
				e.modified = true
				e.countWords()
			}
		}
	}
	return e, nil
}

func (e Editor) View() string {
	var b strings.Builder
	contentWidth := e.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Header
	headerIcon := " "
	if e.modified {
		headerIcon = " "
	}
	headerText := headerIcon + " " + e.filePath
	if e.modified {
		headerText += "  " + lipgloss.NewStyle().Foreground(yellow).Render("[modified]")
	}
	b.WriteString(HeaderStyle.Render(headerText))
	b.WriteString("\n")

	// Separator
	b.WriteString(DimStyle.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	visibleHeight := e.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	end := e.scroll + visibleHeight
	if end > len(e.content) {
		end = len(e.content)
	}

	// Detect frontmatter region
	fmStart, fmEnd := -1, -1
	if len(e.content) > 0 && strings.TrimSpace(e.content[0]) == "---" {
		fmStart = 0
		for j := 1; j < len(e.content); j++ {
			if strings.TrimSpace(e.content[j]) == "---" {
				fmEnd = j
				break
			}
		}
	}

	// Detect code block regions
	inCodeBlock := false
	codeBlockLines := make(map[int]bool)
	for j := 0; j < len(e.content); j++ {
		trimmed := strings.TrimSpace(e.content[j])
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			codeBlockLines[j] = true
		} else if inCodeBlock {
			codeBlockLines[j] = true
		}
	}

	for i := e.scroll; i < end; i++ {
		line := e.content[i]
		isActiveLine := (i == e.cursor && e.focused)

		// Line number
		lineNum := fmt.Sprintf("%4d ", i+1)
		if isActiveLine {
			b.WriteString(ActiveLineNumStyle.Render(lineNum))
			b.WriteString(" ")
		} else {
			b.WriteString(LineNumStyle.Render(lineNum))
			b.WriteString(" ")
		}

		// Determine max display width
		maxWidth := contentWidth - 7
		if maxWidth < 5 {
			maxWidth = 5
		}

		displayLine := line
		if len(displayLine) > maxWidth {
			displayLine = displayLine[:maxWidth]
		}

		if isActiveLine {
			// Render with cursor
			if e.col <= len(displayLine) {
				before := displayLine[:e.col]
				cursorChar := " "
				if e.col < len(displayLine) {
					cursorChar = string(displayLine[e.col])
				}
				after := ""
				if e.col+1 < len(displayLine) {
					after = displayLine[e.col+1:]
				}
				styledBefore := highlightLine(before, i, fmStart, fmEnd, codeBlockLines)
				styledAfter := highlightLine(after, i, fmStart, fmEnd, codeBlockLines)
				b.WriteString(styledBefore + CursorStyle.Render(cursorChar) + styledAfter)
			} else {
				b.WriteString(highlightLine(displayLine, i, fmStart, fmEnd, codeBlockLines) + CursorStyle.Render(" "))
			}
		} else {
			b.WriteString(highlightLine(displayLine, i, fmStart, fmEnd, codeBlockLines))
		}

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Bottom info
	if len(e.content) > visibleHeight {
		b.WriteString("\n")
		pct := float64(e.scroll) / float64(maxInt(1, len(e.content)-visibleHeight)) * 100
		b.WriteString(DimStyle.Render(fmt.Sprintf("  %d lines  %.0f%%", len(e.content), pct)))
	}

	return b.String()
}

func highlightLine(line string, lineIdx int, fmStart, fmEnd int, codeBlocks map[int]bool) string {
	if line == "" {
		return ""
	}

	// Frontmatter
	if fmStart >= 0 && fmEnd >= 0 && lineIdx >= fmStart && lineIdx <= fmEnd {
		return FrontmatterStyle.Render(line)
	}

	// Code blocks
	if codeBlocks[lineIdx] {
		return CodeBlockStyle.Render(line)
	}

	trimmed := strings.TrimSpace(line)

	// Headings
	if strings.HasPrefix(trimmed, "# ") {
		return TitleStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "## ") {
		return H2Style.Render(line)
	}
	if strings.HasPrefix(trimmed, "### ") || strings.HasPrefix(trimmed, "#### ") ||
		strings.HasPrefix(trimmed, "##### ") || strings.HasPrefix(trimmed, "###### ") {
		return H3Style.Render(line)
	}

	// Horizontal rule
	if trimmed == "---" || trimmed == "***" || trimmed == "___" {
		return DimStyle.Render(line)
	}

	// Blockquote
	if strings.HasPrefix(trimmed, "> ") {
		return BlockquoteStyle.Render(line)
	}

	// Checkboxes
	if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
		return CheckboxDone.Render(line)
	}
	if strings.HasPrefix(trimmed, "- [ ] ") {
		return CheckboxTodo.Render(line)
	}

	// List items
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		marker := string(trimmed[0]) + " "
		rest := trimmed[2:]
		indent := line[:len(line)-len(trimmed)]
		return indent + ListMarkerStyle.Render(marker) + highlightInline(rest)
	}

	// Numbered list
	for i, ch := range trimmed {
		if ch == '.' && i > 0 && i < 4 {
			if i+1 < len(trimmed) && trimmed[i+1] == ' ' {
				allDigits := true
				for j := 0; j < i; j++ {
					if trimmed[j] < '0' || trimmed[j] > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					indent := line[:len(line)-len(trimmed)]
					marker := trimmed[:i+2]
					rest := trimmed[i+2:]
					return indent + ListMarkerStyle.Render(marker) + highlightInline(rest)
				}
			}
			break
		}
		if ch < '0' || ch > '9' {
			break
		}
	}

	return highlightInline(line)
}

func highlightInline(line string) string {
	if line == "" {
		return ""
	}

	var result strings.Builder
	i := 0
	runes := []rune(line)
	n := len(runes)

	for i < n {
		// WikiLinks [[...]]
		if i+1 < n && runes[i] == '[' && runes[i+1] == '[' {
			end := findClosing(runes, i+2, ']', ']')
			if end != -1 {
				result.WriteString(LinkStyle.Render(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}

		// Inline code `...`
		if runes[i] == '`' {
			end := findSingle(runes, i+1, '`')
			if end != -1 {
				result.WriteString(CodeStyle.Render(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}

		// Bold **...**
		if i+1 < n && runes[i] == '*' && runes[i+1] == '*' {
			end := findDouble(runes, i+2, '*')
			if end != -1 {
				result.WriteString(BoldTextStyle.Render(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}

		// Italic *...*
		if runes[i] == '*' && (i+1 < n && runes[i+1] != '*') {
			end := findSingle(runes, i+1, '*')
			if end != -1 && end > i+1 {
				result.WriteString(ItalicTextStyle.Render(string(runes[i : end+1])))
				i = end + 1
				continue
			}
		}

		// Tags #tag
		if runes[i] == '#' && (i == 0 || runes[i-1] == ' ') {
			// But not headings (already handled)
			end := i + 1
			for end < n && runes[end] != ' ' && runes[end] != '\t' {
				end++
			}
			if end > i+1 {
				result.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA")).Render(string(runes[i:end])))
				i = end
				continue
			}
		}

		result.WriteRune(runes[i])
		i++
	}

	return result.String()
}

func findClosing(runes []rune, start int, c1, c2 rune) int {
	for i := start; i+1 < len(runes); i++ {
		if runes[i] == c1 && runes[i+1] == c2 {
			return i + 1
		}
	}
	return -1
}

func findSingle(runes []rune, start int, ch rune) int {
	for i := start; i < len(runes); i++ {
		if runes[i] == ch {
			return i
		}
	}
	return -1
}

func findDouble(runes []rune, start int, ch rune) int {
	for i := start; i+1 < len(runes); i++ {
		if runes[i] == ch && runes[i+1] == ch {
			return i + 1
		}
	}
	return -1
}
