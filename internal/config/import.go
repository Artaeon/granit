package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// obsidianApp represents the relevant fields from .obsidian/app.json.
type obsidianApp struct {
	VimMode         *bool   `json:"vimMode"`
	ShowLineNumber  *bool   `json:"showLineNumber"`
	SpellCheck      *bool   `json:"spellcheck"`
	TabSize         *int    `json:"tabSize"`
	DefaultViewMode *string `json:"defaultViewMode"`
}

// obsidianAppearance represents the relevant fields from .obsidian/appearance.json.
type obsidianAppearance struct {
	Theme        *string `json:"theme"`
	CSSTheme     *string `json:"cssTheme"`
	BaseFontSize *int    `json:"baseFontSize"`
	AccentColor  *string `json:"accentColor"`
}

// obsidianDailyNotes represents the relevant fields from .obsidian/daily-notes.json.
type obsidianDailyNotes struct {
	Folder   *string `json:"folder"`
	Format   *string `json:"format"`
	Template *string `json:"template"`
}

// ObsidianThemeMapping maps Obsidian community theme names to Granit theme names.
// If the theme is not recognized, "catppuccin-mocha" is returned as the default.
func ObsidianThemeMapping(obsidianTheme string) string {
	normalized := strings.ToLower(strings.TrimSpace(obsidianTheme))

	mapping := map[string]string{
		"minimal":          "catppuccin-mocha",
		"things":           "catppuccin-mocha",
		"blue topaz":       "nord",
		"california coast": "ayu-light",
		"dracula":          "dracula",
		"gruvbox":          "gruvbox-dark",
		"nord":             "nord",
		"solarized":        "solarized-dark",
		"tokyo night":      "tokyo-night",
	}

	if theme, ok := mapping[normalized]; ok {
		return theme
	}
	return "catppuccin-mocha"
}

// resolveTheme determines the Granit theme from Obsidian's base theme and
// optional community CSS theme. The community theme takes priority when present.
func resolveTheme(baseTheme, cssTheme string) string {
	// If a community CSS theme is set, use its mapping.
	if cssTheme != "" {
		return ObsidianThemeMapping(cssTheme)
	}

	// Fall back to the base theme (built-in Obsidian themes).
	switch strings.ToLower(baseTheme) {
	case "moonstone":
		return "catppuccin-latte"
	case "obsidian":
		return "catppuccin-mocha"
	default:
		return "catppuccin-mocha"
	}
}

// readJSON reads a JSON file into dest. Returns false if the file doesn't
// exist or cannot be parsed.
func readJSON(path string, dest interface{}) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, dest) == nil
}

// countHotkeys returns the number of custom hotkey bindings in hotkeys.json.
func countHotkeys(vaultRoot string) int {
	path := filepath.Join(vaultRoot, ".obsidian", "hotkeys.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var hotkeys map[string]json.RawMessage
	if json.Unmarshal(data, &hotkeys) != nil {
		return 0
	}
	return len(hotkeys)
}

// countPlugins returns the number of community plugins listed in
// community-plugins.json.
func countPlugins(vaultRoot string) int {
	path := filepath.Join(vaultRoot, ".obsidian", "community-plugins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var plugins []string
	if json.Unmarshal(data, &plugins) != nil {
		return 0
	}
	return len(plugins)
}

// ImportObsidianConfig reads Obsidian settings from the vault's .obsidian/
// directory and returns a partially-filled Config with the imported values.
// Only fields that have Obsidian equivalents are set. Returns nil if no
// .obsidian/ directory exists.
func ImportObsidianConfig(vaultRoot string) *Config {
	obsDir := filepath.Join(vaultRoot, ".obsidian")
	info, err := os.Stat(obsDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	cfg := DefaultConfig()

	// --- app.json ---
	var app obsidianApp
	if readJSON(filepath.Join(obsDir, "app.json"), &app) {
		if app.VimMode != nil {
			cfg.VimMode = *app.VimMode
		}
		if app.ShowLineNumber != nil {
			cfg.LineNumbers = *app.ShowLineNumber
		}
		if app.SpellCheck != nil {
			cfg.SpellCheck = *app.SpellCheck
		}
		if app.TabSize != nil {
			cfg.Editor.TabSize = *app.TabSize
		}
		if app.DefaultViewMode != nil && *app.DefaultViewMode == "preview" {
			cfg.DefaultViewMode = true
		}
	}

	// --- appearance.json ---
	var appearance obsidianAppearance
	if readJSON(filepath.Join(obsDir, "appearance.json"), &appearance) {
		baseTheme := ""
		cssTheme := ""
		if appearance.Theme != nil {
			baseTheme = *appearance.Theme
		}
		if appearance.CSSTheme != nil {
			cssTheme = *appearance.CSSTheme
		}
		cfg.Theme = resolveTheme(baseTheme, cssTheme)
	}

	// --- daily-notes.json ---
	var daily obsidianDailyNotes
	if readJSON(filepath.Join(obsDir, "daily-notes.json"), &daily) {
		if daily.Folder != nil {
			cfg.DailyNotesFolder = *daily.Folder
		}
		if daily.Template != nil {
			cfg.DailyNoteTemplate = *daily.Template
		}
	}

	return &cfg
}

// ImportReport returns a human-readable summary of what was imported from
// an Obsidian vault's .obsidian/ directory. It re-reads the Obsidian config
// files to produce the report.
func ImportReport(vaultRoot string) string {
	obsDir := filepath.Join(vaultRoot, ".obsidian")
	info, err := os.Stat(obsDir)
	if err != nil || !info.IsDir() {
		return "No .obsidian/ directory found — nothing to import."
	}

	var lines []string
	lines = append(lines, "Obsidian Import Report:")

	// --- app.json ---
	var app obsidianApp
	if readJSON(filepath.Join(obsDir, "app.json"), &app) {
		if app.VimMode != nil {
			lines = append(lines, fmt.Sprintf("  ✓ Vim mode: %s", enabledDisabled(*app.VimMode)))
		}
		if app.ShowLineNumber != nil {
			lines = append(lines, fmt.Sprintf("  ✓ Line numbers: %s", enabledDisabled(*app.ShowLineNumber)))
		}
		if app.SpellCheck != nil {
			lines = append(lines, fmt.Sprintf("  ✓ Spell check: %s", enabledDisabled(*app.SpellCheck)))
		}
		if app.TabSize != nil {
			lines = append(lines, fmt.Sprintf("  ✓ Tab size: %d", *app.TabSize))
		}
		if app.DefaultViewMode != nil {
			if *app.DefaultViewMode == "preview" {
				lines = append(lines, "  ✓ Default view mode: preview")
			} else {
				lines = append(lines, "  ✓ Default view mode: source")
			}
		}
	}

	// --- daily-notes.json ---
	var daily obsidianDailyNotes
	if readJSON(filepath.Join(obsDir, "daily-notes.json"), &daily) {
		if daily.Folder != nil {
			lines = append(lines, fmt.Sprintf("  ✓ Daily notes folder: %s", *daily.Folder))
		}
		if daily.Template != nil {
			lines = append(lines, fmt.Sprintf("  ✓ Daily note template: %s", *daily.Template))
		}
	}

	// --- appearance.json ---
	var appearance obsidianAppearance
	if readJSON(filepath.Join(obsDir, "appearance.json"), &appearance) {
		baseTheme := ""
		cssTheme := ""
		if appearance.Theme != nil {
			baseTheme = *appearance.Theme
		}
		if appearance.CSSTheme != nil {
			cssTheme = *appearance.CSSTheme
		}
		resolved := resolveTheme(baseTheme, cssTheme)

		if cssTheme != "" {
			lines = append(lines, fmt.Sprintf("  ✓ Theme: %s (from \"%s\" community theme)", resolved, cssTheme))
		} else if baseTheme != "" {
			lines = append(lines, fmt.Sprintf("  ✓ Theme: %s (from \"%s\" base)", resolved, baseTheme))
		}
	}

	// --- hotkeys.json (informational) ---
	if hotkeyCount := countHotkeys(vaultRoot); hotkeyCount > 0 {
		lines = append(lines, fmt.Sprintf("  ✗ Hotkeys: %d custom hotkeys found (not imported)", hotkeyCount))
	}

	// --- community plugins (informational) ---
	if pluginCount := countPlugins(vaultRoot); pluginCount > 0 {
		lines = append(lines, fmt.Sprintf("  ✗ Plugins: %d community plugins found (not imported)", pluginCount))
	}

	return strings.Join(lines, "\n")
}

// enabledDisabled returns "enabled" if b is true, "disabled" otherwise.
func enabledDisabled(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}
