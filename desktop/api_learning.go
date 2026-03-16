package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ==================== Flashcards ====================

type FlashcardDTO struct {
	Front string `json:"front"`
	Back  string `json:"back"`
	ID    string `json:"id"`
}

type QuizQuestionDTO struct {
	Question string   `json:"question"`
	Choices  []string `json:"choices"`
	Answer   int      `json:"answer"`
	Source   string   `json:"source"`
}

type TimelineEntryDTO struct {
	Date      string   `json:"date"`
	Title     string   `json:"title"`
	RelPath   string   `json:"relPath"`
	Tags      []string `json:"tags"`
	WordCount int      `json:"wordCount"`
}

// flashcardID generates a deterministic ID for a card from its front text and source.
func flashcardID(front, source string) string {
	h := sha256.Sum256([]byte(front + "|" + source))
	return hex.EncodeToString(h[:16])
}

// GetFlashcards parses a note for Q&A pairs and returns them as FlashcardDTOs.
// Supported formats:
//   - Q: / A: pairs (line-based)
//   - ## heading with content below (heading = front, content = back)
//   - Definition lists: `- term :: definition`
//   - Cloze deletions: {{c1::answer}}
func (a *GranitApp) GetFlashcards(notePath string) ([]FlashcardDTO, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}
	note := a.vault.GetNote(notePath)
	if note == nil {
		return nil, fmt.Errorf("note not found: %s", notePath)
	}

	var cards []FlashcardDTO
	lines := strings.Split(note.Content, "\n")

	// Pass 1: explicit Q: / A: pairs
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "Q:") || strings.HasPrefix(trimmed, "q:") {
			question := strings.TrimSpace(trimmed[2:])
			answer := ""
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(next, "A:") || strings.HasPrefix(next, "a:") {
					answer = strings.TrimSpace(next[2:])
					i++
				}
			}
			if question != "" && answer != "" {
				cards = append(cards, FlashcardDTO{
					Front: question,
					Back:  answer,
					ID:    flashcardID(question, notePath),
				})
			}
		}
	}

	// Pass 2: ## heading with content below
	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimSpace(trimmed[3:])
			if heading == "" {
				continue
			}
			var bodyLines []string
			for j := i + 1; j < len(lines); j++ {
				jTrimmed := strings.TrimSpace(lines[j])
				if strings.HasPrefix(jTrimmed, "## ") || strings.HasPrefix(jTrimmed, "# ") {
					break
				}
				bodyLines = append(bodyLines, lines[j])
			}
			body := strings.TrimSpace(strings.Join(bodyLines, "\n"))
			if body != "" {
				cards = append(cards, FlashcardDTO{
					Front: heading,
					Back:  body,
					ID:    flashcardID(heading, notePath),
				})
			}
		}
	}

	// Pass 3: definition lists — `- term :: definition`
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") && strings.Contains(trimmed, " :: ") {
			parts := strings.SplitN(trimmed[2:], " :: ", 2)
			if len(parts) == 2 {
				term := strings.TrimSpace(parts[0])
				def := strings.TrimSpace(parts[1])
				if term != "" && def != "" {
					cards = append(cards, FlashcardDTO{
						Front: term,
						Back:  def,
						ID:    flashcardID(term, notePath),
					})
				}
			}
		}
	}

	// Pass 4: cloze deletions — {{c1::answer}}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "{{c") && strings.Contains(trimmed, "::") && strings.Contains(trimmed, "}}") {
			remaining := trimmed
			for strings.Contains(remaining, "{{c") {
				start := strings.Index(remaining, "{{c")
				if start == -1 {
					break
				}
				end := strings.Index(remaining[start:], "}}")
				if end == -1 {
					break
				}
				end += start

				clozeContent := remaining[start : end+2]
				inner := clozeContent[2 : len(clozeContent)-2]
				colonIdx := strings.Index(inner, "::")
				if colonIdx == -1 {
					remaining = remaining[end+2:]
					continue
				}
				answer := inner[colonIdx+2:]
				if hintIdx := strings.Index(answer, "::"); hintIdx != -1 {
					answer = answer[:hintIdx]
				}

				question := strings.Replace(trimmed, clozeContent, "[...]", 1)

				if answer != "" {
					cards = append(cards, FlashcardDTO{
						Front: question,
						Back:  answer,
						ID:    flashcardID(question, notePath),
					})
				}

				remaining = remaining[end+2:]
			}
		}
	}

	return cards, nil
}

// SaveFlashcardProgress saves review progress data to .granit/flashcards.json.
func (a *GranitApp) SaveFlashcardProgress(data string) error {
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	dir := filepath.Join(a.vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "flashcards.json"), []byte(data), 0644)
}

// GetFlashcardProgress loads the saved flashcard progress from .granit/flashcards.json.
func (a *GranitApp) GetFlashcardProgress() (string, error) {
	if a.vaultRoot == "" {
		return "{}", nil
	}
	p := filepath.Join(a.vaultRoot, ".granit", "flashcards.json")
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return "{}", nil
		}
		return "", err
	}
	return string(data), nil
}

// ==================== Quiz ====================

// GetQuizQuestions generates quiz questions from a note's content.
// It produces multiple-choice, fill-in-the-blank, true/false, and definition questions.
func (a *GranitApp) GetQuizQuestions(notePath string) ([]QuizQuestionDTO, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}
	note := a.vault.GetNote(notePath)
	if note == nil {
		return nil, fmt.Errorf("note not found: %s", notePath)
	}

	lines := strings.Split(note.Content, "\n")
	var questions []QuizQuestionDTO

	// Generate definition questions from :: patterns and headings
	questions = append(questions, quizDefinitionQuestions(notePath, lines)...)

	// Generate fill-in-the-blank from **bold** terms
	questions = append(questions, quizFillBlankQuestions(notePath, lines)...)

	// Generate true/false from statements
	questions = append(questions, quizTrueFalseQuestions(notePath, lines)...)

	// Generate multiple-choice from list items
	questions = append(questions, quizMultipleChoiceQuestions(notePath, lines)...)

	// Shuffle and cap at 20
	rand.Shuffle(len(questions), func(i, j int) {
		questions[i], questions[j] = questions[j], questions[i]
	})
	if len(questions) > 20 {
		questions = questions[:20]
	}

	return questions, nil
}

func quizDefinitionQuestions(source string, lines []string) []QuizQuestionDTO {
	var qs []QuizQuestionDTO

	// term :: definition patterns
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if parts := strings.SplitN(line, "::", 2); len(parts) == 2 {
			term := strings.TrimSpace(parts[0])
			term = strings.TrimPrefix(term, "- ")
			def := strings.TrimSpace(parts[1])
			if term != "" && def != "" && len(def) > 5 {
				qs = append(qs, QuizQuestionDTO{
					Question: fmt.Sprintf("Define: %s", term),
					Choices:  []string{def},
					Answer:   0,
					Source:   source,
				})
			}
		}
	}

	// Heading + first content line
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			heading := strings.TrimLeft(trimmed, "# ")
			heading = strings.TrimSpace(heading)
			if heading == "" {
				continue
			}
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				body := strings.TrimSpace(lines[j])
				if body == "" || strings.HasPrefix(body, "#") {
					continue
				}
				if len(body) > 15 {
					qs = append(qs, QuizQuestionDTO{
						Question: fmt.Sprintf("What is described by: %s?", heading),
						Choices:  []string{body},
						Answer:   0,
						Source:   source,
					})
				}
				break
			}
		}
	}

	return qs
}

func quizFillBlankQuestions(source string, lines []string) []QuizQuestionDTO {
	var qs []QuizQuestionDTO

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "-") {
			continue
		}

		startBold := strings.Index(trimmed, "**")
		if startBold < 0 {
			continue
		}
		after := trimmed[startBold+2:]
		endBold := strings.Index(after, "**")
		if endBold < 0 {
			continue
		}

		term := after[:endBold]
		if len(term) < 2 || len(term) > 60 {
			continue
		}

		blanked := trimmed[:startBold] + "______" + trimmed[startBold+2+endBold+2:]
		blanked = strings.ReplaceAll(blanked, "**", "")

		qs = append(qs, QuizQuestionDTO{
			Question: blanked,
			Choices:  []string{term},
			Answer:   0,
			Source:   source,
		})
	}

	return qs
}

func quizTrueFalseQuestions(source string, lines []string) []QuizQuestionDTO {
	var qs []QuizQuestionDTO

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		clean := strings.ReplaceAll(trimmed, "**", "")
		clean = strings.ReplaceAll(clean, "*", "")
		clean = strings.ReplaceAll(clean, "`", "")
		clean = strings.TrimPrefix(clean, "- ")
		clean = strings.TrimSpace(clean)

		if len(clean) > 20 && len(clean) < 200 {
			if len(qs) >= 5 {
				break
			}
			qs = append(qs, QuizQuestionDTO{
				Question: fmt.Sprintf("True or False: %s", clean),
				Choices:  []string{"True", "False"},
				Answer:   0,
				Source:   source,
			})
		}
	}

	return qs
}

func quizMultipleChoiceQuestions(source string, lines []string) []QuizQuestionDTO {
	var qs []QuizQuestionDTO

	type listGroup struct {
		heading string
		items   []string
	}
	var groups []listGroup
	currentHeading := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") || strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			currentHeading = strings.TrimLeft(trimmed, "# ")
			currentHeading = strings.TrimSpace(currentHeading)
			continue
		}
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
			item := strings.TrimSpace(trimmed[2:])
			item = strings.ReplaceAll(item, "**", "")
			item = strings.ReplaceAll(item, "*", "")
			if item == "" {
				continue
			}
			if len(groups) == 0 || groups[len(groups)-1].heading != currentHeading {
				groups = append(groups, listGroup{heading: currentHeading, items: []string{item}})
			} else {
				groups[len(groups)-1].items = append(groups[len(groups)-1].items, item)
			}
		}
	}

	for _, g := range groups {
		if len(g.items) < 4 {
			continue
		}

		correctIdx := rand.Intn(len(g.items))
		correct := g.items[correctIdx]

		var distractors []string
		for i, it := range g.items {
			if i != correctIdx {
				distractors = append(distractors, it)
			}
		}
		rand.Shuffle(len(distractors), func(i, j int) { distractors[i], distractors[j] = distractors[j], distractors[i] })
		if len(distractors) > 3 {
			distractors = distractors[:3]
		}

		options := append([]string{correct}, distractors...)
		rand.Shuffle(len(options), func(i, j int) { options[i], options[j] = options[j], options[i] })

		// Find the correct answer index after shuffle
		answerIdx := 0
		for i, opt := range options {
			if opt == correct {
				answerIdx = i
				break
			}
		}

		prompt := "Which of the following is listed"
		if g.heading != "" {
			prompt = fmt.Sprintf("Which of the following is listed under \"%s\"", g.heading)
		}

		qs = append(qs, QuizQuestionDTO{
			Question: prompt + "?",
			Choices:  options,
			Answer:   answerIdx,
			Source:   source,
		})
	}

	return qs
}

// ==================== Timeline ====================

// GetTimeline returns all notes sorted by modification date with title, date, tags, and word count.
func (a *GranitApp) GetTimeline() ([]TimelineEntryDTO, error) {
	if a.vault == nil {
		return nil, fmt.Errorf("no vault open")
	}

	var entries []TimelineEntryDTO

	for _, p := range a.vault.SortedPaths() {
		note := a.vault.GetNote(p)
		if note == nil {
			continue
		}

		wordCount := len(strings.Fields(note.Content))

		// Extract tags
		var tags []string
		if t, ok := note.Frontmatter["tags"]; ok {
			tags = extractTagSlice(t)
		}

		// Use frontmatter date if available, fall back to mod time
		noteDate := note.ModTime
		if dateStr, ok := note.Frontmatter["date"]; ok {
			if s, ok := dateStr.(string); ok {
				for _, layout := range []string{
					"2006-01-02",
					"2006-01-02 15:04",
					"2006-01-02T15:04:05Z07:00",
					time.RFC3339,
				} {
					if parsed, err := time.Parse(layout, s); err == nil {
						noteDate = parsed
						break
					}
				}
			}
		}

		entries = append(entries, TimelineEntryDTO{
			Date:      noteDate.Format(time.RFC3339),
			Title:     note.Title,
			RelPath:   note.RelPath,
			Tags:      tags,
			WordCount: wordCount,
		})
	}

	// Sort by date descending (newest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Date > entries[j].Date
	})

	return entries, nil
}

// extractTagSlice converts various tag frontmatter formats to a string slice.
func extractTagSlice(tags interface{}) []string {
	var result []string
	switch v := tags.(type) {
	case []interface{}:
		for _, t := range v {
			if s, ok := t.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					result = append(result, s)
				}
			}
		}
	case []string:
		for _, s := range v {
			s = strings.TrimSpace(s)
			if s != "" {
				result = append(result, s)
			}
		}
	case string:
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				result = append(result, s)
			}
		}
	}
	return result
}

// ==================== Mind Map ====================

var mindMapWikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

type mindMapNodeDTO struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Depth    int               `json:"depth"`
	IsLink   bool              `json:"isLink"`
	Children []*mindMapNodeDTO `json:"children"`
}

// GetMindMapData returns hierarchical link data as JSON for the mind map visualization.
// It builds a tree from the center note outward (2 hops deep).
func (a *GranitApp) GetMindMapData(centerNote string) (string, error) {
	if a.vault == nil {
		return "{}", fmt.Errorf("no vault open")
	}

	if centerNote == "" {
		return "{}", fmt.Errorf("no center note specified")
	}

	note := a.vault.GetNote(centerNote)
	if note == nil {
		return "{}", fmt.Errorf("note not found: %s", centerNote)
	}

	rootName := strings.TrimSuffix(filepath.Base(centerNote), ".md")
	root := &mindMapNodeDTO{
		ID:    centerNote,
		Name:  rootName,
		Depth: 0,
	}

	// Build a map: note name -> list of wikilink targets
	vaultLinks := a.scanVaultWikilinks()

	// Get links from the center note (level 1)
	currentName := strings.TrimSuffix(filepath.Base(centerNote), ".md")
	level1Links := vaultLinks[currentName]

	seen := map[string]bool{currentName: true}

	for _, l1 := range level1Links {
		if seen[l1] {
			continue
		}
		seen[l1] = true

		child := &mindMapNodeDTO{
			ID:     l1,
			Name:   l1,
			Depth:  1,
			IsLink: true,
		}

		// Level 2: links from this linked note
		level2Links := vaultLinks[l1]
		seen2 := map[string]bool{}
		for _, l2 := range level2Links {
			if l2 == currentName || seen2[l2] {
				continue
			}
			seen2[l2] = true
			grandchild := &mindMapNodeDTO{
				ID:     l2,
				Name:   l2,
				Depth:  2,
				IsLink: true,
			}
			child.Children = append(child.Children, grandchild)
		}

		root.Children = append(root.Children, child)
	}

	// Also include backlinks as level-1 nodes
	if a.index != nil {
		backlinks := a.index.GetBacklinks(centerNote)
		for _, bl := range backlinks {
			blName := strings.TrimSuffix(filepath.Base(bl), ".md")
			if seen[blName] {
				continue
			}
			seen[blName] = true
			child := &mindMapNodeDTO{
				ID:     bl,
				Name:   blName,
				Depth:  1,
				IsLink: true,
			}

			// Level 2 links from the backlinked note
			level2Links := vaultLinks[blName]
			seen2 := map[string]bool{}
			for _, l2 := range level2Links {
				if l2 == currentName || seen2[l2] {
					continue
				}
				seen2[l2] = true
				grandchild := &mindMapNodeDTO{
					ID:     l2,
					Name:   l2,
					Depth:  2,
					IsLink: true,
				}
				child.Children = append(child.Children, grandchild)
			}

			root.Children = append(root.Children, child)
		}
	}

	data, err := json.Marshal(root)
	if err != nil {
		return "{}", err
	}
	return string(data), nil
}

// scanVaultWikilinks walks the vault and extracts wikilinks from every .md file.
func (a *GranitApp) scanVaultWikilinks() map[string][]string {
	result := make(map[string][]string)

	_ = filepath.Walk(a.vaultRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") && name != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		noteName := strings.TrimSuffix(filepath.Base(path), ".md")
		matches := mindMapWikilinkRe.FindAllStringSubmatch(string(data), -1)

		var links []string
		seen := map[string]bool{}
		for _, m := range matches {
			target := m[1]
			if idx := strings.Index(target, "|"); idx >= 0 {
				target = target[:idx]
			}
			if idx := strings.Index(target, "#"); idx >= 0 {
				target = target[:idx]
			}
			target = strings.TrimSpace(target)
			if target == "" || seen[target] {
				continue
			}
			seen[target] = true
			links = append(links, target)
		}

		if len(links) > 0 {
			result[noteName] = links
		}
		return nil
	})

	return result
}
