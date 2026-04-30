package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// SlashMenuMode controls which items are visible when the menu opens.
//
// modeAll is the classic "/" behaviour — insert templates plus AI actions on
// the current line. modeAI is opened via a dedicated shortcut that preserves
// the editor's selection, so only AI items are shown and they target the
// selected text.
type SlashMenuMode int

const (
	SlashMenuModeAll SlashMenuMode = iota
	SlashMenuModeAI
)

// SlashMenuItem represents a single action in the slash command menu. Either
// `Insert` or `Action` is set — never both. Plain inserts splice literal text
// at the cursor; actions dispatch into application-level handlers (currently
// AI selection edits).
type SlashMenuItem struct {
	Command     string // e.g. "heading1"
	Label       string // display name
	Icon        string // prefix icon
	Description string // short description
	Insert      string // text to insert (replaces the "/" trigger)
	Action      string // dispatch identifier, e.g. "ai:rewrite" — empty for plain inserts
}

// SlashMenu is a popup that appears when the user types "/" at the start of a
// line or after a space. It shows available formatting actions, templates,
// and AI selection-edit actions.
type SlashMenu struct {
	active  bool
	mode    SlashMenuMode
	items   []SlashMenuItem
	matches []SlashMenuItem
	cursor  int
	query   string // characters typed after "/"
	line    int    // editor line where "/" was typed
	col     int    // editor col where "/" was typed
	width   int
}

// NewSlashMenu creates a new slash command menu with built-in items.
func NewSlashMenu() *SlashMenu {
	return &SlashMenu{
		items: slashBuiltinItems(),
	}
}

func slashBuiltinItems() []SlashMenuItem {
	return []SlashMenuItem{
		// Headings
		{Command: "h1", Label: "Heading 1", Icon: "H1", Description: "Large heading", Insert: "# "},
		{Command: "h2", Label: "Heading 2", Icon: "H2", Description: "Medium heading", Insert: "## "},
		{Command: "h3", Label: "Heading 3", Icon: "H3", Description: "Small heading", Insert: "### "},

		// Lists & tasks
		{Command: "bullet", Label: "Bullet List", Icon: "•", Description: "Unordered list item", Insert: "- "},
		{Command: "number", Label: "Numbered List", Icon: "1.", Description: "Ordered list item", Insert: "1. "},
		{Command: "todo", Label: "To-do", Icon: "[ ]", Description: "Checkbox task", Insert: "- [ ] "},
		{Command: "done", Label: "Done", Icon: "[x]", Description: "Completed task", Insert: "- [x] "},

		// Blocks
		{Command: "quote", Label: "Quote", Icon: ">", Description: "Block quote", Insert: "> "},
		{Command: "code", Label: "Code Block", Icon: "<>", Description: "Fenced code block", Insert: "```\n\n```"},
		{Command: "callout", Label: "Callout", Icon: "!", Description: "Callout block", Insert: "> [!note]\n> "},
		{Command: "divider", Label: "Divider", Icon: "--", Description: "Horizontal rule", Insert: "\n---\n"},
		{Command: "table", Label: "Table", Icon: "||", Description: "Markdown table", Insert: "| Column 1 | Column 2 | Column 3 |\n|----------|----------|----------|\n|          |          |          |"},

		// Links & media
		{Command: "link", Label: "Wiki Link", Icon: "[[", Description: "Internal note link", Insert: "[[]]"},
		{Command: "image", Label: "Image", Icon: "![", Description: "Image embed", Insert: "![alt text](url)"},
		{Command: "url", Label: "External Link", Icon: "=>", Description: "URL link", Insert: "[text](url)"},

		// Templates
		{Command: "date", Label: "Today's Date", Icon: "D", Description: "Insert current date", Insert: "{{date}}"},
		{Command: "time", Label: "Current Time", Icon: "T", Description: "Insert current time", Insert: "{{time}}"},
		{Command: "frontmatter", Label: "Frontmatter", Icon: "---", Description: "YAML front matter", Insert: "---\ntitle: \ndate: {{date}}\ntags: []\n---\n"},
		{Command: "meeting", Label: "Meeting Notes", Icon: "M", Description: "Meeting template", Insert: "## Meeting Notes\n\n**Date:** {{date}}\n**Attendees:**\n-\n\n**Agenda:**\n1.\n\n**Notes:**\n\n**Action Items:**\n- [ ] "},
		{Command: "daily", Label: "Daily Note", Icon: "DN", Description: "Daily template", Insert: "# {{date}}\n\n## Tasks\n- [ ] \n\n## Notes\n\n## Reflection\n"},

		// Text formatting
		{Command: "bold", Label: "Bold", Icon: "B", Description: "Bold text", Insert: "****"},
		{Command: "italic", Label: "Italic", Icon: "I", Description: "Italic text", Insert: "**"},
		{Command: "highlight", Label: "Highlight", Icon: "==", Description: "Highlighted text", Insert: "===="},
		{Command: "strikethrough", Label: "Strikethrough", Icon: "~~", Description: "Strikethrough text", Insert: "~~~~"},

		// AI selection actions — operate on the editor's current selection
		// (or the current line when there's no selection). Replace the
		// source text with the model's output.
		{Command: "rewrite", Label: "AI: Rewrite", Icon: "AI", Description: "Rewrite for clarity", Action: "ai:rewrite"},
		{Command: "expand", Label: "AI: Expand", Icon: "AI", Description: "Expand with detail", Action: "ai:expand"},
		{Command: "summarize", Label: "AI: Summarize", Icon: "AI", Description: "Summarize in 1–3 sentences", Action: "ai:summarize"},
		{Command: "improve", Label: "AI: Improve", Icon: "AI", Description: "Tighten word choice & flow", Action: "ai:improve"},
		{Command: "shorten", Label: "AI: Shorten", Icon: "AI", Description: "Make it shorter", Action: "ai:shorten"},
		{Command: "fix", Label: "AI: Fix Grammar", Icon: "AI", Description: "Fix grammar and spelling", Action: "ai:fix"},
	}
}

// aiOnlyItems returns the subset of items that are AI actions. Used by
// SlashMenuModeAI when opened via a selection-preserving shortcut.
func slashAIOnlyItems() []SlashMenuItem {
	all := slashBuiltinItems()
	out := make([]SlashMenuItem, 0, 8)
	for _, it := range all {
		if it.Action != "" {
			out = append(out, it)
		}
	}
	return out
}

// IsActive reports whether the slash menu is currently displayed.
func (sm *SlashMenu) IsActive() bool { return sm.active }

// Mode returns the current opening mode (modeAll or modeAI).
func (sm *SlashMenu) Mode() SlashMenuMode { return sm.mode }

// Activate opens the slash menu at the given editor position with the full
// set of items (insert templates + AI actions). Triggered by typing "/".
func (sm *SlashMenu) Activate(line, col int) {
	sm.active = true
	sm.mode = SlashMenuModeAll
	sm.items = slashBuiltinItems()
	sm.line = line
	sm.col = col
	sm.query = ""
	sm.cursor = 0
	sm.matches = sm.items
}

// ActivateAI opens the slash menu showing only AI actions. Triggered by a
// dedicated shortcut so the editor's selection is preserved (typing "/"
// would have replaced it). The (line, col) is the cursor position at the
// time of opening — used only for popover placement.
func (sm *SlashMenu) ActivateAI(line, col int) {
	sm.active = true
	sm.mode = SlashMenuModeAI
	sm.items = slashAIOnlyItems()
	sm.line = line
	sm.col = col
	sm.query = ""
	sm.cursor = 0
	sm.matches = sm.items
}

// Close dismisses the slash menu.
func (sm *SlashMenu) Close() {
	sm.active = false
	sm.query = ""
	sm.matches = nil
}

// SetWidth sets the display width for the menu.
func (sm *SlashMenu) SetWidth(w int) {
	sm.width = w
}

// QueryLen returns the length of the current filter query (chars typed after /).
func (sm *SlashMenu) QueryLen() int {
	return len(sm.query)
}

// HandleKey processes a key press while the slash menu is active.
// Returns:
//   - item: pointer to the chosen SlashMenuItem when the user pressed Enter
//     or Tab on a match. Nil otherwise. The caller routes on item.Insert
//     (literal splice) vs item.Action (e.g. AI dispatch).
//   - consumed: whether the key was handled by the menu (don't pass through
//     to the editor)
//   - closed: whether the menu was closed by this key
func (sm *SlashMenu) HandleKey(key string) (item *SlashMenuItem, consumed bool, closed bool) {
	switch key {
	case "esc":
		sm.Close()
		return nil, true, true

	case "enter", "tab":
		if len(sm.matches) > 0 && sm.cursor < len(sm.matches) {
			chosen := sm.matches[sm.cursor]
			sm.Close()
			return &chosen, true, true
		}
		sm.Close()
		return nil, true, true

	case "up":
		if sm.cursor > 0 {
			sm.cursor--
		}
		return nil, true, false

	case "down":
		if sm.cursor < len(sm.matches)-1 {
			sm.cursor++
		}
		return nil, true, false

	case "backspace":
		if len(sm.query) > 0 {
			sm.query = TrimLastRune(sm.query)
			sm.filterMatches()
			return nil, true, false
		}
		// Backspace with empty query = close and let the caller delete the "/"
		sm.Close()
		return nil, false, true

	case " ":
		// Space closes the menu without selecting (only meaningful in modeAll;
		// modeAI doesn't have a leading "/" so space is just typed text — but
		// we still close to avoid stealing the keypress mid-edit)
		sm.Close()
		return nil, false, true

	default:
		// Single character — add to query
		if len(key) == 1 && key[0] >= 32 {
			sm.query += key
			sm.filterMatches()
			if len(sm.matches) == 0 {
				sm.Close()
				return nil, false, true
			}
			return nil, true, false
		}
	}

	return nil, false, false
}

// filterMatches updates the match list based on the current query.
func (sm *SlashMenu) filterMatches() {
	if sm.query == "" {
		sm.matches = sm.items
		sm.cursor = 0
		return
	}

	q := strings.ToLower(sm.query)
	sm.matches = nil
	for _, item := range sm.items {
		if strings.Contains(strings.ToLower(item.Command), q) ||
			strings.Contains(strings.ToLower(item.Label), q) ||
			strings.Contains(strings.ToLower(item.Description), q) {
			sm.matches = append(sm.matches, item)
		}
	}
	if sm.cursor >= len(sm.matches) {
		sm.cursor = 0
	}
}

// View renders the slash command menu as a popup.
func (sm *SlashMenu) View() string {
	if !sm.active || len(sm.matches) == 0 {
		return ""
	}

	const menuInner = 44
	maxVisible := 8
	if len(sm.matches) < maxVisible {
		maxVisible = len(sm.matches)
	}

	// Scroll offset
	scrollOffset := 0
	if sm.cursor >= maxVisible {
		scrollOffset = sm.cursor - maxVisible + 1
	}

	var rows []string

	// Header
	headerStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	queryStyle := lipgloss.NewStyle().Foreground(text).Bold(true)
	cursorStyle := lipgloss.NewStyle().Foreground(mauve)

	var headerText string
	if sm.mode == SlashMenuModeAI {
		headerText = headerStyle.Render("AI")
		if sm.query != "" {
			headerText += " " + queryStyle.Render(sm.query)
		}
	} else {
		headerText = headerStyle.Render("/")
		if sm.query != "" {
			headerText += queryStyle.Render(sm.query)
		}
	}
	headerText += cursorStyle.Render("|")
	headerLine := lipgloss.NewStyle().Width(menuInner).Render(" " + headerText)
	rows = append(rows, headerLine)

	// Separator
	sepLine := lipgloss.NewStyle().Foreground(surface1).Width(menuInner).Render(strings.Repeat("─", menuInner))
	rows = append(rows, sepLine)

	end := scrollOffset + maxVisible
	if end > len(sm.matches) {
		end = len(sm.matches)
	}

	iconWidth := 4
	labelWidth := 18

	for i := scrollOffset; i < end; i++ {
		item := sm.matches[i]
		isSelected := i == sm.cursor

		// Pad icon and label using visual width
		icon := smVisualPad(item.Icon, iconWidth)
		label := smVisualPad(item.Label, labelWidth)

		if isSelected {
			st := lipgloss.NewStyle().
				Background(surface0).
				Width(menuInner)

			iconSt := lipgloss.NewStyle().
				Background(surface0).
				Foreground(mauve).
				Bold(true)

			labelSt := lipgloss.NewStyle().
				Background(surface0).
				Foreground(text).
				Bold(true)

			descSt := lipgloss.NewStyle().
				Background(surface0).
				Foreground(overlay0)

			content := " " + iconSt.Render(icon) + " " + labelSt.Render(label) + " " + descSt.Render(item.Description)
			rows = append(rows, st.Render(content))
		} else {
			st := lipgloss.NewStyle().Width(menuInner)

			iconSt := lipgloss.NewStyle().Foreground(surface2)
			labelSt := lipgloss.NewStyle().Foreground(text)
			descSt := lipgloss.NewStyle().Foreground(surface2)

			content := " " + iconSt.Render(icon) + " " + labelSt.Render(label) + " " + descSt.Render(item.Description)
			rows = append(rows, st.Render(content))
		}
	}

	// Scroll indicator
	if len(sm.matches) > maxVisible {
		remaining := len(sm.matches) - end
		if remaining > 0 {
			moreStyle := lipgloss.NewStyle().Foreground(surface2).Italic(true).Width(menuInner)
			rows = append(rows, moreStyle.Render(" +"+smItoa(remaining)+" more..."))
		}
	}

	body := strings.Join(rows, "\n")

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(surface1).
		Padding(0, 1).
		Width(menuInner + 4) // +4 for padding

	return border.Render(body)
}

// smVisualPad pads s to the given visual width using spaces.
func smVisualPad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

func smItoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
