package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type CommandAction int

const (
	CmdNone CommandAction = iota
	CmdOpenFile
	CmdNewNote
	CmdSaveNote
	CmdDailyNote
	CmdToggleView
	CmdSettings
	CmdToggleSidebar
	CmdFocusEditor
	CmdFocusSidebar
	CmdFocusBacklinks
	CmdSearchInFile
	CmdRefreshVault
	CmdDeleteNote
	CmdRenameNote
	CmdShowGraph
	CmdShowTags
	CmdShowHelp
	CmdShowOutline
	CmdShowBookmarks
	CmdToggleBookmark
	CmdFindInFile
	CmdReplaceInFile
	CmdShowStats
	CmdNewFromTemplate
	CmdFocusMode
	CmdQuickSwitch
	CmdShowTrash
	CmdQuit
)

type Command struct {
	Label    string
	Desc     string
	Shortcut string
	Action   CommandAction
}

var AllCommands = []Command{
	{Label: "Open File", Desc: "Quick open a file", Shortcut: "Ctrl+P", Action: CmdOpenFile},
	{Label: "New Note", Desc: "Create a new note", Shortcut: "Ctrl+N", Action: CmdNewNote},
	{Label: "Save Note", Desc: "Save the current note", Shortcut: "Ctrl+S", Action: CmdSaveNote},
	{Label: "Daily Note", Desc: "Open or create today's daily note", Shortcut: "", Action: CmdDailyNote},
	{Label: "Toggle View/Edit", Desc: "Switch between view and edit mode", Shortcut: "Ctrl+E", Action: CmdToggleView},
	{Label: "Settings", Desc: "Open settings panel", Shortcut: "Ctrl+,", Action: CmdSettings},
	{Label: "Focus Editor", Desc: "Switch focus to the editor", Shortcut: "F2", Action: CmdFocusEditor},
	{Label: "Focus Sidebar", Desc: "Switch focus to the file sidebar", Shortcut: "F1", Action: CmdFocusSidebar},
	{Label: "Focus Backlinks", Desc: "Switch focus to the backlinks panel", Shortcut: "F3", Action: CmdFocusBacklinks},
	{Label: "Refresh Vault", Desc: "Rescan vault for changes", Shortcut: "", Action: CmdRefreshVault},
	{Label: "Delete Note", Desc: "Delete the current note", Shortcut: "", Action: CmdDeleteNote},
	{Label: "Rename Note", Desc: "Rename the current note", Shortcut: "F4", Action: CmdRenameNote},
	{Label: "Show Graph", Desc: "Show note connection graph", Shortcut: "Ctrl+G", Action: CmdShowGraph},
	{Label: "Show Tags", Desc: "Browse notes by tags", Shortcut: "Ctrl+T", Action: CmdShowTags},
	{Label: "Help", Desc: "Show keyboard shortcuts", Shortcut: "F5", Action: CmdShowHelp},
	{Label: "Outline", Desc: "Show note heading outline", Shortcut: "Ctrl+O", Action: CmdShowOutline},
	{Label: "Bookmarks", Desc: "View starred & recent notes", Shortcut: "Ctrl+B", Action: CmdShowBookmarks},
	{Label: "Toggle Bookmark", Desc: "Star/unstar current note", Shortcut: "", Action: CmdToggleBookmark},
	{Label: "Find", Desc: "Search within current file", Shortcut: "Ctrl+F", Action: CmdFindInFile},
	{Label: "Find & Replace", Desc: "Find and replace in file", Shortcut: "Ctrl+H", Action: CmdReplaceInFile},
	{Label: "Vault Statistics", Desc: "Show vault stats & charts", Shortcut: "", Action: CmdShowStats},
	{Label: "New from Template", Desc: "Create note from template", Shortcut: "", Action: CmdNewFromTemplate},
	{Label: "Focus Mode", Desc: "Distraction-free writing", Shortcut: "Ctrl+Z", Action: CmdFocusMode},
	{Label: "Quick Switch", Desc: "Switch between recent files", Shortcut: "Ctrl+J", Action: CmdQuickSwitch},
	{Label: "Trash", Desc: "View and restore deleted notes", Shortcut: "", Action: CmdShowTrash},
	{Label: "Quit", Desc: "Exit Granit", Shortcut: "Ctrl+Q", Action: CmdQuit},
}

type CommandPalette struct {
	active   bool
	query    string
	filtered []Command
	cursor   int
	width    int
	height   int
	result   CommandAction
}

func NewCommandPalette() CommandPalette {
	return CommandPalette{
		filtered: AllCommands,
	}
}

func (cp *CommandPalette) SetSize(width, height int) {
	cp.width = width
	cp.height = height
}

func (cp *CommandPalette) Open() {
	cp.active = true
	cp.query = ""
	cp.filtered = AllCommands
	cp.cursor = 0
	cp.result = CmdNone
}

func (cp *CommandPalette) Close() {
	cp.active = false
	cp.query = ""
}

func (cp *CommandPalette) IsActive() bool {
	return cp.active
}

func (cp *CommandPalette) Result() CommandAction {
	r := cp.result
	cp.result = CmdNone
	return r
}

func (cp *CommandPalette) filterCommands() {
	if cp.query == "" {
		cp.filtered = AllCommands
		return
	}
	query := strings.ToLower(cp.query)
	cp.filtered = nil
	for _, cmd := range AllCommands {
		if cmdFuzzyMatch(strings.ToLower(cmd.Label), query) ||
			cmdFuzzyMatch(strings.ToLower(cmd.Desc), query) {
			cp.filtered = append(cp.filtered, cmd)
		}
	}
	if cp.cursor >= len(cp.filtered) {
		cp.cursor = maxInt(0, len(cp.filtered)-1)
	}
}

// cmdFuzzyMatch performs a fuzzy substring match for command filtering.
// This is separate from the sidebar's fuzzyMatch to keep the command palette
// self-contained and allow independent tuning of matching behavior.
func cmdFuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

func (cp CommandPalette) Update(msg tea.Msg) (CommandPalette, tea.Cmd) {
	if !cp.active {
		return cp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			cp.active = false
			return cp, nil
		case "enter":
			if len(cp.filtered) > 0 && cp.cursor < len(cp.filtered) {
				cp.result = cp.filtered[cp.cursor].Action
			}
			cp.active = false
			return cp, nil
		case "up", "ctrl+k":
			if cp.cursor > 0 {
				cp.cursor--
			}
			return cp, nil
		case "down", "ctrl+j":
			if cp.cursor < len(cp.filtered)-1 {
				cp.cursor++
			}
			return cp, nil
		case "backspace":
			if len(cp.query) > 0 {
				cp.query = cp.query[:len(cp.query)-1]
				cp.filterCommands()
			}
			return cp, nil
		default:
			char := msg.String()
			if len(char) == 1 && char[0] >= 32 {
				cp.query += char
				cp.filterCommands()
			}
			return cp, nil
		}
	}
	return cp, nil
}

func (cp CommandPalette) View() string {
	width := cp.width / 2
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
		Render("  Command Palette")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Search input
	prompt := SearchPromptStyle.Render(" > ")
	input := cp.query + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")

	// Results
	maxVisible := 12
	if len(cp.filtered) == 0 {
		b.WriteString(DimStyle.Render("  No commands found"))
	} else {
		start := 0
		if cp.cursor >= maxVisible {
			start = cp.cursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(cp.filtered) {
			end = len(cp.filtered)
		}

		for i := start; i < end; i++ {
			cmd := cp.filtered[i]

			label := cmd.Label
			shortcut := ""
			if cmd.Shortcut != "" {
				shortcut = lipgloss.NewStyle().
					Foreground(overlay0).
					Render(" " + cmd.Shortcut)
			}

			desc := DimStyle.Render("  " + cmd.Desc)

			if i == cp.cursor {
				selected := lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true)

				line := "  " + label
				b.WriteString(selected.Width(width - 6).Render(line + shortcut))
			} else {
				b.WriteString("  " + NormalItemStyle.Render(label) + shortcut)
			}
			b.WriteString("\n")
			if i == cp.cursor {
				b.WriteString(lipgloss.NewStyle().Background(surface0).Width(width - 6).Render(desc))
			} else {
				b.WriteString(desc)
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
