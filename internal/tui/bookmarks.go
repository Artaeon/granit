package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ReadingEntry tracks a note in the reading list with progress.
type ReadingEntry struct {
	Path    string `json:"path"`
	Status  string `json:"status"`   // "to-read", "reading", "completed"
	Rating  int    `json:"rating"`   // 0-5 (0=unrated)
	AddedAt string `json:"added_at"` // YYYY-MM-DD
}

type BookmarkData struct {
	Starred []string       `json:"starred"`
	Recent  []string       `json:"recent"`
	Reading []ReadingEntry `json:"reading,omitempty"`
}

type Bookmarks struct {
	active     bool
	data       BookmarkData
	cursor     int
	scroll     int
	width      int
	height     int
	mode       int // 0=starred, 1=recent, 2=reading
	result     string
	vaultRoot  string
	activeNote string // for adding current note to reading list
	maxRecent  int
}

func NewBookmarks(vaultRoot string) Bookmarks {
	bm := Bookmarks{
		vaultRoot: vaultRoot,
		maxRecent: 20,
	}
	bm.load()
	return bm
}

func (bm *Bookmarks) SetSize(width, height int) {
	bm.width = width
	bm.height = height
}

func (bm *Bookmarks) Open() {
	bm.active = true
	bm.cursor = 0
	bm.scroll = 0
	bm.result = ""
}

func (bm *Bookmarks) Close() {
	bm.active = false
}

func (bm *Bookmarks) IsActive() bool {
	return bm.active
}

func (bm *Bookmarks) SelectedNote() string {
	r := bm.result
	bm.result = ""
	return r
}

func (bm *Bookmarks) ToggleStar(path string) {
	for i, s := range bm.data.Starred {
		if s == path {
			bm.data.Starred = append(bm.data.Starred[:i], bm.data.Starred[i+1:]...)
			bm.save()
			return
		}
	}
	bm.data.Starred = append(bm.data.Starred, path)
	bm.save()
}

func (bm *Bookmarks) IsStarred(path string) bool {
	for _, s := range bm.data.Starred {
		if s == path {
			return true
		}
	}
	return false
}

func (bm *Bookmarks) AddRecent(path string) {
	// Remove if already exists
	for i, r := range bm.data.Recent {
		if r == path {
			bm.data.Recent = append(bm.data.Recent[:i], bm.data.Recent[i+1:]...)
			break
		}
	}
	// Prepend
	bm.data.Recent = append([]string{path}, bm.data.Recent...)
	if len(bm.data.Recent) > bm.maxRecent {
		bm.data.Recent = bm.data.Recent[:bm.maxRecent]
	}
	bm.save()
}

func (bm *Bookmarks) currentItems() []string {
	switch bm.mode {
	case 1:
		return bm.data.Recent
	case 2:
		items := make([]string, len(bm.data.Reading))
		for i, r := range bm.data.Reading {
			items[i] = r.Path
		}
		return items
	default:
		return bm.data.Starred
	}
}

// SetActiveNote sets the current note path for "add to reading list".
func (bm *Bookmarks) SetActiveNote(path string) {
	bm.activeNote = path
}

func (bm *Bookmarks) dataPath() string {
	return filepath.Join(bm.vaultRoot, ".granit-bookmarks.json")
}

func (bm *Bookmarks) load() {
	data, err := os.ReadFile(bm.dataPath())
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &bm.data)
}

func (bm *Bookmarks) save() {
	data, err := json.MarshalIndent(bm.data, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(bm.dataPath(), data, 0644)
}

func (bm Bookmarks) Update(msg tea.Msg) (Bookmarks, tea.Cmd) {
	if !bm.active {
		return bm, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		items := bm.currentItems()
		switch msg.String() {
		case "esc", "ctrl+b":
			bm.active = false
		case "tab":
			bm.mode = (bm.mode + 1) % 3
			bm.cursor = 0
			bm.scroll = 0
		case "up", "k":
			if bm.cursor > 0 {
				bm.cursor--
				if bm.cursor < bm.scroll {
					bm.scroll = bm.cursor
				}
			}
		case "down", "j":
			if bm.cursor < len(items)-1 {
				bm.cursor++
				visH := bm.height - 10
				if visH < 1 {
					visH = 1
				}
				if bm.cursor >= bm.scroll+visH {
					bm.scroll = bm.cursor - visH + 1
				}
			}
		case "enter":
			if len(items) == 0 || bm.cursor >= len(items) {
				return bm, nil
			}
			bm.result = items[bm.cursor]
			bm.active = false
		case "d", "delete":
			if bm.mode == 0 && len(bm.data.Starred) > 0 && bm.cursor < len(bm.data.Starred) {
				bm.data.Starred = append(bm.data.Starred[:bm.cursor], bm.data.Starred[bm.cursor+1:]...)
				if bm.cursor >= len(bm.data.Starred) && bm.cursor > 0 {
					bm.cursor--
				}
				visH := bm.height - 10
				if visH < 1 {
					visH = 1
				}
				if bm.cursor < bm.scroll {
					bm.scroll = bm.cursor
				}
				if bm.cursor >= bm.scroll+visH {
					bm.scroll = bm.cursor - visH + 1
				}
				bm.save()
			} else if bm.mode == 2 && len(bm.data.Reading) > 0 && bm.cursor < len(bm.data.Reading) {
				bm.data.Reading = append(bm.data.Reading[:bm.cursor], bm.data.Reading[bm.cursor+1:]...)
				if bm.cursor >= len(bm.data.Reading) && bm.cursor > 0 {
					bm.cursor--
				}
				visH := bm.height - 10
				if visH < 1 {
					visH = 1
				}
				if bm.cursor < bm.scroll {
					bm.scroll = bm.cursor
				}
				if bm.cursor >= bm.scroll+visH {
					bm.scroll = bm.cursor - visH + 1
				}
				bm.save()
			}
		case "p":
			// Cycle reading status
			if bm.mode == 2 && bm.cursor < len(bm.data.Reading) {
				r := &bm.data.Reading[bm.cursor]
				switch r.Status {
				case "to-read":
					r.Status = "reading"
				case "reading":
					r.Status = "completed"
				default:
					r.Status = "to-read"
				}
				bm.save()
			}
		case "r":
			// Cycle rating (0-5)
			if bm.mode == 2 && bm.cursor < len(bm.data.Reading) {
				r := &bm.data.Reading[bm.cursor]
				r.Rating = (r.Rating + 1) % 6
				bm.save()
			}
		case "a":
			// Add current note to reading list
			if bm.mode == 2 && bm.activeNote != "" {
				// Check if already in reading list
				found := false
				for _, r := range bm.data.Reading {
					if r.Path == bm.activeNote {
						found = true
						break
					}
				}
				if !found {
					bm.data.Reading = append(bm.data.Reading, ReadingEntry{
						Path:    bm.activeNote,
						Status:  "to-read",
						AddedAt: time.Now().Format("2006-01-02"),
					})
					bm.save()
				}
			}
		}
	}
	return bm, nil
}

func (bm Bookmarks) View() string {
	width := bm.width / 2
	if width < 50 {
		width = 50
	}
	if width > 70 {
		width = 70
	}

	var b strings.Builder

	// Tab header
	starCount := len(bm.data.Starred)
	recentCount := len(bm.data.Recent)

	activeTabStyle := lipgloss.NewStyle().
		Foreground(crust).
		Background(mauve).
		Bold(true).
		Padding(0, 1)

	inactiveTabStyle := lipgloss.NewStyle().
		Foreground(overlay0).
		Background(surface0).
		Padding(0, 1)

	readingCount := len(bm.data.Reading)
	tabNames := []struct {
		name  string
		count int
	}{
		{"Starred", starCount},
		{"Recent", recentCount},
		{"Reading", readingCount},
	}
	var tabRow string
	for i, t := range tabNames {
		style := inactiveTabStyle
		if bm.mode == i {
			style = activeTabStyle
		}
		if i > 0 {
			tabRow += " "
		}
		tabRow += style.Render(fmt.Sprintf(" %s %s", t.name, smallNum(t.count)))
	}

	b.WriteString(tabRow)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n\n")

	items := bm.currentItems()
	visH := bm.height - 10
	if visH < 5 {
		visH = 5
	}
	if len(items) == 0 {
		switch bm.mode {
		case 0:
			b.WriteString(DimStyle.Render("  No starred notes\n"))
			b.WriteString(DimStyle.Render("  Use Ctrl+B to bookmark a note"))
		case 2:
			b.WriteString(DimStyle.Render("  No reading list items\n"))
			b.WriteString(DimStyle.Render("  Press 'a' to add current note"))
		default:
			b.WriteString(DimStyle.Render("  No recent files"))
		}
	} else {
		end := bm.scroll + visH
		if end > len(items) {
			end = len(items)
		}

		for i := bm.scroll; i < end; i++ {
			name := strings.TrimSuffix(items[i], ".md")
			icon := lipgloss.NewStyle().Foreground(blue).Render(" ")
			if bm.mode == 0 {
				icon = lipgloss.NewStyle().Foreground(yellow).Render(" ")
			}

			// Reading list: show status badge and rating
			suffix := ""
			if bm.mode == 2 && i < len(bm.data.Reading) {
				re := bm.data.Reading[i]
				switch re.Status {
				case "reading":
					icon = lipgloss.NewStyle().Foreground(teal).Render("📖")
				case "completed":
					icon = lipgloss.NewStyle().Foreground(green).Render("✓")
				default:
					icon = DimStyle.Render("○")
				}
				if re.Rating > 0 {
					suffix = " " + lipgloss.NewStyle().Foreground(yellow).Render(strings.Repeat("★", re.Rating))
				}
			}

			if i == bm.cursor {
				line := "  " + icon + " " + name + suffix
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString("  " + icon + " " + NormalItemStyle.Render(name) + suffix)
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}

		// Scroll indicator
		if len(items) > visH {
			b.WriteString("\n")
			b.WriteString(DimStyle.Render("  " + ScrollIndicator(bm.scroll, len(items), visH)))
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")
	var pairs []struct{ Key, Desc string }
	switch bm.mode {
	case 0:
		pairs = []struct{ Key, Desc string }{
			{"Tab", "switch"}, {"Enter", "open"}, {"d", "unstar"}, {"Esc", "close"},
		}
	case 2:
		pairs = []struct{ Key, Desc string }{
			{"Tab", "switch"}, {"Enter", "open"}, {"a", "add note"}, {"p", "status"},
			{"r", "rating"}, {"d", "remove"}, {"Esc", "close"},
		}
	default:
		pairs = []struct{ Key, Desc string }{
			{"Tab", "switch"}, {"Enter", "open"}, {"Esc", "close"},
		}
	}
	b.WriteString(RenderHelpBar(pairs))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
