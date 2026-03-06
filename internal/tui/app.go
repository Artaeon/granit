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

	newNoteMode      bool
	newNoteName      string
	pendingTemplate  string

	commandPalette CommandPalette
	settings       Settings
	graphView      GraphView
	tagBrowser     TagBrowser
	helpOverlay    HelpOverlay
	outline        Outline
	bookmarks      Bookmarks
	findReplace    FindReplace
	vaultStats     VaultStats
	templates      Templates
	focusMode      FocusMode
	quickSwitch    QuickSwitch
	autocomplete   Autocomplete
	trash          Trash

	// View mode scroll
	viewScroll int

	// Confirm delete
	confirmDelete     bool
	confirmDeleteNote string
}

func NewModel(vaultPath string) (Model, error) {
	cfg := config.LoadForVault(vaultPath)
	ApplyTheme(cfg.Theme)
	ApplyIconTheme(cfg.IconTheme)

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
		outline:        NewOutline(),
		bookmarks:      NewBookmarks(vaultPath),
		findReplace:    NewFindReplace(),
		vaultStats:     NewVaultStats(v, idx),
		templates:      NewTemplates(),
		focusMode:      NewFocusMode(),
		quickSwitch:    NewQuickSwitch(),
		autocomplete:   NewAutocomplete(),
		trash:          NewTrash(vaultPath),
		showSplash:     cfg.ShowSplash,
		splash:         NewSplashModel(vaultPath, v.NoteCount()),
		viewMode:       cfg.DefaultViewMode,
	}

	m.statusbar.SetVaultPath(vaultPath)
	m.statusbar.SetNoteCount(v.NoteCount())
	m.autocomplete.SetNotes(paths)

	// Apply config to components
	m.syncConfigToComponents()

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

	m.bookmarks.AddRecent(relPath)
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
				m.syncConfigToComponents()
			}
			return m, nil
		}

		if m.vaultStats.IsActive() {
			m.vaultStats, _ = m.vaultStats.Update(msg)
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

		if m.outline.IsActive() {
			m.outline, _ = m.outline.Update(msg)
			if jumpLine := m.outline.JumpToLine(); jumpLine >= 0 {
				m.editor.cursor = jumpLine
				m.editor.col = 0
				if jumpLine < m.editor.scroll || jumpLine >= m.editor.scroll+m.editor.height-4 {
					m.editor.scroll = jumpLine
				}
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.bookmarks.IsActive() {
			m.bookmarks, _ = m.bookmarks.Update(msg)
			if nav := m.bookmarks.SelectedNote(); nav != "" {
				m.loadNote(nav)
				m.sidebar.cursor = m.findFileIndex(nav)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.findReplace.IsActive() {
			m.findReplace, _ = m.findReplace.Update(msg)
			m.findReplace.UpdateMatches(m.editor.content)
			if jumpLine := m.findReplace.GetJumpLine(); jumpLine >= 0 {
				m.editor.cursor = jumpLine
				m.editor.col = 0
				if jumpLine < m.editor.scroll || jumpLine >= m.editor.scroll+m.editor.height-4 {
					m.editor.scroll = jumpLine
				}
			}
			if m.findReplace.ShouldReplace() {
				m.doReplace()
			}
			if m.findReplace.ShouldReplaceAll() {
				m.doReplaceAll()
			}
			return m, nil
		}

		if m.templates.IsActive() {
			m.templates, _ = m.templates.Update(msg)
			if tmpl := m.templates.SelectedTemplate(); tmpl != "" {
				m.newNoteMode = true
				m.newNoteName = ""
				// Store template content for use when note name is confirmed
				m.pendingTemplate = tmpl
			}
			return m, nil
		}

		if m.quickSwitch.IsActive() {
			m.quickSwitch, _ = m.quickSwitch.Update(msg)
			if nav := m.quickSwitch.SelectedFile(); nav != "" {
				m.loadNote(nav)
				m.sidebar.cursor = m.findFileIndex(nav)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.trash.IsActive() {
			m.trash, _ = m.trash.Update(msg)
			if m.trash.ShouldRestore() {
				restored := m.trash.RestoreFile()
				if restored != "" {
					m.vault.Scan()
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					paths := m.vault.SortedPaths()
					m.sidebar.SetFiles(paths)
					m.autocomplete.SetNotes(paths)
					m.statusbar.SetNoteCount(m.vault.NoteCount())
					m.loadNote(restored)
					m.sidebar.cursor = m.findFileIndex(restored)
					m.statusbar.SetMessage("Restored: " + restored)
				}
			}
			return m, nil
		}

		if m.confirmDelete {
			switch msg.String() {
			case "y", "Y", "enter":
				m.confirmDelete = false
				if m.confirmDeleteNote != "" {
					if err := m.trash.MoveToTrash(m.confirmDeleteNote); err == nil {
						m.vault.Scan()
						m.index = vault.NewIndex(m.vault)
						m.index.Build()
						paths := m.vault.SortedPaths()
						m.sidebar.SetFiles(paths)
						m.autocomplete.SetNotes(paths)
						m.statusbar.SetNoteCount(m.vault.NoteCount())
						m.statusbar.SetMessage("Moved to trash: " + m.confirmDeleteNote)
						if len(paths) > 0 {
							m.loadNote(paths[0])
						}
					}
					m.confirmDeleteNote = ""
				}
				return m, m.clearMessageAfter(2 * time.Second)
			case "n", "N", "esc":
				m.confirmDelete = false
				m.confirmDeleteNote = ""
				return m, nil
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

		case "ctrl+o":
			m.outline.SetSize(m.width, m.height)
			m.outline.Open(m.editor.GetContent())
			return m, nil

		case "ctrl+b":
			m.bookmarks.SetSize(m.width, m.height)
			m.bookmarks.Open()
			return m, nil

		case "ctrl+f":
			m.findReplace.SetSize(m.width, m.height)
			m.findReplace.OpenFind()
			m.findReplace.UpdateMatches(m.editor.content)
			return m, nil

		case "ctrl+h":
			m.findReplace.SetSize(m.width, m.height)
			m.findReplace.OpenReplace()
			m.findReplace.UpdateMatches(m.editor.content)
			return m, nil

		case "ctrl+j":
			m.quickSwitch.SetSize(m.width, m.height)
			m.quickSwitch.Open(
				m.bookmarks.data.Recent,
				m.bookmarks.data.Starred,
				m.vault.SortedPaths(),
				func(path string) time.Time {
					if note := m.vault.GetNote(path); note != nil {
						return note.ModTime
					}
					return time.Time{}
				},
			)
			return m, nil

		case "ctrl+z":
			if m.focusMode.IsActive() {
				m.focusMode.Close()
			} else {
				m.focusMode.SetSize(m.width, m.height)
				m.focusMode.Open(m.editor.GetWordCount())
				m.setFocus(focusEditor)
			}
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
		paths := m.vault.SortedPaths()
		m.sidebar.SetFiles(paths)
		m.autocomplete.SetNotes(paths)
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.statusbar.SetMessage("Vault refreshed")
		return m, m.clearMessageAfter(2 * time.Second)
	case CmdDeleteNote:
		if m.activeNote != "" {
			if m.config.ConfirmDelete {
				m.confirmDelete = true
				m.confirmDeleteNote = m.activeNote
			} else {
				if err := m.trash.MoveToTrash(m.activeNote); err == nil {
					m.vault.Scan()
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					paths := m.vault.SortedPaths()
					m.sidebar.SetFiles(paths)
					m.autocomplete.SetNotes(paths)
					m.statusbar.SetNoteCount(m.vault.NoteCount())
					m.statusbar.SetMessage("Moved to trash: " + m.activeNote)
					if len(paths) > 0 {
						m.loadNote(paths[0])
					}
				}
				return m, m.clearMessageAfter(2 * time.Second)
			}
		}
	case CmdShowTrash:
		m.trash.SetSize(m.width, m.height)
		m.trash.Open()
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
	case CmdShowOutline:
		m.outline.SetSize(m.width, m.height)
		m.outline.Open(m.editor.GetContent())
	case CmdShowBookmarks:
		m.bookmarks.SetSize(m.width, m.height)
		m.bookmarks.Open()
	case CmdToggleBookmark:
		if m.activeNote != "" {
			m.bookmarks.ToggleStar(m.activeNote)
			if m.bookmarks.IsStarred(m.activeNote) {
				m.statusbar.SetMessage("Starred " + m.activeNote)
			} else {
				m.statusbar.SetMessage("Unstarred " + m.activeNote)
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdFindInFile:
		m.findReplace.SetSize(m.width, m.height)
		m.findReplace.OpenFind()
		m.findReplace.UpdateMatches(m.editor.content)
	case CmdReplaceInFile:
		m.findReplace.SetSize(m.width, m.height)
		m.findReplace.OpenReplace()
		m.findReplace.UpdateMatches(m.editor.content)
	case CmdShowStats:
		m.vaultStats.SetSize(m.width, m.height)
		m.vaultStats.Open()
	case CmdNewFromTemplate:
		m.templates.SetSize(m.width, m.height)
		m.templates.Open()
	case CmdFocusMode:
		if m.focusMode.IsActive() {
			m.focusMode.Close()
		} else {
			m.focusMode.SetSize(m.width, m.height)
			m.focusMode.Open(m.editor.GetWordCount())
			m.setFocus(focusEditor)
		}
	case CmdQuickSwitch:
		m.quickSwitch.SetSize(m.width, m.height)
		m.quickSwitch.Open(
			m.bookmarks.data.Recent,
			m.bookmarks.data.Starred,
			m.vault.SortedPaths(),
			func(path string) time.Time {
				if note := m.vault.GetNote(path); note != nil {
					return note.ModTime
				}
				return time.Time{}
			},
		)
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
		m.pendingTemplate = ""
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
			if m.pendingTemplate != "" {
				content = strings.ReplaceAll(m.pendingTemplate, "{{title}}", title)
				m.pendingTemplate = ""
			}

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
	layout := m.config.Layout
	if layout == "" {
		layout = "default"
	}

	showSidebar := layout == "default" || layout == "writer"
	showBacklinks := layout == "default"

	sidebarWidth := 0
	backlinksWidth := 0
	if showSidebar {
		sidebarWidth = m.width / 5
		if sidebarWidth < 22 {
			sidebarWidth = 22
		}
		if sidebarWidth > 35 {
			sidebarWidth = 35
		}
	}
	if showBacklinks {
		backlinksWidth = m.width / 5
		if backlinksWidth < 22 {
			backlinksWidth = 22
		}
		if backlinksWidth > 30 {
			backlinksWidth = 30
		}
	}

	panelBorders := 0
	if showSidebar {
		panelBorders += 2
	}
	if showBacklinks {
		panelBorders += 2
	}
	editorWidth := m.width - sidebarWidth - backlinksWidth - panelBorders - 2
	if editorWidth < 30 {
		editorWidth = 30
	}

	contentHeight := m.height - 3

	m.sidebar.SetSize(sidebarWidth, contentHeight)
	m.editor.SetSize(editorWidth, contentHeight)
	m.renderer.SetSize(editorWidth, contentHeight)
	m.backlinks.SetSize(backlinksWidth, contentHeight)
	m.statusbar.SetWidth(m.width)
}

func (m *Model) syncConfigToComponents() {
	m.sidebar.showIcons = m.config.ShowIcons
	m.sidebar.compactMode = m.config.CompactMode
	m.editor.showLineNumbers = m.config.LineNumbers
	m.editor.highlightCurrentLine = m.config.HighlightCurrentLine
	m.editor.autoCloseBrackets = m.config.AutoCloseBrackets
	m.editor.tabSize = m.config.Editor.TabSize
	if m.editor.tabSize < 1 {
		m.editor.tabSize = 4
	}
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

	contentHeight := m.height - 3
	layout := m.config.Layout
	if layout == "" {
		layout = "default"
	}

	// Calculate widths based on layout
	showSidebar := layout == "default" || layout == "writer"
	showBacklinks := layout == "default"

	sidebarWidth := 0
	backlinksWidth := 0

	if showSidebar {
		sidebarWidth = m.width / 5
		if sidebarWidth < 22 {
			sidebarWidth = 22
		}
		if sidebarWidth > 35 {
			sidebarWidth = 35
		}
	}
	if showBacklinks {
		backlinksWidth = m.width / 5
		if backlinksWidth < 22 {
			backlinksWidth = 22
		}
		if backlinksWidth > 30 {
			backlinksWidth = 30
		}
	}

	panelBorders := 0
	if showSidebar {
		panelBorders += 2
	}
	if showBacklinks {
		panelBorders += 2
	}
	editorWidth := m.width - sidebarWidth - backlinksWidth - panelBorders - 2
	if editorWidth < 30 {
		editorWidth = 30
	}

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

	var view string
	if m.focusMode.IsActive() {
		focusView := m.focusMode.RenderEditor(editorContent, m.editor.GetWordCount())
		view = focusView
	} else {
		var content string
		switch layout {
		case "minimal":
			content = editor
		case "writer":
			sidebar := SidebarStyle.Copy().
				BorderForeground(sidebarBorderColor).
				Width(sidebarWidth).
				Height(contentHeight).
				Render(m.sidebar.View())
			if m.config.SidebarPosition == "right" {
				content = lipgloss.JoinHorizontal(lipgloss.Top, editor, sidebar)
			} else {
				content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor)
			}
		default: // "default" - 3-panel
			sidebar := SidebarStyle.Copy().
				BorderForeground(sidebarBorderColor).
				Width(sidebarWidth).
				Height(contentHeight).
				Render(m.sidebar.View())
			backlinks := BacklinksStyle.Copy().
				BorderForeground(backlinksBorderColor).
				Width(backlinksWidth).
				Height(contentHeight).
				Render(m.backlinks.View())
			if m.config.SidebarPosition == "right" {
				content = lipgloss.JoinHorizontal(lipgloss.Top, backlinks, editor, sidebar)
			} else {
				content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor, backlinks)
			}
		}
		status := m.statusbar.View()
		view = lipgloss.JoinVertical(lipgloss.Left, content, status)
	}

	// Render overlays (in priority order)
	if m.helpOverlay.IsActive() {
		overlay := m.helpOverlay.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.settings.IsActive() {
		overlay := m.settings.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.vaultStats.IsActive() {
		overlay := m.vaultStats.View()
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
	if m.outline.IsActive() {
		overlay := m.outline.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.bookmarks.IsActive() {
		overlay := m.bookmarks.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.findReplace.IsActive() {
		overlay := m.findReplace.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.templates.IsActive() {
		overlay := m.templates.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.quickSwitch.IsActive() {
		overlay := m.quickSwitch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.trash.IsActive() {
		overlay := m.trash.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.confirmDelete {
		overlay := m.renderConfirmDeleteOverlay()
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
		Foreground(crust).
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

func (m Model) renderConfirmDeleteOverlay() string {
	width := 50

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(red).
		Bold(true).
		Render("  Delete Note")
	b.WriteString(title)
	b.WriteString("\n\n")

	b.WriteString("  Move to trash:\n")
	b.WriteString("  " + lipgloss.NewStyle().Foreground(peach).Bold(true).Render(m.confirmDeleteNote))
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  y/Enter: confirm  n/Esc: cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(red).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m *Model) doReplace() {
	query := m.findReplace.GetFindQuery()
	replacement := m.findReplace.GetReplaceText()
	if query == "" {
		return
	}
	// Replace first occurrence from current position
	for i := m.editor.cursor; i < len(m.editor.content); i++ {
		lower := strings.ToLower(m.editor.content[i])
		idx := strings.Index(lower, strings.ToLower(query))
		if idx >= 0 {
			line := m.editor.content[i]
			m.editor.content[i] = line[:idx] + replacement + line[idx+len(query):]
			m.editor.modified = true
			m.editor.countWords()
			m.findReplace.UpdateMatches(m.editor.content)
			return
		}
	}
}

func (m *Model) doReplaceAll() {
	query := m.findReplace.GetFindQuery()
	replacement := m.findReplace.GetReplaceText()
	if query == "" {
		return
	}
	count := 0
	for i := range m.editor.content {
		lower := strings.ToLower(m.editor.content[i])
		lowerQuery := strings.ToLower(query)
		for strings.Contains(lower, lowerQuery) {
			idx := strings.Index(lower, lowerQuery)
			line := m.editor.content[i]
			m.editor.content[i] = line[:idx] + replacement + line[idx+len(query):]
			lower = strings.ToLower(m.editor.content[i])
			count++
		}
	}
	if count > 0 {
		m.editor.modified = true
		m.editor.countWords()
		m.findReplace.UpdateMatches(m.editor.content)
		m.statusbar.SetMessage(fmt.Sprintf("Replaced %d occurrences", count))
	}
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
