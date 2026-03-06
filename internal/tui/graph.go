package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

type GraphView struct {
	active      bool
	vault       *vault.Vault
	index       *vault.Index
	nodes       []graphNode
	cursor      int
	scroll      int
	width       int
	height      int
	centerNote  string
	selected    string // the note the user selected to navigate to
}

type graphNode struct {
	name       string
	path       string
	incoming   int
	outgoing   int
	total      int
}

func NewGraphView(v *vault.Vault, idx *vault.Index) GraphView {
	return GraphView{
		vault: v,
		index: idx,
	}
}

func (g *GraphView) SetSize(width, height int) {
	g.width = width
	g.height = height
}

func (g *GraphView) Open(centerNote string) {
	g.active = true
	g.centerNote = centerNote
	g.cursor = 0
	g.scroll = 0
	g.selected = ""
	g.buildGraph()
}

func (g *GraphView) Close() {
	g.active = false
}

func (g *GraphView) IsActive() bool {
	return g.active
}

func (g *GraphView) SelectedNote() string {
	s := g.selected
	g.selected = ""
	return s
}

func (g *GraphView) buildGraph() {
	g.nodes = nil

	// Build nodes with connection counts
	for _, path := range g.vault.SortedPaths() {
		note := g.vault.GetNote(path)
		if note == nil {
			continue
		}
		incoming := len(g.index.GetBacklinks(path))
		outgoing := len(note.Links)

		g.nodes = append(g.nodes, graphNode{
			name:     strings.TrimSuffix(path, ".md"),
			path:     path,
			incoming: incoming,
			outgoing: outgoing,
			total:    incoming + outgoing,
		})
	}

	// Sort by total connections (most connected first)
	sort.Slice(g.nodes, func(i, j int) bool {
		return g.nodes[i].total > g.nodes[j].total
	})
}

func (g GraphView) Update(msg tea.Msg) (GraphView, tea.Cmd) {
	if !g.active {
		return g, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+g":
			g.active = false
			return g, nil
		case "up", "k":
			if g.cursor > 0 {
				g.cursor--
				if g.cursor < g.scroll {
					g.scroll = g.cursor
				}
			}
		case "down", "j":
			if g.cursor < len(g.nodes)-1 {
				g.cursor++
				visH := g.height - 12
				if visH < 1 {
					visH = 1
				}
				if g.cursor >= g.scroll+visH {
					g.scroll = g.cursor - visH + 1
				}
			}
		case "enter":
			if len(g.nodes) > 0 && g.cursor < len(g.nodes) {
				g.selected = g.nodes[g.cursor].path
				g.active = false
			}
			return g, nil
		}
	}
	return g, nil
}

func (g GraphView) View() string {
	width := g.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Graph View — Note Connections")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n\n")

	// Legend
	legend := lipgloss.NewStyle().Foreground(overlay0).Render(
		"  ← backlinks  → outgoing  ━ connection strength")
	b.WriteString(legend)
	b.WriteString("\n\n")

	if len(g.nodes) == 0 {
		b.WriteString(DimStyle.Render("  No notes found"))
	} else {
		visH := g.height - 12
		if visH < 1 {
			visH = 1
		}
		end := g.scroll + visH
		if end > len(g.nodes) {
			end = len(g.nodes)
		}

		// Find max connections for scaling
		maxConn := 1
		for _, node := range g.nodes {
			if node.total > maxConn {
				maxConn = node.total
			}
		}

		barWidth := width - 40
		if barWidth < 10 {
			barWidth = 10
		}

		for i := g.scroll; i < end; i++ {
			node := g.nodes[i]
			isSelected := i == g.cursor

			// Name
			name := node.name
			maxNameLen := 25
			if len(name) > maxNameLen {
				name = name[:maxNameLen-3] + "..."
			}
			namePad := maxNameLen - len(name)
			if namePad < 0 {
				namePad = 0
			}

			// Connection bar
			barLen := 0
			if maxConn > 0 {
				barLen = node.total * barWidth / maxConn
			}
			if barLen < 1 && node.total > 0 {
				barLen = 1
			}

			inBar := node.incoming * barLen / maxInt(1, node.total)
			outBar := barLen - inBar

			inBarStr := lipgloss.NewStyle().Foreground(blue).Render(strings.Repeat("━", inBar))
			outBarStr := lipgloss.NewStyle().Foreground(peach).Render(strings.Repeat("━", outBar))
			emptyBar := DimStyle.Render(strings.Repeat("─", barWidth-barLen))

			// Stats
			stats := lipgloss.NewStyle().Foreground(overlay0).
				Render(" ←" + smallNum(node.incoming) + " →" + smallNum(node.outgoing))

			isCurrent := node.path == g.centerNote
			icon := "  "
			if isCurrent {
				icon = lipgloss.NewStyle().Foreground(green).Render("● ")
			}

			line := icon + name + strings.Repeat(" ", namePad) + " " + inBarStr + outBarStr + emptyBar + stats

			if isSelected {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString(NormalItemStyle.Render(line))
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  " + smallNum(len(g.nodes)) + " notes  Enter: navigate  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func smallNum(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
