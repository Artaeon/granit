package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/artaeon/granit/internal/vault"
)

// ==================== Git ====================

func (a *GranitApp) runGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = a.vaultRoot
	out, err := cmd.CombinedOutput()
	return strings.TrimRight(string(out), "\n"), err
}

func (a *GranitApp) GitStatus() ([]string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return nil, fmt.Errorf("no vault open")
	}
	out, err := a.runGit("status", "--porcelain")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (a *GranitApp) GitLog() ([]string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return nil, fmt.Errorf("no vault open")
	}
	out, err := a.runGit("log", "--oneline", "-20")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func (a *GranitApp) GitDiff() (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}
	out, err := a.runGit("diff")
	if err != nil {
		return "", err
	}
	if out == "" {
		return "(no unstaged changes)", nil
	}
	return out, nil
}

func (a *GranitApp) GitCommit(message string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}
	if _, err := a.runGit("add", "-A"); err != nil {
		return "", fmt.Errorf("git add failed: %w", err)
	}
	out, err := a.runGit("commit", "-m", message)
	if err != nil {
		return "", fmt.Errorf("commit failed: %s", out)
	}
	return out, nil
}

func (a *GranitApp) GitPush() (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}
	out, err := a.runGit("push")
	if err != nil {
		return "", fmt.Errorf("push failed: %s", out)
	}
	return "Push successful", nil
}

func (a *GranitApp) GitPull() (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return "", fmt.Errorf("no vault open")
	}
	out, err := a.runGit("pull")
	if err != nil {
		return "", fmt.Errorf("pull failed: %s", out)
	}
	return out, nil
}

// ==================== AI Bots ====================

type BotInfo struct {
	Kind int    `json:"kind"`
	Name string `json:"name"`
	Desc string `json:"desc"`
	Icon string `json:"icon"`
}

type BotResultData struct {
	Response string   `json:"response"`
	Tags     []string `json:"tags,omitempty"`
	Links    []string `json:"links,omitempty"`
}

var botList = []BotInfo{
	{0, "Auto-Tagger", "Suggest tags for current note", "tag"},
	{1, "Link Suggester", "Find related notes", "link"},
	{2, "Summarizer", "Create a brief summary", "file"},
	{3, "Question Bot", "Ask questions about your notes", "search"},
	{4, "Writing Assistant", "Suggest improvements", "edit"},
	{5, "Title Suggester", "Suggest better titles", "edit"},
	{6, "Action Items", "Extract todos & action items", "outline"},
	{7, "MOC Generator", "Create a Map of Content", "graph"},
	{8, "Daily Digest", "Summarize vault activity", "calendar"},
}

func (a *GranitApp) GetBotList() []BotInfo {
	return botList
}

func (a *GranitApp) RunBot(kind int, notePath string, question string) (*BotResultData, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}

	note := a.vault.GetNote(notePath)
	noteContent := ""
	noteTitle := ""
	if note != nil {
		noteContent = note.Content
		noteTitle = note.Title
	}

	prompt := a.buildBotPrompt(kind, noteTitle, noteContent, question)
	provider := a.config.AIProvider

	var response string
	var err error

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
		response, err = doOllamaRequest(url, model, prompt)
	case "openai":
		if a.config.OpenAIKey == "" {
			return nil, fmt.Errorf("OpenAI API key not set — configure in Settings")
		}
		model := a.config.OpenAIModel
		if model == "" {
			model = "gpt-4o-mini"
		}
		response, err = doOpenAIRequest(a.config.OpenAIKey, model, prompt)
	default:
		response = a.runLocalBot(kind, noteTitle, noteContent, question)
	}

	if err != nil {
		return nil, err
	}

	result := &BotResultData{Response: response}

	// Parse response based on bot kind
	switch kind {
	case 0: // Auto-Tagger
		for _, tag := range strings.Split(response, ",") {
			tag = strings.TrimSpace(strings.TrimPrefix(tag, "#"))
			if tag != "" && len(tag) <= 30 {
				result.Tags = append(result.Tags, tag)
			}
		}
	case 1: // Link Suggester
		for _, line := range strings.Split(response, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, " - ", 2)
			name := strings.TrimSpace(parts[0])
			name = strings.TrimLeft(name, "0123456789.- ")
			if name != "" {
				result.Links = append(result.Links, name)
			}
		}
	}

	return result, nil
}

func (a *GranitApp) buildBotPrompt(kind int, title, content, question string) string {
	// Collect vault context
	var noteNames []string
	tagMap := make(map[string]int)
	for _, p := range a.vault.SortedPaths() {
		noteNames = append(noteNames, strings.TrimSuffix(filepath.Base(p), ".md"))
		if n := a.vault.GetNote(p); n != nil {
			if tags, ok := n.Frontmatter["tags"]; ok {
				extractTagsFromValue(tags, tagMap)
			}
		}
	}
	var tagList []string
	for t := range tagMap {
		tagList = append(tagList, t)
	}
	sort.Strings(tagList)

	switch kind {
	case 0:
		return fmt.Sprintf(`You are a note tagging assistant. Analyze the following note and suggest 3-5 relevant tags.

Note title: %s
Existing tags in vault: %s

Note content:
---
%s
---

Respond with ONLY a comma-separated list of tags (lowercase, no # prefix).`, title, strings.Join(tagList, ", "), truncate(content, 2000))
	case 1:
		return fmt.Sprintf(`You are a note linking assistant. Given the current note and a list of other notes, suggest 3-5 notes that should be linked.

Current note: %s
Content:
---
%s
---

Other notes in vault:
%s

Respond with note names (one per line) with a brief reason. Format: "note-name - reason"`, title, truncate(content, 1500), strings.Join(noteNames, "\n"))
	case 2:
		return fmt.Sprintf(`Summarize the following note in 2-4 concise sentences. Focus on the key ideas and main points.

Note: %s
---
%s
---

Provide ONLY the summary, no preamble.`, title, truncate(content, 2000))
	case 3:
		var contextBuf strings.Builder
		for _, p := range a.vault.SortedPaths() {
			n := a.vault.GetNote(p)
			if n != nil {
				contextBuf.WriteString(fmt.Sprintf("## %s\n%s\n\n", n.Title, truncate(n.Content, 300)))
			}
			if contextBuf.Len() > 4000 {
				break
			}
		}
		return fmt.Sprintf(`You are a knowledge assistant. Answer the following question based on the user's notes.

Question: %s

Notes context:
%s

Answer concisely based on what's in the notes. If the answer isn't in the notes, say so.`, question, contextBuf.String())
	case 4:
		return fmt.Sprintf(`Analyze the following note for writing quality. Provide:
1. A brief readability assessment
2. 3-5 specific suggestions to improve clarity, structure, or style
3. Any issues like passive voice, repetition, or missing structure

Note: %s
---
%s
---

Be concise and actionable.`, title, truncate(content, 2000))
	case 5:
		return fmt.Sprintf(`Suggest 5 alternative titles for this note. The current title is "%s".

Note content:
---
%s
---

Respond with ONLY the suggested titles, one per line.`, title, truncate(content, 1500))
	case 6:
		return fmt.Sprintf(`Extract all action items, tasks, and things that need to be done from this note.

Note content:
---
%s
---

Format each item as:
- [ ] action item description

List ONLY the action items, nothing else.`, truncate(content, 2000))
	case 7:
		return fmt.Sprintf(`Create a Map of Content (MOC) for this vault. Group the notes into logical categories and create a structured index with wiki-links.

Notes in vault:
%s

Generate a MOC in markdown format using [[wiki-links]] to link to notes. Group notes into 3-6 categories with headings. Include ALL notes.`, strings.Join(noteNames, "\n"))
	default:
		return ""
	}
}

func (a *GranitApp) runLocalBot(kind int, title, content, question string) string {
	switch kind {
	case 0:
		return a.localAutoTagger(content)
	case 1:
		return a.localLinkSuggester(title, content)
	case 2:
		return localSummarizer(content)
	case 8:
		return a.localDailyDigest()
	default:
		return "Local fallback: This bot requires an AI provider (Ollama or OpenAI). Configure in Settings."
	}
}

var stopwords = map[string]bool{
	"the": true, "a": true, "an": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "have": true, "has": true, "had": true,
	"do": true, "does": true, "did": true, "will": true, "would": true, "could": true,
	"should": true, "may": true, "might": true, "to": true, "of": true, "in": true,
	"for": true, "on": true, "with": true, "at": true, "by": true, "from": true,
	"as": true, "into": true, "through": true, "during": true, "before": true,
	"after": true, "above": true, "below": true, "between": true, "out": true,
	"off": true, "over": true, "under": true, "again": true, "then": true,
	"once": true, "here": true, "there": true, "when": true, "where": true,
	"why": true, "how": true, "all": true, "both": true, "each": true,
	"few": true, "more": true, "most": true, "other": true, "some": true,
	"such": true, "no": true, "not": true, "only": true, "same": true,
	"so": true, "than": true, "too": true, "very": true, "just": true,
	"because": true, "but": true, "and": true, "or": true, "if": true,
	"while": true, "about": true, "this": true, "that": true, "these": true,
	"those": true, "it": true, "its": true,
}

func significantWords(text string) map[string]int {
	words := make(map[string]int)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		w = strings.Trim(w, ".,;:!?()[]{}\"'`#*_-")
		if len(w) > 2 && !stopwords[w] {
			words[w]++
		}
	}
	return words
}

func (a *GranitApp) localAutoTagger(content string) string {
	words := significantWords(content)
	topicKeywords := map[string][]string{
		"technology": {"software", "hardware", "computer", "programming", "code", "api", "server", "database"},
		"personal":   {"journal", "diary", "feeling", "reflection", "thought", "life", "dream", "gratitude"},
		"work":       {"meeting", "deadline", "client", "report", "presentation", "email", "office"},
		"project":    {"milestone", "task", "sprint", "deliverable", "requirement", "scope", "timeline"},
		"idea":       {"concept", "brainstorm", "innovation", "creative", "proposal", "prototype"},
		"health":     {"exercise", "workout", "diet", "nutrition", "sleep", "wellness", "meditation"},
		"book":       {"reading", "chapter", "author", "novel", "review", "summary"},
	}
	scores := make(map[string]int)
	for topic, keywords := range topicKeywords {
		for _, kw := range keywords {
			if words[kw] > 0 {
				scores[topic] += 10
			}
		}
	}
	type ts struct {
		name  string
		score int
	}
	var ranked []ts
	for name, score := range scores {
		if score > 0 {
			ranked = append(ranked, ts{name, score})
		}
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	var tags []string
	for i := 0; i < 5 && i < len(ranked); i++ {
		tags = append(tags, ranked[i].name)
	}
	if len(tags) == 0 {
		return "note"
	}
	return strings.Join(tags, ", ")
}

func (a *GranitApp) localLinkSuggester(title, content string) string {
	words := significantWords(content)
	type match struct {
		path  string
		score int
	}
	var matches []match
	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil || note.Title == title {
			continue
		}
		otherWords := significantWords(note.Content)
		shared := 0
		for w := range words {
			if otherWords[w] > 0 {
				shared++
			}
		}
		if shared > 0 {
			matches = append(matches, match{note.Title, shared})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	var lines []string
	for i := 0; i < 5 && i < len(matches); i++ {
		lines = append(lines, fmt.Sprintf("%s - %d shared keywords", matches[i].path, matches[i].score))
	}
	if len(lines) == 0 {
		return "No related notes found."
	}
	return strings.Join(lines, "\n")
}

func localSummarizer(content string) string {
	lines := strings.Split(vault.StripFrontmatter(content), "\n")
	var sentences []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "- [") {
			continue
		}
		sentences = append(sentences, line)
	}
	if len(sentences) == 0 {
		return "Note is empty or contains only headings/tasks."
	}
	limit := 5
	if len(sentences) < limit {
		limit = len(sentences)
	}
	return strings.Join(sentences[:limit], " ")
}

func (a *GranitApp) localDailyDigest() string {
	stats := a.getVaultStatsInternal()
	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("Vault: %d notes, %d words, %d links\n", stats.TotalNotes, stats.TotalWords, stats.TotalLinks))
	buf.WriteString(fmt.Sprintf("Orphan notes: %d\n", stats.OrphanNotes))
	if len(stats.TopLinked) > 0 {
		buf.WriteString("\nMost connected:\n")
		for _, e := range stats.TopLinked {
			buf.WriteString(fmt.Sprintf("  %s (%d connections)\n", e.Name, e.Value))
		}
	}
	if len(stats.TopTags) > 0 {
		buf.WriteString("\nTop tags:\n")
		for _, e := range stats.TopTags {
			buf.WriteString(fmt.Sprintf("  #%s (%d notes)\n", e.Name, e.Value))
		}
	}
	return buf.String()
}

// Ollama HTTP

type ollamaReq struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type ollamaResp struct {
	Response string `json:"response"`
}

func doOllamaRequest(url, model, prompt string) (string, error) {
	body, _ := json.Marshal(ollamaReq{Model: model, Prompt: prompt, Stream: false})
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Post(url+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("cannot connect to Ollama at %s — is it running? Try: ollama serve", url)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Ollama error (status %d) — check model is pulled: ollama pull %s", resp.StatusCode, model)
	}
	var r ollamaResp
	json.Unmarshal(data, &r)
	return r.Response, nil
}

// OpenAI HTTP

type openaiReq struct {
	Model    string         `json:"model"`
	Messages []openaiMsg    `json:"messages"`
}

type openaiMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func doOpenAIRequest(apiKey, model, prompt string) (string, error) {
	body, _ := json.Marshal(openaiReq{
		Model: model,
		Messages: []openaiMsg{
			{Role: "system", Content: "You are a helpful note-taking assistant. Be concise and actionable."},
			{Role: "user", Content: prompt},
		},
	})
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
	json.Unmarshal(data, &r)
	if r.Error != nil {
		return "", fmt.Errorf("OpenAI: %s", r.Error.Message)
	}
	if len(r.Choices) == 0 {
		return "", fmt.Errorf("OpenAI returned empty response — try a different model")
	}
	return r.Choices[0].Message.Content, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ==================== Export ====================

var (
	reHeading     = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
	reBold        = regexp.MustCompile(`\*\*(.+?)\*\*`)
	reItalic      = regexp.MustCompile(`\*(.+?)\*`)
	reInlineCode  = regexp.MustCompile("`([^`]+)`")
	reWikiLink    = regexp.MustCompile(`\[\[([^\]|]+)(?:\|([^\]]+))?\]\]`)
	reMdLink      = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	reCheckboxOn  = regexp.MustCompile(`^(\s*)- \[x\]\s+(.+)$`)
	reCheckboxOff = regexp.MustCompile(`^(\s*)- \[ \]\s+(.+)$`)
	reListItem    = regexp.MustCompile(`^(\s*)[-*+]\s+(.+)$`)
	reBlockquote  = regexp.MustCompile(`^>\s*(.*)$`)
)

func (a *GranitApp) ExportHTML(relPath string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	note := a.vault.GetNote(relPath)
	if note == nil {
		return "", fmt.Errorf("note not found")
	}
	html := markdownToHTML(note.Content)
	wrapped := wrapHTML(note.Title, html)
	outPath := filepath.Join(a.vaultRoot, strings.TrimSuffix(relPath, ".md")+".html")
	os.MkdirAll(filepath.Dir(outPath), 0755)
	if err := atomicWriteFile(outPath, []byte(wrapped), 0644); err != nil {
		return "", err
	}
	return outPath, nil
}

func (a *GranitApp) ExportText(relPath string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	note := a.vault.GetNote(relPath)
	if note == nil {
		return "", fmt.Errorf("note not found")
	}
	text := stripMarkdown(note.Content)
	outPath := filepath.Join(a.vaultRoot, strings.TrimSuffix(relPath, ".md")+".txt")
	os.MkdirAll(filepath.Dir(outPath), 0755)
	if err := atomicWriteFile(outPath, []byte(text), 0644); err != nil {
		return "", err
	}
	return outPath, nil
}

func (a *GranitApp) ExportPDF(relPath string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	if _, err := exec.LookPath("pandoc"); err != nil {
		return "", fmt.Errorf("pandoc not found — install it for PDF export")
	}
	inputPath := filepath.Join(a.vaultRoot, relPath)
	outPath := filepath.Join(a.vaultRoot, strings.TrimSuffix(relPath, ".md")+".pdf")
	cmd := exec.Command("pandoc", inputPath, "-o", outPath)
	cmd.Dir = a.vaultRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("pandoc error: %s", string(out))
	}
	return outPath, nil
}

func (a *GranitApp) ExportAll() (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	exportDir := filepath.Join(a.vaultRoot, "_export")
	os.MkdirAll(exportDir, 0755)
	var exported int
	var indexLinks []string
	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}
		html := markdownToHTML(note.Content)
		wrapped := wrapHTML(note.Title, html)
		outName := strings.TrimSuffix(p, ".md") + ".html"
		outPath := filepath.Join(exportDir, outName)
		os.MkdirAll(filepath.Dir(outPath), 0755)
		if err := atomicWriteFile(outPath, []byte(wrapped), 0644); err != nil {
			continue
		}
		indexLinks = append(indexLinks, fmt.Sprintf(`<li><a href="%s">%s</a></li>`, outName, htmlEscape(note.Title)))
		exported++
	}
	indexHTML := wrapHTML("Vault Export", "<h1>Vault Export</h1>\n<ul>\n"+strings.Join(indexLinks, "\n")+"\n</ul>")
	_ = atomicWriteFile(filepath.Join(exportDir, "index.html"), []byte(indexHTML), 0644)
	return fmt.Sprintf("Exported %d notes to %s", exported, exportDir), nil
}

func markdownToHTML(content string) string {
	lines := strings.Split(content, "\n")
	var buf strings.Builder
	inCodeBlock := false
	inList := false
	inBlockquote := false
	inFrontmatter := false
	frontmatterCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Frontmatter
		if trimmed == "---" {
			frontmatterCount++
			if frontmatterCount <= 2 {
				inFrontmatter = frontmatterCount == 1
				if frontmatterCount == 2 {
					inFrontmatter = false
				}
				continue
			}
		}
		if inFrontmatter {
			continue
		}

		// Code blocks
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				buf.WriteString("</code></pre>\n")
				inCodeBlock = false
			} else {
				lang := strings.TrimPrefix(trimmed, "```")
				if lang != "" {
					buf.WriteString(fmt.Sprintf(`<pre><code class="language-%s">`, lang))
				} else {
					buf.WriteString("<pre><code>")
				}
				inCodeBlock = true
			}
			continue
		}
		if inCodeBlock {
			buf.WriteString(htmlEscape(line) + "\n")
			continue
		}

		// Empty line
		if trimmed == "" {
			if inList {
				buf.WriteString("</ul>\n")
				inList = false
			}
			if inBlockquote {
				buf.WriteString("</blockquote>\n")
				inBlockquote = false
			}
			continue
		}

		// Headings
		if m := reHeading.FindStringSubmatch(trimmed); m != nil {
			level := len(m[1])
			buf.WriteString(fmt.Sprintf("<h%d>%s</h%d>\n", level, convertInline(m[2]), level))
			continue
		}

		// Checkboxes
		if m := reCheckboxOn.FindStringSubmatch(line); m != nil {
			if !inList {
				buf.WriteString(`<ul class="checklist">` + "\n")
				inList = true
			}
			buf.WriteString(fmt.Sprintf(`<li><input type="checkbox" checked disabled> %s</li>`+"\n", convertInline(m[2])))
			continue
		}
		if m := reCheckboxOff.FindStringSubmatch(line); m != nil {
			if !inList {
				buf.WriteString(`<ul class="checklist">` + "\n")
				inList = true
			}
			buf.WriteString(fmt.Sprintf(`<li><input type="checkbox" disabled> %s</li>`+"\n", convertInline(m[2])))
			continue
		}

		// List items
		if m := reListItem.FindStringSubmatch(line); m != nil {
			if !inList {
				buf.WriteString("<ul>\n")
				inList = true
			}
			buf.WriteString(fmt.Sprintf("<li>%s</li>\n", convertInline(m[2])))
			continue
		}

		// Blockquotes
		if m := reBlockquote.FindStringSubmatch(line); m != nil {
			if !inBlockquote {
				buf.WriteString("<blockquote>\n")
				inBlockquote = true
			}
			buf.WriteString(convertInline(m[1]) + "<br>\n")
			continue
		}

		// Paragraph
		buf.WriteString(fmt.Sprintf("<p>%s</p>\n", convertInline(trimmed)))
	}

	if inList {
		buf.WriteString("</ul>\n")
	}
	if inBlockquote {
		buf.WriteString("</blockquote>\n")
	}
	if inCodeBlock {
		buf.WriteString("</code></pre>\n")
	}

	return buf.String()
}

func convertInline(s string) string {
	s = htmlEscape(s)
	s = reBold.ReplaceAllString(s, "<strong>$1</strong>")
	s = reItalic.ReplaceAllString(s, "<em>$1</em>")
	s = reInlineCode.ReplaceAllString(s, "<code>$1</code>")
	s = reWikiLink.ReplaceAllStringFunc(s, func(m string) string {
		parts := reWikiLink.FindStringSubmatch(m)
		target := parts[1]
		display := target
		if len(parts) > 2 && parts[2] != "" {
			display = parts[2]
		}
		return fmt.Sprintf(`<a href="%s.html">%s</a>`, target, display)
	})
	s = reMdLink.ReplaceAllString(s, `<a href="$2">$1</a>`)
	return s
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, `"`, "&quot;")
	return s
}

func wrapHTML(title, body string) string {
	title = htmlEscape(title)
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>%s</title>
<style>
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;max-width:800px;margin:2rem auto;padding:0 1rem;line-height:1.6;color:#cdd6f4;background:#1e1e2e}
a{color:#89b4fa}code{background:#313244;padding:.2em .4em;border-radius:3px;font-size:.9em}
pre{background:#313244;padding:1em;border-radius:6px;overflow-x:auto}pre code{background:none;padding:0}
blockquote{border-left:3px solid #cba6f7;margin-left:0;padding-left:1em;color:#a6adc8}
h1,h2,h3,h4,h5,h6{color:#cba6f7}
ul.checklist{list-style:none;padding-left:1.2em}input[type="checkbox"]{margin-right:.5em}
</style>
</head>
<body>
%s
</body>
</html>`, title, body)
}

func stripMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	var buf strings.Builder
	inFrontmatter := false
	frontmatterCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			frontmatterCount++
			if frontmatterCount <= 2 {
				inFrontmatter = frontmatterCount == 1
				if frontmatterCount == 2 {
					inFrontmatter = false
				}
				continue
			}
		}
		if inFrontmatter {
			continue
		}
		// Strip heading markers
		if m := reHeading.FindStringSubmatch(trimmed); m != nil {
			buf.WriteString(m[2] + "\n")
			continue
		}
		// Strip formatting
		line = reBold.ReplaceAllString(line, "$1")
		line = reItalic.ReplaceAllString(line, "$1")
		line = reInlineCode.ReplaceAllString(line, "$1")
		line = reWikiLink.ReplaceAllStringFunc(line, func(m string) string {
			parts := reWikiLink.FindStringSubmatch(m)
			if len(parts) > 2 && parts[2] != "" {
				return parts[2]
			}
			return parts[1]
		})
		line = reMdLink.ReplaceAllString(line, "$1")
		buf.WriteString(line + "\n")
	}
	return buf.String()
}

// ==================== Calendar ====================

type CalendarTask struct {
	Text     string `json:"text"`
	Done     bool   `json:"done"`
	NotePath string `json:"notePath"`
	Priority int    `json:"priority"`
	LineNum  int    `json:"lineNum"`
}

type CalendarEventDTO struct {
	Title    string `json:"title"`
	Date     string `json:"date"`
	EndDate  string `json:"endDate"`
	Location string `json:"location"`
	AllDay   bool   `json:"allDay"`
}

type CalendarData struct {
	Year       int                        `json:"year"`
	Month      int                        `json:"month"`
	DailyNotes []string                   `json:"dailyNotes"`
	Tasks      map[string][]CalendarTask  `json:"tasks"`
	Events     []CalendarEventDTO         `json:"events"`
}

var taskPattern = regexp.MustCompile(`^- \[([ xX])\] (.+)`)

func (a *GranitApp) GetCalendarData(year, month int) *CalendarData {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return &CalendarData{Year: year, Month: month, Tasks: make(map[string][]CalendarTask)}
	}

	data := &CalendarData{
		Year: year, Month: month,
		Tasks: make(map[string][]CalendarTask),
	}

	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}

		// Check if filename is a date
		base := strings.TrimSuffix(filepath.Base(p), ".md")
		if _, err := time.Parse("2006-01-02", base); err == nil {
			data.DailyNotes = append(data.DailyNotes, base)
		}

		// Extract tasks
		dateStr := ""
		if _, err := time.Parse("2006-01-02", base); err == nil {
			dateStr = base
		} else if d, ok := note.Frontmatter["date"]; ok {
			if ds, ok := d.(string); ok {
				dateStr = ds
			}
		}

		for lineIdx, line := range strings.Split(note.Content, "\n") {
			m := taskPattern.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			done := m[1] == "x" || m[1] == "X"
			task := CalendarTask{
				Text:     m[2],
				Done:     done,
				NotePath: p,
				Priority: taskPriority(m[2]),
				LineNum:  lineIdx + 1,
			}
			key := dateStr
			if key == "" {
				key = "_undated"
			}
			data.Tasks[key] = append(data.Tasks[key], task)
		}
	}

	// Parse .ics files
	data.Events = a.parseICSFiles()

	return data
}

func taskPriority(text string) int {
	if strings.Contains(text, "\U0001f534") {
		return 4
	}
	if strings.Contains(text, "\U0001f7e0") {
		return 3
	}
	if strings.Contains(text, "\U0001f7e1") {
		return 2
	}
	if strings.Contains(text, "\U0001f535") {
		return 1
	}
	return 0
}

func (a *GranitApp) ToggleTask(notePath string, lineNum int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vault == nil {
		return fmt.Errorf("no vault open")
	}
	absPath, err := a.validatePath(notePath)
	if err != nil {
		return err
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	if lineNum < 1 || lineNum > len(lines) {
		return fmt.Errorf("invalid line number")
	}
	line := lines[lineNum-1]
	if strings.Contains(line, "- [ ] ") {
		lines[lineNum-1] = strings.Replace(line, "- [ ] ", "- [x] ", 1)
	} else if strings.Contains(line, "- [x] ") || strings.Contains(line, "- [X] ") {
		lines[lineNum-1] = strings.Replace(strings.Replace(line, "- [x] ", "- [ ] ", 1), "- [X] ", "- [ ] ", 1)
	}
	newContent := strings.Join(lines, "\n")
	if err := atomicWriteFile(absPath, []byte(newContent), 0644); err != nil {
		return err
	}
	// Update vault
	note := a.vault.Notes[notePath]
	if note != nil {
		note.Content = newContent
	}
	return nil
}

func (a *GranitApp) parseICSFiles() []CalendarEventDTO {
	var events []CalendarEventDTO
	filepath.Walk(a.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(info.Name()), ".ics") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		events = append(events, parseICSContent(string(content))...)
		return nil
	})
	return events
}

func parseICSContent(content string) []CalendarEventDTO {
	var events []CalendarEventDTO
	lines := strings.Split(content, "\n")
	inEvent := false
	var current CalendarEventDTO

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "BEGIN:VEVENT" {
			inEvent = true
			current = CalendarEventDTO{}
			continue
		}
		if line == "END:VEVENT" {
			if inEvent && current.Title != "" {
				events = append(events, current)
			}
			inEvent = false
			continue
		}
		if !inEvent {
			continue
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			continue
		}
		key := line[:idx]
		value := line[idx+1:]
		baseProp := key
		if si := strings.Index(key, ";"); si >= 0 {
			baseProp = key[:si]
		}
		switch baseProp {
		case "SUMMARY":
			current.Title = value
		case "LOCATION":
			current.Location = value
		case "DTSTART":
			t, allDay := parseICSTime(value)
			if !t.IsZero() {
				current.Date = t.Format("2006-01-02")
				current.AllDay = allDay
			}
		case "DTEND":
			t, _ := parseICSTime(value)
			if !t.IsZero() {
				current.EndDate = t.Format("2006-01-02")
			}
		}
	}
	return events
}

func parseICSTime(value string) (time.Time, bool) {
	if t, err := time.Parse("20060102T150405Z", value); err == nil {
		return t.Local(), false
	}
	if t, err := time.Parse("20060102T150405", value); err == nil {
		return t, false
	}
	if t, err := time.Parse("20060102", value); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// ==================== Git Branch ====================

func (a *GranitApp) GetGitBranch() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return ""
	}
	out, err := a.runGit("branch", "--show-current")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
