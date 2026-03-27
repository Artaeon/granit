package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

type briefingResultMsg struct {
	content string
	err     error
}

type briefingTickMsg struct{}

func briefingTickCmd() tea.Cmd {
	return tea.Tick(300*time.Millisecond, func(time.Time) tea.Msg {
		return briefingTickMsg{}
	})
}

// DeepCoven system prompt — the AI persona for daily briefings.
const deepCovenPrompt = `You are DEEPCOVEN — a direct, honest, and action-oriented personal assistant embedded in the user's knowledge base.

CORE PRINCIPLES:
- 100% honesty: every insight must be true and transparent
- 100% service: every word must move the user forward
- Be direct, clear, no filler — focus on what matters NOW
- Encouraging but honest — never sugarcoat, never hold back

YOUR TASK: Generate a morning briefing based on the user's recent notes, open tasks, and vault activity.

FORMAT YOUR RESPONSE EXACTLY LIKE THIS:
## Morning Briefing — {today's date}

### Active Threads
- List the 2-3 topics the user has been working on recently
- Reference specific notes with [[wikilinks]]

### Open Tasks
- [ ] Extract any unchecked tasks from recent notes
- [ ] Highlight overdue or important items

### Today's Focus
Based on what you see in the notes, suggest 1-3 things the user should focus on today. Be specific and actionable.

### Connections You Might Have Missed
Point out 1-2 links between notes that the user may not have noticed.

Keep it concise. No preamble. Start directly with the heading.`

// ---------------------------------------------------------------------------
// DailyBriefing overlay
// ---------------------------------------------------------------------------

const (
	briefingStateQuestion = 0 // asking user what they want to do today
	briefingStateLoading  = 1
	briefingStateResult   = 2
)

type DailyBriefing struct {
	active bool
	width  int
	height int

	state       int
	userGoal    string   // what the user wants to do today
	briefing    string   // generated briefing text
	briefLines  []string // for display
	scroll      int
	loadingTick int

	// vault context
	notes       map[string]string
	recentPaths []string // recently modified note paths
	todayPath   string   // path to today's daily note

	// AI config
	provider   string
	model      string
	ollamaURL  string
	apiKey     string
	nousURL    string
	nousAPIKey string

	// result to write into daily note
	resultContent string
	resultReady   bool
}

func NewDailyBriefing() DailyBriefing {
	return DailyBriefing{}
}

func (db *DailyBriefing) IsActive() bool { return db.active }

func (db *DailyBriefing) Open() {
	db.active = true
	db.state = briefingStateQuestion
	db.userGoal = ""
	db.briefing = ""
	db.briefLines = nil
	db.scroll = 0
	db.loadingTick = 0
	db.resultReady = false
	db.resultContent = ""
}

func (db *DailyBriefing) Close() { db.active = false }

func (db *DailyBriefing) SetSize(w, h int) {
	db.width = w
	db.height = h
}

func (db *DailyBriefing) SetConfig(provider, model, ollamaURL, apiKey string, nousOpts ...string) {
	db.provider = provider
	db.model = model
	db.ollamaURL = ollamaURL
	db.apiKey = apiKey
	if len(nousOpts) > 0 && nousOpts[0] != "" {
		db.nousURL = nousOpts[0]
	}
	if len(nousOpts) > 1 {
		db.nousAPIKey = nousOpts[1]
	}
}

func (db *DailyBriefing) SetVaultData(notes map[string]string, recentPaths []string, todayPath string) {
	db.notes = notes
	db.recentPaths = recentPaths
	db.todayPath = todayPath
}

func (db *DailyBriefing) GetResult() (string, bool) {
	if !db.resultReady {
		return "", false
	}
	r := db.resultContent
	db.resultReady = false
	db.resultContent = ""
	return r, true
}

// ---------------------------------------------------------------------------
// Prompt builder
// ---------------------------------------------------------------------------

func (db *DailyBriefing) buildPrompt() string {
	var sb strings.Builder

	today := time.Now().Format("2006-01-02")
	sb.WriteString(fmt.Sprintf("Today's date: %s\n\n", today))

	// User's stated goal for today
	if db.userGoal != "" {
		sb.WriteString(fmt.Sprintf("The user said their goal for today is: \"%s\"\nIncorporate this into the briefing and help them plan around it.\n\n", db.userGoal))
	}

	// Recent notes (last 10)
	sb.WriteString("RECENT NOTES (most recently modified):\n")
	limit := len(db.recentPaths)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		path := db.recentPaths[i]
		content := db.notes[path]
		preview := strings.ReplaceAll(content, "\n", " ")
		if len(preview) > 300 {
			preview = preview[:300]
		}
		sb.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", strings.TrimSuffix(path, ".md"), preview))
	}

	// Today's daily note if it exists
	if db.todayPath != "" {
		if content, ok := db.notes[db.todayPath]; ok {
			sb.WriteString(fmt.Sprintf("\nTODAY'S DAILY NOTE (%s):\n%s\n", db.todayPath, content))
		}
	}

	// Extract all open tasks across recent notes
	sb.WriteString("\nOPEN TASKS FOUND IN VAULT:\n")
	taskCount := 0
	for _, path := range db.recentPaths {
		content := db.notes[path]
		for _, line := range strings.Split(content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [ ]") {
				sb.WriteString(fmt.Sprintf("  %s (from %s)\n", trimmed, strings.TrimSuffix(path, ".md")))
				taskCount++
				if taskCount >= 20 {
					break
				}
			}
		}
		if taskCount >= 20 {
			break
		}
	}
	if taskCount == 0 {
		sb.WriteString("  (none found)\n")
	}

	return sb.String()
}

// ---------------------------------------------------------------------------
// AI calls
// ---------------------------------------------------------------------------

func (db *DailyBriefing) startBriefing() tea.Cmd {
	prompt := db.buildPrompt()
	provider := db.provider
	model := db.model
	ollamaURL := db.ollamaURL
	apiKey := db.apiKey
	nousURL := db.nousURL
	nousAPIKey := db.nousAPIKey

	return func() tea.Msg {
		switch provider {
		case "openai":
			return doBriefingOpenAI(apiKey, model, prompt)
		case "nous":
			client := NewNousClient(nousURL, nousAPIKey)
			resp, err := client.Chat(prompt)
			return briefingResultMsg{content: resp, err: err}
		default:
			return doBriefingOllama(ollamaURL, model, prompt)
		}
	}
}

func doBriefingOllama(url, model, prompt string) briefingResultMsg {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type req struct {
		Model    string `json:"model"`
		Messages []msg  `json:"messages"`
		Stream   bool   `json:"stream"`
	}
	type resp struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error,omitempty"`
	}

	reqBody := req{
		Model: model,
		Messages: []msg{
			{Role: "system", Content: deepCovenPrompt},
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	data, _ := json.Marshal(reqBody)
	client := &http.Client{Timeout: 120 * time.Second}
	httpResp, err := client.Post(url+"/api/chat", "application/json", bytes.NewReader(data))
	if err != nil {
		return briefingResultMsg{err: fmt.Errorf("cannot connect to Ollama: %w", err)}
	}
	defer httpResp.Body.Close()

	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return briefingResultMsg{err: err}
	}

	var chatResp resp
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return briefingResultMsg{err: err}
	}
	if chatResp.Error != "" {
		return briefingResultMsg{err: fmt.Errorf("Ollama: %s", chatResp.Error)}
	}
	return briefingResultMsg{content: chatResp.Message.Content}
}

func doBriefingOpenAI(apiKey, model, prompt string) briefingResultMsg {
	type msg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type req struct {
		Model    string `json:"model"`
		Messages []msg  `json:"messages"`
	}
	type resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	reqBody := req{
		Model: model,
		Messages: []msg{
			{Role: "system", Content: deepCovenPrompt},
			{Role: "user", Content: prompt},
		},
	}

	data, _ := json.Marshal(reqBody)
	client := &http.Client{Timeout: 60 * time.Second}
	httpReq, _ := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(data))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return briefingResultMsg{err: fmt.Errorf("cannot connect to OpenAI: %w", err)}
	}
	defer httpResp.Body.Close()

	body, _ := io.ReadAll(httpResp.Body)

	var chatResp resp
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return briefingResultMsg{err: err}
	}
	if chatResp.Error != nil {
		return briefingResultMsg{err: fmt.Errorf("OpenAI: %s", chatResp.Error.Message)}
	}
	if len(chatResp.Choices) == 0 {
		return briefingResultMsg{err: fmt.Errorf("OpenAI returned no response")}
	}
	return briefingResultMsg{content: chatResp.Choices[0].Message.Content}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func (db DailyBriefing) Update(msg tea.Msg) (DailyBriefing, tea.Cmd) {
	if !db.active {
		return db, nil
	}

	switch msg := msg.(type) {
	case briefingResultMsg:
		if msg.err != nil {
			db.state = briefingStateResult
			db.briefLines = []string{
				lipgloss.NewStyle().Foreground(red).Render("  Error: " + msg.err.Error()),
				"",
				DimStyle.Render("  Configure your AI provider in Settings (Ctrl+,)"),
			}
			return db, nil
		}
		db.state = briefingStateResult
		db.briefing = msg.content
		db.briefLines = db.formatBriefing(msg.content)
		db.scroll = 0
		return db, nil

	case briefingTickMsg:
		if db.state == briefingStateLoading {
			db.loadingTick++
			return db, briefingTickCmd()
		}
		return db, nil

	case tea.KeyMsg:
		switch db.state {
		case briefingStateQuestion:
			switch msg.String() {
			case "esc":
				db.active = false
			case "enter":
				db.state = briefingStateLoading
				db.loadingTick = 0
				return db, tea.Batch(db.startBriefing(), briefingTickCmd())
			case "backspace":
				if len(db.userGoal) > 0 {
					db.userGoal = db.userGoal[:len(db.userGoal)-1]
				}
			default:
				char := msg.String()
				if len(char) == 1 || char == " " {
					db.userGoal += char
				}
			}

		case briefingStateLoading:
			if msg.String() == "esc" {
				db.active = false
			}

		case briefingStateResult:
			switch msg.String() {
			case "esc", "q":
				db.active = false
			case "up", "k":
				if db.scroll > 0 {
					db.scroll--
				}
			case "down", "j":
				db.scroll++
			case "enter":
				// Write briefing into daily note
				db.resultContent = db.briefing
				db.resultReady = true
				db.active = false
			}
		}
	}
	return db, nil
}

// ---------------------------------------------------------------------------
// Format briefing for display
// ---------------------------------------------------------------------------

func (db *DailyBriefing) formatBriefing(raw string) []string {
	var lines []string
	for _, line := range strings.Split(raw, "\n") {
		if strings.HasPrefix(line, "## ") {
			lines = append(lines, lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  "+line[3:]))
		} else if strings.HasPrefix(line, "### ") {
			lines = append(lines, lipgloss.NewStyle().Foreground(blue).Bold(true).Render("  "+line[4:]))
		} else if strings.HasPrefix(strings.TrimSpace(line), "- [ ]") {
			lines = append(lines, lipgloss.NewStyle().Foreground(yellow).Render("  "+line))
		} else if strings.HasPrefix(strings.TrimSpace(line), "- [x]") {
			lines = append(lines, lipgloss.NewStyle().Foreground(green).Render("  "+line))
		} else if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			lines = append(lines, lipgloss.NewStyle().Foreground(text).Render("  "+line))
		} else if strings.TrimSpace(line) == "" {
			lines = append(lines, "")
		} else {
			lines = append(lines, lipgloss.NewStyle().Foreground(text).Render("  "+line))
		}
	}
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(green).Bold(true).Render("  Press Enter to write to daily note, Esc to close"))
	return lines
}

// ---------------------------------------------------------------------------
// View
// ---------------------------------------------------------------------------

func (db DailyBriefing) View() string {
	panelWidth := db.width * 2 / 3
	if panelWidth < 60 {
		panelWidth = 60
	}
	if panelWidth > 100 {
		panelWidth = 100
	}
	innerWidth := panelWidth - 6

	var b strings.Builder

	// Header with DeepCoven branding
	dcIcon := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("◈")
	title := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render(" DEEPCOVEN")
	subtitle := DimStyle.Render(" — Daily Briefing")
	b.WriteString("  " + dcIcon + title + subtitle)
	b.WriteString("\n")
	b.WriteString(DimStyle.Render(strings.Repeat(ThemeSeparator, innerWidth)))
	b.WriteString("\n")

	switch db.state {
	case briefingStateQuestion:
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(text).Render("  What do you want to accomplish today?"))
		b.WriteString("\n")
		b.WriteString(DimStyle.Render("  (Leave empty for a general briefing)"))
		b.WriteString("\n\n")

		// Input field
		inputBg := lipgloss.NewStyle().
			Background(surface0).
			Foreground(text).
			Width(innerWidth - 4).
			Padding(0, 1)
		prompt := lipgloss.NewStyle().Foreground(mauve).Bold(true).Render("  > ")
		cursor := lipgloss.NewStyle().Foreground(mauve).Render("|")
		b.WriteString(prompt + inputBg.Render(db.userGoal+cursor))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Enter: generate briefing  Esc: cancel"))

	case briefingStateLoading:
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		spinner := lipgloss.NewStyle().Foreground(mauve).Render(frames[db.loadingTick%len(frames)])
		b.WriteString("\n")
		b.WriteString("  " + spinner + lipgloss.NewStyle().Foreground(text).Render(" DeepCoven is analyzing your vault..."))
		b.WriteString("\n\n")
		b.WriteString(DimStyle.Render("  Reviewing recent notes, tasks, and connections."))

	case briefingStateResult:
		visH := db.height - 10
		if visH < 5 {
			visH = 5
		}
		maxScroll := len(db.briefLines) - visH
		if maxScroll < 0 {
			maxScroll = 0
		}
		if db.scroll > maxScroll {
			db.scroll = maxScroll
		}
		end := db.scroll + visH
		if end > len(db.briefLines) {
			end = len(db.briefLines)
		}
		for i := db.scroll; i < end; i++ {
			b.WriteString(db.briefLines[i])
			b.WriteString("\n")
		}
	}

	border := lipgloss.NewStyle().
		BorderStyle(PanelBorder).
		BorderForeground(OverlayBorderColor).
		Padding(1, 2).
		Width(panelWidth).
		Background(mantle)

	return border.Render(b.String())
}
