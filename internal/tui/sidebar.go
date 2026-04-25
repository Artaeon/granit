package tui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// sortStrings is a tiny shim so the View body reads cleanly
// without importing sort everywhere we want a deterministic
// pinned-file order.
func sortStrings(s []string) { sort.Strings(s) }

// sidebarSortMode controls how files within a folder are ordered.
// Persisted across sessions via .granit/sidebar-sort.json so power
// users who pinned themselves to "modified desc" don't have to
// re-cycle each session.
type sidebarSortMode int

const (
	sidebarSortName sidebarSortMode = iota
	sidebarSortModified
	sidebarSortCreated
)

func (m sidebarSortMode) String() string {
	switch m {
	case sidebarSortModified:
		return "modified"
	case sidebarSortCreated:
		return "created"
	}
	return "name"
}

type Sidebar struct {
	files    []string
	filtered []string
	cursor   int
	search    string
	searching bool // true when actively typing in search
	focused  bool
	height   int
	width    int
	scroll   int

	// Config-driven
	showIcons   bool
	compactMode bool
	showHidden  bool

	// File tree view
	treeView bool
	fileTree FileTree

	// vaultRoot is needed by features that touch disk (file
	// mtimes for sort, git status, persistence). Set via
	// SetVaultRoot from the Model right after open.
	vaultRoot string

	// activeNote is the path the editor is currently viewing.
	// 'R' (reveal) uses this to scroll/expand the tree to land
	// on it. Updated via SetActiveNote whenever the model swaps
	// notes — without this the sidebar can't know what to reveal.
	activeNote string

	// pinned maps file path → true for files the user pinned with
	// 'b'. Pinned files render in a "PINNED" section at the top of
	// both flat and tree views, and get a ★ prefix in the tree.
	// Persisted to .granit/sidebar-pinned.json.
	pinned map[string]bool

	// sort + status
	sortMode    sidebarSortMode
	statusMsg   string         // ephemeral status hint (sort changed, etc.)
	gitStatus   map[string]rune // path → status rune ('M'=modified, '?'=untracked, 'C'=conflict)
	gitChecked  bool           // we've already attempted to fetch git status
}

func NewSidebar(files []string) Sidebar {
	ft := NewFileTree()
	ft.SetFiles(files)
	return Sidebar{
		files:    files,
		filtered: files,
		cursor:   0,
		focused:  true,
		treeView: true,
		fileTree: ft,
	}
}

func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height
	// Reserve 3 lines for header, search bar, and separator
	treeHeight := height - 3
	if treeHeight < 1 {
		treeHeight = 1
	}
	s.fileTree.SetSize(width, treeHeight)
}

func (s *Sidebar) SetShowHidden(show bool) {
	s.showHidden = show
	s.fileTree.showHidden = show
	// Re-filter the flat list and rebuild the tree with current files.
	s.applyFilter()
	s.fileTree.SetFiles(s.files)
}

func (s *Sidebar) SetFiles(files []string) {
	s.files = files
	s.applyFilter()
	s.fileTree.SetFiles(files)
}

// SaveExplorerState persists folder expansion state to disk.
func (s *Sidebar) SaveExplorerState(vaultPath string) {
	s.fileTree.SaveState(vaultPath)
}

// LoadExplorerState restores folder expansion state from disk.
func (s *Sidebar) LoadExplorerState(vaultPath string) {
	s.fileTree.LoadState(vaultPath)
	s.vaultRoot = vaultPath
	s.loadPinned()
	s.loadSort()
	// Initial git status read so dots show on first render.
	// Subsequent refreshes happen via RefreshGitStatus, which
	// the model can call after saves if it wants live updates.
	s.RefreshGitStatus()
}

// SetVaultRoot is the late-binding hook for callers that don't
// go through LoadExplorerState (e.g. test harnesses, embeds).
// Required for git status + sort by mtime to know where files
// actually live on disk.
func (s *Sidebar) SetVaultRoot(root string) {
	s.vaultRoot = root
}

// SetActiveNote informs the sidebar of the path the editor is
// currently displaying. Used by 'R' (reveal) to scroll/expand
// the tree to that path. Should be called from the Model after
// every loadNote so reveal lands on the right file.
func (s *Sidebar) SetActiveNote(path string) {
	s.activeNote = path
}

// loadPinned restores the pinned-file set from disk. Silent on
// missing/malformed file — fresh users get the empty default.
func (s *Sidebar) loadPinned() {
	s.pinned = make(map[string]bool)
	if s.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(s.vaultRoot, ".granit", "sidebar-pinned.json"))
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.pinned)
	if s.pinned == nil {
		s.pinned = make(map[string]bool)
	}
	s.fileTree.SetPinned(s.pinned)
}

// savePinned writes the current pinned set to disk.
func (s *Sidebar) savePinned() {
	if s.vaultRoot == "" {
		return
	}
	dir := filepath.Join(s.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(s.pinned, "", "  ")
	if err != nil {
		return
	}
	_ = atomicWriteNote(filepath.Join(dir, "sidebar-pinned.json"), string(data))
}

// loadSort restores the sort mode from disk.
func (s *Sidebar) loadSort() {
	if s.vaultRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(s.vaultRoot, ".granit", "sidebar-sort.json"))
	if err != nil {
		return
	}
	var st struct {
		Mode int `json:"mode"`
	}
	if json.Unmarshal(data, &st) == nil && st.Mode >= 0 && st.Mode <= 2 {
		s.sortMode = sidebarSortMode(st.Mode)
		s.fileTree.SetSortMode(s.sortMode, s.vaultRoot)
	}
}

// saveSort writes the sort mode to disk.
func (s *Sidebar) saveSort() {
	if s.vaultRoot == "" {
		return
	}
	dir := filepath.Join(s.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0755)
	data, _ := json.Marshal(struct {
		Mode int `json:"mode"`
	}{Mode: int(s.sortMode)})
	_ = atomicWriteNote(filepath.Join(dir, "sidebar-sort.json"), string(data))
}

// RefreshGitStatus runs `git status --porcelain` once and caches
// the per-file status. Should be called from the Model after
// each save so dots stay current. Silent failure when git isn't
// available — the dots simply don't render.
func (s *Sidebar) RefreshGitStatus() {
	if s.vaultRoot == "" {
		return
	}
	s.gitChecked = true
	s.gitStatus = make(map[string]rune)
	cmd := exec.Command("git", "-C", s.vaultRoot, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(out), "\n") {
		if len(line) < 4 {
			continue
		}
		// Porcelain format: "XY path" — X is index status,
		// Y is working-tree status. Use whichever is non-blank.
		x, y := line[0], line[1]
		path := strings.TrimSpace(line[3:])
		// Handle rename "old -> new" — only flag the new path.
		if idx := strings.Index(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		var marker rune
		switch {
		case x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D'):
			marker = 'C' // conflict
		case x == '?' && y == '?':
			marker = '?' // untracked
		case y == 'M' || x == 'M':
			marker = 'M' // modified
		case x == 'A':
			marker = 'A' // added
		case x == 'D' || y == 'D':
			marker = 'D' // deleted
		default:
			marker = '?'
		}
		s.gitStatus[path] = marker
	}
	s.fileTree.SetGitStatus(s.gitStatus)
}

func (s *Sidebar) applyFilter() {
	if s.search == "" {
		if s.showHidden {
			s.filtered = s.files
		} else {
			s.filtered = nil
			for _, f := range s.files {
				if !isHiddenPath(f) {
					s.filtered = append(s.filtered, f)
				}
			}
		}
		return
	}
	query := strings.ToLower(s.search)
	s.filtered = nil
	for _, f := range s.files {
		if !s.showHidden && isHiddenPath(f) {
			continue
		}
		if fuzzyMatch(strings.ToLower(f), query) {
			s.filtered = append(s.filtered, f)
		}
	}
	if s.cursor >= len(s.filtered) {
		s.cursor = maxInt(0, len(s.filtered)-1)
	}
}

// isHiddenPath returns true if any segment of the path starts with a dot.
func isHiddenPath(p string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(p), "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func fuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

// fuzzyMatchIndices returns the matched character indices for highlighting.
func fuzzyMatchIndices(str, pattern string) []int {
	lowerStr := strings.ToLower(str)
	lowerPat := strings.ToLower(pattern)
	var indices []int
	pi := 0
	for si := 0; si < len(lowerStr) && pi < len(lowerPat); si++ {
		if lowerStr[si] == lowerPat[pi] {
			indices = append(indices, si)
			pi++
		}
	}
	if pi < len(lowerPat) {
		return nil
	}
	return indices
}

// fuzzyHighlight renders a string with matched characters highlighted.
func fuzzyHighlight(name, query string, baseStyle, matchStyle lipgloss.Style) string {
	if query == "" {
		return baseStyle.Render(name)
	}
	indices := fuzzyMatchIndices(name, query)
	if len(indices) == 0 {
		return baseStyle.Render(name)
	}
	matchSet := make(map[int]bool, len(indices))
	for _, idx := range indices {
		matchSet[idx] = true
	}
	var b strings.Builder
	for i, ch := range name {
		if matchSet[i] {
			b.WriteString(matchStyle.Render(string(ch)))
		} else {
			b.WriteString(baseStyle.Render(string(ch)))
		}
	}
	return b.String()
}

func (s *Sidebar) Selected() string {
	if s.treeView && s.search == "" {
		return s.fileTree.Selected()
	}
	if len(s.filtered) == 0 || s.cursor >= len(s.filtered) {
		return ""
	}
	return s.filtered[s.cursor]
}

func (s Sidebar) Update(msg tea.Msg) (Sidebar, tea.Cmd) {
	if !s.focused {
		return s, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Toggle between tree and flat view
		if msg.String() == "ctrl+t" {
			s.treeView = !s.treeView
			s.fileTree.SetFocused(s.focused)
			return s, nil
		}

		if s.treeView && s.search == "" {
			// In tree mode, delegate most keys to file tree
			switch msg.String() {
			case "/":
				s.searching = true
				s.treeView = false
			case "b":
				// Pin/unpin the file under cursor. Pinned files
				// surface in a "PINNED" section above the tree
				// AND get a ★ icon prefix in the tree.
				if path := s.fileTree.Selected(); path != "" {
					if s.pinned == nil {
						s.pinned = make(map[string]bool)
					}
					if s.pinned[path] {
						delete(s.pinned, path)
						s.statusMsg = "Unpinned"
					} else {
						s.pinned[path] = true
						s.statusMsg = "Pinned"
					}
					s.savePinned()
					s.fileTree.SetPinned(s.pinned)
				}
			case "R":
				// Reveal the currently-edited note: expand
				// parent folders along the way, scroll, and
				// move the cursor onto it. Surface a hint when
				// the active note isn't in the tree (e.g.,
				// scratchpad, external file) so the user
				// doesn't think `R` is broken.
				if s.activeNote == "" {
					s.statusMsg = "No active note to reveal"
				} else if !s.fileTree.RevealPath(s.activeNote) {
					s.statusMsg = "Note not in tree: " + s.activeNote
				} else {
					s.statusMsg = "Revealed " + filepath.Base(s.activeNote)
				}
			case "s":
				// Cycle sort mode: name → modified → created → name.
				s.sortMode = (s.sortMode + 1) % 3
				s.fileTree.SetSortMode(s.sortMode, s.vaultRoot)
				s.fileTree.SetFiles(s.files)
				s.saveSort()
				s.statusMsg = "Sort: " + s.sortMode.String()
			default:
				s.fileTree = s.fileTree.Update(msg)
			}
			return s, nil
		}

		switch msg.String() {
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
				if s.cursor < s.scroll {
					s.scroll = s.cursor
				}
			}
		case "down", "j":
			if s.cursor < len(s.filtered)-1 {
				s.cursor++
				visibleHeight := s.height - 4
				if visibleHeight < 1 {
					visibleHeight = 1
				}
				if s.cursor >= s.scroll+visibleHeight {
					s.scroll = s.cursor - visibleHeight + 1
				}
			}
		case "pgup", "ctrl+u":
			visibleHeight := s.height - 4
			if visibleHeight < 1 {
				visibleHeight = 1
			}
			s.cursor -= visibleHeight / 2
			if s.cursor < 0 {
				s.cursor = 0
			}
			if s.cursor < s.scroll {
				s.scroll = s.cursor
			}
		case "pgdown", "ctrl+d":
			visibleHeight := s.height - 4
			if visibleHeight < 1 {
				visibleHeight = 1
			}
			s.cursor += visibleHeight / 2
			if s.cursor >= len(s.filtered) {
				s.cursor = len(s.filtered) - 1
			}
			if s.cursor >= s.scroll+visibleHeight {
				s.scroll = s.cursor - visibleHeight + 1
			}
		case "home":
			s.cursor = 0
			s.scroll = 0
		case "end":
			s.cursor = maxInt(0, len(s.filtered)-1)
			visibleHeight := s.height - 4
			if visibleHeight < 1 {
				visibleHeight = 1
			}
			if s.cursor >= s.scroll+visibleHeight {
				s.scroll = s.cursor - visibleHeight + 1
			}
		case "/":
			s.searching = true
		case "backspace":
			if s.searching && len(s.search) > 0 {
				s.search = TrimLastRune(s.search)
				s.applyFilter()
			}
		case "esc":
			if s.searching || s.search != "" {
				s.searching = false
				s.search = ""
				s.applyFilter()
			}
		case "enter":
			if s.searching {
				s.searching = false // keep filter, exit search mode
			}
		default:
			if s.searching && len(msg.String()) == 1 && msg.String() >= " " {
				s.search += msg.String()
				s.applyFilter()
			}
		}
	}
	return s, nil
}

func (s Sidebar) View() string {
	var b strings.Builder
	contentWidth := s.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Header with accent bar, file count, and git-changes badge.
	headerAccent := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	fileCountStyle := lipgloss.NewStyle().Foreground(surface2)
	headerLine := headerAccent.Render("  EXPLORER")
	if len(s.filtered) > 0 {
		headerLine += fileCountStyle.Render("  " + sidebarItoa(len(s.filtered)) + " files")
	}
	// Git change count: yellow chip when the working tree has
	// any non-clean files. Power users glance at the sidebar
	// and instantly see "I have 3 uncommitted changes" without
	// flipping to a git overlay.
	if changes := len(s.gitStatus); changes > 0 {
		chip := lipgloss.NewStyle().Foreground(crust).Background(yellow).Bold(true).Padding(0, 1).
			Render("●" + sidebarItoa(changes))
		headerLine += "  " + chip
	}
	b.WriteString(headerLine)
	b.WriteString("\n")

	// Search bar with styled input field
	if s.searching || s.search != "" {
		// Active search mode — show input with cursor
		searchBg := lipgloss.NewStyle().
			Background(surface0).
			Foreground(text).
			Width(contentWidth - 2).
			Padding(0, 1)
		searchIcon := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" > ")
		searchText := s.search
		cursor := lipgloss.NewStyle().Foreground(mauve).Render("|")
		b.WriteString(searchIcon + searchBg.Render(searchText+cursor))
	} else {
		// Tree mode or unfocused — show dimmed placeholder (press / to search)
		placeholder := "  / filter..."
		b.WriteString(lipgloss.NewStyle().Foreground(surface2).Render(placeholder))
	}
	b.WriteString("\n")

	// Thin separator
	b.WriteString(lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat(ThemeSeparator, contentWidth)))
	b.WriteString("\n")

	// If tree view and not searching, use the file tree
	if s.treeView && s.search == "" {
		// PINNED section sits above the tree so bookmarked
		// notes are one keystroke away no matter how deep the
		// vault is. Each row is a one-line shortcut; cursor
		// nav happens inside the file tree, but the user can
		// jump to a pinned file via Reveal (R) after opening
		// it. (A full pinned-cursor mode is left for a future
		// pass — keeps this commit small.)
		pinnedRows := 0
		if len(s.pinned) > 0 {
			pinHeader := lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  PINNED")
			countStr := lipgloss.NewStyle().Foreground(surface2).Render("  " + sidebarItoa(len(s.pinned)))
			b.WriteString(pinHeader + countStr + "\n")
			pinned := make([]string, 0, len(s.pinned))
			for p := range s.pinned {
				pinned = append(pinned, p)
			}
			// Stable alphabetical order so the section doesn't
			// shuffle between renders.
			sortStrings(pinned)
			for _, p := range pinned {
				name := strings.TrimSuffix(filepath.Base(p), ".md")
				star := lipgloss.NewStyle().Foreground(yellow).Render("★ ")
				dir := filepath.Dir(p)
				if dir == "." {
					dir = ""
				} else {
					dir = lipgloss.NewStyle().Foreground(surface2).Render(" " + dir + "/")
				}
				active := ""
				if p == s.activeNote {
					active = lipgloss.NewStyle().Foreground(mauve).Render(" ●")
				}
				b.WriteString("  " + star + lipgloss.NewStyle().Foreground(text).Render(name) + dir + active + "\n")
			}
			b.WriteString(lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat(ThemeSeparator, contentWidth)) + "\n")
			// Pinned section consumed: 1 header + N rows + 1 separator.
			pinnedRows = 2 + len(s.pinned)
		}
		// Reclaim height from the file tree before rendering so
		// it doesn't paint past the bottom of the sidebar pane.
		// SetSize was called with the conservative "height - 3"
		// that ignores the pinned section; do the corrective
		// re-size here based on actual pinned rows.
		treeH := s.height - 3 - pinnedRows
		if treeH < 1 {
			treeH = 1
		}
		s.fileTree.SetSize(s.width, treeH)
		b.WriteString(s.fileTree.View())
		if s.focused {
			b.WriteString("\n")
			hint := "  b:pin  R:reveal  s:sort/" + s.sortMode.String() + "  /:search"
			b.WriteString(DimStyle.Render(hint))
			if s.statusMsg != "" {
				b.WriteString("\n  " + lipgloss.NewStyle().Foreground(mauve).Render(s.statusMsg))
			}
		}
		return b.String()
	}

	// File list
	visibleHeight := s.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	if len(s.filtered) == 0 {
		b.WriteString("\n")
		emptyStyle := lipgloss.NewStyle().Foreground(surface2).Italic(true)
		b.WriteString(emptyStyle.Render("  No files found"))
		return b.String()
	}

	end := s.scroll + visibleHeight
	if end > len(s.filtered) {
		end = len(s.filtered)
	}

	lastDir := ""
	for i := s.scroll; i < end; i++ {
		filePath := s.filtered[i]
		dir := filepath.Dir(filePath)
		name := filepath.Base(filePath)

		// Show directory header if changed
		if dir != "." && dir != lastDir {
			folderLine := lipgloss.NewStyle().Foreground(peach).Bold(true).Render("  " + dir + "/")
			b.WriteString(folderLine)
			b.WriteString("\n")
			lastDir = dir
		}

		// Strip .md extension for cleaner display
		displayName := strings.TrimSuffix(name, ".md")

		// File icon based on type
		icon := ""
		if s.showIcons {
			if isDailyNote(name) {
				icon = lipgloss.NewStyle().Foreground(green).Render(IconDailyChar)
			} else {
				icon = lipgloss.NewStyle().Foreground(blue).Render(IconFileChar)
			}
		}

		indent := " "
		if dir != "." {
			indent = "   "
		}
		if s.compactMode {
			indent = ""
			if dir != "." {
				indent = " "
			}
		}

		iconSpace := ""
		if icon != "" {
			iconSpace = " "
		}

		iconW := 0
		if icon != "" {
			iconW = lipgloss.Width(icon) + 1 // +1 for iconSpace
		}
		maxNameLen := contentWidth - lipgloss.Width(indent) - iconW - 3
		if maxNameLen < 5 {
			maxNameLen = 5
		}
		displayName = TruncateDisplay(displayName, maxNameLen)

		if i == s.cursor && s.focused {
			// Selected item: accent bar + highlighted background
			accentBar := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar)
			accentW := lipgloss.Width(accentBar)
			nameStyle := lipgloss.NewStyle().
				Background(surface0).
				Foreground(mauve).
				Bold(true)
			var renderedName string
			if s.search != "" {
				matchStyle := lipgloss.NewStyle().Background(surface0).Foreground(peach).Bold(true).Underline(true)
				renderedName = fuzzyHighlight(displayName, s.search, nameStyle, matchStyle)
			} else {
				renderedName = nameStyle.Render(displayName)
			}
			inner := nameStyle.Render(indent+icon+iconSpace) + renderedName
			line := accentBar + lipgloss.NewStyle().Background(surface0).Width(contentWidth-accentW).Render(inner)
			b.WriteString(line)
		} else {
			var renderedName string
			if s.search != "" {
				matchStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
				renderedName = fuzzyHighlight(displayName, s.search, NormalItemStyle, matchStyle)
			} else {
				renderedName = NormalItemStyle.Render(displayName)
			}
			b.WriteString(" " + indent + icon + iconSpace + renderedName)
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Scroll indicator bar
	if len(s.filtered) > visibleHeight {
		pct := float64(s.scroll) / float64(len(s.filtered)-visibleHeight)
		b.WriteString("\n")
		b.WriteString(sidebarScrollBar(pct, visibleHeight, len(s.filtered)))
	}

	return b.String()
}

// sidebarScrollBar renders a visual scroll position indicator.
func sidebarScrollBar(pct float64, visH, total int) string {
	if total <= visH {
		return ""
	}
	p := int(pct * 100)
	return lipgloss.NewStyle().Foreground(surface2).Render("  " + sidebarItoa(p) + "%%")
}

func sidebarItoa(n int) string {
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


func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// clampInt constrains v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
