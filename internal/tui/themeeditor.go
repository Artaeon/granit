package tui

import (
	"encoding/json"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
)

// colorRole represents one editable color slot in the theme editor.
type colorRole struct {
	label string
	value string // hex string e.g. "#CBA6F7"
}

// ThemeEditor is an overlay for creating and editing color themes.
type ThemeEditor struct {
	active bool
	width  int
	height int

	// The theme being edited (copy, not pointer to live theme)
	theme    Theme
	roles    []colorRole // flattened color list for editing
	cursor   int         // which role is selected
	editing  bool        // currently typing a hex value
	editBuf  string      // buffer while editing

	// Save-as flow
	naming  bool
	nameBuf string

	// Export feedback
	exported string

	// Config dir for saving
	configDir string

	// Scroll for long lists
	scroll int
}

// NewThemeEditor creates a new theme editor instance.
func NewThemeEditor() ThemeEditor {
	return ThemeEditor{
		configDir: config.ConfigDir(),
	}
}

func (te *ThemeEditor) IsActive() bool {
	return te.active
}

func (te *ThemeEditor) SetSize(width, height int) {
	te.width = width
	te.height = height
}

// Open opens the theme editor with the currently active theme as a starting point.
func (te *ThemeEditor) Open(currentThemeName string) {
	te.active = true
	te.cursor = 0
	te.editing = false
	te.editBuf = ""
	te.naming = false
	te.nameBuf = ""
	te.exported = ""
	te.scroll = 0
	te.theme = GetTheme(currentThemeName)
	te.syncRolesFromTheme()
}

func (te *ThemeEditor) Close() {
	te.active = false
	te.editing = false
	te.naming = false
}

// syncRolesFromTheme populates the editable role list from te.theme.
func (te *ThemeEditor) syncRolesFromTheme() {
	te.roles = []colorRole{
		{"primary", string(te.theme.Primary)},
		{"secondary", string(te.theme.Secondary)},
		{"accent", string(te.theme.Accent)},
		{"warning", string(te.theme.Warning)},
		{"success", string(te.theme.Success)},
		{"error", string(te.theme.Error)},
		{"info", string(te.theme.Info)},
		{"text", string(te.theme.Text)},
		{"subtext", string(te.theme.Subtext)},
		{"dim", string(te.theme.Dim)},
		{"surface2", string(te.theme.Surface2)},
		{"surface1", string(te.theme.Surface1)},
		{"surface0", string(te.theme.Surface0)},
		{"base", string(te.theme.Base)},
		{"mantle", string(te.theme.Mantle)},
		{"crust", string(te.theme.Crust)},
	}
}

// applyRolesToTheme writes the role values back into te.theme.
func (te *ThemeEditor) applyRolesToTheme() {
	for _, r := range te.roles {
		c := lipgloss.Color(r.value)
		switch r.label {
		case "primary":
			te.theme.Primary = c
		case "secondary":
			te.theme.Secondary = c
		case "accent":
			te.theme.Accent = c
		case "warning":
			te.theme.Warning = c
		case "success":
			te.theme.Success = c
		case "error":
			te.theme.Error = c
		case "info":
			te.theme.Info = c
		case "text":
			te.theme.Text = c
		case "subtext":
			te.theme.Subtext = c
		case "dim":
			te.theme.Dim = c
		case "surface2":
			te.theme.Surface2 = c
		case "surface1":
			te.theme.Surface1 = c
		case "surface0":
			te.theme.Surface0 = c
		case "base":
			te.theme.Base = c
		case "mantle":
			te.theme.Mantle = c
		case "crust":
			te.theme.Crust = c
		}
	}
}

// isValidHex checks if a string looks like a hex color (#RGB or #RRGGBB).
func isValidHex(s string) bool {
	if len(s) != 4 && len(s) != 7 {
		return false
	}
	if s[0] != '#' {
		return false
	}
	for _, ch := range s[1:] {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
			return false
		}
	}
	return true
}

func (te ThemeEditor) Update(msg tea.Msg) (ThemeEditor, tea.Cmd) {
	if !te.active {
		return te, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// Naming mode: typing a theme name to save
		if te.naming {
			switch key {
			case "esc":
				te.naming = false
				te.nameBuf = ""
			case "enter":
				name := strings.TrimSpace(te.nameBuf)
				if name != "" {
					te.theme.Name = name
					te.applyRolesToTheme()
					_ = SaveCustomTheme(te.configDir, te.theme)
					// Reload custom themes so the new one appears immediately
					InitCustomThemes(te.configDir)
					te.exported = "Saved as: " + name
				}
				te.naming = false
				te.nameBuf = ""
			case "backspace":
				if len(te.nameBuf) > 0 {
					te.nameBuf = te.nameBuf[:len(te.nameBuf)-1]
				}
			default:
				if len(key) == 1 && key[0] >= 32 {
					te.nameBuf += key
				}
			}
			return te, nil
		}

		// Editing mode: typing a hex value
		if te.editing {
			switch key {
			case "esc":
				te.editing = false
				te.editBuf = ""
			case "enter":
				val := strings.TrimSpace(te.editBuf)
				if isValidHex(val) && te.cursor < len(te.roles) {
					te.roles[te.cursor].value = val
					te.applyRolesToTheme()
				}
				te.editing = false
				te.editBuf = ""
			case "backspace":
				if len(te.editBuf) > 0 {
					te.editBuf = te.editBuf[:len(te.editBuf)-1]
				}
			default:
				if len(key) == 1 && key[0] >= 32 && len(te.editBuf) < 7 {
					te.editBuf += key
				}
			}
			return te, nil
		}

		// Normal navigation
		switch key {
		case "esc", "q":
			te.active = false
			return te, nil
		case "up", "k":
			if te.cursor > 0 {
				te.cursor--
			}
		case "down", "j":
			if te.cursor < len(te.roles)-1 {
				te.cursor++
			}
		case "enter":
			// Start editing the selected color
			if te.cursor < len(te.roles) {
				te.editing = true
				te.editBuf = te.roles[te.cursor].value
			}
		case "s":
			// Save as custom theme
			te.naming = true
			if te.theme.Name != "" {
				te.nameBuf = te.theme.Name
			}
		case "e":
			// Export current theme as JSON
			te.applyRolesToTheme()
			jt := themeToJSON(te.theme)
			data, err := json.MarshalIndent(jt, "", "  ")
			if err == nil {
				te.exported = string(data)
			} else {
				te.exported = "Export failed"
			}
		}
	}
	return te, nil
}

func (te ThemeEditor) View() string {
	width := te.width * 2 / 3
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}

	innerWidth := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  Theme Editor"))
	b.WriteString("\n")

	subtitleStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(subtitleStyle.Render("  Editing: " + te.theme.Name))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	// Naming mode
	if te.naming {
		promptStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		b.WriteString(promptStyle.Render("  Save as: "))
		inputStyle := lipgloss.NewStyle().Background(surface0).Foreground(text)
		cursor := lipgloss.NewStyle().Foreground(mauve).Render("|")
		b.WriteString(inputStyle.Render(te.nameBuf + cursor))
		b.WriteString("\n\n")
	}

	// Feedback
	if te.exported != "" {
		feedbackStyle := lipgloss.NewStyle().Foreground(green).Italic(true)
		// Show only the first line for long exports
		lines := strings.SplitN(te.exported, "\n", 2)
		b.WriteString(feedbackStyle.Render("  " + lines[0]))
		if len(lines) > 1 {
			b.WriteString(feedbackStyle.Render(" ..."))
		}
		b.WriteString("\n\n")
	}

	// Editing indicator
	if te.editing {
		editStyle := lipgloss.NewStyle().Foreground(yellow).Bold(true)
		b.WriteString(editStyle.Render("  Enter hex color: "))
		inputStyle := lipgloss.NewStyle().Background(surface0).Foreground(text)
		cursor := lipgloss.NewStyle().Foreground(mauve).Render("|")
		b.WriteString(inputStyle.Render(te.editBuf + cursor))
		b.WriteString("\n\n")
	}

	// Color roles list with live preview swatches
	maxVisible := te.height - 20
	if maxVisible < 8 {
		maxVisible = 8
	}
	if maxVisible > len(te.roles) {
		maxVisible = len(te.roles)
	}

	// Scroll to keep cursor visible
	if te.cursor < te.scroll {
		te.scroll = te.cursor
	}
	if te.cursor >= te.scroll+maxVisible {
		te.scroll = te.cursor - maxVisible + 1
	}

	end := te.scroll + maxVisible
	if end > len(te.roles) {
		end = len(te.roles)
	}

	labelWidth := 12
	for i := te.scroll; i < end; i++ {
		r := te.roles[i]

		// Color swatch: two block characters in the role color
		swatch := lipgloss.NewStyle().
			Foreground(lipgloss.Color(r.value)).
			Render("\u2588\u2588")

		label := r.label
		for len(label) < labelWidth {
			label += " "
		}

		if i == te.cursor {
			accentBar := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar)
			nameStyle := lipgloss.NewStyle().
				Background(surface0).
				Foreground(mauve).
				Bold(true)
			valStyle := lipgloss.NewStyle().
				Background(surface0).
				Foreground(text)
			b.WriteString(accentBar + " " + swatch + " " + nameStyle.Render(label) + " " + valStyle.Render(r.value))
		} else {
			nameStyle := lipgloss.NewStyle().Foreground(subtext1)
			valStyle := lipgloss.NewStyle().Foreground(overlay0)
			b.WriteString("  " + swatch + " " + nameStyle.Render(label) + " " + valStyle.Render(r.value))
		}
		b.WriteString("\n")
	}

	// Style properties section
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")
	propStyle := lipgloss.NewStyle().Foreground(subtext1)
	propValStyle := lipgloss.NewStyle().Foreground(overlay0)
	b.WriteString(propStyle.Render("  border:    ") + propValStyle.Render(te.theme.Border) + "\n")
	b.WriteString(propStyle.Render("  density:   ") + propValStyle.Render(te.theme.Density) + "\n")
	b.WriteString(propStyle.Render("  accent_bar:") + propValStyle.Render(" "+te.theme.AccentBar) + "\n")
	b.WriteString(propStyle.Render("  separator: ") + propValStyle.Render(te.theme.Separator) + "\n")
	ulVal := "false"
	if te.theme.LinkUnderline {
		ulVal = "true"
	}
	b.WriteString(propStyle.Render("  underline: ") + propValStyle.Render(ulVal) + "\n")

	// Live preview
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  Preview"))
	b.WriteString("\n")

	pt := te.theme
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Primary).Bold(true).Render("  # Heading 1") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Secondary).Bold(true).Render("  ## Heading 2") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Text).Render("  Normal text with "))
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Secondary).Underline(pt.LinkUnderline).Render("[[link]]"))
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Text).Render(" and "))
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Success).Render("`code`"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Accent).Bold(true).Render("  - "))
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Text).Render("List item") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Success).Render("  [x] Done task") + "  ")
	b.WriteString(lipgloss.NewStyle().Foreground(pt.Warning).Render("[ ] Open task") + "\n")

	// Help line
	b.WriteString("\n")
	helpStyle := lipgloss.NewStyle().Foreground(overlay0)
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	b.WriteString("  " + keyStyle.Render("Enter") + helpStyle.Render(" edit color  "))
	b.WriteString(keyStyle.Render("s") + helpStyle.Render(" save  "))
	b.WriteString(keyStyle.Render("e") + helpStyle.Render(" export  "))
	b.WriteString(keyStyle.Render("Esc") + helpStyle.Render(" close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width)

	return border.Render(b.String())
}
