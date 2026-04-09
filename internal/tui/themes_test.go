package tui

import "testing"

func TestThemeNames_NotEmpty(t *testing.T) {
	names := ThemeNames()
	if len(names) == 0 {
		t.Fatal("expected at least one theme")
	}
}

func TestThemeNames_Sorted(t *testing.T) {
	names := ThemeNames()
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("themes not sorted: %q before %q", names[i-1], names[i])
		}
	}
}

func TestThemeNames_ContainsCatppuccin(t *testing.T) {
	names := ThemeNames()
	found := false
	for _, n := range names {
		if n == "catppuccin-mocha" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected catppuccin-mocha in theme names")
	}
}

func TestGetTheme_BuiltIn(t *testing.T) {
	theme := GetTheme("catppuccin-mocha")
	if theme.Name == "" {
		t.Error("expected non-empty theme name")
	}
	if theme.Primary == "" {
		t.Error("expected non-empty primary color")
	}
}

func TestGetTheme_FallbackToDefault(t *testing.T) {
	theme := GetTheme("nonexistent-theme")
	if theme.Name != "catppuccin-mocha" {
		t.Errorf("expected fallback to catppuccin-mocha, got %q", theme.Name)
	}
}

func TestApplyTheme_DoesNotPanic(t *testing.T) {
	// Apply all built-in themes to verify none panic
	for _, name := range ThemeNames() {
		ApplyTheme(name)
	}
	// Restore default
	ApplyTheme("catppuccin-mocha")
}
