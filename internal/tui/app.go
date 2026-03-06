package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/vault"
)

// stopOllama unloads running models to free memory when Granit exits.
func stopOllama(model string) {
	if model != "" {
		exec.Command("ollama", "stop", model).Run()
	}
}

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
	canvas         Canvas
	calendar       Calendar
	bots           Bots
	export         ExportOverlay
	git            GitOverlay
	plugins        PluginManager
	contentSearch  ContentSearch
	spellcheck     SpellChecker
	snippets       *SnippetEngine
	autoSync       AutoSync
	publisher      Publisher
	splitPane      SplitPane
	luaEngine      *LuaEngine
	luaOverlay     LuaOverlay
	flashcards     Flashcards
	quizMode       QuizMode
	learnDash      LearnDashboard
	aiChat         AIChat
	composer       Composer
	knowledgeGraph KnowledgeGraph
	autoLinker     *AutoLinker
	tfidfIndex     *TFIDFIndex
	tableEditor    TableEditor
	semanticSearch SemanticSearch
	ghostWriter    *GhostWriter
	threadWeaver   ThreadWeaver
	noteChat       NoteChat
	autoTagger     *AutoTagger

	// View mode scroll
	viewScroll int

	// Confirm delete
	confirmDelete     bool
	confirmDeleteNote string

	// Folder management
	newFolderMode bool
	newFolderName string
	moveFileMode  bool
	moveFileDirs  []string
	moveFileCursor int
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
		canvas:         NewCanvas(),
		calendar:       NewCalendar(),
		bots:           NewBots(),
		export:         NewExportOverlay(),
		git:            NewGitOverlay(),
		plugins:        NewPluginManager(),
		contentSearch:  NewContentSearch(),
		spellcheck:     NewSpellChecker(),
		snippets:       NewSnippetEngine(),
		autoSync:       NewAutoSync(vaultPath),
		publisher:      NewPublisher(),
		splitPane:      NewSplitPane(),
		luaEngine:      NewLuaEngine(vaultPath),
		luaOverlay:     NewLuaOverlay(),
		flashcards:     NewFlashcards(vaultPath),
		quizMode:       NewQuizMode(),
		learnDash:      NewLearnDashboard(vaultPath),
		aiChat:         NewAIChat(),
		composer:       NewComposer(),
		knowledgeGraph: NewKnowledgeGraph(),
		autoLinker:     NewAutoLinker(),
		tableEditor:    NewTableEditor(),
		semanticSearch: NewSemanticSearch(),
		ghostWriter:    NewGhostWriter(),
		threadWeaver:   NewThreadWeaver(),
		noteChat:       NewNoteChat(),
		autoTagger:     NewAutoTagger(),
		showSplash:     cfg.ShowSplash,
		splash:         NewSplashModel(vaultPath, v.NoteCount()),
		viewMode:       cfg.DefaultViewMode,
	}

	m.statusbar.SetVaultPath(vaultPath)
	m.statusbar.SetNoteCount(v.NoteCount())
	m.autocomplete.SetNotes(paths)
	m.plugins.SetVaultPath(vaultPath)
	m.canvas.SetVaultPath(vaultPath)
	m.publisher.SetVaultPath(vaultPath)
	m.luaOverlay.SetEngine(m.luaEngine)
	m.renderer.SetVaultNotes(m.vault.Notes)

	// Set up renderer note lookup for transclusion
	m.renderer.SetNoteLookup(func(name string) string {
		// Try exact path
		if note := m.vault.GetNote(name); note != nil {
			return note.Content
		}
		// Try with .md extension
		if note := m.vault.GetNote(name + ".md"); note != nil {
			return note.Content
		}
		// Try basename match
		for _, p := range m.vault.SortedPaths() {
			base := strings.TrimSuffix(filepath.Base(p), ".md")
			if base == name {
				if note := m.vault.GetNote(p); note != nil {
					return note.Content
				}
			}
		}
		return ""
	})

	// Set calendar daily notes
	m.calendar.SetDailyNotes(paths)

	// Semantic search setup
	m.semanticSearch.SetVaultPath(vaultPath)

	// Ghost writer setup
	m.ghostWriter.SetEnabled(cfg.GhostWriter)

	// Apply config to components
	m.syncConfigToComponents()

	// Configure auto git sync
	m.autoSync.SetEnabled(cfg.GitAutoSync)

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

	incoming := m.buildBacklinkItems(m.index.GetBacklinks(relPath), relPath)
	outgoing := m.buildOutgoingItems(m.index.GetOutgoingLinks(relPath))
	m.backlinks.SetLinks(incoming, outgoing)

	m.bookmarks.AddRecent(relPath)
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.showSplash {
		cmds = append(cmds, m.splash.Init())
	}
	// Auto git sync: pull on open
	if pullCmd := m.autoSync.PullOnOpen(); pullCmd != nil {
		cmds = append(cmds, pullCmd)
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
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

	case gitCmdResultMsg:
		if m.git.IsActive() {
			var cmd tea.Cmd
			m.git, cmd = m.git.Update(msg)
			return m, cmd
		}
		return m, nil

	case autoSyncResultMsg:
		if msg.err != nil {
			m.statusbar.SetMessage("Git sync error: " + msg.err.Error())
		} else if msg.action == "pull" && msg.output != "" {
			trimmed := strings.TrimSpace(msg.output)
			if trimmed != "" && trimmed != "Already up to date." {
				// Rescan vault after pull brought changes
				m.vault.Scan()
				m.index.Build()
				m.sidebar.SetFiles(m.vault.SortedPaths())
				m.statusbar.SetMessage("Git: pulled latest changes")
			}
		} else if msg.output == "synced" {
			m.statusbar.SetMessage("Git: auto-synced")
		}
		if msg.action != "" {
			return m, m.clearMessageAfter(3 * time.Second)
		}
		return m, nil

	case luaRunResultMsg:
		if m.luaOverlay.IsActive() {
			r := msg.result
			// Apply content/insert if provided
			if r.Content != "" {
				m.editor.SetContent(r.Content)
			}
			if r.Insert != "" {
				m.editor.InsertText(r.Insert)
			}
			var cmd tea.Cmd
			m.luaOverlay, cmd = m.luaOverlay.Update(msg)
			return m, cmd
		}
		return m, nil

	case publishResultMsg, publishProgressMsg:
		if m.publisher.IsActive() {
			var cmd tea.Cmd
			m.publisher, cmd = m.publisher.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiChatResultMsg, aiChatTickMsg:
		if m.aiChat.IsActive() {
			var cmd tea.Cmd
			m.aiChat, cmd = m.aiChat.Update(msg)
			return m, cmd
		}
		return m, nil

	case composerResultMsg, composerTickMsg:
		if m.composer.IsActive() {
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			// Check if user accepted a generated note
			if !m.composer.IsActive() {
				if title, content, ok := m.composer.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := os.WriteFile(path, []byte(content), 0644); err == nil {
							m.vault.Scan()
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.sidebar.cursor = m.findFileIndex(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("AI note created: " + name)
						}
					}
					return m, m.clearMessageAfter(3 * time.Second)
				}
			}
			return m, cmd
		}
		return m, nil

	case semanticSearchMsg, semanticBuildMsg, semanticTickMsg:
		if m.semanticSearch.IsActive() {
			var cmd tea.Cmd
			m.semanticSearch, cmd = m.semanticSearch.Update(msg)
			return m, cmd
		}
		return m, nil

	case threadWeaverResultMsg, threadWeaverTickMsg:
		if m.threadWeaver.IsActive() {
			var cmd tea.Cmd
			m.threadWeaver, cmd = m.threadWeaver.Update(msg)
			if !m.threadWeaver.IsActive() {
				if title, content, ok := m.threadWeaver.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := os.WriteFile(path, []byte(content), 0644); err == nil {
							m.vault.Scan()
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.sidebar.cursor = m.findFileIndex(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("Thread woven: " + name)
						}
					}
					return m, m.clearMessageAfter(3 * time.Second)
				}
			}
			return m, cmd
		}
		return m, nil

	case noteChatResultMsg, noteChatTickMsg:
		if m.noteChat.IsActive() {
			var cmd tea.Cmd
			m.noteChat, cmd = m.noteChat.Update(msg)
			return m, cmd
		}
		return m, nil

	case autoTagResultMsg:
		if msg.err == nil && len(msg.tags) > 0 {
			m.applyTagsToNote(msg.tags)
			m.statusbar.SetMessage("Auto-tagged: " + strings.Join(msg.tags, ", "))
			return m, m.clearMessageAfter(3 * time.Second)
		}
		return m, nil

	case ghostSuggestionMsg, ghostDebounceMsg:
		if m.ghostWriter != nil {
			cmd := m.ghostWriter.HandleMsg(msg)
			return m, cmd
		}
		return m, nil

	case botsTickMsg:
		if m.bots.IsActive() {
			var cmd tea.Cmd
			m.bots, cmd = m.bots.Update(msg)
			return m, cmd
		}
		return m, nil

	case ollamaResultMsg:
		if m.bots.IsActive() {
			var cmd tea.Cmd
			m.bots, cmd = m.bots.Update(msg)
			return m, cmd
		}
		return m, nil

	case ollamaSetupMsg:
		if m.settings.IsActive() {
			m.settings, _ = m.settings.Update(msg)
			if !m.settings.setupRunning && msg.success {
				m.config = m.settings.GetConfig()
				m.config.Save()
			}
		}
		return m, nil

	case openaiResultMsg:
		if m.bots.IsActive() {
			var cmd tea.Cmd
			m.bots, cmd = m.bots.Update(msg)
			return m, cmd
		}
		return m, nil

	case pluginCmdResultMsg:
		if msg.err != nil {
			m.statusbar.SetMessage("Plugin error: " + msg.err.Error())
		} else {
			m.handlePluginOutput(msg.pluginName, msg.output)
		}
		return m, m.clearMessageAfter(3 * time.Second)

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
			var settingsCmd tea.Cmd
			m.settings, settingsCmd = m.settings.Update(msg)
			if !m.settings.IsActive() {
				m.config = m.settings.GetConfig()
				m.config.Save()
				m.syncConfigToComponents()
			}
			return m, settingsCmd
		}

		if m.git.IsActive() {
			var gitCmd tea.Cmd
			m.git, gitCmd = m.git.Update(msg)
			return m, gitCmd
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
			if !m.templates.IsActive() && m.templates.WasSelected() {
				tmpl := m.templates.SelectedTemplate()
				m.newNoteMode = true
				m.newNoteName = ""
				m.pendingTemplate = tmpl // may be "" for blank note
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

		if m.canvas.IsActive() {
			m.canvas, _ = m.canvas.Update(msg)
			if nav := m.canvas.SelectedNote(); nav != "" {
				resolved := m.resolveLink(nav)
				if resolved != "" {
					m.loadNote(resolved)
					m.sidebar.cursor = m.findFileIndex(resolved)
					m.setFocus(focusEditor)
				}
			}
			return m, nil
		}

		if m.calendar.IsActive() {
			m.calendar, _ = m.calendar.Update(msg)
			if date := m.calendar.SelectedDate(); date != "" {
				// Open or create daily note for selected date
				name := date + ".md"
				folder := m.config.DailyNotesFolder
				if folder != "" {
					name = filepath.Join(folder, name)
				}
				path := filepath.Join(m.vault.Root, name)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					os.MkdirAll(filepath.Dir(path), 0755)
					content := fmt.Sprintf("---\ndate: %s\ntype: daily\ntags: [daily]\n---\n\n# %s\n\n", date, date)
					os.WriteFile(path, []byte(content), 0644)
					m.vault.Scan()
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					paths := m.vault.SortedPaths()
					m.sidebar.SetFiles(paths)
					m.autocomplete.SetNotes(paths)
					m.calendar.SetDailyNotes(paths)
					m.statusbar.SetNoteCount(m.vault.NoteCount())
				}
				m.loadNote(name)
				m.sidebar.cursor = m.findFileIndex(name)
				m.setFocus(focusEditor)
			}
			return m, nil
		}

		if m.bots.IsActive() {
			var botCmd tea.Cmd
			m.bots, botCmd = m.bots.Update(msg)
			// Only process result when user closed the overlay (Enter on results)
			if !m.bots.IsActive() {
				if result := m.bots.GetResult(); result.Action != "none" {
					switch result.Action {
					case "tag":
						if m.activeNote != "" && len(result.Tags) > 0 {
							m.applyTagsToNote(result.Tags)
							tagStr := strings.Join(result.Tags, ", ")
							m.statusbar.SetMessage("Applied tags: " + tagStr)
						}
					case "link":
						if len(result.Links) > 0 {
							m.statusbar.SetMessage("Found " + fmt.Sprintf("%d", len(result.Links)) + " related notes")
						}
					case "summary":
						if result.Summary != "" {
							m.statusbar.SetMessage("Summary generated")
						}
					}
					return m, m.clearMessageAfter(3 * time.Second)
				}
			}
			return m, botCmd
		}

		if m.export.IsActive() {
			m.export, _ = m.export.Update(msg)
			return m, nil
		}

		if m.plugins.IsActive() {
			var pluginCmd tea.Cmd
			m.plugins, pluginCmd = m.plugins.Update(msg)
			if !m.plugins.IsActive() {
				if pCmd := m.plugins.PendingCommand(); pCmd != nil {
					return m, RunPluginCommand(pCmd.plugin, pCmd.cmdDef, filepath.Join(m.vault.Root, m.activeNote), m.editor.GetContent(), m.vault.Root)
				}
			}
			return m, pluginCmd
		}

		if m.contentSearch.IsActive() {
			m.contentSearch, _ = m.contentSearch.Update(msg)
			if !m.contentSearch.IsActive() {
				if result := m.contentSearch.SelectedResult(); result != nil {
					m.loadNote(result.FilePath)
					m.sidebar.cursor = m.findFileIndex(result.FilePath)
					m.editor.cursor = result.Line
					m.editor.col = result.Col
					m.setFocus(focusEditor)
				}
			}
			return m, nil
		}

		if m.spellcheck.IsActive() {
			m.spellcheck, _ = m.spellcheck.Update(msg)
			if !m.spellcheck.IsActive() {
				if word, line, col, replacement, ok := m.spellcheck.GetCorrection(); ok {
					_ = word
					if line < len(m.editor.content) {
						lineStr := m.editor.content[line]
						if col+len(word) <= len(lineStr) {
							m.editor.content[line] = lineStr[:col] + replacement + lineStr[col+len(word):]
							m.editor.modified = true
							m.statusbar.SetMessage("Fixed: " + word + " → " + replacement)
						}
					}
				}
			}
			return m, nil
		}

		if m.luaOverlay.IsActive() {
			var cmd tea.Cmd
			m.luaOverlay, cmd = m.luaOverlay.Update(msg)
			return m, cmd
		}

		if m.publisher.IsActive() {
			var cmd tea.Cmd
			m.publisher, cmd = m.publisher.Update(msg)
			return m, cmd
		}

		if m.splitPane.IsActive() {
			var cmd tea.Cmd
			m.splitPane, cmd = m.splitPane.Update(msg)
			return m, cmd
		}

		if m.flashcards.IsActive() {
			var cmd tea.Cmd
			m.flashcards, cmd = m.flashcards.Update(msg)
			if !m.flashcards.IsActive() {
				// Record review activity
				m.learnDash.RecordReview(time.Now().Format("2006-01-02"))
			}
			return m, cmd
		}

		if m.quizMode.IsActive() {
			var cmd tea.Cmd
			m.quizMode, cmd = m.quizMode.Update(msg)
			if !m.quizMode.IsActive() {
				m.learnDash.RecordReview(time.Now().Format("2006-01-02"))
			}
			return m, cmd
		}

		if m.learnDash.IsActive() {
			var cmd tea.Cmd
			m.learnDash, cmd = m.learnDash.Update(msg)
			return m, cmd
		}

		if m.aiChat.IsActive() {
			var cmd tea.Cmd
			m.aiChat, cmd = m.aiChat.Update(msg)
			return m, cmd
		}

		if m.composer.IsActive() {
			var cmd tea.Cmd
			m.composer, cmd = m.composer.Update(msg)
			if !m.composer.IsActive() {
				if title, content, ok := m.composer.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := os.WriteFile(path, []byte(content), 0644); err == nil {
							m.vault.Scan()
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.sidebar.cursor = m.findFileIndex(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("AI note created: " + name)
							return m, m.clearMessageAfter(3 * time.Second)
						}
					}
				}
			}
			return m, cmd
		}

		if m.knowledgeGraph.IsActive() {
			var cmd tea.Cmd
			m.knowledgeGraph, cmd = m.knowledgeGraph.Update(msg)
			return m, cmd
		}

		if m.tableEditor.IsActive() {
			var cmd tea.Cmd
			m.tableEditor, cmd = m.tableEditor.Update(msg)
			if !m.tableEditor.IsActive() {
				if md, startLine, endLine, ok := m.tableEditor.GetResult(); ok {
					// Replace the table lines in the editor
					newLines := strings.Split(md, "\n")
					before := m.editor.content[:startLine]
					after := m.editor.content[endLine+1:]
					m.editor.content = append(before, append(newLines, after...)...)
					m.editor.modified = true
					m.statusbar.SetMessage("Table updated")
				}
			}
			return m, cmd
		}

		if m.semanticSearch.IsActive() {
			var cmd tea.Cmd
			m.semanticSearch, cmd = m.semanticSearch.Update(msg)
			if !m.semanticSearch.IsActive() {
				if nav := m.semanticSearch.SelectedResult(); nav != "" {
					m.loadNote(nav)
					m.sidebar.cursor = m.findFileIndex(nav)
					m.setFocus(focusEditor)
				}
			}
			return m, cmd
		}

		if m.threadWeaver.IsActive() {
			var cmd tea.Cmd
			m.threadWeaver, cmd = m.threadWeaver.Update(msg)
			if !m.threadWeaver.IsActive() {
				if title, content, ok := m.threadWeaver.GetResult(); ok {
					name := title
					if !strings.HasSuffix(name, ".md") {
						name += ".md"
					}
					path := filepath.Join(m.vault.Root, name)
					if err := os.MkdirAll(filepath.Dir(path), 0755); err == nil {
						if err := os.WriteFile(path, []byte(content), 0644); err == nil {
							m.vault.Scan()
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							paths := m.vault.SortedPaths()
							m.sidebar.SetFiles(paths)
							m.autocomplete.SetNotes(paths)
							m.statusbar.SetNoteCount(m.vault.NoteCount())
							m.loadNote(name)
							m.sidebar.cursor = m.findFileIndex(name)
							m.setFocus(focusEditor)
							m.statusbar.SetMessage("Thread woven: " + name)
							return m, m.clearMessageAfter(3 * time.Second)
						}
					}
				}
			}
			return m, cmd
		}

		if m.noteChat.IsActive() {
			var cmd tea.Cmd
			m.noteChat, cmd = m.noteChat.Update(msg)
			return m, cmd
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

		if m.newFolderMode {
			return m.updateNewFolder(msg)
		}

		if m.moveFileMode {
			return m.updateMoveFile(msg)
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
			// Show template picker first, then name input
			m.templates.SetSize(m.width, m.height)
			m.templates.Open()
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

		case "ctrl+w":
			m.canvas.SetSize(m.width, m.height)
			m.canvas.Open()
			return m, nil

		case "ctrl+l":
			m.calendar.SetSize(m.width, m.height)
			// Pass note contents for task parsing
			noteContents := make(map[string]string)
			for _, p := range m.vault.SortedPaths() {
				if note := m.vault.GetNote(p); note != nil {
					noteContents[p] = note.Content
				}
			}
			m.calendar.SetNoteContents(noteContents)
			m.calendar.Open()
			return m, nil

		case "ctrl+r":
			m.bots.SetSize(m.width, m.height)
			m.bots.SetAIConfig(m.config.AIProvider, m.config.OllamaModel, m.config.OllamaURL, m.config.OpenAIKey, m.config.OpenAIModel)
			noteContents := make(map[string]string)
			tagMap := make(map[string][]string)
			for _, p := range m.vault.SortedPaths() {
				if note := m.vault.GetNote(p); note != nil {
					noteContents[p] = note.Content
					if tags, ok := note.Frontmatter["tags"]; ok {
						if tagList, ok := tags.([]interface{}); ok {
							for _, t := range tagList {
								if s, ok := t.(string); ok {
									tagMap[s] = append(tagMap[s], p)
								}
							}
						}
					}
				}
			}
			m.bots.SetVaultData(noteContents, tagMap)
			m.bots.SetCurrentNote(m.activeNote, m.editor.GetContent())
			m.bots.Open()
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
			// If multi-cursors are active, clear them first
			if m.focus == focusEditor && !m.viewMode && m.editor.HasMultiCursors() {
				m.editor.clearMultiCursors()
				return m, nil
			}
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
			// Ghost writer: accept suggestion with Tab
			if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "tab" {
				if m.ghostWriter != nil && m.ghostWriter.GetSuggestion() != "" {
					suggestion := m.ghostWriter.Accept()
					m.editor.InsertText(suggestion)
					line, col := m.editor.GetCursor()
					m.statusbar.SetCursor(line, col)
					m.statusbar.SetWordCount(m.editor.GetWordCount())
					return m, nil
				}
			}
			m.editor, cmd = m.editor.Update(msg)
			// Snippet expansion: when space is typed, check if word before cursor is a snippet trigger
			if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == " " {
				m.tryExpandSnippet()
			}
			line, col := m.editor.GetCursor()
			m.statusbar.SetCursor(line, col)
			m.statusbar.SetWordCount(m.editor.GetWordCount())
			// Ghost writer: trigger completion on edits
			if m.ghostWriter != nil && m.ghostWriter.IsEnabled() {
				if keyMsg, ok := msg.(tea.KeyMsg); ok {
					k := keyMsg.String()
					// Only trigger on actual text input, not navigation
					if len(k) == 1 || k == "space" || k == "backspace" || k == "enter" {
						m.ghostWriter.Dismiss()
						ghostCmd := m.ghostWriter.OnEdit(m.editor.content, m.editor.cursor, m.editor.col)
						if ghostCmd != nil {
							cmd = tea.Batch(cmd, ghostCmd)
						}
					}
				}
			}
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
		m.templates.SetSize(m.width, m.height)
		m.templates.Open()
	case CmdSaveNote:
		cmd := m.saveCurrentNote()
		hookCmd := m.runPluginSaveHooks()
		syncCmd := m.autoSync.CommitAndPush()
		// Auto-tag if enabled
		var tagCmd tea.Cmd
		if m.autoTagger != nil && m.autoTagger.IsEnabled() {
			m.autoTagger.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
			// Collect existing vault tags for consistency
			var existingTags []string
			tagSet := make(map[string]bool)
			for _, p := range m.vault.SortedPaths() {
				if note := m.vault.GetNote(p); note != nil {
					if tags, ok := note.Frontmatter["tags"]; ok {
						if tagList, ok := tags.([]interface{}); ok {
							for _, t := range tagList {
								if s, ok := t.(string); ok && !tagSet[s] {
									existingTags = append(existingTags, s)
									tagSet[s] = true
								}
							}
						}
					}
				}
			}
			m.autoTagger.SetVaultTags(existingTags)
			tagCmd = m.autoTagger.TagNote(m.editor.GetContent())
		}
		m.statusbar.SetMessage("Saved " + m.activeNote)
		return m, tea.Batch(cmd, hookCmd, syncCmd, tagCmd, m.clearMessageAfter(2*time.Second))
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
	case CmdShowCanvas:
		m.canvas.SetSize(m.width, m.height)
		m.canvas.Open()
	case CmdShowCalendar:
		m.calendar.SetSize(m.width, m.height)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.calendar.SetNoteContents(noteContents)
		m.calendar.Open()
	case CmdShowBots:
		m.bots.SetSize(m.width, m.height)
		m.bots.SetAIConfig(m.config.AIProvider, m.config.OllamaModel, m.config.OllamaURL, m.config.OpenAIKey, m.config.OpenAIModel)
		noteContents := make(map[string]string)
		tagMap := make(map[string][]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
				if tags, ok := note.Frontmatter["tags"]; ok {
					if tagList, ok := tags.([]interface{}); ok {
						for _, t := range tagList {
							if s, ok := t.(string); ok {
								tagMap[s] = append(tagMap[s], p)
							}
						}
					}
				}
			}
		}
		m.bots.SetVaultData(noteContents, tagMap)
		m.bots.SetCurrentNote(m.activeNote, m.editor.GetContent())
		m.bots.Open()
	case CmdNewFolder:
		m.newFolderMode = true
		m.newFolderName = ""
	case CmdMoveFile:
		if m.activeNote != "" {
			m.moveFileMode = true
			m.moveFileCursor = 0
			m.moveFileDirs = m.getVaultDirs()
		}
	case CmdExportNote:
		m.export.SetSize(m.width, m.height)
		m.export.Open(m.activeNote, m.editor.GetContent(), m.vault.Root)
	case CmdGitOverlay:
		m.git.SetSize(m.width, m.height)
		m.git.Open()
		return m, m.git.RefreshAll()
	case CmdPluginManager:
		m.plugins.SetSize(m.width, m.height)
		m.plugins.Open()
	case CmdContentSearch:
		m.contentSearch.SetSize(m.width, m.height)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.contentSearch.Open(noteContents)
	case CmdSpellCheck:
		if m.spellcheck.IsAvailable() {
			m.spellcheck.SetSize(m.width, m.height)
			m.spellcheck.Open(m.editor.GetContent())
		} else {
			m.statusbar.SetMessage("Spell check unavailable (install aspell or hunspell)")
			return m, m.clearMessageAfter(3 * time.Second)
		}
	case CmdPublishSite:
		m.publisher.SetSize(m.width, m.height)
		m.publisher.Open()
	case CmdSplitPane:
		m.splitPane.SetSize(m.width, m.height)
		m.splitPane.Open()
		// Load current note and let user pick second note
		if m.activeNote != "" {
			content := strings.Split(m.editor.GetContent(), "\n")
			m.splitPane.SetLeftContent(m.activeNote, content)
		}
	case CmdRunLuaScript:
		m.luaOverlay.SetSize(m.width, m.height)
		m.luaOverlay.Open(m.activeNote, m.editor.GetContent(), nil)
	case CmdFlashcards:
		m.flashcards.SetSize(m.width, m.height)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.flashcards.LoadCards(noteContents)
		m.flashcards.Open()
	case CmdQuizMode:
		m.quizMode.SetSize(m.width, m.height)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.quizMode.SetNoteContents(noteContents)
		m.quizMode.Open()
	case CmdLearnDashboard:
		m.learnDash.SetSize(m.width, m.height)
		// Update mastery stats from flashcards
		fc := &m.flashcards
		total, due, newC, mastered := fc.GetStats()
		m.learnDash.SetCardStats(total, due, newC, mastered)
		m.learnDash.Open()
	case CmdAIChat:
		m.aiChat.SetSize(m.width, m.height)
		m.aiChat.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.aiChat.SetNotes(noteContents)
		m.aiChat.Open()
	case CmdComposer:
		m.composer.SetSize(m.width, m.height)
		m.composer.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
		m.composer.SetExistingNotes(m.vault.SortedPaths())
		m.composer.Open()
	case CmdKnowledgeGraph:
		m.knowledgeGraph.SetSize(m.width, m.height)
		allNotes := m.vault.SortedPaths()
		noteLinks := make(map[string][]string)
		backlinks := make(map[string][]string)
		for _, p := range allNotes {
			noteLinks[p] = m.index.GetOutgoingLinks(p)
			backlinks[p] = m.index.GetBacklinks(p)
		}
		m.knowledgeGraph.SetGraphData(allNotes, noteLinks, backlinks)
		m.knowledgeGraph.Open()
	case CmdAutoLink:
		if m.activeNote != "" {
			m.autoLinker.SetNotes(m.vault.SortedPaths())
			suggestions := m.autoLinker.FindUnlinkedMentions(m.editor.GetContent(), m.activeNote)
			if len(suggestions) > 0 {
				m.statusbar.SetMessage(fmt.Sprintf("Found %d unlinked mentions", len(suggestions)))
				// Apply first suggestion as demo, or show count
			} else {
				m.statusbar.SetMessage("No unlinked mentions found")
			}
			return m, m.clearMessageAfter(3 * time.Second)
		}
	case CmdSimilarNotes:
		if m.activeNote != "" {
			// Build TF-IDF index if needed
			if m.tfidfIndex == nil {
				noteContents := make(map[string]string)
				for _, p := range m.vault.SortedPaths() {
					if note := m.vault.GetNote(p); note != nil {
						noteContents[p] = note.Content
					}
				}
				m.tfidfIndex = BuildTFIDF(noteContents)
			}
			similar := FindSimilar(m.tfidfIndex, m.activeNote, 5)
			if len(similar) > 0 {
				names := make([]string, len(similar))
				for i, s := range similar {
					names[i] = fmt.Sprintf("%s (%.0f%%)", strings.TrimSuffix(filepath.Base(s.Path), ".md"), s.Score*100)
				}
				m.statusbar.SetMessage("Similar: " + strings.Join(names, ", "))
			} else {
				m.statusbar.SetMessage("No similar notes found")
			}
			return m, m.clearMessageAfter(5 * time.Second)
		}
	case CmdTableEditor:
		if m.activeNote != "" {
			m.tableEditor.SetSize(m.width, m.height)
			m.tableEditor.Open(m.editor.content, m.editor.cursor)
		}
	case CmdSemanticSearch:
		m.semanticSearch.SetSize(m.width, m.height)
		m.semanticSearch.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.semanticSearch.SetNotes(noteContents)
		m.semanticSearch.Open()
	case CmdThreadWeaver:
		m.threadWeaver.SetSize(m.width, m.height)
		m.threadWeaver.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.threadWeaver.SetNotes(m.vault.SortedPaths(), noteContents)
		m.threadWeaver.Open()
	case CmdNoteChat:
		if m.activeNote != "" {
			m.noteChat.SetSize(m.width, m.height)
			m.noteChat.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
			m.noteChat.Open(m.activeNote, m.editor.GetContent())
		}
	case CmdToggleGhostWriter:
		if m.ghostWriter != nil {
			m.ghostWriter.SetEnabled(!m.ghostWriter.IsEnabled())
			if m.ghostWriter.IsEnabled() {
				m.ghostWriter.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
				m.statusbar.SetMessage("Ghost Writer enabled")
			} else {
				m.statusbar.SetMessage("Ghost Writer disabled")
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdImportObsidian:
		imported := config.ImportObsidianConfig(m.vault.Root)
		if imported != nil {
			m.config = *imported
			m.config.Save()
			ApplyTheme(m.config.Theme)
			m.syncConfigToComponents()
			report := config.ImportReport(m.vault.Root)
			m.statusbar.SetMessage(report)
		} else {
			m.statusbar.SetMessage("No .obsidian/ directory found")
		}
		return m, m.clearMessageAfter(5 * time.Second)
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
	m.sidebar.fileTree.SetFocused(f == focusSidebar)
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

	// Adaptive layout: auto-collapse panels for small terminals
	if m.width < 80 {
		// Very narrow — editor only (mobile-friendly)
		layout = "minimal"
	} else if m.width < 120 {
		// Medium — sidebar + editor (no backlinks)
		if layout == "default" {
			layout = "writer"
		}
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

	// Compact height for very small terminals
	contentHeight := m.height - 3
	if m.height < 20 && m.config.ShowHelp {
		contentHeight = m.height - 2 // skip help bar
	}

	m.sidebar.SetSize(sidebarWidth, contentHeight)
	m.editor.SetSize(editorWidth, contentHeight)
	m.renderer.SetSize(editorWidth, contentHeight)
	m.backlinks.SetSize(backlinksWidth, contentHeight)
	m.statusbar.SetWidth(m.width)
	m.publisher.SetSize(m.width, m.height)
	m.splitPane.SetSize(m.width, m.height)
	m.luaOverlay.SetSize(m.width, m.height)
	m.flashcards.SetSize(m.width, m.height)
	m.quizMode.SetSize(m.width, m.height)
	m.learnDash.SetSize(m.width, m.height)
	m.aiChat.SetSize(m.width, m.height)
	m.composer.SetSize(m.width, m.height)
	m.knowledgeGraph.SetSize(m.width, m.height)
	m.tableEditor.SetSize(m.width, m.height)
	m.semanticSearch.SetSize(m.width, m.height)
	m.threadWeaver.SetSize(m.width, m.height)
	m.noteChat.SetSize(m.width, m.height)
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
	// AI status indicator
	aiModel := ""
	switch m.config.AIProvider {
	case "ollama":
		aiModel = m.config.OllamaModel
	case "openai":
		aiModel = m.config.OpenAIModel
	}
	m.statusbar.SetAIStatus(m.config.AIProvider, aiModel)
	m.autoSync.SetEnabled(m.config.GitAutoSync)
	if m.ghostWriter != nil {
		m.ghostWriter.SetEnabled(m.config.GhostWriter)
		m.ghostWriter.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
	}
	if m.autoTagger != nil {
		m.autoTagger.SetEnabled(m.config.AutoTag)
	}
}

func (m *Model) applyTagsToNote(tags []string) {
	content := m.editor.GetContent()
	lines := strings.Split(content, "\n")

	// Check if frontmatter exists
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		// Find the closing ---
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				endIdx = i
				break
			}
		}

		if endIdx > 0 {
			// Look for existing tags: line in frontmatter
			tagLineIdx := -1
			for i := 1; i < endIdx; i++ {
				trimmed := strings.TrimSpace(lines[i])
				if strings.HasPrefix(trimmed, "tags:") {
					tagLineIdx = i
					break
				}
			}

			// Build the new tags line — merge with existing
			allTags := make(map[string]bool)
			if tagLineIdx >= 0 {
				// Parse existing tags
				existing := strings.TrimPrefix(strings.TrimSpace(lines[tagLineIdx]), "tags:")
				existing = strings.TrimSpace(existing)
				existing = strings.Trim(existing, "[]")
				for _, t := range strings.Split(existing, ",") {
					t = strings.TrimSpace(t)
					t = strings.Trim(t, "\"' ")
					if t != "" {
						allTags[t] = true
					}
				}
			}
			for _, t := range tags {
				allTags[t] = true
			}

			// Build sorted tag list
			var sortedTags []string
			for t := range allTags {
				sortedTags = append(sortedTags, t)
			}
			sort.Strings(sortedTags)
			newTagLine := "tags: [" + strings.Join(sortedTags, ", ") + "]"

			if tagLineIdx >= 0 {
				lines[tagLineIdx] = newTagLine
			} else {
				// Insert tags line before closing ---
				newLines := make([]string, 0, len(lines)+1)
				newLines = append(newLines, lines[:endIdx]...)
				newLines = append(newLines, newTagLine)
				newLines = append(newLines, lines[endIdx:]...)
				lines = newLines
			}

			newContent := strings.Join(lines, "\n")
			m.editor.LoadContent(newContent, m.activeNote)
			m.editor.modified = true
			// Save directly to disk
			os.WriteFile(filepath.Join(m.vault.Root, m.activeNote), []byte(newContent), 0644)
			// Re-scan vault for updated tags
			m.vault.Scan()
			m.index = vault.NewIndex(m.vault)
			m.index.Build()
			return
		}
	}

	// No frontmatter — create one with tags
	var sortedTags []string
	seen := make(map[string]bool)
	for _, t := range tags {
		if !seen[t] {
			sortedTags = append(sortedTags, t)
			seen[t] = true
		}
	}
	sort.Strings(sortedTags)
	frontmatter := "---\ntags: [" + strings.Join(sortedTags, ", ") + "]\n---\n"
	newContent := frontmatter + content
	m.editor.LoadContent(newContent, m.activeNote)
	m.editor.modified = true
	os.WriteFile(filepath.Join(m.vault.Root, m.activeNote), []byte(newContent), 0644)
	m.vault.Scan()
	m.index = vault.NewIndex(m.vault)
	m.index.Build()
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

// tryExpandSnippet checks if the word before the cursor (before the space just typed)
// matches a snippet trigger and replaces it with the expanded content.
func (m *Model) tryExpandSnippet() {
	if m.snippets == nil {
		return
	}
	line := m.editor.content[m.editor.cursor]
	col := m.editor.col
	// col points after the space. The word is before the space.
	if col < 2 {
		return
	}
	// Find the word before the space (col-1 is the space)
	end := col - 1
	start := end
	for start > 0 && line[start-1] != ' ' && line[start-1] != '\t' {
		start--
	}
	word := line[start:end]
	if expanded, ok := m.snippets.TryExpand(word); ok {
		m.editor.saveSnapshot()
		// Remove the trigger word and the trailing space
		before := line[:start]
		after := line[col:]
		// Handle multi-line expansion
		expandedLines := strings.Split(expanded, "\n")
		if len(expandedLines) == 1 {
			m.editor.content[m.editor.cursor] = before + expandedLines[0] + after
			m.editor.col = start + len(expandedLines[0])
		} else {
			// First line
			m.editor.content[m.editor.cursor] = before + expandedLines[0]
			// Middle lines
			newContent := make([]string, 0, len(m.editor.content)+len(expandedLines)-1)
			newContent = append(newContent, m.editor.content[:m.editor.cursor+1]...)
			for i := 1; i < len(expandedLines)-1; i++ {
				newContent = append(newContent, expandedLines[i])
			}
			// Last line
			lastExp := expandedLines[len(expandedLines)-1]
			newContent = append(newContent, lastExp+after)
			newContent = append(newContent, m.editor.content[m.editor.cursor+1:]...)
			m.editor.content = newContent
			m.editor.cursor += len(expandedLines) - 1
			m.editor.col = len(lastExp)
		}
		m.editor.modified = true
		m.editor.countWords()
	}
}

func (m *Model) getAIModel() string {
	switch m.config.AIProvider {
	case "ollama":
		return m.config.OllamaModel
	case "openai":
		return m.config.OpenAIModel
	default:
		return m.config.OllamaModel
	}
}

func (m *Model) buildBacklinkItems(paths []string, targetNote string) []BacklinkItem {
	var items []BacklinkItem
	baseName := strings.TrimSuffix(filepath.Base(targetNote), ".md")
	for _, p := range paths {
		note := m.vault.GetNote(p)
		ctx := ""
		lineNum := 0
		if note != nil {
			lines := strings.Split(note.Content, "\n")
			for i, line := range lines {
				if strings.Contains(line, "[["+baseName+"]]") || strings.Contains(line, "[["+targetNote+"]]") {
					ctx = strings.TrimSpace(line)
					lineNum = i
					break
				}
			}
		}
		items = append(items, BacklinkItem{Path: p, Context: ctx, LineNum: lineNum})
	}
	return items
}

func (m *Model) buildOutgoingItems(paths []string) []BacklinkItem {
	var items []BacklinkItem
	for _, p := range paths {
		items = append(items, BacklinkItem{Path: p})
	}
	return items
}

func (m *Model) handlePluginOutput(pluginName, output string) {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "MSG:"):
			m.statusbar.SetMessage("[" + pluginName + "] " + strings.TrimPrefix(line, "MSG:"))
		case strings.HasPrefix(line, "CONTENT:"):
			// base64-encoded replacement content
			encoded := strings.TrimPrefix(line, "CONTENT:")
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
				m.editor.SetContent(string(decoded))
			}
		case strings.HasPrefix(line, "INSERT:"):
			encoded := strings.TrimPrefix(line, "INSERT:")
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
				m.editor.InsertText(string(decoded))
			}
		default:
			m.statusbar.SetMessage("[" + pluginName + "] " + line)
		}
	}
}

func (m *Model) runPluginSaveHooks() tea.Cmd {
	enabled := m.plugins.EnabledPlugins()
	if len(enabled) == 0 {
		return nil
	}
	notePath := filepath.Join(m.vault.Root, m.activeNote)
	return RunPluginHook(enabled, "on_save", notePath, m.editor.GetContent(), m.vault.Root)
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
		// Unload Ollama model to free resources
		if m.config.AIProvider == "ollama" {
			stopOllama(m.config.OllamaModel)
		}
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
	if m.canvas.IsActive() {
		overlay := m.canvas.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.calendar.IsActive() {
		overlay := m.calendar.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.bots.IsActive() {
		overlay := m.bots.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.export.IsActive() {
		overlay := m.export.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.git.IsActive() {
		overlay := m.git.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.plugins.IsActive() {
		overlay := m.plugins.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.contentSearch.IsActive() {
		overlay := m.contentSearch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.spellcheck.IsActive() {
		overlay := m.spellcheck.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.publisher.IsActive() {
		overlay := m.publisher.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.luaOverlay.IsActive() {
		overlay := m.luaOverlay.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.flashcards.IsActive() {
		overlay := m.flashcards.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.quizMode.IsActive() {
		overlay := m.quizMode.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.learnDash.IsActive() {
		overlay := m.learnDash.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.aiChat.IsActive() {
		overlay := m.aiChat.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.composer.IsActive() {
		overlay := m.composer.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.knowledgeGraph.IsActive() {
		overlay := m.knowledgeGraph.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.tableEditor.IsActive() {
		overlay := m.tableEditor.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.semanticSearch.IsActive() {
		overlay := m.semanticSearch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.threadWeaver.IsActive() {
		overlay := m.threadWeaver.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.noteChat.IsActive() {
		overlay := m.noteChat.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.splitPane.IsActive() {
		view = m.splitPane.View()
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
	if m.newFolderMode {
		overlay := m.renderNewFolderOverlay()
		view = m.overlayCenter(view, overlay)
	}
	if m.moveFileMode {
		overlay := m.renderMoveFileOverlay()
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
		Render("  " + IconSearchChar + " Quick Open")
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
			icon := lipgloss.NewStyle().Foreground(blue).Render(IconFileChar)
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
		Render("  " + IconNewChar + " New Note")
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
		Render("  " + IconTrashChar + " Delete Note")
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

// ---------------------------------------------------------------------------
// Folder management
// ---------------------------------------------------------------------------

func (m *Model) getVaultDirs() []string {
	dirSet := map[string]bool{".": true}
	for _, p := range m.vault.SortedPaths() {
		dir := filepath.Dir(p)
		if dir != "." {
			dirSet[dir] = true
			// Also add parent dirs
			for dir != "." {
				dirSet[dir] = true
				dir = filepath.Dir(dir)
			}
		}
	}
	dirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		if d == "." {
			dirs = append(dirs, "(root)")
		} else {
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func (m Model) updateNewFolder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.newFolderMode = false
		return m, nil
	case "enter":
		if m.newFolderName != "" {
			folderPath := filepath.Join(m.vault.Root, m.newFolderName)
			if err := os.MkdirAll(folderPath, 0755); err == nil {
				// Create a .gitkeep so folder shows up
				m.statusbar.SetMessage("Created folder: " + m.newFolderName)
			}
		}
		m.newFolderMode = false
		return m, m.clearMessageAfter(2 * time.Second)
	case "backspace":
		if len(m.newFolderName) > 0 {
			m.newFolderName = m.newFolderName[:len(m.newFolderName)-1]
		}
		return m, nil
	default:
		char := msg.String()
		if len(char) == 1 && char[0] >= 32 {
			m.newFolderName += char
		}
		return m, nil
	}
}

func (m Model) updateMoveFile(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.moveFileMode = false
		return m, nil
	case "up", "k":
		if m.moveFileCursor > 0 {
			m.moveFileCursor--
		}
		return m, nil
	case "down", "j":
		if m.moveFileCursor < len(m.moveFileDirs)-1 {
			m.moveFileCursor++
		}
		return m, nil
	case "enter":
		if m.activeNote != "" && m.moveFileCursor < len(m.moveFileDirs) {
			targetDir := m.moveFileDirs[m.moveFileCursor]
			if targetDir == "(root)" {
				targetDir = ""
			}
			baseName := filepath.Base(m.activeNote)
			var newPath string
			if targetDir == "" {
				newPath = baseName
			} else {
				newPath = filepath.Join(targetDir, baseName)
			}

			if newPath != m.activeNote {
				oldFullPath := filepath.Join(m.vault.Root, m.activeNote)
				newFullPath := filepath.Join(m.vault.Root, newPath)
				os.MkdirAll(filepath.Dir(newFullPath), 0755)
				if err := os.Rename(oldFullPath, newFullPath); err == nil {
					m.vault.Scan()
					m.index = vault.NewIndex(m.vault)
					m.index.Build()
					paths := m.vault.SortedPaths()
					m.sidebar.SetFiles(paths)
					m.autocomplete.SetNotes(paths)
					m.statusbar.SetNoteCount(m.vault.NoteCount())
					m.loadNote(newPath)
					m.sidebar.cursor = m.findFileIndex(newPath)
					m.statusbar.SetMessage("Moved to " + newPath)
				}
			}
		}
		m.moveFileMode = false
		return m, m.clearMessageAfter(2 * time.Second)
	}
	return m, nil
}

func (m Model) renderNewFolderOverlay() string {
	width := m.width / 3
	if width < 40 {
		width = 40
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(peach).
		Bold(true).
		Render("  " + IconFolderChar + " New Folder")
	b.WriteString(title)
	b.WriteString("\n\n")

	prompt := lipgloss.NewStyle().Foreground(peach).Bold(true).Render(" Name: ")
	input := m.newFolderName + DimStyle.Render("_")
	b.WriteString(prompt + input)
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Enter to create, Esc to cancel"))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Use / for nested folders (e.g. projects/web)"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(peach).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func (m Model) renderMoveFileOverlay() string {
	width := m.width / 3
	if width < 40 {
		width = 40
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(blue).
		Bold(true).
		Render("  " + IconFolderChar + " Move Note")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Moving: " + m.activeNote))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	visibleItems := m.height - 10
	if visibleItems < 5 {
		visibleItems = 5
	}

	start := 0
	if m.moveFileCursor >= visibleItems {
		start = m.moveFileCursor - visibleItems + 1
	}
	end := start + visibleItems
	if end > len(m.moveFileDirs) {
		end = len(m.moveFileDirs)
	}

	for i := start; i < end; i++ {
		dir := m.moveFileDirs[i]
		icon := lipgloss.NewStyle().Foreground(peach).Render(IconFolderChar)
		if i == m.moveFileCursor {
			line := lipgloss.NewStyle().
				Background(surface0).
				Foreground(peach).
				Bold(true).
				Width(width - 6).
				Render("  " + icon + " " + dir)
			b.WriteString(line)
		} else {
			b.WriteString("  " + icon + " " + NormalItemStyle.Render(dir))
		}
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  Enter: move here  Esc: cancel"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
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
