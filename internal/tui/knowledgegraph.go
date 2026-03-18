package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// KnowledgeGraph provides AI-powered analysis of the vault's note graph.
// It identifies clusters, hub notes, orphans, and suggests missing connections.
type KnowledgeGraph struct {
	active bool
	width  int
	height int
	scroll int

	// Analysis results
	clusters     []NoteCluster
	hubs         []HubNote
	orphans      []string
	suggestions  []ConnectionSuggestion
	analysisMode int // 0=overview, 1=clusters, 2=hubs, 3=orphans, 4=suggestions

	// Source data
	noteLinks map[string][]string // note → outgoing links
	backlinks map[string][]string // note → incoming links
	allNotes  []string
}

// NoteCluster represents a group of tightly interconnected notes.
type NoteCluster struct {
	Name  string   // auto-generated cluster name (from most common words)
	Notes []string // note paths in this cluster
	Links int      // number of internal links
}

// HubNote is a note with many connections (high degree centrality).
type HubNote struct {
	Path     string
	InDegree int // incoming links
	OutDegree int // outgoing links
	Total    int
}

// BridgeNote connects otherwise disconnected clusters.
type BridgeNote struct {
	Path       string
	Clusters   int // number of clusters it connects
	Betweenness float64
}

// ConnectionSuggestion recommends a link between two notes.
type ConnectionSuggestion struct {
	From   string
	To     string
	Reason string // why they should be connected
	Score  float64
}

func NewKnowledgeGraph() KnowledgeGraph {
	return KnowledgeGraph{}
}

func (kg *KnowledgeGraph) IsActive() bool {
	return kg.active
}

func (kg *KnowledgeGraph) Open() {
	kg.active = true
	kg.scroll = 0
	kg.analysisMode = 0
}

func (kg *KnowledgeGraph) Close() {
	kg.active = false
}

func (kg *KnowledgeGraph) SetSize(w, h int) {
	kg.width = w
	kg.height = h
}

// SetGraphData provides the link structure for analysis.
func (kg *KnowledgeGraph) SetGraphData(allNotes []string, noteLinks, backlinks map[string][]string) {
	kg.allNotes = allNotes
	kg.noteLinks = noteLinks
	kg.backlinks = backlinks
	kg.analyze()
}

// analyze runs all graph analysis algorithms.
func (kg *KnowledgeGraph) analyze() {
	kg.findHubs()
	kg.findOrphans()
	kg.findClusters()
	kg.suggestConnections()
}

func (kg *KnowledgeGraph) findHubs() {
	kg.hubs = nil
	for _, note := range kg.allNotes {
		outgoing := len(kg.noteLinks[note])
		incoming := len(kg.backlinks[note])
		total := outgoing + incoming
		if total >= 3 {
			kg.hubs = append(kg.hubs, HubNote{
				Path:      note,
				InDegree:  incoming,
				OutDegree: outgoing,
				Total:     total,
			})
		}
	}
	sort.Slice(kg.hubs, func(i, j int) bool {
		return kg.hubs[i].Total > kg.hubs[j].Total
	})
	if len(kg.hubs) > 20 {
		kg.hubs = kg.hubs[:20]
	}
}

func (kg *KnowledgeGraph) findOrphans() {
	kg.orphans = nil
	for _, note := range kg.allNotes {
		outgoing := len(kg.noteLinks[note])
		incoming := len(kg.backlinks[note])
		if outgoing == 0 && incoming == 0 {
			kg.orphans = append(kg.orphans, note)
		}
	}
}

// findClusters uses a simple connected components approach to find clusters.
func (kg *KnowledgeGraph) findClusters() {
	kg.clusters = nil
	visited := make(map[string]bool)

	// Build adjacency (undirected)
	adj := make(map[string]map[string]bool)
	for _, note := range kg.allNotes {
		if adj[note] == nil {
			adj[note] = make(map[string]bool)
		}
		for _, link := range kg.noteLinks[note] {
			adj[note][link] = true
			if adj[link] == nil {
				adj[link] = make(map[string]bool)
			}
			adj[link][note] = true
		}
	}

	// BFS to find connected components
	for _, note := range kg.allNotes {
		if visited[note] {
			continue
		}
		var component []string
		queue := []string{note}
		visited[note] = true
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			component = append(component, current)
			for neighbor := range adj[current] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}
		if len(component) >= 2 {
			// Count internal links
			internalLinks := 0
			for _, n := range component {
				for _, link := range kg.noteLinks[n] {
					for _, other := range component {
						if link == other {
							internalLinks++
						}
					}
				}
			}
			name := kg.clusterName(component)
			kg.clusters = append(kg.clusters, NoteCluster{
				Name:  name,
				Notes: component,
				Links: internalLinks,
			})
		}
	}

	sort.Slice(kg.clusters, func(i, j int) bool {
		return len(kg.clusters[i].Notes) > len(kg.clusters[j].Notes)
	})
}

// clusterName generates a name from the most common word in note names.
func (kg *KnowledgeGraph) clusterName(notes []string) string {
	wordCount := make(map[string]int)
	for _, note := range notes {
		name := strings.TrimSuffix(note, ".md")
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		words := strings.FieldsFunc(name, func(r rune) bool {
			return r == '-' || r == '_' || r == ' '
		})
		for _, w := range words {
			w = strings.ToLower(w)
			if len(w) > 2 {
				wordCount[w]++
			}
		}
	}

	bestWord := "cluster"
	bestCount := 0
	for w, c := range wordCount {
		if c > bestCount {
			bestWord = w
			bestCount = c
		}
	}
	return titleCase(bestWord)
}

// suggestConnections finds notes that share keywords but aren't linked.
func (kg *KnowledgeGraph) suggestConnections() {
	kg.suggestions = nil

	// Build keyword sets per note
	noteKeywords := make(map[string]map[string]bool)
	for _, note := range kg.allNotes {
		name := strings.TrimSuffix(note, ".md")
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
			return r == '-' || r == '_' || r == ' '
		})
		kw := make(map[string]bool)
		for _, w := range words {
			if len(w) > 2 {
				kw[w] = true
			}
		}
		noteKeywords[note] = kw
	}

	// Find pairs with shared keywords but no link
	existingLinks := make(map[string]map[string]bool)
	for _, note := range kg.allNotes {
		existingLinks[note] = make(map[string]bool)
		for _, link := range kg.noteLinks[note] {
			existingLinks[note][link] = true
		}
	}

	for i, noteA := range kg.allNotes {
		for j := i + 1; j < len(kg.allNotes); j++ {
			noteB := kg.allNotes[j]

			// Skip if already linked
			if existingLinks[noteA][noteB] || existingLinks[noteB][noteA] {
				continue
			}

			// Count shared keywords
			shared := 0
			var commonTerms []string
			for kw := range noteKeywords[noteA] {
				if noteKeywords[noteB][kw] {
					shared++
					commonTerms = append(commonTerms, kw)
				}
			}

			if shared >= 2 {
				score := float64(shared) / float64(len(noteKeywords[noteA])+len(noteKeywords[noteB]))
				kg.suggestions = append(kg.suggestions, ConnectionSuggestion{
					From:   noteA,
					To:     noteB,
					Reason: "shared terms: " + strings.Join(commonTerms, ", "),
					Score:  score,
				})
			}
		}
	}

	sort.Slice(kg.suggestions, func(i, j int) bool {
		return kg.suggestions[i].Score > kg.suggestions[j].Score
	})
	if len(kg.suggestions) > 15 {
		kg.suggestions = kg.suggestions[:15]
	}
}

func (kg KnowledgeGraph) Update(msg tea.Msg) (KnowledgeGraph, tea.Cmd) {
	if !kg.active {
		return kg, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			kg.active = false
		case "1":
			kg.analysisMode = 0
			kg.scroll = 0
		case "2":
			kg.analysisMode = 1
			kg.scroll = 0
		case "3":
			kg.analysisMode = 2
			kg.scroll = 0
		case "4":
			kg.analysisMode = 3
			kg.scroll = 0
		case "5":
			kg.analysisMode = 4
			kg.scroll = 0
		case "up", "k":
			if kg.scroll > 0 {
				kg.scroll--
			}
		case "down", "j":
			kg.scroll++
		}
	}
	return kg, nil
}

func (kg KnowledgeGraph) View() string {
	width := kg.width * 3 / 4
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).
		Render("  " + IconGraphChar + " Knowledge Graph Analysis")
	b.WriteString(title)
	b.WriteString("\n")

	// Tab bar
	tabs := []string{"Overview", "Clusters", "Hubs", "Orphans", "Suggestions"}
	for i, tab := range tabs {
		style := lipgloss.NewStyle().Foreground(overlay0).Padding(0, 1)
		if i == kg.analysisMode {
			style = lipgloss.NewStyle().Foreground(base).Background(mauve).Bold(true).Padding(0, 1)
		}
		b.WriteString(style.Render(fmt.Sprintf("%d:%s", i+1, tab)))
		b.WriteString(" ")
	}
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	switch kg.analysisMode {
	case 0:
		kg.renderOverview(&b, width)
	case 1:
		kg.renderClusters(&b, width)
	case 2:
		kg.renderHubs(&b, width)
	case 3:
		kg.renderOrphans(&b, width)
	case 4:
		kg.renderSuggestions(&b, width)
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  1-5: switch view  j/k: scroll  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (kg *KnowledgeGraph) renderOverview(b *strings.Builder, width int) {
	statStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)

	b.WriteString(labelStyle.Render("  Total Notes:     ") + statStyle.Render(fmt.Sprintf("%d", len(kg.allNotes))) + "\n")
	b.WriteString(labelStyle.Render("  Clusters:        ") + statStyle.Render(fmt.Sprintf("%d", len(kg.clusters))) + "\n")
	b.WriteString(labelStyle.Render("  Hub Notes:       ") + statStyle.Render(fmt.Sprintf("%d", len(kg.hubs))) + "\n")
	b.WriteString(labelStyle.Render("  Orphan Notes:    ") + statStyle.Render(fmt.Sprintf("%d", len(kg.orphans))) + "\n")
	b.WriteString(labelStyle.Render("  Suggestions:     ") + statStyle.Render(fmt.Sprintf("%d", len(kg.suggestions))) + "\n")

	// Link density
	totalLinks := 0
	for _, links := range kg.noteLinks {
		totalLinks += len(links)
	}
	if len(kg.allNotes) > 0 {
		density := float64(totalLinks) / float64(len(kg.allNotes))
		b.WriteString(labelStyle.Render("  Avg Links/Note:  ") + statStyle.Render(fmt.Sprintf("%.1f", density)) + "\n")
	}

	// Connectivity score
	connected := len(kg.allNotes) - len(kg.orphans)
	if len(kg.allNotes) > 0 {
		pct := float64(connected) / float64(len(kg.allNotes)) * 100
		b.WriteString(labelStyle.Render("  Connectivity:    ") + statStyle.Render(fmt.Sprintf("%.0f%%", pct)) + "\n")
	}
}

func (kg *KnowledgeGraph) renderClusters(b *strings.Builder, width int) {
	if len(kg.clusters) == 0 {
		b.WriteString(DimStyle.Render("  No clusters found"))
		return
	}

	visible := kg.height - 12
	if visible < 5 {
		visible = 5
	}

	start := kg.scroll
	if start >= len(kg.clusters) {
		start = len(kg.clusters) - 1
	}
	end := start + visible
	if end > len(kg.clusters) {
		end = len(kg.clusters)
	}

	for i := start; i < end; i++ {
		c := kg.clusters[i]
		nameStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
		b.WriteString("  " + nameStyle.Render(c.Name))
		b.WriteString(DimStyle.Render(fmt.Sprintf(" (%d notes, %d links)", len(c.Notes), c.Links)))
		b.WriteString("\n")

		// Show first 5 notes
		maxShow := 5
		if len(c.Notes) < maxShow {
			maxShow = len(c.Notes)
		}
		for j := 0; j < maxShow; j++ {
			noteName := strings.TrimSuffix(c.Notes[j], ".md")
			b.WriteString("    " + lipgloss.NewStyle().Foreground(blue).Render(noteName))
			b.WriteString("\n")
		}
		if len(c.Notes) > maxShow {
			b.WriteString(DimStyle.Render(fmt.Sprintf("    ... and %d more", len(c.Notes)-maxShow)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
}

func (kg *KnowledgeGraph) renderHubs(b *strings.Builder, width int) {
	if len(kg.hubs) == 0 {
		b.WriteString(DimStyle.Render("  No hub notes found"))
		return
	}

	// Bar chart
	maxTotal := kg.hubs[0].Total
	barWidth := width - 40
	if barWidth < 10 {
		barWidth = 10
	}

	visible := kg.height - 10
	if visible < 3 {
		visible = 3
	}

	start := kg.scroll
	if start >= len(kg.hubs) {
		start = len(kg.hubs) - 1
	}
	end := start + visible
	if end > len(kg.hubs) {
		end = len(kg.hubs)
	}

	for i := start; i < end; i++ {
		h := kg.hubs[i]
		name := strings.TrimSuffix(h.Path, ".md")
		if len(name) > 20 {
			name = name[:17] + "..."
		}

		bar := ""
		if maxTotal > 0 {
			filled := h.Total * barWidth / maxTotal
			bar = lipgloss.NewStyle().Foreground(green).Render(strings.Repeat("\u2588", filled))
			bar += strings.Repeat(" ", barWidth-filled)
		}

		nameStr := lipgloss.NewStyle().Foreground(blue).Width(22).Render(name)
		stats := DimStyle.Render(fmt.Sprintf(" in:%d out:%d", h.InDegree, h.OutDegree))
		b.WriteString("  " + nameStr + bar + stats + "\n")
	}
}

func (kg *KnowledgeGraph) renderOrphans(b *strings.Builder, width int) {
	if len(kg.orphans) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("No orphan notes! All notes are connected."))
		return
	}

	b.WriteString(DimStyle.Render(fmt.Sprintf("  %d notes with no links:", len(kg.orphans))))
	b.WriteString("\n\n")

	visible := kg.height - 10
	if visible < 3 {
		visible = 3
	}

	start := kg.scroll
	if start >= len(kg.orphans) {
		start = len(kg.orphans) - 1
	}
	end := start + visible
	if end > len(kg.orphans) {
		end = len(kg.orphans)
	}

	for i := start; i < end; i++ {
		name := strings.TrimSuffix(kg.orphans[i], ".md")
		icon := lipgloss.NewStyle().Foreground(red).Render(" ")
		b.WriteString("  " + icon + " " + NormalItemStyle.Render(name) + "\n")
	}
}

func (kg *KnowledgeGraph) renderSuggestions(b *strings.Builder, width int) {
	if len(kg.suggestions) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("No connection suggestions. Your vault is well-linked!"))
		return
	}

	b.WriteString(DimStyle.Render("  Notes that should be linked:"))
	b.WriteString("\n\n")

	visible := (kg.height - 12) / 3
	if visible < 2 {
		visible = 2
	}

	start := kg.scroll
	if start >= len(kg.suggestions) {
		start = len(kg.suggestions) - 1
	}
	end := start + visible
	if end > len(kg.suggestions) {
		end = len(kg.suggestions)
	}

	for i := start; i < end; i++ {
		s := kg.suggestions[i]
		fromName := strings.TrimSuffix(s.From, ".md")
		toName := strings.TrimSuffix(s.To, ".md")

		arrow := lipgloss.NewStyle().Foreground(peach).Render(" <-> ")
		from := lipgloss.NewStyle().Foreground(blue).Render(fromName)
		to := lipgloss.NewStyle().Foreground(blue).Render(toName)
		b.WriteString("  " + from + arrow + to + "\n")
		b.WriteString("    " + DimStyle.Render(s.Reason) + "\n\n")
	}
}
