package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
)

type settingItem struct {
	label   string
	key     string
	kind    string // "bool", "string", "int"
	value   interface{}
	options []string // for string types with limited options
}

type Settings struct {
	config  config.Config
	items   []settingItem
	cursor  int
	scroll  int
	width   int
	height  int
	active  bool
	editing bool
	editBuf string
}

func NewSettings(cfg config.Config) Settings {
	s := Settings{
		config: cfg,
	}
	s.buildItems()
	return s
}

func (s *Settings) buildItems() {
	s.items = []settingItem{
		// Editor settings
		{label: "Show Splash Screen", key: "show_splash", kind: "bool", value: s.config.ShowSplash},
		{label: "Show Help Bar", key: "show_help", kind: "bool", value: s.config.ShowHelp},
		{label: "Line Numbers", key: "line_numbers", kind: "bool", value: s.config.LineNumbers},
		{label: "Word Wrap", key: "word_wrap", kind: "bool", value: s.config.WordWrap},
		{label: "Auto Save", key: "auto_save", kind: "bool", value: s.config.AutoSave},
		{label: "Default View Mode", key: "default_view_mode", kind: "bool", value: s.config.DefaultViewMode},
		{label: "Vim Mode", key: "vim_mode", kind: "bool", value: s.config.VimMode},
		{label: "Tab Size", key: "tab_size", kind: "int", value: s.config.Editor.TabSize},
		{label: "Auto Close Brackets", key: "auto_close_brackets", kind: "bool", value: s.config.AutoCloseBrackets},
		{label: "Highlight Current Line", key: "highlight_current_line", kind: "bool", value: s.config.HighlightCurrentLine},

		// Appearance settings
		{label: "Theme", key: "theme", kind: "string", value: s.config.Theme, options: ThemeNames()},
		{label: "Sidebar Position", key: "sidebar_position", kind: "string", value: s.config.SidebarPosition, options: []string{"left", "right"}},
		{label: "Show Icons", key: "show_icons", kind: "bool", value: s.config.ShowIcons},
		{label: "Compact Mode", key: "compact_mode", kind: "bool", value: s.config.CompactMode},

		// Sidebar & Search
		{label: "Sort Files By", key: "sort_by", kind: "string", value: s.config.SortBy, options: []string{"name", "modified", "created"}},
		{label: "Daily Notes Folder", key: "daily_notes_folder", kind: "string", value: s.config.DailyNotesFolder},
		{label: "Search Content by Default", key: "search_content", kind: "bool", value: s.config.SearchContentByDefault},

		// Behavior
		{label: "Confirm Delete", key: "confirm_delete", kind: "bool", value: s.config.ConfirmDelete},
		{label: "Auto Refresh Vault", key: "auto_refresh", kind: "bool", value: s.config.AutoRefresh},
	}
}

func (s *Settings) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *Settings) GetConfig() config.Config {
	return s.config
}

func (s *Settings) Toggle() {
	s.active = !s.active
	if s.active {
		s.buildItems()
	}
}

func (s *Settings) IsActive() bool {
	return s.active
}

func (s Settings) Update(msg tea.Msg) (Settings, tea.Cmd) {
	if !s.active {
		return s, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.editing {
			switch msg.String() {
			case "esc":
				s.editing = false
				s.editBuf = ""
			case "enter":
				s.applyEdit()
				s.editing = false
				s.editBuf = ""
			case "backspace":
				if len(s.editBuf) > 0 {
					s.editBuf = s.editBuf[:len(s.editBuf)-1]
				}
			default:
				char := msg.String()
				if len(char) == 1 {
					s.editBuf += char
				}
			}
			return s, nil
		}

		switch msg.String() {
		case "esc", "ctrl+,":
			s.active = false
			return s, nil
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
			}
		case "down", "j":
			if s.cursor < len(s.items)-1 {
				s.cursor++
			}
		case "enter", " ":
			item := &s.items[s.cursor]
			switch item.kind {
			case "bool":
				val := item.value.(bool)
				item.value = !val
				s.applyValue(item.key, !val)
			case "string":
				if len(item.options) > 0 {
					// Cycle through options
					current := item.value.(string)
					for i, opt := range item.options {
						if opt == current {
							next := item.options[(i+1)%len(item.options)]
							item.value = next
							s.applyValue(item.key, next)
							break
						}
					}
				} else {
					s.editing = true
					if v, ok := item.value.(string); ok {
						s.editBuf = v
					}
				}
			case "int":
				s.editing = true
				// Simple: just start editing
				s.editBuf = ""
			}
		}
	}
	return s, nil
}

func (s *Settings) applyValue(key string, value interface{}) {
	switch key {
	case "show_splash":
		s.config.ShowSplash = value.(bool)
	case "show_help":
		s.config.ShowHelp = value.(bool)
	case "line_numbers":
		s.config.LineNumbers = value.(bool)
	case "word_wrap":
		s.config.WordWrap = value.(bool)
	case "auto_save":
		s.config.AutoSave = value.(bool)
	case "default_view_mode":
		s.config.DefaultViewMode = value.(bool)
	case "vim_mode":
		s.config.VimMode = value.(bool)
	case "sort_by":
		s.config.SortBy = value.(string)
	case "daily_notes_folder":
		s.config.DailyNotesFolder = value.(string)
	case "theme":
		s.config.Theme = value.(string)
		ApplyTheme(s.config.Theme)
	case "search_content":
		s.config.SearchContentByDefault = value.(bool)
	case "auto_close_brackets":
		s.config.AutoCloseBrackets = value.(bool)
	case "highlight_current_line":
		s.config.HighlightCurrentLine = value.(bool)
	case "sidebar_position":
		s.config.SidebarPosition = value.(string)
	case "show_icons":
		s.config.ShowIcons = value.(bool)
	case "compact_mode":
		s.config.CompactMode = value.(bool)
	case "confirm_delete":
		s.config.ConfirmDelete = value.(bool)
	case "auto_refresh":
		s.config.AutoRefresh = value.(bool)
	}
}

func (s *Settings) applyEdit() {
	item := &s.items[s.cursor]
	switch item.kind {
	case "string":
		item.value = s.editBuf
		s.applyValue(item.key, s.editBuf)
	case "int":
		n := 0
		for _, ch := range s.editBuf {
			if ch >= '0' && ch <= '9' {
				n = n*10 + int(ch-'0')
			}
		}
		if n > 0 {
			item.value = n
			if item.key == "tab_size" {
				s.config.Editor.TabSize = n
			}
		}
	}
}

func (s Settings) View() string {
	width := s.width * 2 / 3
	if width < 50 {
		width = 50
	}
	if width > 80 {
		width = 80
	}

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Settings")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n\n")

	visibleItems := s.height - 8
	if visibleItems < 5 {
		visibleItems = 5
	}

	start := 0
	if s.cursor >= visibleItems {
		start = s.cursor - visibleItems + 1
	}

	end := start + visibleItems
	if end > len(s.items) {
		end = len(s.items)
	}

	for i := start; i < end; i++ {
		item := s.items[i]
		isSelected := i == s.cursor

		label := item.label
		var valueStr string

		switch item.kind {
		case "bool":
			if item.value.(bool) {
				valueStr = lipgloss.NewStyle().Foreground(green).Render("● ON")
			} else {
				valueStr = lipgloss.NewStyle().Foreground(red).Render("○ OFF")
			}
		case "string":
			if s.editing && isSelected {
				valueStr = s.editBuf + DimStyle.Render("_")
			} else if v, ok := item.value.(string); ok {
				if v == "" {
					valueStr = DimStyle.Render("(not set)")
				} else {
					valueStr = lipgloss.NewStyle().Foreground(blue).Render(v)
				}
			}
		case "int":
			if s.editing && isSelected {
				valueStr = s.editBuf + DimStyle.Render("_")
			} else {
				valueStr = lipgloss.NewStyle().Foreground(peach).Render(intToStr(item.value))
			}
		}

		// Calculate padding
		labelWidth := width - 20
		if labelWidth < 20 {
			labelWidth = 20
		}
		if len(label) > labelWidth {
			label = label[:labelWidth]
		}
		padding := labelWidth - len(label)
		if padding < 1 {
			padding = 1
		}

		line := "  " + label + strings.Repeat(" ", padding) + valueStr

		if isSelected {
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				Width(width - 6).
				Render(line))
		} else {
			b.WriteString(NormalItemStyle.Render(line))
		}
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter/Space: toggle  Esc: close  Changes auto-save"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func intToStr(v interface{}) string {
	switch val := v.(type) {
	case int:
		if val == 0 {
			return "0"
		}
		s := ""
		n := val
		for n > 0 {
			s = string(rune('0'+n%10)) + s
			n /= 10
		}
		return s
	default:
		return "?"
	}
}
