package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ReadingItem represents a single URL/article to read.
type ReadingItem struct {
	URL      string   `json:"url"`
	Title    string   `json:"title"`
	Tags     []string `json:"tags,omitempty"`
	AddedAt  string   `json:"added_at"`
	ReadAt   string   `json:"read_at,omitempty"`
	Notes    string   `json:"notes,omitempty"`
	Priority int      `json:"priority"`
}

// readingInputMode tracks which input field is active.
type readingInputMode int

const (
	rlInputNone readingInputMode = iota
	rlInputAdd
	rlInputTags
	rlInputFilter
)

// ReadingList is an overlay for tracking URLs and articles to read.
type ReadingList struct {
	active bool
	width  int
	height int

	vaultRoot string
	items     []ReadingItem

	// Navigation
	tab    int // 0=unread, 1=archive
	cursor int
	scroll int

	// Input
	inputMode readingInputMode
	inputBuf  string
	inputStep int // 0=url, 1=title (for add mode)

	// Filter
	filterQuery string
}

// NewReadingList returns a ReadingList in its default (inactive) state.
func NewReadingList() ReadingList {
	return ReadingList{}
}

// IsActive reports whether the reading list overlay is visible.
func (rl ReadingList) IsActive() bool {
	return rl.active
}

// SetSize updates available terminal dimensions.
func (rl *ReadingList) SetSize(w, h int) {
	rl.width = w
	rl.height = h
}

// Open loads reading items from JSON and activates the overlay.
func (rl *ReadingList) Open(vaultRoot string) {
	rl.vaultRoot = vaultRoot
	rl.active = true
	rl.tab = 0
	rl.cursor = 0
	rl.scroll = 0
	rl.inputMode = rlInputNone
	rl.inputBuf = ""
	rl.filterQuery = ""
	rl.loadItems()
}

// Close saves items to JSON and deactivates the overlay.
func (rl *ReadingList) Close() {
	rl.saveItems()
	rl.active = false
}

// AddURL adds a new reading item programmatically.
func (rl *ReadingList) AddURL(url, title string) {
	item := ReadingItem{
		URL:      url,
		Title:    title,
		AddedAt:  time.Now().Format("2006-01-02"),
		Priority: 0,
	}
	rl.items = append(rl.items, item)
	rl.saveItems()
}

// readingListPath returns the storage path.
func (rl *ReadingList) readingListPath() string {
	return filepath.Join(rl.vaultRoot, ".granit", "readinglist.json")
}

// loadItems reads the JSON file.
func (rl *ReadingList) loadItems() {
	rl.items = nil
	raw, err := os.ReadFile(rl.readingListPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(raw, &rl.items)
}

// saveItems writes items to JSON.
func (rl *ReadingList) saveItems() {
	if rl.vaultRoot == "" {
		return
	}
	dir := filepath.Join(rl.vaultRoot, ".granit")
	_ = os.MkdirAll(dir, 0o700)
	raw, err := json.MarshalIndent(rl.items, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(rl.readingListPath(), raw, 0o600)
}

// filtered returns items for the current tab, sorted and filtered.
func (rl *ReadingList) filtered() []int {
	var indices []int
	for i, item := range rl.items {
		isRead := item.ReadAt != ""
		if (rl.tab == 0 && isRead) || (rl.tab == 1 && !isRead) {
			continue
		}
		if rl.filterQuery != "" {
			q := strings.ToLower(rl.filterQuery)
			title := strings.ToLower(item.Title)
			url := strings.ToLower(item.URL)
			tags := strings.ToLower(strings.Join(item.Tags, " "))
			if !strings.Contains(title, q) && !strings.Contains(url, q) && !strings.Contains(tags, q) {
				continue
			}
		}
		indices = append(indices, i)
	}
	// Sort: priority desc, then added date desc
	sort.Slice(indices, func(a, b int) bool {
		ia, ib := rl.items[indices[a]], rl.items[indices[b]]
		if ia.Priority != ib.Priority {
			return ia.Priority > ib.Priority
		}
		return ia.AddedAt > ib.AddedAt
	})
	return indices
}

// Update handles key events.
func (rl ReadingList) Update(msg tea.Msg) (ReadingList, tea.Cmd) {
	if !rl.active {
		return rl, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Input modes first
		if rl.inputMode != rlInputNone {
			return rl.updateInput(msg)
		}

		key := msg.String()
		indices := rl.filtered()

		switch key {
		case "esc":
			if rl.filterQuery != "" {
				rl.filterQuery = ""
				rl.cursor = 0
				rl.scroll = 0
				return rl, nil
			}
			rl.Close()
			return rl, nil
		case "tab":
			rl.tab = (rl.tab + 1) % 2
			rl.cursor = 0
			rl.scroll = 0
			return rl, nil
		case "up", "k":
			if rl.cursor > 0 {
				rl.cursor--
				if rl.cursor < rl.scroll {
					rl.scroll = rl.cursor
				}
			}
		case "down", "j":
			if rl.cursor < len(indices)-1 {
				rl.cursor++
				maxVis := rl.maxVisible()
				if rl.cursor >= rl.scroll+maxVis {
					rl.scroll = rl.cursor - maxVis + 1
				}
			}
		case "a":
			rl.inputMode = rlInputAdd
			rl.inputBuf = ""
			rl.inputStep = 0
		case "d":
			if len(indices) > 0 && rl.cursor < len(indices) {
				idx := indices[rl.cursor]
				if rl.tab == 0 {
					rl.items[idx].ReadAt = time.Now().Format("2006-01-02")
				} else {
					rl.items[idx].ReadAt = ""
				}
				rl.saveItems()
				if rl.cursor >= len(rl.filtered()) && rl.cursor > 0 {
					rl.cursor--
				}
			}
		case "x":
			if len(indices) > 0 && rl.cursor < len(indices) {
				idx := indices[rl.cursor]
				rl.items = append(rl.items[:idx], rl.items[idx+1:]...)
				rl.saveItems()
				if rl.cursor >= len(rl.filtered()) && rl.cursor > 0 {
					rl.cursor--
				}
			}
		case "t":
			if len(indices) > 0 && rl.cursor < len(indices) {
				rl.inputMode = rlInputTags
				rl.inputBuf = strings.Join(rl.items[indices[rl.cursor]].Tags, ", ")
			}
		case "p":
			if len(indices) > 0 && rl.cursor < len(indices) {
				idx := indices[rl.cursor]
				rl.items[idx].Priority = (rl.items[idx].Priority + 1) % 5
				rl.saveItems()
			}
		case "/":
			rl.inputMode = rlInputFilter
			rl.inputBuf = rl.filterQuery
		}
	}
	return rl, nil
}

// updateInput handles keystrokes while in an input mode.
func (rl ReadingList) updateInput(msg tea.KeyMsg) (ReadingList, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		rl.inputMode = rlInputNone
		rl.inputBuf = ""
		return rl, nil
	case "enter":
		switch rl.inputMode {
		case rlInputAdd:
			if rl.inputStep == 0 {
				// URL entered, now ask for title
				if strings.TrimSpace(rl.inputBuf) != "" {
					rl.inputStep = 1
					// Store URL temporarily
					rl.items = append(rl.items, ReadingItem{
						URL:     strings.TrimSpace(rl.inputBuf),
						AddedAt: time.Now().Format("2006-01-02"),
					})
					rl.inputBuf = ""
				}
			} else {
				// Title entered
				if len(rl.items) > 0 {
					rl.items[len(rl.items)-1].Title = strings.TrimSpace(rl.inputBuf)
				}
				rl.saveItems()
				rl.inputMode = rlInputNone
				rl.inputBuf = ""
			}
		case rlInputTags:
			indices := rl.filtered()
			if len(indices) > 0 && rl.cursor < len(indices) {
				idx := indices[rl.cursor]
				tags := strings.Split(rl.inputBuf, ",")
				var cleaned []string
				for _, t := range tags {
					t = strings.TrimSpace(t)
					if t != "" {
						cleaned = append(cleaned, t)
					}
				}
				rl.items[idx].Tags = cleaned
				rl.saveItems()
			}
			rl.inputMode = rlInputNone
			rl.inputBuf = ""
		case rlInputFilter:
			rl.filterQuery = rl.inputBuf
			rl.inputMode = rlInputNone
			rl.inputBuf = ""
			rl.cursor = 0
			rl.scroll = 0
		}
		return rl, nil
	case "backspace":
		if len(rl.inputBuf) > 0 {
			rl.inputBuf = rl.inputBuf[:len(rl.inputBuf)-1]
		}
		return rl, nil
	default:
		if len(key) == 1 && key[0] >= 32 {
			rl.inputBuf += key
		}
		return rl, nil
	}
}

func (rl *ReadingList) maxVisible() int {
	vis := rl.height/2 - 8
	if vis < 5 {
		vis = 5
	}
	if vis > 20 {
		vis = 20
	}
	return vis
}

// View renders the reading list overlay.
func (rl ReadingList) View() string {
	width := rl.width * 3 / 5
	if width < 60 {
		width = 60
	}
	if width > 100 {
		width = 100
	}
	innerW := width - 6

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(titleStyle.Render("Reading List"))
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(surface1).Render(strings.Repeat("─", innerW)))
	b.WriteString("\n")

	// Tabs
	indices := rl.filtered()
	unreadCount := 0
	archiveCount := 0
	for _, item := range rl.items {
		if item.ReadAt == "" {
			unreadCount++
		} else {
			archiveCount++
		}
	}

	tabActive := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	tabInactive := lipgloss.NewStyle().Foreground(overlay0)
	tabSep := lipgloss.NewStyle().Foreground(surface1).Render(" │ ")

	unreadLabel := fmt.Sprintf("Unread (%d)", unreadCount)
	archiveLabel := fmt.Sprintf("Archive (%d)", archiveCount)

	if rl.tab == 0 {
		b.WriteString(tabActive.Render(unreadLabel))
	} else {
		b.WriteString(tabInactive.Render(unreadLabel))
	}
	b.WriteString(tabSep)
	if rl.tab == 1 {
		b.WriteString(tabActive.Render(archiveLabel))
	} else {
		b.WriteString(tabInactive.Render(archiveLabel))
	}

	if rl.filterQuery != "" {
		b.WriteString("  ")
		b.WriteString(lipgloss.NewStyle().Foreground(peach).Render("filter: " + rl.filterQuery))
	}
	b.WriteString("\n\n")

	// Input mode
	if rl.inputMode != rlInputNone {
		promptStyle := lipgloss.NewStyle().Foreground(green).Bold(true)
		inputStyle := lipgloss.NewStyle().Foreground(text)
		cursor := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("│")

		switch rl.inputMode {
		case rlInputAdd:
			if rl.inputStep == 0 {
				b.WriteString(promptStyle.Render("URL: "))
			} else {
				b.WriteString(promptStyle.Render("Title: "))
			}
			b.WriteString(inputStyle.Render(rl.inputBuf) + cursor)
		case rlInputTags:
			b.WriteString(promptStyle.Render("Tags (comma-separated): "))
			b.WriteString(inputStyle.Render(rl.inputBuf) + cursor)
		case rlInputFilter:
			b.WriteString(promptStyle.Render("Filter: "))
			b.WriteString(inputStyle.Render(rl.inputBuf) + cursor)
		}
		b.WriteString("\n\n")
	}

	// Items
	maxVis := rl.maxVisible()
	if len(indices) == 0 {
		dim := lipgloss.NewStyle().Foreground(overlay0).Italic(true)
		if rl.tab == 0 {
			b.WriteString(dim.Render("  No unread items. Press 'a' to add a URL."))
		} else {
			b.WriteString(dim.Render("  No archived items."))
		}
		b.WriteString("\n")
	} else {
		start := rl.scroll
		end := start + maxVis
		if end > len(indices) {
			end = len(indices)
		}

		for vi := start; vi < end; vi++ {
			idx := indices[vi]
			item := rl.items[idx]

			// Display title or URL
			display := item.Title
			if display == "" {
				display = item.URL
			}
			if len(display) > innerW-20 {
				display = display[:innerW-23] + "..."
			}

			// Priority badge
			prioStr := ""
			if item.Priority > 0 {
				prioColors := []lipgloss.Color{overlay0, blue, green, peach, red}
				prioLabels := []string{"", "P4", "P3", "P2", "P1"}
				if item.Priority < len(prioColors) {
					prioStr = lipgloss.NewStyle().
						Foreground(prioColors[item.Priority]).
						Bold(true).
						Render(prioLabels[item.Priority]) + " "
				}
			}

			// Tags
			tagStr := ""
			if len(item.Tags) > 0 {
				tagStyle := lipgloss.NewStyle().Foreground(blue)
				var tagParts []string
				for _, t := range item.Tags {
					tagParts = append(tagParts, tagStyle.Render("#"+t))
				}
				tagStr = " " + strings.Join(tagParts, " ")
			}

			// Date
			dateStr := lipgloss.NewStyle().Foreground(overlay0).Render(" " + item.AddedAt)

			if vi == rl.cursor {
				accent := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(ThemeAccentBar)
				nameStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
				b.WriteString(accent + " " + prioStr + nameStyle.Render(display) + tagStr + dateStr)
			} else {
				nameStyle := lipgloss.NewStyle().Foreground(text)
				b.WriteString("  " + prioStr + nameStyle.Render(display) + tagStr + dateStr)
			}
			b.WriteString("\n")
		}

		if len(indices) > maxVis {
			pos := fmt.Sprintf("  %d/%d", rl.cursor+1, len(indices))
			b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(pos))
			b.WriteString("\n")
		}
	}

	// Footer
	b.WriteString("\n")
	keyStyle := lipgloss.NewStyle().Foreground(lavender).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(overlay0)
	var keys []string
	keys = append(keys, keyStyle.Render("a")+dimStyle.Render(" add"))
	if rl.tab == 0 {
		keys = append(keys, keyStyle.Render("d")+dimStyle.Render(" read"))
	} else {
		keys = append(keys, keyStyle.Render("d")+dimStyle.Render(" unread"))
	}
	keys = append(keys, keyStyle.Render("p")+dimStyle.Render(" priority"))
	keys = append(keys, keyStyle.Render("t")+dimStyle.Render(" tags"))
	keys = append(keys, keyStyle.Render("x")+dimStyle.Render(" delete"))
	keys = append(keys, keyStyle.Render("/")+dimStyle.Render(" filter"))
	keys = append(keys, keyStyle.Render("Tab")+dimStyle.Render(" switch"))
	keys = append(keys, keyStyle.Render("Esc")+dimStyle.Render(" close"))
	b.WriteString(strings.Join(keys, dimStyle.Render("  ")))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(mauve).
		Padding(1, 2).
		Width(width)

	return border.Render(b.String())
}
