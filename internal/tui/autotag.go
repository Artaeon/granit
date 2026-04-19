package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// AutoTagger — AI-powered tag suggestions on save
// ---------------------------------------------------------------------------

// AutoTagger classifies notes and suggests tags using a small AI model.
// It is NOT an overlay; it is a helper invoked by app.go when a note is saved.
type AutoTagger struct {
	enabled bool

	// AI config
	ai AIConfig

	// Existing tags in the vault for consistency
	vaultTags []string

	// inFlight is true while an AI request is running — prevents pile-up
	// on rapid saves with slow local models.
	inFlight bool
}

// SetInFlight marks whether a request is currently running.
func (at *AutoTagger) SetInFlight(v bool) { at.inFlight = v }

// IsInFlight reports whether a request is currently running.
func (at *AutoTagger) IsInFlight() bool { return at.inFlight }

// autoTagResultMsg carries suggested tags back to the Update loop.
type autoTagResultMsg struct {
	tags []string
	err  error
}

// NewAutoTagger creates an AutoTagger with sensible defaults.
func NewAutoTagger() *AutoTagger {
	return &AutoTagger{
		enabled: false,
		ai: AIConfig{
			Provider:  "ollama",
			Model:     "qwen2.5:0.5b",
			OllamaURL: "http://localhost:11434",
		},
	}
}

// SetEnabled enables or disables automatic tagging.
func (at *AutoTagger) SetEnabled(enabled bool) {
	at.enabled = enabled
}

// IsEnabled reports whether the auto-tagger is active.
func (at *AutoTagger) IsEnabled() bool {
	return at.enabled
}

// SetVaultTags provides the set of tags already present in the vault so the
// AI can prefer them for consistency.
func (at *AutoTagger) SetVaultTags(tags []string) {
	at.vaultTags = tags
}

// TagNote sends the note content to the configured AI and returns a tea.Cmd
// that will produce an autoTagResultMsg.
func (at *AutoTagger) TagNote(content string) tea.Cmd {
	if !at.enabled {
		return nil
	}
	if at.inFlight {
		return nil
	}

	// Truncate for speed — use less for small models.
	maxRunes := 1500
	if at.ai.IsSmallModel() {
		maxRunes = 600
	}
	userPrompt := content
	if len([]rune(userPrompt)) > maxRunes {
		runes := []rune(userPrompt)
		userPrompt = string(runes[:maxRunes])
	}

	// Build system prompt.
	systemPrompt := "You are a note classifier. Given a note's content, suggest 2-5 relevant tags. Return ONLY a comma-separated list of lowercase tags, nothing else."
	if len(at.vaultTags) > 0 {
		// Cap tag list size to avoid bloating the system prompt on large vaults.
		tagLimit := 80
		if at.ai.IsSmallModel() {
			tagLimit = 30
		}
		tags := at.vaultTags
		if len(tags) > tagLimit {
			tags = tags[:tagLimit]
		}
		systemPrompt += " Prefer tags from this existing list when applicable: " + strings.Join(tags, ", ")
	}

	// Bail out if the prompt still wouldn't fit — avoids silently sending a
	// truncated prompt the model can't reason about.
	if fits, _, _ := at.ai.PromptFitsContext(systemPrompt, userPrompt); !fits {
		return nil
	}

	at.inFlight = true
	ai := at.ai

	return func() tea.Msg {
		// Auto-tagging runs on save. Cap the deadline so a hung request
		// doesn't block subsequent auto-tag attempts forever.
		deadline := 45 * time.Second
		if ai.IsSmallModel() {
			deadline = 90 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()

		response, err := ai.ChatShortCtx(ctx, systemPrompt, userPrompt)
		if err != nil {
			return autoTagResultMsg{err: err}
		}

		tags := atParseSuggestedTags(response)
		return autoTagResultMsg{tags: tags}
	}
}

// ParseSuggestedTags extracts tags from a raw AI response string.
func (at *AutoTagger) ParseSuggestedTags(response string) []string {
	return atParseSuggestedTags(response)
}

// ---------------------------------------------------------------------------
// AutoTagger — tag parsing
// ---------------------------------------------------------------------------

// atParseSuggestedTags splits a comma-separated AI response into clean tags.
// Each tag is lowercased and stripped of special characters. Unicode letters,
// digits, and hyphens are preserved to support non-English tags.
func atParseSuggestedTags(response string) []string {
	parts := strings.Split(response, ",")
	var tags []string
	for _, p := range parts {
		t := strings.TrimSpace(p)
		t = strings.ToLower(t)

		// Remove characters that aren't letters, digits, or hyphens.
		var cleaned strings.Builder
		for _, ch := range t {
			if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '-' {
				cleaned.WriteRune(ch)
			}
		}
		t = cleaned.String()
		t = strings.Trim(t, "-")

		if t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// ===========================================================================
// NoteChat — "chat with a specific note" overlay
// ===========================================================================

// noteChatMessage represents a single message in the note-focused chat.
type noteChatMessage struct {
	Role    string
	Content string
	Time    time.Time
}

// noteChatResultMsg carries the AI response back to the Update loop.
type noteChatResultMsg struct {
	content string
	err     error
}

// noteChatTickMsg drives the loading animation.
type noteChatTickMsg struct{}

// NoteChat implements a chat overlay focused on a single note.
type NoteChat struct {
	OverlayBase

	notePath    string
	noteContent string

	messages    []noteChatMessage
	input       string
	scroll      int
	loading     bool
	loadingTick int

	ai AIConfig
}

// NewNoteChat creates an empty NoteChat with sensible defaults.
func NewNoteChat() NoteChat {
	return NoteChat{
		ai: AIConfig{
			Provider:  "ollama",
			Model:     "qwen2.5:0.5b",
			OllamaURL: "http://localhost:11434",
		},
	}
}

// ---------------------------------------------------------------------------
// NoteChat — overlay interface
// ---------------------------------------------------------------------------

// Open activates the chat overlay, focused on a specific note.
func (nc *NoteChat) Open(notePath, noteContent string) {
	nc.Activate()
	nc.notePath = notePath
	nc.noteContent = noteContent
	nc.input = ""
	nc.scroll = 0
	nc.loading = false
	nc.loadingTick = 0
	nc.messages = []noteChatMessage{
		{
			Role:    "system",
			Content: fmt.Sprintf("Chatting about: %s", filepath.Base(notePath)),
			Time:    time.Now(),
		},
	}
}

// ---------------------------------------------------------------------------
// NoteChat — AI calling helpers
// ---------------------------------------------------------------------------

// ncBuildSystemPrompt returns the system prompt for note-focused chat.
// Content is truncated to avoid exceeding model context limits.
func ncBuildSystemPrompt(notePath, noteContent string, ai AIConfig) string {
	content := noteContent
	maxContent := 6000
	if ai.IsSmallModel() {
		maxContent = 1500
	}
	if len(content) > maxContent {
		content = content[:maxContent] + "\n... (truncated)"
	}
	return fmt.Sprintf(
		"You are an assistant helping the user understand and work with a specific note. "+
			"The note is titled '%s'. Here is the content:\n\n%s\n\n"+
			"Answer questions about this note. Be specific and reference parts of the note.",
		notePath, content,
	)
}

// ---------------------------------------------------------------------------
// NoteChat — loading animation tick
// ---------------------------------------------------------------------------

func noteChatTick() tea.Cmd {
	return tea.Tick(400*time.Millisecond, func(time.Time) tea.Msg {
		return noteChatTickMsg{}
	})
}

// ---------------------------------------------------------------------------
// NoteChat — Update (value receiver)
// ---------------------------------------------------------------------------

// Update handles key presses and incoming AI responses for the note chat.
func (nc NoteChat) Update(msg tea.Msg) (NoteChat, tea.Cmd) {
	if !nc.active {
		return nc, nil
	}

	switch msg := msg.(type) {
	case noteChatTickMsg:
		if nc.loading {
			nc.loadingTick++
			return nc, noteChatTick()
		}

	case noteChatResultMsg:
		nc.loading = false
		if msg.err != nil {
			nc.messages = append(nc.messages, noteChatMessage{
				Role:    "system",
				Content: fmt.Sprintf("Error: %v", msg.err),
				Time:    time.Now(),
			})
		} else {
			nc.messages = append(nc.messages, noteChatMessage{
				Role:    "assistant",
				Content: msg.content,
				Time:    time.Now(),
			})
		}
		// Auto-scroll to bottom.
		nc.scroll = nc.maxScroll()
		return nc, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			nc.active = false
			return nc, nil

		case "enter":
			if nc.loading {
				return nc, nil
			}
			trimmed := strings.TrimSpace(nc.input)
			if trimmed == "" {
				return nc, nil
			}

			// Add user message.
			nc.messages = append(nc.messages, noteChatMessage{
				Role:    "user",
				Content: trimmed,
				Time:    time.Now(),
			})
			// Cap message history to prevent unbounded growth
			if len(nc.messages) > 200 {
				nc.messages = nc.messages[len(nc.messages)-200:]
			}
			nc.input = ""
			nc.loading = true
			nc.loadingTick = 0

			systemPrompt := ncBuildSystemPrompt(nc.notePath, nc.noteContent, nc.ai)

			// Build combined user prompt from conversation history.
			// Include most recent messages first to preserve context continuity.
			maxHistory := 4000
			if nc.ai.IsSmallModel() {
				maxHistory = 1000
			}
			// Collect non-system messages, then take as many recent ones as fit.
			var entries []string
			for _, m := range nc.messages {
				if m.Role == "system" {
					continue
				}
				entries = append(entries, m.Role+": "+m.Content+"\n")
			}
			totalLen := 0
			startIdx := len(entries)
			for i := len(entries) - 1; i >= 0; i-- {
				if totalLen+len(entries[i]) > maxHistory {
					break
				}
				totalLen += len(entries[i])
				startIdx = i
			}
			var userBuf strings.Builder
			for i := startIdx; i < len(entries); i++ {
				userBuf.WriteString(entries[i])
			}
			ai := nc.ai
			cmd := func() tea.Msg {
				resp, err := ai.Chat(systemPrompt, userBuf.String())
				return noteChatResultMsg{content: resp, err: err}
			}

			return nc, tea.Batch(cmd, noteChatTick())

		case "up":
			if nc.scroll > 0 {
				nc.scroll--
			}
		case "down":
			ms := nc.maxScroll()
			if nc.scroll < ms {
				nc.scroll++
			}

		case "ctrl+u":
			nc.input = ""

		case "backspace":
			if len(nc.input) > 0 {
				nc.input = TrimLastRune(nc.input)
			}

		default:
			// Accept printable runes.
			if len(msg.String()) == 1 || msg.Type == tea.KeyRunes {
				nc.input += msg.String()
			}
		}
	}

	return nc, nil
}

// ---------------------------------------------------------------------------
// NoteChat — scroll helpers
// ---------------------------------------------------------------------------

// ncChatWidth returns the inner width of the chat panel.
func (nc *NoteChat) ncChatWidth() int {
	w := nc.width * 3 / 5
	if w < 50 {
		w = 50
	}
	if w > 100 {
		w = 100
	}
	return w
}

// ncChatHeight returns the number of lines available for messages.
func (nc *NoteChat) ncChatHeight() int {
	// Subtract: title(2) + note-name(1) + separator(1) + input(2) + help(2) + border/padding(6)
	h := nc.height - 14
	if h < 5 {
		h = 5
	}
	return h
}

// ncMessageLineCount estimates how many terminal lines a message occupies.
func (nc *NoteChat) ncMessageLineCount(m noteChatMessage, width int) int {
	if width <= 0 {
		width = 40
	}
	bubbleWidth := width * 2 / 3
	if bubbleWidth < 20 {
		bubbleWidth = 20
	}

	lines := strings.Split(m.Content, "\n")
	total := 0
	for _, line := range lines {
		if len(line) == 0 {
			total++
			continue
		}
		wrapped := (len(line) + bubbleWidth - 1) / bubbleWidth
		total += wrapped
	}
	// Add 1 for timestamp line.
	total++
	return total
}

// maxScroll returns the maximum scroll offset for the message list.
func (nc *NoteChat) maxScroll() int {
	totalLines := 0
	chatWidth := nc.ncChatWidth() - 4
	if chatWidth < 20 {
		chatWidth = 20
	}

	for _, m := range nc.messages {
		totalLines += nc.ncMessageLineCount(m, chatWidth)
		totalLines++ // spacing between messages
	}

	if nc.loading {
		totalLines += 2
	}

	viewable := nc.ncChatHeight()
	if totalLines <= viewable {
		return 0
	}
	return totalLines - viewable
}

// ---------------------------------------------------------------------------
// NoteChat — View (value receiver)
// ---------------------------------------------------------------------------

// View renders the note chat overlay.
func (nc NoteChat) View() string {
	panelWidth := nc.ncChatWidth()
	innerWidth := panelWidth - 6 // border(2) + padding(2*2)

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(mauve).Bold(true)
	noteName := filepath.Base(nc.notePath)
	b.WriteString(titleStyle.Render("  Chat: " + noteName))
	b.WriteString("\n")

	// Note name subtitle
	noteInfo := DimStyle.Render("  Note: " + nc.notePath)
	b.WriteString(noteInfo)
	b.WriteString("\n")

	// Separator
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(strings.Repeat("-", innerWidth)))
	b.WriteString("\n")

	// Render messages
	viewHeight := nc.ncChatHeight()
	var messageLines []string

	for _, m := range nc.messages {
		rendered := nc.renderNCMessage(m, innerWidth)
		messageLines = append(messageLines, rendered)
	}

	// Loading indicator
	if nc.loading {
		dots := strings.Repeat(".", (nc.loadingTick%3)+1)
		loadingLine := lipgloss.NewStyle().Foreground(blue).Italic(true).Render("  Thinking" + dots)
		messageLines = append(messageLines, loadingLine)
	}

	// Flatten to individual terminal lines for scrolling.
	var allLines []string
	for _, ml := range messageLines {
		parts := strings.Split(ml, "\n")
		allLines = append(allLines, parts...)
	}

	// Apply scroll offset.
	start := nc.scroll
	if start > len(allLines) {
		start = len(allLines)
	}
	end := start + viewHeight
	if end > len(allLines) {
		end = len(allLines)
	}

	visible := allLines[start:end]
	b.WriteString(strings.Join(visible, "\n"))

	// Pad remaining height.
	remaining := viewHeight - len(visible)
	for i := 0; i < remaining; i++ {
		b.WriteString("\n")
	}

	// Separator
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Foreground(overlay0).Render(strings.Repeat("-", innerWidth)))
	b.WriteString("\n")

	// Input area
	promptStyle := SearchPromptStyle
	inputStyle := lipgloss.NewStyle().Foreground(text)

	inputLine := promptStyle.Render("> ") + inputStyle.Render(nc.input)
	if !nc.loading {
		inputLine += lipgloss.NewStyle().Foreground(text).Background(surface0).Render(" ")
	}
	b.WriteString(inputLine)
	b.WriteString("\n")

	// Help bar
	helpText := "Enter: send  Esc: close  Up/Down: scroll  Ctrl+U: clear"
	if nc.loading {
		helpText = "Waiting for response...  Esc: close"
	}
	b.WriteString(DimStyle.Render("  " + helpText))

	// Wrap in panel border
	panel := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return panel.Render(b.String())
}

// renderNCMessage renders a single note-chat message as a styled bubble.
func (nc *NoteChat) renderNCMessage(m noteChatMessage, width int) string {
	bubbleWidth := width * 2 / 3
	if bubbleWidth < 20 {
		bubbleWidth = 20
	}

	timeStr := m.Time.Format("15:04")

	switch m.Role {
	case "user":
		// Right-aligned, peach colored.
		msgStyle := lipgloss.NewStyle().
			Foreground(mantle).
			Background(peach).
			Padding(0, 1).
			Width(bubbleWidth).
			Align(lipgloss.Right)

		timeStyle := lipgloss.NewStyle().
			Foreground(overlay0).
			Width(width).
			Align(lipgloss.Right)

		rendered := msgStyle.Render(m.Content)
		padLeft := width - lipgloss.Width(rendered)
		if padLeft < 0 {
			padLeft = 0
		}
		line := strings.Repeat(" ", padLeft) + rendered
		line += "\n" + timeStyle.Render(timeStr)
		return line

	case "assistant":
		// Left-aligned, blue colored.
		msgStyle := lipgloss.NewStyle().
			Foreground(mantle).
			Background(blue).
			Padding(0, 1).
			Width(bubbleWidth)

		timeStyle := lipgloss.NewStyle().
			Foreground(overlay0)

		rendered := msgStyle.Render(m.Content)
		line := rendered
		line += "\n" + timeStyle.Render("  " + timeStr)
		return line

	case "system":
		// Centered, dim.
		msgStyle := lipgloss.NewStyle().
			Foreground(overlay0).
			Italic(true).
			Width(width).
			Align(lipgloss.Center)

		return msgStyle.Render(m.Content)

	default:
		return m.Content
	}
}
