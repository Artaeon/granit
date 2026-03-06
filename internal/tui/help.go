package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type HelpOverlay struct {
	active bool
	width  int
	height int
	scroll int
}

func NewHelpOverlay() HelpOverlay {
	return HelpOverlay{}
}

func (h *HelpOverlay) SetSize(width, height int) {
	h.width = width
	h.height = height
}

func (h *HelpOverlay) Toggle() {
	h.active = !h.active
	h.scroll = 0
}

func (h *HelpOverlay) IsActive() bool {
	return h.active
}

type helpSection struct {
	title    string
	bindings []helpBinding
}

type helpBinding struct {
	key  string
	desc string
}

var helpSections = []helpSection{
	{
		title: "Navigation",
		bindings: []helpBinding{
			{"Tab / Shift+Tab", "Cycle between panels"},
			{"F1", "Focus file sidebar"},
			{"F2", "Focus editor"},
			{"F3", "Focus backlinks panel"},
			{"Esc", "Return to sidebar / close overlay"},
			{"j / k / ↑ / ↓", "Navigate up/down"},
			{"Enter", "Open selected file/link"},
		},
	},
	{
		title: "File Operations",
		bindings: []helpBinding{
			{"Ctrl+P", "Quick open (fuzzy search)"},
			{"Ctrl+N", "Create new note"},
			{"Ctrl+S", "Save current note"},
			{"F4", "Rename current note"},
			{"Ctrl+Shift+P", "Command palette"},
		},
	},
	{
		title: "Editor",
		bindings: []helpBinding{
			{"Ctrl+E", "Toggle view/edit mode"},
			{"← / → / ↑ / ↓", "Move cursor"},
			{"Home / Ctrl+A", "Go to line start"},
			{"End / Ctrl+E", "Go to line end"},
			{"PgUp / PgDown", "Scroll page"},
			{"Ctrl+U", "Undo"},
			{"Ctrl+Y", "Redo"},
			{"Ctrl+K", "Delete to end of line"},
			{"Ctrl+D / Delete", "Delete character forward"},
			{"Tab", "Insert 4 spaces"},
		},
	},
	{
		title: "Views & Tools",
		bindings: []helpBinding{
			{"Ctrl+G", "Show note graph"},
			{"Ctrl+T", "Browse tags"},
			{"Ctrl+O", "Show note outline"},
			{"Ctrl+B", "Bookmarks & recent notes"},
			{"Ctrl+F", "Find in file"},
			{"Ctrl+H", "Find & replace in file"},
			{"Ctrl+J", "Quick switch files"},
			{"Ctrl+W", "Visual canvas / whiteboard"},
			{"Ctrl+L", "Calendar (month/week/agenda)"},
			{"Ctrl+R", "AI bots (Ollama / local)"},
			{"Ctrl+Z", "Focus / zen mode"},
			{"Ctrl+,", "Open settings"},
			{"F5", "Show this help"},
		},
	},
	{
		title: "Sidebar",
		bindings: []helpBinding{
			{"Type", "Fuzzy search files"},
			{"Backspace", "Clear search character"},
			{"Esc", "Clear search"},
		},
	},
	{
		title: "Backlinks Panel",
		bindings: []helpBinding{
			{"Tab", "Toggle backlinks/outgoing"},
			{"Enter", "Navigate to linked note"},
		},
	},
	{
		title: "Application",
		bindings: []helpBinding{
			{"Ctrl+Q / Ctrl+C", "Quit Granit"},
		},
	},
}

func (h HelpOverlay) Update(msg tea.Msg) (HelpOverlay, tea.Cmd) {
	if !h.active {
		return h, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "f5", "q":
			h.active = false
		case "up", "k":
			if h.scroll > 0 {
				h.scroll--
			}
		case "down", "j":
			h.scroll++
		}
	}
	return h, nil
}

func (h HelpOverlay) View() string {
	width := h.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 90 {
		width = 90
	}

	var b strings.Builder

	// Header
	logo := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconHelpChar + " Granit — Keyboard Shortcuts")
	b.WriteString(logo)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	// Build all lines
	var allLines []string
	for _, section := range helpSections {
		allLines = append(allLines, "")
		sectionTitle := lipgloss.NewStyle().
			Foreground(blue).
			Bold(true).
			Render("  " + section.title)
		allLines = append(allLines, sectionTitle)
		allLines = append(allLines, DimStyle.Render("  "+strings.Repeat("─", 30)))

		for _, binding := range section.bindings {
			keyStyle := lipgloss.NewStyle().
				Foreground(lavender).
				Bold(true).
				Width(22).
				Render("    " + binding.key)
			descStyle := lipgloss.NewStyle().
				Foreground(text).
				Render(binding.desc)
			allLines = append(allLines, keyStyle+descStyle)
		}
	}

	// Apply scroll
	visH := h.height - 8
	if visH < 10 {
		visH = 10
	}

	maxScroll := len(allLines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if h.scroll > maxScroll {
		h.scroll = maxScroll
	}

	end := h.scroll + visH
	if end > len(allLines) {
		end = len(allLines)
	}

	for i := h.scroll; i < end; i++ {
		b.WriteString(allLines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  j/k: scroll  Esc/F5/q: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
