package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type OutlineItem struct {
	level int
	text  string
	line  int
}

type Outline struct {
	active  bool
	items   []OutlineItem
	cursor  int
	scroll  int
	width   int
	height  int
	result  int // line number to jump to, -1 if none
}

func NewOutline() Outline {
	return Outline{result: -1}
}

func (o *Outline) SetSize(width, height int) {
	o.width = width
	o.height = height
}

func (o *Outline) Open(content string) {
	o.active = true
	o.cursor = 0
	o.scroll = 0
	o.result = -1
	o.parseHeadings(content)
}

func (o *Outline) Close() {
	o.active = false
}

func (o *Outline) IsActive() bool {
	return o.active
}

func (o *Outline) JumpToLine() int {
	r := o.result
	o.result = -1
	return r
}

func (o *Outline) parseHeadings(content string) {
	o.items = nil
	lines := strings.Split(content, "\n")
	inCodeBlock := false

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		level := 0
		for _, ch := range trimmed {
			if ch == '#' {
				level++
			} else {
				break
			}
		}
		if level >= 1 && level <= 6 && len(trimmed) > level && trimmed[level] == ' ' {
			text := strings.TrimSpace(trimmed[level+1:])
			o.items = append(o.items, OutlineItem{
				level: level,
				text:  text,
				line:  i,
			})
		}
	}
}

func (o Outline) Update(msg tea.Msg) (Outline, tea.Cmd) {
	if !o.active {
		return o, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+o":
			o.active = false
		case "up", "k":
			if o.cursor > 0 {
				o.cursor--
				if o.cursor < o.scroll {
					o.scroll = o.cursor
				}
			}
		case "down", "j":
			if o.cursor < len(o.items)-1 {
				o.cursor++
				visH := o.height - 8
				if visH < 1 {
					visH = 1
				}
				if o.cursor >= o.scroll+visH {
					o.scroll = o.cursor - visH + 1
				}
			}
		case "enter":
			if len(o.items) > 0 && o.cursor < len(o.items) {
				o.result = o.items[o.cursor].line
				o.active = false
			}
		}
	}
	return o, nil
}

func (o Outline) View() string {
	width := o.width / 2
	if width < 45 {
		width = 45
	}
	if width > 70 {
		width = 70
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Outline")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n\n")

	if len(o.items) == 0 {
		b.WriteString(DimStyle.Render("  No headings found"))
		b.WriteString("\n")
	} else {
		visH := o.height - 8
		if visH < 5 {
			visH = 5
		}
		end := o.scroll + visH
		if end > len(o.items) {
			end = len(o.items)
		}

		headingStyles := []lipgloss.Style{
			lipgloss.NewStyle().Foreground(mauve).Bold(true),   // h1
			lipgloss.NewStyle().Foreground(blue).Bold(true),    // h2
			lipgloss.NewStyle().Foreground(sapphire).Bold(true),// h3
			lipgloss.NewStyle().Foreground(teal),               // h4
			lipgloss.NewStyle().Foreground(green),              // h5
			lipgloss.NewStyle().Foreground(yellow),             // h6
		}

		for i := o.scroll; i < end; i++ {
			item := o.items[i]
			indent := strings.Repeat("  ", item.level-1)

			styleIdx := item.level - 1
			if styleIdx >= len(headingStyles) {
				styleIdx = len(headingStyles) - 1
			}

			prefix := "# "
			if item.level == 2 {
				prefix = "## "
			} else if item.level >= 3 {
				prefix = strings.Repeat("#", item.level) + " "
			}

			lineNum := DimStyle.Render("L" + smallNum(item.line+1) + " ")

			text := item.text
			maxLen := width - 12 - len(indent)
			if maxLen < 10 {
				maxLen = 10
			}
			if len(text) > maxLen {
				text = text[:maxLen-3] + "..."
			}

			content := indent + headingStyles[styleIdx].Render(prefix+text)

			if i == o.cursor {
				line := "  " + lineNum + content
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString("  " + lineNum + content)
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Enter: jump to heading  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
