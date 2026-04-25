package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/profiles"
)

// ProfilePicker is the overlay shown by Alt+W (and the
// profile.switch palette command). Renders the available
// profiles as horizontally-arranged cards so the user can
// arrow-key + Enter through them in one screen — no menu
// walking, no two-step modal flow.
//
// Power-user UX: the focused card is highlighted with an accent
// border; the selection is the focused index when Enter fires.
// Esc cancels (no profile change). 1..9 number keys jump
// directly to that index for users who know which card they
// want.
type ProfilePicker struct {
	OverlayBase
	profiles []*profiles.Profile
	current  string // ID of currently active profile (highlighted differently)
	focused  int

	resultID string // selected profile ID when ok==true
	resultOK bool
}

// NewProfilePicker constructs a fresh picker. Open populates
// state per session; the registry is not held here so the picker
// stays a pure UI struct.
func NewProfilePicker() ProfilePicker {
	return ProfilePicker{}
}

// Open activates the picker with the given profile set and the
// current active profile's ID (used to mark "you are here").
// Focus lands on the current profile so Enter is a no-op
// confirm — power users can dismiss without thinking.
func (p *ProfilePicker) Open(all []*profiles.Profile, currentID string) {
	p.Activate()
	p.profiles = all
	p.current = currentID
	p.resultOK = false
	p.resultID = ""
	p.focused = 0
	for i, prof := range all {
		if prof.ID == currentID {
			p.focused = i
			break
		}
	}
}

// Update handles keyboard. Arrow keys cycle, Enter confirms,
// 1..9 jumps, Esc cancels.
func (p *ProfilePicker) Update(msg tea.Msg) (ProfilePicker, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return *p, nil
	}
	switch keyMsg.String() {
	case "esc", "ctrl+c":
		p.Close()
	case "left", "h":
		if p.focused > 0 {
			p.focused--
		}
	case "right", "l":
		if p.focused < len(p.profiles)-1 {
			p.focused++
		}
	case "enter":
		if p.focused < len(p.profiles) {
			p.resultID = p.profiles[p.focused].ID
			p.resultOK = true
		}
		p.Close()
	default:
		// Numeric jump: 1..9 → index 0..8.
		k := keyMsg.String()
		if len(k) == 1 && k[0] >= '1' && k[0] <= '9' {
			idx := int(k[0] - '1')
			if idx < len(p.profiles) {
				p.focused = idx
			}
		}
	}
	return *p, nil
}

// Result returns the selected profile ID once, if the user
// pressed Enter. Subsequent calls return false (consumed-once,
// matches the established overlay pattern in TaskManager etc.).
func (p *ProfilePicker) Result() (string, bool) {
	if !p.resultOK {
		return "", false
	}
	id := p.resultID
	p.resultOK = false
	return id, true
}

// View renders the cards row.
func (p *ProfilePicker) View() string {
	if len(p.profiles) == 0 {
		return lipgloss.NewStyle().Padding(2, 4).Render("No profiles registered.")
	}
	cards := make([]string, len(p.profiles))
	for i, prof := range p.profiles {
		cards[i] = p.renderCard(prof, i == p.focused, prof.ID == p.current)
	}
	row := lipgloss.JoinHorizontal(lipgloss.Top, cards...)

	header := lipgloss.NewStyle().Bold(true).Render("Switch Profile")
	hint := lipgloss.NewStyle().Faint(true).Render(
		"← → choose · 1-9 jump · enter switch · esc cancel")
	return header + "\n\n" + row + "\n\n" + hint
}

// renderCard draws one profile card. Width is fixed (24 cols) —
// fits 3-4 cards on most terminals; on narrow terminals the
// caller's overlay frame handles overflow gracefully.
func (p *ProfilePicker) renderCard(prof *profiles.Profile, focused, current bool) string {
	const cardWidth = 24
	border := lipgloss.RoundedBorder()
	style := lipgloss.NewStyle().
		Border(border).
		Width(cardWidth).
		Padding(1, 2).
		Margin(0, 1)

	switch {
	case focused:
		style = style.BorderForeground(lipgloss.Color("12"))
	case current:
		style = style.BorderForeground(lipgloss.Color("10"))
	default:
		style = style.BorderForeground(lipgloss.Color("8"))
	}

	name := lipgloss.NewStyle().Bold(true).Render(prof.Name)
	if current {
		name += lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(" •")
	}
	desc := lipgloss.NewStyle().Faint(true).Width(cardWidth - 4).Render(prof.Description)

	moduleCount := "all modules"
	if len(prof.EnabledModules) > 0 {
		moduleCount = fmt.Sprintf("%d modules", len(prof.EnabledModules))
	}
	cellCount := fmt.Sprintf("%d widgets", len(prof.Dashboard.Cells))
	stats := lipgloss.NewStyle().Faint(true).Render(
		strings.Join([]string{moduleCount, cellCount}, " · "))

	return style.Render(name + "\n\n" + desc + "\n\n" + stats)
}
