package tui

import (
	"testing"
)

func TestThemeToJSON_RoundTrip(t *testing.T) {
	original := GetTheme("catppuccin-mocha")
	jt := themeToJSON(original)
	restored := jsonToTheme(jt)

	if restored.Name != original.Name {
		t.Errorf("name mismatch: %q vs %q", restored.Name, original.Name)
	}
	if restored.Primary != original.Primary {
		t.Errorf("primary mismatch: %q vs %q", restored.Primary, original.Primary)
	}
	if restored.Border != original.Border {
		t.Errorf("border mismatch: %q vs %q", restored.Border, original.Border)
	}
}

func TestJsonToTheme_FallbackDefaults(t *testing.T) {
	// Empty JSON theme should fall back to catppuccin-mocha defaults
	jt := jsonTheme{Name: "empty-theme"}
	theme := jsonToTheme(jt)

	fb := GetTheme("catppuccin-mocha")
	if theme.Primary != fb.Primary {
		t.Errorf("expected fallback primary %q, got %q", fb.Primary, theme.Primary)
	}
	if theme.Border != "rounded" {
		t.Errorf("expected fallback border 'rounded', got %q", theme.Border)
	}
	if theme.Separator != "─" {
		t.Errorf("expected fallback separator, got %q", theme.Separator)
	}
}

func TestSaveAndLoadCustomTheme(t *testing.T) {
	dir := t.TempDir()

	theme := Theme{
		Name:    "test-custom",
		Primary: "#ff0000",
		Border:  "thick",
	}

	err := SaveCustomTheme(dir, theme)
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}

	loaded := LoadCustomThemes(dir)
	if _, ok := loaded["test-custom"]; !ok {
		t.Fatal("custom theme not found after load")
	}
	if loaded["test-custom"].Primary != "#ff0000" {
		t.Errorf("expected primary #ff0000, got %q", loaded["test-custom"].Primary)
	}
}

func TestLoadCustomThemes_EmptyDir(t *testing.T) {
	loaded := LoadCustomThemes(t.TempDir())
	if len(loaded) != 0 {
		t.Errorf("expected 0 themes for empty dir, got %d", len(loaded))
	}
}

func TestExportTheme(t *testing.T) {
	json := ExportTheme("catppuccin-mocha")
	if json == "" {
		t.Error("expected non-empty JSON export")
	}
	if len(json) < 100 {
		t.Error("expected substantial JSON content")
	}
}

func TestCustomThemeNames_InitiallyEmpty(t *testing.T) {
	// Custom themes are a global map, but CustomThemeNames should work
	names := CustomThemeNames()
	// May or may not have custom themes loaded, just ensure no panic
	_ = names
}
