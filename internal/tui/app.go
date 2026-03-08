package tui

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
type autoSaveTickMsg struct{ editTime time.Time }

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
	globalReplace  GlobalReplace
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

	// Batch 2 features
	vimState      *VimState
	fileWatcher   *FileWatcher
	breadcrumb    *Breadcrumb
	linkCompleter *LinkCompleter
	pomodoro      Pomodoro
	webClipper    WebClipper
	kanban        Kanban
	tabBar        *TabBar
	zettelkasten  *ZettelkastenGenerator

	// AI features
	vaultRefactor  VaultRefactor
	dailyBriefing  DailyBriefing

	// Encryption & preview
	encryption      Encryption
	backlinkPreview BacklinkPreview

	// History, workspaces, timeline
	gitHistory GitHistory
	workspace  Workspace
	timeline   Timeline

	// Vault switch, frontmatter editor, folding
	vaultSwitch     VaultSwitch
	frontmatterEdit FrontmatterEditor
	foldState       FoldState
	research        ResearchAgent
	imageManager    ImageManager
	themeEditor     ThemeEditor
	linkAssist      LinkAssist
	taskManager     TaskManager
	blogPublisher    BlogPublisher
	aiTemplates      AITemplates
	languageLearning LanguageLearning
	habitTracker     HabitTracker
	focusSession     FocusSession
	standupGen       StandupGenerator
	noteHistory      NoteHistory
	smartConnect     SmartConnections
	writingStats     WritingStats
	quickCapture     QuickCapture
	dashboard        Dashboard
	mindMap          MindMap
	journalPrompts   JournalPrompts
	clipManager      ClipManager
	dailyPlanner     DailyPlanner
	aiScheduler      AIScheduler
	recurringTasks   RecurringTasks
	notePreview      NotePreview
	scratchpad       Scratchpad
	backup           Backup
	onboarding       Onboarding
	projectMode      ProjectMode
	nlSearch         NLSearch
	writingCoach     WritingCoach
	dataview         DataviewOverlay
	timeTracker      TimeTracker
	dueTodayCount    int

	// Cross-component refresh flag
	needsRefresh bool

	// Slash command menu
	slashMenu *SlashMenu

	// Toast notifications
	toast *Toast

	// Auto-save debounce
	lastEditTime time.Time

	// Exit splash
	exitSplash    ExitSplash
	showExitSplash bool
	sessionStart  time.Time

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
		globalReplace:  NewGlobalReplace(),
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
		vimState:       NewVimState(),
		fileWatcher:    NewFileWatcher(vaultPath),
		breadcrumb:     NewBreadcrumb(),
		linkCompleter:  NewLinkCompleter(),
		pomodoro:       NewPomodoro(),
		webClipper:     NewWebClipper(),
		kanban:         NewKanban(),
		tabBar:         NewTabBar(),
		zettelkasten:   NewZettelkastenGenerator(),
		vaultRefactor:  NewVaultRefactor(),
		dailyBriefing:  NewDailyBriefing(),
		encryption:      NewEncryption(),
		backlinkPreview: NewBacklinkPreview(),
		gitHistory:      NewGitHistory(),
		workspace:       NewWorkspace(config.ConfigDir()),
		timeline:        NewTimeline(),
		vaultSwitch:     NewVaultSwitch(),
		frontmatterEdit: NewFrontmatterEditor(),
		foldState:       NewFoldState(),
		research:        NewResearchAgent(),
		imageManager:    NewImageManager(),
		themeEditor:     NewThemeEditor(),
		linkAssist:      NewLinkAssist(),
		taskManager:     NewTaskManager(),
		blogPublisher:    NewBlogPublisher(),
		aiTemplates:      NewAITemplates(),
		languageLearning: NewLanguageLearning(),
		habitTracker:     NewHabitTracker(),
		dailyPlanner:    NewDailyPlanner(),
		aiScheduler:     NewAIScheduler(),
		notePreview:     NewNotePreview(),
		dataview:        NewDataviewOverlay(),
		slashMenu:      NewSlashMenu(),
		toast:          NewToast(),
		showSplash:     cfg.ShowSplash,
		splash:         NewSplashModel(vaultPath, v.NoteCount()),
		viewMode:       cfg.DefaultViewMode,
		sessionStart:   time.Now(),
	}

	m.statusbar.SetVaultPath(vaultPath)
	m.statusbar.SetNoteCount(v.NoteCount())
	m.dueTodayCount = CountTasksDueToday(v.Notes)
	m.statusbar.SetDueTodayCount(m.dueTodayCount)
	m.autocomplete.SetNotes(paths)
	m.plugins.SetVaultPath(vaultPath)
	m.canvas.SetVaultPath(vaultPath)
	m.publisher.SetVaultPath(vaultPath)
	m.luaOverlay.SetEngine(m.luaEngine)
	m.renderer.SetVaultNotes(m.vault.Notes)
	m.editor.SetFoldState(&m.foldState)
	m.renderer.SetVaultRoot(vaultPath)

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

	// Link completer setup
	snippets := make(map[string]string)
	for _, p := range paths {
		if note := v.GetNote(p); note != nil {
			preview := note.Content
			if len(preview) > 80 {
				preview = preview[:80]
			}
			preview = strings.ReplaceAll(preview, "\n", " ")
			snippets[p] = preview
		}
	}
	m.linkCompleter.SetNotes(paths, snippets)

	// Renderer vault root for image resolution
	m.renderer.SetVaultRoot(v.Root)

	// Zettelkasten snippets
	if m.zettelkasten != nil && m.snippets != nil {
		zkSnippets := ZettelkastenSnippets(m.zettelkasten)
		for trigger, expansion := range zkSnippets {
			m.snippets.AddSnippet(trigger, expansion)
		}
	}

	// Vim mode setup
	m.vimState.SetEnabled(cfg.VimMode)

	// Ghost writer setup
	m.ghostWriter.SetEnabled(cfg.GhostWriter)

	// Apply config to components
	m.syncConfigToComponents()

	// Configure auto git sync
	m.autoSync.SetEnabled(cfg.GitAutoSync)

	// Restore persisted tabs from previous session
	if m.tabBar != nil {
		validPaths := make(map[string]bool, len(paths))
		for _, p := range paths {
			validPaths[p] = true
		}
		m.tabBar.LoadTabs(vaultPath, validPaths)
		if active := m.tabBar.GetActive(); active != "" {
			m.loadNote(active)
		} else if len(paths) > 0 {
			m.loadNote(paths[0])
		}
	} else if len(paths) > 0 {
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

	m.foldState.UnfoldAll()
	m.bookmarks.AddRecent(relPath)
	if m.breadcrumb != nil {
		m.breadcrumb.Push(relPath)
	}
	if m.tabBar != nil {
		m.tabBar.AddTab(relPath)
		m.tabBar.SetActive(relPath)
	}
	if m.pomodoro.IsRunning() {
		m.pomodoro.NoteEdited(relPath)
	}
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
	// File watcher
	if m.fileWatcher != nil && m.fileWatcher.IsEnabled() {
		cmds = append(cmds, m.fileWatcher.Start())
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
			return m, nil
		case tea.KeyMsg:
			// Any key press immediately dismisses the splash
			m.showSplash = false
			if ShouldShowOnboarding() {
				m.onboarding.SetSize(m.width, m.height)
				m.onboarding.Open()
			}
			return m, nil
		case splashTickMsg:
			var cmd tea.Cmd
			m.splash, cmd = m.splash.Update(msg)
			if m.splash.IsDone() {
				m.showSplash = false
				if ShouldShowOnboarding() {
					m.onboarding.SetSize(m.width, m.height)
					m.onboarding.Open()
				}
				return m, nil
			}
			return m, cmd
		}
		return m, nil
	}

	// Exit splash handling
	if m.showExitSplash {
		switch msg := msg.(type) {
		case tea.WindowSizeMsg:
			m.exitSplash.width = msg.Width
			m.exitSplash.height = msg.Height
		case exitTickMsg:
			var cmd tea.Cmd
			m.exitSplash, cmd = m.exitSplash.Update(msg)
			if m.exitSplash.IsDone() {
				m.quitting = true
				return m, tea.Quit
			}
			return m, cmd
		case tea.KeyMsg:
			m.quitting = true
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.exitSplash, cmd = m.exitSplash.Update(msg)
		return m, cmd
	}

	switch msg := msg.(type) {
	case clearMessageMsg:
		m.statusbar.SetMessage("")
		return m, nil

	case vimMacroReplayMsg:
		if msg.idx >= len(msg.keys) {
			if m.vimState != nil {
				m.vimState.SetPlayingMacro(false)
			}
			return m, nil
		}
		key := msg.keys[msg.idx]
		nextMsg := vimMacroReplayMsg{keys: msg.keys, idx: msg.idx + 1}
		updatedModel, cmd := m.Update(key)
		resultModel := updatedModel.(Model)
		nextCmd := func() tea.Msg { return nextMsg }
		if cmd != nil {
			return resultModel, tea.Batch(cmd, nextCmd)
		}
		return resultModel, nextCmd

	case toastExpireMsg:
		if m.toast != nil {
			m.toast.HandleExpire()
		}
		return m, nil

	case autoSaveTickMsg:
		// Only save if this tick matches the last edit time (debounce)
		if m.config.AutoSave && msg.editTime.Equal(m.lastEditTime) && m.editor.modified && m.activeNote != "" {
			content := m.editor.GetContent()
			path := filepath.Join(m.vault.Root, m.activeNote)
			os.WriteFile(path, []byte(content), 0644)
			m.editor.modified = false
			m.statusbar.SetMessage("Auto-saved " + m.activeNote)
			return m, m.clearMessageAfter(2 * time.Second)
		}
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

	case splitPanePickMsg:
		if m.splitPane.IsActive() {
			note := m.vault.GetNote(msg.notePath)
			if note != nil {
				lines := strings.Split(note.Content, "\n")
				m.splitPane.SetRightContent(msg.notePath, lines)
			}
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

	case blogPublishResultMsg:
		if m.blogPublisher.IsActive() {
			var cmd tea.Cmd
			m.blogPublisher, cmd = m.blogPublisher.Update(msg)
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

	case vaultRefactorResultMsg, vaultRefactorTickMsg:
		if m.vaultRefactor.IsActive() {
			var cmd tea.Cmd
			m.vaultRefactor, cmd = m.vaultRefactor.Update(msg)
			if !m.vaultRefactor.IsActive() {
				if plan, ok := m.vaultRefactor.GetResult(); ok {
					m.applyVaultRefactor(plan)
				}
			}
			return m, cmd
		}
		return m, nil

	case gitHistoryResultMsg:
		if m.gitHistory.IsActive() {
			var cmd tea.Cmd
			m.gitHistory, cmd = m.gitHistory.Update(msg)
			if !m.gitHistory.IsActive() {
				if hash, ok := m.gitHistory.GetRestoreResult(); ok {
					// Restore file content from git
					_ = hash
					content := msg.output
					if content != "" && m.activeNote != "" {
						path := filepath.Join(m.vault.Root, m.activeNote)
						os.WriteFile(path, []byte(content), 0644)
						m.vault.Scan()
						m.index = vault.NewIndex(m.vault)
						m.index.Build()
						m.loadNote(m.activeNote)
						m.statusbar.SetMessage("Restored from git history")
						return m, m.clearMessageAfter(3 * time.Second)
					}
				}
			}
			return m, cmd
		}
		return m, nil

	case briefingResultMsg, briefingTickMsg:
		if m.dailyBriefing.IsActive() {
			var cmd tea.Cmd
			m.dailyBriefing, cmd = m.dailyBriefing.Update(msg)
			if !m.dailyBriefing.IsActive() {
				if content, ok := m.dailyBriefing.GetResult(); ok {
					m.writeBriefingToDailyNote(content)
				}
			}
			return m, cmd
		}
		return m, nil

	case aiTemplateResultMsg:
		if m.aiTemplates.IsActive() {
			var cmd tea.Cmd
			m.aiTemplates, cmd = m.aiTemplates.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiTemplateTickMsg:
		if m.aiTemplates.IsActive() {
			var cmd tea.Cmd
			m.aiTemplates, cmd = m.aiTemplates.Update(msg)
			return m, cmd
		}
		return m, nil

	case focusSessionTickMsg:
		if m.focusSession.IsActive() {
			var cmd tea.Cmd
			m.focusSession, cmd = m.focusSession.Update(msg)
			return m, cmd
		}
		return m, nil

	case noteHistoryResultMsg:
		if m.noteHistory.IsActive() {
			var cmd tea.Cmd
			m.noteHistory, cmd = m.noteHistory.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiSchedulerResultMsg:
		if m.aiScheduler.IsActive() {
			var cmd tea.Cmd
			m.aiScheduler, cmd = m.aiScheduler.Update(msg)
			return m, cmd
		}
		return m, nil

	case aiSchedulerTickMsg:
		if m.aiScheduler.IsActive() {
			var cmd tea.Cmd
			m.aiScheduler, cmd = m.aiScheduler.Update(msg)
			return m, cmd
		}
		return m, nil

	case nlSearchResultMsg:
		if m.nlSearch.IsActive() {
			var cmd tea.Cmd
			m.nlSearch, cmd = m.nlSearch.Update(msg)
			return m, cmd
		}
		return m, nil

	case nlSearchTickMsg:
		if m.nlSearch.IsActive() {
			var cmd tea.Cmd
			m.nlSearch, cmd = m.nlSearch.Update(msg)
			return m, cmd
		}
		return m, nil

	case writingCoachResultMsg:
		if m.writingCoach.IsActive() {
			var cmd tea.Cmd
			m.writingCoach, cmd = m.writingCoach.Update(msg)
			return m, cmd
		}
		return m, nil

	case writingCoachTickMsg:
		if m.writingCoach.IsActive() {
			var cmd tea.Cmd
			m.writingCoach, cmd = m.writingCoach.Update(msg)
			return m, cmd
		}
		return m, nil

	case timeTrackerTickMsg:
		if m.timeTracker.IsTimerRunning() {
			var cmd tea.Cmd
			m.timeTracker, cmd = m.timeTracker.Update(msg)
			return m, cmd
		}
		return m, nil

	case researchResultMsg:
		// Always handle — research runs in background even if overlay is closed
		if m.research.IsRunning() {
			m.research.running = false
			m.statusbar.SetResearchStatus("")
			if msg.err != nil {
				m.research.phase = researchError
				m.research.errorMsg = msg.err.Error()
				m.research.output = msg.output
				m.research.elapsed = time.Since(m.research.startTime).Truncate(time.Second).String()
				m.statusbar.SetMessage("Research failed: " + msg.err.Error())
			} else {
				m.research.phase = researchDone
				m.research.elapsed = time.Since(m.research.startTime).Truncate(time.Second).String()
				m.research.output = msg.output
				// Refresh vault to pick up new files
				m.vault.Scan()
				m.index = vault.NewIndex(m.vault)
				m.index.Build()
				paths := m.vault.SortedPaths()
				m.sidebar.SetFiles(paths)
				m.autocomplete.SetNotes(paths)
				m.statusbar.SetNoteCount(m.vault.NoteCount())
				if len(msg.filesHint) > 0 {
					m.research.createdFiles = msg.filesHint
				} else {
					for _, p := range paths {
						if strings.HasPrefix(p, "Research/") {
							m.research.createdFiles = append(m.research.createdFiles, p)
						}
					}
				}
				m.statusbar.SetMessage(fmt.Sprintf("Research complete: %d notes created — open via command palette", len(m.research.createdFiles)))
				// Auto-open the overlay to show results if it was closed
				if !m.research.IsActive() {
					m.research.Reopen()
				}
			}
			return m, m.clearMessageAfter(5 * time.Second)
		}
		return m, nil

	case researchTickMsg:
		if m.research.IsRunning() {
			m.research.elapsed = time.Since(m.research.startTime).Truncate(time.Second).String()
			m.statusbar.SetResearchStatus(m.research.StatusText())
			return m, m.research.tickElapsed()
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
			m.editor.SetGhostText(m.ghostWriter.GetSuggestion())
			return m, cmd
		}
		return m, nil

	case fileWatchTickMsg:
		if m.fileWatcher != nil && m.fileWatcher.IsEnabled() {
			if changeMsg, changed := m.fileWatcher.Check(); changed {
				// Rescan vault
				m.vault.Scan()
				m.index = vault.NewIndex(m.vault)
				m.index.Build()
				paths := m.vault.SortedPaths()
				m.sidebar.SetFiles(paths)
				m.autocomplete.SetNotes(paths)
				m.statusbar.SetNoteCount(m.vault.NoteCount())
				// Reload current note if it changed
				for _, p := range changeMsg.paths {
					rel, _ := filepath.Rel(m.vault.Root, p)
					if rel == m.activeNote {
						if note := m.vault.GetNote(m.activeNote); note != nil && !m.editor.modified {
							m.editor.LoadContent(note.Content, m.activeNote)
						}
						break
					}
				}
				_ = changeMsg
			}
			return m, m.fileWatcher.Tick()
		}
		return m, nil

	case pomodoroTickMsg:
		var cmd tea.Cmd
		m.pomodoro, cmd = m.pomodoro.Update(msg)
		return m, cmd

	case webClipResult, webClipTickMsg:
		if m.webClipper.IsActive() {
			var cmd tea.Cmd
			m.webClipper, cmd = m.webClipper.Update(msg)
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
			// Handle quick-add event: append task to daily note
			if evDate, evText, ok := m.calendar.PendingEvent(); ok {
				name := evDate + ".md"
				folder := m.config.DailyNotesFolder
				if folder != "" {
					name = filepath.Join(folder, name)
				}
				path := filepath.Join(m.vault.Root, name)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					os.MkdirAll(filepath.Dir(path), 0755)
					content := fmt.Sprintf("---\ndate: %s\ntype: daily\ntags: [daily]\n---\n\n# %s\n\n", evDate, evDate)
					os.WriteFile(path, []byte(content), 0644)
				}
				// Append the task line
				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
				if err == nil {
					f.WriteString("- [ ] " + evText + "\n")
					f.Close()
				}
				// Refresh vault data
				m.vault.Scan()
				m.index = vault.NewIndex(m.vault)
				m.index.Build()
				paths := m.vault.SortedPaths()
				m.sidebar.SetFiles(paths)
				m.autocomplete.SetNotes(paths)
				m.calendar.SetDailyNotes(paths)
				m.statusbar.SetNoteCount(m.vault.NoteCount())
				// Re-parse note contents for the calendar
				noteContents := make(map[string]string)
				for _, p := range paths {
					if note := m.vault.GetNote(p); note != nil {
						noteContents[p] = note.Content
					}
				}
				m.calendar.SetNoteContents(noteContents)
				m.statusbar.SetMessage("Task added to " + evDate)
			}
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

		if m.globalReplace.IsActive() {
			m.globalReplace, _ = m.globalReplace.Update(msg)
			if !m.globalReplace.IsActive() {
				// Reload editor if active note was modified
				if m.globalReplace.ModifiedFiles()[m.activeNote] {
					if note := m.vault.GetNote(m.activeNote); note != nil {
						m.editor.LoadContent(note.Content, m.editor.filePath)
					}
				}
				if file, line, ok := m.globalReplace.GetJumpResult(); ok {
					m.loadNote(file)
					m.sidebar.cursor = m.findFileIndex(file)
					m.editor.cursor = line
					m.setFocus(focusEditor)
				}
			}
			return m, nil
		}

		if m.aiTemplates.IsActive() {
			m.aiTemplates, _ = m.aiTemplates.Update(msg)
			if !m.aiTemplates.IsActive() {
				if title, content, ok := m.aiTemplates.GetResult(); ok {
					// Create the new note file
					fileName := title + ".md"
					relPath := fileName
					absPath := filepath.Join(m.vault.Root, relPath)
					if err := os.WriteFile(absPath, []byte(content), 0644); err == nil {
						m.vault.Scan()
						m.index.Build()
						m.sidebar.SetFiles(m.vault.SortedPaths())
						m.loadNote(relPath)
						m.sidebar.cursor = m.findFileIndex(relPath)
						m.setFocus(focusEditor)
						m.statusbar.SetMessage("Created " + fileName)
					}
				}
			}
			return m, nil
		}

		if m.languageLearning.IsActive() {
			m.languageLearning, _ = m.languageLearning.Update(msg)
			return m, nil
		}

		if m.habitTracker.IsActive() {
			m.habitTracker, _ = m.habitTracker.Update(msg)
			return m, nil
		}

		if m.focusSession.IsActive() {
			var cmd tea.Cmd
			m.focusSession, cmd = m.focusSession.Update(msg)
			return m, cmd
		}

		if m.standupGen.IsActive() {
			m.standupGen, _ = m.standupGen.Update(msg)
			if !m.standupGen.IsActive() {
				m.vault.Scan()
				m.index.Build()
				m.sidebar.SetFiles(m.vault.SortedPaths())
			}
			return m, nil
		}

		if m.noteHistory.IsActive() {
			m.noteHistory, _ = m.noteHistory.Update(msg)
			return m, nil
		}

		if m.smartConnect.IsActive() {
			m.smartConnect, _ = m.smartConnect.Update(msg)
			if !m.smartConnect.IsActive() {
				if notePath, ok := m.smartConnect.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.sidebar.cursor = m.findFileIndex(notePath)
				}
			}
			return m, nil
		}

		if m.writingStats.IsActive() {
			m.writingStats, _ = m.writingStats.Update(msg)
			return m, nil
		}

		if m.quickCapture.IsActive() {
			m.quickCapture, _ = m.quickCapture.Update(msg)
			if !m.quickCapture.IsActive() {
				if filePath, ok := m.quickCapture.GetResult(); ok {
					m.vault.Scan()
					m.index.Build()
					m.sidebar.SetFiles(m.vault.SortedPaths())
					m.statusbar.SetMessage("Saved to " + filePath)
				}
			}
			return m, nil
		}

		if m.dashboard.IsActive() {
			m.dashboard, _ = m.dashboard.Update(msg)
			if !m.dashboard.IsActive() {
				if action, ok := m.dashboard.GetAction(); ok {
					return m.executeCommand(action)
				}
			}
			return m, nil
		}

		if m.mindMap.IsActive() {
			m.mindMap, _ = m.mindMap.Update(msg)
			return m, nil
		}

		if m.journalPrompts.IsActive() {
			m.journalPrompts, _ = m.journalPrompts.Update(msg)
			if !m.journalPrompts.IsActive() {
				if filePath, ok := m.journalPrompts.GetResult(); ok {
					m.vault.Scan()
					m.index.Build()
					m.sidebar.SetFiles(m.vault.SortedPaths())
					m.loadNote(filePath)
					m.statusbar.SetMessage("Journal saved")
				}
			}
			return m, nil
		}

		if m.clipManager.IsActive() {
			m.clipManager, _ = m.clipManager.Update(msg)
			if !m.clipManager.IsActive() {
				if text, ok := m.clipManager.GetResult(); ok {
					m.editor.InsertText(text)
					m.statusbar.SetMessage("Pasted from clipboard")
				}
			}
			return m, nil
		}

		if m.dailyPlanner.IsActive() {
			m.dailyPlanner, _ = m.dailyPlanner.Update(msg)
			if !m.dailyPlanner.IsActive() {
				m.vault.Scan()
				m.index.Build()
				m.sidebar.SetFiles(m.vault.SortedPaths())
				m.statusbar.SetMessage("Daily planner closed")
			}
			return m, nil
		}

		if m.aiScheduler.IsActive() {
			var cmd tea.Cmd
			m.aiScheduler, cmd = m.aiScheduler.Update(msg)
			if !m.aiScheduler.IsActive() {
				if slots, ok := m.aiScheduler.GetSchedule(); ok && len(slots) > 0 {
					tasks, _, habits := m.gatherPlannerData()
					m.dailyPlanner.SetSize(m.width, m.height)
					m.dailyPlanner.Open(m.vault.Root, tasks, nil, habits)
					m.dailyPlanner.ApplyAISchedule(slots)
					m.statusbar.SetMessage("AI schedule applied to planner")
				}
			}
			return m, cmd
		}

		if m.recurringTasks.IsActive() {
			m.recurringTasks, _ = m.recurringTasks.Update(msg)
			if !m.recurringTasks.IsActive() {
				if count, ok := m.recurringTasks.GetCreatedCount(); ok && count > 0 {
					m.vault.Scan()
					m.index.Build()
					m.sidebar.SetFiles(m.vault.SortedPaths())
					m.statusbar.SetMessage(fmt.Sprintf("%d recurring tasks created", count))
				}
			}
			return m, nil
		}

		if m.notePreview.IsActive() {
			m.notePreview, _ = m.notePreview.Update(msg)
			if !m.notePreview.IsActive() {
				if notePath, ok := m.notePreview.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.sidebar.cursor = m.findFileIndex(notePath)
				}
			}
			return m, nil
		}

		if m.scratchpad.IsActive() {
			m.scratchpad, _ = m.scratchpad.Update(msg)
			return m, nil
		}

		if m.backup.IsActive() {
			m.backup, _ = m.backup.Update(msg)
			return m, nil
		}

		if m.onboarding.IsActive() {
			var cmd tea.Cmd
			m.onboarding, cmd = m.onboarding.Update(msg)
			if !m.onboarding.IsActive() {
				m.onboarding.MarkComplete()
			}
			return m, cmd
		}

		if m.projectMode.IsActive() {
			m.projectMode, _ = m.projectMode.Update(msg)
			if !m.projectMode.IsActive() {
				if notePath, ok := m.projectMode.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.sidebar.cursor = m.findFileIndex(notePath)
				}
				if action, ok := m.projectMode.GetAction(); ok {
					return m.executeCommand(action)
				}
			}
			return m, nil
		}

		if m.nlSearch.IsActive() {
			var cmd tea.Cmd
			m.nlSearch, cmd = m.nlSearch.Update(msg)
			if !m.nlSearch.IsActive() {
				if notePath, ok := m.nlSearch.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.sidebar.cursor = m.findFileIndex(notePath)
				}
			}
			return m, cmd
		}

		if m.writingCoach.IsActive() {
			var cmd tea.Cmd
			m.writingCoach, cmd = m.writingCoach.Update(msg)
			if !m.writingCoach.IsActive() {
				if suggestion, ok := m.writingCoach.GetSuggestion(); ok {
					m.editor.InsertText(suggestion)
					m.statusbar.SetMessage("Writing suggestion applied")
				}
			}
			return m, cmd
		}

		if m.dataview.IsActive() {
			m.dataview, _ = m.dataview.Update(msg)
			if !m.dataview.IsActive() {
				if notePath, ok := m.dataview.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.sidebar.cursor = m.findFileIndex(notePath)
				}
			}
			return m, nil
		}

		if m.timeTracker.IsActive() {
			var cmd tea.Cmd
			m.timeTracker, cmd = m.timeTracker.Update(msg)
			return m, cmd
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
					m.editor.saveSnapshot()
					newLines := strings.Split(md, "\n")
					if endLine < startLine {
						// Insert mode: insert before startLine
						before := make([]string, len(m.editor.content[:startLine]))
						copy(before, m.editor.content[:startLine])
						after := m.editor.content[startLine:]
						m.editor.content = append(before, append(newLines, after...)...)
						m.statusbar.SetMessage("Table inserted")
					} else {
						// Replace mode
						before := make([]string, len(m.editor.content[:startLine]))
						copy(before, m.editor.content[:startLine])
						after := m.editor.content[endLine+1:]
						m.editor.content = append(before, append(newLines, after...)...)
						m.statusbar.SetMessage("Table updated")
					}
					m.editor.modified = true
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

		if m.vaultRefactor.IsActive() {
			var cmd tea.Cmd
			m.vaultRefactor, cmd = m.vaultRefactor.Update(msg)
			if !m.vaultRefactor.IsActive() {
				if plan, ok := m.vaultRefactor.GetResult(); ok {
					m.applyVaultRefactor(plan)
				}
			}
			return m, cmd
		}

		if m.dailyBriefing.IsActive() {
			var cmd tea.Cmd
			m.dailyBriefing, cmd = m.dailyBriefing.Update(msg)
			if !m.dailyBriefing.IsActive() {
				if content, ok := m.dailyBriefing.GetResult(); ok {
					m.writeBriefingToDailyNote(content)
				}
			}
			return m, cmd
		}

		if m.encryption.IsActive() {
			var cmd tea.Cmd
			m.encryption, cmd = m.encryption.Update(msg)
			if !m.encryption.IsActive() {
				if result, ok := m.encryption.GetResult(); ok {
					m.applyEncryptionResult(result)
				}
			}
			return m, cmd
		}

		if m.gitHistory.IsActive() {
			var cmd tea.Cmd
			m.gitHistory, cmd = m.gitHistory.Update(msg)
			if !m.gitHistory.IsActive() {
				if _, ok := m.gitHistory.GetRestoreResult(); ok {
					m.statusbar.SetMessage("Restoring from git...")
				}
			}
			return m, cmd
		}

		if m.workspace.IsActive() {
			var cmd tea.Cmd
			m.workspace, cmd = m.workspace.Update(msg)
			if !m.workspace.IsActive() {
				if layout, ok := m.workspace.GetLoadResult(); ok {
					m.applyWorkspaceLayout(layout)
				}
				if m.workspace.IsSaveRequested() {
					name := m.workspace.SaveName()
					layout := m.captureWorkspaceLayout(name)
					m.workspace.SaveLayout(layout)
					m.statusbar.SetMessage("Workspace saved: " + name)
				}
			}
			return m, cmd
		}

		if m.timeline.IsActive() {
			var cmd tea.Cmd
			m.timeline, cmd = m.timeline.Update(msg)
			if !m.timeline.IsActive() {
				if notePath, ok := m.timeline.GetSelectedNote(); ok {
					m.loadNote(notePath)
					m.sidebar.cursor = m.findFileIndex(notePath)
					m.setFocus(focusEditor)
				}
			}
			return m, cmd
		}

		if m.taskManager.IsActive() {
			var cmd tea.Cmd
			m.taskManager, cmd = m.taskManager.Update(msg)
			// Check if task manager wrote any files
			if m.taskManager.WasFileChanged() {
				changedNote := m.taskManager.ActiveNotePath()
				if changedNote == m.activeNote {
					if note := m.vault.GetNote(changedNote); note != nil {
						m.editor.LoadContent(note.Content, m.editor.filePath)
					}
				}
				m.dueTodayCount = CountTasksDueToday(m.vault.Notes)
				m.statusbar.SetDueTodayCount(m.dueTodayCount)
			}
			// Handle jump result (closes overlay)
			if notePath, lineNum, ok := m.taskManager.GetJumpResult(); ok {
				m.loadNote(notePath)
				m.sidebar.cursor = m.findFileIndex(notePath)
				m.setFocus(focusEditor)
				if lineNum > 0 {
					m.editor.cursor = lineNum - 1
					m.editor.scroll = maxInt(0, lineNum-m.editor.height/2)
				}
			}
			return m, cmd
		}

		if m.linkAssist.IsActive() {
			var cmd tea.Cmd
			m.linkAssist, cmd = m.linkAssist.Update(msg)
			if !m.linkAssist.IsActive() {
				if suggestions, ok := m.linkAssist.GetApplyResult(); ok && m.activeNote != "" {
					content := m.editor.GetContent()
					content = applyLinkSuggestions(content, suggestions)
					m.editor.LoadContent(content, m.editor.filePath)
					m.editor.modified = true
					m.statusbar.SetMessage(fmt.Sprintf("Linked %d mentions", len(suggestions)))
				}
			}
			return m, cmd
		}

		if m.blogPublisher.IsActive() {
			var cmd tea.Cmd
			m.blogPublisher, cmd = m.blogPublisher.Update(msg)
			return m, cmd
		}

		if m.themeEditor.IsActive() {
			var cmd tea.Cmd
			m.themeEditor, cmd = m.themeEditor.Update(msg)
			return m, cmd
		}

		if m.imageManager.IsActive() {
			var cmd tea.Cmd
			m.imageManager, cmd = m.imageManager.Update(msg)
			if !m.imageManager.IsActive() {
				if insertText, ok := m.imageManager.GetInsertResult(); ok && m.activeNote != "" {
					m.editor.InsertText(insertText)
					m.editor.modified = true
				}
			}
			return m, cmd
		}

		if m.research.IsActive() {
			var cmd tea.Cmd
			m.research, cmd = m.research.Update(msg)
			if !m.research.IsActive() {
				if filePath, ok := m.research.GetSelectedFile(); ok {
					m.loadNote(filePath)
					m.sidebar.cursor = m.findFileIndex(filePath)
					m.setFocus(focusEditor)
				}
			}
			return m, cmd
		}

		if m.vaultSwitch.IsActive() {
			var cmd tea.Cmd
			m.vaultSwitch, cmd = m.vaultSwitch.Update(msg)
			if !m.vaultSwitch.IsActive() {
				if vaultPath, ok := m.vaultSwitch.GetSelectedVault(); ok {
					// Relaunch with new vault
					newModel, err := NewModel(vaultPath)
					if err == nil {
						newModel.width = m.width
						newModel.height = m.height
						newModel.showSplash = false
						newModel.updateLayout()
						return &newModel, nil
					}
				}
			}
			return m, cmd
		}

		if m.frontmatterEdit.IsActive() {
			var cmd tea.Cmd
			m.frontmatterEdit, cmd = m.frontmatterEdit.Update(msg)
			if !m.frontmatterEdit.IsActive() {
				if yamlBlock, ok := m.frontmatterEdit.GetResult(); ok && m.activeNote != "" {
					content := m.editor.GetContent()
					// Replace existing frontmatter or prepend new one
					newContent := replaceFrontmatter(content, yamlBlock)
					m.editor.LoadContent(newContent, m.editor.filePath)
					m.editor.modified = true
					m.statusbar.SetMessage("Frontmatter updated")
				}
			}
			return m, cmd
		}

		if m.kanban.IsActive() {
			var cmd tea.Cmd
			m.kanban, cmd = m.kanban.Update(msg)
			if !m.kanban.IsActive() {
				// Check if user toggled a task
				if notePath, line, newDone, ok := m.kanban.GetToggleResult(); ok {
					// Update the source note
					if note := m.vault.GetNote(notePath); note != nil {
						lines := strings.Split(note.Content, "\n")
						if line >= 0 && line < len(lines) {
							if newDone {
								lines[line] = strings.Replace(lines[line], "- [ ]", "- [x]", 1)
							} else {
								lines[line] = strings.Replace(lines[line], "- [x]", "- [ ]", 1)
							}
							newContent := strings.Join(lines, "\n")
							os.WriteFile(filepath.Join(m.vault.Root, notePath), []byte(newContent), 0644)
							m.vault.Scan()
							m.index = vault.NewIndex(m.vault)
							m.index.Build()
							if notePath == m.activeNote {
								m.editor.LoadContent(newContent, m.activeNote)
							}
						}
					}
				}
			}
			return m, cmd
		}

		if m.pomodoro.IsActive() {
			var cmd tea.Cmd
			m.pomodoro, cmd = m.pomodoro.Update(msg)
			return m, cmd
		}

		if m.webClipper.IsActive() {
			var cmd tea.Cmd
			m.webClipper, cmd = m.webClipper.Update(msg)
			if !m.webClipper.IsActive() {
				if title, content, ok := m.webClipper.GetResult(); ok {
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
							m.statusbar.SetMessage("Web clip saved: " + name)
							return m, m.clearMessageAfter(3 * time.Second)
						}
					}
				}
			}
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
			if m.showExitSplash {
				m.quitting = true
				return m, tea.Quit
			}
			return m, m.triggerExitSplash()

		case "ctrl+s":
			cmd := m.saveCurrentNote()
			m.statusbar.SetMessage("Saved " + m.activeNote)
			m.dueTodayCount = CountTasksDueToday(m.vault.Notes)
			m.statusbar.SetDueTodayCount(m.dueTodayCount)
			var toastCmd tea.Cmd
			if m.toast != nil {
				toastCmd = m.toast.ShowSuccess("Saved " + m.activeNote)
			}
			return m, tea.Batch(cmd, m.clearMessageAfter(2*time.Second), toastCmd)

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
			m.templates.OpenWithVault(m.vault.Root)
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

		case "ctrl+k":
			m.taskManager.SetSize(m.width, m.height)
			m.taskManager.Open(m.vault)
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

		case "ctrl+1", "ctrl+2", "ctrl+3", "ctrl+4", "ctrl+5",
			"ctrl+6", "ctrl+7", "ctrl+8", "ctrl+9":
			if m.tabBar != nil {
				// Parse digit from key string (last character)
				keyStr := msg.String()
				digit := int(keyStr[len(keyStr)-1] - '0')
				idx := digit - 1 // 1-indexed to 0-indexed
				if path := m.tabBar.SwitchToIndex(idx); path != "" {
					if path != m.activeNote {
						m.loadNote(path)
						m.sidebar.cursor = m.findFileIndex(path)
					}
				}
			}
			return m, nil

		case "alt+left":
			if m.breadcrumb != nil {
				if nav := m.breadcrumb.Back(); nav != "" {
					m.loadNoteWithoutBreadcrumb(nav)
					m.sidebar.cursor = m.findFileIndex(nav)
				}
			}
			return m, nil

		case "alt+right":
			if m.breadcrumb != nil {
				if nav := m.breadcrumb.Forward(); nav != "" {
					m.loadNoteWithoutBreadcrumb(nav)
					m.sidebar.cursor = m.findFileIndex(nav)
				}
			}
			return m, nil

		case "alt+shift+left":
			// Move active tab left
			if m.tabBar != nil {
				m.tabBar.MoveLeft()
			}
			return m, nil

		case "alt+shift+right":
			// Move active tab right
			if m.tabBar != nil {
				m.tabBar.MoveRight()
			}
			return m, nil

		case "alt+f":
			// Toggle fold at cursor
			if m.activeNote != "" {
				m.foldState.ToggleFold(m.editor.cursor, m.editor.content)
			}
			return m, nil

		case "alt+w":
			// Close active tab
			if m.tabBar != nil && len(m.tabBar.Tabs()) > 1 {
				newActive := m.tabBar.CloseActive()
				if newActive != "" && newActive != m.activeNote {
					m.loadNote(newActive)
					m.sidebar.cursor = m.findFileIndex(newActive)
				}
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
			if keyMsg, ok := msg.(tea.KeyMsg); ok {
				k := keyMsg.String()

				// Link completer intercepts keys when active
				if m.linkCompleter != nil && m.linkCompleter.IsActive() {
					switch k {
					case "esc":
						m.linkCompleter.Deactivate()
						return m, nil
					case "tab", "enter":
						result := m.linkCompleter.Confirm()
						if result != "" {
							// Remove the query text typed after [[ and insert the completed link
							q := m.linkCompleter.GetQuery()
							// Remove query chars already typed
							for range q {
								m.editor.col--
								if m.editor.col >= 0 && m.editor.cursor < len(m.editor.content) {
									line := m.editor.content[m.editor.cursor]
									if m.editor.col < len(line) {
										m.editor.content[m.editor.cursor] = line[:m.editor.col] + line[m.editor.col+1:]
									}
								}
							}
							m.editor.InsertText(result + "]]")
							m.editor.modified = true
						}
						m.linkCompleter.Deactivate()
						return m, nil
					case "up":
						m.linkCompleter.MoveUp()
						return m, nil
					case "down":
						m.linkCompleter.MoveDown()
						return m, nil
					case "backspace":
						if m.linkCompleter.GetQuery() == "" {
							m.linkCompleter.Deactivate()
						} else {
							m.linkCompleter.RemoveChar()
						}
						// Also pass through to editor
						m.editor, cmd = m.editor.Update(msg)
						return m, cmd
					default:
						if len(k) == 1 && k[0] >= 32 {
							m.linkCompleter.AddChar(k)
						}
						// Pass through to editor
						m.editor, cmd = m.editor.Update(msg)
						return m, cmd
					}
				}

				// Slash menu intercepts keys when active
				if m.slashMenu != nil && m.slashMenu.IsActive() {
					queryLen := m.slashMenu.QueryLen()
					insertText, consumed, closed := m.slashMenu.HandleKey(k)
					if insertText != "" {
						// Delete the "/" and query chars that triggered the menu
						if m.editor.cursor < len(m.editor.content) {
							line := m.editor.content[m.editor.cursor]
							// Remove "/" + query chars (slash is at col - queryLen - 1)
							slashCol := m.editor.col - queryLen - 1
							if slashCol < 0 {
								slashCol = 0
							}
							if slashCol < len(line) {
								m.editor.content[m.editor.cursor] = line[:slashCol] + line[m.editor.col:]
								m.editor.col = slashCol
							}
						}
						// Expand placeholders
						expanded := m.snippets.ExpandPlaceholders(insertText)
						m.editor.InsertText(expanded)
						m.editor.modified = true
						line, col := m.editor.GetCursor()
						m.statusbar.SetCursor(line, col)
						m.statusbar.SetWordCount(m.editor.GetWordCount())
						return m, nil
					}
					if closed {
						return m, nil
					}
					if consumed {
						return m, nil
					}
				}

				// Clipboard: Ctrl+C copies selection, Ctrl+V pastes
				if k == "ctrl+v" {
					if text, err := ClipboardPaste(); err == nil && text != "" {
						m.editor.InsertText(text)
						line, col := m.editor.GetCursor()
						m.statusbar.SetCursor(line, col)
						m.statusbar.SetWordCount(m.editor.GetWordCount())
						return m, nil
					}
				}

				// Ghost writer: accept suggestion with Tab
				if k == "tab" {
					if m.ghostWriter != nil && m.ghostWriter.GetSuggestion() != "" {
						suggestion := m.ghostWriter.Accept()
						m.editor.InsertText(suggestion)
						m.editor.SetGhostText("")
						line, col := m.editor.GetCursor()
						m.statusbar.SetCursor(line, col)
						m.statusbar.SetWordCount(m.editor.GetWordCount())
						return m, nil
					}
				}

				// Escape dismisses ghost text
				if k == "esc" && m.ghostWriter != nil && m.ghostWriter.GetSuggestion() != "" {
					m.ghostWriter.Dismiss()
					m.editor.SetGhostText("")
					return m, nil
				}

				// Vim mode handling
				if m.vimState != nil && m.vimState.IsEnabled() {
					result := m.vimState.HandleKey(k, m.editor.content, m.editor.cursor, m.editor.col, m.editor.height)
					// Record keystroke for macro (skip keys that start/stop recording)
					if m.vimState.IsRecording() && !result.MacroStop && result.MacroStart == 0 {
						if keyMsg, ok := msg.(tea.KeyMsg); ok {
							m.vimState.RecordKey(keyMsg)
						}
					}
					cmd = m.applyVimResult(result)
					if !result.PassThrough {
						line, col := m.editor.GetCursor()
						m.statusbar.SetCursor(line, col)
						if m.vimState.IsEnabled() {
							mode := "VIM:" + m.vimState.ModeString()
							if rs := m.vimState.RecordingStatus(); rs != "" {
								mode += " [" + rs + "]"
							}
							m.statusbar.SetMode(mode)
						}
						return m, cmd
					}
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

			// Live backlink preview: update on every cursor move
			m.backlinkPreview.Update(m.editor.content, m.editor.cursor, m.editor.col, func(name string) string {
				// Try with and without .md extension
				if note := m.vault.GetNote(name + ".md"); note != nil {
					return note.Content
				}
				if note := m.vault.GetNote(name); note != nil {
					return note.Content
				}
				return ""
			})

			// Detect [[ for link completion
			if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "[" {
				if m.linkCompleter != nil && !m.linkCompleter.IsActive() {
					// Check if the char before cursor is also [
					curLine := m.editor.content[m.editor.cursor]
					c := m.editor.col
					if c >= 2 && curLine[c-2:c] == "[[" {
						m.linkCompleter.Activate(m.editor.cursor, m.editor.col)
					}
				}
			}

			// Detect "/" at start of line or after space for slash menu
			if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "/" {
				if m.slashMenu != nil && !m.slashMenu.IsActive() {
					curLine := m.editor.content[m.editor.cursor]
					c := m.editor.col
					// "/" is valid if at col 1 (just typed at start) or after a space
					if c == 1 || (c >= 2 && curLine[c-2] == ' ') {
						m.slashMenu.Activate(m.editor.cursor, m.editor.col)
					}
				}
			}

			// Ghost writer: trigger completion on edits
			if m.ghostWriter != nil && m.ghostWriter.IsEnabled() {
				if keyMsg, ok := msg.(tea.KeyMsg); ok {
					k := keyMsg.String()
					// Only trigger on actual text input, not navigation
					if len(k) == 1 || k == "space" || k == "backspace" || k == "enter" {
						m.ghostWriter.Dismiss()
						m.editor.SetGhostText("")
						ghostCmd := m.ghostWriter.OnEdit(m.editor.content, m.editor.cursor, m.editor.col)
						if ghostCmd != nil {
							cmd = tea.Batch(cmd, ghostCmd)
						}
					}
				}
			}

			// Pomodoro: track word count during work sessions
			if m.pomodoro.IsRunning() {
				m.pomodoro.UpdateWordCount(m.editor.GetWordCount())
			}

			// Auto-save: debounce 2 seconds after last edit
			if m.config.AutoSave && m.editor.modified {
				if keyMsg, ok := msg.(tea.KeyMsg); ok {
					k := keyMsg.String()
					if len(k) == 1 || k == "space" || k == "backspace" || k == "enter" || k == "tab" || k == "delete" {
						now := time.Now()
						m.lastEditTime = now
						autoSaveCmd := tea.Tick(2*time.Second, func(time.Time) tea.Msg {
							return autoSaveTickMsg{editTime: now}
						})
						cmd = tea.Batch(cmd, autoSaveCmd)
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
		m.templates.OpenWithVault(m.vault.Root)
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
		m.templates.OpenWithVault(m.vault.Root)
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
	case CmdGlobalReplace:
		m.globalReplace.SetSize(m.width, m.height)
		m.globalReplace.Open(m.vault)
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
	case CmdBlogPublish:
		if !m.config.CorePluginEnabled("blog_publisher") {
			break
		}
		if m.activeNote != "" {
			note := m.vault.GetNote(m.activeNote)
			title := strings.TrimSuffix(filepath.Base(m.activeNote), ".md")
			content := ""
			if note != nil {
				content = note.Content
			}
			m.blogPublisher.SetSize(m.width, m.height)
			m.blogPublisher.PreFill(
				m.config.MediumToken,
				m.config.GitHubToken,
				m.config.GitHubRepo,
				m.config.GitHubBranch,
			)
			m.blogPublisher.SetConfigSave(func(target, mediumToken, ghToken, ghRepo, ghBranch string) {
				if target == "medium" {
					m.config.MediumToken = mediumToken
				} else {
					m.config.GitHubToken = ghToken
					m.config.GitHubRepo = ghRepo
					m.config.GitHubBranch = ghBranch
				}
				m.config.Save()
			})
			m.blogPublisher.Open(title, content)
		} else {
			m.statusbar.SetMessage("Open a note first to publish")
			return m, m.clearMessageAfter(3 * time.Second)
		}
	case CmdSplitPane:
		m.splitPane.SetSize(m.width, m.height)
		m.splitPane.SetNotes(m.vault.SortedPaths())
		m.splitPane.Open()
		// Load current note into the left pane; right pane opens the picker
		if m.activeNote != "" {
			content := strings.Split(m.editor.GetContent(), "\n")
			m.splitPane.SetLeftContent(m.activeNote, content)
		}
	case CmdRunLuaScript:
		m.luaOverlay.SetSize(m.width, m.height)
		m.luaOverlay.Open(m.activeNote, m.editor.GetContent(), nil)
	case CmdFlashcards:
		if !m.config.CorePluginEnabled("flashcards") {
			break
		}
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
		composerContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				composerContents[p] = note.Content
			}
		}
		m.composer.SetNoteContents(composerContents)
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
			if !m.tableEditor.IsActive() {
				m.tableEditor.OpenNew(m.editor.cursor)
			}
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
	case CmdPomodoro:
		if !m.config.CorePluginEnabled("pomodoro") {
			break
		}
		m.pomodoro.SetSize(m.width, m.height)
		m.pomodoro.Open()
	case CmdWebClip:
		m.webClipper.SetSize(m.width, m.height)
		// Prompt user for URL — for now open with empty URL (they type in overlay)
		m.webClipper.Open("")
		return m, tea.Tick(200*time.Millisecond, func(time.Time) tea.Msg {
			return webClipTickMsg{}
		})
	case CmdToggleVim:
		if m.vimState != nil {
			m.vimState.SetEnabled(!m.vimState.IsEnabled())
			m.config.VimMode = m.vimState.IsEnabled()
			m.config.Save()
			if m.vimState.IsEnabled() {
				m.statusbar.SetMode("VIM:NORMAL")
				m.statusbar.SetMessage("Vim mode enabled")
			} else {
				m.statusbar.SetMode("EDIT")
				m.statusbar.SetMessage("Vim mode disabled")
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdMacroRecord:
		if m.vimState != nil && m.vimState.IsEnabled() {
			if m.vimState.IsRecording() {
				m.vimState.StopRecording()
				m.statusbar.SetMode("VIM:" + m.vimState.ModeString())
				m.statusbar.SetMessage("Macro recording stopped")
			} else {
				m.vimState.StartRecording('a')
				mode := "VIM:" + m.vimState.ModeString() + " [" + m.vimState.RecordingStatus() + "]"
				m.statusbar.SetMode(mode)
				m.statusbar.SetMessage("Recording macro into @a (press q to stop)")
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}
		m.statusbar.SetMessage("Vim mode must be enabled for macros")
		return m, m.clearMessageAfter(2 * time.Second)
	case CmdMacroPlay:
		if m.vimState != nil && m.vimState.IsEnabled() {
			reg := m.vimState.LastMacroRegister()
			if reg == 0 {
				reg = 'a'
			}
			keys := m.vimState.GetMacro(reg)
			if keys != nil && len(keys) > 0 {
				m.vimState.SetLastMacroRegister(reg)
				m.vimState.SetPlayingMacro(true)
				return m, func() tea.Msg {
					return vimMacroReplayMsg{keys: keys, idx: 0}
				}
			}
			m.statusbar.SetMessage("No macro recorded in @" + string(reg))
			return m, m.clearMessageAfter(2 * time.Second)
		}
		m.statusbar.SetMessage("Vim mode must be enabled for macros")
		return m, m.clearMessageAfter(2 * time.Second)
	case CmdPinNote:
		if m.activeNote != "" && m.breadcrumb != nil {
			m.breadcrumb.Pin(m.activeNote)
			m.statusbar.SetMessage("Pinned: " + m.activeNote)
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdUnpinNote:
		if m.activeNote != "" && m.breadcrumb != nil {
			m.breadcrumb.Unpin(m.activeNote)
			m.statusbar.SetMessage("Unpinned: " + m.activeNote)
			return m, m.clearMessageAfter(2 * time.Second)
		}
	case CmdNavBack:
		if m.breadcrumb != nil {
			if nav := m.breadcrumb.Back(); nav != "" {
				m.loadNoteWithoutBreadcrumb(nav)
				m.sidebar.cursor = m.findFileIndex(nav)
			}
		}
	case CmdNavForward:
		if m.breadcrumb != nil {
			if nav := m.breadcrumb.Forward(); nav != "" {
				m.loadNoteWithoutBreadcrumb(nav)
				m.sidebar.cursor = m.findFileIndex(nav)
			}
		}
	case CmdKanban:
		m.kanban.SetSize(m.width, m.height)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		m.kanban.SetTasks(noteContents)
		m.kanban.Open()
	case CmdZettelNote:
		if m.zettelkasten != nil {
			name := m.zettelkasten.GenerateNoteName("Untitled")
			title := strings.TrimSuffix(name, ".md")
			content := m.zettelkasten.GenerateTemplate(title)
			path := filepath.Join(m.vault.Root, name)
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
				m.statusbar.SetMessage("Created Zettelkasten note: " + name)
				return m, m.clearMessageAfter(3 * time.Second)
			}
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
	case CmdVaultRefactor:
		m.vaultRefactor.SetSize(m.width, m.height)
		m.vaultRefactor.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
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
		m.vaultRefactor.SetVaultData(noteContents, tagMap, m.vault.SortedPaths())
		m.vaultRefactor.Open()

	case CmdDailyBriefing:
		m.dailyBriefing.SetSize(m.width, m.height)
		m.dailyBriefing.SetConfig(m.config.AIProvider, m.getAIModel(), m.config.OllamaURL, m.config.OpenAIKey)
		noteContents := make(map[string]string)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				noteContents[p] = note.Content
			}
		}
		today := time.Now().Format("2006-01-02")
		todayPath := today + ".md"
		m.dailyBriefing.SetVaultData(noteContents, m.vault.SortedPaths(), todayPath)
		m.dailyBriefing.Open()

	case CmdEncryptNote:
		if m.activeNote != "" {
			m.encryption.SetSize(m.width, m.height)
			m.encryption.Open()
		}

	case CmdGitHistory:
		if m.activeNote != "" {
			m.gitHistory.SetSize(m.width, m.height)
			cmd := m.gitHistory.Open(m.activeNote, m.vault.Root)
			return m, cmd
		}

	case CmdWorkspaces:
		m.workspace.SetSize(m.width, m.height)
		m.workspace.Open()

	case CmdTimeline:
		m.timeline.SetSize(m.width, m.height)
		notes := make(map[string]TimelineEntry)
		for _, p := range m.vault.SortedPaths() {
			if note := m.vault.GetNote(p); note != nil {
				title := strings.TrimSuffix(filepath.Base(p), ".md")
				var tags []string
				if t, ok := note.Frontmatter["tags"]; ok {
					if tagList, ok := t.([]interface{}); ok {
						for _, tg := range tagList {
							if s, ok := tg.(string); ok {
								tags = append(tags, s)
							}
						}
					}
				}
				preview := note.Content
				// Strip frontmatter for preview
				if strings.HasPrefix(preview, "---") {
					if idx := strings.Index(preview[3:], "---"); idx >= 0 {
						preview = strings.TrimSpace(preview[3+idx+3:])
					}
				}
				if len(preview) > 80 {
					preview = preview[:80]
				}
				// Remove newlines from preview
				preview = strings.ReplaceAll(preview, "\n", " ")
				notes[p] = TimelineEntry{
					Path:    p,
					Title:   title,
					Date:    note.ModTime,
					Tags:    tags,
					Preview: preview,
				}
			}
		}
		m.timeline.Open(notes)

	case CmdVaultSwitch:
		m.vaultSwitch.SetSize(m.width, m.height)
		m.vaultSwitch.Open()

	case CmdFrontmatterEdit:
		if m.activeNote != "" {
			m.frontmatterEdit.SetSize(m.width, m.height)
			m.frontmatterEdit.Open(m.editor.GetContent())
		}

	case CmdFoldToggle:
		if m.activeNote != "" {
			m.foldState.ToggleFold(m.editor.cursor, m.editor.content)
		}

	case CmdFoldAll:
		if m.activeNote != "" {
			m.foldState.FoldAll(m.editor.content)
		}

	case CmdUnfoldAll:
		m.foldState.UnfoldAll()

	case CmdTaskManager:
		if !m.config.CorePluginEnabled("task_manager") {
			break
		}
		m.taskManager.SetSize(m.width, m.height)
		m.taskManager.Open(m.vault)

	case CmdLinkAssist:
		if m.activeNote != "" {
			m.linkAssist.SetSize(m.width, m.height)
			m.linkAssist.Open(m.editor.GetContent(), m.vault.SortedPaths(), m.activeNote)
		}

	case CmdImageManager:
		m.imageManager.SetSize(m.width, m.height)
		m.imageManager.Open(m.vault.Root)

	case CmdThemeEditor:
		m.themeEditor.SetSize(m.width, m.height)
		m.themeEditor.Open(m.config.Theme)

	case CmdLayoutDefault, CmdLayoutWriter, CmdLayoutMinimal, CmdLayoutReading, CmdLayoutDashboard:
		switch action {
		case CmdLayoutDefault:
			m.config.Layout = LayoutDefault
			m.statusbar.SetMessage("Layout: Default (3-panel)")
		case CmdLayoutWriter:
			m.config.Layout = LayoutWriter
			m.statusbar.SetMessage("Layout: Writer (2-panel)")
		case CmdLayoutMinimal:
			m.config.Layout = LayoutMinimal
			m.statusbar.SetMessage("Layout: Minimal (editor only)")
		case CmdLayoutReading:
			m.config.Layout = LayoutReading
			m.statusbar.SetMessage("Layout: Reading (editor + backlinks)")
		case CmdLayoutDashboard:
			m.config.Layout = LayoutDashboard
			m.statusbar.SetMessage("Layout: Dashboard (4-panel)")
		}
		// Fix focus if current panel is hidden in new layout
		if !LayoutHasSidebar(m.config.Layout) && m.focus == focusSidebar {
			m.setFocus(focusEditor)
		}
		if !LayoutHasBacklinks(m.config.Layout) && m.focus == focusBacklinks {
			m.setFocus(focusEditor)
		}
		m.updateLayout()

	case CmdResearchAgent:
		if !m.config.CorePluginEnabled("research_agent") {
			break
		}
		m.research.SetSize(m.width, m.height)
		if m.research.IsRunning() {
			// Reopen to show progress
			m.research.Reopen()
		} else {
			m.research.Open(m.vault.Root)
		}

	case CmdResearchFollowUp:
		if m.activeNote != "" && !m.research.IsRunning() {
			m.research.SetSize(m.width, m.height)
			m.research.OpenFollowUp(m.vault.Root, m.activeNote, m.editor.GetContent())
		}

	case CmdAITemplate:
		if !m.config.CorePluginEnabled("ai_templates") {
			break
		}
		m.aiTemplates.SetSize(m.width, m.height)
		m.aiTemplates.Open(m.config.AIProvider, m.config.OllamaModel, m.config.OllamaURL, m.config.OpenAIKey)

	case CmdVaultAnalyzer:
		if !m.research.IsRunning() {
			m.research.SetSize(m.width, m.height)
			m.research.OpenVaultAnalyzer(m.vault.Root, m.vault.SortedPaths())
		}

	case CmdNoteEnhancer:
		if m.activeNote != "" && !m.research.IsRunning() {
			m.research.SetSize(m.width, m.height)
			m.research.OpenNoteEnhancer(m.vault.Root, m.activeNote, m.editor.GetContent(), m.vault.SortedPaths())
		}

	case CmdDailyDigest:
		if !m.research.IsRunning() {
			m.research.SetSize(m.width, m.height)
			recentNotes := make(map[string]string)
			cutoff := time.Now().AddDate(0, 0, -7)
			for _, p := range m.vault.SortedPaths() {
				note := m.vault.GetNote(p)
				if note != nil && note.ModTime.After(cutoff) {
					recentNotes[p] = note.Content
				}
			}
			m.research.OpenDailyDigest(m.vault.Root, recentNotes)
		}

	case CmdLanguageLearning:
		if m.config.CorePluginEnabled("language_learning") {
			m.languageLearning.SetSize(m.width, m.height)
			m.languageLearning.Open(m.vault.Root)
		}

	case CmdHabitTracker:
		if m.config.CorePluginEnabled("habit_tracker") {
			m.habitTracker.SetSize(m.width, m.height)
			m.habitTracker.Open(m.vault.Root)
		}

	case CmdFocusSession:
		m.focusSession.SetSize(m.width, m.height)
		m.focusSession.Open(m.vault.Root)

	case CmdStandupGenerator:
		m.standupGen.SetSize(m.width, m.height)
		m.standupGen.Open(m.vault.Root)

	case CmdNoteHistory:
		m.noteHistory.SetSize(m.width, m.height)
		m.noteHistory.OpenForNote(m.vault.Root, m.activeNote)

	case CmdSmartConnections:
		m.smartConnect.SetSize(m.width, m.height)
		content := ""
		if n, ok := m.vault.Notes[m.activeNote]; ok {
			content = n.Content
		}
		m.smartConnect.OpenForNote(m.vault.Root, m.activeNote, content)

	case CmdWritingStats:
		m.writingStats.SetSize(m.width, m.height)
		m.writingStats.Open(m.vault.Root)

	case CmdQuickCapture:
		m.quickCapture.SetSize(m.width, m.height)
		m.quickCapture.Open(m.vault.Root)

	case CmdDashboard:
		m.dashboard.SetSize(m.width, m.height)
		m.dashboard.Open(m.vault.Root)

	case CmdMindMap:
		m.mindMap.SetSize(m.width, m.height)
		content := m.editor.GetContent()
		m.mindMap.OpenForNote(m.vault.Root, m.activeNote, content)

	case CmdJournalPrompts:
		m.journalPrompts.SetSize(m.width, m.height)
		m.journalPrompts.Open(m.vault.Root)

	case CmdClipManager:
		m.clipManager.SetSize(m.width, m.height)
		m.clipManager.Open()

	case CmdDailyPlanner:
		m.dailyPlanner.SetSize(m.width, m.height)
		tasks, events, habits := m.gatherPlannerData()
		m.dailyPlanner.Open(m.vault.Root, tasks, events, habits)

	case CmdAIScheduler:
		m.aiScheduler.SetSize(m.width, m.height)
		tasks, events := m.gatherSchedulerData()
		m.aiScheduler.Open(m.vault.Root, tasks, events,
			m.config.AIProvider, m.config.OllamaURL, m.config.OllamaModel,
			m.config.OpenAIKey, m.config.OpenAIModel)

	case CmdRecurringTasks:
		m.recurringTasks.SetSize(m.width, m.height)
		m.recurringTasks.Open(m.vault.Root)

	case CmdNotePreview:
		if m.activeNote != "" {
			if n, ok := m.vault.Notes[m.activeNote]; ok {
				m.notePreview.SetSize(m.width, m.height)
				m.notePreview.Open(m.activeNote, m.activeNote, n.Content)
			}
		}

	case CmdScratchpad:
		m.scratchpad.SetSize(m.width, m.height)
		m.scratchpad.Open(m.vault.Root)

	case CmdProjectMode:
		m.projectMode.SetSize(m.width, m.height)
		m.projectMode.Open(m.vault.Root)

	case CmdNLSearch:
		m.nlSearch.SetSize(m.width, m.height)
		m.nlSearch.Open(m.vault.Root,
			m.config.AIProvider, m.config.OllamaURL, m.config.OllamaModel,
			m.config.OpenAIKey, m.config.OpenAIModel)

	case CmdWritingCoach:
		m.writingCoach.SetSize(m.width, m.height)
		content := m.editor.GetContent()
		m.writingCoach.Open(m.vault.Root, content, m.activeNote,
			m.config.AIProvider, m.config.OllamaURL, m.config.OllamaModel,
			m.config.OpenAIKey, m.config.OpenAIModel)

	case CmdDataview:
		m.dataview.SetSize(m.width, m.height)
		m.dataview.Open(m.vault.Root)

	case CmdTimeTracker:
		m.timeTracker.SetSize(m.width, m.height)
		m.timeTracker.Open(m.vault.Root)

	case CmdBackup:
		m.backup.SetSize(m.width, m.height)
		m.backup.Open(m.vault.Root)

	case CmdShowTutorial:
		m.onboarding.SetSize(m.width, m.height)
		m.onboarding.Open()

	case CmdCloseOtherTabs:
		if m.tabBar != nil {
			m.tabBar.CloseOthers()
			m.statusbar.SetMessage("Closed other tabs")
			return m, m.clearMessageAfter(2 * time.Second)
		}

	case CmdCloseTabsToRight:
		if m.tabBar != nil {
			m.tabBar.CloseToRight()
			m.statusbar.SetMessage("Closed tabs to the right")
			return m, m.clearMessageAfter(2 * time.Second)
		}

	case CmdTogglePinTab:
		if m.tabBar != nil && m.activeNote != "" {
			m.tabBar.TogglePin()
			if m.tabBar.IsActiveTabPinned() {
				m.statusbar.SetMessage("Pinned tab: " + m.activeNote)
			} else {
				m.statusbar.SetMessage("Unpinned tab: " + m.activeNote)
			}
			return m, m.clearMessageAfter(2 * time.Second)
		}

	case CmdReopenClosedTab:
		if m.tabBar != nil {
			if path := m.tabBar.ReopenLast(); path != "" {
				if m.vault.GetNote(path) != nil {
					m.tabBar.AddTab(path)
					m.loadNote(path)
					m.sidebar.cursor = m.findFileIndex(path)
					m.statusbar.SetMessage("Reopened: " + path)
					return m, m.clearMessageAfter(2 * time.Second)
				}
			} else {
				m.statusbar.SetMessage("No closed tabs to reopen")
				return m, m.clearMessageAfter(2 * time.Second)
			}
		}

	case CmdQuit:
		return m, m.triggerExitSplash()
	}
	return m, nil
}

// gatherPlannerData collects tasks, events, and habits for the daily planner.
func (m *Model) gatherPlannerData() ([]PlannerTask, []PlannerEvent, []PlannerHabit) {
	today := time.Now().Format("2006-01-02")
	var tasks []PlannerTask
	var events []PlannerEvent
	var habits []PlannerHabit

	// Scan Tasks.md for tasks due today
	tasksPath := filepath.Join(m.vault.Root, "Tasks.md")
	if f, err := os.Open(tasksPath); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			if taskPattern.MatchString(line) {
				m2 := taskPattern.FindStringSubmatch(line)
				done := m2[1] != " "
				text := m2[2]
				dueDate := ""
				if dm := regexp.MustCompile(`📅\s*(\d{4}-\d{2}-\d{2})`).FindStringSubmatch(text); dm != nil {
					dueDate = dm[1]
				}
				if dueDate == today || (dueDate != "" && dueDate <= today && !done) {
					tasks = append(tasks, PlannerTask{
						Text:     text,
						Done:     done,
						Priority: taskPriority(text),
						DueDate:  dueDate,
						Source:   "Tasks.md",
					})
				}
			}
		}
		f.Close()
	}

	// Scan calendar events for today
	icsDir := filepath.Join(m.vault.Root, ".granit", "calendars")
	if entries, err := os.ReadDir(icsDir); err == nil {
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".ics") {
				continue
			}
			calEvents, err := ParseICSFile(filepath.Join(icsDir, e.Name()))
			if err != nil {
				continue
			}
			for _, ev := range calEvents {
				if ev.Date.Format("2006-01-02") == today {
					dur := 60
					if !ev.EndDate.IsZero() {
						dur = int(ev.EndDate.Sub(ev.Date).Minutes())
					}
					timeStr := ""
					if !ev.AllDay {
						timeStr = ev.Date.Format("15:04")
					}
					events = append(events, PlannerEvent{
						Title:    ev.Title,
						Time:     timeStr,
						Duration: dur,
					})
				}
			}
		}
	}

	// Scan habits
	habitsDir := filepath.Join(m.vault.Root, "Habits")
	if entries, err := os.ReadDir(habitsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || e.Name() == "goals.md" || e.Name() == "stats.md" {
				continue
			}
			name := strings.TrimSuffix(e.Name(), ".md")
			habits = append(habits, PlannerHabit{Name: name})
		}
	}

	return tasks, events, habits
}

// gatherSchedulerData collects tasks and events for the AI scheduler.
func (m *Model) gatherSchedulerData() ([]SchedulerTask, []SchedulerEvent) {
	plannerTasks, plannerEvents, _ := m.gatherPlannerData()

	var tasks []SchedulerTask
	for _, t := range plannerTasks {
		tasks = append(tasks, SchedulerTask{
			Text:     t.Text,
			Priority: t.Priority,
			DueDate:  t.DueDate,
			Done:     t.Done,
		})
	}

	var events []SchedulerEvent
	for _, e := range plannerEvents {
		dur := e.Duration
		if dur <= 0 {
			dur = 60
		}
		events = append(events, SchedulerEvent{
			Title:    e.Title,
			Time:     e.Time,
			Duration: dur,
		})
	}

	return tasks, events
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

	// Rank results: exact filename > prefix > fuzzy filename > content match
	var exactMatches, prefixMatches, fuzzyMatches, contentMatches []string
	seen := make(map[string]bool)

	for _, path := range m.vault.SortedPaths() {
		lowerPath := strings.ToLower(path)
		baseName := strings.ToLower(strings.TrimSuffix(filepath.Base(path), ".md"))

		if baseName == query {
			exactMatches = append(exactMatches, path)
			seen[path] = true
		} else if strings.HasPrefix(baseName, query) {
			prefixMatches = append(prefixMatches, path)
			seen[path] = true
		} else if fuzzyMatch(lowerPath, query) {
			fuzzyMatches = append(fuzzyMatches, path)
			seen[path] = true
		}
	}

	for _, path := range m.vault.SortedPaths() {
		if seen[path] {
			continue
		}
		note := m.vault.GetNote(path)
		if note != nil && strings.Contains(strings.ToLower(note.Content), query) {
			contentMatches = append(contentMatches, path)
		}
	}

	m.searchResults = append(m.searchResults, exactMatches...)
	m.searchResults = append(m.searchResults, prefixMatches...)
	m.searchResults = append(m.searchResults, fuzzyMatches...)
	m.searchResults = append(m.searchResults, contentMatches...)

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
		} else if m.vimState != nil && m.vimState.IsEnabled() {
			mode := "VIM:" + m.vimState.ModeString()
			if rs := m.vimState.RecordingStatus(); rs != "" {
				mode += " [" + rs + "]"
			}
			m.statusbar.SetMode(mode)
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
		// Medium — drop backlinks/sidebar when space is tight
		if layout == "default" {
			layout = "writer"
		} else if layout == "reading" {
			layout = "minimal"
		} else if layout == "dashboard" {
			layout = "writer"
		} else if layout == "taskboard" || layout == "research" {
			layout = "writer"
		}
	} else if m.width < 160 {
		// Not wide enough for 4 panels — fall back to default 3-panel
		if layout == "dashboard" {
			layout = "default"
		}
	}

	showSidebar := LayoutHasSidebar(layout)
	showBacklinks := LayoutHasBacklinks(layout)
	showOutline := LayoutHasOutline(layout)

	sidebarWidth := 0
	backlinksWidth := 0
	outlineWidth := 0
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
	if showOutline {
		outlineWidth = m.width / 7
		if outlineWidth < 18 {
			outlineWidth = 18
		}
		if outlineWidth > 25 {
			outlineWidth = 25
		}
	}

	panelBorders := 0
	if showSidebar {
		panelBorders += 2
	}
	if showBacklinks {
		panelBorders += 2
	}
	if showOutline {
		panelBorders += 2
	}
	editorWidth := m.width - sidebarWidth - backlinksWidth - outlineWidth - panelBorders - 2

	// Taskboard and research layouts have an extra right panel
	if layout == "taskboard" || layout == "research" {
		extraPanelWidth := m.width / 4
		if extraPanelWidth < 25 {
			extraPanelWidth = 25
		}
		if extraPanelWidth > 40 {
			extraPanelWidth = 40
		}
		editorWidth = m.width - sidebarWidth - extraPanelWidth - 6
	}
	if editorWidth < 30 {
		editorWidth = 30
	}

	// Content height: terminal minus status bar (2 lines) minus panel borders (2 lines)
	contentHeight := m.height - 4
	if m.breadcrumb != nil && (len(m.breadcrumb.Pinned()) > 0 || m.breadcrumb.CanGoBack()) {
		contentHeight-- // breadcrumb nav bar between content and status
	}
	if contentHeight < 6 {
		contentHeight = 6
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
	m.pomodoro.SetSize(m.width, m.height)
	m.webClipper.SetSize(m.width, m.height)
	if m.toast != nil {
		m.toast.SetWidth(m.width)
	}
	m.kanban.SetSize(m.width, m.height)
	m.encryption.SetSize(m.width, m.height)
	m.backlinkPreview.SetSize(m.width, m.height)
	m.gitHistory.SetSize(m.width, m.height)
	m.workspace.SetSize(m.width, m.height)
	m.timeline.SetSize(m.width, m.height)
	m.vaultSwitch.SetSize(m.width, m.height)
	m.frontmatterEdit.SetSize(m.width, m.height)
	m.research.SetSize(m.width, m.height)
	m.imageManager.SetSize(m.width, m.height)
	m.themeEditor.SetSize(m.width, m.height)
	m.linkAssist.SetSize(m.width, m.height)
	m.taskManager.SetSize(m.width, m.height)
	m.blogPublisher.SetSize(m.width, m.height)
	m.backup.SetSize(m.width, m.height)
	m.onboarding.SetSize(m.width, m.height)
}

// refreshComponents re-scans the vault and updates all dependent components
// after any file has been modified by an overlay. If changedPath is non-empty
// and matches the currently open note, the editor is reloaded too.
func (m *Model) refreshComponents(changedPath string) {
	m.vault.Scan()
	m.index.Build()
	paths := m.vault.SortedPaths()
	m.sidebar.SetFiles(paths)
	m.autocomplete.SetNotes(paths)
	m.statusbar.SetNoteCount(m.vault.NoteCount())

	// Update due-today count
	m.dueTodayCount = CountTasksDueToday(m.vault.Notes)
	m.statusbar.SetDueTodayCount(m.dueTodayCount)

	// Update calendar daily notes and note contents
	m.calendar.SetDailyNotes(paths)
	noteContents := make(map[string]string)
	for _, p := range paths {
		if note := m.vault.GetNote(p); note != nil {
			noteContents[p] = note.Content
		}
	}
	m.calendar.SetNoteContents(noteContents)

	// If the changed file is currently open in the editor, reload it
	if changedPath != "" && changedPath == m.activeNote {
		if note := m.vault.GetNote(changedPath); note != nil {
			m.editor.LoadContent(note.Content, m.editor.filePath)
		}
	}

	m.needsRefresh = true
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
	if m.vimState != nil {
		m.vimState.SetEnabled(m.config.VimMode)
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

// loadNoteWithoutBreadcrumb loads a note without pushing to breadcrumb history.
// Used by back/forward navigation to avoid corrupting the navigation stack.
func (m *Model) loadNoteWithoutBreadcrumb(relPath string) {
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

func (m *Model) applyVimResult(r VimResult) tea.Cmd {
	if r.CursorSet {
		m.editor.cursor = r.NewCursor
		m.editor.col = r.NewCol
		// Clamp
		if m.editor.cursor < 0 {
			m.editor.cursor = 0
		}
		if m.editor.cursor >= len(m.editor.content) {
			m.editor.cursor = len(m.editor.content) - 1
		}
		if m.editor.col < 0 {
			m.editor.col = 0
		}
		if m.editor.cursor >= 0 && m.editor.cursor < len(m.editor.content) {
			if m.editor.col > len(m.editor.content[m.editor.cursor]) {
				m.editor.col = len(m.editor.content[m.editor.cursor])
			}
		}
	}
	if r.ScrollSet {
		m.editor.scroll = r.ScrollTo
	}
	if r.DeleteLine {
		if len(m.editor.content) > 1 {
			m.editor.saveSnapshot()
			m.vimState.register = m.editor.content[m.editor.cursor]
			m.editor.content = append(m.editor.content[:m.editor.cursor], m.editor.content[m.editor.cursor+1:]...)
			if m.editor.cursor >= len(m.editor.content) {
				m.editor.cursor = len(m.editor.content) - 1
			}
			m.editor.col = 0
			m.editor.modified = true
		}
	}
	if r.JoinLine {
		if m.editor.cursor < len(m.editor.content)-1 {
			m.editor.saveSnapshot()
			m.editor.content[m.editor.cursor] += " " + strings.TrimSpace(m.editor.content[m.editor.cursor+1])
			m.editor.content = append(m.editor.content[:m.editor.cursor+1], m.editor.content[m.editor.cursor+2:]...)
			m.editor.modified = true
		}
	}
	if r.PasteBelow && m.vimState.register != "" {
		m.editor.saveSnapshot()
		newContent := make([]string, 0, len(m.editor.content)+1)
		newContent = append(newContent, m.editor.content[:m.editor.cursor+1]...)
		newContent = append(newContent, m.vimState.register)
		newContent = append(newContent, m.editor.content[m.editor.cursor+1:]...)
		m.editor.content = newContent
		m.editor.cursor++
		m.editor.col = 0
		m.editor.modified = true
	}
	if r.PasteAbove && m.vimState.register != "" {
		m.editor.saveSnapshot()
		newContent := make([]string, 0, len(m.editor.content)+1)
		newContent = append(newContent, m.editor.content[:m.editor.cursor]...)
		newContent = append(newContent, m.vimState.register)
		newContent = append(newContent, m.editor.content[m.editor.cursor:]...)
		m.editor.col = 0
		m.editor.modified = true
	}
	if r.Undo {
		m.editor.Undo()
	}
	if r.Redo {
		m.editor.Redo()
	}
	if r.InsertLine != "" {
		m.editor.saveSnapshot()
		newContent := make([]string, 0, len(m.editor.content)+1)
		newContent = append(newContent, m.editor.content[:m.editor.cursor+1]...)
		newContent = append(newContent, r.InsertLine)
		newContent = append(newContent, m.editor.content[m.editor.cursor+1:]...)
		m.editor.content = newContent
		m.editor.cursor++
		m.editor.col = 0
		m.editor.modified = true
	}
	if r.StatusMsg != "" {
		switch r.StatusMsg {
		case "save":
			return m.saveCurrentNote()
		case "quit":
			return m.triggerExitSplash()
		case "save_quit":
			m.saveCurrentNote()()
			return m.triggerExitSplash()
		default:
			m.statusbar.SetMessage(r.StatusMsg)
		}
	}
	if r.EnterInsert {
		mode := "VIM:INSERT"
		if rs := m.vimState.RecordingStatus(); rs != "" {
			mode += " [" + rs + "]"
		}
		m.statusbar.SetMode(mode)
	}
	if r.EnterNormal {
		mode := "VIM:NORMAL"
		if rs := m.vimState.RecordingStatus(); rs != "" {
			mode += " [" + rs + "]"
		}
		m.statusbar.SetMode(mode)
	}
	if r.EnterVisual {
		mode := "VIM:VISUAL"
		if rs := m.vimState.RecordingStatus(); rs != "" {
			mode += " [" + rs + "]"
		}
		m.statusbar.SetMode(mode)
	}
	if r.FoldToggle {
		m.foldState.ToggleFold(m.editor.cursor, m.editor.content)
	}
	if r.FoldAll {
		m.foldState.FoldAll(m.editor.content)
	}
	if r.UnfoldAll {
		m.foldState.UnfoldAll()
	}

	// Macro recording start — update status bar
	if r.MacroStart != 0 {
		mode := "VIM:" + m.vimState.ModeString() + " [" + m.vimState.RecordingStatus() + "]"
		m.statusbar.SetMode(mode)
	}

	// Macro recording stop — update status bar
	if r.MacroStop {
		m.statusbar.SetMode("VIM:" + m.vimState.ModeString())
	}

	// Macro replay
	if r.MacroReplay != 0 {
		if !m.vimState.IsPlayingMacro() {
			keys := m.vimState.GetMacro(r.MacroReplay)
			if keys != nil && len(keys) > 0 {
				m.vimState.SetLastMacroRegister(r.MacroReplay)
				m.vimState.SetPlayingMacro(true)
				return func() tea.Msg {
					return vimMacroReplayMsg{keys: keys, idx: 0}
				}
			}
			m.statusbar.SetMessage("macro @" + string(r.MacroReplay) + " is empty")
		}
	}

	return nil
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

func (m *Model) triggerExitSplash() tea.Cmd {
	// Save open tabs for session persistence
	if m.tabBar != nil {
		m.tabBar.SaveTabs(m.vault.Root)
	}
	// Unload Ollama model to free resources
	if m.config.AIProvider == "ollama" {
		stopOllama(m.config.OllamaModel)
	}
	m.showExitSplash = true
	m.exitSplash = NewExitSplash(m.vault.NoteCount(), time.Since(m.sessionStart))
	m.exitSplash.width = m.width
	m.exitSplash.height = m.height
	return m.exitSplash.Init()
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

	// Exit splash screen
	if m.showExitSplash {
		return m.exitSplash.View()
	}

	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return lipgloss.NewStyle().Foreground(mauve).Render("\n  Loading Granit...")
	}

	// Content height: terminal minus status bar (2 lines) minus panel borders (2 lines)
	contentHeight := m.height - 4
	if m.breadcrumb != nil && (len(m.breadcrumb.Pinned()) > 0 || m.breadcrumb.CanGoBack()) {
		contentHeight-- // breadcrumb nav bar between content and status
	}
	if contentHeight < 6 {
		contentHeight = 6
	}
	layout := m.config.Layout
	if layout == "" {
		layout = "default"
	}

	// Calculate widths based on layout
	showSidebar := LayoutHasSidebar(layout)
	showBacklinks := LayoutHasBacklinks(layout)
	showOutline := LayoutHasOutline(layout)

	sidebarWidth := 0
	backlinksWidth := 0
	outlineWidth := 0

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
	if showOutline {
		outlineWidth = m.width / 7
		if outlineWidth < 18 {
			outlineWidth = 18
		}
		if outlineWidth > 25 {
			outlineWidth = 25
		}
	}

	panelBorders := 0
	if showSidebar {
		panelBorders += 2
	}
	if showBacklinks {
		panelBorders += 2
	}
	if showOutline {
		panelBorders += 2
	}
	editorWidth := m.width - sidebarWidth - backlinksWidth - outlineWidth - panelBorders - 2
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

	// Tab bar
	var tabBarStr string
	if m.tabBar != nil && len(m.tabBar.Tabs()) > 0 {
		m.tabBar.SetModified(m.activeNote, m.editor.modified)
		tabBarStr = m.tabBar.Render(editorWidth, m.activeNote)
	}

	// Editor: view mode or edit mode
	var editorContent string
	if m.viewMode {
		editorContent = m.renderViewMode()
	} else {
		editorContent = m.editor.View()
	}

	// Folder-path breadcrumb (between tab bar and editor)
	breadcrumbStr := renderBreadcrumb(m.activeNote, editorWidth)

	// Zen layout hides tab bar and breadcrumb for distraction-free writing
	if layout == "zen" {
		tabBarStr = ""
		breadcrumbStr = ""
	}

	// Combine tab bar + breadcrumb + editor
	editorPanel := editorContent
	if tabBarStr != "" && breadcrumbStr != "" {
		editorPanel = tabBarStr + "\n" + breadcrumbStr + "\n" + editorContent
	} else if tabBarStr != "" {
		editorPanel = tabBarStr + "\n" + editorContent
	} else if breadcrumbStr != "" {
		editorPanel = breadcrumbStr + "\n" + editorContent
	}

	editor := EditorStyle.Copy().
		BorderForeground(editorBorderColor).
		Width(editorWidth).
		Height(contentHeight).
		Render(editorPanel)

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
		case "reading":
			backlinks := BacklinksStyle.Copy().
				BorderForeground(backlinksBorderColor).
				Width(backlinksWidth).
				Height(contentHeight).
				Render(m.backlinks.View())
			content = lipgloss.JoinHorizontal(lipgloss.Top, editor, backlinks)
		case "zen":
			// Centered editor with constrained width, no borders
			maxContentWidth := 82
			if editorWidth > maxContentWidth {
				editorWidth = maxContentWidth
			}
			// Re-render editor panel with constrained width, no border
			zenEditor := lipgloss.NewStyle().
				Width(editorWidth).
				Height(contentHeight).
				Background(base).
				Padding(0, 1).
				Render(editorPanel)
			// Center it horizontally
			leftPad := (m.width - editorWidth - 2) / 2
			if leftPad < 0 {
				leftPad = 0
			}
			content = lipgloss.NewStyle().
				PaddingLeft(leftPad).
				Height(contentHeight + 2).
				Render(zenEditor)
		case "dashboard": // 4-panel: sidebar | editor | outline | backlinks
			sidebar := SidebarStyle.Copy().
				BorderForeground(sidebarBorderColor).
				Width(sidebarWidth).
				Height(contentHeight).
				Render(m.sidebar.View())

			outlinePanelContent := m.outline.RenderPanel(m.editor.GetContent(), outlineWidth, contentHeight)
			outlinePanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(outlineWidth).
				Height(contentHeight).
				Render(outlinePanelContent)

			backlinks := BacklinksStyle.Copy().
				BorderForeground(backlinksBorderColor).
				Width(backlinksWidth).
				Height(contentHeight).
				Render(m.backlinks.View())

			if m.config.SidebarPosition == "right" {
				content = lipgloss.JoinHorizontal(lipgloss.Top, backlinks, outlinePanel, editor, sidebar)
			} else {
				content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, editor, outlinePanel, backlinks)
			}
		case "taskboard":
			sidebar := SidebarStyle.Copy().
				BorderForeground(sidebarBorderColor).
				Width(sidebarWidth).
				Height(contentHeight).
				Render(m.sidebar.View())

			// Task summary panel
			taskPanelWidth := m.width / 4
			if taskPanelWidth < 25 {
				taskPanelWidth = 25
			}
			if taskPanelWidth > 40 {
				taskPanelWidth = 40
			}

			var taskContent strings.Builder
			taskContent.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  TASKS"))
			taskContent.WriteString("\n")
			taskContent.WriteString(DimStyle.Render(strings.Repeat("\u2500", taskPanelWidth-4)))
			taskContent.WriteString("\n\n")

			// Read Tasks.md
			today := time.Now().Format("2006-01-02")
			tasksPath := filepath.Join(m.vault.Root, "Tasks.md")
			taskLines := []string{}
			if data, err := os.ReadFile(tasksPath); err == nil {
				taskLines = strings.Split(string(data), "\n")
			}

			// Show overdue and today's tasks
			overdueCount := 0
			todayCount := 0
			upcomingCount := 0

			taskContent.WriteString(lipgloss.NewStyle().Foreground(red).Bold(true).Render("  Overdue") + "\n")
			for _, line := range taskLines {
				trimmed := strings.TrimSpace(line)
				if !strings.HasPrefix(trimmed, "- [ ]") {
					continue
				}
				if idx := strings.Index(trimmed, "\U0001f4c5 "); idx >= 0 {
					dateStr := trimmed[idx+len("\U0001f4c5 "):]
					if len(dateStr) >= 10 {
						dueDate := dateStr[:10]
						taskText := strings.TrimSpace(trimmed[5:])
						if eIdx := strings.Index(taskText, " \U0001f4c5"); eIdx >= 0 {
							taskText = taskText[:eIdx]
						}
						if len(taskText) > taskPanelWidth-8 {
							taskText = taskText[:taskPanelWidth-11] + "..."
						}
						if dueDate < today {
							overdueCount++
							taskContent.WriteString("  " + lipgloss.NewStyle().Foreground(red).Render("\u2717 "+taskText) + "\n")
						} else if dueDate == today {
							todayCount++
						} else {
							upcomingCount++
						}
					}
				}
			}
			if overdueCount == 0 {
				taskContent.WriteString("  " + DimStyle.Render("none") + "\n")
			}

			taskContent.WriteString("\n" + lipgloss.NewStyle().Foreground(yellow).Bold(true).Render("  Today") + "\n")
			for _, line := range taskLines {
				trimmed := strings.TrimSpace(line)
				if !strings.HasPrefix(trimmed, "- [ ]") {
					continue
				}
				if idx := strings.Index(trimmed, "\U0001f4c5 "); idx >= 0 {
					dateStr := trimmed[idx+len("\U0001f4c5 "):]
					if len(dateStr) >= 10 {
						dueDate := dateStr[:10]
						taskText := strings.TrimSpace(trimmed[5:])
						if eIdx := strings.Index(taskText, " \U0001f4c5"); eIdx >= 0 {
							taskText = taskText[:eIdx]
						}
						if len(taskText) > taskPanelWidth-8 {
							taskText = taskText[:taskPanelWidth-11] + "..."
						}
						if dueDate == today {
							taskContent.WriteString("  " + lipgloss.NewStyle().Foreground(yellow).Render("\u25cb "+taskText) + "\n")
						}
					}
				}
			}
			if todayCount == 0 {
				taskContent.WriteString("  " + DimStyle.Render("none") + "\n")
			}

			// Stats
			taskContent.WriteString("\n" + DimStyle.Render(strings.Repeat("\u2500", taskPanelWidth-4)) + "\n")
			taskContent.WriteString(fmt.Sprintf("  %s %d overdue  %s %d today  %s %d upcoming\n",
				lipgloss.NewStyle().Foreground(red).Render("\u25cf"), overdueCount,
				lipgloss.NewStyle().Foreground(yellow).Render("\u25cf"), todayCount,
				lipgloss.NewStyle().Foreground(green).Render("\u25cf"), upcomingCount))

			taskPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(taskPanelWidth).
				Height(contentHeight).
				Background(base).
				Padding(0, 1).
				Render(taskContent.String())

			// Adjust editor width
			tbEditorWidth := m.width - sidebarWidth - taskPanelWidth - 6
			if tbEditorWidth < 30 {
				tbEditorWidth = 30
			}
			tbEditor := EditorStyle.Copy().
				BorderForeground(editorBorderColor).
				Width(tbEditorWidth).
				Height(contentHeight).
				Render(editorPanel)

			content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, tbEditor, taskPanel)
		case "research":
			sidebar := SidebarStyle.Copy().
				BorderForeground(sidebarBorderColor).
				Width(sidebarWidth).
				Height(contentHeight).
				Render(m.sidebar.View())

			// Research/notes panel
			notesPanelWidth := m.width / 4
			if notesPanelWidth < 25 {
				notesPanelWidth = 25
			}
			if notesPanelWidth > 40 {
				notesPanelWidth = 40
			}

			var notesContent strings.Builder
			notesContent.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  RESEARCH"))
			notesContent.WriteString("\n")
			notesContent.WriteString(DimStyle.Render(strings.Repeat("\u2500", notesPanelWidth-4)))
			notesContent.WriteString("\n\n")

			// Recent notes
			notesContent.WriteString(lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Recent Notes") + "\n")
			paths := m.vault.SortedPaths()
			type recentNote struct {
				path  string
				words int
			}
			var recents []recentNote
			for _, p := range paths {
				note := m.vault.GetNote(p)
				if note == nil {
					continue
				}
				words := len(strings.Fields(note.Content))
				recents = append(recents, recentNote{path: p, words: words})
			}
			// Show up to 20 recent notes
			shown := 0
			for i := len(recents) - 1; i >= 0 && shown < 20; i-- {
				r := recents[i]
				name := filepath.Base(r.path)
				name = strings.TrimSuffix(name, ".md")
				if len(name) > notesPanelWidth-12 {
					name = name[:notesPanelWidth-15] + "..."
				}
				style := lipgloss.NewStyle().Foreground(text)
				if r.path == m.activeNote {
					style = style.Foreground(mauve).Bold(true)
				}
				wordStr := DimStyle.Render(fmt.Sprintf(" %dw", r.words))
				notesContent.WriteString("  " + style.Render("\u00b7 "+name) + wordStr + "\n")
				shown++
			}

			// Backlinks of current note
			notesContent.WriteString("\n" + lipgloss.NewStyle().Foreground(green).Bold(true).Render("  Backlinks") + "\n")
			if m.activeNote != "" {
				backlinks := m.index.GetBacklinks(m.activeNote)
				if len(backlinks) == 0 {
					notesContent.WriteString("  " + DimStyle.Render("none") + "\n")
				}
				for _, bl := range backlinks {
					name := filepath.Base(bl)
					name = strings.TrimSuffix(name, ".md")
					if len(name) > notesPanelWidth-8 {
						name = name[:notesPanelWidth-11] + "..."
					}
					notesContent.WriteString("  " + lipgloss.NewStyle().Foreground(green).Render("\u2190 "+name) + "\n")
				}
			}

			// Outgoing links
			notesContent.WriteString("\n" + lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  Links") + "\n")
			if m.activeNote != "" {
				note := m.vault.GetNote(m.activeNote)
				if note != nil && len(note.Links) > 0 {
					for _, link := range note.Links {
						if len(link) > notesPanelWidth-8 {
							link = link[:notesPanelWidth-11] + "..."
						}
						notesContent.WriteString("  " + lipgloss.NewStyle().Foreground(blue).Render("\u2192 "+link) + "\n")
					}
				} else {
					notesContent.WriteString("  " + DimStyle.Render("none") + "\n")
				}
			}

			notesPanel := lipgloss.NewStyle().
				BorderStyle(PanelBorder).
				BorderForeground(surface1).
				Width(notesPanelWidth).
				Height(contentHeight).
				Background(base).
				Padding(0, 1).
				Render(notesContent.String())

			// Adjust editor width
			rsEditorWidth := m.width - sidebarWidth - notesPanelWidth - 6
			if rsEditorWidth < 30 {
				rsEditorWidth = 30
			}
			rsEditor := EditorStyle.Copy().
				BorderForeground(editorBorderColor).
				Width(rsEditorWidth).
				Height(contentHeight).
				Render(editorPanel)

			content = lipgloss.JoinHorizontal(lipgloss.Top, sidebar, rsEditor, notesPanel)
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
		// Breadcrumb bar (between content and status)
		var breadcrumbBar string
		if m.breadcrumb != nil && (len(m.breadcrumb.Pinned()) > 0 || m.breadcrumb.CanGoBack()) {
			breadcrumbBar = m.breadcrumb.RenderBar(m.width, m.activeNote)
		}
		// Pomodoro status indicator in status bar
		m.statusbar.SetPomodoroStatus(m.pomodoro.StatusString())
		status := m.statusbar.View()
		if breadcrumbBar != "" {
			view = lipgloss.JoinVertical(lipgloss.Left, content, breadcrumbBar, status)
		} else {
			view = lipgloss.JoinVertical(lipgloss.Left, content, status)
		}
	}

	// Safety: truncate output to terminal height to prevent alt-screen overflow
	if viewLines := strings.Split(view, "\n"); len(viewLines) > m.height {
		view = strings.Join(viewLines[:m.height], "\n")
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
	if m.globalReplace.IsActive() {
		overlay := m.globalReplace.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.aiTemplates.IsActive() {
		overlay := m.aiTemplates.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.languageLearning.IsActive() {
		overlay := m.languageLearning.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.habitTracker.IsActive() {
		overlay := m.habitTracker.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.focusSession.IsActive() {
		overlay := m.focusSession.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.standupGen.IsActive() {
		overlay := m.standupGen.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.noteHistory.IsActive() {
		overlay := m.noteHistory.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.smartConnect.IsActive() {
		overlay := m.smartConnect.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.writingStats.IsActive() {
		overlay := m.writingStats.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.quickCapture.IsActive() {
		overlay := m.quickCapture.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dashboard.IsActive() {
		overlay := m.dashboard.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.mindMap.IsActive() {
		overlay := m.mindMap.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.journalPrompts.IsActive() {
		overlay := m.journalPrompts.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.clipManager.IsActive() {
		overlay := m.clipManager.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dailyPlanner.IsActive() {
		overlay := m.dailyPlanner.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.aiScheduler.IsActive() {
		overlay := m.aiScheduler.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.recurringTasks.IsActive() {
		overlay := m.recurringTasks.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.notePreview.IsActive() {
		overlay := m.notePreview.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.scratchpad.IsActive() {
		overlay := m.scratchpad.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.backup.IsActive() {
		overlay := m.backup.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.onboarding.IsActive() {
		overlay := m.onboarding.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.projectMode.IsActive() {
		overlay := m.projectMode.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.nlSearch.IsActive() {
		overlay := m.nlSearch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.writingCoach.IsActive() {
		overlay := m.writingCoach.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dataview.IsActive() {
		overlay := m.dataview.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.timeTracker.IsActive() {
		overlay := m.timeTracker.View()
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
	if m.kanban.IsActive() {
		overlay := m.kanban.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.vaultRefactor.IsActive() {
		overlay := m.vaultRefactor.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.dailyBriefing.IsActive() {
		overlay := m.dailyBriefing.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.encryption.IsActive() {
		overlay := m.encryption.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.backlinkPreview.IsActive() && !m.viewMode {
		popup := m.backlinkPreview.View()
		if popup != "" {
			view = m.overlayCenter(view, popup)
		}
	}
	if m.gitHistory.IsActive() {
		overlay := m.gitHistory.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.workspace.IsActive() {
		overlay := m.workspace.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.timeline.IsActive() {
		overlay := m.timeline.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.vaultSwitch.IsActive() {
		overlay := m.vaultSwitch.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.frontmatterEdit.IsActive() {
		overlay := m.frontmatterEdit.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.taskManager.IsActive() {
		overlay := m.taskManager.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.linkAssist.IsActive() {
		overlay := m.linkAssist.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.blogPublisher.IsActive() {
		overlay := m.blogPublisher.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.themeEditor.IsActive() {
		overlay := m.themeEditor.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.imageManager.IsActive() {
		overlay := m.imageManager.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.research.IsActive() {
		overlay := m.research.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.pomodoro.IsActive() {
		overlay := m.pomodoro.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.webClipper.IsActive() {
		overlay := m.webClipper.View()
		view = m.overlayCenter(view, overlay)
	}
	if m.linkCompleter != nil && m.linkCompleter.IsActive() {
		overlay := m.linkCompleter.Render(m.width/2, m.height/2)
		if overlay != "" {
			view = m.overlayCenter(view, overlay)
		}
	}
	if m.slashMenu != nil && m.slashMenu.IsActive() {
		overlay := m.slashMenu.View()
		if overlay != "" {
			view = m.overlayCenter(view, overlay)
		}
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

	// Toast notifications (top-right corner)
	if m.toast != nil && m.toast.HasItems() {
		toastView := m.toast.View()
		if toastView != "" {
			view = m.overlayTopRight(view, toastView)
		}
	}

	return view
}

// overlayTopRight places an overlay in the top-right corner of the background.
func (m Model) overlayTopRight(bg, overlay string) string {
	bgLines := strings.Split(bg, "\n")
	overlayLines := strings.Split(overlay, "\n")

	overlayWidth := 0
	for _, line := range overlayLines {
		w := lipgloss.Width(line)
		if w > overlayWidth {
			overlayWidth = w
		}
	}

	startX := m.width - overlayWidth - 2
	if startX < 0 {
		startX = 0
	}
	startY := 1

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	pad := strings.Repeat(" ", startX)
	for i, overlayLine := range overlayLines {
		y := startY + i
		if y >= len(result) {
			break
		}
		right := ansiSkipCols(result[y], startX+lipgloss.Width(overlayLine))
		result[y] = pad + overlayLine + right
	}

	return strings.Join(result, "\n")
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

	// Result count
	innerWidth := width - 4
	countStr := ""
	if m.searchQuery != "" {
		countStr = fmt.Sprintf("  %d result", len(m.searchResults))
		if len(m.searchResults) != 1 {
			countStr += "s"
		}
	}
	sepLine := DimStyle.Render(strings.Repeat("─", innerWidth))
	if countStr != "" {
		sepLine = DimStyle.Render(strings.Repeat("─", innerWidth-lipgloss.Width(countStr))) + DimStyle.Render(countStr)
	}
	b.WriteString(sepLine)
	b.WriteString("\n")

	maxResults := 10
	if len(m.searchResults) == 0 {
		b.WriteString(DimStyle.Render("  No results"))
	} else {
		for i := 0; i < len(m.searchResults) && i < maxResults; i++ {
			name := strings.TrimSuffix(m.searchResults[i], ".md")
			icon := lipgloss.NewStyle().Foreground(blue).Render(IconFileChar)
			if i == m.searchCursor {
				selectedBase := lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true)
				matchOnSelected := lipgloss.NewStyle().
					Background(surface0).
					Foreground(yellow).
					Bold(true).
					Underline(true)
				highlighted := fuzzyHighlight(name, m.searchQuery, selectedBase, matchOnSelected)
				line := selectedBase.MaxWidth(innerWidth).Render("  " + icon + " " + highlighted)
				b.WriteString(line)
			} else {
				matchStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
				highlighted := fuzzyHighlight(name, m.searchQuery, NormalItemStyle, matchStyle)
				b.WriteString("  " + icon + " " + highlighted)
			}
			b.WriteString("\n")
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
				MaxWidth(width - 6).
				Render("  " + icon + " " + dir)
			b.WriteString(line)
		} else {
			b.WriteString("  " + icon + " " + NormalItemStyle.Render(dir))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
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

	pad := strings.Repeat(" ", startX)
	for i, overlayLine := range overlayLines {
		y := startY + i
		if y >= len(result) {
			break
		}
		right := ansiSkipCols(result[y], startX+lipgloss.Width(overlayLine))
		result[y] = pad + overlayLine + right
	}

	return strings.Join(result, "\n")
}

// ansiSkipCols returns the suffix of s after skipping n visual columns.
// ANSI escape sequences are correctly skipped without counting as width.
// A reset sequence is prepended to prevent color bleed from the skipped portion.
func ansiSkipCols(s string, n int) string {
	width := 0
	i := 0
	for i < len(s) && width < n {
		if s[i] == '\x1b' {
			// Skip entire ANSI escape sequence
			j := i + 1
			if j < len(s) && s[j] == '[' {
				j++
				for j < len(s) && s[j] != 'm' && s[j] != 'H' && s[j] != 'J' && s[j] != 'K' {
					j++
				}
				if j < len(s) {
					j++
				}
			}
			i = j
		} else {
			width++
			i++
		}
	}
	if i >= len(s) {
		return ""
	}
	return "\x1b[0m" + s[i:]
}

// overlayAtCursor places an overlay near the editor cursor position (below it).
func (m Model) overlayAtCursor(bg, overlay string) string {
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

	// Approximate cursor screen position
	// Editor starts after sidebar, cursor line offset by scroll
	sidebarWidth := 0
	if m.config.Layout == "" || m.config.Layout == "default" || m.config.Layout == "writer" {
		sidebarWidth = m.width / 5
		if sidebarWidth < 22 {
			sidebarWidth = 22
		}
		if sidebarWidth > 35 {
			sidebarWidth = 35
		}
	}
	// Add border + gutter
	gutterWidth := 0
	if m.config.LineNumbers {
		gutterWidth = 7
	}
	startX := sidebarWidth + 3 + gutterWidth + m.editor.col
	startY := m.editor.cursor - m.editor.scroll + 3 // +3 for header/separator

	// Make sure it fits on screen
	if startX+overlayWidth > m.width {
		startX = m.width - overlayWidth - 1
	}
	if startX < 1 {
		startX = 1
	}
	if startY+overlayHeight > m.height-2 {
		startY = m.editor.cursor - m.editor.scroll - overlayHeight
	}
	if startY < 1 {
		startY = 1
	}

	result := make([]string, len(bgLines))
	copy(result, bgLines)

	pad := strings.Repeat(" ", startX)
	for i, overlayLine := range overlayLines {
		y := startY + i
		if y >= len(result) {
			break
		}
		right := ansiSkipCols(result[y], startX+lipgloss.Width(overlayLine))
		result[y] = pad + overlayLine + right
	}

	return strings.Join(result, "\n")
}

// applyVaultRefactor parses the AI refactor plan and applies file moves,
// tag additions, and wikilink insertions.
func (m *Model) applyVaultRefactor(plan string) {
	moveCount := 0
	for _, line := range strings.Split(plan, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "MOVE:") {
			continue
		}
		parts := strings.SplitN(line[5:], "|", 4)
		if len(parts) < 1 {
			continue
		}
		movePart := strings.TrimSpace(parts[0])
		arrow := strings.SplitN(movePart, "->", 2)
		if len(arrow) != 2 {
			continue
		}
		oldName := strings.TrimSpace(arrow[0])
		newName := strings.TrimSpace(arrow[1])

		oldPath := filepath.Join(m.vault.Root, oldName)
		newPath := filepath.Join(m.vault.Root, newName)

		if _, err := os.Stat(oldPath); os.IsNotExist(err) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(newPath), 0755); err != nil {
			continue
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			continue
		}
		moveCount++

		for _, part := range parts[1:] {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "TAGS:") {
				tagStr := strings.TrimSpace(part[5:])
				tags := strings.Split(tagStr, ",")
				var cleanTags []string
				for _, t := range tags {
					t = strings.TrimSpace(t)
					t = strings.TrimPrefix(t, "#")
					if t != "" {
						cleanTags = append(cleanTags, t)
					}
				}
				if len(cleanTags) > 0 {
					m.addTagsToFile(newPath, cleanTags)
				}
			}
		}
	}

	if moveCount > 0 {
		m.vault.Scan()
		m.index = vault.NewIndex(m.vault)
		m.index.Build()
		paths := m.vault.SortedPaths()
		m.sidebar.SetFiles(paths)
		m.autocomplete.SetNotes(paths)
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.statusbar.SetMessage(fmt.Sprintf("Vault refactored: %d files reorganized", moveCount))
	} else {
		m.statusbar.SetMessage("Vault refactor: no changes applied")
	}
}

func (m *Model) addTagsToFile(path string, tags []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "---" {
		fmEnd := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				fmEnd = i
				break
			}
		}
		if fmEnd > 0 {
			hasTagsLine := false
			for i := 1; i < fmEnd; i++ {
				if strings.HasPrefix(strings.TrimSpace(lines[i]), "tags:") {
					hasTagsLine = true
					break
				}
			}
			if !hasTagsLine {
				tagLine := "tags: [" + strings.Join(tags, ", ") + "]"
				newLines := make([]string, 0, len(lines)+1)
				newLines = append(newLines, lines[:fmEnd]...)
				newLines = append(newLines, tagLine)
				newLines = append(newLines, lines[fmEnd:]...)
				os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0644)
			}
			return
		}
	}

	fm := "---\ntags: [" + strings.Join(tags, ", ") + "]\n---\n\n"
	os.WriteFile(path, []byte(fm+content), 0644)
}

// applyEncryptionResult handles the result of an encrypt/decrypt operation.
func (m *Model) applyEncryptionResult(result EncryptionResult) {
	if m.activeNote == "" {
		return
	}

	content := m.editor.GetContent()
	oldPath := filepath.Join(m.vault.Root, m.activeNote)

	switch result.Action {
	case encActionEncrypt:
		encrypted, err := m.encryption.EncryptContent(content)
		if err != nil {
			m.statusbar.SetMessage("Encryption failed: " + err.Error())
			return
		}
		newName := m.encryption.EncryptedName(m.activeNote)
		newPath := filepath.Join(m.vault.Root, newName)
		if err := os.WriteFile(newPath, []byte(encrypted), 0644); err != nil {
			m.statusbar.SetMessage("Failed to write encrypted file")
			return
		}
		// Remove the original unencrypted file
		os.Remove(oldPath)
		m.vault.Scan()
		m.index = vault.NewIndex(m.vault)
		m.index.Build()
		paths := m.vault.SortedPaths()
		m.sidebar.SetFiles(paths)
		m.autocomplete.SetNotes(paths)
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.statusbar.SetMessage("Encrypted: " + m.activeNote + " -> " + newName)
		// Load the encrypted file (shows base64)
		m.loadNote(newName)
		m.sidebar.cursor = m.findFileIndex(newName)

	case encActionDecrypt:
		if !m.encryption.IsEncrypted(m.activeNote) {
			m.statusbar.SetMessage("Note is not encrypted")
			return
		}
		decrypted, err := m.encryption.DecryptContent(content)
		if err != nil {
			m.statusbar.SetMessage("Decryption failed - wrong passphrase?")
			return
		}
		newName := m.encryption.DecryptedName(m.activeNote)
		newPath := filepath.Join(m.vault.Root, newName)
		if err := os.WriteFile(newPath, []byte(decrypted), 0644); err != nil {
			m.statusbar.SetMessage("Failed to write decrypted file")
			return
		}
		// Remove the encrypted file
		os.Remove(oldPath)
		m.vault.Scan()
		m.index = vault.NewIndex(m.vault)
		m.index.Build()
		paths := m.vault.SortedPaths()
		m.sidebar.SetFiles(paths)
		m.autocomplete.SetNotes(paths)
		m.statusbar.SetNoteCount(m.vault.NoteCount())
		m.loadNote(newName)
		m.sidebar.cursor = m.findFileIndex(newName)
		m.statusbar.SetMessage("Decrypted: " + m.activeNote + " -> " + newName)
	}
}

// captureWorkspaceLayout snapshots the current TUI state into a WorkspaceLayout.
func (m *Model) captureWorkspaceLayout(name string) WorkspaceLayout {
	var openNotes []string
	if m.tabBar != nil {
		for _, tab := range m.tabBar.Tabs() {
			openNotes = append(openNotes, tab.Path)
		}
	}
	layoutName := m.config.Layout
	if layoutName == "" {
		layoutName = "default"
	}
	return WorkspaceLayout{
		Name:         name,
		ActiveNote:   m.activeNote,
		OpenNotes:    openNotes,
		SidebarFocus: m.focus == focusSidebar,
		ViewMode:     m.viewMode,
		Layout:       layoutName,
		CreatedAt:    time.Now().Format("2006-01-02 15:04:05"),
	}
}

// applyWorkspaceLayout restores a saved workspace layout.
func (m *Model) applyWorkspaceLayout(layout *WorkspaceLayout) {
	if layout == nil {
		return
	}
	// Restore layout mode
	if layout.Layout != "" {
		m.config.Layout = layout.Layout
		m.updateLayout()
	}
	// Restore open notes in tabs
	if m.tabBar != nil && len(layout.OpenNotes) > 0 {
		// Reset tabs by removing all then re-adding
		for _, tab := range m.tabBar.Tabs() {
			m.tabBar.RemoveTab(tab.Path)
		}
		for _, note := range layout.OpenNotes {
			if m.vault.GetNote(note) != nil {
				m.tabBar.AddTab(note)
			}
		}
	}
	// Restore active note
	if layout.ActiveNote != "" && m.vault.GetNote(layout.ActiveNote) != nil {
		m.loadNote(layout.ActiveNote)
		m.sidebar.cursor = m.findFileIndex(layout.ActiveNote)
	}
	// Restore view mode
	m.viewMode = layout.ViewMode
	if m.viewMode {
		m.statusbar.SetMode("VIEW")
	} else {
		m.statusbar.SetMode("EDIT")
	}
	// Restore focus
	if layout.SidebarFocus {
		m.setFocus(focusSidebar)
	} else {
		m.setFocus(focusEditor)
	}
	m.statusbar.SetMessage("Loaded workspace: " + layout.Name)
}

func (m *Model) writeBriefingToDailyNote(briefingContent string) {
	today := time.Now().Format("2006-01-02")
	dailyName := today + ".md"
	dailyPath := filepath.Join(m.vault.Root, dailyName)

	existing, err := os.ReadFile(dailyPath)
	if err != nil {
		content := fmt.Sprintf("---\ndate: %s\ntype: daily\n---\n\n# %s\n\n%s\n", today, today, briefingContent)
		os.WriteFile(dailyPath, []byte(content), 0644)
	} else {
		newContent := string(existing) + "\n\n---\n\n" + briefingContent + "\n"
		os.WriteFile(dailyPath, []byte(newContent), 0644)
	}

	m.vault.Scan()
	m.index = vault.NewIndex(m.vault)
	m.index.Build()
	paths := m.vault.SortedPaths()
	m.sidebar.SetFiles(paths)
	m.autocomplete.SetNotes(paths)
	m.statusbar.SetNoteCount(m.vault.NoteCount())
	m.loadNote(dailyName)
	m.sidebar.cursor = m.findFileIndex(dailyName)
	m.setFocus(focusEditor)
	m.statusbar.SetMessage("Daily briefing written to " + dailyName)
}

// replaceFrontmatter replaces existing YAML frontmatter in content with newFM,
// or prepends it if none exists.
func replaceFrontmatter(content, newFM string) string {
	if strings.HasPrefix(strings.TrimSpace(content), "---") {
		lines := strings.SplitN(content, "\n", -1)
		// Find end of existing frontmatter
		endIdx := -1
		for i := 1; i < len(lines); i++ {
			if strings.TrimSpace(lines[i]) == "---" {
				endIdx = i
				break
			}
		}
		if endIdx >= 0 {
			// Replace from line 0 through endIdx with newFM
			rest := strings.Join(lines[endIdx+1:], "\n")
			return newFM + "\n" + rest
		}
	}
	// No existing frontmatter — prepend
	return newFM + "\n" + content
}
