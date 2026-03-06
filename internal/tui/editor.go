package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Editor struct {
	content  []string
	cursor   int // line cursor
	col      int // column cursor
	scroll   int
	focused  bool
	height   int
	width    int
	filePath string
	modified bool
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
}

func (e *Editor) GetContent() string {
	return strings.Join(e.content, "\n")
}

func (e *Editor) IsModified() bool {
	return e.modified
}

func (e Editor) Update(msg tea.Msg) (Editor, tea.Cmd) {
	if !e.focused {
		return e, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "ctrl+p":
			if e.cursor > 0 {
				e.cursor--
				if e.col > len(e.content[e.cursor]) {
					e.col = len(e.content[e.cursor])
				}
				if e.cursor < e.scroll {
					e.scroll = e.cursor
				}
			}
		case "down", "ctrl+n":
			if e.cursor < len(e.content)-1 {
				e.cursor++
				if e.col > len(e.content[e.cursor]) {
					e.col = len(e.content[e.cursor])
				}
				visibleHeight := e.height - 3
				if e.cursor >= e.scroll+visibleHeight {
					e.scroll = e.cursor - visibleHeight + 1
				}
			}
		case "left":
			if e.col > 0 {
				e.col--
			}
		case "right":
			if e.col < len(e.content[e.cursor]) {
				e.col++
			}
		case "home", "ctrl+a":
			e.col = 0
		case "end", "ctrl+e":
			e.col = len(e.content[e.cursor])
		case "enter":
			// Split line at cursor
			line := e.content[e.cursor]
			before := line[:e.col]
			after := line[e.col:]
			e.content[e.cursor] = before
			newContent := make([]string, len(e.content)+1)
			copy(newContent, e.content[:e.cursor+1])
			newContent[e.cursor+1] = after
			copy(newContent[e.cursor+2:], e.content[e.cursor+1:])
			e.content = newContent
			e.cursor++
			e.col = 0
			e.modified = true
		case "backspace":
			if e.col > 0 {
				line := e.content[e.cursor]
				e.content[e.cursor] = line[:e.col-1] + line[e.col:]
				e.col--
				e.modified = true
			} else if e.cursor > 0 {
				// Merge with previous line
				prevLen := len(e.content[e.cursor-1])
				e.content[e.cursor-1] += e.content[e.cursor]
				e.content = append(e.content[:e.cursor], e.content[e.cursor+1:]...)
				e.cursor--
				e.col = prevLen
				e.modified = true
			}
		default:
			char := msg.String()
			if len(char) == 1 && char >= " " {
				line := e.content[e.cursor]
				e.content[e.cursor] = line[:e.col] + char + line[e.col:]
				e.col++
				e.modified = true
			}
		}
	}
	return e, nil
}

func (e Editor) View() string {
	var b strings.Builder

	header := e.filePath
	if e.modified {
		header += " [modified]"
	}
	b.WriteString(HeaderStyle.Render(header))
	b.WriteString("\n")

	visibleHeight := e.height - 3
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	end := e.scroll + visibleHeight
	if end > len(e.content) {
		end = len(e.content)
	}

	for i := e.scroll; i < end; i++ {
		line := e.content[i]
		lineNum := DimStyle.Render(padLeft(i+1, 4) + " ")

		if len(line) > e.width-10 {
			line = line[:e.width-10]
		}

		if i == e.cursor && e.focused {
			// Show cursor position
			if e.col <= len(line) {
				before := line[:e.col]
				cursor := " "
				if e.col < len(line) {
					cursor = string(line[e.col])
				}
				after := ""
				if e.col+1 < len(line) {
					after = line[e.col+1:]
				}
				b.WriteString(lineNum + before + lipglossReverse(cursor) + after)
			} else {
				b.WriteString(lineNum + line + lipglossReverse(" "))
			}
		} else {
			// Apply basic markdown highlighting
			b.WriteString(lineNum + highlightMarkdown(line))
		}

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func padLeft(n int, width int) string {
	s := strings.Repeat(" ", width)
	ns := string(rune('0'+n%10))
	if n >= 10 {
		ns = string(rune('0'+n/10%10)) + ns
	}
	if n >= 100 {
		ns = string(rune('0'+n/100%10)) + ns
	}
	if n >= 1000 {
		ns = string(rune('0'+n/1000%10)) + ns
	}
	result := s + ns
	return result[len(result)-width:]
}

func lipglossReverse(s string) string {
	return "\033[7m" + s + "\033[0m"
}

func highlightMarkdown(line string) string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "# ") {
		return TitleStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
		return TitleStyle.Render(line)
	}
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		return line
	}
	// Highlight [[wikilinks]]
	result := line
	if strings.Contains(result, "[[") {
		parts := strings.Split(result, "[[")
		var b strings.Builder
		b.WriteString(parts[0])
		for _, part := range parts[1:] {
			endIdx := strings.Index(part, "]]")
			if endIdx != -1 {
				b.WriteString(LinkStyle.Render("[[" + part[:endIdx] + "]]"))
				b.WriteString(part[endIdx+2:])
			} else {
				b.WriteString("[[" + part)
			}
		}
		result = b.String()
	}
	return result
}
