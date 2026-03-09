package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	// Editor settings
	Editor EditorConfig `json:"editor"`

	// Appearance
	Theme    string `json:"theme"`
	ShowHelp bool   `json:"show_help"`

	// Vault settings
	DailyNotesFolder  string `json:"daily_notes_folder"`
	DailyNoteTemplate string `json:"daily_note_template"`

	// Editor enhancements
	AutoCloseBrackets    bool `json:"auto_close_brackets"`
	HighlightCurrentLine bool `json:"highlight_current_line"`

	// Appearance
	SidebarPosition string `json:"sidebar_position"` // "left" or "right"
	ShowIcons       bool   `json:"show_icons"`
	CompactMode     bool   `json:"compact_mode"`
	IconTheme       string `json:"icon_theme"` // "unicode", "nerd", "emoji", "ascii"
	Layout          string `json:"layout"`     // "default", "writer", "minimal", "reading", "dashboard"

	// Behavior
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
	AIProvider  string `json:"ai_provider"`  // "local", "ollama", "openai"
	OllamaModel string `json:"ollama_model"` // e.g. "qwen2.5:0.5b", "phi3:mini"
	OllamaURL   string `json:"ollama_url"`   // e.g. "http://localhost:11434"
	OpenAIKey   string `json:"openai_key"`   // API key for OpenAI
	OpenAIModel string `json:"openai_model"` // e.g. "gpt-4o-mini", "gpt-4o"
	BackgroundBots bool `json:"background_bots"` // auto-analyze on save
	AutoTag        bool `json:"auto_tag"`        // auto-tag notes on save
	GhostWriter    bool `json:"ghost_writer"`    // inline AI writing suggestions

	// Sidebar
	ShowHiddenFiles bool   `json:"show_hidden_files"`
	SortBy          string `json:"sort_by"` // "name", "modified", "created"

	// Search
	SearchContentByDefault bool `json:"search_content_by_default"`
	MaxSearchResults       int  `json:"max_search_results"`

	// Blog Publisher
	MediumToken  string `json:"medium_token,omitempty"`
	GitHubToken  string `json:"github_token,omitempty"`
	GitHubRepo   string `json:"github_repo,omitempty"`
	GitHubBranch string `json:"github_branch,omitempty"`

	// Tutorial
	TutorialCompleted bool `json:"tutorial_completed"`

	// Core Plugins — toggle built-in modules on/off
	CorePlugins map[string]bool `json:"core_plugins"`

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
		ShowHelp:               true,
		DailyNotesFolder:       "",
		DailyNoteTemplate:      "",
		AutoCloseBrackets:      true,
		HighlightCurrentLine:   true,
		SidebarPosition:        "left",
		ShowIcons:              true,
		CompactMode:            false,
		IconTheme:              "unicode",
		Layout:                 "default",
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
		BackgroundBots:         false,
		ShowHiddenFiles:        false,
		SortBy:                 "name",
		SearchContentByDefault: true,
		MaxSearchResults:       50,
		CorePlugins:            DefaultCorePlugins(),
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
		json.Unmarshal(data, &cfg)
	}
	cfg.filePath = globalPath

	return cfg
}

func LoadForVault(vaultRoot string) Config {
	cfg := Load()

	// Override with vault-specific config
	vaultPath := VaultConfigPath(vaultRoot)
	if data, err := os.ReadFile(vaultPath); err == nil {
		json.Unmarshal(data, &cfg)
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

	return os.WriteFile(path, data, 0600)
}

func (c Config) SaveToVault(vaultRoot string) error {
	path := VaultConfigPath(vaultRoot)

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}
