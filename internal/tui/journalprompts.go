package tui

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type journalPrompt struct {
	Category string
	Text     string
}

// JournalPrompts provides an overlay with curated daily reflection prompts
// organized by category, and a write mode for composing journal entries.
type JournalPrompts struct {
	active    bool
	width     int
	height    int
	vaultRoot string

	prompts    []journalPrompt
	allPrompts []journalPrompt // full list
	cursor     int
	scroll     int
	category   int // 0=all, 1-8=specific category

	// Write mode
	writing       bool
	response      string
	currentPrompt journalPrompt

	saved     bool
	statusMsg string

	// GetResult consumed-once
	resultPath string
	hasResult  bool
}

var journalCategories = []string{
	"All",
	"Gratitude",
	"Reflection",
	"Growth",
	"Creativity",
	"Relationships",
	"Goals",
	"Mindfulness",
	"Challenge",
}

var curatedPrompts = []journalPrompt{
	// ── Gratitude (1) ──────────────────────────────────────
	{Category: "Gratitude", Text: "What are 3 things you're grateful for today?"},
	{Category: "Gratitude", Text: "Who is someone you appreciate but haven't thanked recently?"},
	{Category: "Gratitude", Text: "What small comfort did you enjoy today that you usually overlook?"},
	{Category: "Gratitude", Text: "What ability or skill do you have that you're thankful for?"},
	{Category: "Gratitude", Text: "What is something in nature that brought you joy recently?"},
	{Category: "Gratitude", Text: "What modern convenience are you most grateful for?"},
	{Category: "Gratitude", Text: "What memory from the past week makes you smile?"},
	{Category: "Gratitude", Text: "What challenge in your life are you secretly grateful for?"},
	{Category: "Gratitude", Text: "What is something free that you truly value?"},
	{Category: "Gratitude", Text: "Who has helped shape who you are today?"},
	{Category: "Gratitude", Text: "What did someone do for you recently that you appreciated?"},
	{Category: "Gratitude", Text: "What part of your daily routine brings you the most peace?"},
	{Category: "Gratitude", Text: "What place feels like home to you, and why are you grateful for it?"},

	// ── Reflection (2) ─────────────────────────────────────
	{Category: "Reflection", Text: "What was the most meaningful moment of your day?"},
	{Category: "Reflection", Text: "If you could relive one moment from today, which would it be?"},
	{Category: "Reflection", Text: "What surprised you today?"},
	{Category: "Reflection", Text: "What is something you believed a year ago that you no longer believe?"},
	{Category: "Reflection", Text: "What would your younger self think of who you are now?"},
	{Category: "Reflection", Text: "What was the highlight of your day and why?"},
	{Category: "Reflection", Text: "When did you feel most like yourself today?"},
	{Category: "Reflection", Text: "What conversation stayed with you after it ended?"},
	{Category: "Reflection", Text: "What pattern in your life do you keep noticing?"},
	{Category: "Reflection", Text: "If today were your last day, would you be satisfied with how you spent it?"},
	{Category: "Reflection", Text: "What is something you need to let go of?"},
	{Category: "Reflection", Text: "How have you changed in the past six months?"},
	{Category: "Reflection", Text: "What did you learn about yourself today?"},

	// ── Growth (3) ─────────────────────────────────────────
	{Category: "Growth", Text: "What skill did you practice or improve today?"},
	{Category: "Growth", Text: "What mistake taught you the most this week?"},
	{Category: "Growth", Text: "What is one habit you're actively building?"},
	{Category: "Growth", Text: "What book, article, or idea expanded your thinking recently?"},
	{Category: "Growth", Text: "What feedback have you received recently that was hard to hear but valuable?"},
	{Category: "Growth", Text: "What is something you were afraid to try but did anyway?"},
	{Category: "Growth", Text: "What would you attempt if you knew you couldn't fail?"},
	{Category: "Growth", Text: "How did you step outside your comfort zone today?"},
	{Category: "Growth", Text: "What is a weakness you're turning into a strength?"},
	{Category: "Growth", Text: "What knowledge gap do you want to fill next?"},
	{Category: "Growth", Text: "What is one thing you did better today than yesterday?"},
	{Category: "Growth", Text: "Who inspires you to grow, and what quality of theirs do you admire?"},
	{Category: "Growth", Text: "What is a lesson you keep having to relearn?"},

	// ── Creativity (4) ─────────────────────────────────────
	{Category: "Creativity", Text: "If you could create anything tomorrow, what would it be?"},
	{Category: "Creativity", Text: "What idea has been nagging at you that you haven't explored yet?"},
	{Category: "Creativity", Text: "If you had an entire day with no obligations, what would you make?"},
	{Category: "Creativity", Text: "What two unrelated interests could you combine into something new?"},
	{Category: "Creativity", Text: "What would you build if you had unlimited resources?"},
	{Category: "Creativity", Text: "Describe a world you'd like to live in. What's different?"},
	{Category: "Creativity", Text: "What problem around you could be solved with a creative approach?"},
	{Category: "Creativity", Text: "What is the most creative thing you've done recently?"},
	{Category: "Creativity", Text: "If you could master any art form overnight, which would you choose?"},
	{Category: "Creativity", Text: "What does your ideal creative workspace look like?"},
	{Category: "Creativity", Text: "Write a six-word story about your day."},
	{Category: "Creativity", Text: "What is an unusual connection you noticed between two ideas recently?"},

	// ── Relationships (5) ──────────────────────────────────
	{Category: "Relationships", Text: "Who made a positive impact on your life recently?"},
	{Category: "Relationships", Text: "What quality do you value most in your closest friend?"},
	{Category: "Relationships", Text: "How did you show up for someone today?"},
	{Category: "Relationships", Text: "What is something you wish you had said to someone today?"},
	{Category: "Relationships", Text: "Who do you need to reconnect with?"},
	{Category: "Relationships", Text: "What boundary do you need to set or reinforce in a relationship?"},
	{Category: "Relationships", Text: "What act of kindness did you witness or perform today?"},
	{Category: "Relationships", Text: "How has a relationship shaped your values?"},
	{Category: "Relationships", Text: "What's the most important thing you've learned about communication?"},
	{Category: "Relationships", Text: "Who challenges you to be better, and how?"},
	{Category: "Relationships", Text: "What do you bring to your relationships that you're proud of?"},
	{Category: "Relationships", Text: "What is something you admire about someone you disagree with?"},
	{Category: "Relationships", Text: "How can you be a better listener tomorrow?"},

	// ── Goals (6) ──────────────────────────────────────────
	{Category: "Goals", Text: "What's one step you can take tomorrow toward your biggest goal?"},
	{Category: "Goals", Text: "Where do you see yourself in one year if you stay on your current path?"},
	{Category: "Goals", Text: "What goal have you been putting off, and what's holding you back?"},
	{Category: "Goals", Text: "What does success look like for you right now?"},
	{Category: "Goals", Text: "What is a small win you achieved today?"},
	{Category: "Goals", Text: "If you could accomplish only one thing this month, what would it be?"},
	{Category: "Goals", Text: "What is a goal you've outgrown and need to release?"},
	{Category: "Goals", Text: "What resources do you need to reach your next milestone?"},
	{Category: "Goals", Text: "How will you hold yourself accountable this week?"},
	{Category: "Goals", Text: "What would your ideal morning routine look like?"},
	{Category: "Goals", Text: "What is one distraction you can eliminate to make progress?"},
	{Category: "Goals", Text: "What would your future self thank you for starting today?"},

	// ── Mindfulness (7) ────────────────────────────────────
	{Category: "Mindfulness", Text: "What emotions did you experience most strongly today?"},
	{Category: "Mindfulness", Text: "What does your body need right now that you've been ignoring?"},
	{Category: "Mindfulness", Text: "Describe the present moment using all five senses."},
	{Category: "Mindfulness", Text: "When did you feel most calm today, and what were you doing?"},
	{Category: "Mindfulness", Text: "What thought kept recurring today?"},
	{Category: "Mindfulness", Text: "How are you feeling right now, honestly, without judgment?"},
	{Category: "Mindfulness", Text: "What is one thing you can do tonight to sleep better?"},
	{Category: "Mindfulness", Text: "What is weighing on your mind that you can write down and release?"},
	{Category: "Mindfulness", Text: "When did you last feel truly present, without thinking of past or future?"},
	{Category: "Mindfulness", Text: "What is a simple pleasure you experienced today?"},
	{Category: "Mindfulness", Text: "What sound, smell, or sight brought you unexpected comfort?"},
	{Category: "Mindfulness", Text: "How did you take care of yourself today?"},
	{Category: "Mindfulness", Text: "What worry can you choose to set aside tonight?"},

	// ── Challenge (8) ──────────────────────────────────────
	{Category: "Challenge", Text: "What's a problem you're working through? What have you tried?"},
	{Category: "Challenge", Text: "What obstacle did you face today and how did you respond?"},
	{Category: "Challenge", Text: "What is the hardest decision you're currently facing?"},
	{Category: "Challenge", Text: "What fear is limiting you, and what would you do without it?"},
	{Category: "Challenge", Text: "When did you last fail at something, and what did you take from it?"},
	{Category: "Challenge", Text: "What's a conflict you've been avoiding?"},
	{Category: "Challenge", Text: "What assumption are you making that might be wrong?"},
	{Category: "Challenge", Text: "What would you do differently if you had to start over?"},
	{Category: "Challenge", Text: "What is a hard truth you've been reluctant to accept?"},
	{Category: "Challenge", Text: "How do you typically react under pressure, and is it serving you?"},
	{Category: "Challenge", Text: "What is something you need help with but haven't asked for?"},
	{Category: "Challenge", Text: "What would you tell a friend facing the same challenge you are?"},
}

// NewJournalPrompts creates a new JournalPrompts in its default (inactive) state.
func NewJournalPrompts() JournalPrompts {
	all := make([]journalPrompt, len(curatedPrompts))
	copy(all, curatedPrompts)
	return JournalPrompts{
		allPrompts: all,
		prompts:    all,
	}
}

// IsActive reports whether the journal prompts overlay is currently visible.
func (jp JournalPrompts) IsActive() bool {
	return jp.active
}

// Open activates the overlay and resets state.
func (jp *JournalPrompts) Open(vaultRoot string) {
	jp.active = true
	jp.vaultRoot = vaultRoot
	jp.cursor = 0
	jp.scroll = 0
	jp.category = 0
	jp.writing = false
	jp.response = ""
	jp.saved = false
	jp.statusMsg = ""
	jp.hasResult = false
	jp.resultPath = ""
	jp.filterPrompts()
}

// Close deactivates the overlay.
func (jp *JournalPrompts) Close() {
	jp.active = false
}

// SetSize updates the available terminal dimensions.
func (jp *JournalPrompts) SetSize(w, h int) {
	jp.width = w
	jp.height = h
}

// GetResult returns the path of the last saved journal file (consumed-once).
func (jp *JournalPrompts) GetResult() (filePath string, ok bool) {
	if jp.hasResult {
		jp.hasResult = false
		path := jp.resultPath
		jp.resultPath = ""
		return path, true
	}
	return "", false
}

// filterPrompts rebuilds the prompts slice based on the current category filter.
func (jp *JournalPrompts) filterPrompts() {
	if jp.category == 0 {
		jp.prompts = jp.allPrompts
	} else {
		catName := journalCategories[jp.category]
		var filtered []journalPrompt
		for _, p := range jp.allPrompts {
			if p.Category == catName {
				filtered = append(filtered, p)
			}
		}
		jp.prompts = filtered
	}
	jp.cursor = 0
	jp.scroll = 0
}

// dailyPrompt returns a deterministic prompt for today's date.
func (jp *JournalPrompts) dailyPrompt() journalPrompt {
	today := time.Now().Format("2006-01-02")
	h := sha256.Sum256([]byte(today))
	idx := binary.BigEndian.Uint64(h[:8]) % uint64(len(jp.allPrompts))
	return jp.allPrompts[idx]
}

// shufflePrompt picks a random prompt from the current prompt list.
func (jp *JournalPrompts) shufflePrompt() {
	if len(jp.prompts) == 0 {
		return
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	jp.cursor = r.Intn(len(jp.prompts))
	// Adjust scroll to keep cursor visible
	visH := jp.visibleHeight()
	if jp.cursor < jp.scroll {
		jp.scroll = jp.cursor
	} else if jp.cursor >= jp.scroll+visH {
		jp.scroll = jp.cursor - visH + 1
	}
}

// visibleHeight returns the number of prompt lines that fit in the list area.
func (jp *JournalPrompts) visibleHeight() int {
	h := jp.height - 16 // account for title, daily prompt, footer, borders
	if h < 3 {
		h = 3
	}
	return h
}

// saveEntry saves the current journal response to Journal/YYYY-MM-DD.md.
func (jp *JournalPrompts) saveEntry() tea.Cmd {
	if strings.TrimSpace(jp.response) == "" {
		jp.statusMsg = "Nothing to save"
		return nil
	}

	today := time.Now().Format("2006-01-02")
	journalDir := filepath.Join(jp.vaultRoot, "Journal")
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		jp.statusMsg = "Error: " + err.Error()
		return nil
	}

	fileName := today + ".md"
	filePath := filepath.Join(journalDir, fileName)

	heading := fmt.Sprintf("## %s: %s", jp.currentPrompt.Category, jp.currentPrompt.Text)
	entryBody := heading + "\n\n" + jp.response + "\n"

	// Check if file already exists
	existing, err := os.ReadFile(filePath)
	if err == nil && len(existing) > 0 {
		// Append new entry with separator
		content := string(existing)
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n---\n\n" + entryBody
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			jp.statusMsg = "Error: " + err.Error()
			return nil
		}
	} else {
		// Create new file with frontmatter
		var b strings.Builder
		b.WriteString("---\n")
		b.WriteString("date: " + today + "\n")
		b.WriteString("type: journal\n")
		b.WriteString("---\n\n")
		b.WriteString(entryBody)
		if err := os.WriteFile(filePath, []byte(b.String()), 0644); err != nil {
			jp.statusMsg = "Error: " + err.Error()
			return nil
		}
	}

	jp.saved = true
	jp.statusMsg = "Saved to Journal/" + fileName
	jp.resultPath = filepath.Join("Journal", fileName)
	jp.hasResult = true
	jp.writing = false

	return nil
}

// Update handles keyboard input for the journal prompts overlay.
func (jp JournalPrompts) Update(msg tea.Msg) (JournalPrompts, tea.Cmd) {
	if !jp.active {
		return jp, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if jp.writing {
			return jp.updateWriteMode(msg)
		}
		return jp.updateBrowseMode(msg)
	}
	return jp, nil
}

// updateBrowseMode handles keys when browsing prompts.
func (jp JournalPrompts) updateBrowseMode(msg tea.KeyMsg) (JournalPrompts, tea.Cmd) {
	switch msg.String() {
	case "esc":
		jp.active = false
	case "j", "down":
		if jp.cursor < len(jp.prompts)-1 {
			jp.cursor++
			visH := jp.visibleHeight()
			if jp.cursor >= jp.scroll+visH {
				jp.scroll = jp.cursor - visH + 1
			}
		}
	case "k", "up":
		if jp.cursor > 0 {
			jp.cursor--
			if jp.cursor < jp.scroll {
				jp.scroll = jp.cursor
			}
		}
	case "r":
		jp.shufflePrompt()
	case "1":
		jp.category = 1
		jp.filterPrompts()
	case "2":
		jp.category = 2
		jp.filterPrompts()
	case "3":
		jp.category = 3
		jp.filterPrompts()
	case "4":
		jp.category = 4
		jp.filterPrompts()
	case "5":
		jp.category = 5
		jp.filterPrompts()
	case "6":
		jp.category = 6
		jp.filterPrompts()
	case "7":
		jp.category = 7
		jp.filterPrompts()
	case "8":
		jp.category = 8
		jp.filterPrompts()
	case "0":
		jp.category = 0
		jp.filterPrompts()
	case "enter":
		if len(jp.prompts) > 0 && jp.cursor < len(jp.prompts) {
			jp.writing = true
			jp.currentPrompt = jp.prompts[jp.cursor]
			jp.response = ""
			jp.saved = false
			jp.statusMsg = ""
		}
	}
	return jp, nil
}

// updateWriteMode handles keys when composing a journal response.
func (jp JournalPrompts) updateWriteMode(msg tea.KeyMsg) (JournalPrompts, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		jp.writing = false
		jp.statusMsg = ""
	case "ctrl+s":
		jp.saveEntry()
	case "backspace":
		if len(jp.response) > 0 {
			jp.response = jp.response[:len(jp.response)-1]
		}
	case "enter":
		jp.response += "\n"
	default:
		if len(key) == 1 || msg.Type == tea.KeySpace {
			jp.response += key
		} else if msg.Type == tea.KeyRunes {
			jp.response += string(msg.Runes)
		}
	}
	return jp, nil
}

// View renders the journal prompts overlay.
func (jp JournalPrompts) View() string {
	width := jp.width / 2
	if width < 54 {
		width = 54
	}
	if width > 76 {
		width = 76
	}

	var b strings.Builder

	if jp.writing {
		jp.renderWriteMode(&b, width)
	} else {
		jp.renderBrowseMode(&b, width)
	}

	border := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(width).
		Background(mantle)

	return border.Render(b.String())
}

// renderBrowseMode draws the prompt browser view.
func (jp JournalPrompts) renderBrowseMode(b *strings.Builder, width int) {
	// Title
	title := lipgloss.NewStyle().
		Foreground(mauve).
		Bold(true).
		Render("  " + IconDailyChar + " Journal Prompts")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	// Daily prompt
	dp := jp.dailyPrompt()
	today := time.Now().Format("January 2, 2006")
	dateLabel := lipgloss.NewStyle().Foreground(lavender).Bold(true).
		Render("  Daily Prompt for " + today)
	b.WriteString(dateLabel)
	b.WriteString("\n")
	catStyle := lipgloss.NewStyle().Foreground(teal)
	b.WriteString("  " + catStyle.Render("Category: "+dp.Category))
	b.WriteString("\n\n")

	quoteStyle := lipgloss.NewStyle().Foreground(peach).Italic(true)
	promptText := wrapPromptText(dp.Text, width-10)
	for _, line := range strings.Split(promptText, "\n") {
		b.WriteString("  " + quoteStyle.Render("\""+line+"\""))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-10)))
	b.WriteString("\n")

	// Category filter indicator
	filterLabel := "  "
	if jp.category == 0 {
		filterLabel += lipgloss.NewStyle().Foreground(green).Render("All categories")
	} else {
		filterLabel += lipgloss.NewStyle().Foreground(yellow).Render(journalCategories[jp.category])
	}
	filterLabel += DimStyle.Render(fmt.Sprintf(" (%d prompts)", len(jp.prompts)))
	b.WriteString(filterLabel)
	b.WriteString("\n\n")

	// Prompt list
	if len(jp.prompts) == 0 {
		b.WriteString(DimStyle.Render("  No prompts in this category"))
	} else {
		visH := jp.visibleHeight()
		end := jp.scroll + visH
		if end > len(jp.prompts) {
			end = len(jp.prompts)
		}

		catColors := map[string]lipgloss.Color{
			"Gratitude":     green,
			"Reflection":    blue,
			"Growth":        peach,
			"Creativity":    mauve,
			"Relationships": pink,
			"Goals":         yellow,
			"Mindfulness":   teal,
			"Challenge":     red,
		}

		for i := jp.scroll; i < end; i++ {
			p := jp.prompts[i]
			col, ok := catColors[p.Category]
			if !ok {
				col = text
			}
			dot := lipgloss.NewStyle().Foreground(col).Render("\u2022")

			truncated := truncateText(p.Text, width-12)

			if i == jp.cursor {
				line := "  > " + dot + " " + truncated
				b.WriteString(lipgloss.NewStyle().
					Background(surface0).
					Foreground(peach).
					Bold(true).
					Width(width - 6).
					Render(line))
			} else {
				b.WriteString("    " + dot + " " + NormalItemStyle.Render(truncated))
			}
			if i < end-1 {
				b.WriteString("\n")
			}
		}
	}

	// Status message
	if jp.statusMsg != "" {
		b.WriteString("\n\n")
		statusStyle := lipgloss.NewStyle().Foreground(green).Italic(true)
		b.WriteString("  " + statusStyle.Render(jp.statusMsg))
	}

	// Footer
	b.WriteString("\n\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-10)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"1-8", "category"}, {"0", "all"}, {"r", "shuffle"}, {"Enter", "write"},
	}))
	b.WriteString("\n")
	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"j/k", "navigate"}, {"Esc", "close"},
	}))
}

// renderWriteMode draws the journal response composition view.
func (jp JournalPrompts) renderWriteMode(b *strings.Builder, width int) {
	// Title
	title := lipgloss.NewStyle().
		Foreground(green).
		Bold(true).
		Render("  " + IconEditChar + " Write Journal Entry")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat("\u2500", width-6)))
	b.WriteString("\n\n")

	// Current prompt
	catStyle := lipgloss.NewStyle().Foreground(teal)
	b.WriteString("  " + catStyle.Render(jp.currentPrompt.Category))
	b.WriteString("\n")

	quoteStyle := lipgloss.NewStyle().Foreground(peach).Italic(true)
	promptText := wrapPromptText(jp.currentPrompt.Text, width-10)
	for _, line := range strings.Split(promptText, "\n") {
		b.WriteString("  " + quoteStyle.Render("\""+line+"\""))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-10)))
	b.WriteString("\n")

	responseLabel := lipgloss.NewStyle().Foreground(lavender).Bold(true).
		Render("  Your response:")
	b.WriteString(responseLabel)
	b.WriteString("\n\n")

	// Response text area
	responseLines := strings.Split(jp.response, "\n")
	maxLines := jp.height - 20
	if maxLines < 4 {
		maxLines = 4
	}

	startLine := 0
	if len(responseLines) > maxLines {
		startLine = len(responseLines) - maxLines
	}

	responseStyle := lipgloss.NewStyle().Foreground(text)
	for i := startLine; i < len(responseLines); i++ {
		b.WriteString("  " + responseStyle.Render(responseLines[i]))
		if i < len(responseLines)-1 {
			b.WriteString("\n")
		}
	}

	// Cursor indicator
	cursorStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	b.WriteString(cursorStyle.Render("\u2588"))
	b.WriteString("\n")

	// Status message
	if jp.statusMsg != "" {
		b.WriteString("\n")
		var statusStyle lipgloss.Style
		if jp.saved {
			statusStyle = lipgloss.NewStyle().Foreground(green).Italic(true)
		} else {
			statusStyle = lipgloss.NewStyle().Foreground(red).Italic(true)
		}
		b.WriteString("  " + statusStyle.Render(jp.statusMsg))
		b.WriteString("\n")
	}

	// Footer
	b.WriteString("\n")
	b.WriteString(DimStyle.Render("  " + strings.Repeat("\u2500", width-10)))
	b.WriteString("\n")

	b.WriteString(RenderHelpBar([]struct{ Key, Desc string }{
		{"Ctrl+S", "save"}, {"Esc", "cancel"},
	}))
}

// wrapPromptText wraps text to fit within the given width.
func wrapPromptText(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return ""
	}

	var lines []string
	current := words[0]
	for _, w := range words[1:] {
		if len(current)+1+len(w) > maxWidth {
			lines = append(lines, current)
			current = w
		} else {
			current += " " + w
		}
	}
	lines = append(lines, current)
	return strings.Join(lines, "\n")
}

// truncateText truncates a string to maxLen, adding ellipsis if needed.
func truncateText(s string, maxLen int) string {
	if maxLen <= 3 {
		return s
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
