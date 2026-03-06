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
// Supports: flowcharts (graph TD/LR/TB/BT), sequence diagrams, pie charts.
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
	default:
		fallbackStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		body = append(body, "  "+fallbackStyle.Render("Could not render diagram"))
	}

	lines = append(lines, body...)
	lines = append(lines, "")
	lines = append(lines, topBorder)

	return lines
}

// parseMermaidType returns the diagram type: "flowchart", "sequence", "pie", or "unknown".
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

	// Build ordered list of node IDs preserving first-seen order
	seen := map[string]bool{}
	var ordered []string
	for _, e := range edges {
		if !seen[e.from] {
			seen[e.from] = true
			ordered = append(ordered, e.from)
		}
		if !seen[e.to] {
			seen[e.to] = true
			ordered = append(ordered, e.to)
		}
	}
	// Add any nodes that have no edges
	for id := range nodes {
		if !seen[id] {
			ordered = append(ordered, id)
		}
	}

	// Build edge lookup from->to for labels
	edgeLabels := map[string]string{} // "from->to" => label
	for _, e := range edges {
		key := e.from + "->" + e.to
		edgeLabels[key] = e.label
	}

	isVertical := direction == "TD" || direction == "TB" || direction == "BT"

	if isVertical {
		return renderFlowchartVertical(ordered, nodes, edges, edgeLabels, direction, maxWidth)
	}
	return renderFlowchartHorizontal(ordered, nodes, edges, edgeLabels, direction, maxWidth)
}

func renderFlowchartVertical(ordered []string, nodes map[string]mermaidNode, edges []mermaidEdge, edgeLabels map[string]string, direction string, maxWidth int) []string {
	var result []string

	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	textStyle := lipgloss.NewStyle().Foreground(text)
	arrowStyle := lipgloss.NewStyle().Foreground(blue)
	labelStyle := lipgloss.NewStyle().Foreground(peach)

	// If BT (bottom-top), reverse the node order
	if direction == "BT" {
		reversed := make([]string, len(ordered))
		for i, id := range ordered {
			reversed[len(ordered)-1-i] = id
		}
		ordered = reversed
	}

	for i, id := range ordered {
		node := nodes[id]
		label := node.label
		if label == "" {
			label = id
		}

		boxLines := renderNodeBox(label, node.shape, maxWidth-8, borderStyle, textStyle)
		for _, bl := range boxLines {
			result = append(result, "    "+bl)
		}

		// Draw connector to next node
		if i < len(ordered)-1 {
			nextID := ordered[i+1]
			edgeKey := id + "->" + nextID
			if direction == "BT" {
				edgeKey = nextID + "->" + id
			}
			edgeLabel := edgeLabels[edgeKey]

			// Find the center of the box for the arrow
			if edgeLabel != "" {
				result = append(result, "      "+arrowStyle.Render("│"))
				result = append(result, "      "+labelStyle.Render(edgeLabel))
				result = append(result, "      "+arrowStyle.Render("│"))
			} else {
				result = append(result, "      "+arrowStyle.Render("│"))
			}
			if direction == "BT" {
				result = append(result, "      "+arrowStyle.Render("▲"))
			} else {
				result = append(result, "      "+arrowStyle.Render("▼"))
			}
		}
	}

	return result
}

func renderFlowchartHorizontal(ordered []string, nodes map[string]mermaidNode, edges []mermaidEdge, edgeLabels map[string]string, direction string, maxWidth int) []string {
	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	textStyle := lipgloss.NewStyle().Foreground(text)
	arrowStyle := lipgloss.NewStyle().Foreground(blue)
	labelStyle := lipgloss.NewStyle().Foreground(peach)

	isRL := direction == "RL"

	if isRL {
		reversed := make([]string, len(ordered))
		for i, id := range ordered {
			reversed[len(ordered)-1-i] = id
		}
		ordered = reversed
	}

	// Calculate max node width to fit
	numNodes := len(ordered)
	if numNodes == 0 {
		return nil
	}
	arrowWidth := 5 // " ──► "
	availPerNode := (maxWidth - 4 - (numNodes-1)*arrowWidth) / numNodes
	if availPerNode < 5 {
		availPerNode = 5
	}

	// Build three rows: top border, text, bottom border
	var topRow, midRow, botRow strings.Builder
	topRow.WriteString("    ")
	midRow.WriteString("    ")
	botRow.WriteString("    ")

	for i, id := range ordered {
		node := nodes[id]
		label := node.label
		if label == "" {
			label = id
		}

		// Truncate label if needed
		if len(label) > availPerNode-4 {
			label = label[:availPerNode-5] + "…"
		}

		innerWidth := len([]rune(label)) + 2
		if innerWidth < 5 {
			innerWidth = 5
		}

		topRow.WriteString(borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮"))
		midRow.WriteString(borderStyle.Render("│") + textStyle.Render(" "+label+" ") + borderStyle.Render("│"))
		botRow.WriteString(borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯"))

		if i < numNodes-1 {
			nextID := ordered[i+1]
			edgeKey := id + "->" + nextID
			if isRL {
				edgeKey = nextID + "->" + id
			}
			edgeLabel := edgeLabels[edgeKey]
			_ = edgeLabel // labels rendered on a separate line for horizontal

			if isRL {
				topRow.WriteString("     ")
				midRow.WriteString(arrowStyle.Render(" ◄── "))
				botRow.WriteString("     ")
			} else {
				topRow.WriteString("     ")
				midRow.WriteString(arrowStyle.Render(" ──► "))
				botRow.WriteString("     ")
			}
		}
	}

	result := []string{topRow.String(), midRow.String(), botRow.String()}

	// Render edge labels below for horizontal layout
	var labelLine strings.Builder
	hasLabels := false
	labelLine.WriteString("    ")
	for i, id := range ordered {
		node := nodes[id]
		label := node.label
		if label == "" {
			label = id
		}
		if len(label) > availPerNode-4 {
			label = label[:availPerNode-5] + "…"
		}
		innerWidth := len([]rune(label)) + 2
		if innerWidth < 5 {
			innerWidth = 5
		}
		labelLine.WriteString(strings.Repeat(" ", innerWidth+2))

		if i < numNodes-1 {
			nextID := ordered[i+1]
			edgeKey := id + "->" + nextID
			if isRL {
				edgeKey = nextID + "->" + id
			}
			el := edgeLabels[edgeKey]
			if el != "" {
				hasLabels = true
				pad := 5 - len([]rune(el))
				if pad < 0 {
					pad = 0
				}
				labelLine.WriteString(labelStyle.Render(el) + strings.Repeat(" ", pad))
			} else {
				labelLine.WriteString("     ")
			}
		}
	}
	if hasLabels {
		result = append(result, labelLine.String())
	}

	return result
}

func renderNodeBox(label string, shape string, maxW int, borderStyle, textStyle lipgloss.Style) []string {
	rLabel := []rune(label)
	if len(rLabel) > maxW-4 {
		rLabel = append(rLabel[:maxW-5], '…')
	}
	displayLabel := string(rLabel)
	innerWidth := len(rLabel) + 2
	if innerWidth < 5 {
		innerWidth = 5
	}
	padRight := innerWidth - len(rLabel) - 1
	if padRight < 0 {
		padRight = 0
	}

	switch shape {
	case "diamond":
		// Diamond shape with < > markers
		topW := innerWidth + 2
		topLine := borderStyle.Render("  ◇" + strings.Repeat("─", topW) + "◇")
		midLine := borderStyle.Render("◁ ") + textStyle.Render(" "+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render(" ▷")
		botLine := borderStyle.Render("  ◇" + strings.Repeat("─", topW) + "◇")
		return []string{topLine, midLine, botLine}
	case "circle":
		topLine := borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")
		midLine := borderStyle.Render("│") + textStyle.Render(" "+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render("│")
		botLine := borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
		return []string{topLine, midLine, botLine}
	case "round":
		topLine := borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")
		midLine := borderStyle.Render("(") + textStyle.Render(" "+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render(")")
		botLine := borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
		return []string{topLine, midLine, botLine}
	case "asymmetric":
		topLine := borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")
		midLine := borderStyle.Render("▶") + textStyle.Render(" "+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render("│")
		botLine := borderStyle.Render("╰" + strings.Repeat("─", innerWidth) + "╯")
		return []string{topLine, midLine, botLine}
	default: // "rect"
		topLine := borderStyle.Render("╭" + strings.Repeat("─", innerWidth) + "╮")
		midLine := borderStyle.Render("│") + textStyle.Render(" "+displayLabel+strings.Repeat(" ", padRight)) + borderStyle.Render("│")
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
	from    string
	to      string
	message string
	dashed  bool
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

	// Calculate column positions for each participant
	numP := len(participants)
	spacing := (maxWidth - 8) / numP
	if spacing < 12 {
		spacing = 12
	}

	partMap := map[string]*seqParticipant{}
	for i := range participants {
		participants[i].col = 4 + spacing/2 + i*spacing
		partMap[participants[i].name] = &participants[i]
	}

	var result []string

	// Draw participant boxes at top
	topLine, midLine, botLine := buildParticipantRow(participants, spacing, borderStyle, nameStyle)
	result = append(result, topLine, midLine, botLine)

	// Process messages and notes
	allEvents := buildEventList(messages, notes)
	for _, evt := range allEvents {
		// Draw a lifeline row
		lifeRow := buildLifeline(participants, spacing, lifelineStyle)
		result = append(result, lifeRow)

		if evt.isNote {
			// Render note
			noteRow := buildNoteRow(evt.note, partMap, spacing, borderStyle, noteStyle)
			result = append(result, noteRow)
		} else {
			// Render message arrow
			msgRow := buildMessageRow(evt.msg, partMap, spacing, arrowStyle, msgStyle)
			result = append(result, msgRow)
		}
	}

	// Final lifeline
	lifeRow := buildLifeline(participants, spacing, lifelineStyle)
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
		// Insert any notes that appear before this message (simplified: interleave notes)
		if noteIdx < len(notes) {
			events = append(events, seqEvent{isNote: true, note: notes[noteIdx]})
			noteIdx++
		}
		events = append(events, seqEvent{isNote: false, msg: m})
	}
	// Remaining notes
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
		topParts = append(topParts, strings.Repeat(" ", padLeft)+borderStyle.Render("╭"+strings.Repeat("─", boxW)+"╮")+strings.Repeat(" ", padRight))
		innerPad := boxW - nameLen - 1
		if innerPad < 0 {
			innerPad = 0
		}
		midParts = append(midParts, strings.Repeat(" ", padLeft)+borderStyle.Render("│")+nameStyle.Render(" "+p.name+strings.Repeat(" ", innerPad))+borderStyle.Render("│")+strings.Repeat(" ", padRight))
		botParts = append(botParts, strings.Repeat(" ", padLeft)+borderStyle.Render("╰"+strings.Repeat("─", boxW)+"╯")+strings.Repeat(" ", padRight))
	}
	return "    " + strings.Join(topParts, ""),
		"    " + strings.Join(midParts, ""),
		"    " + strings.Join(botParts, "")
}

func buildLifeline(participants []seqParticipant, spacing int, lifelineStyle lipgloss.Style) string {
	var parts []string
	for range participants {
		center := spacing / 2
		left := center
		right := spacing - center - 1
		parts = append(parts, strings.Repeat(" ", left)+lifelineStyle.Render("│")+strings.Repeat(" ", right))
	}
	return "    " + strings.Join(parts, "")
}

func buildMessageRow(msg seqMessage, partMap map[string]*seqParticipant, spacing int, arrowStyle, msgStyle lipgloss.Style) string {
	fromP, fromOK := partMap[msg.from]
	toP, toOK := partMap[msg.to]
	if !fromOK || !toOK {
		return "    " + msgStyle.Render(msg.message)
	}

	fromCol := fromP.col
	toCol := toP.col

	leftToRight := fromCol < toCol
	minCol := fromCol
	maxCol := toCol
	if !leftToRight {
		minCol = toCol
		maxCol = fromCol
	}

	arrowLen := maxCol - minCol
	if arrowLen < 3 {
		arrowLen = 3
	}

	// Build arrow line
	var arrow string
	if msg.dashed {
		if leftToRight {
			body := strings.Repeat("─ ", (arrowLen-1)/2)
			if len([]rune(body)) > arrowLen-1 {
				body = string([]rune(body)[:arrowLen-1])
			}
			arrow = body + "►"
		} else {
			body := strings.Repeat(" ─", (arrowLen-1)/2)
			if len([]rune(body)) > arrowLen-1 {
				body = string([]rune(body)[:arrowLen-1])
			}
			arrow = "◄" + body
		}
	} else {
		if leftToRight {
			arrow = strings.Repeat("─", arrowLen-1) + "►"
		} else {
			arrow = "◄" + strings.Repeat("─", arrowLen-1)
		}
	}

	_ = arrow // arrow is pre-computed but the simplified renderer below builds inline

	// Render label above the arrow
	labelStr := ""
	if msg.message != "" {
		labelStr = msgStyle.Render(msg.message)
	}

	// Position the arrow using the participant index positions
	fromIdx := -1
	toIdx := -1
	for name, p := range partMap {
		if name == msg.from {
			fromIdx = p.col
		}
		if name == msg.to {
			toIdx = p.col
		}
	}
	_ = fromIdx
	_ = toIdx

	// Simplified row: render all participants as lifelines, draw arrow in between
	numParticipants := len(partMap)
	orderedParts := make([]string, 0, numParticipants)
	orderedNames := make([]string, 0, numParticipants)
	for name := range partMap {
		orderedNames = append(orderedNames, name)
	}
	sort.Slice(orderedNames, func(i, j int) bool {
		return partMap[orderedNames[i]].col < partMap[orderedNames[j]].col
	})
	_ = orderedParts

	// Build a simple text representation
	var line strings.Builder
	line.WriteString("    ")

	fromPartIdx := -1
	toPartIdx := -1
	for idx, name := range orderedNames {
		if name == msg.from {
			fromPartIdx = idx
		}
		if name == msg.to {
			toPartIdx = idx
		}
	}

	center := spacing / 2

	for idx := range orderedNames {
		if idx == fromPartIdx || idx == toPartIdx {
			// Part of the arrow range
			if fromPartIdx < toPartIdx {
				// Left to right
				if idx == fromPartIdx {
					left := center
					arrowSegLen := spacing - center
					if idx+1 == toPartIdx {
						// Direct connection
						arrowSeg := strings.Repeat("─", arrowSegLen-1)
						line.WriteString(strings.Repeat(" ", left))
						line.WriteString(arrowStyle.Render(arrowSeg))
					} else {
						arrowSeg := strings.Repeat("─", arrowSegLen)
						line.WriteString(strings.Repeat(" ", left))
						line.WriteString(arrowStyle.Render(arrowSeg))
					}
				} else if idx == toPartIdx {
					arrowSeg := strings.Repeat("─", center-1) + "►"
					line.WriteString(arrowStyle.Render(arrowSeg))
					right := spacing - center - 1
					if right > 0 {
						line.WriteString(strings.Repeat(" ", right))
					}
				} else {
					// Middle participant -- arrow passes through
					arrowSeg := strings.Repeat("─", spacing)
					line.WriteString(arrowStyle.Render(arrowSeg))
				}
			} else {
				// Right to left
				if idx == toPartIdx {
					left := center - 1
					line.WriteString(strings.Repeat(" ", left))
					arrowSeg := "◄" + strings.Repeat("─", spacing-center-1)
					line.WriteString(arrowStyle.Render(arrowSeg))
				} else if idx == fromPartIdx {
					arrowSeg := strings.Repeat("─", center)
					line.WriteString(arrowStyle.Render(arrowSeg))
					right := spacing - center - 1
					if right > 0 {
						line.WriteString(strings.Repeat(" ", right))
					}
				} else {
					arrowSeg := strings.Repeat("─", spacing)
					line.WriteString(arrowStyle.Render(arrowSeg))
				}
			}
		} else {
			// Not part of arrow, just lifeline
			left := center
			right := spacing - center - 1
			if (fromPartIdx < toPartIdx && idx > fromPartIdx && idx < toPartIdx) ||
				(fromPartIdx > toPartIdx && idx > toPartIdx && idx < fromPartIdx) {
				// Between from and to: arrow passes through
				arrowSeg := strings.Repeat("─", spacing)
				line.WriteString(arrowStyle.Render(arrowSeg))
			} else {
				line.WriteString(strings.Repeat(" ", left) + lipgloss.NewStyle().Foreground(surface1).Render("│") + strings.Repeat(" ", right))
			}
		}
	}

	// For the label, build a second line above
	var lines []string
	if labelStr != "" {
		var labelLine strings.Builder
		labelLine.WriteString("    ")
		labelPos := 0
		if fromPartIdx < toPartIdx {
			labelPos = fromPartIdx
		} else {
			labelPos = toPartIdx
		}
		preSpaces := labelPos*spacing + center + 1
		labelLine.WriteString(strings.Repeat(" ", preSpaces))
		labelLine.WriteString(labelStr)
		lines = append(lines, labelLine.String())
	}
	lines = append(lines, line.String())

	return strings.Join(lines, "\n")
}

func buildNoteRow(note seqNote, partMap map[string]*seqParticipant, spacing int, borderStyle, noteStyle lipgloss.Style) string {
	p, ok := partMap[note.over]
	if !ok {
		return "    " + noteStyle.Render("[Note: "+note.text+"]")
	}

	_ = p

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

	center := spacing / 2
	noteText := note.text
	noteW := len([]rune(noteText)) + 2
	noteOffset := partIdx*spacing + center - noteW/2
	if noteOffset < 0 {
		noteOffset = 0
	}

	var lines []string
	topLine := strings.Repeat(" ", 4+noteOffset) + borderStyle.Render("┌"+strings.Repeat("─", noteW)+"┐")
	midLine := strings.Repeat(" ", 4+noteOffset) + borderStyle.Render("│") + noteStyle.Render(" "+noteText+" ") + borderStyle.Render("│")
	botLine := strings.Repeat(" ", 4+noteOffset) + borderStyle.Render("└"+strings.Repeat("─", noteW)+"┘")
	lines = append(lines, topLine, midLine, botLine)

	return strings.Join(lines, "\n")
}

func parseSequenceDiagram(source string) ([]seqParticipant, []seqMessage, []seqNote) {
	var participants []seqParticipant
	var messages []seqMessage
	var notes []seqNote

	participantSet := map[string]bool{}

	participantRe := regexp.MustCompile(`(?i)^\s*participant\s+(\S+)`)
	// Matches: A->>B: msg, A-->>B: msg, A->>+B: msg, A->>-B: msg
	messageRe := regexp.MustCompile(`^\s*(\S+?)\s*(->>|-->>|->>[\+\-]|-->>[\+\-]|->|-->)\s*[\+\-]?(\S+?)\s*:\s*(.*)$`)
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
			// Strip trailing comma if present (e.g. "Note over A,B")
			over = strings.TrimRight(over, ",")
			notes = append(notes, seqNote{over: over, text: strings.TrimSpace(nm[3])})
			// Ensure participant exists
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
			to := mm[3]
			msg := strings.TrimSpace(mm[4])

			dashed := strings.Contains(arrow, "--")

			messages = append(messages, seqMessage{
				from:    from,
				to:      to,
				message: msg,
				dashed:  dashed,
			})

			// Auto-add participants
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

// renderPieASCII renders a pie chart as horizontal bar chart.
func renderPieASCII(source string, maxWidth int) []string {
	slices := parsePieChart(source)
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
		total = 1 // avoid division by zero
	}
	for i := range slices {
		slices[i].percent = (slices[i].value / total) * 100.0
	}

	// Color palette cycling through catppuccin accent colors
	barColors := []lipgloss.Color{blue, green, peach, yellow, pink, teal}

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
	barAreaWidth := maxWidth - 6 - maxLabelLen - percentWidth - 4
	if barAreaWidth < 10 {
		barAreaWidth = 10
	}

	var result []string

	for i, s := range slices {
		colorIdx := i % len(barColors)
		barStyle := lipgloss.NewStyle().Foreground(barColors[colorIdx])
		labelStyle := lipgloss.NewStyle().Foreground(text)
		pctStyle := lipgloss.NewStyle().Foreground(peach)

		// Truncate label
		label := s.label
		labelRunes := []rune(label)
		if len(labelRunes) > maxLabelLen {
			label = string(labelRunes[:maxLabelLen-1]) + "…"
		}
		// Pad label
		labelPad := maxLabelLen - len([]rune(label))

		// Calculate bar length
		barLen := int(math.Round(s.percent / 100.0 * float64(barAreaWidth)))
		if barLen < 1 && s.value > 0 {
			barLen = 1
		}

		// Build bar with block characters
		bar := buildBarString(barLen, barAreaWidth)

		pctStr := fmt.Sprintf("%5.1f%%", s.percent)

		line := "    " +
			labelStyle.Render(label+strings.Repeat(" ", labelPad)) +
			"  " +
			barStyle.Render(bar) +
			" " +
			pctStyle.Render(pctStr)
		result = append(result, line)
	}

	// Total line
	totalStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
	totalVal := fmt.Sprintf("Total: %.0f", total)
	result = append(result, "")
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

func parsePieChart(source string) []pieSlice {
	var slices []pieSlice

	// Also look for a title line
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
		if strings.HasPrefix(lower, "title") {
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

	return slices
}
