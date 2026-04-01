package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ToastLevel controls the color and icon of a toast notification.
type ToastLevel int

const (
	ToastInfo ToastLevel = iota
	ToastSuccess
	ToastWarning
	ToastError
)

// ToastItem is a single notification in the toast queue.
type ToastItem struct {
	Message string
	Level   ToastLevel
	Created time.Time
}

// toastExpireMsg is sent when a toast should be dismissed.
type toastExpireMsg struct{}

// Toast manages ephemeral notification messages shown briefly at the top-right.
type Toast struct {
	items    []ToastItem
	duration time.Duration
	width    int
}

// NewToast creates a new Toast manager.
func NewToast() *Toast {
	return &Toast{
		duration: 3 * time.Second,
	}
}

// SetWidth stores the available terminal width for positioning.
func (t *Toast) SetWidth(w int) {
	t.width = w
}

// Show adds a toast notification and returns a command to auto-dismiss it.
func (t *Toast) Show(msg string, level ToastLevel) tea.Cmd {
	t.items = append(t.items, ToastItem{
		Message: msg,
		Level:   level,
		Created: time.Now(),
	})
	return t.expireAfter(t.duration)
}

// ShowInfo is a convenience for info-level toasts.
func (t *Toast) ShowInfo(msg string) tea.Cmd {
	return t.Show(msg, ToastInfo)
}

// ShowSuccess is a convenience for success-level toasts.
func (t *Toast) ShowSuccess(msg string) tea.Cmd {
	return t.Show(msg, ToastSuccess)
}

// ShowWarning is a convenience for warning-level toasts.
func (t *Toast) ShowWarning(msg string) tea.Cmd {
	return t.Show(msg, ToastWarning)
}

// ShowError is a convenience for error-level toasts.
func (t *Toast) ShowError(msg string) tea.Cmd {
	return t.Show(msg, ToastError)
}

// HandleExpire removes expired toasts. Call this when a toastExpireMsg arrives.
func (t *Toast) HandleExpire() {
	now := time.Now()
	var remaining []ToastItem
	for _, item := range t.items {
		if now.Sub(item.Created) < t.duration {
			remaining = append(remaining, item)
		}
	}
	t.items = remaining
}

// HasItems reports whether there are any visible toasts.
func (t *Toast) HasItems() bool {
	return len(t.items) > 0
}

func toastStyle(level ToastLevel) (icon string, color lipgloss.Color) {
	switch level {
	case ToastSuccess:
		return "✓", green
	case ToastWarning:
		return "⚠", yellow
	case ToastError:
		return "✗", red
	default:
		return "ℹ", blue
	}
}

// View renders the toast notifications as a vertical stack.
func (t *Toast) View() string {
	if len(t.items) == 0 {
		return ""
	}

	var lines []string
	// Show at most 3 toasts
	start := 0
	if len(t.items) > 3 {
		start = len(t.items) - 3
	}

	for _, item := range t.items[start:] {
		// ensure accent color and icon matches toast type
		icon, accentColor := toastStyle(item.Level)

		iconStyled := lipgloss.NewStyle().
			Foreground(accentColor).
			Bold(true).
			Render(icon)

		msgStyled := lipgloss.NewStyle().
			Foreground(text).
			Render(item.Message)

		timeStyled := lipgloss.NewStyle().
			Foreground(surface2).
			Render("now")

		content := lipgloss.JoinHorizontal(lipgloss.Left,
			iconStyled,
			"  ",
			msgStyled,
			"  ",
			timeStyled,
		)

		toastBox := lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(accentColor).
			Background(mantle).
			Padding(0, 1).
			MarginBottom(1).
			Render(content)

		lines = append(lines, toastBox)
	}

	return lipgloss.JoinVertical(lipgloss.Right, lines...)
}

func (t *Toast) expireAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg {
		return toastExpireMsg{}
	})
}
