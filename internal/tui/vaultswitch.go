package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
)

// VaultSwitch is an in-app overlay that lets the user switch between known
// vaults without restarting. It loads the vault list from the global config,
// allows navigation, removal, and adding new vault paths.
type VaultSwitch struct {
	active bool
	width  int
	height int

	vaults config.VaultList
	cursor int
	scroll int

	// Add-vault text input mode
	adding   bool
	addInput string

	// consumed-once result — app.go reads via GetSelectedVault()
	selectedPath string
	selected     bool
}

// NewVaultSwitch creates an inactive VaultSwitch ready to be opened.
func NewVaultSwitch() VaultSwitch {
	return VaultSwitch{}
}

// IsActive reports whether the vault switch overlay is visible.
func (vs VaultSwitch) IsActive() bool {
	return vs.active
}

// SetSize updates the available terminal dimensions for the overlay.
func (vs *VaultSwitch) SetSize(w, h int) {
	vs.width = w
	vs.height = h
}

// Open loads the vault list from config and activates the overlay.
func (vs *VaultSwitch) Open() {
	vs.active = true
	vs.vaults = config.LoadVaultList()
	vs.cursor = 0
	vs.scroll = 0
	vs.adding = false
	vs.addInput = ""
	vs.selectedPath = ""
	vs.selected = false
}

// Close hides the overlay.
func (vs *VaultSwitch) Close() {
	vs.active = false
}

// GetSelectedVault returns the path of the vault the user chose and resets the
// flag. The second return value is false when nothing was selected.
func (vs *VaultSwitch) GetSelectedVault() (string, bool) {
	if !vs.selected {
		return "", false
	}
	path := vs.selectedPath
	vs.selectedPath = ""
	vs.selected = false
	return path, true
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update handles key input. Value receiver so it works with Bubble Tea's
// message-passing pattern.
func (vs VaultSwitch) Update(msg tea.KeyMsg) (VaultSwitch, tea.Cmd) {
	if !vs.active {
		return vs, nil
	}

	if vs.adding {
		return vs.updateAddInput(msg)
	}
	return vs.updateList(msg)
}

// updateList handles key events in the vault list mode.
func (vs VaultSwitch) updateList(msg tea.KeyMsg) (VaultSwitch, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		vs.active = false
		return vs, nil

	case "up", "k":
		if vs.cursor > 0 {
			vs.cursor--
			vs.ensureVisible()
		}

	case "down", "j":
		if vs.cursor < len(vs.vaults.Vaults)-1 {
			vs.cursor++
			vs.ensureVisible()
		}

	case "enter":
		if len(vs.vaults.Vaults) > 0 && vs.cursor < len(vs.vaults.Vaults) {
			entry := vs.vaults.Vaults[vs.cursor]
			vs.selectedPath = entry.Path
			vs.selected = true
			// Update last-opened time and persist.
			vs.vaults.AddVault(entry.Path)
			config.SaveVaultList(vs.vaults)
			vs.active = false
		}
		return vs, nil

	case "d":
		if len(vs.vaults.Vaults) > 0 && vs.cursor < len(vs.vaults.Vaults) {
			entry := vs.vaults.Vaults[vs.cursor]
			vs.vaults.RemoveVault(entry.Path)
			config.SaveVaultList(vs.vaults)
			if vs.cursor >= len(vs.vaults.Vaults) && vs.cursor > 0 {
				vs.cursor--
			}
			vs.ensureVisible()
		}

	case "a":
		vs.adding = true
		vs.addInput = ""
	}
	return vs, nil
}

// updateAddInput handles key events while adding a new vault path.
func (vs VaultSwitch) updateAddInput(msg tea.KeyMsg) (VaultSwitch, tea.Cmd) {
	switch msg.String() {
	case "esc":
		vs.adding = false
		vs.addInput = ""

	case "enter":
		path := strings.TrimSpace(vs.addInput)
		if path == "" {
			vs.adding = false
			return vs, nil
		}
		// Expand ~ to home directory.
		if strings.HasPrefix(path, "~/") || path == "~" {
			if home, err := os.UserHomeDir(); err == nil {
				path = filepath.Join(home, path[1:])
			}
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			absPath = path
		}
		// Only add if the directory exists.
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			vs.vaults.AddVault(absPath)
			config.SaveVaultList(vs.vaults)
			// Move cursor to the newly added vault.
			for i, v := range vs.vaults.Vaults {
				if v.Path == absPath {
					vs.cursor = i
					break
				}
			}
			vs.ensureVisible()
		}
		vs.adding = false
		vs.addInput = ""

	case "backspace":
		if len(vs.addInput) > 0 {
			vs.addInput = vs.addInput[:len(vs.addInput)-1]
		}

	case "ctrl+u":
		vs.addInput = ""

	default:
		ch := msg.String()
		if len(ch) == 1 || ch == " " {
			vs.addInput += ch
		}
	}
	return vs, nil
}

// ensureVisible adjusts the scroll so the cursor stays in the visible area.
func (vs *VaultSwitch) ensureVisible() {
	maxVisible := vs.maxVisible()
	if vs.cursor < vs.scroll {
		vs.scroll = vs.cursor
	}
	if vs.cursor >= vs.scroll+maxVisible {
		vs.scroll = vs.cursor - maxVisible + 1
	}
}

// maxVisible returns the max number of vault entries visible at once.
func (vs VaultSwitch) maxVisible() int {
	// Each vault entry occupies 3 lines (name, path, date) plus 1 separator
	// Reserve some lines for header, help bar, border padding, add-input area
	available := vs.height - 14
	vis := available / 4
	if vis < 3 {
		vis = 3
	}
	return vis
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

// View renders the overlay. Value receiver per Bubble Tea convention.
func (vs VaultSwitch) View() string {
	width := vs.width / 2
	if width < 50 {
		width = 50
	}
	if width > 80 {
		width = 80
	}
	innerWidth := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("  " + IconFolderChar + " Switch Vault"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render("  " + strings.Repeat("─", innerWidth-4)))
	b.WriteString("\n")

	if len(vs.vaults.Vaults) == 0 {
		emptyStyle := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		b.WriteString("\n")
		b.WriteString(emptyStyle.Render("  No vaults registered. Press a to add one."))
		b.WriteString("\n")
	} else {
		maxVisible := vs.maxVisible()
		start := vs.scroll
		end := start + maxVisible
		if end > len(vs.vaults.Vaults) {
			end = len(vs.vaults.Vaults)
		}

		for i := start; i < end; i++ {
			entry := vs.vaults.Vaults[i]

			// Note count
			noteCount := countNotesInDir(entry.Path)
			noteStr := ""
			if noteCount > 0 {
				noteStr = lipgloss.NewStyle().Foreground(green).
					Render(fmt.Sprintf(" %d notes", noteCount))
			}

			// Last opened
			lastOpenStr := formatLastOpen(entry.LastOpen)

			if i == vs.cursor {
				accentBar := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar)
				nameStyle := lipgloss.NewStyle().
					Foreground(mauve).
					Bold(true)
				pathStyle := lipgloss.NewStyle().
					Foreground(overlay0)
				dateStyle := lipgloss.NewStyle().
					Foreground(surface2)
				blockStyle := lipgloss.NewStyle().
					Background(surface0).
					Width(innerWidth)

				nameLine := accentBar + " " + nameStyle.Render(entry.Name) + noteStr
				pathLine := "   " + pathStyle.Render(entry.Path)
				dateLine := "   " + dateStyle.Render(lastOpenStr)

				b.WriteString(blockStyle.Render(nameLine))
				b.WriteString("\n")
				b.WriteString(blockStyle.Render(pathLine))
				b.WriteString("\n")
				b.WriteString(blockStyle.Render(dateLine))
			} else {
				nameLine := "  " + NormalItemStyle.Render(entry.Name) + noteStr
				pathLine := "  " + DimStyle.Render(entry.Path)
				dateLine := "  " + lipgloss.NewStyle().Foreground(surface2).Render(lastOpenStr)

				b.WriteString(nameLine)
				b.WriteString("\n")
				b.WriteString(pathLine)
				b.WriteString("\n")
				b.WriteString(dateLine)
			}

			if i < end-1 {
				b.WriteString("\n")
				b.WriteString(lipgloss.NewStyle().Foreground(surface0).
					Render("  " + strings.Repeat("·", innerWidth-4)))
			}
			b.WriteString("\n")
		}

		// Scroll indicator
		if len(vs.vaults.Vaults) > maxVisible {
			moreStyle := lipgloss.NewStyle().Foreground(surface2).Italic(true)
			remaining := len(vs.vaults.Vaults) - end
			if remaining > 0 {
				b.WriteString(moreStyle.Render(fmt.Sprintf("  +%d more...", remaining)))
				b.WriteString("\n")
			}
		}
	}

	// Add-vault input area
	if vs.adding {
		b.WriteString("\n")
		promptStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
		b.WriteString(promptStyle.Render("  Enter vault path:"))
		b.WriteString("\n")

		cursorChar := lipgloss.NewStyle().Foreground(mauve).Render("|")
		inputStyle := lipgloss.NewStyle().
			Foreground(text).
			Background(surface0).
			Padding(0, 1).
			Width(innerWidth - 2)
		b.WriteString("  " + inputStyle.Render(vs.addInput+cursorChar))
		b.WriteString("\n")
	}

	// Help bar at bottom of overlay
	b.WriteString("\n")
	if vs.adding {
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"Enter", "confirm"}, {"Esc", "cancel"},
		}))
	} else {
		b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
			{"j/k", "navigate"}, {"Enter", "switch"}, {"a", "add"},
			{"d", "remove"}, {"Esc", "close"},
		}))
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
