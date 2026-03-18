package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Tree node
// ---------------------------------------------------------------------------

type mindMapNode struct {
	Label    string
	Children []*mindMapNode
	IsLink   bool // true if this is a [[wikilink]]
	Depth    int
}

// ---------------------------------------------------------------------------
// MindMap overlay
// ---------------------------------------------------------------------------

// MindMap renders an ASCII mind map from the current note's headings and
// wikilinks, or from its link neighbourhood (2 levels deep).
type MindMap struct {
	active    bool
	width     int
	height    int
	vaultRoot string
	notePath  string

	root *mindMapNode
	mode int // 0=headings, 1=links

	// Rendered lines for scrolling
	lines   []string
	scrollX int
	scrollY int

	statusMsg string
}

// NewMindMap returns a zero-value, inactive MindMap.
func NewMindMap() MindMap {
	return MindMap{}
}

// IsActive reports whether the overlay is visible.
func (mm MindMap) IsActive() bool {
	return mm.active
}

// SetSize updates the overlay dimensions.
func (mm *MindMap) SetSize(w, h int) {
	mm.width = w
	mm.height = h
}

// OpenForNote initialises the mind map for the given note and activates
// the overlay.
func (mm *MindMap) OpenForNote(vaultRoot, notePath, noteContent string) {
	mm.active = true
	mm.vaultRoot = vaultRoot
	mm.notePath = notePath
	mm.mode = 0
	mm.scrollX = 0
	mm.scrollY = 0
	mm.statusMsg = ""
	mm.buildHeadingsTree(noteContent)
	mm.renderLines()
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (mm MindMap) Update(msg tea.Msg) (MindMap, tea.Cmd) {
	if !mm.active {
		return mm, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			mm.active = false
			return mm, nil

		// Toggle mode
		case "m":
			if mm.mode == 0 {
				mm.mode = 1
				mm.buildLinksTree()
			} else {
				mm.mode = 0
				// Re-read note content from disk for headings mode
				content := mm.readNoteContent(mm.notePath)
				mm.buildHeadingsTree(content)
			}
			mm.scrollX = 0
			mm.scrollY = 0
			mm.renderLines()

		// Scroll vertical
		case "j", "down":
			mm.scrollY++
			mm.clampScroll()
		case "k", "up":
			if mm.scrollY > 0 {
				mm.scrollY--
			}

		// Scroll horizontal
		case "l", "right":
			mm.scrollX++
			mm.clampScroll()
		case "h", "left":
			if mm.scrollX > 0 {
				mm.scrollX--
			}

		// Center view
		case "c":
			mm.scrollX = 0
			mm.scrollY = 0
		}
	}
	return mm, nil
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (mm MindMap) View() string {
	width := mm.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 140 {
		width = 140
	}
	innerWidth := width - 6

	var b strings.Builder

	// Header
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	modeStyle := lipgloss.NewStyle().Foreground(overlay0)
	modeName := "Headings"
	if mm.mode == 1 {
		modeName = "Links"
	}
	b.WriteString(titleStyle.Render("  Mind Map"))
	b.WriteString(modeStyle.Render("  [" + modeName + " mode]"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	// Visible area
	visW := width - 8
	if visW < 20 {
		visW = 20
	}
	visH := mm.height - 14
	if visH < 5 {
		visH = 5
	}

	if len(mm.lines) == 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  No content to map"))
		b.WriteString("\n")
	} else {
		b.WriteString("\n")
		endY := mm.scrollY + visH
		if endY > len(mm.lines) {
			endY = len(mm.lines)
		}
		startY := mm.scrollY
		if startY > len(mm.lines) {
			startY = len(mm.lines)
		}

		for i := startY; i < endY; i++ {
			line := mm.lines[i]

			// Apply horizontal scroll
			displayed := mmShiftLine(line, mm.scrollX)

			// Truncate to visible width
			displayed = mmTruncateLine(displayed, visW)

			b.WriteString("  ")
			b.WriteString(displayed)
			b.WriteString("\n")
		}

		// Pad remaining lines if content is shorter than viewport
		for i := endY - startY; i < visH; i++ {
			b.WriteString("\n")
		}
	}

	// Status
	if mm.statusMsg != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(yellow).Render("  " + mm.statusMsg))
		b.WriteString("\n")
	}

	// Footer
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"m", "mode"}, {"hjkl", "scroll"}, {"c", "center"}, {"Esc", "close"},
	}))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// ---------------------------------------------------------------------------
// Headings tree builder
// ---------------------------------------------------------------------------

var mmWikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

func (mm *MindMap) buildHeadingsTree(content string) {
	lines := strings.Split(content, "\n")

	// Derive root label from note path
	rootLabel := strings.TrimSuffix(filepath.Base(mm.notePath), ".md")
	mm.root = &mindMapNode{Label: rootLabel, Depth: 0}

	// Stack for tracking parent at each heading level.
	// stack[level] points to the last node created at that level.
	stack := make([]*mindMapNode, 7) // indices 0-6
	stack[0] = mm.root

	inCodeBlock := false
	var pendingLinks []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track fenced code blocks
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}

		// Detect heading
		level := 0
		for _, ch := range trimmed {
			if ch == '#' {
				level++
			} else {
				break
			}
		}

		if level >= 1 && level <= 6 && len(trimmed) > level && trimmed[level] == ' ' {
			headingText := strings.TrimSpace(trimmed[level+1:])

			// Before adding this heading, flush any pending links to the
			// current parent (i.e. the most recently created heading node).
			parent := mmFindParent(stack, level)
			if parent != nil {
				for _, lnk := range pendingLinks {
					parent.Children = append(parent.Children, &mindMapNode{
						Label:  lnk,
						IsLink: true,
						Depth:  parent.Depth + 1,
					})
				}
			}
			pendingLinks = nil

			// Find parent for this heading
			parent = mmFindParent(stack, level)

			node := &mindMapNode{
				Label: headingText,
				Depth: level,
			}
			parent.Children = append(parent.Children, node)

			// Update stack: set this level and clear deeper levels
			stack[level] = node
			for i := level + 1; i < len(stack); i++ {
				stack[i] = nil
			}
		} else {
			// Collect wikilinks from non-heading lines
			matches := mmWikilinkRe.FindAllStringSubmatch(trimmed, -1)
			for _, m := range matches {
				linkTarget := m[1]
				// Strip display alias (e.g. [[note|alias]] -> note)
				if idx := strings.Index(linkTarget, "|"); idx >= 0 {
					linkTarget = linkTarget[:idx]
				}
				pendingLinks = append(pendingLinks, linkTarget)
			}
		}
	}

	// Flush remaining pending links
	deepest := mmDeepestNode(stack)
	for _, lnk := range pendingLinks {
		deepest.Children = append(deepest.Children, &mindMapNode{
			Label:  lnk,
			IsLink: true,
			Depth:  deepest.Depth + 1,
		})
	}
}

// mmFindParent walks the stack backwards from level-1 to find the nearest
// ancestor for a node at the given level.
func mmFindParent(stack []*mindMapNode, level int) *mindMapNode {
	for i := level - 1; i >= 0; i-- {
		if stack[i] != nil {
			return stack[i]
		}
	}
	return stack[0]
}

// mmDeepestNode returns the deepest non-nil node in the stack.
func mmDeepestNode(stack []*mindMapNode) *mindMapNode {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] != nil {
			return stack[i]
		}
	}
	return stack[0]
}

// ---------------------------------------------------------------------------
// Links tree builder (2 levels deep)
// ---------------------------------------------------------------------------

func (mm *MindMap) buildLinksTree() {
	rootLabel := strings.TrimSuffix(filepath.Base(mm.notePath), ".md")
	mm.root = &mindMapNode{Label: rootLabel, Depth: 0}

	if mm.vaultRoot == "" {
		mm.statusMsg = "No vault root set"
		return
	}

	// Build a map: note name -> list of wikilink targets
	vaultLinks := mm.scanVaultLinks()

	// Get links from the current note (level 1)
	currentName := strings.TrimSuffix(filepath.Base(mm.notePath), ".md")
	level1Links := vaultLinks[currentName]

	seen := map[string]bool{currentName: true}

	for _, l1 := range level1Links {
		if seen[l1] {
			continue
		}
		seen[l1] = true
		child := &mindMapNode{
			Label:  l1,
			IsLink: true,
			Depth:  1,
		}

		// Level 2: links from this linked note
		level2Links := vaultLinks[l1]
		seen2 := map[string]bool{}
		for _, l2 := range level2Links {
			if l2 == currentName || seen2[l2] {
				continue
			}
			seen2[l2] = true
			grandchild := &mindMapNode{
				Label:  l2,
				IsLink: true,
				Depth:  2,
			}
			child.Children = append(child.Children, grandchild)
		}
		mm.root.Children = append(mm.root.Children, child)
	}

	if len(mm.root.Children) == 0 {
		mm.statusMsg = "No wikilinks found in this note"
	} else {
		mm.statusMsg = fmt.Sprintf("%d linked notes", len(mm.root.Children))
	}
}

// scanVaultLinks walks the vault and extracts wikilinks from every .md file.
// Returns a map from note name (without .md) to a slice of link target names.
func (mm *MindMap) scanVaultLinks() map[string][]string {
	result := make(map[string][]string)

	_ = filepath.Walk(mm.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip hidden directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		noteName := strings.TrimSuffix(filepath.Base(path), ".md")
		matches := mmWikilinkRe.FindAllStringSubmatch(string(data), -1)

		var links []string
		seen := map[string]bool{}
		for _, m := range matches {
			target := m[1]
			if idx := strings.Index(target, "|"); idx >= 0 {
				target = target[:idx]
			}
			if idx := strings.Index(target, "#"); idx >= 0 {
				target = target[:idx]
			}
			target = strings.TrimSpace(target)
			if target == "" || seen[target] {
				continue
			}
			seen[target] = true
			links = append(links, target)
		}

		if len(links) > 0 {
			result[noteName] = links
		}
		return nil
	})

	return result
}

// readNoteContent reads a note from disk by its path.
func (mm *MindMap) readNoteContent(notePath string) string {
	fullPath := notePath
	if !filepath.IsAbs(notePath) {
		fullPath = filepath.Join(mm.vaultRoot, notePath)
	}
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ""
	}
	return string(data)
}

// ---------------------------------------------------------------------------
// ASCII tree renderer
// ---------------------------------------------------------------------------

// renderLines builds mm.lines from the current tree.
func (mm *MindMap) renderLines() {
	mm.lines = nil
	if mm.root == nil {
		return
	}
	mm.lines = mmRenderTree(mm.root)
}

// mmRenderTree renders a tree as a right-branching ASCII mind map.
//
// The output looks like:
//
//	                ┌─ Child A
//	 Root Label ────┤
//	                └─ Child B ── Leaf
func mmRenderTree(root *mindMapNode) []string {
	if len(root.Children) == 0 {
		// Single node, no children
		return []string{mmStyleLabel(root)}
	}

	// Render all child subtrees first, collecting their rendered blocks.
	type childBlock struct {
		lines []string
		// The row within the block that holds the "entry" line
		// (the line where the connector attaches).
		entryRow int
	}

	var blocks []childBlock
	for _, child := range root.Children {
		bl := mmRenderSubtree(child)
		blocks = append(blocks, bl)
	}

	// Compute the total height of the output.
	totalHeight := 0
	for i, bl := range blocks {
		totalHeight += len(bl.lines)
		if i < len(blocks)-1 {
			totalHeight++ // gap line between siblings
		}
	}

	// Find the vertical midpoint for the root label.
	rootLabel := mmStyleLabel(root)
	rootPlain := mmPlainLabel(root)
	rootLen := len(rootPlain)

	// Connector between root and the bracket: " ───┤" or " ─── " for single child.
	connLen := 4
	connStr := " " + strings.Repeat("─", connLen-1)

	// Prefix width: rootLen + connLen
	prefixWidth := rootLen + len(connStr) + 1 // +1 for bracket column

	// Build output lines.
	out := make([]string, totalHeight)
	row := 0

	// Track where each child's entry row falls in the global output.
	entryRows := make([]int, len(blocks))
	for i, bl := range blocks {
		entryRows[i] = row + bl.entryRow
		for j, line := range bl.lines {
			out[row+j] = line
			_ = j
		}
		row += len(bl.lines)
		if i < len(blocks)-1 {
			out[row] = "" // gap line
			row++
		}
	}

	// Determine the vertical center for root label placement.
	var rootRow int
	if len(blocks) == 1 {
		rootRow = entryRows[0]
	} else {
		// Place root between first and last child entry rows
		rootRow = (entryRows[0] + entryRows[len(entryRows)-1]) / 2
	}

	// Now prepend prefix columns to every line.
	result := make([]string, totalHeight)
	for i := 0; i < totalHeight; i++ {
		var prefix string

		// Root label column
		if i == rootRow {
			prefix = rootLabel + connStr
		} else {
			prefix = strings.Repeat(" ", rootLen) + strings.Repeat(" ", len(connStr))
		}

		// Bracket column
		if len(blocks) == 1 {
			// Single child: just a straight connector
			if i == rootRow {
				prefix += "── "
			} else {
				prefix += "   "
			}
		} else {
			// Multiple children: bracket
			firstEntry := entryRows[0]
			lastEntry := entryRows[len(entryRows)-1]

			if i == firstEntry {
				prefix += "┌─ "
			} else if i == lastEntry {
				prefix += "└─ "
			} else if i > firstEntry && i < lastEntry {
				// Check if this row is an entry row for a middle child
				isEntry := false
				for _, er := range entryRows[1 : len(entryRows)-1] {
					if i == er {
						isEntry = true
						break
					}
				}
				if isEntry {
					prefix += "├─ "
				} else {
					prefix += "│  "
				}
			} else {
				prefix += "   "
			}
		}

		_ = prefixWidth
		result[i] = prefix + out[i]
	}

	return result
}

// mmRenderSubtree renders a child node and its descendants, returning the
// block of lines and which row the connector should attach to.
func mmRenderSubtree(node *mindMapNode) struct {
	lines    []string
	entryRow int
} {
	label := mmStyleLabel(node)

	if len(node.Children) == 0 {
		return struct {
			lines    []string
			entryRow int
		}{lines: []string{label}, entryRow: 0}
	}

	// Recursively render children
	type childBlock struct {
		lines    []string
		entryRow int
	}
	var blocks []childBlock
	for _, child := range node.Children {
		bl := mmRenderSubtree(child)
		blocks = append(blocks, childBlock(bl))
	}

	totalHeight := 0
	for i, bl := range blocks {
		totalHeight += len(bl.lines)
		if i < len(blocks)-1 {
			totalHeight++ // gap line
		}
	}

	// Entry rows for children in this local coordinate space
	entryRows := make([]int, len(blocks))
	row := 0
	for i, bl := range blocks {
		entryRows[i] = row + bl.entryRow
		row += len(bl.lines)
		if i < len(blocks)-1 {
			row++ // gap
		}
	}

	// Collect child content lines
	childLines := make([]string, totalHeight)
	row = 0
	for i, bl := range blocks {
		for j, line := range bl.lines {
			childLines[row+j] = line
		}
		row += len(bl.lines)
		if i < len(blocks)-1 {
			childLines[row] = ""
			row++
		}
	}

	// Determine where this node's label sits (vertical center)
	var nodeRow int
	if len(blocks) == 1 {
		nodeRow = entryRows[0]
	} else {
		nodeRow = (entryRows[0] + entryRows[len(entryRows)-1]) / 2
	}

	plainLabel := mmPlainLabel(node)
	labelLen := len(plainLabel)
	connStr := " ─"

	result := make([]string, totalHeight)
	for i := 0; i < totalHeight; i++ {
		var prefix string

		// Label column
		if i == nodeRow {
			prefix = label + connStr
		} else {
			prefix = strings.Repeat(" ", labelLen) + strings.Repeat(" ", len(connStr))
		}

		// Bracket column
		if len(blocks) == 1 {
			if i == nodeRow {
				prefix += "── "
			} else {
				prefix += "   "
			}
		} else {
			firstEntry := entryRows[0]
			lastEntry := entryRows[len(entryRows)-1]

			if i == firstEntry {
				prefix += "┌─ "
			} else if i == lastEntry {
				prefix += "└─ "
			} else if i > firstEntry && i < lastEntry {
				isEntry := false
				for _, er := range entryRows[1 : len(entryRows)-1] {
					if i == er {
						isEntry = true
						break
					}
				}
				if isEntry {
					prefix += "├─ "
				} else {
					prefix += "│  "
				}
			} else {
				prefix += "   "
			}
		}

		result[i] = prefix + childLines[i]
	}

	return struct {
		lines    []string
		entryRow int
	}{lines: result, entryRow: nodeRow}
}

// ---------------------------------------------------------------------------
// Label styling helpers
// ---------------------------------------------------------------------------

// headingDepthColors maps heading depth to a lipgloss colour.
var headingDepthColors = []lipgloss.Color{
	mauve, // depth 0 (root / h1)
	blue,  // depth 1 (h2)
	teal,  // depth 2 (h3)
	green, // depth 3 (h4)
	peach, // depth 4 (h5)
}

func mmHeadingColor(depth int) lipgloss.Color {
	if depth < 0 {
		depth = 0
	}
	if depth >= len(headingDepthColors) {
		depth = len(headingDepthColors) - 1
	}
	return headingDepthColors[depth]
}

// mmStyleLabel returns the styled (coloured) label for a node.
func mmStyleLabel(node *mindMapNode) string {
	if node.IsLink {
		style := lipgloss.NewStyle().Foreground(lavender)
		return style.Render("[[" + node.Label + "]]")
	}
	col := mmHeadingColor(node.Depth)
	style := lipgloss.NewStyle().Foreground(col).Bold(node.Depth <= 1)
	return style.Render(node.Label)
}

// mmPlainLabel returns the unstyled label string (for width measurement).
func mmPlainLabel(node *mindMapNode) string {
	if node.IsLink {
		return "[[" + node.Label + "]]"
	}
	return node.Label
}

// ---------------------------------------------------------------------------
// Scroll helpers
// ---------------------------------------------------------------------------

func (mm *MindMap) clampScroll() {
	visH := mm.height - 14
	if visH < 5 {
		visH = 5
	}
	maxScrollY := len(mm.lines) - visH
	if maxScrollY < 0 {
		maxScrollY = 0
	}
	if mm.scrollY > maxScrollY {
		mm.scrollY = maxScrollY
	}

	// Horizontal: find maximum line width
	maxWidth := 0
	for _, line := range mm.lines {
		w := mmPlainLen(line)
		if w > maxWidth {
			maxWidth = w
		}
	}
	visW := mm.width*2/3 - 8
	if visW < 20 {
		visW = 20
	}
	maxScrollX := maxWidth - visW
	if maxScrollX < 0 {
		maxScrollX = 0
	}
	if mm.scrollX > maxScrollX {
		mm.scrollX = maxScrollX
	}
}

// mmPlainLen estimates the display width of a string by stripping ANSI
// escape sequences.
func mmPlainLen(s string) int {
	inEsc := false
	n := 0
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}

// mmShiftLine shifts a line horizontally by dropping the first `offset`
// visible characters while preserving ANSI escape sequences.
func mmShiftLine(s string, offset int) string {
	if offset <= 0 {
		return s
	}
	var b strings.Builder
	skipped := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			if skipped >= offset {
				b.WriteRune(r)
			}
			continue
		}
		if inEsc {
			if skipped >= offset {
				b.WriteRune(r)
			}
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if skipped < offset {
			skipped++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// mmTruncateLine truncates a line to maxWidth visible characters while
// preserving ANSI escape sequences.
func mmTruncateLine(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	var b strings.Builder
	visible := 0
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			b.WriteRune(r)
			continue
		}
		if inEsc {
			b.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if visible >= maxWidth {
			break
		}
		b.WriteRune(r)
		visible++
	}
	return b.String()
}
