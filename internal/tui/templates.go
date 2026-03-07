package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type NoteTemplate struct {
	name     string
	content  string
	isUser   bool // true if loaded from vault's templates/ folder
}

type Templates struct {
	active    bool
	cursor    int
	scroll    int
	width     int
	height    int
	templates []NoteTemplate
	result    string
	selected  bool
}

func NewTemplates() Templates {
	return Templates{
		templates: builtinTemplates(),
	}
}

func builtinTemplates() []NoteTemplate {
	return []NoteTemplate{
		{
			name: "Blank Note (no template)",
			content: "",
		},
		{
			name: "Standard Note",
			content: `---
title: {{title}}
date: {{date}}
tags: []
---

# {{title}}

`,
		},
		{
			name: "Meeting Notes",
			content: `---
title: Meeting Notes
date: {{date}}
type: meeting
tags: [meeting]
---

# Meeting Notes

## Attendees
-

## Agenda
1.

## Notes


## Action Items
- [ ]
`,
		},
		{
			name: "Project Plan",
			content: `---
title: Project Plan
date: {{date}}
type: project
tags: [project]
---

# Project Plan

## Overview


## Goals
-

## Timeline
| Phase | Start | End | Status |
|-------|-------|-----|--------|
|       |       |     |        |

## Tasks
- [ ]

## Resources
-
`,
		},
		{
			name: "Weekly Review",
			content: `---
title: Weekly Review
date: {{date}}
type: review
tags: [weekly, review]
---

# Weekly Review - {{date}}

## Accomplishments
-

## Challenges
-

## Next Week
- [ ]

## Notes

`,
		},
		{
			name: "Book Notes",
			content: `---
title: Book Notes
date: {{date}}
author: ""
type: book
tags: [book, notes]
---

# Book Notes

## Summary


## Key Ideas
1.

## Quotes
>

## Thoughts

`,
		},
		{
			name: "Decision Record",
			content: `---
title: Decision Record
date: {{date}}
status: proposed
type: decision
tags: [decision]
---

# Decision Record

## Context


## Decision


## Consequences

### Positive
-

### Negative
-

### Risks
-
`,
		},
		{
			name: "Journal Entry",
			content: `---
title: Journal - {{date}}
date: {{date}}
type: journal
tags: [journal]
---

# {{date}}

## Mood


## What happened today


## Gratitude
1.
2.
3.

## Tomorrow
- [ ]
`,
		},
		{
			name: "Research Note",
			content: `---
title: {{title}}
date: {{date}}
type: research
tags: [research]
source: ""
---

# {{title}}

## Key Findings


## Methodology


## Data / Evidence


## Questions
-

## Related Notes
-
`,
		},
		{
			name: "Learning Note (Zettelkasten)",
			content: `---
title: {{title}}
date: {{date}}
type: zettel
tags: []
---

# {{title}}

## Main Idea


## In My Own Words


## Source


## Connections
- [[]]

## Questions
-
`,
		},
	}
}

func (t *Templates) IsActive() bool {
	return t.active
}

func (t *Templates) Open() {
	t.active = true
	t.cursor = 0
	t.scroll = 0
	t.result = ""
	t.selected = false
}

// OpenWithVault loads user templates from the vault's templates/ folder
// and merges them with built-in templates.
func (t *Templates) OpenWithVault(vaultRoot string) {
	t.Open()
	t.templates = builtinTemplates()
	userTemplates := loadUserTemplates(vaultRoot)
	if len(userTemplates) > 0 {
		// Insert separator then user templates after "Blank Note"
		t.templates = append(t.templates, userTemplates...)
	}
}

// loadUserTemplates scans vaultRoot/templates/ for .md files.
func loadUserTemplates(vaultRoot string) []NoteTemplate {
	templatesDir := filepath.Join(vaultRoot, "templates")
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		return nil
	}
	var templates []NoteTemplate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(templatesDir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md")
		templates = append(templates, NoteTemplate{
			name:    name,
			content: string(data),
			isUser:  true,
		})
	}
	return templates
}

func (t *Templates) Close() {
	t.active = false
}

func (t *Templates) SetSize(width, height int) {
	t.width = width
	t.height = height
}

func (t *Templates) WasSelected() bool {
	return t.selected
}

func (t *Templates) SelectedTemplate() string {
	if !t.selected {
		return ""
	}
	t.selected = false
	r := t.result
	t.result = ""
	today := time.Now().Format("2006-01-02")
	r = strings.ReplaceAll(r, "{{date}}", today)
	return r // may be "" for blank note
}

func (t Templates) Update(msg tea.Msg) (Templates, tea.Cmd) {
	if !t.active {
		return t, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			t.active = false
		case "up", "k":
			if t.cursor > 0 {
				t.cursor--
				if t.cursor < t.scroll {
					t.scroll = t.cursor
				}
			}
		case "down", "j":
			if t.cursor < len(t.templates)-1 {
				t.cursor++
				visH := t.height - 10
				if visH < 1 {
					visH = 1
				}
				if t.cursor >= t.scroll+visH {
					t.scroll = t.cursor - visH + 1
				}
			}
		case "enter":
			if len(t.templates) > 0 && t.cursor < len(t.templates) {
				t.result = t.templates[t.cursor].content
				t.selected = true
				t.active = false
			}
		}
	}
	return t, nil
}

func (t Templates) View() string {
	width := t.width / 2
	if width < 50 {
		width = 50
	}
	if width > 70 {
		width = 70
	}

	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconNewChar + " New Note — Choose Template")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	if len(t.templates) == 0 {
		b.WriteString(DimStyle.Render("  No templates available"))
		b.WriteString("\n")
	} else {
		visH := t.height - 10
		if visH < 5 {
			visH = 5
		}
		end := t.scroll + visH
		if end > len(t.templates) {
			end = len(t.templates)
		}

		icons := []lipgloss.Style{
			lipgloss.NewStyle().Foreground(blue),
			lipgloss.NewStyle().Foreground(peach),
			lipgloss.NewStyle().Foreground(green),
			lipgloss.NewStyle().Foreground(sapphire),
			lipgloss.NewStyle().Foreground(yellow),
			lipgloss.NewStyle().Foreground(red),
		}

		iconChars := []string{" ", " ", " ", " ", " ", " "}

		showedUserHeader := false
		for i := t.scroll; i < end; i++ {
			tmpl := t.templates[i]

			// Show "Your Templates" header before first user template
			if tmpl.isUser && !showedUserHeader {
				if i > t.scroll {
					b.WriteString("\n")
				}
				b.WriteString("  " + lipgloss.NewStyle().Foreground(lavender).Bold(true).Render("Your Templates"))
				b.WriteString("\n")
				showedUserHeader = true
			}

			iconIdx := i % len(icons)
			icon := icons[iconIdx].Render(iconChars[iconIdx])
			if tmpl.isUser {
				icon = lipgloss.NewStyle().Foreground(green).Render("*")
			}

			if i == t.cursor {
				line := "  " + icon + " " + tmpl.name
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString("  " + icon + " " + NormalItemStyle.Render(tmpl.name))
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  Enter: select template  Esc: close"))

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(blue).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}
