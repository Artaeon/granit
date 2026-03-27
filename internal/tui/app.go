package tui

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/vault"
)

// stopOllama unloads running models to free memory when Granit exits.
func stopOllama(model string) {
	if model != "" {
		_ = exec.Command("ollama", "stop", model).Run()
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
type saveResultMsg struct{ err error }

// scrollPosition stores cursor and scroll state for a note so it can be
// restored when the user reopens the file.
type scrollPosition struct {
	Line   int `json:"line"`
	Col    int `json:"col"`
	Scroll int `json:"scroll"`
}

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
	tfidfDirty     bool
	tableEditor    TableEditor
	semanticSearch *SemanticSearch
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
	tutorial          Tutorial
	projectMode      ProjectMode
	nlSearch         NLSearch
	writingCoach     WritingCoach
	dataview         DataviewOverlay
	timeTracker      TimeTracker
	knowledgeGaps    KnowledgeGaps
	planMyDay        PlanMyDay
	clockIn          ClockIn
	commandCenter    CommandCenter
	dueTodayCount    int

	// Cross-component refresh flag
	needsRefresh bool
	lastRefresh  time.Time

	// Slash command menu
	slashMenu *SlashMenu

	// Toast notifications
	toast *Toast

	// Auto-save debounce
	lastEditTime time.Time
	lastSaveTime time.Time

	// Inline spell check debounce
	lastSpellEditTime time.Time

	// Exit splash
	exitSplash    ExitSplash
	showExitSplash bool
	sessionStart  time.Time

	// View mode scroll
	viewScroll int

	// Scroll position memory — restores cursor/scroll when reopening a note
	scrollCache map[string]scrollPosition

	// Confirm delete
	confirmDelete     bool
	confirmDeleteNote string

	// External file reload confirmation
	pendingReload     bool
	pendingReloadPath string

	// Folder management
	newFolderMode bool
	newFolderName string
	moveFileMode  bool
	moveFileDirs  []string
	moveFileCursor int

	// Extract to note
	extractMode bool
	extractName string

	// Auto daily note on startup
	pendingDailyNote bool
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
		commandCenter:   NewCommandCenter(),
		dailyPlanner:    NewDailyPlanner(),
		aiScheduler:     NewAIScheduler(),
		planMyDay:       NewPlanMyDay(),
		clockIn:         NewClockIn(vaultPath),
		notePreview:     NewNotePreview(),
		dataview:        NewDataviewOverlay(),
		slashMenu:      NewSlashMenu(),
		toast:          NewToast(),
		tutorial:       NewTutorial(&cfg),
		showSplash:       cfg.ShowSplash,
		splash:           NewSplashModel(vaultPath, v.NoteCount()),
		viewMode:         cfg.DefaultViewMode,
		sessionStart:     time.Now(),
		pendingDailyNote: cfg.AutoDailyNote,
	}

	m.statusbar.SetVaultPath(vaultPath)
	m.statusbar.SetNoteCount(v.NoteCount())
	m.dueTodayCount = CountTasksDueToday(v.Notes)
	m.statusbar.SetDueTodayCount(m.dueTodayCount)
	m.autocomplete.SetNotes(paths)
	m.plugins.SetVaultPath(vaultPath)
	m.pomodoro.SetVaultRoot(vaultPath)
	m.canvas.SetVaultPath(vaultPath)
	m.publisher.SetVaultPath(vaultPath)
	m.luaOverlay.SetEngine(m.luaEngine)
	m.renderer.SetVaultNotes(m.vault.Notes)
	m.editor.SetFoldState(&m.foldState)
	m.renderer.SetVaultRoot(vaultPath)

	// Initialize view mode on status bar if default is view mode
	if cfg.DefaultViewMode {
		m.statusbar.SetViewMode(true)
	}

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

	// Set calendar daily notes and parse task data
	m.calendar.SetDailyNotes(paths)
	noteContents := make(map[string]string)
	for _, p := range paths {
		if note := v.GetNote(p); note != nil {
			noteContents[p] = note.Content
		}
	}
	m.calendar.SetNoteContents(noteContents)

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

	// Restore scroll position cache from previous session
	m.scrollCache = loadScrollCache(vaultPath)

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

// saveScrollPosition caches the current editor cursor and scroll state
// for the active note so it can be restored later.
func (m *Model) saveScrollPosition() {
	if m.activeNote == "" {
		return
	}
	if m.scrollCache == nil {
		m.scrollCache = make(map[string]scrollPosition)
	}
	m.scrollCache[m.activeNote] = scrollPosition{
		Line:   m.editor.CursorLine(),
		Col:    m.editor.CursorCol(),
		Scroll: m.editor.ScrollOffset(),
	}
}

// restoreScrollPosition restores cached cursor and scroll state for a note
// after its content has been loaded into the editor.
func (m *Model) restoreScrollPosition(relPath string) {
	if pos, ok := m.scrollCache[relPath]; ok {
		m.editor.SetCursorPosition(pos.Line, pos.Col)
		m.editor.SetScroll(pos.Scroll)
	}
}

const maxScrollCacheEntries = 100

// saveScrollCache persists the scroll position cache to <vaultRoot>/.granit/viewport.json.
// It performs LRU eviction when the cache exceeds maxScrollCacheEntries.
func (m *Model) saveScrollCache(vaultRoot string) {
	if vaultRoot == "" || len(m.scrollCache) == 0 {
		return
	}
	// Save the currently active note's position before writing
	m.saveScrollPosition()

	cache := m.scrollCache

	// LRU eviction: keep only the most recent maxScrollCacheEntries.
	// Since Go maps are unordered, just trim to size.
	if len(cache) > maxScrollCacheEntries {
		trimmed := make(map[string]scrollPosition, maxScrollCacheEntries)
		count := 0
		for k, v := range cache {
			if count >= maxScrollCacheEntries {
				break
			}
			trimmed[k] = v
			count++
		}
		cache = trimmed
	}

	dir := filepath.Join(vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	raw, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(dir, "viewport.json"), raw, 0o600); err != nil {
		m.statusbar.SetMessage("Failed to save scroll cache: " + err.Error())
	}
}

// loadScrollCache reads the scroll position cache from <vaultRoot>/.granit/viewport.json.
func loadScrollCache(vaultRoot string) map[string]scrollPosition {
	if vaultRoot == "" {
		return make(map[string]scrollPosition)
	}
	fp := filepath.Join(vaultRoot, ".granit", "viewport.json")
	raw, err := os.ReadFile(fp)
	if err != nil {
		return make(map[string]scrollPosition)
	}
	var cache map[string]scrollPosition
	if err := json.Unmarshal(raw, &cache); err != nil {
		_ = os.Remove(fp)
		return make(map[string]scrollPosition)
	}
	return cache
}

func (m *Model) loadNote(relPath string) {
	note := m.vault.GetNote(relPath)
	if note == nil {
		return
	}
	// Save scroll position of the note we're leaving
	m.saveScrollPosition()

	m.activeNote = relPath
	m.editor.LoadContent(note.Content, relPath)
	m.statusbar.SetActiveNote(relPath)
	m.statusbar.SetWordCount(m.editor.GetWordCount())
	m.viewScroll = 0

	// Restore scroll position if we've seen this note before
	m.restoreScrollPosition(relPath)

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

	// Run inline spell check on the newly loaded note
	if m.spellcheck.InlineEnabled() {
		words := m.spellcheck.Check(note.Content)
		m.spellcheck.HandleInlineResult(words)
		m.editor.SetSpellPositions(m.spellcheck.InlinePositions())
	} else {
		m.editor.SetSpellPositions(nil)
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
	// Clock-in timer and reminder tick loop
	cmds = append(cmds, m.clockIn.StartTicking())
	// Background embedding index
	if m.config.SemanticSearchEnabled && m.config.AIProvider != "local" {
		if bgCmd := m.startSemanticBgIndex(); bgCmd != nil {
			cmds = append(cmds, bgCmd)
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

