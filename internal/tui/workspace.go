package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// WorkspaceLayout captures a named snapshot of the user's TUI state so it
// can be restored later.
type WorkspaceLayout struct {
	Name         string   `json:"name"`
	ActiveNote   string   `json:"active_note"`
	OpenNotes    []string `json:"open_notes"`
	SidebarFocus bool     `json:"sidebar_focus"`
	ViewMode     bool     `json:"view_mode"`
	Layout       string   `json:"layout"`
	CreatedAt    string   `json:"created_at"`
}

// Workspace provides the overlay for saving, loading, renaming, and deleting
// named workspace layouts.  Storage is a single JSON file under configDir.
type Workspace struct {
	active bool
	width  int
	height int

	// Data
	layouts []WorkspaceLayout
	cursor  int
	scroll  int

	// Save mode — text input for new workspace name
	saveMode bool
	saveName string

	// Rename mode — inline rename of the selected workspace
	renameMode bool
	renameBuf  string

	// Result — consumed once by app.go after the user selects a workspace
	loadResult *WorkspaceLayout
	loadReady  bool

	// Save request — app.go checks this flag, captures state, calls SaveLayout
	saveRequested bool

	// Storage
	configDir string
}

// NewWorkspace creates a Workspace component that persists layouts under
// configDir (typically ~/.config/granit/).
func NewWorkspace(configDir string) Workspace {
	return Workspace{
		configDir: configDir,
	}
}

// IsActive returns whether the workspace overlay is currently visible.
func (w *Workspace) IsActive() bool {
	return w.active
}

// SetSize updates available dimensions for the overlay.
func (w *Workspace) SetSize(width, height int) {
	w.width = width
	w.height = height
}

// Close hides the overlay and resets transient state.
func (w *Workspace) Close() {
	w.active = false
	w.saveMode = false
	w.saveName = ""
	w.renameMode = false
	w.renameBuf = ""
}

// Open shows the overlay and reloads saved workspaces from disk.
func (w *Workspace) Open() {
	w.active = true
	w.cursor = 0
	w.scroll = 0
	w.saveMode = false
	w.saveName = ""
	w.renameMode = false
	w.renameBuf = ""
	w.loadResult = nil
	w.loadReady = false
	w.saveRequested = false
	w.loadWorkspaces()
}

// GetLoadResult returns the workspace the user chose to load and resets
// the flag.  The second return value is false when there is no pending
// result.
func (w *Workspace) GetLoadResult() (*WorkspaceLayout, bool) {
	if w.loadReady {
		r := w.loadResult
		w.loadResult = nil
		w.loadReady = false
		return r, true
	}
	return nil, false
}

// IsSaveRequested returns true (once) when the user confirmed a new
// workspace name via save mode.  app.go should then capture the current
// TUI state into a WorkspaceLayout and call SaveLayout.
func (w *Workspace) IsSaveRequested() bool {
	if w.saveRequested {
		w.saveRequested = false
		return true
	}
	return false
}

// SaveName returns the name the user entered during save mode.
func (w *Workspace) SaveName() string {
	return w.saveName
}

// SaveLayout persists a layout to the workspace list and writes the
// file to disk.  If a workspace with the same name already exists it
// is replaced.
func (w *Workspace) SaveLayout(layout WorkspaceLayout) {
	// Replace existing workspace with the same name.
	replaced := false
	for i, l := range w.layouts {
		if l.Name == layout.Name {
			w.layouts[i] = layout
			replaced = true
			break
		}
	}
	if !replaced {
		w.layouts = append(w.layouts, layout)
	}
	w.saveWorkspaces()
}

// DeleteCurrent removes the workspace under the cursor and persists
// the change to disk.
func (w *Workspace) DeleteCurrent() {
	if len(w.layouts) == 0 || w.cursor >= len(w.layouts) {
		return
	}
	w.layouts = append(w.layouts[:w.cursor], w.layouts[w.cursor+1:]...)
	if w.cursor >= len(w.layouts) && w.cursor > 0 {
		w.cursor--
	}
	w.saveWorkspaces()
}

// ---------------------------------------------------------------------------
// Persistence
// ---------------------------------------------------------------------------

func (w *Workspace) storagePath() string {
	return filepath.Join(w.configDir, "workspaces.json")
}

func (w *Workspace) loadWorkspaces() {
	w.layouts = nil
	data, err := os.ReadFile(w.storagePath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &w.layouts)
}

func (w *Workspace) saveWorkspaces() {
	if err := os.MkdirAll(w.configDir, 0700); err != nil {
		return
	}
	data, err := json.MarshalIndent(w.layouts, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(w.storagePath(), data, 0600)
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles keyboard input for the workspace overlay.
func (w Workspace) Update(msg tea.Msg) (Workspace, tea.Cmd) {
	if !w.active {
		return w, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		key := msg.String()

		// ---- Save mode (text input for workspace name) ----
		if w.saveMode {
			switch key {
			case "esc":
				w.saveMode = false
				w.saveName = ""
			case "enter":
				name := strings.TrimSpace(w.saveName)
				if name != "" {
					w.saveRequested = true
					w.saveMode = false
				}
			case "backspace":
				if len(w.saveName) > 0 {
					w.saveName = w.saveName[:len(w.saveName)-1]
				}
			default:
				if len(key) == 1 {
					w.saveName += key
				} else if key == "space" {
					w.saveName += " "
				}
			}
			return w, nil
		}

		// ---- Rename mode (inline rename) ----
		if w.renameMode {
			switch key {
			case "esc":
				w.renameMode = false
				w.renameBuf = ""
			case "enter":
				name := strings.TrimSpace(w.renameBuf)
				if name != "" && w.cursor < len(w.layouts) {
					w.layouts[w.cursor].Name = name
					w.saveWorkspaces()
				}
				w.renameMode = false
				w.renameBuf = ""
			case "backspace":
				if len(w.renameBuf) > 0 {
					w.renameBuf = w.renameBuf[:len(w.renameBuf)-1]
				}
			default:
				if len(key) == 1 {
					w.renameBuf += key
				} else if key == "space" {
					w.renameBuf += " "
				}
			}
			return w, nil
		}

		// ---- Normal navigation ----
		switch key {
		case "esc":
			w.active = false
		case "up", "k":
			if w.cursor > 0 {
				w.cursor--
				if w.cursor < w.scroll {
					w.scroll = w.cursor
				}
			}
		case "down", "j":
			if w.cursor < len(w.layouts)-1 {
				w.cursor++
				visH := w.visibleHeight()
				if w.cursor >= w.scroll+visH {
					w.scroll = w.cursor - visH + 1
				}
			}
		case "enter":
			if len(w.layouts) > 0 && w.cursor < len(w.layouts) {
				layout := w.layouts[w.cursor]
				w.loadResult = &layout
				w.loadReady = true
				w.active = false
			}
		case "s":
			w.saveMode = true
			w.saveName = ""
		case "d":
			w.DeleteCurrent()
		case "r":
			if len(w.layouts) > 0 && w.cursor < len(w.layouts) {
				w.renameMode = true
				w.renameBuf = w.layouts[w.cursor].Name
			}
		}
	}
	return w, nil
}

// visibleHeight returns how many workspace rows fit in the overlay body.
func (w *Workspace) visibleHeight() int {
	h := w.height - 12
	if h < 1 {
		h = 1
	}
	return h
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the workspace overlay.
func (w Workspace) View() string {
	width := w.width / 2
	if width < 55 {
		width = 55
	}
	if width > 75 {
		width = 75
	}

	innerW := width - 6 // account for border + padding

	var b strings.Builder

	// ---- Save mode view ----
	if w.saveMode {
		b.WriteString(w.viewSaveMode(innerW))
		return w.wrapBorder(width, b.String())
	}

	// ---- Normal list view ----

	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Workspaces")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	// Subtitle — count
	countStr := fmt.Sprintf("  %d saved workspace", len(w.layouts))
	if len(w.layouts) != 1 {
		countStr += "s"
	}
	b.WriteString(DimStyle.Render(countStr))
	b.WriteString("\n\n")

	// List
	if len(w.layouts) == 0 {
		b.WriteString(DimStyle.Render("  No workspaces saved yet"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  Press s to save the current layout"))
	} else {
		visH := w.visibleHeight()
		end := w.scroll + visH
		if end > len(w.layouts) {
			end = len(w.layouts)
		}

		nameStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
		noteStyle := lipgloss.NewStyle().Foreground(lavender)
		dateStyle := lipgloss.NewStyle().Foreground(overlay0)

		for i := w.scroll; i < end; i++ {
			layout := w.layouts[i]

			// Build columns: indicator, name, active note, date
			indicator := "  "
			if i == w.cursor {
				indicator = lipgloss.NewStyle().Foreground(peach).Bold(true).Render(ThemeAccentBar + " ")
			}

			// Workspace name
			name := layout.Name
			maxNameLen := 16
			if len(name) > maxNameLen {
				name = name[:maxNameLen-1] + "…"
			}

			// Active note — show just the basename without .md
			activeNote := ""
			if layout.ActiveNote != "" {
				activeNote = strings.TrimSuffix(filepath.Base(layout.ActiveNote), ".md")
			}
			maxNoteLen := innerW - maxNameLen - 18
			if maxNoteLen < 8 {
				maxNoteLen = 8
			}
			if len(activeNote) > maxNoteLen {
				activeNote = activeNote[:maxNoteLen-1] + "…"
			}

			// Date — show just the date portion
			date := layout.CreatedAt
			if len(date) > 10 {
				date = date[:10]
			}

			if i == w.cursor {
				// Selected row
				nameRendered := lipgloss.NewStyle().
					Foreground(peach).
					Bold(true).
					Render(padRight(name, maxNameLen))
				noteRendered := noteStyle.Render(padRight(activeNote, maxNoteLen))
				dateRendered := dateStyle.Render(date)
				line := indicator + nameRendered + "  " + noteRendered + "  " + dateRendered

				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Width(innerW).
					Render(line))
			} else {
				nameRendered := nameStyle.Render(padRight(name, maxNameLen))
				noteRendered := noteStyle.Render(padRight(activeNote, maxNoteLen))
				dateRendered := dateStyle.Render(date)
				b.WriteString(indicator + nameRendered + "  " + noteRendered + "  " + dateRendered)
			}

			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	// Footer with keybinding hints
	enterKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Enter")
	enterDesc := DimStyle.Render(": load  ")
	saveKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("s")
	saveDesc := DimStyle.Render(": save current  ")
	delKey := lipgloss.NewStyle().Foreground(red).Bold(true).Render("d")
	delDesc := DimStyle.Render(": delete  ")
	renKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("r")
	renDesc := DimStyle.Render(": rename  ")
	escKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Esc")
	escDesc := DimStyle.Render(": close")

	b.WriteString("  " + enterKey + enterDesc + saveKey + saveDesc + delKey + delDesc + renKey + renDesc + escKey + escDesc)

	return w.wrapBorder(width, b.String())
}

// viewSaveMode renders the save-mode input screen.
func (w *Workspace) viewSaveMode(innerW int) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  Save Workspace")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n\n")

	// Input line
	promptStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	inputStyle := lipgloss.NewStyle().Foreground(text)
	cursorChar := lipgloss.NewStyle().
		Background(text).
		Foreground(mantle).
		Render(" ")

	b.WriteString("  " + promptStyle.Render("Name: ") + inputStyle.Render(w.saveName) + cursorChar)

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	enterKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Enter")
	enterDesc := DimStyle.Render(": confirm  ")
	escKey := lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Esc")
	escDesc := DimStyle.Render(": cancel")

	b.WriteString("  " + enterKey + enterDesc + escKey + escDesc)

	return b.String()
}


// wrapBorder wraps content in the standard overlay border.
func (w *Workspace) wrapBorder(width int, content string) string {
	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width).
		Background(mantle)
	return border.Render(content)
}
