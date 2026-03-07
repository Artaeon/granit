package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// customThemes holds user-defined themes loaded from disk.
var customThemes map[string]Theme

// jsonTheme is the JSON-serializable representation of a Theme.
type jsonTheme struct {
	Name          string `json:"name"`
	Primary       string `json:"primary"`
	Secondary     string `json:"secondary"`
	Accent        string `json:"accent"`
	Warning       string `json:"warning"`
	Success       string `json:"success"`
	Error         string `json:"error"`
	Info          string `json:"info"`
	Text          string `json:"text"`
	Subtext       string `json:"subtext"`
	Dim           string `json:"dim"`
	Surface2      string `json:"surface2"`
	Surface1      string `json:"surface1"`
	Surface0      string `json:"surface0"`
	Base          string `json:"base"`
	Mantle        string `json:"mantle"`
	Crust         string `json:"crust"`
	Border        string `json:"border"`
	Density       string `json:"density"`
	AccentBar     string `json:"accent_bar"`
	Separator     string `json:"separator"`
	LinkUnderline bool   `json:"link_underline"`
}

// themeToJSON converts a Theme to its JSON-serializable form.
func themeToJSON(t Theme) jsonTheme {
	return jsonTheme{
		Name:          t.Name,
		Primary:       string(t.Primary),
		Secondary:     string(t.Secondary),
		Accent:        string(t.Accent),
		Warning:       string(t.Warning),
		Success:       string(t.Success),
		Error:         string(t.Error),
		Info:          string(t.Info),
		Text:          string(t.Text),
		Subtext:       string(t.Subtext),
		Dim:           string(t.Dim),
		Surface2:      string(t.Surface2),
		Surface1:      string(t.Surface1),
		Surface0:      string(t.Surface0),
		Base:          string(t.Base),
		Mantle:        string(t.Mantle),
		Crust:         string(t.Crust),
		Border:        t.Border,
		Density:       t.Density,
		AccentBar:     t.AccentBar,
		Separator:     t.Separator,
		LinkUnderline: t.LinkUnderline,
	}
}

// jsonToTheme converts a JSON-deserialized form back to a Theme.
func jsonToTheme(jt jsonTheme) Theme {
	t := Theme{
		Name:          jt.Name,
		Primary:       lipgloss.Color(jt.Primary),
		Secondary:     lipgloss.Color(jt.Secondary),
		Accent:        lipgloss.Color(jt.Accent),
		Warning:       lipgloss.Color(jt.Warning),
		Success:       lipgloss.Color(jt.Success),
		Error:         lipgloss.Color(jt.Error),
		Info:          lipgloss.Color(jt.Info),
		Text:          lipgloss.Color(jt.Text),
		Subtext:       lipgloss.Color(jt.Subtext),
		Dim:           lipgloss.Color(jt.Dim),
		Surface2:      lipgloss.Color(jt.Surface2),
		Surface1:      lipgloss.Color(jt.Surface1),
		Surface0:      lipgloss.Color(jt.Surface0),
		Base:          lipgloss.Color(jt.Base),
		Mantle:        lipgloss.Color(jt.Mantle),
		Crust:         lipgloss.Color(jt.Crust),
		LinkUnderline: jt.LinkUnderline,
	}
	if jt.Border != "" {
		t.Border = jt.Border
	} else {
		t.Border = "rounded"
	}
	if jt.Density != "" {
		t.Density = jt.Density
	} else {
		t.Density = "normal"
	}
	if jt.AccentBar != "" {
		t.AccentBar = jt.AccentBar
	} else {
		t.AccentBar = "┃"
	}
	if jt.Separator != "" {
		t.Separator = jt.Separator
	} else {
		t.Separator = "─"
	}
	return t
}

// themesDir returns the directory where custom themes live.
func themesDir(configDir string) string {
	return filepath.Join(configDir, "themes")
}

// LoadCustomThemes reads all .json files from ~/.config/granit/themes/ and
// returns them as a map keyed by theme name.
func LoadCustomThemes(configDir string) map[string]Theme {
	dir := themesDir(configDir)
	themes := make(map[string]Theme)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return themes
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}

		var jt jsonTheme
		if err := json.Unmarshal(data, &jt); err != nil {
			continue
		}

		// Use the name from JSON content; fall back to filename stem.
		if jt.Name == "" {
			jt.Name = strings.TrimSuffix(entry.Name(), ".json")
		}

		t := jsonToTheme(jt)
		themes[t.Name] = t
	}

	return themes
}

// SaveCustomTheme writes a theme as JSON into the custom themes directory.
func SaveCustomTheme(configDir string, theme Theme) error {
	dir := themesDir(configDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	jt := themeToJSON(theme)
	data, err := json.MarshalIndent(jt, "", "  ")
	if err != nil {
		return err
	}

	filename := strings.ReplaceAll(strings.ToLower(theme.Name), " ", "-") + ".json"
	return os.WriteFile(filepath.Join(dir, filename), data, 0644)
}

// ExportTheme serializes a named theme (built-in or custom) to pretty JSON.
// Returns an empty string if the theme is not found.
func ExportTheme(name string) string {
	t := GetTheme(name)
	if t.Name == "" {
		return ""
	}
	jt := themeToJSON(t)
	data, err := json.MarshalIndent(jt, "", "  ")
	if err != nil {
		return ""
	}
	return string(data)
}

// InitCustomThemes loads user themes from configDir and merges them into
// the package-level customThemes map. Call from NewModel.
func InitCustomThemes(configDir string) {
	customThemes = LoadCustomThemes(configDir)
}

// CustomThemeNames returns a sorted list of custom theme names.
func CustomThemeNames() []string {
	if len(customThemes) == 0 {
		return nil
	}
	names := make([]string, 0, len(customThemes))
	for name := range customThemes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
