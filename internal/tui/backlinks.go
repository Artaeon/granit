package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Backlinks struct {
	incoming []string
	outgoing []string
	cursor   int
	focused  bool
	height   int
	width    int
	mode     int // 0=incoming, 1=outgoing
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
			}
		case "down", "j":
			if bl.cursor < len(items)-1 {
				bl.cursor++
			}
		case "tab":
			bl.mode = (bl.mode + 1) % 2
			bl.cursor = 0
		}
	}
	return bl, nil
}

func (bl Backlinks) View() string {
	var b strings.Builder

	// Tab header
	inTab := "Backlinks"
	outTab := "Outgoing"
	if bl.mode == 0 {
		inTab = SelectedStyle.Render("[Backlinks]")
		outTab = DimStyle.Render(" Outgoing ")
	} else {
		inTab = DimStyle.Render(" Backlinks ")
		outTab = SelectedStyle.Render("[Outgoing]")
	}
	b.WriteString(inTab + " " + outTab)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", bl.width-4)))
	b.WriteString("\n")

	items := bl.currentItems()
	if len(items) == 0 {
		b.WriteString(DimStyle.Render("  (none)"))
		return b.String()
	}

	visibleHeight := bl.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	for i := 0; i < len(items) && i < visibleHeight; i++ {
		name := items[i]
		if len(name) > bl.width-6 {
			name = name[:bl.width-9] + "..."
		}
		if i == bl.cursor && bl.focused {
			b.WriteString(SelectedStyle.Render("▸ " + name))
		} else {
			b.WriteString(LinkStyle.Render("  " + name))
		}
		if i < len(items)-1 && i < visibleHeight-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}
