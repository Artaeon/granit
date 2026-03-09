package tui

import (
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Sidebar struct {
	files    []string
	filtered []string
	cursor   int
	search   string
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
	if len(s.filtered) == 0 {
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
			case "backspace":
				// no-op in tree mode without search
			default:
				char := msg.String()
				if len(char) == 1 && char[0] >= 32 && char[0] != ' ' {
					// Start search mode in tree view
					s.search += char
					s.applyFilter()
					s.treeView = false // temporarily switch to flat for search
					return s, nil
				}
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
		case "backspace":
			if len(s.search) > 0 {
				s.search = s.search[:len(s.search)-1]
				s.applyFilter()
			}
		case "esc":
			if s.search != "" {
				s.search = ""
				s.applyFilter()
			}
		default:
			if len(msg.String()) == 1 && msg.String() >= " " {
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

	// Header with accent bar and file count
	headerAccent := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	fileCountStyle := lipgloss.NewStyle().Foreground(surface2)
	headerLine := headerAccent.Render("  EXPLORER")
	if len(s.filtered) > 0 {
		headerLine += fileCountStyle.Render("  " + sidebarItoa(len(s.filtered)) + " files")
	}
	b.WriteString(headerLine)
	b.WriteString("\n")

	// Search bar with styled input field
	if s.search != "" || s.focused {
		searchBg := lipgloss.NewStyle().
			Background(surface0).
			Foreground(text).
			Width(contentWidth - 2).
			Padding(0, 1)
		searchIcon := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" > ")
		searchText := s.search
		if s.focused && s.search == "" {
			searchText = lipgloss.NewStyle().Foreground(surface2).Render("filter...")
		}
		cursor := lipgloss.NewStyle().Foreground(mauve).Render("|")
		b.WriteString(searchIcon + searchBg.Render(searchText+cursor))
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(surface2).Render("  > filter..."))
	}
	b.WriteString("\n")

	// Thin separator
	b.WriteString(lipgloss.NewStyle().Foreground(surface0).Render(strings.Repeat(ThemeSeparator, contentWidth)))
	b.WriteString("\n")

	// If tree view and not searching, use the file tree
	if s.treeView && s.search == "" {
		b.WriteString(s.fileTree.View())
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
			isDaily := len(displayName) >= 10 && displayName[4] == '-' && displayName[7] == '-'
			if isDaily {
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

		maxNameLen := contentWidth - len(indent) - 4
		if maxNameLen < 5 {
			maxNameLen = 5
		}
		if len(displayName) > maxNameLen {
			displayName = displayName[:maxNameLen-3] + "..."
		}

		if i == s.cursor && s.focused {
			// Selected item: accent bar + highlighted background
			accentBar := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar)
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
			line := accentBar + nameStyle.Render(indent+icon+iconSpace) + renderedName
			b.WriteString(lipgloss.NewStyle().Background(surface0).Width(contentWidth).Render(line))
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

func formatCount(filtered, total int) string {
	if filtered == total {
		return DimStyle.Render(strings.Repeat(" ", 0) + string(rune('0'+filtered%10)))
	}
	// Show "N/M files"
	return ""
}

func scrollIndicator(pct float64) string {
	if pct <= 0 {
		return "  TOP"
	}
	if pct >= 1 {
		return "  BOT"
	}
	p := int(pct * 100)
	return "  " + string(rune('0'+p/10)) + string(rune('0'+p%10)) + "%"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
