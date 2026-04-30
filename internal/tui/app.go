package tui

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/config"
	"github.com/artaeon/granit/internal/modules"
	"github.com/artaeon/granit/internal/objects"
	"github.com/artaeon/granit/internal/profiles"
	"github.com/artaeon/granit/internal/tasks"
	"github.com/artaeon/granit/internal/tui/widgets"
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
	renderer  *Renderer
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
	registry       *modules.Registry
	// cmdActionToModuleID lets the palette filter ask "is the module
	// that owns this command currently enabled?" — populated by
	// RegisterBuiltins. Commands without an entry have no module
	// owner and stay visible.
	cmdActionToModuleID map[CommandAction]string
	// moduleCommandToAction lets the keybind dispatcher resolve a
	// registry-routed Keybind.CommandID back to the CommandAction
	// executeCommand can invoke. Empty entries fall through to the
	// legacy switch in app_update.go.
	moduleCommandToAction map[string]CommandAction
	// taskStore is the unified canonical task layer (Phase 2 of
	// the relaunch). Nil when cfg.UseTaskStore is false — readers
	// must check before dereferencing. When non-nil, m.cachedTasks
	// flows from store.All() instead of ParseAllTasks.
	taskStore *tasks.TaskStore
	// profileRegistry holds the available profiles + the active
	// pointer (Phase 3 of the relaunch). Nil when
	// cfg.UseProfiles is false. Daily Hub overlay and profile
	// picker (commits 5+6) read the active profile from here.
	profileRegistry *profiles.ProfileRegistry
	settings        Settings
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
	autoTagger      *AutoTagger
	autoLinkSuggest *AutoLinkSuggest
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
	vaultRefactor   VaultRefactor
	dailyBriefing   DailyBriefing
	devotional      Devotional
	morningRoutine  MorningRoutine

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
	statusTray      StatusTray
	imageManager    ImageManager
	themeEditor     ThemeEditor
	layoutPicker    LayoutPicker
	linkAssist      LinkAssist
	taskManager     TaskManager
	blogPublisher    BlogPublisher
	aiTemplates      AITemplates
	languageLearning LanguageLearning
	habitTracker     HabitTracker
	goalsMode        GoalsMode
	universalSearch  UniversalSearch
	ideasBoard       IdeasBoard
	eventStore       *EventStore
	focusSession     FocusSession
	standupGen       StandupGenerator
	dailyReview      DailyReview
	noteHistory      NoteHistory
	smartConnect     SmartConnections
	writingStats     WritingStats
	quickCapture     QuickCapture
	dashboard        Dashboard
	dailyHub         DailyHub
	widgetRegistry   *widgets.Registry
	profilePicker    ProfilePicker
	triageQueue      TriageQueue
	mindMap          MindMap
	journalPrompts   JournalPrompts
	clipManager      ClipManager
	dailyPlanner     DailyPlanner
	dailyJot         DailyJot
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
	sheetView         SheetView
	objectBrowser      ObjectBrowser
	agentRunner        AgentRunner
	typedMentionPicker TypedMentionPicker
	weeklyReview      WeeklyReview
	readingList       ReadingList
	aiProjectPlanner  AIProjectPlanner
	projectDashboard  ProjectDashboard
	blogDraft         BlogDraft
	taskTriage        TaskTriage
	nextcloudOverlay  NextcloudOverlay
	calendarPanel     CalendarPanel
	rightPanelCalendar bool // toggle: show calendar panel instead of backlinks
	dueTodayCount     int
	cachedTasks       []Task // cached ParseAllTasks result, refreshed on vault changes

	// Typed-objects layer (Capacities-style). Registry is loaded
	// once at startup (built-ins + vault overrides at .granit/types/).
	// Index is rebuilt every time the vault scan finishes — cheap
	// because it's pure in-memory frontmatter scanning.
	objectsRegistry *objects.Registry
	objectsIndex    *objects.Index

	// Saved-views catalog: built-ins + vault overlays from
	// .granit/views/<id>.json. Re-evaluated against objectsIndex on
	// every Refresh so the list stays current as the user adds notes.
	viewCatalog *objects.ViewCatalog
	savedViews  SavedViews

	// Repo Tracker: scans config.RepoScanRoot for git repositories
	// and lets the user import each as a typed-project note.
	repoTracker RepoTracker

	// Startup message (e.g. "Loaded 247 notes in 12ms")
	startupMsg string

	// Cross-component refresh flag
	needsRefresh bool
	lastRefresh  time.Time

	// Slash command menu
	slashMenu *SlashMenu

	// Inline AI selection edit — true while a request is in flight. Used to
	// reject overlapping dispatches (rapid Alt+A spam) and to render a
	// spinner-ish hint in the status bar.
	aiEditPending bool

	// Diff preview overlay for inline AI edits. When the model
	// returns, instead of writing immediately we show this for the
	// user to accept/discard (Deepnote-style "see what AI proposed").
	// Bypassed when config.AIAutoApplyEdits is true.
	aiDiffPreview AIDiffPreview

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
	pendingSheetPath string
}

func NewModel(vaultPath string) (Model, error) {
	cfg := config.LoadForVault(vaultPath)
	// Migrate retired layout names to their replacement. The "calendar" and
	// "taskboard" layouts were merged into "cockpit"; users with older
	// settings.json files would otherwise hit the unknown-layout fallback.
	if cfg.Layout == "calendar" || cfg.Layout == "taskboard" {
		cfg.Layout = LayoutCockpit
	}
	themeName := ResolveThemeName(cfg.Theme, cfg.AutoDarkMode, cfg.DarkTheme, cfg.LightTheme)
	ApplyTheme(themeName)
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

	// Typed-objects layer: load registry (built-ins + vault overrides
	// at .granit/types/) and build the initial Object index by scanning
	// frontmatter. Errors during type loading are logged but don't
	// block startup — vault overrides are optional and a malformed
	// JSON shouldn't lock the user out of their entire vault.
	objReg := objects.NewRegistry()
	if _, errs := objReg.LoadVaultDir(v.Root); len(errs) > 0 {
		for _, err := range errs {
			log.Printf("granit: object types: %v", err)
		}
	}
	objIdx := rebuildObjectsIndex(objReg, v)

	// Saved-views: built-ins ship with granit, vault-local overrides at
	// .granit/views/. Errors are logged, not fatal — same rationale as
	// type loading above.
	viewCat := objects.NewViewCatalog(objects.BuiltinViews())
	if _, errs := viewCat.LoadVaultDir(v.Root); len(errs) > 0 {
		for _, err := range errs {
			log.Printf("granit: saved views: %v", err)
		}
	}

	paths := v.SortedPaths()

	m := Model{
		vault:          v,
		index:          idx,
		objectsRegistry: objReg,
		objectsIndex:    objIdx,
		viewCatalog:     viewCat,
		savedViews:      NewSavedViews(),
		repoTracker:     NewRepoTracker(),
		aiDiffPreview:   NewAIDiffPreview(),
		sidebar:        NewSidebar(paths),
		editor:         NewEditor(),
		renderer:       NewRenderer(),
		backlinks:      NewBacklinks(),
		calendarPanel:  NewCalendarPanel(),
		statusbar:      NewStatusBar(),
		config:         cfg,
		focus:          focusSidebar,
		commandPalette: NewCommandPalette(),
		settings:       NewSettings(cfg, v),
		graphView:      NewGraphView(v, idx),
		tagBrowser:     NewTagBrowser(v),
		helpOverlay:    NewHelpOverlay(),
		outline:        NewOutline(),
		bookmarks:      NewBookmarks(v.Root),
		findReplace:    NewFindReplace(),
		vaultStats:     NewVaultStats(v, idx),
		templates:      NewTemplates(),
		focusMode:      NewFocusMode(),
		quickSwitch:    NewQuickSwitch(),
		autocomplete:   NewAutocomplete(),
		trash:          NewTrash(v.Root),
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
		autoSync:       NewAutoSync(v.Root),
		publisher:      NewPublisher(),
		splitPane:      NewSplitPane(),
		luaEngine:      NewLuaEngine(v.Root),
		luaOverlay:     NewLuaOverlay(),
		flashcards:     NewFlashcards(v.Root),
		quizMode:       NewQuizMode(),
		learnDash:      NewLearnDashboard(v.Root),
		aiChat:         NewAIChat(),
		composer:       NewComposer(),
		knowledgeGraph: NewKnowledgeGraph(),
		autoLinker:     NewAutoLinker(),
		tableEditor:    NewTableEditor(),
		semanticSearch: NewSemanticSearch(),
		ghostWriter:    NewGhostWriter(),
		threadWeaver:   NewThreadWeaver(),
		noteChat:       NewNoteChat(),
		autoTagger:      NewAutoTagger(),
		autoLinkSuggest: NewAutoLinkSuggest(),
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
		statusTray:      NewStatusTray(),
		imageManager:    NewImageManager(),
		themeEditor:     NewThemeEditor(),
		layoutPicker:    NewLayoutPicker(),
		linkAssist:      NewLinkAssist(),
		taskManager:     NewTaskManager(),
		blogPublisher:    NewBlogPublisher(),
		aiTemplates:      NewAITemplates(),
		languageLearning: NewLanguageLearning(),
		habitTracker:     NewHabitTracker(),
		focusSession:     NewFocusSession(),
		goalsMode:        NewGoalsMode(),
		universalSearch:  NewUniversalSearch(),
		ideasBoard:       NewIdeasBoard(),
		eventStore:       NewEventStore(vaultPath),
		commandCenter:   NewCommandCenter(),
		sheetView:       NewSheetView(),
		objectBrowser:      NewObjectBrowser(),
		agentRunner:        NewAgentRunner(),
		typedMentionPicker: NewTypedMentionPicker(),
		dailyPlanner:    NewDailyPlanner(),
		dailyJot:        NewDailyJot(),
		aiScheduler:     NewAIScheduler(),
		planMyDay:       NewPlanMyDay(),
		clockIn:         NewClockIn(vaultPath),
		notePreview:     NewNotePreview(),
		weeklyReview:     NewWeeklyReview(),
		readingList:      NewReadingList(),
		aiProjectPlanner: NewAIProjectPlanner(),
		projectDashboard: NewProjectDashboard(),
		nextcloudOverlay: NewNextcloudOverlay(),
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

	m.tutorial.vaultRoot = vaultPath
	m.statusbar.SetVaultPath(vaultPath)
	m.statusbar.SetNoteCount(v.NoteCount())
	m.cachedTasks = m.currentTasks()
	m.dueTodayCount = CountTasksDueTodayFromList(m.cachedTasks)
	m.statusbar.SetDueTodayCount(m.dueTodayCount)
	m.statusbar.SetOverdueCount(CountOverdueTasksFromList(m.cachedTasks))
	m.checkDayPlanned()
	m.autocomplete.SetNotes(paths)
	m.plugins.SetVaultPath(vaultPath)
	m.pomodoro.SetVaultRoot(vaultPath)
	m.pomodoro.SetGoal(cfg.PomodoroGoal)
	m.canvas.SetVaultPath(vaultPath)
	m.publisher.SetVaultPath(vaultPath)
	m.luaOverlay.SetEngine(m.luaEngine)
	m.renderer.SetVaultNotes(m.vault.Notes)
	m.renderer.SetViewStyle(cfg.ViewStyle)
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

	// Restore persisted explorer (folder collapse) state
	m.sidebar.LoadExplorerState(vaultPath)

	// Wire typed-objects into the sidebar so 'm' (mode-cycle key)
	// in the explorer can flip to the Types view. Sidebar holds
	// references and rebuilds its own row list on each call.
	m.sidebar.SetTypedObjects(m.objectsRegistry, m.objectsIndex)

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

	// Module registry — single source of truth for which features are
	// enabled. Mirrors legacy CorePlugins config so existing user
	// toggles survive Phase 1; Load() picks up any explicit overrides
	// the user has saved through the new registry. RegisterBuiltins is
	// a no-op until pilot commits start populating allBuiltins().
	m.registry = modules.New(v.Root)
	m.registry.MirrorLegacy(cfg.CorePlugins)
	if err := m.registry.Load(); err != nil {
		log.Printf("warning: load module state: %v", err)
	}
	m.cmdActionToModuleID, m.moduleCommandToAction = RegisterBuiltins(m.registry)
	m.settings.SetRegistry(m.registry)

	// Phase 2 task store, behind a feature flag during rollout.
	// When on, ingests existing Tasks.md / daily-note tasks and
	// writes a stable-ID sidecar. Readers and writers migrate over
	// in subsequent commits; this commit only flips the cachedTasks
	// source so a misbehaving store can't corrupt user notes.
	if cfg.UseTaskStore {
		store, err := tasks.Load(v.Root, func() []tasks.NoteContent {
			items := make([]tasks.NoteContent, 0, len(v.Notes))
			for _, n := range v.Notes {
				items = append(items, tasks.NoteContent{Path: n.RelPath, Content: n.Content})
			}
			return items
		})
		if err != nil {
			log.Printf("warning: task store load: %v", err)
		}
		m.taskStore = store
		// Wire the store into every overlay that creates tasks so
		// their writes flow through it (stable IDs, sidecar
		// markers like OriginRecurring/OriginManual, GoalID
		// links). Each setter is nil-safe; passing nil restores
		// the legacy appendTaskLine path.
		m.ideasBoard.SetTaskStore(store)
		m.recurringTasks.SetTaskStore(store)
		m.goalsMode.SetTaskStore(store)
		m.morningRoutine.SetTaskStore(store)
		m.taskManager.SetTaskStore(store)
	}

	// Phase 3 profile boot. When the flag is on, load profiles
	// from disk + built-ins, resolve the active profile, apply
	// its module set + layout. Registry stays nil when off so
	// commits 5+6 (Daily Hub, picker) can nil-check before
	// dereferencing. Behavior with the flag off is identical to
	// pre-Phase-3.
	if cfg.UseProfiles {
		bootProfiles(&m)
		// Widget registry + Daily Hub overlay. Done here (not
		// inside bootProfiles) because the registry is widget-
		// runtime concern, not profile manifest concern, and the
		// hub needs the registry pointer at construction time.
		m.widgetRegistry = widgets.NewRegistry()
		if err := widgets.RegisterBuiltins(m.widgetRegistry); err != nil {
			log.Printf("warning: widgets register builtins: %v", err)
		}
		m.dailyHub = NewDailyHub(m.widgetRegistry)
		m.profilePicker = NewProfilePicker()
		m.triageQueue = NewTriageQueue(m.taskStore)

		// First-launch UX: brand-new vaults get the profile
		// picker on first frame so the user picks a workflow
		// explicitly. Existing vaults skip silently — Classic
		// is already active, no nag.
		if isNewVault(v.Root) {
			m.profilePicker.SetSize(m.width, m.height)
			m.profilePicker.Open(m.profileRegistry.All(), m.profileRegistry.ActiveID())
		}
	}
	registry := m.registry
	cmdMap := m.cmdActionToModuleID
	m.commandPalette.SetVisibilityFilter(func(a CommandAction) bool {
		if id, owned := cmdMap[a]; owned {
			return registry.Enabled(id)
		}
		return true
	})
	m.loadCommandCenterData(&m.commandCenter)

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
	if err := atomicWriteState(filepath.Join(dir, "viewport.json"), raw); err != nil {
		m.reportError("save scroll cache", err)
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
	// Inform the sidebar so 'R' (reveal) knows what to land on.
	m.sidebar.SetActiveNote(relPath)

	// Restore scroll position if we've seen this note before
	m.restoreScrollPosition(relPath)

	incoming := m.buildBacklinkItems(m.index.GetBacklinks(relPath), relPath)
	outgoing := m.buildOutgoingItems(m.index.GetOutgoingLinks(relPath))
	m.backlinks.SetLinks(incoming, outgoing)

	// Refresh calendar panel if active
	if m.rightPanelCalendar || LayoutHasCalendarPanel(m.config.Layout) {
		m.refreshCalendarPanel()
	}

	m.foldState.UnfoldAll()
	m.bookmarks.AddRecent(relPath)
	m.reportError("persist bookmarks", m.bookmarks.ConsumeSaveError())
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

// NoteCount returns the number of notes in the vault.
func (m Model) NoteCount() int {
	if m.vault == nil {
		return 0
	}
	return m.vault.NoteCount()
}

// SetStartupMessage sets a message to show on the status bar at launch.
func (m *Model) SetStartupMessage(msg string) {
	m.startupMsg = msg
}

// QueueOpenSheet asks the model to open the given spreadsheet
// file in a SheetView feature tab on first Init. Used by the
// CLI entry `granit path/to/file.csv` so the user lands directly
// inside the spreadsheet surface.
func (m *Model) QueueOpenSheet(path string) {
	m.pendingSheetPath = path
}

func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	if m.startupMsg != "" {
		m.statusbar.SetMessage(m.startupMsg)
		cmds = append(cmds, tea.Tick(3*time.Second, func(time.Time) tea.Msg {
			return clearMessageMsg{}
		}))
	}
	if m.showSplash {
		cmds = append(cmds, m.splash.Init())
	}
	// Auto git sync: pull on open
	if pullCmd := m.autoSync.PullOnOpen(); pullCmd != nil {
		cmds = append(cmds, pullCmd)
	}
	// Check initial git status for status bar indicator
	if gitCmd := m.autoSync.CheckStatus(); gitCmd != nil {
		cmds = append(cmds, gitCmd)
	}
	// File watcher
	if m.fileWatcher != nil && m.fileWatcher.IsEnabled() {
		cmds = append(cmds, m.fileWatcher.Start())
	}
	// Clock-in timer and reminder tick loop
	cmds = append(cmds, m.clockIn.StartTicking())
	// Nous: auto-ingest vault notes on startup. Vault.SnapshotNotes
	// gives the goroutine its own map so the main loop's concurrent
	// save/delete traffic doesn't panic under Go's "concurrent map
	// read and write" check.
	if m.config.AIProvider == "nous" {
		snapshot := m.vault.SnapshotNotes()
		nousURL := m.config.NousURL
		nousAPIKey := m.config.NousAPIKey
		go func() {
			client := NewNousClient(nousURL, nousAPIKey)
			count, err := client.IngestVault(snapshot)
			if err != nil {
				log.Printf("Nous ingest failed: %v", err)
			} else {
				log.Printf("Nous: indexed %d notes", count)
			}
		}()
	}
	// Ollama: check availability and auto-pull model on startup
	if m.config.AIProvider == "ollama" || m.config.AIProvider == "" {
		ollamaURL := m.config.OllamaURL
		ollamaModel := m.config.OllamaModel
		cmds = append(cmds, func() tea.Msg {
			msg := OllamaEnsureModel(ollamaURL, ollamaModel)
			return ollamaStatusMsg{text: msg, ready: OllamaIsReady()}
		})
	}
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
