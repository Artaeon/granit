package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Backlinks struct {
	incoming []string
	outgoing []string
	cursor   int
	focused  bool
	height   int
	width    int
	mode     int // 0=incoming, 1=outgoing
	scroll   int
}

func NewBacklinks() Backlinks {
	return Backlinks{}
}

func (bl *Backlinks) SetSize(width, height int) {
	bl.width = width
	bl.height = height
}

func (bl *Backlinks) SetLinks(incoming, outgoing []string) {
	bl.incoming = incoming
	bl.outgoing = outgoing
	bl.cursor = 0
	bl.scroll = 0
}

func (bl *Backlinks) Selected() string {
	items := bl.currentItems()
	if len(items) == 0 || bl.cursor >= len(items) {
		return ""
	}
	return items[bl.cursor]
}

func (bl *Backlinks) currentItems() []string {
	if bl.mode == 0 {
		return bl.incoming
	}
	return bl.outgoing
}

func (bl Backlinks) Update(msg tea.Msg) (Backlinks, tea.Cmd) {
	if !bl.focused {
		return bl, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		items := bl.currentItems()
		switch msg.String() {
		case "up", "k":
			if bl.cursor > 0 {
				bl.cursor--
				if bl.cursor < bl.scroll {
					bl.scroll = bl.cursor
				}
			}
		case "down", "j":
			if bl.cursor < len(items)-1 {
				bl.cursor++
				visibleHeight := bl.height - 8
				if visibleHeight < 1 {
					visibleHeight = 1
				}
				if bl.cursor >= bl.scroll+visibleHeight {
					bl.scroll = bl.cursor - visibleHeight + 1
				}
			}
		case "tab":
			bl.mode = (bl.mode + 1) % 2
			bl.cursor = 0
			bl.scroll = 0
		}
	}
	return bl, nil
}

func (bl Backlinks) View() string {
	var b strings.Builder
	contentWidth := bl.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Tab header with pill style
	inCount := len(bl.incoming)
	outCount := len(bl.outgoing)

	activeTabStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#1E1E2E")).
		Background(lipgloss.Color("#CBA6F7")).
		Bold(true).
		Padding(0, 1)

	inactiveTabStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#6C7086")).
		Background(lipgloss.Color("#313244")).
		Padding(0, 1)

	var inTab, outTab string
	if bl.mode == 0 {
		inTab = activeTabStyle.Render(formatTabLabel("Backlinks", inCount))
		outTab = inactiveTabStyle.Render(formatTabLabel("Outgoing", outCount))
	} else {
		inTab = inactiveTabStyle.Render(formatTabLabel("Backlinks", inCount))
		outTab = activeTabStyle.Render(formatTabLabel("Outgoing", outCount))
	}

	b.WriteString(inTab + " " + outTab)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	items := bl.currentItems()
	if len(items) == 0 {
		b.WriteString("\n")
		emptyIcon := DimStyle.Render("  ")
		emptyText := DimStyle.Render(" No links found")
		b.WriteString(emptyIcon + emptyText)
		return b.String()
	}

	visibleHeight := bl.height - 8
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	end := bl.scroll + visibleHeight
	if end > len(items) {
		end = len(items)
	}

	for i := bl.scroll; i < end; i++ {
		name := items[i]
		displayName := strings.TrimSuffix(name, ".md")

		// Truncate if needed
		maxLen := contentWidth - 6
		if maxLen < 5 {
			maxLen = 5
		}
		if len(displayName) > maxLen {
			displayName = displayName[:maxLen-3] + "..."
		}

		icon := lipgloss.NewStyle().Foreground(lipgloss.Color("#89B4FA")).Render(" ")

		if i == bl.cursor && bl.focused {
			line := "  " + icon + " " + displayName
			padLen := contentWidth - lipgloss.Width(line)
			if padLen < 0 {
				padLen = 0
			}
			highlighted := lipgloss.NewStyle().
				Background(lipgloss.Color("#313244")).
				Foreground(lipgloss.Color("#FAB387")).
				Bold(true).
				Width(contentWidth).
				Render("  " + icon + " " + displayName + strings.Repeat(" ", padLen))
			b.WriteString(highlighted)
		} else {
			b.WriteString("  " + icon + " " + NormalItemStyle.Render(displayName))
		}

		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func formatTabLabel(label string, count int) string {
	if count == 0 {
		return label
	}
	c := string(rune('0' + count%10))
	if count >= 10 {
		c = string(rune('0'+count/10)) + c
	}
	return label + " " + c
}
