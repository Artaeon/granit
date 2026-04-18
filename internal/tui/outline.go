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
	OverlayBase
	items  []OutlineItem
	cursor int
	scroll int
	result int // line number to jump to, -1 if none
}

func NewOutline() Outline {
	return Outline{result: -1}
}

func (o *Outline) Open(content string) {
	o.Activate()
	o.cursor = 0
	o.scroll = 0
	o.result = -1
	o.parseHeadings(content)
	if o.cursor >= len(o.items) {
		o.cursor = maxInt(0, len(o.items)-1)
	}
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

// RenderPanel renders the outline as a persistent side panel (no overlay border).
// It parses headings from content and renders them in a compact list.
func (o *Outline) RenderPanel(content string, width, height int) string {
	// Re-parse headings from the current content
	o.parseHeadings(content)
	if o.cursor >= len(o.items) {
		o.cursor = maxInt(0, len(o.items)-1)
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  OUTLINE")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-4)))
	b.WriteString("\n")

	if len(o.items) == 0 {
		b.WriteString(DimStyle.Render("  No headings found"))
		b.WriteString("\n")
	} else {
		visH := height - 4
		if visH < 3 {
			visH = 3
		}
		end := o.scroll + visH
		if end > len(o.items) {
			end = len(o.items)
		}

		headingStyles := []lipgloss.Style{
			lipgloss.NewStyle().Foreground(mauve).Bold(true),    // h1
			lipgloss.NewStyle().Foreground(blue).Bold(true),     // h2
			lipgloss.NewStyle().Foreground(sapphire).Bold(true), // h3
			lipgloss.NewStyle().Foreground(teal),                // h4
			lipgloss.NewStyle().Foreground(green),               // h5
			lipgloss.NewStyle().Foreground(yellow),              // h6
		}

		for i := o.scroll; i < end; i++ {
			item := o.items[i]
			indent := strings.Repeat("  ", item.level-1)

			styleIdx := item.level - 1
			if styleIdx >= len(headingStyles) {
				styleIdx = len(headingStyles) - 1
			}

			prefix := "\u25cf "
			if item.level > 1 {
				prefix = "\u25e6 "
			}

			maxLen := width - 6 - len(indent)*2
			if maxLen < 8 {
				maxLen = 8
			}
			text := TruncateDisplay(item.text, maxLen)

			styled := headingStyles[styleIdx].Render(prefix + text)

			if i == o.cursor {
				line := "  " + indent + styled
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(width - 4).
					Render(line))
			} else {
				b.WriteString("  " + indent + styled)
			}
			b.WriteString("\n")
		}
	}

	return b.String()
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

			maxLen := width - 12 - len(indent)
			if maxLen < 10 {
				maxLen = 10
			}
			text := TruncateDisplay(item.text, maxLen)

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
	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"Enter", "jump"}, {"j/k", "nav"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
