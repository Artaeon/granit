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
}

func NewSidebar(files []string) Sidebar {
	return Sidebar{
		files:    files,
		filtered: files,
		cursor:   0,
		focused:  true,
	}
}

func (s *Sidebar) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *Sidebar) SetFiles(files []string) {
	s.files = files
	s.applyFilter()
}

func (s *Sidebar) applyFilter() {
	if s.search == "" {
		s.filtered = s.files
		return
	}
	query := strings.ToLower(s.search)
	s.filtered = nil
	for _, f := range s.files {
		if fuzzyMatch(strings.ToLower(f), query) {
			s.filtered = append(s.filtered, f)
		}
	}
	if s.cursor >= len(s.filtered) {
		s.cursor = maxInt(0, len(s.filtered)-1)
	}
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

func (s *Sidebar) Selected() string {
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
				visibleHeight := s.height - 6
				if visibleHeight < 1 {
					visibleHeight = 1
				}
				if s.cursor >= s.scroll+visibleHeight {
					s.scroll = s.cursor - visibleHeight + 1
				}
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

	// Header with icon
	header := HeaderStyle.Render("  Explorer")
	b.WriteString(header)
	b.WriteString("\n")

	// Search bar
	if s.search != "" || s.focused {
		searchIcon := SearchPromptStyle.Render("  ")
		searchText := s.search
		if s.focused {
			searchText += DimStyle.Render("_")
		}
		searchBg := SearchInputStyle.Width(contentWidth - 4)
		b.WriteString(searchIcon + searchBg.Render(searchText))
	} else {
		b.WriteString(DimStyle.Render("  search..."))
	}
	b.WriteString("\n")

	// Separator
	b.WriteString(DimStyle.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	// File count
	countStr := DimStyle.Render(strings.Repeat(" ", 1) +
		formatCount(len(s.filtered), len(s.files)))
	b.WriteString(countStr)
	b.WriteString("\n")

	// File list
	visibleHeight := s.height - 6
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	if len(s.filtered) == 0 {
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  No files found"))
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
			folderIcon := ""
			if s.showIcons {
				folderIcon = "  "
			}
			dirDisplay := "  " + lipgloss.NewStyle().Foreground(peach).Render(folderIcon + dir + "/")
			b.WriteString(dirDisplay)
			if !s.compactMode {
				b.WriteString("\n")
			} else {
				b.WriteString("\n")
			}
			lastDir = dir
		}

		// Strip .md extension for cleaner display
		displayName := strings.TrimSuffix(name, ".md")

		// File icon (conditional)
		icon := ""
		if s.showIcons {
			icon = lipgloss.NewStyle().Foreground(blue).Render(" ")
			// Check if it's a daily note
			if len(displayName) >= 10 && displayName[4] == '-' && displayName[7] == '-' {
				icon = lipgloss.NewStyle().Foreground(green).Render(" ")
			}
		}

		indent := "  "
		if dir != "." {
			indent = "    "
		}
		if s.compactMode {
			indent = " "
			if dir != "." {
				indent = "  "
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
			// Full-width highlight for selected item
			line := indent + icon + iconSpace + displayName
			padLen := contentWidth - lipgloss.Width(line)
			if padLen < 0 {
				padLen = 0
			}
			highlighted := lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				Width(contentWidth).
				Render(indent + icon + iconSpace + displayName + strings.Repeat(" ", padLen))
			b.WriteString(highlighted)
		} else {
			b.WriteString(indent + icon + iconSpace + NormalItemStyle.Render(displayName))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Scroll indicator
	if len(s.filtered) > visibleHeight {
		pct := float64(s.scroll) / float64(len(s.filtered)-visibleHeight)
		indicator := DimStyle.Render(scrollIndicator(pct))
		b.WriteString("\n" + indicator)
	}

	return b.String()
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
