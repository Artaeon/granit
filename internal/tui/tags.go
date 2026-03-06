package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/artaeon/granit/internal/vault"
)

type TagBrowser struct {
	active   bool
	vault    *vault.Vault
	tags     []tagEntry
	cursor   int
	scroll   int
	width    int
	height   int
	mode     int // 0=tag list, 1=notes for selected tag
	selected string // selected tag
	notes    []string // notes for selected tag
	noteCursor int
	result   string // note path to navigate to
}

type tagEntry struct {
	name  string
	count int
}

func NewTagBrowser(v *vault.Vault) TagBrowser {
	return TagBrowser{
		vault: v,
	}
}

func (t *TagBrowser) SetSize(width, height int) {
	t.width = width
	t.height = height
}

func (t *TagBrowser) Open() {
	t.active = true
	t.cursor = 0
	t.scroll = 0
	t.mode = 0
	t.selected = ""
	t.result = ""
	t.collectTags()
}

func (t *TagBrowser) Close() {
	t.active = false
}

func (t *TagBrowser) IsActive() bool {
	return t.active
}

func (t *TagBrowser) SelectedNote() string {
	s := t.result
	t.result = ""
	return s
}

func (t *TagBrowser) collectTags() {
	tagMap := make(map[string]int)

	for _, path := range t.vault.SortedPaths() {
		note := t.vault.GetNote(path)
		if note == nil {
			continue
		}

		// Extract tags from frontmatter
		if tags, ok := note.Frontmatter["tags"]; ok {
			switch v := tags.(type) {
			case []string:
				for _, tag := range v {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tagMap[tag]++
					}
				}
			case string:
				for _, tag := range strings.Split(v, ",") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tagMap[tag]++
					}
				}
			}
		}

		// Also extract inline #tags from content
		words := strings.Fields(note.Content)
		for _, word := range words {
			if strings.HasPrefix(word, "#") && len(word) > 1 {
				tag := strings.TrimRight(word[1:], ".,;:!?)")
				if tag != "" && !strings.HasPrefix(tag, "#") {
					tagMap[tag]++
				}
			}
		}
	}

	t.tags = nil
	for name, count := range tagMap {
		t.tags = append(t.tags, tagEntry{name: name, count: count})
	}

	sort.Slice(t.tags, func(i, j int) bool {
		if t.tags[i].count != t.tags[j].count {
			return t.tags[i].count > t.tags[j].count
		}
		return t.tags[i].name < t.tags[j].name
	})
}

func (t *TagBrowser) findNotesForTag(tag string) {
	t.notes = nil
	for _, path := range t.vault.SortedPaths() {
		note := t.vault.GetNote(path)
		if note == nil {
			continue
		}

		found := false
		// Check frontmatter tags
		if tags, ok := note.Frontmatter["tags"]; ok {
			switch v := tags.(type) {
			case []string:
				for _, ft := range v {
					if strings.TrimSpace(ft) == tag {
						found = true
						break
					}
				}
			case string:
				for _, ft := range strings.Split(v, ",") {
					if strings.TrimSpace(ft) == tag {
						found = true
						break
					}
				}
			}
		}

		// Check inline tags
		if !found && strings.Contains(note.Content, "#"+tag) {
			found = true
		}

		if found {
			t.notes = append(t.notes, path)
		}
	}
}

func (t TagBrowser) Update(msg tea.Msg) (TagBrowser, tea.Cmd) {
	if !t.active {
		return t, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if t.mode == 1 {
				t.mode = 0
				t.noteCursor = 0
				return t, nil
			}
			t.active = false
			return t, nil
		case "ctrl+t":
			t.active = false
			return t, nil
		case "up", "k":
			if t.mode == 0 {
				if t.cursor > 0 {
					t.cursor--
				}
			} else {
				if t.noteCursor > 0 {
					t.noteCursor--
				}
			}
		case "down", "j":
			if t.mode == 0 {
				if t.cursor < len(t.tags)-1 {
					t.cursor++
				}
			} else {
				if t.noteCursor < len(t.notes)-1 {
					t.noteCursor++
				}
			}
		case "enter":
			if t.mode == 0 && len(t.tags) > 0 {
				t.selected = t.tags[t.cursor].name
				t.findNotesForTag(t.selected)
				t.mode = 1
				t.noteCursor = 0
			} else if t.mode == 1 && len(t.notes) > 0 {
				t.result = t.notes[t.noteCursor]
				t.active = false
			}
			return t, nil
		}
	}
	return t, nil
}

func (t TagBrowser) View() string {
	width := t.width / 2
	if width < 50 {
		width = 50
	}
	if width > 70 {
		width = 70
	}

	var b strings.Builder

	if t.mode == 0 {
		// Tag list
		title := lipgloss.NewStyle().
			Foreground(yellow).
			Bold(true).
			Render("  Tags")
		b.WriteString(title)
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
		b.WriteString("\n\n")

		if len(t.tags) == 0 {
			b.WriteString(DimStyle.Render("  No tags found"))
		} else {
			visH := t.height - 10
			if visH < 5 {
				visH = 5
			}
			start := 0
			if t.cursor >= visH {
				start = t.cursor - visH + 1
			}
			end := start + visH
			if end > len(t.tags) {
				end = len(t.tags)
			}

			for i := start; i < end; i++ {
				tag := t.tags[i]
				icon := lipgloss.NewStyle().Foreground(yellow).Render(" ")
				count := DimStyle.Render(" (" + smallNum(tag.count) + ")")

				tagPill := lipgloss.NewStyle().
					Foreground(lipgloss.Color("#11111B")).
					Background(blue).
					Render(" #" + tag.name + " ")

				if i == t.cursor {
					line := "  " + icon + " " + tagPill + count
					b.WriteString(lipgloss.NewStyle().
						Background(surface0).
						Width(width - 6).
						Render(line))
				} else {
					b.WriteString("  " + icon + " " + tagPill + count)
				}
				if i < end-1 {
					b.WriteString("\n")
				}
			}
		}
	} else {
		// Notes for selected tag
		title := lipgloss.NewStyle().
			Foreground(yellow).
			Bold(true).
			Render("  #" + t.selected)
		b.WriteString(title)
		b.WriteString("  ")
		b.WriteString(DimStyle.Render(smallNum(len(t.notes)) + " notes"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
		b.WriteString("\n\n")

		if len(t.notes) == 0 {
			b.WriteString(DimStyle.Render("  No notes found"))
		} else {
			visH := t.height - 10
			if visH < 5 {
				visH = 5
			}
			start := 0
			if t.noteCursor >= visH {
				start = t.noteCursor - visH + 1
			}
			end := start + visH
			if end > len(t.notes) {
				end = len(t.notes)
			}

			for i := start; i < end; i++ {
				name := strings.TrimSuffix(t.notes[i], ".md")
				icon := lipgloss.NewStyle().Foreground(blue).Render(" ")

				if i == t.noteCursor {
					line := "  " + icon + " " + name
					b.WriteString(lipgloss.NewStyle().
						Background(surface0).
						Foreground(peach).
						Bold(true).
						Width(width - 6).
						Render(line))
				} else {
					b.WriteString("  " + icon + " " + NormalItemStyle.Render(name))
				}
				if i < end-1 {
					b.WriteString("\n")
				}
			}
		}

		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Esc: back to tags  Enter: open note"))
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(yellow).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
