package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type quickSwitchItem struct {
	path    string
	modTime string // formatted relative time like "2m ago", "1h ago", "3d ago"
	starred bool
}

type QuickSwitch struct {
	active bool
	all    []quickSwitchItem // unfiltered, in display order from Open()
	items  []quickSwitchItem // filtered view shown to the user
	cursor int
	query  string
	width  int
	height int
	result string
}

func NewQuickSwitch() QuickSwitch {
	return QuickSwitch{}
}

func (qs *QuickSwitch) SetSize(width, height int) {
	qs.width = width
	qs.height = height
}

// Open builds the item list from the provided sources.
// Order: starred files first, then recent files, then remaining files sorted
// by modification time (most recent first). Duplicates are suppressed.
func (qs *QuickSwitch) Open(recentFiles []string, starredFiles []string, allPaths []string, getModTime func(string) time.Time) {
	qs.active = true
	qs.cursor = 0
	qs.result = ""
	qs.query = ""
	qs.all = nil
	qs.items = nil

	seen := make(map[string]bool)
	now := time.Now()

	// Helper to append a path if not already seen.
	addItem := func(path string, starred bool) {
		if seen[path] {
			return
		}
		seen[path] = true
		mt := getModTime(path)
		qs.all = append(qs.all, quickSwitchItem{
			path:    path,
			modTime: formatRelativeTime(now, mt),
			starred: starred,
		})
	}

	// Build a set of starred paths for quick lookup.
	starredSet := make(map[string]bool, len(starredFiles))
	for _, s := range starredFiles {
		starredSet[s] = true
	}

	// 1. Starred files first.
	for _, path := range starredFiles {
		addItem(path, true)
	}

	// 2. Recent files (mark starred if applicable).
	for _, path := range recentFiles {
		addItem(path, starredSet[path])
	}

	// 3. Remaining files sorted by modification time (most recent first).
	type modEntry struct {
		path string
		mt   time.Time
	}
	var remaining []modEntry
	for _, path := range allPaths {
		if seen[path] {
			continue
		}
		remaining = append(remaining, modEntry{path: path, mt: getModTime(path)})
	}
	sort.Slice(remaining, func(i, j int) bool {
		return remaining[i].mt.After(remaining[j].mt)
	})
	for _, entry := range remaining {
		addItem(entry.path, starredSet[entry.path])
	}

	qs.applyFilter()
}

// applyFilter rebuilds qs.items from qs.all according to the current
// query. Empty query keeps the original Open() ordering (starred →
// recent → modtime). With a query, items are scored by fuzzy match on
// the file basename + path; non-matches are dropped, matches sort by
// score with starred status as a soft tie-breaker.
func (qs *QuickSwitch) applyFilter() {
	if qs.query == "" {
		qs.items = make([]quickSwitchItem, len(qs.all))
		copy(qs.items, qs.all)
		if qs.cursor >= len(qs.items) {
			qs.cursor = maxInt(0, len(qs.items)-1)
		}
		return
	}
	q := strings.ToLower(qs.query)
	type scored struct {
		item  quickSwitchItem
		score int
	}
	var hits []scored
	for _, it := range qs.all {
		base := strings.ToLower(strings.TrimSuffix(filepath.Base(it.path), ".md"))
		full := strings.ToLower(it.path)
		// Basename matches outrank deep-path matches: people remember the
		// note title, not where it lives in the folder tree.
		s := cmdFuzzyScore(base, q)
		if pathScore := cmdFuzzyScore(full, q) / 3; pathScore > s {
			s = pathScore
		}
		if s == 0 {
			continue
		}
		if it.starred {
			s += 30 // small boost; doesn't override a strong fuzzy hit
		}
		hits = append(hits, scored{it, s})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		return hits[i].score > hits[j].score
	})
	qs.items = make([]quickSwitchItem, len(hits))
	for i, h := range hits {
		qs.items[i] = h.item
	}
	if qs.cursor >= len(qs.items) {
		qs.cursor = maxInt(0, len(qs.items)-1)
	}
}

func (qs *QuickSwitch) Close() {
	qs.active = false
}

func (qs *QuickSwitch) IsActive() bool {
	return qs.active
}

// SelectedFile returns the selected path and resets the result.
func (qs *QuickSwitch) SelectedFile() string {
	r := qs.result
	qs.result = ""
	return r
}

func (qs QuickSwitch) Update(msg tea.Msg) (QuickSwitch, tea.Cmd) {
	if !qs.active {
		return qs, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "tab":
			qs.active = false
		case "up", "ctrl+k", "ctrl+p":
			if qs.cursor > 0 {
				qs.cursor--
			}
		case "down", "ctrl+j", "ctrl+n":
			if qs.cursor < len(qs.items)-1 {
				qs.cursor++
			}
		case "enter":
			if qs.cursor >= len(qs.items) {
				qs.cursor = maxInt(0, len(qs.items)-1)
			}
			if len(qs.items) > 0 && qs.cursor < len(qs.items) {
				qs.result = qs.items[qs.cursor].path
				qs.active = false
			}
		case "backspace":
			if len(qs.query) > 0 {
				qs.query = TrimLastRune(qs.query)
				qs.cursor = 0
				qs.applyFilter()
			}
		default:
			// Printable characters narrow the result set. j/k as raw
			// chars are intentionally treated as typed input, not
			// navigation — vim-style nav lives on Ctrl+J/Ctrl+N so the
			// query field can include those letters in note titles.
			char := msg.String()
			if len(char) == 1 && char[0] >= 32 && char[0] < 127 {
				qs.query += char
				qs.cursor = 0
				qs.applyFilter()
			}
		}
	}
	return qs, nil
}

func (qs QuickSwitch) View() string {
	panelWidth := qs.width / 3
	if panelWidth < 45 {
		panelWidth = 45
	}
	if panelWidth > 65 {
		panelWidth = 65
	}

	// Inner content width accounts for border (2) + padding (2*2 = 4).
	innerWidth := panelWidth - 6

	var b strings.Builder

	// Title + live query field
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Quick Switch")
	b.WriteString(title)
	b.WriteString("\n")
	queryDisplay := qs.query
	if queryDisplay == "" {
		queryDisplay = lipgloss.NewStyle().Foreground(overlay0).Render("type to filter…")
	} else {
		queryDisplay = lipgloss.NewStyle().Foreground(text).Render(queryDisplay) +
			lipgloss.NewStyle().Foreground(mauve).Blink(true).Render("▍")
	}
	b.WriteString("  " + lipgloss.NewStyle().Foreground(overlay0).Render("›") + " " + queryDisplay + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")

	if len(qs.items) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  No files available"))
	} else {
		// Calculate visible window.
		maxVisible := qs.height - 10
		if maxVisible < 5 {
			maxVisible = 5
		}
		if maxVisible > len(qs.items) {
			maxVisible = len(qs.items)
		}

		start := 0
		if qs.cursor >= maxVisible {
			start = qs.cursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(qs.items) {
			end = len(qs.items)
		}

		for i := start; i < end; i++ {
			item := qs.items[i]

			// Star indicator.
			starIcon := "  "
			if item.starred {
				starIcon = lipgloss.NewStyle().Foreground(yellow).Render(" ") + " "
			}

			// File icon based on extension.
			icon := fileIconForPath(item.path)

			// File name (without .md extension for cleanliness).
			name := filepath.Base(item.path)
			name = strings.TrimSuffix(name, ".md")

			// Relative time, right-aligned and dim.
			modTimeStr := lipgloss.NewStyle().Foreground(overlay0).Render(item.modTime)

			// Build line content: star + icon + name ... modTime
			prefix := starIcon + icon + " "
			// We need to calculate how much space the name can take.
			// prefixWidth: star(2) + icon(~2) + space(1) = ~5
			// modTimeWidth: varies, but reserve enough.
			prefixWidth := lipgloss.Width(prefix)
			modTimeWidth := lipgloss.Width(modTimeStr)
			nameMaxWidth := innerWidth - prefixWidth - modTimeWidth - 1
			if nameMaxWidth < 10 {
				nameMaxWidth = 10
			}

			// Truncate name if needed.
			if lipgloss.Width(name) > nameMaxWidth {
				name = name[:nameMaxWidth-1] + "…"
			}

			// Pad name to fill available space for right-aligned time.
			namePadded := name + strings.Repeat(" ", maxInt(0, nameMaxWidth-lipgloss.Width(name)))

			if i == qs.cursor {
				selectedStyle := lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true)
				dimOnSelected := lipgloss.NewStyle().
					Background(surface0).
					Foreground(overlay0)

				line := prefix + selectedStyle.Render(namePadded) + " " + dimOnSelected.Render(item.modTime)
				b.WriteString(selectedStyle.MaxWidth(innerWidth).Render(line))
			} else {
				line := prefix + lipgloss.NewStyle().Foreground(text).Render(namePadded) + " " + modTimeStr
				b.WriteString(line)
			}

			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(strings.Repeat("─", innerWidth)))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render("  type to filter  ↑↓/Ctrl+N/P navigate  Enter open  Esc close"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(lavender).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return border.Render(b.String())
}

// fileIconForPath returns a styled nerd-font icon appropriate for the file.
func fileIconForPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".md":
		return lipgloss.NewStyle().Foreground(blue).Render("")
	case ".txt":
		return lipgloss.NewStyle().Foreground(subtext1).Render("")
	case ".json":
		return lipgloss.NewStyle().Foreground(yellow).Render("")
	case ".yaml", ".yml":
		return lipgloss.NewStyle().Foreground(peach).Render("")
	case ".toml":
		return lipgloss.NewStyle().Foreground(peach).Render("")
	default:
		return lipgloss.NewStyle().Foreground(blue).Render("")
	}
}

// formatRelativeTime returns a human-readable relative time string.
func formatRelativeTime(now, t time.Time) string {
	if t.IsZero() {
		return ""
	}

	d := now.Sub(t)
	if d < 0 {
		d = -d
	}

	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%dm ago", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%dh ago", h)
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%dd ago", days)
	case d < 30*24*time.Hour:
		weeks := int(d.Hours() / (24 * 7))
		return fmt.Sprintf("%dw ago", weeks)
	case d < 365*24*time.Hour:
		months := int(d.Hours() / (24 * 30))
		if months < 1 {
			months = 1
		}
		return fmt.Sprintf("%dmo ago", months)
	default:
		years := int(d.Hours() / (24 * 365))
		if years < 1 {
			years = 1
		}
		return fmt.Sprintf("%dy ago", years)
	}
}
