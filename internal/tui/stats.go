package tui

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artaeon/granit/internal/vault"
)

type VaultStats struct {
	active     bool
	vault      *vault.Vault
	index      *vault.Index
	width      int
	height     int
	scroll     int

	totalNotes     int
	totalWords     int
	totalLinks     int
	totalBacklinks int
	totalTags      int
	orphanNotes    int
	avgLinks       float64
	topLinked      []statEntry
	topTags        []statEntry
	largestNotes   []statEntry
}

type statEntry struct {
	name  string
	value int
}

func NewVaultStats(v *vault.Vault, idx *vault.Index) VaultStats {
	return VaultStats{
		vault: v,
		index: idx,
	}
}

func (vs *VaultStats) SetSize(width, height int) {
	vs.width = width
	vs.height = height
}

func (vs *VaultStats) Open() {
	vs.active = true
	vs.scroll = 0
	vs.compute()
}

func (vs *VaultStats) Close() {
	vs.active = false
}

func (vs *VaultStats) IsActive() bool {
	return vs.active
}

func (vs *VaultStats) compute() {
	vs.totalNotes = vs.vault.NoteCount()
	vs.totalWords = 0
	vs.totalLinks = 0
	vs.totalBacklinks = 0
	vs.orphanNotes = 0

	tagMap := make(map[string]int)
	var linked []statEntry
	var sizes []statEntry

	for _, path := range vs.vault.SortedPaths() {
		note := vs.vault.GetNote(path)
		if note == nil {
			continue
		}

		words := len(strings.Fields(note.Content))
		vs.totalWords += words
		vs.totalLinks += len(note.Links)

		backlinks := vs.index.GetBacklinks(path)
		vs.totalBacklinks += len(backlinks)

		if len(note.Links) == 0 && len(backlinks) == 0 {
			vs.orphanNotes++
		}

		linked = append(linked, statEntry{
			name:  strings.TrimSuffix(path, ".md"),
			value: len(note.Links) + len(backlinks),
		})

		sizes = append(sizes, statEntry{
			name:  strings.TrimSuffix(path, ".md"),
			value: words,
		})

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
	}

	if vs.totalNotes > 0 {
		vs.avgLinks = float64(vs.totalLinks) / float64(vs.totalNotes)
	}

	vs.totalTags = len(tagMap)

	// Top linked
	sort.Slice(linked, func(i, j int) bool { return linked[i].value > linked[j].value })
	if len(linked) > 5 {
		linked = linked[:5]
	}
	vs.topLinked = linked

	// Largest notes
	sort.Slice(sizes, func(i, j int) bool { return sizes[i].value > sizes[j].value })
	if len(sizes) > 5 {
		sizes = sizes[:5]
	}
	vs.largestNotes = sizes

	// Top tags
	var tagEntries []statEntry
	for name, count := range tagMap {
		tagEntries = append(tagEntries, statEntry{name: name, value: count})
	}
	sort.Slice(tagEntries, func(i, j int) bool { return tagEntries[i].value > tagEntries[j].value })
	if len(tagEntries) > 5 {
		tagEntries = tagEntries[:5]
	}
	vs.topTags = tagEntries
}

func (vs VaultStats) Update(msg tea.Msg) (VaultStats, tea.Cmd) {
	if !vs.active {
		return vs, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "q":
			vs.active = false
		case "up", "k":
			vs.scroll--
			if vs.scroll < 0 {
				vs.scroll = 0
			}
		case "down", "j":
			vs.scroll++
		}
		if vs.scroll < 0 {
			vs.scroll = 0
		}
	}
	return vs, nil
}

func (vs VaultStats) View() string {
	width := vs.width * 2 / 3
	if width < 55 {
		width = 55
	}
	if width > 80 {
		width = 80
	}

	var lines []string

	sectionStyle := lipgloss.NewStyle().Foreground(blue).Bold(true)
	labelStyle := lipgloss.NewStyle().Foreground(text)
	numStyle := lipgloss.NewStyle().Foreground(peach).Bold(true)
	barStyle := lipgloss.NewStyle().Foreground(mauve)

	// Overview
	lines = append(lines, sectionStyle.Render("  Overview"))
	lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
	lines = append(lines, labelStyle.Render("  Total Notes:     ")+numStyle.Render(smallNum(vs.totalNotes)))
	lines = append(lines, labelStyle.Render("  Total Words:     ")+numStyle.Render(formatNum(vs.totalWords)))
	lines = append(lines, labelStyle.Render("  Total Links:     ")+numStyle.Render(smallNum(vs.totalLinks)))
	lines = append(lines, labelStyle.Render("  Total Backlinks: ")+numStyle.Render(smallNum(vs.totalBacklinks)))
	lines = append(lines, labelStyle.Render("  Unique Tags:     ")+numStyle.Render(smallNum(vs.totalTags)))
	lines = append(lines, labelStyle.Render("  Orphan Notes:    ")+numStyle.Render(smallNum(vs.orphanNotes)))
	avgStr := smallNum(int(vs.avgLinks)) + "." + smallNum(int(vs.avgLinks*10)%10)
	lines = append(lines, labelStyle.Render("  Avg Links/Note:  ")+numStyle.Render(avgStr))

	lines = append(lines, "")

	// Most connected
	if len(vs.topLinked) > 0 {
		lines = append(lines, sectionStyle.Render("  Most Connected Notes"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		maxVal := vs.topLinked[0].value
		for _, entry := range vs.topLinked {
			barLen := 0
			if maxVal > 0 {
				barLen = entry.value * 20 / maxVal
			}
			if barLen < 1 && entry.value > 0 {
				barLen = 1
			}
			name := entry.name
			if len(name) > 20 {
				name = name[:17] + "..."
			}
			bar := barStyle.Render(strings.Repeat("█", barLen)) + DimStyle.Render(strings.Repeat("░", 20-barLen))
			lines = append(lines, "  "+labelStyle.Render(padRight(name, 22))+bar+" "+numStyle.Render(smallNum(entry.value)))
		}
	}

	lines = append(lines, "")

	// Largest notes
	if len(vs.largestNotes) > 0 {
		lines = append(lines, sectionStyle.Render("  Largest Notes (by words)"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		maxVal := vs.largestNotes[0].value
		for _, entry := range vs.largestNotes {
			barLen := 0
			if maxVal > 0 {
				barLen = entry.value * 20 / maxVal
			}
			if barLen < 1 && entry.value > 0 {
				barLen = 1
			}
			name := entry.name
			if len(name) > 20 {
				name = name[:17] + "..."
			}
			bar := barStyle.Render(strings.Repeat("█", barLen)) + DimStyle.Render(strings.Repeat("░", 20-barLen))
			lines = append(lines, "  "+labelStyle.Render(padRight(name, 22))+bar+" "+numStyle.Render(formatNum(entry.value)+" w"))
		}
	}

	lines = append(lines, "")

	// Top tags
	if len(vs.topTags) > 0 {
		lines = append(lines, sectionStyle.Render("  Top Tags"))
		lines = append(lines, DimStyle.Render("  "+strings.Repeat("─", 30)))
		for _, entry := range vs.topTags {
			tagPill := lipgloss.NewStyle().
				Foreground(crust).
				Background(blue).
				Render(" #" + entry.name + " ")
			lines = append(lines, "  "+tagPill+" "+numStyle.Render(smallNum(entry.value)+" notes"))
		}
	}

	// Build view with scroll
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  Vault Statistics")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("─", width-6)))
	b.WriteString("\n")

	visH := vs.height - 8
	if visH < 10 {
		visH = 10
	}
	maxScroll := len(lines) - visH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if vs.scroll > maxScroll {
		vs.scroll = maxScroll
	}
	end := vs.scroll + visH
	if end > len(lines) {
		end = len(lines)
	}

	for i := vs.scroll; i < end; i++ {
		b.WriteString(lines[i])
		if i < end-1 {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  j/k: scroll  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func formatNum(n int) string {
	s := smallNum(n)
	if n < 1000 {
		return s
	}
	// Add thousand separators
	result := ""
	for i, ch := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result += ","
		}
		result += string(ch)
	}
	return result
}
