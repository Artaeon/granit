package tui

import (
	"strings"
	"time"
)

// Snippet represents a shortcode that expands into template text when triggered.
type Snippet struct {
	Trigger     string // e.g. "/meeting", "/todo", "/date"
	Description string
	Content     string // the expanded text (supports {{date}}, {{time}} placeholders)
}

// SnippetEngine manages snippet expansion and matching.
type SnippetEngine struct {
	snippets []Snippet
}

// builtinSnippets contains all built-in snippets available by default.
var builtinSnippets = []Snippet{
	{Trigger: "/date", Description: "Insert today's date", Content: "{{date}}"},
	{Trigger: "/time", Description: "Insert current time", Content: "{{time}}"},
	{Trigger: "/datetime", Description: "Insert date and time", Content: "{{date}} {{time}}"},
	{Trigger: "/todo", Description: "Insert checkbox", Content: "- [ ] "},
	{Trigger: "/done", Description: "Insert checked box", Content: "- [x] "},
	{Trigger: "/h1", Description: "Heading 1", Content: "# "},
	{Trigger: "/h2", Description: "Heading 2", Content: "## "},
	{Trigger: "/h3", Description: "Heading 3", Content: "### "},
	{Trigger: "/link", Description: "Insert wikilink", Content: "[[]]"},
	{Trigger: "/code", Description: "Code block", Content: "```\n\n```"},
	{Trigger: "/table", Description: "Insert table", Content: "| Column 1 | Column 2 | Column 3 |\n|----------|----------|----------|\n|          |          |          |"},
	{Trigger: "/meeting", Description: "Meeting notes template", Content: "## Meeting Notes\n\n**Date:** {{date}}\n**Attendees:**\n-\n\n**Agenda:**\n1.\n\n**Notes:**\n\n**Action Items:**\n- [ ] "},
	{Trigger: "/daily", Description: "Daily note header", Content: "# {{date}}\n\n## Tasks\n- [ ] \n\n## Notes\n\n## Reflection\n"},
	{Trigger: "/callout", Description: "Callout block", Content: "> [!note]\n> "},
	{Trigger: "/divider", Description: "Horizontal divider", Content: "\n---\n"},
	{Trigger: "/quote", Description: "Block quote", Content: "> "},
	{Trigger: "/img", Description: "Image placeholder", Content: "![alt text](url)"},
	{Trigger: "/frontmatter", Description: "YAML frontmatter", Content: "---\ntitle: \ndate: {{date}}\ntags: []\n---\n"},
}

// NewSnippetEngine creates a new SnippetEngine loaded with built-in snippets.
func NewSnippetEngine() *SnippetEngine {
	return &SnippetEngine{
		snippets: builtinSnippets,
	}
}

// GetSnippets returns all available snippets.
func (se *SnippetEngine) GetSnippets() []Snippet {
	return se.snippets
}

// TryExpand checks if the given word matches a snippet trigger and returns the
// expanded content with all placeholders replaced. The ok return value indicates
// whether a matching snippet was found.
func (se *SnippetEngine) TryExpand(word string) (expanded string, ok bool) {
	for _, s := range se.snippets {
		if s.Trigger == word {
			return se.ExpandPlaceholders(s.Content), true
		}
	}
	return "", false
}

// MatchPrefix returns all snippets whose trigger starts with the given prefix.
// This is used for autocomplete suggestions as the user types a snippet trigger.
func (se *SnippetEngine) MatchPrefix(prefix string) []Snippet {
	if prefix == "" {
		return nil
	}
	var matches []Snippet
	for _, s := range se.snippets {
		if strings.HasPrefix(s.Trigger, prefix) {
			matches = append(matches, s)
		}
	}
	return matches
}

// ExpandPlaceholders replaces template placeholders in content with their
// corresponding values. Supported placeholders:
//   - {{date}}     — current date in YYYY-MM-DD format
//   - {{time}}     — current time in HH:MM format
//   - {{datetime}} — current date and time in YYYY-MM-DD HH:MM format
//   - {{title}}    — empty string (to be filled by the user)
func (se *SnippetEngine) ExpandPlaceholders(content string) string {
	now := time.Now()
	content = strings.ReplaceAll(content, "{{date}}", now.Format("2006-01-02"))
	content = strings.ReplaceAll(content, "{{time}}", now.Format("15:04"))
	content = strings.ReplaceAll(content, "{{datetime}}", now.Format("2006-01-02 15:04"))
	content = strings.ReplaceAll(content, "{{title}}", "")
	return content
}
