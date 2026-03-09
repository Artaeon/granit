package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfig_CorruptJSON(t *testing.T) {
	// Malformed JSON in a vault config file should not crash; the config
	// should still have default values (LoadForVault prints a warning to stderr).
	vaultDir := t.TempDir()
	vaultConfigPath := filepath.Join(vaultDir, ".granit.json")

	// Write corrupt JSON.
	if err := os.WriteFile(vaultConfigPath, []byte("{this is not valid json!!!}"), 0644); err != nil {
		t.Fatalf("failed to write corrupt config: %v", err)
	}

	cfg := LoadForVault(vaultDir)

	// Should have default values despite the corrupt vault config.
	if cfg.Theme == "" {
		t.Error("Theme should not be empty after loading corrupt config")
	}
	if cfg.Editor.TabSize == 0 {
		t.Error("TabSize should not be 0 after loading corrupt config")
	}
	if cfg.AIProvider != "local" {
		t.Errorf("AIProvider = %q, want %q (default)", cfg.AIProvider, "local")
	}
}

func TestConfig_EmptyFile(t *testing.T) {
	// An empty config file should result in defaults being used.
	vaultDir := t.TempDir()
	vaultConfigPath := filepath.Join(vaultDir, ".granit.json")

	// Write an empty file.
	if err := os.WriteFile(vaultConfigPath, []byte(""), 0644); err != nil {
		t.Fatalf("failed to write empty config: %v", err)
	}

	cfg := LoadForVault(vaultDir)

	// Defaults should still be applied (empty file fails JSON parse).
	if cfg.Theme == "" {
		t.Error("Theme should not be empty")
	}
	if cfg.Editor.TabSize == 0 {
		t.Error("TabSize should not be 0")
	}
}

func TestConfig_PartialConfig(t *testing.T) {
	// Only some fields set in the vault config; others should use defaults.
	vaultDir := t.TempDir()
	vaultConfigPath := filepath.Join(vaultDir, ".granit.json")

	partial := map[string]interface{}{
		"theme":    "nord",
		"vim_mode": true,
	}
	data, err := json.Marshal(partial)
	if err != nil {
		t.Fatalf("failed to marshal partial config: %v", err)
	}
	if err := os.WriteFile(vaultConfigPath, data, 0644); err != nil {
		t.Fatalf("failed to write partial config: %v", err)
	}

	cfg := LoadForVault(vaultDir)

	// Overridden fields.
	if cfg.Theme != "nord" {
		t.Errorf("Theme = %q, want %q", cfg.Theme, "nord")
	}
	if !cfg.VimMode {
		t.Error("VimMode should be true")
	}

	// Fields not in partial config should retain defaults.
	if cfg.Editor.TabSize != 4 {
		t.Errorf("TabSize = %d, want 4 (default)", cfg.Editor.TabSize)
	}
	if cfg.AIProvider != "local" {
		t.Errorf("AIProvider = %q, want %q (default)", cfg.AIProvider, "local")
	}
	if cfg.SidebarPosition != "left" {
		t.Errorf("SidebarPosition = %q, want %q (default)", cfg.SidebarPosition, "left")
	}
	if cfg.OllamaModel != "qwen2.5:0.5b" {
		t.Errorf("OllamaModel = %q, want %q (default)", cfg.OllamaModel, "qwen2.5:0.5b")
	}
	if !cfg.ShowHelp {
		t.Error("ShowHelp should be true (default)")
	}
	if !cfg.ConfirmDelete {
		t.Error("ConfirmDelete should be true (default)")
	}
	if cfg.MaxSearchResults != 50 {
		t.Errorf("MaxSearchResults = %d, want 50 (default)", cfg.MaxSearchResults)
	}
}

func TestConfig_SaveAndReload(t *testing.T) {
	// Save a config with custom values, reload it, verify all fields match.
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	original := DefaultConfig()
	original.filePath = configPath
	original.Theme = "dracula"
	original.Editor.TabSize = 8
	original.Editor.InsertTabs = true
	original.Editor.AutoIndent = false
	original.VimMode = true
	original.AutoSave = true
	original.LineNumbers = false
	original.WordWrap = true
	original.AIProvider = "openai"
	original.OllamaModel = "llama3"
	original.OllamaURL = "http://remote:11434"
	original.OpenAIKey = "sk-test-key-123"
	original.OpenAIModel = "gpt-4o"
	original.SidebarPosition = "right"
	original.IconTheme = "nerd"
	original.Layout = "writer"
	original.ShowSplash = false
	original.CompactMode = true
	original.DailyNotesFolder = "journal"
	original.DailyNoteTemplate = "templates/daily"
	original.SortBy = "modified"
	original.MaxSearchResults = 100
	original.ShowHiddenFiles = true
	original.SpellCheck = true
	original.GitAutoSync = true

	if err := original.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read back from file.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal saved config: %v", err)
	}

	// Verify all customized fields.
	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"Theme", loaded.Theme, "dracula"},
		{"TabSize", loaded.Editor.TabSize, 8},
		{"InsertTabs", loaded.Editor.InsertTabs, true},
		{"AutoIndent", loaded.Editor.AutoIndent, false},
		{"VimMode", loaded.VimMode, true},
		{"AutoSave", loaded.AutoSave, true},
		{"LineNumbers", loaded.LineNumbers, false},
		{"WordWrap", loaded.WordWrap, true},
		{"AIProvider", loaded.AIProvider, "openai"},
		{"OllamaModel", loaded.OllamaModel, "llama3"},
		{"OllamaURL", loaded.OllamaURL, "http://remote:11434"},
		{"OpenAIKey", loaded.OpenAIKey, "sk-test-key-123"},
		{"OpenAIModel", loaded.OpenAIModel, "gpt-4o"},
		{"SidebarPosition", loaded.SidebarPosition, "right"},
		{"IconTheme", loaded.IconTheme, "nerd"},
		{"Layout", loaded.Layout, "writer"},
		{"ShowSplash", loaded.ShowSplash, false},
		{"CompactMode", loaded.CompactMode, true},
		{"DailyNotesFolder", loaded.DailyNotesFolder, "journal"},
		{"DailyNoteTemplate", loaded.DailyNoteTemplate, "templates/daily"},
		{"SortBy", loaded.SortBy, "modified"},
		{"MaxSearchResults", loaded.MaxSearchResults, 100},
		{"ShowHiddenFiles", loaded.ShowHiddenFiles, true},
		{"SpellCheck", loaded.SpellCheck, true},
		{"GitAutoSync", loaded.GitAutoSync, true},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if c.got != c.want {
				t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
			}
		})
	}
}
