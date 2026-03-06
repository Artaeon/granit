package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

type focus int

const (
	focusSidebar focus = iota
	focusEditor
	focusBacklinks
)

type clearMessageMsg struct{}

type Model struct {
	vault     *vault.Vault
	index     *vault.Index
	sidebar   Sidebar
	editor    Editor
	backlinks Backlinks
	statusbar StatusBar

	focus      focus
	width      int
	height     int
	activeNote string
	quitting   bool

	// Search overlay
	searchMode    bool
	searchQuery   string
	searchResults []string
	searchCursor  int

	// New note overlay
	newNoteMode bool
	newNoteName string
}

type vaultScannedMsg struct{}

func NewModel(vaultPath string) (Model, error) {
	v, err := vault.NewVault(vaultPath)
	if err != nil {
		return Model{}, err
	}

	if err := v.Scan(); err != nil {
		return Model{}, err
	}

	idx := vault.NewIndex(v)
	idx.Build()

	paths := v.SortedPaths()

	m := Model{
		vault:     v,
		index:     idx,
		sidebar:   NewSidebar(paths),
		editor:    NewEditor(),
		backlinks: NewBacklinks(),
		statusbar: NewStatusBar(),
		focus:     focusSidebar,
	}

	m.statusbar.SetVaultPath(vaultPath)
	m.statusbar.SetNoteCount(v.NoteCount())

	if len(paths) > 0 {
		m.loadNote(paths[0])
	}

	return m, nil
}

func (m *Model) loadNote(relPath string) {
	note := m.vault.GetNote(relPath)
	if note == nil {
		return
	}
	m.activeNote = relPath
	m.editor.LoadContent(note.Content, relPath)
	m.statusbar.SetActiveNote(relPath)
	m.statusbar.SetWordCount(m.editor.GetWordCount())

	incoming := m.index.GetBacklinks(relPath)
	outgoing := m.index.GetOutgoingLinks(relPath)
	m.backlinks.SetLinks(incoming, outgoing)
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearMessageMsg:
		m.statusbar.SetMessage("")
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case tea.KeyMsg:
		// Search overlay mode
		if m.searchMode {
			return m.updateSearch(msg)
		}

		// New note overlay mode
		if m.newNoteMode {
			return m.updateNewNote(msg)
		}

		// Global keys (work regardless of focus)
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			m.quitting = true
			return m, tea.Quit

		case "ctrl+s":
			cmd := m.saveCurrentNote()
			m.statusbar.SetMessage("Saved " + m.activeNote)
			return m, tea.Batch(cmd, m.clearMessageAfter(2*time.Second))

		case "f1":
			m.setFocus(focusSidebar)
			return m, nil

		case "f2":
			m.setFocus(focusEditor)
			return m, nil

		case "f3":
			m.setFocus(focusBacklinks)
			return m, nil

		case "tab":
			// Cycle focus forward (but not when in editor to allow tab insertion)
			if m.focus != focusEditor {
				m.cycleFocus(1)
				return m, nil
			}

		case "shift+tab":
			m.cycleFocus(-1)
			return m, nil

		case "ctrl+p":
			m.searchMode = true
			m.searchQuery = ""
			m.searchResults = m.vault.SortedPaths()
			m.searchCursor = 0
			return m, nil

		case "ctrl+n":
			m.newNoteMode = true
			m.newNoteName = ""
			return m, nil

		case "esc":
			if m.focus == focusEditor || m.focus == focusBacklinks {
				m.setFocus(focusSidebar)
				return m, nil
			}

		case "enter":
			if m.focus == focusSidebar {
				selected := m.sidebar.Selected()
				if selected != "" {
					m.loadNote(selected)
					m.setFocus(focusEditor)
				}
				return m, nil
			}
			if m.focus == focusBacklinks {
				selected := m.backlinks.Selected()
				if selected != "" {
					resolved := m.resolveLink(selected)
					if resolved != "" {
						m.loadNote(resolved)
						m.sidebar.cursor = m.findFileIndex(resolved)
					}
				}
				return m, nil
			}
		}
	}

	// Delegate to focused component
	var cmd tea.Cmd
	switch m.focus {
	case focusSidebar:
		m.sidebar, cmd = m.sidebar.Update(msg)
	case focusEditor:
		m.editor, cmd = m.editor.Update(msg)
		// Update cursor position in status bar
		line, col := m.editor.GetCursor()
		m.statusbar.SetCursor(line, col)
		m.statusbar.SetWordCount(m.editor.GetWordCount())
	case focusBacklinks:
		m.backlinks, cmd = m.backlinks.Update(msg)
	}

	return m, cmd
}

func (m *Model) updateSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+p":
		m.searchMode = false
		return m, nil
	case "enter":
		if len(m.searchResults) > 0 && m.searchCursor < len(m.searchResults) {
			m.loadNote(m.searchResults[m.searchCursor])
			m.sidebar.cursor = m.findFileIndex(m.searchResults[m.searchCursor])
			m.setFocus(focusEditor)
		}
		m.searchMode = false
		return m, nil
	case "up", "ctrl+k":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil
	case "down", "ctrl+j":
		if m.searchCursor < len(m.searchResults)-1 {
			m.searchCursor++
		}
		return m, nil
	case "backspace":
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
			m.filterSearch()
		}
		return m, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			m.searchQuery += char
			m.filterSearch()
		}
		return m, nil
	}
}

func (m *Model) filterSearch() {
	if m.searchQuery == "" {
		m.searchResults = m.vault.SortedPaths()
		m.searchCursor = 0
		return
	}
	query := strings.ToLower(m.searchQuery)
	m.searchResults = nil
	for _, path := range m.vault.SortedPaths() {
		if fuzzyMatch(strings.ToLower(path), query) {
			m.searchResults = append(m.searchResults, path)
		}
	}
	// Also search content
	for _, path := range m.vault.SortedPaths() {
		note := m.vault.GetNote(path)
		if note != nil && strings.Contains(strings.ToLower(note.Content), query) {
			// Avoid duplicates
			found := false
			for _, r := range m.searchResults {
				if r == path {
					found = true
					break
				}
			}
			if !found {
				m.searchResults = append(m.searchResults, path)
			}
		}
	}
	if m.searchCursor >= len(m.searchResults) {
		m.searchCursor = maxInt(0, len(m.searchResults)-1)
	}
}

func (m *Model) updateNewNote(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.newNoteMode = false
		return m, nil
	case "enter":
		if m.newNoteName != "" {
			name := m.newNoteName
			if !strings.HasSuffix(name, ".md") {
				name += ".md"
			}
			path := filepath.Join(m.vault.Root, name)
			title := strings.TrimSuffix(filepath.Base(name), ".md")
			content := "---\ntitle: " + title + "\ndate: " + time.Now().Format("2006-01-02") + "\ntags: []\n---\n\n# " + title + "\n\n"

			if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
				if err := os.WriteFile(path, []byte(content), 0644); err == nil {
					m.vault.Scan()
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					m.sidebar.SetFiles(m.vault.SortedPaths())
					m.statusbar.SetNoteCount(m.vault.NoteCount())
					m.loadNote(name)
					m.sidebar.cursor = m.findFileIndex(name)
					m.setFocus(focusEditor)
					m.statusbar.SetMessage("Created " + name)
				}
			}
		}
		m.newNoteMode = false
		return m, tea.Batch(nil, m.clearMessageAfter(2*time.Second))
	case "backspace":
		if len(m.newNoteName) > 0 {
			m.newNoteName = m.newNoteName[:len(m.newNoteName)-1]
		}
		return m, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			m.newNoteName += char
		}
		return m, nil
	}
}

func (m *Model) resolveLink(link string) string {
	if m.vault.GetNote(link) != nil {
		return link
	}
	if !strings.HasSuffix(link, ".md") {
		withMd := link + ".md"
		if m.vault.GetNote(withMd) != nil {
			return withMd
		}
	}
	base := filepath.Base(link)
	if !strings.HasSuffix(base, ".md") {
		base += ".md"
	}
	for _, p := range m.vault.SortedPaths() {
		if filepath.Base(p) == base {
			return p
		}
	}
	return ""
}

func (m *Model) findFileIndex(relPath string) int {
	for i, f := range m.sidebar.filtered {
		if f == relPath {
			return i
		}
	}
	return 0
}

func (m *Model) setFocus(f focus) {
	m.focus = f
	m.sidebar.focused = (f == focusSidebar)
	m.editor.focused = (f == focusEditor)
	m.backlinks.focused = (f == focusBacklinks)

	switch f {
	case focusSidebar:
		m.statusbar.SetMode("FILES")
	case focusEditor:
		m.statusbar.SetMode("EDIT")
	case focusBacklinks:
		m.statusbar.SetMode("LINKS")
	}
}

func (m *Model) cycleFocus(direction int) {
	newFocus := (int(m.focus) + direction + 3) % 3
	m.setFocus(focus(newFocus))
}

func (m *Model) updateLayout() {
	sidebarWidth := m.width / 5
	if sidebarWidth < 22 {
		sidebarWidth = 22
	}
	if sidebarWidth > 35 {
		sidebarWidth = 35
	}
	backlinksWidth := m.width / 5
	if backlinksWidth < 22 {
		backlinksWidth = 22
	}
	if backlinksWidth > 30 {
		backlinksWidth = 30
	}
	editorWidth := m.width - sidebarWidth - backlinksWidth - 6

	contentHeight := m.height - 3 // status + help bars

	m.sidebar.SetSize(sidebarWidth, contentHeight)
	m.editor.SetSize(editorWidth, contentHeight)
	m.backlinks.SetSize(backlinksWidth, contentHeight)
	m.statusbar.SetWidth(m.width)
}

func (m Model) saveCurrentNote() tea.Cmd {
	return func() tea.Msg {
		if m.activeNote == "" {
			return nil
		}
		content := m.editor.GetContent()
		path := filepath.Join(m.vault.Root, m.activeNote)
		os.WriteFile(path, []byte(content), 0644)
		return nil
	}
}

func (m Model) clearMessageAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return clearMessageMsg{}
	})
}

func (m Model) View() string {
	if m.quitting {
		return lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("\n  Goodbye from Granit!\n\n")
	}

	if m.width == 0 {
		return lipgloss.NewStyle().Foreground(mauve).Render("\n  Loading Granit...")
	}

	sidebarWidth := m.width / 5
	if sidebarWidth < 22 {
		sidebarWidth = 22
	}
	if sidebarWidth > 35 {
		sidebarWidth = 35
	}
	backlinksWidth := m.width / 5
	if backlinksWidth < 22 {
		backlinksWidth = 22
	}
	if backlinksWidth > 30 {
		backlinksWidth = 30
	}
	editorWidth := m.width - sidebarWidth - backlinksWidth - 6

	contentHeight := m.height - 3

	// Apply focus-aware borders
	sidebarBorderColor := surface1
	editorBorderColor := surface1
	backlinksBorderColor := surface1

	switch m.focus {
	case focusSidebar:
		sidebarBorderColor = FocusedBorderColor
	case focusEditor:
		editorBorderColor = FocusedBorderColor
	case focusBacklinks:
		backlinksBorderColor = FocusedBorderColor
	}

	sidebar := SidebarStyle.Copy().
		BorderForeground(sidebarBorderColor).
		Width(sidebarWidth).
		Height(contentHeight).
		Render(m.sidebar.View())

	editor := EditorStyle.Copy().
		BorderForeground(editorBorderColor).
		Width(editorWidth).
		Height(contentHeight).
		Render(m.editor.View())

	backlinks := BacklinksStyle.Copy().
		BorderForeground(backlinksBorderColor).
		Width(backlinksWidth).
		Height(contentHeight).
		Render(m.backlinks.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor, backlinks)
	status := m.statusbar.View()

	view := lipgloss.JoinVertical(lipgloss.Left, content, status)

	// Overlay: Search
	if m.searchMode {
		overlay := m.renderSearchOverlay()
		view = m.overlayCenter(view, overlay)
	}

	// Overlay: New Note
	if m.newNoteMode {
		overlay := m.renderNewNoteOverlay()
		view = m.overlayCenter(view, overlay)
	}

	return view
}

func (m Model) renderSearchOverlay() string {
	width := m.width / 2
	if width < 40 {
		width = 40
	}
	if width > 80 {
		width = 80
	}

	var b strings.Builder

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Quick Open")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Search input
	prompt := SearchPromptStyle.Render(" > ")
	input := m.searchQuery + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-4)))
	b.WriteString("\n")

	// Results
	maxResults := 10
	if len(m.searchResults) == 0 {
		b.WriteString(DimStyle.Render("  No results"))
	} else {
		for i := 0; i < len(m.searchResults) && i < maxResults; i++ {
			name := strings.TrimSuffix(m.searchResults[i], ".md")
			icon := lipgloss.NewStyle().Foreground(blue).Render(" ")
			if i == m.searchCursor {
				line := lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 4).
					Render("  " + icon + " " + name)
				b.WriteString(line)
			} else {
				b.WriteString("  " + icon + " " + NormalItemStyle.Render(name))
			}
			if i < len(m.searchResults)-1 && i < maxResults-1 {
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

func (m Model) renderNewNoteOverlay() string {
	width := m.width / 3
	if width < 40 {
		width = 40
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  New Note")
	b.WriteString(title)
	b.WriteString("\n\n")

	prompt := lipgloss.NewStyle().Foreground(green).Bold(true).Render(" Name: ")
	input := m.newNoteName + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Enter to create, Esc to cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(green).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m Model) overlayCenter(bg, overlay string) string {
	bgLines := strings.Split(bg, "\n")
	overlayLines := strings.Split(overlay, "\n")

	overlayHeight := len(overlayLines)
	overlayWidth := 0
	for _, line := range overlayLines {
		w := lipgloss.Width(line)
		if w > overlayWidth {
			overlayWidth = w
		}
	}

	startY := (m.height - overlayHeight) / 3
	startX := (m.width - overlayWidth) / 2
	if startY < 1 {
		startY = 1
	}
	if startX < 0 {
		startX = 0
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	for i, overlayLine := range overlayLines {
		y := startY + i
		if y >= len(result) {
			break
		}
		bgLine := result[y]
		bgRunes := []rune(bgLine)

		// Pad bg line if needed
		for len(bgRunes) < startX+lipgloss.Width(overlayLine) {
			bgRunes = append(bgRunes, ' ')
		}

		// Simple overlay: replace characters
		newLine := string(bgRunes[:startX]) + overlayLine
		if startX+lipgloss.Width(overlayLine) < len(bgRunes) {
			newLine += string(bgRunes[startX+lipgloss.Width(overlayLine):])
		}
		result[y] = newLine
	}

	return strings.Join(result, "\n")
}
