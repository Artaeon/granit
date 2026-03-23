package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ==================== Habits ====================

// GetHabits loads and returns the habits JSON from .granit/habits.json.
func (a *GranitApp) GetHabits() (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}
	p := filepath.Join(a.vaultRoot, ".granit", "habits.json")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "[]", nil
		}
		return "", err
	}
	return string(data), nil
}

// SaveHabits writes the habits JSON to .granit/habits.json.
func (a *GranitApp) SaveHabits(data string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vault == nil {
		return fmt.Errorf("no vault open")
	}
	dir := filepath.Join(a.vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	p := filepath.Join(dir, "habits.json")
	return atomicWriteFile(p, []byte(data), 0o644)
}

// ==================== Daily Briefing ====================

// GetDailyBriefing returns today's vault activity summary.
func (a *GranitApp) GetDailyBriefing() (map[string]interface{}, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}

	now := time.Now()
	todayStr := now.Format("2006-01-02")
	greeting := "Good morning"
	if now.Hour() >= 12 && now.Hour() < 17 {
		greeting = "Good afternoon"
	} else if now.Hour() >= 17 {
		greeting = "Good evening"
	}

	// Collect notes modified today and recent notes
	type modEntry struct {
		relPath string
		modTime time.Time
	}
	var allNotes []modEntry
	modifiedToday := 0
	totalTasks := 0
	completedTasks := 0
	var recentNotes []map[string]interface{}

	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}

		absPath := filepath.Join(a.vaultRoot, p)
		info, err := os.Stat(absPath)
		if err != nil {
			continue
		}

		mt := info.ModTime()
		allNotes = append(allNotes, modEntry{relPath: p, modTime: mt})

		if mt.Format("2006-01-02") == todayStr {
			modifiedToday++
		}

		// Count tasks
		for _, line := range strings.Split(note.Content, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "- [ ]") {
				totalTasks++
			} else if strings.HasPrefix(trimmed, "- [x]") {
				totalTasks++
				completedTasks++
			}
		}
	}

	// Sort by modification time (newest first)
	sort.Slice(allNotes, func(i, j int) bool {
		return allNotes[i].modTime.After(allNotes[j].modTime)
	})

	limit := 5
	if len(allNotes) < limit {
		limit = len(allNotes)
	}
	for _, entry := range allNotes[:limit] {
		recentNotes = append(recentNotes, map[string]interface{}{
			"relPath": entry.relPath,
			"title":   strings.TrimSuffix(filepath.Base(entry.relPath), ".md"),
			"modTime": entry.modTime.Format(time.RFC3339),
		})
	}

	// Upcoming events from calendar
	var upcomingEvents []map[string]interface{}
	calEvents := a.parseICSFiles()
	for _, ev := range calEvents {
		if ev.Date >= todayStr {
			upcomingEvents = append(upcomingEvents, map[string]interface{}{
				"title":    ev.Title,
				"date":     ev.Date,
				"location": ev.Location,
				"allDay":   ev.AllDay,
			})
			if len(upcomingEvents) >= 5 {
				break
			}
		}
	}

	result := map[string]interface{}{
		"greeting":       greeting,
		"date":           todayStr,
		"dateFormatted":  now.Format("Monday, January 2, 2006"),
		"modifiedToday":  modifiedToday,
		"totalNotes":     len(allNotes),
		"totalTasks":     totalTasks,
		"completedTasks": completedTasks,
		"recentNotes":    recentNotes,
		"upcomingEvents": upcomingEvents,
	}

	return result, nil
}

// ==================== Journal Prompts ====================

// GetJournalPrompts returns a list of curated writing prompts.
func (a *GranitApp) GetJournalPrompts() []map[string]string {
	prompts := []map[string]string{
		// Gratitude
		{"category": "Gratitude", "text": "What are 3 things you're grateful for today?"},
		{"category": "Gratitude", "text": "Who is someone you appreciate but haven't thanked recently?"},
		{"category": "Gratitude", "text": "What small comfort did you enjoy today that you usually overlook?"},
		{"category": "Gratitude", "text": "What ability or skill do you have that you're thankful for?"},
		{"category": "Gratitude", "text": "What memory from the past week makes you smile?"},
		{"category": "Gratitude", "text": "What challenge in your life are you secretly grateful for?"},
		// Reflection
		{"category": "Reflection", "text": "What was the most meaningful moment of your day?"},
		{"category": "Reflection", "text": "If you could relive one moment from today, which would it be?"},
		{"category": "Reflection", "text": "What surprised you today?"},
		{"category": "Reflection", "text": "What is something you believed a year ago that you no longer believe?"},
		{"category": "Reflection", "text": "What would your younger self think of who you are now?"},
		{"category": "Reflection", "text": "What pattern in your life do you keep noticing?"},
		// Creativity
		{"category": "Creativity", "text": "If you could create anything tomorrow, what would it be?"},
		{"category": "Creativity", "text": "What idea has been nagging at you that you haven't explored yet?"},
		{"category": "Creativity", "text": "What two unrelated interests could you combine into something new?"},
		{"category": "Creativity", "text": "Describe a world you'd like to live in. What's different?"},
		{"category": "Creativity", "text": "Write a six-word story about your day."},
		// Goals
		{"category": "Goals", "text": "What's one step you can take tomorrow toward your biggest goal?"},
		{"category": "Goals", "text": "Where do you see yourself in one year if you stay on your current path?"},
		{"category": "Goals", "text": "What goal have you been putting off, and what's holding you back?"},
		{"category": "Goals", "text": "What does success look like for you right now?"},
		{"category": "Goals", "text": "What would your future self thank you for starting today?"},
		// Mindfulness
		{"category": "Mindfulness", "text": "What emotions did you experience most strongly today?"},
		{"category": "Mindfulness", "text": "Describe the present moment using all five senses."},
		{"category": "Mindfulness", "text": "How are you feeling right now, honestly, without judgment?"},
		{"category": "Mindfulness", "text": "What is weighing on your mind that you can write down and release?"},
		{"category": "Mindfulness", "text": "When did you last feel truly present, without thinking of past or future?"},
	}
	return prompts
}

// ==================== Writing Feedback ====================

// GetWritingFeedback uses the configured AI provider to give writing feedback.
func (a *GranitApp) GetWritingFeedback(content string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vault == nil {
		return "", fmt.Errorf("no vault open")
	}

	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("no content to analyze")
	}

	prompt := buildWritingFeedbackPrompt(content)
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
			return "", fmt.Errorf("OpenAI API key not set — configure in Settings")
		}
		model := a.config.OpenAIModel
		if model == "" {
			model = "gpt-4o-mini"
		}
		response, err = doOpenAIRequest(a.config.OpenAIKey, model, prompt)
	default:
		response = localWritingFeedback(content)
	}

	if err != nil {
		// Fall back to local analysis on AI error
		response = localWritingFeedback(content)
	}

	return response, nil
}

func buildWritingFeedbackPrompt(content string) string {
	truncated := content
	if len(truncated) > 4000 {
		truncated = truncated[:4000]
	}

	return fmt.Sprintf(`You are a writing coach. Analyze the following text and provide feedback.

Give your response as a JSON array of objects with these fields:
- "category": one of "clarity", "structure", "style", "grammar", "tone"
- "severity": 1 (minor), 2 (moderate), or 3 (major)
- "issue": brief description of the issue
- "suggestion": actionable recommendation

Also start your response with a JSON object containing:
- "wordCount": number of words
- "sentenceCount": number of sentences
- "paragraphCount": number of paragraphs
- "readabilityScore": estimated readability score 0-100 (higher = easier to read)
- "feedback": the array of feedback items

Return ONLY valid JSON, no markdown formatting.

Text to analyze:
---
%s
---`, truncated)
}

// localWritingFeedback provides basic writing analysis without AI.
func localWritingFeedback(content string) string {
	words := strings.Fields(content)
	wordCount := len(words)

	// Count sentences
	sentenceCount := 0
	for _, ch := range content {
		if ch == '.' || ch == '!' || ch == '?' {
			sentenceCount++
		}
	}
	if sentenceCount == 0 {
		sentenceCount = 1
	}

	// Count paragraphs
	paragraphs := strings.Split(content, "\n\n")
	paragraphCount := 0
	for _, p := range paragraphs {
		if strings.TrimSpace(p) != "" {
			paragraphCount++
		}
	}
	if paragraphCount == 0 {
		paragraphCount = 1
	}

	// Simple readability (avg words per sentence)
	avgWordsPerSentence := float64(wordCount) / float64(sentenceCount)
	readability := 80
	if avgWordsPerSentence > 25 {
		readability = 50
	} else if avgWordsPerSentence > 20 {
		readability = 60
	} else if avgWordsPerSentence > 15 {
		readability = 70
	}

	var feedback []map[string]interface{}

	// Check for long sentences
	sentences := strings.FieldsFunc(content, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	for _, sent := range sentences {
		wc := len(strings.Fields(sent))
		if wc > 40 {
			feedback = append(feedback, map[string]interface{}{
				"category":   "clarity",
				"severity":   2,
				"issue":      fmt.Sprintf("Long sentence with %d words may be hard to follow", wc),
				"suggestion": "Consider breaking this into shorter sentences for better readability.",
			})
			break
		}
	}

	// Check for passive voice patterns
	passiveCount := 0
	for _, w := range []string{"was ", "were ", "been ", "being "} {
		passiveCount += strings.Count(strings.ToLower(content), w)
	}
	if passiveCount > 5 {
		feedback = append(feedback, map[string]interface{}{
			"category":   "style",
			"severity":   1,
			"issue":      fmt.Sprintf("Found %d potential passive voice constructions", passiveCount),
			"suggestion": "Consider using active voice for more direct, engaging writing.",
		})
	}

	// Check for paragraph length
	for i, p := range paragraphs {
		pw := len(strings.Fields(p))
		if pw > 200 {
			feedback = append(feedback, map[string]interface{}{
				"category":   "structure",
				"severity":   2,
				"issue":      fmt.Sprintf("Paragraph %d has %d words", i+1, pw),
				"suggestion": "Break this into smaller paragraphs for better readability.",
			})
			break
		}
	}

	// Check for adverb density
	adverbCount := 0
	for _, w := range words {
		clean := strings.ToLower(strings.Trim(w, ".,;:!?\"'`()[]{}"))
		if strings.HasSuffix(clean, "ly") && len(clean) > 3 {
			switch clean {
			case "only", "early", "family", "holy", "reply", "apply", "supply", "fly":
				continue
			}
			adverbCount++
		}
	}
	if wordCount > 50 && float64(adverbCount)/float64(wordCount) > 0.04 {
		feedback = append(feedback, map[string]interface{}{
			"category":   "style",
			"severity":   1,
			"issue":      fmt.Sprintf("High adverb density: %d adverbs in %d words", adverbCount, wordCount),
			"suggestion": "Replace adverbs with stronger verbs for more impactful writing.",
		})
	}

	// No headings check
	hasHeading := false
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			hasHeading = true
			break
		}
	}
	if !hasHeading && wordCount > 300 {
		feedback = append(feedback, map[string]interface{}{
			"category":   "structure",
			"severity":   2,
			"issue":      "No headings found in a note with over 300 words",
			"suggestion": "Add headings to organize content and improve scannability.",
		})
	}

	if len(feedback) == 0 {
		feedback = append(feedback, map[string]interface{}{
			"category":   "style",
			"severity":   1,
			"issue":      "No significant issues detected",
			"suggestion": "Your writing looks solid. Consider reading it aloud for final polish.",
		})
	}

	result := map[string]interface{}{
		"wordCount":        wordCount,
		"sentenceCount":    sentenceCount,
		"paragraphCount":   paragraphCount,
		"readabilityScore": readability,
		"feedback":         feedback,
	}

	data, _ := json.Marshal(result)
	return string(data)
}
