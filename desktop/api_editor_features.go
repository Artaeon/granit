package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ==================== DTO Types ====================

// SnippetDTO represents a snippet trigger/expansion pair for the frontend.
type SnippetDTO struct {
	Trigger     string `json:"trigger"`
	Content     string `json:"content"`
	Description string `json:"description"`
}

// TableDataDTO represents parsed markdown table data.
type TableDataDTO struct {
	Headers   []string   `json:"headers"`
	Rows      [][]string `json:"rows"`
	StartLine int        `json:"startLine"`
	EndLine   int        `json:"endLine"`
}

// ChatMessageDTO represents a single message in the AI chat conversation.
type ChatMessageDTO struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ==================== AI Chat ====================

// ChatWithAI sends a conversation to the configured AI provider and returns the
// assistant response. The messages parameter is a JSON string of ChatMessageDTO
// objects representing the conversation history.
func (a *GranitApp) ChatWithAI(messages string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}

	var chatMsgs []ChatMessageDTO
	if err := json.Unmarshal([]byte(messages), &chatMsgs); err != nil {
		return "", fmt.Errorf("invalid message format: %w", err)
	}

	if len(chatMsgs) == 0 {
		return "", fmt.Errorf("no messages provided")
	}

	// Extract the latest user message for context finding.
	var latestUserMsg string
	for i := len(chatMsgs) - 1; i >= 0; i-- {
		if chatMsgs[i].Role == "user" {
			latestUserMsg = chatMsgs[i].Content
			break
		}
	}

	// Build note context by finding relevant notes.
	noteContents := make(map[string]string)
	for _, p := range a.vault.SortedPaths() {
		n := a.vault.GetNote(p)
		if n != nil {
			noteContents[p] = n.Content
		}
	}

	contextText, usedNotes := chatFindRelevantNotes(latestUserMsg, noteContents, 4000)

	// Build the system prompt with context.
	systemPrompt := "You are a helpful assistant answering questions about the user's personal knowledge base. Use the provided note excerpts to give accurate, specific answers. If the notes don't contain relevant information, say so. Keep answers concise and reference specific notes when possible."
	if contextText != "" {
		systemPrompt += "\n\nHere are relevant excerpts from the user's notes:\n" + contextText
	}

	provider := a.config.AIProvider

	switch provider {
	case "ollama":
		url := a.config.OllamaURL
		if url == "" {
			url = "http://localhost:11434"
		}
		model := a.config.OllamaModel
		if model == "" {
			model = "qwen2.5:0.5b"
		}
		return doChatOllama(url, model, systemPrompt, chatMsgs)

	case "openai":
		if a.config.OpenAIKey == "" {
			return "", fmt.Errorf("OpenAI API key not set -- configure in Settings")
		}
		model := a.config.OpenAIModel
		if model == "" {
			model = "gpt-4o-mini"
		}
		return doChatOpenAI(a.config.OpenAIKey, model, systemPrompt, chatMsgs)

	default: // "local"
		return chatLocalFallback(latestUserMsg, usedNotes, contextText), nil
	}
}

// doChatOllama sends a multi-turn conversation to Ollama using the /api/chat endpoint.
func doChatOllama(url, model, systemPrompt string, msgs []ChatMessageDTO) (string, error) {
	type ollamaChatMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	type ollamaChatReq struct {
		Model    string          `json:"model"`
		Messages []ollamaChatMsg `json:"messages"`
		Stream   bool            `json:"stream"`
	}

	var ollamaMsgs []ollamaChatMsg
	ollamaMsgs = append(ollamaMsgs, ollamaChatMsg{Role: "system", Content: systemPrompt})
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		ollamaMsgs = append(ollamaMsgs, ollamaChatMsg{Role: m.Role, Content: m.Content})
	}

	body, _ := json.Marshal(ollamaChatReq{Model: model, Messages: ollamaMsgs, Stream: false})
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(url+"/api/chat", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("cannot connect to Ollama at %s -- is it running? Try: ollama serve", url)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Ollama error (status %d) -- check model is pulled: ollama pull %s", resp.StatusCode, model)
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}

	return result.Message.Content, nil
}

// doChatOpenAI sends a multi-turn conversation to OpenAI's chat completions API.
func doChatOpenAI(apiKey, model, systemPrompt string, msgs []ChatMessageDTO) (string, error) {
	var oaiMsgs []openaiMsg
	oaiMsgs = append(oaiMsgs, openaiMsg{Role: "system", Content: systemPrompt})
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		oaiMsgs = append(oaiMsgs, openaiMsg{Role: m.Role, Content: m.Content})
	}

	body, _ := json.Marshal(openaiReq{Model: model, Messages: oaiMsgs})
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("cannot connect to OpenAI: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var r openaiResp
	if err := json.Unmarshal(data, &r); err != nil {
		return "", fmt.Errorf("invalid OpenAI response: %w", err)
	}
	if r.Error != nil {
		return "", fmt.Errorf("OpenAI: %s", r.Error.Message)
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("OpenAI returned empty response -- try a different model")
	}
	return r.Choices[0].Message.Content, nil
}

// chatLocalFallback provides a keyword-based response when no AI provider is configured.
func chatLocalFallback(query string, usedNotes []string, contextText string) string {
	if len(usedNotes) == 0 {
		return "No notes matched your query. Try different keywords or switch to an AI provider (Ollama/OpenAI) in Settings for smarter search."
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Found %d relevant note(s): **%s**\n\n", len(usedNotes), strings.Join(usedNotes, "**, **")))
	b.WriteString("Here are the matching excerpts:\n")
	b.WriteString(contextText)
	b.WriteString("\n\n_Using local keyword search. For AI-powered answers, configure Ollama or OpenAI in Settings._")
	return b.String()
}

// chatFindRelevantNotes scores notes by keyword frequency against the query and
// returns the top matches concatenated up to maxChars.
func chatFindRelevantNotes(query string, notes map[string]string, maxChars int) (string, []string) {
	if len(notes) == 0 || query == "" {
		return "", nil
	}

	chatStopwords := map[string]bool{
		"the": true, "a": true, "an": true, "is": true, "are": true,
		"was": true, "were": true, "be": true, "been": true,
		"have": true, "has": true, "had": true, "do": true, "does": true,
		"did": true, "will": true, "would": true, "could": true, "should": true,
		"to": true, "of": true, "in": true, "for": true, "on": true,
		"with": true, "at": true, "by": true, "from": true, "as": true,
		"and": true, "or": true, "but": true, "if": true, "not": true,
		"no": true, "this": true, "that": true, "it": true, "my": true,
		"your": true, "what": true, "how": true, "why": true, "when": true,
		"where": true, "which": true,
	}

	words := strings.Fields(strings.ToLower(query))
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?\"'()[]{}#")
		if len(w) < 2 {
			continue
		}
		if chatStopwords[w] {
			continue
		}
		keywords = append(keywords, w)
	}
	if len(keywords) == 0 {
		for _, w := range words {
			w = strings.Trim(w, ".,;:!?\"'()[]{}#")
			if len(w) >= 2 {
				keywords = append(keywords, w)
			}
		}
	}
	if len(keywords) == 0 {
		return "", nil
	}

	type scored struct {
		path  string
		score int
	}
	var scores []scored

	for path, body := range notes {
		lowerBody := strings.ToLower(body)
		lowerPath := strings.ToLower(path)
		score := 0
		for _, kw := range keywords {
			score += strings.Count(lowerBody, kw)
			if strings.Contains(lowerPath, kw) {
				score += 3
			}
		}
		if score > 0 {
			scores = append(scores, scored{path: path, score: score})
		}
	}

	if len(scores) == 0 {
		return "", nil
	}

	// Sort descending by score.
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score > scores[i].score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}

	var b strings.Builder
	totalLen := 0
	var usedNotes []string
	for _, s := range scores {
		body := notes[s.path]
		noteName := strings.TrimSuffix(s.path, ".md")
		header := fmt.Sprintf("\n--- Note: %s ---\n", noteName)
		remaining := maxChars - totalLen - len(header)
		if remaining <= 0 {
			break
		}
		if len(body) > remaining {
			body = body[:remaining]
		}
		b.WriteString(header)
		b.WriteString(body)
		totalLen += len(header) + len(body)
		usedNotes = append(usedNotes, noteName)
		if totalLen >= maxChars {
			break
		}
	}

	return b.String(), usedNotes
}

// ==================== Snippets ====================

// builtinSnippetDTOs returns the default set of snippets.
func builtinSnippetDTOs() []SnippetDTO {
	return []SnippetDTO{
		{Trigger: "/date", Content: "{{date}}", Description: "Insert today's date"},
		{Trigger: "/time", Content: "{{time}}", Description: "Insert current time"},
		{Trigger: "/datetime", Content: "{{date}} {{time}}", Description: "Insert date and time"},
		{Trigger: "/todo", Content: "- [ ] ", Description: "Insert checkbox"},
		{Trigger: "/done", Content: "- [x] ", Description: "Insert checked box"},
		{Trigger: "/h1", Content: "# ", Description: "Heading 1"},
		{Trigger: "/h2", Content: "## ", Description: "Heading 2"},
		{Trigger: "/h3", Content: "### ", Description: "Heading 3"},
		{Trigger: "/link", Content: "[[]]", Description: "Insert wikilink"},
		{Trigger: "/code", Content: "```\n\n```", Description: "Code block"},
		{Trigger: "/table", Content: "| Column 1 | Column 2 | Column 3 |\n|----------|----------|----------|\n|          |          |          |", Description: "Insert table"},
		{Trigger: "/meeting", Content: "## Meeting Notes\n\n**Date:** {{date}}\n**Attendees:**\n-\n\n**Agenda:**\n1.\n\n**Notes:**\n\n**Action Items:**\n- [ ] ", Description: "Meeting notes template"},
		{Trigger: "/daily", Content: "# {{date}}\n\n## Tasks\n- [ ] \n\n## Notes\n\n## Reflection\n", Description: "Daily note header"},
		{Trigger: "/callout", Content: "> [!note]\n> ", Description: "Callout block"},
		{Trigger: "/divider", Content: "\n---\n", Description: "Horizontal divider"},
		{Trigger: "/quote", Content: "> ", Description: "Block quote"},
		{Trigger: "/img", Content: "![alt text](url)", Description: "Image placeholder"},
		{Trigger: "/frontmatter", Content: "---\ntitle: \ndate: {{date}}\ntags: []\n---\n", Description: "YAML frontmatter"},
	}
}

// snippetsFilePath returns the path to the user's custom snippets file.
func (a *GranitApp) snippetsFilePath() string {
	if a.vaultRoot != "" {
		return filepath.Join(a.vaultRoot, ".granit", "snippets.json")
	}
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".config", "granit", "snippets.json")
}

// GetSnippets returns all available snippets (built-in plus user-defined).
func (a *GranitApp) GetSnippets() ([]SnippetDTO, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	snippets := builtinSnippetDTOs()

	// Load user snippets from .granit/snippets.json
	data, err := os.ReadFile(a.snippetsFilePath())
	if err == nil {
		var userSnippets []SnippetDTO
		if json.Unmarshal(data, &userSnippets) == nil {
			snippets = append(snippets, userSnippets...)
		}
	}

	// Load snippets from <vault>/snippets/ directory
	if a.vaultRoot != "" {
		snippetsDir := filepath.Join(a.vaultRoot, "snippets")
		entries, err := os.ReadDir(snippetsDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
					continue
				}
				content, err := os.ReadFile(filepath.Join(snippetsDir, entry.Name()))
				if err != nil {
					continue
				}
				trigger := "/" + strings.TrimSuffix(entry.Name(), ".md")
				snippets = append(snippets, SnippetDTO{
					Trigger:     trigger,
					Content:     string(content),
					Description: "Custom: " + strings.TrimSuffix(entry.Name(), ".md"),
				})
			}
		}
	}

	return snippets, nil
}

// SaveSnippets persists user-defined snippets to disk.
func (a *GranitApp) SaveSnippets(data string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	var snippets []SnippetDTO
	if err := json.Unmarshal([]byte(data), &snippets); err != nil {
		return fmt.Errorf("invalid snippet data: %w", err)
	}

	filePath := a.snippetsFilePath()
	os.MkdirAll(filepath.Dir(filePath), 0755)

	jsonData, err := json.MarshalIndent(snippets, "", "  ")
	if err != nil {
		return err
	}

	return atomicWriteFile(filePath, jsonData, 0644)
}

// ==================== Table Parsing ====================

// ParseMarkdownTable parses a markdown table from content starting near the given line.
func (a *GranitApp) ParseMarkdownTable(content string, lineStart int) (TableDataDTO, error) {
	lines := strings.Split(content, "\n")
	if lineStart < 0 || lineStart >= len(lines) {
		return TableDataDTO{}, fmt.Errorf("line %d out of range", lineStart)
	}

	// Check if current line looks like a table line (contains |).
	if !strings.Contains(lines[lineStart], "|") {
		return TableDataDTO{}, fmt.Errorf("no table found at line %d", lineStart)
	}

	// Find table boundaries by scanning up and down.
	start := lineStart
	for start > 0 && strings.Contains(lines[start-1], "|") {
		start--
	}
	end := lineStart
	for end < len(lines)-1 && strings.Contains(lines[end+1], "|") {
		end++
	}

	tableLines := lines[start : end+1]
	if len(tableLines) < 2 {
		return TableDataDTO{}, fmt.Errorf("table too short")
	}

	// Parse header row.
	headers := tableParseRow(tableLines[0])
	if len(headers) == 0 {
		return TableDataDTO{}, fmt.Errorf("no headers found")
	}
	numCols := len(headers)

	// Parse data rows (skip separator if present).
	var rows [][]string
	startIdx := 1
	if len(tableLines) >= 2 && tableIsSeparator(tableLines[1]) {
		startIdx = 2
	}
	for i := startIdx; i < len(tableLines); i++ {
		cells := tableParseRow(tableLines[i])
		// Normalize to numCols.
		row := make([]string, numCols)
		for j := 0; j < numCols && j < len(cells); j++ {
			row[j] = cells[j]
		}
		rows = append(rows, row)
	}

	return TableDataDTO{
		Headers:   headers,
		Rows:      rows,
		StartLine: start,
		EndLine:   end,
	}, nil
}

// tableParseRow splits a markdown table row into trimmed cell values.
func tableParseRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "|") {
		trimmed = trimmed[1:]
	}
	if strings.HasSuffix(trimmed, "|") {
		trimmed = trimmed[:len(trimmed)-1]
	}
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// tableIsSeparator checks if a line is a markdown table separator row (e.g., |---|---|).
func tableIsSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.Contains(trimmed, "|") {
		return false
	}
	// Remove pipes and spaces, check if only dashes and colons remain.
	cleaned := strings.ReplaceAll(trimmed, "|", "")
	cleaned = strings.ReplaceAll(cleaned, " ", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, ":", "")
	return cleaned == ""
}
