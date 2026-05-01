package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/templates"
)

type NoteTemplate struct {
	name     string
	content  string
	isUser   bool // true if loaded from vault's templates/ folder
}

type Templates struct {
	OverlayBase
	cursor    int
	scroll    int
	templates []NoteTemplate
	result    string
	selected  bool
}

func NewTemplates() Templates {
	return Templates{
		templates: builtinTemplates(),
	}
}

func builtinTemplates() []NoteTemplate {
	// The actual template data lives in the shared internal/templates package
	// so the TUI and the web (granit web /templates) stay in lockstep.
	src := templates.Builtin()
	out := make([]NoteTemplate, len(src))
	for i, t := range src {
		out[i] = NoteTemplate{name: t.Name, content: t.Content, isUser: t.IsUser}
	}
	return out
}

func (t *Templates) Open() {
	t.Activate()
	t.cursor = 0
	t.scroll = 0
	t.result = ""
	t.selected = false
}

// OpenWithVault loads user templates from the vault's templates/ folder
// and merges them with built-in templates.
func (t *Templates) OpenWithVault(vaultRoot string) {
	t.Open()
	t.templates = builtinTemplates()
	userTemplates := loadUserTemplates(vaultRoot)
	if len(userTemplates) > 0 {
		// Insert separator then user templates after "Blank Note"
		t.templates = append(t.templates, userTemplates...)
	}
}

// loadUserTemplates scans vaultRoot/templates/ for .md files.
func loadUserTemplates(vaultRoot string) []NoteTemplate {
	templatesDir := filepath.Join(vaultRoot, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return nil
	}
	var templates []NoteTemplate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(templatesDir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		templates = append(templates, NoteTemplate{
			name:    name,
			content: string(data),
			isUser:  true,
		})
	}
	return templates
}

func (t *Templates) WasSelected() bool {
	return t.selected
}

func (t *Templates) SelectedTemplate() string {
	if !t.selected {
		return ""
	}
	t.selected = false
	r := t.result
	t.result = ""
	now := time.Now()
	r = strings.ReplaceAll(r, "{{date}}", now.Format("2006-01-02"))
	r = strings.ReplaceAll(r, "{{time}}", now.Format("15:04"))
	r = strings.ReplaceAll(r, "{{datetime}}", now.Format("2006-01-02 15:04"))
	r = strings.ReplaceAll(r, "{{yesterday}}", now.AddDate(0, 0, -1).Format("2006-01-02"))
	r = strings.ReplaceAll(r, "{{tomorrow}}", now.AddDate(0, 0, 1).Format("2006-01-02"))
	r = strings.ReplaceAll(r, "{{weekday}}", now.Weekday().String())
	return r // may be "" for blank note
}

func (t Templates) Update(msg tea.Msg) (Templates, tea.Cmd) {
	if !t.active {
		return t, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			t.active = false
		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
				if t.cursor < t.scroll {
					t.scroll = t.cursor
				}
			}
		case "down", "j":
			if t.cursor < len(t.templates)-1 {
				t.cursor++
				visH := t.height - 10
				if visH < 1 {
					visH = 1
				}
				if t.cursor >= t.scroll+visH {
					t.scroll = t.cursor - visH + 1
				}
			}
		case "enter":
			if len(t.templates) > 0 && t.cursor < len(t.templates) {
				t.result = t.templates[t.cursor].content
				t.selected = true
				t.active = false
			}
		}
	}
	return t, nil
}

func (t Templates) View() string {
	width := t.width / 2
	if width < 50 {
		width = 50
	}
	if width > 70 {
		width = 70
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconNewChar + " New Note — Choose Template")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	if len(t.templates) == 0 {
		b.WriteString(DimStyle.Render("  No templates available"))
		b.WriteString("\n")
	} else {
		visH := t.height - 10
		if visH < 5 {
			visH = 5
		}
		end := t.scroll + visH
		if end > len(t.templates) {
			end = len(t.templates)
		}

		icons := []lipgloss.Style{
			lipgloss.NewStyle().Foreground(blue),
			lipgloss.NewStyle().Foreground(peach),
			lipgloss.NewStyle().Foreground(green),
			lipgloss.NewStyle().Foreground(sapphire),
			lipgloss.NewStyle().Foreground(yellow),
			lipgloss.NewStyle().Foreground(red),
		}

		iconChars := []string{" ", " ", " ", " ", " ", " "}

		showedUserHeader := false
		for i := t.scroll; i < end; i++ {
			tmpl := t.templates[i]

			// Show "Your Templates" header before first user template
			if tmpl.isUser && !showedUserHeader {
				if i > t.scroll {
					b.WriteString("\n")
				}
				b.WriteString("  " + lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Your Templates"))
				b.WriteString("\n")
				showedUserHeader = true
			}

			iconIdx := i % len(icons)
			icon := icons[iconIdx].Render(iconChars[iconIdx])
			if tmpl.isUser {
				icon = lipgloss.NewStyle().Foreground(green).Render("*")
			}

			if i == t.cursor {
				line := "  " + icon + " " + tmpl.name
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString("  " + icon + " " + NormalItemStyle.Render(tmpl.name))
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: select template  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
