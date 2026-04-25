package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

type GraphView struct {
	OverlayBase
	vault      *vault.Vault
	index      *vault.Index
	nodes      []graphNode
	cursor     int
	scroll     int
	centerNote string
	selected   string // the note the user selected to navigate to
	localMode  bool   // true = local graph (1-2 hops), false = global graph
	depth      int    // hop depth for local mode: 1 or 2
}

type graphNode struct {
	name     string
	path     string
	incoming int
	outgoing int
	total    int
	hopDist  int // distance from center note (0=center, 1=direct, 2=second-degree)
}

func NewGraphView(v *vault.Vault, idx *vault.Index) GraphView {
	return GraphView{
		vault: v,
		index: idx,
		depth: 1,
	}
}

func (g *GraphView) Open(centerNote string) {
	g.Activate()
	g.centerNote = centerNote
	g.cursor = 0
	g.scroll = 0
	g.selected = ""
	g.buildGraph()
}

func (g *GraphView) SelectedNote() string {
	s := g.selected
	g.selected = ""
	return s
}

// SetCurrentNote sets the center note for local graph mode.
func (g *GraphView) SetCurrentNote(name string) {
	g.centerNote = name
}

func (g *GraphView) buildGraph() {
	if g.localMode && g.centerNote != "" {
		g.buildLocalGraph()
	} else {
		g.buildGlobalGraph()
	}
}

func (g *GraphView) buildGlobalGraph() {
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
			hopDist:  -1,
		})
	}

	// Sort by total connections (most connected first)
	sort.Slice(g.nodes, func(i, j int) bool {
		return g.nodes[i].total > g.nodes[j].total
	})
}

func (g *GraphView) buildLocalGraph() {
	g.nodes = nil

	// Validate that centerNote still exists in the vault
	if g.centerNote == "" || g.vault.GetNote(g.centerNote) == nil {
		// Center note was deleted or is empty; fall back to global mode
		g.localMode = false
		g.buildGlobalGraph()
		return
	}

	// Collect nodes within g.depth hops of centerNote
	// hopMap: path -> minimum hop distance from center
	hopMap := make(map[string]int)
	hopMap[g.centerNote] = 0

	// BFS from center note
	frontier := []string{g.centerNote}
	for hop := 1; hop <= g.depth; hop++ {
		var nextFrontier []string
		for _, path := range frontier {
			neighbors := g.getNeighbors(path)
			for _, nb := range neighbors {
				if _, seen := hopMap[nb]; !seen {
					hopMap[nb] = hop
					nextFrontier = append(nextFrontier, nb)
				}
			}
		}
		frontier = nextFrontier
	}

	// Build node list from collected paths
	for path, dist := range hopMap {
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
			hopDist:  dist,
		})
	}

	// Sort: center first, then by hop distance, then by total connections
	sort.Slice(g.nodes, func(i, j int) bool {
		if g.nodes[i].hopDist != g.nodes[j].hopDist {
			return g.nodes[i].hopDist < g.nodes[j].hopDist
		}
		return g.nodes[i].total > g.nodes[j].total
	})
}

// getNeighbors returns all directly connected note paths (both incoming and outgoing).
func (g *GraphView) getNeighbors(path string) []string {
	seen := make(map[string]bool)
	var result []string

	// Outgoing links: resolve wikilink names to paths
	note := g.vault.GetNote(path)
	if note != nil {
		for _, link := range note.Links {
			resolved := g.index.ResolveLink(link)
			if resolved != "" && !seen[resolved] {
				seen[resolved] = true
				result = append(result, resolved)
			}
		}
	}

	// Incoming links (backlinks)
	for _, bl := range g.index.GetBacklinks(path) {
		if !seen[bl] {
			seen[bl] = true
			result = append(result, bl)
		}
	}

	return result
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
		case "tab":
			// When switching to local mode, validate centerNote exists
			if !g.localMode && (g.centerNote == "" || g.vault.GetNote(g.centerNote) == nil) {
				// Can't switch to local mode without a valid center note
				break
			}
			g.localMode = !g.localMode
			g.cursor = 0
			g.scroll = 0
			g.buildGraph()
			if g.cursor >= len(g.nodes) {
				g.cursor = maxInt(0, len(g.nodes)-1)
			}
		case "1":
			if g.localMode && g.depth != 1 {
				g.depth = 1
				g.cursor = 0
				g.scroll = 0
				g.buildGraph()
				if g.cursor >= len(g.nodes) {
					g.cursor = maxInt(0, len(g.nodes)-1)
				}
			}
		case "2":
			if g.localMode && g.depth != 2 {
				g.depth = 2
				g.cursor = 0
				g.scroll = 0
				g.buildGraph()
				if g.cursor >= len(g.nodes) {
					g.cursor = maxInt(0, len(g.nodes)-1)
				}
			}
		}
	}
	return g, nil
}

func (g GraphView) View() string {
	// Tab mode uses the full editor pane — graph layouts always
	// benefit from horizontal real estate, especially in global
	// mode where dozens of nodes need spreading out.
	var width int
	if g.IsTabMode() {
		width = g.width - 2
		if width < 60 {
			width = 60
		}
	} else {
		width = g.width * 2 / 3
		if width < 60 {
			width = 60
		}
		if width > 100 {
			width = 100
		}
	}

	innerWidth := width - 6

	var b strings.Builder

	// Header with stats and mode indicator
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	modeStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	if g.localMode {
		b.WriteString(titleStyle.Render("  Note Graph"))
		b.WriteString(modeStyle.Render("  Local"))
		depthStyle := lipgloss.NewStyle().Foreground(overlay0)
		b.WriteString(depthStyle.Render(" (" + smallNum(g.depth) + " hop)"))
	} else {
		b.WriteString(titleStyle.Render("  Note Graph"))
		b.WriteString(modeStyle.Render("  Global"))
	}

	// Connection stats
	totalLinks := 0
	orphanCount := 0
	for _, node := range g.nodes {
		totalLinks += node.total
		if node.total == 0 {
			orphanCount++
		}
	}
	statsStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(statsStyle.Render("  " + smallNum(len(g.nodes)) + " notes  " +
		smallNum(totalLinks/2) + " links  " + smallNum(orphanCount) + " orphans"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	// Legend with color indicators
	inStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	outStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	hubStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	orphanStyle := lipgloss.NewStyle().Foreground(red)
	b.WriteString("  " +
		inStyle.Render("━") + statsStyle.Render(" backlinks  ") +
		outStyle.Render("━") + statsStyle.Render(" outgoing  ") +
		hubStyle.Render("*") + statsStyle.Render(" hub  ") +
		orphanStyle.Render("o") + statsStyle.Render(" orphan"))
	b.WriteString("\n\n")

	if len(g.nodes) == 0 {
		if g.localMode {
			b.WriteString(DimStyle.Render("  No connected notes found"))
		} else {
			b.WriteString(DimStyle.Render("  No notes found"))
		}
	} else {
		visH := g.height - 14
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

		barWidth := innerWidth - 42
		if barWidth < 8 {
			barWidth = 8
		}

		for i := g.scroll; i < end; i++ {
			node := g.nodes[i]
			isSelected := i == g.cursor

			// Name with truncation
			name := TruncateDisplay(node.name, 22)
			maxNameLen := 22
			namePad := maxNameLen - lipgloss.Width(name)
			if namePad < 0 {
				namePad = 0
			}

			// Node icon based on connection count and hop distance
			isCurrent := node.path == g.centerNote
			isHub := node.total >= 5
			isOrphan := node.total == 0
			var icon string
			switch {
			case isCurrent:
				icon = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("@ ")
			case g.localMode && node.hopDist == 2:
				icon = lipgloss.NewStyle().Foreground(surface2).Render("~ ")
			case isHub:
				icon = lipgloss.NewStyle().Foreground(green).Bold(true).Render("* ")
			case isOrphan:
				icon = lipgloss.NewStyle().Foreground(red).Render("o ")
			default:
				icon = lipgloss.NewStyle().Foreground(surface2).Render("- ")
			}

			// Connection bar with gradient
			barLen := 0
			if maxConn > 0 {
				barLen = node.total * barWidth / maxConn
			}
			if barLen < 1 && node.total > 0 {
				barLen = 1
			}

			inBar := 0
			outBar := 0
			if node.total > 0 {
				inBar = node.incoming * barLen / node.total
				outBar = barLen - inBar
			}

			inBarStr := lipgloss.NewStyle().Foreground(blue).Render(strings.Repeat("━", inBar))
			outBarStr := lipgloss.NewStyle().Foreground(peach).Render(strings.Repeat("━", outBar))
			emptyBar := lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat("─", barWidth-barLen))

			// Stats with color
			inCount := lipgloss.NewStyle().Foreground(blue).Render(smallNum(node.incoming))
			outCount := lipgloss.NewStyle().Foreground(peach).Render(smallNum(node.outgoing))
			stats := statsStyle.Render(" ") + inCount + statsStyle.Render("<") +
				outCount + statsStyle.Render(">")

			// Name styling based on connection importance and hop distance
			nameColor := text
			if isCurrent {
				nameColor = mauve
			} else if g.localMode && node.hopDist == 2 {
				nameColor = surface2
			} else if isHub {
				nameColor = green
			} else if isOrphan {
				nameColor = surface2
			}

			nameStyled := lipgloss.NewStyle().Foreground(nameColor).Render(name)
			pad := strings.Repeat(" ", namePad)

			line := "  " + icon + nameStyled + pad + " " + inBarStr + outBarStr + emptyBar + stats

			if isSelected {
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Bold(true).
					MaxWidth(innerWidth).
					Render(line))
			} else {
				b.WriteString(line)
			}
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	pairs := []struct{ Key, Desc string }{
		{"j/k", "nav"}, {"Enter", "open"}, {"Tab", "local/global"},
	}
	if g.localMode {
		pairs = append(pairs, struct{ Key, Desc string }{"1/2", "depth"})
	}
	pairs = append(pairs, struct{ Key, Desc string }{"Esc", "close"})
	b.WriteString(RenderHelpBar(pairs))

	if g.IsTabMode() {
		return b.String()
	}
	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width)

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
