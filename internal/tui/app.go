package tui

import (
	"os"
	"path/filepath"
	"strings"

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

type Model struct {
	vault     *vault.Vault
	index     *vault.Index
	sidebar   Sidebar
	editor    Editor
	backlinks Backlinks
	statusbar StatusBar

	focus       focus
	width       int
	height      int
	activeNote  string
	quitting    bool
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

	// Load first note if available
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

	// Update backlinks
	incoming := m.index.GetBacklinks(relPath)
	outgoing := m.index.GetOutgoingLinks(relPath)
	m.backlinks.SetLinks(incoming, outgoing)
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updateLayout()
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+q":
			m.quitting = true
			return m, tea.Quit
		case "ctrl+s":
			return m, m.saveCurrentNote()
		case "ctrl+1":
			m.setFocus(focusSidebar)
			return m, nil
		case "ctrl+2":
			m.setFocus(focusEditor)
			return m, nil
		case "ctrl+3":
			m.setFocus(focusBacklinks)
			return m, nil
		case "enter":
			if m.focus == focusSidebar {
				selected := m.sidebar.Selected()
				if selected != "" {
					m.loadNote(selected)
				}
				return m, nil
			}
			if m.focus == focusBacklinks {
				selected := m.backlinks.Selected()
				if selected != "" {
					// Resolve the link to a path
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
	case focusBacklinks:
		m.backlinks, cmd = m.backlinks.Update(msg)
	}

	return m, cmd
}

func (m *Model) resolveLink(link string) string {
	// Try direct path
	if m.vault.GetNote(link) != nil {
		return link
	}
	// Try with .md
	if !strings.HasSuffix(link, ".md") {
		withMd := link + ".md"
		if m.vault.GetNote(withMd) != nil {
			return withMd
		}
	}
	// Search by basename
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

func (m *Model) updateLayout() {
	sidebarWidth := m.width / 5
	if sidebarWidth < 20 {
		sidebarWidth = 20
	}
	backlinksWidth := m.width / 5
	if backlinksWidth < 20 {
		backlinksWidth = 20
	}
	editorWidth := m.width - sidebarWidth - backlinksWidth - 6 // borders

	contentHeight := m.height - 2 // status bar

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

func (m Model) View() string {
	if m.quitting {
		return "Goodbye from Granit!\n"
	}

	if m.width == 0 {
		return "Loading..."
	}

	sidebarWidth := m.width / 5
	if sidebarWidth < 20 {
		sidebarWidth = 20
	}
	backlinksWidth := m.width / 5
	if backlinksWidth < 20 {
		backlinksWidth = 20
	}
	editorWidth := m.width - sidebarWidth - backlinksWidth - 6

	contentHeight := m.height - 2

	sidebar := SidebarStyle.
		Width(sidebarWidth).
		Height(contentHeight).
		Render(m.sidebar.View())

	editor := EditorStyle.
		Width(editorWidth).
		Height(contentHeight).
		Render(m.editor.View())

	backlinks := BacklinksStyle.
		Width(backlinksWidth).
		Height(contentHeight).
		Render(m.backlinks.View())

	content := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor, backlinks)
	status := m.statusbar.View()

	return lipgloss.JoinVertical(lipgloss.Left, content, status)
}
