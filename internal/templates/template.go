// Package templates holds note-template data shared between granit's TUI
// (`Ctrl+N` template picker) and serveapi's web "new note from template"
// flow. Single source of truth so the two surfaces never drift.
package templates

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Template is one entry shown in the picker. Content is raw markdown with
// {{date}} and {{title}} placeholders that callers expand via Apply.
//
// IsUser=true marks templates loaded from the vault's `.granit/templates/`
// folder (so the UI can label them differently from built-ins).
type Template struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	IsUser  bool   `json:"isUser,omitempty"`
}

// Builtin returns the granit-shipped template set. Order matters — the TUI
// surfaces them in this order, and we keep parity here so a user moving
// between TUI and web sees the same picker.
func Builtin() []Template {
	return []Template{
		{Name: "Blank Note (no template)", Content: ""},
		{
			Name: "Standard Note",
			Content: `---
title: {{title}}
date: {{date}}
tags: []
---

# {{title}}

`,
		},
		{
			Name: "Meeting Notes",
			Content: `---
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
			Name: "Project Plan",
			Content: `---
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
			Name: "Weekly Review",
			Content: `---
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
			Name: "Book Notes",
			Content: `---
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
			Name: "Idea / Concept",
			Content: `---
title: {{title}}
date: {{date}}
type: idea
tags: [idea]
---

# {{title}}

## What
A one-line description.

## Why
What problem does it solve?

## How
First steps to validate / build.

## Open questions
-

`,
		},
		{
			Name: "Person / Contact",
			Content: `---
title: {{title}}
date: {{date}}
type: person
tags: [person]
last_contact: {{date}}
---

# {{title}}

## How we know each other


## Context


## Notes

`,
		},
		{
			Name: "Daily Journal",
			Content: `---
date: {{date}}
type: journal
tags: [journal]
---

# {{date}}

## What happened


## Mood

## Gratitude
1.
2.
3.

## Tomorrow
-

`,
		},
		{
			Name: "Research Note",
			Content: `---
title: {{title}}
date: {{date}}
type: research
tags: [research]
sources: 0
status: active
---

# {{title}}

## Topic


## Sources
-

## Findings


## Open questions
-

`,
		},
	}
}

// LoadUser reads `<vaultRoot>/.granit/templates/*.md` and returns one
// Template per file (Name = filename without .md, IsUser = true). Missing
// directory is a no-op.
func LoadUser(vaultRoot string) []Template {
	if vaultRoot == "" {
		return nil
	}
	dir := filepath.Join(vaultRoot, ".granit", "templates")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Template
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, Template{
			Name:    strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
			Content: string(data),
			IsUser:  true,
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// Apply substitutes {{date}} (YYYY-MM-DD), {{title}}, and {{date_long}}
// (Monday, January 2, 2006) in raw template content.
func Apply(content, title string, now time.Time) string {
	out := content
	out = strings.ReplaceAll(out, "{{date}}", now.Format("2006-01-02"))
	out = strings.ReplaceAll(out, "{{date_long}}", now.Format("Monday, January 2, 2006"))
	out = strings.ReplaceAll(out, "{{title}}", title)
	return out
}

// All returns Builtin() followed by LoadUser(vaultRoot), so the picker
// shows built-ins first and user templates after.
func All(vaultRoot string) []Template {
	return append(Builtin(), LoadUser(vaultRoot)...)
}
