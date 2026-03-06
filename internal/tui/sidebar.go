package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
		s.cursor = max(0, len(s.filtered)-1)
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
				visibleHeight := s.height - 4 // account for header, search, borders
				if s.cursor >= s.scroll+visibleHeight {
					s.scroll = s.cursor - visibleHeight + 1
				}
			}
		case "backspace":
			if len(s.search) > 0 {
				s.search = s.search[:len(s.search)-1]
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

	header := HeaderStyle.Render("Files")
	b.WriteString(header)
	b.WriteString("\n")

	searchLine := DimStyle.Render("> ") + s.search
	if s.focused {
		searchLine += "_"
	}
	b.WriteString(searchLine)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", s.width-4)))
	b.WriteString("\n")

	visibleHeight := s.height - 4
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	end := s.scroll + visibleHeight
	if end > len(s.filtered) {
		end = len(s.filtered)
	}

	for i := s.scroll; i < end; i++ {
		name := s.filtered[i]
		if len(name) > s.width-6 {
			name = name[:s.width-9] + "..."
		}
		if i == s.cursor && s.focused {
			b.WriteString(SelectedStyle.Render("▸ " + name))
		} else {
			b.WriteString("  " + name)
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
