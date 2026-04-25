package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// quickPicker is a tiny inline overlay for "pick one of N
// short-labeled options with a single keypress." Used by the
// triage queue for `s` (schedule when?) and `z` (snooze how
// long?), but it's generic — any place that wants
// "1=tomorrow, 2=+3d, 3=+1w" reuses it.
//
// Single-line render so it sits below the focused content
// instead of opening a modal. Power-user UX: number keys pick
// directly, no arrow navigation needed.
type quickPicker struct {
	active   bool
	label    string
	options  []pickerOption
	result   pickerOption
	resultOK bool
}

// pickerOption is one choice. Key is the digit shown to the
// user (and the actual key they press); Label is the
// description; Value carries the picked data — typed as
// time.Duration here because every current use is a relative
// date offset, but a future use could swap this for any
// payload by promoting Value to interface{}.
type pickerOption struct {
	Key   string
	Label string
	Value time.Duration
}

// Open activates the picker with the given label and options.
// The first option is the default — Enter (with no number)
// picks it.
func (p *quickPicker) Open(label string, options []pickerOption) {
	p.active = true
	p.label = label
	p.options = options
	p.resultOK = false
}

// IsActive reports whether the picker is currently up.
func (p *quickPicker) IsActive() bool { return p.active }

// Update consumes one key. Returns handled=true when the key was
// the picker's (so the caller doesn't double-process).
func (p *quickPicker) Update(msg tea.Msg) bool {
	if !p.active {
		return false
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false
	}
	key := keyMsg.String()
	switch key {
	case "esc", "q":
		p.active = false
		return true
	case "enter":
		if len(p.options) > 0 {
			p.result = p.options[0]
			p.resultOK = true
		}
		p.active = false
		return true
	}
	// Number key direct pick.
	for _, opt := range p.options {
		if opt.Key == key {
			p.result = opt
			p.resultOK = true
			p.active = false
			return true
		}
	}
	// Unrecognized key while picker is up — consume it so it
	// doesn't leak into the underlying overlay (e.g. typing 'd'
	// while the picker is up shouldn't fire triage's drop).
	return true
}

// Result returns the picked option once. Subsequent calls
// return false (consumed-once, matches every other overlay's
// pattern in the codebase).
func (p *quickPicker) Result() (pickerOption, bool) {
	if !p.resultOK {
		return pickerOption{}, false
	}
	r := p.result
	p.resultOK = false
	return r, true
}

// View renders the inline option strip. Designed to be folded
// into a parent overlay's View output, not displayed
// standalone.
func (p *quickPicker) View() string {
	if !p.active {
		return ""
	}
	header := lipgloss.NewStyle().Bold(true).Render(p.label)
	parts := make([]string, len(p.options))
	for i, opt := range p.options {
		key := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")).Render(opt.Key)
		parts[i] = key + " " + opt.Label
	}
	hint := lipgloss.NewStyle().Faint(true).Render("enter=default · esc cancel")
	return header + "\n" + strings.Join(parts, "  ·  ") + "\n" + hint
}

// scheduleOptions returns the "schedule when?" choices for
// triage's s key. "today" is meaningful here ("work on this
// today") even though that means ScheduledStart=now.
func scheduleOptions() []pickerOption {
	day := 24 * time.Hour
	return []pickerOption{
		{Key: "1", Label: "today", Value: 0},
		{Key: "2", Label: "tomorrow", Value: day},
		{Key: "3", Label: "+3d", Value: 3 * day},
		{Key: "4", Label: "+1w", Value: 7 * day},
		{Key: "5", Label: "+1mo", Value: 30 * day},
	}
}

// snoozeOptions returns the "snooze how long?" choices for
// triage's z key. Skips "today" — snoozing to today is a
// no-op; user wants future dates only.
func snoozeOptions() []pickerOption {
	day := 24 * time.Hour
	return []pickerOption{
		{Key: "1", Label: "tomorrow", Value: day},
		{Key: "2", Label: "+3d", Value: 3 * day},
		{Key: "3", Label: "+1w", Value: 7 * day},
		{Key: "4", Label: "+2w", Value: 14 * day},
		{Key: "5", Label: "+1mo", Value: 30 * day},
	}
}
