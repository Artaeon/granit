package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/config"
)

// ── NewSettings ──

func TestNewSettings_InitialState(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	if s.IsActive() {
		t.Error("NewSettings should not be active initially")
	}
	// Cursor lands on first non-header item (headers are skipped)
	if len(s.visible) > 0 && s.items[s.visible[s.cursor]].kind == "header" {
		t.Error("cursor should not rest on a header initially")
	}
	if s.scroll != 0 {
		t.Errorf("scroll should be 0, got %d", s.scroll)
	}
	if s.editing {
		t.Error("should not be in editing mode")
	}
	if s.searching {
		t.Error("should not be in searching mode")
	}
}

func TestNewSettings_HasItems(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	if len(s.items) == 0 {
		t.Fatal("NewSettings should have settings items")
	}
	if len(s.visible) == 0 {
		t.Fatal("NewSettings should have visible items")
	}
}

// ── Toggle / IsActive ──

func TestSettings_ToggleActivation(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	if s.IsActive() {
		t.Error("should start inactive")
	}

	s.Toggle()
	if !s.IsActive() {
		t.Error("should be active after first Toggle")
	}

	s.Toggle()
	if s.IsActive() {
		t.Error("should be inactive after second Toggle")
	}
}

func TestSettings_ToggleResetsState(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	// Activate and modify state
	s.Toggle()
	s.cursor = 10
	s.scroll = 5
	s.searching = true
	s.searchBuf = "test"

	// Deactivate then reactivate
	s.Toggle()
	s.Toggle()

	// Cursor starts on first non-header item (skipHeaderForward from 0)
	if len(s.visible) > 0 && s.items[s.visible[s.cursor]].kind == "header" {
		t.Error("Toggle should place cursor on a non-header item")
	}
	if s.scroll != 0 {
		t.Errorf("Toggle should reset scroll to 0, got %d", s.scroll)
	}
	if s.searching {
		t.Error("Toggle should reset searching to false")
	}
	if s.searchBuf != "" {
		t.Errorf("Toggle should reset searchBuf to empty, got %q", s.searchBuf)
	}
}

// ── SetSize ──

func TestSettings_SetSize(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	s.SetSize(100, 50)
	if s.width != 100 {
		t.Errorf("width should be 100, got %d", s.width)
	}
	if s.height != 50 {
		t.Errorf("height should be 50, got %d", s.height)
	}

	s.SetSize(200, 80)
	if s.width != 200 {
		t.Errorf("width should be 200 after second SetSize, got %d", s.width)
	}
	if s.height != 80 {
		t.Errorf("height should be 80 after second SetSize, got %d", s.height)
	}
}

// ── Navigation ──

func TestSettings_NavigateDown(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()
	s.SetSize(120, 60)

	initialCursor := s.cursor

	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	if s.cursor <= initialCursor {
		t.Errorf("cursor should advance on 'j' key, was %d now %d", initialCursor, s.cursor)
	}
}

func TestSettings_NavigateUp(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()
	s.SetSize(120, 60)

	// Move down first
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	afterDown := s.cursor

	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})

	if s.cursor >= afterDown {
		t.Errorf("cursor should go back on 'k' key, was %d now %d", afterDown, s.cursor)
	}
}

func TestSettings_NavigateUpAtTop(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()
	s.SetSize(120, 60)

	startCursor := s.cursor

	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})

	// Cursor should not go negative
	if s.cursor < 0 {
		t.Errorf("cursor should not be negative, got %d", s.cursor)
	}
	// When already at top, cursor stays the same or at minimum valid position
	if s.cursor > startCursor {
		t.Errorf("cursor should not increase when pressing up at top, was %d now %d", startCursor, s.cursor)
	}
}

func TestSettings_NavigateDownWithArrowKey(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()
	s.SetSize(120, 60)

	initialCursor := s.cursor

	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})

	if s.cursor <= initialCursor {
		t.Errorf("cursor should advance on down arrow, was %d now %d", initialCursor, s.cursor)
	}
}

// ── Category filtering ──

func TestSettings_CategoriesPresent(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	expectedCategories := []string{"Appearance", "Editor", "AI", "Files", "Plugins", "Advanced"}
	categoryFound := make(map[string]bool)

	for _, item := range s.items {
		if item.kind != "header" && item.category != "" {
			categoryFound[item.category] = true
		}
	}

	for _, cat := range expectedCategories {
		if !categoryFound[cat] {
			t.Errorf("expected category %q to have items, but none found", cat)
		}
	}
}

func TestSettings_HeadersInVisibleList(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	headerCount := 0
	for _, idx := range s.visible {
		if s.items[idx].kind == "header" {
			headerCount++
		}
	}

	if headerCount == 0 {
		t.Error("visible list should contain category headers")
	}
}

func TestSettings_CursorSkipsHeaders(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()

	// After Toggle, cursor should not be on a header
	if len(s.visible) > 0 {
		item := s.items[s.visible[s.cursor]]
		if item.kind == "header" {
			t.Error("cursor should not rest on a header after Toggle")
		}
	}
}

// ── defaultValueForKey ──

func TestSettings_DefaultValueForKey_Theme(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("theme")
	if val != "catppuccin-mocha" {
		t.Errorf("default theme should be 'catppuccin-mocha', got %v", val)
	}
}

func TestSettings_DefaultValueForKey_VimMode(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("vim_mode")
	if val != false {
		t.Errorf("default vim_mode should be false, got %v", val)
	}
}

func TestSettings_DefaultValueForKey_WordWrap(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("word_wrap")
	if val != false {
		t.Errorf("default word_wrap should be false, got %v", val)
	}
}

func TestSettings_DefaultValueForKey_TabSize(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("tab_size")
	if val != 4 {
		t.Errorf("default tab_size should be 4, got %v", val)
	}
}

func TestSettings_DefaultValueForKey_LineNumbers(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("line_numbers")
	if val != true {
		t.Errorf("default line_numbers should be true, got %v", val)
	}
}

func TestSettings_DefaultValueForKey_ShowSplash(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("show_splash")
	if val != true {
		t.Errorf("default show_splash should be true, got %v", val)
	}
}

func TestSettings_DefaultValueForKey_AutoCloseBrackets(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("auto_close_brackets")
	if val != true {
		t.Errorf("default auto_close_brackets should be true, got %v", val)
	}
}

func TestSettings_DefaultValueForKey_AIProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("ai_provider")
	if val != "local" {
		t.Errorf("default ai_provider should be 'local', got %v", val)
	}
}

func TestSettings_DefaultValueForKey_SortBy(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("sort_by")
	if val != "name" {
		t.Errorf("default sort_by should be 'name', got %v", val)
	}
}

func TestSettings_DefaultValueForKey_CorePlugin(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("cp_calendar")
	if val != true {
		t.Errorf("default for core plugin cp_calendar should be true, got %v", val)
	}
}

func TestSettings_DefaultValueForKey_UnknownKey(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("nonexistent_key_xyz")
	if val != nil {
		t.Errorf("unknown key should return nil, got %v", val)
	}
}

func TestSettings_DefaultValueForKey_ConfirmDelete(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("confirm_delete")
	if val != true {
		t.Errorf("default confirm_delete should be true, got %v", val)
	}
}

func TestSettings_DefaultValueForKey_OllamaURL(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	val := s.defaultValueForKey("ollama_url")
	if val != "http://localhost:11434" {
		t.Errorf("default ollama_url should be 'http://localhost:11434', got %v", val)
	}
}

// ── applyValue ──

func TestSettings_ApplyValue_BoolToggle(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	// vim_mode defaults to false
	s.applyValue("vim_mode", true)
	if !s.config.VimMode {
		t.Error("applyValue should set VimMode to true")
	}

	s.applyValue("vim_mode", false)
	if s.config.VimMode {
		t.Error("applyValue should set VimMode to false")
	}
}

func TestSettings_ApplyValue_StringValue(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	s.applyValue("theme", "nord")
	if s.config.Theme != "nord" {
		t.Errorf("applyValue should set theme to 'nord', got %q", s.config.Theme)
	}
}

func TestSettings_ApplyValue_Layout(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	s.applyValue("layout", "writer")
	if s.config.Layout != "writer" {
		t.Errorf("applyValue should set layout to 'writer', got %q", s.config.Layout)
	}
}

func TestSettings_ApplyValue_SortBy(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	s.applyValue("sort_by", "modified")
	if s.config.SortBy != "modified" {
		t.Errorf("applyValue should set sort_by to 'modified', got %q", s.config.SortBy)
	}
}

func TestSettings_ApplyValue_AIProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	s.applyValue("ai_provider", "ollama")
	if s.config.AIProvider != "ollama" {
		t.Errorf("applyValue should set ai_provider to 'ollama', got %q", s.config.AIProvider)
	}
}

func TestSettings_ApplyValue_BoolSettings(t *testing.T) {
	tests := []struct {
		key   string
		check func(config.Config) bool
	}{
		{"show_splash", func(c config.Config) bool { return c.ShowSplash }},
		{"show_help", func(c config.Config) bool { return c.ShowHelp }},
		{"line_numbers", func(c config.Config) bool { return c.LineNumbers }},
		{"word_wrap", func(c config.Config) bool { return c.WordWrap }},
		{"auto_save", func(c config.Config) bool { return c.AutoSave }},
		{"auto_close_brackets", func(c config.Config) bool { return c.AutoCloseBrackets }},
		{"highlight_current_line", func(c config.Config) bool { return c.HighlightCurrentLine }},
		{"show_icons", func(c config.Config) bool { return c.ShowIcons }},
		{"compact_mode", func(c config.Config) bool { return c.CompactMode }},
		{"confirm_delete", func(c config.Config) bool { return c.ConfirmDelete }},
		{"auto_refresh", func(c config.Config) bool { return c.AutoRefresh }},
		{"background_bots", func(c config.Config) bool { return c.BackgroundBots }},
		{"git_auto_sync", func(c config.Config) bool { return c.GitAutoSync }},
		{"auto_tag", func(c config.Config) bool { return c.AutoTag }},
		{"ghost_writer", func(c config.Config) bool { return c.GhostWriter }},
		{"spell_check", func(c config.Config) bool { return c.SpellCheck }},
		{"semantic_search_enabled", func(c config.Config) bool { return c.SemanticSearchEnabled }},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			cfg := config.DefaultConfig()
			s := NewSettings(cfg)

			s.applyValue(tt.key, true)
			if !tt.check(s.config) {
				t.Errorf("applyValue(%q, true) should set config field to true", tt.key)
			}

			s.applyValue(tt.key, false)
			if tt.check(s.config) {
				t.Errorf("applyValue(%q, false) should set config field to false", tt.key)
			}
		})
	}
}

func TestSettings_ApplyValue_StringSettings(t *testing.T) {
	tests := []struct {
		key   string
		val   string
		check func(config.Config) string
	}{
		{"daily_notes_folder", "journal", func(c config.Config) string { return c.DailyNotesFolder }},
		{"weekly_notes_folder", "weekly", func(c config.Config) string { return c.WeeklyNotesFolder }},
		{"weekly_note_template", "tmpl.md", func(c config.Config) string { return c.WeeklyNoteTemplate }},
		{"sidebar_position", "right", func(c config.Config) string { return c.SidebarPosition }},
		{"icon_theme", "nerd", func(c config.Config) string { return c.IconTheme }},
		{"ollama_model", "llama3.2", func(c config.Config) string { return c.OllamaModel }},
		{"ollama_url", "http://example.com:11434", func(c config.Config) string { return c.OllamaURL }},
		{"openai_key", "sk-test123", func(c config.Config) string { return c.OpenAIKey }},
		{"openai_model", "gpt-4o", func(c config.Config) string { return c.OpenAIModel }},
		{"search_content", "", func(c config.Config) string { return "" }}, // skip, bool
	}

	for _, tt := range tests {
		if tt.key == "search_content" {
			continue // this is a bool, tested elsewhere
		}
		t.Run(tt.key, func(t *testing.T) {
			cfg := config.DefaultConfig()
			s := NewSettings(cfg)

			s.applyValue(tt.key, tt.val)
			got := tt.check(s.config)
			if got != tt.val {
				t.Errorf("applyValue(%q, %q): got %q", tt.key, tt.val, got)
			}
		})
	}
}

func TestSettings_ApplyValue_CorePlugin(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	s.applyValue("cp_calendar", false)
	if s.config.CorePlugins["calendar"] {
		t.Error("applyValue(cp_calendar, false) should disable calendar plugin")
	}

	s.applyValue("cp_calendar", true)
	if !s.config.CorePlugins["calendar"] {
		t.Error("applyValue(cp_calendar, true) should enable calendar plugin")
	}
}

func TestSettings_ApplyValue_CorePlugin_NilMap(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CorePlugins = nil
	s := NewSettings(cfg)

	// Should not panic and should create the map
	s.applyValue("cp_flashcards", false)
	if s.config.CorePlugins == nil {
		t.Fatal("applyValue should initialize CorePlugins map if nil")
	}
	if s.config.CorePlugins["flashcards"] {
		t.Error("flashcards should be disabled")
	}
}

// ── Search functionality ──

func TestSettings_SearchMode(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()
	s.SetSize(120, 60)

	// Enter search mode with '/'
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})

	if !s.searching {
		t.Error("pressing '/' should enter search mode")
	}
}

func TestSettings_SearchFiltering(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()
	s.SetSize(120, 60)

	totalVisible := len(s.visible)

	// Enter search mode and type "vim"
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})

	if len(s.visible) >= totalVisible {
		t.Errorf("search for 'vim' should reduce visible items from %d, got %d",
			totalVisible, len(s.visible))
	}

	// At least "Vim Mode" should be visible
	found := false
	for _, idx := range s.visible {
		item := s.items[idx]
		if item.kind != "header" && item.key == "vim_mode" {
			found = true
			break
		}
	}
	if !found {
		t.Error("search for 'vim' should include vim_mode setting")
	}
}

func TestSettings_SearchEscClearsSearch(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()
	s.SetSize(120, 60)

	// Enter search mode and type something
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})

	// Press Esc to clear search
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if s.searching {
		t.Error("Esc should exit search mode")
	}
	if s.searchBuf != "" {
		t.Errorf("Esc in search mode should clear searchBuf, got %q", s.searchBuf)
	}
}

func TestSettings_SearchBackspace(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()
	s.SetSize(120, 60)

	// Enter search and type "ab"
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")})

	if s.searchBuf != "ab" {
		t.Errorf("searchBuf should be 'ab', got %q", s.searchBuf)
	}

	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	if s.searchBuf != "a" {
		t.Errorf("after backspace, searchBuf should be 'a', got %q", s.searchBuf)
	}
}

// ── settingsFuzzyMatch ──

func TestSettingsFuzzyMatch_ExactMatch(t *testing.T) {
	if !settingsFuzzyMatch("vim mode", "vim mode") {
		t.Error("exact match should return true")
	}
}

func TestSettingsFuzzyMatch_Subsequence(t *testing.T) {
	if !settingsFuzzyMatch("vim mode", "vm") {
		t.Error("subsequence should match")
	}
}

func TestSettingsFuzzyMatch_NoMatch(t *testing.T) {
	if settingsFuzzyMatch("vim mode", "xyz") {
		t.Error("non-matching pattern should return false")
	}
}

func TestSettingsFuzzyMatch_EmptyPattern(t *testing.T) {
	if !settingsFuzzyMatch("anything", "") {
		t.Error("empty pattern should match everything")
	}
}

// ── GetConfig ──

func TestSettings_GetConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Theme = "nord"
	s := NewSettings(cfg)

	got := s.GetConfig()
	if got.Theme != "nord" {
		t.Errorf("GetConfig should return config with theme 'nord', got %q", got.Theme)
	}
}

func TestSettings_GetConfigReflectsChanges(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	s.applyValue("vim_mode", true)
	got := s.GetConfig()
	if !got.VimMode {
		t.Error("GetConfig should reflect changes made via applyValue")
	}
}

// ── currentItem ──

func TestSettings_CurrentItemNotNilAfterToggle(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()

	item := s.currentItem()
	if item == nil {
		t.Error("currentItem should not be nil after Toggle with default config")
	}
	if item != nil && item.kind == "header" {
		t.Error("currentItem should not return a header")
	}
}

// ── Esc closes settings ──

func TestSettings_EscClosesWhenNotSearching(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	s.Toggle()
	s.SetSize(120, 60)

	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if s.IsActive() {
		t.Error("Esc should close settings when not searching")
	}
}

// ── intToStr ──

func TestIntToStr(t *testing.T) {
	tests := []struct {
		input    interface{}
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{4, "4"},
		{42, "42"},
		{100, "100"},
		{"not int", "?"},
	}

	for _, tt := range tests {
		got := intToStr(tt.input)
		if got != tt.expected {
			t.Errorf("intToStr(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// ── Update when inactive ──

func TestSettings_UpdateWhenInactive(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)
	// Not toggled, so inactive

	s2, cmd := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	if cmd != nil {
		t.Error("Update on inactive settings should return nil cmd")
	}
	if s2.IsActive() {
		t.Error("Update on inactive settings should not activate it")
	}
}

// ---------------------------------------------------------------------------
// No duplicate keys in buildItems
// ---------------------------------------------------------------------------

func TestSettings_NoDuplicateItems(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	seen := make(map[string]bool)
	for _, item := range s.items {
		if item.kind == "header" {
			continue
		}
		if seen[item.key] {
			t.Errorf("duplicate settings key: %q", item.key)
		}
		seen[item.key] = true
	}
}

// ---------------------------------------------------------------------------
// Every category has at least one item
// ---------------------------------------------------------------------------

func TestSettings_AllCategoriesHaveItems(t *testing.T) {
	cfg := config.DefaultConfig()
	s := NewSettings(cfg)

	categoryItems := make(map[string]int)
	for _, item := range s.items {
		if item.kind == "header" {
			continue
		}
		if item.category != "" {
			categoryItems[item.category]++
		}
	}

	expectedCategories := []string{
		"Appearance", "Editor", "AI", "Files", "Plugins", "Advanced",
	}
	for _, cat := range expectedCategories {
		if categoryItems[cat] == 0 {
			t.Errorf("category %q has no items", cat)
		}
	}
}
