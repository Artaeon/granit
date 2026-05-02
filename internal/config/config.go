package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	// Editor settings
	Editor EditorConfig `json:"editor"`

	// Appearance
	Theme        string `json:"theme"`
	AutoDarkMode bool   `json:"auto_dark_mode"`
	DarkTheme    string `json:"dark_theme"`
	LightTheme   string `json:"light_theme"`
	ShowHelp     bool   `json:"show_help"`

	// Calendar
	DisabledCalendars []string `json:"disabled_calendars"`


	// Vault settings
	DailyNotesFolder    string   `json:"daily_notes_folder"`
	DailyNoteTemplate   string   `json:"daily_note_template"`
	DailyRecurringTasks []string `json:"daily_recurring_tasks"`
	WeeklyNotesFolder   string   `json:"weekly_notes_folder"`
	WeeklyNoteTemplate  string   `json:"weekly_note_template"`

	// RepoScanRoot is the absolute path the Repo Tracker scans for
	// local git repositories (each subdirectory containing a `.git`
	// becomes a row). Empty means "skip the scan" — the tracker
	// renders an empty hint asking the user to set this. Default
	// resolves to ~/Projects when unset on first launch.
	RepoScanRoot string `json:"repo_scan_root"`

	// Editor enhancements
	AutoCloseBrackets    bool `json:"auto_close_brackets"`
	HighlightCurrentLine bool `json:"highlight_current_line"`

	// Appearance
	SidebarPosition string `json:"sidebar_position"` // "left" or "right"
	ShowIcons       bool   `json:"show_icons"`
	CompactMode     bool   `json:"compact_mode"`
	IconTheme       string `json:"icon_theme"` // "unicode", "nerd", "emoji", "ascii"
	Layout          string `json:"layout"`     // "default", "writer", "minimal", "reading", "dashboard"
	ViewStyle       string `json:"view_style"` // "default", "reading", "minimal"

	// Behavior
	AutoDailyNote   bool `json:"auto_daily_note"`
	AutoSave        bool `json:"auto_save"`
	ShowSplash      bool `json:"show_splash"`
	VimMode         bool `json:"vim_mode"`
	LineNumbers     bool `json:"line_numbers"`
	WordWrap        bool `json:"word_wrap"`
	DefaultViewMode bool `json:"default_view_mode"`
	ConfirmDelete   bool `json:"confirm_delete"`
	AutoRefresh     bool `json:"auto_refresh"`
	SpellCheck      bool `json:"spell_check"`
	GitAutoSync     bool `json:"git_auto_sync"`

	// AI / Bots
	AIProvider  string `json:"ai_provider"`  // "local", "ollama", "openai", "anthropic", "nous", "nerve"
	OllamaModel string `json:"ollama_model"` // e.g. "qwen2.5:0.5b", "phi3:mini"
	OllamaURL   string `json:"ollama_url"`   // e.g. "http://localhost:11434"
	OpenAIKey   string `json:"openai_key"`   // API key for OpenAI
	OpenAIModel string `json:"openai_model"` // e.g. "gpt-4o-mini", "gpt-4o"
	AnthropicKey   string `json:"anthropic_key"`   // API key for Anthropic Claude
	AnthropicModel string `json:"anthropic_model"` // e.g. "claude-haiku-4-5", "claude-sonnet-4-6"
	NousURL     string `json:"nous_url"`     // e.g. "http://localhost:3333"
	NousAPIKey  string `json:"nous_api_key"` // optional API key for Nous
	NerveBinary string `json:"nerve_binary"` // path to nerve binary (default: "nerve")
	NerveModel  string `json:"nerve_model"`  // model for nerve (e.g. "sonnet", "llama3.2")
	NerveProvider string `json:"nerve_provider"` // nerve's internal provider (e.g. "ollama", "claude_code")
	BackgroundBots        bool `json:"background_bots"`         // auto-analyze on save
	AutoTag               bool `json:"auto_tag"`                // auto-tag notes on save
	GhostWriter           bool `json:"ghost_writer"`            // inline AI writing suggestions
	SemanticSearchEnabled bool `json:"semantic_search_enabled"` // background embedding index for semantic search
	// AIAutoApplyEdits skips the diff-preview overlay for inline AI
	// edits (rewrite/expand/summarize/improve/shorten/fix). Defaults
	// to false — the preview is the safer UX. Power users who trust
	// the model can flip it on for one-keystroke edits.
	AIAutoApplyEdits bool `json:"ai_auto_apply_edits"`

	// Sidebar
	ShowHiddenFiles bool   `json:"show_hidden_files"`
	SortBy          string `json:"sort_by"` // "name", "modified", "created"

	// Search
	SearchContentByDefault bool `json:"search_content_by_default"`
	MaxSearchResults       int  `json:"max_search_results"`

	// Nextcloud Sync
	NextcloudURL      string `json:"nextcloud_url"`
	NextcloudUser     string `json:"nextcloud_user"`
	NextcloudPass     string `json:"nextcloud_pass"`
	NextcloudPath     string `json:"nextcloud_path"`
	NextcloudAutoSync bool   `json:"nextcloud_auto_sync"`

	// Task Manager
	TaskFilterMode     string   `json:"task_filter_mode"`      // "all", "tagged", "folders"
	TaskRequiredTags   []string `json:"task_required_tags"`    // e.g. ["task", "todo"] — only show tasks with these tags
	TaskExcludeFolders []string `json:"task_exclude_folders"`  // e.g. ["Archive/", "templates/"]
	TaskExcludeDone    bool     `json:"task_exclude_done"`     // hide completed tasks

	// Focus / Pomodoro
	PomodoroGoal int `json:"pomodoro_goal"` // daily session target, default 8

	// Blog Publisher
	MediumToken  string `json:"medium_token,omitempty"`
	GitHubToken  string `json:"github_token,omitempty"`
	GitHubRepo   string `json:"github_repo,omitempty"`
	GitHubBranch string `json:"github_branch,omitempty"`

	// Tutorial
	TutorialCompleted bool `json:"tutorial_completed"`

	// Kanban board configuration
	KanbanColumns    []string          `json:"kanban_columns,omitempty"`     // column names (default: Backlog, In Progress, Done)
	KanbanColumnTags map[string]string `json:"kanban_column_tags,omitempty"` // column -> tag list (e.g. "In Progress": "#doing,#wip")
	KanbanColumnWIP  map[string]int    `json:"kanban_column_wip,omitempty"`  // column -> max in-progress count (0 / unset = no limit)

	// Core Plugins — toggle built-in modules on/off
	CorePlugins map[string]bool `json:"core_plugins"`

	// UseTaskStore opts into the unified TaskStore (Phase 2 of the
	// relaunch). On by default — the store is the canonical task
	// layer, with stable IDs in .granit/tasks-meta.json glued to
	// the markdown via fingerprint reconciliation. Setting this
	// to false reverts to the legacy markdown-only path and
	// granit will ignore the sidecar (the file stays on disk for
	// when you flip back).
	UseTaskStore bool `json:"use_task_store"`

	// UseProfiles opts into the Profiles + Daily Hub system
	// (Phase 3 of the relaunch). On by default — the active
	// profile (read from <vault>/.granit/active-profile, default
	// "classic") drives which modules are enabled and which
	// layout boots, and Alt+H opens the new widget-grid Daily
	// Hub instead of the legacy dashboard.go overlay. Setting
	// this to false reverts to the pre-Phase-3 boot path
	// (legacy dashboard, no profile switching, all modules
	// stay in their last-known enabled state).
	UseProfiles bool `json:"use_profiles"`

	// File path (not serialized)
	filePath string `json:"-"`
}

type EditorConfig struct {
	TabSize    int  `json:"tab_size"`
	InsertTabs bool `json:"insert_tabs"`
	AutoIndent bool `json:"auto_indent"`
}

func DefaultConfig() Config {
	return Config{
		Editor: EditorConfig{
			TabSize:    4,
			InsertTabs: false,
			AutoIndent: true,
		},
		Theme:                  "catppuccin-mocha",
		AutoDarkMode:           false,
		DarkTheme:              "catppuccin-mocha",
		LightTheme:             "catppuccin-latte",
		ShowHelp:               true,
		GitAutoSync:            true,
		DailyNotesFolder:       "",
		DailyNoteTemplate:      "",
		DailyRecurringTasks:    []string{},
		WeeklyNotesFolder:      "",
		WeeklyNoteTemplate:     "",
		AutoCloseBrackets:      true,
		HighlightCurrentLine:   true,
		SidebarPosition:        "left",
		ShowIcons:              true,
		CompactMode:            false,
		IconTheme:              "unicode",
		Layout:                 "default",
		ViewStyle:              "default",
		AutoSave:               false,
		ShowSplash:             true,
		VimMode:                false,
		LineNumbers:            true,
		WordWrap:               false,
		DefaultViewMode:        false,
		ConfirmDelete:          true,
		AutoRefresh:            true,
		SpellCheck:             false,
		AIProvider:             "local",
		OllamaModel:            "qwen2.5:0.5b",
		OllamaURL:              "http://localhost:11434",
		OpenAIKey:              "",
		OpenAIModel:            "gpt-4o-mini",
		AnthropicKey:           "",
		AnthropicModel:         "claude-haiku-4-5",
		NousURL:                "http://localhost:3333",
		NousAPIKey:             "",
		NerveBinary:            "nerve",
		NerveModel:             "",
		NerveProvider:          "",
		BackgroundBots:         false,
		ShowHiddenFiles:        false,
		SortBy:                 "name",
		SearchContentByDefault: true,
		MaxSearchResults:       50,
		TaskFilterMode:         "tagged",
		TaskRequiredTags:       []string{"task", "todo"},
		TaskExcludeFolders:     []string{},
		TaskExcludeDone:        false,
		PomodoroGoal:           8,
		CorePlugins:            DefaultCorePlugins(),
		UseTaskStore:           true,
		UseProfiles:            true,
	}
}

// DefaultCorePlugins returns the default set of core plugins, all enabled.
func DefaultCorePlugins() map[string]bool {
	return map[string]bool{
		"task_manager":      true,
		"calendar":          true,
		"canvas":            true,
		"graph_view":        true,
		"flashcards":        true,
		"quiz_mode":         true,
		"pomodoro":          true,
		"git_integration":   true,
		"blog_publisher":    true,
		"ai_templates":      true,
		"research_agent":    true,
		"language_learning": true,
		"habit_tracker":     true,
		"ghost_writer":      true,
		"encryption":        true,
		"spell_check":       true,
	}
}

// CorePluginEnabled returns whether a core plugin is enabled.
// Returns true if the plugin is not in the map (default enabled).
func (c Config) CorePluginEnabled(name string) bool {
	if c.CorePlugins == nil {
		return true
	}
	enabled, exists := c.CorePlugins[name]
	if !exists {
		return true // default to enabled for unknown plugins
	}
	return enabled
}

// SetFilePath overrides the path used by Save.
func (c *Config) SetFilePath(path string) {
	c.filePath = path
}

// GetFilePath returns the path that Save will write to.
func (c *Config) GetFilePath() string {
	return c.filePath
}

func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(home, ".config", "granit")
}

func ConfigPath() string {
	return filepath.Join(ConfigDir(), "config.json")
}

func VaultConfigPath(vaultRoot string) string {
	return filepath.Join(vaultRoot, ".granit.json")
}

func Load() Config {
	cfg := DefaultConfig()

	// Load global config
	globalPath := ConfigPath()
	if data, err := os.ReadFile(globalPath); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v (using defaults)\n", globalPath, err)
		}
	}
	cfg.filePath = globalPath

	return cfg
}

func LoadForVault(vaultRoot string) Config {
	cfg := Load()

	// Override with vault-specific config
	vaultPath := VaultConfigPath(vaultRoot)
	if data, err := os.ReadFile(vaultPath); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to parse %s: %v (using defaults)\n", vaultPath, err)
		}
	}
	cfg.filePath = vaultPath

	return cfg
}

func (c Config) Save() error {
	path := c.filePath
	if path == "" {
		path = ConfigPath()
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return atomicWriteFile(path, data, 0600)
}

func (c Config) SaveToVault(vaultRoot string) error {
	path := VaultConfigPath(vaultRoot)

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	return atomicWriteFile(path, data, 0600)
}

// atomicWriteFile writes data to path atomically by first writing to a
// temporary file in the same directory, then renaming it into place. This
// prevents leaving a partial or zero-byte file on disk if the process is
// interrupted (crash, power loss, OOM kill) mid-write.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
