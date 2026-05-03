package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. DefaultConfig
// ---------------------------------------------------------------------------

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("editor defaults", func(t *testing.T) {
		if cfg.Editor.TabSize != 4 {
			t.Errorf("TabSize = %d, want 4", cfg.Editor.TabSize)
		}
		if cfg.Editor.InsertTabs {
			t.Error("InsertTabs should be false by default")
		}
		if !cfg.Editor.AutoIndent {
			t.Error("AutoIndent should be true by default")
		}
	})

	t.Run("theme", func(t *testing.T) {
		if cfg.Theme != "catppuccin-mocha" {
			t.Errorf("Theme = %q, want %q", cfg.Theme, "catppuccin-mocha")
		}
	})

	t.Run("ai defaults", func(t *testing.T) {
		if cfg.AIProvider != "local" {
			t.Errorf("AIProvider = %q, want %q", cfg.AIProvider, "local")
		}
		if cfg.OllamaModel != "qwen2.5:0.5b" {
			t.Errorf("OllamaModel = %q, want %q", cfg.OllamaModel, "qwen2.5:0.5b")
		}
		if cfg.OllamaURL != "http://localhost:11434" {
			t.Errorf("OllamaURL = %q, want %q", cfg.OllamaURL, "http://localhost:11434")
		}
		if cfg.OpenAIModel != "gpt-4o-mini" {
			t.Errorf("OpenAIModel = %q, want %q", cfg.OpenAIModel, "gpt-4o-mini")
		}
		if cfg.OpenAIKey != "" {
			t.Error("OpenAIKey should be empty by default")
		}
	})

	t.Run("appearance defaults", func(t *testing.T) {
		if cfg.SidebarPosition != "left" {
			t.Errorf("SidebarPosition = %q, want %q", cfg.SidebarPosition, "left")
		}
		if !cfg.ShowIcons {
			t.Error("ShowIcons should be true by default")
		}
		if cfg.IconTheme != "unicode" {
			t.Errorf("IconTheme = %q, want %q", cfg.IconTheme, "unicode")
		}
		if cfg.Layout != "default" {
			t.Errorf("Layout = %q, want %q", cfg.Layout, "default")
		}
	})

	t.Run("behavior defaults", func(t *testing.T) {
		if cfg.AutoSave {
			t.Error("AutoSave should be false by default")
		}
		if !cfg.ShowSplash {
			t.Error("ShowSplash should be true by default")
		}
		if !cfg.LineNumbers {
			t.Error("LineNumbers should be true by default")
		}
		if !cfg.ConfirmDelete {
			t.Error("ConfirmDelete should be true by default")
		}
		if !cfg.AutoRefresh {
			t.Error("AutoRefresh should be true by default")
		}
		if cfg.VimMode {
			t.Error("VimMode should be false by default")
		}
		if !cfg.ShowHelp {
			t.Error("ShowHelp should be true by default")
		}
	})

	t.Run("search defaults", func(t *testing.T) {
		if !cfg.SearchContentByDefault {
			t.Error("SearchContentByDefault should be true by default")
		}
		if cfg.MaxSearchResults != 50 {
			t.Errorf("MaxSearchResults = %d, want 50", cfg.MaxSearchResults)
		}
	})

	t.Run("sort defaults", func(t *testing.T) {
		if cfg.SortBy != "name" {
			t.Errorf("SortBy = %q, want %q", cfg.SortBy, "name")
		}
	})
}

// ---------------------------------------------------------------------------
// 2. CorePluginEnabled
// ---------------------------------------------------------------------------

func TestCorePluginEnabled(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("known enabled plugin", func(t *testing.T) {
		if !cfg.CorePluginEnabled("calendar") {
			t.Error("calendar should be enabled")
		}
		if !cfg.CorePluginEnabled("canvas") {
			t.Error("canvas should be enabled")
		}
	})

	t.Run("unknown plugin defaults to enabled", func(t *testing.T) {
		if !cfg.CorePluginEnabled("totally_unknown_plugin") {
			t.Error("unknown plugin should default to enabled")
		}
	})

	t.Run("explicitly disabled plugin", func(t *testing.T) {
		cfg.CorePlugins["calendar"] = false
		if cfg.CorePluginEnabled("calendar") {
			t.Error("calendar was explicitly disabled, should be false")
		}
	})

	t.Run("nil core plugins map defaults to enabled", func(t *testing.T) {
		cfg2 := Config{CorePlugins: nil}
		if !cfg2.CorePluginEnabled("anything") {
			t.Error("nil CorePlugins map should default to enabled")
		}
	})
}

// ---------------------------------------------------------------------------
// 3. DefaultCorePlugins
// ---------------------------------------------------------------------------

func TestDefaultCorePlugins(t *testing.T) {
	plugins := DefaultCorePlugins()

	expectedPlugins := []string{
		"task_manager", "calendar", "canvas", "graph_view",
		"flashcards", "quiz_mode", "pomodoro", "git_integration",
		"blog_publisher", "ai_templates", "research_agent",
		"language_learning", "habit_tracker", "ghost_writer",
		"encryption", "spell_check",
	}

	if len(plugins) != 16 {
		t.Errorf("DefaultCorePlugins has %d entries, want 16", len(plugins))
	}

	for _, name := range expectedPlugins {
		enabled, exists := plugins[name]
		if !exists {
			t.Errorf("plugin %q missing from DefaultCorePlugins", name)
		} else if !enabled {
			t.Errorf("plugin %q should be true by default", name)
		}
	}
}

// ---------------------------------------------------------------------------
// 4. Config Save/Load roundtrip
// ---------------------------------------------------------------------------

func TestConfigSaveLoadRoundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	cfg := DefaultConfig()
	cfg.Theme = "tokyo-night"
	cfg.Editor.TabSize = 2
	cfg.VimMode = true
	cfg.AIProvider = "ollama"
	cfg.filePath = configPath

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Read back and verify
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config file: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if loaded.Theme != "tokyo-night" {
		t.Errorf("Theme = %q, want %q", loaded.Theme, "tokyo-night")
	}
	if loaded.Editor.TabSize != 2 {
		t.Errorf("TabSize = %d, want 2", loaded.Editor.TabSize)
	}
	if !loaded.VimMode {
		t.Error("VimMode should be true")
	}
	if loaded.AIProvider != "ollama" {
		t.Errorf("AIProvider = %q, want %q", loaded.AIProvider, "ollama")
	}
}

func TestConfigSaveCreatesIntermediateDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "a", "b", "c", "config.json")

	cfg := DefaultConfig()
	cfg.filePath = nestedPath

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Fatal("config file was not created in nested directory")
	}
}

func TestSaveToVault(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := DefaultConfig()
	cfg.Theme = "dracula"
	cfg.VimMode = true

	if err := cfg.SaveToVault(tmpDir); err != nil {
		t.Fatalf("SaveToVault failed: %v", err)
	}

	expectedPath := filepath.Join(tmpDir, ".granit.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Fatal(".granit.json was not created in vault root")
	}

	data, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("failed to read vault config: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal vault config: %v", err)
	}

	if loaded.Theme != "dracula" {
		t.Errorf("Theme = %q, want %q", loaded.Theme, "dracula")
	}
	if !loaded.VimMode {
		t.Error("VimMode should be true in vault config")
	}
}

// ---------------------------------------------------------------------------
// 5. LoadForVault overlays vault config on global config
// ---------------------------------------------------------------------------

func TestLoadForVault(t *testing.T) {
	// ConfigDir() uses os.UserHomeDir(), which honors HOME on Unix —
	// pointing it at a fresh tempdir gives us a clean global config
	// (defaults only) without the developer's real ~/.config/granit
	// bleeding in.
	t.Setenv("HOME", t.TempDir())

	vaultDir := t.TempDir()

	// Write a vault-specific config that overrides only Theme and VimMode.
	vaultCfg := map[string]interface{}{
		"theme":    "nord",
		"vim_mode": true,
	}
	data, err := json.Marshal(vaultCfg)
	if err != nil {
		t.Fatalf("failed to marshal vault config: %v", err)
	}
	vaultConfigPath := filepath.Join(vaultDir, ".granit.json")
	if err := os.WriteFile(vaultConfigPath, data, 0644); err != nil {
		t.Fatalf("failed to write vault config: %v", err)
	}

	cfg := LoadForVault(vaultDir)

	// Vault-specific overrides should apply.
	if cfg.Theme != "nord" {
		t.Errorf("Theme = %q, want %q (vault override)", cfg.Theme, "nord")
	}
	if !cfg.VimMode {
		t.Error("VimMode should be true (vault override)")
	}

	// Fields not in vault config should come from the global/default config.
	if cfg.AIProvider != "local" {
		t.Errorf("AIProvider = %q, want %q (from default)", cfg.AIProvider, "local")
	}
	if cfg.Editor.TabSize != 4 {
		t.Errorf("TabSize = %d, want 4 (from default)", cfg.Editor.TabSize)
	}
}

// ---------------------------------------------------------------------------
// 6. VaultList AddVault / RemoveVault / GetLastUsed
// ---------------------------------------------------------------------------

func TestVaultListAddRemoveGetLastUsed(t *testing.T) {
	t.Run("AddVault adds entry and sets LastUsed", func(t *testing.T) {
		var vl VaultList
		vl.AddVault("/tmp/test-vault-1")

		if len(vl.Vaults) != 1 {
			t.Fatalf("len(Vaults) = %d, want 1", len(vl.Vaults))
		}
		if vl.Vaults[0].Name != "test-vault-1" {
			t.Errorf("Name = %q, want %q", vl.Vaults[0].Name, "test-vault-1")
		}
		if vl.GetLastUsed() != vl.Vaults[0].Path {
			t.Errorf("GetLastUsed() = %q, want %q", vl.GetLastUsed(), vl.Vaults[0].Path)
		}
	})

	t.Run("AddVault multiple vaults", func(t *testing.T) {
		var vl VaultList
		vl.AddVault("/tmp/vault-a")
		vl.AddVault("/tmp/vault-b")

		if len(vl.Vaults) != 2 {
			t.Fatalf("len(Vaults) = %d, want 2", len(vl.Vaults))
		}
		// LastUsed should be the most recently added
		absB, _ := filepath.Abs("/tmp/vault-b")
		if vl.GetLastUsed() != absB {
			t.Errorf("GetLastUsed() = %q, want %q", vl.GetLastUsed(), absB)
		}
	})

	t.Run("RemoveVault removes entry", func(t *testing.T) {
		var vl VaultList
		vl.AddVault("/tmp/vault-x")

		absX, _ := filepath.Abs("/tmp/vault-x")
		vl.RemoveVault(absX)

		if len(vl.Vaults) != 0 {
			t.Errorf("len(Vaults) = %d, want 0 after removal", len(vl.Vaults))
		}
	})

	t.Run("RemoveVault clears LastUsed if it was removed", func(t *testing.T) {
		var vl VaultList
		vl.AddVault("/tmp/vault-y")

		absY, _ := filepath.Abs("/tmp/vault-y")
		vl.RemoveVault(absY)

		if vl.GetLastUsed() != "" {
			t.Errorf("GetLastUsed() = %q, want empty after removing last used", vl.GetLastUsed())
		}
	})

	t.Run("RemoveVault nonexistent path is a no-op", func(t *testing.T) {
		var vl VaultList
		vl.AddVault("/tmp/vault-keep")

		vl.RemoveVault("/tmp/does-not-exist")

		if len(vl.Vaults) != 1 {
			t.Errorf("len(Vaults) = %d, want 1 (no-op removal)", len(vl.Vaults))
		}
	})

	t.Run("GetLastUsed on empty list", func(t *testing.T) {
		var vl VaultList
		if vl.GetLastUsed() != "" {
			t.Errorf("GetLastUsed() = %q, want empty string", vl.GetLastUsed())
		}
	})
}

// ---------------------------------------------------------------------------
// 7. AddVault updates existing entry's LastOpen
// ---------------------------------------------------------------------------

func TestAddVaultUpdatesExistingEntry(t *testing.T) {
	var vl VaultList

	// Use t.TempDir for a real absolute path to avoid Abs() surprises.
	vaultPath := t.TempDir()

	vl.AddVault(vaultPath)
	if len(vl.Vaults) != 1 {
		t.Fatalf("len(Vaults) = %d, want 1", len(vl.Vaults))
	}

	originalLastOpen := vl.Vaults[0].LastOpen

	// Add the same vault again — should update, not duplicate.
	vl.AddVault(vaultPath)

	if len(vl.Vaults) != 1 {
		t.Fatalf("len(Vaults) = %d after re-add, want 1 (no duplicate)", len(vl.Vaults))
	}

	// LastOpen should be today's date (same day, so it won't change in practice,
	// but it should still be the formatted now).
	today := time.Now().Format("2006-01-02")
	if vl.Vaults[0].LastOpen != today {
		t.Errorf("LastOpen = %q, want %q", vl.Vaults[0].LastOpen, today)
	}

	// Original should match today (since both happen on the same day).
	if originalLastOpen != today {
		t.Errorf("originalLastOpen = %q, want %q", originalLastOpen, today)
	}
}

// ---------------------------------------------------------------------------
// 8. ObsidianThemeMapping
// ---------------------------------------------------------------------------

func TestObsidianThemeMapping(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"dracula", "dracula"},
		{"Dracula", "dracula"},
		{"  Dracula  ", "dracula"},
		{"nord", "nord"},
		{"Nord", "nord"},
		{"gruvbox", "gruvbox-dark"},
		{"tokyo night", "tokyo-night"},
		{"Tokyo Night", "tokyo-night"},
		{"california coast", "ayu-light"},
		{"solarized", "solarized-dark"},
		{"minimal", "catppuccin-mocha"},
		{"things", "catppuccin-mocha"},
		{"blue topaz", "nord"},
		// Unknown themes fall back to catppuccin-mocha.
		{"unknown-theme-xyz", "catppuccin-mocha"},
		{"", "catppuccin-mocha"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ObsidianThemeMapping(tt.input)
			if got != tt.want {
				t.Errorf("ObsidianThemeMapping(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 9. ImportObsidianConfig with mock .obsidian directory
// ---------------------------------------------------------------------------

func TestImportObsidianConfig(t *testing.T) {
	vaultDir := t.TempDir()
	obsDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsDir, 0755); err != nil {
		t.Fatalf("failed to create .obsidian dir: %v", err)
	}

	// Write app.json
	appJSON := map[string]interface{}{
		"vimMode":         true,
		"showLineNumber":  false,
		"spellcheck":      true,
		"tabSize":         2,
		"defaultViewMode": "preview",
	}
	writeTestJSON(t, filepath.Join(obsDir, "app.json"), appJSON)

	// Write appearance.json
	appearanceJSON := map[string]interface{}{
		"theme":    "obsidian",
		"cssTheme": "Dracula",
	}
	writeTestJSON(t, filepath.Join(obsDir, "appearance.json"), appearanceJSON)

	// Write daily-notes.json
	dailyJSON := map[string]interface{}{
		"folder":   "Daily",
		"template": "templates/daily",
	}
	writeTestJSON(t, filepath.Join(obsDir, "daily-notes.json"), dailyJSON)

	cfg := ImportObsidianConfig(vaultDir)

	if cfg == nil {
		t.Fatal("ImportObsidianConfig returned nil, expected a config")
	}

	t.Run("app.json settings", func(t *testing.T) {
		if !cfg.VimMode {
			t.Error("VimMode should be true")
		}
		if cfg.LineNumbers {
			t.Error("LineNumbers should be false (showLineNumber=false)")
		}
		if !cfg.SpellCheck {
			t.Error("SpellCheck should be true")
		}
		if cfg.Editor.TabSize != 2 {
			t.Errorf("TabSize = %d, want 2", cfg.Editor.TabSize)
		}
		if !cfg.DefaultViewMode {
			t.Error("DefaultViewMode should be true (preview mode)")
		}
	})

	t.Run("appearance.json settings", func(t *testing.T) {
		// CSS theme "Dracula" takes priority over base theme "obsidian".
		if cfg.Theme != "dracula" {
			t.Errorf("Theme = %q, want %q", cfg.Theme, "dracula")
		}
	})

	t.Run("daily-notes.json settings", func(t *testing.T) {
		if cfg.DailyNotesFolder != "Daily" {
			t.Errorf("DailyNotesFolder = %q, want %q", cfg.DailyNotesFolder, "Daily")
		}
		if cfg.DailyNoteTemplate != "templates/daily" {
			t.Errorf("DailyNoteTemplate = %q, want %q", cfg.DailyNoteTemplate, "templates/daily")
		}
	})
}

func TestImportObsidianConfigBaseThemeOnly(t *testing.T) {
	vaultDir := t.TempDir()
	obsDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsDir, 0755); err != nil {
		t.Fatalf("failed to create .obsidian dir: %v", err)
	}

	// appearance.json with only the base theme "moonstone" (light) — no cssTheme.
	appearanceJSON := map[string]interface{}{
		"theme": "moonstone",
	}
	writeTestJSON(t, filepath.Join(obsDir, "appearance.json"), appearanceJSON)

	cfg := ImportObsidianConfig(vaultDir)
	if cfg == nil {
		t.Fatal("ImportObsidianConfig returned nil")
	}

	if cfg.Theme != "catppuccin-latte" {
		t.Errorf("Theme = %q, want %q (moonstone base theme)", cfg.Theme, "catppuccin-latte")
	}
}

func TestImportObsidianConfigPartialAppJSON(t *testing.T) {
	vaultDir := t.TempDir()
	obsDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsDir, 0755); err != nil {
		t.Fatalf("failed to create .obsidian dir: %v", err)
	}

	// app.json with only vimMode set — other fields should remain as defaults.
	appJSON := map[string]interface{}{
		"vimMode": true,
	}
	writeTestJSON(t, filepath.Join(obsDir, "app.json"), appJSON)

	cfg := ImportObsidianConfig(vaultDir)
	if cfg == nil {
		t.Fatal("ImportObsidianConfig returned nil")
	}

	if !cfg.VimMode {
		t.Error("VimMode should be true")
	}
	// LineNumbers should remain at the default value (true).
	if !cfg.LineNumbers {
		t.Error("LineNumbers should stay at default true when not in app.json")
	}
	// TabSize should remain at default 4.
	if cfg.Editor.TabSize != 4 {
		t.Errorf("TabSize = %d, want 4 (default)", cfg.Editor.TabSize)
	}
}

func TestImportObsidianConfigDefaultViewModeSource(t *testing.T) {
	vaultDir := t.TempDir()
	obsDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsDir, 0755); err != nil {
		t.Fatalf("failed to create .obsidian dir: %v", err)
	}

	appJSON := map[string]interface{}{
		"defaultViewMode": "source",
	}
	writeTestJSON(t, filepath.Join(obsDir, "app.json"), appJSON)

	cfg := ImportObsidianConfig(vaultDir)
	if cfg == nil {
		t.Fatal("ImportObsidianConfig returned nil")
	}

	// "source" mode should NOT set DefaultViewMode to true.
	if cfg.DefaultViewMode {
		t.Error("DefaultViewMode should be false for 'source' mode")
	}
}

// ---------------------------------------------------------------------------
// 10. ImportObsidianConfig returns nil when no .obsidian exists
// ---------------------------------------------------------------------------

func TestImportObsidianConfigNoObsidianDir(t *testing.T) {
	vaultDir := t.TempDir()
	// No .obsidian directory created.
	cfg := ImportObsidianConfig(vaultDir)

	if cfg != nil {
		t.Error("ImportObsidianConfig should return nil when no .obsidian directory exists")
	}
}

func TestImportObsidianConfigObsidianIsFile(t *testing.T) {
	vaultDir := t.TempDir()
	// Create .obsidian as a file, not a directory.
	obsPath := filepath.Join(vaultDir, ".obsidian")
	if err := os.WriteFile(obsPath, []byte("not a dir"), 0644); err != nil {
		t.Fatalf("failed to create .obsidian file: %v", err)
	}

	cfg := ImportObsidianConfig(vaultDir)

	if cfg != nil {
		t.Error("ImportObsidianConfig should return nil when .obsidian is a file, not a directory")
	}
}

// ---------------------------------------------------------------------------
// 11. ImportReport
// ---------------------------------------------------------------------------

func TestImportReport(t *testing.T) {
	vaultDir := t.TempDir()
	obsDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsDir, 0755); err != nil {
		t.Fatalf("failed to create .obsidian dir: %v", err)
	}

	// Write app.json
	appJSON := map[string]interface{}{
		"vimMode":    true,
		"spellcheck": false,
		"tabSize":    8,
	}
	writeTestJSON(t, filepath.Join(obsDir, "app.json"), appJSON)

	// Write appearance.json with community theme
	appearanceJSON := map[string]interface{}{
		"theme":    "obsidian",
		"cssTheme": "Tokyo Night",
	}
	writeTestJSON(t, filepath.Join(obsDir, "appearance.json"), appearanceJSON)

	// Write daily-notes.json
	dailyJSON := map[string]interface{}{
		"folder": "journal",
	}
	writeTestJSON(t, filepath.Join(obsDir, "daily-notes.json"), dailyJSON)

	// Write hotkeys.json with 3 custom bindings
	hotkeysJSON := map[string]interface{}{
		"editor:save-file":     []map[string]interface{}{{"modifiers": []string{"Ctrl"}, "key": "S"}},
		"editor:toggle-bold":   []map[string]interface{}{{"modifiers": []string{"Ctrl"}, "key": "B"}},
		"app:go-back":          []map[string]interface{}{{"modifiers": []string{"Alt"}, "key": "ArrowLeft"}},
	}
	writeTestJSON(t, filepath.Join(obsDir, "hotkeys.json"), hotkeysJSON)

	// Write community-plugins.json with 2 plugins
	communityPlugins := []string{"dataview", "templater-obsidian"}
	writeTestJSON(t, filepath.Join(obsDir, "community-plugins.json"), communityPlugins)

	report := ImportReport(vaultDir)

	t.Run("contains header", func(t *testing.T) {
		if !strings.Contains(report, "Obsidian Import Report:") {
			t.Error("report should contain 'Obsidian Import Report:'")
		}
	})

	t.Run("contains vim mode", func(t *testing.T) {
		if !strings.Contains(report, "Vim mode: enabled") {
			t.Error("report should mention 'Vim mode: enabled'")
		}
	})

	t.Run("contains spell check", func(t *testing.T) {
		if !strings.Contains(report, "Spell check: disabled") {
			t.Error("report should mention 'Spell check: disabled'")
		}
	})

	t.Run("contains tab size", func(t *testing.T) {
		if !strings.Contains(report, "Tab size: 8") {
			t.Error("report should mention 'Tab size: 8'")
		}
	})

	t.Run("contains theme with community name", func(t *testing.T) {
		if !strings.Contains(report, "tokyo-night") {
			t.Errorf("report should mention resolved theme 'tokyo-night'")
		}
		if !strings.Contains(report, "Tokyo Night") {
			t.Errorf("report should mention original community theme 'Tokyo Night'")
		}
		if !strings.Contains(report, "community theme") {
			t.Error("report should indicate it came from a community theme")
		}
	})

	t.Run("contains daily notes folder", func(t *testing.T) {
		if !strings.Contains(report, "Daily notes folder: journal") {
			t.Error("report should mention 'Daily notes folder: journal'")
		}
	})

	t.Run("contains hotkey count", func(t *testing.T) {
		if !strings.Contains(report, "3 custom hotkeys found") {
			t.Error("report should mention '3 custom hotkeys found'")
		}
	})

	t.Run("contains plugin count", func(t *testing.T) {
		if !strings.Contains(report, "2 community plugins found") {
			t.Error("report should mention '2 community plugins found'")
		}
	})
}

func TestImportReportNoObsidianDir(t *testing.T) {
	vaultDir := t.TempDir()

	report := ImportReport(vaultDir)

	expected := "No .obsidian/ directory found — nothing to import."
	if report != expected {
		t.Errorf("report = %q, want %q", report, expected)
	}
}

func TestImportReportBaseThemeOnly(t *testing.T) {
	vaultDir := t.TempDir()
	obsDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsDir, 0755); err != nil {
		t.Fatalf("failed to create .obsidian dir: %v", err)
	}

	appearanceJSON := map[string]interface{}{
		"theme": "moonstone",
	}
	writeTestJSON(t, filepath.Join(obsDir, "appearance.json"), appearanceJSON)

	report := ImportReport(vaultDir)

	if !strings.Contains(report, "catppuccin-latte") {
		t.Error("report should contain 'catppuccin-latte' for moonstone base theme")
	}
	if !strings.Contains(report, "moonstone") {
		t.Error("report should mention the 'moonstone' base theme name")
	}
	if !strings.Contains(report, "base") {
		t.Error("report should indicate it came from a base theme")
	}
}

// ---------------------------------------------------------------------------
// 12. ConfigPath and VaultConfigPath
// ---------------------------------------------------------------------------

func TestConfigPath(t *testing.T) {
	path := ConfigPath()

	if !strings.HasSuffix(path, filepath.Join(".config", "granit", "config.json")) {
		t.Errorf("ConfigPath() = %q, expected suffix %q", path, filepath.Join(".config", "granit", "config.json"))
	}
}

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()

	if !strings.HasSuffix(dir, filepath.Join(".config", "granit")) {
		t.Errorf("ConfigDir() = %q, expected suffix %q", dir, filepath.Join(".config", "granit"))
	}
}

func TestVaultConfigPath(t *testing.T) {
	path := VaultConfigPath("/home/user/my-vault")
	expected := filepath.Join("/home/user/my-vault", ".granit.json")

	if path != expected {
		t.Errorf("VaultConfigPath() = %q, want %q", path, expected)
	}
}

func TestVaultConfigPathRelative(t *testing.T) {
	path := VaultConfigPath("some/relative/path")
	expected := filepath.Join("some/relative/path", ".granit.json")

	if path != expected {
		t.Errorf("VaultConfigPath() = %q, want %q", path, expected)
	}
}

// ---------------------------------------------------------------------------
// Additional edge case tests
// ---------------------------------------------------------------------------

func TestResolveTheme(t *testing.T) {
	tests := []struct {
		name      string
		baseTheme string
		cssTheme  string
		want      string
	}{
		{"css theme takes priority", "obsidian", "Dracula", "dracula"},
		{"moonstone base", "moonstone", "", "catppuccin-latte"},
		{"obsidian base", "obsidian", "", "catppuccin-mocha"},
		{"unknown base", "random", "", "catppuccin-mocha"},
		{"empty both", "", "", "catppuccin-mocha"},
		{"unknown css theme", "obsidian", "Some Unknown Theme", "catppuccin-mocha"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveTheme(tt.baseTheme, tt.cssTheme)
			if got != tt.want {
				t.Errorf("resolveTheme(%q, %q) = %q, want %q", tt.baseTheme, tt.cssTheme, got, tt.want)
			}
		})
	}
}

func TestLoadReturnsDefaultsWhenNoFileExists(t *testing.T) {
	// Load() reads from the user's home config path, but if no file exists
	// it should return defaults. Since we can't easily mock the path, we
	// just verify the returned config has sensible defaults.
	cfg := Load()

	// These should always be set from DefaultConfig.
	if cfg.Editor.TabSize == 0 {
		t.Error("TabSize should not be 0 after Load()")
	}
	if cfg.Theme == "" {
		t.Error("Theme should not be empty after Load()")
	}
}

func TestVaultListSaveLoadRoundtrip(t *testing.T) {
	// Note: SaveVaultList/LoadVaultList use ConfigDir() which depends on
	// os.UserHomeDir(). We test the data structures directly instead.
	var vl VaultList

	vaultPath := t.TempDir()
	vl.AddVault(vaultPath)
	vl.AddVault(t.TempDir())

	if len(vl.Vaults) != 2 {
		t.Fatalf("expected 2 vaults, got %d", len(vl.Vaults))
	}

	// Verify JSON roundtrip of the data structure.
	data, err := json.Marshal(vl)
	if err != nil {
		t.Fatalf("failed to marshal VaultList: %v", err)
	}

	var loaded VaultList
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal VaultList: %v", err)
	}

	if len(loaded.Vaults) != 2 {
		t.Fatalf("loaded %d vaults, want 2", len(loaded.Vaults))
	}
	if loaded.LastUsed != vl.LastUsed {
		t.Errorf("LastUsed = %q, want %q", loaded.LastUsed, vl.LastUsed)
	}
}

func TestRemoveVaultPreservesOtherEntries(t *testing.T) {
	var vl VaultList

	pathA := t.TempDir()
	pathB := t.TempDir()
	pathC := t.TempDir()

	vl.AddVault(pathA)
	vl.AddVault(pathB)
	vl.AddVault(pathC)

	if len(vl.Vaults) != 3 {
		t.Fatalf("expected 3 vaults, got %d", len(vl.Vaults))
	}

	// Remove the middle one.
	absB, _ := filepath.Abs(pathB)
	vl.RemoveVault(absB)

	if len(vl.Vaults) != 2 {
		t.Fatalf("expected 2 vaults after removal, got %d", len(vl.Vaults))
	}

	// Verify A and C still present.
	absA, _ := filepath.Abs(pathA)
	absC, _ := filepath.Abs(pathC)

	foundA, foundC := false, false
	for _, v := range vl.Vaults {
		if v.Path == absA {
			foundA = true
		}
		if v.Path == absC {
			foundC = true
		}
	}
	if !foundA {
		t.Error("vault A should still be present")
	}
	if !foundC {
		t.Error("vault C should still be present")
	}
}

func TestRemoveVaultDoesNotClearLastUsedIfDifferent(t *testing.T) {
	var vl VaultList

	pathA := t.TempDir()
	pathB := t.TempDir()

	vl.AddVault(pathA)
	vl.AddVault(pathB) // LastUsed = pathB

	absA, _ := filepath.Abs(pathA)
	absB, _ := filepath.Abs(pathB)

	// Remove A, which is NOT the last used.
	vl.RemoveVault(absA)

	if vl.GetLastUsed() != absB {
		t.Errorf("GetLastUsed() = %q, want %q (should not be cleared)", vl.GetLastUsed(), absB)
	}
}

func TestCountHotkeysAndPlugins(t *testing.T) {
	vaultDir := t.TempDir()
	obsDir := filepath.Join(vaultDir, ".obsidian")
	if err := os.MkdirAll(obsDir, 0755); err != nil {
		t.Fatalf("failed to create .obsidian dir: %v", err)
	}

	t.Run("no files returns zero", func(t *testing.T) {
		if got := countHotkeys(vaultDir); got != 0 {
			t.Errorf("countHotkeys = %d, want 0", got)
		}
		if got := countPlugins(vaultDir); got != 0 {
			t.Errorf("countPlugins = %d, want 0", got)
		}
	})

	t.Run("with files", func(t *testing.T) {
		hotkeys := map[string]interface{}{
			"a": "x",
			"b": "y",
		}
		writeTestJSON(t, filepath.Join(obsDir, "hotkeys.json"), hotkeys)

		plugins := []string{"p1", "p2", "p3"}
		writeTestJSON(t, filepath.Join(obsDir, "community-plugins.json"), plugins)

		if got := countHotkeys(vaultDir); got != 2 {
			t.Errorf("countHotkeys = %d, want 2", got)
		}
		if got := countPlugins(vaultDir); got != 3 {
			t.Errorf("countPlugins = %d, want 3", got)
		}
	})
}

func TestEnabledDisabled(t *testing.T) {
	if got := enabledDisabled(true); got != "enabled" {
		t.Errorf("enabledDisabled(true) = %q, want %q", got, "enabled")
	}
	if got := enabledDisabled(false); got != "disabled" {
		t.Errorf("enabledDisabled(false) = %q, want %q", got, "disabled")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeTestJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON for %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

// ---------------------------------------------------------------------------
// atomicWriteFile — temp+rename, no leftover .tmp on success
// ---------------------------------------------------------------------------

func TestAtomicWriteFile_LeavesNoTmp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := atomicWriteFile(path, []byte(`{"x":1}`), 0600); err != nil {
		t.Fatalf("atomicWriteFile failed: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(got) != `{"x":1}` {
		t.Errorf("unexpected content: %q", got)
	}

	// Ensure no .tmp file is left behind
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("expected .tmp to be cleaned up, stat err = %v", err)
	}
}

func TestAtomicWriteFile_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteFile(path, []byte("new"), 0600); err != nil {
		t.Fatalf("atomicWriteFile failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Errorf("expected 'new', got %q", got)
	}
}

// Regression: Save() must use atomic write so a crash mid-write
// cannot leave the user's settings file truncated.
func TestConfigSave_IsAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DefaultConfig()
	cfg.SetFilePath(path)
	cfg.Theme = "dracula"

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// No .tmp file should remain after a successful Save.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Errorf("Save left a .tmp file behind: %v", err)
	}

	// Reload and confirm the value round-tripped.
	cfg2 := DefaultConfig()
	cfg2.SetFilePath(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if cfg2.Theme != "dracula" {
		t.Errorf("expected theme 'dracula', got %q", cfg2.Theme)
	}
}
