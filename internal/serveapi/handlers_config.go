package serveapi

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/artaeon/granit/internal/agentruntime"
	"github.com/artaeon/granit/internal/config"
)

// configView is the curated subset of config.Config we expose to the
// web. We deliberately don't ship every field — many TUI-specific
// settings (vim mode, sidebar position, etc.) don't have a web equivalent
// yet, and shipping them with no UI would suggest support that doesn't
// exist. Add fields here as the web learns to honor them.
//
// Secret fields (API keys, Nextcloud password) come back as a "set"/
// "unset" boolean rather than the value itself — never reflect a
// secret back to a UI that ships the data over a channel the user may
// inspect in devtools. Writing a non-empty string updates; writing an
// explicit empty string clears.
type configView struct {
	// AI / Bots
	AIProvider     string `json:"ai_provider"`
	OpenAIModel    string `json:"openai_model"`
	OpenAIKeySet   bool   `json:"openai_key_set"`
	AnthropicModel string `json:"anthropic_model"`
	AnthropicKeySet bool  `json:"anthropic_key_set"`
	OllamaURL      string `json:"ollama_url"`
	OllamaModel    string `json:"ollama_model"`
	AIAutoApplyEdits bool `json:"ai_auto_apply_edits"`
	AutoTag          bool `json:"auto_tag"`
	GhostWriter      bool `json:"ghost_writer"`
	BackgroundBots        bool `json:"background_bots"`
	SemanticSearchEnabled bool `json:"semantic_search_enabled"`

	// Daily / weekly
	DailyNotesFolder    string   `json:"daily_notes_folder"`
	DailyNoteTemplate   string   `json:"daily_note_template"`
	DailyRecurringTasks []string `json:"daily_recurring_tasks"`
	WeeklyNotesFolder   string   `json:"weekly_notes_folder"`
	WeeklyNoteTemplate  string   `json:"weekly_note_template"`
	AutoDailyNote       bool     `json:"auto_daily_note"`

	// Editor / appearance
	Theme        string `json:"theme"`
	AutoDarkMode bool   `json:"auto_dark_mode"`
	DarkTheme    string `json:"dark_theme"`
	LightTheme   string `json:"light_theme"`
	LineNumbers  bool   `json:"line_numbers"`
	WordWrap     bool   `json:"word_wrap"`
	AutoSave     bool   `json:"auto_save"`
	// Editor sub-config — surfaced flat so the web's settings UI
	// doesn't have to nest. Renamed to editor_* to avoid colliding
	// with top-level booleans like word_wrap.
	EditorTabSize         int  `json:"editor_tab_size"`
	EditorInsertTabs      bool `json:"editor_insert_tabs"`
	EditorAutoIndent      bool `json:"editor_auto_indent"`
	AutoCloseBrackets     bool `json:"auto_close_brackets"`
	HighlightCurrentLine  bool `json:"highlight_current_line"`

	// Tasks
	TaskFilterMode     string   `json:"task_filter_mode"`
	TaskRequiredTags   []string `json:"task_required_tags"`
	TaskExcludeFolders []string `json:"task_exclude_folders"`
	TaskExcludeDone    bool     `json:"task_exclude_done"`

	// Search
	SearchContentByDefault bool `json:"search_content_by_default"`
	MaxSearchResults       int  `json:"max_search_results"`

	// Sync
	GitAutoSync bool `json:"git_auto_sync"`

	// Pomodoro
	PomodoroGoal int `json:"pomodoro_goal"`

	// Kanban — surfaced read-only for the web kanban view so it can
	// honor user-defined columns / tag routing / WIP limits without
	// reaching for the raw config file.
	KanbanColumns    []string          `json:"kanban_columns"`
	KanbanColumnTags map[string]string `json:"kanban_column_tags"`
	KanbanColumnWIP  map[string]int    `json:"kanban_column_wip"`
}

// configPatch mirrors configView but every field is a pointer so the
// PATCH handler can tell "user explicitly set this to empty/false"
// from "user didn't include this field". Lets a single endpoint
// handle "update OpenAI model only" without clobbering the Anthropic
// settings.
type configPatch struct {
	AIProvider       *string   `json:"ai_provider,omitempty"`
	OpenAIKey        *string   `json:"openai_key,omitempty"` // empty string clears
	OpenAIModel      *string   `json:"openai_model,omitempty"`
	AnthropicKey     *string   `json:"anthropic_key,omitempty"`
	AnthropicModel   *string   `json:"anthropic_model,omitempty"`
	OllamaURL        *string   `json:"ollama_url,omitempty"`
	OllamaModel      *string   `json:"ollama_model,omitempty"`
	AIAutoApplyEdits      *bool `json:"ai_auto_apply_edits,omitempty"`
	AutoTag               *bool `json:"auto_tag,omitempty"`
	GhostWriter           *bool `json:"ghost_writer,omitempty"`
	BackgroundBots        *bool `json:"background_bots,omitempty"`
	SemanticSearchEnabled *bool `json:"semantic_search_enabled,omitempty"`

	DailyNotesFolder    *string   `json:"daily_notes_folder,omitempty"`
	DailyNoteTemplate   *string   `json:"daily_note_template,omitempty"`
	DailyRecurringTasks *[]string `json:"daily_recurring_tasks,omitempty"`
	WeeklyNotesFolder   *string   `json:"weekly_notes_folder,omitempty"`
	WeeklyNoteTemplate  *string   `json:"weekly_note_template,omitempty"`
	AutoDailyNote       *bool     `json:"auto_daily_note,omitempty"`

	Theme        *string `json:"theme,omitempty"`
	AutoDarkMode *bool   `json:"auto_dark_mode,omitempty"`
	DarkTheme    *string `json:"dark_theme,omitempty"`
	LightTheme   *string `json:"light_theme,omitempty"`
	LineNumbers  *bool   `json:"line_numbers,omitempty"`
	WordWrap     *bool   `json:"word_wrap,omitempty"`
	AutoSave     *bool   `json:"auto_save,omitempty"`
	// Editor sub-config — flat names match configView.
	EditorTabSize        *int  `json:"editor_tab_size,omitempty"`
	EditorInsertTabs     *bool `json:"editor_insert_tabs,omitempty"`
	EditorAutoIndent     *bool `json:"editor_auto_indent,omitempty"`
	AutoCloseBrackets    *bool `json:"auto_close_brackets,omitempty"`
	HighlightCurrentLine *bool `json:"highlight_current_line,omitempty"`

	TaskFilterMode     *string   `json:"task_filter_mode,omitempty"`
	TaskRequiredTags   *[]string `json:"task_required_tags,omitempty"`
	TaskExcludeFolders *[]string `json:"task_exclude_folders,omitempty"`
	TaskExcludeDone    *bool     `json:"task_exclude_done,omitempty"`

	SearchContentByDefault *bool `json:"search_content_by_default,omitempty"`
	MaxSearchResults       *int  `json:"max_search_results,omitempty"`

	GitAutoSync  *bool `json:"git_auto_sync,omitempty"`
	PomodoroGoal *int  `json:"pomodoro_goal,omitempty"`

	// Kanban — write paths so the web can save WIP limits / column edits
	// without sidestepping the config layer.
	KanbanColumns    *[]string          `json:"kanban_columns,omitempty"`
	KanbanColumnTags *map[string]string `json:"kanban_column_tags,omitempty"`
	KanbanColumnWIP  *map[string]int    `json:"kanban_column_wip,omitempty"`
}

func toView(c config.Config) configView {
	return configView{
		AIProvider:            c.AIProvider,
		OpenAIModel:           c.OpenAIModel,
		OpenAIKeySet:          strings.TrimSpace(c.OpenAIKey) != "",
		AnthropicModel:        c.AnthropicModel,
		AnthropicKeySet:       strings.TrimSpace(c.AnthropicKey) != "",
		OllamaURL:             c.OllamaURL,
		OllamaModel:           c.OllamaModel,
		AIAutoApplyEdits:      c.AIAutoApplyEdits,
		AutoTag:               c.AutoTag,
		GhostWriter:           c.GhostWriter,
		BackgroundBots:        c.BackgroundBots,
		SemanticSearchEnabled: c.SemanticSearchEnabled,

		DailyNotesFolder:    c.DailyNotesFolder,
		DailyNoteTemplate:   c.DailyNoteTemplate,
		DailyRecurringTasks: c.DailyRecurringTasks,
		WeeklyNotesFolder:   c.WeeklyNotesFolder,
		WeeklyNoteTemplate:  c.WeeklyNoteTemplate,
		AutoDailyNote:       c.AutoDailyNote,

		Theme:                c.Theme,
		AutoDarkMode:         c.AutoDarkMode,
		DarkTheme:            c.DarkTheme,
		LightTheme:           c.LightTheme,
		LineNumbers:          c.LineNumbers,
		WordWrap:             c.WordWrap,
		AutoSave:             c.AutoSave,
		EditorTabSize:        c.Editor.TabSize,
		EditorInsertTabs:     c.Editor.InsertTabs,
		EditorAutoIndent:     c.Editor.AutoIndent,
		AutoCloseBrackets:    c.AutoCloseBrackets,
		HighlightCurrentLine: c.HighlightCurrentLine,

		TaskFilterMode:    c.TaskFilterMode,
		TaskRequiredTags:  c.TaskRequiredTags,
		TaskExcludeFolders: c.TaskExcludeFolders,
		TaskExcludeDone:   c.TaskExcludeDone,

		SearchContentByDefault: c.SearchContentByDefault,
		MaxSearchResults:       c.MaxSearchResults,

		GitAutoSync:  c.GitAutoSync,
		PomodoroGoal: c.PomodoroGoal,

		KanbanColumns:    c.KanbanColumns,
		KanbanColumnTags: c.KanbanColumnTags,
		KanbanColumnWIP:  c.KanbanColumnWIP,
	}
}

// applyPatch mutates c in-place with whatever fields p specified. Pure
// shovelware — every field gets the same pointer-deref pattern.
func applyPatch(c *config.Config, p configPatch) {
	if p.AIProvider != nil {
		c.AIProvider = *p.AIProvider
	}
	if p.OpenAIKey != nil {
		c.OpenAIKey = *p.OpenAIKey
	}
	if p.OpenAIModel != nil {
		c.OpenAIModel = *p.OpenAIModel
	}
	if p.AnthropicKey != nil {
		c.AnthropicKey = *p.AnthropicKey
	}
	if p.AnthropicModel != nil {
		c.AnthropicModel = *p.AnthropicModel
	}
	if p.OllamaURL != nil {
		c.OllamaURL = *p.OllamaURL
	}
	if p.OllamaModel != nil {
		c.OllamaModel = *p.OllamaModel
	}
	if p.AIAutoApplyEdits != nil {
		c.AIAutoApplyEdits = *p.AIAutoApplyEdits
	}
	if p.AutoTag != nil {
		c.AutoTag = *p.AutoTag
	}
	if p.GhostWriter != nil {
		c.GhostWriter = *p.GhostWriter
	}
	if p.BackgroundBots != nil {
		c.BackgroundBots = *p.BackgroundBots
	}
	if p.SemanticSearchEnabled != nil {
		c.SemanticSearchEnabled = *p.SemanticSearchEnabled
	}
	if p.DailyNotesFolder != nil {
		c.DailyNotesFolder = *p.DailyNotesFolder
	}
	if p.DailyNoteTemplate != nil {
		c.DailyNoteTemplate = *p.DailyNoteTemplate
	}
	if p.DailyRecurringTasks != nil {
		c.DailyRecurringTasks = *p.DailyRecurringTasks
	}
	if p.WeeklyNotesFolder != nil {
		c.WeeklyNotesFolder = *p.WeeklyNotesFolder
	}
	if p.WeeklyNoteTemplate != nil {
		c.WeeklyNoteTemplate = *p.WeeklyNoteTemplate
	}
	if p.AutoDailyNote != nil {
		c.AutoDailyNote = *p.AutoDailyNote
	}
	if p.Theme != nil {
		c.Theme = *p.Theme
	}
	if p.AutoDarkMode != nil {
		c.AutoDarkMode = *p.AutoDarkMode
	}
	if p.DarkTheme != nil {
		c.DarkTheme = *p.DarkTheme
	}
	if p.LightTheme != nil {
		c.LightTheme = *p.LightTheme
	}
	if p.LineNumbers != nil {
		c.LineNumbers = *p.LineNumbers
	}
	if p.WordWrap != nil {
		c.WordWrap = *p.WordWrap
	}
	if p.AutoSave != nil {
		c.AutoSave = *p.AutoSave
	}
	if p.EditorTabSize != nil {
		c.Editor.TabSize = *p.EditorTabSize
	}
	if p.EditorInsertTabs != nil {
		c.Editor.InsertTabs = *p.EditorInsertTabs
	}
	if p.EditorAutoIndent != nil {
		c.Editor.AutoIndent = *p.EditorAutoIndent
	}
	if p.AutoCloseBrackets != nil {
		c.AutoCloseBrackets = *p.AutoCloseBrackets
	}
	if p.HighlightCurrentLine != nil {
		c.HighlightCurrentLine = *p.HighlightCurrentLine
	}
	if p.TaskFilterMode != nil {
		c.TaskFilterMode = *p.TaskFilterMode
	}
	if p.TaskRequiredTags != nil {
		c.TaskRequiredTags = *p.TaskRequiredTags
	}
	if p.TaskExcludeFolders != nil {
		c.TaskExcludeFolders = *p.TaskExcludeFolders
	}
	if p.TaskExcludeDone != nil {
		c.TaskExcludeDone = *p.TaskExcludeDone
	}
	if p.SearchContentByDefault != nil {
		c.SearchContentByDefault = *p.SearchContentByDefault
	}
	if p.MaxSearchResults != nil {
		c.MaxSearchResults = *p.MaxSearchResults
	}
	if p.GitAutoSync != nil {
		c.GitAutoSync = *p.GitAutoSync
	}
	if p.PomodoroGoal != nil {
		c.PomodoroGoal = *p.PomodoroGoal
	}
	if p.KanbanColumns != nil {
		c.KanbanColumns = *p.KanbanColumns
	}
	if p.KanbanColumnTags != nil {
		c.KanbanColumnTags = *p.KanbanColumnTags
	}
	if p.KanbanColumnWIP != nil {
		c.KanbanColumnWIP = *p.KanbanColumnWIP
	}
}

// handleGetConfig returns the curated config view for the web settings
// page. Reads from the merged config (global + vault overrides), so the
// web sees exactly what the TUI sees.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := config.LoadForVault(s.cfg.Vault.Root)
	writeJSON(w, http.StatusOK, toView(cfg))
}

// handlePatchConfig applies a partial update to whichever config file
// will actually be read by the next LoadForVault call — vault override
// when one exists, global config otherwise. The previous behavior
// (always write global) silently lost saves whenever a `.granit.json`
// existed because the vault override wins on read; the user clicked
// "Save", the file got written, the next GET returned the unchanged
// merged value, and the UI looked broken.
//
// Routing rule:
//   - <vault>/.granit.json exists → patch the vault override.
//   - otherwise                    → patch the global config.
//
// Either way the saved value is the one the web AND the TUI will see
// next time they read the merged config — single source of truth.
func (s *Server) handlePatchConfig(w http.ResponseWriter, r *http.Request) {
	var patch configPatch
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	// Pick the file whose value will actually win on the next read.
	// If a vault override exists, patch it (so saves take effect on
	// the next merged read). If not, patch the global config (so we
	// don't materialise a vault override and snowball the user's
	// sparse vault file into a copy of every default).
	vaultPath := config.VaultConfigPath(s.cfg.Vault.Root)
	var cfg config.Config
	if _, err := os.Stat(vaultPath); err == nil {
		cfg = config.LoadForVault(s.cfg.Vault.Root) // filePath = vaultPath
	} else {
		cfg = config.Load() // filePath = global
	}
	applyPatch(&cfg, patch)
	if err := cfg.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Read merged view back so the client sees what's effective.
	merged := config.LoadForVault(s.cfg.Vault.Root)
	writeJSON(w, http.StatusOK, toView(merged))
}

// handleListOpenAIModels returns the curated picker list defined in
// agentruntime.RecommendedOpenAIModels — keeps the settings UI from
// shipping a stale list of model IDs that no longer exist.
func (s *Server) handleListOpenAIModels(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"models": agentruntime.RecommendedOpenAIModels(),
	})
}
