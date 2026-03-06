package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
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
	renderer  Renderer
	backlinks Backlinks
	statusbar StatusBar
	config    config.Config

	focus      focus
	width      int
	height     int
	activeNote string
	quitting   bool

	// View/Edit mode
	viewMode bool

	// Splash screen
	splash     SplashModel
	showSplash bool

	// Overlays
	searchMode    bool
	searchQuery   string
	searchResults []string
	searchCursor  int

	newNoteMode bool
	newNoteName string

	commandPalette CommandPalette
	settings       Settings
	graphView      GraphView
	tagBrowser     TagBrowser
	helpOverlay    HelpOverlay

	// View mode scroll
	viewScroll int
}

func NewModel(vaultPath string) (Model, error) {
	cfg := config.LoadForVault(vaultPath)

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
		vault:          v,
		index:          idx,
		sidebar:        NewSidebar(paths),
		editor:         NewEditor(),
		renderer:       NewRenderer(),
		backlinks:      NewBacklinks(),
		statusbar:      NewStatusBar(),
		config:         cfg,
		focus:          focusSidebar,
		commandPalette: NewCommandPalette(),
		settings:       NewSettings(cfg),
		graphView:      NewGraphView(v, idx),
		tagBrowser:     NewTagBrowser(v),
		helpOverlay:    NewHelpOverlay(),
		showSplash:     cfg.ShowSplash,
		splash:         NewSplashModel(vaultPath, v.NoteCount()),
		viewMode:       cfg.DefaultViewMode,
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
	m.viewScroll = 0

	incoming := m.index.GetBacklinks(relPath)
	outgoing := m.index.GetOutgoingLinks(relPath)
	m.backlinks.SetLinks(incoming, outgoing)
}

func (m Model) Init() tea.Cmd {
	if m.showSplash {
		return m.splash.Init()
	}
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Splash screen phase
	if m.showSplash {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.splash.width = msg.Width
			m.splash.height = msg.Height
			m.updateLayout()
		case tea.KeyMsg:
			m.splash.done = true
		case splashTickMsg:
			var cmd tea.Cmd
			m.splash, cmd = m.splash.Update(msg)
			if m.splash.IsDone() {
				m.showSplash = false
				return m, nil
			}
			return m, cmd
		}

		if m.splash.IsDone() {
			m.showSplash = false
			return m, nil
		}

		var cmd tea.Cmd
		m.splash, cmd = m.splash.Update(msg)
		return m, cmd
	}

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
		// Handle overlay modes first (in priority order)
		if m.helpOverlay.IsActive() {
			m.helpOverlay, _ = m.helpOverlay.Update(msg)
			return m, nil
		}

		if m.settings.IsActive() {
			m.settings, _ = m.settings.Update(msg)
			if !m.settings.IsActive() {
				m.config = m.settings.GetConfig()
				m.config.Save()
			}
			return m, nil
		}

		if m.graphView.IsActive() {
			m.graphView, _ = m.graphView.Update(msg)
			if nav := m.graphView.SelectedNote(); nav != "" {
				m.loadNote(nav)
				m.sidebar.cursor = m.findFileIndex(nav)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.tagBrowser.IsActive() {
			m.tagBrowser, _ = m.tagBrowser.Update(msg)
			if nav := m.tagBrowser.SelectedNote(); nav != "" {
				m.loadNote(nav)
				m.sidebar.cursor = m.findFileIndex(nav)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.commandPalette.IsActive() {
			m.commandPalette, _ = m.commandPalette.Update(msg)
			if action := m.commandPalette.Result(); action != CmdNone {
				return m.executeCommand(action)
			}
			return m, nil
		}

		if m.searchMode {
			return m.updateSearch(msg)
		}

		if m.newNoteMode {
			return m.updateNewNote(msg)
		}

		// Global keys
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

		case "f4":
			// Rename note
			if m.activeNote != "" {
				m.newNoteMode = true
				m.newNoteName = strings.TrimSuffix(m.activeNote, ".md")
			}
			return m, nil

		case "f5":
			m.helpOverlay.SetSize(m.width, m.height)
			m.helpOverlay.Toggle()
			return m, nil

		case "tab":
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

		case "ctrl+e":
			m.viewMode = !m.viewMode
			if m.viewMode {
				m.statusbar.SetMode("VIEW")
				m.viewScroll = 0
			} else {
				m.statusbar.SetMode("EDIT")
			}
			return m, nil

		case "ctrl+,":
			m.settings.SetSize(m.width, m.height)
			m.settings.Toggle()
			return m, nil

		case "ctrl+g":
			m.graphView.SetSize(m.width, m.height)
			m.graphView.Open(m.activeNote)
			return m, nil

		case "ctrl+t":
			m.tagBrowser.SetSize(m.width, m.height)
			m.tagBrowser.Open()
			return m, nil

		case "ctrl+x":
			m.commandPalette.SetSize(m.width, m.height)
			m.commandPalette.Open()
			return m, nil

		case "esc":
			if m.viewMode {
				m.viewMode = false
				m.statusbar.SetMode("EDIT")
				return m, nil
			}
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

		// View mode scrolling
		if m.viewMode && m.focus == focusEditor {
			switch msg.String() {
			case "up", "k":
				if m.viewScroll > 0 {
					m.viewScroll--
				}
				return m, nil
			case "down", "j":
				m.viewScroll++
				return m, nil
			case "pgup":
				m.viewScroll -= m.height / 2
				if m.viewScroll < 0 {
					m.viewScroll = 0
				}
				return m, nil
			case "pgdown":
				m.viewScroll += m.height / 2
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
		if !m.viewMode {
			m.editor, cmd = m.editor.Update(msg)
			line, col := m.editor.GetCursor()
			m.statusbar.SetCursor(line, col)
			m.statusbar.SetWordCount(m.editor.GetWordCount())
		}
	case focusBacklinks:
		m.backlinks, cmd = m.backlinks.Update(msg)
	}

	return m, cmd
}

func (m *Model) executeCommand(action CommandAction) (tea.Model, tea.Cmd) {
	switch action {
	case CmdOpenFile:
		m.searchMode = true
		m.searchQuery = ""
		m.searchResults = m.vault.SortedPaths()
		m.searchCursor = 0
	case CmdNewNote:
		m.newNoteMode = true
		m.newNoteName = ""
	case CmdSaveNote:
		cmd := m.saveCurrentNote()
		m.statusbar.SetMessage("Saved " + m.activeNote)
		return m, tea.Batch(cmd, m.clearMessageAfter(2*time.Second))
	case CmdDailyNote:
		today := time.Now().Format("2006-01-02")
		name := today + ".md"
		folder := m.config.DailyNotesFolder
		if folder != "" {
			name = filepath.Join(folder, name)
		}
		path := filepath.Join(m.vault.Root, name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			os.MkdirAll(filepath.Dir(path), 0755)
			content := fmt.Sprintf("---\ndate: %s\ntype: daily\ntags: [daily]\n---\n\n# %s\n\n## Tasks\n- [ ] \n\n## Notes\n\n", today, today)
			os.WriteFile(path, []byte(content), 0644)
			m.vault.Scan()
			m.index = vault.NewIndex(m.vault)
			m.index.Build()
			m.sidebar.SetFiles(m.vault.SortedPaths())
			m.statusbar.SetNoteCount(m.vault.NoteCount())
			m.statusbar.SetMessage("Created daily note: " + name)
		}
		m.loadNote(name)
		m.sidebar.cursor = m.findFileIndex(name)
		m.setFocus(focusEditor)
	case CmdToggleView:
		m.viewMode = !m.viewMode
		if m.viewMode {
			m.statusbar.SetMode("VIEW")
			m.viewScroll = 0
		} else {
			m.statusbar.SetMode("EDIT")
		}
	case CmdSettings:
		m.settings.SetSize(m.width, m.height)
		m.settings.Toggle()
	case CmdFocusEditor:
		m.setFocus(focusEditor)
	case CmdFocusSidebar:
		m.setFocus(focusSidebar)
	case CmdFocusBacklinks:
		m.setFocus(focusBacklinks)
	case CmdRefreshVault:
		m.vault.Scan()
		m.index = vault.NewIndex(m.vault)
		m.index.Build()
		m.sidebar.SetFiles(m.vault.SortedPaths())
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.statusbar.SetMessage("Vault refreshed")
		return m, m.clearMessageAfter(2 * time.Second)
	case CmdDeleteNote:
		if m.activeNote != "" {
			path := filepath.Join(m.vault.Root, m.activeNote)
			os.Remove(path)
			m.vault.Scan()
			m.index = vault.NewIndex(m.vault)
			m.index.Build()
			paths := m.vault.SortedPaths()
			m.sidebar.SetFiles(paths)
			m.statusbar.SetNoteCount(m.vault.NoteCount())
			m.statusbar.SetMessage("Deleted " + m.activeNote)
			if len(paths) > 0 {
				m.loadNote(paths[0])
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdRenameNote:
		if m.activeNote != "" {
			m.newNoteMode = true
			m.newNoteName = strings.TrimSuffix(m.activeNote, ".md")
		}
	case CmdShowGraph:
		m.graphView.SetSize(m.width, m.height)
		m.graphView.Open(m.activeNote)
	case CmdShowTags:
		m.tagBrowser.SetSize(m.width, m.height)
		m.tagBrowser.Open()
	case CmdShowHelp:
		m.helpOverlay.SetSize(m.width, m.height)
		m.helpOverlay.Toggle()
	case CmdQuit:
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
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
	case "up":
		if m.searchCursor > 0 {
			m.searchCursor--
		}
		return m, nil
	case "down":
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
	for _, path := range m.vault.SortedPaths() {
		note := m.vault.GetNote(path)
		if note != nil && strings.Contains(strings.ToLower(note.Content), query) {
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
		if m.viewMode {
			m.statusbar.SetMode("VIEW")
		} else {
			m.statusbar.SetMode("EDIT")
		}
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

	contentHeight := m.height - 3

	m.sidebar.SetSize(sidebarWidth, contentHeight)
	m.editor.SetSize(editorWidth, contentHeight)
	m.renderer.SetSize(editorWidth, contentHeight)
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
	// Splash screen
	if m.showSplash {
		return m.splash.View()
	}

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

	// Focus-aware borders
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

	// Editor: view mode or edit mode
	var editorContent string
	if m.viewMode {
		editorContent = m.renderViewMode()
	} else {
		editorContent = m.editor.View()
	}

	editor := EditorStyle.Copy().
		BorderForeground(editorBorderColor).
		Width(editorWidth).
		Height(contentHeight).
		Render(editorContent)

	backlinks := BacklinksStyle.Copy().
		BorderForeground(backlinksBorderColor).
		Width(backlinksWidth).
		Height(contentHeight).
		Render(m.backlinks.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor, backlinks)
	status := m.statusbar.View()

	view := lipgloss.JoinVertical(lipgloss.Left, content, status)

	// Render overlays (in priority order)
	if m.helpOverlay.IsActive() {
		overlay := m.helpOverlay.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.settings.IsActive() {
		overlay := m.settings.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.graphView.IsActive() {
		overlay := m.graphView.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.tagBrowser.IsActive() {
		overlay := m.tagBrowser.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.commandPalette.IsActive() {
		overlay := m.commandPalette.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.searchMode {
		overlay := m.renderSearchOverlay()
		view = m.overlayCenter(view, overlay)
	}
	if m.newNoteMode {
		overlay := m.renderNewNoteOverlay()
		view = m.overlayCenter(view, overlay)
	}

	return view
}

func (m Model) renderViewMode() string {
	var b strings.Builder
	contentWidth := m.editor.width - 4
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Header
	modeIndicator := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#11111B")).
		Background(green).
		Bold(true).
		Padding(0, 1).
		Render("VIEW")
	headerText := modeIndicator + "  " + HeaderStyle.Render(m.editor.filePath)
	b.WriteString(headerText)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", contentWidth)))
	b.WriteString("\n")

	// Render markdown content
	rendered := m.renderer.Render(m.editor.GetContent(), m.viewScroll)
	b.WriteString(rendered)

	return b.String()
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

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Quick Open")
	b.WriteString(title)
	b.WriteString("\n\n")

	prompt := SearchPromptStyle.Render(" > ")
	input := m.searchQuery + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-4)))
	b.WriteString("\n")

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

		for len(bgRunes) < startX+lipgloss.Width(overlayLine) {
			bgRunes = append(bgRunes, ' ')
		}

		newLine := string(bgRunes[:startX]) + overlayLine
		if startX+lipgloss.Width(overlayLine) < len(bgRunes) {
			newLine += string(bgRunes[startX+lipgloss.Width(overlayLine):])
		}
		result[y] = newLine
	}

	return strings.Join(result, "\n")
}
