package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"path/filepath"
	"os"
	"github.com/artaeon/granit/internal/vault"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
)

// ollamaSetupMsg carries the result of the Ollama setup wizard
type ollamaSetupMsg struct {
	step    string // "check", "install", "pull", "done"
	success bool
	message string
}

// Settings category constants
const (
	catAppearance = "Appearance"
	catEditor     = "Editor"
	catAI         = "AI"
	catFiles      = "Files"
	catSync       = "Sync"
	catTasks      = "Tasks"
	catFocus      = "Focus"
	catPlugins    = "Plugins"
	catAdvanced   = "Advanced"
)

// settingsCategories defines the display order of categories.
var settingsCategories = []string{
	catAppearance,
	catEditor,
	catAI,
	catFiles,
	catTasks,
	catFocus,
	catSync,
	catPlugins,
	catAdvanced,
}

type settingItem struct {
	label       string
	key         string
	kind        string // "bool", "string", "int", "action", "header"
	value       interface{}
	options     []string // for string types with limited options
	category    string   // which group this setting belongs to
	description string   // extra text for search matching
}

type Settings struct {
	config  config.Config
	vault   *vault.Vault
	items   []settingItem
	visible []int // indices into items that are currently visible (after filtering)
	cursor  int   // index into visible
	scroll  int
	width   int
	height  int
	active  bool
	editing bool
	editBuf string

	// Search / filter
	searching bool
	searchBuf string

	// Ollama setup wizard
	setupRunning bool
	setupStatus  string
}

func NewSettings(cfg config.Config, v *vault.Vault) Settings {
	if len(cfg.DisabledCalendars) == 0 { cfg.DisabledCalendars = []string{} }

	s := Settings{
		config: cfg,
		vault:  v,
	}
	s.buildItems()
	s.rebuildVisible()
	return s
}

func (s *Settings) buildItems() {
	var icsItems []settingItem
	if s.vault != nil {
		_ = filepath.Walk(s.vault.Root, func(p string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".ics") {
				isDisabled := false
				for _, dc := range s.config.DisabledCalendars {
					if dc == info.Name() { isDisabled = true; break }
				}
				icsItems = append(icsItems, settingItem{
					label: "Show " + info.Name(),
					key: "cal_" + info.Name(),
					kind: "bool",
					value: !isDisabled,
					category: catFiles,
					description: "Toggle calendar visibility",
				})
			}
			return nil
		})
	}

	s.items = []settingItem{
		// ── Appearance ──
		{label: "Theme", key: "theme", kind: "string", value: s.config.Theme, options: ThemeNames(), category: catAppearance, description: "color scheme palette"},
		{label: "Icon Theme", key: "icon_theme", kind: "string", value: s.config.IconTheme, options: []string{"unicode", "nerd", "emoji", "ascii"}, category: catAppearance, description: "icon set style"},
		{label: "Layout", key: "layout", kind: "string", value: s.config.Layout, options: AllLayouts(), category: catAppearance, description: "panel arrangement"},
		{label: "View Style", key: "view_style", kind: "string", value: s.config.ViewStyle, options: []string{"default", "reading", "minimal"}, category: catAppearance, description: "Ctrl+E view mode style"},
		{label: "Sidebar Position", key: "sidebar_position", kind: "string", value: s.config.SidebarPosition, options: []string{"left", "right"}, category: catAppearance, description: "file explorer side"},
		{label: "Show Icons", key: "show_icons", kind: "bool", value: s.config.ShowIcons, category: catAppearance, description: "display file icons"},
		{label: "Compact Mode", key: "compact_mode", kind: "bool", value: s.config.CompactMode, category: catAppearance, description: "reduce padding spacing"},
		{label: "Show Splash Screen", key: "show_splash", kind: "bool", value: s.config.ShowSplash, category: catAppearance, description: "startup animation"},
		{label: "Show Help Bar", key: "show_help", kind: "bool", value: s.config.ShowHelp, category: catAppearance, description: "bottom key hints"},

		// ── Editor ──
		{label: "Vim Mode", key: "vim_mode", kind: "bool", value: s.config.VimMode, category: catEditor, description: "vi keybindings modal editing"},
		{label: "Word Wrap", key: "word_wrap", kind: "bool", value: s.config.WordWrap, category: catEditor, description: "wrap long lines"},
		{label: "Tab Size", key: "tab_size", kind: "int", value: s.config.Editor.TabSize, category: catEditor, description: "indentation width spaces"},
		{label: "Line Numbers", key: "line_numbers", kind: "bool", value: s.config.LineNumbers, category: catEditor, description: "show gutter numbers"},
		{label: "Auto Close Brackets", key: "auto_close_brackets", kind: "bool", value: s.config.AutoCloseBrackets, category: catEditor, description: "pair matching parentheses"},
		{label: "Highlight Current Line", key: "highlight_current_line", kind: "bool", value: s.config.HighlightCurrentLine, category: catEditor, description: "cursor line background"},
		{label: "Default View Mode", key: "default_view_mode", kind: "bool", value: s.config.DefaultViewMode, category: catEditor, description: "open in preview reader"},
		{label: "Inline Spell Check", key: "spell_check", kind: "bool", value: s.config.SpellCheck, category: catEditor, description: "highlight misspelled words"},

		// ── AI ──
		{label: "AI Provider", key: "ai_provider", kind: "string", value: s.config.AIProvider, options: []string{"local", "ollama", "openai", "nous"}, category: catAI, description: "language model backend"},
		{label: "Ollama Model", key: "ollama_model", kind: "string", value: s.config.OllamaModel, options: []string{"qwen2.5:0.5b", "qwen2.5:1.5b", "qwen2.5:3b", "phi3:mini", "phi3.5:3.8b", "gemma2:2b", "tinyllama", "llama3.2", "llama3.2:1b", "mistral", "gemma2"}, category: catAI, description: "local LLM model name"},
		{label: "Ollama URL", key: "ollama_url", kind: "string", value: s.config.OllamaURL, category: catAI, description: "server endpoint address"},
		{label: ">> Setup Ollama (install + model)", key: "setup_ollama", kind: "action", value: "run", category: catAI, description: "wizard install configure"},
		{label: "OpenAI API Key", key: "openai_key", kind: "string", value: s.config.OpenAIKey, category: catAI, description: "secret token authentication"},
		{label: "OpenAI Model", key: "openai_model", kind: "string", value: s.config.OpenAIModel, options: []string{"gpt-4o-mini", "gpt-4o", "gpt-4.1-mini", "gpt-4.1-nano"}, category: catAI, description: "GPT model version"},
		{label: "Nous URL", key: "nous_url", kind: "string", value: s.config.NousURL, category: catAI, description: "local Nous AI server endpoint"},
		{label: "Nous API Key", key: "nous_api_key", kind: "string", value: s.config.NousAPIKey, category: catAI, description: "optional Nous authentication key"},
		{label: "Background Bots (auto-analyze)", key: "background_bots", kind: "bool", value: s.config.BackgroundBots, category: catAI, description: "automatic analysis on save"},
		{label: "Ghost Writer (AI completions)", key: "ghost_writer", kind: "bool", value: s.config.GhostWriter, category: catAI, description: "inline writing suggestions"},
		{label: "Semantic Search (embedding index)", key: "semantic_search_enabled", kind: "bool", value: s.config.SemanticSearchEnabled, category: catAI, description: "background vector embedding index for meaning-based search"},

		// ── Files ──
		{label: "Auto Save", key: "auto_save", kind: "bool", value: s.config.AutoSave, category: catFiles, description: "save on focus change"},
		{label: "Auto Daily Note", key: "auto_daily_note", kind: "bool", value: s.config.AutoDailyNote, category: catFiles, description: "open daily note on startup"},
		{label: "Daily Notes Folder", key: "daily_notes_folder", kind: "string", value: s.config.DailyNotesFolder, category: catFiles, description: "journal directory path"},
		{label: "Weekly Notes Folder", key: "weekly_notes_folder", kind: "string", value: s.config.WeeklyNotesFolder, category: catFiles, description: "weekly review directory path"},
		{label: "Weekly Note Template", key: "weekly_note_template", kind: "string", value: s.config.WeeklyNoteTemplate, category: catFiles, description: "weekly note template file"},
		{label: "Sort Files By", key: "sort_by", kind: "string", value: s.config.SortBy, options: []string{"name", "modified", "created"}, category: catFiles, description: "file list ordering"},
		{label: "Search Content by Default", key: "search_content", kind: "bool", value: s.config.SearchContentByDefault, category: catFiles, description: "full text search"},
		{label: "Confirm Delete", key: "confirm_delete", kind: "bool", value: s.config.ConfirmDelete, category: catFiles, description: "ask before removing"},
		{label: "Auto Refresh Vault", key: "auto_refresh", kind: "bool", value: s.config.AutoRefresh, category: catFiles, description: "reload on external change"},
		

		// ── Tasks ──
		{label: "Task Filter Mode", key: "task_filter_mode", kind: "string", value: s.config.TaskFilterMode, options: []string{"all", "tagged", "folders"}, category: catTasks, description: "filter tasks: all checkboxes, only tagged, or by folder"},
		{label: "Required Tags", key: "task_required_tags", kind: "string", value: strings.Join(s.config.TaskRequiredTags, ", "), category: catTasks, description: "comma-separated tags to filter tasks (e.g. task, todo)"},
		{label: "Exclude Folders", key: "task_exclude_folders", kind: "string", value: strings.Join(s.config.TaskExcludeFolders, ", "), category: catTasks, description: "comma-separated folders to exclude from task list"},
		{label: "Hide Completed Tasks", key: "task_exclude_done", kind: "bool", value: s.config.TaskExcludeDone, category: catTasks, description: "hide completed tasks from all views"},

		// ── Focus ──
		{label: "Daily Pomodoro Goal", key: "pomodoro_goal", kind: "int", value: s.config.PomodoroGoal, category: catFocus, description: "target sessions per day focus timer"},

		// ── Sync ──
		{label: "Nextcloud URL", key: "nextcloud_url", kind: "string", value: s.config.NextcloudURL, category: catSync, description: "server address WebDAV"},
		{label: "Nextcloud Username", key: "nextcloud_user", kind: "string", value: s.config.NextcloudUser, category: catSync, description: "login user account"},
		{label: "Nextcloud Password", key: "nextcloud_pass", kind: "string", value: s.config.NextcloudPass, category: catSync, description: "app password token"},
		{label: "Nextcloud Remote Path", key: "nextcloud_path", kind: "string", value: s.config.NextcloudPath, category: catSync, description: "remote folder directory"},
		{label: "Nextcloud Auto Sync", key: "nextcloud_auto_sync", kind: "bool", value: s.config.NextcloudAutoSync, category: catSync, description: "sync on save automatic"},

		// ── Plugins ──
		{label: "Task Manager", key: "cp_task_manager", kind: "bool", value: s.corePluginVal("task_manager"), category: catPlugins, description: "todo checklist management"},
		{label: "Calendar", key: "cp_calendar", kind: "bool", value: s.corePluginVal("calendar"), category: catPlugins, description: "date event scheduling"},
		{label: "Canvas", key: "cp_canvas", kind: "bool", value: s.corePluginVal("canvas"), category: catPlugins, description: "visual 2D board"},
		{label: "Graph View", key: "cp_graph_view", kind: "bool", value: s.corePluginVal("graph_view"), category: catPlugins, description: "note link visualization"},
		{label: "Flashcards", key: "cp_flashcards", kind: "bool", value: s.corePluginVal("flashcards"), category: catPlugins, description: "spaced repetition study"},
		{label: "Quiz Mode", key: "cp_quiz_mode", kind: "bool", value: s.corePluginVal("quiz_mode"), category: catPlugins, description: "test knowledge review"},
		{label: "Pomodoro Timer", key: "cp_pomodoro", kind: "bool", value: s.corePluginVal("pomodoro"), category: catPlugins, description: "focus work interval timer"},
		{label: "Git Integration", key: "cp_git_integration", kind: "bool", value: s.corePluginVal("git_integration"), category: catPlugins, description: "version control sync"},
		{label: "Blog Publisher", key: "cp_blog_publisher", kind: "bool", value: s.corePluginVal("blog_publisher"), category: catPlugins, description: "publish medium github"},
		{label: "AI Templates", key: "cp_ai_templates", kind: "bool", value: s.corePluginVal("ai_templates"), category: catPlugins, description: "smart note generators"},
		{label: "Research Agent", key: "cp_research_agent", kind: "bool", value: s.corePluginVal("research_agent"), category: catPlugins, description: "automated web lookup"},
		{label: "Language Learning", key: "cp_language_learning", kind: "bool", value: s.corePluginVal("language_learning"), category: catPlugins, description: "vocabulary translation"},
		{label: "Habit Tracker", key: "cp_habit_tracker", kind: "bool", value: s.corePluginVal("habit_tracker"), category: catPlugins, description: "daily streak progress"},
		{label: "Ghost Writer", key: "cp_ghost_writer", kind: "bool", value: s.corePluginVal("ghost_writer"), category: catPlugins, description: "AI completion plugin"},
		{label: "Encryption", key: "cp_encryption", kind: "bool", value: s.corePluginVal("encryption"), category: catPlugins, description: "note security protect"},
		{label: "Spell Check", key: "cp_spell_check", kind: "bool", value: s.corePluginVal("spell_check"), category: catPlugins, description: "grammar typo detection"},

		// ── Advanced ──
		{label: "Git Auto Sync", key: "git_auto_sync", kind: "bool", value: s.config.GitAutoSync, category: catAdvanced, description: "auto commit push pull"},
		{label: "Auto-Tag on Save", key: "auto_tag", kind: "bool", value: s.config.AutoTag, category: catAdvanced, description: "automatic tag extraction"},
	}
}

func (s *Settings) corePluginVal(name string) bool {
	return s.config.CorePluginEnabled(name)
}

// rebuildVisible recomputes the visible list based on the current search query.
// It inserts category headers before each group of matching items.
func (s *Settings) rebuildVisible() {
	// Strip previously-added temporary headers so they don't accumulate
	// Use a new slice to avoid aliasing bugs with the backing array
	clean := make([]settingItem, 0, len(s.items))
	for _, item := range s.items {
		if item.kind != "header" {
			clean = append(clean, item)
		}
	}
	s.items = clean

	s.visible = s.visible[:0]

	if s.searchBuf == "" {
		// No filter — show all items grouped by category
		for _, cat := range settingsCategories {
			headerIdx := s.addTempHeader(cat)
			hasItems := false
			for i, item := range s.items {
				if item.category == cat {
					if !hasItems {
						s.visible = append(s.visible, headerIdx)
						hasItems = true
					}
					s.visible = append(s.visible, i)
				}
			}
		}
	} else {
		// Filter mode — fuzzy match against label, category, and description
		query := strings.ToLower(s.searchBuf)
		for _, cat := range settingsCategories {
			headerIdx := s.addTempHeader(cat)
			hasItems := false
			for i, item := range s.items {
				if item.category == cat && s.settingMatchesQuery(item, query) {
					if !hasItems {
						s.visible = append(s.visible, headerIdx)
						hasItems = true
					}
					s.visible = append(s.visible, i)
				}
			}
		}
	}

	// Clamp cursor
	if len(s.visible) == 0 {
		s.cursor = 0
		return
	}
	if s.cursor >= len(s.visible) {
		s.cursor = len(s.visible) - 1
	}
	// Skip header if cursor landed on one
	s.skipHeaderForward()
}

// settingMatchesQuery does case-insensitive fuzzy matching against label, category, and description.
func (s *Settings) settingMatchesQuery(item settingItem, query string) bool {
	combined := strings.ToLower(item.label + " " + item.description + " " + item.category)
	return settingsFuzzyMatch(combined, query)
}

// settingsFuzzyMatch checks if all characters in pattern appear in str in order.
func settingsFuzzyMatch(str, pattern string) bool {
	pi := 0
	for si := 0; si < len(str) && pi < len(pattern); si++ {
		if str[si] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}

// addTempHeader appends a temporary header item to s.items and returns its index.
func (s *Settings) addTempHeader(cat string) int {
	idx := len(s.items)
	s.items = append(s.items, settingItem{
		label:    cat,
		key:      "_header_" + strings.ToLower(cat),
		kind:     "header",
		category: cat,
	})
	return idx
}

func (s *Settings) skipHeaderForward() {
	for attempts := 0; attempts < len(s.visible) && s.cursor < len(s.visible) && s.items[s.visible[s.cursor]].kind == "header"; attempts++ {
		s.cursor++
	}
	if s.cursor >= len(s.visible) {
		s.cursor = maxInt(0, len(s.visible)-1)
		// Try going backward if we went past the end
		for attempts := 0; attempts < len(s.visible) && s.cursor > 0 && s.items[s.visible[s.cursor]].kind == "header"; attempts++ {
			s.cursor--
		}
	}
}

func (s *Settings) skipHeaderBackward() {
	for attempts := 0; attempts < len(s.visible) && s.cursor > 0 && s.items[s.visible[s.cursor]].kind == "header"; attempts++ {
		s.cursor--
	}
}

// currentItem returns a pointer to the item at the current cursor position,
// or nil if there are no visible items or cursor is on a header.
func (s *Settings) currentItem() *settingItem {
	if len(s.visible) == 0 || s.cursor < 0 || s.cursor >= len(s.visible) {
		return nil
	}
	item := &s.items[s.visible[s.cursor]]
	if item.kind == "header" {
		return nil
	}
	return item
}

// defaultValueForKey returns the default value for a setting key.
func (s *Settings) defaultValueForKey(key string) interface{} {
	def := config.DefaultConfig()
	switch key {
	case "show_splash":
		return def.ShowSplash
	case "show_help":
		return def.ShowHelp
	case "line_numbers":
		return def.LineNumbers
	case "word_wrap":
		return def.WordWrap
	case "auto_save":
		return def.AutoSave
	case "default_view_mode":
		return def.DefaultViewMode
	case "vim_mode":
		return def.VimMode
	case "tab_size":
		return def.Editor.TabSize
	case "auto_close_brackets":
		return def.AutoCloseBrackets
	case "highlight_current_line":
		return def.HighlightCurrentLine
	case "theme":
		return def.Theme
	case "icon_theme":
		return def.IconTheme
	case "layout":
		return def.Layout
	case "view_style":
		return def.ViewStyle
	case "sidebar_position":
		return def.SidebarPosition
	case "show_icons":
		return def.ShowIcons
	case "compact_mode":
		return def.CompactMode
	case "sort_by":
		return def.SortBy
	case "auto_daily_note":
		return def.AutoDailyNote
	case "daily_notes_folder":
		return def.DailyNotesFolder
	case "weekly_notes_folder":
		return def.WeeklyNotesFolder
	case "weekly_note_template":
		return def.WeeklyNoteTemplate
	case "search_content":
		return def.SearchContentByDefault
	case "ai_provider":
		return def.AIProvider
	case "ollama_model":
		return def.OllamaModel
	case "ollama_url":
		return def.OllamaURL
	case "openai_key":
		return def.OpenAIKey
	case "openai_model":
		return def.OpenAIModel
	case "nous_url":
		return def.NousURL
	case "nous_api_key":
		return def.NousAPIKey
	case "background_bots":
		return def.BackgroundBots
	case "confirm_delete":
		return def.ConfirmDelete
	case "auto_refresh":
		return def.AutoRefresh
	case "git_auto_sync":
		return def.GitAutoSync
	case "auto_tag":
		return def.AutoTag
	case "ghost_writer":
		return def.GhostWriter
	case "spell_check":
		return def.SpellCheck
	case "semantic_search_enabled":
		return def.SemanticSearchEnabled
	case "task_filter_mode":
		return def.TaskFilterMode
	case "task_required_tags":
		return strings.Join(def.TaskRequiredTags, ", ")
	case "task_exclude_folders":
		return strings.Join(def.TaskExcludeFolders, ", ")
	case "task_exclude_done":
		return def.TaskExcludeDone
	case "pomodoro_goal":
		return def.PomodoroGoal
	default:
		if strings.HasPrefix(key, "cp_") {
			return true // core plugins default to enabled
		}
		return nil
	}
}

func (s *Settings) SetSize(width, height int) {
	s.width = width
	s.height = height
}

func (s *Settings) GetConfig() config.Config {
	return s.config
}

func (s *Settings) Toggle() {
	s.active = !s.active
	if s.active {
		s.searching = false
		s.searchBuf = ""
		s.scroll = 0
		s.cursor = 0
		s.buildItems()
		s.rebuildVisible()
	}
}

func (s *Settings) IsActive() bool {
	return s.active
}

func (s Settings) Update(msg tea.Msg) (Settings, tea.Cmd) {
	if !s.active {
		return s, nil
	}

	switch msg := msg.(type) {
	case ollamaSetupMsg:
		if msg.step == "done" {
			s.setupRunning = false
			if msg.success {
				s.setupStatus = "Setup complete! Ollama is ready."
				s.config.AIProvider = "ollama"
				s.buildItems()
				s.rebuildVisible()
			} else {
				s.setupStatus = "Setup failed: " + msg.message
			}
		} else {
			s.setupStatus = msg.message
		}
		return s, nil
	case tea.KeyMsg:
		// ── Search mode input ──
		if s.searching {
			switch msg.String() {
			case "esc":
				s.searching = false
				s.searchBuf = ""
				s.buildItems()
				s.rebuildVisible()
			case "enter":
				// Jump to first matching setting and exit search mode
				s.searching = false
				s.buildItems()
				s.rebuildVisible()
			case "backspace":
				if len(s.searchBuf) > 0 {
					s.searchBuf = s.searchBuf[:len(s.searchBuf)-1]
					s.cursor = 0
					s.buildItems()
					s.rebuildVisible()
				}
			default:
				char := msg.String()
				if len(char) == 1 {
					s.searchBuf += char
					s.cursor = 0
					s.buildItems()
					s.rebuildVisible()
				}
			}
			return s, nil
		}

		// ── Editing mode input ──
		if s.editing {
			switch msg.String() {
			case "esc":
				s.editing = false
				s.editBuf = ""
			case "enter":
				s.applyEdit()
				s.editing = false
				s.editBuf = ""
			case "backspace":
				if len(s.editBuf) > 0 {
					s.editBuf = s.editBuf[:len(s.editBuf)-1]
				}
			default:
				char := msg.String()
				if len(char) == 1 {
					s.editBuf += char
				}
			}
			return s, nil
		}

		switch msg.String() {
		case "esc", "ctrl+,":
			if s.searchBuf != "" {
				// Clear search first
				s.searchBuf = ""
				s.buildItems()
				s.rebuildVisible()
			} else {
				s.active = false
			}
			return s, nil
		case "/":
			s.searching = true
			s.searchBuf = ""
			s.editing = false
			s.editBuf = ""
			return s, nil
		case "up", "k":
			if s.cursor > 0 {
				s.cursor--
				s.skipHeaderBackward()
			}
		case "down", "j":
			if s.cursor < len(s.visible)-1 {
				s.cursor++
				s.skipHeaderForward()
			}
		case "backspace", "delete":
			// Reset current setting to default
			item := s.currentItem()
			if item != nil && item.kind != "action" && item.kind != "header" {
				defVal := s.defaultValueForKey(item.key)
				if defVal != nil {
					item.value = defVal
					s.applyValue(item.key, defVal)
				}
			}
		case "enter", " ":
			item := s.currentItem()
			if item == nil {
				break
			}
			switch item.kind {
			case "bool":
				val := item.value.(bool)
				item.value = !val
				s.applyValue(item.key, !val)
			case "string":
				if len(item.options) > 0 {
					// Cycle through options
					current := item.value.(string)
					for i, opt := range item.options {
						if opt == current {
							next := item.options[(i+1)%len(item.options)]
							item.value = next
							s.applyValue(item.key, next)
							break
						}
					}
				} else {
					s.editing = true
					if v, ok := item.value.(string); ok {
						s.editBuf = v
					}
				}
			case "int":
				s.editing = true
				s.editBuf = ""
			case "action":
				if item.key == "setup_ollama" && !s.setupRunning {
					s.setupRunning = true
					s.setupStatus = "Checking for Ollama..."
					return s, s.runOllamaSetup()
				}
			}
		}
	}
	return s, nil
}

func (s *Settings) runOllamaSetup() tea.Cmd {
	model := s.config.OllamaModel
	if model == "" {
		model = "qwen2.5:0.5b"
	}
	return func() tea.Msg {
		// Check if ollama is installed
		if _, err := exec.LookPath("ollama"); err != nil {
			// Try to install
			cmd := exec.Command("bash", "-c", "curl -fsSL https://ollama.com/install.sh | sh")
			if out, err := cmd.CombinedOutput(); err != nil {
				return ollamaSetupMsg{
					step:    "done",
					success: false,
					message: fmt.Sprintf("Failed to install Ollama: %v\n%s", err, string(out)),
				}
			}
		}

		// Pull the model
		cmd := exec.Command("ollama", "pull", model)
		if out, err := cmd.CombinedOutput(); err != nil {
			return ollamaSetupMsg{
				step:    "done",
				success: false,
				message: fmt.Sprintf("Failed to pull model %s: %v\n%s", model, err, string(out)),
			}
		}

		return ollamaSetupMsg{
			step:    "done",
			success: true,
			message: "Ollama installed with model " + model,
		}
	}
}

func (s *Settings) applyValue(key string, value interface{}) {
	switch key {
	case "show_splash":
		s.config.ShowSplash = value.(bool)
	case "show_help":
		s.config.ShowHelp = value.(bool)
	case "line_numbers":
		s.config.LineNumbers = value.(bool)
	case "word_wrap":
		s.config.WordWrap = value.(bool)
	case "auto_save":
		s.config.AutoSave = value.(bool)
	case "default_view_mode":
		s.config.DefaultViewMode = value.(bool)
	case "vim_mode":
		s.config.VimMode = value.(bool)
	case "sort_by":
		s.config.SortBy = value.(string)
	case "auto_daily_note":
		s.config.AutoDailyNote = value.(bool)
	case "daily_notes_folder":
		s.config.DailyNotesFolder = value.(string)
	case "weekly_notes_folder":
		s.config.WeeklyNotesFolder = value.(string)
	case "weekly_note_template":
		s.config.WeeklyNoteTemplate = value.(string)
	case "theme":
		s.config.Theme = value.(string)
		ApplyTheme(s.config.Theme)
		ApplyIconTheme(s.config.IconTheme)
	case "icon_theme":
		s.config.IconTheme = value.(string)
		ApplyIconTheme(s.config.IconTheme)
	case "layout":
		s.config.Layout = value.(string)
	case "view_style":
		s.config.ViewStyle = value.(string)
	case "search_content":
		s.config.SearchContentByDefault = value.(bool)
	case "auto_close_brackets":
		s.config.AutoCloseBrackets = value.(bool)
	case "highlight_current_line":
		s.config.HighlightCurrentLine = value.(bool)
	case "sidebar_position":
		s.config.SidebarPosition = value.(string)
	case "show_icons":
		s.config.ShowIcons = value.(bool)
	case "compact_mode":
		s.config.CompactMode = value.(bool)
	case "confirm_delete":
		s.config.ConfirmDelete = value.(bool)
	case "auto_refresh":
		s.config.AutoRefresh = value.(bool)
	case "ai_provider":
		s.config.AIProvider = value.(string)
	case "ollama_model":
		s.config.OllamaModel = value.(string)
	case "ollama_url":
		s.config.OllamaURL = value.(string)
	case "openai_key":
		s.config.OpenAIKey = value.(string)
	case "openai_model":
		s.config.OpenAIModel = value.(string)
	case "nous_url":
		s.config.NousURL = value.(string)
	case "nous_api_key":
		s.config.NousAPIKey = value.(string)
	case "background_bots":
		s.config.BackgroundBots = value.(bool)
	case "git_auto_sync":
		s.config.GitAutoSync = value.(bool)
	case "auto_tag":
		s.config.AutoTag = value.(bool)
	case "ghost_writer":
		s.config.GhostWriter = value.(bool)
	case "spell_check":
		s.config.SpellCheck = value.(bool)
	case "semantic_search_enabled":
		s.config.SemanticSearchEnabled = value.(bool)
	case "task_filter_mode":
		s.config.TaskFilterMode = value.(string)
	case "task_required_tags":
		s.config.TaskRequiredTags = splitCommaTags(value.(string))
	case "task_exclude_folders":
		s.config.TaskExcludeFolders = splitCommaTags(value.(string))
	case "disabled_calendars":
		s.config.DisabledCalendars = splitCommaTags(value.(string))
	case "task_exclude_done":
		s.config.TaskExcludeDone = value.(bool)
	case "pomodoro_goal":
		s.config.PomodoroGoal = value.(int)
	case "nextcloud_url":
		s.config.NextcloudURL = value.(string)
	case "nextcloud_user":
		s.config.NextcloudUser = value.(string)
	case "nextcloud_pass":
		s.config.NextcloudPass = value.(string)
	case "nextcloud_path":
		s.config.NextcloudPath = value.(string)
	case "nextcloud_auto_sync":
		s.config.NextcloudAutoSync = value.(bool)
	default:
		// Handle core plugin toggles (cp_*)
		if strings.HasPrefix(key, "cp_") {
			pluginName := strings.TrimPrefix(key, "cp_")
			if s.config.CorePlugins == nil {
				s.config.CorePlugins = config.DefaultCorePlugins()
			}
			s.config.CorePlugins[pluginName] = value.(bool)
		}
	}
}

func (s *Settings) applyEdit() {
	item := s.currentItem()
	if item == nil {
		return
	}
	switch item.kind {
	case "string":
		item.value = s.editBuf
		s.applyValue(item.key, s.editBuf)
	case "int":
		n := 0
		for _, ch := range s.editBuf {
			if ch >= '0' && ch <= '9' {
				n = n*10 + int(ch-'0')
			}
		}
		if n > 0 {
			item.value = n
			if item.key == "tab_size" {
				s.config.Editor.TabSize = n
			}
		}
	}
}

// themePreview renders a small color swatch for a theme name.
func themePreview(themeName string) string {
	t := GetTheme(themeName)
	swatch := func(c lipgloss.Color) string {
		return lipgloss.NewStyle().Background(c).Render("  ")
	}
	return swatch(t.Primary) + swatch(t.Secondary) + swatch(t.Accent) +
		swatch(t.Success) + swatch(t.Error) + swatch(t.Base)
}

func (s Settings) View() string {
	width := s.width * 3 / 4
	if width < 60 {
		width = 60
	}
	if width > 120 {
		width = 120
	}

	var b strings.Builder

	// Header
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconSettingsChar + " Settings")
	countLabel := DimStyle.Render(fmt.Sprintf("%d settings", len(s.visible)))
	b.WriteString(title)
	titleW := lipgloss.Width(title)
	countW := lipgloss.Width(countLabel)
	gap := width - 6 - titleW - countW
	if gap < 1 {
		gap = 1
	}
	b.WriteString(strings.Repeat(" ", gap) + countLabel)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	// Search bar
	if s.searching {
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  / ")
		input := s.searchBuf + lipgloss.NewStyle().Foreground(overlay0).Render("_")
		b.WriteString(prompt + input)
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
		b.WriteString("\n")
	} else if s.searchBuf != "" {
		filterInfo := DimStyle.Render(fmt.Sprintf("  filter: %q  (/ to search, Esc to clear)", s.searchBuf))
		b.WriteString(filterInfo)
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
		b.WriteString("\n")
	}

	b.WriteString("\n")

	// Calculate visible area
	extraLines := 0
	if s.searching || s.searchBuf != "" {
		extraLines = 2
	}
	visibleItems := s.height - 10 - extraLines
	if visibleItems < 5 {
		visibleItems = 5
	}

	start := 0
	if s.cursor >= visibleItems {
		start = s.cursor - visibleItems + 1
	}

	end := start + visibleItems
	if end > len(s.visible) {
		end = len(s.visible)
	}

	for vi := start; vi < end; vi++ {
		itemIdx := s.visible[vi]
		item := s.items[itemIdx]
		isSelected := vi == s.cursor

		label := item.label
		var valueStr string

		switch item.kind {
		case "header":
			headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
			lineStyle := lipgloss.NewStyle().Foreground(surface1)
			headerText := " " + label + " "
			lineLen := (width - 8 - len(headerText)) / 2
			if lineLen < 2 {
				lineLen = 2
			}
			b.WriteString(lineStyle.Render("  "+strings.Repeat("─", lineLen)) +
				headerStyle.Render(headerText) +
				lineStyle.Render(strings.Repeat("─", lineLen)))
			b.WriteString("\n")
			continue
		case "bool":
			if item.value.(bool) {
				valueStr = lipgloss.NewStyle().Foreground(green).Render("[ ON ]")
			} else {
				valueStr = lipgloss.NewStyle().Foreground(red).Render("[ OFF ]")
			}
			// Show reset indicator if value differs from default
			defVal := s.defaultValueForKey(item.key)
			if defVal != nil && defVal.(bool) != item.value.(bool) {
				valueStr += DimStyle.Render(" *")
			}
		case "string":
			if s.editing && isSelected {
				valueStr = s.editBuf + DimStyle.Render("_")
			} else if v, ok := item.value.(string); ok {
				if v == "" {
					valueStr = DimStyle.Render("(not set)")
				} else {
					valueStr = lipgloss.NewStyle().Foreground(blue).Render(v)
				}
				// Show reset indicator if value differs from default
				defVal := s.defaultValueForKey(item.key)
				if defVal != nil {
					if defStr, ok := defVal.(string); ok && defStr != v {
						valueStr += DimStyle.Render(" *")
					}
				}
			}
		case "int":
			if s.editing && isSelected {
				valueStr = s.editBuf + DimStyle.Render("_")
			} else {
				valueStr = lipgloss.NewStyle().Foreground(peach).Render(intToStr(item.value))
				defVal := s.defaultValueForKey(item.key)
				if defVal != nil {
					if defInt, ok := defVal.(int); ok {
						if curInt, ok2 := item.value.(int); ok2 && defInt != curInt {
							valueStr += DimStyle.Render(" *")
						}
					}
				}
			}
		case "action":
			if s.setupRunning && item.key == "setup_ollama" {
				valueStr = lipgloss.NewStyle().Foreground(yellow).Render("running...")
			} else {
				valueStr = lipgloss.NewStyle().Foreground(sapphire).Render("[run]")
			}
		}

		// Calculate padding
		labelWidth := width - 20
		if labelWidth < 20 {
			labelWidth = 20
		}
		if len(label) > labelWidth {
			label = label[:labelWidth]
		}
		padding := labelWidth - len(label)
		if padding < 1 {
			padding = 1
		}

		prefix := "  "
		if isSelected {
			prefix = lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("▶ ")
		}
		line := prefix + label + strings.Repeat(" ", padding+4) + valueStr

		if isSelected {
			b.WriteString(lipgloss.NewStyle().
				Background(surface0).
				Bold(true).
				Width(width - 6).
				Render(line))

			// Theme preview — show color swatches when the selected item is the theme setting
			if item.key == "theme" {
				if themeName, ok := item.value.(string); ok {
					b.WriteString("\n")
					preview := "    " + themePreview(themeName) + DimStyle.Render("  "+themeName)
					b.WriteString(preview)
				}
			}
		} else {
			b.WriteString(NormalItemStyle.Render(line))
		}
		b.WriteString("\n")
	}

	// No results message
	if len(s.visible) == 0 && s.searchBuf != "" {
		b.WriteString("\n  " + DimStyle.Render("No settings match your search.") + "\n")
	}

	// Scroll indicators
	if start > 0 {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  ... %d above", start)) + "\n")
	}
	if end < len(s.visible) {
		b.WriteString(DimStyle.Render(fmt.Sprintf("  ... %d below", len(s.visible)-end)) + "\n")
	}

	// Setup status
	if s.setupStatus != "" {
		b.WriteString("\n")
		statusColor := green
		if strings.Contains(s.setupStatus, "failed") || strings.Contains(s.setupStatus, "Failed") {
			statusColor = red
		} else if s.setupRunning {
			statusColor = yellow
		}
		b.WriteString(lipgloss.NewStyle().Foreground(statusColor).Render("  " + s.setupStatus))
	}

	// Selected item detail
	item := s.currentItem()
	if item != nil && item.kind != "header" && item.kind != "action" {
		defVal := s.defaultValueForKey(item.key)
		if defVal != nil {
			modified := false
			switch v := item.value.(type) {
			case bool:
				if dv, ok := defVal.(bool); ok {
					modified = v != dv
				}
			case string:
				if dv, ok := defVal.(string); ok {
					modified = v != dv
				}
			case int:
				if dv, ok := defVal.(int); ok {
					modified = v != dv
				}
			}
			if modified {
				defStr := fmt.Sprintf("%v", defVal)
				b.WriteString("\n  " + lipgloss.NewStyle().Foreground(yellow).Render("modified") +
					DimStyle.Render(" (default: "+defStr+")"))
			}
		}
	}

	// Help bar
	b.WriteString("\n\n")
	ks := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	ds := DimStyle
	var helpParts []string
	if s.searching {
		helpParts = []string{
			ds.Render("type to filter"),
			ks.Render("Enter") + ds.Render(":apply"),
			ks.Render("Esc") + ds.Render(":cancel"),
		}
	} else {
		helpParts = []string{
			ks.Render("j/k") + ds.Render(":nav"),
			ks.Render("Enter") + ds.Render(":edit"),
			ks.Render("/") + ds.Render(":search"),
			ks.Render("Del") + ds.Render(":reset"),
			ks.Render("Esc") + ds.Render(":close"),
		}
	}
	b.WriteString("  " + strings.Join(helpParts, "  "))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func intToStr(v interface{}) string {
	switch val := v.(type) {
	case int:
		if val == 0 {
			return "0"
		}
		s := ""
		n := val
		for n > 0 {
			s = string(rune('0'+n%10)) + s
			n /= 10
		}
		return s
	default:
		return "?"
	}
}

// splitCommaTags splits a comma-separated string into trimmed, non-empty tokens.
func splitCommaTags(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
