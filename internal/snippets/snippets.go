// Package snippets is the shared snippet/template engine used by both
// the granit TUI editor and the web Codemirror editor. Lifted out of
// internal/tui so the web side can import the same builtin set, the
// same trigger semantics, and the same {{date}}/{{time}} placeholder
// expansion — keeping a single source of truth means new snippets show
// up everywhere with one edit.
//
// Pure data + logic. No UI, no rendering. Stdlib-only imports so any
// caller (TUI, HTTP handler, future scheduled job) can pull it in
// without bringing along bubbletea or any other heavyweight dep.
package snippets

import (
	"strings"
	"time"
)

// Snippet represents a shortcode that expands into template text when
// triggered. Trigger is the literal token the editor matches on
// (typically '/' followed by a name); Content is the text that
// replaces the trigger when the user accepts it. Content may contain
// placeholders documented on ExpandPlaceholders.
type Snippet struct {
	Trigger     string
	Description string
	Content     string
}

// Engine manages snippet expansion and matching. Methods are not
// concurrent-safe — typical usage is one Engine per editor session.
type Engine struct {
	snippets []Snippet
}

// builtin is the default snippet library shipped with granit. Order
// here drives the order users see in the autocomplete picker.
var builtin = []Snippet{
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
	{Trigger: "/mermaid", Description: "Mermaid diagram", Content: "```mermaid\nflowchart LR\n  A --> B\n```"},
	{Trigger: "/footnote", Description: "Footnote ref + definition", Content: "[^1]\n\n[^1]: "},
	{Trigger: "/prayer", Description: "Prayer intention block", Content: "## Prayer\n\n- ask: \n- scripture: \n- for: "},
}

// New constructs an Engine pre-loaded with the builtin snippet set.
// Callers can layer additional triggers on top with AddSnippet — for
// example the TUI mixes in zettelkasten templates at startup.
func New() *Engine {
	return &Engine{snippets: append([]Snippet(nil), builtin...)}
}

// AddSnippet appends a custom trigger. Description defaults to the
// trigger string itself when callers don't have a richer label —
// matches the behaviour of the original TUI engine so the integration
// path stays identical.
func (e *Engine) AddSnippet(trigger, content string) {
	e.snippets = append(e.snippets, Snippet{Trigger: trigger, Description: trigger, Content: content})
}

// AddSnippetWithDescription is the explicit form for callers (e.g.
// the future custom-snippets-from-vault loader) that have a label.
func (e *Engine) AddSnippetWithDescription(trigger, description, content string) {
	e.snippets = append(e.snippets, Snippet{Trigger: trigger, Description: description, Content: content})
}

// All returns every loaded snippet in registration order. Returned
// slice is a defensive copy so callers can sort/filter without
// mutating the engine's internal state.
func (e *Engine) All() []Snippet {
	out := make([]Snippet, len(e.snippets))
	copy(out, e.snippets)
	return out
}

// TryExpand checks if the given word matches a snippet trigger and
// returns the expanded content with all placeholders replaced. The
// ok return value indicates whether a matching snippet was found.
func (e *Engine) TryExpand(word string) (expanded string, ok bool) {
	for _, s := range e.snippets {
		if s.Trigger == word {
			return e.ExpandPlaceholders(s.Content), true
		}
	}
	return "", false
}

// MatchPrefix returns all snippets whose trigger starts with the
// given prefix. Used by the autocomplete picker as the user types
// (`/m` → meeting, mermaid, etc.). Empty prefix returns nil — the
// caller decides whether "show everything" makes sense.
func (e *Engine) MatchPrefix(prefix string) []Snippet {
	if prefix == "" {
		return nil
	}
	var matches []Snippet
	for _, s := range e.snippets {
		if strings.HasPrefix(s.Trigger, prefix) {
			matches = append(matches, s)
		}
	}
	return matches
}

// ExpandPlaceholders replaces template tokens with their current
// values:
//
//	{{date}}     → YYYY-MM-DD
//	{{time}}     → HH:MM
//	{{datetime}} → YYYY-MM-DD HH:MM
//	{{title}}    → empty string (caller fills in)
//
// Exposed as a method so a future engine variant could override the
// substitution table (e.g. inject a per-vault timezone) without
// rewriting every callsite.
func (e *Engine) ExpandPlaceholders(content string) string {
	now := time.Now()
	content = strings.ReplaceAll(content, "{{date}}", now.Format("2006-01-02"))
	content = strings.ReplaceAll(content, "{{time}}", now.Format("15:04"))
	content = strings.ReplaceAll(content, "{{datetime}}", now.Format("2006-01-02 15:04"))
	content = strings.ReplaceAll(content, "{{title}}", "")
	return content
}
