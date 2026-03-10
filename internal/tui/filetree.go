package tui

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// dailyPattern matches YYYY-MM-DD date filenames (daily notes).
var dailyPattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// weeklyPattern matches YYYY-Www filenames (weekly notes).
var weeklyPattern = regexp.MustCompile(`^\d{4}-W\d{2}$`)

// TreeNode represents a single entry (file or directory) in the file tree.
type TreeNode struct {
	Name     string
	Path     string // relative path (for files) or dir path (for folders)
	IsDir    bool
	Expanded bool
	Children []*TreeNode
	Depth    int
}

// fileCount returns the total number of file (non-directory) descendants.
func (n *TreeNode) fileCount() int {
	count := 0
	for _, c := range n.Children {
		if c.IsDir {
			count += c.fileCount()
		} else {
			count++
		}
	}
	return count
}

// FileTree is a collapsible tree-view component for navigating files and
// folders. It is designed to be embedded inside the Sidebar.
type FileTree struct {
	root    *TreeNode
	visible []*TreeNode // flattened list of currently visible nodes
	cursor  int
	scroll  int
	height  int
	width   int
	focused bool

	// Config-driven
	showHidden bool
}

// NewFileTree returns a zero-value FileTree ready for use.
func NewFileTree() FileTree {
	return FileTree{}
}

// SetFiles takes a sorted list of relative file paths and builds the
// internal tree hierarchy.
func (ft *FileTree) SetFiles(files []string) {
	// Remember which directories were expanded before rebuild.
	expandedDirs := make(map[string]bool)
	if ft.root != nil {
		var collectExpanded func(node *TreeNode)
		collectExpanded = func(node *TreeNode) {
			if node.IsDir && node.Expanded {
				expandedDirs[node.Path] = true
			}
			for _, c := range node.Children {
				collectExpanded(c)
			}
		}
		collectExpanded(ft.root)
	}
	isFirstBuild := ft.root == nil

	root := &TreeNode{Name: "", Path: "", IsDir: true, Expanded: true, Depth: -1}

	dirMap := make(map[string]*TreeNode)
	dirMap[""] = root

	var ensureDir func(dirPath string, depth int) *TreeNode
	ensureDir = func(dirPath string, depth int) *TreeNode {
		if n, ok := dirMap[dirPath]; ok {
			return n
		}
		parentPath := filepath.Dir(dirPath)
		if parentPath == "." {
			parentPath = ""
		}
		parentDepth := depth - 1
		if parentDepth < 0 {
			parentDepth = 0
		}
		parent := ensureDir(parentPath, parentDepth)
		node := &TreeNode{
			Name:  filepath.Base(dirPath),
			Path:  dirPath,
			IsDir: true,
			Depth: depth,
		}
		parent.Children = append(parent.Children, node)
		dirMap[dirPath] = node
		return node
	}

	for _, f := range files {
		// Skip hidden files/paths when showHidden is false.
		if !ft.showHidden && isHiddenPath(f) {
			continue
		}

		dir := filepath.Dir(f)
		name := filepath.Base(f)

		var parent *TreeNode
		if dir == "." {
			parent = root
		} else {
			depth := strings.Count(dir, string(filepath.Separator)) + 1
			parent = ensureDir(dir, depth-1)
		}

		fileNode := &TreeNode{
			Name:  name,
			Path:  f,
			IsDir: false,
			Depth: parent.Depth + 1,
		}
		parent.Children = append(parent.Children, fileNode)
	}

	// Sort: directories first (alphabetically), then files (alphabetically).
	var sortChildren func(node *TreeNode)
	sortChildren = func(node *TreeNode) {
		sort.SliceStable(node.Children, func(i, j int) bool {
			a, b := node.Children[i], node.Children[j]
			if a.IsDir != b.IsDir {
				return a.IsDir
			}
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		})
		for _, child := range node.Children {
			if child.IsDir {
				sortChildren(child)
			}
		}
	}
	sortChildren(root)

	// Restore expansion state, or default for first build.
	if isFirstBuild {
		// First build: expand root-level directories only.
		for _, child := range root.Children {
			if child.IsDir {
				child.Expanded = true
			}
		}
	} else {
		// Subsequent builds: restore previous expansion state.
		var restoreExpanded func(node *TreeNode)
		restoreExpanded = func(node *TreeNode) {
			if node.IsDir {
				node.Expanded = expandedDirs[node.Path]
			}
			for _, c := range node.Children {
				restoreExpanded(c)
			}
		}
		// Root is always expanded.
		root.Expanded = true
		restoreExpanded(root)
	}

	ft.root = root
	ft.rebuild()
	ft.clampCursor()
}

// SetSize updates the viewport dimensions available for rendering.
func (ft *FileTree) SetSize(width, height int) {
	ft.width = width
	ft.height = height
	ft.clampScroll()
}

// SetFocused sets whether this component currently has keyboard focus.
func (ft *FileTree) SetFocused(focused bool) {
	ft.focused = focused
}

// Selected returns the relative path of the currently highlighted file, or an
// empty string if a directory is selected (or the tree is empty).
func (ft *FileTree) Selected() string {
	if len(ft.visible) == 0 || ft.cursor < 0 || ft.cursor >= len(ft.visible) {
		return ""
	}
	node := ft.visible[ft.cursor]
	if node.IsDir {
		return ""
	}
	return node.Path
}

// Update handles a single key message and returns the (possibly updated) tree.
func (ft FileTree) Update(msg tea.KeyMsg) FileTree {
	if len(ft.visible) == 0 {
		return ft
	}

	vh := ft.viewHeight()

	switch msg.String() {
	case "up", "k":
		if ft.cursor > 0 {
			ft.cursor--
			ft.ensureVisible()
		}

	case "down", "j":
		if ft.cursor < len(ft.visible)-1 {
			ft.cursor++
			ft.ensureVisible()
		}

	case "pgup", "ctrl+u":
		ft.cursor -= vh / 2
		if ft.cursor < 0 {
			ft.cursor = 0
		}
		ft.ensureVisible()

	case "pgdown", "ctrl+d":
		ft.cursor += vh / 2
		if ft.cursor >= len(ft.visible) {
			ft.cursor = len(ft.visible) - 1
		}
		ft.ensureVisible()

	case "home", "g":
		ft.cursor = 0
		ft.scroll = 0

	case "end", "G":
		ft.cursor = len(ft.visible) - 1
		ft.ensureVisible()

	case "enter", " ":
		node := ft.visible[ft.cursor]
		if node.IsDir {
			node.Expanded = !node.Expanded
			ft.rebuild()
			ft.clampCursor()
		}

	case "left", "h":
		node := ft.visible[ft.cursor]
		if node.IsDir && node.Expanded {
			node.Expanded = false
			ft.rebuild()
			ft.clampCursor()
		} else {
			ft.goToParent()
		}

	case "right", "l":
		node := ft.visible[ft.cursor]
		if node.IsDir && !node.Expanded {
			node.Expanded = true
			ft.rebuild()
			ft.clampCursor()
		} else if node.IsDir && node.Expanded && len(node.Children) > 0 {
			// Move into expanded directory.
			ft.cursor++
			ft.ensureVisible()
		}

	case "z":
		// Collapse all directories.
		ft.setAllExpanded(false)
		ft.rebuild()
		ft.clampCursor()

	case "Z":
		// Expand all directories.
		ft.setAllExpanded(true)
		ft.rebuild()
		ft.clampCursor()
	}

	return ft
}

// View renders the tree into a string suitable for embedding in the sidebar.
func (ft FileTree) View() string {
	if len(ft.visible) == 0 {
		return DimStyle.Render("  No files")
	}

	var b strings.Builder

	vh := ft.viewHeight()
	end := ft.scroll + vh
	if end > len(ft.visible) {
		end = len(ft.visible)
	}

	contentWidth := ft.width
	if contentWidth < 10 {
		contentWidth = 10
	}

	for i := ft.scroll; i < end; i++ {
		node := ft.visible[i]
		line := ft.renderNode(node, contentWidth)

		if i == ft.cursor && ft.focused {
			line = lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				Width(contentWidth).
				Render(ft.renderNodePlain(node, contentWidth))
		}

		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Visual scroll bar on right edge.
	if len(ft.visible) > vh {
		b.WriteString("\n")
		b.WriteString(ft.renderScrollBar(vh))
	}

	return b.String()
}

// ---------- internal helpers ----------

// rebuild flattens the tree according to the current expanded state.
func (ft *FileTree) rebuild() {
	ft.visible = ft.visible[:0]
	if ft.root == nil {
		return
	}
	var walk func(nodes []*TreeNode)
	walk = func(nodes []*TreeNode) {
		for _, n := range nodes {
			ft.visible = append(ft.visible, n)
			if n.IsDir && n.Expanded {
				walk(n.Children)
			}
		}
	}
	walk(ft.root.Children)
}

// viewHeight returns the number of rows available for tree lines.
func (ft FileTree) viewHeight() int {
	h := ft.height
	if h < 1 {
		h = 1
	}
	return h
}

// ensureVisible adjusts scroll so the cursor is within the viewport.
func (ft *FileTree) ensureVisible() {
	vh := ft.viewHeight()
	if ft.cursor < ft.scroll {
		ft.scroll = ft.cursor
	}
	if ft.cursor >= ft.scroll+vh {
		ft.scroll = ft.cursor - vh + 1
	}
	ft.clampScroll()
}

// clampScroll ensures scroll doesn't go past the end.
func (ft *FileTree) clampScroll() {
	vh := ft.viewHeight()
	maxScroll := len(ft.visible) - vh
	if maxScroll < 0 {
		maxScroll = 0
	}
	if ft.scroll > maxScroll {
		ft.scroll = maxScroll
	}
	if ft.scroll < 0 {
		ft.scroll = 0
	}
}

// clampCursor ensures cursor and scroll are within valid bounds.
func (ft *FileTree) clampCursor() {
	if ft.cursor >= len(ft.visible) {
		ft.cursor = maxInt(0, len(ft.visible)-1)
	}
	if ft.cursor < 0 {
		ft.cursor = 0
	}
	ft.ensureVisible()
}

// goToParent moves the cursor to the parent directory.
func (ft *FileTree) goToParent() {
	if ft.cursor < 0 || ft.cursor >= len(ft.visible) {
		return
	}
	cur := ft.visible[ft.cursor]
	targetDepth := cur.Depth - 1
	for i := ft.cursor - 1; i >= 0; i-- {
		if ft.visible[i].IsDir && ft.visible[i].Depth == targetDepth {
			ft.cursor = i
			ft.ensureVisible()
			return
		}
	}
}

// setAllExpanded sets the expanded state of all directory nodes (except root).
func (ft *FileTree) setAllExpanded(expanded bool) {
	if ft.root == nil {
		return
	}
	var walk func(node *TreeNode)
	walk = func(node *TreeNode) {
		if node.IsDir {
			node.Expanded = expanded
		}
		for _, c := range node.Children {
			walk(c)
		}
	}
	walk(ft.root)
	ft.root.Expanded = true // root always expanded
}

// isDailyNote returns true when the file name matches YYYY-MM-DD.
func isDailyNote(name string) bool {
	bare := strings.TrimSuffix(name, filepath.Ext(name))
	return dailyPattern.MatchString(bare)
}

// isWeeklyNote returns true when the file name matches YYYY-Www.
func isWeeklyNote(name string) bool {
	bare := strings.TrimSuffix(name, filepath.Ext(name))
	return weeklyPattern.MatchString(bare)
}

// renderNode produces a styled single line for a tree node.
func (ft FileTree) renderNode(node *TreeNode, maxWidth int) string {
	indent := strings.Repeat("  ", node.Depth)

	if node.IsDir {
		arrow := "\u25b8" // ▸ collapsed
		if node.Expanded {
			arrow = "\u25be" // ▾ expanded
		}
		folderStyle := lipgloss.NewStyle().Foreground(peach)
		countStyle := lipgloss.NewStyle().Foreground(surface2)
		fc := node.fileCount()
		countStr := countStyle.Render(" " + treeItoa(fc))
		return indent + folderStyle.Render(arrow+" "+IconFolderChar+" "+node.Name+"/") + countStr
	}

	// File node.
	displayName := strings.TrimSuffix(node.Name, ".md")

	maxNameLen := maxWidth - (node.Depth*2 + 4)
	if maxNameLen < 5 {
		maxNameLen = 5
	}
	if len(displayName) > maxNameLen {
		displayName = displayName[:maxNameLen-3] + "..."
	}

	icon := lipgloss.NewStyle().Foreground(blue).Render(IconFileChar)
	if isDailyNote(node.Name) {
		icon = lipgloss.NewStyle().Foreground(green).Render(IconDailyChar)
	} else if isWeeklyNote(node.Name) {
		icon = lipgloss.NewStyle().Foreground(sapphire).Render(IconCalendarChar)
	}

	return indent + icon + " " + NormalItemStyle.Render(displayName)
}

// renderNodePlain produces a plain-text line for the selected/highlighted row.
func (ft FileTree) renderNodePlain(node *TreeNode, maxWidth int) string {
	indent := strings.Repeat("  ", node.Depth)

	if node.IsDir {
		arrow := "\u25b8"
		if node.Expanded {
			arrow = "\u25be"
		}
		fc := node.fileCount()
		return indent + arrow + " " + IconFolderChar + " " + node.Name + "/ " + treeItoa(fc)
	}

	displayName := strings.TrimSuffix(node.Name, ".md")
	maxNameLen := maxWidth - (node.Depth*2 + 4)
	if maxNameLen < 5 {
		maxNameLen = 5
	}
	if len(displayName) > maxNameLen {
		displayName = displayName[:maxNameLen-3] + "..."
	}

	iconChar := IconFileChar
	if isDailyNote(node.Name) {
		iconChar = IconDailyChar
	} else if isWeeklyNote(node.Name) {
		iconChar = IconCalendarChar
	}

	return indent + iconChar + " " + displayName
}

// renderScrollBar creates a visual scrollbar showing position within the tree.
func (ft FileTree) renderScrollBar(vh int) string {
	total := len(ft.visible)
	if total <= vh {
		return ""
	}

	barWidth := ft.width - 6
	if barWidth < 10 {
		barWidth = 10
	}

	// Position indicator.
	pos := ft.cursor + 1
	posStr := treeItoa(pos) + "/" + treeItoa(total)

	// Visual thumb.
	trackLen := barWidth - len(posStr) - 4
	if trackLen < 4 {
		trackLen = 4
	}
	thumbSize := maxInt(1, trackLen*vh/total)
	if thumbSize > trackLen {
		thumbSize = trackLen
	}
	maxScroll := total - vh
	thumbPos := 0
	if maxScroll > 0 {
		thumbPos = ft.scroll * (trackLen - thumbSize) / maxScroll
	}
	if thumbPos+thumbSize > trackLen {
		thumbPos = trackLen - thumbSize
	}
	if thumbPos < 0 {
		thumbPos = 0
	}

	trackStyle := lipgloss.NewStyle().Foreground(surface0)
	thumbStyle := lipgloss.NewStyle().Foreground(surface2)
	posStyle := lipgloss.NewStyle().Foreground(surface2)

	track := ""
	for i := 0; i < trackLen; i++ {
		if i >= thumbPos && i < thumbPos+thumbSize {
			track += thumbStyle.Render("█")
		} else {
			track += trackStyle.Render("░")
		}
	}

	return "  " + track + " " + posStyle.Render(posStr)
}

func treeItoa(n int) string {
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
