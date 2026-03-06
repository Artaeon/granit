package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/config"
)

// VaultSelector is a full-screen Bubble Tea model shown before the main app
// when no vault path is provided. It lets the user pick from recent vaults,
// open an existing path, or create a new vault.
type VaultSelector struct {
	vaults   config.VaultList
	cursor   int
	mode     int    // 0=list, 1=new path input, 2=new vault name
	input    string
	width    int
	height   int
	selected string // the vault path user selected
	done     bool

	// New vault creation
	newPath string
	newName string
}

// NewVaultSelector creates a VaultSelector pre-loaded with the persisted vault list.
func NewVaultSelector() VaultSelector {
	vl := config.LoadVaultList()
	return VaultSelector{
		vaults: vl,
		mode:   0,
	}
}

// Init satisfies tea.Model.
func (vs VaultSelector) Init() tea.Cmd {
	return nil
}

// Update satisfies tea.Model.
func (vs VaultSelector) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		vs.width = msg.Width
		vs.height = msg.Height
		return vs, nil

	case tea.KeyMsg:
		switch vs.mode {
		case 0:
			return vs.updateList(msg)
		case 1:
			return vs.updatePathInput(msg)
		case 2:
			return vs.updateNameInput(msg)
		}
	}
	return vs, nil
}

// updateList handles key events in the vault list mode (mode 0).
func (vs VaultSelector) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c", "ctrl+q":
		return vs, tea.Quit

	case "up", "k":
		if vs.cursor > 0 {
			vs.cursor--
		}

	case "down", "j":
		if vs.cursor < len(vs.vaults.Vaults)-1 {
			vs.cursor++
		}

	case "enter":
		if len(vs.vaults.Vaults) > 0 && vs.cursor < len(vs.vaults.Vaults) {
			entry := vs.vaults.Vaults[vs.cursor]
			vs.selected = entry.Path
			vs.done = true
			// Update last-used info and persist.
			vs.vaults.AddVault(entry.Path)
			config.SaveVaultList(vs.vaults)
		}

	case "n":
		// Create new vault.
		vs.mode = 1
		vs.input = ""

	case "o":
		// Open existing path.
		vs.mode = 1
		vs.input = ""

	case "d":
		if len(vs.vaults.Vaults) > 0 && vs.cursor < len(vs.vaults.Vaults) {
			entry := vs.vaults.Vaults[vs.cursor]
			vs.vaults.RemoveVault(entry.Path)
			config.SaveVaultList(vs.vaults)
			if vs.cursor >= len(vs.vaults.Vaults) && vs.cursor > 0 {
				vs.cursor--
			}
		}
	}
	return vs, nil
}

// updatePathInput handles key events in the path input mode (mode 1).
func (vs VaultSelector) updatePathInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		vs.mode = 0
		vs.input = ""

	case "enter":
		path := strings.TrimSpace(vs.input)
		if path == "" {
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

		info, err := os.Stat(absPath)
		if err == nil && info.IsDir() {
			// Existing directory -- open it directly.
			vs.selected = absPath
			vs.done = true
			vs.vaults.AddVault(absPath)
			config.SaveVaultList(vs.vaults)
		} else {
			// Path doesn't exist -- switch to name input for new vault creation.
			vs.newPath = absPath
			vs.newName = filepath.Base(absPath)
			vs.input = vs.newName
			vs.mode = 2
		}

	case "backspace":
		if len(vs.input) > 0 {
			vs.input = vs.input[:len(vs.input)-1]
		}

	case "ctrl+u":
		vs.input = ""

	default:
		// Only accept printable single characters.
		if len(msg.String()) == 1 || msg.String() == " " {
			vs.input += msg.String()
		}
	}
	return vs, nil
}

// updateNameInput handles key events in the new vault name mode (mode 2).
func (vs VaultSelector) updateNameInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		vs.mode = 0
		vs.input = ""
		vs.newPath = ""
		vs.newName = ""

	case "enter":
		name := strings.TrimSpace(vs.input)
		if name == "" {
			return vs, nil
		}

		vs.newName = name

		// Create the vault directory.
		if err := os.MkdirAll(vs.newPath, 0755); err != nil {
			// Fall back to list mode on error.
			vs.mode = 0
			return vs, nil
		}

		vs.selected = vs.newPath
		vs.done = true

		// Update the vault name if it differs from the basename.
		vs.vaults.AddVault(vs.newPath)
		// Fix the name to what the user typed.
		for i, v := range vs.vaults.Vaults {
			if v.Path == vs.newPath {
				vs.vaults.Vaults[i].Name = vs.newName
				break
			}
		}
		config.SaveVaultList(vs.vaults)

	case "backspace":
		if len(vs.input) > 0 {
			vs.input = vs.input[:len(vs.input)-1]
		}

	case "ctrl+u":
		vs.input = ""

	default:
		if len(msg.String()) == 1 || msg.String() == " " {
			vs.input += msg.String()
		}
	}
	return vs, nil
}

// View satisfies tea.Model.
func (vs VaultSelector) View() string {
	if vs.width == 0 || vs.height == 0 {
		return ""
	}

	switch vs.mode {
	case 1:
		return vs.viewPathInput()
	case 2:
		return vs.viewNameInput()
	default:
		return vs.viewList()
	}
}

// SelectedVault returns the vault path selected by the user.
func (vs *VaultSelector) SelectedVault() string {
	return vs.selected
}

// IsDone returns true when the user has made a selection (or quit).
func (vs *VaultSelector) IsDone() bool {
	return vs.done
}

// ---------------------------------------------------------------------------
// View helpers
// ---------------------------------------------------------------------------

// viewList renders the main vault list (mode 0).
func (vs VaultSelector) viewList() string {
	var content strings.Builder

	// Logo
	logoStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	for _, line := range splashLogo {
		content.WriteString(logoStyle.Render(line))
		content.WriteString("\n")
	}
	content.WriteString("\n")

	tagline := lipgloss.NewStyle().
		Foreground(overlay0).
		Italic(true).
		Render("  Terminal Knowledge Manager")
	content.WriteString(tagline)
	content.WriteString("\n\n")

	// Vault list box
	panelWidth := vs.width / 2
	if panelWidth < 50 {
		panelWidth = 50
	}
	if panelWidth > 80 {
		panelWidth = 80
	}
	innerWidth := panelWidth - 6 // border(2) + padding(2*2)

	var list strings.Builder

	listTitle := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Vaults")
	list.WriteString(listTitle)
	list.WriteString("\n")
	list.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))
	list.WriteString("\n")

	if len(vs.vaults.Vaults) == 0 {
		emptyMsg := lipgloss.NewStyle().Foreground(overlay0).Italic(true).
			Render("  No vaults yet. Press n to create one or o to open a path.")
		list.WriteString(emptyMsg)
		list.WriteString("\n")
	} else {
		// Calculate visible window.
		maxVisible := vs.height - 22
		if maxVisible < 3 {
			maxVisible = 3
		}
		if maxVisible > len(vs.vaults.Vaults) {
			maxVisible = len(vs.vaults.Vaults)
		}

		start := 0
		if vs.cursor >= maxVisible {
			start = vs.cursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(vs.vaults.Vaults) {
			end = len(vs.vaults.Vaults)
		}

		for i := start; i < end; i++ {
			entry := vs.vaults.Vaults[i]

			// Note count hint.
			noteCount := countNotesInDir(entry.Path)
			noteStr := ""
			if noteCount > 0 {
				noteStr = lipgloss.NewStyle().Foreground(green).
					Render(fmt.Sprintf(" %d notes", noteCount))
			}

			// Format last opened date nicely.
			lastOpenStr := formatLastOpen(entry.LastOpen)

			if i == vs.cursor {
				// Selected vault line.
				nameStyle := lipgloss.NewStyle().
					Foreground(mauve).
					Bold(true)
				pathStyle := lipgloss.NewStyle().
					Foreground(overlay0)
				indicator := lipgloss.NewStyle().
					Foreground(mauve).
					Bold(true).
					Render(" > ")

				nameLine := indicator + nameStyle.Render(entry.Name) + noteStr
				pathLine := "   " + pathStyle.Render(entry.Path)
				dateLine := "   " + lipgloss.NewStyle().Foreground(surface2).Render(lastOpenStr)

				// Highlight background for the block.
				blockStyle := lipgloss.NewStyle().
					Background(surface0).
					Width(innerWidth)

				list.WriteString(blockStyle.Render(nameLine))
				list.WriteString("\n")
				list.WriteString(blockStyle.Render(pathLine))
				list.WriteString("\n")
				list.WriteString(blockStyle.Render(dateLine))
			} else {
				nameStyle := NormalItemStyle
				pathStyle := DimStyle

				nameLine := "   " + nameStyle.Render(entry.Name) + noteStr
				pathLine := "   " + pathStyle.Render(entry.Path)
				dateLine := "   " + lipgloss.NewStyle().Foreground(surface2).Render(lastOpenStr)

				list.WriteString(nameLine)
				list.WriteString("\n")
				list.WriteString(pathLine)
				list.WriteString("\n")
				list.WriteString(dateLine)
			}

			if i < end-1 {
				list.WriteString("\n")
				list.WriteString(lipgloss.NewStyle().Foreground(surface0).
					Render(strings.Repeat("·", innerWidth)))
			}
			list.WriteString("\n")
		}
	}

	list.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerWidth)))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(surface1).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	content.WriteString(border.Render(list.String()))
	content.WriteString("\n\n")

	// Help bar
	helpBar := vs.renderHelpBar([]vsHelpBinding{
		{"j/k", "navigate"},
		{"Enter", "open"},
		{"n", "new vault"},
		{"o", "open path"},
		{"d", "remove"},
		{"q", "quit"},
	})
	content.WriteString(helpBar)

	return lipgloss.Place(
		vs.width,
		vs.height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
		lipgloss.WithWhitespaceBackground(crust),
	)
}

// viewPathInput renders the path input screen (mode 1).
func (vs VaultSelector) viewPathInput() string {
	var content strings.Builder

	// Logo (smaller, just the title)
	logoStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	for _, line := range splashLogo {
		content.WriteString(logoStyle.Render(line))
		content.WriteString("\n")
	}
	content.WriteString("\n")

	panelWidth := vs.width / 2
	if panelWidth < 50 {
		panelWidth = 50
	}
	if panelWidth > 80 {
		panelWidth = 80
	}
	innerWidth := panelWidth - 6

	var panel strings.Builder

	prompt := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Enter vault path:")
	panel.WriteString(prompt)
	panel.WriteString("\n\n")

	// Input field
	inputDisplay := vs.input
	cursorChar := lipgloss.NewStyle().
		Background(text).
		Foreground(base).
		Render(" ")
	inputStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1).
		Width(innerWidth)

	panel.WriteString(inputStyle.Render(inputDisplay + cursorChar))
	panel.WriteString("\n\n")

	// Hint
	hint := lipgloss.NewStyle().
		Foreground(overlay0).
		Italic(true).
		Render("  Enter an existing directory path or a new path to create")
	panel.WriteString(hint)
	panel.WriteString("\n")

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	content.WriteString(border.Render(panel.String()))
	content.WriteString("\n\n")

	// Help bar
	helpBar := vs.renderHelpBar([]vsHelpBinding{
		{"Enter", "confirm"},
		{"Esc", "back"},
	})
	content.WriteString(helpBar)

	return lipgloss.Place(
		vs.width,
		vs.height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
		lipgloss.WithWhitespaceBackground(crust),
	)
}

// viewNameInput renders the new vault name screen (mode 2).
func (vs VaultSelector) viewNameInput() string {
	var content strings.Builder

	// Logo
	logoStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	for _, line := range splashLogo {
		content.WriteString(logoStyle.Render(line))
		content.WriteString("\n")
	}
	content.WriteString("\n")

	panelWidth := vs.width / 2
	if panelWidth < 50 {
		panelWidth = 50
	}
	if panelWidth > 80 {
		panelWidth = 80
	}
	innerWidth := panelWidth - 6

	var panel strings.Builder

	prompt := lipgloss.NewStyle().
		Foreground(peach).
		Bold(true).
		Render("  Name for new vault:")
	panel.WriteString(prompt)
	panel.WriteString("\n\n")

	// Show the path being created.
	pathInfo := lipgloss.NewStyle().
		Foreground(overlay0).
		Render("  Path: " + vs.newPath)
	panel.WriteString(pathInfo)
	panel.WriteString("\n\n")

	// Input field
	inputDisplay := vs.input
	cursorChar := lipgloss.NewStyle().
		Background(text).
		Foreground(base).
		Render(" ")
	inputStyle := lipgloss.NewStyle().
		Foreground(text).
		Background(surface0).
		Padding(0, 1).
		Width(innerWidth)

	panel.WriteString(inputStyle.Render(inputDisplay + cursorChar))
	panel.WriteString("\n")

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(peach).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	content.WriteString(border.Render(panel.String()))
	content.WriteString("\n\n")

	// Help bar
	helpBar := vs.renderHelpBar([]vsHelpBinding{
		{"Enter", "create & open"},
		{"Esc", "back"},
	})
	content.WriteString(helpBar)

	return lipgloss.Place(
		vs.width,
		vs.height,
		lipgloss.Center,
		lipgloss.Center,
		content.String(),
		lipgloss.WithWhitespaceBackground(crust),
	)
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

type vsHelpBinding struct {
	key  string
	desc string
}

// renderHelpBar builds a bottom help bar string from key/description pairs.
func (vs VaultSelector) renderHelpBar(bindings []vsHelpBinding) string {
	var parts []string
	for _, b := range bindings {
		key := HelpKeyStyle.Render(b.key)
		desc := HelpDescStyle.Render(" " + b.desc)
		parts = append(parts, key+desc)
	}
	return HelpBarStyle.Width(vs.width).Render(strings.Join(parts, "    "))
}

// formatLastOpen returns a human-friendly string from an ISO date string,
// e.g. "today", "yesterday", "3 days ago", "2 weeks ago".
func formatLastOpen(isoDate string) string {
	if isoDate == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return isoDate
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	opened := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())

	days := int(today.Sub(opened).Hours() / 24)

	switch {
	case days == 0:
		return "opened today"
	case days == 1:
		return "opened yesterday"
	case days < 7:
		return fmt.Sprintf("opened %d days ago", days)
	case days < 30:
		weeks := days / 7
		if weeks == 1 {
			return "opened 1 week ago"
		}
		return fmt.Sprintf("opened %d weeks ago", weeks)
	case days < 365:
		months := days / 30
		if months == 1 {
			return "opened 1 month ago"
		}
		return fmt.Sprintf("opened %d months ago", months)
	default:
		years := days / 365
		if years == 1 {
			return "opened 1 year ago"
		}
		return fmt.Sprintf("opened %d years ago", years)
	}
}

// countNotesInDir counts .md files in a directory (non-recursive, quick check).
// Returns 0 if the directory cannot be read.
func countNotesInDir(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			count++
		}
	}
	// Also check one level of subdirectories for a more accurate count.
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			subEntries, err := os.ReadDir(filepath.Join(dir, e.Name()))
			if err != nil {
				continue
			}
			for _, se := range subEntries {
				if !se.IsDir() && strings.HasSuffix(strings.ToLower(se.Name()), ".md") {
					count++
				}
			}
		}
	}
	return count
}
