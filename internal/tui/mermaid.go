package tui

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Mermaid ASCII renderer
//
// Supports: flowcharts (graph TD/LR/TB/BT), sequence diagrams, pie charts,
// class diagrams, and Gantt charts.
// Called from renderer.go when a ```mermaid code block is encountered in
// view mode.  Returns styled lines ready for display.
// ---------------------------------------------------------------------------

// RenderMermaidASCII takes mermaid source and maxWidth, returns styled lines.
func RenderMermaidASCII(source string, maxWidth int) []string {
	if maxWidth < 20 {
		maxWidth = 20
	}

	dtype := parseMermaidType(source)

	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	borderStyle := lipgloss.NewStyle().Foreground(surface1)

	var lines []string

	// Diagram header
	label := "Diagram"
	switch dtype {
	case "flowchart":
		label = "Flowchart"
	case "sequence":
		label = "Sequence Diagram"
	case "pie":
		label = "Pie Chart"
	case "classDiagram":
		label = "Class Diagram"
	case "gantt":
		label = "Gantt Chart"
	}
	topBorder := borderStyle.Render("  " + strings.Repeat("─", maxWidth-4))
	lines = append(lines, topBorder)
	lines = append(lines, "  "+headerStyle.Render(label))
	lines = append(lines, "")

	var body []string
	switch dtype {
	case "flowchart":
		body = renderFlowchartASCII(source, maxWidth)
	case "sequence":
		body = renderSequenceASCII(source, maxWidth)
	case "pie":
		body = renderPieASCII(source, maxWidth)
	case "classDiagram":
		body = renderClassDiagramASCII(source, maxWidth)
	case "gantt":
		body = renderGanttASCII(source, maxWidth)
	default:
		fallbackStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		body = append(body, "  "+fallbackStyle.Render("Could not render diagram"))
	}

	lines = append(lines, body...)
	lines = append(lines, "")
	lines = append(lines, topBorder)

	return lines
}

// parseMermaidType returns the diagram type.
func parseMermaidType(source string) string {
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "graph ") || strings.HasPrefix(lower, "flowchart ") {
			return "flowchart"
		}
		if strings.HasPrefix(lower, "sequencediagram") {
			return "sequence"
		}
		if strings.HasPrefix(lower, "pie") {
			return "pie"
		}
		if strings.HasPrefix(lower, "classdiagram") {
			return "classDiagram"
		}
		if strings.HasPrefix(lower, "gantt") {
			return "gantt"
		}
		// First non-comment, non-empty line determines the type
		return "unknown"
	}
	return "unknown"
}

// ---------------------------------------------------------------------------
// Flowchart rendering
// ---------------------------------------------------------------------------

type mermaidNode struct {
	id    string
	label string
	shape string // "rect", "round", "diamond", "circle", "asymmetric"
}

type mermaidEdge struct {
	from      string
	to        string
	label     string
	edgeStyle string // "solid", "dotted", "thick"
}

// renderFlowchartASCII renders a flowchart as ASCII box-and-arrow art.
func renderFlowchartASCII(source string, maxWidth int) []string {
	direction, nodes, edges := parseFlowchart(source)
	if len(nodes) == 0 {
		fallbackStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		return []string{"  " + fallbackStyle.Render("Could not render diagram")}
	}

	// Assign nodes to levels using topological ordering
	levels := assignLevels(nodes, edges)

	isVertical := direction == "TD" || direction == "TB" || direction == "BT"

	if isVertical {
		return renderFlowchartVertical(levels, nodes, edges, direction, maxWidth)
	}
	return renderFlowchartHorizontal(levels, nodes, edges, direction, maxWidth)
}

// assignLevels does a topological BFS to assign nodes to depth levels.
// Returns a slice of levels, each level is a slice of node IDs.
func assignLevels(nodes map[string]mermaidNode, edges []mermaidEdge) [][]string {
	// Build adjacency and in-degree
	inDeg := map[string]int{}
	adj := map[string][]string{}
	for id := range nodes {
		inDeg[id] = 0
	}
	for _, e := range edges {
		adj[e.from] = append(adj[e.from], e.to)
		inDeg[e.to]++
	}

	// BFS from sources
	var queue []string
	level := map[string]int{}
	for id := range nodes {
		if inDeg[id] == 0 {
			queue = append(queue, id)
			level[id] = 0
		}
	}
	// Sort queue for deterministic output
	sort.Strings(queue)

	maxLevel := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		targets := adj[cur]
		sort.Strings(targets)
		for _, next := range targets {
			newLevel := level[cur] + 1
			if newLevel > level[next] {
				level[next] = newLevel
			}
			inDeg[next]--
			if inDeg[next] == 0 {
				queue = append(queue, next)
			}
			if newLevel > maxLevel {
				maxLevel = newLevel
			}
		}
	}

	// Handle nodes not reached (cycles) — assign them to level 0
	for id := range nodes {
		if _, ok := level[id]; !ok {
			level[id] = 0
		}
	}

	// Group by level
	result := make([][]string, maxLevel+1)
	for id, lv := range level {
		result[lv] = append(result[lv], id)
	}
	// Sort each level for deterministic output
	for i := range result {
		sort.Strings(result[i])
	}

	return result
}

// edgeLabelLookup builds a map of "from->to" => edgeLabel and "from->to" => edgeStyle.
func edgeLabelLookup(edges []mermaidEdge) (map[string]string, map[string]string) {
	labels := map[string]string{}
	styles := map[string]string{}
	for _, e := range edges {
		key := e.from + "->" + e.to
		labels[key] = e.label
		styles[key] = e.edgeStyle
	}
	return labels, styles
}

func renderFlowchartVertical(levels [][]string, nodes map[string]mermaidNode, edges []mermaidEdge, direction string, maxWidth int) []string {
	var result []string

	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	textStyle := lipgloss.NewStyle().Foreground(text)
	arrowStyle := lipgloss.NewStyle().Foreground(blue)
	labelStyle := lipgloss.NewStyle().Foreground(peach)

	edgeLabels, edgeStyles := edgeLabelLookup(edges)

	// If BT (bottom-top), reverse the level order
	if direction == "BT" {
		for i, j := 0, len(levels)-1; i < j; i, j = i+1, j-1 {
			levels[i], levels[j] = levels[j], levels[i]
		}
	}

	// Build outgoing edges map for drawing connectors
	outgoing := map[string][]string{}
	for _, e := range edges {
		outgoing[e.from] = append(outgoing[e.from], e.to)
	}

	for li, lvl := range levels {
		if len(lvl) == 0 {
			continue
		}

		// Calculate box width — fit all nodes in this level side by side
		numNodes := len(lvl)
		availPerNode := (maxWidth - 4) / numNodes
		if availPerNode < 10 {
			availPerNode = 10
		}
		maxBoxW := availPerNode - 2
		if maxBoxW > maxWidth-8 {
			maxBoxW = maxWidth - 8
		}

		if numNodes == 1 {
			// Single node — center it
			id := lvl[0]
			node := nodes[id]
			lbl := node.label
			if lbl == "" {
				lbl = id
			}
			boxLines := renderNodeBox(lbl, node.shape, maxBoxW, borderStyle, textStyle)
			boxWidth := runeWidth(stripAnsi(boxLines[0]))
			pad := (maxWidth - boxWidth) / 2
			if pad < 2 {
				pad = 2
			}
			for _, bl := range boxLines {
				result = append(result, strings.Repeat(" ", pad)+bl)
			}
		} else {
			// Multiple nodes — render side by side
			allBoxLines := make([][]string, numNodes)
			maxLines := 0
			for ni, id := range lvl {
				node := nodes[id]
				lbl := node.label
				if lbl == "" {
					lbl = id
				}
				allBoxLines[ni] = renderNodeBox(lbl, node.shape, availPerNode-4, borderStyle, textStyle)
				if len(allBoxLines[ni]) > maxLines {
					maxLines = len(allBoxLines[ni])
				}
			}

			// Compose rows
			for row := 0; row < maxLines; row++ {
				var rowBuf strings.Builder
				rowBuf.WriteString("  ")
				for ni := range lvl {
					var segment string
					if row < len(allBoxLines[ni]) {
						segment = allBoxLines[ni][row]
					}
					segW := runeWidth(stripAnsi(segment))
					padR := availPerNode - segW
					if padR < 1 {
						padR = 1
					}
					rowBuf.WriteString(segment)
					rowBuf.WriteString(strings.Repeat(" ", padR))
				}
				result = append(result, rowBuf.String())
			}
		}

		// Draw connectors to next level
		if li < len(levels)-1 {
			nextLvl := levels[li+1]
			nextSet := map[string]bool{}
			for _, nid := range nextLvl {
				nextSet[nid] = true
			}

			// Collect all edges from this level to the next
			type conn struct {
				from  string
				to    string
				label string
				style string
			}
			var conns []conn
			for _, id := range lvl {
				for _, tgt := range outgoing[id] {
					if nextSet[tgt] {
						key := id + "->" + tgt
						if direction == "BT" {
							key = tgt + "->" + id
						}
						conns = append(conns, conn{
							from:  id,
							to:    tgt,
							label: edgeLabels[key],
							style: edgeStyles[key],
						})
					}
				}
			}

			if len(conns) == 0 {
				// No direct edges to next level — just show a gap
				result = append(result, "")
			} else {
				// Render connector lines
				centerCol := maxWidth / 2

				// Check if any connections have labels
				hasLabel := false
				for _, c := range conns {
					if c.label != "" {
						hasLabel = true
						break
					}
				}

				// Vertical connector
				arrowChar := "▼"
				if direction == "BT" {
					arrowChar = "▲"
				}

				connChar := "│"
				if len(conns) == 1 {
					if conns[0].style == "dotted" {
						connChar = "┊"
					} else if conns[0].style == "thick" {
						connChar = "┃"
					}
				}

				pad := strings.Repeat(" ", centerCol-1)
				result = append(result, pad+arrowStyle.Render(connChar))
				if hasLabel {
					for _, c := range conns {
						if c.label != "" {
							lblPad := centerCol - len([]rune(c.label))/2 - 1
							if lblPad < 2 {
								lblPad = 2
							}
							result = append(result, strings.Repeat(" ", lblPad)+labelStyle.Render(c.label))
						}
					}
					result = append(result, pad+arrowStyle.Render(connChar))
				}
				result = append(result, pad+arrowStyle.Render(arrowChar))
			}
		}
	}

	return result
}

func renderFlowchartHorizontal(levels [][]string, nodes map[string]mermaidNode, edges []mermaidEdge, direction string, maxWidth int) []string {
	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	textStyle := lipgloss.NewStyle().Foreground(text)
	arrowStyle := lipgloss.NewStyle().Foreground(blue)
	labelStyle := lipgloss.NewStyle().Foreground(peach)

	edgeLabels, _ := edgeLabelLookup(edges)

	isRL := direction == "RL"

	if isRL {
		for i, j := 0, len(levels)-1; i < j; i, j = i+1, j-1 {
			levels[i], levels[j] = levels[j], levels[i]
		}
	}

	// For horizontal: each level becomes a column, nodes stack vertically
	// We render level by level left-to-right with arrows between
	numLevels := len(levels)
	if numLevels == 0 {
		return nil
	}

	arrowWidth := 5 // " ──→ "
	availPerLevel := (maxWidth - 4 - (numLevels-1)*arrowWidth) / numLevels
	if availPerLevel < 8 {
		availPerLevel = 8
	}

	// Find max nodes in any level (determines how many vertical rows we need)
	maxNodesInLevel := 0
	for _, lvl := range levels {
		if len(lvl) > maxNodesInLevel {
			maxNodesInLevel = len(lvl)
		}
	}

	// For each node in each level, compute the box
	type boxData struct {
		lines []string
		width int
	}
	levelBoxes := make([][]boxData, numLevels)
	for li, lvl := range levels {
		for _, id := range lvl {
			node := nodes[id]
			lbl := node.label
			if lbl == "" {
				lbl = id
			}
			blines := renderNodeBox(lbl, node.shape, availPerLevel-2, borderStyle, textStyle)
			w := 0
			for _, bl := range blines {
				bw := runeWidth(stripAnsi(bl))
				if bw > w {
					w = bw
				}
			}
			levelBoxes[li] = append(levelBoxes[li], boxData{lines: blines, width: w})
		}
	}

	// Render each vertical slice (each node row)
	var result []string
	for ni := 0; ni < maxNodesInLevel; ni++ {
		// Each box is 3 lines tall (top, mid, bot)
		for row := 0; row < 3; row++ {
			var line strings.Builder
			line.WriteString("    ")

			for li := 0; li < numLevels; li++ {
				if ni < len(levelBoxes[li]) {
					bd := levelBoxes[li][ni]
					if row < len(bd.lines) {
						line.WriteString(bd.lines[row])
						padR := availPerLevel - bd.width
						if padR > 0 {
							line.WriteString(strings.Repeat(" ", padR))
						}
					} else {
						line.WriteString(strings.Repeat(" ", availPerLevel))
					}
				} else {
					line.WriteString(strings.Repeat(" ", availPerLevel))
				}

				// Arrow connector between levels on middle row
				if li < numLevels-1 {
					if row == 1 && ni < len(levelBoxes[li]) && ni < len(levelBoxes[li+1]) {
						// There are nodes on both sides
						fromID := ""
						toID := ""
						if ni < len(levels[li]) {
							fromID = levels[li][ni]
						}
						if ni < len(levels[li+1]) {
							toID = levels[li+1][ni]
						}

						edgeKey := fromID + "->" + toID
						if isRL {
							edgeKey = toID + "->" + fromID
						}
						el := edgeLabels[edgeKey]

						if isRL {
							arr := arrowStyle.Render(" ←── ")
							line.WriteString(arr)
						} else {
							arr := arrowStyle.Render(" ──→ ")
							line.WriteString(arr)
						}
						_ = el
					} else {
						line.WriteString("     ")
					}
				}
			}
			result = append(result, line.String())
		}

		// Edge labels row
		hasEdgeLabel := false
		var labelLine strings.Builder
		labelLine.WriteString("    ")
		for li := 0; li < numLevels; li++ {
			labelLine.WriteString(strings.Repeat(" ", availPerLevel))
			if li < numLevels-1 {
				fromID := ""
				toID := ""
				if ni < len(levels[li]) {
					fromID = levels[li][ni]
				}
				if ni < len(levels[li+1]) {
					toID = levels[li+1][ni]
				}
				edgeKey := fromID + "->" + toID
				if isRL {
					edgeKey = toID + "->" + fromID
				}
				el := edgeLabels[edgeKey]
				if el != "" {
					hasEdgeLabel = true
					pad := arrowWidth - len([]rune(el))
					if pad < 0 {
						pad = 0
					}
					labelLine.WriteString(labelStyle.Render(el) + strings.Repeat(" ", pad))
				} else {
					labelLine.WriteString(strings.Repeat(" ", arrowWidth))
				}
			}
		}
		if hasEdgeLabel {
			result = append(result, labelLine.String())
		}

		// Add spacing between node rows
		if ni < maxNodesInLevel-1 {
			result = append(result, "")
		}
	}

	return result
}

func renderNodeBox(label string, shape string, maxW int, borderStyle, textStyle lipgloss.Style) []string {
	rLabel := []rune(label)
	if maxW < 6 {
		maxW = 6
	}
	if len(rLabel) > maxW-4 {
		rLabel = append(rLabel[:maxW-5], '…')
	}
	displayLabel := string(rLabel)
	innerWidth := len(rLabel) + 2
	if innerWidth < 5 {
		innerWidth = 5
	}

	// Center the label within the inner width
	labelLen := len(rLabel)
	totalPad := innerWidth - labelLen
	padLeft := totalPad / 2
	padRight := totalPad - padLeft

	switch shape {
	case "diamond":
		topW := innerWidth + 2
		topLine := borderStyle.Render("  ◇" + strings.Repeat("─", topW) + "◇")
		midLine := borderStyle.Render("◁ ") + textStyle.Render(strings.Repeat(" ", padLeft)+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render(" ▷")
		botLine := borderStyle.Render("  ◇" + strings.Repeat("─", topW) + "◇")
		return []string{topLine, midLine, botLine}
	case "circle":
		topLine := borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")
		midLine := borderStyle.Render("│") + textStyle.Render(strings.Repeat(" ", padLeft)+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render("│")
		botLine := borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
		return []string{topLine, midLine, botLine}
	case "round":
		topLine := borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")
		midLine := borderStyle.Render("(") + textStyle.Render(strings.Repeat(" ", padLeft)+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render(")")
		botLine := borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
		return []string{topLine, midLine, botLine}
	case "asymmetric":
		topLine := borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")
		midLine := borderStyle.Render("▶") + textStyle.Render(strings.Repeat(" ", padLeft)+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render("│")
		botLine := borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
		return []string{topLine, midLine, botLine}
	default: // "rect"
		topLine := borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")
		midLine := borderStyle.Render("│") + textStyle.Render(strings.Repeat(" ", padLeft)+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render("│")
		botLine := borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
		return []string{topLine, midLine, botLine}
	}
}

// Regex patterns for flowchart parsing
var (
	// Node definitions: A[text], A(text), A{text}, A((text)), A>text]
	flowNodeRe = regexp.MustCompile(`([A-Za-z_]\w*)\s*(\[([^\]]*)\]|\(([^)]*)\)|\{([^}]*)\}|\(\(([^)]*)\)\)|>([^\]]*)\])`)
	// Edge patterns: A-->B, A-->|label|B, A---B, A-.->B, A==>B
	flowEdgeRe = regexp.MustCompile(`([A-Za-z_]\w*)\s*(-->\|([^|]*)\||-->|---|\.-\.?->|==>)\s*(\|([^|]*)\|)?\s*([A-Za-z_]\w*)`)
)

func parseFlowchart(source string) (string, map[string]mermaidNode, []mermaidEdge) {
	nodes := map[string]mermaidNode{}
	var edges []mermaidEdge
	direction := "TD" // default

	lines := strings.Split(source, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}

		// Parse direction from graph/flowchart line
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "graph ") || strings.HasPrefix(lower, "flowchart ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				dir := strings.ToUpper(parts[1])
				if dir == "TD" || dir == "TB" || dir == "LR" || dir == "BT" || dir == "RL" {
					direction = dir
				}
			}
			continue
		}

		// Try to parse edges (which also contain node info)
		edgeMatches := flowEdgeRe.FindAllStringSubmatch(trimmed, -1)
		for _, m := range edgeMatches {
			fromID := m[1]
			edgeType := m[2]
			toID := m[6]

			// Determine edge label
			edgeLabel := ""
			if m[3] != "" {
				edgeLabel = m[3]
			}
			if m[5] != "" {
				edgeLabel = m[5]
			}

			// Determine edge style
			style := "solid"
			if strings.Contains(edgeType, ".") {
				style = "dotted"
			} else if strings.Contains(edgeType, "==") {
				style = "thick"
			}

			edges = append(edges, mermaidEdge{
				from:      fromID,
				to:        toID,
				label:     edgeLabel,
				edgeStyle: style,
			})

			// Ensure nodes exist
			if _, ok := nodes[fromID]; !ok {
				nodes[fromID] = mermaidNode{id: fromID, label: fromID, shape: "rect"}
			}
			if _, ok := nodes[toID]; !ok {
				nodes[toID] = mermaidNode{id: toID, label: toID, shape: "rect"}
			}
		}

		// Parse standalone node definitions
		nodeMatches := flowNodeRe.FindAllStringSubmatch(trimmed, -1)
		for _, m := range nodeMatches {
			id := m[1]
			// Skip the direction keywords
			idLower := strings.ToLower(id)
			if idLower == "graph" || idLower == "flowchart" || idLower == "subgraph" || idLower == "end" {
				continue
			}

			var label string
			shape := "rect"
			if m[3] != "" {
				label = m[3]
				shape = "rect"
			} else if m[4] != "" {
				label = m[4]
				shape = "round"
			} else if m[5] != "" {
				label = m[5]
				shape = "diamond"
			} else if m[6] != "" {
				label = m[6]
				shape = "circle"
			} else if m[7] != "" {
				label = m[7]
				shape = "asymmetric"
			}

			if label != "" {
				nodes[id] = mermaidNode{id: id, label: label, shape: shape}
			}
		}
	}

	return direction, nodes, edges
}

// ---------------------------------------------------------------------------
// Sequence diagram rendering
// ---------------------------------------------------------------------------

type seqParticipant struct {
	name string
	col  int // center column position (calculated during rendering)
}

type seqMessage struct {
	from       string
	to         string
	message    string
	dashed     bool
	activate   bool // ->>+ activates target
	deactivate bool // ->>- deactivates target
}

type seqNote struct {
	over string
	text string
}

// renderSequenceASCII renders a sequence diagram as ASCII art.
func renderSequenceASCII(source string, maxWidth int) []string {
	participants, messages, notes := parseSequenceDiagram(source)

	if len(participants) == 0 {
		fallbackStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		return []string{"  " + fallbackStyle.Render("Could not render diagram")}
	}

	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	nameStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	arrowStyle := lipgloss.NewStyle().Foreground(blue)
	msgStyle := lipgloss.NewStyle().Foreground(text)
	noteStyle := lipgloss.NewStyle().Foreground(peach)
	lifelineStyle := lipgloss.NewStyle().Foreground(surface1)
	activStyle := lipgloss.NewStyle().Foreground(lavender)

	// Calculate column positions for each participant
	numP := len(participants)
	spacing := (maxWidth - 8) / numP
	if spacing < 14 {
		spacing = 14
	}

	partMap := map[string]*seqParticipant{}
	for i := range participants {
		participants[i].col = 4 + spacing/2 + i*spacing
		partMap[participants[i].name] = &participants[i]
	}

	// Track active lifelines for activation boxes
	activeLifelines := map[string]bool{}

	var result []string

	// Draw participant boxes at top
	topLine, midLine, botLine := buildParticipantRow(participants, spacing, borderStyle, nameStyle)
	result = append(result, topLine, midLine, botLine)

	// Process messages and notes
	allEvents := buildEventList(messages, notes)
	for _, evt := range allEvents {
		// Draw a lifeline row
		lifeRow := buildLifelineWithActivation(participants, spacing, lifelineStyle, activStyle, activeLifelines)
		result = append(result, lifeRow)

		if evt.isNote {
			noteRow := buildNoteRow(evt.note, partMap, spacing, borderStyle, noteStyle)
			result = append(result, noteRow)
		} else {
			// Handle activation/deactivation
			if evt.msg.activate {
				activeLifelines[evt.msg.to] = true
			}
			if evt.msg.deactivate {
				delete(activeLifelines, evt.msg.to)
			}

			msgRows := buildMessageRowImproved(evt.msg, partMap, participants, spacing, arrowStyle, msgStyle, lifelineStyle, activStyle, activeLifelines)
			result = append(result, msgRows...)
		}
	}

	// Final lifeline
	lifeRow := buildLifelineWithActivation(participants, spacing, lifelineStyle, activStyle, activeLifelines)
	result = append(result, lifeRow)

	// Bottom participant boxes
	topLine, midLine, botLine = buildParticipantRow(participants, spacing, borderStyle, nameStyle)
	result = append(result, topLine, midLine, botLine)

	return result
}

type seqEvent struct {
	isNote bool
	msg    seqMessage
	note   seqNote
}

func buildEventList(messages []seqMessage, notes []seqNote) []seqEvent {
	var events []seqEvent
	noteIdx := 0
	for _, m := range messages {
		if noteIdx < len(notes) {
			events = append(events, seqEvent{isNote: true, note: notes[noteIdx]})
			noteIdx++
		}
		events = append(events, seqEvent{isNote: false, msg: m})
	}
	for ; noteIdx < len(notes); noteIdx++ {
		events = append(events, seqEvent{isNote: true, note: notes[noteIdx]})
	}
	return events
}

func buildParticipantRow(participants []seqParticipant, spacing int, borderStyle, nameStyle lipgloss.Style) (string, string, string) {
	var topParts, midParts, botParts []string
	for _, p := range participants {
		nameLen := len([]rune(p.name))
		boxW := nameLen + 2
		if boxW < 6 {
			boxW = 6
		}
		padLeft := (spacing - boxW - 2) / 2
		if padLeft < 0 {
			padLeft = 0
		}
		padRight := spacing - boxW - 2 - padLeft
		if padRight < 0 {
			padRight = 0
		}

		// Center the name in the box
		innerPad := boxW - nameLen
		innerLeft := innerPad / 2
		innerRight := innerPad - innerLeft

		topParts = append(topParts, strings.Repeat(" ", padLeft)+borderStyle.Render("╭"+strings.Repeat("─", boxW)+"╮")+strings.Repeat(" ", padRight))
		midParts = append(midParts, strings.Repeat(" ", padLeft)+borderStyle.Render("│")+nameStyle.Render(strings.Repeat(" ", innerLeft)+p.name+strings.Repeat(" ", innerRight))+borderStyle.Render("│")+strings.Repeat(" ", padRight))
		botParts = append(botParts, strings.Repeat(" ", padLeft)+borderStyle.Render("╰"+strings.Repeat("─", boxW)+"╯")+strings.Repeat(" ", padRight))
	}
	return "    " + strings.Join(topParts, ""),
		"    " + strings.Join(midParts, ""),
		"    " + strings.Join(botParts, "")
}


func buildLifelineWithActivation(participants []seqParticipant, spacing int, lifelineStyle, activStyle lipgloss.Style, active map[string]bool) string {
	var parts []string
	for _, p := range participants {
		center := spacing / 2
		left := center
		right := spacing - center - 1
		if active[p.name] {
			// Active lifeline — draw activation box
			if left > 0 {
				left--
			}
			if right > 0 {
				right--
			}
			parts = append(parts, strings.Repeat(" ", left)+activStyle.Render("┃│┃")+strings.Repeat(" ", right))
		} else {
			parts = append(parts, strings.Repeat(" ", left)+lifelineStyle.Render("│")+strings.Repeat(" ", right))
		}
	}
	return "    " + strings.Join(parts, "")
}

func buildMessageRowImproved(msg seqMessage, partMap map[string]*seqParticipant, participants []seqParticipant, spacing int, arrowStyle, msgStyle, lifelineStyle, activStyle lipgloss.Style, active map[string]bool) []string {
	// Find indices
	fromIdx := -1
	toIdx := -1
	orderedNames := make([]string, len(participants))
	for i, p := range participants {
		orderedNames[i] = p.name
		if p.name == msg.from {
			fromIdx = i
		}
		if p.name == msg.to {
			toIdx = i
		}
	}
	if fromIdx < 0 || toIdx < 0 {
		return []string{"    " + msgStyle.Render(msg.message)}
	}

	center := spacing / 2
	var lines []string

	// Self-message
	if fromIdx == toIdx {
		// Render label
		preSpaces := fromIdx*spacing + center + 2
		if msg.message != "" {
			lines = append(lines, strings.Repeat(" ", 4+preSpaces)+msgStyle.Render(msg.message))
		}
		lines = append(lines, strings.Repeat(" ", 4+preSpaces)+arrowStyle.Render("╭──╮"))
		lines = append(lines, strings.Repeat(" ", 4+preSpaces)+arrowStyle.Render("╰─→╯"))
		return lines
	}

	// Render label above the arrow
	if msg.message != "" {
		minIdx := fromIdx
		if toIdx < minIdx {
			minIdx = toIdx
		}
		preSpaces := minIdx*spacing + center + 1
		lines = append(lines, strings.Repeat(" ", 4+preSpaces)+msgStyle.Render(msg.message))
	}

	// Render arrow line
	var arrowLine strings.Builder
	arrowLine.WriteString("    ")

	leftToRight := fromIdx < toIdx

	for idx, name := range orderedNames {
		isActive := active[name]

		inArrowRange := false
		if leftToRight {
			inArrowRange = idx >= fromIdx && idx <= toIdx
		} else {
			inArrowRange = idx >= toIdx && idx <= fromIdx
		}

		if inArrowRange {
			if leftToRight {
				if idx == fromIdx {
					left := center
					arrowSegLen := spacing - center
					if idx+1 == toIdx {
						// Direct connection to next
						lineChar := "─"
						if msg.dashed {
							lineChar = "╌"
						}
						arrowLine.WriteString(strings.Repeat(" ", left))
						arrowLine.WriteString(arrowStyle.Render(strings.Repeat(lineChar, arrowSegLen-1)))
					} else {
						lineChar := "─"
						if msg.dashed {
							lineChar = "╌"
						}
						arrowLine.WriteString(strings.Repeat(" ", left))
						arrowLine.WriteString(arrowStyle.Render(strings.Repeat(lineChar, arrowSegLen)))
					}
				} else if idx == toIdx {
					lineChar := "─"
					if msg.dashed {
						lineChar = "╌"
					}
					arrowLine.WriteString(arrowStyle.Render(strings.Repeat(lineChar, center-1) + "→"))
					right := spacing - center - 1
					if right > 0 {
						arrowLine.WriteString(strings.Repeat(" ", right))
					}
				} else {
					// Middle — arrow passes through
					lineChar := "─"
					if msg.dashed {
						lineChar = "╌"
					}
					arrowLine.WriteString(arrowStyle.Render(strings.Repeat(lineChar, spacing)))
				}
			} else {
				// Right to left
				if idx == toIdx {
					left := center - 1
					if left < 0 {
						left = 0
					}
					lineChar := "─"
					if msg.dashed {
						lineChar = "╌"
					}
					arrowLine.WriteString(strings.Repeat(" ", left))
					arrowLine.WriteString(arrowStyle.Render("←" + strings.Repeat(lineChar, spacing-center-1)))
				} else if idx == fromIdx {
					lineChar := "─"
					if msg.dashed {
						lineChar = "╌"
					}
					arrowLine.WriteString(arrowStyle.Render(strings.Repeat(lineChar, center)))
					right := spacing - center - 1
					if right > 0 {
						arrowLine.WriteString(strings.Repeat(" ", right))
					}
				} else {
					// Middle — arrow passes through
					lineChar := "─"
					if msg.dashed {
						lineChar = "╌"
					}
					arrowLine.WriteString(arrowStyle.Render(strings.Repeat(lineChar, spacing)))
				}
			}
		} else {
			// Not part of arrow — draw lifeline
			left := center
			right := spacing - center - 1
			if isActive {
				if left > 0 {
					left--
				}
				if right > 0 {
					right--
				}
				arrowLine.WriteString(strings.Repeat(" ", left) + activStyle.Render("┃│┃") + strings.Repeat(" ", right))
			} else {
				arrowLine.WriteString(strings.Repeat(" ", left) + lifelineStyle.Render("│") + strings.Repeat(" ", right))
			}
		}
	}

	lines = append(lines, arrowLine.String())
	return lines
}

func buildNoteRow(note seqNote, partMap map[string]*seqParticipant, spacing int, borderStyle, noteStyle lipgloss.Style) string {
	p, ok := partMap[note.over]
	if !ok {
		return "    " + noteStyle.Render("┊ Note: "+note.text+" ┊")
	}

	// Find index of the participant
	orderedNames := make([]string, 0, len(partMap))
	for name := range partMap {
		orderedNames = append(orderedNames, name)
	}
	sort.Slice(orderedNames, func(i, j int) bool {
		return partMap[orderedNames[i]].col < partMap[orderedNames[j]].col
	})

	partIdx := 0
	for idx, name := range orderedNames {
		if name == note.over {
			partIdx = idx
			break
		}
	}
	_ = p

	center := spacing / 2
	noteText := note.text
	noteW := len([]rune(noteText)) + 2
	noteOffset := partIdx*spacing + center - noteW/2
	if noteOffset < 0 {
		noteOffset = 0
	}

	var lines []string
	topLine := strings.Repeat(" ", 4+noteOffset) + borderStyle.Render("╭"+strings.Repeat("─", noteW)+"╮")
	midLine := strings.Repeat(" ", 4+noteOffset) + borderStyle.Render("│") + noteStyle.Render(" "+noteText+" ") + borderStyle.Render("│")
	botLine := strings.Repeat(" ", 4+noteOffset) + borderStyle.Render("╰"+strings.Repeat("─", noteW)+"╯")
	lines = append(lines, topLine, midLine, botLine)

	return strings.Join(lines, "\n")
}

func parseSequenceDiagram(source string) ([]seqParticipant, []seqMessage, []seqNote) {
	var participants []seqParticipant
	var messages []seqMessage
	var notes []seqNote

	participantSet := map[string]bool{}

	participantRe := regexp.MustCompile(`(?i)^\s*participant\s+(\S+)`)
	// Matches: A->>B: msg, A-->>B: msg, A->>+B: msg, A->>-B: msg, A->B, A-->B
	messageRe := regexp.MustCompile(`^\s*(\S+?)\s*(->>|-->>|->|-->)\s*([\+\-]?)(\S+?)\s*:\s*(.*)$`)
	noteRe := regexp.MustCompile(`(?i)^\s*note\s+(over|left of|right of)\s+(\S+?)\s*:\s*(.*)$`)

	lines := strings.Split(source, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "sequencediagram") {
			continue
		}

		// Participant
		if pm := participantRe.FindStringSubmatch(trimmed); pm != nil {
			name := pm[1]
			if !participantSet[name] {
				participantSet[name] = true
				participants = append(participants, seqParticipant{name: name})
			}
			continue
		}

		// Note
		if nm := noteRe.FindStringSubmatch(trimmed); nm != nil {
			over := nm[2]
			over = strings.TrimRight(over, ",")
			notes = append(notes, seqNote{over: over, text: strings.TrimSpace(nm[3])})
			if !participantSet[over] {
				participantSet[over] = true
				participants = append(participants, seqParticipant{name: over})
			}
			continue
		}

		// Message
		if mm := messageRe.FindStringSubmatch(trimmed); mm != nil {
			from := mm[1]
			arrow := mm[2]
			activeMark := mm[3]
			to := mm[4]
			msg := strings.TrimSpace(mm[5])

			dashed := strings.Contains(arrow, "--")

			messages = append(messages, seqMessage{
				from:       from,
				to:         to,
				message:    msg,
				dashed:     dashed,
				activate:   activeMark == "+",
				deactivate: activeMark == "-",
			})

			if !participantSet[from] {
				participantSet[from] = true
				participants = append(participants, seqParticipant{name: from})
			}
			if !participantSet[to] {
				participantSet[to] = true
				participants = append(participants, seqParticipant{name: to})
			}
			continue
		}
	}

	return participants, messages, notes
}

// ---------------------------------------------------------------------------
// Pie chart rendering
// ---------------------------------------------------------------------------

type pieSlice struct {
	label   string
	value   float64
	percent float64
}

// renderPieASCII renders a pie chart as horizontal bar chart with visual improvements.
func renderPieASCII(source string, maxWidth int) []string {
	title, slices := parsePieChart(source)
	if len(slices) == 0 {
		fallbackStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		return []string{"  " + fallbackStyle.Render("Could not render diagram")}
	}

	// Calculate percentages
	total := 0.0
	for _, s := range slices {
		total += s.value
	}
	if total == 0 {
		total = 1
	}
	for i := range slices {
		slices[i].percent = (slices[i].value / total) * 100.0
	}

	// Color palette
	barColors := []lipgloss.Color{blue, green, peach, yellow, pink, teal, mauve, lavender}

	// Find longest label for alignment
	maxLabelLen := 0
	for _, s := range slices {
		if len([]rune(s.label)) > maxLabelLen {
			maxLabelLen = len([]rune(s.label))
		}
	}
	if maxLabelLen > maxWidth/3 {
		maxLabelLen = maxWidth / 3
	}

	// Bar area width
	percentWidth := 8 // " XX.X% "
	barAreaWidth := maxWidth - 6 - maxLabelLen - percentWidth - 6
	if barAreaWidth < 10 {
		barAreaWidth = 10
	}

	var result []string

	// Show title if present
	if title != "" {
		titleStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		result = append(result, "    "+titleStyle.Render(title))
		result = append(result, "")
	}

	for i, s := range slices {
		colorIdx := i % len(barColors)
		barStyle := lipgloss.NewStyle().Foreground(barColors[colorIdx])
		labelStyle := lipgloss.NewStyle().Foreground(text)
		pctStyle := lipgloss.NewStyle().Foreground(peach)
		legendStyle := lipgloss.NewStyle().Foreground(barColors[colorIdx])

		// Truncate label
		label := s.label
		labelRunes := []rune(label)
		if len(labelRunes) > maxLabelLen {
			label = string(labelRunes[:maxLabelLen-1]) + "…"
		}
		labelPad := maxLabelLen - len([]rune(label))

		// Calculate bar length
		barLen := int(math.Round(s.percent / 100.0 * float64(barAreaWidth)))
		if barLen < 1 && s.value > 0 {
			barLen = 1
		}

		bar := buildBarString(barLen, barAreaWidth)

		pctStr := fmt.Sprintf("%5.1f%%", s.percent)

		line := "    " +
			legendStyle.Render("■") + " " +
			labelStyle.Render(label+strings.Repeat(" ", labelPad)) +
			"  " +
			barStyle.Render(bar) +
			" " +
			pctStyle.Render(pctStr)
		result = append(result, line)
	}

	// Summary
	totalStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
	sepStyle := lipgloss.NewStyle().Foreground(surface1)
	result = append(result, "")
	result = append(result, "    "+sepStyle.Render(strings.Repeat("─", maxLabelLen+barAreaWidth+percentWidth+4)))
	totalVal := fmt.Sprintf("Total: %.0f  |  %d categories", total, len(slices))
	result = append(result, "    "+totalStyle.Render(totalVal))

	return result
}

func buildBarString(filled int, total int) string {
	if filled > total {
		filled = total
	}
	bar := strings.Repeat("█", filled)
	remainder := total - filled
	if remainder > 0 {
		bar += strings.Repeat("░", remainder)
	}
	return bar
}

var pieLineRe = regexp.MustCompile(`^\s*"([^"]+)"\s*:\s*([\d.]+)\s*$`)

func parsePieChart(source string) (string, []pieSlice) {
	var slices []pieSlice
	title := ""

	lines := strings.Split(source, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "pie") {
			continue
		}
		if strings.HasPrefix(lower, "title ") {
			title = strings.TrimSpace(trimmed[6:])
			continue
		}

		m := pieLineRe.FindStringSubmatch(trimmed)
		if m != nil {
			val, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				continue
			}
			slices = append(slices, pieSlice{
				label: m[1],
				value: val,
			})
		}
	}

	return title, slices
}

// ---------------------------------------------------------------------------
// Class diagram rendering
// ---------------------------------------------------------------------------

type classInfo struct {
	name    string
	fields  []string
	methods []string
}

type classRelation struct {
	from     string
	to       string
	relType  string // "--|>", "--*", "--o", "--", "..|>", ".."
	label    string
	fromCard string
	toCard   string
}

func renderClassDiagramASCII(source string, maxWidth int) []string {
	classes, relations := parseClassDiagram(source)
	if len(classes) == 0 {
		fallbackStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		return []string{"  " + fallbackStyle.Render("Could not render diagram")}
	}

	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	nameStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
	fieldStyle := lipgloss.NewStyle().Foreground(text)
	methodStyle := lipgloss.NewStyle().Foreground(blue)
	relStyle := lipgloss.NewStyle().Foreground(peach)

	var result []string

	// Render each class as a box
	numClasses := len(classes)
	perClass := (maxWidth - 4) / numClasses
	if perClass < 20 {
		perClass = 20
	}

	// Check if they fit side-by-side
	sideBySide := perClass*numClasses <= maxWidth-4 && numClasses <= 4

	if sideBySide && numClasses > 1 {
		// Render classes side by side
		boxWidth := perClass - 4
		if boxWidth > 30 {
			boxWidth = 30
		}

		// Build each class box
		allBoxes := make([][]string, numClasses)
		maxLines := 0
		for ci, cls := range classes {
			allBoxes[ci] = renderClassBox(cls, boxWidth, borderStyle, nameStyle, fieldStyle, methodStyle)
			if len(allBoxes[ci]) > maxLines {
				maxLines = len(allBoxes[ci])
			}
		}

		// Pad all boxes to same height
		for ci := range allBoxes {
			for len(allBoxes[ci]) < maxLines {
				allBoxes[ci] = append(allBoxes[ci], strings.Repeat(" ", boxWidth+2))
			}
		}

		// Render rows
		for row := 0; row < maxLines; row++ {
			var line strings.Builder
			line.WriteString("    ")
			for ci := range allBoxes {
				line.WriteString(allBoxes[ci][row])
				gap := perClass - boxWidth - 2
				if gap < 2 {
					gap = 2
				}
				if ci < numClasses-1 {
					line.WriteString(strings.Repeat(" ", gap))
				}
			}
			result = append(result, line.String())
		}
	} else {
		// Render classes stacked vertically
		boxWidth := maxWidth - 12
		if boxWidth > 50 {
			boxWidth = 50
		}
		for ci, cls := range classes {
			boxLines := renderClassBox(cls, boxWidth, borderStyle, nameStyle, fieldStyle, methodStyle)
			for _, bl := range boxLines {
				result = append(result, "    "+bl)
			}
			if ci < numClasses-1 {
				result = append(result, "")
			}
		}
	}

	// Render relationships below
	if len(relations) > 0 {
		result = append(result, "")
		relHeaderStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		result = append(result, "    "+relHeaderStyle.Render("Relationships:"))
		for _, rel := range relations {
			arrow := renderRelationArrow(rel.relType)
			label := ""
			if rel.label != "" {
				label = " : " + rel.label
			}
			cardinality := ""
			if rel.fromCard != "" || rel.toCard != "" {
				cardinality = " [" + rel.fromCard + ".." + rel.toCard + "]"
			}
			result = append(result, "      "+relStyle.Render(rel.from+" "+arrow+" "+rel.to+label+cardinality))
		}
	}

	return result
}

func renderClassBox(cls classInfo, width int, borderStyle, nameStyle, fieldStyle, methodStyle lipgloss.Style) []string {
	if width < 10 {
		width = 10
	}
	var lines []string

	// Top border
	lines = append(lines, borderStyle.Render("╭"+strings.Repeat("─", width)+"╮"))

	// Class name — centered
	nameRunes := []rune(cls.name)
	if len(nameRunes) > width-2 {
		nameRunes = append(nameRunes[:width-3], '…')
	}
	nameStr := string(nameRunes)
	namePad := width - len(nameRunes)
	nameLeft := namePad / 2
	nameRight := namePad - nameLeft
	lines = append(lines, borderStyle.Render("│")+nameStyle.Render(strings.Repeat(" ", nameLeft)+nameStr+strings.Repeat(" ", nameRight))+borderStyle.Render("│"))

	// Separator
	lines = append(lines, borderStyle.Render("├"+strings.Repeat("─", width)+"┤"))

	// Fields
	if len(cls.fields) > 0 {
		for _, f := range cls.fields {
			fRunes := []rune(f)
			if len(fRunes) > width-2 {
				fRunes = append(fRunes[:width-3], '…')
			}
			fStr := string(fRunes)
			fPad := width - len(fRunes) - 1
			if fPad < 0 {
				fPad = 0
			}
			lines = append(lines, borderStyle.Render("│")+fieldStyle.Render(" "+fStr+strings.Repeat(" ", fPad))+borderStyle.Render("│"))
		}
	} else {
		emptyPad := width - 1
		if emptyPad < 0 {
			emptyPad = 0
		}
		lines = append(lines, borderStyle.Render("│")+fieldStyle.Render(" "+strings.Repeat(" ", emptyPad))+borderStyle.Render("│"))
	}

	// Separator
	lines = append(lines, borderStyle.Render("├"+strings.Repeat("─", width)+"┤"))

	// Methods
	if len(cls.methods) > 0 {
		for _, m := range cls.methods {
			mRunes := []rune(m)
			if len(mRunes) > width-2 {
				mRunes = append(mRunes[:width-3], '…')
			}
			mStr := string(mRunes)
			mPad := width - len(mRunes) - 1
			if mPad < 0 {
				mPad = 0
			}
			lines = append(lines, borderStyle.Render("│")+methodStyle.Render(" "+mStr+strings.Repeat(" ", mPad))+borderStyle.Render("│"))
		}
	} else {
		emptyPad := width - 1
		if emptyPad < 0 {
			emptyPad = 0
		}
		lines = append(lines, borderStyle.Render("│")+methodStyle.Render(" "+strings.Repeat(" ", emptyPad))+borderStyle.Render("│"))
	}

	// Bottom border
	lines = append(lines, borderStyle.Render("╰"+strings.Repeat("─", width)+"╯"))

	return lines
}

func renderRelationArrow(relType string) string {
	switch relType {
	case "--|>":
		return "───▷" // inheritance
	case "..|>":
		return "╌╌╌▷" // implementation
	case "--*":
		return "───◆" // composition
	case "--o":
		return "───◇" // aggregation
	case "..":
		return "╌╌╌╌" // dependency
	default:
		return "────" // association
	}
}

func parseClassDiagram(source string) ([]classInfo, []classRelation) {
	var classes []classInfo
	var relations []classRelation
	classMap := map[string]*classInfo{}

	// Regex patterns
	classDefRe := regexp.MustCompile(`(?i)^\s*class\s+(\w+)\s*\{?\s*$`)
	memberRe := regexp.MustCompile(`^\s*([\+\-\#\~]?)\s*(\w[\w\s\[\]<>,]*?)\s*(\w+)\s*(\(.*\))?\s*$`)
	relationRe := regexp.MustCompile(`^\s*(\w+)\s*(--|\.\.)([\|><\*o]*)?\s*(\w+)\s*(?::\s*(.*))?$`)
	closeBraceRe := regexp.MustCompile(`^\s*\}\s*$`)

	lines := strings.Split(source, "\n")
	var currentClass *classInfo
	inClassBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "classdiagram") {
			continue
		}

		// Close brace ends class block
		if inClassBlock && closeBraceRe.MatchString(trimmed) {
			inClassBlock = false
			currentClass = nil
			continue
		}

		// Inside class block — parse members
		if inClassBlock && currentClass != nil {
			member := strings.TrimSpace(trimmed)
			if member == "" {
				continue
			}
			// Check if it's a method (has parentheses)
			if strings.Contains(member, "(") {
				currentClass.methods = append(currentClass.methods, member)
			} else {
				currentClass.fields = append(currentClass.fields, member)
			}
			continue
		}

		// Class definition
		if cm := classDefRe.FindStringSubmatch(trimmed); cm != nil {
			name := cm[1]
			cls := classInfo{name: name}
			if strings.HasSuffix(trimmed, "{") {
				inClassBlock = true
			}
			classes = append(classes, cls)
			classMap[name] = &classes[len(classes)-1]
			currentClass = classMap[name]
			continue
		}

		// Relation
		if rm := relationRe.FindStringSubmatch(trimmed); rm != nil {
			from := rm[1]
			lineType := rm[2]
			arrowHead := rm[3]
			to := rm[4]
			label := ""
			if len(rm) > 5 {
				label = strings.TrimSpace(rm[5])
			}

			relType := lineType
			if arrowHead != "" {
				relType = lineType + arrowHead
			}

			relations = append(relations, classRelation{
				from:    from,
				to:      to,
				relType: relType,
				label:   label,
			})

			// Ensure classes exist
			if _, ok := classMap[from]; !ok {
				classes = append(classes, classInfo{name: from})
				classMap[from] = &classes[len(classes)-1]
			}
			if _, ok := classMap[to]; !ok {
				classes = append(classes, classInfo{name: to})
				classMap[to] = &classes[len(classes)-1]
			}
			continue
		}

		// Simple member definition outside braces: ClassName : member
		colonRe := regexp.MustCompile(`^\s*(\w+)\s*:\s*(.+)$`)
		if cm := colonRe.FindStringSubmatch(trimmed); cm != nil {
			className := cm[1]
			member := strings.TrimSpace(cm[2])

			// Skip if this looks like a relation label (already caught above)
			if _, ok := classMap[className]; !ok {
				cls := classInfo{name: className}
				classes = append(classes, cls)
				classMap[className] = &classes[len(classes)-1]
			}
			if strings.Contains(member, "(") {
				classMap[className].methods = append(classMap[className].methods, member)
			} else {
				classMap[className].fields = append(classMap[className].fields, member)
			}
			continue
		}

		_ = memberRe
	}

	return classes, relations
}

// ---------------------------------------------------------------------------
// Gantt chart rendering
// ---------------------------------------------------------------------------

type ganttTask struct {
	name     string
	section  string
	status   string // "done", "active", "crit", ""
	barStart float64 // normalized 0..1
	barEnd   float64 // normalized 0..1
}

func renderGanttASCII(source string, maxWidth int) []string {
	title, tasks := parseGantt(source)
	if len(tasks) == 0 {
		fallbackStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		return []string{"  " + fallbackStyle.Render("Could not render diagram")}
	}

	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	taskStyle := lipgloss.NewStyle().Foreground(text)
	sectionStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	doneStyle := lipgloss.NewStyle().Foreground(green)
	activeStyle := lipgloss.NewStyle().Foreground(blue)
	critStyle := lipgloss.NewStyle().Foreground(red)
	defaultBarStyle := lipgloss.NewStyle().Foreground(lavender)

	var result []string

	// Title
	if title != "" {
		titleStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
		result = append(result, "    "+titleStyle.Render(title))
		result = append(result, "")
	}

	// Find max task name length
	maxNameLen := 0
	for _, t := range tasks {
		if len([]rune(t.name)) > maxNameLen {
			maxNameLen = len([]rune(t.name))
		}
	}
	if maxNameLen > maxWidth/3 {
		maxNameLen = maxWidth / 3
	}

	barAreaWidth := maxWidth - maxNameLen - 12
	if barAreaWidth < 10 {
		barAreaWidth = 10
	}

	// Assign normalized positions to tasks (simple sequential layout)
	totalTasks := len(tasks)
	for i := range tasks {
		tasks[i].barStart = float64(i) / float64(totalTasks)
		tasks[i].barEnd = float64(i+1) / float64(totalTasks)
	}

	// Header timeline
	headerStyle := lipgloss.NewStyle().Foreground(overlay0)
	timelineHeader := strings.Repeat("─", barAreaWidth)
	result = append(result, "    "+strings.Repeat(" ", maxNameLen+2)+headerStyle.Render("├"+timelineHeader+"┤"))

	lastSection := ""
	for _, t := range tasks {
		// Section header
		if t.section != "" && t.section != lastSection {
			lastSection = t.section
			result = append(result, "")
			result = append(result, "    "+sectionStyle.Render("▎ "+t.section))
		}

		// Task name
		name := t.name
		nameRunes := []rune(name)
		if len(nameRunes) > maxNameLen {
			name = string(nameRunes[:maxNameLen-1]) + "…"
		}
		namePad := maxNameLen - len([]rune(name))

		// Build the bar
		barStart := int(math.Round(t.barStart * float64(barAreaWidth)))
		barEnd := int(math.Round(t.barEnd * float64(barAreaWidth)))
		if barEnd <= barStart {
			barEnd = barStart + 1
		}
		if barEnd > barAreaWidth {
			barEnd = barAreaWidth
		}

		var barBuf strings.Builder
		preSpace := barStart
		barLen := barEnd - barStart
		postSpace := barAreaWidth - barEnd

		if preSpace > 0 {
			barBuf.WriteString(strings.Repeat(" ", preSpace))
		}

		barStr := strings.Repeat("█", barLen)
		switch t.status {
		case "done":
			barBuf.WriteString(doneStyle.Render(barStr))
		case "active":
			barBuf.WriteString(activeStyle.Render(barStr))
		case "crit":
			barBuf.WriteString(critStyle.Render(barStr))
		default:
			barBuf.WriteString(defaultBarStyle.Render(barStr))
		}

		if postSpace > 0 {
			barBuf.WriteString(strings.Repeat(" ", postSpace))
		}

		// Status indicator
		statusIcon := " "
		switch t.status {
		case "done":
			statusIcon = doneStyle.Render("✓")
		case "active":
			statusIcon = activeStyle.Render("●")
		case "crit":
			statusIcon = critStyle.Render("!")
		}

		line := "    " +
			taskStyle.Render(name+strings.Repeat(" ", namePad)) +
			" " + statusIcon + " " +
			borderStyle.Render("│") +
			barBuf.String() +
			borderStyle.Render("│")
		result = append(result, line)
	}

	// Footer timeline
	result = append(result, "    "+strings.Repeat(" ", maxNameLen+2)+headerStyle.Render("├"+timelineHeader+"┤"))

	return result
}

func parseGantt(source string) (string, []ganttTask) {
	var tasks []ganttTask
	title := ""
	currentSection := ""

	// dateFormat line (we note it but don't use it for ASCII rendering)
	titleRe := regexp.MustCompile(`(?i)^\s*title\s+(.+)$`)
	sectionRe := regexp.MustCompile(`(?i)^\s*section\s+(.+)$`)
	// Task line: TaskName :status, id, after prev, duration
	// Simplified: TaskName :done, 2024-01-01, 30d
	// Or just: TaskName :2024-01-01, 30d
	// Or: TaskName :active, a1, 2024-01-01, 30d
	taskRe := regexp.MustCompile(`^\s*(.+?)\s*:\s*(.+)$`)

	lines := strings.Split(source, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "gantt") {
			continue
		}
		if strings.HasPrefix(lower, "dateformat") {
			continue
		}
		if strings.HasPrefix(lower, "axisformat") {
			continue
		}
		if strings.HasPrefix(lower, "todaymarker") {
			continue
		}
		if strings.HasPrefix(lower, "excludes") {
			continue
		}

		// Title
		if tm := titleRe.FindStringSubmatch(trimmed); tm != nil {
			title = strings.TrimSpace(tm[1])
			continue
		}

		// Section
		if sm := sectionRe.FindStringSubmatch(trimmed); sm != nil {
			currentSection = strings.TrimSpace(sm[1])
			continue
		}

		// Task
		if tm := taskRe.FindStringSubmatch(trimmed); tm != nil {
			taskName := strings.TrimSpace(tm[1])
			params := strings.TrimSpace(tm[2])

			// Detect status from params
			status := ""
			parts := strings.Split(params, ",")
			for _, p := range parts {
				p = strings.TrimSpace(strings.ToLower(p))
				if p == "done" {
					status = "done"
				} else if p == "active" {
					status = "active"
				} else if p == "crit" {
					status = "crit"
				}
			}

			tasks = append(tasks, ganttTask{
				name:    taskName,
				section: currentSection,
				status:  status,
			})
		}
	}

	return title, tasks
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

// stripAnsi removes ANSI escape codes for width calculation.
func stripAnsi(s string) string {
	re := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	return re.ReplaceAllString(s, "")
}

// runeWidth returns the rune-based display width of a string.
func runeWidth(s string) int {
	return len([]rune(s))
}
