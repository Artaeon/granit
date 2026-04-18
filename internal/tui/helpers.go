package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Shared helpers — reusable across all TUI components to eliminate redundancy
// ---------------------------------------------------------------------------

// parseHHMM parses "HH:MM" into (hours, minutes). Returns (0, 0) on failure.
func parseHHMM(s string) (int, int) {
	h, m := 0, 0
	_, _ = fmt.Sscanf(s, "%d:%d", &h, &m)
	return h, m
}

// makePill renders a small colored badge pill: colored background, crust text, bold.
// Used consistently across all overlays for OVERDUE/TODAY/FOCUS/TARGET/etc. labels.
func makePill(bg lipgloss.Color, label string) string {
	return lipgloss.NewStyle().
		Foreground(crust).
		Background(bg).
		Bold(true).
		Padding(0, 1).
		Render(label)
}

// ---------------------------------------------------------------------------
// Date comparison helpers — exported for use across all files.
// Canonical implementations live in taskmanager.go (tm* functions); these are
// thin wrappers with clearer names.
// ---------------------------------------------------------------------------

// IsToday returns true if dateStr ("YYYY-MM-DD") is today.
func IsToday(dateStr string) bool { return tmIsToday(dateStr) }

// IsOverdue returns true if dateStr is before today (and not empty).
func IsOverdue(dateStr string) bool { return tmIsOverdue(dateStr) }

// DaysUntil returns the number of days from today to dateStr. Negative = overdue.
func DaysUntil(dateStr string) int { return tmDaysUntil(dateStr) }

// FormatDueDate formats a due date as a human-readable relative string.
func FormatDueDate(dateStr string) string { return tmFormatDue(dateStr) }

// ---------------------------------------------------------------------------
// Time formatting helpers
// ---------------------------------------------------------------------------

// FormatMinutes formats minutes as "30m" or "1h30m".
func FormatMinutes(mins int) string {
	if mins <= 0 {
		return ""
	}
	h, m := mins/60, mins%60
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh%02dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

// TodayStr returns today's date as "YYYY-MM-DD".
func TodayStr() string {
	return time.Now().Format("2006-01-02")
}

// ---------------------------------------------------------------------------
// Priority helpers — single source of truth for icons and colors
// ---------------------------------------------------------------------------

// PriorityIcon returns a short icon string for a priority level.
func PriorityIcon(p int) string {
	return tmPriorityIcon(p)
}

// PriorityColor returns the theme color for a priority level.
func PriorityColor(p int) lipgloss.Color {
	return tmPriorityColor(p)
}

// ---------------------------------------------------------------------------
// Time-block helpers
// ---------------------------------------------------------------------------

// timeBlockGroup classifies a task into a Plan view group based on its
// ScheduledTime and DueDate. Returns one of: "morning", "midday",
// "afternoon", "evening", "overdue", "today", "tomorrow", or "".
func timeBlockGroup(t Task) string {
	if t.ScheduledTime != "" {
		parts := strings.SplitN(t.ScheduledTime, "-", 2)
		if len(parts) >= 1 {
			h, _ := parseHHMM(parts[0])
			switch {
			case h < 10:
				return "morning"
			case h < 14:
				return "midday"
			case h < 18:
				return "afternoon"
			default:
				return "evening"
			}
		}
	}
	if tmIsOverdue(t.DueDate) {
		return "overdue"
	}
	if tmIsToday(t.DueDate) {
		return "today"
	}
	if t.DueDate != "" && tmDaysUntil(t.DueDate) == 1 {
		return "tomorrow"
	}
	return ""
}

// ---------------------------------------------------------------------------
// String helpers
// ---------------------------------------------------------------------------

// PlanWrap breaks text into lines of at most maxWidth characters.
func PlanWrap(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	var lines []string
	currentLine := words[0]
	for _, word := range words[1:] {
		if len(currentLine)+1+len(word) <= maxWidth {
			currentLine += " " + word
		} else {
			lines = append(lines, currentLine)
			currentLine = word
		}
	}
	lines = append(lines, currentLine)
	return strings.Join(lines, "\n")
}
