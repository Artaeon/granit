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
	ShowMinimap          bool `json:"show_minimap"`

	// Appearance
	SidebarPosition string `json:"sidebar_position"` // "left" or "right"
	ShowIcons       bool   `json:"show_icons"`
	CompactMode     bool   `json:"compact_mode"`

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

	// Sidebar
	ShowHiddenFiles bool   `json:"show_hidden_files"`
	SortBy          string `json:"sort_by"` // "name", "modified", "created"

	// Search
	SearchContentByDefault bool `json:"search_content_by_default"`
	MaxSearchResults       int  `json:"max_search_results"`

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
		ShowMinimap:            false,
		SidebarPosition:        "left",
		ShowIcons:              true,
		CompactMode:            false,
		AutoSave:               false,
		ShowSplash:             true,
		VimMode:                false,
		LineNumbers:            true,
		WordWrap:               false,
		DefaultViewMode:        false,
		ConfirmDelete:          true,
		AutoRefresh:            true,
		SpellCheck:             false,
		ShowHiddenFiles:        false,
		SortBy:                 "name",
		SearchContentByDefault: true,
		MaxSearchResults:       50,
	}
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

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (c Config) SaveToVault(vaultRoot string) error {
	path := VaultConfigPath(vaultRoot)

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
