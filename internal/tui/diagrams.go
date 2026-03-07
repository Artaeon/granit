package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Custom Diagram Engine
//
// Renders ```diagram code blocks in view mode. Supports 5 diagram types
// optimized for terminal rendering:
//
//   - sequence:   Linear flow of connected steps (combos, workflows)
//   - tree:       Branching decision diagram
//   - movement:   Grid-based position/footwork diagram
//   - timeline:   Horizontal timeline with labeled events
//   - comparison: Side-by-side comparison table
//
// Syntax:
//
//	```diagram
//	type: sequence
//	title: My Combo
//	Jab > Cross > Hook > Low Kick
//	```
//
// ---------------------------------------------------------------------------

// RenderDiagramASCII takes diagram source and maxWidth, returns styled lines.
func RenderDiagramASCII(source string, maxWidth int) []string {
	if maxWidth < 20 {
		maxWidth = 20
	}

	dtype, title, body := parseDiagramBlock(source)

	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	borderStyle := lipgloss.NewStyle().Foreground(surface1)
	titleStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)

	var lines []string

	// Top border
	topBorder := borderStyle.Render("  " + strings.Repeat("─", maxWidth-4))
	lines = append(lines, topBorder)

	// Diagram type label
	label := "Diagram"
	switch dtype {
	case "sequence":
		label = "Sequence"
	case "tree":
		label = "Decision Tree"
	case "movement":
		label = "Movement"
	case "timeline":
		label = "Timeline"
	case "comparison":
		label = "Comparison"
	}
	lines = append(lines, "  "+headerStyle.Render(label))

	// Title
	if title != "" {
		lines = append(lines, "  "+titleStyle.Render(title))
	}
	lines = append(lines, "")

	// Render body based on type
	var bodyLines []string
	switch dtype {
	case "sequence":
		bodyLines = renderSequenceDiagram(body, maxWidth)
	case "tree":
		bodyLines = renderTreeDiagram(body, maxWidth)
	case "movement":
		bodyLines = renderMovementDiagram(body, maxWidth)
	case "timeline":
		bodyLines = renderTimelineDiagram(body, maxWidth)
	case "comparison":
		bodyLines = renderComparisonDiagram(body, maxWidth)
	default:
		// Fallback: render as sequence if there are > separators, else as tree
		if strings.Contains(strings.Join(body, " "), " > ") {
			bodyLines = renderSequenceDiagram(body, maxWidth)
		} else {
			bodyLines = renderTreeDiagram(body, maxWidth)
		}
	}

	lines = append(lines, bodyLines...)
	lines = append(lines, "")

	// Bottom border
	bottomBorder := borderStyle.Render("  " + strings.Repeat("─", maxWidth-4))
	lines = append(lines, bottomBorder)

	return lines
}

// parseDiagramBlock extracts type, title, and body lines from diagram source.
func parseDiagramBlock(source string) (dtype, title string, body []string) {
	rawLines := strings.Split(source, "\n")

	for _, line := range rawLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "type:") {
			dtype = strings.TrimSpace(trimmed[5:])
			dtype = strings.ToLower(dtype)
			continue
		}
		if strings.HasPrefix(lower, "title:") {
			title = strings.TrimSpace(trimmed[6:])
			continue
		}

		body = append(body, line)
	}

	return dtype, title, body
}

// ---------------------------------------------------------------------------
// Sequence Diagram — connected boxes in rows
// ---------------------------------------------------------------------------

func renderSequenceDiagram(body []string, maxWidth int) []string {
	boxStyle := lipgloss.NewStyle().Foreground(blue)
	arrowStyle := lipgloss.NewStyle().Foreground(green)
	labelStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)

	var allLines []string

	for _, line := range body {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Split by > separator
		steps := strings.Split(trimmed, ">")
		var names []string
		for _, s := range steps {
			s = strings.TrimSpace(s)
			if s != "" {
				names = append(names, s)
			}
		}

		if len(names) == 0 {
			continue
		}

		// Calculate box width based on longest step name
		maxNameLen := 0
		for _, n := range names {
			if len(n) > maxNameLen {
				maxNameLen = len(n)
			}
		}
		boxW := maxNameLen + 4
		if boxW < 7 {
			boxW = 7
		}

		arrowStr := "──→"
		arrowW := 3

		// Calculate how many boxes fit per row
		totalPerBox := boxW + arrowW
		perRow := (maxWidth - 6) / totalPerBox
		if perRow < 1 {
			perRow = 1
		}

		// Render in rows
		for rowStart := 0; rowStart < len(names); rowStart += perRow {
			rowEnd := rowStart + perRow
			if rowEnd > len(names) {
				rowEnd = len(names)
			}
			chunk := names[rowStart:rowEnd]

			// Top of boxes: ╭───╮
			topLine := "    "
			for i, _ := range chunk {
				topLine += boxStyle.Render("╭" + strings.Repeat("─", boxW-2) + "╮")
				if i < len(chunk)-1 {
					topLine += "   " // arrow space
				}
			}
			allLines = append(allLines, topLine)

			// Middle: │ Name │──→
			midLine := "    "
			for i, name := range chunk {
				padded := centerPad(name, boxW-4)
				midLine += boxStyle.Render("│") + " " + labelStyle.Render(padded) + " " + boxStyle.Render("│")
				if i < len(chunk)-1 {
					midLine += arrowStyle.Render(arrowStr)
				} else if rowEnd < len(names) {
					// Show continuation arrow down
					midLine += " " + dimStyle.Render("↓")
				}
			}
			allLines = append(allLines, midLine)

			// Bottom: ╰───╯
			botLine := "    "
			for i, _ := range chunk {
				botLine += boxStyle.Render("╰" + strings.Repeat("─", boxW-2) + "╯")
				if i < len(chunk)-1 {
					botLine += "   " // arrow space
				}
			}
			allLines = append(allLines, botLine)

			// If there's a next row, show connector
			if rowEnd < len(names) {
				// Connecting arrow from end of this row to start of next
				connLine := "    " + dimStyle.Render("↓")
				allLines = append(allLines, connLine)
			}
		}

		allLines = append(allLines, "")
	}

	// Trim trailing empty line
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
		allLines = allLines[:len(allLines)-1]
	}

	return allLines
}

// ---------------------------------------------------------------------------
// Tree Diagram — branching structure
// ---------------------------------------------------------------------------

func renderTreeDiagram(body []string, maxWidth int) []string {
	rootStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	branchStyle := lipgloss.NewStyle().Foreground(blue)
	nodeStyle := lipgloss.NewStyle().Foreground(green)
	textStyle := lipgloss.NewStyle().Foreground(text)
	treeLineStyle := lipgloss.NewStyle().Foreground(surface1)

	var allLines []string

	for _, line := range body {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Calculate indent level
		indent := 0
		for _, ch := range line {
			if ch == ' ' {
				indent++
			} else if ch == '\t' {
				indent += 2
			} else {
				break
			}
		}
		level := indent / 2

		// Check if it's a branch line (starts with >)
		isBranch := strings.HasPrefix(trimmed, ">")
		if isBranch {
			trimmed = strings.TrimSpace(trimmed[1:])
			if level == 0 {
				level = 1
			}
		}

		// Split on : for label: description
		var label, desc string
		if colonIdx := strings.Index(trimmed, ":"); colonIdx > 0 && !isBranch {
			label = strings.TrimSpace(trimmed[:colonIdx])
			desc = strings.TrimSpace(trimmed[colonIdx+1:])
		} else if colonIdx := strings.Index(trimmed, ":"); colonIdx > 0 && isBranch {
			label = strings.TrimSpace(trimmed[:colonIdx])
			desc = strings.TrimSpace(trimmed[colonIdx+1:])
		} else {
			label = trimmed
		}

		// Check if description contains sub-steps with >
		var subSteps []string
		if desc != "" && strings.Contains(desc, " > ") {
			subSteps = strings.Split(desc, " > ")
			for i := range subSteps {
				subSteps[i] = strings.TrimSpace(subSteps[i])
			}
			desc = ""
		}

		// Build the tree prefix
		prefix := "    "
		if level == 0 {
			// Root node
			allLines = append(allLines, prefix+rootStyle.Render("● "+label))
			if desc != "" {
				allLines = append(allLines, prefix+"  "+textStyle.Render(desc))
			}
		} else {
			// Branch node
			treePrefix := prefix
			for l := 0; l < level-1; l++ {
				treePrefix += treeLineStyle.Render("│") + "   "
			}
			treePrefix += treeLineStyle.Render("├──") + " "

			rendered := treePrefix + branchStyle.Render(label)
			if desc != "" {
				rendered += textStyle.Render(": "+desc)
			}
			allLines = append(allLines, rendered)

			// Sub-steps chain
			if len(subSteps) > 0 {
				subPrefix := prefix
				for l := 0; l < level-1; l++ {
					subPrefix += treeLineStyle.Render("│") + "   "
				}
				subPrefix += treeLineStyle.Render("│") + "   "

				chain := subPrefix
				for si, step := range subSteps {
					chain += nodeStyle.Render(step)
					if si < len(subSteps)-1 {
						chain += textStyle.Render(" → ")
					}
				}
				allLines = append(allLines, chain)
			}
		}
	}

	return allLines
}

// ---------------------------------------------------------------------------
// Movement Diagram — grid-based position/footwork
// ---------------------------------------------------------------------------

func renderMovementDiagram(body []string, maxWidth int) []string {
	posStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	arrowStyle := lipgloss.NewStyle().Foreground(green)
	gridStyle := lipgloss.NewStyle().Foreground(surface1)
	labelStyle := lipgloss.NewStyle().Foreground(blue)
	textStyle := lipgloss.NewStyle().Foreground(text)

	var allLines []string

	// Movement diagram uses special characters:
	// @ or O = current position (rendered as ●)
	// ^ v < > = directional arrows
	// . = empty grid cell
	// * = target/destination
	// # = obstacle/boundary
	// Text labels can follow grid lines

	// Check if body uses grid syntax (contains . or @ or ^ v < >)
	isGrid := false
	for _, line := range body {
		trimmed := strings.TrimSpace(line)
		if strings.ContainsAny(trimmed, ".@^v<>*#O") && !strings.Contains(trimmed, ":") {
			isGrid = true
			break
		}
	}

	if isGrid {
		// Render as visual grid
		for _, line := range body {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			rendered := "    "
			runes := []rune(trimmed)
			for _, ch := range runes {
				switch ch {
				case '.':
					rendered += gridStyle.Render(" · ")
				case '@', 'O':
					rendered += posStyle.Render(" ● ")
				case '^':
					rendered += arrowStyle.Render(" ↑ ")
				case 'v', 'V':
					rendered += arrowStyle.Render(" ↓ ")
				case '<':
					rendered += arrowStyle.Render(" ← ")
				case '>':
					rendered += arrowStyle.Render(" → ")
				case '*':
					rendered += posStyle.Render(" ★ ")
				case '#':
					rendered += gridStyle.Render(" ▓ ")
				case ' ':
					// skip spaces between grid chars
				default:
					rendered += textStyle.Render(string(ch))
				}
			}
			allLines = append(allLines, rendered)
		}
	} else {
		// Render as step-by-step movement list
		stepNum := 1
		for _, line := range body {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}

			// Parse direction and label
			var icon string
			label := trimmed

			// Check for direction prefix
			dirPrefixes := map[string]string{
				"->":  "→", "-->": "→", "=>": "→",
				"<-":  "←", "<--": "←", "<=": "←",
				"up":  "↑", "down": "↓", "left": "←", "right": "→",
				"fwd": "↑", "back": "↓",
				"cw":  "↻", "ccw": "↺",
			}

			lower := strings.ToLower(trimmed)
			for prefix, arrow := range dirPrefixes {
				if strings.HasPrefix(lower, prefix+" ") {
					icon = arrow
					label = strings.TrimSpace(trimmed[len(prefix):])
					break
				}
			}

			// Check for emoji direction at start
			if len([]rune(trimmed)) > 0 {
				first := string([]rune(trimmed)[0])
				switch first {
				case "→", "←", "↑", "↓", "↗", "↘", "↙", "↖", "↻", "↺":
					icon = first
					label = strings.TrimSpace(string([]rune(trimmed)[1:]))
				}
			}

			if icon == "" {
				icon = "●"
			}

			prefix := "    "
			numStr := labelStyle.Render(string(rune('0'+stepNum)) + ".")
			allLines = append(allLines, prefix+numStr+" "+arrowStyle.Render(icon)+" "+textStyle.Render(label))
			stepNum++
		}
	}

	return allLines
}

// ---------------------------------------------------------------------------
// Timeline Diagram — horizontal timeline with events
// ---------------------------------------------------------------------------

func renderTimelineDiagram(body []string, maxWidth int) []string {
	timeStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	eventStyle := lipgloss.NewStyle().Foreground(text)
	lineStyle := lipgloss.NewStyle().Foreground(surface1)
	dotStyle := lipgloss.NewStyle().Foreground(green).Bold(true)

	var allLines []string

	type event struct {
		time  string
		label string
	}

	var events []event

	for _, line := range body {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Parse "TIME LABEL" or "TIME: LABEL" or "TIME | LABEL"
		var t, l string

		if colonIdx := strings.Index(trimmed, ":"); colonIdx > 0 && colonIdx < 10 {
			// Check if it's a time format like "1:30" vs "Label: desc"
			beforeColon := trimmed[:colonIdx]
			afterColon := trimmed[colonIdx+1:]

			// If after colon starts with digits, it might be "1:30 Label"
			afterTrimmed := strings.TrimSpace(afterColon)
			if len(afterTrimmed) > 0 && afterTrimmed[0] >= '0' && afterTrimmed[0] <= '9' {
				// Looks like time format "M:SS rest"
				// Find the next space after the full time
				fullTime := trimmed[:colonIdx+1]
				rest := afterColon
				spaceIdx := strings.Index(rest, " ")
				if spaceIdx > 0 {
					fullTime += rest[:spaceIdx]
					l = strings.TrimSpace(rest[spaceIdx:])
				} else {
					fullTime += rest
				}
				t = strings.TrimSpace(fullTime)
			} else {
				t = strings.TrimSpace(beforeColon)
				l = strings.TrimSpace(afterColon)
			}
		} else if pipeIdx := strings.Index(trimmed, "|"); pipeIdx > 0 {
			t = strings.TrimSpace(trimmed[:pipeIdx])
			l = strings.TrimSpace(trimmed[pipeIdx+1:])
		} else {
			// First word is time, rest is label
			spaceIdx := strings.Index(trimmed, " ")
			if spaceIdx > 0 {
				t = trimmed[:spaceIdx]
				l = strings.TrimSpace(trimmed[spaceIdx:])
			} else {
				l = trimmed
			}
		}

		events = append(events, event{time: t, label: l})
	}

	if len(events) == 0 {
		return allLines
	}

	// Render as vertical timeline
	for i, ev := range events {
		prefix := "    "

		// Time label
		timePart := ""
		if ev.time != "" {
			timePart = timeStyle.Render(ev.time) + " "
		}

		// Dot and connector
		dot := dotStyle.Render("●")
		connector := ""
		if i < len(events)-1 {
			connector = lineStyle.Render("│")
		}

		// Event line
		allLines = append(allLines, prefix+timePart+dot+"── "+eventStyle.Render(ev.label))

		// Vertical connector to next event
		if connector != "" {
			timeWidth := 0
			if ev.time != "" {
				timeWidth = len(ev.time) + 1
			}
			allLines = append(allLines, prefix+strings.Repeat(" ", timeWidth)+connector)
		}
	}

	return allLines
}

// ---------------------------------------------------------------------------
// Comparison Diagram — side-by-side table
// ---------------------------------------------------------------------------

func renderComparisonDiagram(body []string, maxWidth int) []string {
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	cellStyle := lipgloss.NewStyle().Foreground(text)
	borderStyle := lipgloss.NewStyle().Foreground(surface1)

	var allLines []string

	// Parse table rows (pipe-separated or space-separated)
	type row struct {
		cells []string
	}
	var rows []row

	for _, line := range body {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip separator rows (--- | ---)
		if strings.Count(trimmed, "-") > len(trimmed)/2 {
			continue
		}

		var cells []string
		if strings.Contains(trimmed, "|") {
			parts := strings.Split(trimmed, "|")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" {
					cells = append(cells, p)
				}
			}
		} else if strings.Contains(trimmed, "\t") {
			parts := strings.Split(trimmed, "\t")
			for _, p := range parts {
				cells = append(cells, strings.TrimSpace(p))
			}
		} else {
			cells = []string{trimmed}
		}

		if len(cells) > 0 {
			rows = append(rows, row{cells: cells})
		}
	}

	if len(rows) == 0 {
		return allLines
	}

	// Determine column count and widths
	numCols := 0
	for _, r := range rows {
		if len(r.cells) > numCols {
			numCols = len(r.cells)
		}
	}

	colWidths := make([]int, numCols)
	for _, r := range rows {
		for ci, c := range r.cells {
			if len(c) > colWidths[ci] {
				colWidths[ci] = len(c)
			}
		}
	}

	// Cap total width
	totalWidth := 4 + 3 // prefix + last border
	for _, w := range colWidths {
		totalWidth += w + 3 // cell + padding + border
	}
	if totalWidth > maxWidth {
		// Scale down proportionally
		available := maxWidth - 4 - 3 - numCols*3
		if available < numCols*3 {
			available = numCols * 3
		}
		totalOriginal := 0
		for _, w := range colWidths {
			totalOriginal += w
		}
		if totalOriginal > 0 {
			for ci := range colWidths {
				colWidths[ci] = colWidths[ci] * available / totalOriginal
				if colWidths[ci] < 3 {
					colWidths[ci] = 3
				}
			}
		}
	}

	// Top border
	topLine := "    ╭"
	for ci, w := range colWidths {
		topLine += strings.Repeat("─", w+2)
		if ci < numCols-1 {
			topLine += "┬"
		}
	}
	topLine += "╮"
	allLines = append(allLines, borderStyle.Render(topLine))

	for ri, r := range rows {
		line := "    " + borderStyle.Render("│")
		for ci := 0; ci < numCols; ci++ {
			cell := ""
			if ci < len(r.cells) {
				cell = r.cells[ci]
			}

			padded := rightPad(cell, colWidths[ci])
			if ri == 0 {
				// Header row
				line += " " + headerStyle.Render(padded) + " "
			} else if ci == 0 {
				// First column = label
				line += " " + labelStyle.Render(padded) + " "
			} else {
				line += " " + cellStyle.Render(padded) + " "
			}
			line += borderStyle.Render("│")
		}
		allLines = append(allLines, line)

		// Separator after header
		if ri == 0 {
			sepLine := "    " + borderStyle.Render("├")
			for ci, w := range colWidths {
				sepLine += borderStyle.Render(strings.Repeat("─", w+2))
				if ci < numCols-1 {
					sepLine += borderStyle.Render("┼")
				}
			}
			sepLine += borderStyle.Render("┤")
			allLines = append(allLines, sepLine)
		}
	}

	// Bottom border
	botLine := "    ╰"
	for ci, w := range colWidths {
		botLine += strings.Repeat("─", w+2)
		if ci < numCols-1 {
			botLine += "┴"
		}
	}
	botLine += "╯"
	allLines = append(allLines, borderStyle.Render(botLine))

	return allLines
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func centerPad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	total := width - len(s)
	left := total / 2
	right := total - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func rightPad(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}
