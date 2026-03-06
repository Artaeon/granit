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

// TreeNode represents a single entry (file or directory) in the file tree.
type TreeNode struct {
	Name     string
	Path     string // relative path (for files) or dir path (for folders)
	IsDir    bool
	Expanded bool
	Children []*TreeNode
	Depth    int
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
}

// NewFileTree returns a zero-value FileTree ready for use.
func NewFileTree() FileTree {
	return FileTree{}
}

// SetFiles takes a sorted list of relative file paths (e.g.
// ["daily/2024-01-01.md", "notes/project.md", "readme.md"]) and builds the
// internal tree hierarchy.
func (ft *FileTree) SetFiles(files []string) {
	root := &TreeNode{Name: "", Path: "", IsDir: true, Expanded: true, Depth: -1}

	// dirMap tracks already-created directory nodes keyed by their path.
	dirMap := make(map[string]*TreeNode)
	dirMap[""] = root

	// ensureDir creates (or returns) the TreeNode for the given directory path,
	// recursively ensuring parent directories exist.
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
			Name:     filepath.Base(dirPath),
			Path:     dirPath,
			IsDir:    true,
			Expanded: false,
			Depth:    depth,
		}
		parent.Children = append(parent.Children, node)
		dirMap[dirPath] = node
		return node
	}

	for _, f := range files {
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

	// Sort children at every level: directories first (alphabetically), then
	// files (alphabetically).
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

	// Root-level directories start expanded; nested directories start collapsed.
	for _, child := range root.Children {
		if child.IsDir {
			child.Expanded = true
		}
	}

	ft.root = root
	ft.rebuild()

	// Clamp cursor.
	if ft.cursor >= len(ft.visible) {
		ft.cursor = maxInt(0, len(ft.visible)-1)
	}
}

// SetSize updates the viewport dimensions available for rendering.
func (ft *FileTree) SetSize(width, height int) {
	ft.width = width
	ft.height = height
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

	visibleHeight := ft.viewHeight()

	switch msg.String() {
	case "up", "k":
		if ft.cursor > 0 {
			ft.cursor--
			if ft.cursor < ft.scroll {
				ft.scroll = ft.cursor
			}
		}

	case "down", "j":
		if ft.cursor < len(ft.visible)-1 {
			ft.cursor++
			if ft.cursor >= ft.scroll+visibleHeight {
				ft.scroll = ft.cursor - visibleHeight + 1
			}
		}

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
			// Move to parent directory.
			ft.goToParent()
		}

	case "right", "l":
		node := ft.visible[ft.cursor]
		if node.IsDir && !node.Expanded {
			node.Expanded = true
			ft.rebuild()
			ft.clampCursor()
		}
	}

	return ft
}

// View renders the tree into a string suitable for embedding in the sidebar.
func (ft FileTree) View() string {
	if len(ft.visible) == 0 {
		return DimStyle.Render("  No files")
	}

	var b strings.Builder

	visibleHeight := ft.viewHeight()
	end := ft.scroll + visibleHeight
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

	// Scroll indicator.
	if len(ft.visible) > visibleHeight {
		pct := float64(ft.scroll) / float64(len(ft.visible)-visibleHeight)
		b.WriteString("\n" + DimStyle.Render(scrollIndicator(pct)))
	}

	return b.String()
}

// ---------- internal helpers ----------

// rebuild flattens the tree according to the current expanded state of each
// directory, populating ft.visible.
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

// clampCursor ensures cursor and scroll are within valid bounds after the
// visible list has been rebuilt.
func (ft *FileTree) clampCursor() {
	if ft.cursor >= len(ft.visible) {
		ft.cursor = maxInt(0, len(ft.visible)-1)
	}
	visibleHeight := ft.viewHeight()
	if ft.cursor < ft.scroll {
		ft.scroll = ft.cursor
	}
	if ft.cursor >= ft.scroll+visibleHeight {
		ft.scroll = ft.cursor - visibleHeight + 1
	}
}

// goToParent moves the cursor to the parent directory of the currently selected
// node.
func (ft *FileTree) goToParent() {
	if ft.cursor < 0 || ft.cursor >= len(ft.visible) {
		return
	}
	cur := ft.visible[ft.cursor]
	targetDepth := cur.Depth - 1
	for i := ft.cursor - 1; i >= 0; i-- {
		if ft.visible[i].IsDir && ft.visible[i].Depth == targetDepth {
			ft.cursor = i
			if ft.cursor < ft.scroll {
				ft.scroll = ft.cursor
			}
			return
		}
	}
}

// isDailyNote returns true when the file name (without extension) matches the
// YYYY-MM-DD daily-note pattern.
func isDailyNote(name string) bool {
	bare := strings.TrimSuffix(name, filepath.Ext(name))
	return dailyPattern.MatchString(bare)
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
		return indent + folderStyle.Render(arrow+" "+IconFolderChar+" "+node.Name+"/")
	}

	// File node.
	displayName := strings.TrimSuffix(node.Name, ".md")

	// Truncate if needed.
	maxNameLen := maxWidth - (node.Depth*2 + 4) // indent + icon + spaces
	if maxNameLen < 5 {
		maxNameLen = 5
	}
	if len(displayName) > maxNameLen {
		displayName = displayName[:maxNameLen-3] + "..."
	}

	icon := lipgloss.NewStyle().Foreground(blue).Render(IconFileChar)
	if isDailyNote(node.Name) {
		icon = lipgloss.NewStyle().Foreground(green).Render(IconDailyChar)
	}

	return indent + icon + " " + NormalItemStyle.Render(displayName)
}

// renderNodePlain produces a plain-text (uncolored) line used as the basis for
// the highlighted/selected row, which gets its own uniform styling applied on
// top.
func (ft FileTree) renderNodePlain(node *TreeNode, maxWidth int) string {
	indent := strings.Repeat("  ", node.Depth)

	if node.IsDir {
		arrow := "\u25b8"
		if node.Expanded {
			arrow = "\u25be"
		}
		return indent + arrow + " " + IconFolderChar + " " + node.Name + "/"
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
	}

	return indent + iconChar + " " + displayName
}
